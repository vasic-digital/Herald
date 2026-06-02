package slack

import (
	"context"
	"log"
	"strconv"
	"time"

	slackgo "github.com/slack-go/slack"

	"github.com/vasic-digital/herald/commons"
)

// threadContextLimit bounds the conversations.replies fetch so a long thread
// cannot inflate the inbound envelope unboundedly. Slack returns thread order
// (oldest→newest); we keep the most recent threadContextLimit prior messages.
const threadContextLimit = 20

// fetchThreadContext returns the PRIOR messages of the Slack thread rooted at
// threadTS (oldest→newest), EXCLUDING the current inbound message (the entry
// whose ts == excludeTS). It is deliberately NON-FATAL: a replies-fetch error
// (or a degenerate/empty response) logs and yields an empty slice so the
// inbound message is still dispatched — a context-fetch failure must never
// drop the subscriber's message, it merely deprives Claude of prior context.
//
// Per the operator mandate (2026-06-02) this is what binds a reply to the
// thread's MEANING: the dispatcher feeds ThreadContext to Claude so a reply is
// a contribution to the thread, not an isolated answer.
func (a *Adapter) fetchThreadContext(ctx context.Context, channelID, threadTS, excludeTS string) []commons.ThreadMessage {
	msgs, _, _, err := a.api.GetConversationRepliesContext(ctx, &slackgo.GetConversationRepliesParameters{
		ChannelID: channelID,
		Timestamp: threadTS, // the thread root ts
		Limit:     threadContextLimit,
	})
	if err != nil {
		log.Printf("slack.fetchThreadContext: conversations.replies channel=%s thread_ts=%s: %v (dispatching without prior context)", channelID, threadTS, err)
		return nil
	}
	out := make([]commons.ThreadMessage, 0, len(msgs))
	for _, m := range msgs {
		// Exclude the current inbound message — it is already in ev.Body.
		if m.Timestamp == excludeTS {
			continue
		}
		out = append(out, commons.ThreadMessage{
			SenderHandle: threadSenderHandle(m.User, m.Username, m.BotID),
			SenderIsBot:  m.BotID != "",
			Text:         m.Text,
			Timestamp:    parseSlackTS(m.Timestamp),
		})
	}
	return out
}

// threadSenderHandle resolves the best available identifier for a thread
// message's author: the user id when present, else the bot's username, else
// the bot id. Empty only when Slack supplied none of the three.
func threadSenderHandle(user, username, botID string) string {
	if user != "" {
		return user
	}
	if username != "" {
		return username
	}
	return botID
}

// parseSlackTS converts a Slack "ts" (epoch seconds with a fractional
// microsecond suffix, e.g. "1654.000100") to a time.Time. It is best-effort:
// any parse failure yields the zero time, matching the contract's "zero if the
// adapter cannot determine it".
func parseSlackTS(ts string) time.Time {
	if ts == "" {
		return time.Time{}
	}
	secs, err := strconv.ParseFloat(ts, 64)
	if err != nil {
		return time.Time{}
	}
	whole := int64(secs)
	frac := secs - float64(whole)
	return time.Unix(whole, int64(frac*1e9)).UTC()
}
