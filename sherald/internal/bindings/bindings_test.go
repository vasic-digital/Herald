package bindings_test

// HRD-020 — sherald host-safety + repo-safety constitution bindings
// (v1.0.0 Batch B, unit 2).
//
// These tests are written FIRST (TDD RED) against the bindings package that
// wires sherald's §42.3-owned host/repo-safety constitution rules into the
// Batch-A commons_constitution foundation (Evaluator + Registry + ModeLadder +
// ConstitutionStore + AuditStore + the typed safety-event emitter). They mirror
// the PROVEN cherald HRD-019 pattern (cherald/internal/bindings), specialised
// for sherald's SAFETY domain: the emit routes through the .host.safety.breach
// / .repo.safety.breach / .gate.recovered / .gate.failed / .bundle.updated /
// .bundle.update.failed / .policy.violation classes (cherald never touched the
// safety classes — they are sherald's responsibility per §42.3).
//
// Every assertion checks an observable side-effect of the REAL foundation
// running end-to-end (real MemoryBus, real ConstitutionStore, real ModeLadder,
// real MemoryAudit) — no mocks-of-mocks (§11.4.27).
//
// The load-bearing §107 anti-bluff round-trip these tests prove: a SAFETY rule
// violation (a destructive op detected, a force-push without merge-first, a
// 60%-mem-budget breach) is detected by a registered sherald binding → emitted
// as the rule's safety event class on the bus → persisted as a
// constitution_state row AND a constitution_audit row → queryable via the same
// ConstitutionStore.List a future /v1/safety/* surface reads. A binding that
// "registered" but could never produce a queryable, audited breach would be the
// metadata-only PASS-bluff the covenant forbids.
//
// §12 / §12.6 host-safety CRITICAL for this unit: the detection hooks DETECT +
// REPORT. They NEVER perform a destructive op or breach §12 themselves. Every
// test below feeds a SIMULATED breach signal (a Subject describing an attempted
// op) — no real rm / reset / force-push / suspend / mem-exhaustion happens.

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	constitution "github.com/vasic-digital/herald/commons_constitution"
	"github.com/vasic-digital/herald/commons_constitution/ladder"
	"github.com/vasic-digital/herald/commons_constitution/state"
	"github.com/vasic-digital/herald/sherald/internal/bindings"
)

// newPipeline builds a sherald Pipeline backed entirely by the
// production-faithful in-memory foundation backends. Returns the pipeline + the
// live store/ladder/audit/bus so tests can assert persisted side-effects.
func newPipeline(t *testing.T) (*bindings.Pipeline, constitution.ConstitutionStore, constitution.ModeLadder, constitution.AuditStore, constitution.EventBus) {
	t.Helper()
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{BufferSize: 256})
	t.Cleanup(func() { _ = bus.Close() })
	em, err := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "digital.vasic.herald/sherald"})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}
	st := state.NewMemory()
	la := ladder.NewMemory()
	au := state.NewMemoryAudit()
	p, err := bindings.NewPipeline(bindings.Config{Ladder: la, Store: st, Emitter: em, Audit: au})
	if err != nil {
		t.Fatalf("NewPipeline: %v", err)
	}
	return p, st, la, au, bus
}

// subscribeClass returns an atomic counter that increments for every event of
// EventNamespace+"."+class observed on the bus.
func subscribeClass(t *testing.T, bus constitution.EventBus, class string) *int64 {
	t.Helper()
	sub, err := bus.Subscribe(constitution.EventNamespace + "." + class)
	if err != nil {
		t.Fatalf("Subscribe %s: %v", class, err)
	}
	t.Cleanup(sub.Cancel)
	var n int64
	go func() {
		for range sub.Channel {
			atomic.AddInt64(&n, 1)
		}
	}()
	return &n
}

