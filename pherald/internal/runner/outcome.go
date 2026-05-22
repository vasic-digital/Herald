package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// OutcomeRecorder is Stage 7 — the last stage before the orchestrator
// returns. Two entry points:
//
//   Process(ctx, rc):   normal happy/warn path; writes one
//                       outbound_delivery_evidence row per dispatched
//                       recipient + one events_processed archive row.
//
//   RecordDenied(ctx, rc): short-circuit path called by the orchestrator
//                       when Stage 4 returned DecisionFail. Writes a
//                       single "denied" evidence row + events_processed
//                       row (so replay protection still works) and skips
//                       stages 5/6.
//
// Per §107: an OutcomeRecorder that no-ops on the PG write would let
// duplicates re-dispatch on every retry. The events_processed row is
// the load-bearing replay-prevention artifact.
type OutcomeRecorder struct {
	Evidence        evidenceStore
	EventsProcessed eventsProcessedStore
}

// evidenceStore is the subset of the outbound_delivery_evidence-table
// access this stage performs. The real PG adapter (T9) implements it
// against pgxpool; tests inject fakeEvidenceStore.
type evidenceStore interface {
	Insert(ctx context.Context, r evidenceRow) (uuid.UUID, error)
}

// eventsProcessedStore is the subset of events_processed-table access
// this stage performs. The Lookup half is shared with the
// IdempotencyChecker; OutcomeRecorder owns the Insert half (archive on
// fresh-accept). The real PG adapter (T9) implements both.
type eventsProcessedStore interface {
	Lookup(ctx context.Context, tenantID uuid.UUID, idemKey string) (*eventsProcessedRow, bool)
	Insert(ctx context.Context, row eventsProcessedRow) error
}

// evidenceRow mirrors the PG `outbound_delivery_evidence` table row.
// Lives in production code (not fakes_test.go) so the real PG adapter
// (T9: pgEvidenceAdapter.Insert) can accept the same struct the runner
// constructs, mirroring the eventsProcessedRow / subscriberRow precedent.
type evidenceRow struct {
	ID               uuid.UUID
	TenantID         uuid.UUID
	ChannelID        string
	ChannelMessageID string
	Evidence         int
	SentAt           time.Time
}

// Process writes one evidence row per recipient + one archive row. Used
// by the orchestrator after Stages 5/6 ran (i.e. PolicyDecision allowed
// dispatch). Mutates rc.OutboundEvidenceIDs and returns the assembled
// Receipt the HTTP handler will serialize as 202 Accepted body.
func (o *OutcomeRecorder) Process(ctx context.Context, rc *RunCtx) (*Receipt, error) {
	var ids []uuid.UUID
	now := time.Now()
	for _, r := range rc.Receipts {
		id, err := o.Evidence.Insert(rc.TenantPGCtx, evidenceRow{
			TenantID:         rc.TenantID,
			ChannelID:        r.ChannelID,
			ChannelMessageID: r.ChannelMsgID,
			Evidence:         int(r.Evidence),
			SentAt:           now,
		})
		if err != nil {
			return nil, fmt.Errorf("outcome: insert evidence: %w", err)
		}
		ids = append(ids, id)
	}
	rcpt := &Receipt{
		EventID:             rc.Event.ID,
		IdempotencyKey:      rc.IdemKey,
		AcceptedAt:          now,
		Recipients:          len(rc.Receipts),
		Results:             rc.Receipts,
		OutboundEvidenceIDs: ids,
	}
	if err := o.EventsProcessed.Insert(rc.TenantPGCtx, eventsProcessedRow{
		TenantID:    rc.TenantID,
		IdemKey:     rc.IdemKey,
		EventID:     rc.Event.ID,
		FirstSeenAt: now,
		Receipt:     rcpt,
	}); err != nil {
		return nil, fmt.Errorf("outcome: archive events_processed: %w", err)
	}
	rc.OutboundEvidenceIDs = ids
	return rcpt, nil
}

// RecordDenied is the Stage-4-DecisionFail short-circuit path. Writes a
// single "policy_denied" evidence row carrying the policy reason in
// ChannelMessageID + one events_processed archive row (so the next
// retry of the same idempotency key gets the cached denied receipt back
// instead of re-running the pipeline).
func (o *OutcomeRecorder) RecordDenied(ctx context.Context, rc *RunCtx) (*Receipt, error) {
	now := time.Now()
	id, err := o.Evidence.Insert(rc.TenantPGCtx, evidenceRow{
		TenantID:         rc.TenantID,
		ChannelID:        "policy_denied",
		ChannelMessageID: rc.PolicyReason,
		Evidence:         0, // DeliveryUnknown
		SentAt:           now,
	})
	if err != nil {
		return nil, fmt.Errorf("outcome: insert denial evidence: %w", err)
	}
	rcpt := &Receipt{
		EventID:             rc.Event.ID,
		IdempotencyKey:      rc.IdemKey,
		AcceptedAt:          now,
		Recipients:          0,
		Results:             nil,
		OutboundEvidenceIDs: []uuid.UUID{id},
	}
	if err := o.EventsProcessed.Insert(rc.TenantPGCtx, eventsProcessedRow{
		TenantID:    rc.TenantID,
		IdemKey:     rc.IdemKey,
		EventID:     rc.Event.ID,
		FirstSeenAt: now,
		Receipt:     rcpt,
	}); err != nil {
		return nil, fmt.Errorf("outcome: archive denied events_processed: %w", err)
	}
	rc.OutboundEvidenceIDs = []uuid.UUID{id}
	return rcpt, nil
}
