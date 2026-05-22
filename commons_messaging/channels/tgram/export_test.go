package tgram

import telebot "gopkg.in/telebot.v3"

// SelfFilterForTest exposes the unexported shouldDropBotSelf helper for
// black-box _test packages (e.g. subscribe_test.go). Production callers
// must use Subscribe, which captures selfUsername from bot.Me at
// construction time and applies the filter inside each handler.
func SelfFilterForTest(selfUsername string) func(*telebot.Message) bool {
	return func(msg *telebot.Message) bool { return shouldDropBotSelf(msg, selfUsername) }
}
