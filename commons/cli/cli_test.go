package cli

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/vasic-digital/herald/commons"
)

func TestStubCmd_ReturnsErrorWithHRDPointer(t *testing.T) {
	cmd := StubCmd("destructive-guard", "HRD-033", "wrap rm + git-reset with prerequisite checks")
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	cmd.SetOut(&stderr)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected non-nil error from stub")
	}
	msg := err.Error()
	if !strings.Contains(msg, "HRD-033") {
		t.Errorf("error should contain HRD reference, got: %q", msg)
	}
	if !strings.Contains(msg, "destructive-guard") {
		t.Errorf("error should contain command name, got: %q", msg)
	}
	if !strings.Contains(strings.ToLower(msg), "not yet implemented") {
		t.Errorf("error should explain non-implementation, got: %q", msg)
	}
}

func TestNewRootCmd_BindsBranding(t *testing.T) {
	br := commons.Branding{
		Flavor:      "sherald",
		Prefix:      "s",
		DisplayName: "System Herald",
		DefaultPort: 24793,
		Mission:     "Host safety + destructive-op intercept",
	}
	cmd := NewRootCmd(br)
	if cmd.Use != "sherald" {
		t.Errorf("Use = %q, want %q", cmd.Use, "sherald")
	}
	if !strings.Contains(cmd.Short, "System Herald") {
		t.Errorf("Short should contain DisplayName, got %q", cmd.Short)
	}
}

func TestVersionCmd_JSONOutputShape(t *testing.T) {
	br := commons.Branding{Flavor: "sherald", DisplayName: "System Herald"}
	cmd := VersionCmd(br)
	cmd.SetArgs([]string{"--json"})
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("JSON unmarshal failed: %v; raw=%q", err, stdout.String())
	}
	for _, key := range []string{"binary", "flavor", "version", "go_version", "os", "arch"} {
		v, ok := got[key]
		if !ok {
			t.Errorf("missing key %q in version JSON", key)
		}
		if s, ok := v.(string); ok && s == "" {
			t.Errorf("key %q is empty string — §107 bluff guard", key)
		}
	}
	if got["flavor"] != "sherald" {
		t.Errorf("flavor field = %v, want %q", got["flavor"], "sherald")
	}
}

func TestHealthzHandler_Returns200WithBuildInfo(t *testing.T) {
	br := commons.Branding{Flavor: "sherald", DisplayName: "System Herald"}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/v1/healthz", HealthzHandler(br))
	req := httptest.NewRequest("GET", "/v1/healthz", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body unmarshal: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status field = %v, want \"ok\"", body["status"])
	}
	build, ok := body["build"].(map[string]any)
	if !ok {
		t.Fatalf("build is not a map, got %T", body["build"])
	}
	if v, _ := build["version"].(string); v == "" {
		t.Errorf("build.version empty — §107 bluff guard")
	}
}

func TestMetricsHandler_EmitsBuildInfoGauge(t *testing.T) {
	br := commons.Branding{Flavor: "sherald"}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/metrics", MetricsHandler(br))
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "sherald_build_info{") {
		t.Errorf("expected gauge sherald_build_info{...} in body, got:\n%s", body)
	}
}
