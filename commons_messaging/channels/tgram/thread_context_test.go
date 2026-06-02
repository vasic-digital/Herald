package tgram_test

import (
	"testing"
	"time"

	telebot "gopkg.in/telebot.v3"

	"github.com/vasic-digital/herald/commons_messaging/channels/tgram"
)

// TestThreadContextFromReply_PopulatesParent proves the thread-context-awareness
// mandate (operator 2026-06-02): an inbound Telegram message that is a REPLY to
// another message yields a one-entry ThreadContext carrying the quoted parent's
// text, bot-flag, sender handle, and timestamp. Constructed hermetically from
// telebot fixtures — no network.
func TestThreadContextFromReply_PopulatesParent(t *testing.T) {
	parentTime := time.Unix(1_717_000_000, 0)
	msg := &telebot.Message{
		Sender:   &telebot.User{IsBot: false, Username: "alice"},
		Text:     "yes, close it",
		Unixtime: parentTime.Unix() + 60,
		ReplyTo: &telebot.Message{
			Sender:   &telebot.User{IsBot: true, Username: "MyHeraldBot"},
			Text:     "Should I close HRD-200?",
			Unixtime: parentTime.Unix(),
		},
	}

	tc := tgram.ThreadContextFromReplyForTest(msg)
	if len(tc) != 1 {
		t.Fatalf("expected exactly 1 ThreadMessage (the immediate quoted parent), got %d", len(tc))
	}
	got := tc[0]
	if got.Text != "Should I close HRD-200?" {
		t.Errorf("parent Text: got %q want %q", got.Text, "Should I close HRD-200?")
	}
	if !got.SenderIsBot {
		t.Error("parent SenderIsBot: got false, want true (parent authored by the bot)")
	}
	if got.SenderHandle != "MyHeraldBot" {
		t.Errorf("parent SenderHandle: got %q want %q", got.SenderHandle, "MyHeraldBot")
	}
	if !got.Timestamp.Equal(parentTime) {
		t.Errorf("parent Timestamp: got %v want %v", got.Timestamp, parentTime)
	}
}

// TestThreadContextFromReply_NoReplyEmpty proves a fresh / top-level message
// (ReplyTo == nil) yields an EMPTY ThreadContext — the correct signal that
// there is no prior thread to bind the reply to.
func TestThreadContextFromReply_NoReplyEmpty(t *testing.T) {
	msg := &telebot.Message{
		Sender:  &telebot.User{IsBot: false, Username: "alice"},
		Text:    "hello, fresh message",
		ReplyTo: nil,
	}
	tc := tgram.ThreadContextFromReplyForTest(msg)
	if len(tc) != 0 {
		t.Fatalf("expected empty ThreadContext for a non-reply message, got %d entries: %+v", len(tc), tc)
	}
}

// TestThreadContextFromReply_CaptionFallback proves a reply to a captioned
// media message (Text empty, Caption set) surfaces the Caption as the bound
// thread context.
func TestThreadContextFromReply_CaptionFallback(t *testing.T) {
	msg := &telebot.Message{
		Sender: &telebot.User{Username: "alice"},
		Text:   "what is this?",
		ReplyTo: &telebot.Message{
			Sender:  &telebot.User{Username: "bob"},
			Text:    "",
			Caption: "screenshot of the failing test",
		},
	}
	tc := tgram.ThreadContextFromReplyForTest(msg)
	if len(tc) != 1 {
		t.Fatalf("expected 1 ThreadMessage, got %d", len(tc))
	}
	if tc[0].Text != "screenshot of the failing test" {
		t.Errorf("caption fallback Text: got %q want %q", tc[0].Text, "screenshot of the failing test")
	}
	if tc[0].SenderHandle != "bob" {
		t.Errorf("SenderHandle: got %q want %q", tc[0].SenderHandle, "bob")
	}
}

// TestBuildEditedEvent_ThreadContext proves the wiring through a real
// InboundEvent builder: buildEditedEvent (exercised via BuildEditedEventForTest)
// populates ev.ThreadContext from the reply, and leaves it empty otherwise. This
// is the anti-bluff link — the helper is actually called from the event builders
// the Subscribe handlers use, not merely defined.
func TestBuildEditedEvent_ThreadContext(t *testing.T) {
	withReply := &telebot.Message{
		Chat:   &telebot.Chat{ID: 42},
		Sender: &telebot.User{Username: "alice"},
		Text:   "actually, reopen it",
		ReplyTo: &telebot.Message{
			Sender: &telebot.User{Username: "bob"},
			Text:   "HRD-200 is closed",
		},
	}
	ev := tgram.BuildEditedEventForTest(withReply)
	if len(ev.ThreadContext) != 1 {
		t.Fatalf("edited reply: expected 1 ThreadContext entry, got %d", len(ev.ThreadContext))
	}
	if ev.ThreadContext[0].Text != "HRD-200 is closed" {
		t.Errorf("edited reply parent Text: got %q want %q", ev.ThreadContext[0].Text, "HRD-200 is closed")
	}

	noReply := &telebot.Message{
		Chat:   &telebot.Chat{ID: 42},
		Sender: &telebot.User{Username: "alice"},
		Text:   "fresh edit",
	}
	ev2 := tgram.BuildEditedEventForTest(noReply)
	if len(ev2.ThreadContext) != 0 {
		t.Fatalf("edited non-reply: expected empty ThreadContext, got %d", len(ev2.ThreadContext))
	}
}
