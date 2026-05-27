// Package bindings wires pherald's §42.3-owned PROJECT-lifecycle constitution
// rules into the Batch-A commons_constitution foundation (Evaluator + Registry +
// ModeLadder + ConstitutionStore + AuditStore + the typed event emitter).
//
// HRD-023 (v1.0.0 Batch B, unit 5) — pherald is the PROJECT flavor. Per spec
// §42.3 it owns the project-lifecycle rule rows: §2 (commit+push mechanics — the
// single locked entrypoint), §3 (submodule propagation order), §11.4.36
// (install-upstreams), §11.4.37 (fetch-before-edit), §11.4.55 (reopens-history),
// and §11.4.71 (pre-push fetch+investigate+integrate). The repo-safety rows
// (§2/§3/§11.4.37/§11.4.71) fan out as .repo.safety.breach events; the
// policy rows (§11.4.36 install-upstreams + §11.4.55 reopens-history) fan out as
// .policy.violation. This package supplies:
//
//   - RuleSpec: the declarative description of one pherald-owned §42.3 project
//     rule (its §42.3 RuleID, default Severity + Mode, the emitted EventClass,
//     the BreachKind name for the repo-safety event payload, and a deterministic
//     CheckFunc that CLASSIFIES a recorded project-op outcome).
//   - Binding: a commons_constitution.Evaluator adapter over a RuleSpec.
//   - PheraldRules(): the catalogue of every pherald-owned §42.3 project rule.
//   - Pipeline: composes the catalogue into a Registry + a class-aware
//     evaluate→record→gate→emit→audit flow that drives REAL repo-safety + policy
//     events through the Batch-A emitter + persists them so they surface on a
//     future /v1/project pull surface.
//
// Why a pherald-local Pipeline (mirroring cherald HRD-019 + sherald HRD-020 +
// bherald HRD-021 + rherald HRD-022): the commons_constitution.Runner hardcodes
// the .policy.violation / .policy.cleared emit class. pherald binds the BULK of
// its rules to the repo-safety class — .repo.safety.breach — so the emit must be
// class-aware. The Pipeline reuses the Runner's exact gate semantics
// (transitions-only emission, allow/warn/enforce ladder, audit-on-warn-or-
// enforce, emitted-event-id capture) but routes the emit through the EventClass
// declared on each RuleSpec.
//
// §107 anti-bluff: a binding that registers but can never produce a queryable,
// audited project verdict is a PASS-bluff. Every Pipeline.EvaluateSubject call
// drives the REAL emitter + REAL store + REAL audit — the persisted
// side-effects ARE the positive runtime evidence (see bindings_test.go
// round-trip proofs).
//
// PURE detectors (CRITICAL §12 host-safety). Every CheckFunc is PURE: it
// CLASSIFIES a Subject string description of an already-observed project-op
// outcome (a commit entrypoint state, a submodule propagation order, a pre-push
// fetch state, a fetch-before-edit rebase state, a reopens-record presence). It
// NEVER commits, pushes, force-pushes, runs git, configures remotes, or touches
// the filesystem. The upstream §43 command bodies (HRD-029 commit-push, HRD-030
// submodule-propagate, HRD-043 install-upstreams, HRD-044 fetch-guard, HRD-049
// reopen, HRD-053 pre-push) supply the live project integration that feeds these
// detectors their Subjects.
package bindings

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/google/uuid"

	constitution "github.com/vasic-digital/herald/commons_constitution"
)

