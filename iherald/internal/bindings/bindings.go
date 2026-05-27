// Package bindings wires iherald's §42.3-owned incident/escalation constitution
// rules into the Batch-A commons_constitution foundation (Evaluator + Registry +
// ModeLadder + ConstitutionStore + AuditStore + the typed event emitter).
//
// HRD-024 (v1.0.0 Batch B, unit 6) — iherald is the INCIDENT / escalation
// flavor. Per spec §42.3 (master binding table rows §11.4.10, §11.4.10.A,
// §11.4.21, §11.4.66) and §42.5 step 7 it owns the page-out / escalation rule
// rows whose verdicts fan out as .credential.leak events (the §11.4.10 /
// §11.4.10.A credential-leak page-outs) and .policy.violation events (the
// §11.4.21 operator-blocked escalation + §11.4.66 blocker-resolution
// clarification). This package supplies:
//
//   - RuleSpec: the declarative description of one iherald-owned §42.3
//     incident/escalation rule (its §42.3 RuleID, default Severity + Mode, the
//     emitted EventClass, the EscalationKind for the event payload, and a
//     deterministic CheckFunc that CLASSIFIES a recorded incident/escalation
//     signal).
//   - Binding: a commons_constitution.Evaluator adapter over a RuleSpec.
//   - IheraldRules(): the catalogue of every iherald-owned §42.3 escalation rule.
//   - Pipeline: composes the catalogue into a Registry + a class-aware
//     evaluate→record→gate→emit→audit flow that drives REAL escalation events
//     through the Batch-A emitter + persists them so they surface on a future
//     /v1/webhooks/page pull surface.
//
// Why an iherald-local Pipeline (mirroring cherald HRD-019 + sherald HRD-020 +
// bherald HRD-021 + rherald HRD-022): the commons_constitution.Runner hardcodes
// the .policy.violation / .policy.cleared emit class. iherald binds its
// credential-leak rows to the CREDENTIAL class — .credential.leak — and its
// operator-blocked rows to the POLICY class, so the emit must be class-aware.
// The Pipeline reuses the Runner's exact gate semantics (transitions-only
// emission, allow/warn/enforce ladder, audit-on-warn-or-enforce, emitted-event-
// id capture) but routes the emit through the EventClass declared on each
// RuleSpec.
//
// §107 anti-bluff: a binding that registers but can never produce a queryable,
// audited escalation verdict is a PASS-bluff. Every Pipeline.EvaluateSubject
// call drives the REAL emitter + REAL store + REAL audit — the persisted
// side-effects ARE the positive runtime evidence (see bindings_test.go
// round-trip proofs). A credential-leak signal that "evaluated" but never paged
// out (no .credential.leak event, no audit row) is precisely the metadata-only
// PASS the §11.4.10 credentials covenant forbids.
//
// PURE detectors (CRITICAL §12 host-safety). Every CheckFunc is PURE: it
// CLASSIFIES a Subject string description of an already-observed incident
// signal — a credential-leak detection outcome, an operator-blocked status
// transition, a blocker-without-clarification flag, an incident-severity
// routing decision. It NEVER scans a real .env, reads a real secret, pages a
// real on-call, runs git, or touches the filesystem. The live paging integration
// (the /v1/webhooks/page handler body + the §43 escalation command bodies) is
// scope-locked to the HRD-024-paging follow-ups; this package supplies the
// classification + emit + audit vertical slice that those upstream bodies feed.
package bindings

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/google/uuid"

	constitution "github.com/vasic-digital/herald/commons_constitution"
)

// Subject.Kind constants for iherald incident/escalation subjects. The
// /v1/webhooks/page handler (or a §43 escalation command body) constructs a
// Subject with one of these Kinds + an ID encoding the already-observed signal;
// the binding CLASSIFIES it. The detector never performs the page-out — it reads
// the recorded signal.
const (
	// SubjectCredentialLeak: ID = "<location>|leaked=<bool>[|kind=<env|source|log|url>][|origin=<...>]"
	// — §11.4.10 credential-handling page-out (a detected plaintext credential /
	// tracked .env triggers a critical page-out).
	SubjectCredentialLeak = "credential-leak"
	// SubjectPreStoreAudit: ID = "<store-op>|audited=<bool>[|leaked=<bool>]" —
	// §11.4.10.A pre-store leak audit (a credential-storage commit that skipped
	// its pre-store leak audit, OR whose audit found a leak, triggers a page-out).
	SubjectPreStoreAudit = "pre-store-audit"
	// SubjectOperatorBlocked: ID = "<item>|status=<status>[|oncall_paged=<bool>]"
	// — §11.4.21 operator-blocked escalation (a workable item that enters
	// operator-blocked WITHOUT the on-call page is an escalation gap).
	SubjectOperatorBlocked = "operator-blocked"
	// SubjectBlockerClarification: ID = "<item>|blocked=<bool>|clarification=<bool>"
	// — §11.4.66 blocker-resolution clarification (an operator-blocked item with
	// no clarification prompt queued is a §11.4.66 violation).
	SubjectBlockerClarification = "blocker-clarification"
	// SubjectIncidentSeverity: ID = "<incident>|severity=<sev1|sev2|sev3|sev4>[|paged=<bool>]"
	// — incident-severity routing (bespoke iherald detector): a high-severity
	// incident (sev1/sev2) that was NOT paged out is a routing failure.
	SubjectIncidentSeverity = "incident-severity"
)

