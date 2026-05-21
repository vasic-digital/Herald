// Package http exports pherald's HTTP plane: the pherald-specific Route
// slice for /v1/events + /v1/compliance, plus the RequestIDMiddleware
// that propagates the X-Request-ID header. cli.ServeCmd consumes both
// via ServeOpts.Routes + ServeOpts.Middleware.
//
// Wave 2 Task 6 refactor (2026-05-21): the previous 150-line gin.Engine
// owner (Server / Config / BuildInfo + 5 method handlers) was deleted
// in favour of the shared commons/cli/ scaffold — pherald's serve
// subcommand now delegates engine construction + lifecycle to
// cli.ServeCmd and only contributes its flavor-specific routes +
// middleware through this package.
//
// Anti-bluff (§107 / §11.4.69): /v1/events + /v1/compliance return 501
// with explicit HRD-016 pointers until the Runner wiring lands. A 200
// stub returning empty would be a §11.4 violation.
package http

import (
	"github.com/vasic-digital/herald/commons/cli"
)

// Routes returns the pherald-specific /v1 routes. healthz/readyz/metrics
// are handled by cli.ServeCmd's built-ins (commons/cli/routes.go) — flavor
// implementations only need to declare their flavor-specific routes.
//
// Both routes are 501-stubs pending HRD-016 (Runner.Run wiring) — the
// honest §11.4.69 anti-bluff posture (501 + HRD pointer beats a 200
// stub returning empty).
func Routes() []cli.Route {
	return []cli.Route{
		{
			Method:      "POST",
			Path:        "/v1/events",
			HRD:         "HRD-016",
			Description: "Inbound CloudEvent ingestion (spec §41)",
		},
		{
			Method:      "GET",
			Path:        "/v1/compliance",
			HRD:         "HRD-016",
			Description: "constitution_state pull surface (spec §41)",
		},
	}
}
