package storage

import (
	"context"
	"errors"
	"fmt"

	db "digital.vasic.database/pkg/database"
	"digital.vasic.database/pkg/postgres"
	"github.com/google/uuid"
)

// Open returns a connected Postgres client per the §11.4.74 catalogue-check
// pivot: wraps `digital.vasic.database/pkg/postgres` rather than reinventing
// the pgx pool here. The returned `db.Database` is the universal Helix-stack
// abstraction — pkg/migration and the constitution backends consume it
// unchanged.
//
// The caller is responsible for `defer client.Close()`.
//
// Foundation default config: max-conns = (cpu × 4) with floor 4 / ceiling 64,
// statement-cache enabled, application_name = "herald". Overrides via the
// `cfg` argument flow straight through to digital.vasic.database.
func Open(ctx context.Context, cfg *postgres.Config) (db.Database, error) {
	if cfg == nil {
		return nil, errors.New("commons_storage: Open: nil cfg")
	}
	if cfg.ApplicationName == "" {
		cfg.ApplicationName = "herald"
	}
	client := postgres.New(cfg)
	if err := client.Connect(ctx); err != nil {
		return nil, fmt.Errorf("commons_storage: Open: connect: %w", err)
	}
	return client, nil
}

// ConfigForHerald returns a populated postgres.Config for a local-dev
// Postgres reachable on host:port with the given credentials. Wraps
// postgres.DefaultConfig + applies Herald-friendly defaults (application
// name, SSL disabled for local-dev).
//
// Use this from tests + the `pherald migrate` subcommand. Production
// deployments build their own *postgres.Config from a typed config block
// rather than a DSN string.
func ConfigForHerald(host string, port int, user, password, dbName string) *postgres.Config {
	cfg := postgres.DefaultConfig()
	cfg.Driver = "postgres"
	cfg.Host = host
	cfg.Port = port
	cfg.User = user
	cfg.Password = password
	cfg.DBName = dbName
	cfg.SSLMode = "disable"
	cfg.ApplicationName = "herald"
	return cfg
}

// WithTenantContext executes fn inside a transaction with
// `app.tenant_id` GUC pre-set AND with the connection role downgraded to
// `herald_app` so RLS policies actually apply per spec §16 + §44.6.
//
// Contract:
//   - Begins a transaction on database.
//   - Runs `SET LOCAL ROLE herald_app` (drops bootstrap-user superuser
//     privileges; herald_app is NOBYPASSRLS per migration 000001 + has
//     CRUD grants from migration 000008).
//   - Runs `SET LOCAL app.tenant_id = '<uuid>'`.
//   - Invokes fn with the open Tx.
//   - Commits if fn returns nil, rolls back otherwise.
//   - If commit fails, returns the commit error.
//
// Anti-bluff (§11.4.4 + §107 E14): the SET LOCAL ROLE step is load-bearing.
// Without it, `WithTenantContext` is a bluff — calls from the bootstrap
// POSTGRES_USER (typically a superuser, e.g. quickstart's `herald`) would
// bypass RLS entirely regardless of FORCE ROW LEVEL SECURITY, and tests
// asserting tenant isolation would PASS while production was wide open.
// This was discovered 2026-05-20 by the §107 E14 round-trip test in
// commons_storage/storage_integration_test.go.
//
// Idempotent across nested calls: the SET LOCAL has transaction-scope so
// nested transactions (if a future caller adds them) would each set their
// own context.
func WithTenantContext(ctx context.Context, database db.Database, tenantID uuid.UUID, fn func(tx db.Tx) error) error {
	tx, err := database.Begin(ctx)
	if err != nil {
		return fmt.Errorf("commons_storage: WithTenantContext: begin: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	// Drop superuser privileges so RLS policies apply. The bootstrap
	// POSTGRES_USER (the role this connection authenticated as) is
	// typically SUPERUSER which bypasses RLS regardless of FORCE ROW LEVEL
	// SECURITY. herald_app is NOBYPASSRLS per migration 000001 and has
	// SELECT/INSERT/UPDATE/DELETE grants on all multi-tenant tables per
	// migration 000008.
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE herald_app"); err != nil {
		return fmt.Errorf("commons_storage: WithTenantContext: set herald_app role: %w", err)
	}

	// SET LOCAL takes effect only inside the current transaction — perfect
	// for tenant-scoped RLS. We use a parameterised inline string because
	// SET LOCAL does NOT accept query parameters (`SET LOCAL ... = $1`
	// is rejected); UUID format constrained by uuid.UUID's String() method
	// so injection isn't possible.
	guc := "SET LOCAL app.tenant_id = '" + tenantID.String() + "'"
	if _, err := tx.Exec(ctx, guc); err != nil {
		return fmt.Errorf("commons_storage: WithTenantContext: set tenant GUC: %w", err)
	}

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commons_storage: WithTenantContext: commit: %w", err)
	}
	committed = true
	return nil
}
