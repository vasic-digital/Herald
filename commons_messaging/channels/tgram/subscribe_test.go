package tgram_test

import (
	"testing"

	telebot "gopkg.in/telebot.v3"

	"github.com/vasic-digital/herald/commons_messaging/channels/tgram"
)

// TestSubscribeBotSelfFilter pins the Wave 6 anti-echo-loop guarantee per
// docs/superpowers/plans/2026-05-22-wave6-inbound-runtime.md T4 and §107
// anti-bluff anchor (CLAUDE.md §107.x): pherald MUST NOT re-dispatch its
// own outbound replies through the Claude Code pipeline.
//
// Three sub-assertions:
//
//	(a) bot-own message dropped — primary anti-echo guarantee
//	(b) human message kept — the inbound runtime must still run for users
//	(c) cross-bot message kept — different bot in the same chat is real
//	    subscriber traffic (multi-bot collaboration); filtering it would
//	    be too broad
//
// The third case is load-bearing: a "drop all bot messages" filter would
// pass (a) and (b) but break legitimate multi-bot scenarios silently.
func TestSubscribeBotSelfFilter(t *testing.T) {
	const selfUsername = "MyHeraldBot"
	filter := tgram.SelfFilterForTest(selfUsername)

	botOwn := &telebot.Message{
		Sender: &telebot.User{IsBot: true, Username: selfUsername},
		Text:   "echo loop bait",
	}
	if !filter(botOwn) {
		t.Fatal("(a) expected filter to drop bot-own message (echo loop risk)")
	}

	human := &telebot.Message{
		Sender: &telebot.User{IsBot: false, Username: "milos85vasic"},
		Text:   "ping",
	}
	if filter(human) {
		t.Fatal("(b) expected filter to KEEP human message")
	}

	otherBot := &telebot.Message{
		Sender: &telebot.User{IsBot: true, Username: "SomeOtherBot"},
		Text:   "cross-bot chatter",
	}
	if filter(otherBot) {
		t.Fatal("(c) expected filter to KEEP other-bot message (cross-bot collab is real subscriber traffic)")
	}

	// Belt-and-braces: nil Sender (channel post) must not panic; current
	// production code drops these via the `if msg == nil` guard, but
	// the filter itself must be safe.
	nilSender := &telebot.Message{Sender: nil, Text: "channel post"}
	if filter(nilSender) {
		t.Fatal("(d) expected filter to KEEP nil-Sender message (channel post — not a bot self message)")
	}
}
