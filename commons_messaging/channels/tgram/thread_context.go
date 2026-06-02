package tgram

import (
	"strconv"

	telebot "gopkg.in/telebot.v3"

	"github.com/vasic-digital/herald/commons"
)

// threadContextFromReply extracts the conversational thread context that binds
// an inbound Telegram message to the message it is REPLYING to (operator mandate
// 2026-06-02). The result is a slice of commons.ThreadMessage in oldest→newest
// order, EXCLUDING the current inbound message itself (that lives in the event's
// Body). It is fed to the Claude Code dispatcher so a reply is bound by the
// thread's MEANING — a contribution to a thread, not an isolated answer.
//
// Telegram Bot API limitation (honest documentation):
//
//	getUpdates delivers ONLY the IMMEDIATE quoted parent of a reply, exposed by
//	telebot as msg.ReplyTo (the single message this one is a reply to). The Bot
//	API provides NO call to fetch an arbitrary full thread history for a basic
//	group chat — there is no getMessages / getThreadHistory equivalent for bots.
//	For forum topics, msg.ThreadID (message_thread_id) identifies the topic but
//	the Bot API STILL exposes no full-history fetch for it. Therefore the maximum
//	thread context achievable here is the single immediate parent message — which
//	is exactly the message this reply is bound to, so it is the load-bearing
//	context for the dispatcher. (A full multi-message history would require an
//	MTProto user-client, which this Bot-API adapter is not.)
//
// When msg.ReplyTo == nil (a fresh / top-level message that replies to nothing)
// the function returns nil — an empty ThreadContext is the correct signal that
// there is no prior thread to bind to.
//
// The function is robust and non-fatal by construction: it never returns an
// error and never panics on missing sub-fields, so inbound handlers can call it
// unconditionally without risking dropping the message over context extraction.
func threadContextFromReply(m *telebot.Message) []commons.ThreadMessage {
	if m == nil || m.ReplyTo == nil {
		return nil
	}
	parent := m.ReplyTo

	text := parent.Text
	if text == "" {
		// Media messages carry their human-authored text in Caption rather
		// than Text; fall back so a reply to a captioned photo still surfaces
		// the caption as the bound thread context.
		text = parent.Caption
	}

	tm := commons.ThreadMessage{
		Text:      text,
		Timestamp: parent.Time(), // telebot helper; time.Unix(0,0) when Unixtime unset
	}
	if parent.Sender != nil {
		tm.SenderHandle = senderHandle(parent.Sender)
		tm.SenderIsBot = parent.Sender.IsBot
	}

	return []commons.ThreadMessage{tm}
}

// senderHandle resolves a best-effort human-readable handle for a telebot user:
// @username first, then "FirstName LastName" (trimmed), then the numeric id.
// Never empty for a non-nil user (the numeric id is always available).
func senderHandle(u *telebot.User) string {
	if u == nil {
		return ""
	}
	if u.Username != "" {
		return u.Username
	}
	name := u.FirstName
	if u.LastName != "" {
		if name != "" {
			name += " "
		}
		name += u.LastName
	}
	if name != "" {
		return name
	}
	return strconv.FormatInt(u.ID, 10)
}
