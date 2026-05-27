package bindings_test

// HRD-025 — scherald scheduled-audit constitution bindings (v1.0.0 Batch B,
// unit 7).
//
// These tests are written FIRST (TDD RED) against the bindings package that
// wires scherald's §42.3-owned scheduled-audit rules into the Batch-A
// commons_constitution Evaluator + Registry + AuditStore + ModeLadder
// foundation. Every assertion checks an observable side-effect of the REAL
// foundation running end-to-end (real MemoryBus, real ConstitutionStore, real
// ModeLadder, real MemoryAudit, real emit) — no mocks-of-mocks (§11.4.27).
//
// The load-bearing round-trip these tests prove (the §107 anti-bluff bar for
// HRD-025): a SCHEDULED-AUDIT finding (a periodic Status.md sweep that flagged
// drift, a compliance digest that fell due but was never emitted, a stale-item
// count above threshold) is detected by a registered scherald binding →
// emitted as a .policy.violation event on the bus → persisted as a
// constitution_state row AND a constitution_audit row → visible via the same
// ConstitutionStore.List a future /v1/schedule pull surface reads. A binding
// that "registered" but could never produce a queryable, audited audit verdict
// would be exactly the metadata-only PASS the covenant forbids.
//
// §12 host-safety: every detector CLASSIFIES a recorded sweep / digest /
// staleness outcome string — it NEVER reads Status.md, runs cron, or
// regenerates a digest. The §43 command body (HRD-047) + scheduler supply the
// live integration upstream.

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	constitution "github.com/vasic-digital/herald/commons_constitution"
	"github.com/vasic-digital/herald/commons_constitution/ladder"
	"github.com/vasic-digital/herald/commons_constitution/state"
	"github.com/vasic-digital/herald/scherald/internal/bindings"
)

// newPipeline builds a Pipeline backed entirely by the production-faithful
// in-memory foundation backends (the same seam cherald/sherald/bherald/rherald
// use). Returns the pipeline + the live store/ladder/audit/bus so tests can
// assert persisted side-effects directly.
func newPipeline(t *testing.T) (*bindings.Pipeline, constitution.ConstitutionStore, constitution.ModeLadder, constitution.AuditStore, constitution.EventBus) {
	t.Helper()
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{BufferSize: 256})
	t.Cleanup(func() { _ = bus.Close() })
	em, err := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "digital.vasic.herald/scherald"})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}
	st := state.NewMemory()
	la := ladder.NewMemory()
	au := state.NewMemoryAudit()
	p, err := bindings.NewPipeline(bindings.Config{
		Ladder:  la,
		Store:   st,
		Emitter: em,
		Audit:   au,
	})
	if err != nil {
		t.Fatalf("NewPipeline: %v", err)
	}
	return p, st, la, au, bus
}

