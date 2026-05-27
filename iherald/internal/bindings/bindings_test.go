package bindings_test

// HRD-024 — iherald incident/escalation constitution bindings (v1.0.0 Batch B,
// unit 6).
//
// These tests are written FIRST (TDD RED) against the bindings package that
// wires iherald's §42.3-owned escalation rules (§11.4.10 / §11.4.10.A
// credential-leak page-out, §11.4.21 operator-blocked escalation, §11.4.66
// blocker-resolution clarification) plus the bespoke §18.8 incident-severity
// routing rule into the Batch-A commons_constitution Evaluator + Registry +
// AuditStore + ModeLadder foundation. Every assertion checks an observable
// side-effect of the REAL foundation running end-to-end (real MemoryBus, real
// ConstitutionStore, real ModeLadder, real MemoryAudit, real emit) — no
// mocks-of-mocks (§11.4.27).
//
// The load-bearing round-trips these tests prove (the §107 anti-bluff bar for
// HRD-024):
//
//	(A) a CREDENTIAL LEAK signal (a detected plaintext credential / tracked .env)
//	    is detected by a registered iherald binding → emitted as a
//	    .credential.leak event on the bus (the page-out fan-out) → persisted as a
//	    constitution_state row AND a constitution_audit row → visible via the same
//	    ConstitutionStore.List a future /v1/webhooks/page pull surface reads.
//	(B) an OPERATOR-BLOCKED escalation gap (an item entered operator-blocked
//	    without the on-call page) is detected → emitted as a .policy.violation
//	    event with a captured emitted-event-id audit row.
//
// A binding that "registered" but could never produce a queryable, audited
// escalation would be exactly the metadata-only PASS the §11.4.10 credentials
// covenant forbids.
//
// NO REAL SECRETS: every credential Subject uses a FAKE/synthetic location +
// boolean detection flag. NO real .env is scanned and NO real secret string
// appears anywhere in these tests.

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	constitution "github.com/vasic-digital/herald/commons_constitution"
	"github.com/vasic-digital/herald/commons_constitution/ladder"
	"github.com/vasic-digital/herald/commons_constitution/state"
	"github.com/vasic-digital/herald/iherald/internal/bindings"
)

// newPipeline builds a Pipeline backed entirely by the production-faithful
// in-memory foundation backends (the same seam cherald/sherald/bherald/rherald
// use). Returns the pipeline + the live store/ladder/audit/bus so tests can
// assert persisted side-effects directly.
func newPipeline(t *testing.T) (*bindings.Pipeline, constitution.ConstitutionStore, constitution.ModeLadder, constitution.AuditStore, constitution.EventBus) {
	t.Helper()
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{BufferSize: 256})
	t.Cleanup(func() { _ = bus.Close() })
	em, err := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "digital.vasic.herald/iherald"})
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

