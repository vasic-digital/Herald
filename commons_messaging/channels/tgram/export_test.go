package tgram

import (
	"context"
	"time"

	telebot "gopkg.in/telebot.v3"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons_messaging/channels"
)

// SetSleeperForTest injects a fake sleeper for HRD-134 send_ratelimit_test.go.
// The fake records the requested duration and returns immediately so
// retry-loop tests don't pay real wall-clock time. Production callers
// leave Adapter.sleeper nil; Send/SendReply default to realSleep.
func (a *Adapter) SetSleeperForTest(sleeper func(context.Context, time.Duration) error) {
	a.sleeper = sleeper
}

// SanitizeTgramErrorForTest exposes sanitizeTgramError for the HRD-133
// send_security_test.go assertions (unit-level proof that the helper
// scrubs the token substring — paired with the live httptest assertion
// in TestTgram_Send_ErrorDoesNotLeakToken).
func SanitizeTgramErrorForTest(msg, token string) string {
	return sanitizeTgramError(msg, token)
}

// ExtractRetryAfterForTest exposes extractRetryAfter so a HRD-134
// unit test can pin the telebot.FloodError → seconds mapping without
// driving the full Send path.
func ExtractRetryAfterForTest(err error) (int, bool) {
	return extractRetryAfter(err)
}

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

// ThreadContextFromReplyForTest exposes threadContextFromReply for the
// thread-context-awareness tests (operator mandate 2026-06-02). Production
// callers invoke it inside the OnText / OnEdited / media InboundEvent builders.
func ThreadContextFromReplyForTest(msg *telebot.Message) []commons.ThreadMessage {
	return threadContextFromReply(msg)
}

// BotKickedFromUpdateForTest exposes botKickedFromUpdate for HRD-135 §3
// tests — the predicate the OnMyChatMember closure consults before emitting
// the bot-kicked log line + ErrBotKicked sentinel.
func BotKickedFromUpdateForTest(upd *telebot.ChatMemberUpdate, self channels.SelfIdentity) (telebot.MemberStatus, bool) {
	return botKickedFromUpdate(upd, self)
}
