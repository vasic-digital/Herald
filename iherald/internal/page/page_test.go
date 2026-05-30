package page_test

// HRD-024 — POST /v1/webhooks/page live-handler tests (anti-bluff, §107).
//
// These drive the REAL iherald bindings.Pipeline (REAL emitter + REAL store +
// REAL ladder + REAL audit) through the production page.Handler mounted on a
// REAL Gin router via httptest. The load-bearing assertions are NOT
// "status==202" alone — they prove the escalation actually fanned out:
//
//	(A) no bearer token, REAL commons_auth.GinMiddleware → 401 (genuine auth
//	    rejection, not a fake claims-injection bypass);
//	(B) a self-minted valid HS256 token + a credential-leak page body → 202 +
//	    Receipt{emitted:true, audited:true, decision:"deny"} AND a
//	    .credential.leak event observed on the constitution bus (the page-out
//	    fan-out) AND a persisted constitution_state row visible to the store;
//	(C) a malformed body → 400 tagged `event_parser:` (the pherald /v1/events
//	    convention);
//	(D) an unknown rule_id → 400 tagged `event_parser:`;
//	(E) the HERALD_OPERATOR_IDS allow-list rejects an operator outside the list
//	    (403) for an operator-gated rule, and admits one inside it (202).
//
// Only the EXTERNAL boundary is touched per §11.4.27: scenarios that aren't
// specifically exercising the JWT gate inject claims via fakeAuth (the same
// seam cherald/internal/compliance tests use); the 401 scenario AND the
// primary 202 round-trip mount the REAL commons_auth.GinMiddleware over a REAL
// HMAC verifier so the auth path is proven, not assumed.
//
// NO REAL SECRETS: every credential Subject uses a FAKE/synthetic location +
// boolean detection flag. NO real .env is scanned and NO real secret string
// appears anywhere.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/vasic-digital/herald/commons_auth"
	constitution "github.com/vasic-digital/herald/commons_constitution"
	"github.com/vasic-digital/herald/commons_constitution/ladder"
	"github.com/vasic-digital/herald/commons_constitution/state"
	"github.com/vasic-digital/herald/iherald/internal/bindings"
	"github.com/vasic-digital/herald/iherald/internal/page"
)

const testHMACSecret = "iherald-hrd024-page-test-hmac-secret-32b"

// fixture builds a Pipeline + the live backends so a test can both POST a page
// (drive the pipeline) and read back the persisted state row against the SAME
// store. The bus is returned so a test can subscribe and observe the page-out
// emit reaching it.
func fixture(t *testing.T) (*bindings.Pipeline, constitution.ConstitutionStore, constitution.ModeLadder, constitution.EventBus) {
	t.Helper()
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{BufferSize: 128})
	t.Cleanup(func() { _ = bus.Close() })
	em, err := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "digital.vasic.herald/iherald"})
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
	return p, st, la, bus
}

// fakeAuth injects claims for the given tenant — the §11.4.27 boundary stand-in
// for commons_auth.GinMiddleware's JWT verification (identical to the cherald
// compliance handler_test seam). Used by the scenarios NOT specifically
// exercising the auth gate.
func fakeAuth(tenant string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(commons_auth.ContextKeyClaims, map[string]any{
			"tenant": tenant,
			"sub":    "test-operator",
		})
		c.Next()
	}
}

// realAuthRouter mounts the REAL commons_auth.GinMiddleware over a REAL HMAC
// verifier so the JWT gate is genuinely exercised (no claims-injection bypass).
func realAuthRouter(t *testing.T, p *bindings.Pipeline) *gin.Engine {
	t.Helper()
	verifier, err := commons_auth.NewVerifier(commons_auth.Config{
		Mode:           commons_auth.ModeHMAC,
		HMACSecret:     []byte(testHMACSecret),
		RequiredClaims: []string{"tenant"},
	}, nil)
	if err != nil {
		t.Fatalf("NewVerifier(hmac): %v", err)
	}
	r := gin.New()
	r.Use(commons_auth.GinMiddleware(verifier))
	r.POST("/v1/webhooks/page", page.Handler(p))
	return r
}

// mintHMAC produces a valid HS256 bearer token carrying the tenant claim,
// signed with the same secret the realAuthRouter verifier checks. This is the
// in-test analogue of an operator's real JWT — it lets the 202 round-trip run
// THROUGH the genuine auth gate without any fake bypass.
func mintHMAC(t *testing.T, tenant string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"tenant": tenant,
		"sub":    "iherald-page-test",
		"exp":    time.Now().Add(time.Hour).Unix(),
	})
	signed, err := tok.SignedString([]byte(testHMACSecret))
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}
	return signed
}

