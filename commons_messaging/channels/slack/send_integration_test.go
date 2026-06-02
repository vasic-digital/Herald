//go:build integration

package slack

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/vasic-digital/herald/commons"
)

// TestSend_LiveWebAPI exercises Adapter.Send against the live Slack Web API
// (chat.postMessage). Without HERALD_SLACK_BOT_TOKEN + HERALD_SLACK_CHANNEL_ID
// this SKIPs cleanly per §11.4.3 (hardware_not_present). With credentials it
// sends a real message and asserts the response carries a Slack-side `ts` in
// Receipt.ChannelMsgID — the §107 bluff guard (proves Slack actually stored the
// message and returned a chat-side timestamp, not a synthetic Herald id).
//
// Self-cleaning: Slack has no bot-side message delete that is guaranteed across
// workspace plans, so the test posts a clearly test-tagged, timestamped line to
// the operator-designated QA channel rather than mutating shared state.
func TestSend_LiveWebAPI(t *testing.T) {
	botToken := os.Getenv("HERALD_SLACK_BOT_TOKEN")
	channelID := os.Getenv("HERALD_SLACK_CHANNEL_ID")
	if botToken == "" || channelID == "" {
		t.Skipf("skip: hardware_not_present — HERALD_SLACK_BOT_TOKEN or HERALD_SLACK_CHANNEL_ID absent per §11.4.3")
	}

	a := New(botToken, "", channelID)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	msg := commons.OutboundMessage{
		To: []commons.Recipient{{Channel: "slack", ChannelUserID: channelID}},
		Body: commons.Body{
			Plain: "Herald HRD-116 Slack integration test " + time.Now().Format(time.RFC3339Nano),
		},
	}
	receipt, err := a.Send(ctx, msg)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if receipt.ChannelMsgID == "" {
		t.Fatal("Send returned empty ChannelMsgID — §107 bluff guard (proves Slack actually received the message and returned a chat-side ts)")
	}
	if receipt.Evidence != commons.DeliveryRouted {
		t.Errorf("Send returned Evidence=%v, want DeliveryRouted", receipt.Evidence)
	}
	t.Logf("Send OK: ChannelMsgID=%s Evidence=%s LatencyMillis=%d", receipt.ChannelMsgID, receipt.Evidence, receipt.LatencyMillis)
}
