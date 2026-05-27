// Package bindings wires cherald's §42.3-owned constitution rules into the
// Batch-A commons_constitution foundation (Evaluator + Registry + Runner +
// ConstitutionStore + ModeLadder + AuditStore + bundle-hash captureer).
//
// HRD-019 (Batch B, unit 1) — cherald is the COMPLIANCE flavor. Per spec
// §42.5 step 2 it "gets the bulk of bindings" (~30 rules). This package
// supplies:
//
//   - RuleSpec: the declarative description of one cherald-owned rule (its
//     §42.3 RuleID, default Severity + Mode, the emitted EventClass, and the
//     CheckFunc that decides PASS/WARN/FAIL for a Subject).
//   - Binding: a commons_constitution.Evaluator adapter over a RuleSpec.
//   - CheraldRules(): the catalogue of every cherald-owned §42.3 rule.
//   - Pipeline: composes the catalogue into a Registry + a class-aware
//     evaluate→record→gate→emit→audit flow that drives REAL events through the
//     Batch-A emitter + persists them so they surface on /v1/compliance.
//
// Why a cherald-local Pipeline instead of commons_constitution.Runner: the
// Runner hardcodes the .policy.violation / .policy.cleared emit class (it is
// the generic policy path). cherald binds rules to FIVE event classes —
// .policy.violation, .gate.failed, .gate.recovered, .credential.leak,
// .spec.revision_drift, .catalogue.miss — so the emit must be class-aware. The
// Pipeline reuses the Runner's exact gate semantics (transitions-only emission,
// allow/warn/enforce ladder, audit-on-warn-or-enforce, emitted-event-id capture)
// but routes the emit through the EventClass declared on each RuleSpec.
//
// §107 anti-bluff: a binding that registers but can never produce a queryable,
// audited violation is a PASS-bluff. Every Pipeline.EvaluateSubject call drives
// the REAL emitter + REAL store + REAL audit — the persisted side-effects ARE
// the positive runtime evidence (see bindings_test.go round-trip proofs).
package bindings

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/google/uuid"

	constitution "github.com/vasic-digital/herald/commons_constitution"
)

// CheckFunc decides the compliance verdict for one Subject under one rule.
// It returns a Result whose Decision is PASS/WARN/FAIL (or ERROR/SKIP) plus an
// Evidence string + a stable DigestSHA used by the transition gate. CheckFunc
// MUST be deterministic for a given (subject, bundle) and safe for concurrent
// calls — the Pipeline may evaluate many subjects in parallel.
type CheckFunc func(ctx context.Context, s constitution.Subject, bundle constitution.BundleHash) (constitution.Result, error)

// RuleSpec is the declarative description of one cherald-owned §42.3 rule.
type RuleSpec struct {
	// RuleID is the canonical constitution section anchor, e.g. "§11.4.29".
	RuleID string
	// Title is the human-readable rule name from the Constitution ToC.
	Title string
	// Severity is the §42.3 default severity tier.
	Severity constitution.Severity
	// DefaultMode is the §42.3 default enforcement mode. The runtime ladder
	// (constitution_bindings) may override it per tenant; this is the seed.
	DefaultMode constitution.Mode
	// EventClass is the commons_constitution.Class* the emit routes through
	// when this rule transitions in warn/enforce mode. One of the §42.2 classes.
	EventClass string
	// Check decides the verdict. Never nil for a shipped rule.
	Check CheckFunc
	// SubjectKinds documents what Subject.Kind values this rule evaluates
	// (informational — used by the catalogue + future sweep wiring).
	SubjectKinds []string
}

// Binding adapts a RuleSpec to the commons_constitution.Evaluator interface so
// it can register into the shared Registry + flow through the standard sweep.
type Binding struct {
	spec RuleSpec
}

// NewBinding wraps a RuleSpec. Panics on an invalid spec (nil Check / empty
// RuleID / empty EventClass) — those are programming errors caught at startup,
// never runtime conditions (mirrors Registry.Register's fail-fast posture).
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

// Spec returns the underlying RuleSpec (read-only view for callers).
func (b *Binding) Spec() RuleSpec { return b.spec }

// RuleID implements commons_constitution.Evaluator.
func (b *Binding) RuleID() string { return b.spec.RuleID }

// Severity implements commons_constitution.Evaluator.
func (b *Binding) Severity() constitution.Severity { return b.spec.Severity }

// PushTriggers implements commons_constitution.Evaluator. cherald bindings are
// pull/sweep-driven (the /v1/compliance surface + periodic sweep); none declare
// push-triggers in HRD-019. Returns nil.
func (b *Binding) PushTriggers() []string { return nil }

