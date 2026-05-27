package state

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vasic-digital/herald/commons_constitution"
)

// MemoryAudit is the in-memory constitution.AuditStore used at M1 + unit
// tests. Append-only: RecordAudit only ever grows the slice; there is no
// mutator that removes or rewrites a recorded row (mirrors the Postgres
// append-only RLS policy in migration 000006).
//
// Concurrent-safe via a single RWMutex; audit volume is bounded by the
// transition rate, which is far below contention thresholds.
type MemoryAudit struct {
	mu  sync.RWMutex
	all []constitution.AuditRow
	now func() time.Time // injectable clock for deterministic tests
}

// NewMemoryAudit returns an empty in-memory AuditStore using time.Now.
func NewMemoryAudit() *MemoryAudit {
	return &MemoryAudit{now: time.Now}
}

// WithClock returns the MemoryAudit with `now` as its clock. Foundation
// tests use this for deterministic AuditedAt timestamps.
func (m *MemoryAudit) WithClock(now func() time.Time) *MemoryAudit {
	m.now = now
	return m
}

// RecordAudit appends row. If row.ID is uuid.Nil a fresh UUID is assigned
// (matching the Postgres uuidv7 server-default). If row.AuditedAt is zero
// the clock fills it. Returns the row's ID.
func (m *MemoryAudit) RecordAudit(_ context.Context, row constitution.AuditRow) (uuid.UUID, error) {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	if row.AuditedAt.IsZero() {
		row.AuditedAt = m.now()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.all = append(m.all, row)
	return row.ID, nil
}

// ListAudit returns rows for tenantID matching q, newest-first (AuditedAt
// DESC), then Offset+Limit applied — mirroring the Postgres backend.
func (m *MemoryAudit) ListAudit(_ context.Context, tenantID uuid.UUID, q constitution.AuditQuery) ([]constitution.AuditRow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]constitution.AuditRow, 0, len(m.all))
	for _, r := range m.all {
		if r.TenantID != tenantID {
			continue
		}
		if q.RuleID != "" && r.RuleID != q.RuleID {
			continue
		}
		if q.Subject != "" && r.Subject != q.Subject {
			continue
		}
		if !q.Since.IsZero() && r.AuditedAt.Before(q.Since) {
			continue
		}
		if !q.Until.IsZero() && r.AuditedAt.After(q.Until) {
			continue
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].AuditedAt.After(out[j].AuditedAt)
	})
	if q.Offset > 0 {
		if q.Offset >= len(out) {
			return []constitution.AuditRow{}, nil
		}
		out = out[q.Offset:]
	}
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, nil
}
