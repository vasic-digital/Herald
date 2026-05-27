package bindings_test

// HRD-022 — rherald release constitution bindings (v1.0.0 Batch B, unit 4).
//
// These tests are written FIRST (TDD RED) against the bindings package that
// wires rherald's §42.3-owned release gate rules into the Batch-A
// commons_constitution Evaluator + Registry + AuditStore + ModeLadder
// foundation. Every assertion checks an observable side-effect of the REAL
// foundation running end-to-end (real MemoryBus, real ConstitutionStore, real
// ModeLadder, real MemoryAudit, real emit) — no mocks-of-mocks (§11.4.27).
//
// The load-bearing round-trip these tests prove (the §107 anti-bluff bar for
// HRD-022): a RELEASE gate RESULT (a tag missing on an owned mirror, a release
// asset that fails installability, a tag attempted WITHOUT the full-suite
// retest, a non-conforming changelog) is detected by a registered rherald
// binding → emitted as a .release.gate.blocked (or .policy.violation for the
// shared §5 changelog row) event on the bus → persisted as a
// constitution_state row AND a constitution_audit row → visible via the same
// ConstitutionStore.List a future /v1/release pull surface reads. A binding
// that "registered" but could never produce a queryable, audited release
// verdict would be exactly the metadata-only PASS the covenant forbids.
//
// §12 host-safety: every detector CLASSIFIES a recorded release-op outcome
// string — it NEVER tags, pushes, force-pushes, or runs git. The §43 command
// bodies (HRD-031/032/045) supply the live release integration upstream.

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	constitution "github.com/vasic-digital/herald/commons_constitution"
	"github.com/vasic-digital/herald/commons_constitution/ladder"
	"github.com/vasic-digital/herald/commons_constitution/state"
	"github.com/vasic-digital/herald/rherald/internal/bindings"
)

// newPipeline builds a Pipeline backed entirely by the production-faithful
// in-memory foundation backends (the same seam cherald/sherald/bherald use).
// Returns the pipeline + the live store/ladder/audit/bus so tests can assert
// persisted side-effects directly.
func newPipeline(t *testing.T) (*bindings.Pipeline, constitution.ConstitutionStore, constitution.ModeLadder, constitution.AuditStore, constitution.EventBus) {
	t.Helper()
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{BufferSize: 256})
	t.Cleanup(func() { _ = bus.Close() })
	em, err := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "digital.vasic.herald/rherald"})
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

