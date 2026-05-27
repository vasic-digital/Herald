package channels

import "github.com/vasic-digital/herald/commons"

// Raw-map keys the adapters stamp so the channel-agnostic filter can compare
// the sender's native identity against the bot's SelfIdentity without any
// channel-specific knowledge. The values are written by StampSender and read
// by IsSelfEcho — keep the two in lock-step.
const (
	RawSenderIsBot       = "sender_is_bot"
	RawSenderIdentityKnd = "sender_identity_kind"
	RawSenderIdentity    = "sender_identity"
)

// StampSender records the sender's native identity into raw so IsSelfEcho can
// compare it against the bot's SelfIdentity without channel-specific
// knowledge. Adapters call this from their Subscribe handlers after building
// the InboundEvent (pass ev.Raw). A nil map is a no-op — callers that build
// events without a Raw map simply skip the self-filter.
func StampSender(raw map[string]any, isBot bool, kind IdentityKind, value string) {
	if raw == nil {
		return
	}
	raw[RawSenderIsBot] = isBot
	raw[RawSenderIdentityKnd] = string(kind)
	raw[RawSenderIdentity] = value
}

// IsSelfEcho reports whether ev originates from THIS bot (self-echo) so the
// inbound runtime drops it before re-dispatching its own reply — the Wave 6
// §32.9 anti-echo-loop guarantee, now channel-agnostic.
//
// Scope is deliberately narrow: a DIFFERENT bot in the same conversation is
// KEPT (multi-bot collaboration is real subscriber traffic). An empty self
// Value never classifies as echo — Subscribe refuses to boot without a
// self-identity, so reaching here with empty self is a conservative KEEP that
// surfaces as duplicate traffic rather than silent message loss.
func IsSelfEcho(ev commons.InboundEvent, self SelfIdentity) bool {
	if self.Value == "" {
		return false
	}
	if ev.Raw == nil {
		return false
	}
	isBot, _ := ev.Raw[RawSenderIsBot].(bool)
	if !isBot {
		return false
	}
	kind, _ := ev.Raw[RawSenderIdentityKnd].(string)
	id, _ := ev.Raw[RawSenderIdentity].(string)
	return IdentityKind(kind) == self.Kind && id == self.Value
}
