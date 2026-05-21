// Package http exports sherald's HTTP plane. Wave 2 ships /v1/safety_state
// as a 501 stub (→ HRD-098) per the anti-bluff posture (§11.4.69): an
// honest 501 + HRD pointer beats a 200 stub returning empty. cli.ServeCmd
// provides healthz/readyz/metrics; only the flavor-specific route is
// declared here.
package http

import "github.com/vasic-digital/herald/commons/cli"

// Routes returns sherald-specific HTTP routes. Wave 2 ships 1 501 stub.
func Routes() []cli.Route {
	return []cli.Route{
		{
			Method:      "GET",
			Path:        "/v1/safety_state",
			HRD:         "HRD-098",
			Description: "Daemon status — open events, current mem%, last destructive-op log",
		},
	}
}
