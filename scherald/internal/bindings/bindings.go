// Package bindings wires scherald's §42.3-owned scheduled-audit constitution
// rules into the Batch-A commons_constitution foundation (Evaluator + Registry +
// ModeLadder + ConstitutionStore + AuditStore + the typed event emitter).
//
// HRD-025 (v1.0.0 Batch B, unit 7) — scherald is the SCHEDULED-AUDIT flavor.
// Per spec §42.3 master binding table, scherald is the default owner of the
// §11.4.45 Integration-Status-Doc Maintenance row (`.policy.violation`, warn,
// low — "Status.md stale; composes with §37"). The §43 command catalogue
// (HRD-047 `scherald status digest [--cadence=daily|weekly|monthly]`) couples
// that row with the §11.4.56 Status_Summary parity facet for the periodic
// Status.md sweep + Status_Summary.md regen + daily/weekly/monthly compliance
// digest. This package supplies:
//
//   - RuleSpec: the declarative description of one scherald-owned §42.3
//     scheduled-audit rule (its RuleID, default Severity + Mode, the emitted
//     EventClass, a scheduled-audit Kind label, and a deterministic CheckFunc
//     that CLASSIFIES a recorded sweep / digest-cadence / staleness outcome).
//   - Binding: a commons_constitution.Evaluator adapter over a RuleSpec.
//   - ScheraldRules(): the catalogue of every scherald-owned §42.3 rule.
//   - Pipeline: composes the catalogue into a Registry + a class-aware
//     evaluate→record→gate→emit→audit flow that drives REAL policy.violation
//     events through the Batch-A emitter + persists them so they surface on a
//     future /v1/schedule pull surface.
//
// Why a scherald-local Pipeline (mirroring cherald HRD-019 + sherald HRD-020 +
// bherald HRD-021 + rherald HRD-022): the commons_constitution.Runner hardcodes
// the .policy.violation / .policy.cleared emit class, which IS scherald's
// class — but scherald binds its rules through the Pipeline so its emit path is
// uniform with the rest of the §42.3 flavor family and so a future scherald
// rule that needs a non-policy class (e.g. a `.gate.failed` digest-build
// failure) can route through the same seam. The Pipeline reuses the Runner's
// exact gate semantics (transitions-only emission, allow/warn/enforce ladder,
// audit-on-warn-or-enforce, emitted-event-id capture).
//
// §107 anti-bluff: a binding that registers but can never produce a queryable,
// audited scheduled-audit verdict is a PASS-bluff. Every Pipeline.EvaluateSubject
// call drives the REAL emitter + REAL store + REAL audit — the persisted
// side-effects ARE the positive runtime evidence (see bindings_test.go
// round-trip proofs).
//
// PURE detectors (CRITICAL §12 host-safety). Every CheckFunc is PURE: it
// CLASSIFIES a Subject string description of an already-observed scheduled-audit
// outcome (a Status.md sweep finding, a digest-cadence tally, a stale-item
// count). It NEVER reads Status.md, runs a cron tick, regenerates a digest, or
// touches the filesystem / process table / network. The live cron / scheduler
// integration that feeds these detectors their Subjects is scope-locked to the
// §43 / HRD-047 follow-up (`scherald status digest` + `POST /v1/schedule/
// status-digest`).
package bindings

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/google/uuid"

	constitution "github.com/vasic-digital/herald/commons_constitution"
)

