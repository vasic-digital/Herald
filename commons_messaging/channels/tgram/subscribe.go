package tgram

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	telebot "gopkg.in/telebot.v3"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons_messaging/channels"
)

// shouldDropBotSelf returns true when msg originates from THIS bot
// (self-echo), so the caller can short-circuit before re-dispatching its
// own outbound reply through the Claude Code pipeline.
//
// Wave 6 anti-echo-loop guarantee per plan T4 / CLAUDE.md §107.x:
// without this filter, every reply pherald posts to a Telegram group is
// re-delivered by getUpdates and re-dispatched to claude --resume —
// hallucinating to itself in an infinite loop on the operator's quota.
//
// Scope is deliberately narrow: cross-bot messages (a DIFFERENT bot in
// the same chat) are KEPT. Multi-bot collaboration is real subscriber
// traffic; a "drop all bot messages" filter would be too broad.
//
// Wave 7 T4 status: the LIVE Subscribe path now routes through the
// channel-agnostic channels.IsSelfEcho (see stampAndIsSelfEcho below); this
// helper is preserved byte-for-byte because (a) the Wave 6 M1 paired-mutation
// gate (tests/test_wave6_mutation_meta.sh) anchors its exact-text regex here
// and (b) TestSubscribeBotSelfFilter exercises it directly. Both express the
// SAME §32.9 invariant the live path now expresses generically — the gate
// stays load-bearing on the tgram-native shape while production runs on the
// generalized filter.
func shouldDropBotSelf(msg *telebot.Message, selfUsername string) bool {
	if msg == nil || msg.Sender == nil {
		return false
	}
	if !msg.Sender.IsBot {
		return false
	}
	return msg.Sender.Username == selfUsername
}

// stampAndIsSelfEcho stamps the Telegram sender's native identity (IsBot +
// @username) into ev.Raw via the channel-agnostic stamper, then asks
// channels.IsSelfEcho whether ev is THIS bot's own echo. This is the single
// live self-filter path for all four inbound handlers — replacing the Wave 6
// per-handler shouldDropBotSelf(msg, selfUsername) early-returns with one
// channel-agnostic check fed by self captured at Subscribe boot.
func stampAndIsSelfEcho(ev commons.InboundEvent, msg *telebot.Message, self channels.SelfIdentity) bool {
	senderBot, senderName := false, ""
	if msg.Sender != nil {
		senderBot = msg.Sender.IsBot
		senderName = msg.Sender.Username
	}
	channels.StampSender(ev.Raw, senderBot, channels.IdentityUsername, senderName)
	return channels.IsSelfEcho(ev, self)
}

