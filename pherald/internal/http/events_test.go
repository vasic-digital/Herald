// Wave 3b Task 10 + Wave 4b Task 5 — pherald POST /v1/events handler tests.
//
// Wave 3b coverage: mapErrorToStatus translator + the auth-bypass refusal
// path (TestEventsHandler_MissingClaims).
//
// Wave 4b coverage adds TOON content-type negotiation through the full
// EventsHandler → Runner → Receipt path:
//
//   - Accept: application/toon  → response body starts with non-`{` byte
//     AND Content-Type: application/toon AND the bytes round-trip back to
//     a Receipt struct via the REAL digital.vasic.toon codec (§107
//     byte-level anti-bluff — no header-only assertions).
//   - Content-Type: application/toon (request) → handler transcodes the
//     TOON body to JSON via cli.UnmarshalChosen before the Runner sees it
//     (Option A in the W4b plan §Task 5); the Runner still produces a
//     Receipt with the right event_id (§107 — round-trip evidence proves
//     the transcode worked, not that "no error" was returned).
//   - Both TOON in and TOON out — request body TOON, Accept TOON, end-to-
//     end round-trip equality on event_id + recipients fields.
//   - JSON-only request + JSON Accept response is unchanged from Wave 3b
//     (regression guard for the JSON-only contract many existing
//     integrations rely on).
//
// Tests use runner.NewFakeRunner() (exported test-helper added in W4b T5
// — pherald/internal/runner/runnertest.go) so the full 7-stage Runner
// drives end-to-end with in-memory fakes. The Runner produces a real
// Receipt; assertions are made against the wire bytes the EventsHandler +
// TOONMiddleware emit.
//
// TestMain sets HERALD_DEFAULT_RESPONSE_CODEC="" so the Herald-default
// codec is in play for every test. Tests that need the JSON regression
// path set the Accept header explicitly to application/json (rather than
// flipping the env per-test) — explicit is honest, matches what a real
// client would send.
package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"

	toon "digital.vasic.toon/pkg/toon"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/vasic-digital/herald/commons/cli"
	"github.com/vasic-digital/herald/commons_auth"
	"github.com/vasic-digital/herald/pherald/internal/runner"
)

// TestMain ensures HERALD_DEFAULT_RESPONSE_CODEC is empty for every test
// in this file. Wave 4b TOONMiddleware reads the env at every request,
// so an unset env (Herald-default = TOON) is the canonical starting
// point. Tests that need JSON output set Accept: application/json
// explicitly.
func TestMain(m *testing.M) {
	_ = os.Setenv(cli.EnvDefaultResponseCodec, "")
	os.Exit(m.Run())
}

// Test the mapErrorToStatus translator in isolation. Each Runner stage tags
// its errors with a stable prefix; the handler maps stage-prefix → HTTP
// status. Unknown errors default to 500 so a new error class doesn't
// accidentally leak as 400.
func TestMapErrorToStatus(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"event_parser → 400", errors.New("event_parser: invalid CloudEvent: missing id"), http.StatusBadRequest},
		{"missing tenant claim → 401", errors.New("runner: claims missing 'tenant'"), http.StatusUnauthorized},
		{"tenant not string → 401", errors.New("runner: 'tenant' claim not a string (got int)"), http.StatusUnauthorized},
		{"tenant unparseable → 401", errors.New("runner: parse 'tenant' claim: invalid uuid"), http.StatusUnauthorized},
		{"tenant_resolver → 400", errors.New("tenant_resolver: GUC bind failed"), http.StatusBadRequest},
		{"unknown error → 500", errors.New("policy_gate: redis dead"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mapErrorToStatus(tc.err)
			if got != tc.want {
				t.Errorf("mapErrorToStatus(%q) = %d, want %d", tc.err.Error(), got, tc.want)
			}
		})
	}
}

// TestEventsHandler_MissingClaims proves the handler refuses to process a
// request that wasn't gated by commons_auth.GinMiddleware — even if the
// route was somehow wired without it.
func TestEventsHandler_MissingClaims(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewBufferString("{}"))
	// Do NOT call c.Set(commons_auth.ContextKeyClaims, …) — simulate
	// auth-middleware bypass.

	// Runner can be nil here — the handler short-circuits before calling it.
	EventsHandler(nil)(c)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (no auth claims in context)", rec.Code)
	}
}

