// Package bindings wires rherald's §42.3-owned release gate constitution rules
// into the Batch-A commons_constitution foundation (Evaluator + Registry +
// ModeLadder + ConstitutionStore + AuditStore + the typed event emitter).
//
// HRD-022 (v1.0.0 Batch B, unit 4) — rherald is the RELEASE flavor. Per spec
// §42.3 it owns the release-lifecycle rule rows (§4 tag mirroring, §5 changelog
// + multi-format export [shared with cherald], §11.4.38 installable-asset
// evidence, §11.4.40 full-suite retest before release tag) whose verdicts fan
// out as .release.gate.blocked events (plus .policy.violation for the shared §5
// changelog row). This package supplies:
//
//   - RuleSpec: the declarative description of one rherald-owned §42.3 release
//     rule (its §42.3 RuleID, default Severity + Mode, the emitted EventClass,
//     the ReleaseGate name for the event payload, and a deterministic CheckFunc
//     that CLASSIFIES a recorded release-op outcome).
//   - Binding: a commons_constitution.Evaluator adapter over a RuleSpec.
//   - RheraldRules(): the catalogue of every rherald-owned §42.3 release rule.
//   - Pipeline: composes the catalogue into a Registry + a class-aware
//     evaluate→record→gate→emit→audit flow that drives REAL release-gate events
//     through the Batch-A emitter + persists them so they surface on a future
//     /v1/release pull surface.
//
// Why a rherald-local Pipeline (mirroring cherald HRD-019 + sherald HRD-020 +
// bherald HRD-021): the commons_constitution.Runner hardcodes the
// .policy.violation / .policy.cleared emit class. rherald binds the BULK of its
// rules to the RELEASE gate class — .release.gate.blocked — so the emit must be
// class-aware. The Pipeline reuses the Runner's exact gate semantics
// (transitions-only emission, allow/warn/enforce ladder, audit-on-warn-or-
// enforce, emitted-event-id capture) but routes the emit through the EventClass
// declared on each RuleSpec.
//
// §107 anti-bluff: a binding that registers but can never produce a queryable,
// audited release verdict is a PASS-bluff. Every Pipeline.EvaluateSubject call
// drives the REAL emitter + REAL store + REAL audit — the persisted
// side-effects ARE the positive runtime evidence (see bindings_test.go
// round-trip proofs).
//
// PURE detectors (CRITICAL §12 host-safety). Every CheckFunc is PURE: it
// CLASSIFIES a Subject string description of an already-observed release-op
// outcome (a tag-mirror parity tally, a changelog conformance flag, a retest
// outcome, an install-check result). It NEVER tags, pushes, force-pushes, runs
// git, downloads an asset, or touches the filesystem. The upstream §43 command
// bodies (HRD-031 tag-mirror, HRD-032 changelog-generate, HRD-045 gate-retest)
// supply the live release integration that feeds these detectors their
// Subjects.
package bindings

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/google/uuid"

	constitution "github.com/vasic-digital/herald/commons_constitution"
)

// Subject.Kind constants for rherald release subjects. The §43 command bodies
// (or a release gate) construct a Subject with one of these Kinds + an ID
// encoding the already-observed outcome; the binding CLASSIFIES it. The
// detector never performs the release op — it reads the recorded result.
const (
	// SubjectTagMirror: ID = "<tag>|tag=<present|absent>|mirrors=<n>|with_tag=<n>"
	// — §4 tag-mirror parity (a tag present on the parent but missing on any
	// owned mirror is a §4 violation).
	SubjectTagMirror = "tag-mirror"
	// SubjectChangelog: ID = "<version>|conforming=<bool>[|export_stale=<bool>]"
	// — §5 changelog conformance (a non-Conventional-Commits changelog or a
	// stale multi-format export is a §5 policy violation).
	SubjectChangelog = "changelog"
	// SubjectInstallAsset: ID = "<asset-name>|installed=<bool>" — §11.4.38
	// installable-asset evidence (a release asset that fails its install check
	// blocks the release).
	SubjectInstallAsset = "install-asset"
	// SubjectRetestGate: ID = "<tag>|retest=<green|red|skipped>[|tiers=<n>]" —
	// §11.4.40 pre-tag full-suite retest (a tag attempted without a green
	// all-tier retest blocks the release).
	SubjectRetestGate = "retest-gate"
)

// CheckFunc decides the release verdict for one Subject under one rule. It
// returns a Result whose Decision is PASS/WARN/FAIL plus an Evidence string + a
// stable DigestSHA used by the transition gate. CheckFunc MUST be deterministic
// for a given subject, safe for concurrent calls, and — for rherald — PURE: it
// classifies the recorded release-op outcome, it never performs the op.
type CheckFunc func(ctx context.Context, s constitution.Subject, bundle constitution.BundleHash) (constitution.Result, error)

