package http

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestRequestIDMiddleware_GeneratesIfMissing proves the middleware
// synthesises a UUID-shaped X-Request-ID when the inbound request
// omits the header. Anti-bluff: asserts on the response header (the
// only user-visible evidence the middleware actually ran).
func TestRequestIDMiddleware_GeneratesIfMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestIDMiddleware())
	r.GET("/", func(c *gin.Context) { c.Status(204) })

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	rid := rec.Header().Get(HeaderRequestID)
	if rid == "" {
		t.Fatalf("response %s missing — middleware did not run", HeaderRequestID)
	}
	if !isUUIDShape(rid) {
		t.Errorf("response %s = %q not UUID-shape (generated path must use UUID)", HeaderRequestID, rid)
	}
}

// TestRequestIDMiddleware_PropagatesInbound proves the middleware
// preserves an inbound X-Request-ID verbatim — even for opaque
// correlation strings (matches the legacy requestid semantics).
func TestRequestIDMiddleware_PropagatesInbound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestIDMiddleware())
	r.GET("/", func(c *gin.Context) { c.Status(204) })

	const inbound = "01234567-89ab-cdef-0123-456789abcdef"
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set(HeaderRequestID, inbound)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get(HeaderRequestID); got != inbound {
		t.Errorf("%s = %q, want propagated %q", HeaderRequestID, got, inbound)
	}
}

// TestRequestIDMiddleware_PropagatesOpaqueInbound proves the legacy
// semantics survive: arbitrary correlation strings (not UUID-shaped)
// pass through unmodified. The previous server_test.go's
// RequestIDHeaderPropagation depended on this — preserve the contract.
func TestRequestIDMiddleware_PropagatesOpaqueInbound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestIDMiddleware())
	r.GET("/", func(c *gin.Context) { c.Status(204) })

	const inbound = "test-correlation-id-12345"
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set(HeaderRequestID, inbound)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get(HeaderRequestID); got != inbound {
		t.Errorf("opaque inbound %s = %q, want preserved %q", HeaderRequestID, got, inbound)
	}
}
