package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestServer_HealthzReturnsOK proves the basic Gin engine + middleware
// chain compose correctly and respond to a real HTTP request.
//
// Anti-bluff: uses httptest.NewRecorder against the real handler — no
// mocks at the HTTP layer. A response code mismatch fails the test.
func TestServer_HealthzReturnsOK(t *testing.T) {
	s := New(Config{Build: BuildInfo{Version: "M3-test"}})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/healthz", nil)
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"status":"ok"`) {
		t.Errorf("body missing status:ok; got %s", body)
	}
	if !strings.Contains(body, `"version":"M3-test"`) {
		t.Errorf("body missing build.version; got %s", body)
	}
	// requestid middleware must set X-Request-ID on the response.
	if w.Header().Get("X-Request-ID") == "" {
		t.Errorf("X-Request-ID header missing — requestid middleware not wired")
	}
}

// TestServer_EventsIngestStubReturns501 proves the stub returns the
// canonical not-implemented sentinel with the HRD pointer. Anti-bluff:
// a 200 stub returning empty would lie about feature completeness.
func TestServer_EventsIngestStubReturns501(t *testing.T) {
	s := New(Config{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/cloudevents+json")
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("status = %d; want 501", w.Code)
	}
	if !strings.Contains(w.Body.String(), "HRD-016") {
		t.Errorf("body missing HRD-016 pointer; got %s", w.Body.String())
	}
}

func TestServer_ComplianceListStubReturns501(t *testing.T) {
	s := New(Config{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/compliance", nil)
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotImplemented {
		t.Errorf("status = %d; want 501", w.Code)
	}
}

// TestServer_MetricsExposed proves the /metrics endpoint serves
// Prometheus-format text (anti-bluff: a 200 returning HTML would be
// invisible to scrapers).
func TestServer_MetricsExposed(t *testing.T) {
	s := New(Config{Build: BuildInfo{Version: "metrics-test", GitCommit: "deadbeef"}})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q; want text/plain*", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "herald_build_info") {
		t.Errorf("body missing herald_build_info; got %s", body)
	}
	if !strings.Contains(body, `version="metrics-test"`) {
		t.Errorf("body missing version label; got %s", body)
	}
}

// TestServer_RequestIDHeaderPropagation proves the requestid middleware
// preserves an incoming X-Request-ID rather than overwriting it.
func TestServer_RequestIDHeaderPropagation(t *testing.T) {
	s := New(Config{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/healthz", nil)
	const inboundID = "test-correlation-id-12345"
	req.Header.Set("X-Request-ID", inboundID)
	s.Handler().ServeHTTP(w, req)

	if got := w.Header().Get("X-Request-ID"); got != inboundID {
		t.Errorf("X-Request-ID = %q; want %q (incoming should be preserved)", got, inboundID)
	}
}
