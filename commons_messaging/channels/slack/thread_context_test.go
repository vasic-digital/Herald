package slack_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/slack-go/slack/slackevents"

	"github.com/vasic-digital/herald/commons_messaging/channels"
	"github.com/vasic-digital/herald/commons_messaging/channels/slack"
)

// repliesServer stands up an httptest server impersonating the Slack Web API
// conversations.replies method, returning the supplied raw message maps (in
// the order given) plus counting the hits. Any non-replies path 404s so a
// stray request is loud, not silent.
func repliesServer(t *testing.T, hits *int, gotChannel, gotTS *string, msgs []map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/conversations.replies") {
			http.Error(w, "bad path "+r.URL.Path, 404)
			return
		}
		*hits++
		_ = r.ParseForm()
		*gotChannel = r.FormValue("channel")
		*gotTS = r.FormValue("ts")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":       true,
			"messages": msgs,
			"has_more": false,
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestSlackThreadContextPopulatedFromReplies is the canonical thread-context
// happy path (operator mandate 2026-06-02): an inbound message inside a thread
// triggers a conversations.replies fetch, and ev.ThreadContext carries the
// PRIOR messages oldest→newest with the CURRENT message excluded and
// SenderIsBot set correctly.
//
// §107 anti-bluff: the httptest server counts the replies round-trip — a
// dispatch that "claimed" thread context without crossing the wire would fail
// the hits assertion, and the per-field assertions catch a populated-but-wrong
// slice.
func TestSlackThreadContextPopulatedFromReplies(t *testing.T) {
	var hits int
	var gotChannel, gotTS string
	// Slack returns thread order oldest→newest. Includes a human, a bot, and
	// the CURRENT inbound message (ts == 1654.0003) which MUST be excluded.
	srv := repliesServer(t, &hits, &gotChannel, &gotTS, []map[string]any{
		{"user": "U0HUMAN", "text": "first question", "ts": "1654.0000"},
		{"bot_id": "B0HERALD", "user": "U0HERALD", "text": "bot reply", "ts": "1654.0001"},
		{"user": "U0HUMAN2", "text": "follow up", "ts": "1654.0002"},
		{"user": "U0HUMAN", "text": "the current message", "ts": "1654.0003"},
	})

	a := slack.NewWithBaseURL("xoxb-test", "", "Cdefault", srv.URL)
	h := &captureHandler{}
	self := channels.SelfIdentity{Kind: channels.IdentityUserID, Value: "U0NOTME"}
	inner := &slackevents.MessageEvent{
		Type:            "message",
		User:            "U0HUMAN",
		Text:            "the current message",
		TimeStamp:       "1654.0003",
		Channel:         "C0CHAT",
		ThreadTimeStamp: "1654.0000",
	}
	if err := slack.DispatchMessageEventForTest(a, context.Background(), h, inner, self); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if hits != 1 {
		t.Fatalf("conversations.replies hits=%d want 1 (§107: thread context must cross the wire)", hits)
	}
	if gotChannel != "C0CHAT" {
		t.Fatalf("replies channel=%q want C0CHAT", gotChannel)
	}
	if gotTS != "1654.0000" {
		t.Fatalf("replies ts=%q want thread root 1654.0000", gotTS)
	}
	if len(h.events) != 1 {
		t.Fatalf("handler got %d events want 1", len(h.events))
	}
	tc := h.events[0].ThreadContext
	if len(tc) != 3 {
		t.Fatalf("ThreadContext len=%d want 3 (current message excluded): %+v", len(tc), tc)
	}
	// Oldest→newest, current message (1654.0003) excluded.
	if tc[0].Text != "first question" || tc[0].SenderHandle != "U0HUMAN" || tc[0].SenderIsBot {
		t.Fatalf("tc[0]=%+v want {first question, U0HUMAN, bot=false}", tc[0])
	}
	if tc[1].Text != "bot reply" || tc[1].SenderHandle != "U0HERALD" || !tc[1].SenderIsBot {
		t.Fatalf("tc[1]=%+v want {bot reply, U0HERALD, bot=true}", tc[1])
	}
	if tc[2].Text != "follow up" || tc[2].SenderHandle != "U0HUMAN2" || tc[2].SenderIsBot {
		t.Fatalf("tc[2]=%+v want {follow up, U0HUMAN2, bot=false}", tc[2])
	}
	// Ordering pin: ascending by timestamp (oldest→newest).
	if !tc[0].Timestamp.Before(tc[1].Timestamp) || !tc[1].Timestamp.Before(tc[2].Timestamp) {
		t.Fatalf("ThreadContext not ascending oldest→newest: %v / %v / %v", tc[0].Timestamp, tc[1].Timestamp, tc[2].Timestamp)
	}
	// Timestamp parsed (not zero) for a well-formed slack ts.
	if tc[0].Timestamp.IsZero() {
		t.Fatalf("tc[0].Timestamp zero — slack ts %q should parse", "1654.0000")
	}
}

