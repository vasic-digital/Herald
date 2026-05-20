// pherald migrate — apply or inspect commons_storage SQL migrations.
//
// Per HRD-010 step 6 (2026-05-20): replaces the stub previously kept in
// stubs.go. Drives commons_storage.RunMigrations from the CLI so the
// quickstart compose stack + operator workflows have a one-shot way to
// roll schema forward. No `down` subcommand — destructive rollback is
// out-of-scope until the operator authorises it (HRD-NNN, future).
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
  up     — apply every pending migration in version order
  status — report the highest applied migration version

No silent defaults: HERALD_PG_DSN must be set explicitly.`,
	}
	cmd.AddCommand(newMigrateUpCmd())
	cmd.AddCommand(newMigrateStatusCmd())
	return cmd
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
