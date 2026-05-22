// §43 GitOps stub commands for pherald.
//
// Wave 2 r1 refactor (2026-05-21): replaced the previous one-off
// newSendCmd / newDoctorCmd / newSubscriberCmd / newDeadletterCmd
// stubs (none of which had real bodies anyway) with the §43 mandate
// commands that pherald actually owns per spec V3 §43. Each is
// registered as a cli.StubCmd that returns a 501-style error with an
// HRD pointer so the operator knows where the implementation is tracked.
//
// Wave 2 Task 6 refactor (2026-05-21): newServeCmd now delegates to the
// shared cli.ServeCmd scaffold.
//
// Wave 3b Task 10 refactor (2026-05-22): newServeCmd LAZILY builds the
// Runner + JWT verifier when serve actually runs (not at process start).
// This keeps `pherald --help`, `pherald version`, `pherald migrate ...`
// runnable even when HERALD_PG_DSN / HERALD_AUTH_MODE aren't set. The
// serve subcommand is the only path that requires PG + auth wired up.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons/cli"
	"github.com/vasic-digital/herald/commons_auth"
	constitution "github.com/vasic-digital/herald/commons_constitution"
	httpsrv "github.com/vasic-digital/herald/pherald/internal/http"
	"github.com/vasic-digital/herald/pherald/internal/runner"
)

// registerStubs adds every §43 GitOps command targeted at pherald as a
// 501-stub. HRD pointers track implementation status.
func registerStubs(root *cobra.Command) {
	root.AddCommand(cli.StubCmd("commit-push", "HRD-029", "Single-entrypoint locked commit + multi-mirror push (§2)"))
	root.AddCommand(cli.StubCmd("submodule-propagate", "HRD-030", "Owned-submodule walk in propagation order (§3)"))
	root.AddCommand(cli.StubCmd("install-upstreams", "HRD-043", "install_upstreams wrapper (§11.4.36)"))
	root.AddCommand(cli.StubCmd("fetch-guard", "HRD-044", "Pre-edit fetch + rebase enforcement (§11.4.37)"))
	root.AddCommand(cli.StubCmd("reopen", "HRD-049", "Issues→Fixed reversal + Reopens history (§11.4.55)"))
	root.AddCommand(cli.StubCmd("pre-push", "HRD-053", "Fetch + investigate + integrate hook (§11.4.71)"))
}

// newServeCmd returns pherald's `serve` subcommand. Wave 3b Task 10:
// builds the §32 7-stage Runner + JWT verifier INSIDE RunE so the
// HERALD_PG_DSN / HERALD_AUTH_MODE requirements only fire when serve
// actually runs (not at process start). `pherald --help`, `version`,
// `migrate` paths don't need PG/auth wired up.
//
// Routes:
//   - POST /v1/events → httpsrv.EventsHandler(runner) [LIVE]
//   - GET  /v1/healthz → cli.HealthzHandler               [built-in]
//   - GET  /v1/readyz  → cli.ReadyzHandler                [built-in]
//   - GET  /metrics   → cli.MetricsHandler                [built-in]
//
// Middleware stack (in order):
//  1. commons_auth.GinMiddleware(verifier) — 401 on missing/invalid JWT
//  2. httpsrv.RequestIDMiddleware()        — propagates X-Request-ID
func newServeCmd(br commons.Branding) *cobra.Command {
	var port int
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the " + br.DisplayName + " HTTP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			// Build the Runner and verifier here, NOT at process start.
			runnerInstance, pgCloser := buildRunner(ctx)
			defer pgCloser()
			verifier := buildVerifier()

			if port == 0 {
				port = br.DefaultPort
			}
			gin.SetMode(gin.ReleaseMode)
			r := gin.New()
			r.Use(gin.Recovery())
			// Health/observability endpoints registered BEFORE the auth
			// chain so probes don't need a JWT.
			r.GET("/v1/healthz", cli.HealthzHandler(br))
			r.GET("/v1/readyz", cli.ReadyzHandler(br))
			r.GET("/metrics", cli.MetricsHandler(br))
			// Auth + request-ID middleware for everything below.
			r.Use(commons_auth.GinMiddleware(verifier))
			r.Use(httpsrv.RequestIDMiddleware())
			// Flavor-specific routes (POST /v1/events live).
			for _, route := range httpsrv.Routes(runnerInstance) {
				h := route.Handler
				if h == nil && route.HRD != "" {
					h = cli.StubRouteHandler(route)
				}
				r.Handle(route.Method, route.Path, h)
			}
			srv := &http.Server{
				Addr:    ":" + strconv.Itoa(port),
				Handler: r,
			}
			errCh := make(chan error, 1)
			go func() {
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					errCh <- err
				}
			}()
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			defer signal.Stop(sigCh)
			select {
			case err := <-errCh:
				return err
			case <-sigCh:
			case <-ctx.Done():
			}
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return srv.Shutdown(shutdownCtx)
		},
	}
	cmd.Flags().IntVar(&port, "http-port", 0, "TCP port to bind (default = flavor's DefaultPort)")
	return cmd
}

// buildRunner assembles the §32 Runner with all real adapters. The
// returned closer flushes the pgxpool on graceful shutdown.
func buildRunner(ctx context.Context) (*runner.Runner, func()) {
	pgPool, pgCloser := mustOpenPGPool(ctx)
	rdb := buildRedisClient()
	r := runner.NewRunner(runner.Deps{
		PG:        pgPool,
		Redis:     rdb,
		Evaluator: constitution.NewRegistry(), // Wave 3b ships permissive — flavor binaries that need policy enforcement register evaluators
		Channels:  buildChannelRegistry(),
		Logger:    slog.Default(),
	})
	return r, pgCloser
}
