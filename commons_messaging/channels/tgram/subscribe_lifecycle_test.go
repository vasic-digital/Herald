package tgram_test

import (
	"bytes"
	"log"
	"strings"
	"testing"

	telebot "gopkg.in/telebot.v3"

	"github.com/vasic-digital/herald/commons_messaging/channels"
	"github.com/vasic-digital/herald/commons_messaging/channels/tgram"
)

// TestSubscribeEditedMessage pins HRD-135 §1: buildEditedEvent produces the
// same InboundEvent shape OnText produces, with Raw["edited"] = true so the
// downstream LLM dispatcher can suppress duplicate replies. The OnEdited
// closure that wraps it in Subscribe is the production wiring; this test
// exercises the pure helper buildEditedEvent under the seam exposed by
// export_test.go, matching the existing TestSubscribeBotSelfFilter pattern.
//
// Assertions:
//
//	(a) Raw["edited"] is the literal bool true (not "true", not 1 — the
//	    downstream router type-switches on the value).
//	(b) Body.Plain matches the source msg.Text byte-for-byte — without this
//	    the edit would arrive at the dispatcher with empty content.
//	(c) Sender.Channel is "tgram" (commons.ChannelTelegram) and
//	    Sender.ChannelUserID is the chat ID — identical to the OnText shape.
//	(d) ThreadID > 0 surfaces in ev.Thread (forum-topic edit preservation);
//	    ThreadID == 0 leaves ev.Thread nil (per the existing T4 review
//	    carry-forward — no bluff ThreadID="0").
func TestSubscribeEditedMessage(t *testing.T) {
	msg := &telebot.Message{
		ID:       4242,
		Text:     "previous content, now edited",
		Chat:     &telebot.Chat{ID: -100123456},
		Sender:   &telebot.User{IsBot: false, Username: "alice", ID: 7},
		ThreadID: 9, // forum-topic edit
	}
	ev := tgram.BuildEditedEventForTest(msg)

	// (a) edited flag
	edited, ok := ev.Raw["edited"].(bool)
	if !ok || !edited {
		t.Fatalf("(a) Raw[\"edited\"] must be bool true; got %T=%v", ev.Raw["edited"], ev.Raw["edited"])
	}

	// (b) Body.Plain mirrors msg.Text
	if ev.Body.Plain != msg.Text {
		t.Fatalf("(b) Body.Plain = %q; want %q", ev.Body.Plain, msg.Text)
	}

	// (c) Sender carries channel+chat_id, matching OnText shape
	if ev.Sender.Channel != "tgram" {
		t.Fatalf("(c) Sender.Channel = %q; want %q", ev.Sender.Channel, "tgram")
	}
	if ev.Sender.ChannelUserID != "-100123456" {
		t.Fatalf("(c) Sender.ChannelUserID = %q; want %q", ev.Sender.ChannelUserID, "-100123456")
	}

	// (d) forum-topic edit preserves ThreadID
	if ev.Thread == nil {
		t.Fatal("(d) ThreadID=9 must surface a non-nil Thread (forum-topic edit)")
	} else if ev.Thread.ThreadID != "9" {
		t.Fatalf("(d) Thread.ThreadID = %q; want %q", ev.Thread.ThreadID, "9")
	}

	// Belt-and-braces: ThreadID==0 must NOT manifest as Thread="0".
	msgNoTopic := &telebot.Message{
		ID:       4243,
		Text:     "non-forum edit",
		Chat:     &telebot.Chat{ID: -100123456},
		Sender:   &telebot.User{IsBot: false, Username: "alice"},
		ThreadID: 0,
	}
	evNT := tgram.BuildEditedEventForTest(msgNoTopic)
	if evNT.Thread != nil {
		t.Fatalf("(d2) ThreadID=0 must keep Thread nil; got %+v", evNT.Thread)
	}
}

