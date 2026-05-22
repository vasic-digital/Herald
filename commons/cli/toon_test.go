// Wave 4b Task 4 — Gin TOON middleware anti-bluff tests.
//
// Per §107 / Sixth Law: each test below makes a load-bearing positive
// assertion about WIRE BYTES — not just headers, not "no error", not
// "middleware compiled". The original 2026-05-17 PASS-bluff manifested
// as a `Content-Type: application/toon` response carrying JSON bytes;
// header-only assertions would silently pass on that bug. Tests in this
// file pin the byte-level contract so a future regression cannot slip.
//
// §107 anti-bluff anchors enforced here:
//
//   - TestTOONMiddleware_AcceptTOON_EmitsTOON inspects body[0]; it MUST
//     NOT be `{`. If the middleware regresses to writing JSON bytes
//     under an application/toon CT, this test FAILS at the byte level.
//   - TestTOONMiddleware_RoundTrip_StructEqual encodes a Receipt-shaped
//     struct through the middleware, decodes the wire body via the real
//     TOON codec, and asserts reflect.DeepEqual against the original.
//     "No error" alone is insufficient — the load-bearing invariant is
//     value-level round-trip equality.
//   - TestTOONMiddleware_RoundTrip_SizeSmaller computes BOTH encodings
//     explicitly and asserts TOON < JSON × 0.9 (at least 10% smaller).
//     This proves the codec is doing real work — a header-swap-only bug
//     would FAIL because the body would still be JSON-sized.
//   - TestBindNegotiated_TOONRequestBody_DecodesCorrectly round-trips a
//     POSTed TOON body all the way back to a Go struct via the real
//     UnmarshalChosen path and deep-compares. The plain "no error"
//     bluff is structurally impossible.
//
// Mutating toon.go's transcode branch to no-op MUST cause
// TestTOONMiddleware_AcceptTOON_EmitsTOON to FAIL (the response body
// would stay JSON and start with `{`). That is the falsifiability
// contract anchored in the Wave 4b plan §Task 4 §107 column.

package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	toon "digital.vasic.toon/pkg/toon"
	"github.com/gin-gonic/gin"
)

// receiptFixture mirrors the shape of Herald's real Receipt type
// (CloudEvent ack envelope; see commons/types.go). It carries enough
// shape — nested slice, scalar mix — that JSON↔TOON round-trip
// asymmetries (e.g., int↔float64 coercion) surface immediately.
type receiptFixture struct {
	EventID string   `json:"event_id" toon:"event_id"`
	Subject string   `json:"subject"  toon:"subject"`
	Tags    []string `json:"tags"     toon:"tags"`
	Count   int      `json:"count"    toon:"count"`
	Ok      bool     `json:"ok"       toon:"ok"`
}

// sampleReceipt is the canonical fixture for the suite. Stable values
// make assertion-failure messages diff-friendly.
func sampleReceipt() receiptFixture {
	return receiptFixture{
		EventID: "evt_w4b_t4",
		Subject: "toon_middleware_test",
		Tags:    []string{"unit", "anti-bluff"},
		Count:   3,
		Ok:      true,
	}
}

// newTOONRouter builds a Gin engine with TOONMiddleware mounted on /r.
// Centralised so each test names its intent (Accept header, body, …)
// without duplicating the wiring.
func newTOONRouter(handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(TOONMiddleware())
	r.GET("/r", handler)
	r.POST("/r", handler)
	return r
}

// TestTOONMiddleware_AcceptTOON_EmitsTOON — §107 byte-level anti-bluff:
// when Accept is application/toon, the response body MUST NOT start
// with `{` (JSON syntax) AND Content-Type MUST be application/toon.
// A regression that wrote JSON bytes under a TOON CT would FAIL on the
// byte-0 check; a regression that wrote TOON bytes but forgot to update
// the CT header would FAIL on the CT check. Both failure modes are
// distinct §107 bluffs the test pins independently.
func TestTOONMiddleware_AcceptTOON_EmitsTOON(t *testing.T) {
	t.Setenv(EnvDefaultResponseCodec, "")
	r := newTOONRouter(func(c *gin.Context) {
		c.JSON(http.StatusOK, sampleReceipt())
	})

	req := httptest.NewRequest(http.MethodGet, "/r", nil)
	req.Header.Set("Accept", "application/toon")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	body := rec.Body.Bytes()
	if len(body) == 0 {
		t.Fatal("empty response body (§107: middleware swallowed handler output)")
	}
	if body[0] == '{' || body[0] == '[' {
		t.Fatalf("§107 BLUFF — body[0] = %q (JSON syntax); body=%q",
			string(body[0]), string(body[:min(80, len(body))]))
	}

	gotCT := canonicalCT(rec.Header().Get("Content-Type"))
	if gotCT != MediaTypeTOON {
		t.Errorf("Content-Type = %q, want %q (canonicalised)", gotCT, MediaTypeTOON)
	}

	// Defence-in-depth: the body MUST decode via the real TOON codec.
	// If a future regression writes "looks-like-TOON-but-isn't" bytes,
	// this Unmarshal step catches it (no header check can).
	var back receiptFixture
	if err := toon.Unmarshal(body, &back); err != nil {
		t.Fatalf("response body did not decode as TOON: %v; wire=%q", err, string(body))
	}
	if !reflect.DeepEqual(sampleReceipt(), back) {
		t.Fatalf("§107 round-trip failed: orig=%+v back=%+v wire=%q",
			sampleReceipt(), back, string(body))
	}
}

