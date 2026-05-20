// Package http is pherald's M3 Gin REST surface per spec V3 §41 + §44.3.
//
// Composes `digital.vasic.middleware` + `digital.vasic.auth` +
// `digital.vasic.observability` per the §11.4.74 catalogue-check
// "strong-extend" verdict. The Herald-side surface here is intentionally
// thin — the Helix-stack submodules carry the heavy lifting (request-id
// propagation, panic recovery with localised messages, JWT validation,
// OTel HTTP middleware) so this file stays focused on Herald-specific
// route registration + graceful-shutdown ergonomics.
//
// Routes (Foundation M3):
//
//	GET  /v1/healthz       — liveness probe; always 200 OK with build info.
//	GET  /v1/readyz        — readiness probe; 200 only when dependencies OK.
//	GET  /metrics          — Prometheus scrape (gin-prometheus-aware).
//	POST /v1/events        — CloudEvents ingest stub (HRD-016 expansion).
//	GET  /v1/compliance    — constitution_state pull surface stub (HRD-016).
//
// `/v1/events` and `/v1/compliance` return 501 Not Implemented with an
// HRD pointer until the Runner wiring lands. Anti-bluff per §11.4.69: a
// 501 with explicit "needs HRD-NNN" body is honest; a 200 stub
// returning empty would be a §11.4 violation.
package http

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	mwgin "digital.vasic.middleware/pkg/gin"
	"digital.vasic.middleware/pkg/recovery"
	"digital.vasic.middleware/pkg/requestid"
)

// Server is the Foundation M3 Gin REST surface.
type Server struct {
	cfg     Config
	engine  *gin.Engine
	httpSrv *http.Server
	startMu sync.Mutex
	started bool
}

// Config configures Server. Defaults applied at New time.
type Config struct {
	// Addr is the listen address. Default "0.0.0.0:24091" per spec §9.4
	// + the quickstart compose port mapping.
	Addr string

	// ReadTimeout / WriteTimeout / IdleTimeout bound socket lifetimes per
	// §3.1 graceful-shutdown discipline. Defaults: 10s / 30s / 90s.
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration

	// ShutdownTimeout bounds the Graceful Shutdown drain after SIGTERM.
	// Default 25s (matches §3.1's "drain in-flight then exit").
	ShutdownTimeout time.Duration

	// Build is the build-info payload returned by /v1/healthz. Populated
	// from pherald/internal/version at server-construction time.
	Build BuildInfo
}

// BuildInfo is the build-time provenance for /v1/healthz.
type BuildInfo struct {
	Version    string `json:"version"`
	GitCommit  string `json:"git_commit,omitempty"`
	BuildDate  string `json:"build_date,omitempty"`
	GoVersion  string `json:"go_version,omitempty"`
}

// New constructs a Server with the canonical Herald middleware chain.
func New(cfg Config) *Server {
	if cfg.Addr == "" {
		cfg.Addr = "0.0.0.0:24091"
	}
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 10 * time.Second
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 30 * time.Second
	}
	if cfg.IdleTimeout == 0 {
		cfg.IdleTimeout = 90 * time.Second
	}
	if cfg.ShutdownTimeout == 0 {
		cfg.ShutdownTimeout = 25 * time.Second
	}

	// gin.New (NOT gin.Default) — we wire our own middleware stack instead
	// of relying on Gin's built-in logger/recovery.
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()

	// Canonical Herald middleware chain (order matters):
	//
	//   1. requestid — earliest so panics get tagged with the id.
	//   2. recovery  — catches panics in subsequent handlers / middlewares.
	//   3. (later) JWT auth, OTel, RLS tenant-context.
	engine.Use(mwgin.Wrap(requestid.New()))
	engine.Use(mwgin.Wrap(recovery.New(&recovery.Config{
		// Default Output → stderr; PrintStack → true; ResponseBody → nil
		// (translator-rendered, CONST-046).
	})))

	srv := &Server{
		cfg:    cfg,
		engine: engine,
	}
	srv.registerRoutes()

	srv.httpSrv = &http.Server{
		Addr:         cfg.Addr,
		Handler:      engine,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}
	return srv
}

// Handler returns the raw HTTP handler (Gin engine) so tests can call
// httptest.NewServer / NewRecorder without binding a real listener.
func (s *Server) Handler() http.Handler { return s.engine }

// Start begins serving HTTP requests. Idempotent — re-calling on an
// already-started server returns nil without restarting.
func (s *Server) Start() error {
	s.startMu.Lock()
	if s.started {
		s.startMu.Unlock()
		return nil
	}
	s.started = true
	s.startMu.Unlock()

	if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown drains in-flight requests within cfg.ShutdownTimeout and
// closes the listener. Per spec §3.1 graceful-shutdown contract.
func (s *Server) Shutdown(ctx context.Context) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, s.cfg.ShutdownTimeout)
	defer cancel()
	return s.httpSrv.Shutdown(timeoutCtx)
}