// claimsInjector returns a Gin middleware that drops a claims map into the
// context exactly where commons_auth.GinMiddleware would in production.
// Tests use this in lieu of running the real verifier so the focus is
// the handler + TOON middleware behaviour, not JWT validation.
func claimsInjector(tenantID uuid.UUID) gin.HandlerFunc {
	claims := map[string]any{"tenant": tenantID.String()}
	return func(c *gin.Context) {
		c.Set(commons_auth.ContextKeyClaims, claims)
		c.Next()
	}
}

// newTestEngine assembles a Gin engine wired with the exact production
// middleware chain (cli.TOONMiddleware → claims injector → EventsHandler)
// and the in-memory FakeRunner. Mirrors the pherald serve plane at the
// HTTP layer so the tests exercise the same code paths a real client
// would hit.
func newTestEngine(t *testing.T, tenantID uuid.UUID) (*gin.Engine, *runner.FakeStores) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r, stores := runner.NewFakeRunner()
	// Seed one subscriber under the tenant so Stage 5 emits at least one
	// recipient — gives Stage 6/7 something to dispatch + archive, so
	// the Receipt has Recipients=1.
	stores.AddSubscriber(tenantID, "alice", "Alice", "null", "sandbox-alice")

	eng := gin.New()
	eng.Use(cli.TOONMiddleware())
	eng.Use(claimsInjector(tenantID))
	eng.POST("/v1/events", EventsHandler(r))
	return eng, stores
}

// jsonCloudEventBody builds a structured-mode CloudEvent JSON body with a
// fresh event_id (so each test gets a non-replay path). The Herald
// idempotency key defaults to event_id when not set; using uuid.NewString
// here keeps tests independent.
func jsonCloudEventBody(t *testing.T) (string, []byte) {
	t.Helper()
	id := uuid.NewString()
	body, err := json.Marshal(map[string]any{
		"specversion": "1.0",
		"id":          id,
		"source":      "//w4b-t5-test",
		"type":        "digital.vasic.herald.test",
	})
	if err != nil {
		t.Fatalf("marshal cloudevent body: %v", err)
	}
	return id, body
}

// TestEventsHandler_AcceptTOON_ReceiptIsTOON — §107 byte-level anti-bluff
// for the Receipt response codec. POST a JSON CloudEvent body with
// `Accept: application/toon`; the response body MUST:
//
//   - have Content-Type: application/toon (canonicalised)
//   - NOT start with `{` (the JSON-syntax marker; a TOON response that
//     bluffed with a header swap would still emit JSON bytes and FAIL here)
//   - decode back to a Receipt struct via the REAL digital.vasic.toon
//     codec — populated event_id matches what the client POSTed
//     (round-trip equality, not "no error")
func TestEventsHandler_AcceptTOON_ReceiptIsTOON(t *testing.T) {
	tenantID := uuid.New()
	eng, _ := newTestEngine(t, tenantID)

	id, body := jsonCloudEventBody(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/toon")
	rec := httptest.NewRecorder()
	eng.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (fresh event); body=%s", rec.Code, rec.Body.String())
	}

	respBody := rec.Body.Bytes()
	if len(respBody) == 0 {
		t.Fatal("empty response body (§107: TOON-middleware swallowed handler output)")
	}
	if respBody[0] == '{' || respBody[0] == '[' {
		t.Fatalf("§107 BLUFF — Receipt body[0]=%q (JSON syntax) under Accept: application/toon; body=%q",
			string(respBody[0]), string(respBody[:min(120, len(respBody))]))
	}

	gotCT := rec.Header().Get("Content-Type")
	// Match either "application/toon" or "application/toon; charset=utf-8"
	// post-canonicalisation.
	if !startsWithCT(gotCT, "application/toon") {
		t.Errorf("Content-Type = %q, want application/toon prefix", gotCT)
	}

	// §107 round-trip: decode the wire bytes as TOON → Receipt struct →
	// assert event_id matches what we POSTed.
	var got receiptStub
	if err := toon.Unmarshal(respBody, &got); err != nil {
		t.Fatalf("toon.Unmarshal Receipt body: %v; wire=%q", err, string(respBody))
	}
	if got.EventID != id {
		t.Errorf("Receipt.event_id = %q, want %q (TOON round-trip dropped/mangled the field)", got.EventID, id)
	}
	if got.Recipients != 1 {
		t.Errorf("Receipt.recipients = %d, want 1 (subscriber was seeded by newTestEngine)", got.Recipients)
	}
}

