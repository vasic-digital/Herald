package cli

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/vasic-digital/herald/commons"
)

// Route declares one HTTP route. Wave 2 stubs return 501 with the HRD
// pointer body when Handler is nil + HRD is non-empty.
type Route struct {
	Method      string          // "GET", "POST", ...
	Path        string          // "/v1/compliance"
	Handler     gin.HandlerFunc // if nil + HRD non-empty, StubRouteHandler is used
	Description string          // operator-readable summary
	HRD         string          // "HRD-028" for 501 stubs; "" for live routes
}

// HealthzHandler returns 200 with {status:"ok",flavor,build:{version,commit,go_version}}.
func HealthzHandler(br commons.Branding) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"flavor": br.Flavor,
			"build": gin.H{
				"version":    BuildVersion,
				"commit":     BuildCommit,
				"go_version": buildGoVersion(),
			},
		})
	}
}

// ReadyzHandler returns 200 with {status:"ready"}. Wave 2 doesn't probe
// real readiness — flavor implementations may extend later via custom
// Route entries that override this builtin.
func ReadyzHandler(br commons.Branding) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ready", "flavor": br.Flavor})
	}
}

// MetricsHandler returns Prometheus text with a <prefix>_build_info gauge.
// The gauge prefix follows the documented Prometheus convention for
// Herald flavors: BinaryName takes precedence (so pherald emits
// "pherald_build_info", sherald emits "sherald_build_info", …); when
// BinaryName is empty we fall back to Flavor (Wave 2 tests pass Flavor
// directly as the binary name). Wave 2 emits just the gauge; full
// Prometheus client wiring is a follow-up HRD (live observability per
// spec §17).
func MetricsHandler(br commons.Branding) gin.HandlerFunc {
	return func(c *gin.Context) {
		prefix := br.BinaryName
		if prefix == "" {
			prefix = br.Flavor
		}
		c.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		fmt.Fprintf(c.Writer, "# HELP %s_build_info Build information for %s.\n", prefix, br.DisplayName)
		fmt.Fprintf(c.Writer, "# TYPE %s_build_info gauge\n", prefix)
		fmt.Fprintf(c.Writer, "%s_build_info{version=%q,commit=%q,go_version=%q} 1\n",
			prefix, BuildVersion, BuildCommit, buildGoVersion())
	}
}

// StubRouteHandler returns 501 with a JSON body citing the HRD that will
// implement the route. Used for Wave 2 placeholder routes.
func StubRouteHandler(route Route) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusNotImplemented, gin.H{
			"error":       "not yet implemented",
			"hrd":         route.HRD,
			"path":        route.Path,
			"description": route.Description,
		})
	}
}
