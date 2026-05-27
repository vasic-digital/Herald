package channels_test

import (
	"testing"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons_messaging/channels"
)

// ev builds an InboundEvent whose Raw is stamped via the SAME production
// stamper IsSelfEcho reads — DRY against StampSender so the test can never
// drift from the wire-key contract the filter depends on.
func ev(isBot bool, kind channels.IdentityKind, value string) commons.InboundEvent {
	raw := map[string]any{}
	channels.StampSender(raw, isBot, kind, value)
	return commons.InboundEvent{Sender: commons.Recipient{Channel: "x", ChannelUserID: "1"}, Raw: raw}
}

// TestIsSelfEchoUsername pins the channel-agnostic Wave 6 §32.9 anti-echo
// guarantee for the username IdentityKind (Telegram). Three sub-assertions:
//
//	(a) THIS bot's own username → self-echo (dropped) — the echo-loop guard
//	(b) a DIFFERENT bot → KEPT (multi-bot collaboration is real traffic;
//	    this is the §107 anchor proving the filter is not over-broad)
//	(c) a human sender → KEPT (the runtime must still run for subscribers)
func TestIsSelfEchoUsername(t *testing.T) {
	self := channels.SelfIdentity{Kind: channels.IdentityUsername, Value: "herald_bot"}
	if !channels.IsSelfEcho(ev(true, channels.IdentityUsername, "herald_bot"), self) {
		t.Fatal("bot-own username should be self-echo")
	}
	if channels.IsSelfEcho(ev(true, channels.IdentityUsername, "other_bot"), self) {
		t.Fatal("different bot must NOT be self-echo (multi-bot is real traffic)")
	}
	if channels.IsSelfEcho(ev(false, channels.IdentityUsername, "alice"), self) {
		t.Fatal("human sender must NOT be self-echo")
	}
}

// TestIsSelfEchoUserID pins the same guarantee for the user_id IdentityKind
// (Slack bot_user_id). A matching bot id is self-echo; a different id is KEPT.
func TestIsSelfEchoUserID(t *testing.T) {
	self := channels.SelfIdentity{Kind: channels.IdentityUserID, Value: "U0HERALD"}
	if !channels.IsSelfEcho(ev(true, channels.IdentityUserID, "U0HERALD"), self) {
		t.Fatal("Slack bot_user_id match should be self-echo")
	}
	if channels.IsSelfEcho(ev(true, channels.IdentityUserID, "U0OTHER"), self) {
		t.Fatal("different Slack bot id must NOT be self-echo")
	}
}

// TestIsSelfEchoEmptySelfNeverEchoes pins the conservative-KEEP semantics:
// an empty self Value never classifies as echo, so a misconfigured runtime
// surfaces as DUPLICATE traffic (visible, auditable) rather than SILENT
// message loss. The real guard is Subscribe refusing to boot on empty self.
func TestIsSelfEchoEmptySelfNeverEchoes(t *testing.T) {
	if channels.IsSelfEcho(ev(true, channels.IdentityUsername, "herald_bot"), channels.SelfIdentity{}) {
		t.Fatal("empty self must never echo")
	}
}