// TestEventsHandler_ContentTypeTOON_RequestDecoded — the handler MUST
// transcode an inbound TOON-encoded CloudEvent body to JSON BEFORE the
// Runner sees it (Option A in the W4b plan §Task 5). The client sends
// `Content-Type: application/toon` + TOON bytes; the response (still
// JSON because Accept = application/json) MUST be a valid Receipt with
// event_id matching what the client encoded into the TOON body.
//
// §107 anti-bluff: the inbound TOON body is decoded via the REAL
// digital.vasic.toon codec inside transcodeRequestBody. A "ignore the
// Content-Type and parse as JSON anyway" regression would FAIL at
// event_parser (TOON bytes don't parse as JSON), surfacing as a 400
// rather than the 202 this test asserts.
func TestEventsHandler_ContentTypeTOON_RequestDecoded(t *testing.T) {
	tenantID := uuid.New()
	eng, _ := newTestEngine(t, tenantID)

	id := uuid.NewString()
	// Build the CloudEvent payload + encode it as TOON. Note: the body
	// must round-trip through json+toon — TOON-decode into a map first,
	// then re-encode to JSON inside the handler. The handler does the
	// reverse (TOON → JSON) before handing to the Runner.
	cloudEvent := map[string]any{
		"specversion": "1.0",
		"id":          id,
		"source":      "//w4b-t5-test",
		"type":        "digital.vasic.herald.test",
	}
	toonBody, err := toon.Marshal(cloudEvent)
	if err != nil {
		t.Fatalf("toon.Marshal cloudevent: %v", err)
	}
	if len(toonBody) > 0 && (toonBody[0] == '{' || toonBody[0] == '[') {
		t.Fatalf("§107 fixture builder bluff — toon.Marshal returned JSON-looking bytes (first 40=%q)",
			string(toonBody[:min(40, len(toonBody))]))
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/events", io.NopCloser(bytes.NewReader(toonBody)))
	req.Header.Set("Content-Type", "application/toon")
	req.Header.Set("Accept", "application/json") // JSON response for this test
	rec := httptest.NewRecorder()
	eng.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (fresh event after TOON→JSON transcode); body=%s",
			rec.Code, rec.Body.String())
	}

	// Response is JSON per explicit Accept; sanity-decode and assert
	// event_id round-tripped through the transcode + the Runner.
	respBody := rec.Body.Bytes()
	if respBody[0] != '{' {
		t.Fatalf("Accept=json + Content-Type=toon: response body[0]=%q, want '{' (JSON); body=%q",
			string(respBody[0]), string(respBody[:min(120, len(respBody))]))
	}
	var got receiptStub
	if err := json.Unmarshal(respBody, &got); err != nil {
		t.Fatalf("json.Unmarshal Receipt body: %v; wire=%q", err, string(respBody))
	}
	if got.EventID != id {
		t.Errorf("Receipt.event_id = %q, want %q (TOON→JSON request transcode dropped the field)", got.EventID, id)
	}
}

