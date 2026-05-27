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

	"github.com/vasic-digital/herald/cherald/internal/compliance"
	"github.com/vasic-digital/herald/cherald/internal/modes"
	"github.com/vasic-digital/herald/commons/cli"
	constitution "github.com/vasic-digital/herald/commons_constitution"
)

// Routes returns cherald-specific HTTP routes. /v1/compliance is LIVE
// (Wave 3a). The /v1/compliance/modes admin surface is LIVE (HRD-027) —
// operators flip a rule's enforcement mode per tenant without redeploy.
func Routes(store constitution.ConstitutionStore, ladder constitution.ModeLadder) []cli.Route {
	return []cli.Route{
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
}

// _ = gin.HandlerFunc(nil) // import-guard for the gin package to stay
// referenced even if compliance.Handler's import is later refactored.
var _ gin.HandlerFunc = nil