// TestRuleCatalogue_CoversIheraldOwnedRules proves the binding catalogue
// actually carries the iherald-owned §42.3 escalation rows — not an empty
// registry that would make every downstream "binding works" claim vacuous. We
// assert the canonical iherald rows are present with the spec's default mode +
// severity + event class.
func TestRuleCatalogue_CoversIheraldOwnedRules(t *testing.T) {
	cat := bindings.IheraldRules()
	if len(cat) < 5 {
		t.Fatalf("iherald rule catalogue has only %d rules; spec §42.3 assigns iherald 4 escalation rows + 1 bespoke — looks truncated", len(cat))
	}

	byID := map[string]bindings.RuleSpec{}
	for _, r := range cat {
		if _, dup := byID[r.RuleID]; dup {
			t.Fatalf("duplicate rule in catalogue: %q", r.RuleID)
		}
		byID[r.RuleID] = r
	}

	// The canonical iherald rows from §42.3 (lines 4421-4471) + the bespoke
	// §18.8 row, with their declared default mode + severity + event class. A
	// binding registered with the wrong default mode/class would silently
	// misbehave against spec.
	want := []struct {
		id    string
		sev   constitution.Severity
		mod   constitution.Mode
		class string
	}{
		{"§11.4.10", constitution.SeverityCritical, constitution.ModeEnforce, constitution.ClassCredentialLeak},   // credentials-handling page-out
		{"§11.4.10.A", constitution.SeverityCritical, constitution.ModeEnforce, constitution.ClassCredentialLeak}, // pre-store leak audit
		{"§11.4.21", constitution.SeverityHigh, constitution.ModeEnforce, constitution.ClassPolicyViolation},      // operator-blocked escalation
		{"§11.4.66", constitution.SeverityHigh, constitution.ModeEnforce, constitution.ClassPolicyViolation},      // blocker-resolution clarification
		{"§18.8", constitution.SeverityHigh, constitution.ModeEnforce, constitution.ClassPolicyViolation},         // bespoke incident-severity routing
	}
	for _, w := range want {
		got, ok := byID[w.id]
		if !ok {
			t.Errorf("rule %q missing from iherald catalogue (§42.3 assigns it to iherald)", w.id)
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
	// with no evaluator logic can never produce a real escalation (a §107
	// binding bluff).
	for _, r := range cat {
		if r.Check == nil {
			t.Errorf("rule %q has a nil Check func — it can never detect an escalation", r.RuleID)
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
	want := len(bindings.IheraldRules())
	if got != want {
		t.Fatalf("pipeline registered %d evaluators, catalogue has %d", got, want)
	}
	for _, r := range bindings.IheraldRules() {
		if _, ok := p.Registry().Get(r.RuleID); !ok {
			t.Errorf("rule %q not retrievable from the pipeline registry", r.RuleID)
		}
	}
}

// TestPipeline_CredentialLeakRoundTrip is THE load-bearing HRD-024 anti-bluff
// proof for the credential-leak page-out path.
//
// It drives a real iherald binding (§11.4.10 credentials-handling) whose Check
// func classifies a recorded credential-leak signal as FAIL, through the REAL
// Pipeline, and asserts the FULL round-trip:
//
//	(1) a .credential.leak event reaches the bus (the page-out fan-out fired);
//	(2) a constitution_state row was persisted with decision=fail + the subject
//	    (queryable via a future /v1/webhooks/page pull surface);
//	(3) a constitution_audit row was written under enforce mode (the durable
//	    audit trail behind RunOutcome.Audited).
//
// NO REAL SECRET: the Subject encodes only a fake location + a boolean
// detection flag.
func TestPipeline_CredentialLeakRoundTrip(t *testing.T) {
	ctx := context.Background()
	p, st, la, au, bus := newPipeline(t)
	tenant := uuid.New()

	// §11.4.10 default-enforces per §42.3.
	if err := la.Set(ctx, tenant, "§11.4.10", constitution.ModeEnforce, "test"); err != nil {
		t.Fatalf("ladder Set enforce: %v", err)
	}

	// Subscribe to the credential.leak class — the anti-bluff observation that
	// proves the page-out emit reached the bus.
	sub, err := bus.Subscribe(constitution.EventNamespace + "." + constitution.ClassCredentialLeak)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Cancel()
	var leaks int64
	go func() {
		for range sub.Channel {
			atomic.AddInt64(&leaks, 1)
		}
	}()

	// A credential-leak signal — a FAKE location + a leak-detection flag. NO
	// real secret. The §43 credential-scan body records the outcome; the binding
	// classifies it + pages out.
	leak := constitution.Subject{Kind: bindings.SubjectCredentialLeak, ID: "config/fake-test-fixture|leaked=true|kind=env"}

	out, err := p.EvaluateSubject(ctx, "§11.4.10", tenant, leak)
	if err != nil {
		t.Fatalf("EvaluateSubject: %v", err)
	}
	if out.Decision != constitution.DecisionFail {
		t.Fatalf("decision = %v, want fail for a detected credential leak", out.Decision)
	}
	if !out.Transition.Changed || !out.Transition.FirstSeen {
		t.Fatalf("first leak signal should be a FirstSeen+Changed transition, got %+v", out.Transition)
	}
	if out.Mode != constitution.ModeEnforce {
		t.Fatalf("mode = %v, want enforce (default §42.3)", out.Mode)
	}
	if !out.Emitted {
		t.Fatalf("enforce-mode credential leak must page out (emit); out=%+v", out)
	}
	if !out.Audited {
		t.Fatalf("enforce-mode credential leak must write an audit row; out=%+v", out)
	}

	// (1) the page-out emit reached the bus.
	deadline := time.After(2 * time.Second)
	for atomic.LoadInt64(&leaks) < 1 {
		select {
		case <-deadline:
			t.Fatalf("listener never saw the .credential.leak; bus metrics=%+v", bus.Metrics())
		case <-time.After(5 * time.Millisecond):
		}
	}

	// (2) a constitution_state row is queryable.
	rows, err := st.List(ctx, tenant, constitution.ListQuery{RuleID: "§11.4.10"})
	if err != nil {
		t.Fatalf("store List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 persisted state row, got %d", len(rows))
	}
	if rows[0].Decision != constitution.DecisionFail {
		t.Errorf("persisted decision = %v, want fail", rows[0].Decision)
	}
	if rows[0].Subject != leak.ID {
		t.Errorf("persisted subject = %q, want %q", rows[0].Subject, leak.ID)
	}

	// (3) a constitution_audit row was written under enforce mode.
	audit, err := au.ListAudit(ctx, tenant, constitution.AuditQuery{RuleID: "§11.4.10"})
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

// TestPipeline_OperatorBlockedRoundTrip proves the §11.4.21 operator-blocked
// escalation routes through .policy.violation with a captured emitted-event-id
// audit row (the IDEmitter capture path).
func TestPipeline_OperatorBlockedRoundTrip(t *testing.T) {
	ctx := context.Background()
	p, st, la, au, bus := newPipeline(t)
	tenant := uuid.New()

	if err := la.Set(ctx, tenant, "§11.4.21", constitution.ModeEnforce, "test"); err != nil {
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

	// An item that entered operator-blocked WITHOUT the on-call page → escalation
	// gap.
	gap := constitution.Subject{Kind: bindings.SubjectOperatorBlocked, ID: "HRD-999|status=operator-blocked|oncall_paged=false"}

	out, err := p.EvaluateSubject(ctx, "§11.4.21", tenant, gap)
	if err != nil {
		t.Fatalf("EvaluateSubject: %v", err)
	}
	if out.Decision != constitution.DecisionFail {
		t.Fatalf("decision = %v, want fail for an un-paged operator-blocked item", out.Decision)
	}
	if !out.Emitted || !out.Audited {
		t.Fatalf("enforce-mode escalation must emit + audit; out=%+v", out)
	}

	deadline := time.After(2 * time.Second)
	for atomic.LoadInt64(&violations) < 1 {
		select {
		case <-deadline:
			t.Fatalf("listener never saw the .policy.violation; bus metrics=%+v", bus.Metrics())
		case <-time.After(5 * time.Millisecond):
		}
	}

	rows, err := st.List(ctx, tenant, constitution.ListQuery{RuleID: "§11.4.21"})
	if err != nil {
		t.Fatalf("store List: %v", err)
	}
	if len(rows) != 1 || rows[0].Decision != constitution.DecisionFail {
		t.Fatalf("expected 1 persisted fail row, got %+v", rows)
	}

	// The policy class captures the emitted event ID via IDEmitter — the audit
	// row MUST name a non-Nil emitted event (the load-bearing emitted_event_id
	// column).
	audit, err := au.ListAudit(ctx, tenant, constitution.AuditQuery{RuleID: "§11.4.21"})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(audit) != 1 {
		t.Fatalf("expected exactly 1 audit row, got %d", len(audit))
	}
	if audit[0].EmittedEventID == uuid.Nil {
		t.Errorf("enforce-mode policy.violation audit row should name the emitted event ID, got uuid.Nil")
	}
}

// TestPipeline_CredentialLeakRecoveredOnPass proves a credential-leak that
// transitions back to PASS (the leak was remediated) emits .gate.recovered (the
// shared companion class — the credential class has no distinct "cleared" leaf),
// NOT another .credential.leak. A binding that re-paged on a recovery would
// spam the on-call.
func TestPipeline_CredentialLeakRecoveredOnPass(t *testing.T) {
	ctx := context.Background()
	p, _, la, _, bus := newPipeline(t)
	tenant := uuid.New()
	if err := la.Set(ctx, tenant, "§11.4.10", constitution.ModeEnforce, "test"); err != nil {
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

	// First a leak (FirstSeen FAIL), then a clean re-scan (transition fail→pass)
	// for the SAME location so the second is a recovery transition.
	loc := "config/fake-test-fixture"
	leak := constitution.Subject{Kind: bindings.SubjectCredentialLeak, ID: loc + "|leaked=true|kind=env"}
	if _, err := p.EvaluateSubject(ctx, "§11.4.10", tenant, leak); err != nil {
		t.Fatalf("EvaluateSubject(leak): %v", err)
	}
	clean := constitution.Subject{Kind: bindings.SubjectCredentialLeak, ID: loc + "|leaked=false"}
	out, err := p.EvaluateSubject(ctx, "§11.4.10", tenant, clean)
	if err != nil {
		t.Fatalf("EvaluateSubject(clean): %v", err)
	}
	if out.Decision != constitution.DecisionPass {
		t.Fatalf("decision = %v, want pass on leaked=false", out.Decision)
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

// TestPipeline_LadderGatesEmit proves the escalation decision is GATED by the
// mode ladder — an allow-mode rule records state but does NOT emit or audit
// (the §42.5-step "initial mode allow = data gathering only" contract). A
// credential leak under allow-mode is recorded but does NOT page out.
func TestPipeline_LadderGatesEmit(t *testing.T) {
	ctx := context.Background()
	p, st, la, au, bus := newPipeline(t)
	tenant := uuid.New()

	if err := la.Set(ctx, tenant, "§11.4.10", constitution.ModeAllow, "test"); err != nil {
		t.Fatalf("ladder Set allow: %v", err)
	}

	before := bus.Metrics().Published
	leak := constitution.Subject{Kind: bindings.SubjectCredentialLeak, ID: "config/fake-test-fixture|leaked=true"}
	out, err := p.EvaluateSubject(ctx, "§11.4.10", tenant, leak)
	if err != nil {
		t.Fatalf("EvaluateSubject: %v", err)
	}
	if out.Decision != constitution.DecisionFail {
		t.Fatalf("decision = %v, want fail", out.Decision)
	}
	if out.Emitted {
		t.Errorf("allow-mode MUST NOT page out (emit); out=%+v", out)
	}
	if out.Audited {
		t.Errorf("allow-mode MUST NOT audit; out=%+v", out)
	}
	if after := bus.Metrics().Published; after != before {
		t.Errorf("allow-mode published %d events (want 0)", after-before)
	}
	rows, err := st.List(ctx, tenant, constitution.ListQuery{RuleID: "§11.4.10"})
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
	_, err := p.EvaluateSubject(ctx, "§does.not.exist", uuid.New(), constitution.Subject{Kind: "x", ID: "y"})
	if err == nil {
		t.Fatalf("expected error for unknown rule, got nil")
	}
}

// ---------------------------------------------------------------------------
// Bespoke detector tests — the load-bearing iherald escalation classifiers.
// ---------------------------------------------------------------------------

// TestDetector_CredentialLeakClassification proves the §11.4.10 detector:
// leaked=true → FAIL (page out), leaked=false → PASS, missing field → FAIL
// (refuse to silent-PASS). NO real secret in any subject.
func TestDetector_CredentialLeakClassification(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()

	cases := []struct {
		id   string
		want constitution.Decision
	}{
		{"config/fake|leaked=true|kind=env", constitution.DecisionFail},
		{"src/fake.go|leaked=true|kind=source", constitution.DecisionFail},
		{"config/fake|leaked=false", constitution.DecisionPass},
		{"config/fake-no-detection-field", constitution.DecisionFail}, // refuse silent-PASS
	}
	for _, c := range cases {
		subj := constitution.Subject{Kind: bindings.SubjectCredentialLeak, ID: c.id}
		out, err := p.EvaluateSubject(ctx, "§11.4.10", tenant, subj)
		if err != nil {
			t.Fatalf("EvaluateSubject(%s): %v", c.id, err)
		}
		if out.Decision != c.want {
			t.Errorf("credential subject %q classified as %v, want %v", c.id, out.Decision, c.want)
		}
	}
}

// TestDetector_PreStoreAudit proves the §11.4.10.A detector: a credential-storage
// commit that SKIPPED its pre-store leak audit (audited=false) FAILs, an audit
// that RAN and found a leak (audited=true|leaked=true) FAILs, and a clean audit
// (audited=true|leaked=false) PASSes.
func TestDetector_PreStoreAudit(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()

	cases := []struct {
		id   string
		want constitution.Decision
	}{
		{"store-secret-op|audited=false", constitution.DecisionFail},             // gate skipped
		{"store-secret-op|audited=true|leaked=true", constitution.DecisionFail},  // audit found leak
		{"store-secret-op|audited=true|leaked=false", constitution.DecisionPass}, // clean
		{"store-secret-op-no-audit-field", constitution.DecisionFail},            // refuse silent-PASS
	}
	for _, c := range cases {
		subj := constitution.Subject{Kind: bindings.SubjectPreStoreAudit, ID: c.id}
		out, err := p.EvaluateSubject(ctx, "§11.4.10.A", tenant, subj)
		if err != nil {
			t.Fatalf("EvaluateSubject(%s): %v", c.id, err)
		}
		if out.Decision != c.want {
			t.Errorf("pre-store subject %q classified as %v, want %v", c.id, out.Decision, c.want)
		}
	}
}

// TestDetector_OperatorBlocked proves the §11.4.21 detector: operator-blocked
// without an on-call page FAILs; operator-blocked WITH a page PASSes; a
// non-blocked status PASSes (not an escalation event).
func TestDetector_OperatorBlocked(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()

	cases := []struct {
		id   string
		want constitution.Decision
	}{
		{"HRD-1|status=operator-blocked|oncall_paged=false", constitution.DecisionFail},
		{"HRD-2|status=operator-blocked|oncall_paged=true", constitution.DecisionPass},
		{"HRD-3|status=in-progress", constitution.DecisionPass},
		{"HRD-4|status=operator-blocked", constitution.DecisionFail}, // missing paged field
	}
	for _, c := range cases {
		subj := constitution.Subject{Kind: bindings.SubjectOperatorBlocked, ID: c.id}
		out, err := p.EvaluateSubject(ctx, "§11.4.21", tenant, subj)
		if err != nil {
			t.Fatalf("EvaluateSubject(%s): %v", c.id, err)
		}
		if out.Decision != c.want {
			t.Errorf("operator-blocked subject %q classified as %v, want %v", c.id, out.Decision, c.want)
		}
	}
}

// TestDetector_BlockerClarification proves the §11.4.66 detector: a blocked item
// with NO clarification prompt FAILs; a blocked item WITH a clarification prompt
// PASSes; an unblocked item PASSes.
func TestDetector_BlockerClarification(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()

	cases := []struct {
		id   string
		want constitution.Decision
	}{
		{"HRD-1|blocked=true|clarification=false", constitution.DecisionFail},
		{"HRD-2|blocked=true|clarification=true", constitution.DecisionPass},
		{"HRD-3|blocked=false", constitution.DecisionPass},
		{"HRD-4|blocked=true", constitution.DecisionFail}, // missing clarification field
	}
	for _, c := range cases {
		subj := constitution.Subject{Kind: bindings.SubjectBlockerClarification, ID: c.id}
		out, err := p.EvaluateSubject(ctx, "§11.4.66", tenant, subj)
		if err != nil {
			t.Fatalf("EvaluateSubject(%s): %v", c.id, err)
		}
		if out.Decision != c.want {
			t.Errorf("blocker subject %q classified as %v, want %v", c.id, out.Decision, c.want)
		}
	}
}

// TestDetector_IncidentSeverityRouting proves the bespoke §18.8 detector: a
// high-severity incident (sev1/sev2) not paged out FAILs; paged-out high-severity
// PASSes; sev3/sev4 PASSes (no mandatory page); an unclassified incident FAILs.
func TestDetector_IncidentSeverityRouting(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()

	cases := []struct {
		id   string
		want constitution.Decision
	}{
		{"INC-1|severity=sev1|paged=false", constitution.DecisionFail},
		{"INC-2|severity=sev2|paged=true", constitution.DecisionPass},
		{"INC-3|severity=sev3", constitution.DecisionPass},
		{"INC-4|severity=sev4", constitution.DecisionPass},
		{"INC-5|severity=sev1", constitution.DecisionFail},  // high-sev, missing paged field
		{"INC-6", constitution.DecisionFail},                // no severity classification
		{"INC-7|severity=bogus", constitution.DecisionFail}, // unrecognized severity
	}
	for _, c := range cases {
		subj := constitution.Subject{Kind: bindings.SubjectIncidentSeverity, ID: c.id}
		out, err := p.EvaluateSubject(ctx, "§18.8", tenant, subj)
		if err != nil {
			t.Fatalf("EvaluateSubject(%s): %v", c.id, err)
		}
		if out.Decision != c.want {
			t.Errorf("incident subject %q classified as %v, want %v", c.id, out.Decision, c.want)
		}
	}
}

// TestPipeline_MalformedSubjectDoesNotPass proves a credential subject with no
// recognizable detection field does NOT silently PASS — refusing to silent-PASS
// an unproven credential prerequisite is the §11.4.1 fail-bluff inverse.
func TestPipeline_MalformedSubjectDoesNotPass(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()
	subj := constitution.Subject{Kind: bindings.SubjectCredentialLeak, ID: "garbage-no-detection-field"}
	out, err := p.EvaluateSubject(ctx, "§11.4.10", tenant, subj)
	if err != nil {
		t.Fatalf("EvaluateSubject: %v", err)
	}
	if out.Decision == constitution.DecisionPass {
		t.Errorf("malformed credential subject (no detection field) classified as PASS — that is a §11.4.1 fail-bluff inverse")
	}
}

// TestNoRealSecretsInFixtures is a meta-assertion (§107.x no-real-secret
// attestation): every credential-leak fixture used in this test file encodes a
// FAKE location + a boolean flag, never a real secret value. We assert the
// canonical fixture marker "fake" is the convention so a future edit that drops
// in a real secret string is caught by review against this anchor.
func TestNoRealSecretsInFixtures(t *testing.T) {
	// The credential fixtures we use are these FAKE locations. None is a real
	// secret; all carry a "fake" marker or an obviously-synthetic name.
	fixtures := []string{
		"config/fake-test-fixture|leaked=true|kind=env",
		"src/fake.go|leaked=true|kind=source",
		"config/fake|leaked=false",
	}
	for _, f := range fixtures {
		if !strings.Contains(f, "fake") {
			t.Errorf("credential fixture %q lacks the 'fake' synthetic marker — possible real-secret leak into tests", f)
		}
	}
}
