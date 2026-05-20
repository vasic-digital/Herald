package constitution

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Transition describes what changed when a Result is Record'd. The
// Changed flag is the core of the §42.2 "transitions-only emission"
// discipline:
//
//	Changed == true iff
//	    OldDecision != NewDecision           OR
//	    OldDigest   != NewDigest             OR
//	    OldBundleHash != NewBundleHash
//
// All three axes matter — the (rule, decision) verdict can stay the same
// while the underlying rationale (DigestSHA) or bundle revision changes,
// and that's still a transition worth emitting.
type Transition struct {
	OldDecision   Decision
	NewDecision   Decision
	OldDigest     [32]byte
	NewDigest     [32]byte
	OldBundleHash BundleHash
	NewBundleHash BundleHash
	Changed       bool
	FirstSeen     bool // true iff this (tenant, rule, subject) row didn't exist before
	At            time.Time
}

// StateRow is the persisted constitution_state representation.
// Mirrors the migration-000006 schema columns 1:1.
type StateRow struct {
	TenantID       uuid.UUID
	RuleID         string
	Subject        string
	Decision       Decision
	Digest         [32]byte
	BundleHash     BundleHash
	EvidenceURI    string
	TransitionedAt time.Time
}

// ConstitutionStore is the transition-gate persistence interface.
//
// Backends:
//   - state/memory.go (M1)   — sync.Map keyed by (tenant, rule, subject).
//   - state/postgres.go (M2) — RLS-guarded UPSERT into constitution_state.
//
// Record is the only mutator. Reads happen via List + Get for the
// /v1/compliance pull surface (M3).
type ConstitutionStore interface {
	// Record UPSERTs the row keyed by (tenantID, ruleID, subject), returning
	// the Transition observed. The Result.DigestSHA + bundle MUST be carried
	// through unchanged so future Record calls can compare digests.
	//
	// On first sight of (tenant, rule, subject) the Transition has
	// FirstSeen=true and OldDecision is the zero value (DecisionPass).
	// Callers MUST check FirstSeen — a brand-new failing row is a
	// transition worth emitting.
	Record(
		ctx context.Context,
		tenantID uuid.UUID,
		ruleID, subject string,
		r Result,
		bundle BundleHash,
		evidenceURI string,
	) (Transition, error)

	// Get returns the current StateRow + true, or zero+false if absent.
	Get(ctx context.Context, tenantID uuid.UUID, ruleID, subject string) (StateRow, bool, error)

	// List returns rows for tenantID matching the optional filters.
	// Pass empty strings to skip a filter.
	// `limit` of 0 means no limit.
	List(ctx context.Context, tenantID uuid.UUID, q ListQuery) ([]StateRow, error)
}

// ListQuery filters a ConstitutionStore.List call.
// Mirrors the M3 /v1/compliance query-param surface.
type ListQuery struct {
	RuleID   string
	Subject  string
	Decision *Decision // nil = no filter
	Limit    int       // 0 = no limit
}
