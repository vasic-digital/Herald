package constitution

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// envelope is the three-axis (rule_id, severity_category, decision_result)
// metadata carried on every emitted event per spec §42.1.1.
type envelope struct {
	RuleID            string `json:"rule_id"`
	SeverityCategory string `json:"severity_category"`
	DecisionResult   string `json:"decision_result"`
	BundleHash       string `json:"bundle_hash"`
	TraceParent      string `json:"traceparent,omitempty"`
	EvidenceURI      string `json:"evidence_uri,omitempty"`
	TenantID         string `json:"tenant_id"`
	TransitionFrom   string `json:"transition_from,omitempty"`
	TransitionTo     string `json:"transition_to,omitempty"`
}

// GateEvent carries data for .gate.failed + .gate.recovered (CI / pre-commit
// gate emissions). Maps to spec §42.2 class "gate".
type GateEvent struct {
	TenantID    uuid.UUID
	RuleID      string
	Severity    Severity
	GateName    string // "test-suite" / "lint" / "security-scan" / "type-check"
	Bundle      BundleHash
	Transition  Transition
	TraceParent string
	EvidenceURI string
}

// PolicyEvent carries data for .policy.violation + .policy.cleared.
type PolicyEvent struct {
	TenantID    uuid.UUID
	RuleID      string
	Severity    Severity
	Subject     Subject
	Bundle      BundleHash
	Transition  Transition
	TraceParent string
	EvidenceURI string
}

// SafetyEvent carries data for .host.safety.breach + .repo.safety.breach.
type SafetyEvent struct {
	TenantID    uuid.UUID
	RuleID      string
	Severity    Severity
	BreachKind  string // "destructive-op" / "force-push" / "mem-budget" / "unauthorised-rm"
	Subject     Subject
	Bundle      BundleHash
	Transition  Transition
	TraceParent string
	EvidenceURI string
}

// CredentialEvent carries data for .credential.leak.
type CredentialEvent struct {
	TenantID    uuid.UUID
	RuleID      string
	Subject     Subject
	Bundle      BundleHash
	Transition  Transition
	TraceParent string
	EvidenceURI string
	SourceOrigin string // "stdin" / "rule:11.4.10" / "rule:9.1" - what tripped detection
}

// BundleEvent carries data for .bundle.updated + .bundle.update.failed.
type BundleEvent struct {
	TenantID    uuid.UUID
	OldBundle   BundleHash
	NewBundle   BundleHash
	Transition  Transition
	TraceParent string
	Error       string // populated for .bundle.update.failed
}

// ReleaseEvent carries data for .release.gate.blocked.
type ReleaseEvent struct {
	TenantID    uuid.UUID
	RuleID      string
	Severity    Severity
	ReleaseRef  string // "v1.4.0"
	Reason      string // "no-slsa-attestation" / "changelog-missing" / ...
	Bundle      BundleHash
	Transition  Transition
	TraceParent string
	EvidenceURI string
}

// DriftEvent carries data for .spec.revision.drift (when a spec doc
// changed without the corresponding work landing per §11.4.73).
type DriftEvent struct {
	TenantID    uuid.UUID
	RuleID      string
	SpecPath    string // "docs/specs/mvp/specification.V3.md"
	OldRevision string
	NewRevision string
	Bundle      BundleHash
	Transition  Transition
	TraceParent string
	EvidenceURI string
}

// CatalogueEvent carries data for .catalogue.miss (when an evaluator's
// required bundle / reference doc is unreadable).
type CatalogueEvent struct {
	TenantID    uuid.UUID
	RuleID      string
	Severity    Severity
	MissingRef  string // "docs/specs/mvp/specification.V3.md" / "<ancestor>/constitution/Constitution.md"
	Bundle      BundleHash
	TraceParent string
}

// EventEmitter is the typed emit surface — every class has a method.
// Tests assert "emitter received exactly N events of class X" rather than
// inspecting raw bus events.
type EventEmitter interface {
	GateFailed(ctx context.Context, e GateEvent) error
	GateRecovered(ctx context.Context, e GateEvent) error
	PolicyViolation(ctx context.Context, e PolicyEvent) error
	PolicyCleared(ctx context.Context, e PolicyEvent) error
	HostSafetyBreach(ctx context.Context, e SafetyEvent) error
	RepoSafetyBreach(ctx context.Context, e SafetyEvent) error
	CredentialLeak(ctx context.Context, e CredentialEvent) error
	BundleUpdated(ctx context.Context, e BundleEvent) error
	BundleUpdateFailed(ctx context.Context, e BundleEvent) error
	ReleaseGateBlocked(ctx context.Context, e ReleaseEvent) error
	SpecRevisionDrift(ctx context.Context, e DriftEvent) error
	CatalogueMiss(ctx context.Context, e CatalogueEvent) error
}

