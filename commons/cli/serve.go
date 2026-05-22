// Wave 4a Task 6 — dual-listener (TCP/H2 + UDP/H3) serve scaffold.
//
// Before T6, ServeCmd bound a single TCP listener via http.Server.
// After T6, a single cli.ServeCmd(opts) call binds BOTH a TCP/H2 listener
// AND a UDP/H3 listener on the same port, sharing the same Gin engine.
// Both listeners serve identical routes/handlers. Either listener's
// failure does NOT crash the other — the orchestration logs + degrades
// (TCP-only mode if H3 fails to bind; H3-only is not a supported
// degradation since TCP is the §107 baseline every client falls back to).
//
// Resolution policy:
//
//   - opts.DisableH3 == true (HERALD_DISABLE_HTTP3=1 escape hatch for
//     UDP-blocked environments): ONLY the TCP listener binds. The
//     auto-injected Alt-Svc middleware becomes a no-op (port 0 disables
//     advertisement). TLS becomes optional — if no cert is supplied, the
//     listener runs plaintext HTTP/1.1+H2c, matching pre-T6 behavior. This
//     is the backward-compatibility branch existing flavors / tests rely
//     on.
//
//   - opts.DisableH3 == false (default): BOTH listeners bind. A TLS cert
//     is REQUIRED — HTTP/3 mandates TLS 1.3 on the wire, and the TCP
//     listener piggybacks on the same cert so both protocols speak HTTPS
//     on the same port. commons_tls.ResolveCertSource handles the policy:
//     production mode fails-loud when no explicit cert is supplied; dev
//     mode auto-generates a self-signed cert at ~/.herald/dev-{cert,key}.pem.
//
// Per §107: a "dual-listener configured" PASS without observing both
// listeners actually bind + actually shut down is a §11.4 bluff. The
// integration tests below dial both listeners, drive a real round-trip,
// and post-shutdown probe BOTH the TCP port (expect connection refused)
// AND the UDP port (expect a successful re-bind by an unrelated
// listener — proves the QUIC stack released the port).

package cli

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/commons"
	commons_tls "github.com/vasic-digital/herald/commons_tls"
)

// ServeOpts configures ServeCmd.
//
// Required:
//
//	Branding   — drives the default port + healthz/metrics gauge name.
//
// Optional flavor-specific:
//
//	Routes     — flavor-specific routes appended after healthz/readyz/metrics.
//	Middleware — flavor-specific Gin middleware chain registered immediately
//	             after gin.Recovery() + the auto-wired Brotli/Alt-Svc layers
//	             + before any handler. Typical use is request-ID
//	             propagation, tenant extraction, OTel tracing.
//
// Wave 4a TLS / HTTP/3 controls (zero-value defaults preserve pre-T6
// behavior so existing flavor binaries are not broken by the dual-
// listener refactor):
//
//	TLSCertPath    — operator-supplied --tls-cert path (forwarded to
//	                 commons_tls.ResolveCertSource).
//	TLSKeyPath     — operator-supplied --tls-key path.
//	ProdMode       — true ⇒ ResolveCertSource refuses dev-autogen. The
//	                 caller's convention: set this to
//	                 (os.Getenv("HERALD_AUTH_MODE") == "jwks").
//	DisableH3      — true ⇒ HTTP/3 listener is NOT started, the Alt-Svc
//	                 middleware becomes a no-op, and the TCP listener
//	                 falls back to plaintext (or TLS if a cert was
//	                 supplied explicitly). Driven by
//	                 HERALD_DISABLE_HTTP3=1 in the caller.
//	DisableBrotli  — true ⇒ Brotli middleware is NOT auto-wired. Useful
//	                 for routes that stream + cannot be buffered.
type ServeOpts struct {
	Branding      commons.Branding
	Routes        []Route
	Middleware    []gin.HandlerFunc
	TLSCertPath   string
	TLSKeyPath    string
	ProdMode      bool
	DisableH3     bool
	DisableBrotli bool
}

