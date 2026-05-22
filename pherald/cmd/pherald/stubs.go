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
//
// Wave 4a Task 7.5 refactor (2026-05-22): newServeCmd now delegates the
// listener body to cli.RunServe so pherald gets the EXACT same dual
// TCP/H2 + UDP/H3 + Brotli + Alt-Svc engine as every other flavor. The
// lazy Runner construction is preserved — buildRunner/buildVerifier are
// still called INSIDE RunE, so `pherald version` / `pherald migrate`
// remain runnable without PG/JWT env vars. This closes the §107 covenant
// gap that left POST /v1/events TCP-only after Wave 4a T6/T7.
package main

import (
	"context"
	"log/slog"

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
// Wave 4a Task 7.5 (pherald unification onto cli.RunServe): the
// dual-listener engine (TCP/H2 + UDP/H3 + Brotli + Alt-Svc) is the
// shared cli.RunServe body — pherald no longer rolls its own
// http.Server. The lazy Runner/verifier construction is preserved: we
// build them here in RunE, plug them into ServeOpts, and hand off to
// cli.RunServe for the listener lifecycle. Result: POST /v1/events
// gets HTTP/3 + Brotli + Alt-Svc exactly like every other route.
//
// Routes:
//   - POST /v1/events → httpsrv.EventsHandler(runner) [LIVE]
//   - GET  /v1/healthz → cli.HealthzHandler               [built-in]
//   - GET  /v1/readyz  → cli.ReadyzHandler                [built-in]
//   - GET  /metrics   → cli.MetricsHandler                [built-in]
//
// Middleware stack (in order, downstream of cli.RunServe's auto-wired
// Brotli + Alt-Svc layers):
//  1. cli.TOONMiddleware()                 — transcodes c.JSON → TOON when
//                                            client Accept prefers TOON
//                                            (Wave 4b T5)
//  2. commons_auth.GinMiddleware(verifier) — 401 on missing/invalid JWT
//  3. httpsrv.RequestIDMiddleware()        — propagates X-Request-ID
//
// TOONMiddleware sits BEFORE auth so the Accept-negotiation buffer wraps
// the writer for the entire downstream chain (auth-failure responses also
// get codec-correct encoding). The auth + RequestID middlewares observe
// the wrapped writer transparently — both call c.JSON which lands in
// TOONMiddleware's response buffer.
func newServeCmd(br commons.Branding) *cobra.Command {
	var (
		port     int
		tlsCert  string
		tlsKey   string
		noBrotli bool
	)
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the " + br.DisplayName + " HTTP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			// Build the Runner and verifier here, NOT at process start.
			// `pherald version` / `pherald migrate` / `pherald --help`
			// must remain runnable without HERALD_PG_DSN / HERALD_AUTH_MODE.
			runnerInstance, pgCloser := buildRunner(ctx)
			defer pgCloser()
			verifier := buildVerifier()

			opts := cli.ServeOpts{
				Branding: br,
				Routes:   httpsrv.Routes(runnerInstance),
				Middleware: []gin.HandlerFunc{
					cli.TOONMiddleware(), // Wave 4b T5: TOON↔JSON content negotiation
					commons_auth.GinMiddleware(verifier),
					httpsrv.RequestIDMiddleware(),
				},
				TLSCertPath:   tlsCert,
				TLSKeyPath:    tlsKey,
				DisableBrotli: noBrotli,
				// ProdMode + DisableH3 are env-detected inside cli.RunServe
				// (HERALD_AUTH_MODE=jwks ⇒ ProdMode, HERALD_DISABLE_HTTP3=1
				// ⇒ DisableH3). Keeping them out of main.go matches the
				// Wave 4a T6/T7 contract every other flavor uses.
			}
			return cli.RunServe(ctx, opts, port)
		},
	}
	cli.BindServeFlags(cmd, &port, &tlsCert, &tlsKey, &noBrotli)
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
