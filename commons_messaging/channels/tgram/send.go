package tgram

import (
	"context"
	"fmt"
	"strconv"
	"time"

	telebot "gopkg.in/telebot.v3"

	"github.com/vasic-digital/herald/commons"
)

// Send dispatches an OutboundMessage to the bot's configured chat via
// the live Bot API sendMessage endpoint with parse_mode=MarkdownV2 per
// spec §11.1 (V1 tier).
//
// Body selection: prefers msg.Body.Markdown if present (rendered with
// MarkdownV2 parse mode); otherwise falls back to msg.Body.Plain. The
// caller is responsible for MarkdownV2-escaping per Bot API rules
// (`_*[]()~\`>#+-=|{}.!` must be backslash-escaped in literal text).
//
// On success the returned Receipt carries the *chat-side* integer
// message ID assigned by Telegram (the `message_id` field in the Bot
// API response) — not a Herald-generated UUID. This is the §107 bluff
// guard: a fake/synthetic ID would mean the message never actually
// landed in the chat. Evidence is DeliveryRouted, matching the
// Capabilities.DeliveryCeiling declared for this channel — the Bot API
// 200 response confirms platform-stored-and-broadcast, not recipient
// transport delivery (which Telegram does not expose to bots without
// Business API read markers).
func (a *Adapter) Send(ctx context.Context, msg commons.OutboundMessage) (commons.Receipt, error) {
	_ = ctx // reserved — telebot.v3.3.8 does not thread ctx through sendMessage

	if err := a.ensureBot(); err != nil {
		return commons.Receipt{}, fmt.Errorf("tgram.Send: %w", err)
	}

	chatIDInt, err := strconv.ParseInt(a.chatID, 10, 64)
	if err != nil {
		return commons.Receipt{}, fmt.Errorf("tgram.Send: chatID %q not numeric: %w", a.chatID, err)
	}
	chat := &telebot.Chat{ID: chatIDInt}

	// Body selection: MarkdownV2 first (matches our parse_mode), else Plain.
	text := msg.Body.Markdown
	parseMode := telebot.ModeMarkdownV2
	if text == "" {
		text = msg.Body.Plain
		parseMode = "" // no parse_mode: Telegram renders as plain text
	}
	if text == "" {
		return commons.Receipt{}, fmt.Errorf("tgram.Send: OutboundMessage.Body has neither Markdown nor Plain content")
	}

	opts := &telebot.SendOptions{ParseMode: parseMode}

	// Honor msg.Thread.ThreadID as message_thread_id for forum-topic chats.
	if msg.Thread != nil && msg.Thread.ThreadID != "" {
		if tid, terr := strconv.Atoi(msg.Thread.ThreadID); terr == nil {
			opts.ThreadID = tid
		}
	}

	start := time.Now()
	sent, err := a.bot.Send(chat, text, opts)
	if err != nil {
		return commons.Receipt{}, fmt.Errorf("tgram.Send: sendMessage to chat %d: %w", chatIDInt, err)
	}
	if sent == nil || sent.ID == 0 {
		return commons.Receipt{}, fmt.Errorf("tgram.Send: empty Message in sendMessage response (§107 bluff guard — Telegram did not return a chat-side message_id)")
	}

	return commons.Receipt{
		Evidence:      commons.DeliveryRouted,
		ChannelMsgID:  strconv.Itoa(sent.ID),
		SentAt:        time.Now(),
		LatencyMillis: time.Since(start).Milliseconds(),
		Native: map[string]any{
			"message_id": sent.ID,
			"chat_id":    chatIDInt,
		},
	}, nil
}