func waitCount(t *testing.T, n *int64, want int64, what string, bus constitution.EventBus) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for atomic.LoadInt64(n) < want {
		select {
		case <-deadline:
			t.Fatalf("listener never saw %d %s events (got %d); bus metrics=%+v", want, what, atomic.LoadInt64(n), bus.Metrics())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestRuleCatalogue_CoversSheraldSafetyRules proves the binding catalogue
// carries the sherald-owned §42.3 host/repo-safety rules — not an empty
// registry that would make every downstream "binding works" claim vacuous.
func TestRuleCatalogue_CoversSheraldSafetyRules(t *testing.T) {
	cat := bindings.SheraldRules()
	if len(cat) < 12 {
		t.Fatalf("sherald rule catalogue has only %d rules; §42.3 assigns sherald the full host+repo-safety set (~15) — looks truncated", len(cat))
	}

	byID := map[string]bindings.RuleSpec{}
	for _, r := range cat {
		if _, dup := byID[r.RuleID]; dup {
			t.Fatalf("duplicate rule in catalogue: %q", r.RuleID)
		}
		byID[r.RuleID] = r
	}

	// Spot-check the canonical sherald rows from §42.3 with their declared
	// default mode + severity + event class. These are load-bearing: a binding
	// registered with the wrong class would route the emit through the wrong
	// fan-out (e.g. a host breach showing up as a generic policy violation).
	want := []struct {
		id    string
		sev   constitution.Severity
		mod   constitution.Mode
		class string
	}{
		{"§9.1", constitution.SeverityCritical, constitution.ModeEnforce, constitution.ClassRepoSafetyBreach},    // destructive-op
		{"§9.2", constitution.SeverityCritical, constitution.ModeEnforce, constitution.ClassRepoSafetyBreach},    // force-push auth
		{"§9.3", constitution.SeverityMiddle, constitution.ModeEnforce, constitution.ClassGateRecovered},         // hardlinked backup
		{"§11.4.41", constitution.SeverityCritical, constitution.ModeEnforce, constitution.ClassRepoSafetyBreach},// pre-force-push merge-first
		{"§11.4.71", constitution.SeverityHigh, constitution.ModeEnforce, constitution.ClassRepoSafetyBreach},    // pre-push fetch-guard
		{"§11.4.36", constitution.SeverityMiddle, constitution.ModeWarn, constitution.ClassRepoSafetyBreach},     // install_upstreams
		{"§11.4.32", constitution.SeverityHigh, constitution.ModeEnforce, constitution.ClassBundleUpdated},       // post-pull validation
		{"§12.1", constitution.SeverityCritical, constitution.ModeEnforce, constitution.ClassHostSafetyBreach},   // forbidden host ops
		{"§12.2", constitution.SeverityHigh, constitution.ModeEnforce, constitution.ClassHostSafetyBreach},       // required safeguards
		{"§12.3", constitution.SeverityHigh, constitution.ModeEnforce, constitution.ClassHostSafetyBreach},       // container hygiene
		{"§12.6", constitution.SeverityCritical, constitution.ModeEnforce, constitution.ClassHostSafetyBreach},   // mem-budget 60%
	}
	for _, w := range want {
		got, ok := byID[w.id]
		if !ok {
			t.Errorf("rule %q missing from sherald catalogue (§42.3 assigns it to sherald)", w.id)
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

	// Every rule MUST carry a non-nil Check + non-empty EventClass + a
	// BreachKind tag (sherald-specific: the safety-event payload needs it).
	for _, r := range cat {
		if r.Check == nil {
			t.Errorf("rule %q has a nil Check func — it can never detect a breach", r.RuleID)
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
	want := len(bindings.SheraldRules())
	if got != want {
		t.Fatalf("pipeline registered %d evaluators, catalogue has %d", got, want)
	}
	for _, r := range bindings.SheraldRules() {
		if _, ok := p.Registry().Get(r.RuleID); !ok {
			t.Errorf("rule %q not retrievable from the pipeline registry", r.RuleID)
		}
	}
}

// TestPipeline_DestructiveOpRoundTrip is THE load-bearing HRD-020 anti-bluff
// proof for the DESTRUCTIVE-OP detection hook (§9.1).
//
// It feeds a SIMULATED destructive op ("git reset --hard" with no backup) as a
// Subject — the detector DETECTS it and emits, it NEVER runs the op. The full
// round-trip is asserted: (1) a .repo.safety.breach event reaches the bus;
// (2) a constitution_state row decision=fail persisted; (3) a constitution_audit
// row naming the emitted event ID.
func TestPipeline_DestructiveOpRoundTrip(t *testing.T) {
	ctx := context.Background()
	p, st, _, au, bus := newPipeline(t)
	tenant := uuid.New()

	breaches := subscribeClass(t, bus, constitution.ClassRepoSafetyBreach)

	// A SIMULATED destructive op: "git reset --hard" attempted with NO backup.
	// The detector reads this description; it does not execute anything.
	bad := constitution.Subject{Kind: bindings.SubjectDestructiveOp, ID: "git reset --hard|backup=false"}
	out, err := p.EvaluateSubject(ctx, "§9.1", tenant, bad)
	if err != nil {
		t.Fatalf("EvaluateSubject §9.1: %v", err)
	}
	if out.Decision != constitution.DecisionFail {
		t.Fatalf("destructive op without backup must FAIL §9.1, got %v", out.Decision)
	}
	if !out.Transition.Changed || !out.Transition.FirstSeen {
		t.Fatalf("first breach should be FirstSeen+Changed, got %+v", out.Transition)
	}
	if out.Mode != constitution.ModeEnforce {
		t.Fatalf("§9.1 default-enforces; mode = %v", out.Mode)
	}
	if !out.Emitted {
		t.Fatalf("enforce-mode breach must emit; out=%+v", out)
	}
	if !out.Audited {
		t.Fatalf("enforce-mode breach must audit; out=%+v", out)
	}

	waitCount(t, breaches, 1, "repo.safety.breach", bus)

	rows, err := st.List(ctx, tenant, constitution.ListQuery{RuleID: "§9.1"})
	if err != nil {
		t.Fatalf("store List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 persisted state row, got %d", len(rows))
	}
	if rows[0].Decision != constitution.DecisionFail {
		t.Errorf("persisted decision = %v, want fail", rows[0].Decision)
	}

	audit, err := au.ListAudit(ctx, tenant, constitution.AuditQuery{RuleID: "§9.1"})
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

// TestPipeline_DestructiveOpWithBackupPasses proves the §9.1 detector does NOT
// false-positive: a destructive op WITH a hardlinked backup recorded is
// compliant (PASS). §11.4.1 anti-FAIL-bluff at the binding layer.
func TestPipeline_DestructiveOpWithBackupPasses(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()

	good := constitution.Subject{Kind: bindings.SubjectDestructiveOp, ID: "rm -rf build/|backup=true"}
	out, err := p.EvaluateSubject(ctx, "§9.1", tenant, good)
	if err != nil {
		t.Fatalf("EvaluateSubject: %v", err)
	}
	if out.Decision != constitution.DecisionPass {
		t.Fatalf("destructive op WITH backup should PASS §9.1 (false positive), got %v", out.Decision)
	}
}

// TestPipeline_ForcePushInterceptedRoundTrip is the FORCE-PUSH interceptor proof
// (§11.4.41 pre-force-push merge-first). A force-push attempt WITHOUT a
// preceding merge is INTERCEPTED (detected + emitted), never executed.
func TestPipeline_ForcePushInterceptedRoundTrip(t *testing.T) {
	ctx := context.Background()
	p, st, _, _, bus := newPipeline(t)
	tenant := uuid.New()

	breaches := subscribeClass(t, bus, constitution.ClassRepoSafetyBreach)

	// SIMULATED force-push WITHOUT merge-first + WITHOUT session auth.
	bad := constitution.Subject{Kind: bindings.SubjectForcePush, ID: "origin/main|merged=false|authorized=false"}
	out, err := p.EvaluateSubject(ctx, "§11.4.41", tenant, bad)
	if err != nil {
		t.Fatalf("EvaluateSubject §11.4.41: %v", err)
	}
	if out.Decision != constitution.DecisionFail {
		t.Fatalf("force-push without merge-first must FAIL §11.4.41, got %v", out.Decision)
	}
	if !out.Emitted {
		t.Fatalf("enforce-mode §11.4.41 must emit; out=%+v", out)
	}
	waitCount(t, breaches, 1, "repo.safety.breach", bus)

	rows, err := st.List(ctx, tenant, constitution.ListQuery{RuleID: "§11.4.41"})
	if err != nil {
		t.Fatalf("store List: %v", err)
	}
	if len(rows) != 1 || rows[0].Decision != constitution.DecisionFail {
		t.Fatalf("expected 1 failed state row, got %+v", rows)
	}
}

// TestPipeline_ForcePushMergedAndAuthorizedPasses proves a force-push that WAS
// preceded by a merge AND carries session authorization PASSes §11.4.41.
func TestPipeline_ForcePushMergedAndAuthorizedPasses(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()

	good := constitution.Subject{Kind: bindings.SubjectForcePush, ID: "origin/main|merged=true|authorized=true"}
	out, err := p.EvaluateSubject(ctx, "§11.4.41", tenant, good)
	if err != nil {
		t.Fatalf("EvaluateSubject: %v", err)
	}
	if out.Decision != constitution.DecisionPass {
		t.Fatalf("merge-first + authorized force-push should PASS §11.4.41, got %v", out.Decision)
	}
}

// TestPipeline_MemBudgetBreachRoundTrip is the MEM-BUDGET watcher proof (§12.6
// 60% ceiling). It feeds a SIMULATED mem-usage signal (a fraction) — it NEVER
// actually allocates or exhausts host memory (§12.6 host-safety). A reading
// above 60% is a host.safety.breach; at/below 60% is compliant.
func TestPipeline_MemBudgetBreachRoundTrip(t *testing.T) {
	ctx := context.Background()
	p, st, _, au, bus := newPipeline(t)
	tenant := uuid.New()

	breaches := subscribeClass(t, bus, constitution.ClassHostSafetyBreach)

	// SIMULATED 0.83 (83%) used-fraction reading — over the 60% ceiling.
	// No memory is allocated; the detector reads the reported fraction string.
	bad := constitution.Subject{Kind: bindings.SubjectMemBudget, ID: "used_fraction=0.83"}
	out, err := p.EvaluateSubject(ctx, "§12.6", tenant, bad)
	if err != nil {
		t.Fatalf("EvaluateSubject §12.6: %v", err)
	}
	if out.Decision != constitution.DecisionFail {
		t.Fatalf("83%% mem usage must breach §12.6 (60%% ceiling), got %v", out.Decision)
	}
	if !out.Emitted {
		t.Fatalf("enforce-mode §12.6 breach must emit; out=%+v", out)
	}
	waitCount(t, breaches, 1, "host.safety.breach", bus)

	rows, err := st.List(ctx, tenant, constitution.ListQuery{RuleID: "§12.6"})
	if err != nil {
		t.Fatalf("store List: %v", err)
	}
	if len(rows) != 1 || rows[0].Decision != constitution.DecisionFail {
		t.Fatalf("expected 1 failed state row, got %+v", rows)
	}
	audit, err := au.ListAudit(ctx, tenant, constitution.AuditQuery{RuleID: "§12.6"})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(audit) != 1 {
		t.Fatalf("expected 1 audit row, got %d", len(audit))
	}
}

// TestPipeline_MemBudgetUnderCeilingPasses proves a reading at/below 60% is
// compliant — no false-positive breach.
func TestPipeline_MemBudgetUnderCeilingPasses(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()

	for _, frac := range []string{"used_fraction=0.40", "used_fraction=0.60"} {
		good := constitution.Subject{Kind: bindings.SubjectMemBudget, ID: frac}
		out, err := p.EvaluateSubject(ctx, "§12.6", tenant, good)
		if err != nil {
			t.Fatalf("EvaluateSubject %s: %v", frac, err)
		}
		if out.Decision != constitution.DecisionPass {
			t.Fatalf("%s should PASS §12.6 (at/under 60%% ceiling), got %v", frac, out.Decision)
		}
	}
}

// TestPipeline_HostSafetyClassRouting proves the §12.1 forbidden-host-op binding
// routes through the .host.safety.breach class (NOT generic .policy.violation).
// A suspend/logout attempt is detected + reported, never executed (§12-safe).
func TestPipeline_HostSafetyClassRouting(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, bus := newPipeline(t)
	tenant := uuid.New()

	hostBreaches := subscribeClass(t, bus, constitution.ClassHostSafetyBreach)
	policyViolations := subscribeClass(t, bus, constitution.ClassPolicyViolation)

	// SIMULATED forbidden host op: "systemctl suspend" attempt. Detected only.
	bad := constitution.Subject{Kind: bindings.SubjectHostOp, ID: "systemctl suspend"}
	out, err := p.EvaluateSubject(ctx, "§12.1", tenant, bad)
	if err != nil {
		t.Fatalf("EvaluateSubject §12.1: %v", err)
	}
	if out.Decision != constitution.DecisionFail {
		t.Fatalf("forbidden host op must FAIL §12.1, got %v", out.Decision)
	}
	waitCount(t, hostBreaches, 1, "host.safety.breach", bus)
	// And it must NOT have fanned out as a generic policy violation.
	time.Sleep(50 * time.Millisecond)
	if n := atomic.LoadInt64(policyViolations); n != 0 {
		t.Fatalf("§12.1 breach leaked %d .policy.violation events (should route ONLY to host.safety.breach)", n)
	}
}

// TestPipeline_BackupGateRecovered proves §9.3 (hardlinked backup) routes
// through .gate.recovered on a SUCCESSFUL backup — it is the audit-trail
// "backup-created" event, not a breach. A PASS in enforce mode emits recovered.
func TestPipeline_BackupGateRecovered(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, bus := newPipeline(t)
	tenant := uuid.New()

	recovered := subscribeClass(t, bus, constitution.ClassGateRecovered)

	// A backup that WAS created — §9.3 PASS → .gate.recovered audit event.
	ok := constitution.Subject{Kind: bindings.SubjectBackup, ID: "/repo|created=true"}
	out, err := p.EvaluateSubject(ctx, "§9.3", tenant, ok)
	if err != nil {
		t.Fatalf("EvaluateSubject §9.3: %v", err)
	}
	if out.Decision != constitution.DecisionPass {
		t.Fatalf("a created backup should PASS §9.3, got %v", out.Decision)
	}
	if !out.Emitted {
		t.Fatalf("enforce-mode §9.3 PASS transition must emit .gate.recovered; out=%+v", out)
	}
	waitCount(t, recovered, 1, "gate.recovered", bus)
}

// TestPipeline_BundleUpdatedRouting proves §11.4.32 (post-constitution-pull
// validation) routes a PASS through .bundle.updated.
func TestPipeline_BundleUpdatedRouting(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, bus := newPipeline(t)
	tenant := uuid.New()

	updated := subscribeClass(t, bus, constitution.ClassBundleUpdated)

	ok := constitution.Subject{Kind: bindings.SubjectBundleValidation, ID: "constitution|validated=true"}
	out, err := p.EvaluateSubject(ctx, "§11.4.32", tenant, ok)
	if err != nil {
		t.Fatalf("EvaluateSubject §11.4.32: %v", err)
	}
	if out.Decision != constitution.DecisionPass {
		t.Fatalf("a validated post-pull should PASS §11.4.32, got %v", out.Decision)
	}
	if !out.Emitted {
		t.Fatalf("enforce-mode §11.4.32 PASS must emit .bundle.updated; out=%+v", out)
	}
	waitCount(t, updated, 1, "bundle.updated", bus)
}

// TestPipeline_LadderGatesEmit proves the breach decision is GATED by the mode
// ladder — an allow-mode rule records state but does NOT emit or audit
// (data-gathering only, §42.5-step-3 initial-mode-allow contract).
func TestPipeline_LadderGatesEmit(t *testing.T) {
	ctx := context.Background()
	p, st, la, au, bus := newPipeline(t)
	tenant := uuid.New()

	if err := la.Set(ctx, tenant, "§9.1", constitution.ModeAllow, "test"); err != nil {
		t.Fatalf("ladder Set allow: %v", err)
	}

	before := bus.Metrics().Published
	bad := constitution.Subject{Kind: bindings.SubjectDestructiveOp, ID: "rm -rf /important|backup=false"}
	out, err := p.EvaluateSubject(ctx, "§9.1", tenant, bad)
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
	rows, err := st.List(ctx, tenant, constitution.ListQuery{RuleID: "§9.1"})
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

// TestPipeline_UnknownRule errors loudly rather than silently passing (§11.4.6).
func TestPipeline_UnknownRule(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	_, err := p.EvaluateSubject(ctx, "§does.not.exist", uuid.New(), constitution.Subject{Kind: "x", ID: "y"})
	if err == nil {
		t.Fatalf("expected error for unknown rule, got nil")
	}
}

// TestNewPipeline_RejectsNilBackends proves a nil emit/audit/store/ladder is a
// hard error — an emit/audit that goes nowhere is a §107 bluff.
func TestNewPipeline_RejectsNilBackends(t *testing.T) {
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{})
	defer bus.Close()
	em, _ := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "x"})
	st := state.NewMemory()
	la := ladder.NewMemory()
	au := state.NewMemoryAudit()

	cases := []bindings.Config{
		{Ladder: nil, Store: st, Emitter: em, Audit: au},
		{Ladder: la, Store: nil, Emitter: em, Audit: au},
		{Ladder: la, Store: st, Emitter: nil, Audit: au},
		{Ladder: la, Store: st, Emitter: em, Audit: nil},
	}
	for i, cfg := range cases {
		if _, err := bindings.NewPipeline(cfg); err == nil {
			t.Errorf("case %d: NewPipeline with a nil backend must error", i)
		}
	}
}

// TestDetectors_NeverPerformOps is the §12 SAFETY attestation test: it confirms
// the detection hooks are PURE — they take only a string Subject describing an
// attempted op and return a verdict. There is no exec, no os call, no fork. We
// prove this structurally by driving every destructive-class subject through
// the pipeline and confirming the process is unaffected (the test itself is the
// witness: if a detector actually ran `rm`/`reset`/`suspend` the test harness
// would not survive). This documents the contract for the reviewer.
func TestDetectors_NeverPerformOps(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()

	// Each of these describes a DANGEROUS op. If any detector executed instead
	// of merely classifying, this test (and the host) would be harmed. The
	// fact that all return cleanly is the structural §12-safety proof.
	dangerous := []struct {
		rule string
		subj constitution.Subject
	}{
		{"§9.1", constitution.Subject{Kind: bindings.SubjectDestructiveOp, ID: "rm -rf /|backup=false"}},
		{"§11.4.41", constitution.Subject{Kind: bindings.SubjectForcePush, ID: "origin/main|merged=false|authorized=false"}},
		{"§12.1", constitution.Subject{Kind: bindings.SubjectHostOp, ID: "shutdown -h now"}},
		{"§12.6", constitution.Subject{Kind: bindings.SubjectMemBudget, ID: "used_fraction=0.99"}},
	}
	for _, d := range dangerous {
		out, err := p.EvaluateSubject(ctx, d.rule, tenant, d.subj)
		if err != nil {
			t.Fatalf("%s detector errored (should classify, not error): %v", d.rule, err)
		}
		if out.Decision != constitution.DecisionFail {
			t.Errorf("%s: dangerous op %q should be detected as FAIL, got %v", d.rule, d.subj.ID, out.Decision)
		}
	}
	// If we reached here, no detector performed a real destructive operation.
}