// TestRuleCatalogue_CoversScheraldOwnedRules proves the binding catalogue
// actually carries the scherald-owned §42.3 scheduled-audit rows — not an empty
// registry that would make every downstream "binding works" claim vacuous.
func TestRuleCatalogue_CoversScheraldOwnedRules(t *testing.T) {
	cat := bindings.ScheraldRules()
	if len(cat) < 3 {
		t.Fatalf("scherald rule catalogue has only %d rules; HRD-025 lands §11.4.45 + 2 scheduled-audit facets — looks truncated", len(cat))
	}

	byID := map[string]bindings.RuleSpec{}
	for _, r := range cat {
		if _, dup := byID[r.RuleID]; dup {
			t.Fatalf("duplicate rule in catalogue: %q", r.RuleID)
		}
		byID[r.RuleID] = r
	}

	// The three canonical scherald rows from §42.3 + the scheduled-audit facets
	// with their declared default mode + severity + event class.
	want := []struct {
		id    string
		sev   constitution.Severity
		mod   constitution.Mode
		class string
	}{
		{"§11.4.45", constitution.SeverityLow, constitution.ModeWarn, constitution.ClassPolicyViolation},        // Status-Doc maintenance (spec-table owner)
		{"§11.4.45.digest", constitution.SeverityLow, constitution.ModeWarn, constitution.ClassPolicyViolation}, // compliance-digest cadence
		{"§11.4.45.stale", constitution.SeverityLow, constitution.ModeWarn, constitution.ClassPolicyViolation},  // stale-item detection
	}
	for _, w := range want {
		got, ok := byID[w.id]
		if !ok {
			t.Errorf("rule %q missing from scherald catalogue (§42.3 / HRD-025 assigns it to scherald)", w.id)
			continue
		}
		if got.Severity != w.sev {
			t.Errorf("rule %q severity = %v, want %v", w.id, got.Severity, w.sev)
		}
		if got.DefaultMode != w.mod {
			t.Errorf("rule %q default mode = %v, want %v", w.id, got.DefaultMode, w.mod)
		}
		if got.EventClass != w.class {
			t.Errorf("rule %q event class = %q, want %q", w.id, got.EventClass, w.class)
		}
	}

	// Every rule MUST carry a non-nil Check func + non-empty EventClass — a rule
	// with no evaluator logic can never produce a real audit verdict (a §107
	// binding bluff).
	for _, r := range cat {
		if r.Check == nil {
			t.Errorf("rule %q has a nil Check func — it can never detect an audit verdict", r.RuleID)
		}
		if r.EventClass == "" {
			t.Errorf("rule %q has an empty EventClass — cannot route the emit", r.RuleID)
		}
	}
}

// TestPipeline_RegistersEveryRule proves NewPipeline registered every catalogue
// rule into the underlying commons_constitution.Registry.
func TestPipeline_RegistersEveryRule(t *testing.T) {
	p, _, _, _, _ := newPipeline(t)
	got := p.Registry().Len()
	want := len(bindings.ScheraldRules())
	if got != want {
		t.Fatalf("pipeline registered %d evaluators, catalogue has %d", got, want)
	}
	for _, r := range bindings.ScheraldRules() {
		if _, ok := p.Registry().Get(r.RuleID); !ok {
			t.Errorf("rule %q not retrievable from the pipeline registry", r.RuleID)
		}
	}
}

