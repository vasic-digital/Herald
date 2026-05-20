package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// registerRoutes wires the M3 route set onto s.engine. Per V3 §44.3, M3
// ships the minimum-viable subset; richer routes (full §41 surface) are
// expanded under HRD-016.
func (s *Server) registerRoutes() {
	v1 := s.engine.Group("/v1")
	v1.GET("/healthz", s.healthz)
	v1.GET("/readyz", s.readyz)
	v1.POST("/events", s.eventsIngest)
	v1.GET("/compliance", s.complianceList)

	// /metrics lives at the root (Prometheus convention) — not under /v1.
	s.engine.GET("/metrics", s.metrics)
}

// healthz: liveness probe. Returns build info + 200 OK as long as the
// process is up. Does NOT probe dependencies (that's readyz's job).
func (s *Server) healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"build":  s.cfg.Build,
	})
}

// readyz: readiness probe. Currently a thin stub — returns 200 once the
// server is started. Real implementation (Postgres ping + Redis ping +
// constitution-bundle hash refresh) is HRD-016 expansion work.
func (s *Server) readyz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ready",
		"checks": gin.H{
			"postgres": "not-yet-wired",
			"redis":    "not-yet-wired",
			"bundle":   "not-yet-wired",
		},
		"note": "real readiness checks land with HRD-016 (depend on Runner wiring)",
	})
}

// eventsIngest: stub for CloudEvents POST /v1/events. Returns 501 with
// an HRD pointer per §11.4.69 anti-bluff (501 = "feature not yet
// implemented"; a 200 stub would lie).
func (s *Server) eventsIngest(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error":  "not-implemented",
		"detail": "POST /v1/events full ingest pipeline pending HRD-016 (Runner.Run wiring) — Foundation M3 ships the route + middleware chain only",
		"hrd":    "HRD-016",
	})
}

// complianceList: stub for GET /v1/compliance. Same 501 + HRD pointer.
func (s *Server) complianceList(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error":  "not-implemented",
		"detail": "GET /v1/compliance constitution_state pull surface pending HRD-016 (ConstitutionStore.List wiring through tenant-extracted JWT) — Foundation M3 ships the route + middleware chain only",
		"hrd":    "HRD-016",
	})
}

// metrics: Prometheus scrape endpoint. Foundation M3 ships a placeholder
// emitting the build-info gauge so scrapers can detect the endpoint;
// real counters land with HRD-016 (request-duration histogram, RLS-
// scoped counters per rule_id, etc.).
func (s *Server) metrics(c *gin.Context) {
	c.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	c.String(http.StatusOK,
		"# HELP herald_build_info Build-time provenance for the running pherald binary.\n"+
			"# TYPE herald_build_info gauge\n"+
			"herald_build_info{version=\""+s.cfg.Build.Version+"\",commit=\""+s.cfg.Build.GitCommit+"\"} 1\n",
	)
}
