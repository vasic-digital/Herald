//go:build integration

package tgram

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vasic-digital/herald/commons"
)

// handlerFunc adapts a plain function to commons.InboundHandler.
// Test-scoped only — commons.InboundHandler defines a single Handle method.
type handlerFunc func(context.Context, commons.InboundEvent) error

func (h handlerFunc) Handle(ctx context.Context, ev commons.InboundEvent) error {
	return h(ctx, ev)
}

// TestSubscribe_LiveBotAPI exercises the live long-poll loop end-to-end.
// §107 bluff guard: requires the operator to hand-send a message to the
// configured chat within the 60s window; a Subscribe that returns nil
// without ever invoking the handler would be a bluff (getUpdates returned
// but never produced an update, OR the handler dispatch was wired to
// nothing). Asserting ≥1 invocation proves the loop actually pulled a real
// update produced by a human.
func TestSubscribe_LiveBotAPI(t *testing.T) {
	token := os.Getenv("HERALD_TGRAM_BOT_TOKEN")
	chatID := os.Getenv("HERALD_TGRAM_CHAT_ID")
	if token == "" || chatID == "" {
		t.Skipf("skip: hardware_not_present — HERALD_TGRAM_BOT_TOKEN or HERALD_TGRAM_CHAT_ID absent per §11.4.3")
	}
	if os.Getenv("HERALD_TGRAM_LIVE_INBOUND") != "1" {
		t.Skipf("skip: hardware_not_present — HERALD_TGRAM_LIVE_INBOUND=1 not set; would not exercise inbound path")
	}

	a, err := New("tgram://" + token + "/" + chatID)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var got atomic.Int64
	h := handlerFunc(func(ctx context.Context, ev commons.InboundEvent) error {
		got.Add(1)
		return nil
	})
	err = a.Subscribe(ctx, h)
	if err != nil && err != context.DeadlineExceeded {
		t.Fatalf("Subscribe: %v", err)
	}
	if got.Load() == 0 {
		t.Fatal("Subscribe received 0 messages from operator's hand-sent input — §107 bluff guard (proves getUpdates actually pulled real updates)")
	}
}