// RuleSpec is the declarative description of one rherald-owned §42.3 release rule.
type RuleSpec struct {
	// RuleID is the canonical constitution section anchor, e.g. "§11.4.40".
	RuleID string
	// Title is the human-readable rule name from the Constitution ToC.
	Title string
	// Severity is the §42.3 default severity tier.
	Severity constitution.Severity
	// DefaultMode is the §42.3 default enforcement mode (the ladder seed).
	DefaultMode constitution.Mode
	// EventClass is the commons_constitution.Class* the emit routes through:
	// ClassReleaseGateBlocked (the release-gate class — §4/§11.4.38/§11.4.40) or
	// ClassPolicyViolation (the shared §5 changelog row).
	EventClass string
	// ReleaseGate names the release gate for the ReleaseEvent.Reason payload
	// ("tag-mirror" / "changelog" / "install-asset" / "pre-tag-retest") so
	// subscribers can disambiguate which release gate fired.
	ReleaseGate string
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

// PushTriggers implements commons_constitution.Evaluator. rherald release
// bindings are driven by the release gate / §43 command bodies — none declare
// push-triggers in HRD-022. Returns nil.
func (b *Binding) PushTriggers() []string { return nil }

// Subjects implements commons_constitution.Evaluator. HRD-022 evaluates
// explicit subjects supplied by the caller (the release gate / §43 command
// bodies). Returns nil.
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

// Pipeline composes the rherald rule catalogue into a Registry and drives the
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

// NewPipeline builds a Pipeline, registering every RheraldRules() entry into a
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
	for _, spec := range RheraldRules() {
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
// dispatches the emit through the per-rule release EventClass.
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
// (policy class via IDEmitter); the release class returns uuid.Nil but still
// fans out the wire event.
//
// The Decision drives the blocked-vs-recovered variant for the release class: a
// FAIL/ERROR transition is the release-gate block (.release.gate.blocked); a
// PASS/WARN transition in enforce mode is the recovery. The release event-class
// family has no distinct "recovered" leaf, so a release recovery maps to the
// shared .gate.recovered class — mirroring Runner.Run's cleared branch.
func (p *Pipeline) emit(ctx context.Context, spec RuleSpec, tenantID uuid.UUID, subject constitution.Subject, result constitution.Result, trans constitution.Transition) (uuid.UUID, error) {
	cleared := result.Decision == constitution.DecisionPass || result.Decision == constitution.DecisionWarn

	switch spec.EventClass {
	case constitution.ClassReleaseGateBlocked:
		if cleared {
			// A release gate that transitions back to PASS is a recovery; the
			// release class has no distinct "recovered" leaf, so route through the
			// shared .gate.recovered companion class.
			return uuid.Nil, p.emitter.GateRecovered(ctx, p.gateEvent(spec, tenantID, result, trans))
		}
		return uuid.Nil, p.emitter.ReleaseGateBlocked(ctx, constitution.ReleaseEvent{
			TenantID:    tenantID,
			RuleID:      spec.RuleID,
			Severity:    spec.Severity,
			ReleaseRef:  releaseRef(subject),
			Reason:      p.releaseReason(spec, result),
			Bundle:      p.bundle,
			Transition:  trans,
			EvidenceURI: result.Evidence,
		})
	default: // ClassPolicyViolation / ClassPolicyCleared — the shared §5 changelog row.
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
// carrying the rule's ReleaseGate name as the GateName so subscribers can
// disambiguate which release gate recovered.
func (p *Pipeline) gateEvent(spec RuleSpec, tenantID uuid.UUID, result constitution.Result, trans constitution.Transition) constitution.GateEvent {
	name := spec.ReleaseGate
	if name == "" {
		name = spec.RuleID
	}
	return constitution.GateEvent{
		TenantID:    tenantID,
		RuleID:      spec.RuleID,
		Severity:    spec.Severity,
		GateName:    "release:" + name,
		Bundle:      p.bundle,
		Transition:  trans,
		EvidenceURI: result.Evidence,
	}
}

// releaseReason derives the ReleaseEvent.Reason from the rule's release gate
// name (the §42.2 release-event payload field that names WHY the gate blocked).
func (p *Pipeline) releaseReason(spec RuleSpec, _ constitution.Result) string {
	if spec.ReleaseGate != "" {
		return spec.ReleaseGate + "-block"
	}
	return spec.RuleID + "-block"
}

// releaseRef extracts the release ref (tag / version) from the subject ID's head
// segment so the ReleaseEvent.ReleaseRef carries it. The head is the first
// "|"-delimited segment without an "=" (the tag/version description). Falls back
// to the whole Subject.ID when no clean head is present.
func releaseRef(subject constitution.Subject) string {
	head, _ := subjectFields(subject.ID)
	if head != "" {
		return head
	}
	return subject.ID
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
