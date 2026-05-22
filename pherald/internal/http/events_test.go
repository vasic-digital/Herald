package http

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

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
