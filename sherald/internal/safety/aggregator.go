// Package safety implements sherald's process-local safety state +
// the GET /v1/safety_state handler. Per Wave 3 design §3, sherald's
// daemon-mode state is in-memory only — no PG read on the hot path.
//
// Counters update via:
//   - RecordDestructiveOp (called by §43 destructive-guard stub bodies
//     when they ship; for now exposed to tests + sampler.go demo paths).
//   - UpdateMemPercent (called by the background sampler.go goroutine
//     every HERALD_SAFETY_MEM_SAMPLE_INTERVAL — default 10s).
//
// §107 anti-bluff: every field on the SafetyState struct is read-positive
// — Snapshot() takes one RLock and deep-copies the destructive_op pointer
// so the caller can JSON-encode without holding the lock. There is no
// "absence-of-error" PASS; the tests assert each field's content under
// race-detector load.
package safety

import (
	"sync"
	"sync/atomic"
	"time"
)

// Aggregator holds sherald's process-local safety state.
//
// Concurrency model:
//   - openEvents: atomic counter — safe to Add from any goroutine without mu.
//   - lastDestructiveOp / lastMemSampleAt / lastMemPercent: guarded by mu.
//     Writes take mu.Lock; Snapshot() takes mu.RLock so multiple readers
//     never block each other.
type Aggregator struct {
	startedAt         time.Time
	openEvents        atomic.Int64
	mu                sync.RWMutex
	lastDestructiveOp *DestructiveOp
	lastMemSampleAt   time.Time
	lastMemPercent    float64
}

// NewAggregator returns a fresh Aggregator with startedAt=now.
func NewAggregator() *Aggregator {
	return &Aggregator{startedAt: time.Now()}
}

// DestructiveOp records one observed destructive operation (rm, git-reset,
// git-push-force, etc.) per V3 §43 destructive-guard semantics.
type DestructiveOp struct {
	Op        string    `json:"op"`
	Path      string    `json:"path"`
	Operator  string    `json:"operator"`
	Blocked   bool      `json:"blocked"`
	BlockedAt time.Time `json:"at"`
	HRDRule   string    `json:"hrd_rule"`
}

// SafetyState is the public snapshot — what GET /v1/safety_state returns.
//
// LastDestructiveOp is a pointer so the JSON encoder emits
// `"last_destructive_op": null` (not `{}`) on a fresh aggregator. The §107
// covenant on this field: nil iff RecordDestructiveOp has never been called
// on this Aggregator instance.
type SafetyState struct {
	Binary            string         `json:"binary"`
	StartedAt         time.Time      `json:"started_at"`
	UptimeSeconds     int64          `json:"uptime_seconds"`
	OpenEvents        int64          `json:"open_events"`
	CurrentMemPercent float64        `json:"current_mem_percent"`
	LastMemSampleAt   time.Time      `json:"last_mem_sample_at"`
	LastDestructiveOp *DestructiveOp `json:"last_destructive_op"` // nil = none seen yet
}

// Snapshot returns a deep copy of the current state, safe to JSON-encode
// without holding the lock. The destructive-op pointer is shallow-copied
// under RLock — that is safe because DestructiveOp has no pointer fields
// (strings, time.Time, bool are value-typed).
func (a *Aggregator) Snapshot() SafetyState {
	now := time.Now()
	a.mu.RLock()
	defer a.mu.RUnlock()
	var op *DestructiveOp
	if a.lastDestructiveOp != nil {
		cp := *a.lastDestructiveOp // shallow copy is fine — no pointer fields
		op = &cp
	}
	return SafetyState{
		Binary:            "sherald",
		StartedAt:         a.startedAt,
		UptimeSeconds:     int64(now.Sub(a.startedAt).Seconds()),
		OpenEvents:        a.openEvents.Load(),
		CurrentMemPercent: a.lastMemPercent,
		LastMemSampleAt:   a.lastMemSampleAt,
		LastDestructiveOp: op,
	}
}

// RecordDestructiveOp records a destructive operation observation AND
// increments openEvents.
//
// The atomic.Add on openEvents is independent of the mutex — concurrent
// callers will each see a monotonically increasing counter. The mutex
// (write lock — NOT RLock) protects the lastDestructiveOp pointer swap.
func (a *Aggregator) RecordDestructiveOp(op DestructiveOp) {
	a.openEvents.Add(1)
	a.mu.Lock()
	a.lastDestructiveOp = &op
	a.mu.Unlock()
}

// UpdateMemPercent refreshes the current memory-usage percentage AND
// timestamp under a single write lock. Called by the background sampler.
func (a *Aggregator) UpdateMemPercent(pct float64) {
	a.mu.Lock()
	a.lastMemPercent = pct
	a.lastMemSampleAt = time.Now()
	a.mu.Unlock()
}
