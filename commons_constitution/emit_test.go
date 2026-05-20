package constitution

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func newTestEmitter(t *testing.T, bus EventBus) EventEmitter {
	t.Helper()
	em, err := NewEmitter(bus, EmitterConfig{
		Source: "digital.vasic.herald/test",
		Now:    func() time.Time { return time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC) },
		NewID:  func() string { return "ce-fixture-id" },
	})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}
	return em
}

func TestNewEmitter_RejectsBadConfig(t *testing.T) {
	if _, err := NewEmitter(nil, EmitterConfig{Source: "x"}); err == nil {
		t.Error("nil bus did not error")
	}
	bus := NewMemoryBus(MemoryBusConfig{})
	defer bus.Close()
	if _, err := NewEmitter(bus, EmitterConfig{}); err == nil {
		t.Error("empty Source did not error")
	}
}

// TestAll12ClassesEmitOnCorrectType is the anti-bluff backbone of the emit
// suite: every helper MUST actually publish on its declared CloudEvents type.
func TestAll12ClassesEmitOnCorrectType(t *testing.T) {
	bus := NewMemoryBus(MemoryBusConfig{})
	defer bus.Close()
	em := newTestEmitter(t, bus)
	sub, _ := bus.Subscribe("*")
	defer sub.Cancel()

	tenant := uuid.New()
	bundle := CaptureBytes([]byte("v1"))
	tr := Transition{Changed: true, NewDecision: DecisionFail, OldDecision: DecisionPass}
	ctx := context.Background()

	// Fire one of each.
	cases := []struct {
		class string
		fire  func() error
	}{
		{ClassGateFailed, func() error {
			return em.GateFailed(ctx, GateEvent{TenantID: tenant, RuleID: "§1", Bundle: bundle, Transition: tr, GateName: "ci-test"})
		}},
		{ClassGateRecovered, func() error {
			return em.GateRecovered(ctx, GateEvent{TenantID: tenant, RuleID: "§1", Bundle: bundle, Transition: tr, GateName: "ci-test"})
		}},
		{ClassPolicyViolation, func() error {
			return em.PolicyViolation(ctx, PolicyEvent{TenantID: tenant, RuleID: "§11.4.10", Bundle: bundle, Transition: tr, Subject: Subject{Kind: "file", ID: "/x"}})
		}},
		{ClassPolicyCleared, func() error {
			return em.PolicyCleared(ctx, PolicyEvent{TenantID: tenant, RuleID: "§11.4.10", Bundle: bundle, Transition: tr})
		}},
		{ClassHostSafetyBreach, func() error {
			return em.HostSafetyBreach(ctx, SafetyEvent{TenantID: tenant, RuleID: "§9.1", Bundle: bundle, Transition: tr, BreachKind: "destructive-op"})
		}},
		{ClassRepoSafetyBreach, func() error {
			return em.RepoSafetyBreach(ctx, SafetyEvent{TenantID: tenant, RuleID: "§12.1", Bundle: bundle, Transition: tr, BreachKind: "force-push"})
		}},
		{ClassCredentialLeak, func() error {
			return em.CredentialLeak(ctx, CredentialEvent{TenantID: tenant, RuleID: "§11.4.10", Bundle: bundle, Transition: tr})
		}},
		{ClassBundleUpdated, func() error {
			return em.BundleUpdated(ctx, BundleEvent{TenantID: tenant, NewBundle: bundle, Transition: tr})
		}},
		{ClassBundleUpdateFailed, func() error {
			return em.BundleUpdateFailed(ctx, BundleEvent{TenantID: tenant, NewBundle: bundle, Transition: tr, Error: "permission denied"})
		}},
		{ClassReleaseGateBlocked, func() error {
			return em.ReleaseGateBlocked(ctx, ReleaseEvent{TenantID: tenant, RuleID: "§11.4.65", Bundle: bundle, Transition: tr, ReleaseRef: "v1.4.0", Reason: "no-slsa"})
		}},
		{ClassSpecRevisionDrift, func() error {
			return em.SpecRevisionDrift(ctx, DriftEvent{TenantID: tenant, RuleID: "§11.4.73", Bundle: bundle, Transition: tr, SpecPath: "docs/specs/mvp/specification.V3.md", OldRevision: "5", NewRevision: "6"})
		}},
		{ClassCatalogueMiss, func() error {
			return em.CatalogueMiss(ctx, CatalogueEvent{TenantID: tenant, RuleID: "§11.4.74", Bundle: bundle, MissingRef: "<ancestor>/constitution/Constitution.md"})
		}},
	}

	for _, c := range cases {
		if err := c.fire(); err != nil {
			t.Errorf("%s emit returned error: %v", c.class, err)
		}
	}

	// Anti-bluff: drain bus and assert exactly 12 events with the right types.
	deadline := time.After(time.Second)
	got := make(map[string]int, 12)
	for len(got) < 12 {
		select {
		case e := <-sub.Channel:
			got[e.Type]++
		case <-deadline:
			t.Fatalf("did not receive 12 events; got %d: %v", len(got), got)
		}
	}

	for _, class := range AllClasses() {
		want := EventNamespace + "." + class
		if got[want] != 1 {
			t.Errorf("type %s delivered %d times; want 1", want, got[want])
		}
	}
}

