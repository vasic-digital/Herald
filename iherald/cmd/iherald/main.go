// iherald — Incident Herald per spec V3 §18 + §43 (HRD-024 live).
//
// Single-binary CLI for the Incident Herald flavor (credential-leak
// page-out + operator-blocked escalation + §18.8 incident-severity
// routing). HRD-024's paging surface lives only on the HTTP plane, so this
// flavor ships ZERO §43 stub subcommands by design — the stubs package is
// intentionally omitted (no internal/stubs/ directory) to keep main.go
// minimal. The HTTP serve plane exposes POST /v1/webhooks/page LIVE
// (HRD-024) — the PagerDuty/Opsgenie-compatible inbound escalation surface
// — plus the shared healthz/readyz/metrics from commons/cli/.
//
// §107 anti-bluff posture (mirrors cherald + sherald): the serve plane
// refuses to start without a usable JWT verifier (HERALD_AUTH_MODE + the
// matching secret/URL must be set) — but that check, the bindings.Pipeline
// (emitter + store + ladder + audit), and the Redis client are all built
// LAZILY inside the `serve` command's RunE. This is load-bearing for §107
// end-user usability: `iherald version` is designed for bare CI/cron/agent
// invocation and MUST run WITHOUT a JWT secret, a Postgres DSN, or a Redis
// URL. Eagerly building the verifier / pipeline in main() would make even
// `iherald version` die with "build verifier: HERALD_AUTH_MODE must be set",
// which is itself a §107 PASS-bluff: the command "exists" but no operator
// can run it. Building an unauthenticated serve plane is still refused — the
// verifier check fires when `serve` actually runs.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons/cli"
	"github.com/vasic-digital/herald/commons_auth"
	constitution "github.com/vasic-digital/herald/commons_constitution"
	"github.com/vasic-digital/herald/commons_constitution/ladder"
	"github.com/vasic-digital/herald/commons_constitution/state"
	storage "github.com/vasic-digital/herald/commons_storage"
	"github.com/vasic-digital/herald/iherald/internal/bindings"
	flavhttp "github.com/vasic-digital/herald/iherald/internal/http"
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

	branding := commons.DefaultBranding("i", version)

	root := cli.NewRootCmd(branding)
	root.Version = version + " (" + commit + ")"

	root.AddCommand(cli.VersionCmd(branding))
	root.AddCommand(newServeCmd(branding)) // builds pipeline + verifier lazily inside serve

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "iherald:", err)
		os.Exit(1)
	}
}

// newServeCmd builds the iherald `serve` command with LAZY dependency
// construction: the HRD-024 bindings.Pipeline (the escalation detect→emit→
// persist→audit foundation behind POST /v1/webhooks/page), the Redis client,
// and the JWT verifier are all built inside RunE, so `iherald version` runs
// WITHOUT requiring HERALD_AUTH_MODE / a JWT secret / HERALD_PG_DSN (§107
// end-user usability). An anonymous serve plane is still refused: the
// verifier check fires when `serve` actually runs. Flags + listener behavior
// are identical to cli.ServeCmd (shared BindServeFlags + RunServe — the §107
// lockstep-flag-UX contract).
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

			// HRD-024 bindings pipeline: registers iherald's §42.3 escalation
			// rule catalogue + the §18.8 incident-severity row and drives the
			// classify→emit→persist→audit flow behind POST /v1/webhooks/page.
			pipeline, err := buildPipeline(ctx)
			if err != nil {
				return err
			}

			// JWT verifier (Redis optional — JWKS-mode caches keys when
			// present; HMAC ignores rdb). Built here, not in main(), so
			// `iherald version` never requires auth env.
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
				Routes:     flavhttp.Routes(pipeline),
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

// buildPipeline assembles the HRD-024 bindings.Pipeline. The
// ConstitutionStore + ModeLadder + AuditStore mirror cherald's selection:
// Postgres over the HERALD_PG_DSN pool when set (production path), in-memory
// backends otherwise (dev/CI). The emitter publishes onto a fresh in-process
// MemoryBus (M1 substrate) — the escalation events fan out on the
// constitution bus; the external PagerDuty/Opsgenie egress is a separate
// subscriber (HRD-024-paging follow-up, see iherald/internal/page doc). The
// bundle hash is captured from the discovered Constitution.md when readable;
// a missing bundle degrades to the zero hash (a valid "no bundle" sentinel).
func buildPipeline(ctx context.Context) (*bindings.Pipeline, error) {
	store, modeLadder, audit, err := buildBackends(ctx)
	if err != nil {
		return nil, err
	}

	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{})
	emitter, err := constitution.NewEmitter(bus, constitution.EmitterConfig{
		Source: "digital.vasic.herald/iherald",
	})
	if err != nil {
		return nil, fmt.Errorf("build emitter: %w", err)
	}

	var bundle constitution.BundleHash
	if path := os.Getenv("HELIX_CONSTITUTION_PATH"); path != "" {
		if h, cerr := constitution.Capture(path); cerr == nil {
			bundle = h
		}
	}

	return bindings.NewPipeline(bindings.Config{
		Ladder:  modeLadder,
		Store:   store,
		Emitter: emitter,
		Audit:   audit,
		Bundle:  bundle,
	})
}

// buildBackends returns the ConstitutionStore + ModeLadder + AuditStore
// selected by env, all backed by ONE shared pool when Postgres is used.
// Postgres wins when HERALD_PG_DSN is present (the value is parsed via
// storage.ParseDSN — same path used by `pherald migrate`); otherwise the
// in-memory backends are used so dev/CI runs without a database.
func buildBackends(ctx context.Context) (constitution.ConstitutionStore, constitution.ModeLadder, constitution.AuditStore, error) {
	dsn := os.Getenv("HERALD_PG_DSN")
	if dsn == "" {
		return state.NewMemory(), ladder.NewMemory(), state.NewMemoryAudit(), nil
	}
	cfg, err := storage.ParseDSN(dsn)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse HERALD_PG_DSN: %w", err)
	}
	database, err := storage.Open(ctx, cfg)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open pg pool: %w", err)
	}
	return state.NewPostgres(database), ladder.NewPostgres(database), state.NewPostgresAudit(database), nil
}

// buildRedis returns a Redis client when HERALD_REDIS_URL is set, or
// (nil, nil) when it isn't. JWKS-mode verifier accepts a nil rdb and falls
// back to in-memory key caching; HMAC mode ignores rdb entirely.
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
