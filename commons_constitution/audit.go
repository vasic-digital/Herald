package constitution

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AuditRow is the append-only constitution_audit representation. Mirrors
// migration-000006 constitution_audit columns 1:1.
//
// One AuditRow is written by Runner.Run for every CHANGED transition whose
// mode is ModeWarn or ModeEnforce (ModeAllow is recorded in state only — no
// audit row, no emit). This is the load-bearing durable trail behind the
// RunOutcome.Audited flag: prior to HRD-018 persistence, Audited was set
// true but NOTHING was written — a §107 PASS-bluff at the audit layer.
//
// EmittedEventID is the ID of the channel event emitted for ModeEnforce
// transitions; it is uuid.Nil (NULL in the DB) for ModeWarn rows, which are
// audit-only (informed via the pull surface, never pushed).
type AuditRow struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	RuleID         string
	Subject        string
	OldDecision    *Decision // nil iff FirstSeen (no prior verdict)
	NewDecision    Decision
	OldDigest      *[32]byte // nil iff FirstSeen
	NewDigest      [32]byte
	BundleHash     BundleHash
	EvidenceURI    string
	EmittedEventID uuid.UUID // uuid.Nil for ModeWarn (audit-only)
	ModeAtEmission Mode
	AuditedAt      time.Time
}

// AuditQuery filters an AuditStore.ListAudit call. Mirrors the
// /v1/compliance/audit pull-surface query params. Zero values disable each
// filter; results are sorted by AuditedAt DESC (newest-first — audit windows
// are walked most-recent-first by operators) then Offset+Limit applied.
type AuditQuery struct {
	RuleID  string
	Subject string
	Since   time.Time // zero = no lower bound (inclusive)
	Until   time.Time // zero = no upper bound (inclusive)
	Limit   int       // 0 = no limit
	Offset  int       // 0 = no offset
}

// AuditStore is the append-only audit-trail persistence interface.
//
// Backends:
//   - state/memory.go MemoryAudit (M1)   — slice, test-only.
//   - state/postgres.go PostgresAudit (M2) — RLS-guarded INSERT into
//     constitution_audit (append-only; UPDATE/DELETE forbidden by RLS policy).
//
// RecordAudit is the only mutator and is append-only — there is no Update or
// Delete. ListAudit serves the /v1/compliance/audit pull surface.
type AuditStore interface {
	// RecordAudit appends one row. If row.ID is uuid.Nil the backend assigns
	// a fresh UUID (uuidv7 server-side for Postgres). Returns the assigned ID.
	RecordAudit(ctx context.Context, row AuditRow) (uuid.UUID, error)

	// ListAudit returns audit rows for tenantID matching q, newest-first.
	ListAudit(ctx context.Context, tenantID uuid.UUID, q AuditQuery) ([]AuditRow, error)
}
