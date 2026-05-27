// Package http exports cherald's HTTP plane. Wave 3a ships /v1/compliance
// LIVE — the Wave 2 501-stub (HRD-028) is now replaced by the
// cherald/internal/compliance.Handler backed by a
// commons_constitution.ConstitutionStore.
//
// cli.ServeCmd provides healthz/readyz/metrics; only the flavor-specific
// route is declared here.
package http

import (
	"github.com/gin-gonic/gin"

	"github.com/vasic-digital/herald/cherald/internal/bindings"
	"github.com/vasic-digital/herald/cherald/internal/compliance"
	"github.com/vasic-digital/herald/cherald/internal/modes"
	"github.com/vasic-digital/herald/commons/cli"
	constitution "github.com/vasic-digital/herald/commons_constitution"
)

// Routes returns cherald-specific HTTP routes. /v1/compliance is LIVE
// (Wave 3a). The /v1/compliance/modes admin surface is LIVE (HRD-027) —
// operators flip a rule's enforcement mode per tenant without redeploy.
//
// pipeline is the HRD-019 bindings pipeline driving the constitution
// rule-evaluation surface. When non-nil, POST /v1/compliance/evaluate is
// mounted (the detect→emit→persist write side). It is nil only in the
// Memory-fallback dev path where the emitter/audit backends aren't wired —
// the read surfaces stay live regardless.
func Routes(store constitution.ConstitutionStore, ladder constitution.ModeLadder, pipeline *bindings.Pipeline) []cli.Route {
	routes := []cli.Route{
		{
			Method:      "GET",
			Path:        "/v1/compliance",
			Handler:     compliance.Handler(store),
			Description: "Constitution-state pull surface (V3 §41 / Wave 3a live)",
		},
		{
			Method:      "GET",
			Path:        "/v1/compliance/modes",
			Handler:     modes.ListHandler(ladder),
			Description: "List per-tenant rule enforcement-mode bindings (V3 §42.1.4 / HRD-027)",
		},
		{
			Method:      "GET",
			Path:        "/v1/compliance/modes/:rule_id",
			Handler:     modes.GetHandler(ladder),
			Description: "Get one rule's enforcement mode (enforce default when unbound) (HRD-027)",
		},
		{
			Method:      "PUT",
			Path:        "/v1/compliance/modes/:rule_id",
			Handler:     modes.PutHandler(ladder),
			Description: "Flip a rule's enforcement mode allow|warn|enforce without redeploy (HRD-027)",
		},
	}
	if pipeline != nil {
		routes = append(routes, cli.Route{
			Method:      "POST",
			Path:        "/v1/compliance/evaluate",
			Handler:     compliance.EvaluateHandler(pipeline),
			Description: "Evaluate a constitution rule against a subject; emit+persist on violation (V3 §42.3 / HRD-019)",
		})
	}
	return routes
}

// _ = gin.HandlerFunc(nil) // import-guard for the gin package to stay
// referenced even if compliance.Handler's import is later refactored.
var _ gin.HandlerFunc = nil
