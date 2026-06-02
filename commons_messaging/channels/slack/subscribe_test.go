package slack_test

import (
	"context"
	"testing"

	"github.com/slack-go/slack/slackevents"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons_messaging/channels"
	"github.com/vasic-digital/herald/commons_messaging/channels/slack"
)

// captureHandler records every InboundEvent so subscribe_test.go can
// assert the dispatch path landed the right shape per event.
type captureHandler struct{ events []commons.InboundEvent }

func (c *captureHandler) Handle(_ context.Context, ev commons.InboundEvent) error {
	c.events = append(c.events, ev)
	return nil
}

// TestSlackSubscribeMessageEventReachesHandler pins the canonical inbound
// happy path — a MessageEvent with text + thread_ts builds an InboundEvent
// with the right Sender / Body / Thread / Raw and reaches the handler.
//
// §107 anti-bluff: a Subscribe that returned nil without invoking the
// handler would be the canonical bluff class (the loop "ran" but never
// dispatched). The captureHandler counts events directly.
func TestSlackSubscribeMessageEventReachesHandler(t *testing.T) {
	a := slack.NewWithBaseURL("xoxb-test", "", "Cdefault", "http://localhost")
	h := &captureHandler{}
	self := channels.SelfIdentity{Kind: channels.IdentityUserID, Value: "U0HERALD"}
	inner := &slackevents.MessageEvent{
		Type:            "message",
		User:            "U0HUMAN",
		Text:            "hello slack",
		TimeStamp:       "1654.0001",
		Channel:         "C0CHAT",
		ThreadTimeStamp: "1654.0000",
	}
	if err := slack.DispatchMessageEventForTest(a, context.Background(), h, inner, self); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(h.events) != 1 {
		t.Fatalf("handler got %d events want 1 (§107: dispatch must reach handler)", len(h.events))
	}
	ev := h.events[0]
	if ev.Sender.Channel != "slack" {
		t.Fatalf("Sender.Channel=%q want slack", ev.Sender.Channel)
	}
	// The inbound SENDER is the message AUTHOR (inner.User), NOT the
	// channel/conversation id — attributing every poster in a shared channel
	// to the channel id was the original mis-attribution bug. The channel id
	// is preserved in ev.Raw["channel"] for the reply destination.
	if ev.Sender.ChannelUserID != "U0HUMAN" {
		t.Fatalf("Sender.ChannelUserID=%q want U0HUMAN (the author, not the channel)", ev.Sender.ChannelUserID)
	}
	if ev.Raw["channel"] != "C0CHAT" {
		t.Fatalf("Raw.channel=%v want C0CHAT (channel id preserved for reply dest)", ev.Raw["channel"])
	}
	// With baseURL=http://localhost users.info is unreachable, so DisplayName
	// falls back deterministically to the raw user id (never drops the sender).
	if ev.Sender.DisplayName != "U0HUMAN" {
		t.Fatalf("Sender.DisplayName=%q want U0HUMAN fallback (users.info unreachable)", ev.Sender.DisplayName)
	}
	if ev.Body.Plain != "hello slack" {
		t.Fatalf("Body.Plain=%q want hello slack", ev.Body.Plain)
	}
	if ev.Thread == nil || ev.Thread.ThreadID != "1654.0000" {
		t.Fatalf("Thread=%+v want ThreadID=1654.0000", ev.Thread)
	}
	if ev.Raw["message_id"] != "1654.0001" {
		t.Fatalf("Raw.message_id=%v want 1654.0001", ev.Raw["message_id"])
	}
}

// TestSlackSubscribeSelfEchoDropped pins the §32.9 anti-echo-loop
// guarantee — a bot-authored MessageEvent whose user matches the bot's
// own user_id is silently dropped. Without this filter every reply
// pherald posts would be re-dispatched as a fresh inbound event.
func TestSlackSubscribeSelfEchoDropped(t *testing.T) {
	a := slack.NewWithBaseURL("xoxb-test", "", "Cdefault", "http://localhost")
	h := &captureHandler{}
	self := channels.SelfIdentity{Kind: channels.IdentityUserID, Value: "U0HERALD"}

	// Bot-authored echo: BotID non-empty + User == self.Value.
	echo := &slackevents.MessageEvent{
		Type:      "message",
		BotID:     "B0HERALD",
		User:      "U0HERALD",
		Text:      "echo loop bait",
		TimeStamp: "1654.0001",
		Channel:   "C0CHAT",
	}
	if err := slack.DispatchMessageEventForTest(a, context.Background(), h, echo, self); err != nil {
		t.Fatalf("dispatch echo: %v", err)
	}
	if len(h.events) != 0 {
		t.Fatalf("self-echo leaked: handler saw %d events", len(h.events))
	}

	// Human message: keeps flowing.
	human := &slackevents.MessageEvent{
		Type:      "message",
		User:      "U0HUMAN",
		Text:      "ping",
		TimeStamp: "1654.0002",
		Channel:   "C0CHAT",
	}
	if err := slack.DispatchMessageEventForTest(a, context.Background(), h, human, self); err != nil {
		t.Fatalf("dispatch human: %v", err)
	}
	if len(h.events) != 1 {
		t.Fatalf("human dropped: handler saw %d events", len(h.events))
	}
}

