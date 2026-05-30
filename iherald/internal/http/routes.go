// Package http exports iherald's HTTP plane. HRD-024 ships
// POST /v1/webhooks/page LIVE — the Wave 2 r1 501-stub is now replaced by
// the iherald/internal/page.Handler backed by the HRD-024
// bindings.Pipeline (the §42.3 credential-leak page-out + operator-blocked
// escalation + §18.8 incident-severity routing detectors). cli.ServeCmd
// provides healthz/readyz/metrics; only the flavor-specific route is
// declared here.
package http

import (
	"github.com/vasic-digital/herald/commons/cli"
	"github.com/vasic-digital/herald/iherald/internal/bindings"
	"github.com/vasic-digital/herald/iherald/internal/page"
)

// Routes returns iherald-specific HTTP routes. POST /v1/webhooks/page is
// LIVE (HRD-024) when a non-nil pipeline is supplied — it drives the
// PagerDuty/Opsgenie-compatible inbound escalation surface (spec §42 /
// §18.8) through the iherald bindings.Pipeline (classify → emit → persist →
// audit) and returns a 202 + Receipt.
//
// pipeline nil is the §107-honest degraded path: when the emitter/audit
// backends couldn't be wired (a dev path with no usable bus), the route
// falls back to the 501 stub rather than panicking — the operator gets the
// HRD pointer instead of a crash. The lazy serve wiring (cmd/iherald) always
// supplies a non-nil pipeline on the production path.
func Routes(pipeline *bindings.Pipeline) []cli.Route {
	if pipeline == nil {
		return []cli.Route{
			{
				Method:      "POST",
				Path:        "/v1/webhooks/page",
				HRD:         "HRD-024",
				Description: "PagerDuty/Opsgenie-compatible inbound (spec §42) — pipeline unavailable",
			},
		}
	}
	return []cli.Route{
		{
			Method:      "POST",
			Path:        "/v1/webhooks/page",
			Handler:     page.Handler(pipeline),
			Description: "PagerDuty/Opsgenie-compatible inbound escalation page-out; emit+persist+audit (spec §42 / §18.8 / HRD-024)",
		},
	}
}
