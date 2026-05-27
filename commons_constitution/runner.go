package constitution

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Runner is the per-evaluation orchestrator. It composes the Evaluator,
// the ModeLadder, the ConstitutionStore, and the EventEmitter into the
// canonical [6]→[7]→[8] flow from Foundation design §3.1:
//
//   evaluate → record → if changed: gate by mode → audit + (optional) emit.
//
// Runner is the only consumer of the panic-isolation contract: bad
// evaluators are caught here, never propagate to other evaluators, and
// surface as DecisionError + an emitted SafetyEvent.
type Runner struct {
	registry *Registry
	ladder   ModeLadder
	store    ConstitutionStore
	emitter  EventEmitter
	audit    AuditStore
}

// NewRunner returns a configured Runner. All dependencies are required.
//
// The AuditStore is load-bearing per HRD-018: every CHANGED transition in
// ModeWarn or ModeEnforce writes a durable constitution_audit row. Prior to
// HRD-018 RunOutcome.Audited was set true but nothing was persisted — a §107
// PASS-bluff at the audit layer. Passing a nil AuditStore is a hard error.
func NewRunner(reg *Registry, ladder ModeLadder, store ConstitutionStore, emitter EventEmitter, audit AuditStore) (*Runner, error) {
	if reg == nil {
		return nil, fmt.Errorf("constitution: NewRunner: nil Registry")
	}
	if ladder == nil {
		return nil, fmt.Errorf("constitution: NewRunner: nil ModeLadder")
	}
	if store == nil {
		return nil, fmt.Errorf("constitution: NewRunner: nil ConstitutionStore")
	}
	if emitter == nil {
		return nil, fmt.Errorf("constitution: NewRunner: nil EventEmitter")
	}
	if audit == nil {
		return nil, fmt.Errorf("constitution: NewRunner: nil AuditStore")
	}
	return &Runner{registry: reg, ladder: ladder, store: store, emitter: emitter, audit: audit}, nil
}

// RunOutcome reports what Runner.Run did for a single (evaluator, subject)
// pair. Useful for tests + the M3 /v1/events response body.
type RunOutcome struct {
	Evaluator   string
	Subject     Subject
	Decision    Decision
	Mode        Mode
	Transition  Transition
	Emitted     bool   // true iff a channel emit fired
	Audited     bool   // true iff an audit row was written (Warn or Enforce)
	PanicValue  string // populated iff the evaluator panicked
}