// TestPipeline_StatusSweepPolicyViolationRoundTrip is THE load-bearing HRD-025
// anti-bluff proof.
//
// It drives a real scherald binding (§11.4.45 status-sweep) whose Check func
// classifies a recorded sweep finding as a STALE Status.md, through the REAL
// Pipeline, and asserts the FULL round-trip:
//
//	(1) a .policy.violation event reaches the bus (the channel emit fired);
//	(2) a constitution_state row was persisted with decision=fail + the bundle
//	    hash + the subject (queryable via a future /v1/schedule pull surface);
//	(3) a constitution_audit row was written naming the emitted event ID.
//
// A binding that "evaluated" but produced no queryable/audited evidence is a
// §107 PASS-bluff; this test makes that impossible to ship.
func TestPipeline_StatusSweepPolicyViolationRoundTrip(t *testing.T) {
	ctx := context.Background()
	p, st, la, au, bus := newPipeline(t)
	tenant := uuid.New()

	// §11.4.45 is warn by default — flip to enforce so the emit fires (warn
	// audits only).
	if err := la.Set(ctx, tenant, "§11.4.45", constitution.ModeEnforce, "test"); err != nil {
		t.Fatalf("ladder Set enforce: %v", err)
	}

	sub, err := bus.Subscribe(constitution.EventNamespace + "." + constitution.ClassPolicyViolation)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Cancel()
	var violations int64
	go func() {
		for range sub.Channel {
			atomic.AddInt64(&violations, 1)
		}
	}()

	// A periodic sweep that found Status.md stale. The §43 scheduler records the
	// sweep finding; the binding classifies it. NO real Status.md read / cron.
	staleSweep := constitution.Subject{Kind: bindings.SubjectStatusSweep, ID: "Status.md|sweep=stale|stale_items=4"}

	out, err := p.EvaluateSubject(ctx, "§11.4.45", tenant, staleSweep)
	if err != nil {
		t.Fatalf("EvaluateSubject: %v", err)
	}
	if out.Decision != constitution.DecisionFail {
		t.Fatalf("decision = %v, want fail for a stale Status.md sweep", out.Decision)
	}
	if !out.Transition.Changed || !out.Transition.FirstSeen {
		t.Fatalf("first sweep result should be a FirstSeen+Changed transition, got %+v", out.Transition)
	}
	if out.Mode != constitution.ModeEnforce {
		t.Fatalf("mode = %v, want enforce", out.Mode)
	}
	if !out.Emitted {
		t.Fatalf("enforce-mode transition must emit; out=%+v", out)
	}
	if !out.Audited {
		t.Fatalf("enforce-mode transition must write an audit row; out=%+v", out)
	}

	// (1) the emit reached the bus.
	deadline := time.After(2 * time.Second)
	for atomic.LoadInt64(&violations) < 1 {
		select {
		case <-deadline:
			t.Fatalf("listener never saw the .policy.violation; bus metrics=%+v", bus.Metrics())
		case <-time.After(5 * time.Millisecond):
		}
	}

	// (2) a constitution_state row is queryable.
	rows, err := st.List(ctx, tenant, constitution.ListQuery{RuleID: "§11.4.45"})
	if err != nil {
		t.Fatalf("store List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 persisted state row, got %d", len(rows))
	}
	if rows[0].Decision != constitution.DecisionFail {
		t.Errorf("persisted decision = %v, want fail", rows[0].Decision)
	}
	if rows[0].Subject != staleSweep.ID {
		t.Errorf("persisted subject = %q, want %q", rows[0].Subject, staleSweep.ID)
	}

	// (3) a constitution_audit row was written naming the emitted event ID.
	audit, err := au.ListAudit(ctx, tenant, constitution.AuditQuery{RuleID: "§11.4.45"})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(audit) != 1 {
		t.Fatalf("expected exactly 1 audit row, got %d", len(audit))
	}
	if audit[0].ModeAtEmission != constitution.ModeEnforce {
		t.Errorf("audit mode = %v, want enforce", audit[0].ModeAtEmission)
	}
	if audit[0].EmittedEventID == uuid.Nil {
		t.Errorf("policy.violation audit row should capture the emitted event ID (IDEmitter), got Nil")
	}

	// The wire event MUST be exactly .policy.violation.
	published := bus.Metrics().PublishedByType[constitution.EventNamespace+"."+constitution.ClassPolicyViolation]
	if published != 1 {
		t.Fatalf("bus published %d .policy.violation events, want 1", published)
	}
}