// TestTOONMiddleware_AcceptJSON_EmitsJSON — Accept: application/json
// MUST yield a response whose body starts with `{` (encoding/json struct
// output) and whose Content-Type is application/json. The middleware
// MUST honour an explicit JSON Accept even though Herald's default is
// TOON.
func TestTOONMiddleware_AcceptJSON_EmitsJSON(t *testing.T) {
	t.Setenv(EnvDefaultResponseCodec, "")
	r := newTOONRouter(func(c *gin.Context) {
		c.JSON(http.StatusOK, sampleReceipt())
	})

	req := httptest.NewRequest(http.MethodGet, "/r", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.Bytes()
	if len(body) == 0 {
		t.Fatal("empty response body")
	}
	if body[0] != '{' {
		t.Fatalf("body[0] = %q, want '{' (encoding/json struct output); wire=%q",
			string(body[0]), string(body[:min(80, len(body))]))
	}
	gotCT := canonicalCT(rec.Header().Get("Content-Type"))
	if gotCT != MediaTypeJSON {
		t.Errorf("Content-Type = %q, want %q (canonicalised)", gotCT, MediaTypeJSON)
	}

	var back receiptFixture
	if err := json.Unmarshal(body, &back); err != nil {
		t.Fatalf("response body did not decode as JSON: %v; wire=%q", err, string(body))
	}
	if !reflect.DeepEqual(sampleReceipt(), back) {
		t.Fatalf("JSON round-trip failed: orig=%+v back=%+v", sampleReceipt(), back)
	}
}

// TestTOONMiddleware_DefaultPrefersTOON — no Accept header AND empty
// HERALD_DEFAULT_RESPONSE_CODEC env (i.e., Herald's compiled default)
// MUST emit TOON. This pins W4b operator decision 1 (TOON is the
// Herald-default codec) at the middleware layer.
func TestTOONMiddleware_DefaultPrefersTOON(t *testing.T) {
	t.Setenv(EnvDefaultResponseCodec, "")
	r := newTOONRouter(func(c *gin.Context) {
		c.JSON(http.StatusOK, sampleReceipt())
	})

	req := httptest.NewRequest(http.MethodGet, "/r", nil)
	// Intentionally NO Accept header.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.Bytes()
	if len(body) == 0 {
		t.Fatal("empty response body")
	}
	if body[0] == '{' || body[0] == '[' {
		t.Fatalf("§107 BLUFF — no-Accept default did not transcode to TOON; body[0]=%q wire=%q",
			string(body[0]), string(body[:min(80, len(body))]))
	}
	gotCT := canonicalCT(rec.Header().Get("Content-Type"))
	if gotCT != MediaTypeTOON {
		t.Errorf("default Content-Type = %q, want %q", gotCT, MediaTypeTOON)
	}
}

// TestTOONMiddleware_DefaultEnvJSON_PrefersJSON — operator opt-out via
// HERALD_DEFAULT_RESPONSE_CODEC=json flips the no-Accept default to
// JSON. Pins W4b operator decision 3 at the middleware layer.
func TestTOONMiddleware_DefaultEnvJSON_PrefersJSON(t *testing.T) {
	t.Setenv(EnvDefaultResponseCodec, "json")
	r := newTOONRouter(func(c *gin.Context) {
		c.JSON(http.StatusOK, sampleReceipt())
	})

	req := httptest.NewRequest(http.MethodGet, "/r", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.Bytes()
	if body[0] != '{' {
		t.Fatalf("env=json + no-Accept did not stay JSON; body[0]=%q", string(body[0]))
	}
	if got := canonicalCT(rec.Header().Get("Content-Type")); got != MediaTypeJSON {
		t.Errorf("env=json + no-Accept Content-Type = %q, want %q", got, MediaTypeJSON)
	}
}

// TestBindNegotiated_TOONRequestBody_DecodesCorrectly — POST a TOON body
// with Content-Type: application/toon; BindNegotiated MUST populate the
// destination struct via the real TOON codec (deep-equal vs the
// original). The §107 anti-bluff anchor is the round-trip equality
// assertion — "no error" alone is insufficient.
func TestBindNegotiated_TOONRequestBody_DecodesCorrectly(t *testing.T) {
	t.Setenv(EnvDefaultResponseCodec, "")
	original := sampleReceipt()
	toonBody, err := toon.Marshal(original)
	if err != nil {
		t.Fatalf("toon.Marshal fixture: %v", err)
	}
	// Sanity: bytes really are TOON, not JSON.
	if toonBody[0] == '{' || toonBody[0] == '[' {
		t.Fatalf("§107 fixture-builder bluff — toon.Marshal returned JSON-looking bytes; first 40=%q",
			string(toonBody[:min(40, len(toonBody))]))
	}

	var got receiptFixture
	captured := func(c *gin.Context) {
		if err := BindNegotiated(c, &got); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"err": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
	r := newTOONRouter(captured)

	req := httptest.NewRequest(http.MethodPost, "/r", io.NopCloser(bytes.NewReader(toonBody)))
	req.Header.Set("Content-Type", "application/toon")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !reflect.DeepEqual(original, got) {
		t.Fatalf("§107 BindNegotiated round-trip failed:\n  orig=%+v\n  got =%+v", original, got)
	}
}

// TestBindNegotiated_JSONRequestBody_DecodesCorrectly — POST a JSON body
// with Content-Type: application/json; BindNegotiated MUST populate the
// destination struct via encoding/json (deep-equal vs the original).
func TestBindNegotiated_JSONRequestBody_DecodesCorrectly(t *testing.T) {
	t.Setenv(EnvDefaultResponseCodec, "")
	original := sampleReceipt()
	jsonBody, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal fixture: %v", err)
	}
	if jsonBody[0] != '{' {
		t.Fatalf("fixture-builder error — JSON struct did not start with '{': %q", string(jsonBody[:min(40, len(jsonBody))]))
	}

	var got receiptFixture
	captured := func(c *gin.Context) {
		if err := BindNegotiated(c, &got); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"err": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
	r := newTOONRouter(captured)

	req := httptest.NewRequest(http.MethodPost, "/r", io.NopCloser(bytes.NewReader(jsonBody)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !reflect.DeepEqual(original, got) {
		t.Fatalf("BindNegotiated JSON round-trip failed:\n  orig=%+v\n  got =%+v", original, got)
	}
}

// TestTOONMiddleware_RoundTrip_StructEqual — end-to-end through the
// HTTP layer. Encode struct via c.JSON, middleware transcodes to TOON,
// decode the wire body via the real TOON codec, assert reflect.DeepEqual
// against the original. This is the canonical §107 round-trip evidence
// the Wave 4b plan calls out as "the load-bearing invariant".
func TestTOONMiddleware_RoundTrip_StructEqual(t *testing.T) {
	t.Setenv(EnvDefaultResponseCodec, "")
	original := sampleReceipt()
	r := newTOONRouter(func(c *gin.Context) {
		c.JSON(http.StatusOK, original)
	})

	req := httptest.NewRequest(http.MethodGet, "/r", nil)
	req.Header.Set("Accept", "application/toon")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.Bytes()
	if body[0] == '{' || body[0] == '[' {
		t.Fatalf("§107 — body[0]=%q (JSON), middleware did not transcode", string(body[0]))
	}

	var back receiptFixture
	if err := toon.Unmarshal(body, &back); err != nil {
		t.Fatalf("toon.Unmarshal on the wire body: %v; wire=%q", err, string(body))
	}
	if !reflect.DeepEqual(original, back) {
		t.Fatalf("§107 round-trip failed:\n  orig=%+v\n  back=%+v\n  wire=%q",
			original, back, string(body))
	}
}

// TestTOONMiddleware_RoundTrip_SizeSmaller — TOON-encoded response MUST
// be measurably smaller than the JSON form of the same payload. Asserts
// at least 10% reduction (matches the W4b plan target; T2's observation
// on Receipt-shaped payloads showed ~35%).
//
// §107 anti-bluff: a "header swap only" regression would FAIL this test
// — the body would still be JSON-sized. A bogus encoder that emitted
// shorter gibberish would FAIL the round-trip test above. Both bluffs
// are excluded.
func TestTOONMiddleware_RoundTrip_SizeSmaller(t *testing.T) {
	t.Setenv(EnvDefaultResponseCodec, "")
	// A multi-row tabular payload — TOON's compaction sweet spot per
	// upstream docs. The W4b plan's reference fixture uses a similar
	// shape; we mirror it here so the size comparison is meaningful.
	tabular := map[string]any{
		"results": []map[string]any{
			{"rule_id": "11.4.10", "subject": "user-alice", "decision": "pass", "transitioned_at": "2026-05-22T12:00:00Z"},
			{"rule_id": "11.4.10", "subject": "user-bob", "decision": "pass", "transitioned_at": "2026-05-22T12:00:01Z"},
			{"rule_id": "11.4.10", "subject": "user-carol", "decision": "warn", "transitioned_at": "2026-05-22T12:00:02Z"},
			{"rule_id": "11.4.10", "subject": "user-dave", "decision": "deny", "transitioned_at": "2026-05-22T12:00:03Z"},
			{"rule_id": "11.4.10", "subject": "user-eve", "decision": "pass", "transitioned_at": "2026-05-22T12:00:04Z"},
		},
	}

	// Compute the JSON baseline via a parallel request that explicitly
	// asks for JSON. Using the same router + payload guarantees the
	// comparison is apples-to-apples (no fixture-builder skew).
	r := newTOONRouter(func(c *gin.Context) {
		c.JSON(http.StatusOK, tabular)
	})

	jsonReq := httptest.NewRequest(http.MethodGet, "/r", nil)
	jsonReq.Header.Set("Accept", "application/json")
	jsonRec := httptest.NewRecorder()
	r.ServeHTTP(jsonRec, jsonReq)
	jsonBytes := jsonRec.Body.Bytes()
	if jsonBytes[0] != '{' {
		t.Fatalf("JSON baseline body[0]=%q, want '{'", string(jsonBytes[0]))
	}

	toonReq := httptest.NewRequest(http.MethodGet, "/r", nil)
	toonReq.Header.Set("Accept", "application/toon")
	toonRec := httptest.NewRecorder()
	r.ServeHTTP(toonRec, toonReq)
	toonBytes := toonRec.Body.Bytes()
	if toonBytes[0] == '{' || toonBytes[0] == '[' {
		t.Fatalf("§107 — TOON body[0]=%q (JSON-looking); middleware did not transcode", string(toonBytes[0]))
	}

	if len(toonBytes) >= len(jsonBytes) {
		t.Fatalf("§107 — TOON (%d B) not smaller than JSON (%d B); codec is not providing real benefit",
			len(toonBytes), len(jsonBytes))
	}

	ratio := float64(len(toonBytes)) / float64(len(jsonBytes))
	if ratio > 0.90 {
		t.Fatalf("TOON/JSON ratio = %.3f (toon=%d B, json=%d B); want ≤ 0.90 (at least 10%% smaller, per W4b plan)",
			ratio, len(toonBytes), len(jsonBytes))
	}
	t.Logf("TOON/JSON ratio = %.3f (toon=%d B, json=%d B) — %.1f%% smaller",
		ratio, len(toonBytes), len(jsonBytes), (1-ratio)*100)
}

// TestTOONMiddleware_NonJSONContentType_Passthrough — when a handler
// emits text/plain (e.g., MetricsHandler), the middleware MUST NOT
// transcode. Pins the explicit non-goal documented at the top of
// toon.go: the middleware ONLY transcodes application/json bodies.
func TestTOONMiddleware_NonJSONContentType_Passthrough(t *testing.T) {
	t.Setenv(EnvDefaultResponseCodec, "")
	r := newTOONRouter(func(c *gin.Context) {
		c.Header("Content-Type", "text/plain; charset=utf-8")
		c.String(http.StatusOK, "# HELP build_info\n")
	})

	req := httptest.NewRequest(http.MethodGet, "/r", nil)
	req.Header.Set("Accept", "application/toon") // would normally trigger transcode
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.HasPrefix(body, "# HELP") {
		t.Errorf("text/plain passthrough corrupted: body=%q (expected '# HELP' prefix)", body)
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain prefix (middleware MUST NOT rewrite non-JSON CT)",
			rec.Header().Get("Content-Type"))
	}
}
