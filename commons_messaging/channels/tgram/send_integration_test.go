//go:build integration

package tgram

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/vasic-digital/herald/commons"
)

// TestSend_LiveBotAPI exercises Adapter.Send against the live Bot API.
// Without HERALD_TGRAM_BOT_TOKEN + HERALD_TGRAM_CHAT_ID this SKIPs
// cleanly per §11.4.3 (hardware_not_present). With credentials it sends
// a real message and asserts the response includes a Telegram-side
// integer message_id — the §107 bluff guard.
func TestSend_LiveBotAPI(t *testing.T) {
	token := os.Getenv("HERALD_TGRAM_BOT_TOKEN")
	chatID := os.Getenv("HERALD_TGRAM_CHAT_ID")
	if token == "" || chatID == "" {
		t.Skipf("skip: hardware_not_present — HERALD_TGRAM_BOT_TOKEN or HERALD_TGRAM_CHAT_ID absent per §11.4.3")
	}

	a, err := New("tgram://" + token + "/" + chatID)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	msg := commons.OutboundMessage{
		Body: commons.Body{
			Plain: "Herald HRD-011 step 3 integration test " + time.Now().Format(time.RFC3339Nano),
		},
	}
	receipt, err := a.Send(ctx, msg)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if receipt.ChannelMsgID == "" {
		t.Fatal("Send returned empty ChannelMsgID — §107 bluff guard (proves Telegram actually received the message and returned a chat-side message_id)")
	}
	if receipt.Evidence != commons.DeliveryRouted {
		t.Errorf("Send returned Evidence=%v, want DeliveryRouted", receipt.Evidence)
	}
	t.Logf("Send OK: ChannelMsgID=%s Evidence=%s LatencyMillis=%d", receipt.ChannelMsgID, receipt.Evidence, receipt.LatencyMillis)
}
