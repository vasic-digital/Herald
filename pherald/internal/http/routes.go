// Package http exports pherald's HTTP plane: the pherald-specific Route
// slice for /v1/events plus the RequestIDMiddleware that propagates the
// X-Request-ID header. cli.ServeCmd consumes both via ServeOpts.Routes +
// ServeOpts.Middleware.
//
// Wave 2 Task 6 refactor (2026-05-21): the previous 150-line gin.Engine
// owner (Server / Config / BuildInfo + 5 method handlers) was deleted
// in favour of the shared commons/cli/ scaffold — pherald's serve
// subcommand now delegates engine construction + lifecycle to
// cli.ServeCmd and only contributes its flavor-specific routes +
// middleware through this package.
//
// Wave 3b Task 10 refactor (2026-05-22):
//
//  1. /v1/events flips from 501-stub to LIVE — EventsHandler drives the
//     §32 7-stage Runner pipeline; replaces the Wave 2 placeholder.
//  2. /v1/compliance is REMOVED from pherald's route list — it's a
//     cherald-owned route (real implementation in
//     cherald/internal/compliance/) that Wave 2 Task 6 inadvertently left
//     here as a 501-stub. Removing it here makes the binary boundary
//     honest: pherald ingests events; cherald serves the constitution
//     state-pull surface.
//
// Anti-bluff (§107 / §11.4.69): every Route now either carries a real
// Handler OR an HRD-NNN pointer. There are no silent 200-stubs.
package http

import (
	"github.com/vasic-digital/herald/commons/cli"
	"github.com/vasic-digital/herald/pherald/internal/runner"
)

// Routes returns the pherald-specific /v1 routes. healthz/readyz/metrics
// are handled by cli.ServeCmd's built-ins (commons/cli/routes.go) — flavor
// implementations only need to declare their flavor-specific routes.
//
// As of Wave 3b /v1/events is LIVE: EventsHandler(r) wires it into the
// Runner instance constructed in pherald/cmd/pherald/main.go. The Runner
// owns the §32 pipeline (parse → idempotency → tenant → policy →
// subscribers → dispatch → outcome) end-to-end.
func Routes(r *runner.Runner) []cli.Route {
	return []cli.Route{
		{
			Method:      "POST",
			Path:        "/v1/events",
			Handler:     EventsHandler(r),
			Description: "Inbound CloudEvent ingestion (spec §41 / Wave 3b live)",
		},
	}
}
