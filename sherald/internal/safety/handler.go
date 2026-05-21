package safety

import (
	"net/http"

	"github.com/gin-gonic/gin"

	commons_auth "github.com/vasic-digital/herald/commons_auth"
)

// Handler returns the gin.HandlerFunc serving GET /v1/safety_state.
//
// Reads agg.Snapshot() and emits the SafetyState JSON. Requires JWT (any
// tenant — process-global state, NOT tenant-scoped per Wave 3 design §3:
// sherald's safety state describes the daemon process, not a per-tenant
// projection). Middleware (commons_auth.GinMiddleware) must have set
// commons_auth.ContextKeyClaims upstream; absent or zero claims → 401.
//
// §107 anti-bluff: this handler does NOT return success on missing auth —
// the test suite exercises both the 401 path (no claims) and the 200 path
// (claims present, snapshot serialized end-to-end). A stub returning {} on
// 401 would be a PASS-bluff.
func Handler(agg *Aggregator) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, ok := c.Get(commons_auth.ContextKeyClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "no auth claims"})
			return
		}
		c.JSON(http.StatusOK, agg.Snapshot())
	}
}
