package tgram

import (
	"context"
	"fmt"
	"strconv"
	"time"

	telebot "gopkg.in/telebot.v3"

	"github.com/vasic-digital/herald/commons"
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
func shouldDropBotSelf(msg *telebot.Message, selfUsername string) bool {
	if msg == nil || msg.Sender == nil {
		return false
	}
	if !msg.Sender.IsBot {
		return false
	}
	return msg.Sender.Username == selfUsername
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

	bot.Handle(telebot.OnText, func(c telebot.Context) error {
		msg := c.Message()
		if msg == nil {
			return nil
		}
		if shouldDropBotSelf(msg, selfUsername) {
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
