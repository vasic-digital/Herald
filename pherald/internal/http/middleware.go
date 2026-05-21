// Request-ID middleware for pherald. Read X-Request-ID from inbound;
// generate UUID v4 if absent; propagate in response header. Semantics
// preserve any non-empty inbound ID verbatim (matches the previous
// digital.vasic.middleware/pkg/requestid behaviour wrapped by the
// pre-refactor server.go so existing integrations that send opaque
// correlation IDs continue to work end-to-end).
package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// HeaderRequestID is the canonical HTTP header for request correlation.
const HeaderRequestID = "X-Request-ID"

// requestIDContextKey is the Gin context key for handler retrieval.
const requestIDContextKey = "request_id"

// RequestIDMiddleware returns a Gin middleware that ensures every
// request has an X-Request-ID and propagates it in the response.
// Preserves any non-empty inbound value (matches the legacy
// digital.vasic.middleware/pkg/requestid semantics — opaque correlation
// IDs from upstream proxies / load balancers / clients pass through).
//
// Anti-bluff (§107): the response header carrying the ID is the only
// honest evidence the middleware ran — pherald's e2e_bluff_hunt asserts
// it on every /v1/healthz round-trip.
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader(HeaderRequestID)
		if rid == "" {
			rid = uuid.NewString()
		}
		c.Set(requestIDContextKey, rid)
		c.Header(HeaderRequestID, rid)
		c.Next()
	}
}

// isUUIDShape returns true when s is a 36-char canonical UUID. Kept
// exported-to-the-package so the middleware test can assert that
// generated IDs (the empty-inbound branch) are UUID-shaped — verifying
// the legitimate generator path without forcing inbound IDs to be
// UUID-shaped at runtime.
func isUUIDShape(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			isHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
			if !isHex {
				return false
			}
		}
	}
	return true
}