// TestEventsHandler_BothTOONInAndOut_RoundTrip — full round-trip:
// request body is TOON, response body is TOON. Asserts the Receipt
// recovered from the wire bytes carries the same event_id the client
// encoded into the TOON request body. This is the headline §107
// load-bearing assertion for W4b T5: it would FAIL on any of three
// regressions independently:
//
//   - Request transcode missing → event_parser rejects the TOON bytes (400)
//   - Response transcode missing → response body is JSON, byte-0 check fails
//   - Either codec swap regression → Unmarshal fails or event_id mismatches
func TestEventsHandler_BothTOONInAndOut_RoundTrip(t *testing.T) {
	tenantID := uuid.New()
	eng, _ := newTestEngine(t, tenantID)

	id := uuid.NewString()
	cloudEvent := map[string]any{
		"specversion": "1.0",
		"id":          id,
		"source":      "//w4b-t5-test",
		"type":        "digital.vasic.herald.test",
	}
	toonBody, err := toon.Marshal(cloudEvent)
	if err != nil {
		t.Fatalf("toon.Marshal: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/events", io.NopCloser(bytes.NewReader(toonBody)))
	req.Header.Set("Content-Type", "application/toon")
	req.Header.Set("Accept", "application/toon")
	rec := httptest.NewRecorder()
	eng.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body.String())
	}
	respBody := rec.Body.Bytes()
	if respBody[0] == '{' || respBody[0] == '[' {
		t.Fatalf("§107 — both-TOON round-trip emitted JSON bytes: body[0]=%q wire=%q",
			string(respBody[0]), string(respBody[:min(120, len(respBody))]))
	}
	gotCT := rec.Header().Get("Content-Type")
	if !startsWithCT(gotCT, "application/toon") {
		t.Errorf("Content-Type = %q, want application/toon prefix", gotCT)
	}
	var got receiptStub
	if err := toon.Unmarshal(respBody, &got); err != nil {
		t.Fatalf("toon.Unmarshal Receipt body: %v; wire=%q", err, string(respBody))
	}
	if got.EventID != id {
		t.Errorf("round-trip Receipt.event_id = %q, want %q", got.EventID, id)
	}
	if got.Recipients != 1 {
		t.Errorf("round-trip Receipt.recipients = %d, want 1", got.Recipients)
	}

	// Extra defence-in-depth: prove the Receipt struct extracted from the
	// TOON response is non-zero — a zero-value Receipt smuggled through
	// a TOON header swap would FAIL the equality below.
	if reflect.DeepEqual(got, receiptStub{}) {
		t.Fatalf("§107 — Receipt extracted from TOON wire is zero-valued (header swap bluff?)")
	}
}

// TestEventsHandler_JSONStillWorks_BackwardCompat — Wave 3b regression
// guard. POST a JSON CloudEvent body with explicit `Accept:
// application/json`; the response MUST be a JSON Receipt as it was
// before W4b T4/T5 mounted the TOON middleware. Anything else is a
// covenant breach (existing integrations must keep working).
func TestEventsHandler_JSONStillWorks_BackwardCompat(t *testing.T) {
	tenantID := uuid.New()
	eng, _ := newTestEngine(t, tenantID)

	id, body := jsonCloudEventBody(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	eng.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (fresh event); body=%s", rec.Code, rec.Body.String())
	}
	respBody := rec.Body.Bytes()
	if respBody[0] != '{' {
		t.Fatalf("JSON-only contract broken: response body[0]=%q, want '{'; body=%q",
			string(respBody[0]), string(respBody[:min(120, len(respBody))]))
	}
	gotCT := rec.Header().Get("Content-Type")
	if !startsWithCT(gotCT, "application/json") {
		t.Errorf("Content-Type = %q, want application/json prefix", gotCT)
	}
	var got receiptStub
	if err := json.Unmarshal(respBody, &got); err != nil {
		t.Fatalf("json.Unmarshal Receipt body: %v; wire=%q", err, string(respBody))
	}
	if got.EventID != id {
		t.Errorf("Receipt.event_id = %q, want %q (Wave 3b JSON contract regressed)", got.EventID, id)
	}
}

// receiptStub mirrors the fields of runner.Receipt this test cares about.
// We don't decode into runner.Receipt directly because TOON's int↔float64
// coercion on numeric fields would force us to write field-by-field
// comparisons anyway. A focused stub keeps the assertions diff-friendly
// and side-steps any tag drift between encoding/json and toon's struct
// tag conventions.
type receiptStub struct {
	EventID    string `json:"event_id"    toon:"event_id"`
	Recipients int    `json:"recipients"  toon:"recipients"`
}

// startsWithCT returns true when the canonicalised Content-Type begins
// with the prefix (ignoring case + any trailing `; charset=...`
// parameters Gin appends to JSON responses).
func startsWithCT(got, prefix string) bool {
	// Strip parameters (`; charset=utf-8`, etc.) and lowercase.
	end := len(got)
	for i, r := range got {
		if r == ';' {
			end = i
			break
		}
	}
	bare := got[:end]
	if len(bare) != len(prefix) {
		// Allow any whitespace-trimming Gin may have done.
		for len(bare) > 0 && (bare[len(bare)-1] == ' ' || bare[len(bare)-1] == '\t') {
			bare = bare[:len(bare)-1]
		}
	}
	if len(bare) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		a := bare[i]
		b := prefix[i]
		if a >= 'A' && a <= 'Z' {
			a += 'a' - 'A'
		}
		if b >= 'A' && b <= 'Z' {
			b += 'a' - 'A'
		}
		if a != b {
			return false
		}
	}
	return true
}
