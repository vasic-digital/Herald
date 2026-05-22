package tgram

import (
	"os"
	"regexp"
	"testing"
)

// TestNewClient_LiveGetMe is the ONLY live test in this package. It
// SKIPs (per Universal §11.4.3) when HERALD_TGRAM_BOT_TOKEN is unset,
// and otherwise dispatches a real getMe via telebot.NewBot — the
// canonical network-proof for Wave 5 T3. The bot username MUST match
// Telegram's bot naming rule (`[A-Za-z0-9_]+bot$`); failure here means
// either the token is invalid or the §107 anti-bluff guard caught a
// stub that fabricated bot.Me without crossing the network.
//
// Paid surfaces (Send / Upload) are NEVER unit-tested — those land in
// Wave 5 T7's integration test and T8's live run. Constraining the
// unit test to getMe keeps CI cost at zero.
func TestNewClient_LiveGetMe(t *testing.T) {
	token := os.Getenv("HERALD_TGRAM_BOT_TOKEN")
	if token == "" {
		t.Skip("HERALD_TGRAM_BOT_TOKEN unset — live getMe SKIP per §11.4.3")
	}
	c, err := NewClient(token, 0) // chatID 0: getMe smoke only, no Send
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close()
	username := c.Bot().Me.Username
	if !regexp.MustCompile(`^[A-Za-z0-9_]+bot$`).MatchString(username) {
		t.Fatalf("bot username %q does not match Telegram's bot naming rule", username)
	}
}
