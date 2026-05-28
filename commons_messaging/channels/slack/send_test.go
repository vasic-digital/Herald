package slack_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons_messaging/channels/slack"
)

// TestSlackSendCrossesWireWithText pins the canonical Send happy path.
// httptest impersonates the Slack Web API, and three wire-byte assertions
// catch the §107 bluff classes for an outbound adapter:
//
//	(a) channel form value == the resolved channel id;
//	(b) text form value == the rendered body;
//	(c) returned Receipt.ChannelMsgID == the server's ts (real, not synthetic).
//
// A Send that compiled cleanly but never hit chat.postMessage would fail
// (a) + (b) immediately (the captured strings would stay empty).
func TestSlackSendCrossesWireWithText(t *testing.T) {
	var gotChannel, gotText string
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat.postMessage") {
			http.Error(w, "bad path "+r.URL.Path, 404)
			return
		}
		hits++
		_ = r.ParseForm()
		gotChannel = r.FormValue("channel")
		gotText = r.FormValue("text")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "channel": gotChannel, "ts": "1654.0001"})
	}))
	defer srv.Close()
	a := slack.NewWithBaseURL("xoxb-test", "", "C123", srv.URL)
	receipt, err := a.Send(context.Background(), commons.OutboundMessage{
		To:   []commons.Recipient{{Channel: "slack", ChannelUserID: "C123"}},
		Body: commons.Body{Plain: "hello slack"},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if hits != 1 {
		t.Fatalf("chat.postMessage hits=%d want 1 (§107: Send must cross the wire)", hits)
	}
	if gotChannel != "C123" {
		t.Fatalf("wire channel=%q want C123", gotChannel)
	}
	if gotText != "hello slack" {
		t.Fatalf("wire text=%q want %q", gotText, "hello slack")
	}
	if receipt.ChannelMsgID != "1654.0001" {
		t.Fatalf("ChannelMsgID=%q want %q (§107: real ts not synthetic id)", receipt.ChannelMsgID, "1654.0001")
	}
	if receipt.Evidence != commons.DeliveryRouted {
		t.Fatalf("Evidence=%v want DeliveryRouted", receipt.Evidence)
	}
	if receipt.Native == nil || receipt.Native["ts"] != "1654.0001" {
		t.Fatalf("Native[ts]=%v want %q", receipt.Native, "1654.0001")
	}
}

// TestSlackSendUsesDefaultChannelWhenToEmpty: when msg.To is empty the
// adapter falls back to its constructor-bound channelID. Verifies the
// per-message override is NOT mandatory.
func TestSlackSendUsesDefaultChannelWhenToEmpty(t *testing.T) {
	var gotChannel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotChannel = r.FormValue("channel")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ts": "1654.0099"})
	}))
	defer srv.Close()
	a := slack.NewWithBaseURL("xoxb-test", "", "Cdefault", srv.URL)
	_, err := a.Send(context.Background(), commons.OutboundMessage{Body: commons.Body{Plain: "ping"}})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotChannel != "Cdefault" {
		t.Fatalf("channel=%q want default Cdefault", gotChannel)
	}
}

// TestSlackSendEmptyBodyErrors pins the §107 anti-bluff guard — an empty
// body short-circuits BEFORE hitting the wire. The canonical bluff class
// here is a Send that silently posts the empty string and reports success.
func TestSlackSendEmptyBodyErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("expected NO wire calls for empty body, got %s", r.URL.Path)
	}))
	defer srv.Close()
	a := slack.NewWithBaseURL("xoxb-test", "", "C1", srv.URL)
	if _, err := a.Send(context.Background(), commons.OutboundMessage{}); err == nil {
		t.Fatal("Send(empty body) want error")
	}
}

// TestSlackSendReplyUsesThreadTS pins the §32.9 reply-threading anchor —
// SendReply with a non-empty replyToID MUST set thread_ts on the wire.
// A SendReply that compiled cleanly but never threaded would degrade every
// reply to a fresh message.
func TestSlackSendReplyUsesThreadTS(t *testing.T) {
	var gotThreadTS, gotChannel, gotText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat.postMessage") {
			http.Error(w, "bad "+r.URL.Path, 404)
			return
		}
		_ = r.ParseForm()
		gotThreadTS = r.FormValue("thread_ts")
		gotChannel = r.FormValue("channel")
		gotText = r.FormValue("text")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ts": "1654.0002"})
	}))
	defer srv.Close()
	a := slack.NewWithBaseURL("xoxb-test", "", "Cdefault", srv.URL)
	id, err := a.SendReply(context.Background(), commons.Recipient{Channel: "slack", ChannelUserID: "C123"}, "threaded reply", "1654.0001", nil)
	if err != nil {
		t.Fatalf("SendReply: %v", err)
	}
	if gotThreadTS != "1654.0001" {
		t.Fatalf("thread_ts=%q want 1654.0001 (§32.9 reply-threading anchor)", gotThreadTS)
	}
	if gotChannel != "C123" {
		t.Fatalf("channel=%q want C123", gotChannel)
	}
	if gotText != "threaded reply" {
		t.Fatalf("text=%q want %q", gotText, "threaded reply")
	}
	if id != "1654.0002" {
		t.Fatalf("reply ts=%q want 1654.0002", id)
	}
}

// TestSlackSendReplyNoReplyToOmitsThreadTS pins the no-reply sentinel
// (replyToID == "") — a fresh top-level message MUST NOT carry thread_ts.
func TestSlackSendReplyNoReplyToOmitsThreadTS(t *testing.T) {
	var gotThreadTS string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotThreadTS = r.FormValue("thread_ts")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ts": "1654.0010"})
	}))
	defer srv.Close()
	a := slack.NewWithBaseURL("xoxb-test", "", "Cdefault", srv.URL)
	_, err := a.SendReply(context.Background(), commons.Recipient{ChannelUserID: "C1"}, "fresh msg", "", nil)
	if err != nil {
		t.Fatalf("SendReply: %v", err)
	}
	if gotThreadTS != "" {
		t.Fatalf("thread_ts=%q want empty (no-reply sentinel)", gotThreadTS)
	}
}

// TestSlackSendReplyGenericDelegates proves SendReplyGeneric satisfies the
// channels.Channel interface and routes through the same wire path as
// SendReply (the interface seam is the public surface every multi-channel
// caller hits).
func TestSlackSendReplyGenericDelegates(t *testing.T) {
	var gotText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotText = r.FormValue("text")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ts": "1654.0042"})
	}))
	defer srv.Close()
	a := slack.NewWithBaseURL("xoxb-test", "", "Cdefault", srv.URL)
	id, err := a.SendReplyGeneric(context.Background(), commons.Recipient{ChannelUserID: "C1"}, "iface call", "", nil)
	if err != nil {
		t.Fatalf("SendReplyGeneric: %v", err)
	}
	if gotText != "iface call" {
		t.Fatalf("text=%q want iface call", gotText)
	}
	if id != "1654.0042" {
		t.Fatalf("id=%q want 1654.0042", id)
	}
}
