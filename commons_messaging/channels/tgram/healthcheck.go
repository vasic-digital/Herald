package tgram

import (
	"context"
	"fmt"
)

// HealthCheck verifies the bot token by issuing a getMe call against the
// live Bot API. Returns nil only if the API responds with a populated
// User struct (non-empty Username) — proves the token is valid AND the
// bot is enabled.
//
// Implementation note: telebot.NewBot() itself dispatches getMe during
// construction (see telebot/bot.go:58: `user, err := bot.getMe()`) and
// stores the parsed *User on bot.Me. So the constructor IS the live
// roundtrip; we then assert non-empty Username post-construction.
//
// Per §107: a PASS without observing a real getMe response would be a
// PASS-bluff. The non-empty-Username assertion makes that bluff
// impossible — Offline:true would yield bot.Me = &User{} with empty
// Username, which fails the assertion.
//
// Bot construction is delegated to ensureBot() which uses sync.Once so
// concurrent HealthCheck/Send/Subscribe calls share a single Bot API
// roundtrip instead of racing (Task 2 review carry-forward).
//
// The ctx parameter is reserved for future use (telebot.v3.3.8 does not
// thread ctx through getMe; later versions or a Raw("getMe", nil) +
// http.Client with ctx may be wired here when telebot exposes it).
func (a *Adapter) HealthCheck(ctx context.Context) error {
	_ = ctx // reserved — see doc comment
	if err := a.ensureBot(); err != nil {
		return fmt.Errorf("tgram.HealthCheck: %w", err)
	}
	if a.bot.Me == nil {
		return fmt.Errorf("tgram.HealthCheck: getMe returned nil User (§107 bluff guard)")
	}
	if a.bot.Me.Username == "" {
		return fmt.Errorf("tgram.HealthCheck: getMe returned User with empty Username (§107 bluff guard — Offline mode or unenabled bot)")
	}
	return nil
}
