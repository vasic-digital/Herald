package slack

import (
	"context"
	"fmt"

	"github.com/vasic-digital/herald/commons_messaging/channels"
)

// BotSelfIdentity returns the channel-native bot identity via the Slack
// auth.test endpoint. The first call crosses the wire (the §32.9 anti-
// echo-loop anchor); subsequent calls return the cached user_id without an
// additional round-trip so Subscribe's hot path does not re-dial auth.test
// per inbound event.
//
// IdentityUserID is the canonical kind for Slack (the bot_user_id, e.g.
// "U01ABC…") — Slack messages carry the sender's user_id directly, so the
// generic self-filter (channels.IsSelfEcho) can compare without channel-
// specific knowledge. An empty user_id from auth.test is an echo-loop
// hazard — Subscribe refuses to boot when BotSelfIdentity errors out, and
// returning an empty Value here would silently disable the filter.
func (a *Adapter) BotSelfIdentity(ctx context.Context) (channels.SelfIdentity, error) {
	a.selfMu.Lock()
	cached := a.selfID
	a.selfMu.Unlock()
	if cached != "" {
		return channels.SelfIdentity{Kind: channels.IdentityUserID, Value: cached}, nil
	}
	resp, err := a.api.AuthTestContext(ctx)
	if err != nil {
		return channels.SelfIdentity{}, fmt.Errorf("slack.BotSelfIdentity: auth.test: %w", err)
	}
	if resp == nil || resp.UserID == "" {
		return channels.SelfIdentity{}, fmt.Errorf("slack.BotSelfIdentity: empty user_id (echo-loop hazard)")
	}
	a.selfMu.Lock()
	a.selfID = resp.UserID
	a.selfMu.Unlock()
	return channels.SelfIdentity{Kind: channels.IdentityUserID, Value: resp.UserID}, nil
}
