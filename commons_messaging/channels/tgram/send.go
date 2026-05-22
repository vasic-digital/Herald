package tgram

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
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

// SendReply dispatches a text reply that quotes the original message via
// Telegram's reply_to_message_id field per Wave 6 T8 + Wave 6.5 T6
// outbound attachment fan-out.
//
// telebot.SendOptions.ReplyTo (verified at submodules/telebot/options.go:61)
// is the canonical field name; embedSendOptions at options.go:178 populates
// reply_to_message_id in the URL form payload when ReplyTo.ID != 0. This is
// the §107 anti-bluff anchor for the text reply — a SendReply that compiled
// cleanly but left opts.ReplyTo nil would pass type-checks while silently
// degrading every reply to a fresh message. The paired send_reply_test.go
// asserts the wire-byte form value directly; Wave 6 mutation gate (c)
// drops the assignment to prove the detector catches it.
//
// Wave 6.5 T6 attachment fan-out: when attachments is non-nil, for each
// commons.Attachment in order, dispatch by MIMEType to the appropriate
// telebot media type (Photo / Document / Voice / Audio / Video) and send
// as a separate Bot API call (sendPhoto / sendDocument / sendVoice /
// sendAudio / sendVideo). Each media call also carries opts.ReplyTo =
// original message so the text + every attachment thread under the same
// operator message at the same depth. The §107 anti-bluff anchor for the
// attachment path is the integration test against httptest — every real
// wire call (multipart/form-data body, send<Kind> URL path) is
// intercepted and asserted. Wave 6.5 mutation gate M5 plants a SendReply
// that skips the attachment loop; the single-photo test fails on the
// call count.
//
// commons.Attachment.Filename carries the on-disk content-addressed path
// per the Wave 6 T5 inbound side (subscribe.go buildEventWithAttachment
// writes Filename = ~/.herald/inbox/<sha>.<ext>). T6 reads from the same
// field for the outbound side; telebot.FromDisk(att.Filename) constructs
// the File. The display caption is the basename of the on-disk path.
//
// Returns the chat-side Telegram message_id of the text reply (the same
// kind of integer ID that Send returns in Receipt.ChannelMsgID).
// replyToID == 0 is the no-reply sentinel — opts.ReplyTo stays nil and
// the reply renders as a fresh message.
func (a *Adapter) SendReply(ctx context.Context, chatID int64, text string, replyToID int, attachments []commons.Attachment) (int, error) {
	_ = ctx // reserved — telebot.v3.3.8 does not thread ctx through sendMessage

	if err := a.ensureBot(); err != nil {
		return 0, fmt.Errorf("tgram.SendReply: %w", err)
	}
	if text == "" {
		return 0, fmt.Errorf("tgram.SendReply: empty text")
	}

	chat := &telebot.Chat{ID: chatID}
	textOpts := &telebot.SendOptions{}
	if replyToID > 0 {
		// Stub Message with only ID set — telebot's embedSendOptions reads
		// only ReplyTo.ID (options.go:178); the full Message struct is not
		// required for the URL form serialisation.
		textOpts.ReplyTo = &telebot.Message{ID: replyToID}
	}

	sentText, err := a.bot.Send(chat, text, textOpts)
	if err != nil {
		return 0, fmt.Errorf("tgram.SendReply: %w", err)
	}
	if sentText == nil || sentText.ID == 0 {
		return 0, fmt.Errorf("tgram.SendReply: empty Message in sendMessage response (§107 bluff guard — Telegram did not return a chat-side message_id)")
	}

	// Attachment fan-out. Each attachment is its own ReplyTo=original
	// call so all sit at the same thread depth as the text reply. Wave
	// 6.5 keeps it sequential (one wire call per attachment) — no
	// sendMediaGroup batching — so every attachment is a real, auditable
	// multipart upload (§107 anti-bluff anchor).
	for i, att := range attachments {
		if att.Filename == "" {
			return 0, fmt.Errorf("tgram.SendReply: attachment[%d] Filename empty (Wave 6 T5 convention: on-disk content-addressed path)", i)
		}
		media := buildTelebotMedia(att)
		mediaOpts := &telebot.SendOptions{}
		if replyToID > 0 {
			mediaOpts.ReplyTo = &telebot.Message{ID: replyToID}
		}
		if _, merr := a.bot.Send(chat, media, mediaOpts); merr != nil {
			return 0, fmt.Errorf("tgram.SendReply (attachment[%d] mime=%q): %w", i, att.MIMEType, merr)
		}
	}

	return sentText.ID, nil
}

// buildTelebotMedia maps a commons.Attachment to the appropriate telebot
// media struct per Wave 6.5 T6. Routing by MIMEType prefix:
//
//	image/*       → telebot.Photo      (endpoint sendPhoto)
//	audio/ogg*    → telebot.Voice      (endpoint sendVoice — Telegram waveform UI)
//	audio/*       → telebot.Audio      (endpoint sendAudio)
//	video/*       → telebot.Video      (endpoint sendVideo)
//	application/* → telebot.Document   (endpoint sendDocument)
//	*             → telebot.Document   (safe fallback)
//
// commons.Attachment.Filename carries the on-disk path (Wave 6 T5
// convention); the display caption/file name is the basename so the
// chat UI doesn't expose the sha256-content-addressed inbox path. Photo
// + Voice telebot structs do NOT carry FileName fields (verified
// submodules/telebot/media.go:75 + :322); Audio/Document/Video do.
func buildTelebotMedia(att commons.Attachment) telebot.Sendable {
	file := telebot.FromDisk(att.Filename)
	displayName := filepath.Base(att.Filename)
	mime := strings.ToLower(att.MIMEType)
	switch {
	case strings.HasPrefix(mime, "image/"):
		return &telebot.Photo{File: file, Caption: displayName}
	case mime == "audio/ogg" || strings.HasPrefix(mime, "audio/ogg;"):
		return &telebot.Voice{File: file, Caption: displayName, MIME: att.MIMEType}
	case strings.HasPrefix(mime, "audio/"):
		return &telebot.Audio{File: file, Caption: displayName, FileName: displayName, MIME: att.MIMEType}
	case strings.HasPrefix(mime, "video/"):
		return &telebot.Video{File: file, Caption: displayName, FileName: displayName, MIME: att.MIMEType}
	default:
		return &telebot.Document{File: file, Caption: displayName, FileName: displayName, MIME: att.MIMEType}
	}
}

// NewAdapterWithBaseURL is the test seam for httptest-based assertions of
// wire-byte details (e.g. reply_to_message_id form value in
// send_reply_test.go). The baseURL overrides telebot.Settings.URL; in
// production callers leave it unset and use New / NewWithStorage, which
// default to api.telegram.org (see submodules/telebot/bot.go:29-30).
//
// Token is the raw bot token (no "bot" prefix — telebot adds it); chatID
// is the numeric chat ID as a decimal string, matching the existing
// Adapter.chatID convention.
func NewAdapterWithBaseURL(token, chatID, baseURL string) *Adapter {
	return &Adapter{
		botToken: token,
		chatID:   chatID,
		baseURL:  baseURL,
	}
}