func TestEmit_EnvelopePopulated(t *testing.T) {
	bus := NewMemoryBus(MemoryBusConfig{})
	defer bus.Close()
	em := newTestEmitter(t, bus)
	sub, _ := bus.Subscribe("*")
	defer sub.Cancel()

	tenant := uuid.MustParse("00000000-0000-7000-8000-000000000001")
	bundle := CaptureBytes([]byte("v1"))
	tr := Transition{Changed: true, OldDecision: DecisionPass, NewDecision: DecisionFail}

	_ = em.PolicyViolation(context.Background(), PolicyEvent{
		TenantID: tenant, RuleID: "§11.4.10", Severity: SeverityCritical, Bundle: bundle, Transition: tr,
		Subject: Subject{Kind: "file", ID: "/etc/passwd"}, EvidenceURI: "evidence://x",
	})

	select {
	case e := <-sub.Channel:
		var body struct {
			Envelope envelope        `json:"envelope"`
			Payload  json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(e.Data, &body); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if body.Envelope.RuleID != "§11.4.10" {
			t.Errorf("envelope rule_id = %q; want §11.4.10", body.Envelope.RuleID)
		}
		if body.Envelope.SeverityCategory != "critical" {
			t.Errorf("severity = %q; want critical", body.Envelope.SeverityCategory)
		}
		if body.Envelope.DecisionResult != "fail" {
			t.Errorf("decision_result = %q; want fail", body.Envelope.DecisionResult)
		}
		if body.Envelope.TransitionFrom != "pass" || body.Envelope.TransitionTo != "fail" {
			t.Errorf("transition direction wrong: %q→%q", body.Envelope.TransitionFrom, body.Envelope.TransitionTo)
		}
		if !strings.HasPrefix(body.Envelope.BundleHash, "") || len(body.Envelope.BundleHash) != 64 {
			t.Errorf("bundle_hash should be 64-char hex; got %q", body.Envelope.BundleHash)
		}
		if body.Envelope.EvidenceURI != "evidence://x" {
			t.Errorf("evidence_uri = %q; want evidence://x", body.Envelope.EvidenceURI)
		}
		if e.Metadata["rule_id"] != "§11.4.10" {
			t.Errorf("event metadata rule_id = %q; want §11.4.10", e.Metadata["rule_id"])
		}
	case <-time.After(time.Second):
		t.Fatal("event not received within 1s")
	}
}

func TestEmit_CredentialLeakIsAlwaysCritical(t *testing.T) {
	// Anti-bluff: credential-leak severity is hard-coded to Critical
	// regardless of input.
	bus := NewMemoryBus(MemoryBusConfig{})
	defer bus.Close()
	em := newTestEmitter(t, bus)
	sub, _ := bus.Subscribe("*")
	defer sub.Cancel()

	tenant := uuid.New()
	bundle := CaptureBytes([]byte("v1"))
	_ = em.CredentialLeak(context.Background(), CredentialEvent{TenantID: tenant, RuleID: "§9.1", Bundle: bundle})

	select {
	case e := <-sub.Channel:
		if e.Metadata["severity"] != "critical" {
			t.Errorf("CredentialLeak severity = %q; want critical (always)", e.Metadata["severity"])
		}
	case <-time.After(time.Second):
		t.Fatal("credential-leak event not received")
	}
}