// Subject.Kind constants for pherald project subjects. The §43 command bodies
// (or a project gate) construct a Subject with one of these Kinds + an ID
// encoding the already-observed outcome; the binding CLASSIFIES it. The detector
// never performs the project op — it reads the recorded result.
const (
	// SubjectCommitPush: ID = "<sha>|entrypoint=<bool>|lock_held=<bool>" — §2
	// commit-push discipline (a commit made outside the single locked entrypoint,
	// or without the commit-lock held, is a §2 repo-safety breach).
	SubjectCommitPush = "commit-push"
	// SubjectSubmodulePropagate: ID =
	// "propagate|order=<inner-first|parent-first>|inner_pushed=<bool>" — §3
	// submodule propagation order (committing the parent before the inner
	// submodules, or pinning a not-yet-pushed inner SHA, is a §3 violation).
	SubjectSubmodulePropagate = "submodule-propagate"
	// SubjectFetchGuard: ID = "<branch>|rebased=<bool>" — §11.4.37 fetch-before-edit
	// (an edit on a tree not rebased on origin is a §11.4.37 repo-safety breach).
	SubjectFetchGuard = "fetch-guard"
	// SubjectPrePush: ID = "<branch>|fetched=<bool>|integrated=<bool>" — §11.4.71
	// pre-push fetch+investigate+integrate (a push without the pre-push fetch, or
	// without integrating incoming changes, is a §11.4.71 repo-safety breach).
	SubjectPrePush = "pre-push"
	// SubjectInstallUpstreams: ID = "<mirror-set>|configured=<n>|declared=<n>" —
	// §11.4.36 install-upstreams (fewer configured mirror remotes than declared
	// Upstreams/*.sh entries is a §11.4.36 policy violation).
	SubjectInstallUpstreams = "install-upstreams"
	// SubjectReopen: ID = "<HRD-NNN>|recorded=<bool>" — §11.4.55 reopens-history
	// (an Issues←Fixed reversal without a docs/Reopens/<HRD>.md record is a
	// §11.4.55 policy violation).
	SubjectReopen = "reopen"
)

// CheckFunc decides the project verdict for one Subject under one rule. It
// returns a Result whose Decision is PASS/WARN/FAIL plus an Evidence string + a
// stable DigestSHA used by the transition gate. CheckFunc MUST be deterministic
// for a given subject, safe for concurrent calls, and — for pherald — PURE: it
// classifies the recorded project-op outcome, it never performs the op.
type CheckFunc func(ctx context.Context, s constitution.Subject, bundle constitution.BundleHash) (constitution.Result, error)

