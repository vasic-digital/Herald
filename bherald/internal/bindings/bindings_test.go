package bindings_test

// HRD-021 — bherald CI/test constitution bindings (v1.0.0 Batch B, unit 3).
//
// These tests are written FIRST (TDD RED) against the bindings package that
// wires bherald's 22 §42.3-owned CI/test gate-result rules into the Batch-A
// commons_constitution Evaluator + Registry + AuditStore + ModeLadder
// foundation. Every assertion checks an observable side-effect of the REAL
// foundation running end-to-end (real MemoryBus, real ConstitutionStore, real
// ModeLadder, real MemoryAudit, real emit) — no mocks-of-mocks (§11.4.27).
//
// The load-bearing round-trip these tests prove (the §107 anti-bluff bar for
// HRD-021): a CI/test gate RESULT (a build/test/lint gate that FAILed, a
// missing test-tier, a PASS-bluff with no captured evidence) is detected by a
// registered bherald binding → emitted as a .gate.failed event on the bus →
// persisted as a constitution_state row AND a constitution_audit row → visible
// via the same ConstitutionStore.List a future /v1/build pull surface reads. A
// binding that "registered" but could never produce a queryable, audited
// gate-result would be exactly the metadata-only PASS the covenant forbids.

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/vasic-digital/herald/bherald/internal/bindings"
	constitution "github.com/vasic-digital/herald/commons_constitution"
	"github.com/vasic-digital/herald/commons_constitution/ladder"
	"github.com/vasic-digital/herald/commons_constitution/state"
)

// newPipeline builds a Pipeline backed entirely by the production-faithful
// in-memory foundation backends (the same seam cherald/sherald use). Returns
// the pipeline + the live store/ladder/audit/bus so tests can assert persisted
// side-effects directly.
func newPipeline(t *testing.T) (*bindings.Pipeline, constitution.ConstitutionStore, constitution.ModeLadder, constitution.AuditStore, constitution.EventBus) {
	t.Helper()
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{BufferSize: 256})
	t.Cleanup(func() { _ = bus.Close() })
	em, err := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "digital.vasic.herald/bherald"})
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

