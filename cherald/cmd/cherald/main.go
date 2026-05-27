// cherald — Constitution Herald per spec V3 §18 + §43 (Wave 3a live).
//
// Wave 3a wires:
//   - commons_auth.GinMiddleware for JWT (HMAC or JWKS per env)
//   - commons_constitution.ConstitutionStore (Postgres via state.NewPostgres
//     when HERALD_PG_DSN is set, Memory fallback otherwise)
//   - cherald/internal/compliance.Handler serving GET /v1/compliance
//   - cherald/internal/modes admin surface (/v1/compliance/modes) — HRD-027
//
// §43 commands register via:
//   - registerDocsOps (HRD-037/039/048/050/052 — the docs-pipeline command
//     bodies that PRODUCE the Subjects the HRD-019 cherald bindings classify)
//   - cherald/internal/stubs (the remaining §43 verify/check stubs — cluster
//     C3b: HRD-036/038/042/051/054/055).
//
// §107 anti-bluff posture: the serve plane refuses to start without a usable
// JWT verifier (HERALD_AUTH_MODE + the matching secret/URL must be set) — but
// that check, the ConstitutionStore + ModeLadder + bindings pipeline + Redis
// client are all built LAZILY inside the `serve` command's RunE (mirroring
// pherald + sherald). This is load-bearing for §107 end-user usability: the
// §43 CLI commands (`cherald docs-sync`, `cherald fixed-align`, …) plus
// `cherald version` are designed for bare CI/cron/agent invocation and MUST
// run WITHOUT a JWT secret, a Postgres DSN, or a Redis URL. Eagerly building
// the verifier / store / pipeline in main() (the prior Wave 3a wiring) made
// every subcommand — even `cherald version` — die with "build verifier:
// HERALD_AUTH_MODE must be set", which is itself a §107 PASS-bluff: the
// command "exists" but no operator can run it. Building an unauthenticated
// serve plane is still refused — the verifier check fires when `serve` runs.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/cherald/internal/bindings"
	flavhttp "github.com/vasic-digital/herald/cherald/internal/http"
	"github.com/vasic-digital/herald/cherald/internal/stubs"
	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons/cli"
	"github.com/vasic-digital/herald/commons_auth"
	constitution "github.com/vasic-digital/herald/commons_constitution"
	"github.com/vasic-digital/herald/commons_constitution/ladder"
	"github.com/vasic-digital/herald/commons_constitution/state"
	storage "github.com/vasic-digital/herald/commons_storage"
)

// version is overridden at build time:
//
//	go build -ldflags "-X main.version=$(git describe --tags)"
var version = "0.0.0-dev"

// commit is overridden at build time:
//
//	go build -ldflags "-X main.commit=$(git rev-parse --short HEAD)"
var commit = "unknown"

func main() {
	cli.BuildVersion = version
	cli.BuildCommit = commit

	branding := commons.DefaultBranding("c", version)

	root := cli.NewRootCmd(branding)
	root.Version = version + " (" + commit + ")"

	root.AddCommand(cli.VersionCmd(branding))
	root.AddCommand(newServeCmd(branding)) // builds store/ladder/pipeline/verifier lazily inside serve
	stubs.Register(root)                   // remaining §43 stubs (now empty — cluster C3b landed)
	registerDocsOps(root)                  // v1.0.0 Batch C C3a: §43 docs-pipeline command bodies (HRD-037/039/048/050/052)
	registerCheckOps(root)                 // v1.0.0 Batch C C3b: §43 verify/check command bodies (HRD-036/038/042/051/054/055)

	if rerr := root.Execute(); rerr != nil {
		fmt.Fprintln(os.Stderr, "cherald:", rerr)
		os.Exit(1)
	}
}

// newServeCmd builds the cherald `serve` command with LAZY dependency
// construction: the ConstitutionStore + ModeLadder, the HRD-019 bindings
// pipeline, the Redis client, and the JWT verifier are all built inside RunE,
// so the §43 CLI subcommands run WITHOUT requiring HERALD_AUTH_MODE / a JWT
// secret / HERALD_PG_DSN (§107 end-user usability — see the package doc). An
// anonymous serve plane is still refused: the verifier check fires when
// `serve` actually runs. Flags + listener behavior are identical to
// cli.ServeCmd (shared BindServeFlags + RunServe — the §107 lockstep-flag-UX
// contract).
func newServeCmd(branding commons.Branding) *cobra.Command {
	var (
		port     int
		tlsCert  string
		tlsKey   string
		noBrotli bool
	)
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the " + branding.DisplayName + " HTTP server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			// ConstitutionStore + ModeLadder. Postgres when HERALD_PG_DSN is
			// present (production path), Memory otherwise (dev/CI). Both back-ends
			// satisfy their interfaces identically; the compliance + modes
			// handlers don't care which is wired. Built here, not in main(), so
			// CLI subcommands never require a database.
			store, modeLadder, err := buildStoreAndLadder(ctx)
			if err != nil {
				return err
			}

			// HRD-019 bindings pipeline: registers cherald's §42.3 rule catalogue
			// and drives the detect→emit→persist→audit flow behind
			// POST /v1/compliance/evaluate.
			pipeline, err := buildPipeline(modeLadder, store)
			if err != nil {
				return err
			}

			// JWT verifier (Redis optional — JWKS-mode caches keys when present;
			// HMAC ignores rdb). Built here, not in main(), so CLI subcommands
			// never require auth env.
			rdb, err := buildRedis()
			if err != nil {
				return err
			}
			verifier, err := commons_auth.NewVerifierFromEnv(rdb)
			if err != nil {
				return fmt.Errorf("build verifier: %w", err)
			}

			opts := cli.ServeOpts{
				Branding:   branding,
				Routes:     flavhttp.Routes(store, modeLadder, pipeline),
				Middleware: []gin.HandlerFunc{commons_auth.GinMiddleware(verifier)},
			}
			if tlsCert != "" {
				opts.TLSCertPath = tlsCert
			}
			if tlsKey != "" {
				opts.TLSKeyPath = tlsKey
			}
			if noBrotli {
				opts.DisableBrotli = true
			}
			return cli.RunServe(ctx, opts, port)
		},
	}
	cli.BindServeFlags(cmd, &port, &tlsCert, &tlsKey, &noBrotli)
	return cmd
}