// TestPipeline_PolicyClearedOnRecovery proves a scheduled-audit gate that
// transitions back to PASS emits .policy.cleared (NOT another .policy.violation)
// — the recovery signal subscribers key on. A binding that emitted
// .policy.violation on a recovery would mis-signal subscribers.
func TestPipeline_PolicyClearedOnRecovery(t *testing.T) {
	ctx := context.Background()
	p, _, la, _, bus := newPipeline(t)
	tenant := uuid.New()
	if err := la.Set(ctx, tenant, "§11.4.45.digest", constitution.ModeEnforce, "test"); err != nil {
		t.Fatalf("ladder Set: %v", err)
	}

	clearedSub, err := bus.Subscribe(constitution.EventNamespace + "." + constitution.ClassPolicyCleared)
	if err != nil {
		t.Fatalf("Subscribe policy.cleared: %v", err)
	}
	defer clearedSub.Cancel()
	var cleared int64
	go func() {
		for range clearedSub.Channel {
			atomic.AddInt64(&cleared, 1)
		}
	}()

	// First a FAIL (digest due but not emitted), then a PASS (digest emitted on
	// schedule) for the SAME cadence so the second is a recovery transition.
	bad := constitution.Subject{Kind: bindings.SubjectDigestCadence, ID: "weekly|due=true|emitted=false"}
	if _, err := p.EvaluateSubject(ctx, "§11.4.45.digest", tenant, bad); err != nil {
		t.Fatalf("EvaluateSubject(fail): %v", err)
	}
	good := constitution.Subject{Kind: bindings.SubjectDigestCadence, ID: "weekly|due=true|emitted=true"}
	out, err := p.EvaluateSubject(ctx, "§11.4.45.digest", tenant, good)
	if err != nil {
		t.Fatalf("EvaluateSubject(pass): %v", err)
	}
	if out.Decision != constitution.DecisionPass {
		t.Fatalf("decision = %v, want pass on an emitted digest", out.Decision)
	}
	if !out.Transition.Changed {
		t.Fatalf("fail→pass should be a Changed transition, got %+v", out.Transition)
	}
	if !out.Emitted {
		t.Fatalf("enforce-mode recovery must emit; out=%+v", out)
	}

	deadline := time.After(2 * time.Second)
	for atomic.LoadInt64(&cleared) < 1 {
		select {
		case <-deadline:
			t.Fatalf("listener never saw the .policy.cleared; bus metrics=%+v", bus.Metrics())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestPipeline_LadderGatesEmit proves the binding decision is GATED by the mode
// ladder — an allow-mode rule records state but does NOT emit or audit (the
// §42.5-step "initial mode allow = data gathering only" contract).
func TestPipeline_LadderGatesEmit(t *testing.T) {
	ctx := context.Background()
	p, st, la, au, bus := newPipeline(t)
	tenant := uuid.New()

	if err := la.Set(ctx, tenant, "§11.4.45", constitution.ModeAllow, "test"); err != nil {
		t.Fatalf("ladder Set allow: %v", err)
	}

	before := bus.Metrics().Published
	bad := constitution.Subject{Kind: bindings.SubjectStatusSweep, ID: "Status.md|sweep=stale|stale_items=2"}
	out, err := p.EvaluateSubject(ctx, "§11.4.45", tenant, bad)
	if err != nil {
		t.Fatalf("EvaluateSubject: %v", err)
	}
	if out.Decision != constitution.DecisionFail {
		t.Fatalf("decision = %v, want fail", out.Decision)
	}
	if out.Emitted {
		t.Errorf("allow-mode MUST NOT emit; out=%+v", out)
	}
	if out.Audited {
		t.Errorf("allow-mode MUST NOT audit; out=%+v", out)
	}
	if after := bus.Metrics().Published; after != before {
		t.Errorf("allow-mode published %d events (want 0)", after-before)
	}
	rows, err := st.List(ctx, tenant, constitution.ListQuery{RuleID: "§11.4.45"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("allow-mode still records state: want 1 row, got %d", len(rows))
	}
	audit, err := au.ListAudit(ctx, tenant, constitution.AuditQuery{})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(audit) != 0 {
		t.Errorf("allow-mode wrote %d audit rows (want 0)", len(audit))
	}
}

// TestPipeline_UnknownRule errors loudly rather than silently passing — a typo'd
// rule ID must not be swallowed (§11.4.6 no-guessing at the API layer).
func TestPipeline_UnknownRule(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	_, err := p.EvaluateSubject(ctx, "§does.not.exist", uuid.New(), constitution.Subject{Kind: "schedule", ID: "x"})
	if err == nil {
		t.Fatalf("expected error for unknown rule, got nil")
	}
}

// ---------------------------------------------------------------------------
// Bespoke detector tests — the 3 load-bearing scherald scheduled-audit classifiers.
// ---------------------------------------------------------------------------

// TestDetector_StatusSweep proves the §11.4.45 status-sweep detector classifies
// a recorded sweep finding: a clean Status.md (summary synced) PASSes; a stale
// Status.md FAILs; a clean Status.md with an out-of-sync Status_Summary.md FAILs
// (the §11.4.56 composition). PURE: it reads the recorded sweep result; it NEVER
// reads Status.md or runs the sweep.
func TestDetector_StatusSweep(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()

	cases := []struct {
		id   string
		want constitution.Decision
	}{
		{"Status.md|sweep=clean", constitution.DecisionPass},
		{"Status.md|sweep=clean|summary_synced=true", constitution.DecisionPass},
		{"Status.md|sweep=stale", constitution.DecisionFail},
		{"Status.md|sweep=stale|stale_items=7", constitution.DecisionFail},
		{"Status.md|sweep=clean|summary_synced=false", constitution.DecisionFail}, // §11.4.56 facet
		{"Status.md", constitution.DecisionFail},                                  // no sweep field → refuse to silent-PASS
	}
	for _, c := range cases {
		subj := constitution.Subject{Kind: bindings.SubjectStatusSweep, ID: c.id}
		out, err := p.EvaluateSubject(ctx, "§11.4.45", tenant, subj)
		if err != nil {
			t.Fatalf("EvaluateSubject(%s): %v", c.id, err)
		}
		if out.Decision != c.want {
			t.Errorf("status-sweep %q classified as %v, want %v", c.id, out.Decision, c.want)
		}
	}
}

// TestDetector_DigestCadence proves the §18.7 digest-cadence detector: a digest
// not due this tick PASSes; a digest due AND emitted PASSes; a digest due but
// NOT emitted FAILs (a missed scheduled digest). PURE: it reads the recorded
// cadence tally; it NEVER regenerates a digest or consults a clock.
func TestDetector_DigestCadence(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()

	cases := []struct {
		id   string
		want constitution.Decision
	}{
		{"daily|due=false", constitution.DecisionPass},
		{"daily|due=true|emitted=true", constitution.DecisionPass},
		{"weekly|due=true|emitted=false", constitution.DecisionFail},
		{"monthly|due=true|emitted=false|overdue_by_h=26", constitution.DecisionFail},
		{"daily", constitution.DecisionFail}, // no due field → refuse to silent-PASS
	}
	for _, c := range cases {
		subj := constitution.Subject{Kind: bindings.SubjectDigestCadence, ID: c.id}
		out, err := p.EvaluateSubject(ctx, "§11.4.45.digest", tenant, subj)
		if err != nil {
			t.Fatalf("EvaluateSubject(%s): %v", c.id, err)
		}
		if out.Decision != c.want {
			t.Errorf("digest-cadence %q classified as %v, want %v", c.id, out.Decision, c.want)
		}
	}
}

// TestDetector_StaleItem proves the §11.4.45 stale-item detector: a count within
// the threshold PASSes; a count above the threshold FAILs; a missing count FAILs
// (no audit evidence). PURE: it reads the recorded count; it NEVER walks the HRD
// trackers.
func TestDetector_StaleItem(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()

	cases := []struct {
		id   string
		want constitution.Decision
	}{
		{"trackers|stale_items=0", constitution.DecisionPass},
		{"trackers|stale_items=0|threshold=3", constitution.DecisionPass},
		{"trackers|stale_items=3|threshold=3", constitution.DecisionPass}, // at threshold = OK
		{"trackers|stale_items=4|threshold=3", constitution.DecisionFail},
		{"trackers|stale_items=2", constitution.DecisionFail},             // default threshold 0
		{"trackers", constitution.DecisionFail},                           // no count → refuse to silent-PASS
	}
	for _, c := range cases {
		subj := constitution.Subject{Kind: bindings.SubjectStaleItem, ID: c.id}
		out, err := p.EvaluateSubject(ctx, "§11.4.45.stale", tenant, subj)
		if err != nil {
			t.Fatalf("EvaluateSubject(%s): %v", c.id, err)
		}
		if out.Decision != c.want {
			t.Errorf("stale-item %q classified as %v, want %v", c.id, out.Decision, c.want)
		}
	}
}

// TestPipeline_MalformedSubjectDoesNotPass proves a scheduled-audit subject with
// no recognizable outcome MUST NOT silently PASS (§11.4.1 fail-bluff inverse).
func TestPipeline_MalformedSubjectDoesNotPass(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()
	subj := constitution.Subject{Kind: bindings.SubjectStatusSweep, ID: "garbage-no-fields"}
	out, err := p.EvaluateSubject(ctx, "§11.4.45", tenant, subj)
	if err != nil {
		t.Fatalf("EvaluateSubject: %v", err)
	}
	if out.Decision == constitution.DecisionPass {
		t.Errorf("malformed status-sweep (no fields) classified as PASS — that is a §11.4.1 fail-bluff inverse")
	}
}
