// Package tgram is qaherald's Telegram client impersonator.
//
// Unlike commons_messaging/channels/tgram (the Herald outbound channel
// adapter), this package is a READ+WRITE long-poll client: every Wave 5
// scenario uses it to Send text, Upload attachments, Download
// round-tripped files (via the getFile fileID surface), and
// WaitForMessage / WaitForReply on the inbox.
//
// The bot token authorizes against Telegram's Bot API; qaherald
// impersonates operator-side traffic by addressing the configured
// chatID directly.
//
// §107 anti-bluff anchor: every Send / Upload crosses an HTTPS socket
// to api.telegram.org — telebot.NewBot dispatches getMe at
// construction (the canonical live-roundtrip smoke), and the Wave 5
// T10 mutation gate (b) plants a stub that fakes a message_id without
// crossing the network. The deny-path scenario's
// "zero TG messages observed" assertion catches it because the stub
// erroneously records non-zero.
//
// Scope: the live getMe test in client_test.go is the ONLY test that
// dispatches over the real network — paid surfaces (Send, Upload)
// remain scenario-driven so CI never burns Telegram API budget. The
// HERALD_TGRAM_BOT_TOKEN env var gates the live test (SKIP-with-reason
// per Universal §11.4.3 when unset).
package tgram

import (
	"context"
	"fmt"
	"io"
	"time"

	tele "gopkg.in/telebot.v3"
)

// Client wraps a *tele.Bot with a buffered inbox channel populated by
// the long-poll loop. It is safe for concurrent use by scenario
// orchestrators.
type Client struct {
	bot    *tele.Bot
	chatID int64
	inbox  chan tele.Message
	cancel context.CancelFunc
}

// NewClient constructs a Telegram client by dispatching getMe (telebot
// does this internally during NewBot) and wiring text / photo /
// document / voice handlers that funnel inbound messages into the
// buffered inbox channel.
//
// chatID may be 0 for read-only smoke usage (the live getMe test
// passes 0 because Send is never exercised there). For live scenarios
// chatID MUST be the operator-supplied destination chat.
func NewClient(token string, chatID int64) (*Client, error) {
	pref := tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}
	bot, err := tele.NewBot(pref)
	if err != nil {
		return nil, fmt.Errorf("tgram: NewBot: %w", err)
	}
	// telebot.NewBot dispatches getMe internally (submodules/telebot/bot.go
	// :58). bot.Me should be populated; defend against an empty identity
	// because the §107 anti-bluff guard relies on a real bot username.
	if bot.Me == nil || bot.Me.Username == "" {
		return nil, fmt.Errorf("tgram: getMe returned empty bot identity")
	}
	c := &Client{
		bot:    bot,
		chatID: chatID,
		inbox:  make(chan tele.Message, 64),
	}
	bot.Handle(tele.OnText, func(ctx tele.Context) error {
		c.inbox <- *ctx.Message()
		return nil
	})
	bot.Handle(tele.OnPhoto, func(ctx tele.Context) error {
		c.inbox <- *ctx.Message()
		return nil
	})
	bot.Handle(tele.OnDocument, func(ctx tele.Context) error {
		c.inbox <- *ctx.Message()
		return nil
	})
	bot.Handle(tele.OnVoice, func(ctx tele.Context) error {
		c.inbox <- *ctx.Message()
		return nil
	})
	pCtx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	go func() {
		bot.Start()
		<-pCtx.Done()
		bot.Stop()
	}()
	return c, nil
}

// Send delivers a text message to the configured chat and returns the
// Telegram-assigned message_id. The message crosses an HTTPS socket to
// api.telegram.org — there is no caching or stub path.
func (c *Client) Send(text string) (int, error) {
	msg, err := c.bot.Send(&tele.Chat{ID: c.chatID}, text)
	if err != nil {
		return 0, fmt.Errorf("tgram: Send: %w", err)
	}
	if msg == nil || msg.ID == 0 {
		return 0, fmt.Errorf("tgram: Send: empty response (§107 bluff guard — no chat-side message_id)")
	}
	return msg.ID, nil
}