// TestRuleCatalogue_CoversRheraldOwnedRules proves the binding catalogue
// actually carries the rherald-owned §42.3 release rows — not an empty registry
// that would make every downstream "binding works" claim vacuous. We assert (a)
// the four spec §42.3 rherald rows are present and (b) each carries the spec's
// default mode + severity + event class.
func TestRuleCatalogue_CoversRheraldOwnedRules(t *testing.T) {
	cat := bindings.RheraldRules()
	if len(cat) < 4 {
		t.Fatalf("rherald rule catalogue has only %d rules; spec §42.3 assigns rherald §4/§5/§11.4.38/§11.4.40 — looks truncated", len(cat))
	}

	byID := map[string]bindings.RuleSpec{}
	for _, r := range cat {
		if _, dup := byID[r.RuleID]; dup {
			t.Fatalf("duplicate rule in catalogue: %q", r.RuleID)
		}
		byID[r.RuleID] = r
	}

	// The four canonical rherald rows from §42.3 with their declared default
	// mode + severity + event class. These are load-bearing: a binding
	// registered with the wrong default mode/class would silently misbehave
	// against spec.
	want := []struct {
		id    string
		sev   constitution.Severity
		mod   constitution.Mode
		class string
	}{
		{"§4", constitution.SeverityHigh, constitution.ModeEnforce, constitution.ClassReleaseGateBlocked},      // tag mirroring
		{"§5", constitution.SeverityMiddle, constitution.ModeWarn, constitution.ClassPolicyViolation},          // changelog (shared with cherald)
		{"§11.4.38", constitution.SeverityHigh, constitution.ModeEnforce, constitution.ClassReleaseGateBlocked}, // installable-asset evidence
		{"§11.4.40", constitution.SeverityCritical, constitution.ModeEnforce, constitution.ClassReleaseGateBlocked}, // full-suite retest before tag
	}
	for _, w := range want {
		got, ok := byID[w.id]
		if !ok {
			t.Errorf("rule %q missing from rherald catalogue (§42.3 assigns it to rherald)", w.id)
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
	// with no evaluator logic can never produce a real release verdict (a §107
	// binding bluff).
	for _, r := range cat {
		if r.Check == nil {
			t.Errorf("rule %q has a nil Check func — it can never detect a release verdict", r.RuleID)
		}
		if r.EventClass == "" {
			t.Errorf("rule %q has an empty EventClass — cannot route the emit", r.RuleID)
		}
	}
}

// TestPipeline_RegistersEveryRule proves NewPipeline registered every catalogue
// rule into the underlying commons_constitution.Registry — so a sweep over the
// registry actually sees the rherald rule set (not a partial registration).
func TestPipeline_RegistersEveryRule(t *testing.T) {
	p, _, _, _, _ := newPipeline(t)
	got := p.Registry().Len()
	want := len(bindings.RheraldRules())
	if got != want {
		t.Fatalf("pipeline registered %d evaluators, catalogue has %d", got, want)
	}
	for _, r := range bindings.RheraldRules() {
		if _, ok := p.Registry().Get(r.RuleID); !ok {
			t.Errorf("rule %q not retrievable from the pipeline registry", r.RuleID)
		}
	}
}

// TestPipeline_ReleaseGateBlockedRoundTrip is THE load-bearing HRD-022
// anti-bluff proof.
//
// It drives a real rherald binding (§4 tag-mirror-parity) whose Check func
// classifies a recorded tag-mirror state as a parity FAIL (tag missing on an
// owned mirror), through the REAL Pipeline, and asserts the FULL round-trip:
//
//	(1) a .release.gate.blocked event reaches the bus (the channel emit fired);
//	(2) a constitution_state row was persisted with decision=fail + the bundle
//	    hash + the subject (queryable via a future /v1/release pull surface);
//	(3) a constitution_audit row was written naming the emitted event ID
//	    (the durable audit trail behind RunOutcome.Audited).
//
// A binding that "evaluated" but produced no queryable/audited evidence is a
// §107 PASS-bluff; this test makes that impossible to ship.
func TestPipeline_ReleaseGateBlockedRoundTrip(t *testing.T) {
	ctx := context.Background()
	p, st, la, au, bus := newPipeline(t)
	tenant := uuid.New()

	// §4 default-enforces per §42.3.
	if err := la.Set(ctx, tenant, "§4", constitution.ModeEnforce, "test"); err != nil {
		t.Fatalf("ladder Set enforce: %v", err)
	}

	// Subscribe to the release-gate-blocked class — the anti-bluff observation
	// that proves the emit reached the bus.
	sub, err := bus.Subscribe(constitution.EventNamespace + "." + constitution.ClassReleaseGateBlocked)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Cancel()
	var blocked int64
	go func() {
		for range sub.Channel {
			atomic.AddInt64(&blocked, 1)
		}
	}()

	// A tag-mirror parity check that FAILed (the tag is present on the parent
	// but missing on an owned submodule mirror). The §43 release command body
	// records the parity state; the binding classifies it. NO real git/tag/push.
	badTag := constitution.Subject{Kind: bindings.SubjectTagMirror, ID: "v1.4.0|tag=present|mirrors=4|with_tag=3"}

	out, err := p.EvaluateSubject(ctx, "§4", tenant, badTag)
	if err != nil {
		t.Fatalf("EvaluateSubject: %v", err)
	}
	if out.Decision != constitution.DecisionFail {
		t.Fatalf("decision = %v, want fail for a tag-mirror parity miss", out.Decision)
	}
	if !out.Transition.Changed || !out.Transition.FirstSeen {
		t.Fatalf("first release-result should be a FirstSeen+Changed transition, got %+v", out.Transition)
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
	for atomic.LoadInt64(&blocked) < 1 {
		select {
		case <-deadline:
			t.Fatalf("listener never saw the .release.gate.blocked; bus metrics=%+v", bus.Metrics())
		case <-time.After(5 * time.Millisecond):
		}
	}

	// (2) a constitution_state row is queryable.
	rows, err := st.List(ctx, tenant, constitution.ListQuery{RuleID: "§4"})
	if err != nil {
		t.Fatalf("store List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 persisted state row, got %d", len(rows))
	}
	if rows[0].Decision != constitution.DecisionFail {
		t.Errorf("persisted decision = %v, want fail", rows[0].Decision)
	}
	if rows[0].Subject != badTag.ID {
		t.Errorf("persisted subject = %q, want %q", rows[0].Subject, badTag.ID)
	}

	// (3) a constitution_audit row was written.
	audit, err := au.ListAudit(ctx, tenant, constitution.AuditQuery{RuleID: "§4"})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(audit) != 1 {
		t.Fatalf("expected exactly 1 audit row, got %d", len(audit))
	}
	if audit[0].ModeAtEmission != constitution.ModeEnforce {
		t.Errorf("audit mode = %v, want enforce", audit[0].ModeAtEmission)
	}

	// The wire event MUST be exactly .release.gate.blocked — NOT a generic
	// .policy.violation (a misrouted class would mis-signal release subscribers).
	published := bus.Metrics().PublishedByType[constitution.EventNamespace+"."+constitution.ClassReleaseGateBlocked]
	if published != 1 {
		t.Fatalf("bus published %d .release.gate.blocked events, want 1", published)
	}
}

// TestPipeline_ReleaseGateRecoveredOnPass proves a release gate that
// transitions back to PASS emits .release.gate.blocked again is NOT fired — the
// recovery routes through the .gate.recovered companion (the release classes
// have no distinct "recovered" leaf, so the Pipeline maps a release recovery to
// .gate.recovered, mirroring the safety/release recovery convention). A binding
// that emitted .release.gate.blocked on a recovery would mis-signal subscribers.
func TestPipeline_ReleaseGateRecoveredOnPass(t *testing.T) {
	ctx := context.Background()
	p, _, la, _, bus := newPipeline(t)
	tenant := uuid.New()
	if err := la.Set(ctx, tenant, "§11.4.40", constitution.ModeEnforce, "test"); err != nil {
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

	// First a FAIL (retest NOT run), then a PASS (retest run + all tiers green)
	// for the SAME release ref so the second is a recovery transition.
	bad := constitution.Subject{Kind: bindings.SubjectRetestGate, ID: "v1.4.0|retest=skipped"}
	if _, err := p.EvaluateSubject(ctx, "§11.4.40", tenant, bad); err != nil {
		t.Fatalf("EvaluateSubject(fail): %v", err)
	}
	good := constitution.Subject{Kind: bindings.SubjectRetestGate, ID: "v1.4.0|retest=green|tiers=8"}
	out, err := p.EvaluateSubject(ctx, "§11.4.40", tenant, good)
	if err != nil {
		t.Fatalf("EvaluateSubject(pass): %v", err)
	}
	if out.Decision != constitution.DecisionPass {
		t.Fatalf("decision = %v, want pass on a green retest", out.Decision)
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

// TestPipeline_ChangelogPolicyViolation proves the shared §5 changelog row
// routes through .policy.violation (NOT a release-gate class — changelog is a
// warn-mode policy concern, shared with cherald). A non-conforming changelog
// (missing / stale export) is flagged as a policy violation.
func TestPipeline_ChangelogPolicyViolation(t *testing.T) {
	ctx := context.Background()
	p, _, la, au, bus := newPipeline(t)
	tenant := uuid.New()
	// §5 is warn by default — flip to enforce so the emit fires (warn audits only).
	if err := la.Set(ctx, tenant, "§5", constitution.ModeEnforce, "test"); err != nil {
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

	// A non-conforming changelog (no Conventional-Commits-derived entries).
	bad := constitution.Subject{Kind: bindings.SubjectChangelog, ID: "v1.4.0|conforming=false"}
	out, err := p.EvaluateSubject(ctx, "§5", tenant, bad)
	if err != nil {
		t.Fatalf("EvaluateSubject: %v", err)
	}
	if out.Decision != constitution.DecisionFail {
		t.Fatalf("decision = %v, want fail for a non-conforming changelog", out.Decision)
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
	audit, err := au.ListAudit(ctx, tenant, constitution.AuditQuery{RuleID: "§5"})
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

	if err := la.Set(ctx, tenant, "§4", constitution.ModeAllow, "test"); err != nil {
		t.Fatalf("ladder Set allow: %v", err)
	}

	before := bus.Metrics().Published
	bad := constitution.Subject{Kind: bindings.SubjectTagMirror, ID: "v1.4.0|tag=present|mirrors=4|with_tag=3"}
	out, err := p.EvaluateSubject(ctx, "§4", tenant, bad)
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
	rows, err := st.List(ctx, tenant, constitution.ListQuery{RuleID: "§4"})
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
	_, err := p.EvaluateSubject(ctx, "§does.not.exist", uuid.New(), constitution.Subject{Kind: "release", ID: "x"})
	if err == nil {
		t.Fatalf("expected error for unknown rule, got nil")
	}
}

// ---------------------------------------------------------------------------
// Bespoke detector tests — the 3 load-bearing rherald release classifiers.
// ---------------------------------------------------------------------------

// TestDetector_TagMirrorParity proves the §4 tag-mirror-parity detector
// classifies a recorded tag-mirror state: a tag present on every owned mirror
// PASSes; a tag missing on ANY owned mirror FAILs (the §4 "tag missing on owned
// submodule" violation). PURE: it reads the recorded mirror tally; it NEVER
// tags, pushes, or runs git.
func TestDetector_TagMirrorParity(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()

	cases := []struct {
		id   string
		want constitution.Decision
	}{
		{"v1.4.0|tag=present|mirrors=4|with_tag=4", constitution.DecisionPass}, // full parity
		{"v1.4.0|tag=present|mirrors=4|with_tag=3", constitution.DecisionFail}, // one mirror missing the tag
		{"v1.4.0|tag=present|mirrors=4|with_tag=0", constitution.DecisionFail}, // none mirrored
		{"v1.4.0|tag=present|mirrors=1|with_tag=1", constitution.DecisionPass}, // single mirror, parity
		{"v1.4.0|tag=absent|mirrors=4|with_tag=4", constitution.DecisionFail},  // parent tag itself absent
	}
	for _, c := range cases {
		subj := constitution.Subject{Kind: bindings.SubjectTagMirror, ID: c.id}
		out, err := p.EvaluateSubject(ctx, "§4", tenant, subj)
		if err != nil {
			t.Fatalf("EvaluateSubject(%s): %v", c.id, err)
		}
		if out.Decision != c.want {
			t.Errorf("tag-mirror %q classified as %v, want %v", c.id, out.Decision, c.want)
		}
	}
}

// TestDetector_ChangelogConformance proves the §5 changelog-conformance
// detector: a changelog flagged conforming=true PASSes; conforming=false (or a
// stale export) FAILs. This is the §5 "changelog missing or stale export"
// detector. PURE: it reads the recorded conformance flag; it NEVER regenerates
// the changelog or runs git-log.
func TestDetector_ChangelogConformance(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()

	cases := []struct {
		id   string
		want constitution.Decision
	}{
		{"v1.4.0|conforming=true", constitution.DecisionPass},
		{"v1.4.0|conforming=false", constitution.DecisionFail},
		{"v1.4.0|conforming=true|export_stale=true", constitution.DecisionFail}, // stale multi-format export
		{"v1.4.0", constitution.DecisionFail},                                   // no conformance field → refuse to silent-PASS
	}
	for _, c := range cases {
		subj := constitution.Subject{Kind: bindings.SubjectChangelog, ID: c.id}
		out, err := p.EvaluateSubject(ctx, "§5", tenant, subj)
		if err != nil {
			t.Fatalf("EvaluateSubject(%s): %v", c.id, err)
		}
		if out.Decision != c.want {
			t.Errorf("changelog %q classified as %v, want %v", c.id, out.Decision, c.want)
		}
	}
}

// TestDetector_PreTagRetestGate proves the §11.4.40 pre-tag-retest-gate
// detector — THE critical-severity release gate: a tag attempted WITHOUT the
// full-suite retest (retest=skipped / missing) FAILs; a green retest covering
// all 8 §40.2 tiers PASSes; a retest that ran but is RED FAILs. PURE: it reads
// the recorded retest outcome; it NEVER tags or re-runs the suite.
func TestDetector_PreTagRetestGate(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()

	cases := []struct {
		id   string
		want constitution.Decision
	}{
		{"v1.4.0|retest=green|tiers=8", constitution.DecisionPass}, // full retest, all tiers
		{"v1.4.0|retest=skipped", constitution.DecisionFail},       // §11.4.40 violation — tag without retest
		{"v1.4.0|retest=red", constitution.DecisionFail},           // retest ran but failed
		{"v1.4.0|retest=green|tiers=5", constitution.DecisionFail}, // incomplete tier coverage
		{"v1.4.0", constitution.DecisionFail},                      // no retest field → refuse to silent-PASS
	}
	for _, c := range cases {
		subj := constitution.Subject{Kind: bindings.SubjectRetestGate, ID: c.id}
		out, err := p.EvaluateSubject(ctx, "§11.4.40", tenant, subj)
		if err != nil {
			t.Fatalf("EvaluateSubject(%s): %v", c.id, err)
		}
		if out.Decision != c.want {
			t.Errorf("retest-gate %q classified as %v, want %v", c.id, out.Decision, c.want)
		}
	}
}

// TestDetector_InstallableAssetEvidence proves the §11.4.38 installable-asset
// detector: a release asset whose install check passed (installed=true) PASSes;
// install=false FAILs (the §11.4.38 "release asset fails installability check"
// violation). PURE: it reads the recorded install-check outcome; it NEVER
// downloads or installs the asset.
func TestDetector_InstallableAssetEvidence(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()

	cases := []struct {
		id   string
		want constitution.Decision
	}{
		{"pherald_linux_amd64|installed=true", constitution.DecisionPass},
		{"pherald_linux_amd64|installed=false", constitution.DecisionFail},
		{"pherald_darwin_arm64", constitution.DecisionFail}, // no install evidence → fail
	}
	for _, c := range cases {
		subj := constitution.Subject{Kind: bindings.SubjectInstallAsset, ID: c.id}
		out, err := p.EvaluateSubject(ctx, "§11.4.38", tenant, subj)
		if err != nil {
			t.Fatalf("EvaluateSubject(%s): %v", c.id, err)
		}
		if out.Decision != c.want {
			t.Errorf("install-asset %q classified as %v, want %v", c.id, out.Decision, c.want)
		}
	}
}

// TestPipeline_MalformedSubjectDoesNotPass proves a release-result subject with
// no recognizable outcome MUST NOT silently PASS (§11.4.1 fail-bluff inverse).
func TestPipeline_MalformedSubjectDoesNotPass(t *testing.T) {
	ctx := context.Background()
	p, _, _, _, _ := newPipeline(t)
	tenant := uuid.New()
	subj := constitution.Subject{Kind: bindings.SubjectTagMirror, ID: "garbage-no-fields"}
	out, err := p.EvaluateSubject(ctx, "§4", tenant, subj)
	if err != nil {
		t.Fatalf("EvaluateSubject: %v", err)
	}
	if out.Decision == constitution.DecisionPass {
		t.Errorf("malformed tag-mirror (no fields) classified as PASS — that is a §11.4.1 fail-bluff inverse")
	}
}