// Subscribe runs the live getUpdates long-poll loop until ctx is cancelled,
// dispatching each inbound text message to h.Handle.
//
// Per spec §32.2:
//   - 25s telebot.LongPoller timeout (Telegram Bot API long-poll window).
//   - 30s observational safety-net timer that fires if no updates flow.
//     telebot.LongPoller has its own internal retry on getUpdates errors,
//     so the safety-net is observational only (future HRD: OTel span/metric).
//
// §107 bluff guard: a Subscribe that returns nil without ever invoking h
// would be a bluff (the loop "ran" but never dispatched). The integration
// test asserts ≥1 handler invocation from an operator-hand-sent message —
// proving getUpdates actually pulled real updates.
//
// Wave 6 bot self-filter: bot.Me.Username is captured here (populated by
// telebot.NewBot via getMe synchronously) and threaded into the OnText
// closure. If Me.Username is empty we refuse to boot — an unfiltered
// runtime is an echo-loop hazard. Cross-bot messages remain dispatchable.
//
// Implementation note: telebot.v3.3.8 requires Settings.Poller at Bot
// construction time — LongPoller cannot be attached to an existing *Bot.
// HealthCheck/Send use the ensureBot-managed a.bot (no poller). Subscribe
// therefore constructs its own *telebot.Bot here with the LongPoller wired.
func (a *Adapter) Subscribe(ctx context.Context, h commons.InboundHandler) error {
	bot, err := telebot.NewBot(telebot.Settings{
		Token:  a.botToken,
		Poller: &telebot.LongPoller{Timeout: 25 * time.Second},
	})
	if err != nil {
		return fmt.Errorf("tgram.Subscribe: connect with poller: %w", err)
	}

	// Wave 7 T4: capture the channel-native SelfIdentity once at boot. We use
	// the @username from this poller's own bot.Me (populated synchronously by
	// telebot.NewBot via getMe) rather than a.BotSelfIdentity — Subscribe
	// constructs its OWN *Bot here (the LongPoller cannot be attached to the
	// ensureBot-managed a.bot), so this bot.Me is the authoritative self for
	// the loop that actually receives updates. self is then threaded into
	// every handler's channels.IsSelfEcho check (channel-agnostic).
	selfUsername := ""
	if bot.Me != nil {
		selfUsername = bot.Me.Username
	}
	if selfUsername == "" {
		// telebot populates bot.Me synchronously via getMe inside NewBot.
		// An empty Username here means getMe returned a degenerate user
		// record (or Offline mode was used). Refuse to boot — running
		// without a self-filter is the §107 echo-loop hazard the gate
		// is designed to catch.
		return fmt.Errorf("tgram.Subscribe: bot.Me.Username unset after NewBot — getMe likely failed; refusing to boot without self-filter (echo-loop hazard)")
	}
	self := channels.SelfIdentity{Kind: channels.IdentityUsername, Value: selfUsername}

	bot.Handle(telebot.OnText, func(c telebot.Context) error {
		msg := c.Message()
		if msg == nil {
			return nil
		}
		ev := commons.InboundEvent{
			Sender: commons.Recipient{
				Channel:       string(commons.ChannelTelegram),
				ChannelUserID: strconv.FormatInt(msg.Chat.ID, 10),
			},
			Body: commons.Body{
				Plain: msg.Text,
			},
			Raw: map[string]any{
				"message_id":        msg.ID,
				"chat_id":           msg.Chat.ID,
				"message_thread_id": msg.ThreadID,
				"text":              msg.Text,
			},
		}
		// Wave 7 T4: stamp the sender's native identity into ev.Raw and drop
		// THIS bot's own echo via the channel-agnostic channels.IsSelfEcho
		// (replaces the Wave 6 shouldDropBotSelf early-return).
		if stampAndIsSelfEcho(ev, msg, self) {
			return nil
		}
		// T4 review carry-forward: only set Thread for actual forum-topic
		// messages. msg.ThreadID == 0 is Telegram's "no topic" sentinel and
		// must NOT surface as ThreadID="0" — that's a bluff thread identity
		// that would mislead downstream Slack/Discord bridges.
		if msg.ThreadID != 0 {
			ev.Thread = &commons.ConversationRef{
				Channel:  commons.ChannelTelegram,
				ThreadID: strconv.Itoa(msg.ThreadID),
			}
		}
		return h.Handle(ctx, ev)
	})

	// Wave 6 T5 — photo / document / voice handlers. Each one:
	//   1. content-addresses the file under ~/.herald/inbox/tgram/<sha>.<ext>
	//   2. drops bot-own (self-echo) messages via the channel-agnostic
	//      stampAndIsSelfEcho (Wave 7 T4 — replaces shouldDropBotSelf) before
	//      dispatch, so the bot never re-dispatches its own outbound media.
	//   3. dispatches an InboundEvent with Attachments[] populated so the
	//      Claude Code pre-text renderer (T3) sees the file alongside the
	//      caption text.
	//
	// MIME-type sources:
	//   - Photo:    Telegram never sends a MIME for photos; we hardcode
	//               image/jpeg (Telegram always re-encodes uploads to jpg).
	//   - Document: msg.Document.MIME is the subscriber-supplied MIME;
	//               fall back to application/octet-stream when empty.
	//   - Voice:    Telegram voice notes are always OggOpus → audio/ogg.
	bot.Handle(telebot.OnPhoto, func(c telebot.Context) error {
		msg := c.Message()
		if msg == nil || msg.Photo == nil {
			return nil
		}
		const mime = "image/jpeg"
		path, sumHex, err := DownloadAttachment(ctx, bot, msg.Photo.FileID, mime)
		if err != nil {
			return fmt.Errorf("tgram.Subscribe OnPhoto: download: %w", err)
		}
		ev := buildEventWithAttachment(msg, msg.Caption, path, sumHex, mime, msg.Photo.FileSize)
		if stampAndIsSelfEcho(ev, msg, self) {
			return nil
		}
		return h.Handle(ctx, ev)
	})

	bot.Handle(telebot.OnDocument, func(c telebot.Context) error {
		msg := c.Message()
		if msg == nil || msg.Document == nil {
			return nil
		}
		mime := msg.Document.MIME
		if mime == "" {
			mime = "application/octet-stream"
		}
		path, sumHex, err := DownloadAttachment(ctx, bot, msg.Document.FileID, mime)
		if err != nil {
			return fmt.Errorf("tgram.Subscribe OnDocument: download: %w", err)
		}
		ev := buildEventWithAttachment(msg, msg.Caption, path, sumHex, mime, msg.Document.FileSize)
		if stampAndIsSelfEcho(ev, msg, self) {
			return nil
		}
		return h.Handle(ctx, ev)
	})

	bot.Handle(telebot.OnVoice, func(c telebot.Context) error {
		msg := c.Message()
		if msg == nil || msg.Voice == nil {
			return nil
		}
		const mime = "audio/ogg"
		path, sumHex, err := DownloadAttachment(ctx, bot, msg.Voice.FileID, mime)
		if err != nil {
			return fmt.Errorf("tgram.Subscribe OnVoice: download: %w", err)
		}
		ev := buildEventWithAttachment(msg, msg.Caption, path, sumHex, mime, msg.Voice.FileSize)
		if stampAndIsSelfEcho(ev, msg, self) {
			return nil
		}
		return h.Handle(ctx, ev)
	})

	// 30s safety-net per §32.2 — observational only; telebot.LongPoller
	// self-heals via its own getUpdates retry loop. A future HRD will emit
	// OTel spans/metrics here when a stall is detected.
	stallTicker := time.NewTicker(30 * time.Second)
	defer stallTicker.Stop()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-stallTicker.C:
				// Reserved for future OTel instrumentation (HRD-011 follow-up).
			}
		}
	}()

	// telebot.Bot.Start is blocking; run it in a goroutine so we can watch
	// ctx for cancellation. Bot.Stop halts the poller cleanly.
	go bot.Start()
	defer bot.Stop()
	<-ctx.Done()
	return ctx.Err()
}