// Subjects implements commons_constitution.Evaluator. The HRD-019 surface
// evaluates explicit subjects supplied by the caller (the REST surface / a
// sweep walking changed files), so the default Subjects enumeration is empty —
// callers drive EvaluateSubject directly. A future sweep (HRD-025 scherald)
// may supply a real enumerator.
func (b *Binding) Subjects(_ context.Context, _ uuid.UUID) ([]constitution.Subject, error) {
	return nil, nil
}

// Evaluate implements commons_constitution.Evaluator by delegating to the
// RuleSpec's CheckFunc.
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
	// replayability (§42.1.3). Zero value is permitted (an all-zeros hash is a
	// valid "no bundle captured" sentinel; production wires a real Captureer).
	Bundle constitution.BundleHash
}

// Pipeline composes the cherald rule catalogue into a Registry and drives the
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

// NewPipeline builds a Pipeline, registering every CheraldRules() entry into a
// fresh Registry. Returns an error if any backend is nil or a duplicate rule ID
// is present in the catalogue.
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
	for _, spec := range CheraldRules() {
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
// This mirrors commons_constitution.Runner.Run's gate semantics exactly, but
// dispatches the emit through the per-rule EventClass (Runner hardcodes the
// policy class). Returns the RunOutcome describing what happened.
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
// the emitted event ID (for the audit row's emitted_event_id column) when the
// class supports ID capture (policy.violation / .cleared via the IDEmitter
// surface); other classes return uuid.Nil but still fan out the wire event.
//
// The Decision drives the cleared-vs-violation variant for the policy class
// (a PASS/WARN transition in enforce mode is a "cleared", a FAIL/ERROR is a
// "violation") — mirroring Runner.Run's branch.
func (p *Pipeline) emit(ctx context.Context, spec RuleSpec, tenantID uuid.UUID, subject constitution.Subject, result constitution.Result, trans constitution.Transition) (uuid.UUID, error) {
	cleared := result.Decision == constitution.DecisionPass || result.Decision == constitution.DecisionWarn

	switch spec.EventClass {
	case constitution.ClassCredentialLeak:
		return uuid.Nil, p.emitter.CredentialLeak(ctx, constitution.CredentialEvent{
			TenantID:     tenantID,
			RuleID:       spec.RuleID,
			Subject:      subject,
			Bundle:       p.bundle,
			Transition:   trans,
			EvidenceURI:  result.Evidence,
			SourceOrigin: "rule:" + spec.RuleID,
		})
	case constitution.ClassGateFailed:
		if cleared {
			return uuid.Nil, p.emitter.GateRecovered(ctx, p.gateEvent(spec, tenantID, result, trans))
		}
		return uuid.Nil, p.emitter.GateFailed(ctx, p.gateEvent(spec, tenantID, result, trans))
	case constitution.ClassGateRecovered:
		return uuid.Nil, p.emitter.GateRecovered(ctx, p.gateEvent(spec, tenantID, result, trans))
	case constitution.ClassSpecRevisionDrift:
		return uuid.Nil, p.emitter.SpecRevisionDrift(ctx, constitution.DriftEvent{
			TenantID:    tenantID,
			RuleID:      spec.RuleID,
			SpecPath:    subject.ID,
			Bundle:      p.bundle,
			Transition:  trans,
			EvidenceURI: result.Evidence,
		})
	case constitution.ClassCatalogueMiss:
		return uuid.Nil, p.emitter.CatalogueMiss(ctx, constitution.CatalogueEvent{
			TenantID:   tenantID,
			RuleID:     spec.RuleID,
			Severity:   spec.Severity,
			MissingRef: subject.ID,
			Bundle:     p.bundle,
		})
	default: // ClassPolicyViolation / ClassPolicyCleared — the generic path.
		policyEvent := constitution.PolicyEvent{
			TenantID:    tenantID,
			RuleID:      spec.RuleID,
			Severity:    spec.Severity,
			Subject:     subject,
			Bundle:      p.bundle,
			Transition:  trans,
			EvidenceURI: result.Evidence,
		}
		// Capture the emitted event ID via the optional IDEmitter surface so
		// the audit row records the EXACT id that fanned out (§42.1.3).
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
// commons_constitution.safeEvaluate (which is unexported) so the Pipeline
// honors the same panic-isolation contract: a panicking binding never
// propagates, and an error-returning binding surfaces as DecisionError.
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
