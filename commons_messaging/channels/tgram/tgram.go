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
	"fmt"
	"net/url"
	"strconv"
	"sync"

	db "digital.vasic.database/pkg/database"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons_messaging/channels"
	telebot "gopkg.in/telebot.v3"
)

// Adapter is the Telegram Bot API channel adapter.
type Adapter struct {
	botToken string
	chatID   string
	baseURL  string       // override for telebot.Settings.URL; "" => telebot default (api.telegram.org). Test seam — see NewAdapterWithBaseURL.
	bot      *telebot.Bot // lazy-initialized on first live call (HealthCheck/Send/Subscribe)
	botOnce  sync.Once    // guards bot construction across goroutines (Task 2 review carry-forward)
	botErr   error        // captured by botOnce if NewBot fails
	pool     db.Database  // optional; nil = persistence disabled (Send-only adapter)
}

// ensureBot lazily constructs a.bot exactly once across all goroutines.
// Subsequent calls return the cached error if init failed.
//
// telebot.NewBot itself dispatches getMe during construction (see
// telebot/bot.go:58: `user, err := bot.getMe()`), so this IS the live
// roundtrip — callers can then rely on a.bot.Me being populated.
func (a *Adapter) ensureBot() error {
	a.botOnce.Do(func() {
		settings := telebot.Settings{Token: a.botToken}
		if a.baseURL != "" {
			// Test seam: NewAdapterWithBaseURL routes the bot through an
			// httptest server (Wave 6 T8 wire-byte assertions). Production
			// callers leave baseURL empty and telebot defaults to
			// api.telegram.org (see submodules/telebot/bot.go:29-30).
			settings.URL = a.baseURL
		}
		bot, err := telebot.NewBot(settings)
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
//
// §107 anti-bluff (Wave 5 T8 live finding, 2026-05-22): real Telegram
// bot tokens are formatted "<numeric_id>:<alphanumeric_hmac>" — the
// embedded colon collides with Go's url.Parse "host:port" rule and
// produces "invalid port" errors. New therefore detects the canonical
// "tgram://<id>:<hmac>/<chat>" shape and short-circuits the url.Parse
// path entirely. Callers that already have token + chat_id in hand
// SHOULD use NewWithCreds (below) instead; New's URL form remains for
// op-supplied config strings (env, YAML).
func New(rawURL string) (*Adapter, error) {
	// Short-circuit the canonical "tgram://<id>:<hmac>/<chat>" shape so
	// real Telegram bot tokens (which contain a colon as part of the
	// token itself, not a host:port separator) don't trip url.Parse.
	const prefix = "tgram://"
	if len(rawURL) > len(prefix) && rawURL[:len(prefix)] == prefix {
		rest := rawURL[len(prefix):]
		// Find the path-separator that delimits creds from chat_id.
		for i := 0; i < len(rest); i++ {
			if rest[i] == '/' {
				cred := rest[:i]
				chat := rest[i+1:]
				if cred == "" || chat == "" {
					break
				}
				// Strip any ?query suffix on chat (e.g. ?tags=foo).
				for j := 0; j < len(chat); j++ {
					if chat[j] == '?' {
						chat = chat[:j]
						break
					}
				}
				return NewWithCreds(cred, chat), nil
			}
		}
	}
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

// NewWithCreds constructs a tgram Adapter from already-parsed token +
// chat_id pair, bypassing the url.Parse codepath entirely. This is the
// recommended constructor for callers that already hold the credentials
// (e.g. pherald main.go reading HERALD_TGRAM_BOT_TOKEN from env). Real
// Telegram bot tokens contain a colon ("<numeric_id>:<alphanumeric>")
// which trips Go's url.Parse host:port rule; bypassing url.Parse here
// makes the §107 live-evidence path work end-to-end without env-side
// percent-encoding contortions.
//
// Returns a non-nil *Adapter (never nil) — empty token or chatID will
// be caught later by ensureBot (telebot.NewBot rejects empty tokens).
// Surfacing the error at first-call rather than at constructor time
// matches the existing lazy-init pattern in Adapter.ensureBot.
func NewWithCreds(botToken, chatID string) *Adapter {
	return &Adapter{
		botToken: botToken,
		chatID:   chatID,
	}
}

// NewWithStorage constructs an Adapter that persists outbound_delivery_evidence
// rows to the given live pool. Persistence happens AFTER a successful
// sendMessage — a Send-then-persist failure leaves the message delivered
// without a persistence row, which is a known limitation. Per §107 we prefer
// dropping a persistence row over double-sending the message: the user has
// already received the Telegram notification; silently re-sending would be
// the bigger UX bug.
//
// Callers wanting persistence MUST use SendForTenant (which routes through
// the tenant RLS context); the plain Send method on this Adapter remains
// pool-agnostic so existing in-process callers don't accidentally pick up
// a persistence dependency.
func NewWithStorage(rawURL string, pool db.Database) (*Adapter, error) {
	a, err := New(rawURL)
	if err != nil {
		return nil, err
	}
	a.pool = pool
	return a, nil
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

// --- Wave 7 T1: channels.Channel interface satisfaction ---
//
// These three methods make *Adapter satisfy commons_messaging/channels.Channel
// (the richer inbound interface that embeds commons.Channel). They are thin
// adapters over the existing Wave 6 substrate — pure refactor, no behaviour
// change. The native int64/int SendReply (send.go) is intentionally left
// untouched (its migration to the generic signature is T5's scope).

// BotSelfIdentity returns the bot's @username via getMe (ensureBot IS the live
// roundtrip — telebot.NewBot dispatches getMe synchronously). Empty username =
// §107 echo-loop hazard; inbound runtime refuses to boot on it.
func (a *Adapter) BotSelfIdentity(ctx context.Context) (channels.SelfIdentity, error) {
	_ = ctx // telebot.v3 getMe is not ctx-aware
	if err := a.ensureBot(); err != nil {
		return channels.SelfIdentity{}, fmt.Errorf("tgram.BotSelfIdentity: %w", err)
	}
	if a.bot == nil || a.bot.Me == nil || a.bot.Me.Username == "" {
		return channels.SelfIdentity{}, errors.New("tgram.BotSelfIdentity: getMe empty username (echo-loop hazard)")
	}
	return channels.SelfIdentity{Kind: channels.IdentityUsername, Value: a.bot.Me.Username}, nil
}

// SendReplyGeneric adapts the string-typed channels.Channel reply method to the
// native int64/int Telegram SendReply (send.go). recipient.ChannelUserID =
// decimal chat_id; replyToID = decimal message_id ("" => no reply). Returns
// decimal message_id.
func (a *Adapter) SendReplyGeneric(ctx context.Context, recipient commons.Recipient, body, replyToID string, attachments []commons.Attachment) (string, error) {
	chatID, err := strconv.ParseInt(recipient.ChannelUserID, 10, 64)
	if err != nil {
		return "", fmt.Errorf("tgram.SendReplyGeneric: chatID %q not numeric: %w", recipient.ChannelUserID, err)
	}
	replyTo := 0
	if replyToID != "" {
		if r, perr := strconv.Atoi(replyToID); perr == nil {
			replyTo = r
		}
	}
	id, err := a.SendReply(ctx, chatID, body, replyTo, attachments)
	if err != nil {
		return "", err
	}
	return strconv.Itoa(id), nil
}

// DownloadAttachment (method) satisfies channels.Channel — routes the package-
// level DownloadAttachment (attachments.go) through ensureBot's live bot. (T3
// migrates the package func to per-channel inbox; this method is the stable
// interface seam.)
func (a *Adapter) DownloadAttachment(ctx context.Context, externalID, mime string) (string, string, error) {
	if err := a.ensureBot(); err != nil {
		return "", "", fmt.Errorf("tgram.DownloadAttachment: %w", err)
	}
	return DownloadAttachment(ctx, a.bot, externalID, mime)
}
