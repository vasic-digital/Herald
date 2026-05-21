//go:build integration

package tgram

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestHealthCheck_LiveBotAPI exercises the tgram HealthCheck against the
// real Telegram Bot API. It SKIPs cleanly (per Universal §11.4.3) if
// either HERALD_TGRAM_BOT_TOKEN or HERALD_TGRAM_CHAT_ID is absent —
// NEVER PASS-by-default without credentials.
//
// On a host with credentials, it asserts the Bot API responds with a
// populated User (non-empty Username), which proves the token is valid
// AND the bot is enabled.
func TestHealthCheck_LiveBotAPI(t *testing.T) {
	token := os.Getenv("HERALD_TGRAM_BOT_TOKEN")
	if token == "" {
		t.Skipf("skip: hardware_not_present — HERALD_TGRAM_BOT_TOKEN absent per §11.4.3 explicit SKIP-with-reason")
	}
	chatID := os.Getenv("HERALD_TGRAM_CHAT_ID")
	if chatID == "" {
		t.Skipf("skip: hardware_not_present — HERALD_TGRAM_CHAT_ID absent per §11.4.3")
	}

	a, err := New("tgram://" + token + "/" + chatID)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := a.HealthCheck(ctx); err != nil {
		t.Fatalf("HealthCheck against live Bot API: %v", err)
	}
}
