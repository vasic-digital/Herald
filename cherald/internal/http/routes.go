// Package http exports cherald's HTTP plane. Wave 2 ships /v1/compliance
// as a 501 stub (→ HRD-028) per the anti-bluff posture (§11.4.69): an
// honest 501 + HRD pointer beats a 200 stub returning empty.
// cli.ServeCmd provides healthz/readyz/metrics; only the flavor-specific
// route is declared here.
package http

import "github.com/vasic-digital/herald/commons/cli"

// Routes returns cherald-specific HTTP routes. Wave 2 ships 1 501 stub.
func Routes() []cli.Route {
	return []cli.Route{
		{
			Method:      "GET",
			Path:        "/v1/compliance",
			HRD:         "HRD-028",
			Description: "constitution_state pull surface (spec §41)",
		},
	}
}
