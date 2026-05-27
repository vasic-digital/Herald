package compliance_test

// HRD-019 — POST /v1/compliance/evaluate handler tests (TDD RED first).
//
// These drive the REAL bindings.Pipeline (REAL emitter + REAL store + REAL
// ladder + REAL audit) through the production Gin handler, then assert the
// violation is visible on GET /v1/compliance — the bidirectional REST
// round-trip that is HRD-019's §107.x evidence anchor. Only the EXTERNAL
// boundary (JWT verification) is faked via fakeAuth, identical to the existing
// handler_test.go seam.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/vasic-digital/herald/cherald/internal/bindings"
	"github.com/vasic-digital/herald/cherald/internal/compliance"
	constitution "github.com/vasic-digital/herald/commons_constitution"
	"github.com/vasic-digital/herald/commons_constitution/ladder"
	"github.com/vasic-digital/herald/commons_constitution/state"
)

// evalFixture builds a Pipeline + the shared store so a test can both evaluate
// (POST) and read back (GET /v1/compliance) against the SAME store.
func evalFixture(t *testing.T) (*bindings.Pipeline, constitution.ConstitutionStore, constitution.ModeLadder) {
	t.Helper()
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{BufferSize: 128})
	t.Cleanup(func() { _ = bus.Close() })
	em, err := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "digital.vasic.herald/cherald"})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}
	st := state.NewMemory()
	la := ladder.NewMemory()
	au := state.NewMemoryAudit()
	p, err := bindings.NewPipeline(bindings.Config{Ladder: la, Store: st, Emitter: em, Audit: au})
	if err != nil {
		t.Fatalf("NewPipeline: %v", err)
	}
	return p, st, la
}