// RuleSpec is the declarative description of one pherald-owned §42.3 project rule.
type RuleSpec struct {
	// RuleID is the canonical constitution section anchor, e.g. "§11.4.71".
	RuleID string
	// Title is the human-readable rule name from the Constitution ToC.
	Title string
	// Severity is the §42.3 default severity tier.
	Severity constitution.Severity
	// DefaultMode is the §42.3 default enforcement mode (the ladder seed).
	DefaultMode constitution.Mode
	// EventClass is the commons_constitution.Class* the emit routes through:
	// ClassRepoSafetyBreach (the repo-safety class — §2/§3/§11.4.37/§11.4.71) or
	// ClassPolicyViolation (the §11.4.36 install-upstreams + §11.4.55 reopens rows).
	EventClass string
	// BreachKind names the repo-safety breach kind for the SafetyEvent.BreachKind
	// payload ("commit-push" / "submodule-order" / "fetch-before-edit" /
	// "pre-push") so subscribers can disambiguate which project gate fired. For
	// the policy rows it doubles as the policy subject category.
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

// PushTriggers implements commons_constitution.Evaluator. pherald project
// bindings are driven by the project gate / §43 command bodies — none declare
// push-triggers in HRD-023. Returns nil.
func (b *Binding) PushTriggers() []string { return nil }

// Subjects implements commons_constitution.Evaluator. HRD-023 evaluates explicit
// subjects supplied by the caller (the project gate / §43 command bodies).
// Returns nil.
func (b *Binding) Subjects(_ context.Context, _ uuid.UUID) ([]constitution.Subject, error) {
	return nil, nil
}

// Evaluate implements commons_constitution.Evaluator by delegating to Check.
func (b *Binding) Evaluate(ctx context.Context, s constitution.Subject, bundle constitution.BundleHash) (constitution.Result, error) {
	return b.spec.Check(ctx, s, bundle)
}

// Config configures a Pipeline. All four backends are required (a nil backend is
// a §107 bluff — an emit/audit that goes nowhere).
type Config struct {
	Ladder  constitution.ModeLadder
	Store   constitution.ConstitutionStore
	Emitter constitution.EventEmitter
	Audit   constitution.AuditStore
	// Bundle is the constitution bundle hash carried on every emitted event for
	// replayability (§42.1.3). Zero value is a valid "no bundle" sentinel.
	Bundle constitution.BundleHash
}

// Pipeline composes the pherald rule catalogue into a Registry and drives the
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

// NewPipeline builds a Pipeline, registering every PheraldRules() entry into a
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
	for _, spec := range PheraldRules() {
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
// dispatches the emit through the per-rule project EventClass.
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

// emit routes the transition through the rule's declared EventClass. Returns the
// emitted event ID (for the audit row) when the class supports ID capture
// (policy class via IDEmitter); the repo-safety class returns uuid.Nil but still
// fans out the wire event.
//
// The Decision drives the breach-vs-recovered variant for the repo-safety class:
// a FAIL/ERROR transition is the repo-safety breach (.repo.safety.breach); a
// PASS/WARN transition in enforce mode is the recovery. The safety event-class
// family has no distinct "recovered" leaf, so a safety recovery maps to the
// shared .gate.recovered class — mirroring Runner.Run's cleared branch + the
// rherald release-recovery convention.
func (p *Pipeline) emit(ctx context.Context, spec RuleSpec, tenantID uuid.UUID, subject constitution.Subject, result constitution.Result, trans constitution.Transition) (uuid.UUID, error) {
	cleared := result.Decision == constitution.DecisionPass || result.Decision == constitution.DecisionWarn

	switch spec.EventClass {
	case constitution.ClassRepoSafetyBreach:
		if cleared {
			// A repo-safety gate that transitions back to PASS is a recovery; the
			// safety class has no distinct "recovered" leaf, so route through the
			// shared .gate.recovered companion class.
			return uuid.Nil, p.emitter.GateRecovered(ctx, p.gateEvent(spec, tenantID, result, trans))
		}
		return uuid.Nil, p.emitter.RepoSafetyBreach(ctx, constitution.SafetyEvent{
			TenantID:    tenantID,
			RuleID:      spec.RuleID,
			Severity:    spec.Severity,
			BreachKind:  p.breachKind(spec),
			Subject:     subject,
			Bundle:      p.bundle,
			Transition:  trans,
			EvidenceURI: result.Evidence,
		})
	default: // ClassPolicyViolation / ClassPolicyCleared — the §11.4.36 + §11.4.55 rows.
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

// gateEvent builds the GateEvent for the .gate.recovered recovery emission,
// carrying the rule's BreachKind as the GateName so subscribers can disambiguate
// which project gate recovered.
func (p *Pipeline) gateEvent(spec RuleSpec, tenantID uuid.UUID, result constitution.Result, trans constitution.Transition) constitution.GateEvent {
	name := spec.BreachKind
	if name == "" {
		name = spec.RuleID
	}
	return constitution.GateEvent{
		TenantID:    tenantID,
		RuleID:      spec.RuleID,
		Severity:    spec.Severity,
		GateName:    "project:" + name,
		Bundle:      p.bundle,
		Transition:  trans,
		EvidenceURI: result.Evidence,
	}
}

// breachKind derives the SafetyEvent.BreachKind from the rule's BreachKind name
// (the §42.2 safety-event payload field that names WHICH project gate breached).
func (p *Pipeline) breachKind(spec RuleSpec) string {
	if spec.BreachKind != "" {
		return spec.BreachKind
	}
	return spec.RuleID
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