// Upload sends a file (photo / voice / document, picked from
// contentType) to the configured chat. Returns the chat-side
// message_id plus the Telegram-side fileID (the round-trip handle the
// caller passes to Download for sha256 verification).
//
// contentType selection:
//   - image/* → tele.Photo (the bot API picks an array of size
//     variants; we return the LARGEST photo's fileID per §107 fidelity)
//   - audio/ogg or audio/opus → tele.Voice
//   - everything else → tele.Document with FileName preserved so the
//     receiver sees the original filename instead of Telegram's
//     synthetic name
func (c *Client) Upload(r io.Reader, contentType, filename string) (msgID int, fileID string, err error) {
	chat := &tele.Chat{ID: c.chatID}
	var sent *tele.Message
	switch {
	case isImageContentType(contentType):
		photo := &tele.Photo{File: tele.FromReader(r)}
		sent, err = c.bot.Send(chat, photo)
		if err != nil {
			return 0, "", fmt.Errorf("tgram: Upload photo: %w", err)
		}
		if sent == nil || sent.Photo == nil {
			return 0, "", fmt.Errorf("tgram: Upload photo: empty response Photo (§107 bluff guard)")
		}
		fileID = sent.Photo.FileID
	case isVoiceContentType(contentType):
		voice := &tele.Voice{File: tele.FromReader(r)}
		sent, err = c.bot.Send(chat, voice)
		if err != nil {
			return 0, "", fmt.Errorf("tgram: Upload voice: %w", err)
		}
		if sent == nil || sent.Voice == nil {
			return 0, "", fmt.Errorf("tgram: Upload voice: empty response Voice (§107 bluff guard)")
		}
		fileID = sent.Voice.FileID
	default:
		doc := &tele.Document{File: tele.FromReader(r), FileName: filename}
		sent, err = c.bot.Send(chat, doc)
		if err != nil {
			return 0, "", fmt.Errorf("tgram: Upload document: %w", err)
		}
		if sent == nil || sent.Document == nil {
			return 0, "", fmt.Errorf("tgram: Upload document: empty response Document (§107 bluff guard)")
		}
		fileID = sent.Document.FileID
	}
	if sent.ID == 0 {
		return 0, "", fmt.Errorf("tgram: Upload: empty message_id (§107 bluff guard)")
	}
	if fileID == "" {
		return 0, "", fmt.Errorf("tgram: Upload: empty fileID (§107 bluff guard — round-trip impossible)")
	}
	return sent.ID, fileID, nil
}

// Download fetches a Telegram-side file by its fileID. The two-step
// dance — FileByID refreshes the File.FilePath, then Bot.File issues
// the actual GET against api.telegram.org/file/bot<token>/<path> —
// matches the telebot v3.3.8 surface (submodules/telebot/bot.go:967
// and :1011). The returned io.ReadCloser MUST be Closed by the caller
// after reading.
func (c *Client) Download(fileID string) (io.ReadCloser, error) {
	f, err := c.bot.FileByID(fileID)
	if err != nil {
		return nil, fmt.Errorf("tgram: Download FileByID: %w", err)
	}
	rc, err := c.bot.File(&f)
	if err != nil {
		return nil, fmt.Errorf("tgram: Download File: %w", err)
	}
	if rc == nil {
		return nil, fmt.Errorf("tgram: Download: nil ReadCloser (§107 bluff guard)")
	}
	return rc, nil
}

// WaitForMessage drains the inbox until a message matching predicate
// arrives or timeout elapses. Returns the first match or
// context.DeadlineExceeded.
func (c *Client) WaitForMessage(timeout time.Duration, predicate func(tele.Message) bool) (tele.Message, error) {
	deadline := time.After(timeout)
	for {
		select {
		case m := <-c.inbox:
			if predicate(m) {
				return m, nil
			}
		case <-deadline:
			return tele.Message{}, context.DeadlineExceeded
		}
	}
}

// WaitForReply waits for a message whose ReplyTo.ID equals toMsgID
// AND that also satisfies innerPredicate. Wraps WaitForMessage with
// the reply-chain predicate.
func (c *Client) WaitForReply(timeout time.Duration, toMsgID int, innerPredicate func(tele.Message) bool) (tele.Message, error) {
	return c.WaitForMessage(timeout, func(m tele.Message) bool {
		if m.ReplyTo == nil || m.ReplyTo.ID != toMsgID {
			return false
		}
		if innerPredicate == nil {
			return true
		}
		return innerPredicate(m)
	})
}

// Close stops the long-poll loop. Idempotent.
func (c *Client) Close() error {
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	return nil
}

// Bot exposes the underlying *tele.Bot for tests and advanced callers
// that need direct Bot API access (e.g. live-getMe smoke assertions on
// bot.Me.Username). Production scenario code SHOULD use Send / Upload
// / Download / WaitFor* instead.
func (c *Client) Bot() *tele.Bot { return c.bot }

// isImageContentType returns true for the image/* MIME family.
func isImageContentType(ct string) bool {
	return len(ct) >= 6 && ct[:6] == "image/"
}

// isVoiceContentType returns true for the audio/ogg + audio/opus voice
// MIME types Telegram accepts as tele.Voice. Other audio types fall
// through to tele.Document (Telegram does not render them as voice
// notes).
func isVoiceContentType(ct string) bool {
	return ct == "audio/ogg" || ct == "audio/opus"
}
