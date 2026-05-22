// Package runner implements pherald's §32 7-stage event ingest pipeline.
//
// Each stage is its own concrete struct (Approach C per Wave 3 design):
// no shared `RunnerStage` interface — stages communicate exclusively
// via `RunCtx`. The orchestrator (runner.go) holds stage instances as
// fields and calls them in fixed order in `Run`.
//
// Per §107 anti-bluff: every stage that claims success MUST observe
// positive runtime evidence — getMe-style validations, real DB writes,
// real channel API responses. Stages that no-op on success are §11.4
// PASS-bluffs.
package runner

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/vasic-digital/herald/commons"
	constitution "github.com/vasic-digital/herald/commons_constitution"
)

// RunCtx is the per-event work order. Each stage reads + writes fields
// it owns; later stages may not mutate fields owned by earlier stages.
// Lifecycle: one RunCtx per inbound event; lives in memory until Run
// returns. NOT persisted (events_processed gets the archive row, not
// this struct).
//
// Field-ownership convention (enforced by code review, not Go types):
//
//   Set by HTTP handler before Runner.Run:
//     AuthClaims, TenantID, Raw
//
//   Set by stage 1 EventParser:
//     Event, Trace, IdemKey
//
//   Set by stage 2 IdempotencyChecker:
//     Duplicate, CachedRcpt
//
//   Set by stage 3 TenantResolver:
//     TenantPGCtx
//
//   Set by stage 4 PolicyGate:
//     PolicyDecision, PolicyReason
//
//   Set by stage 5 SubscriberResolver:
//     Recipients
//
//   Set by stage 6 ChannelDispatcher:
//     Receipts
//
//   Set by stage 7 OutcomeRecorder:
//     OutboundEvidenceIDs
type RunCtx struct {
	// Pre-Runner (set by HTTP handler):
	AuthClaims map[string]any
	TenantID   uuid.UUID
	Raw        []byte

	// Stage 1 outputs:
	Event   commons.CloudEventEnvelope
	Trace   commons.TraceContext
	IdemKey string

	// Stage 2 outputs:
	Duplicate  bool
	CachedRcpt *Receipt

	// Stage 3 outputs:
	TenantPGCtx context.Context

	// Stage 4 outputs:
	PolicyDecision constitution.Decision
	PolicyReason   string

	// Stage 5 outputs:
	Recipients []commons.Recipient

	// Stage 6 outputs:
	Receipts []ChannelDispatchResult

	// Stage 7 outputs:
	OutboundEvidenceIDs []uuid.UUID
}

// Receipt is the per-event outcome the HTTP handler returns to the
// client. JSON-encoded as 202 Accepted body (or 200 on replay).
type Receipt struct {
	EventID             string                   `json:"event_id"`
	IdempotencyKey      string                   `json:"idempotency_key"`
	AcceptedAt          time.Time                `json:"accepted_at"`
	Recipients          int                      `json:"recipients"`
	Results             []ChannelDispatchResult  `json:"results"`
	WasReplay           bool                     `json:"was_replay"`
	OutboundEvidenceIDs []uuid.UUID              `json:"outbound_evidence_ids"`
}

// ChannelDispatchResult is the per-recipient outcome captured by
// ChannelDispatcher and surfaced in Receipt.Results.
type ChannelDispatchResult struct {
	ChannelID     string                  `json:"channel_id"`
	ChannelUserID string                  `json:"channel_user_id"`
	Evidence      commons.DeliveryEvidence `json:"evidence"`
	ChannelMsgID  string                  `json:"channel_msg_id,omitempty"`
	Error         string                  `json:"error,omitempty"` // populated only when Evidence == DeliveryUnknown
}
