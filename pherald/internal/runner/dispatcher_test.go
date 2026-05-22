package runner

import (
	"context"
	"testing"

	"github.com/vasic-digital/herald/commons"
)

func TestChannelDispatcher_SingleRecipient(t *testing.T) {
	nullCh := newFakeChannel("null")
	registry := map[commons.ChannelID]commons.Channel{
		commons.ChannelNull: nullCh,
	}
	d := &ChannelDispatcher{Channels: registry}

	rc := &RunCtx{
		Event:      cloudEventStub("evt-1", "x"),
		Recipients: []commons.Recipient{{Channel: "null", ChannelUserID: "sandbox-1"}},
	}
	if err := d.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(rc.Receipts) != 1 {
		t.Fatalf("Receipts = %d, want 1", len(rc.Receipts))
	}
	if rc.Receipts[0].Evidence != commons.DeliveryRouted {
		t.Errorf("Evidence = %s, want Routed", rc.Receipts[0].Evidence)
	}
	if rc.Receipts[0].ChannelMsgID == "" {
		t.Errorf("ChannelMsgID empty — channel Send didn't populate it")
	}
}

func TestChannelDispatcher_UnknownChannel_RecordsUnknown(t *testing.T) {
	d := &ChannelDispatcher{Channels: map[commons.ChannelID]commons.Channel{}}
	rc := &RunCtx{
		Event:      cloudEventStub("evt-1", "x"),
		Recipients: []commons.Recipient{{Channel: "no-such-channel", ChannelUserID: "x"}},
	}
	if err := d.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process should not error on unknown channel; got %v", err)
	}
	if len(rc.Receipts) != 1 {
		t.Fatalf("Receipts = %d, want 1", len(rc.Receipts))
	}
	if rc.Receipts[0].Evidence != commons.DeliveryUnknown {
		t.Errorf("Unknown channel must produce Evidence=Unknown; got %s", rc.Receipts[0].Evidence)
	}
	if rc.Receipts[0].Error == "" {
		t.Errorf("Unknown channel must populate Error field for observability")
	}
}

func TestChannelDispatcher_MultipleRecipients_ParallelSafe(t *testing.T) {
	nullCh := newFakeChannel("null")
	registry := map[commons.ChannelID]commons.Channel{commons.ChannelNull: nullCh}
	d := &ChannelDispatcher{Channels: registry}

	rc := &RunCtx{
		Event: cloudEventStub("evt-1", "x"),
		Recipients: []commons.Recipient{
			{Channel: "null", ChannelUserID: "a"},
			{Channel: "null", ChannelUserID: "b"},
			{Channel: "null", ChannelUserID: "c"},
		},
	}
	if err := d.Process(context.Background(), rc); err != nil {
		t.Fatal(err)
	}
	if len(rc.Receipts) != 3 {
		t.Errorf("Receipts = %d, want 3", len(rc.Receipts))
	}
}