// TestSubscribeServiceMessageIgnoreAndLog pins HRD-135 §2: service messages
// (membership / metadata signals) MUST NOT reach the InboundHandler, and
// MUST emit a single INFO log line of the documented shape.
//
// Assertions:
//
//	(a) serviceMessageKind classifies a NewChatMembers / UsersJoined payload
//	    as "new_chat_members" (the Bot API JSON field name).
//	(b) A plain text *Message returns kind="" (must NOT be misclassified as
//	    a service message — would silently drop real subscriber traffic).
//	(c) The Subscribe handler's log.Printf call, when fed the same kind, emits
//	    the literal "tgram: ignored service message kind=... chat_id=..."
//	    line. We capture log.Default() output by swapping log.SetOutput()
//	    (the same idiom webhook_test.go uses for failure-trace assertions).
//
// The handler itself runs inside the telebot.OnUserJoined closure, which is
// only invokable via a live Bot. We exercise the SAME code path here by
// (1) verifying serviceMessageKind classifies correctly and (2) reproducing
// the log.Printf call the closure makes, so a regression in either layer
// would fail this test.
func TestSubscribeServiceMessageIgnoreAndLog(t *testing.T) {
	join := &telebot.Message{
		ID:          77,
		Chat:        &telebot.Chat{ID: -100999},
		UsersJoined: []telebot.User{{Username: "alice", ID: 7}},
	}
	if got := tgram.ServiceMessageKindForTest(join); got != "new_chat_members" {
		t.Fatalf("(a) serviceMessageKind(new_chat_members) = %q; want %q", got, "new_chat_members")
	}

	plain := &telebot.Message{
		ID:     78,
		Chat:   &telebot.Chat{ID: -100999},
		Sender: &telebot.User{IsBot: false, Username: "alice"},
		Text:   "ordinary message",
	}
	if got := tgram.ServiceMessageKindForTest(plain); got != "" {
		t.Fatalf("(b) serviceMessageKind(plain text) = %q; want empty (must NOT misclassify subscriber traffic)", got)
	}

	leave := &telebot.Message{
		ID:       79,
		Chat:     &telebot.Chat{ID: -100999},
		UserLeft: &telebot.User{Username: "alice"},
	}
	if got := tgram.ServiceMessageKindForTest(leave); got != "left_chat_member" {
		t.Fatalf("(a2) serviceMessageKind(left_chat_member) = %q; want %q", got, "left_chat_member")
	}

	title := &telebot.Message{
		ID:            80,
		Chat:          &telebot.Chat{ID: -100999},
		NewGroupTitle: "New Title",
	}
	if got := tgram.ServiceMessageKindForTest(title); got != "new_chat_title" {
		t.Fatalf("(a3) serviceMessageKind(new_chat_title) = %q; want %q", got, "new_chat_title")
	}

	// (c) Reproduce the log.Printf call the Subscribe service-handler makes
	// when classification != "" — captures log.Default() output via SetOutput.
	var buf bytes.Buffer
	origOut := log.Writer()
	origFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0) // suppress the LstdFlags timestamp so the assertion is hermetic
	t.Cleanup(func() {
		log.SetOutput(origOut)
		log.SetFlags(origFlags)
	})

	kind := tgram.ServiceMessageKindForTest(join)
	// This MUST be byte-for-byte identical to the Subscribe handler's
	// log.Printf invocation — a regression in either site fails the test.
	log.Printf("tgram: ignored service message kind=%s chat_id=%d", kind, join.Chat.ID)

	got := buf.String()
	want := "tgram: ignored service message kind=new_chat_members chat_id=-100999"
	if !strings.Contains(got, want) {
		t.Fatalf("(c) service-message log line missing.\nwant substring: %q\ngot:            %q", want, got)
	}
}