func postPage(r *gin.Engine, bearer string, body any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/v1/webhooks/page", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// (A) No bearer token + REAL auth middleware → 401. Proves the route is
// genuinely auth-gated; a missing token never reaches the pipeline.
func TestPage_NoToken_Returns401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	p, _, _, _ := fixture(t)
	r := realAuthRouter(t, p)

	rec := postPage(r, "", map[string]string{
		"rule_id":    "§11.4.10",
		"subject_id": "config/fake-fixture|leaked=true|kind=env",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", rec.Code, rec.Body.String())
	}
}

// (B) THE load-bearing anti-bluff round-trip. A valid token + a credential-leak
// page → 202 + Receipt{emitted,audited,decision:deny} AND the .credential.leak
// event reaches the bus AND a constitution_state row is persisted.
func TestPage_CredentialLeak_EmitsAndPersists(t *testing.T) {
	gin.SetMode(gin.TestMode)
	p, store, la, bus := fixture(t)
	tid := uuid.New()

	// §11.4.10 default-enforces per §42.3; set it explicitly so the page-out emit fires.
	if err := la.Set(context.Background(), tid, "§11.4.10", constitution.ModeEnforce, "op"); err != nil {
		t.Fatalf("ladder Set enforce: %v", err)
	}

	// Subscribe to the credential.leak class — the anti-bluff observation that
	// proves the page-out emit reached the bus.
	sub, err := bus.Subscribe(constitution.EventNamespace + "." + constitution.ClassCredentialLeak)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Cancel()
	var leaks int64
	go func() {
		for range sub.Channel {
			atomic.AddInt64(&leaks, 1)
		}
	}()

	r := realAuthRouter(t, p)
	token := mintHMAC(t, tid.String())

	rec := postPage(r, token, map[string]string{
		"rule_id":      "§11.4.10",
		"subject_kind": "credential-leak",
		"subject_id":   "config/fake-fixture|leaked=true|kind=env",
	})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body.String())
	}

	var got page.Receipt
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal receipt: %v; body=%s", err, rec.Body.String())
	}
	if got.Decision != "deny" {
		t.Errorf("decision = %q, want deny (a detected leak)", got.Decision)
	}
	if got.Mode != "enforce" {
		t.Errorf("mode = %q, want enforce", got.Mode)
	}
	if !got.Emitted {
		t.Errorf("emitted = false, want true — the page-out must fan out")
	}
	if !got.Audited {
		t.Errorf("audited = false, want true — the page-out must be audited")
	}
	if !got.Changed || !got.FirstSeen {
		t.Errorf("first page should be a FirstSeen+Changed transition; got changed=%v first_seen=%v", got.Changed, got.FirstSeen)
	}
	if got.Escalation != "credential-leak" {
		t.Errorf("escalation = %q, want credential-leak", got.Escalation)
	}
	if got.TenantID != tid.String() {
		t.Errorf("tenant_id = %q, want %q", got.TenantID, tid.String())
	}
	if got.PagerNote == "" {
		t.Errorf("pager_delivery note missing — the external-egress seam must be disclosed honestly")
	}

	// (1) the page-out emit reached the bus.
	deadline := time.After(2 * time.Second)
	for atomic.LoadInt64(&leaks) < 1 {
		select {
		case <-deadline:
			t.Fatalf("listener never saw the .credential.leak; bus metrics=%+v", bus.Metrics())
		case <-time.After(5 * time.Millisecond):
		}
	}

	// (2) a constitution_state row was persisted (queryable via the store the
	// /v1/webhooks/page pull surface would read).
	rows, err := store.List(context.Background(), tid, constitution.ListQuery{})
	if err != nil {
		t.Fatalf("store.List: %v", err)
	}
	found := false
	for _, row := range rows {
		if row.RuleID == "§11.4.10" {
			found = true
			if row.Decision != constitution.DecisionFail {
				t.Errorf("persisted decision = %v, want fail", row.Decision)
			}
		}
	}
	if !found {
		t.Errorf("no persisted constitution_state row for §11.4.10 — the page-out left no auditable trace (§107 bluff)")
	}
}

// (C) Malformed body → 400 tagged `event_parser:`.
func TestPage_MalformedBody_Returns400EventParser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	p, _, _, _ := fixture(t)
	tid := uuid.New()
	r := gin.New()
	r.POST("/v1/webhooks/page", fakeAuth(tid.String()), page.Handler(p))

	req := httptest.NewRequest("POST", "/v1/webhooks/page", strings.NewReader(`{"rule_id": "§11.4.10", `)) // truncated JSON
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "event_parser:") {
		t.Errorf("body %q missing the `event_parser:` tag (pherald /v1/events convention)", rec.Body.String())
	}
}

// (C2) Missing required field (empty subject_id) → 400 tagged `event_parser:`.
func TestPage_MissingSubject_Returns400EventParser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	p, _, _, _ := fixture(t)
	tid := uuid.New()
	r := gin.New()
	r.POST("/v1/webhooks/page", fakeAuth(tid.String()), page.Handler(p))

	rec := postPage(r, "", map[string]string{"rule_id": "§11.4.10"}) // no subject_id
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "event_parser:") {
		t.Errorf("body %q missing the `event_parser:` tag", rec.Body.String())
	}
}

