// Package storage is Herald's L1 storage layer (commons_storage) per
// spec V3 §10. It owns the Postgres connection wrapper, RLS context
// helpers, embedded migrations, and Redis tenant-namespacing client.
//
// Status: SCAFFOLD. Embedded migrations are bundled and the RLS
// tenant-context helper is exported; the actual pgx connection pool +
// River queue + Redis client wiring is HRD-010-follow-up work that
// needs a running Postgres for integration testing (no fakes beyond
// unit tests per Universal §11.4.27).
package storage

import (
	"context"
	"embed"
	"errors"

	"github.com/google/uuid"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// MigrationsFS exposes the embedded migration filesystem so callers
// (golang-migrate driver, doctor command, schema dump) can read the
// canonical SQL without filesystem access.
func MigrationsFS() embed.FS { return migrationsFS }

// SetTenantContext returns the SQL to execute at the start of every
// transaction so PostgreSQL RLS policies key on the right tenant.
//
// Per spec §16, runtime code MUST run this before any SELECT/INSERT
// against multi-tenant tables, otherwise the policies fail-closed
// (returning zero rows / blocking writes).
func SetTenantContext(tenantID uuid.UUID) string {
	return "SET LOCAL app.tenant_id = '" + tenantID.String() + "'"
}

// MigrationDriver is the abstraction layer over golang-migrate so the
// pherald migrate subcommand can run schema changes without depending
// on a specific migration tool from the CLI side.
//
// SCAFFOLD: real implementation pulls in github.com/golang-migrate/migrate
// with the postgres + iofs drivers. Implementation is HRD-010-follow-up.
type MigrationDriver interface {
	Up(ctx context.Context) (applied int, err error)
	Down(ctx context.Context, steps int) (rolledBack int, err error)
	Status(ctx context.Context) (current uint, dirty bool, err error)
	Validate(ctx context.Context) error
}

// NewMigrationDriver returns a MigrationDriver bound to the given
// Postgres DSN. NOT YET IMPLEMENTED (HRD-010).
func NewMigrationDriver(_ string) (MigrationDriver, error) {
	return nil, errors.New("commons_storage: NewMigrationDriver not implemented (HRD-010)")
}
