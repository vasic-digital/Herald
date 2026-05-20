// pherald migrate — apply or inspect commons_storage SQL migrations.
//
// Per HRD-010 step 6 (2026-05-20): replaces the stub previously kept in
// stubs.go. Drives commons_storage.RunMigrations from the CLI so the
// quickstart compose stack + operator workflows have a one-shot way to
// roll schema forward.
//
// Four subcommands are registered:
//   - up:       LIVE — applies pending migrations.
//   - status:   LIVE — reports current schema version.
//   - down:     honest-stub 501 (destructive op per §9.1, future HRD).
//   - validate: honest-stub 501 (schema-drift detection, future HRD).
//
// `down` and `validate` deliberately return helpful "not yet implemented"
// errors pointing operators at docs/Issues.md to open an HRD — better UX
// than Cobra's generic "unknown command" and preserves traceability.
//
// The subcommand REQUIRES the HERALD_PG_DSN environment variable. Per
// constitution §11.4.6 (no-guessing) we do NOT silently default to
// localhost-with-quickstart-defaults — the operator must be explicit
// about which Postgres to mutate. Format:
//
//	HERALD_PG_DSN=postgres://herald:herald_dev@127.0.0.1:24100/herald
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	storage "github.com/vasic-digital/herald/commons_storage"
)

const heraldPGDSNEnvVar = "HERALD_PG_DSN"

// newMigrateCmd wires up `pherald migrate up` and `pherald migrate status`.
// Both subcommands share the env-var requirement + the open/close pool
// dance, but each owns its own RunE so unit tests can target them
// independently.
func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Apply or inspect commons_storage migrations (spec §9.6)",
		Long: `Run Herald's embedded SQL migrations against the configured
Postgres instance. Requires the HERALD_PG_DSN environment variable
(format: postgres://user:pass@host:port/dbname[?sslmode=disable]).

Subcommands:
  up       — apply every pending migration in version order
  status   — report the highest applied migration version
  down     — (not yet implemented) destructive op per §9.1, future HRD
  validate — (not yet implemented) schema-drift detection, future HRD

No silent defaults: HERALD_PG_DSN must be set explicitly.`,
	}
	cmd.AddCommand(newMigrateUpCmd())
	cmd.AddCommand(newMigrateStatusCmd())
	cmd.AddCommand(newMigrateDownCmd())
	cmd.AddCommand(newMigrateValidateCmd())
	return cmd
}

// newMigrateDownCmd returns a deliberate 501-stub. Down-migrations are a
// destructive operation per Constitution §9.1; not exposed via CLI until
// an operator authorises a Herald-side workflow (a future HRD will track
// the design — open an issue in docs/Issues.md before relying on this).
func newMigrateDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down",
		Short: "Roll back the most recent migration (not yet implemented)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("pherald migrate down: not yet implemented — destructive op per §9.1 requires operator authorisation; open an HRD in docs/Issues.md before requesting")
		},
	}
}

// newMigrateValidateCmd returns a deliberate 501-stub. Schema-validation
// (compare applied migrations against embedded set, detect drift) is a
// useful future feature; not yet implemented.
func newMigrateValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Audit applied migrations vs embedded set (not yet implemented)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("pherald migrate validate: not yet implemented — open an HRD in docs/Issues.md to request schema-drift detection")
		},
	}
}

// newMigrateUpCmd applies all pending migrations and reports the count.
func newMigrateUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up",
		Short: "Apply all pending migrations",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dsn := os.Getenv(heraldPGDSNEnvVar)
			if dsn == "" {
				return fmt.Errorf("%s environment variable must be set "+
					"(format: postgres://user:pass@host:port/dbname)", heraldPGDSNEnvVar)
			}
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()
			cfg, err := storage.ParseDSN(dsn)
			if err != nil {
				return fmt.Errorf("parse %s: %w", heraldPGDSNEnvVar, err)
			}
			pool, err := storage.Open(ctx, cfg)
			if err != nil {
				return fmt.Errorf("open pool: %w", err)
			}
			defer func() { _ = pool.Close() }()
			applied, err := storage.RunMigrations(ctx, pool)
			if err != nil {
				return fmt.Errorf("apply migrations: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "applied %d migration(s)\n", len(applied))
			return nil
		},
	}
}

// newMigrateStatusCmd reports the highest migration version applied.
func newMigrateStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Report current schema version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dsn := os.Getenv(heraldPGDSNEnvVar)
			if dsn == "" {
				return fmt.Errorf("%s environment variable must be set "+
					"(format: postgres://user:pass@host:port/dbname)", heraldPGDSNEnvVar)
			}
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()
			cfg, err := storage.ParseDSN(dsn)
			if err != nil {
				return fmt.Errorf("parse %s: %w", heraldPGDSNEnvVar, err)
			}
			pool, err := storage.Open(ctx, cfg)
			if err != nil {
				return fmt.Errorf("open pool: %w", err)
			}
			defer func() { _ = pool.Close() }()
			ver, err := storage.CurrentVersion(ctx, pool)
			if err != nil {
				return fmt.Errorf("current version: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "schema version: %d\n", ver)
			return nil
		},
	}
}