// TestSlackThreadContextEmptyForNonThreaded pins that a message with NO
// thread_ts yields an empty ThreadContext AND never hits conversations.replies
// — fetching thread context for a top-level message would be a wasted
// round-trip and a §107 bluff (claiming context that does not exist).
func TestSlackThreadContextEmptyForNonThreaded(t *testing.T) {
	var hits int
	var gotChannel, gotTS string
	srv := repliesServer(t, &hits, &gotChannel, &gotTS, nil)

	a := slack.NewWithBaseURL("xoxb-test", "", "Cdefault", srv.URL)
	h := &captureHandler{}
	self := channels.SelfIdentity{Kind: channels.IdentityUserID, Value: "U0NOTME"}
	inner := &slackevents.MessageEvent{
		Type:      "message",
		User:      "U0HUMAN",
		Text:      "fresh top-level message",
		TimeStamp: "1654.0010",
		Channel:   "C0CHAT",
		// no ThreadTimeStamp
	}
	if err := slack.DispatchMessageEventForTest(a, context.Background(), h, inner, self); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if hits != 0 {
		t.Fatalf("conversations.replies hits=%d want 0 (no thread → no fetch)", hits)
	}
	if len(h.events) != 1 {
		t.Fatalf("handler got %d events want 1", len(h.events))
	}
	if len(h.events[0].ThreadContext) != 0 {
		t.Fatalf("ThreadContext=%+v want empty for non-threaded message", h.events[0].ThreadContext)
	}
}

// TestSlackThreadContextErrorDoesNotDropMessage pins the non-fatal contract: a
// conversations.replies failure leaves ThreadContext empty BUT still dispatches
// the message to the handler. A context-fetch error must never swallow the
// subscriber's message (that would be the worst §107 bluff — a silently dropped
// inbound).
func TestSlackThreadContextErrorDoesNotDropMessage(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		// Slack-level error: ok=false → slack-go returns an error.
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "thread_not_found"})
	}))
	defer srv.Close()

	a := slack.NewWithBaseURL("xoxb-test", "", "Cdefault", srv.URL)
	h := &captureHandler{}
	self := channels.SelfIdentity{Kind: channels.IdentityUserID, Value: "U0NOTME"}
	inner := &slackevents.MessageEvent{
		Type:            "message",
		User:            "U0HUMAN",
		Text:            "message in a thread we cannot read",
		TimeStamp:       "1654.0021",
		Channel:         "C0CHAT",
		ThreadTimeStamp: "1654.0020",
	}
	if err := slack.DispatchMessageEventForTest(a, context.Background(), h, inner, self); err != nil {
		t.Fatalf("dispatch must NOT return error on replies failure: %v", err)
	}
	if hits < 1 {
		t.Fatalf("conversations.replies hits=%d want >=1 (fetch attempted)", hits)
	}
	if len(h.events) != 1 {
		t.Fatalf("handler got %d events want 1 (message MUST still be dispatched)", len(h.events))
	}
	if len(h.events[0].ThreadContext) != 0 {
		t.Fatalf("ThreadContext=%+v want empty on fetch error", h.events[0].ThreadContext)
	}
	// The message body still reached the handler intact.
	if h.events[0].Body.Plain != "message in a thread we cannot read" {
		t.Fatalf("Body.Plain=%q lost on thread-context error", h.events[0].Body.Plain)
	}
}