// IDEmitter is the optional capability that lets the Runner capture the
// generated event ID of a policy emit for the constitution_audit
// emitted_event_id column. busEmitter satisfies it; Runner.Run
// type-asserts for it and falls back to a plain emit (Nil event ID in the
// audit row) if a custom EventEmitter does not implement it.
type IDEmitter interface {
	PolicyViolationID(ctx context.Context, e PolicyEvent) (uuid.UUID, error)
	PolicyClearedID(ctx context.Context, e PolicyEvent) (uuid.UUID, error)
}

// busEmitter wraps an EventBus + a Source identifier and synthesises
// canonical Events for the 12 classes.
type busEmitter struct {
	bus    EventBus
	source string
	now    func() time.Time
	newID  func() string
}

// EmitterConfig configures a busEmitter. Zero values pick defaults.
type EmitterConfig struct {
	Source string         // "digital.vasic.herald/cherald" etc; required.
	Now    func() time.Time // default time.Now (UTC)
	NewID  func() string    // default UUIDv7-via-google/uuid
}

// NewEmitter returns an EventEmitter that publishes to bus using the
// canonical ce-source = cfg.Source and ce-type = EventNamespace + "." + class.
func NewEmitter(bus EventBus, cfg EmitterConfig) (EventEmitter, error) {
	if bus == nil {
		return nil, fmt.Errorf("constitution: NewEmitter: nil bus")
	}
	if cfg.Source == "" {
		return nil, fmt.Errorf("constitution: NewEmitter: empty Source")
	}
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	if cfg.NewID == nil {
		cfg.NewID = func() string { return uuid.NewString() }
	}
	return &busEmitter{bus: bus, source: cfg.Source, now: cfg.Now, newID: cfg.NewID}, nil
}

// --- typed emit methods ----------------------------------------------------

func (b *busEmitter) GateFailed(ctx context.Context, e GateEvent) error {
	_, err := b.emit(ctx, ClassGateFailed, e.TenantID, e.RuleID, e.Severity, e.Bundle, e.Transition, e.TraceParent, e.EvidenceURI, "gate:"+e.GateName, e)
	return err
}

func (b *busEmitter) GateRecovered(ctx context.Context, e GateEvent) error {
	_, err := b.emit(ctx, ClassGateRecovered, e.TenantID, e.RuleID, e.Severity, e.Bundle, e.Transition, e.TraceParent, e.EvidenceURI, "gate:"+e.GateName, e)
	return err
}

func (b *busEmitter) PolicyViolation(ctx context.Context, e PolicyEvent) error {
	_, err := b.PolicyViolationID(ctx, e)
	return err
}

func (b *busEmitter) PolicyCleared(ctx context.Context, e PolicyEvent) error {
	_, err := b.PolicyClearedID(ctx, e)
	return err
}

// PolicyViolationID / PolicyClearedID are the ID-returning variants used by
// Runner.Run so the constitution_audit row can record the EXACT event ID
// that fanned out to channels (the load-bearing emitted_event_id audit
// column). They satisfy the optional IDEmitter interface; busEmitter is the
// only EventEmitter implementation, so the Runner's type-assertion always
// succeeds — but keeping the public EventEmitter surface unchanged avoids
// rippling the signature into every (current + future) caller.
func (b *busEmitter) PolicyViolationID(ctx context.Context, e PolicyEvent) (uuid.UUID, error) {
	return b.emit(ctx, ClassPolicyViolation, e.TenantID, e.RuleID, e.Severity, e.Bundle, e.Transition, e.TraceParent, e.EvidenceURI, e.Subject.String(), e)
}

func (b *busEmitter) PolicyClearedID(ctx context.Context, e PolicyEvent) (uuid.UUID, error) {
	return b.emit(ctx, ClassPolicyCleared, e.TenantID, e.RuleID, e.Severity, e.Bundle, e.Transition, e.TraceParent, e.EvidenceURI, e.Subject.String(), e)
}

func (b *busEmitter) HostSafetyBreach(ctx context.Context, e SafetyEvent) error {
	_, err := b.emit(ctx, ClassHostSafetyBreach, e.TenantID, e.RuleID, e.Severity, e.Bundle, e.Transition, e.TraceParent, e.EvidenceURI, e.Subject.String(), e)
	return err
}

