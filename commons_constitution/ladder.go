package constitution

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// Mode is the per-tenant per-rule enforcement level. Per spec §42.1.4 and
// the Foundation design §3.1 step [7] transition gate.
type Mode int

const (
	// ModeAllow: state is recorded; no audit row; no channel emit.
	// Tenant has explicitly opted out of this rule (e.g., dev-only tenant).
	ModeAllow Mode = iota

	// ModeWarn: state is recorded; audit row written; no channel emit.
	// Tenant is informed via the pull surface, not pushed.
	ModeWarn

	// ModeEnforce: state is recorded; audit row written; channel emit fans
	// out to all subscribed channels. This is the default for new bindings.
	ModeEnforce
)

func (m Mode) String() string {
	switch m {
	case ModeAllow:
		return "allow"
	case ModeWarn:
		return "warn"
	case ModeEnforce:
		return "enforce"
	default:
		return fmt.Sprintf("mode(%d)", int(m))
	}
}

// ParseMode is the inverse of Mode.String. Returns an error for unknown
// values rather than silently defaulting — config typos must be loud.
func ParseMode(s string) (Mode, error) {
	switch s {
	case "allow":
		return ModeAllow, nil
	case "warn":
		return ModeWarn, nil
	case "enforce":
		return ModeEnforce, nil
	default:
		return ModeAllow, fmt.Errorf("constitution: ParseMode: unknown mode %q", s)
	}
}

// ModeLadder is the per-tenant per-rule enforcement-mode lookup interface.
// Backends:
//
//   - ladder/memory.go (M1)  — in-memory map, test-only.
//   - ladder/postgres.go (M2) — source-of-truth via constitution_bindings.
//   - ladder/redis_cache.go (M3) — 60s read-cache wrapping postgres.
//
// Get is on the hot path of every evaluator call — implementations MUST be
// fast (sub-millisecond) and lock-light for concurrent reads.
//
// Set is rare (admin action via the REST surface) and MUST be durable
// before returning.
type ModeLadder interface {
	// Get returns the configured Mode for (tenantID, ruleID).
	// If no binding exists, Get returns ModeEnforce (the safe default —
	// new rules enforce until an operator explicitly relaxes them).
	Get(ctx context.Context, tenantID uuid.UUID, ruleID string) (Mode, error)

	// Set writes the binding (tenant, rule) = mode, attributed to `by`
	// (operator identity for audit). Returns once the write is durable.
	Set(ctx context.Context, tenantID uuid.UUID, ruleID string, m Mode, by string) error

	// List returns a snapshot of every binding for tenantID, keyed by ruleID.
	// Used by the admin UI + the /v1/compliance/modes pull surface.
	List(ctx context.Context, tenantID uuid.UUID) (map[string]Mode, error)
}
