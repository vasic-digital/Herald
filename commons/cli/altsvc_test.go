// Wave 4a Task 4 — Alt-Svc Gin middleware anti-bluff tests.
//
// Per §107 / Sixth Law: each test below makes a load-bearing positive
// assertion against the EXACT response header value emitted by
// AltSvcMiddleware. A grep-only / "header is non-empty" / "middleware
// compiled" assertion would be a PASS-bluff because it would not
// distinguish between the correct h3 port + max-age combination and
// any garbled / silently-wrong value.
//
// Mutating altsvc.go to no-op MUST cause TestAltSvcMiddleware_EmitsHeader
// to FAIL — that is the falsifiability contract recorded in the Wave 4a
// plan §Task 4 (T9 M2 mutation gate).

package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestAltSvcMiddleware_EmitsHeader is the §107 anti-bluff load-bearing
// test for Wave 4a Task 4. It asserts the EXACT Alt-Svc header value —
// not a substring, not a regex, not "is the header set at all" — because
// the operator-facing failure mode (a silently wrong h3 port or a missing
// max-age) would slip past a fuzzy assertion.
//
// Format: `h3=":24791"; ma=2592000`
//   - h3=":24791"  — Alt-Svc h3 token + the configured UDP port
//   - ma=2592000   — 30 days in seconds (RFC 7838 §3.1)
//
// The space + semicolon ordering is exactly what
// digital.vasic.middleware/pkg/altsvc.New emits via its fmt.Sprintf
// format string (`h3=":%s"; ma=%d`) — locking the exact value here
// gives us a regression sentinel if the upstream package ever changes
// its format.
func TestAltSvcMiddleware_EmitsHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AltSvcMiddleware(24791))
	r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — handler did not run", rec.Code)
	}

	const want = `h3=":24791"; ma=2592000`
	got := rec.Header().Get("Alt-Svc")
	if got != want {
		t.Errorf("Alt-Svc header = %q, want %q — exact-value assertion failed (PASS-bluff would be 'header non-empty')", got, want)
	}
}

// TestAltSvcMiddleware_DisabledWhenPortZero verifies the explicit-disable
// path: a port of 0 means HTTP/3 is off (HERALD_DISABLE_HTTP3=1 escape
// hatch surfaced by serve.go's port resolver). The middleware MUST NOT
// emit an Alt-Svc header — telling a client to upgrade to a port that
// isn't listening costs them a UDP handshake round-trip + silent fallback.
//
// A regression here would surface as production clients trying to dial
// QUIC on a host that's serving TCP-only and observing slow first-byte
// times — exactly the operator-invisible failure class §107 forbids.
func TestAltSvcMiddleware_DisabledWhenPortZero(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AltSvcMiddleware(0))
	r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — handler did not run", rec.Code)
	}

	if got := rec.Header().Get("Alt-Svc"); got != "" {
		t.Errorf("Alt-Svc header = %q with port=0; want empty (HTTP/3 disabled means do not advertise)", got)
	}
}

// TestAltSvcMiddleware_NegativePortNoOp guards the defensive-coding
// branch. A negative port can surface if a future config path
// represents "disabled" as -1 (sentinel) or if an integer-parsing bug
// upstream lets a negative value through. Either way, the middleware
// MUST refuse to advertise — the same reasoning as the port=0 case:
// don't lie to clients about an HTTP/3 endpoint that doesn't exist.
//
// This is a §11.4.6 (no-guessing) hardening: the implementation uses
// `h3Port <= 0` rather than `h3Port == 0` so the disabled branch
// covers every non-positive port, and this test pins that contract.
func TestAltSvcMiddleware_NegativePortNoOp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AltSvcMiddleware(-1))
	r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — handler did not run", rec.Code)
	}

	if got := rec.Header().Get("Alt-Svc"); got != "" {
		t.Errorf("Alt-Svc header = %q with port=-1; want empty (defensive negative-port no-op)", got)
	}
}