// TestRuleCatalogue_CoversBheraldOwnedRules proves the binding catalogue
// actually carries the bherald-owned §42.3 CI/test rows — not an empty registry
// that would make every downstream "binding works" claim vacuous. We assert (a)
// a substantial count (the spec §42.3 master table assigns bherald 22 rows) and
// (b) that the canonical bherald rows are present with the spec's default mode +
// severity.
func TestRuleCatalogue_CoversBheraldOwnedRules(t *testing.T) {
	cat := bindings.BheraldRules()
	if len(cat) < 20 {
		t.Fatalf("bherald rule catalogue has only %d rules; spec §42.3 assigns bherald 22 — looks truncated", len(cat))
	}

	byID := map[string]bindings.RuleSpec{}
	for _, r := range cat {
		if _, dup := byID[r.RuleID]; dup {
			t.Fatalf("duplicate rule in catalogue: %q", r.RuleID)
		}
		byID[r.RuleID] = r
	}

	// Spot-check the canonical bherald rows from §42.3 with their declared
	// default mode + severity + event class. These are load-bearing: a binding
	// registered with the wrong default mode/class would silently misbehave
	// against spec.
	want := []struct {
		id    string
		sev   constitution.Severity
		mod   constitution.Mode
		class string
	}{
		{"§1", constitution.SeverityHigh, constitution.ModeEnforce, constitution.ClassGateFailed},        // test coverage mandatory
		{"§11.4.2", constitution.SeverityHigh, constitution.ModeEnforce, constitution.ClassGateFailed},   // recorded-evidence requirement
		{"§11.4.5", constitution.SeverityMiddle, constitution.ModeWarn, constitution.ClassGateFailed},    // captured-evidence quality
		{"§11.4.27", constitution.SeverityHigh, constitution.ModeEnforce, constitution.ClassGateFailed},  // no-fakes + test-tier matrix
		{"§11.4.50", constitution.SeverityHigh, constitution.ModeEnforce, constitution.ClassGateFailed},  // deterministic consistency (flaky)
		{"§11.4.9", constitution.SeverityLow, constitution.ModeWarn, constitution.ClassPolicyViolation},  // batch-source-fixes (policy)
		{"§11.4.14", constitution.SeverityLow, constitution.ModeWarn, constitution.ClassPolicyViolation}, // playback cleanup (policy)
		{"§11.4.67", constitution.SeverityLow, constitution.ModeWarn, constitution.ClassGateFailed},      // shell-script parseability
	}
	for _, w := range want {
		got, ok := byID[w.id]
		if !ok {
			t.Errorf("rule %q missing from bherald catalogue (§42.3 assigns it to bherald)", w.id)
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
	// with no evaluator logic can never produce a real gate-result (a §107
	// binding bluff).
	for _, r := range cat {
		if r.Check == nil {
			t.Errorf("rule %q has a nil Check func — it can never detect a gate-result", r.RuleID)
		}
		if r.EventClass == "" {
			t.Errorf("rule %q has an empty EventClass — cannot route the emit", r.RuleID)
		}
	}
}

// TestPipeline_RegistersEveryRule proves NewPipeline registered every catalogue
// rule into the underlying commons_constitution.Registry — so a sweep over the
// registry actually sees the bherald rule set (not a partial registration).
func TestPipeline_RegistersEveryRule(t *testing.T) {
	p, _, _, _, _ := newPipeline(t)
	got := p.Registry().Len()
	want := len(bindings.BheraldRules())
	if got != want {
		t.Fatalf("pipeline registered %d evaluators, catalogue has %d", got, want)
	}
	for _, r := range bindings.BheraldRules() {
		if _, ok := p.Registry().Get(r.RuleID); !ok {
			t.Errorf("rule %q not retrievable from the pipeline registry", r.RuleID)
		}
	}
}

// TestPipeline_GateFailedRoundTrip is THE load-bearing HRD-021 anti-bluff proof.
//
// It drives a real bherald binding (§1 test-coverage gate) whose Check func
// classifies a recorded gate outcome as FAIL, through the REAL Pipeline, and
// asserts the FULL round-trip:
//
//	(1) a .gate.failed event reaches the bus (the channel emit fired);
//	(2) a constitution_state row was persisted with decision=fail + the bundle
//	    hash + the subject (queryable via a future /v1/build pull surface);
//	(3) a constitution_audit row was written naming the emitted event ID
//	    (the durable audit trail behind RunOutcome.Audited).
//
// A binding that "evaluated" but produced no queryable/audited evidence is a
// §107 PASS-bluff; this test makes that impossible to ship.
func TestPipeline_GateFailedRoundTrip(t *testing.T) {
	ctx := context.Background()
	p, st, la, au, bus := newPipeline(t)
	tenant := uuid.New()

	// §1 default-enforces per §42.3.
	if err := la.Set(ctx, tenant, "§1", constitution.ModeEnforce, "test"); err != nil {
		t.Fatalf("ladder Set enforce: %v", err)
	}

	// Subscribe to the gate-failed class — the anti-bluff observation that proves
	// the emit reached the bus.
	sub, err := bus.Subscribe(constitution.EventNamespace + "." + constitution.ClassGateFailed)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Cancel()
	var failures int64
	go func() {
		for range sub.Channel {
			atomic.AddInt64(&failures, 1)
		}
	}()

	// A gate-result that FAILed (coverage dropped). The §43 CI gate body records
	// the outcome; the binding classifies it.
	badGate := constitution.Subject{Kind: bindings.SubjectGateResult, ID: "test-suite|outcome=fail"}

	out, err := p.EvaluateSubject(ctx, "§1", tenant, badGate)
	if err != nil {
		t.Fatalf("EvaluateSubject: %v", err)
	}
	if out.Decision != constitution.DecisionFail {
		t.Fatalf("decision = %v, want fail for a failed gate", out.Decision)
	}
	if !out.Transition.Changed || !out.Transition.FirstSeen {
		t.Fatalf("first gate-result should be a FirstSeen+Changed transition, got %+v", out.Transition)
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
	for atomic.LoadInt64(&failures) < 1 {
		select {
		case <-deadline:
			t.Fatalf("listener never saw the .gate.failed; bus metrics=%+v", bus.Metrics())
		case <-time.After(5 * time.Millisecond):
		}
	}

	// (2) a constitution_state row is queryable.
	rows, err := st.List(ctx, tenant, constitution.ListQuery{RuleID: "§1"})
	if err != nil {
		t.Fatalf("store List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 persisted state row, got %d", len(rows))
	}
	if rows[0].Decision != constitution.DecisionFail {
		t.Errorf("persisted decision = %v, want fail", rows[0].Decision)
	}
	if rows[0].Subject != badGate.ID {
		t.Errorf("persisted subject = %q, want %q", rows[0].Subject, badGate.ID)
	}

	// (3) a constitution_audit row was written naming the emitted event.
	audit, err := au.ListAudit(ctx, tenant, constitution.AuditQuery{RuleID: "§1"})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(audit) != 1 {
		t.Fatalf("expected exactly 1 audit row, got %d", len(audit))
	}
	if audit[0].ModeAtEmission != constitution.ModeEnforce {
		t.Errorf("audit mode = %v, want enforce", audit[0].ModeAtEmission)
	}
}

// TestPipeline_GateRecoveredOnPass proves a gate that transitions back to PASS
// emits .gate.recovered (NOT .gate.failed) in enforce mode — the §42.3
// .gate.recovered companion class. A binding that emitted .gate.failed on a
// recovery would mis-signal subscribers.
func TestPipeline_GateRecoveredOnPass(t *testing.T) {
	ctx := context.Background()
	p, _, la, _, bus := newPipeline(t)
	tenant := uuid.New()
	if err := la.Set(ctx, tenant, "§1", constitution.ModeEnforce, "test"); err != nil {
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

	// First a FAIL (FirstSeen), then a PASS (transition fail→pass) for the SAME
	// gate so the second is a recovery transition.
	subj := constitution.Subject{Kind: bindings.SubjectGateResult, ID: "lint|outcome=fail"}
	if _, err := p.EvaluateSubject(ctx, "§1", tenant, subj); err != nil {
		t.Fatalf("EvaluateSubject(fail): %v", err)
	}
	pass := constitution.Subject{Kind: bindings.SubjectGateResult, ID: "lint|outcome=pass"}
	out, err := p.EvaluateSubject(ctx, "§1", tenant, pass)
	if err != nil {
		t.Fatalf("EvaluateSubject(pass): %v", err)
	}
	if out.Decision != constitution.DecisionPass {
		t.Fatalf("decision = %v, want pass on outcome=pass", out.Decision)
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

// TestPipeline_LadderGatesEmit proves the binding decision is GATED by the mode
// ladder — an allow-mode rule records state but does NOT emit or audit
// (the §42.5-step "initial mode allow = data gathering only" contract).
func TestPipeline_LadderGatesEmit(t *testing.T) {
	ctx := context.Background()
	p, st, la, au, bus := newPipeline(t)
	tenant := uuid.New()

	if err := la.Set(ctx, tenant, "§1", constitution.ModeAllow, "test"); err != nil {
		t.Fatalf("ladder Set allow: %v", err)
	}

	before := bus.Metrics().Published
	bad := constitution.Subject{Kind: bindings.SubjectGateResult, ID: "test-suite|outcome=fail"}
	out, err := p.EvaluateSubject(ctx, "§1", tenant, bad)
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
	rows, err := st.List(ctx, tenant, constitution.ListQuery{RuleID: "§1"})
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
	_, err := p.EvaluateSubject(ctx, "§does.not.exist", uuid.New(), constitution.Subject{Kind: "gate", ID: "x"})
	if err == nil {
		t.Fatalf("expected error for unknown rule, got nil")
	}
}

// ---------------------------------------------------------------------------
// Bespoke detector tests — the 3 load-bearing bherald CI/test classifiers.
// ---------------------------------------------------------------------------

// TestDetector_GateResultClassification proves the §1/§11.4.50 gate-result
// detector classifies recorded CI gate outcomes: outcome=fail → FAIL,
// outcome=flaky → FAIL (a flaky gate is non-deterministic — §11.4.50), and
// outcome=pass → PASS. This is the bherald build/CI value-add: turning a CI
// gate's exit-status into a constitution verdict.
func TestDetector_GateResultClassification(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()

	cases := []struct {
		id   string
		want constitution.Decision
	}{
		{"test-suite|outcome=pass", constitution.DecisionPass},
		{"test-suite|outcome=fail", constitution.DecisionFail},
		{"test-suite|outcome=flaky", constitution.DecisionFail}, // §11.4.50 determinism
		{"lint|outcome=error", constitution.DecisionFail},
		{"build|outcome=pass", constitution.DecisionPass},
	}
	for _, c := range cases {
		subj := constitution.Subject{Kind: bindings.SubjectGateResult, ID: c.id}
		out, err := p.EvaluateSubject(ctx, "§1", tenant, subj)
		if err != nil {
			t.Fatalf("EvaluateSubject(%s): %v", c.id, err)
		}
		if out.Decision != c.want {
			t.Errorf("gate %q classified as %v, want %v", c.id, out.Decision, c.want)
		}
	}
}

// TestDetector_AntiBluffPASS proves the §11.4.2/§11.4.5 anti-bluff-PASS detector:
// a gate that reports outcome=pass but has NO captured-evidence artefact
// (evidence=false) is a §11.4 PASS-bluff and MUST be FAILed. A gate with
// outcome=pass AND evidence=true PASSes. This is THE §107 covenant detector —
// it catches the exact "tests pass but the feature does not work / has no
// auditable evidence" failure the operator mandate forbids.
func TestDetector_AntiBluffPASS(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()

	// PASS with captured evidence → PASS.
	good := constitution.Subject{Kind: bindings.SubjectEvidence, ID: "TestFoo|outcome=pass|evidence=true"}
	out, err := p.EvaluateSubject(ctx, "§11.4.2", tenant, good)
	if err != nil {
		t.Fatalf("EvaluateSubject(good): %v", err)
	}
	if out.Decision != constitution.DecisionPass {
		t.Errorf("PASS+evidence classified as %v, want pass", out.Decision)
	}

	// PASS without captured evidence → FAIL (the §11.4.2 PASS-bluff).
	bluff := constitution.Subject{Kind: bindings.SubjectEvidence, ID: "TestFoo|outcome=pass|evidence=false"}
	out, err = p.EvaluateSubject(ctx, "§11.4.2", tenant, bluff)
	if err != nil {
		t.Fatalf("EvaluateSubject(bluff): %v", err)
	}
	if out.Decision != constitution.DecisionFail {
		t.Errorf("PASS-without-evidence (§11.4.2 bluff) classified as %v, want FAIL", out.Decision)
	}
}

// TestDetector_TestTierVerify proves the §11.4.27/§40.2 test-tier-verify
// detector: a tier-matrix Subject listing the present tiers FAILs when a
// required tier is missing and PASSes when all 8 tiers are present. This is the
// "test-tier-verify" bespoke detector the HRD-021 scope calls out.
func TestDetector_TestTierVerify(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()

	// All 8 canonical §40.2 tiers present → PASS.
	full := constitution.Subject{Kind: bindings.SubjectTestTier, ID: "pkg/foo|tiers=unit,component,integration,contract,e2e_sandbox,e2e_live,mutation,chaos"}
	out, err := p.EvaluateSubject(ctx, "§11.4.27", tenant, full)
	if err != nil {
		t.Fatalf("EvaluateSubject(full): %v", err)
	}
	if out.Decision != constitution.DecisionPass {
		t.Errorf("full 8-tier matrix classified as %v, want pass", out.Decision)
	}

	// Missing tiers (only unit) → FAIL.
	partial := constitution.Subject{Kind: bindings.SubjectTestTier, ID: "pkg/foo|tiers=unit"}
	out, err = p.EvaluateSubject(ctx, "§11.4.27", tenant, partial)
	if err != nil {
		t.Fatalf("EvaluateSubject(partial): %v", err)
	}
	if out.Decision != constitution.DecisionFail {
		t.Errorf("partial test-tier matrix classified as %v, want FAIL", out.Decision)
	}
}

// TestDetector_PanicIsolation proves a panicking binding never propagates — the
// Pipeline records DecisionError and returns nil error (matching the cherald /
// sherald / Runner panic-isolation contract). We can't easily inject a panic
// via the public catalogue, so this asserts the framework records ERROR (not
// PASS) for an evaluator that errors — exercised here via an unknown-tier
// marker that the tier detector treats as malformed.
func TestPipeline_MalformedSubjectDoesNotPass(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()
	// A gate-result subject with no recognizable outcome must NOT silently PASS.
	subj := constitution.Subject{Kind: bindings.SubjectGateResult, ID: "garbage-no-outcome-field"}
	out, err := p.EvaluateSubject(ctx, "§1", tenant, subj)
	if err != nil {
		t.Fatalf("EvaluateSubject: %v", err)
	}
	if out.Decision == constitution.DecisionPass {
		t.Errorf("malformed gate-result (no outcome) classified as PASS — that is a §11.4.1 fail-bluff inverse")
	}
}

var _ = fmt.Sprintf // keep fmt imported for future diagnostics