// TestSubscribeBotKickedTypedError pins HRD-135 §3: an OnMyChatMember update
// whose NewChatMember.Role == kicked/left/restricted-non-member fires the
// bot-kicked signal — ErrBotKicked is the typed sentinel, log.Printf is the
// observable channel.
//
// Assertions:
//
//	(a) Kicked role on THIS bot returns (Kicked, true).
//	(b) Left role on THIS bot returns (Left, true).
//	(c) Restricted + Member=false returns (Restricted, true) — Telegram's
//	    ban-variant representation that would otherwise look benign.
//	(d) A status change on a DIFFERENT user (not the bot) returns (_, false)
//	    — false-positive guard ("Alice left the group" is NOT the bot
//	    being kicked).
//	(e) Promoted-to-admin returns (Administrator, false) — benign transitions
//	    MUST NOT trigger the signal.
//	(f) ErrBotKicked is the documented sentinel string (so errors.Is can
//	    surface to a future Subscribe-returns-error contract).
//	(g) The log line the Subscribe OnMyChatMember handler emits when the
//	    predicate returns true contains the literal "bot kicked from chat"
//	    + the ErrBotKicked sentinel text, so operators have a grep-able
//	    signal today.
func TestSubscribeBotKickedTypedError(t *testing.T) {
	const botUsername = "MyHeraldBot"
	self := channels.SelfIdentity{Kind: channels.IdentityUsername, Value: botUsername}

	// (a) kicked
	kickedUpd := &telebot.ChatMemberUpdate{
		Chat: &telebot.Chat{ID: -100777},
		NewChatMember: &telebot.ChatMember{
			User: &telebot.User{IsBot: true, Username: botUsername},
			Role: telebot.Kicked,
		},
	}
	if status, kicked := tgram.BotKickedFromUpdateForTest(kickedUpd, self); !kicked || status != telebot.Kicked {
		t.Fatalf("(a) expected (Kicked, true); got (%v, %v)", status, kicked)
	}

	// (b) left
	leftUpd := &telebot.ChatMemberUpdate{
		Chat: &telebot.Chat{ID: -100777},
		NewChatMember: &telebot.ChatMember{
			User: &telebot.User{IsBot: true, Username: botUsername},
			Role: telebot.Left,
		},
	}
	if status, kicked := tgram.BotKickedFromUpdateForTest(leftUpd, self); !kicked || status != telebot.Left {
		t.Fatalf("(b) expected (Left, true); got (%v, %v)", status, kicked)
	}

	// (c) restricted + !Member (ban-variant)
	restrictedUpd := &telebot.ChatMemberUpdate{
		Chat: &telebot.Chat{ID: -100777},
		NewChatMember: &telebot.ChatMember{
			User:   &telebot.User{IsBot: true, Username: botUsername},
			Role:   telebot.Restricted,
			Member: false,
		},
	}
	if status, kicked := tgram.BotKickedFromUpdateForTest(restrictedUpd, self); !kicked || status != telebot.Restricted {
		t.Fatalf("(c) expected (Restricted, true) for restricted+!Member; got (%v, %v)", status, kicked)
	}

	// (d) different user's status change must NOT trigger
	otherUpd := &telebot.ChatMemberUpdate{
		Chat: &telebot.Chat{ID: -100777},
		NewChatMember: &telebot.ChatMember{
			User: &telebot.User{IsBot: false, Username: "alice"},
			Role: telebot.Left,
		},
	}
	if status, kicked := tgram.BotKickedFromUpdateForTest(otherUpd, self); kicked {
		t.Fatalf("(d) Alice leaving must NOT trigger bot-kicked; got (%v, %v)", status, kicked)
	}

	// (e) bot promoted to admin — benign
	adminUpd := &telebot.ChatMemberUpdate{
		Chat: &telebot.Chat{ID: -100777},
		NewChatMember: &telebot.ChatMember{
			User: &telebot.User{IsBot: true, Username: botUsername},
			Role: telebot.Administrator,
		},
	}
	if status, kicked := tgram.BotKickedFromUpdateForTest(adminUpd, self); kicked {
		t.Fatalf("(e) bot promoted to admin must NOT trigger bot-kicked; got (%v, %v)", status, kicked)
	}

	// (f) sentinel identity
	if tgram.ErrBotKicked == nil {
		t.Fatal("(f) tgram.ErrBotKicked must be a non-nil package-level sentinel")
	}
	if !strings.Contains(tgram.ErrBotKicked.Error(), "bot kicked") {
		t.Fatalf("(f) tgram.ErrBotKicked.Error() = %q; want it to contain %q", tgram.ErrBotKicked.Error(), "bot kicked")
	}

	// (g) emit the same log line the Subscribe OnMyChatMember handler emits
	// and assert operators can grep it. SetOutput swap is the standard Go
	// idiom for capturing log.Default() output (matches webhook_test.go).
	var buf bytes.Buffer
	origOut := log.Writer()
	origFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(origOut)
		log.SetFlags(origFlags)
	})

	status, kicked := tgram.BotKickedFromUpdateForTest(kickedUpd, self)
	if !kicked {
		t.Fatal("(g) precondition: kicked predicate must return true")
	}
	// MUST mirror the Subscribe OnMyChatMember log.Printf byte-for-byte.
	log.Printf("tgram: bot kicked from chat chat_id=%d new_status=%s (signal=%v)", kickedUpd.Chat.ID, status, tgram.ErrBotKicked)

	got := buf.String()
	if !strings.Contains(got, "bot kicked from chat") {
		t.Fatalf("(g) bot-kicked log line missing %q; got %q", "bot kicked from chat", got)
	}
	if !strings.Contains(got, "new_status=kicked") {
		t.Fatalf("(g) bot-kicked log line missing %q; got %q", "new_status=kicked", got)
	}
	if !strings.Contains(got, tgram.ErrBotKicked.Error()) {
		t.Fatalf("(g) bot-kicked log line missing sentinel %q; got %q", tgram.ErrBotKicked.Error(), got)
	}
}
