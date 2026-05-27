// Package bindings wires bherald's §42.3-owned CI/test gate-result constitution
// rules into the Batch-A commons_constitution foundation (Evaluator + Registry
// + ModeLadder + ConstitutionStore + AuditStore + the typed event emitter).
//
// HRD-021 (v1.0.0 Batch B, unit 3) — bherald is the BUILD / CI flavor. Per spec
// §42.3 it owns the CI/test gate-result rule rows (§1, §1.1, §11.4.2/.3/.4/.5/
// .7/.9/.13/.14/.24/.27/.30/.39/.43/.46/.48–.52/.67) whose verdicts fan out as
// .gate.failed / .gate.recovered events (plus .policy.violation for the
// hygiene rows + .repo.safety.breach for the shared §11.4.30 build-artifact
// row). This package supplies:
//
//   - RuleSpec: the declarative description of one bherald-owned CI/test rule
//     (its §42.3 RuleID, default Severity + Mode, the emitted EventClass, the
//     GateName for the gate-event payload, and a deterministic CheckFunc that
//     CLASSIFIES a recorded gate outcome / test-tier matrix / evidence marker).
//   - Binding: a commons_constitution.Evaluator adapter over a RuleSpec.
//   - BheraldRules(): the catalogue of every bherald-owned §42.3 rule.
//   - Pipeline: composes the catalogue into a Registry + a class-aware
//     evaluate→record→gate→emit→audit flow that drives REAL gate-result events
//     through the Batch-A emitter + persists them so they surface on a future
//     /v1/build pull surface.
//
// Why a bherald-local Pipeline (mirroring cherald's HRD-019 + sherald's HRD-020
// Pipeline) instead of commons_constitution.Runner: the Runner hardcodes the
// .policy.violation / .policy.cleared emit class. bherald binds the BULK of its
// rules to the gate classes — .gate.failed / .gate.recovered — so the emit must
// be class-aware. The Pipeline reuses the Runner's exact gate semantics
// (transitions-only emission, allow/warn/enforce ladder, audit-on-warn-or-
// enforce, emitted-event-id capture) but routes the emit through the EventClass
// declared on each RuleSpec.
//
// §107 anti-bluff: a binding that registers but can never produce a queryable,
// audited gate-result is a PASS-bluff. Every Pipeline.EvaluateSubject call
// drives the REAL emitter + REAL store + REAL audit — the persisted
// side-effects ARE the positive runtime evidence (see bindings_test.go
// round-trip proofs). The §11.4.2 anti-bluff-PASS detector is itself the
// constitutional enforcement of the §107 covenant at the CI layer: a gate that
// reports PASS without a captured-evidence artefact is FAILed.
//
// PURE detectors. Every CheckFunc is PURE: it CLASSIFIES a Subject string
// description of an already-observed CI/test outcome. It NEVER runs the build,
// re-executes the test suite, spawns a process, or touches the filesystem. The
// upstream §43 command bodies (HRD-035 evidence-capture, HRD-041 test-tier-
// verify, HRD-045 gate-retest) supply the live CI integration that feeds these
// detectors their Subjects.
package bindings

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/google/uuid"

	constitution "github.com/vasic-digital/herald/commons_constitution"
)

// Subject.Kind constants for bherald CI/test subjects. The §43 command bodies
// (or a CI gate) construct a Subject with one of these Kinds + an ID encoding
// the already-observed outcome; the binding CLASSIFIES it. The detector never
// runs the gate — it reads the recorded result.
const (
	// SubjectGateResult: ID = "<gate-name>|outcome=<pass|fail|flaky|error>" —
	// §1 / §11.4.50 gate-result classification.
	SubjectGateResult = "gate-result"
	// SubjectEvidence: ID = "<test-id>|outcome=<pass|fail>|evidence=<bool>" —
	// §11.4.2 / §11.4.5 anti-bluff-PASS detection (a PASS with no captured
	// evidence is a §11.4 PASS-bluff → FAIL).
	SubjectEvidence = "evidence"
	// SubjectTestTier: ID = "<pkg>|tiers=<comma-separated present tiers>" —
	// §11.4.27 / §40.2 test-tier-verify (a missing required tier → FAIL).
	SubjectTestTier = "test-tier"
)

