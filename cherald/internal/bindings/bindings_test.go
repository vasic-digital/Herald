package bindings_test

// HRD-019 — cherald constitution bindings (Batch B, unit 1).
//
// These tests are written FIRST (TDD RED) against the bindings package that
// wires cherald's ~32 §42.3-owned constitution rules into the Batch-A
// commons_constitution Evaluator + Runner + AuditStore + ModeLadder
// foundation. Every assertion checks an observable side-effect of the REAL
// foundation running end-to-end (real MemoryBus, real ConstitutionStore, real
// ModeLadder, real MemoryAudit, real Runner) — no mocks-of-mocks (§11.4.27).
//
// The load-bearing round-trip these tests prove (the §107 anti-bluff bar for
// HRD-019): a constitution RULE VIOLATION is detected by a registered cherald
// binding → emitted as a .policy.violation event on the bus → persisted as a
// constitution_state row AND a constitution_audit row → visible via the same
// ConstitutionStore.List the /v1/compliance pull surface reads. A binding that
// "registered" but could never produce a queryable, audited violation would be
// exactly the metadata-only PASS the covenant forbids.

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/vasic-digital/herald/cherald/internal/bindings"
	constitution "github.com/vasic-digital/herald/commons_constitution"
	"github.com/vasic-digital/herald/commons_constitution/ladder"
	"github.com/vasic-digital/herald/commons_constitution/state"
)

// newPipeline builds a Pipeline backed entirely by the production-faithful
// in-memory foundation backends (the same seam integration_test.go uses for
// the M1 smoke). Returns the pipeline + the live store/ladder/audit/bus so
// tests can assert persisted side-effects directly.
func newPipeline(t *testing.T) (*bindings.Pipeline, constitution.ConstitutionStore, constitution.ModeLadder, constitution.AuditStore, constitution.EventBus) {
	t.Helper()
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{BufferSize: 256})
	t.Cleanup(func() { _ = bus.Close() })
	em, err := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "digital.vasic.herald/cherald"})
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

// TestRuleCatalogue_CoversCheraldOwnedRules proves the binding catalogue
// actually carries the cherald-owned §42.3 rules — not an empty registry that
// would make every downstream "binding works" claim vacuous. We assert (a) a
// substantial count (the spec says cherald gets "the bulk" / ~30 rules) and
// (b) that the canonical cherald-only rules are present with the spec's
// default mode + severity.
func TestRuleCatalogue_CoversCheraldOwnedRules(t *testing.T) {
	cat := bindings.CheraldRules()
	if len(cat) < 25 {
		t.Fatalf("cherald rule catalogue has only %d rules; spec §42.3 assigns cherald ~30 — looks truncated", len(cat))
	}

	byID := map[string]bindings.RuleSpec{}
	for _, r := range cat {
		if _, dup := byID[r.RuleID]; dup {
			t.Fatalf("duplicate rule in catalogue: %q", r.RuleID)
		}
		byID[r.RuleID] = r
	}

	// Spot-check the canonical cherald rows from §42.3 with their declared
	// default mode + severity. These are load-bearing: a binding registered
	// with the wrong default mode would silently allow/enforce against spec.
	want := []struct {
		id  string
		sev constitution.Severity
		mod constitution.Mode
	}{
		{"§7.1", constitution.SeverityCritical, constitution.ModeEnforce},     // NO-BLUFF positive-evidence
		{"§11.4.1", constitution.SeverityCritical, constitution.ModeEnforce},  // FAIL-bluffs forbidden
		{"§11.4.10", constitution.SeverityCritical, constitution.ModeEnforce}, // credentials-handling (.credential.leak)
		{"§11.4.29", constitution.SeverityLow, constitution.ModeWarn},         // lowercase_snake_case naming
		{"§11.4.44", constitution.SeverityLow, constitution.ModeWarn},         // doc revision header
		{"§11.4.73", constitution.SeverityHigh, constitution.ModeEnforce},     // spec revision drift
		{"§11.4.74", constitution.SeverityHigh, constitution.ModeEnforce},     // catalogue-miss
		{"§11.4.60", constitution.SeverityHigh, constitution.ModeEnforce},     // documentation composite covenant
		{"§12.10", constitution.SeverityHigh, constitution.ModeEnforce},       // CONTINUATION sacred invariant
	}
	for _, w := range want {
		got, ok := byID[w.id]
		if !ok {
			t.Errorf("rule %q missing from cherald catalogue (§42.3 assigns it to cherald)", w.id)
			continue
		}
		if got.Severity != w.sev {
			t.Errorf("rule %q severity = %v, want %v", w.id, got.Severity, w.sev)
		}
		if got.DefaultMode != w.mod {
			t.Errorf("rule %q default mode = %v, want %v", w.id, got.DefaultMode, w.mod)
		}
	}

	// Every rule MUST carry a non-nil Check func — a rule with no evaluator
	// logic can never produce a real violation (a §107 binding bluff).
	for _, r := range cat {
		if r.Check == nil {
			t.Errorf("rule %q has a nil Check func — it can never detect a violation", r.RuleID)
		}
		if r.EventClass == "" {
			t.Errorf("rule %q has an empty EventClass — cannot route the emit", r.RuleID)
		}
	}
}

