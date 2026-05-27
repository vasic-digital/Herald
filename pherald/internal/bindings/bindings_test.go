package bindings_test

// HRD-023 — pherald PROJECT constitution bindings (v1.0.0 Batch B, unit 5).
//
// Written FIRST (TDD RED) against the pherald bindings package that wires
// pherald's §42.3-owned PROJECT-lifecycle constitution rules into the Batch-A
// commons_constitution Evaluator + Registry + AuditStore + ModeLadder + typed
// emitter foundation. Every assertion checks an observable side-effect of the
// REAL foundation running end-to-end (real MemoryBus, real ConstitutionStore,
// real ModeLadder, real MemoryAudit, real emit) — no mocks-of-mocks (§11.4.27).
//
// The load-bearing round-trip these tests prove (the §107 anti-bluff bar for
// HRD-023): a PROJECT-discipline RESULT (a commit made outside the single locked
// entrypoint, a submodule propagated parent-before-inner, a push attempted
// without a pre-push fetch, an edit on a stale tree) is detected by a registered
// pherald binding → emitted as a .repo.safety.breach (or .policy.violation for
// the reopens/install-upstreams policy rows) event on the bus → persisted as a
// constitution_state row AND a constitution_audit row → visible via the same
// ConstitutionStore.List a future /v1/project pull surface reads. A binding that
// "registered" but could never produce a queryable, audited project verdict is
// exactly the metadata-only PASS the covenant forbids.
//
// §12 host-safety: every detector CLASSIFIES a recorded project-op outcome
// string — it NEVER commits, pushes, force-pushes, runs git, or touches the
// filesystem. The §43 command bodies (HRD-029 commit-push, HRD-030 submodule
// propagate, HRD-044 fetch-guard, HRD-053 pre-push) supply the live project
// integration upstream — live op interception is scope-locked to those §43
// follow-ups.

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	constitution "github.com/vasic-digital/herald/commons_constitution"
	"github.com/vasic-digital/herald/commons_constitution/ladder"
	"github.com/vasic-digital/herald/commons_constitution/state"
	"github.com/vasic-digital/herald/pherald/internal/bindings"
)

// newPipeline builds a Pipeline backed entirely by the production-faithful
// in-memory foundation backends (the same seam cherald/sherald/bherald/rherald
// use). Returns the pipeline + the live store/ladder/audit/bus so tests can
// assert persisted side-effects directly.
func newPipeline(t *testing.T) (*bindings.Pipeline, constitution.ConstitutionStore, constitution.ModeLadder, constitution.AuditStore, constitution.EventBus) {
	t.Helper()
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{BufferSize: 256})
	t.Cleanup(func() { _ = bus.Close() })
	em, err := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "digital.vasic.herald/pherald"})
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