// ServeCmd is the `<flavor>herald serve` subcommand for serving flavors.
// Binds a Gin engine with healthz/readyz/metrics + every Route in
// opts.Routes (nil-Handler+non-empty-HRD routes get StubRouteHandler).
//
// Listener model (Wave 4a Task 6):
//
//   - DisableH3 == false (default): binds BOTH a TCP/HTTPS+H2 listener
//     AND a UDP/H3 listener on --http-port. Both serve the same Gin
//     engine. Auto-injects AltSvcMiddleware + BrotliMiddleware into the
//     chain so TCP clients see Alt-Svc upgrade hints + Accept-Encoding:
//     br responses get compressed. Requires a TLS cert (operator flag,
//     env, or dev-autogen depending on ProdMode).
//   - DisableH3 == true: binds ONLY the TCP listener (HERALD_DISABLE_HTTP3=1
//     escape hatch). If a cert is supplied, the listener runs TLS; if
//     not, it falls back to plaintext HTTP/1.1+H2c, matching pre-T6
//     behavior for dev workflows that don't want TLS.
//
// Graceful shutdown on SIGTERM/SIGINT/ctx-cancel: BOTH listeners get up
// to 5s grace each. Returns nil on clean shutdown, error on bind failure.
//
// Per §107: a "serve started" PASS without observing a healthz round-
// trip on BOTH listeners is a bluff. The integration tests start the
// server and verify both listeners actually accept traffic before
// declaring success; post-shutdown, the TCP port returns connection-
// refused and the UDP port is re-bindable by an unrelated listener
// (proof the QUIC stack released the socket).
//
// Wave 4a Task 7.5 (pherald unification): the dual-listener body is now
// in RunServe(ctx, opts). ServeCmd is just flag-binding + a thin RunE
// shim. Flavor binaries that need lazy dependency construction (e.g.
// pherald, which builds its Runner against PG only at serve-time) can
// call BindServeFlags + RunServe directly, bypassing the cobra wrapper
// while still getting the exact same dual-listener engine.
func ServeCmd(opts ServeOpts) *cobra.Command {
	var (
		port     int
		tlsCert  string
		tlsKey   string
		noBrotli bool
	)
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the " + opts.Branding.DisplayName + " HTTP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Wave 4a Task 7: flag-supplied paths override opts defaults.
			// Empty flags leave opts as-supplied by the caller. This lets
			// per-binary main.go either pre-populate opts (programmatic)
			// or rely on operator-supplied flags (CLI).
			if tlsCert != "" {
				opts.TLSCertPath = tlsCert
			}
			if tlsKey != "" {
				opts.TLSKeyPath = tlsKey
			}
			if noBrotli {
				opts.DisableBrotli = true
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return RunServe(ctx, opts, port)
		},
	}
	BindServeFlags(cmd, &port, &tlsCert, &tlsKey, &noBrotli)
	return cmd
}

// BindServeFlags attaches the standard `serve` flag set (--http-port,
// --tls-cert, --tls-key, --no-brotli) to cmd, writing into the supplied
// destination variables. Exported so flavor binaries that compose a
// custom cobra command (pherald — needs lazy Runner construction before
// calling RunServe) get IDENTICAL flag UX to the shared ServeCmd path.
//
// Per §107: divergent flag UX between flavors is a covenant violation —
// operators muscle-memorising --tls-cert on cherald and getting silent
// failure on pherald would be a paper-cut bluff. Centralising the flag
// definitions here keeps every binary lockstep.
func BindServeFlags(cmd *cobra.Command, port *int, tlsCert, tlsKey *string, noBrotli *bool) {
	cmd.Flags().IntVar(port, "http-port", 0, "TCP+UDP port to bind (default = flavor's DefaultPort)")
	cmd.Flags().StringVar(tlsCert, "tls-cert", "", "Path to PEM-encoded TLS certificate (required when HTTP/3 is enabled, optional for TCP-only)")
	cmd.Flags().StringVar(tlsKey, "tls-key", "", "Path to PEM-encoded TLS private key (paired with --tls-cert)")
	cmd.Flags().BoolVar(noBrotli, "no-brotli", false, "Disable auto-wired Brotli compression middleware (useful for streaming routes that cannot be buffered)")
}

