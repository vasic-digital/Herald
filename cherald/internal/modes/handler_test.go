package modes_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/vasic-digital/herald/cherald/internal/modes"
	"github.com/vasic-digital/herald/commons_auth"
	constitution "github.com/vasic-digital/herald/commons_constitution"
	"github.com/vasic-digital/herald/commons_constitution/ladder"
)

func fakeAuth(tenant string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(commons_auth.ContextKeyClaims, map[string]any{
			"tenant": tenant,
			"sub":    "ops@test",
		})
		c.Next()
	}
}

func newRouter(la constitution.ModeLadder, tenant string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/v1/compliance/modes", fakeAuth(tenant), modes.ListHandler(la))
	r.GET("/v1/compliance/modes/:rule_id", fakeAuth(tenant), modes.GetHandler(la))
	r.PUT("/v1/compliance/modes/:rule_id", fakeAuth(tenant), modes.PutHandler(la))
	return r
}

// TestModes_FlipReflectsImmediately is the load-bearing HRD-027 proof:
// a PUT that flips a rule to warn must be reflected by a subsequent GET AND
// by a direct ladder.Get (the evaluation hot path) — no redeploy, durable
// before the 200 returns.
func TestModes_FlipReflectsImmediately(t *testing.T) {
	la := ladder.NewMemory()
	tenant := uuid.New()
	r := newRouter(la, tenant.String())

	// Unbound rule defaults to enforce.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/compliance/modes/%C2%A711.4.10", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET unbound status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var got struct{ Mode string }
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Mode != "enforce" {
		t.Errorf("unbound default mode = %q; want enforce", got.Mode)
	}

	// PUT flip to warn.
	rec = httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/v1/compliance/modes/%C2%A711.4.10",
		strings.NewReader(`{"mode":"warn"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT flip status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var put struct {
		Mode      string `json:"mode"`
		MutatedBy string `json:"mutated_by"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &put); err != nil {
		t.Fatalf("PUT body unmarshal: %v; raw=%s", err, rec.Body.String())
	}
	if put.Mode != "warn" {
		t.Errorf("PUT echoed mode = %q; want warn", put.Mode)
	}
	if put.MutatedBy != "ops@test" {
		t.Errorf("mutated_by = %q; want ops@test (operator identity from sub claim)", put.MutatedBy)
	}

	// GET reflects the flip.
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/compliance/modes/%C2%A711.4.10", nil))
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Mode != "warn" {
		t.Errorf("after flip, GET mode = %q; want warn", got.Mode)
	}

	// The evaluation hot path (direct ladder.Get) ALSO reflects the flip —
	// this is what proves "no redeploy": the runtime config is live.
	m, err := la.Get(req.Context(), tenant, "§11.4.10")
	if err != nil {
		t.Fatalf("ladder.Get: %v", err)
	}
	if m != constitution.ModeWarn {
		t.Errorf("ladder.Get after flip = %v; want ModeWarn", m)
	}
}

// TestModes_List returns bindings + the unbound default note.
func TestModes_List(t *testing.T) {
	la := ladder.NewMemory()
	tenant := uuid.New()
	r := newRouter(la, tenant.String())

	// Flip two rules.
	for _, body := range []string{`{"mode":"allow"}`, `{"mode":"enforce"}`} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("PUT", "/v1/compliance/modes/ruleX", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(rec, req)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/compliance/modes", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("List status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Bindings       map[string]string `json:"bindings"`
		UnboundDefault string            `json:"unbound_default"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("List unmarshal: %v; raw=%s", err, rec.Body.String())
	}
	if body.Bindings["ruleX"] != "enforce" {
		t.Errorf("ruleX binding = %q; want enforce (last write wins)", body.Bindings["ruleX"])
	}
	if body.UnboundDefault != "enforce" {
		t.Errorf("unbound_default = %q; want enforce", body.UnboundDefault)
	}
}

// TestModes_RejectsBadMode is the §107 anti-bluff: an invalid mode is a
// loud 400, not a silent default.
func TestModes_RejectsBadMode(t *testing.T) {
	la := ladder.NewMemory()
	r := newRouter(la, uuid.New().String())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/v1/compliance/modes/ruleY", strings.NewReader(`{"mode":"banana"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad mode status = %d; want 400; body=%s", rec.Code, rec.Body.String())
	}
}

// TestModes_RejectsNoAuth proves the handler 401s without claims (NOT 200).
func TestModes_RejectsNoAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	la := ladder.NewMemory()
	r := gin.New()
	// No fakeAuth — claims are absent.
	r.GET("/v1/compliance/modes", modes.ListHandler(la))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/compliance/modes", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-auth status = %d; want 401; body=%s", rec.Code, rec.Body.String())
	}
}

// TestModes_RejectsBadTenant proves a malformed tenant claim is 401.
func TestModes_RejectsBadTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	la := ladder.NewMemory()
	r := gin.New()
	r.GET("/v1/compliance/modes", fakeAuth("not-a-uuid"), modes.ListHandler(la))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/compliance/modes", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad-tenant status = %d; want 401; body=%s", rec.Code, rec.Body.String())
	}
}
