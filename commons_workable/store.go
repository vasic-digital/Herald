// Package workable mirrors ATMOSphere's workable-items SQLite SSoT so
// that Herald and ATMOSphere can share one database file. It exposes a
// Store (schema-applying connection holder), a CRUD repo over the
// `items` table, a per-property change feed, and a tolerant parser for
// ATMOSphere's real Markdown tracker format.
package workable

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, registered as "sqlite"
)

// schemaDDL is the canonical ATMOSphere workable-items schema, mirrored
// verbatim so a DB created by either project is interchangeable. Every
// statement is idempotent (CREATE TABLE IF NOT EXISTS).
const schemaDDL = `
CREATE TABLE IF NOT EXISTS items (
    atm_id           TEXT NOT NULL,
    type             TEXT CHECK (type IN ('Bug','Feature','Task')),
    status           TEXT,
    severity         TEXT,
    title            TEXT,
    description      TEXT,
    forensic_anchor  TEXT,
    closure_criteria TEXT,
    composes_with    TEXT,
    current_location TEXT CHECK (current_location IN ('Issues','Fixed')) DEFAULT 'Issues',
    body_md          TEXT,
    created_at       TEXT,
    last_modified    TEXT,
    created_by       TEXT NOT NULL DEFAULT '',
    assigned_to      TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (atm_id, current_location)
);

CREATE TABLE IF NOT EXISTS item_history (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    atm_id        TEXT,
    event_type    TEXT CHECK (event_type IN ('Opened','Updated','Reopened','Fixed','Implemented','Completed','Obsolete')),
    by            TEXT CHECK (by IN ('AI','User')),
    on_date       TEXT,
    reason        TEXT,
    evidence_path TEXT,
    created_at    TEXT
);

CREATE TABLE IF NOT EXISTS meta (
    key           TEXT PRIMARY KEY,
    value         TEXT,
    last_modified TEXT
);
`

// Store holds an open connection to the workable-items SQLite DB with
// the canonical schema applied and the mandated PRAGMAs (WAL +
// foreign_keys=ON) active.
type Store struct {
	db *sql.DB
}

// Open opens (creating if absent) the SQLite DB at path, enables WAL +
// foreign keys, and applies the canonical schema idempotently.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("workable: open %q: %w", path, err)
	}

	// modernc.org/sqlite multiplexes a pool; pin to a single connection
	// so connection-scoped PRAGMAs (journal_mode/foreign_keys) hold for
	// every query the Store issues.
	db.SetMaxOpenConns(1)

	for _, pragma := range []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA foreign_keys=ON`,
	} {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("workable: %s: %w", pragma, err)
		}
	}

	if _, err := db.Exec(schemaDDL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("workable: apply schema: %w", err)
	}

	// Forward-migrate an EXISTING DB created under the pre-attribution
	// schema: CREATE TABLE IF NOT EXISTS above is a no-op when `items`
	// already exists, so the new columns must be added in-place. Each ADD
	// COLUMN is guarded by a pragma_table_info presence check so the
	// migration is idempotent and never errors on an already-current DB.
	// No data is lost — ADD COLUMN with a DEFAULT backfills existing rows.
	if err := migrateAddColumns(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("workable: migrate items: %w", err)
	}

	return &Store{db: db}, nil
}

// migrateAddColumns adds the attribution columns (created_by, assigned_to)
// to a legacy `items` table that predates them. It is idempotent: a column
// already present is skipped via a pragma_table_info() lookup.
func migrateAddColumns(db *sql.DB) error {
	type col struct{ name, ddl string }
	wanted := []col{
		{"created_by", `ALTER TABLE items ADD COLUMN created_by TEXT NOT NULL DEFAULT ''`},
		{"assigned_to", `ALTER TABLE items ADD COLUMN assigned_to TEXT NOT NULL DEFAULT ''`},
	}
	for _, c := range wanted {
		ok, err := columnExists(db, "items", c.name)
		if err != nil {
			return err
		}
		if ok {
			continue
		}
		if _, err := db.Exec(c.ddl); err != nil {
			return fmt.Errorf("add column %s: %w", c.name, err)
		}
	}
	return nil
}

// columnExists reports whether `table` has a column named `name`.
func columnExists(db *sql.DB, table, name string) (bool, error) {
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name=?`, table, name).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("pragma_table_info(%s): %w", table, err)
	}
	return n > 0, nil
}

// DB exposes the underlying *sql.DB for callers that need raw access
// (history inserts, meta reads, tests).
func (s *Store) DB() *sql.DB { return s.db }

// Close releases the underlying connection pool.
func (s *Store) Close() error { return s.db.Close() }