// buildStoreAndLadder returns the ConstitutionStore + ModeLadder selected by
// env, both backed by ONE shared pool. Postgres wins when HERALD_PG_DSN is
// present (the value is parsed via storage.ParseDSN — same path used by
// `pherald migrate`); otherwise the in-memory backends are used so dev/CI
// runs without a database.
//
// commons_storage.Open returns a digital.vasic.database.Database, which is
// what both state.NewPostgres + ladder.NewPostgres expect — so no adapter
// shim is needed and the single pool serves /v1/compliance (state) and
// /v1/compliance/modes (ladder) alike.
func buildStoreAndLadder(ctx context.Context) (constitution.ConstitutionStore, constitution.ModeLadder, error) {
	dsn := os.Getenv("HERALD_PG_DSN")
	if dsn == "" {
		return state.NewMemory(), ladder.NewMemory(), nil
	}
	cfg, err := storage.ParseDSN(dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("parse HERALD_PG_DSN: %w", err)
	}
	database, err := storage.Open(ctx, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("open pg pool: %w", err)
	}
	return state.NewPostgres(database), ladder.NewPostgres(database), nil
}

// buildPipeline assembles the HRD-019 bindings.Pipeline. The emitter publishes
// onto a fresh in-process MemoryBus (M1 substrate). The AuditStore mirrors the
// store backend: PostgresAudit over the HERALD_PG_DSN pool when set, MemoryAudit
// otherwise. The bundle hash is captured from the discovered Constitution.md
// (HELIX_CONSTITUTION_PATH or the conventional sibling path) when readable; a
// missing bundle degrades to the zero hash (a valid "no bundle" sentinel) so
// dev runs without a constitution checkout still serve the surface.
func buildPipeline(ladderImpl constitution.ModeLadder, store constitution.ConstitutionStore) (*bindings.Pipeline, error) {
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{})
	emitter, err := constitution.NewEmitter(bus, constitution.EmitterConfig{
		Source: "digital.vasic.herald/cherald",
	})
	if err != nil {
		return nil, fmt.Errorf("build emitter: %w", err)
	}

	audit, err := buildAudit(context.Background())
	if err != nil {
		return nil, fmt.Errorf("build audit: %w", err)
	}

	var bundle constitution.BundleHash
	if path := os.Getenv("HELIX_CONSTITUTION_PATH"); path != "" {
		if h, cerr := constitution.Capture(path); cerr == nil {
			bundle = h
		}
	}

	return bindings.NewPipeline(bindings.Config{
		Ladder:  ladderImpl,
		Store:   store,
		Emitter: emitter,
		Audit:   audit,
		Bundle:  bundle,
	})
}

// buildAudit returns the AuditStore matching the store backend selection:
// PostgresAudit over the HERALD_PG_DSN pool when set, MemoryAudit otherwise.
func buildAudit(ctx context.Context) (constitution.AuditStore, error) {
	dsn := os.Getenv("HERALD_PG_DSN")
	if dsn == "" {
		return state.NewMemoryAudit(), nil
	}
	cfg, err := storage.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse HERALD_PG_DSN: %w", err)
	}
	database, err := storage.Open(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open pg pool (audit): %w", err)
	}
	return state.NewPostgresAudit(database), nil
}

// buildRedis returns a Redis client when HERALD_REDIS_URL is set, or
// (nil, nil) when it isn't. JWKS-mode verifier accepts a nil rdb and
// falls back to in-memory key caching; HMAC mode ignores rdb entirely.
func buildRedis() (redis.Cmdable, error) {
	url := os.Getenv("HERALD_REDIS_URL")
	if url == "" {
		return nil, nil
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse HERALD_REDIS_URL: %w", err)
	}
	return redis.NewClient(opts), nil
}
