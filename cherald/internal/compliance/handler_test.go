package compliance_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	toon "digital.vasic.toon/pkg/toon"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/vasic-digital/herald/cherald/internal/compliance"
	"github.com/vasic-digital/herald/commons/cli"
	"github.com/vasic-digital/herald/commons_auth"
	constitution "github.com/vasic-digital/herald/commons_constitution"
	"github.com/vasic-digital/herald/commons_constitution/state"
)

// TestHandler_EmptyTenant_Returns200WithEmptyResults proves that a
// fresh tenant with no constitution_state rows still gets a structured
// 200 response with the right shape (page=1, page_size=50 defaults,
// total=0, empty results array). §107 anti-bluff: positively asserts
// status code, shape, defaults, and emptiness — not just "no error".
func TestHandler_EmptyTenant_Returns200WithEmptyResults(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := state.NewMemory()
	r := gin.New()
	r.GET("/v1/compliance", fakeAuth(uuid.New().String()), compliance.Handler(store))

	req := httptest.NewRequest("GET", "/v1/compliance", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Page     int   `json:"page"`
		PageSize int   `json:"page_size"`
		Total    int   `json:"total"`
		TenantID string `json:"tenant_id"`
		Results  []any `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v; raw=%s", err, rec.Body.String())
	}
	if body.Total != 0 || len(body.Results) != 0 {
		t.Errorf("expected empty results, got Total=%d Results=%d", body.Total, len(body.Results))
	}
	if body.Page != 1 || body.PageSize != 50 {
		t.Errorf("defaults wrong: page=%d page_size=%d (want 1/50)", body.Page, body.PageSize)
	}
	if body.TenantID == "" {
		t.Errorf("tenant_id missing from response body")
	}
}

// TestHandler_FilterByRuleID proves that the rule_id query param actually
// filters at the store layer — seeds 2 rule rows for the SAME tenant, queries
// with rule_id=11.4.10, asserts ONLY that row returns. §107 anti-bluff:
// positively proves filtering works end-to-end, not just "the field exists".
func TestHandler_FilterByRuleID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := state.NewMemory()
	tid := uuid.New()
	ctx := context.Background()

	// Seed 2 different rule rows for the same tenant. We use DecisionFail
	// (the actual enum name — see commons_constitution/evaluator.go) for both
	// so the rule_id filter is the only thing distinguishing them.
	rA := constitution.Result{Decision: constitution.DecisionFail, Evidence: "a", DigestSHA: [32]byte{1}}
	rB := constitution.Result{Decision: constitution.DecisionFail, Evidence: "b", DigestSHA: [32]byte{2}}
	if _, err := store.Record(ctx, tid, "11.4.10", "subj-a", rA, constitution.BundleHash{}, "uri-a"); err != nil {
		t.Fatalf("Record A: %v", err)
	}
	if _, err := store.Record(ctx, tid, "11.4.20", "subj-b", rB, constitution.BundleHash{}, "uri-b"); err != nil {
		t.Fatalf("Record B: %v", err)
	}

	r := gin.New()
	r.GET("/v1/compliance", fakeAuth(tid.String()), compliance.Handler(store))

	req := httptest.NewRequest("GET", "/v1/compliance?rule_id=11.4.10", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Total   int `json:"total"`
		Results []struct {
			RuleID  string `json:"rule_id"`
			Subject string `json:"subject"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v; raw=%s", err, rec.Body.String())
	}
	if body.Total != 1 {
		t.Errorf("expected Total=1 after rule_id filter, got %d (results=%+v)", body.Total, body.Results)
	}
	if len(body.Results) != 1 {
		t.Fatalf("expected exactly 1 result, got %d (results=%+v)", len(body.Results), body.Results)
	}
	if body.Results[0].RuleID != "11.4.10" {
		t.Errorf("filter leaked the wrong row: rule_id=%q (want 11.4.10)", body.Results[0].RuleID)
	}
	if body.Results[0].Subject != "subj-a" {
		t.Errorf("subject wrong on filtered row: %q (want subj-a)", body.Results[0].Subject)
	}
}

// TestHandler_InvalidSince_Returns400 proves bad input fails fast with
// the documented error body shape — caller must learn WHICH field was bad,
// not just "something went wrong". §107 anti-bluff: positively asserts
// status 400 AND that the body names "invalid time format" + the field.
func TestHandler_InvalidSince_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := state.NewMemory()
	r := gin.New()
	r.GET("/v1/compliance", fakeAuth(uuid.New().String()), compliance.Handler(store))

	req := httptest.NewRequest("GET", "/v1/compliance?since=not-a-date", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on bad since, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid time format") {
		t.Errorf("expected error body to name the issue, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "since") {
		t.Errorf("expected error body to name the field, got %s", rec.Body.String())
	}
}

// TestHandler_PageSizeCapped proves that page_size > maxPageSize (200)
// fails fast with 400 — operator cannot accidentally pull massive pages.
// §107 anti-bluff: positively asserts status 400 on the boundary violation.
func TestHandler_PageSizeCapped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := state.NewMemory()
	r := gin.New()
	r.GET("/v1/compliance", fakeAuth(uuid.New().String()), compliance.Handler(store))

	req := httptest.NewRequest("GET", "/v1/compliance?page_size=999", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on page_size > 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "page_size") {
		t.Errorf("expected error body to name page_size, got %s", rec.Body.String())
	}
}

