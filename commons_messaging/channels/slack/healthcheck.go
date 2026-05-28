package slack

import (
	"context"
	"fmt"
)

// HealthCheck verifies the bot token by issuing auth.test against the
// live Slack Web API. Returns nil only if the API responds with ok=true
// AND a non-empty user_id (the bot's own user id) — proves the token is
// valid AND not deactivated.
//
// §107: a PASS without observing a real auth.test response would be a
// PASS-bluff. The non-empty user_id assertion makes that bluff impossible
// — an unauthenticated/revoked token errors at the slack-go layer, and a
// degenerate ok=true response with empty user_id is rejected here as an
// echo-loop hazard (BotSelfIdentity uses the same field as the self-filter
// anchor).
func (a *Adapter) HealthCheck(ctx context.Context) error {
	resp, err := a.api.AuthTestContext(ctx)
	if err != nil {
		return fmt.Errorf("slack.HealthCheck: auth.test: %w", err)
	}
	if resp == nil {
		return fmt.Errorf("slack.HealthCheck: auth.test returned nil response (§107 bluff guard)")
	}
	if resp.UserID == "" {
		return fmt.Errorf("slack.HealthCheck: auth.test returned empty user_id (§107 bluff guard — bot deactivated or token degenerate)")
	}
	return nil
}