// CheckFunc decides the CI/test verdict for one Subject under one rule. It
// returns a Result whose Decision is PASS/WARN/FAIL plus an Evidence string + a
// stable DigestSHA used by the transition gate. CheckFunc MUST be deterministic
// for a given subject, safe for concurrent calls, and — for bherald — PURE: it
// classifies the recorded gate/test outcome, it never re-runs the gate.
type CheckFunc func(ctx context.Context, s constitution.Subject, bundle constitution.BundleHash) (constitution.Result, error)

// RuleSpec is the declarative description of one bherald-owned §42.3 CI/test rule.
type RuleSpec struct {
	// RuleID is the canonical constitution section anchor, e.g. "§11.4.2".
	RuleID string
	// Title is the human-readable rule name from the Constitution ToC.
	Title string
	// Severity is the §42.3 default severity tier.
	Severity constitution.Severity
	// DefaultMode is the §42.3 default enforcement mode (the ladder seed).
	DefaultMode constitution.Mode
	// EventClass is the commons_constitution.Class* the emit routes through:
	// ClassGateFailed (the CI/test gate-result class — pairs with
	// ClassGateRecovered on a PASS transition), ClassPolicyViolation (the
	// hygiene rows), or ClassRepoSafetyBreach (the shared §11.4.30 build-artifact
	// row).
	EventClass string
	// GateName names the CI gate for the GateEvent payload ("test-suite" /
	// "lint" / "coverage" / "test-tier" / ...) so subscribers can disambiguate.
	GateName string
	// Check decides the verdict. Never nil for a shipped rule.
	Check CheckFunc
	// SubjectKinds documents the Subject.Kind values this rule classifies.
	SubjectKinds []string
}

// Binding adapts a RuleSpec to the commons_constitution.Evaluator interface.
type Binding struct {
	spec RuleSpec
}

// NewBinding wraps a RuleSpec. Panics on an invalid spec (nil Check / empty
// RuleID / empty EventClass) — startup programming errors, never runtime
// conditions (mirrors Registry.Register's fail-fast posture).
func NewBinding(spec RuleSpec) *Binding {
	if spec.RuleID == "" {
		panic("bindings: NewBinding: empty RuleID")
	}
	if spec.Check == nil {
		panic(fmt.Sprintf("bindings: NewBinding(%s): nil Check", spec.RuleID))
	}
	if spec.EventClass == "" {
		panic(fmt.Sprintf("bindings: NewBinding(%s): empty EventClass", spec.RuleID))
	}
	return &Binding{spec: spec}
}

// Spec returns the underlying RuleSpec (read-only view).
func (b *Binding) Spec() RuleSpec { return b.spec }

// RuleID implements commons_constitution.Evaluator.
func (b *Binding) RuleID() string { return b.spec.RuleID }

// Severity implements commons_constitution.Evaluator.
func (b *Binding) Severity() constitution.Severity { return b.spec.Severity }

// PushTriggers implements commons_constitution.Evaluator. bherald CI/test
// bindings are driven by the CI gate / §43 command bodies — none declare
// push-triggers in HRD-021. Returns nil.
func (b *Binding) PushTriggers() []string { return nil }

// Subjects implements commons_constitution.Evaluator. HRD-021 evaluates
// explicit subjects supplied by the caller (the CI gate / §43 command bodies).
// Returns nil.
func (b *Binding) Subjects(_ context.Context, _ uuid.UUID) ([]constitution.Subject, error) {
	return nil, nil
}

// Evaluate implements commons_constitution.Evaluator by delegating to Check.
func (b *Binding) Evaluate(ctx context.Context, s constitution.Subject, bundle constitution.BundleHash) (constitution.Result, error) {
	return b.spec.Check(ctx, s, bundle)
}