// (D) Unknown rule_id → 400 tagged `event_parser:`.
func TestPage_UnknownRule_Returns400EventParser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	p, _, _, _ := fixture(t)
	tid := uuid.New()
	r := gin.New()
	r.POST("/v1/webhooks/page", fakeAuth(tid.String()), page.Handler(p))

	rec := postPage(r, "", map[string]string{
		"rule_id":    "§99.99.99-nonexistent",
		"subject_id": "whatever|x=1",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event_parser:") || !strings.Contains(body, "unknown rule") {
		t.Errorf("body %q should tag `event_parser:` + cite unknown rule", body)
	}
}

// (E) HERALD_OPERATOR_IDS allow-list gate for operator-gated rules (§11.4.21).
// An operator outside the list → 403; one inside it → 202.
func TestPage_OperatorAllowList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("HERALD_OPERATOR_IDS", "alice,bob")

	p, _, la, _ := fixture(t)
	tid := uuid.New()
	if err := la.Set(context.Background(), tid, "§11.4.21", constitution.ModeEnforce, "op"); err != nil {
		t.Fatalf("ladder Set: %v", err)
	}
	r := gin.New()
	r.POST("/v1/webhooks/page", fakeAuth(tid.String()), page.Handler(p))

	// Operator NOT in the list → 403, never reaches the pipeline.
	rec := postPage(r, "", map[string]string{
		"rule_id":     "§11.4.21",
		"subject_id":  "HRD-999|status=operator-blocked|oncall_paged=false",
		"operator_id": "mallory",
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-allowlisted operator: status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}

	// Missing operator_id when the list is configured → 403.
	rec = postPage(r, "", map[string]string{
		"rule_id":    "§11.4.21",
		"subject_id": "HRD-999|status=operator-blocked|oncall_paged=false",
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("missing operator_id: status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}

	// Operator IN the list → 202, escalation fires.
	rec = postPage(r, "", map[string]string{
		"rule_id":     "§11.4.21",
		"subject_id":  "HRD-999|status=operator-blocked|oncall_paged=false",
		"operator_id": "alice",
	})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("allowlisted operator: status = %d, want 202; body=%s", rec.Code, rec.Body.String())
	}
	var got page.Receipt
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Decision != "deny" || !got.Emitted {
		t.Errorf("operator-blocked escalation gap should be deny+emitted; got decision=%q emitted=%v", got.Decision, got.Emitted)
	}
	if got.Escalation != "operator-blocked" {
		t.Errorf("escalation = %q, want operator-blocked", got.Escalation)
	}
}

// (E2) When HERALD_OPERATOR_IDS is UNSET, the operator gate is OPEN — an
// operator-gated page with no operator_id still processes (202). This proves
// the gate is opt-in via config, not a hard requirement that would break the
// dev/CI default path.
func TestPage_OperatorGateOpenWhenUnset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Ensure the env is empty for this test.
	t.Setenv("HERALD_OPERATOR_IDS", "")

	p, _, la, _ := fixture(t)
	tid := uuid.New()
	if err := la.Set(context.Background(), tid, "§11.4.21", constitution.ModeEnforce, "op"); err != nil {
		t.Fatalf("ladder Set: %v", err)
	}
	r := gin.New()
	r.POST("/v1/webhooks/page", fakeAuth(tid.String()), page.Handler(p))

	rec := postPage(r, "", map[string]string{
		"rule_id":    "§11.4.21",
		"subject_id": "HRD-001|status=operator-blocked|oncall_paged=false",
	})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (gate open when unset); body=%s", rec.Code, rec.Body.String())
	}
}

// (F) The incident-severity routing row (§18.8, the bespoke iherald detector) —
// a high-severity incident not paged out is a routing failure that fans out.
func TestPage_IncidentSeverity_RoutingFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	p, _, la, _ := fixture(t)
	tid := uuid.New()
	if err := la.Set(context.Background(), tid, "§18.8", constitution.ModeEnforce, "op"); err != nil {
		t.Fatalf("ladder Set: %v", err)
	}
	r := gin.New()
	r.POST("/v1/webhooks/page", fakeAuth(tid.String()), page.Handler(p))

	rec := postPage(r, "", map[string]string{
		"rule_id":      "§18.8",
		"subject_kind": "incident-severity",
		"subject_id":   "incident-42|severity=sev1|paged=false",
	})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body.String())
	}
	var got page.Receipt
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Decision != "deny" || !got.Emitted {
		t.Errorf("sev1-not-paged should be deny+emitted; got decision=%q emitted=%v", got.Decision, got.Emitted)
	}
	if got.Escalation != "incident-severity" {
		t.Errorf("escalation = %q, want incident-severity", got.Escalation)
	}
}
