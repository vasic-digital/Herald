package storage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	db "digital.vasic.database/pkg/database"
	"digital.vasic.database/pkg/migration"
)

// LoadEmbeddedMigrations reads every `NNNNNN_<name>.up.sql` + matching
// `.down.sql` from the embedded migrations/ FS and returns them as
// `[]migration.Migration` ready for `migration.Runner.Up`.
//
// Per §11.4.74 catalogue-check + §44.7 Foundation pivot: this is the
// thin bridge from Herald's `//go:embed migrations/*.sql` layout to
// `digital.vasic.database/pkg/migration`'s in-memory Migration struct.
// The Submodule's migration runner handles all the durable logic
// (tracking table, applied-versions detection, idempotent re-run).
//
// Returns migrations sorted ascending by version. Skips down.sql files
// that have no matching up.sql.
func LoadEmbeddedMigrations() ([]migration.Migration, error) {
	type entry struct {
		version int
		name    string
		up      string
		down    string
	}
	byVersion := make(map[int]*entry)

	err := fs.WalkDir(migrationsFS, "migrations", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".sql") {
			return nil
		}
		// Parse `NNNNNN_<descr>.{up,down}.sql`.
		stem := strings.TrimSuffix(name, ".sql")
		variant := ""
		if strings.HasSuffix(stem, ".up") {
			variant = "up"
			stem = strings.TrimSuffix(stem, ".up")
		} else if strings.HasSuffix(stem, ".down") {
			variant = "down"
			stem = strings.TrimSuffix(stem, ".down")
		} else {
			return fmt.Errorf("commons_storage: migration name %q lacks .up/.down suffix", name)
		}
		under := strings.IndexByte(stem, '_')
		if under <= 0 {
			return fmt.Errorf("commons_storage: migration name %q lacks NNN_<name> prefix", name)
		}
		version, err := strconv.Atoi(stem[:under])
		if err != nil {
			return fmt.Errorf("commons_storage: migration %q version parse: %w", name, err)
		}
		body, err := migrationsFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("commons_storage: read migration %q: %w", name, err)
		}
		e, ok := byVersion[version]
		if !ok {
			e = &entry{version: version, name: stem[under+1:]}
			byVersion[version] = e
		}
		if variant == "up" {
			e.up = string(body)
		} else {
			e.down = string(body)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	out := make([]migration.Migration, 0, len(byVersion))
	for _, e := range byVersion {
		if e.up == "" {
			// Down without up = skip (warning, not fatal).
			continue
		}
		out = append(out, migration.Migration{
			Version: e.version,
			Name:    e.name,
			Up:      e.up,
			Down:    e.down,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// RunMigrations applies all embedded migrations against database. Returns
// the list of versions applied THIS RUN (already-applied versions are
// skipped). Tracking table: "schema_migrations".
//
// IMPORTANT: this is a Postgres-native re-implementation of the runner
// loop because `digital.vasic.database/pkg/migration` v0 uses `?`
// placeholders in its INSERT (a SQLite/MySQL convention) which pgx
// rejects. Tracked as HRD-082: extend the Submodule's migration package
// to use pg-native `$N` placeholders OR run via pkg/connection's
// placeholder rewriter. Once HRD-082 lands, this re-implementation
// collapses back into a one-line `runner.Apply()`.
func RunMigrations(ctx context.Context, database db.Database) ([]int, error) {
	migrations, err := LoadEmbeddedMigrations()
	if err != nil {
		return nil, fmt.Errorf("commons_storage: RunMigrations: load: %w", err)
	}
	// Init the tracking table (this uses no parameters → upstream Init is safe).
	runner := migration.NewRunner(database, "schema_migrations")
	if err := runner.Init(ctx); err != nil {
		return nil, fmt.Errorf("commons_storage: RunMigrations: init: %w", err)
	}
	applied, err := runner.Applied(ctx)
	if err != nil {
		return nil, fmt.Errorf("commons_storage: RunMigrations: applied: %w", err)
	}
	appliedSet := make(map[int]bool, len(applied))
	for _, v := range applied {
		appliedSet[v] = true
	}

	newly := []int{}
	for _, m := range migrations {
		if appliedSet[m.Version] {
			continue
		}
		if err := applyMigration(ctx, database, m); err != nil {
			return newly, fmt.Errorf("commons_storage: apply v%d (%s): %w", m.Version, m.Name, err)
		}
		newly = append(newly, m.Version)
	}
	return newly, nil
}

// applyMigration applies a single migration in its own transaction +
// records it in schema_migrations using pg-native placeholders.
func applyMigration(ctx context.Context, database db.Database, m migration.Migration) error {
	tx, err := database.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()
	if _, err := tx.Exec(ctx, m.Up); err != nil {
		return fmt.Errorf("exec up: %w", err)
	}
	if _, err := tx.Exec(ctx,
		"INSERT INTO schema_migrations (version, name, applied_at) VALUES ($1, $2, $3)",
		m.Version, m.Name, time.Now().UTC(),
	); err != nil {
		return fmt.Errorf("record: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	committed = true
	return nil
}

// ErrMigrationBundleEmpty signals no migrations were found (probably a
// build-tag misconfiguration).
var ErrMigrationBundleEmpty = errors.New("commons_storage: migration bundle is empty")