// Config configures a Pipeline. All four backends are required (a nil backend
// is a §107 bluff — an emit/audit that goes nowhere).
type Config struct {
	Ladder  constitution.ModeLadder
	Store   constitution.ConstitutionStore
	Emitter constitution.EventEmitter
	Audit   constitution.AuditStore
	// Bundle is the constitution bundle hash carried on every emitted event for
	// replayability (§42.1.3). Zero value is a valid "no bundle" sentinel.
	Bundle constitution.BundleHash
}

// Pipeline composes the bherald rule catalogue into a Registry and drives the
// class-aware evaluate→record→gate→emit→audit flow.
type Pipeline struct {
	reg     *constitution.Registry
	ladder  constitution.ModeLadder
	store   constitution.ConstitutionStore
	emitter constitution.EventEmitter
	audit   constitution.AuditStore
	bundle  constitution.BundleHash
	specs   map[string]RuleSpec
}

// NewPipeline builds a Pipeline, registering every BheraldRules() entry into a
// fresh Registry. Returns an error on a nil backend or a duplicate rule ID.
func NewPipeline(cfg Config) (*Pipeline, error) {
	if cfg.Ladder == nil {
		return nil, fmt.Errorf("bindings: NewPipeline: nil Ladder")
	}
	if cfg.Store == nil {
		return nil, fmt.Errorf("bindings: NewPipeline: nil Store")
	}
	if cfg.Emitter == nil {
		return nil, fmt.Errorf("bindings: NewPipeline: nil Emitter")
	}
	if cfg.Audit == nil {
		return nil, fmt.Errorf("bindings: NewPipeline: nil Audit")
	}
	reg := constitution.NewRegistry()
	specs := make(map[string]RuleSpec)
	for _, spec := range BheraldRules() {
		if _, dup := specs[spec.RuleID]; dup {
			return nil, fmt.Errorf("bindings: NewPipeline: duplicate rule %q in catalogue", spec.RuleID)
		}
		specs[spec.RuleID] = spec
		reg.Register(NewBinding(spec))
	}
	return &Pipeline{
		reg:     reg,
		ladder:  cfg.Ladder,
		store:   cfg.Store,
		emitter: cfg.Emitter,
		audit:   cfg.Audit,
		bundle:  cfg.Bundle,
		specs:   specs,
	}, nil
}

// Registry exposes the underlying registry (read paths: Len/Get/All).
func (p *Pipeline) Registry() *constitution.Registry { return p.reg }

