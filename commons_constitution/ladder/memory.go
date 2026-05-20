// Package ladder backs the constitution.ModeLadder interface with an
// in-memory map. Test-only — production deployments use the Postgres
// backend at M2 + the Redis cache wrapper at M3.
package ladder

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/vasic-digital/herald/commons_constitution"
)

// Memory is the in-memory ModeLadder used in M1 unit tests and in the
// memory-only path of the M1 smoke test.
//
// Concurrent-safe via a single RWMutex; sufficient for the tiny binding
// counts that fit in memory (~1k tenants × ~70 rules = ~70k entries; well
// within a map's no-contention range).
type Memory struct {
	mu sync.RWMutex
	// keyed by tenantID; inner map keyed by ruleID.
	bindings map[uuid.UUID]map[string]constitution.Mode
	// audit trail of every Set call, for tests that assert mutation provenance.
	mutations []Mutation
}

// Mutation records who Set what when. Mirrors the constitution_bindings
// audit columns from migration 000007 (lands at M2).
type Mutation struct {
	TenantID uuid.UUID
	RuleID   string
	Mode     constitution.Mode
	By       string
}

// NewMemory returns an empty Memory ladder.
func NewMemory() *Memory {
	return &Memory{bindings: make(map[uuid.UUID]map[string]constitution.Mode)}
}

// Get returns the configured mode or ModeEnforce (the safe default) if no
// binding exists.
func (m *Memory) Get(_ context.Context, tenantID uuid.UUID, ruleID string) (constitution.Mode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if t, ok := m.bindings[tenantID]; ok {
		if mode, ok := t[ruleID]; ok {
			return mode, nil
		}
	}
	return constitution.ModeEnforce, nil
}

// Set writes the binding and appends to the audit trail. Always succeeds
// in-memory; production backends may return errors.
func (m *Memory) Set(_ context.Context, tenantID uuid.UUID, ruleID string, mode constitution.Mode, by string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.bindings[tenantID] == nil {
		m.bindings[tenantID] = make(map[string]constitution.Mode)
	}
	m.bindings[tenantID][ruleID] = mode
	m.mutations = append(m.mutations, Mutation{
		TenantID: tenantID, RuleID: ruleID, Mode: mode, By: by,
	})
	return nil
}

// List returns a copy of the tenant's bindings — safe to range over even
// if Set is called concurrently afterwards.
func (m *Memory) List(_ context.Context, tenantID uuid.UUID) (map[string]constitution.Mode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]constitution.Mode, len(m.bindings[tenantID]))
	for k, v := range m.bindings[tenantID] {
		out[k] = v
	}
	return out, nil
}

// Mutations returns a snapshot of every Set call so far — used by tests
// that assert audit-trail completeness.
func (m *Memory) Mutations() []Mutation {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]Mutation(nil), m.mutations...)
}
