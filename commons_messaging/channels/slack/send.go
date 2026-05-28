package slack

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/slack-go/slack"

	"github.com/vasic-digital/herald/commons"
)

// Send dispatches an OutboundMessage via chat.postMessage. The returned
// Receipt carries the real Slack `ts` in ChannelMsgID — NOT a synthetic
// Herald UUID. This is the §107 bluff guard: a Send that compiled cleanly
// but never hit chat.postMessage would pass type-checks; the hermetic test
// counts wire-byte hits + asserts the ts comes back from the server.
//
// Channel resolution: msg.To[0].ChannelUserID overrides a.channelID when
// present (per-message routing), else the adapter's default channel is
// used. Body selection: Markdown first (Slack renders mrkdwn), else Plain.
// Empty body → §107 error (a no-op Send is the canonical bluff class).
//
// Evidence ceiling: DeliveryRouted matches Capabilities — a postMessage
// 200 means platform-stored + fanned-out, NOT recipient read/delivered
// (Slack's `delivered` semantics are not exposed to bots).
func (a *Adapter) Send(ctx context.Context, msg commons.OutboundMessage) (commons.Receipt, error) {
	channelID := a.channelID
	if len(msg.To) > 0 && msg.To[0].ChannelUserID != "" {
		channelID = msg.To[0].ChannelUserID
	}
	if channelID == "" {
		return commons.Receipt{}, fmt.Errorf("slack.Send: no channel id (msg.To empty + adapter default empty)")
	}
	text := msg.Body.Markdown
	if text == "" {
		text = msg.Body.Plain
	}
	if text == "" {
		return commons.Receipt{}, fmt.Errorf("slack.Send: OutboundMessage.Body has neither Markdown nor Plain content")
	}
	start := time.Now()
	_, ts, err := a.api.PostMessageContext(ctx, channelID, slack.MsgOptionText(text, false))
	if err != nil {
		return commons.Receipt{}, fmt.Errorf("slack.Send: chat.postMessage to %s: %w", channelID, err)
	}
	if ts == "" {
		return commons.Receipt{}, fmt.Errorf("slack.Send: empty ts in chat.postMessage response (§107 bluff guard — Slack did not return a chat-side ts)")
	}
	return commons.Receipt{
		Evidence:      commons.DeliveryRouted,
		ChannelMsgID:  ts,
		SentAt:        time.Now(),
		LatencyMillis: time.Since(start).Milliseconds(),
		Native: map[string]any{
			"ts":      ts,
			"channel": channelID,
		},
	}, nil
}

// SendReply posts a threaded reply via chat.postMessage with thread_ts set
// to the parent message's ts. The signature differs from
// channels.Channel.SendReplyGeneric — this is the convenience entry point
// the plan's send_test.go exercises directly. SendReplyGeneric (see
// SendReplyGeneric below) is the interface-method this method delegates to.
//
// recipient.ChannelUserID is the channel id (Cxxx). replyToID is the
// parent message ts ("" → posts a fresh message, no thread). attachments
// are fan-out via files.uploadV2 with the same thread_ts so the text +
// every attachment thread under the same operator message at the same
// depth — mirrors the tgram T6 attachment fan-out pattern.
//
// Returns the chat-side ts of the text reply.
func (a *Adapter) SendReply(ctx context.Context, recipient commons.Recipient, body, replyToID string, attachments []commons.Attachment) (string, error) {
	channelID := recipient.ChannelUserID
	if channelID == "" {
		channelID = a.channelID
	}
	if channelID == "" {
		return "", fmt.Errorf("slack.SendReply: no channel id (recipient empty + adapter default empty)")
	}
	if body == "" {
		return "", fmt.Errorf("slack.SendReply: empty body")
	}
	opts := []slack.MsgOption{slack.MsgOptionText(body, false)}
	if replyToID != "" {
		// MsgOptionTS is the slack-go canonical name for "set thread_ts on
		// this post" (verified: submodules/slack-go/chat.go:641). It is
		// the §107 reply-threading anchor — a SendReply that compiled
		// cleanly but never appended this option would silently degrade
		// every reply to a fresh top-level message.
		opts = append(opts, slack.MsgOptionTS(replyToID))
	}
	_, ts, err := a.api.PostMessageContext(ctx, channelID, opts...)
	if err != nil {
		return "", fmt.Errorf("slack.SendReply: chat.postMessage to %s: %w", channelID, err)
	}
	if ts == "" {
		return "", fmt.Errorf("slack.SendReply: empty ts in chat.postMessage response (§107 bluff guard)")
	}
	for i, att := range attachments {
		if att.Filename == "" {
			return "", fmt.Errorf("slack.SendReply: attachment[%d] empty Filename (Wave 6 T5 convention: on-disk content-addressed path)", i)
		}
		params := slack.UploadFileV2Parameters{
			Channel:         channelID,
			File:            att.Filename,
			Filename:        filepath.Base(att.Filename),
			ThreadTimestamp: replyToID,
		}
		if _, uerr := a.api.UploadFileV2Context(ctx, params); uerr != nil {
			return "", fmt.Errorf("slack.SendReply: files.uploadV2 attachment[%d] (mime=%q): %w", i, att.MIMEType, uerr)
		}
	}
	return ts, nil
}

// SendReplyGeneric satisfies channels.Channel — delegates to SendReply.
// The two methods have the same signature; SendReplyGeneric is the
// canonical interface method, SendReply is the convenience name (mirroring
// tgram's native SendReply / SendReplyGeneric split, see channel.go
// package doc for the rationale).
func (a *Adapter) SendReplyGeneric(ctx context.Context, recipient commons.Recipient, body, replyToID string, attachments []commons.Attachment) (string, error) {
	return a.SendReply(ctx, recipient, body, replyToID, attachments)
}