// EvaluateSubject runs one (rule, subject) pair through the canonical
// transition-gate flow and routes the emit through the rule's EventClass:
//
//	evaluate → record(state) → if !changed: stop →
//	gate by ladder mode → allow: stop · warn: audit-only · enforce: emit+audit
//
// Mirrors commons_constitution.Runner.Run's gate semantics exactly, but
// dispatches the emit through the per-rule CI/test EventClass.
//
// Unknown rule IDs return an error (no silent pass — §11.4.6 no-guessing).
func (p *Pipeline) EvaluateSubject(ctx context.Context, ruleID string, tenantID uuid.UUID, subject constitution.Subject) (constitution.RunOutcome, error) {
	spec, ok := p.specs[ruleID]
	if !ok {
		return constitution.RunOutcome{}, fmt.Errorf("bindings: EvaluateSubject: unknown rule %q", ruleID)
	}
	b := NewBinding(spec)

	out := constitution.RunOutcome{Evaluator: ruleID, Subject: subject}

	// Step 1: evaluate inside a panic-recovery wrapper.
	result, panicVal := safeEvaluate(ctx, b, subject, p.bundle)
	if panicVal != "" {
		out.PanicValue = panicVal
		out.Decision = constitution.DecisionError
		errResult := constitution.Result{
			Decision:  constitution.DecisionError,
			Evidence:  "evaluator panic: " + panicVal,
			DigestSHA: sha256.Sum256([]byte("panic:" + panicVal)),
		}
		trans, recErr := p.store.Record(ctx, tenantID, ruleID, subject.ID, errResult, p.bundle, errResult.Evidence)
		if recErr != nil {
			return out, fmt.Errorf("bindings: EvaluateSubject: record-after-panic: %w", recErr)
		}
		out.Transition = trans
		return out, nil // panic does NOT propagate
	}
	out.Decision = result.Decision

	// Step 2: persist + compute transition.
	trans, err := p.store.Record(ctx, tenantID, ruleID, subject.ID, result, p.bundle, result.Evidence)
	if err != nil {
		return out, fmt.Errorf("bindings: EvaluateSubject: record: %w", err)
	}
	out.Transition = trans

	// Transitions-only emission discipline (§42.2).
	if !trans.Changed {
		return out, nil
	}

	// Step 3: gate by mode.
	mode, err := p.ladder.Get(ctx, tenantID, ruleID)
	if err != nil {
		return out, fmt.Errorf("bindings: EvaluateSubject: ladder-get: %w", err)
	}
	out.Mode = mode

	switch mode {
	case constitution.ModeAllow:
		return out, nil
	case constitution.ModeWarn:
		out.Audited = true
		if err := p.writeAudit(ctx, tenantID, spec, subject, result, trans, constitution.ModeWarn, uuid.Nil); err != nil {
			return out, fmt.Errorf("bindings: EvaluateSubject: audit(warn): %w", err)
		}
		return out, nil
	case constitution.ModeEnforce:
		out.Audited = true
		out.Emitted = true
		emittedID, emitErr := p.emit(ctx, spec, tenantID, subject, result, trans)
		if emitErr != nil {
			return out, fmt.Errorf("bindings: EvaluateSubject: emit: %w", emitErr)
		}
		if err := p.writeAudit(ctx, tenantID, spec, subject, result, trans, constitution.ModeEnforce, emittedID); err != nil {
			return out, fmt.Errorf("bindings: EvaluateSubject: audit(enforce): %w", err)
		}
		return out, nil
	default:
		return out, fmt.Errorf("bindings: EvaluateSubject: unknown mode %v", mode)
	}
}

// emit routes the transition through the rule's declared EventClass. Returns
// the emitted event ID (for the audit row) when the class supports ID capture
// (policy class via IDEmitter); the gate / safety classes return uuid.Nil but
// still fan out the wire event.
//
// The Decision drives the failed-vs-recovered variant for the gate classes: a
// FAIL/ERROR transition is the gate failure; a PASS/WARN transition in enforce
// mode is the recovery (.gate.recovered) — mirroring Runner.Run's cleared
// branch.
func (p *Pipeline) emit(ctx context.Context, spec RuleSpec, tenantID uuid.UUID, subject constitution.Subject, result constitution.Result, trans constitution.Transition) (uuid.UUID, error) {
	cleared := result.Decision == constitution.DecisionPass || result.Decision == constitution.DecisionWarn

	switch spec.EventClass {
	case constitution.ClassGateFailed, constitution.ClassGateRecovered:
		if cleared {
			return uuid.Nil, p.emitter.GateRecovered(ctx, p.gateEvent(spec, tenantID, result, trans))
		}
		return uuid.Nil, p.emitter.GateFailed(ctx, p.gateEvent(spec, tenantID, result, trans))
	case constitution.ClassRepoSafetyBreach:
		// §11.4.30 No-Versioned-Build-Artifacts (shared with cherald): a committed
		// build artifact is a repo-safety breach; its removal is a recovery.
		if cleared {
			return uuid.Nil, p.emitter.GateRecovered(ctx, p.gateEvent(spec, tenantID, result, trans))
		}
		return uuid.Nil, p.emitter.RepoSafetyBreach(ctx, constitution.SafetyEvent{
			TenantID:    tenantID,
			RuleID:      spec.RuleID,
			Severity:    spec.Severity,
			BreachKind:  "build-artifact",
			Subject:     subject,
			Bundle:      p.bundle,
			Transition:  trans,
			EvidenceURI: result.Evidence,
		})
	default: // ClassPolicyViolation / ClassPolicyCleared — the hygiene rows.
		policyEvent := constitution.PolicyEvent{
			TenantID:    tenantID,
			RuleID:      spec.RuleID,
			Severity:    spec.Severity,
			Subject:     subject,
			Bundle:      p.bundle,
			Transition:  trans,
			EvidenceURI: result.Evidence,
		}
		// Capture the emitted event ID via the optional IDEmitter surface so the
		// audit row records the EXACT id that fanned out (§42.1.3).
		if ide, ok := p.emitter.(constitution.IDEmitter); ok {
			if cleared {
				return ide.PolicyClearedID(ctx, policyEvent)
			}
			return ide.PolicyViolationID(ctx, policyEvent)
		}
		if cleared {
			return uuid.Nil, p.emitter.PolicyCleared(ctx, policyEvent)
		}
		return uuid.Nil, p.emitter.PolicyViolation(ctx, policyEvent)
	}
}

