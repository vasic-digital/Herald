// Package state backs the constitution.ConstitutionStore interface with an
// in-memory map. Test-only — production deployments use the Postgres
// backend at M2 (state/postgres.go).
package state

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vasic-digital/herald/commons_constitution"
)

// stateKey is the composite PK that mirrors (tenant_id, rule_id, subject).
type stateKey struct {
	tenant  uuid.UUID
	rule    string
	subject string
}

// Memory is the in-memory ConstitutionStore used at M1.
type Memory struct {
	mu  sync.RWMutex
	now func() time.Time // injectable clock for deterministic tests
	all map[stateKey]constitution.StateRow
}

// NewMemory returns an empty in-memory store using time.Now as the clock.
func NewMemory() *Memory {
	return &Memory{now: time.Now, all: make(map[stateKey]constitution.StateRow)}
}

// WithClock returns a Memory whose TransitionedAt timestamps use `now`.
// Foundation tests use this for deterministic time injection.
func (m *Memory) WithClock(now func() time.Time) *Memory {
	m.now = now
	return m
}

// Record implements ConstitutionStore. Pure in-memory UPSERT + transition
// computation per the §42.2 transitions-only discipline.
func (m *Memory) Record(
	_ context.Context,
	tenantID uuid.UUID,
	ruleID, subject string,
	r constitution.Result,
	bundle constitution.BundleHash,
	evidenceURI string,
) (constitution.Transition, error) {
	key := stateKey{tenant: tenantID, rule: ruleID, subject: subject}
	now := m.now()

	m.mu.Lock()
	defer m.mu.Unlock()

	prev, prevExists := m.all[key]

	newRow := constitution.StateRow{
		TenantID:       tenantID,
		RuleID:         ruleID,
		Subject:        subject,
		Decision:       r.Decision,
		Digest:         r.DigestSHA,
		BundleHash:     bundle,
		EvidenceURI:    evidenceURI,
		TransitionedAt: now,
	}

	trans := constitution.Transition{
		NewDecision:   r.Decision,
		NewDigest:     r.DigestSHA,
		NewBundleHash: bundle,
		At:            now,
		FirstSeen:     !prevExists,
	}

	if prevExists {
		trans.OldDecision = prev.Decision
		trans.OldDigest = prev.Digest
		trans.OldBundleHash = prev.BundleHash
		trans.Changed = prev.Decision != r.Decision ||
			prev.Digest != r.DigestSHA ||
			prev.BundleHash != bundle
		if !trans.Changed {
			// nothing changed — preserve the original TransitionedAt to
			// avoid lying about "when this verdict first occurred."
			newRow.TransitionedAt = prev.TransitionedAt
			trans.At = prev.TransitionedAt
		}
	} else {
		// First sighting is by definition a transition (from "unknown" to "this").
		trans.Changed = true
	}

	m.all[key] = newRow
	return trans, nil
}

// Get returns the current StateRow + true, or zero+false if absent.
func (m *Memory) Get(_ context.Context, tenantID uuid.UUID, ruleID, subject string) (constitution.StateRow, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	row, ok := m.all[stateKey{tenant: tenantID, rule: ruleID, subject: subject}]
	return row, ok, nil
}

// List returns rows matching the query, sorted by TransitionedAt desc.
func (m *Memory) List(_ context.Context, tenantID uuid.UUID, q constitution.ListQuery) ([]constitution.StateRow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]constitution.StateRow, 0, len(m.all))
	for k, v := range m.all {
		if k.tenant != tenantID {
			continue
		}
		if q.RuleID != "" && k.rule != q.RuleID {
			continue
		}
		if q.Subject != "" && k.subject != q.Subject {
			continue
		}
		if q.Decision != nil && *q.Decision != v.Decision {
			continue
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].TransitionedAt.After(out[j].TransitionedAt)
	})
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, nil
}
