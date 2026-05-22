package runner

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/vasic-digital/herald/commons"
)

func TestOutcomeRecorder_TwoRecipients_TwoEvidenceRows(t *testing.T) {
	tenantID := mustParse("44444444-4444-4444-4444-444444444444")
	evid := newFakeEvidenceStore()
	pg := newFakeEventsProcessedStore()
	o := &OutcomeRecorder{Evidence: evid, EventsProcessed: pg}

	rc := &RunCtx{
		TenantID: tenantID,
		IdemKey:  "k1",
		Event:    cloudEventStub("evt-1", "x"),
		Receipts: []ChannelDispatchResult{
			{ChannelID: "null", ChannelUserID: "a", Evidence: commons.DeliveryRouted, ChannelMsgID: "n-a"},
			{ChannelID: "null", ChannelUserID: "b", Evidence: commons.DeliveryRouted, ChannelMsgID: "n-b"},
		},
	}
	rc.TenantPGCtx = withTenantCtx(context.Background(), tenantID)
	rcpt, err := o.Process(context.Background(), rc)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(evid.All()) != 2 {
		t.Errorf("evidence rows = %d, want 2", len(evid.All()))
	}
	if _, found := pg.Lookup(context.Background(), tenantID, "k1"); !found {
		t.Errorf("events_processed row missing after Process")
	}
	if len(rcpt.OutboundEvidenceIDs) != 2 {
		t.Errorf("Receipt.OutboundEvidenceIDs len = %d, want 2", len(rcpt.OutboundEvidenceIDs))
	}
}

func TestOutcomeRecorder_RecordDenied_NoRecipientFanOut(t *testing.T) {
	tenantID := mustParse("44444444-4444-4444-4444-444444444444")
	evid := newFakeEvidenceStore()
	pg := newFakeEventsProcessedStore()
	o := &OutcomeRecorder{Evidence: evid, EventsProcessed: pg}

	rc := &RunCtx{
		TenantID:       tenantID,
		IdemKey:        "k2",
		Event:          cloudEventStub("evt-2", "x"),
		PolicyDecision: 3, // DecisionFail per commons_constitution
		PolicyReason:   "11.4.10: credential leak",
	}
	rc.TenantPGCtx = withTenantCtx(context.Background(), tenantID)
	rcpt, err := o.RecordDenied(context.Background(), rc)
	if err != nil {
		t.Fatalf("RecordDenied: %v", err)
	}
	if len(evid.All()) != 1 {
		t.Errorf("RecordDenied should write exactly 1 evidence row (the denial); got %d", len(evid.All()))
	}
	if rcpt.Recipients != 0 {
		t.Errorf("RecordDenied receipt should have 0 recipients; got %d", rcpt.Recipients)
	}
	if _, found := pg.Lookup(context.Background(), tenantID, "k2"); !found {
		t.Errorf("events_processed row missing after RecordDenied — replay protection broken")
	}
}

// Ensure mustParse and other helpers are in scope (they live in fakes_test.go).
var _ = uuid.Nil
var _ = time.Second