// gateEvent builds the GateEvent for the .gate.failed / .gate.recovered classes,
// carrying the rule's GateName so subscribers can disambiguate which CI gate
// fired.
func (p *Pipeline) gateEvent(spec RuleSpec, tenantID uuid.UUID, result constitution.Result, trans constitution.Transition) constitution.GateEvent {
	name := spec.GateName
	if name == "" {
		name = spec.RuleID
	}
	return constitution.GateEvent{
		TenantID:    tenantID,
		RuleID:      spec.RuleID,
		Severity:    spec.Severity,
		GateName:    name,
		Bundle:      p.bundle,
		Transition:  trans,
		EvidenceURI: result.Evidence,
	}
}

// writeAudit composes + appends one constitution_audit row for a CHANGED
// transition. OldDecision/OldDigest are nil on FirstSeen (no prior verdict) —
// identical semantics to commons_constitution.Runner.writeAudit.
func (p *Pipeline) writeAudit(
	ctx context.Context,
	tenantID uuid.UUID,
	spec RuleSpec,
	subject constitution.Subject,
	result constitution.Result,
	trans constitution.Transition,
	mode constitution.Mode,
	emittedID uuid.UUID,
) error {
	row := constitution.AuditRow{
		TenantID:       tenantID,
		RuleID:         spec.RuleID,
		Subject:        subject.ID,
		NewDecision:    result.Decision,
		NewDigest:      result.DigestSHA,
		BundleHash:     p.bundle,
		EvidenceURI:    result.Evidence,
		EmittedEventID: emittedID,
		ModeAtEmission: mode,
	}
	if !trans.FirstSeen {
		od := trans.OldDecision
		row.OldDecision = &od
		odg := trans.OldDigest
		row.OldDigest = &odg
	}
	_, err := p.audit.RecordAudit(ctx, row)
	return err
}

// safeEvaluate runs b.Evaluate inside a recover() wrapper. Mirrors
// commons_constitution.safeEvaluate's panic-isolation contract: a panicking
// binding never propagates, and an error-returning binding surfaces as
// DecisionError.
func safeEvaluate(ctx context.Context, b *Binding, s constitution.Subject, bundle constitution.BundleHash) (r constitution.Result, panicValue string) {
	defer func() {
		if pv := recover(); pv != nil {
			panicValue = fmt.Sprintf("%v", pv)
		}
	}()
	res, err := b.Evaluate(ctx, s, bundle)
	if err != nil {
		res = constitution.Result{
			Decision:  constitution.DecisionError,
			Evidence:  "evaluator error: " + err.Error(),
			DigestSHA: sha256.Sum256([]byte("err:" + err.Error())),
		}
	}
	return res, ""
}
