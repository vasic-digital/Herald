package slack

import (
	"context"
	"fmt"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons_messaging/channels"
)

// Subscribe runs the Socket Mode event loop until ctx is cancelled,
// dispatching each inbound message + file-share event to h.Handle.
//
// Spec §32.2 parity: Socket Mode is the continuous WebSocket transport
// Slack offers — the Slack equivalent of Telegram's getUpdates long-poll
// path. The app-level token (xapp-…) is REQUIRED in addition to the bot
// token; constructing this loop without one is a deterministic boot error.
//
// §107 anti-bluff. A Subscribe that returns nil without ever invoking h
// would be a bluff (the loop "ran" but never dispatched). The bot self-
// filter is wired via BotSelfIdentity — an EMPTY self-identity refuses to
// boot (echo-loop hazard), matching the tgram pattern. Cross-bot messages
// (a DIFFERENT bot in the same channel) are KEPT (multi-bot collab is real
// subscriber traffic).
//
// File handling: each MessageEvent.Files entry triggers a content-
// addressed download via DownloadAttachment, and the resulting path +
// sha256 is appended to ev.Attachments so the Claude Code pre-text
// formatter sees the file alongside the caption text.
//
// Implementation note (slack-go v0.16.0): socketmode.Client.RunContext
// blocks until ctx cancels; we run it in a goroutine so this method can
// select on client.Events and ctx.Done() in the same loop.
func (a *Adapter) Subscribe(ctx context.Context, h commons.InboundHandler) error {
	if a.appToken == "" {
		return fmt.Errorf("slack.Subscribe: app-level token (xapp-…) required for Socket Mode")
	}
	self, err := a.BotSelfIdentity(ctx)
	if err != nil {
		return fmt.Errorf("slack.Subscribe: resolve self-identity (echo-loop hazard): %w", err)
	}
	// Construct a Socket Mode client with the app-level token. The bot
	// token already configured on a.api stays in use for outbound Web API
	// calls (chat.postMessage, files.uploadV2) from within the handler.
	api := slack.New(a.botToken, slack.OptionAppLevelToken(a.appToken))
	client := socketmode.New(api)
	runErrCh := make(chan error, 1)
	go func() { runErrCh <- client.RunContext(ctx) }()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case rerr := <-runErrCh:
			// socketmode.RunContext returned — either ctx cancellation or
			// a connection-level error. Surface the err (nil on graceful
			// ctx.Done(), wrapped otherwise).
			if rerr != nil && !isCtxErr(rerr) {
				return fmt.Errorf("slack.Subscribe: socketmode.RunContext: %w", rerr)
			}
			return ctx.Err()
		case evt, ok := <-client.Events:
			if !ok {
				return fmt.Errorf("slack.Subscribe: events channel closed unexpectedly")
			}
			if evt.Type != socketmode.EventTypeEventsAPI {
				continue
			}
			eventsAPI, ok := evt.Data.(slackevents.EventsAPIEvent)
			if !ok {
				continue
			}
			if evt.Request != nil {
				client.Ack(*evt.Request)
			}
			inner, ok := eventsAPI.InnerEvent.Data.(*slackevents.MessageEvent)
			if !ok {
				continue
			}
			if err := a.dispatchMessageEvent(ctx, h, inner, self); err != nil {
				return err
			}
		}
	}
}

// dispatchMessageEvent is the message → InboundEvent dispatch path
// extracted so subscribe_test.go can exercise it without spinning up a
// real Socket Mode WebSocket. It builds the InboundEvent, applies the
// channel-agnostic self-filter (channels.IsSelfEcho), and invokes
// h.Handle. File shares trigger content-addressed downloads which append
// to ev.Attachments.
//
// Returning a non-nil error halts the Subscribe loop — only download
// failures + handler errors propagate. Self-echoes and malformed events
// return nil (drop silently is the right behaviour; a self-echo is not
// an error condition).
func (a *Adapter) dispatchMessageEvent(ctx context.Context, h commons.InboundHandler, inner *slackevents.MessageEvent, self channels.SelfIdentity) error {
	if inner == nil {
		return nil
	}
	ev := commons.InboundEvent{
		Sender: commons.Recipient{
			Channel:       string(commons.ChannelSlack),
			ChannelUserID: inner.Channel,
		},
		Body: commons.Body{Plain: inner.Text},
		Raw: map[string]any{
			"message_id": inner.TimeStamp,
			"channel":    inner.Channel,
			"thread_ts":  inner.ThreadTimeStamp,
			"user":       inner.User,
			"bot_id":     inner.BotID,
			"subtype":    inner.SubType,
			"text":       inner.Text,
		},
	}
	// Wave 7 T4: stamp the sender's native identity into ev.Raw and drop
	// THIS bot's own echo via channels.IsSelfEcho. Slack identifies the
	// bot by user_id (IdentityUserID) — MessageEvent.BotID is non-empty
	// for bot-authored posts; for plain user messages we still stamp via
	// inner.User so the filter has the right shape.
	isBot := inner.BotID != ""
	channels.StampSender(ev.Raw, isBot, channels.IdentityUserID, inner.User)
	if channels.IsSelfEcho(ev, self) {
		return nil
	}
	if inner.ThreadTimeStamp != "" {
		ev.Thread = &commons.ConversationRef{
			Channel:  commons.ChannelSlack,
			ThreadID: inner.ThreadTimeStamp,
		}
		// Thread-context awareness (operator mandate 2026-06-02): fetch the
		// thread's PRIOR messages (oldest→newest, current message excluded)
		// so the dispatcher can bind a reply to the thread's MEANING. This is
		// non-fatal — a fetch error leaves ThreadContext empty without
		// dropping the message (see fetchThreadContext).
		ev.ThreadContext = a.fetchThreadContext(ctx, inner.Channel, inner.ThreadTimeStamp, inner.TimeStamp)
	}
	for _, f := range inner.Files {
		path, sumHex, derr := a.DownloadAttachment(ctx, f.ID, f.Mimetype)
		if derr != nil {
			return fmt.Errorf("slack.Subscribe: download file %s: %w", f.ID, derr)
		}
		ev.Attachments = append(ev.Attachments, commons.Attachment{
			Filename:  path,
			MIMEType:  f.Mimetype,
			SizeBytes: int64(f.Size),
			CID:       sumHex,
		})
	}
	if err := h.Handle(ctx, ev); err != nil {
		return fmt.Errorf("slack.Subscribe: handle: %w", err)
	}
	return nil
}

// isCtxErr reports whether err is a context cancellation/timeout —
// socketmode.RunContext returns ctx.Err() on graceful shutdown.
func isCtxErr(err error) bool {
	if err == nil {
		return false
	}
	return err == context.Canceled || err == context.DeadlineExceeded
}