// TestPipeline_RegistersEveryRule proves NewPipeline registered every catalogue
// rule into the underlying commons_constitution.Registry — so a sweep over the
// registry actually sees the cherald rule set (not a partial registration).
func TestPipeline_RegistersEveryRule(t *testing.T) {
	p, _, _, _, _ := newPipeline(t)
	got := p.Registry().Len()
	want := len(bindings.CheraldRules())
	if got != want {
		t.Fatalf("pipeline registered %d evaluators, catalogue has %d", got, want)
	}
	// And each one is retrievable by RuleID.
	for _, r := range bindings.CheraldRules() {
		if _, ok := p.Registry().Get(r.RuleID); !ok {
			t.Errorf("rule %q not retrievable from the pipeline registry", r.RuleID)
		}
	}
}

// TestPipeline_ViolationRoundTrip is THE load-bearing HRD-019 anti-bluff proof.
//
// It drives a real cherald binding (§11.4.29 lowercase_snake_case naming) whose
// Check func returns a FAIL for a deliberately bad subject, through the REAL
// Runner, and asserts the FULL round-trip:
//
//	(1) a .policy.violation event reaches the bus (the channel emit fired);
//	(2) a constitution_state row was persisted with decision=fail + the bundle
//	    hash + the subject (queryable via the /v1/compliance pull surface);
//	(3) a constitution_audit row was written naming the emitted event ID
//	    (the durable audit trail behind RunOutcome.Audited).
//
// A binding that "evaluated" but produced no queryable/audited evidence is a
// §107 PASS-bluff; this test makes that impossible to ship.
func TestPipeline_ViolationRoundTrip(t *testing.T) {
	ctx := context.Background()
	p, st, la, au, bus := newPipeline(t)
	tenant := uuid.New()

	// §11.4.29 default-enforces (it's a low-severity warn rule per §42.3, so
	// flip it to enforce for this test so the emit + audit fire deterministically;
	// this also exercises the ladder gate the binding consults).
	if err := la.Set(ctx, tenant, "§11.4.29", constitution.ModeEnforce, "test"); err != nil {
		t.Fatalf("ladder Set enforce: %v", err)
	}

	// Subscribe to the policy-violation class — the anti-bluff observation that
	// proves the emit reached the bus.
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

	// A subject that VIOLATES §11.4.29 (uppercase / camelCase path component).
	badSubject := constitution.Subject{Kind: "file", ID: "commons_messaging/BadName.go"}

	out, err := p.EvaluateSubject(ctx, "§11.4.29", tenant, badSubject)
	if err != nil {
		t.Fatalf("EvaluateSubject: %v", err)
	}
	if out.Decision != constitution.DecisionFail {
		t.Fatalf("decision = %v, want fail for a snake_case violation", out.Decision)
	}
	if !out.Transition.Changed || !out.Transition.FirstSeen {
		t.Fatalf("first violation should be a FirstSeen+Changed transition, got %+v", out.Transition)
	}
	if out.Mode != constitution.ModeEnforce {
		t.Fatalf("mode = %v, want enforce (we Set it)", out.Mode)
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

	// (2) a constitution_state row is queryable (this is what /v1/compliance reads).
	rows, err := st.List(ctx, tenant, constitution.ListQuery{RuleID: "§11.4.29"})
	if err != nil {
		t.Fatalf("store List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 persisted state row, got %d", len(rows))
	}
	if rows[0].Decision != constitution.DecisionFail {
		t.Errorf("persisted decision = %v, want fail", rows[0].Decision)
	}
	if rows[0].Subject != badSubject.ID {
		t.Errorf("persisted subject = %q, want %q", rows[0].Subject, badSubject.ID)
	}

	// (3) a constitution_audit row was written naming the emitted event.
	audit, err := au.ListAudit(ctx, tenant, constitution.AuditQuery{RuleID: "§11.4.29"})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(audit) != 1 {
		t.Fatalf("expected exactly 1 audit row, got %d", len(audit))
	}
	if audit[0].EmittedEventID == uuid.Nil {
		t.Errorf("enforce-mode audit row must carry the emitted event ID, got Nil")
	}
	if audit[0].ModeAtEmission != constitution.ModeEnforce {
		t.Errorf("audit mode = %v, want enforce", audit[0].ModeAtEmission)
	}
}

// TestPipeline_LadderGatesEmit proves the binding decision is actually GATED by
// the mode ladder — an allow-mode rule records state but does NOT emit or audit.
// This is the §42.5-step-2 "initial mode allow = data gathering only" contract.
func TestPipeline_LadderGatesEmit(t *testing.T) {
	ctx := context.Background()
	p, st, la, au, bus := newPipeline(t)
	tenant := uuid.New()

	if err := la.Set(ctx, tenant, "§11.4.29", constitution.ModeAllow, "test"); err != nil {
		t.Fatalf("ladder Set allow: %v", err)
	}

	before := bus.Metrics().Published
	bad := constitution.Subject{Kind: "file", ID: "commons_messaging/BadName.go"}
	out, err := p.EvaluateSubject(ctx, "§11.4.29", tenant, bad)
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
	// State IS still recorded (data-gathering).
	rows, err := st.List(ctx, tenant, constitution.ListQuery{RuleID: "§11.4.29"})
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

// TestPipeline_CleanSubjectPasses proves a compliant subject produces a PASS
// (no false-positive violation). §11.4.1 anti-FAIL-bluff at the binding layer:
// the binding must not flag a conforming subject.
func TestPipeline_CleanSubjectPasses(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()

	good := constitution.Subject{Kind: "file", ID: "commons_messaging/good_name.go"}
	out, err := p.EvaluateSubject(ctx, "§11.4.29", tenant, good)
	if err != nil {
		t.Fatalf("EvaluateSubject: %v", err)
	}
	if out.Decision != constitution.DecisionPass {
		t.Fatalf("compliant snake_case file flagged as %v (false positive)", out.Decision)
	}
}

// TestPipeline_UnknownRule errors loudly rather than silently passing — a typo'd
// rule ID must not be swallowed (§11.4.6 no-guessing at the API layer).
func TestPipeline_UnknownRule(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	_, err := p.EvaluateSubject(ctx, "§does.not.exist", uuid.New(), constitution.Subject{Kind: "file", ID: "x"})
	if err == nil {
		t.Fatalf("expected error for unknown rule, got nil")
	}
}

// TestPipeline_CredentialLeakClass proves the §11.4.10 binding routes through
// the .credential.leak class (not generic .policy.violation) — the spec §42.3
// row binds §11.4.10 to .credential.leak, which fans out to iherald too.
func TestPipeline_CredentialLeakClass(t *testing.T) {
	ctx := context.Background()
	p, _, la, _, bus := newPipeline(t)
	tenant := uuid.New()
	if err := la.Set(ctx, tenant, "§11.4.10", constitution.ModeEnforce, "test"); err != nil {
		t.Fatalf("ladder Set: %v", err)
	}

	leakSub, err := bus.Subscribe(constitution.EventNamespace + "." + constitution.ClassCredentialLeak)
	if err != nil {
		t.Fatalf("Subscribe credential.leak: %v", err)
	}
	defer leakSub.Cancel()
	var leaks int64
	go func() {
		for range leakSub.Channel {
			atomic.AddInt64(&leaks, 1)
		}
	}()

	// A subject that looks like a tracked .env credential file.
	bad := constitution.Subject{Kind: "file", ID: ".env"}
	out, err := p.EvaluateSubject(ctx, "§11.4.10", tenant, bad)
	if err != nil {
		t.Fatalf("EvaluateSubject §11.4.10: %v", err)
	}
	if out.Decision != constitution.DecisionFail {
		t.Fatalf("tracked .env should FAIL §11.4.10, got %v", out.Decision)
	}
	if !out.Emitted {
		t.Fatalf("enforce-mode §11.4.10 violation must emit; out=%+v", out)
	}

	deadline := time.After(2 * time.Second)
	for atomic.LoadInt64(&leaks) < 1 {
		select {
		case <-deadline:
			t.Fatalf("no .credential.leak event observed; metrics=%+v", bus.Metrics())
		case <-time.After(5 * time.Millisecond):
		}
	}
}
