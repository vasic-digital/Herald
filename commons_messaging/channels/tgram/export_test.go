package tgram

import (
	telebot "gopkg.in/telebot.v3"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons_messaging/channels"
)

// SelfFilterForTest exposes the unexported shouldDropBotSelf helper for
// black-box _test packages (e.g. subscribe_test.go). Production callers
// must use Subscribe, which captures selfUsername from bot.Me at
// construction time and applies the filter inside each handler.
func SelfFilterForTest(selfUsername string) func(*telebot.Message) bool {
	return func(msg *telebot.Message) bool { return shouldDropBotSelf(msg, selfUsername) }
}

// BuildEditedEventForTest exposes buildEditedEvent for HRD-135 §1 tests.
// Production callers never need it directly — it's invoked only from the
// telebot OnEdited closure inside Subscribe.
func BuildEditedEventForTest(msg *telebot.Message) commons.InboundEvent {
	return buildEditedEvent(msg)
}

// ServiceMessageKindForTest exposes serviceMessageKind for HRD-135 §2 tests.
func ServiceMessageKindForTest(msg *telebot.Message) string {
	return serviceMessageKind(msg)
}

// BotKickedFromUpdateForTest exposes botKickedFromUpdate for HRD-135 §3
// tests — the predicate the OnMyChatMember closure consults before emitting
// the bot-kicked log line + ErrBotKicked sentinel.
func BotKickedFromUpdateForTest(upd *telebot.ChatMemberUpdate, self channels.SelfIdentity) (telebot.MemberStatus, bool) {
	return botKickedFromUpdate(upd, self)
}
