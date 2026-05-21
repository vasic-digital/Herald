package http

import (
	"testing"
)

// TestRoutes_IncludesEventsAndCompliance proves Routes() returns the
// two flavor-specific stubs that pherald owns; healthz/readyz/metrics
// are built-in to cli.ServeCmd and do not appear here. Anti-bluff
// (§11.4.69): each stub must carry the HRD-016 pointer so the operator
// can locate the implementation gap.
func TestRoutes_IncludesEventsAndCompliance(t *testing.T) {
	routes := Routes()
	if len(routes) != 2 {
		t.Fatalf("Routes() returned %d routes, want 2", len(routes))
	}
	want := map[string]string{
		"POST /v1/events":    "HRD-016",
		"GET /v1/compliance": "HRD-016",
	}
	for _, r := range routes {
		key := r.Method + " " + r.Path
		hrd, ok := want[key]
		if !ok {
			t.Errorf("unexpected route: %s", key)
			continue
		}
		if r.HRD != hrd {
			t.Errorf("%s HRD = %q, want %q", key, r.HRD, hrd)
		}
		if r.Description == "" {
			t.Errorf("%s has empty Description — §107 bluff guard (operator needs the spec pointer)", key)
		}
		delete(want, key)
	}
	for missing := range want {
		t.Errorf("missing route: %s", missing)
	}
}
