// Package bindings wires sherald's §42.3-owned host-safety + repo-safety
// constitution rules into the Batch-A commons_constitution foundation
// (Evaluator + Registry + ModeLadder + ConstitutionStore + AuditStore + the
// typed safety-event emitter).
//
// HRD-020 (v1.0.0 Batch B, unit 2) — sherald is the SYSTEM / SAFETY flavor.
// Per spec §42.3 it owns the host-safety + repo-safety rule rows (§9.1/.2/.3,
// §11.4.32/.36/.41/.71, §12.1/.2/.3/.6) whose breaches fan out as
// .host.safety.breach / .repo.safety.breach / .gate.recovered / .bundle.updated
// events. This package supplies:
//
//   - RuleSpec: the declarative description of one sherald-owned safety rule
//     (its §42.3 RuleID, default Severity + Mode, the emitted EventClass, a
//     BreachKind tag for the safety-event payload, and a deterministic
//     CheckFunc that DETECTS a violation from a Subject description).
//   - Binding: a commons_constitution.Evaluator adapter over a RuleSpec.
//   - SheraldRules(): the catalogue of every sherald-owned §42.3 safety rule.
//   - Pipeline: composes the catalogue into a Registry + a class-aware
//     evaluate→record→gate→emit→audit flow that drives REAL safety events
//     through the Batch-A emitter + persists them so they surface on a
//     future /v1/safety/* pull surface.
//
// Why a sherald-local Pipeline (mirroring cherald's HRD-019 Pipeline) instead
// of commons_constitution.Runner: the Runner hardcodes the .policy.violation /
// .policy.cleared emit class. sherald binds its rules to SAFETY classes that
// cherald never touched — .host.safety.breach, .repo.safety.breach,
// .gate.recovered, .bundle.updated, .bundle.update.failed — so the emit must be
// class-aware. The Pipeline reuses the Runner's exact gate semantics
// (transitions-only emission, allow/warn/enforce ladder, audit-on-warn-or-
// enforce) but routes the emit through the EventClass + BreachKind declared on
// each RuleSpec.
//
// §107 anti-bluff: a safety binding that registers but can never produce a
// queryable, audited breach is a PASS-bluff. Every Pipeline.EvaluateSubject
// call drives the REAL emitter + REAL store + REAL audit — the persisted
// side-effects ARE the positive runtime evidence (see bindings_test.go).
//
// §12 / §12.6 host-safety: every CheckFunc is PURE. It DETECTS a violation from
// a Subject string description and returns a verdict. It NEVER executes the
// destructive op, force-pushes, suspends the host, or allocates memory. The
// detection hooks GUARD — they do not act. The upstream §43 command bodies
// (HRD-033 destructive-guard, HRD-046 force-push-gate, HRD-056 mem-budget-watch)
// supply the live op-interception that feeds these detectors their Subjects.
package bindings

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/google/uuid"

	constitution "github.com/vasic-digital/herald/commons_constitution"
)

// Subject.Kind constants for sherald safety subjects. The §43 command bodies
// (or a sweep) construct a Subject with one of these Kinds + an ID encoding the
// already-observed op parameters; the binding CLASSIFIES it. The detector never
// performs the op — it reads the description.
const (
	// SubjectDestructiveOp: ID = "<cmd>|backup=<true|false>" — §9.1.
	SubjectDestructiveOp = "destructive-op"
	// SubjectForcePush: ID = "<ref>|merged=<bool>|authorized=<bool>" — §9.2/§11.4.41.
	SubjectForcePush = "force-push"
	// SubjectBackup: ID = "<path>|created=<bool>" — §9.3 hardlinked backup.
	SubjectBackup = "backup"
	// SubjectMemBudget: ID = "used_fraction=<0..1>" — §12.6 60% ceiling.
	SubjectMemBudget = "mem-budget"
	// SubjectHostOp: ID = "<forbidden host command>" — §12.1.
	SubjectHostOp = "host-op"
	// SubjectSafeguard: ID = "<work>|bounded=<bool>" — §12.2 required safeguards.
	SubjectSafeguard = "safeguard"
	// SubjectContainer: ID = "<name>|mem_limit=<bool>" — §12.3 container hygiene.
	SubjectContainer = "container"
	// SubjectBundleValidation: ID = "<bundle>|validated=<bool>" — §11.4.32.
	SubjectBundleValidation = "bundle-validation"
	// SubjectUpstreams: ID = "<submodule>|configured=<bool>" — §11.4.36.
	SubjectUpstreams = "upstreams"
	// SubjectPush: ID = "<ref>|fetched=<bool>|integrated=<bool>" — §11.4.71.
	SubjectPush = "push"
	// SubjectCommit: ID = "<entrypoint>|complete=<bool>" — §2 commit+push.
	SubjectCommit = "commit"
	// SubjectBuildStats: ID = "<build>|stats=<bool>" — §11.4.24.
	SubjectBuildStats = "build-stats"
	// SubjectConstitutionPull: ID = "<sha>|ok=<bool>" — §11.4.26.
	SubjectConstitutionPull = "constitution-pull"
	// SubjectFirebaseReview: ID = "<dataset>|reviewed=<bool>" — §11.4.47.
	SubjectFirebaseReview = "firebase-review"
)

