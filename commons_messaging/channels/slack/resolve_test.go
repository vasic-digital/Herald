package slack_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/slack-go/slack/slackevents"

	"github.com/vasic-digital/herald/commons_messaging/channels"
	"github.com/vasic-digital/herald/commons_messaging/channels/slack"
)

// usersInfoReplyServer stands up an httptest Slack Web API that serves BOTH
// conversations.replies (the thread-context fetch) AND users.info (the
// participant-handle resolver). users.info hits are counted so a §107 bluff —
// claiming a resolved @handle without crossing the wire — is caught, and the
// user→profile table is the known-good mapping the assertions check against.
//
// users.info responds from `profiles` keyed by the `user` form value; an
// unknown id returns ok=false so the resolver's deterministic raw-id fallback
// is exercised. Both hit-counters are pointers so the test can assert on them.
func usersInfoReplyServer(
	t *testing.T,
	repliesHits, usersInfoHits *int,
	mu *sync.Mutex,
	replies []map[string]any,
	profiles map[string]map[string]any,
) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = r.ParseForm()
		switch {
		case strings.HasSuffix(r.URL.Path, "/conversations.replies"):
			mu.Lock()
			*repliesHits++
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":       true,
				"messages": replies,
				"has_more": false,
			})
		case strings.HasSuffix(r.URL.Path, "/users.info"):
			mu.Lock()
			*usersInfoHits++
			mu.Unlock()
			uid := r.FormValue("user")
			prof, ok := profiles[uid]
			if !ok {
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "user_not_found"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "user": prof})
		default:
			http.Error(w, "bad path "+r.URL.Path, 404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestSlackInboundSenderResolvesToUsername is the canonical positive proof for
// the participant-resolution fix:
//
//  1. The inbound SENDER is the message AUTHOR (inner.User), NOT the channel id
//     (the original mis-attribution bug).
//  2. Sender.DisplayName is the resolved "@username" from users.info, NOT the
//     raw "U0…" id.
//  3. users.info actually crossed the wire (counted) — a synthetic handle
//     without a round-trip would fail the §107 hits assertion.
func TestSlackInboundSenderResolvesToUsername(t *testing.T) {
	var repliesHits, usersInfoHits int
	var mu sync.Mutex
	srv := usersInfoReplyServer(t, &repliesHits, &usersInfoHits, &mu, nil, map[string]map[string]any{
		"U0HUMAN": {"id": "U0HUMAN", "name": "alice", "real_name": "Alice Example"},
	})

	a := slack.NewWithBaseURL("xoxb-test", "", "Cdefault", srv.URL)
	h := &captureHandler{}
	self := channels.SelfIdentity{Kind: channels.IdentityUserID, Value: "U0HERALD"}
	inner := &slackevents.MessageEvent{
		Type:      "message",
		User:      "U0HUMAN",
		Text:      "hello from a real person",
		TimeStamp: "1654.0100",
		Channel:   "C0CHAT", // the conversation id — must NOT become the sender
	}
	if err := slack.DispatchMessageEventForTest(a, context.Background(), h, inner, self); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(h.events) != 1 {
		t.Fatalf("handler got %d events want 1", len(h.events))
	}
	ev := h.events[0]
	// (1) author, not channel.
	if ev.Sender.ChannelUserID != "U0HUMAN" {
		t.Fatalf("Sender.ChannelUserID=%q want U0HUMAN (author, not channel C0CHAT)", ev.Sender.ChannelUserID)
	}
	if ev.Sender.ChannelUserID == "C0CHAT" {
		t.Fatalf("regression: sender mis-attributed to the channel id")
	}
	// (2) resolved @username, not raw id.
	if ev.Sender.DisplayName != "@alice" {
		t.Fatalf("Sender.DisplayName=%q want @alice (resolved username, not raw U0…)", ev.Sender.DisplayName)
	}
	if ev.Sender.DisplayName == "U0HUMAN" {
		t.Fatalf("regression: sender handle is the raw user id, not the @username")
	}
	// (3) the resolution crossed the wire.
	if usersInfoHits != 1 {
		t.Fatalf("users.info hits=%d want 1 (§107: handle resolution must cross the wire)", usersInfoHits)
	}
}

// TestSlackThreadContextResolvesUsernames proves the thread-context authors are
// surfaced as "@username" (not raw "U0…" ids), the app/bot user resolves to its
// real handle (NOT the literal "bot"), and resolutions are CACHED — a repeated
// author is resolved once across the whole thread.
func TestSlackThreadContextResolvesUsernames(t *testing.T) {
	var repliesHits, usersInfoHits int
	var mu sync.Mutex
	replies := []map[string]any{
		{"user": "U0HUMAN", "text": "first question", "ts": "1654.0200"},
		// App/bot user: bot_id present but the user id resolves to a real handle.
		{"bot_id": "B0HERALD", "user": "U0APP", "text": "bot reply", "ts": "1654.0201"},
		{"user": "U0HUMAN", "text": "same author again", "ts": "1654.0202"},
		{"user": "U0HUMAN", "text": "the current message", "ts": "1654.0203"},
	}
	profiles := map[string]map[string]any{
		"U0HUMAN": {"id": "U0HUMAN", "name": "alice"},
		// No `name` → falls back to display_name; proves the bot is NOT "bot".
		"U0APP": {"id": "U0APP", "profile": map[string]any{"display_name": "herald-qa"}},
	}
	srv := usersInfoReplyServer(t, &repliesHits, &usersInfoHits, &mu, replies, profiles)

	a := slack.NewWithBaseURL("xoxb-test", "", "Cdefault", srv.URL)
	h := &captureHandler{}
	self := channels.SelfIdentity{Kind: channels.IdentityUserID, Value: "U0NOTME"}
	inner := &slackevents.MessageEvent{
		Type:            "message",
		User:            "U0HUMAN",
		Text:            "the current message",
		TimeStamp:       "1654.0203",
		Channel:         "C0CHAT",
		ThreadTimeStamp: "1654.0200",
	}
	if err := slack.DispatchMessageEventForTest(a, context.Background(), h, inner, self); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(h.events) != 1 {
		t.Fatalf("handler got %d events want 1", len(h.events))
	}
	tc := h.events[0].ThreadContext
	if len(tc) != 3 {
		t.Fatalf("ThreadContext len=%d want 3 (current excluded): %+v", len(tc), tc)
	}
	// (1) human author → @alice (not the raw U0HUMAN id).
	if tc[0].SenderHandle != "@alice" {
		t.Fatalf("tc[0].SenderHandle=%q want @alice", tc[0].SenderHandle)
	}
	// (2) the app/bot user resolves to a real handle, NOT "bot".
	if tc[1].SenderHandle != "@herald-qa" {
		t.Fatalf("tc[1].SenderHandle=%q want @herald-qa (app user, not literal bot)", tc[1].SenderHandle)
	}
	if tc[1].SenderHandle == "bot" || tc[1].SenderHandle == "B0HERALD" {
		t.Fatalf("regression: app user shows as %q instead of its real handle", tc[1].SenderHandle)
	}
	if !tc[1].SenderIsBot {
		t.Fatalf("tc[1].SenderIsBot=false want true (bot_id present)")
	}
	// (3) repeated author resolved once: U0HUMAN appears twice in-context +
	//     once as the (excluded) current message author + once for the sender.
	//     The cache means users.info is hit at most once per DISTINCT id, so
	//     two distinct ids (U0HUMAN, U0APP) => exactly 2 users.info round-trips.
	if usersInfoHits != 2 {
		t.Fatalf("users.info hits=%d want 2 (one per distinct id — cache violated)", usersInfoHits)
	}
}

// TestSlackHandleResolutionFallsBackToRawID is the deterministic negative case
// AND the paired guard: an UNKNOWN user id (users.info ok=false) must fall back
// to the raw id rather than dropping the sender or emitting an empty handle.
// This is the assertion a resolver-dropping mutation flips: if dispatch stopped
// resolving and stored the raw id directly as DisplayName, this test STILL
// passes — but TestSlackInboundSenderResolvesToUsername FAILS (it requires the
// @username). Conversely a mutation that drops the AUTHOR fix (sender = channel)
// flips both. The pair therefore genuinely bites.
func TestSlackHandleResolutionFallsBackToRawID(t *testing.T) {
	var repliesHits, usersInfoHits int
	var mu sync.Mutex
	srv := usersInfoReplyServer(t, &repliesHits, &usersInfoHits, &mu, nil, map[string]map[string]any{
		// deliberately EMPTY: every users.info lookup returns user_not_found.
	})

	a := slack.NewWithBaseURL("xoxb-test", "", "Cdefault", srv.URL)
	h := &captureHandler{}
	self := channels.SelfIdentity{Kind: channels.IdentityUserID, Value: "U0HERALD"}
	inner := &slackevents.MessageEvent{
		Type:      "message",
		User:      "U0GHOST",
		Text:      "who am I",
		TimeStamp: "1654.0300",
		Channel:   "C0CHAT",
	}
	if err := slack.DispatchMessageEventForTest(a, context.Background(), h, inner, self); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(h.events) != 1 {
		t.Fatalf("handler got %d events want 1 (unknown id must NOT drop the sender)", len(h.events))
	}
	ev := h.events[0]
	if ev.Sender.ChannelUserID != "U0GHOST" {
		t.Fatalf("Sender.ChannelUserID=%q want U0GHOST", ev.Sender.ChannelUserID)
	}
	// Deterministic fallback: the raw id, never "" and never "bot".
	if ev.Sender.DisplayName != "U0GHOST" {
		t.Fatalf("Sender.DisplayName=%q want U0GHOST (raw-id fallback on unknown user)", ev.Sender.DisplayName)
	}
	if usersInfoHits != 1 {
		t.Fatalf("users.info hits=%d want 1 (resolution attempted even on the miss)", usersInfoHits)
	}
}
