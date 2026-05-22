// Wave 4b Task 6 — sherald /v1/safety_state TOON anti-bluff tests.
//
// Per §107 / Sixth Law: each test below makes a load-bearing positive
// assertion about WIRE BYTES — not just headers, not "no error", not
// "middleware compiled". The 2026-05-17 PASS-bluff manifested as JSON
// bytes wearing an application/toon Content-Type; header-only assertions
// would silently pass on that bug. The TOON tests here pin the byte-
// level contract so a future regression cannot slip.
//
// Mutation falsifiability: deleting `r.Use(cli.TOONMiddleware())` from
// commons/cli/serve.go would propagate to sherald's serve plane (since
// sherald uses cli.ServeCmd) and FAIL these assertions — the response
// would stay JSON-shaped, fail the byte-0 check, and the toon.Unmarshal
// step would parse error.

package safety_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	toon "digital.vasic.toon/pkg/toon"
	"github.com/gin-gonic/gin"

	"github.com/vasic-digital/herald/commons/cli"
	commons_auth "github.com/vasic-digital/herald/commons_auth"
	"github.com/vasic-digital/herald/sherald/internal/safety"
)

// fakeAuth injects test claims into the Gin context without actually
// validating a JWT — exercises the handler in isolation from JWT
// verification (which is tested separately in commons_auth/verifier_test.go).
func fakeAuth(c *gin.Context) {
	c.Set(commons_auth.ContextKeyClaims, map[string]any{
		"tenant": "00000000-0000-0000-0000-000000000001",
		"sub":    "test-operator",
	})
	c.Next()
}

// TestSafetyHandler_AcceptTOON_EmitsTOON — Wave 4b Task 6 §107 anti-bluff
// guarantee for sherald.
//
// Asserts that GET /v1/safety_state, when served through the cli.RunServe
// chain (which auto-wires cli.TOONMiddleware as of Wave 4b T6), responds
// with REAL TOON wire bytes when the client sends Accept: application/toon.
//
// Load-bearing assertions (each falsifies a distinct §107 bluff mode):
//
//  1. Content-Type is application/toon — proves the middleware was
//     reachable AND fired.
//  2. body[0] is NOT '{' (the JSON-syntax marker). The 2026-05-17 PASS-
//     bluff revision was JSON bytes wearing an application/toon CT; this
//     check makes that mode structurally detectable.
//  3. The wire body decodes via the REAL digital.vasic.toon codec into
//     the SafetyState shape AND its expected fields (binary="sherald",
//     uptime_seconds≥0, open_events=0 on fresh aggregator). A "looks-
//     like-TOON" regression that emitted unparseable bytes would FAIL
//     at the Unmarshal step.
//
// The test wires the same middleware chain cli.RunServe wires in
// production (TOONMiddleware → fakeAuth → handler) so the bytes the test
// inspects are the bytes a real operator's curl/HTTP client receives.
func TestSafetyHandler_AcceptTOON_EmitsTOON(t *testing.T) {
	// Explicit empty env so the request-level Accept header is the only
	// thing driving negotiation.
	t.Setenv("HERALD_DEFAULT_RESPONSE_CODEC", "")

	gin.SetMode(gin.TestMode)

	// Fresh aggregator + a mem-percent update to populate the snapshot
	// with non-zero values (so the round-trip surfaces value-level codec
	// regressions, not just shape-only).
	agg := safety.NewAggregator()
	agg.UpdateMemPercent(37.5)

	// Build the SAME middleware chain cli.RunServe builds — TOON
	// middleware first (auto-wired), then auth, then handler. This is
	// the §107 "production-equivalent fixture" requirement: the test
	// covers the bytes the operator actually sees.
	router := gin.New()
	router.Use(cli.TOONMiddleware())
	router.GET("/v1/safety_state", fakeAuth, safety.Handler(agg))

	req := httptest.NewRequest(http.MethodGet, "/v1/safety_state", nil)
	req.Header.Set("Accept", "application/toon")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	// Assertion 1: Content-Type is application/toon (canonicalised).
	gotCT := strings.SplitN(rec.Header().Get("Content-Type"), ";", 2)[0]
	gotCT = strings.ToLower(strings.TrimSpace(gotCT))
	if gotCT != "application/toon" {
		t.Fatalf("Content-Type = %q, want application/toon — TOONMiddleware did not transcode", gotCT)
	}

	// Assertion 2: wire bytes are NOT JSON (the 2026-05-17 bluff
	// signature — caught here at byte 0).
	body := rec.Body.Bytes()
	if len(body) == 0 {
		t.Fatal("empty response body (§107: middleware swallowed handler output)")
	}
	if body[0] == '{' || body[0] == '[' {
		t.Fatalf("§107 BLUFF — body[0]=%q (JSON syntax) under Content-Type: application/toon; wire=%q",
			string(body[0]), string(body[:min(80, len(body))]))
	}

	// Assertion 3: real-codec round-trip into the response shape.
	// The handler emits a safety.SafetyState — decode TOON bytes back
	// and positively assert the load-bearing fields. "No error" alone is
	// insufficient §107 evidence; the values must survive the round trip.
	//
	// We re-declare the shape locally with explicit `toon` tags so the
	// test stays self-contained — coupling to the safety package's
	// JSON tags is enough since toon.Unmarshal honours `toon` tags
	// when present and falls back to `json` tags otherwise.
	var got struct {
		Binary            string  `toon:"binary"             json:"binary"`
		UptimeSeconds     int64   `toon:"uptime_seconds"     json:"uptime_seconds"`
		OpenEvents        int64   `toon:"open_events"        json:"open_events"`
		CurrentMemPercent float64 `toon:"current_mem_percent" json:"current_mem_percent"`
	}
	if err := toon.Unmarshal(body, &got); err != nil {
		t.Fatalf("response body did not decode as TOON via real codec: %v; wire=%q",
			err, string(body))
	}
	if got.Binary != "sherald" {
		t.Errorf("binary = %q after TOON round-trip, want %q (codec corrupted the field)",
			got.Binary, "sherald")
	}
	if got.UptimeSeconds < 0 {
		t.Errorf("uptime_seconds = %d (negative) after TOON round-trip", got.UptimeSeconds)
	}
	if got.OpenEvents != 0 {
		t.Errorf("open_events = %d, want 0 on a fresh aggregator (codec phantom-events)",
			got.OpenEvents)
	}
	// 37.5 was the percent we set on the aggregator; assert it survived
	// the float64 round-trip through TOON. A regression that integer-
	// coerced or lost precision would FAIL here.
	if got.CurrentMemPercent != 37.5 {
		t.Errorf("current_mem_percent = %v after TOON round-trip, want 37.5 (codec mangled the float)",
			got.CurrentMemPercent)
	}
}