// CheckFunc decides the escalation verdict for one Subject under one rule. It
// returns a Result whose Decision is PASS/WARN/FAIL plus an Evidence string + a
// stable DigestSHA used by the transition gate. CheckFunc MUST be deterministic
// for a given subject, safe for concurrent calls, and — for iherald — PURE: it
// classifies the recorded incident signal, it never performs the page-out.
type CheckFunc func(ctx context.Context, s constitution.Subject, bundle constitution.BundleHash) (constitution.Result, error)

// RuleSpec is the declarative description of one iherald-owned §42.3
// incident/escalation rule.
type RuleSpec struct {
	// RuleID is the canonical constitution section anchor, e.g. "§11.4.10".
	RuleID string
	// Title is the human-readable rule name from the Constitution ToC.
	Title string
	// Severity is the §42.3 default severity tier.
	Severity constitution.Severity
	// DefaultMode is the §42.3 default enforcement mode (the ladder seed).
	DefaultMode constitution.Mode
	// EventClass is the commons_constitution.Class* the emit routes through:
	// ClassCredentialLeak (the §11.4.10 / §11.4.10.A page-out class) or
	// ClassPolicyViolation (the §11.4.21 operator-blocked + §11.4.66
	// clarification rows + the bespoke incident-severity routing row).
	EventClass string
	// EscalationKind names the escalation surface for the event payload
	// ("credential-leak" / "pre-store-audit" / "operator-blocked" /
	// "blocker-clarification" / "incident-severity") so paging subscribers can
	// disambiguate which escalation fired.
	EscalationKind string
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

// PushTriggers implements commons_constitution.Evaluator. iherald escalation
// bindings are driven by the page webhook / §43 escalation command bodies —
// none declare push-triggers in HRD-024. Returns nil.
func (b *Binding) PushTriggers() []string { return nil }

// Subjects implements commons_constitution.Evaluator. HRD-024 evaluates
// explicit subjects supplied by the caller (the page webhook / §43 escalation
// command bodies). Returns nil.
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

// Pipeline composes the iherald rule catalogue into a Registry and drives the
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

// NewPipeline builds a Pipeline, registering every IheraldRules() entry into a
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
	for _, spec := range IheraldRules() {
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
// dispatches the emit through the per-rule incident/escalation EventClass.
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
// (the policy class via IDEmitter); the credential class returns uuid.Nil but
// still fans out the wire event.
//
// The Decision drives the breach-vs-cleared variant:
//   - ClassCredentialLeak: a FAIL/ERROR transition is the credential page-out
//     (.credential.leak); a PASS/WARN transition in enforce mode is a recovery.
//     The credential class has no distinct "cleared" leaf, so a recovery maps to
//     the shared .gate.recovered companion class — mirroring Runner.Run's
//     cleared branch + bherald's RepoSafetyBreach recovery handling.
//   - ClassPolicyViolation: a FAIL/ERROR transition is the escalation
//     (.policy.violation); a PASS/WARN transition is the .policy.cleared
//     companion. Event ID captured via the optional IDEmitter surface.
func (p *Pipeline) emit(ctx context.Context, spec RuleSpec, tenantID uuid.UUID, subject constitution.Subject, result constitution.Result, trans constitution.Transition) (uuid.UUID, error) {
	cleared := result.Decision == constitution.DecisionPass || result.Decision == constitution.DecisionWarn

	switch spec.EventClass {
	case constitution.ClassCredentialLeak:
		if cleared {
			// A credential-leak page-out that transitions back to PASS is a
			// recovery; the credential class has no distinct "cleared" leaf, so
			// route through the shared .gate.recovered companion class.
			return uuid.Nil, p.emitter.GateRecovered(ctx, p.recoveryEvent(spec, tenantID, result, trans))
		}
		// CredentialLeak is forced critical inside the emitter regardless of the
		// per-rule severity (§11.4.10 / §11.4.10.A are always critical page-outs).
		return uuid.Nil, p.emitter.CredentialLeak(ctx, constitution.CredentialEvent{
			TenantID:     tenantID,
			RuleID:       spec.RuleID,
			Subject:      subject,
			Bundle:       p.bundle,
			Transition:   trans,
			EvidenceURI:  result.Evidence,
			SourceOrigin: "rule:" + spec.RuleID + "/" + p.escalationKind(spec),
		})
	default: // ClassPolicyViolation / ClassPolicyCleared — operator-blocked + clarification + incident-severity rows.
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

// recoveryEvent builds the GateEvent for the .gate.recovered recovery emission
// used by the credential class (which has no distinct "cleared" leaf), carrying
// the rule's EscalationKind as the GateName so subscribers can disambiguate
// which escalation recovered.
func (p *Pipeline) recoveryEvent(spec RuleSpec, tenantID uuid.UUID, result constitution.Result, trans constitution.Transition) constitution.GateEvent {
	return constitution.GateEvent{
		TenantID:    tenantID,
		RuleID:      spec.RuleID,
		Severity:    spec.Severity,
		GateName:    "escalation:" + p.escalationKind(spec),
		Bundle:      p.bundle,
		Transition:  trans,
		EvidenceURI: result.Evidence,
	}
}

// escalationKind returns the rule's EscalationKind, defaulting to the RuleID
// when unset so the payload always carries a non-empty disambiguator.
func (p *Pipeline) escalationKind(spec RuleSpec) string {
	if spec.EscalationKind != "" {
		return spec.EscalationKind
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
