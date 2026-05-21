// Package http exports sherald's HTTP plane. Wave 3a flips /v1/safety_state
// from a 501-stub (HRD-098 placeholder) to a LIVE handler backed by an
// in-process safety.Aggregator + background mem-sampler. cli.ServeCmd
// provides healthz/readyz/metrics; only the flavor-specific live route
// is declared here.
package http

import (
	"github.com/gin-gonic/gin"

	"github.com/vasic-digital/herald/commons/cli"
	"github.com/vasic-digital/herald/sherald/internal/safety"
)

// Routes returns sherald-specific HTTP routes. /v1/safety_state is now
// LIVE (Wave 3a) — replaces the 501-stub that pointed at HRD-098.
func Routes(agg *safety.Aggregator) []cli.Route {
	return []cli.Route{
		{
			Method:      "GET",
			Path:        "/v1/safety_state",
			Handler:     safety.Handler(agg),
			Description: "Daemon-local safety state (V3 §41 / Wave 3a live)",
		},
	}
}

var _ gin.HandlerFunc = nil