// TestSlackSubscribeCrossBotMessageKept pins the multi-bot collaboration
// invariant — a DIFFERENT bot in the same channel is real subscriber
// traffic and MUST NOT be filtered. Mirrors the tgram cross-bot pin
// (TestSubscribeBotSelfFilter case (c)).
func TestSlackSubscribeCrossBotMessageKept(t *testing.T) {
	a := slack.NewWithBaseURL("xoxb-test", "", "Cdefault", "http://localhost")
	h := &captureHandler{}
	self := channels.SelfIdentity{Kind: channels.IdentityUserID, Value: "U0HERALD"}
	otherBot := &slackevents.MessageEvent{
		Type:      "message",
		BotID:     "B0OTHER",
		User:      "U0OTHERBOT",
		Text:      "cross-bot chatter",
		TimeStamp: "1654.0003",
		Channel:   "C0CHAT",
	}
	if err := slack.DispatchMessageEventForTest(a, context.Background(), h, otherBot, self); err != nil {
		t.Fatalf("dispatch cross-bot: %v", err)
	}
	if len(h.events) != 1 {
		t.Fatalf("cross-bot dropped: handler saw %d events want 1", len(h.events))
	}
}

// TestSlackSubscribeNoThreadOmitsThread pins the no-thread sentinel — a
// MessageEvent without ThreadTimeStamp MUST NOT surface Thread in the
// InboundEvent (a bluff "ThreadID="""" would mislead downstream consumers).
func TestSlackSubscribeNoThreadOmitsThread(t *testing.T) {
	a := slack.NewWithBaseURL("xoxb-test", "", "Cdefault", "http://localhost")
	h := &captureHandler{}
	self := channels.SelfIdentity{Kind: channels.IdentityUserID, Value: "U0HERALD"}
	msg := &slackevents.MessageEvent{
		Type:      "message",
		User:      "U0HUMAN",
		Text:      "fresh msg",
		TimeStamp: "1654.0004",
		Channel:   "C0CHAT",
	}
	if err := slack.DispatchMessageEventForTest(a, context.Background(), h, msg, self); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(h.events) != 1 {
		t.Fatalf("want 1 event, got %d", len(h.events))
	}
	if h.events[0].Thread != nil {
		t.Fatalf("Thread=%+v want nil (no thread_ts in event)", h.events[0].Thread)
	}
}

// TestSlackSubscribeNilEventIsNoOp protects the dispatch path from a nil
// MessageEvent (defensive — slack-go can in principle return a nil-Data
// inner event; we drop silently rather than panic).
func TestSlackSubscribeNilEventIsNoOp(t *testing.T) {
	a := slack.NewWithBaseURL("xoxb-test", "", "Cdefault", "http://localhost")
	h := &captureHandler{}
	self := channels.SelfIdentity{Kind: channels.IdentityUserID, Value: "U0HERALD"}
	if err := slack.DispatchMessageEventForTest(a, context.Background(), h, nil, self); err != nil {
		t.Fatalf("dispatch(nil): %v", err)
	}
	if len(h.events) != 0 {
		t.Fatalf("nil event leaked: handler saw %d events", len(h.events))
	}
}

// TestSlackSubscribeRequiresAppToken pins the boot-time guard — calling
// Subscribe without an app-level token (xapp-…) returns an explicit error,
// NOT a silent no-op (which would be the §107 bluff class for inbound).
func TestSlackSubscribeRequiresAppToken(t *testing.T) {
	a := slack.NewWithBaseURL("xoxb-test", "", "Cdefault", "http://localhost")
	h := &captureHandler{}
	err := a.Subscribe(context.Background(), h)
	if err == nil {
		t.Fatal("Subscribe(no app token) want error")
	}
}