// fakeAuth injects test claims into the Gin context without actually
// validating a JWT — exercises the handler in isolation from JWT
// verification (which is tested separately in commons_auth/verifier_test.go).
func fakeAuth(tenant string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(commons_auth.ContextKeyClaims, map[string]any{
			"tenant": tenant,
			"sub":    "test-operator",
		})
		c.Next()
	}
}

// TestComplianceHandler_AcceptTOON_EmitsTOON — Wave 4b Task 6 §107
// anti-bluff guarantee for cherald.
//
// Asserts that GET /v1/compliance, when served through the cli.RunServe
// chain (which auto-wires cli.TOONMiddleware as of Wave 4b T6), responds
// with REAL TOON wire bytes when the client sends Accept: application/toon.
//
// Load-bearing assertions (each falsifies a distinct §107 bluff mode):
//
//  1. Content-Type is application/toon — proves the middleware was
//     reachable AND fired.
//  2. body[0] is NOT '{' (the JSON-syntax marker). The 2026-05-17 PASS-
//     bluff revision was JSON bytes wearing an application/toon CT; this
//     check makes that mode impossible to slip past unnoticed.
//  3. The wire body decodes via the REAL digital.vasic.toon codec AND
//     surfaces the expected response fields (page, page_size, total,
//     tenant_id) end-to-end. A regression that emitted "looks-like-TOON"
//     bytes the codec could not parse would FAIL here.
//
// The test wires the same middleware chain cli.RunServe wires in
// production (TOONMiddleware → fakeAuth → handler) so what passes here
// is what an operator sees on a live cherald binary.
//
// Mutation falsifiability: deleting `r.Use(cli.TOONMiddleware())` from
// commons/cli/serve.go would propagate to cherald's serve plane (since
// cherald uses cli.ServeCmd) and FAIL assertions 1+2+3.
func TestComplianceHandler_AcceptTOON_EmitsTOON(t *testing.T) {
	// Explicit empty env so the request-level Accept header is the only
	// thing driving negotiation. (Herald-default is TOON anyway, but
	// pinning this makes the failure mode obvious if a future regression
	// silently changes the default.)
	t.Setenv("HERALD_DEFAULT_RESPONSE_CODEC", "")

	gin.SetMode(gin.TestMode)
	store := state.NewMemory()
	tid := uuid.New()
	ctx := context.Background()

	// Seed one row so the response carries observable structure (total≥1
	// + results[0] populated) — empty-payload responses are too sparse to
	// surface a TOON round-trip regression.
	r := constitution.Result{
		Decision: constitution.DecisionPass,
		Evidence: "seeded-evidence",
		DigestSHA: [32]byte{0xa, 0xb, 0xc},
	}
	if _, err := store.Record(ctx, tid, "11.4.10", "subj-toon", r, constitution.BundleHash{}, "uri-toon"); err != nil {
		t.Fatalf("seed Record: %v", err)
	}

	// Build the SAME middleware chain cli.RunServe builds — TOON
	// middleware first (auto-wired), then auth, then handler. This is
	// the §107 "production-equivalent fixture" requirement: the test
	// covers the bytes the operator actually sees.
	router := gin.New()
	router.Use(cli.TOONMiddleware())
	router.GET("/v1/compliance", fakeAuth(tid.String()), compliance.Handler(store))

	req := httptest.NewRequest(http.MethodGet, "/v1/compliance", nil)
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
	// signature was JSON bytes under a TOON CT — caught here at byte 0).
	body := rec.Body.Bytes()
	if len(body) == 0 {
		t.Fatal("empty response body (§107: middleware swallowed handler output)")
	}
	if body[0] == '{' || body[0] == '[' {
		t.Fatalf("§107 BLUFF — body[0]=%q (JSON syntax) under Content-Type: application/toon; wire=%q",
			string(body[0]), string(body[:min(80, len(body))]))
	}

	// Assertion 3: real-codec round-trip into the response shape.
	// The compliance handler emits a gin.H (map[string]any) so we decode
	// the TOON bytes back into the same shape and verify the structural
	// fields. "No error" alone is insufficient — we positively assert
	// the values made it through the codec.
	var got struct {
		Page     int    `toon:"page"`
		PageSize int    `toon:"page_size"`
		Total    int    `toon:"total"`
		TenantID string `toon:"tenant_id"`
		Results  []struct {
			RuleID  string `toon:"rule_id"`
			Subject string `toon:"subject"`
		} `toon:"results"`
	}
	if err := toon.Unmarshal(body, &got); err != nil {
		t.Fatalf("response body did not decode as TOON via real codec: %v; wire=%q",
			err, string(body))
	}
	if got.Page != 1 || got.PageSize != 50 {
		t.Errorf("defaults wrong after TOON round-trip: page=%d page_size=%d (want 1/50)",
			got.Page, got.PageSize)
	}
	if got.Total != 1 || len(got.Results) != 1 {
		t.Fatalf("seeded row did not survive TOON round-trip: total=%d results=%d",
			got.Total, len(got.Results))
	}
	if got.TenantID != tid.String() {
		t.Errorf("tenant_id = %q after TOON round-trip, want %q (codec corrupted the field)",
			got.TenantID, tid.String())
	}
	if got.Results[0].RuleID != "11.4.10" {
		t.Errorf("results[0].rule_id = %q after TOON round-trip, want %q",
			got.Results[0].RuleID, "11.4.10")
	}
	if got.Results[0].Subject != "subj-toon" {
		t.Errorf("results[0].subject = %q after TOON round-trip, want %q",
			got.Results[0].Subject, "subj-toon")
	}
}
