// sherald — System Herald per spec V3 §18 + §43 (Wave 3a live).
//
// Wave 3a wires:
//   - commons_auth.GinMiddleware for JWT (HMAC or JWKS per env)
//   - safety.Aggregator + safety.StartMemSampler background goroutine
//     (lifecycle bound to a SIGTERM-cancelled context)
//   - sherald/internal/safety.Handler serving GET /v1/safety_state
//
// §43 system/safety commands register via registerSysOps (HRD-033/040/046/056);
// the remaining stubs via sherald/internal/stubs.
//
// §107 anti-bluff posture: the serve plane refuses to start without a usable JWT
// verifier (HERALD_AUTH_MODE + the matching secret/URL must be set) — but that
// check, the safety Aggregator, and the mem-sampler goroutine are all built
// LAZILY inside the `serve` command's RunE (mirroring pherald). This is
// load-bearing for §107 end-user usability: the §43 CLI commands
// (`sherald destructive-guard`, `mem-budget-watch`, …) are designed for bare
// CI/cron/agent invocation and MUST run WITHOUT a JWT secret or a spawned
// background sampler. Eagerly building the verifier in main() (the prior Wave 3a
// wiring) made every subcommand — even `sherald version` — die with "build
// verifier: HERALD_AUTH_MODE must be set", which is itself a §107 PASS-bluff:
// the command "exists" but no operator can run it. The sampler context is bound
// to os.Interrupt + syscall.SIGTERM so the goroutine cleanly exits on shutdown.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons/cli"
	"github.com/vasic-digital/herald/commons_auth"
	flavhttp "github.com/vasic-digital/herald/sherald/internal/http"
	"github.com/vasic-digital/herald/sherald/internal/safety"
	"github.com/vasic-digital/herald/sherald/internal/stubs"
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

	branding := commons.DefaultBranding("s", version)

	root := cli.NewRootCmd(branding)
	root.Version = version + " (" + commit + ")"

	root.AddCommand(cli.VersionCmd(branding))
	root.AddCommand(newServeCmd(branding))
	stubs.Register(root)
	registerSysOps(root)

	if rerr := root.Execute(); rerr != nil {
		fmt.Fprintln(os.Stderr, "sherald:", rerr)
		os.Exit(1)
	}
}

// newServeCmd builds the sherald `serve` command with LAZY dependency
// construction: the safety Aggregator, the mem-sampler goroutine, and the JWT
// verifier are all built inside RunE, so the §43 CLI subcommands run WITHOUT
// requiring HERALD_AUTH_MODE / a JWT secret and WITHOUT spawning a background
// sampler (§107 end-user usability — see the package doc). An anonymous serve
// plane is still refused: the verifier check fires when `serve` actually runs.
// Flags + listener behavior are identical to cli.ServeCmd (shared BindServeFlags
// + RunServe — the §107 lockstep-flag-UX contract).
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

			// Aggregator + sampler goroutine, lifecycle bound to the serve ctx
			// so SIGINT/SIGTERM cleanly terminates the sampler.
			agg := safety.NewAggregator()
			samplerCtx, stopSampler := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
			defer stopSampler()
			safety.StartMemSampler(samplerCtx, agg)

			// JWT verifier (Redis optional — JWKS caches keys when present;
			// HMAC ignores rdb). Built here, not in main(), so CLI subcommands
			// never require auth env.
			var rdb redis.Cmdable
			if url := os.Getenv("HERALD_REDIS_URL"); url != "" {
				opts, err := redis.ParseURL(url)
				if err != nil {
					return fmt.Errorf("parse HERALD_REDIS_URL: %w", err)
				}
				rdb = redis.NewClient(opts)
			}
			verifier, err := commons_auth.NewVerifierFromEnv(rdb)
			if err != nil {
				return fmt.Errorf("build verifier: %w", err)
			}

			opts := cli.ServeOpts{
				Branding:   branding,
				Routes:     flavhttp.Routes(agg),
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
