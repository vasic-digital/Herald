package commons_auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// ContextKeyClaims is the Gin context key under which authenticated
// claims are stored. Downstream handlers retrieve via c.Get(ContextKeyClaims).
const ContextKeyClaims = "herald.auth.claims"

// GinMiddleware returns a Gin handler that gates every request behind v.
// On success the claims map is stored in the Gin context under
// ContextKeyClaims; on failure the handler short-circuits with 401 + a
// typed JSON body. The body schema is intentionally minimal — clients
// should not depend on internal error formatting beyond the top-level
// "error" field.
func GinMiddleware(v Verifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		hdr := c.GetHeader("Authorization")
		if hdr == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		const prefix = "Bearer "
		if !strings.HasPrefix(hdr, prefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		token := strings.TrimPrefix(hdr, prefix)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		claims, err := v.Verify(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		c.Set(ContextKeyClaims, claims)
		c.Next()
	}
}