// TestEvaluateHandler_ViolationThenVisibleOnPull is the §107.x bidirectional
// round-trip: POST a §11.4.29 naming violation → 200 with decision=deny +
// emitted=true → GET /v1/compliance shows the persisted row. This is the
// auditable runtime evidence that an end user can actually exercise the binding
// surface end-to-end.
func TestEvaluateHandler_ViolationThenVisibleOnPull(t *testing.T) {
	gin.SetMode(gin.TestMode)
	p, store, la := evalFixture(t)
	tid := uuid.New()
	// §11.4.29 defaults to warn; flip to enforce so the emit fires.
	if err := la.Set(context.Background(), tid, "§11.4.29", constitution.ModeEnforce, "op"); err != nil {
		t.Fatalf("ladder Set: %v", err)
	}

	r := gin.New()
	r.POST("/v1/compliance/evaluate", fakeAuth(tid.String()), compliance.EvaluateHandler(p))
	r.GET("/v1/compliance", fakeAuth(tid.String()), compliance.Handler(store))

	// POST the violation.
	reqBody, _ := json.Marshal(map[string]string{
		"rule_id":      "§11.4.29",
		"subject_kind": "file",
		"subject_id":   "commons_messaging/BadName.go",
	})
	req := httptest.NewRequest("POST", "/v1/compliance/evaluate", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("evaluate status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var ev struct {
		Decision string `json:"decision"`
		Mode     string `json:"mode"`
		Emitted  bool   `json:"emitted"`
		Audited  bool   `json:"audited"`
		Changed  bool   `json:"changed"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &ev); err != nil {
		t.Fatalf("unmarshal evaluate body: %v; raw=%s", err, rec.Body.String())
	}
	if ev.Decision != "deny" {
		t.Errorf("decision = %q, want deny (FAIL)", ev.Decision)
	}
	if ev.Mode != "enforce" {
		t.Errorf("mode = %q, want enforce", ev.Mode)
	}
	if !ev.Emitted || !ev.Audited || !ev.Changed {
		t.Errorf("emitted/audited/changed = %v/%v/%v, want all true", ev.Emitted, ev.Audited, ev.Changed)
	}

	// GET /v1/compliance — the violation must be queryable.
	getReq := httptest.NewRequest("GET", "/v1/compliance?rule_id=§11.4.29", nil)
	getRec := httptest.NewRecorder()
	r.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("pull status = %d, want 200; body=%s", getRec.Code, getRec.Body.String())
	}
	var pull struct {
		Total   int `json:"total"`
		Results []struct {
			RuleID   string `json:"rule_id"`
			Subject  string `json:"subject"`
			Decision string `json:"decision"`
		} `json:"results"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &pull); err != nil {
		t.Fatalf("unmarshal pull body: %v; raw=%s", err, getRec.Body.String())
	}
	if pull.Total != 1 || len(pull.Results) != 1 {
		t.Fatalf("expected 1 row on pull, got total=%d results=%d", pull.Total, len(pull.Results))
	}
	if pull.Results[0].RuleID != "§11.4.29" {
		t.Errorf("pull rule_id = %q, want §11.4.29", pull.Results[0].RuleID)
	}
	if pull.Results[0].Subject != "commons_messaging/BadName.go" {
		t.Errorf("pull subject = %q, want the violating path", pull.Results[0].Subject)
	}
	if pull.Results[0].Decision != "deny" {
		t.Errorf("pull decision = %q, want deny", pull.Results[0].Decision)
	}
}

// TestEvaluateHandler_CleanSubjectPasses proves a compliant subject yields a
// PASS (decision=pass) and does not emit — no false positive.
func TestEvaluateHandler_CleanSubjectPasses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	p, _, _ := evalFixture(t)
	tid := uuid.New()
	r := gin.New()
	r.POST("/v1/compliance/evaluate", fakeAuth(tid.String()), compliance.EvaluateHandler(p))

	reqBody, _ := json.Marshal(map[string]string{
		"rule_id":    "§11.4.29",
		"subject_id": "commons_messaging/good_name.go",
	})
	req := httptest.NewRequest("POST", "/v1/compliance/evaluate", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var ev struct {
		Decision string `json:"decision"`
		Emitted  bool   `json:"emitted"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &ev)
	if ev.Decision != "pass" {
		t.Errorf("compliant file got decision=%q, want pass (false positive)", ev.Decision)
	}
}

// TestEvaluateHandler_UnknownRule_400 proves a typo'd rule fails fast with 400
// naming the field — never a silent 200.
func TestEvaluateHandler_UnknownRule_400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	p, _, _ := evalFixture(t)
	r := gin.New()
	r.POST("/v1/compliance/evaluate", fakeAuth(uuid.New().String()), compliance.EvaluateHandler(p))

	reqBody, _ := json.Marshal(map[string]string{"rule_id": "§nope", "subject_id": "x"})
	req := httptest.NewRequest("POST", "/v1/compliance/evaluate", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown rule status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "rule_id") {
		t.Errorf("error body must name rule_id field; got %s", rec.Body.String())
	}
}

// TestEvaluateHandler_NoAuth_401 proves the §107 fail-closed posture.
func TestEvaluateHandler_NoAuth_401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	p, _, _ := evalFixture(t)
	r := gin.New()
	r.POST("/v1/compliance/evaluate", compliance.EvaluateHandler(p)) // NO fakeAuth

	reqBody, _ := json.Marshal(map[string]string{"rule_id": "§11.4.29", "subject_id": "x.go"})
	req := httptest.NewRequest("POST", "/v1/compliance/evaluate", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no-auth status = %d, want 401; body=%s", rec.Code, rec.Body.String())
	}
}

// TestEvaluateHandler_EmptySubject_400 proves an empty subject_id is rejected.
func TestEvaluateHandler_EmptySubject_400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	p, _, _ := evalFixture(t)
	r := gin.New()
	r.POST("/v1/compliance/evaluate", fakeAuth(uuid.New().String()), compliance.EvaluateHandler(p))

	reqBody, _ := json.Marshal(map[string]string{"rule_id": "§11.4.29", "subject_id": ""})
	req := httptest.NewRequest("POST", "/v1/compliance/evaluate", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty subject status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "subject_id") {
		t.Errorf("error body must name subject_id; got %s", rec.Body.String())
	}
}