// Run executes one (evaluator, subject) pair through the canonical flow.
//
// Failure modes (per design §4):
//
//   - Evaluator panics → recover + RunOutcome{Decision: DecisionError,
//                          PanicValue: <stringified>}. NO emit, NO audit;
//                          the runner itself surfaces the panic via the
//                          returned RunOutcome.
//   - ModeLadder.Get returns err → propagated (defence-in-depth: fail loud).
//   - Store.Record returns err → propagated.
//   - Emitter returns err → propagated *after* state was recorded.
//
// The transition gate is checked AFTER Record returns — so even
// transitions-not-emitted (mode=allow + no-change) are persisted, satisfying
// the "we always know what state every (tenant, rule, subject) is in"
// guarantee from spec §42.1.2.
func (r *Runner) Run(
	ctx context.Context,
	e Evaluator,
	tenantID uuid.UUID,
	subject Subject,
	bundle BundleHash,
) (out RunOutcome, err error) {
	out.Evaluator = e.RuleID()
	out.Subject = subject

	// Step 1: evaluate inside a panic-recovery wrapper.
	result, panicValue := safeEvaluate(ctx, e, subject, bundle)
	if panicValue != "" {
		out.PanicValue = panicValue
		out.Decision = DecisionError
		// Record the error state so future runs see the regression.
		errResult := Result{
			Decision:  DecisionError,
			Evidence:  "evaluator panic: " + panicValue,
			DigestSHA: sha256.Sum256([]byte("panic:" + panicValue)),
		}
		trans, recErr := r.store.Record(ctx, tenantID, e.RuleID(), subject.ID, errResult, bundle, errResult.Evidence)
		if recErr != nil {
			return out, fmt.Errorf("constitution: Run: record-after-panic: %w", recErr)
		}
		out.Transition = trans
		return out, nil // panic does NOT propagate
	}
	out.Decision = result.Decision

	// Step 2: persist + compute transition.
	trans, err := r.store.Record(ctx, tenantID, e.RuleID(), subject.ID, result, bundle, result.Evidence)
	if err != nil {
		return out, fmt.Errorf("constitution: Run: record: %w", err)
	}
	out.Transition = trans

	// Transitions-only emission discipline (§42.2).
	if !trans.Changed {
		return out, nil
	}

	// Step 3: gate by mode.
	mode, err := r.ladder.Get(ctx, tenantID, e.RuleID())
	if err != nil {
		return out, fmt.Errorf("constitution: Run: ladder-get: %w", err)
	}
	out.Mode = mode

	switch mode {
	case ModeAllow:
		// recorded; no audit; no emit.
		return out, nil
	case ModeWarn:
		out.Audited = true
		// Audit-only: durable row written, but no channel emit (no event ID).
		if err := r.writeAudit(ctx, tenantID, e.RuleID(), subject.ID, result, bundle, trans, ModeWarn, uuid.Nil); err != nil {
			return out, fmt.Errorf("constitution: Run: audit(warn): %w", err)
		}
		return out, nil
	case ModeEnforce:
		out.Audited = true
		out.Emitted = true
		policyEvent := PolicyEvent{
			TenantID:    tenantID,
			RuleID:      e.RuleID(),
			Severity:    e.Severity(),
			Subject:     subject,
			Bundle:      bundle,
			Transition:  trans,
			EvidenceURI: result.Evidence,
		}
		// Emit + capture the event ID so the audit row records the EXACT
		// emitted_event_id (load-bearing for replay correlation, §42.1.3).
		var (
			emittedID uuid.UUID
			emitErr   error
		)
		ide, hasID := r.emitter.(IDEmitter)
		switch result.Decision {
		case DecisionPass, DecisionWarn:
			if hasID {
				emittedID, emitErr = ide.PolicyClearedID(ctx, policyEvent)
			} else {
				emitErr = r.emitter.PolicyCleared(ctx, policyEvent)
			}
		default:
			if hasID {
				emittedID, emitErr = ide.PolicyViolationID(ctx, policyEvent)
			} else {
				emitErr = r.emitter.PolicyViolation(ctx, policyEvent)
			}
		}
		if emitErr != nil {
			return out, fmt.Errorf("constitution: Run: emit: %w", emitErr)
		}
		// Write the audit row AFTER a successful emit so emitted_event_id is
		// only recorded for events that actually reached the bus.
		if err := r.writeAudit(ctx, tenantID, e.RuleID(), subject.ID, result, bundle, trans, ModeEnforce, emittedID); err != nil {
			return out, fmt.Errorf("constitution: Run: audit(enforce): %w", err)
		}
		return out, nil
	default:
		return out, fmt.Errorf("constitution: Run: unknown mode %v", mode)
	}
}

// writeAudit composes + appends one constitution_audit row for a CHANGED
// transition. OldDecision/OldDigest are nil on FirstSeen (no prior verdict).
func (r *Runner) writeAudit(
	ctx context.Context,
	tenantID uuid.UUID,
	ruleID, subject string,
	result Result,
	bundle BundleHash,
	trans Transition,
	mode Mode,
	emittedID uuid.UUID,
) error {
	row := AuditRow{
		TenantID:       tenantID,
		RuleID:         ruleID,
		Subject:        subject,
		NewDecision:    result.Decision,
		NewDigest:      result.DigestSHA,
		BundleHash:     bundle,
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
	_, err := r.audit.RecordAudit(ctx, row)
	return err
}

// safeEvaluate runs e.Evaluate inside a recover() wrapper. If the
// evaluator panics, returns (zero Result, stringified panic value).
// If the evaluator returns an error, returns (DecisionError, "").
func safeEvaluate(ctx context.Context, e Evaluator, s Subject, bundle BundleHash) (r Result, panicValue string) {
	defer func() {
		if p := recover(); p != nil {
			panicValue = fmt.Sprintf("%v", p)
		}
	}()
	res, err := e.Evaluate(ctx, s, bundle)
	if err != nil {
		// Translate evaluator-returned errors to DecisionError. Distinguish
		// from panics by keeping panicValue empty.
		res = Result{
			Decision:  DecisionError,
			Evidence:  "evaluator error: " + err.Error(),
			DigestSHA: sha256.Sum256([]byte("err:" + err.Error())),
		}
	}
	return res, ""
}

// ErrCancelled wraps context.Canceled with a Runner-specific message.
var ErrCancelled = errors.New("constitution: Runner: context cancelled")