// CheckFunc decides the safety verdict for one Subject under one rule. It
// returns a Result whose Decision is PASS/WARN/FAIL plus an Evidence string + a
// stable DigestSHA used by the transition gate. CheckFunc MUST be deterministic
// for a given subject, safe for concurrent calls, and — CRITICAL for sherald —
// PURE: it classifies the Subject description, it never performs the op.
type CheckFunc func(ctx context.Context, s constitution.Subject, bundle constitution.BundleHash) (constitution.Result, error)

// RuleSpec is the declarative description of one sherald-owned §42.3 safety rule.
type RuleSpec struct {
	// RuleID is the canonical constitution section anchor, e.g. "§9.1".
	RuleID string
	// Title is the human-readable rule name from the Constitution ToC.
	Title string
	// Severity is the §42.3 default severity tier.
	Severity constitution.Severity
	// DefaultMode is the §42.3 default enforcement mode (the ladder seed).
	DefaultMode constitution.Mode
	// EventClass is the commons_constitution.Class* the emit routes through.
	EventClass string
	// BreachKind tags the safety-event payload (SafetyEvent.BreachKind):
	// "destructive-op" / "force-push" / "mem-budget" / "host-op" / ... — used by
	// subscribers to disambiguate breach origin. Empty for non-safety classes.
	BreachKind string
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

// PushTriggers implements commons_constitution.Evaluator. sherald safety
// bindings are driven by the §43 command bodies / the mem-sampler — none
// declare push-triggers in HRD-020. Returns nil.
func (b *Binding) PushTriggers() []string { return nil }

// Subjects implements commons_constitution.Evaluator. HRD-020 evaluates
// explicit subjects supplied by the caller (the §43 destructive-guard /
// force-push-gate / mem-budget-watch command bodies). Returns nil.
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

// Pipeline composes the sherald rule catalogue into a Registry and drives the
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

// NewPipeline builds a Pipeline, registering every SheraldRules() entry into a
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
	for _, spec := range SheraldRules() {
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
// dispatches the emit through the per-rule safety EventClass.
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

// emit routes the transition through the rule's declared safety EventClass.
// Returns the emitted event ID (for the audit row) when the class supports ID
// capture (policy class via IDEmitter); the safety classes return uuid.Nil but
// still fan out the wire event.
//
// The Decision drives the breach-vs-recovered variant: a FAIL/ERROR transition
// is the breach; a PASS/WARN transition in enforce mode is the recovered/clear
// signal (mirroring Runner.Run's cleared branch). For the safety classes this
// means a FAIL → .host/.repo.safety.breach, a PASS → .gate.recovered (the
// "safeguard satisfied" audit-trail event).
func (p *Pipeline) emit(ctx context.Context, spec RuleSpec, tenantID uuid.UUID, subject constitution.Subject, result constitution.Result, trans constitution.Transition) (uuid.UUID, error) {
	cleared := result.Decision == constitution.DecisionPass || result.Decision == constitution.DecisionWarn

	switch spec.EventClass {
	case constitution.ClassHostSafetyBreach:
		if cleared {
			// A satisfied host-safeguard is an audit-trail recovery, not a breach.
			return uuid.Nil, p.emitter.GateRecovered(ctx, p.gateEvent(spec, tenantID, result, trans))
		}
		return uuid.Nil, p.emitter.HostSafetyBreach(ctx, p.safetyEvent(spec, tenantID, subject, result, trans))
	case constitution.ClassRepoSafetyBreach:
		if cleared {
			return uuid.Nil, p.emitter.GateRecovered(ctx, p.gateEvent(spec, tenantID, result, trans))
		}
		return uuid.Nil, p.emitter.RepoSafetyBreach(ctx, p.safetyEvent(spec, tenantID, subject, result, trans))
	case constitution.ClassGateRecovered:
		// §9.3 backup-created: a PASS is the recovery; a FAIL (backup missing)
		// is a gate failure.
		if cleared {
			return uuid.Nil, p.emitter.GateRecovered(ctx, p.gateEvent(spec, tenantID, result, trans))
		}
		return uuid.Nil, p.emitter.GateFailed(ctx, p.gateEvent(spec, tenantID, result, trans))
	case constitution.ClassGateFailed:
		if cleared {
			return uuid.Nil, p.emitter.GateRecovered(ctx, p.gateEvent(spec, tenantID, result, trans))
		}
		return uuid.Nil, p.emitter.GateFailed(ctx, p.gateEvent(spec, tenantID, result, trans))
	case constitution.ClassBundleUpdated:
		// §11.4.32 post-pull validation: PASS → .bundle.updated; FAIL →
		// .bundle.update.failed.
		if cleared {
			return uuid.Nil, p.emitter.BundleUpdated(ctx, p.bundleEvent(tenantID, result, trans, ""))
		}
		return uuid.Nil, p.emitter.BundleUpdateFailed(ctx, p.bundleEvent(tenantID, result, trans, result.Evidence))
	default: // ClassPolicyViolation / ClassPolicyCleared — §11.4.47 generic path.
		policyEvent := constitution.PolicyEvent{
			TenantID:    tenantID,
			RuleID:      spec.RuleID,
			Severity:    spec.Severity,
			Subject:     subject,
			Bundle:      p.bundle,
			Transition:  trans,
			EvidenceURI: result.Evidence,
		}
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

// safetyEvent builds the SafetyEvent for the .host/.repo.safety.breach classes,
// carrying the rule's BreachKind so subscribers can disambiguate origin.
func (p *Pipeline) safetyEvent(spec RuleSpec, tenantID uuid.UUID, subject constitution.Subject, result constitution.Result, trans constitution.Transition) constitution.SafetyEvent {
	return constitution.SafetyEvent{
		TenantID:    tenantID,
		RuleID:      spec.RuleID,
		Severity:    spec.Severity,
		BreachKind:  spec.BreachKind,
		Subject:     subject,
		Bundle:      p.bundle,
		Transition:  trans,
		EvidenceURI: result.Evidence,
	}
}

// gateEvent builds the GateEvent for the .gate.failed / .gate.recovered classes.
func (p *Pipeline) gateEvent(spec RuleSpec, tenantID uuid.UUID, result constitution.Result, trans constitution.Transition) constitution.GateEvent {
	return constitution.GateEvent{
		TenantID:    tenantID,
		RuleID:      spec.RuleID,
		Severity:    spec.Severity,
		GateName:    spec.RuleID,
		Bundle:      p.bundle,
		Transition:  trans,
		EvidenceURI: result.Evidence,
	}
}

// bundleEvent builds the BundleEvent for the .bundle.updated /
// .bundle.update.failed classes (§11.4.32 post-pull validation outcome).
func (p *Pipeline) bundleEvent(tenantID uuid.UUID, result constitution.Result, trans constitution.Transition, errMsg string) constitution.BundleEvent {
	return constitution.BundleEvent{
		TenantID:   tenantID,
		NewBundle:  p.bundle,
		Transition: trans,
		Error:      errMsg,
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