// RunServe is the exported dual-listener serve loop. It builds the Gin
// engine, registers healthz/readyz/metrics + opts.Middleware +
// opts.Routes, resolves the TLS cert source, starts the TCP/H2 listener
// (and the UDP/H3 listener when not opts.DisableH3), and blocks until
// SIGINT/SIGTERM/ctx-cancel. On terminal signal it gracefully shuts
// down BOTH listeners in parallel (5s grace each).
//
// Callable two ways:
//
//  1. Via ServeCmd's RunE — the common case for flavors with eager
//     dependency construction (cherald, sherald, bherald, …).
//  2. Directly from a flavor's custom cobra command — for flavors that
//     need lazy dependency construction (pherald builds its Runner and
//     verifier inside its own RunE so `pherald version` / `pherald
//     migrate` don't require PG + JWT env vars). The flavor binds the
//     standard flags via BindServeFlags, builds opts.Routes +
//     opts.Middleware lazily, then calls RunServe(ctx, opts, port).
//
// The two paths produce IDENTICAL listener behavior (dual TCP+UDP when
// not disabled, single TCP when DisableH3). Env-driven auto-detection
// (HERALD_DISABLE_HTTP3=1, HERALD_AUTH_MODE=jwks) lives here so both
// callers get it for free.
//
// Pass port = 0 to use opts.Branding.DefaultPort.
func RunServe(ctx context.Context, opts ServeOpts, port int) error {
	if port == 0 {
		port = opts.Branding.DefaultPort
	}
	// Env-driven auto-detection — keeps main.go free of env reads per
	// the T6 contract. HERALD_DISABLE_HTTP3=1 is the UDP-blocked-
	// environment escape hatch; HERALD_AUTH_MODE=jwks is the
	// production-mode signal that gates dev-cert autogen.
	if os.Getenv("HERALD_DISABLE_HTTP3") == "1" {
		opts.DisableH3 = true
	}
	if os.Getenv("HERALD_AUTH_MODE") == "jwks" {
		opts.ProdMode = true
	}
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	// Health/observability endpoints are registered BEFORE the flavor
	// middleware chain AND before the auto-wired Brotli/Alt-Svc layers
	// so they remain reachable without auth and without compression
	// overhead. The standard practice for k8s liveness/readiness probes
	// and Prometheus scrapers. Per §107, blocking healthz behind JWT
	// is a PASS-bluff: load balancers can't probe the service and the
	// operator gets a green deploy with a dead canary.
	r.GET("/v1/healthz", HealthzHandler(opts.Branding))
	r.GET("/v1/readyz", ReadyzHandler(opts.Branding))
	r.GET("/metrics", MetricsHandler(opts.Branding))

	// Wave 4a Task 6: auto-wire Brotli and Alt-Svc into the middleware
	// chain ahead of flavor-supplied middleware. Brotli applies to
	// flavor routes (not healthz/metrics — those are registered above
	// and Gin's r.Use() only affects routes registered AFTER).
	// Alt-Svc emits the HTTP/3 upgrade header on every TCP response
	// (no-op when h3Port <= 0).
	if !opts.DisableBrotli {
		// Quality 0 ⇒ consult HERALD_BROTLI_LEVEL env or fall back to
		// HeraldBrotliDefaultLevel (6).
		r.Use(BrotliMiddleware(0))
	}
	altSvcPort := 0
	if !opts.DisableH3 {
		altSvcPort = port
	}
	r.Use(AltSvcMiddleware(altSvcPort))

	for _, mw := range opts.Middleware {
		if mw != nil {
			r.Use(mw)
		}
	}
	for _, route := range opts.Routes {
		h := route.Handler
		if h == nil && route.HRD != "" {
			h = StubRouteHandler(route)
		}
		r.Handle(route.Method, route.Path, h)
	}

	// Resolve TLS cert source. Required for HTTP/3 (mandatory TLS 1.3);
	// optional for TCP-only mode. When the operator explicitly disables
	// H3 AND supplies no cert/key, we run plaintext for backward compat
	// with pre-T6 dev flows.
	var cert *tls.Certificate
	if !opts.DisableH3 {
		src, err := commons_tls.ResolveCertSource(commons_tls.Config{
			CertPath: opts.TLSCertPath,
			KeyPath:  opts.TLSKeyPath,
			ProdMode: opts.ProdMode,
		})
		if err != nil {
			return fmt.Errorf("resolve TLS cert: %w", err)
		}
		cert = src.Cert
	} else if opts.TLSCertPath != "" || opts.TLSKeyPath != "" {
		// H3 disabled but operator supplied TLS material — honor it for
		// the TCP listener so the "H3 off + TLS on" combo works.
		src, err := commons_tls.ResolveCertSource(commons_tls.Config{
			CertPath: opts.TLSCertPath,
			KeyPath:  opts.TLSKeyPath,
			ProdMode: opts.ProdMode,
		})
		if err != nil {
			return fmt.Errorf("resolve TLS cert (TCP-only): %w", err)
		}
		cert = src.Cert
	}

	addr := fmt.Sprintf(":%d", port)

	// Start the H3 listener first when enabled. If it fails to bind,
	// fail-fast — there's no point starting the TCP listener that would
	// advertise Alt-Svc pointing at a dead UDP port (which the §107
	// anti-bluff principle forbids).
	var (
		h3Srv      h3Handle // nil interface ⇒ H3 disabled
		h3StartErr error
	)
	if !opts.DisableH3 {
		h3Srv, h3StartErr = startQUIC(ctx, addr, r, cert)
		if h3StartErr != nil {
			return fmt.Errorf("start HTTP/3 listener: %w", h3StartErr)
		}
	}

	// Build the TCP server. When a cert is available, run HTTPS+H2;
	// otherwise plaintext HTTP/1.1+H2c (only allowed in DisableH3 +
	// no-cert mode for backward compat).
	tcpSrv := &http.Server{
		Addr:    addr,
		Handler: r,
	}
	if cert != nil {
		tcpSrv.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{*cert},
			MinVersion:   tls.VersionTLS12,
			NextProtos:   []string{"h2", "http/1.1"},
		}
	}

	tcpErrCh := make(chan error, 1)
	go func() {
		var err error
		if cert != nil {
			// Empty cert/key file args ⇒ use TLSConfig.Certificates.
			err = tcpSrv.ListenAndServeTLS("", "")
		} else {
			err = tcpSrv.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			tcpErrCh <- err
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	// Wait for either: TCP listener crash, signal, ctx cancel. We do
	// NOT wait for the H3 listener's terminal error here because the
	// upstream startQUIC already runs Start() in a goroutine; its
	// terminal error lands on Done() during shutdown and is logged
	// below.
	select {
	case err := <-tcpErrCh:
		// TCP listener crashed. Shut down H3 if it was running so we
		// don't leak the UDP socket past process exit.
		if h3Srv != nil {
			shutdownH3(h3Srv)
		}
		return fmt.Errorf("TCP listen: %w", err)
	case <-sigCh:
		// graceful shutdown
	case <-ctx.Done():
		// graceful shutdown
	}

	// Graceful shutdown of BOTH listeners, each with up to 5s grace.
	// Run them in parallel so a slow shutdown on one doesn't starve
	// the other.
	var wg sync.WaitGroup
	var tcpShutErr, h3ShutErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		tcpShutErr = tcpSrv.Shutdown(shutdownCtx)
	}()
	if h3Srv != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := h3Srv.Shutdown(shutdownCtx); err != nil {
				h3ShutErr = err
			}
		}()
	}
	wg.Wait()

	// Report shutdown errors. Per the brief, either listener's failure
	// should NOT crash the other — but we DO want to surface failures
	// so the operator knows what happened. The first non-nil wins;
	// the second is logged for posterity.
	if tcpShutErr != nil && h3ShutErr != nil {
		log.Printf("serve: secondary H3 shutdown error after TCP: %v", h3ShutErr)
		return fmt.Errorf("TCP shutdown: %w", tcpShutErr)
	}
	if tcpShutErr != nil {
		return fmt.Errorf("TCP shutdown: %w", tcpShutErr)
	}
	if h3ShutErr != nil {
		return fmt.Errorf("H3 shutdown: %w", h3ShutErr)
	}
	return nil
}

// h3Handle is the narrow interface ServeCmd needs from the HTTP/3
// server. It exists so a future hand-rolled mock (or a future
// configuration where startQUIC returns a different concrete type) can
// be slotted in without breaking the orchestration shape.
//
// The real implementation in commons/cli/h3.go returns a
// *digital.vasic.http3/pkg/server.Server which satisfies this
// interface via its Shutdown(ctx) method.
type h3Handle interface {
	Shutdown(ctx context.Context) error
}

// shutdownH3 best-efforts a graceful shutdown of the H3 listener with
// a short timeout. Used in the TCP-listener-crashed branch where we
// want to release the UDP socket before returning the TCP error.
func shutdownH3(srv h3Handle) {
	if srv == nil {
		return
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("serve: H3 shutdown after TCP crash: %v", err)
	}
}
