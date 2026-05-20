package constitution

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/google/uuid"
)

// Severity enumerates the four severity tiers from Universal Constitution
// §11.4.X taxonomy. Higher values trigger more aggressive enforcement
// (push-trigger evaluation for SeverityCritical; sweep-trigger for the rest).
type Severity int

const (
	SeverityLow Severity = iota
	SeverityMiddle
	SeverityHigh
	SeverityCritical
)

func (s Severity) String() string {
	switch s {
	case SeverityLow:
		return "low"
	case SeverityMiddle:
		return "middle"
	case SeverityHigh:
		return "high"
	case SeverityCritical:
		return "critical"
	default:
		return fmt.Sprintf("severity(%d)", int(s))
	}
}

// Decision is the outcome an Evaluator returns for a single Subject.
// Per spec §42.1.1 + the transition gate in the Foundation design.
type Decision int

const (
	DecisionPass Decision = iota
	DecisionWarn
	DecisionFail
	DecisionError // evaluator itself failed (panic, network, config) — not a violation
	DecisionSkip  // evaluator declined to evaluate (e.g., bundle unreadable)
)

func (d Decision) String() string {
	switch d {
	case DecisionPass:
		return "pass"
	case DecisionWarn:
		return "warn"
	case DecisionFail:
		return "fail"
	case DecisionError:
		return "error"
	case DecisionSkip:
		return "skip"
	default:
		return fmt.Sprintf("decision(%d)", int(d))
	}
}

// Subject is what an Evaluator evaluates. The Kind+ID pair MUST be stable
// across runs — it forms half of the (tenant, rule, subject) PK in the
// constitution_state table.
type Subject struct {
	Kind string // "file" / "repo" / "commit" / "tenant" / "release-tag" / "spec-doc" / ...
	ID   string // path, repo URL, SHA, tenant UUID, ref, doc path
}

func (s Subject) String() string { return s.Kind + ":" + s.ID }

// Result is what an Evaluator returns for a single Subject + BundleHash.
// DigestSHA is opaque — the Evaluator computes it however it likes (typically
// a hash of the evidence + the decision); the transition gate uses it to
// detect "verdict-unchanged-but-rationale-changed" transitions (spec §3.4 of
// the Foundation design).
type Result struct {
	Decision  Decision
	Evidence  string   // URI or short message describing why
	DigestSHA [32]byte // hash of evaluator's output for transition detection
}

// Evaluator is the framework's primary extension point. Every constitution
// rule (e.g., §11.4.10 credential-leak, §11.4.65 release-gate) implements
// this interface and registers via Registry.Register.
//
// Lifetime contract:
//   - RuleID, Severity, PushTriggers are STABLE for the process lifetime.
//   - Subjects + Evaluate may make I/O but MUST honor ctx cancellation.
//   - Evaluate MUST be safe to call concurrently for the same evaluator.
//   - Panics inside Evaluate are caught by Registry.runOne (see emit.go)
//     and translated to DecisionError without propagating to other
//     evaluators (per §4 row 1 of the Foundation design).
type Evaluator interface {
	RuleID() string                                                          // "§11.4.10"
	Severity() Severity                                                      // critical / high / middle / low
	PushTriggers() []string                                                  // CloudEvents `type` values that cause re-evaluation
	Subjects(ctx context.Context, tenantID uuid.UUID) ([]Subject, error)     // what to evaluate
	Evaluate(ctx context.Context, s Subject, bundle BundleHash) (Result, error)
}

// Registry maps RuleID to Evaluator. Concurrent-safe.
//
// Foundation only ships the Registry primitives; actual evaluator
// implementations land in Sub-projects 2+ (HRD-019..HRD-025).
type Registry struct {
	mu  sync.RWMutex
	all map[string]Evaluator
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{all: make(map[string]Evaluator)}
}

// Register adds e under e.RuleID(). Re-registering the same RuleID with a
// different evaluator panics — duplicate registration is a programming
// error, not a runtime condition.
func (r *Registry) Register(e Evaluator) {
	if e == nil {
		panic("constitution: Registry.Register: nil evaluator")
	}
	id := e.RuleID()
	if id == "" {
		panic("constitution: Registry.Register: empty RuleID")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.all[id]; ok && existing != e {
		panic(fmt.Sprintf("constitution: Registry.Register: duplicate RuleID %q", id))
	}
	r.all[id] = e
}

// Get returns the evaluator for ruleID + true, or (nil, false) if absent.
func (r *Registry) Get(ruleID string) (Evaluator, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.all[ruleID]
	return e, ok
}

// Len returns the number of registered evaluators.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.all)
}

// IterByMaxSeverity returns evaluators whose Severity() <= max, sorted
// by RuleID for deterministic iteration. Used by the pull-trigger sweep
// (Sub-project 5) to walk non-critical rules.
//
// The returned slice is a snapshot — safe to range over even if the
// Registry is mutated concurrently.
func (r *Registry) IterByMaxSeverity(max Severity) []Evaluator {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Evaluator, 0, len(r.all))
	for _, e := range r.all {
		if e.Severity() <= max {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RuleID() < out[j].RuleID() })
	return out
}

// All returns a snapshot of every registered evaluator, sorted by RuleID.
func (r *Registry) All() []Evaluator {
	return r.IterByMaxSeverity(SeverityCritical)
}
