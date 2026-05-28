//go:build integration_mtproto

package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vasic-digital/herald/qaherald/internal/mtproto"
)

// TestMTProto_Subscribe_AutonomousRoundTrip is the §11.4.98-compliant
// replacement for TestSubscribe_LiveBotAPI. Drives a MTProto user-account
// (NOT a bot) to send a message to HERALD_TGRAM_CHAT_ID; asserts the bot
// at HERALD_TGRAM_BOT_TOKEN sees that message via its own getUpdates.
//
// Honest-SKIP per §11.4.3 when credentials absent OR session file missing.
// NEVER falls back to a manual-dep path. NEVER reports PASS without the
// real bot-side observation.
//
// Build tag: integration_mtproto — exercised by the e2e gate, not default
// go test.
func TestMTProto_Subscribe_AutonomousRoundTrip(t *testing.T) {
	appIDStr := os.Getenv("HERALD_MTPROTO_APP_ID")
	appHash := os.Getenv("HERALD_MTPROTO_APP_HASH")
	phone := os.Getenv("HERALD_MTPROTO_PHONE")
	password := os.Getenv("HERALD_MTPROTO_PASSWORD")
	botToken := os.Getenv("HERALD_TGRAM_BOT_TOKEN")
	chatIDStr := os.Getenv("HERALD_TGRAM_CHAT_ID")

	if appIDStr == "" || appHash == "" || phone == "" || botToken == "" || chatIDStr == "" {
		t.Skipf("skip: MTProto+Tgram credentials missing per §11.4.3 (HERALD_MTPROTO_APP_ID/APP_HASH/PHONE + HERALD_TGRAM_BOT_TOKEN + HERALD_TGRAM_CHAT_ID required)")
	}

	appID, err := strconv.Atoi(appIDStr)
	if err != nil {
		t.Skipf("skip: HERALD_MTPROTO_APP_ID not an integer")
	}
	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		t.Skipf("skip: HERALD_TGRAM_CHAT_ID not an integer")
	}

	cfg := mtproto.Config{AppID: appID, AppHash: appHash, Phone: phone, Password: password}
	exists, err := cfg.SessionExists()
	if err != nil {
		t.Fatalf("SessionExists: %v", err)
	}
	if !exists {
		t.Skipf("skip: MTProto session file missing — run `qaherald mtproto login` first")
	}

	client, err := mtproto.New(cfg)
	if err != nil {
		t.Fatalf("mtproto.New: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	myID, myUsername, err := client.WhoAmI(ctx)
	if err != nil {
		t.Fatalf("WhoAmI: %v", err)
	}
	t.Logf("MTProto user active: @%s (user_id=%d)", myUsername, myID)

	baselineOffset, err := captureBaselineOffset(botToken)
	if err != nil {
		t.Fatalf("baseline getUpdates: %s", sanitizeURL(err.Error(), botToken))
	}
	t.Logf("baseline getUpdates max_update_id=%d", baselineOffset)

	testMsg := fmt.Sprintf("herald-mtproto-autonomous-%d", time.Now().UnixNano())
	sentID, err := client.SendMessage(ctx, chatID, testMsg)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	t.Logf("MTProto sent message_id=%d text=%q", sentID, testMsg)

	deadline := time.Now().Add(60 * time.Second)
	var observed bool
	var observedUpdateID int64
	for time.Now().Before(deadline) {
		updates, err := pollGetUpdates(botToken, baselineOffset+1)
		if err != nil {
			t.Logf("poll getUpdates: %s (continuing)", sanitizeURL(err.Error(), botToken))
			time.Sleep(2 * time.Second)
			continue
		}
		for _, u := range updates {
			if u.Message == nil {
				continue
			}
			if u.Message.Chat.ID != chatID {
				continue
			}
			if u.Message.From.IsBot {
				continue
			}
			if u.Message.Text == testMsg {
				observed = true
				observedUpdateID = u.UpdateID
				break
			}
		}
		if observed {
			break
		}
		time.Sleep(2 * time.Second)
	}

	if !observed {
		t.Fatalf("bot did NOT receive MTProto-sent message within 60s — §11.4.98 autonomous round-trip FAILED")
	}
	t.Logf("PASS: autonomous round-trip — bot saw update_id=%d carrying our MTProto-sent text", observedUpdateID)
}

type tgUpdate struct {
	UpdateID int64      `json:"update_id"`
	Message  *tgMessage `json:"message"`
}

type tgMessage struct {
	MessageID int64  `json:"message_id"`
	Text      string `json:"text"`
	Chat      tgChat `json:"chat"`
	From      tgFrom `json:"from"`
}

type tgChat struct {
	ID int64 `json:"id"`
}

type tgFrom struct {
	IsBot    bool   `json:"is_bot"`
	Username string `json:"username"`
}

type tgGetUpdatesResp struct {
	OK     bool       `json:"ok"`
	Result []tgUpdate `json:"result"`
}

func captureBaselineOffset(botToken string) (int64, error) {
	resp, err := http.Get(fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?limit=100&timeout=0", botToken))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var r tgGetUpdatesResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return 0, err
	}
	var max int64
	for _, u := range r.Result {
		if u.UpdateID > max {
			max = u.UpdateID
		}
	}
	return max, nil
}

func pollGetUpdates(botToken string, offset int64) ([]tgUpdate, error) {
	u := fmt.Sprintf(
		"https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=2&allowed_updates=%s",
		botToken, offset, url.QueryEscape(`["message"]`),
	)
	resp, err := http.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var r tgGetUpdatesResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return r.Result, nil
}

// sanitizeURL scrubs the bot token from any error string per HRD-133.
func sanitizeURL(s, botToken string) string {
	if botToken == "" {
		return s
	}
	return strings.ReplaceAll(s, botToken, "<redacted-bot-token>")
}
