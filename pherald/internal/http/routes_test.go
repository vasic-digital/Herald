package http

import (
	"testing"

	"github.com/vasic-digital/herald/pherald/internal/runner"
)

// TestRoutes_LiveEvents proves Routes() returns the one pherald-owned
// flavor-specific route (POST /v1/events) and that as of Wave 3b it
// carries a live Handler (no HRD pointer) — the 501-stub era ended at the
// HRD-016 close-out.
//
// /v1/compliance is intentionally absent: it's a cherald-owned route
// living in cherald/internal/compliance/. The Wave 2 Task 6 placeholder
// was removed in Wave 3b Task 10. Anti-bluff (§107 / §11.4.69):
// healthz/readyz/metrics still come from cli.ServeCmd's built-ins; they
// don't appear here.
func TestRoutes_LiveEvents(t *testing.T) {
	// Routes() requires a *runner.Runner — pass a zero-value pointer
	// (handler closure captures it but never derefs in this test).
	r := &runner.Runner{}
	routes := Routes(r)
	if len(routes) != 1 {
		t.Fatalf("Routes() returned %d routes, want 1 (POST /v1/events)", len(routes))
	}
	got := routes[0]
	if got.Method != "POST" || got.Path != "/v1/events" {
		t.Errorf("Routes()[0] = %s %s, want POST /v1/events", got.Method, got.Path)
	}
	if got.Handler == nil {
		t.Errorf("Routes()[0].Handler is nil — Wave 3b should have flipped this to live")
	}
	if got.HRD != "" {
		t.Errorf("Routes()[0].HRD = %q, want \"\" (live route, no stub pointer)", got.HRD)
	}
	if got.Description == "" {
		t.Errorf("Routes()[0].Description is empty — §107 bluff guard")
	}
}
