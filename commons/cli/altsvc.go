// Wave 4a Task 4 — Alt-Svc Gin middleware.
//
// AltSvcMiddleware wraps digital.vasic.middleware/pkg/altsvc into a Gin
// handler that advertises HTTP/3 (QUIC) availability on the configured
// UDP port to TCP/HTTPS clients. The advertised header instructs
// Alt-Svc-aware clients (curl --http3, Chromium, Firefox, the Go
// http3.Transport in digital.vasic.http3 consumer-side) to upgrade
// subsequent requests to the QUIC listener bound by startQUIC (see
// commons/cli/h3.go).
//
// Per §107 / Sixth Law: the load-bearing assertion in altsvc_test.go
// is the EXACT response header value — not "header non-empty", not
// "middleware compiled". Mutating this file to no-op MUST cause
// TestAltSvcMiddleware_EmitsHeader to FAIL.

package cli

import (
	"strconv"

	altsvc "digital.vasic.middleware/pkg/altsvc"
	mwgin "digital.vasic.middleware/pkg/gin"
	"github.com/gin-gonic/gin"
)

// AltSvcMiddleware returns a Gin handler that emits the Alt-Svc response
// header advertising HTTP/3 availability on the supplied UDP port. The
// emitted header is exactly:
//
//	Alt-Svc: h3=":<h3Port>"; ma=2592000
//
// ma=2592000 is 30 days in seconds — RFC 7838 §3.1 recommends a long
// max-age so clients retain the upgrade hint across browser sessions.
//
// Disabled mode: when h3Port <= 0 (HTTP/3 disabled via
// HERALD_DISABLE_HTTP3=1 or any configuration path that surfaces a
// non-positive port to serve.go), the middleware is a no-op and does
// NOT emit the header. Telling a client to upgrade to a port that
// isn't listening is a worse failure mode than letting them stay on
// HTTP/1.1+HTTP/2 — the client would burn UDP handshake attempts and
// silently fall back, polluting metrics.
//
// The wrapped middleware is digital.vasic.middleware/pkg/altsvc.New
// bridged through digital.vasic.middleware/pkg/gin.Wrap so the Gin
// chain sees a native gin.HandlerFunc.
func AltSvcMiddleware(h3Port int) gin.HandlerFunc {
	if h3Port <= 0 {
		// No-op middleware: pass through to the next handler without
		// touching response headers.
		return func(c *gin.Context) { c.Next() }
	}
	cfg := &altsvc.Config{
		Enabled: true,
		H3Port:  strconv.Itoa(h3Port),
		MaxAge:  2592000, // 30 days in seconds (RFC 7838 §3.1 recommendation)
	}
	return mwgin.Wrap(altsvc.New(cfg))
}
