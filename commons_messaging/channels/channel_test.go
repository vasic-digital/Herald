package channels_test

import (
	"testing"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons_messaging/channels"
	"github.com/vasic-digital/herald/commons_messaging/channels/tgram"
)

// compile-time proof tgram satisfies the richer interface. This assignment is
// the §107 anti-bluff anchor for T1: a refactor that renamed a method or
// drifted a signature breaks the BUILD here, not a runtime assertion.
var _ channels.Channel = (*tgram.Adapter)(nil)

// TestTgramSatisfiesChannel: compile-time + identity assertion that tgram
// satisfies the richer interface. A pure-refactor regression (renamed method)
// breaks the build here.
func TestTgramSatisfiesChannel(t *testing.T) {
	var c channels.Channel = tgram.NewWithCreds("123:abc", "456")
	if c.Name() != string(commons.ChannelTelegram) {
		t.Fatalf("Name()=%q want %q", c.Name(), commons.ChannelTelegram)
	}
	if !c.Capabilities().Text {
		t.Fatal("tgram Capabilities().Text should be true")
	}
}

func TestSelfIdentityType(t *testing.T) { // pins the SelfIdentity shape T4 consumes
	id := channels.SelfIdentity{Kind: channels.IdentityUsername, Value: "herald_bot"}
	if id.Kind != channels.IdentityUsername || id.Value != "herald_bot" {
		t.Fatalf("SelfIdentity mismatch: %+v", id)
	}
}
