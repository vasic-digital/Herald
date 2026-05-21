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
	"errors"
	"fmt"
	"net/url"
	"sync"

	"github.com/vasic-digital/herald/commons"
	telebot "gopkg.in/telebot.v3"
)

// Adapter is the Telegram Bot API channel adapter.
type Adapter struct {
	botToken string
	chatID   string
	bot      *telebot.Bot // lazy-initialized on first live call (HealthCheck/Send/Subscribe)
	botOnce  sync.Once    // guards bot construction across goroutines (Task 2 review carry-forward)
	botErr   error        // captured by botOnce if NewBot fails
}

// ensureBot lazily constructs a.bot exactly once across all goroutines.
// Subsequent calls return the cached error if init failed.
//
// telebot.NewBot itself dispatches getMe during construction (see
// telebot/bot.go:58: `user, err := bot.getMe()`), so this IS the live
// roundtrip — callers can then rely on a.bot.Me being populated.
func (a *Adapter) ensureBot() error {
	a.botOnce.Do(func() {
		bot, err := telebot.NewBot(telebot.Settings{Token: a.botToken})
		if err != nil {
			a.botErr = fmt.Errorf("tgram.ensureBot: connect to Bot API (getMe): %w", err)
			return
		}
		a.bot = bot
	})
	return a.botErr
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
		AttachmentMaxMiB: 50,   // Telegram Bot API limit
		Threads:          true, // forum-topic via message_thread_id
		InteractiveURL:   true,
		InteractiveCall:  false, // V2 advanced (HRD-011 follow-up)
		DeliveryCeiling:  commons.DeliveryRouted,
	}
}

// Send is implemented in send.go (live Bot API sendMessage).

// Subscribe is implemented in subscribe.go (live Bot API getUpdates long-poll).

// HealthCheck is implemented in healthcheck.go (live Bot API getMe).
