// Package tgram is the Telegram Bot API channel adapter per spec §11.1.
//
// Status: SCAFFOLD ONLY (HRD-011). The full implementation pulls in
// gopkg.in/telebot.v3 (or mymmrac/telego) and wires:
//
//   - sendMessage with MarkdownV2 parse mode (V1 tier).
//   - sendPhoto / sendDocument for attachments.
//   - InlineKeyboardMarkup with URL buttons + callback_data.
//   - Webhook ingress with X-Telegram-Bot-Api-Secret-Token verification.
//   - V2 advanced: callback_query handler, editMessageText, web_app
//     buttons, sendMediaGroup, forum-topic threads via
//     message_thread_id.
//
// Live API tests deferred until the operator provides a bot token; see
// HRD-008 quickstart compose for the integration-test harness.
package tgram

import (
	"context"
	"errors"
	"net/url"

	"github.com/vasic-digital/herald/commons"
)

// Adapter is the (stub) Telegram Bot API channel adapter.
type Adapter struct {
	botToken string
	chatID   string
}

// New parses tgram://<bot_token>/<chat_id>?tags=... and returns a stub
// adapter. The real implementation will dial the Bot API and verify
// credentials in HealthCheck.
func New(rawURL string) (*Adapter, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "tgram" {
		return nil, errors.New("tgram adapter requires tgram:// URL scheme")
	}
	if u.Host == "" || u.Path == "" || u.Path == "/" {
		return nil, errors.New("tgram:// URL must be tgram://<bot_token>/<chat_id>[?tags=...]")
	}
	return &Adapter{
		botToken: u.Host,
		chatID:   u.Path[1:], // strip leading "/"
	}, nil
}

// Name returns the canonical channel ID.
func (a *Adapter) Name() string { return string(commons.ChannelTelegram) }

// Capabilities (per spec §11.1 V1 tier).
func (a *Adapter) Capabilities() commons.Capabilities {
	return commons.Capabilities{
		Text:             true,
		Markdown:         true, // MarkdownV2
		HTML:             true,
		Attachments:      true,
		AttachmentMaxMiB: 50, // Telegram Bot API limit
		Threads:          true, // forum-topic via message_thread_id
		InteractiveURL:   true,
		InteractiveCall:  false, // V2 advanced (HRD-011 follow-up)
		DeliveryCeiling:  commons.DeliveryRouted,
	}
}

// Send is NOT YET IMPLEMENTED (HRD-011).
//
// The real implementation:
//
//  1. Build a sendMessage / sendPhoto / sendDocument call from msg.Body
//     + msg.Attachments.
//  2. Apply MarkdownV2 escaping where msg.Body.Markdown is present.
//  3. Render msg.Actions as InlineKeyboardMarkup (URL + callback_data).
//  4. Honor msg.Thread.ThreadID as message_thread_id when set.
//  5. Map Bot API response → commons.Receipt (Evidence=Routed).
func (a *Adapter) Send(ctx context.Context, msg commons.OutboundMessage) (commons.Receipt, error) {
	return commons.Receipt{}, errors.New("tgram adapter: not implemented (HRD-011)")
}

// Subscribe is NOT YET IMPLEMENTED (HRD-011).
//
// The real implementation runs Bot API getUpdates long-poll (25 s
// timeout) per spec §32.2, with a 30 s safety-net timer that re-fires
// getUpdates if the long-poll thread stalls.
func (a *Adapter) Subscribe(ctx context.Context, h commons.InboundHandler) error {
	return errors.New("tgram adapter: not implemented (HRD-011)")
}

// HealthCheck calls Bot API getMe — NOT YET IMPLEMENTED.
func (a *Adapter) HealthCheck(ctx context.Context) error {
	return errors.New("tgram adapter: not implemented (HRD-011)")
}
