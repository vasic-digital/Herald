package compliance_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/vasic-digital/herald/cherald/internal/compliance"
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
