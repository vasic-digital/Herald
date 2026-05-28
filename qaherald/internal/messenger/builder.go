// Channel-keyed MessengerClient builder (Wave 7 T7 — HRD-116).
//
// Build dispatches construction to the right concrete impl based on
// cfg.Channel:
//
//	"tgram" / ""  → NewTelegramClient (cfg.Token + cfg.ChatID)
//	"slack"       → NewSlackClient (cfg.Token + cfg.ChannelID)
//	other         → error (no silent nil — a qaherald run against an
//	                unsupported channel must fail loud)
//
// This is the seam the qaherald orchestrator will use (Wave 7 T7b or
// later — out of scope for this task) to construct the right client
// from operator configuration. The builder itself adds no policy
// beyond the channel switch; per-channel construction errors surface
// verbatim so config-time mistakes are visible.
//
// §107 anti-bluff: an unknown channel returning nil + nil would pass
// type-checks and SILENTLY no-op every messenger call (lifecycle
// preflight would "succeed" against a nil client because there is no
// wire to fail). The builder forbids this by construction —
// TestBuilderUnknownErrors pins it.
package messenger

import "fmt"

// BuildConfig carries channel-agnostic construction inputs for Build.
// Per-channel field usage:
//
//	tgram: Token (bot token), ChatID (int64 numeric chat id), BaseURL
//	slack: Token (xoxb-… bot token), ChannelID (Cxxx), BaseURL
//
// Fields not used by the selected channel are IGNORED — the builder
// does not validate cross-channel field consistency. Callers SHOULD
// populate only the relevant subset for their target channel.
type BuildConfig struct {
	Channel   string // "tgram" (default) | "slack"
	Token     string // bot token (Telegram bot token / Slack xoxb-…)
	ChatID    int64  // Telegram numeric chat id (REQUIRED for tgram, non-zero)
	ChannelID string // Slack channel id (Cxxx; REQUIRED for slack)
	BaseURL   string // httptest seam; "" => live endpoint
}

// Build constructs the MessengerClient for cfg.Channel. Unknown
// channels error (no silent nil) — a qaherald run against an
// unsupported channel must fail loud, not no-op.
//
// Telegram construction validates cfg.Token + cfg.ChatID; an error
// from NewTelegramClient surfaces verbatim. Slack construction does
// not return an error from the constructor itself (loud-fail happens
// at the first wire call via auth.test/chat.postMessage returning
// ok=false), so Build returns a SlackClient unconditionally for the
// "slack" case.
func Build(cfg BuildConfig) (MessengerClient, error) {
	switch cfg.Channel {
	case "tgram", "":
		c, err := NewTelegramClient(cfg.Token, cfg.ChatID, cfg.BaseURL)
		if err != nil {
			return nil, fmt.Errorf("messenger.Build(tgram): %w", err)
		}
		return c, nil
	case "slack":
		return NewSlackClient(cfg.Token, cfg.ChannelID, cfg.BaseURL), nil
	default:
		return nil, fmt.Errorf("messenger.Build: unknown channel %q (supported: tgram, slack)", cfg.Channel)
	}
}
