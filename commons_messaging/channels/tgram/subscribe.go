package tgram

import (
	"context"
	"fmt"
	"strconv"
	"time"

	telebot "gopkg.in/telebot.v3"

	"github.com/vasic-digital/herald/commons"
)

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
			Thread: &commons.ConversationRef{
				Channel:  commons.ChannelTelegram,
				ThreadID: strconv.Itoa(msg.ThreadID), // 0 if not a forum-topic message
			},
			Raw: map[string]any{
				"message_id":        msg.ID,
				"chat_id":           msg.Chat.ID,
				"message_thread_id": msg.ThreadID,
				"text":              msg.Text,
			},
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
