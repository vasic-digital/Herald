// Package http exports iherald's HTTP plane. Wave 2 ships
// /v1/webhooks/page as a 501 stub (→ HRD-024) per the anti-bluff
// posture (§11.4.69): an honest 501 + HRD pointer beats a 200 stub
// returning empty. cli.ServeCmd provides healthz/readyz/metrics; only
// the flavor-specific route is declared here.
package http

import "github.com/vasic-digital/herald/commons/cli"

// Routes returns iherald-specific HTTP routes. Wave 2 ships 1 501 stub.
func Routes() []cli.Route {
	return []cli.Route{
		{
			Method:      "POST",
			Path:        "/v1/webhooks/page",
			HRD:         "HRD-024",
			Description: "PagerDuty/Opsgenie-compatible inbound (spec §42)",
		},
	}
}