// TestRuleCatalogue_CoversPheraldOwnedRules proves the binding catalogue
// actually carries the pherald-owned §42.3 PROJECT rows — not an empty registry
// that would make every downstream "binding works" claim vacuous. We assert (a)
// the canonical spec §42.3 pherald rows are present and (b) each carries the
// spec's default mode + severity + event class.
func TestRuleCatalogue_CoversPheraldOwnedRules(t *testing.T) {
	cat := bindings.PheraldRules()
	if len(cat) < 6 {
		t.Fatalf("pherald rule catalogue has only %d rules; spec §42.3 assigns pherald §2/§3/§11.4.36/§11.4.37/§11.4.55/§11.4.71 — looks truncated", len(cat))
	}

	byID := map[string]bindings.RuleSpec{}
	for _, r := range cat {
		if _, dup := byID[r.RuleID]; dup {
			t.Fatalf("duplicate rule in catalogue: %q", r.RuleID)
		}
		byID[r.RuleID] = r
	}

	// The canonical pherald rows from §42.3 with their declared default mode +
	// severity + event class. These are load-bearing: a binding registered with
	// the wrong default mode/class would silently misbehave against spec.
	want := []struct {
		id    string
		sev   constitution.Severity
		mod   constitution.Mode
		class string
	}{
		{"§2", constitution.SeverityCritical, constitution.ModeEnforce, constitution.ClassRepoSafetyBreach},   // commit+push entrypoint
		{"§3", constitution.SeverityHigh, constitution.ModeEnforce, constitution.ClassRepoSafetyBreach},       // submodule propagation order
		{"§11.4.36", constitution.SeverityMiddle, constitution.ModeWarn, constitution.ClassPolicyViolation},   // install-upstreams
		{"§11.4.37", constitution.SeverityHigh, constitution.ModeEnforce, constitution.ClassRepoSafetyBreach}, // fetch-before-edit
		{"§11.4.55", constitution.SeverityMiddle, constitution.ModeWarn, constitution.ClassPolicyViolation},   // reopens-history
		{"§11.4.71", constitution.SeverityHigh, constitution.ModeEnforce, constitution.ClassRepoSafetyBreach}, // pre-push fetch+integrate
	}
	for _, w := range want {
		got, ok := byID[w.id]
		if !ok {
			t.Errorf("rule %q missing from pherald catalogue (§42.3 assigns it to pherald)", w.id)
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
	// with no evaluator logic can never produce a real project verdict (a §107
	// binding bluff).
	for _, r := range cat {
		if r.Check == nil {
			t.Errorf("rule %q has a nil Check func — it can never detect a project verdict", r.RuleID)
		}
		if r.EventClass == "" {
			t.Errorf("rule %q has an empty EventClass — cannot route the emit", r.RuleID)
		}
	}
}

// TestPipeline_RegistersEveryRule proves NewPipeline registered every catalogue
// rule into the underlying commons_constitution.Registry — so a sweep over the
// registry actually sees the pherald rule set (not a partial registration).
func TestPipeline_RegistersEveryRule(t *testing.T) {
	p, _, _, _, _ := newPipeline(t)
	got := p.Registry().Len()
	want := len(bindings.PheraldRules())
	if got != want {
		t.Fatalf("pipeline registered %d evaluators, catalogue has %d", got, want)
	}
	for _, r := range bindings.PheraldRules() {
		if _, ok := p.Registry().Get(r.RuleID); !ok {
			t.Errorf("rule %q not retrievable from the pipeline registry", r.RuleID)
		}
	}
}

// TestPipeline_RepoSafetyBreachRoundTrip is THE load-bearing HRD-023 anti-bluff
// proof.
//
// It drives a real pherald binding (§2 commit-push-discipline) whose Check func
// classifies a recorded commit as made OUTSIDE the single locked entrypoint —
// the §2 "entrypoint bypassed" repo-safety breach — through the REAL Pipeline,
// and asserts the FULL round-trip:
//
//	(1) a .repo.safety.breach event reaches the bus (the channel emit fired);
//	(2) a constitution_state row was persisted with decision=fail + the bundle
//	    hash + the subject (queryable via a future /v1/project pull surface);
//	(3) a constitution_audit row was written naming the emitted event ID.
//
// A binding that "evaluated" but produced no queryable/audited evidence is a
// §107 PASS-bluff; this test makes that impossible to ship.
func TestPipeline_RepoSafetyBreachRoundTrip(t *testing.T) {
	ctx := context.Background()
	p, st, la, au, bus := newPipeline(t)
	tenant := uuid.New()

	// §2 default-enforces per §42.3 (critical repo-safety).
	if err := la.Set(ctx, tenant, "§2", constitution.ModeEnforce, "test"); err != nil {
		t.Fatalf("ladder Set enforce: %v", err)
	}

	sub, err := bus.Subscribe(constitution.EventNamespace + "." + constitution.ClassRepoSafetyBreach)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Cancel()
	var breaches int64
	go func() {
		for range sub.Channel {
			atomic.AddInt64(&breaches, 1)
		}
	}()

	// A commit recorded as made OUTSIDE the locked single entrypoint (the §2
	// "entrypoint bypassed" breach). The §43 command body records the entrypoint
	// state; the binding classifies it. NO real git/commit/push.
	badCommit := constitution.Subject{Kind: bindings.SubjectCommitPush, ID: "abc1234|entrypoint=false|lock_held=false"}

	out, err := p.EvaluateSubject(ctx, "§2", tenant, badCommit)
	if err != nil {
		t.Fatalf("EvaluateSubject: %v", err)
	}
	if out.Decision != constitution.DecisionFail {
		t.Fatalf("decision = %v, want fail for an entrypoint-bypassed commit", out.Decision)
	}
	if !out.Transition.Changed || !out.Transition.FirstSeen {
		t.Fatalf("first project-result should be a FirstSeen+Changed transition, got %+v", out.Transition)
	}
	if out.Mode != constitution.ModeEnforce {
		t.Fatalf("mode = %v, want enforce (default §42.3)", out.Mode)
	}
	if !out.Emitted {
		t.Fatalf("enforce-mode transition must emit; out=%+v", out)
	}
	if !out.Audited {
		t.Fatalf("enforce-mode transition must write an audit row; out=%+v", out)
	}

	// (1) the emit reached the bus.
	deadline := time.After(2 * time.Second)
	for atomic.LoadInt64(&breaches) < 1 {
		select {
		case <-deadline:
			t.Fatalf("listener never saw the .repo.safety.breach; bus metrics=%+v", bus.Metrics())
		case <-time.After(5 * time.Millisecond):
		}
	}

	// (2) a constitution_state row is queryable.
	rows, err := st.List(ctx, tenant, constitution.ListQuery{RuleID: "§2"})
	if err != nil {
		t.Fatalf("store List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 persisted state row, got %d", len(rows))
	}
	if rows[0].Decision != constitution.DecisionFail {
		t.Errorf("persisted decision = %v, want fail", rows[0].Decision)
	}
	if rows[0].Subject != badCommit.ID {
		t.Errorf("persisted subject = %q, want %q", rows[0].Subject, badCommit.ID)
	}

	// (3) a constitution_audit row was written.
	audit, err := au.ListAudit(ctx, tenant, constitution.AuditQuery{RuleID: "§2"})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(audit) != 1 {
		t.Fatalf("expected exactly 1 audit row, got %d", len(audit))
	}
	if audit[0].ModeAtEmission != constitution.ModeEnforce {
		t.Errorf("audit mode = %v, want enforce", audit[0].ModeAtEmission)
	}

	// The wire event MUST be exactly .repo.safety.breach — NOT a generic
	// .policy.violation (a misrouted class would mis-signal repo-safety subscribers).
	published := bus.Metrics().PublishedByType[constitution.EventNamespace+"."+constitution.ClassRepoSafetyBreach]
	if published != 1 {
		t.Fatalf("bus published %d .repo.safety.breach events, want 1", published)
	}
}

// TestPipeline_RepoSafetyRecoveredOnPass proves a repo-safety gate that
// transitions back to PASS does NOT re-fire .repo.safety.breach — the recovery
// routes through the .gate.recovered companion (the safety classes have no
// distinct "recovered" leaf, so the Pipeline maps a safety recovery to
// .gate.recovered, mirroring the rherald release-recovery convention). A binding
// that emitted .repo.safety.breach on a recovery would mis-signal subscribers.
func TestPipeline_RepoSafetyRecoveredOnPass(t *testing.T) {
	ctx := context.Background()
	p, _, la, _, bus := newPipeline(t)
	tenant := uuid.New()
	if err := la.Set(ctx, tenant, "§11.4.71", constitution.ModeEnforce, "test"); err != nil {
		t.Fatalf("ladder Set: %v", err)
	}

	recSub, err := bus.Subscribe(constitution.EventNamespace + "." + constitution.ClassGateRecovered)
	if err != nil {
		t.Fatalf("Subscribe gate.recovered: %v", err)
	}
	defer recSub.Cancel()
	var recovered int64
	go func() {
		for range recSub.Channel {
			atomic.AddInt64(&recovered, 1)
		}
	}()

	// First a FAIL (push attempted without a pre-push fetch), then a PASS
	// (fetched + integrated) for the SAME push so the second is a recovery.
	bad := constitution.Subject{Kind: bindings.SubjectPrePush, ID: "main|fetched=false|integrated=false"}
	if _, err := p.EvaluateSubject(ctx, "§11.4.71", tenant, bad); err != nil {
		t.Fatalf("EvaluateSubject(fail): %v", err)
	}
	good := constitution.Subject{Kind: bindings.SubjectPrePush, ID: "main|fetched=true|integrated=true"}
	out, err := p.EvaluateSubject(ctx, "§11.4.71", tenant, good)
	if err != nil {
		t.Fatalf("EvaluateSubject(pass): %v", err)
	}
	if out.Decision != constitution.DecisionPass {
		t.Fatalf("decision = %v, want pass on a fetched+integrated pre-push", out.Decision)
	}
	if !out.Transition.Changed {
		t.Fatalf("fail→pass should be a Changed transition, got %+v", out.Transition)
	}
	if !out.Emitted {
		t.Fatalf("enforce-mode recovery must emit; out=%+v", out)
	}

	deadline := time.After(2 * time.Second)
	for atomic.LoadInt64(&recovered) < 1 {
		select {
		case <-deadline:
			t.Fatalf("listener never saw the .gate.recovered; bus metrics=%+v", bus.Metrics())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestPipeline_ReopensPolicyViolation proves the §11.4.55 reopens-history row
// routes through .policy.violation (NOT a repo-safety class — reopens-history is
// a warn-mode policy concern). A reopen that lacks its docs/Reopens/<HRD>.md
// record is flagged as a policy violation.
func TestPipeline_ReopensPolicyViolation(t *testing.T) {
	ctx := context.Background()
	p, _, la, au, bus := newPipeline(t)
	tenant := uuid.New()
	// §11.4.55 is warn by default — flip to enforce so the emit fires (warn audits only).
	if err := la.Set(ctx, tenant, "§11.4.55", constitution.ModeEnforce, "test"); err != nil {
		t.Fatalf("ladder Set: %v", err)
	}

	sub, err := bus.Subscribe(constitution.EventNamespace + "." + constitution.ClassPolicyViolation)
	if err != nil {
		t.Fatalf("Subscribe policy.violation: %v", err)
	}
	defer sub.Cancel()
	var violations int64
	go func() {
		for range sub.Channel {
			atomic.AddInt64(&violations, 1)
		}
	}()

	// A reopen with no docs/Reopens record (the §11.4.55 history is missing).
	bad := constitution.Subject{Kind: bindings.SubjectReopen, ID: "HRD-099|recorded=false"}
	out, err := p.EvaluateSubject(ctx, "§11.4.55", tenant, bad)
	if err != nil {
		t.Fatalf("EvaluateSubject: %v", err)
	}
	if out.Decision != constitution.DecisionFail {
		t.Fatalf("decision = %v, want fail for an unrecorded reopen", out.Decision)
	}
	if !out.Emitted {
		t.Fatalf("enforce-mode transition must emit; out=%+v", out)
	}

	deadline := time.After(2 * time.Second)
	for atomic.LoadInt64(&violations) < 1 {
		select {
		case <-deadline:
			t.Fatalf("listener never saw the .policy.violation; bus metrics=%+v", bus.Metrics())
		case <-time.After(5 * time.Millisecond):
		}
	}

	// The policy emit must capture an event ID in the audit row (IDEmitter path).
	audit, err := au.ListAudit(ctx, tenant, constitution.AuditQuery{RuleID: "§11.4.55"})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(audit) != 1 {
		t.Fatalf("expected 1 audit row, got %d", len(audit))
	}
	if audit[0].EmittedEventID == uuid.Nil {
		t.Errorf("policy.violation audit row should capture the emitted event ID (IDEmitter), got Nil")
	}
}

// TestPipeline_LadderGatesEmit proves the binding decision is GATED by the mode
// ladder — an allow-mode rule records state but does NOT emit or audit (the
// §42.5-step "initial mode allow = data gathering only" contract).
func TestPipeline_LadderGatesEmit(t *testing.T) {
	ctx := context.Background()
	p, st, la, au, bus := newPipeline(t)
	tenant := uuid.New()

	if err := la.Set(ctx, tenant, "§2", constitution.ModeAllow, "test"); err != nil {
		t.Fatalf("ladder Set allow: %v", err)
	}

	before := bus.Metrics().Published
	bad := constitution.Subject{Kind: bindings.SubjectCommitPush, ID: "abc1234|entrypoint=false|lock_held=false"}
	out, err := p.EvaluateSubject(ctx, "§2", tenant, bad)
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
	rows, err := st.List(ctx, tenant, constitution.ListQuery{RuleID: "§2"})
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
	_, err := p.EvaluateSubject(ctx, "§does.not.exist", uuid.New(), constitution.Subject{Kind: "project", ID: "x"})
	if err == nil {
		t.Fatalf("expected error for unknown rule, got nil")
	}
}

// ---------------------------------------------------------------------------
// Bespoke detector tests — the 3 load-bearing pherald project classifiers.
// ---------------------------------------------------------------------------

// TestDetector_CommitPushDiscipline proves the §2 commit-push-discipline
// detector classifies a recorded commit: a commit made THROUGH the single locked
// entrypoint with the lock held PASSes; a commit made OUTSIDE the entrypoint (or
// without the lock) FAILs (the §2 "entrypoint bypassed" repo-safety breach).
// PURE: it reads the recorded commit state; it NEVER commits, pushes, or runs git.
func TestDetector_CommitPushDiscipline(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()

	cases := []struct {
		id   string
		want constitution.Decision
	}{
		{"abc1234|entrypoint=true|lock_held=true", constitution.DecisionPass},   // through the locked entrypoint
		{"abc1234|entrypoint=false|lock_held=false", constitution.DecisionFail}, // entrypoint bypassed
		{"abc1234|entrypoint=true|lock_held=false", constitution.DecisionFail},  // entrypoint used but no lock held
		{"abc1234|entrypoint=false|lock_held=true", constitution.DecisionFail},  // lock without the entrypoint
		{"abc1234", constitution.DecisionFail},                                  // no entrypoint evidence → refuse to silent-PASS
	}
	for _, c := range cases {
		subj := constitution.Subject{Kind: bindings.SubjectCommitPush, ID: c.id}
		out, err := p.EvaluateSubject(ctx, "§2", tenant, subj)
		if err != nil {
			t.Fatalf("EvaluateSubject(%s): %v", c.id, err)
		}
		if out.Decision != c.want {
			t.Errorf("commit-push %q classified as %v, want %v", c.id, out.Decision, c.want)
		}
	}
}

// TestDetector_SubmodulePropagationOrder proves the §3 submodule-propagation
// detector: a propagation that committed inner-submodules-first then parent
// (order=inner-first) PASSes; a propagation that committed the parent before the
// inner submodules (order=parent-first) FAILs (the §3 "wrong propagation order"
// violation — the parent would pin a not-yet-pushed inner SHA). PURE: it reads
// the recorded order; it NEVER commits or runs git submodule.
func TestDetector_SubmodulePropagationOrder(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()

	cases := []struct {
		id   string
		want constitution.Decision
	}{
		{"propagate|order=inner-first|inner_pushed=true", constitution.DecisionPass},  // correct §3 order
		{"propagate|order=parent-first|inner_pushed=true", constitution.DecisionFail}, // parent committed before inner
		{"propagate|order=inner-first|inner_pushed=false", constitution.DecisionFail}, // inner not pushed → parent pins a dangling SHA
		{"propagate", constitution.DecisionFail},                                      // no order evidence → refuse to silent-PASS
	}
	for _, c := range cases {
		subj := constitution.Subject{Kind: bindings.SubjectSubmodulePropagate, ID: c.id}
		out, err := p.EvaluateSubject(ctx, "§3", tenant, subj)
		if err != nil {
			t.Fatalf("EvaluateSubject(%s): %v", c.id, err)
		}
		if out.Decision != c.want {
			t.Errorf("submodule-propagate %q classified as %v, want %v", c.id, out.Decision, c.want)
		}
	}
}

// TestDetector_PrePushFetchGuard proves the §11.4.71 pre-push-fetch-guard
// detector: a push preceded by a fetch + integrate of incoming changes
// (fetched=true|integrated=true) PASSes; a push attempted WITHOUT the pre-push
// fetch (fetched=false) FAILs; a fetch done but incoming changes NOT integrated
// (fetched=true|integrated=false) FAILs (the §11.4.71 "skipped pre-push
// investigate+integrate" repo-safety breach). PURE: it reads the recorded
// pre-push state; it NEVER fetches or pushes.
func TestDetector_PrePushFetchGuard(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()

	cases := []struct {
		id   string
		want constitution.Decision
	}{
		{"main|fetched=true|integrated=true", constitution.DecisionPass},   // fetched + integrated
		{"main|fetched=false|integrated=false", constitution.DecisionFail}, // no pre-push fetch
		{"main|fetched=true|integrated=false", constitution.DecisionFail},  // fetched but incoming not integrated
		{"main", constitution.DecisionFail},                                // no pre-push evidence → refuse to silent-PASS
	}
	for _, c := range cases {
		subj := constitution.Subject{Kind: bindings.SubjectPrePush, ID: c.id}
		out, err := p.EvaluateSubject(ctx, "§11.4.71", tenant, subj)
		if err != nil {
			t.Fatalf("EvaluateSubject(%s): %v", c.id, err)
		}
		if out.Decision != c.want {
			t.Errorf("pre-push %q classified as %v, want %v", c.id, out.Decision, c.want)
		}
	}
}

// TestDetector_FetchBeforeEdit proves the §11.4.37 fetch-before-edit detector: an
// edit made on a tree rebased on origin (rebased=true) PASSes; an edit made on a
// stale tree (rebased=false) FAILs (the §11.4.37 "edit on a stale tree"
// repo-safety breach). PURE: it reads the recorded rebase state; it NEVER fetches.
func TestDetector_FetchBeforeEdit(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()

	cases := []struct {
		id   string
		want constitution.Decision
	}{
		{"main|rebased=true", constitution.DecisionPass},
		{"main|rebased=false", constitution.DecisionFail},
		{"main", constitution.DecisionFail}, // no rebase evidence → refuse to silent-PASS
	}
	for _, c := range cases {
		subj := constitution.Subject{Kind: bindings.SubjectFetchGuard, ID: c.id}
		out, err := p.EvaluateSubject(ctx, "§11.4.37", tenant, subj)
		if err != nil {
			t.Fatalf("EvaluateSubject(%s): %v", c.id, err)
		}
		if out.Decision != c.want {
			t.Errorf("fetch-guard %q classified as %v, want %v", c.id, out.Decision, c.want)
		}
	}
}

// TestPipeline_MalformedSubjectDoesNotPass proves a project-result subject with
// no recognizable outcome MUST NOT silently PASS (§11.4.1 fail-bluff inverse).
func TestPipeline_MalformedSubjectDoesNotPass(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()
	subj := constitution.Subject{Kind: bindings.SubjectCommitPush, ID: "garbage-no-fields"}
	out, err := p.EvaluateSubject(ctx, "§2", tenant, subj)
	if err != nil {
		t.Fatalf("EvaluateSubject: %v", err)
	}
	if out.Decision == constitution.DecisionPass {
		t.Errorf("malformed commit-push (no fields) classified as PASS — that is a §11.4.1 fail-bluff inverse")
	}
}
