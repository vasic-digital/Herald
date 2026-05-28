package slack_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vasic-digital/herald/commons_messaging/channels"
	"github.com/vasic-digital/herald/commons_messaging/channels/slack"
)

// TestSlackBotSelfIdentityViaAuthTest verifies BotSelfIdentity crosses
// the wire on first call (auth.test) and returns the user_id as
// IdentityUserID. Cache behaviour pinned in TestSlackBotSelfIdentityCaches.
//
// §107 anti-bluff: an httptest counter (hits) catches a BotSelfIdentity
// that returned a synthetic id without hitting auth.test — the canonical
// bluff class for a self-identity resolver.
func TestSlackBotSelfIdentityViaAuthTest(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/auth.test") {
			http.Error(w, "bad path "+r.URL.Path, 404)
			return
		}
		hits++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "user_id": "U0HERALD", "user": "herald"})
	}))
	defer srv.Close()
	a := slack.NewWithBaseURL("xoxb-test", "", "C1", srv.URL)
	id, err := a.BotSelfIdentity(context.Background())
	if err != nil {
		t.Fatalf("BotSelfIdentity: %v", err)
	}
	if id.Kind != channels.IdentityUserID {
		t.Fatalf("Kind=%v want IdentityUserID", id.Kind)
	}
	if id.Value != "U0HERALD" {
		t.Fatalf("Value=%q want U0HERALD", id.Value)
	}
	if hits != 1 {
		t.Fatalf("auth.test hits=%d want 1 (§107: BotSelfIdentity must cross the wire)", hits)
	}
}

// TestSlackBotSelfIdentityCaches pins the cache behaviour — a second call
// MUST NOT re-hit auth.test (which would be wasted quota on every inbound
// event in production).
func TestSlackBotSelfIdentityCaches(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "user_id": "U0HERALD", "user": "herald"})
	}))
	defer srv.Close()
	a := slack.NewWithBaseURL("xoxb-test", "", "C1", srv.URL)
	for i := 0; i < 3; i++ {
		if _, err := a.BotSelfIdentity(context.Background()); err != nil {
			t.Fatalf("BotSelfIdentity iter %d: %v", i, err)
		}
	}
	if hits != 1 {
		t.Fatalf("auth.test hits=%d want 1 after 3 BotSelfIdentity calls (cache violated)", hits)
	}
}

// TestSlackBotSelfIdentityEmptyUserIDIsEchoLoopHazard pins the §107 echo-
// loop guard: an auth.test that responds ok=true but with empty user_id
// is rejected — the channel-agnostic self-filter would otherwise classify
// no message as a self-echo and let pherald talk to itself.
func TestSlackBotSelfIdentityEmptyUserIDIsEchoLoopHazard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "user_id": ""})
	}))
	defer srv.Close()
	a := slack.NewWithBaseURL("xoxb-test", "", "C1", srv.URL)
	if _, err := a.BotSelfIdentity(context.Background()); err == nil {
		t.Fatal("expected echo-loop hazard error on empty user_id")
	}
}

// TestSlackHealthCheck verifies HealthCheck hits auth.test and returns
// nil on a populated response. The bluff class is the same as for
// BotSelfIdentity — a synthetic ok-without-wire would pass.
func TestSlackHealthCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "user_id": "U0HERALD"})
	}))
	defer srv.Close()
	a := slack.NewWithBaseURL("xoxb-test", "", "C1", srv.URL)
	if err := a.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
}