// Subject.Kind constants for scherald scheduled-audit subjects. The §43 command
// body (HRD-047) or the scheduler constructs a Subject with one of these Kinds +
// an ID encoding the already-observed outcome; the binding CLASSIFIES it. The
// detector never performs the sweep / digest — it reads the recorded result.
const (
	// SubjectStatusSweep: ID =
	//   "<doc>|sweep=<clean|stale>[|stale_items=<n>][|summary_synced=<bool>]"
	// — §11.4.45 periodic Status.md sweep. A sweep that found Status.md stale
	// (or its Status_Summary.md derivative out of sync) is a policy violation.
	SubjectStatusSweep = "status-sweep"
	// SubjectDigestCadence: ID =
	//   "<cadence>|due=<bool>|emitted=<bool>[|overdue_by_h=<n>]"
	// — periodic compliance-digest cadence (daily / weekly / monthly). A digest
	// that fell DUE but was never emitted (a missed scheduled digest) is a
	// policy violation against the §18.7 digest cadence contract.
	SubjectDigestCadence = "digest-cadence"
	// SubjectStaleItem: ID =
	//   "<scope>|stale_items=<n>[|threshold=<n>]"
	// — stale-work-item detection across the HRD trackers (open items with no
	// status movement past the configured threshold). A count above threshold
	// is a policy violation surfaced by the periodic audit sweep.
	SubjectStaleItem = "stale-item"
)

// CheckFunc decides the scheduled-audit verdict for one Subject under one rule.
// It returns a Result whose Decision is PASS/WARN/FAIL plus an Evidence string +
// a stable DigestSHA used by the transition gate. CheckFunc MUST be
// deterministic for a given subject, safe for concurrent calls, and — for
// scherald — PURE: it classifies the recorded sweep / digest / staleness
// outcome, it never performs the sweep, runs cron, or reads a doc.
type CheckFunc func(ctx context.Context, s constitution.Subject, bundle constitution.BundleHash) (constitution.Result, error)

// RuleSpec is the declarative description of one scherald-owned §42.3 rule.
type RuleSpec struct {
	// RuleID is the canonical constitution section anchor, e.g. "§11.4.45".
	RuleID string
	// Title is the human-readable rule name from the Constitution ToC.
	Title string
	// Severity is the §42.3 default severity tier.
	Severity constitution.Severity
	// DefaultMode is the §42.3 default enforcement mode (the ladder seed).
	DefaultMode constitution.Mode
	// EventClass is the commons_constitution.Class* the emit routes through. For
	// scherald this is ClassPolicyViolation (the scheduled-audit class per the
	// §42.3 master table).
	EventClass string
	// AuditGate names the scheduled-audit gate for subscriber disambiguation
	// ("status-sweep" / "digest-cadence" / "stale-item").
	AuditGate string
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

// PushTriggers implements commons_constitution.Evaluator. scherald
// scheduled-audit bindings are driven by the scheduler / §43 command body — none
// declare push-triggers in HRD-025. Returns nil.
func (b *Binding) PushTriggers() []string { return nil }

// Subjects implements commons_constitution.Evaluator. HRD-025 evaluates explicit
// subjects supplied by the caller (the scheduler / §43 command body). Returns
// nil.
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

// Pipeline composes the scherald rule catalogue into a Registry and drives the
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

// NewPipeline builds a Pipeline, registering every ScheraldRules() entry into a
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
	for _, spec := range ScheraldRules() {
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
// dispatches the emit through the per-rule scheduled-audit EventClass.
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

// emit routes the transition through the rule's declared EventClass. For
// scherald every rule binds to the .policy.violation / .policy.cleared family.
// A FAIL/ERROR transition is a policy violation; a PASS/WARN transition in
// enforce mode is the clearance. Returns the emitted event ID (for the audit
// row) via the optional IDEmitter surface so the audit records the EXACT id
// that fanned out (§42.1.3).
func (p *Pipeline) emit(ctx context.Context, spec RuleSpec, tenantID uuid.UUID, subject constitution.Subject, result constitution.Result, trans constitution.Transition) (uuid.UUID, error) {
	cleared := result.Decision == constitution.DecisionPass || result.Decision == constitution.DecisionWarn

	switch spec.EventClass {
	case constitution.ClassPolicyViolation, constitution.ClassPolicyCleared:
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
	default:
		return uuid.Nil, fmt.Errorf("bindings: emit: rule %q has unsupported event class %q", spec.RuleID, spec.EventClass)
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
