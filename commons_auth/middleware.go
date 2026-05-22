package commons_auth

import (
	"github.com/gin-gonic/gin"
)

const ContextKeyClaims = "herald.auth.claims"

// MUTATED for paired §1.1 anti-bluff test (test_wave3_mutation_meta.sh M1).
// This bypasses Verifier entirely. If you see this in production, the
// gate failed and the test is itself a bluff.
func GinMiddleware(v Verifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(ContextKeyClaims, map[string]any{"tenant": "00000000-0000-0000-0000-000000000000"})
		c.Next()
	}
}