// buildEventWithAttachment mirrors the OnText InboundEvent shape (Sender,
// Body.Plain, Raw, optional Thread) and appends a single commons.Attachment
// whose Filename is the on-disk content-addressed path (so the Claude Code
// pre-text formatter in T3 can list it verbatim), MIMEType + SizeBytes
// carry the descriptor, and CID carries the sha256 for downstream
// traceability (the Helix Attachment struct has no dedicated Sha256
// field; CID is the closest stable carrier).
//
// Reader is a closure that lazy-opens the file on disk — adapters that
// need the bytes (Email inline-image, Slack file_upload) call this; the
// Claude Code dispatcher only needs the path + sha256 + descriptor.
//
// caption is whatever the subscriber typed alongside the media (Telegram
// allows captions on photo/document/voice); it goes into Body.Plain so
// the LLM sees the user's intent in natural language.
func buildEventWithAttachment(msg *telebot.Message, caption, path, sumHex, mime string, sizeBytes int64) commons.InboundEvent {
	ev := commons.InboundEvent{
		Sender: commons.Recipient{
			Channel:       string(commons.ChannelTelegram),
			ChannelUserID: strconv.FormatInt(msg.Chat.ID, 10),
		},
		Body: commons.Body{
			Plain: caption,
		},
		Attachments: []commons.Attachment{
			{
				Filename:  path,
				MIMEType:  mime,
				SizeBytes: sizeBytes,
				// CID carries the sha256 hex so downstream consumers
				// (diary, Claude Code dispatcher, QA transcript) can
				// re-derive content-addressing without reading the
				// file. The Helix commons.Attachment struct has no
				// dedicated Sha256 field; CID is the closest stable
				// carrier and is otherwise unused for inbound media
				// (it's an Email-only inline-image hint).
				CID: sumHex,
				Reader: func() (io.ReadCloser, error) {
					return os.Open(path)
				},
			},
		},
		Raw: map[string]any{
			"message_id":        msg.ID,
			"chat_id":           msg.Chat.ID,
			"message_thread_id": msg.ThreadID,
			"text":              caption,
			"attachment_sha256": sumHex,
			"attachment_path":   path,
			"attachment_mime":   mime,
		},
	}
	if msg.ThreadID != 0 {
		ev.Thread = &commons.ConversationRef{
			Channel:  commons.ChannelTelegram,
			ThreadID: strconv.Itoa(msg.ThreadID),
		}
	}
	return ev
}
