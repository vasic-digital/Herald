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
	"github.com/vasic-digital/herald/commons/cli"
	constitution "github.com/vasic-digital/herald/commons_constitution"
)

// Routes returns cherald-specific HTTP routes. /v1/compliance is now
// LIVE (Wave 3a) — replaces the 501-stub that pointed at HRD-028.
func Routes(store constitution.ConstitutionStore) []cli.Route {
	return []cli.Route{
		{
			Method:      "GET",
			Path:        "/v1/compliance",
			Handler:     compliance.Handler(store),
			Description: "Constitution-state pull surface (V3 §41 / Wave 3a live)",
		},
	}
}

// _ = gin.HandlerFunc(nil) // import-guard for the gin package to stay
// referenced even if compliance.Handler's import is later refactored.
var _ gin.HandlerFunc = nil
