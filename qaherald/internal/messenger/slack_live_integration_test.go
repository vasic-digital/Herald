//go:build integration

package messenger

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestSlack_Live_Send is the live-evidence anchor that scripts/e2e_bluff_hunt.sh
// E127 greps for by literal name. It constructs the REAL qaherald SlackClient
// (NewSlackClient, default https://slack.com/api base URL) and performs a real
// chat.postMessage against the operator-designated QA channel.
//
// Credential guard (§11.4.3): without HERALD_SLACK_BOT_TOKEN +
// HERALD_SLACK_CHANNEL_ID the test SKIPs with an explicit reason — never a
// silent or fake pass. With credentials it asserts a non-empty Slack `ts`
// MessageID, the §107 bluff guard proving Slack actually stored the message.
//
// This is the qaherald-side analog of the channel adapter's
// commons_messaging/channels/slack TestSend_LiveWebAPI — same env contract, same
// build tag, different layer (qaherald is the QA bot; that adapter is the
// pherald-consumed channel).
func TestSlack_Live_Send(t *testing.T) {
	botToken := os.Getenv("HERALD_SLACK_BOT_TOKEN")
	channelID := os.Getenv("HERALD_SLACK_CHANNEL_ID")
	if botToken == "" || channelID == "" {
		t.Skipf("skip: hardware_not_present — HERALD_SLACK_BOT_TOKEN or HERALD_SLACK_CHANNEL_ID absent per §11.4.3")
	}

	c := NewSlackClient(botToken, channelID, "")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	text := "qaherald Slack live round-trip " + time.Now().Format(time.RFC3339Nano)
	id, err := c.Send(ctx, text)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if id == "" {
		t.Fatal("Send returned empty MessageID (ts) — §107 bluff guard (proves Slack actually received the message and returned a chat-side ts)")
	}
	t.Logf("Send OK: MessageID(ts)=%s", id)
}