func (b *busEmitter) RepoSafetyBreach(ctx context.Context, e SafetyEvent) error {
	_, err := b.emit(ctx, ClassRepoSafetyBreach, e.TenantID, e.RuleID, e.Severity, e.Bundle, e.Transition, e.TraceParent, e.EvidenceURI, e.Subject.String(), e)
	return err
}

func (b *busEmitter) CredentialLeak(ctx context.Context, e CredentialEvent) error {
	// Credential leak is always critical regardless of severity-on-evaluator.
	_, err := b.emit(ctx, ClassCredentialLeak, e.TenantID, e.RuleID, SeverityCritical, e.Bundle, e.Transition, e.TraceParent, e.EvidenceURI, e.Subject.String(), e)
	return err
}

func (b *busEmitter) BundleUpdated(ctx context.Context, e BundleEvent) error {
	_, err := b.emit(ctx, ClassBundleUpdated, e.TenantID, "bundle", SeverityMiddle, e.NewBundle, e.Transition, e.TraceParent, "", "bundle:rotation", e)
	return err
}

func (b *busEmitter) BundleUpdateFailed(ctx context.Context, e BundleEvent) error {
	_, err := b.emit(ctx, ClassBundleUpdateFailed, e.TenantID, "bundle", SeverityHigh, e.NewBundle, e.Transition, e.TraceParent, "", "bundle:rotation", e)
	return err
}

func (b *busEmitter) ReleaseGateBlocked(ctx context.Context, e ReleaseEvent) error {
	_, err := b.emit(ctx, ClassReleaseGateBlocked, e.TenantID, e.RuleID, e.Severity, e.Bundle, e.Transition, e.TraceParent, e.EvidenceURI, "release:"+e.ReleaseRef, e)
	return err
}

func (b *busEmitter) SpecRevisionDrift(ctx context.Context, e DriftEvent) error {
	_, err := b.emit(ctx, ClassSpecRevisionDrift, e.TenantID, e.RuleID, SeverityMiddle, e.Bundle, e.Transition, e.TraceParent, e.EvidenceURI, "spec:"+e.SpecPath, e)
	return err
}

func (b *busEmitter) CatalogueMiss(ctx context.Context, e CatalogueEvent) error {
	_, err := b.emit(ctx, ClassCatalogueMiss, e.TenantID, e.RuleID, e.Severity, e.Bundle, Transition{}, e.TraceParent, "", "catalogue:"+e.MissingRef, e)
	return err
}

// emit is the single Event-composition path. JSON-encodes the per-class
// payload as the Event.Data so subscribers can deserialise according to
// the ce-type. Returns the generated event ID so callers (Runner.Run) can
// record it in the constitution_audit emitted_event_id column.
func (b *busEmitter) emit(
	ctx context.Context,
	class string,
	tenant uuid.UUID,
	ruleID string,
	sev Severity,
	bundle BundleHash,
	trans Transition,
	traceParent, evidence, subject string,
	payload any,
) (uuid.UUID, error) {
	env := envelope{
		RuleID:           ruleID,
		SeverityCategory: sev.String(),
		DecisionResult:   trans.NewDecision.String(),
		BundleHash:       bundle.Hex(),
		TraceParent:      traceParent,
		EvidenceURI:      evidence,
		TenantID:         tenant.String(),
	}
	if trans.Changed {
		env.TransitionFrom = trans.OldDecision.String()
		env.TransitionTo = trans.NewDecision.String()
	}
	body := struct {
		Envelope envelope `json:"envelope"`
		Payload  any      `json:"payload"`
	}{Envelope: env, Payload: payload}
	data, err := json.Marshal(body)
	if err != nil {
		return uuid.Nil, fmt.Errorf("constitution: emit %s: marshal: %w", class, err)
	}
	eventID := b.newID()
	ev := Event{
		ID:      eventID,
		Type:    EventNamespace + "." + class,
		Source:  b.source,
		Time:    b.now(),
		Subject: subject,
		Metadata: map[string]string{
			"rule_id":           ruleID,
			"severity":          sev.String(),
			"tenant_id":         tenant.String(),
			"bundle_hash":       bundle.Hex(),
			"decision_result":   trans.NewDecision.String(),
			"first_seen":        boolStr(trans.FirstSeen),
		},
		Data: data,
	}
	if err := b.bus.Publish(ctx, ev); err != nil {
		return uuid.Nil, err
	}
	// Parse the string event ID back to a UUID for the audit trail. The
	// default NewID is uuid.NewString so this always succeeds; a custom
	// non-UUID NewID degrades gracefully to uuid.Nil (publish already
	// succeeded — the wire event is unaffected).
	id, perr := uuid.Parse(eventID)
	if perr != nil {
		return uuid.Nil, nil
	}
	return id, nil
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
