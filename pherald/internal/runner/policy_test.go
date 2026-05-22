package runner

import (
	"context"
	"testing"

	"github.com/google/uuid"

	constitution "github.com/vasic-digital/herald/commons_constitution"
)

func TestPolicyGate_NoEvaluator_Permissive(t *testing.T) {
	// Wave 3b default: no evaluator registered for this event type → continue.
	reg := constitution.NewRegistry()
	g := &PolicyGate{Registry: reg}
	rc := &RunCtx{
		Event: cloudEventStub("evt-1", "digital.vasic.herald.unknown"),
	}
	if err := g.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if rc.PolicyDecision != constitution.DecisionPass {
		t.Errorf("PolicyDecision = %s, want Pass (permissive default)", rc.PolicyDecision)
	}
}

// fakeEvaluator returns a configured Decision for any subject.
type fakeEvaluator struct {
	ruleID   string
	severity constitution.Severity
	triggers []string
	verdict  constitution.Decision
	reason   string
}

func (f *fakeEvaluator) RuleID() string                  { return f.ruleID }
func (f *fakeEvaluator) Severity() constitution.Severity { return f.severity }
func (f *fakeEvaluator) PushTriggers() []string          { return f.triggers }
func (f *fakeEvaluator) Subjects(ctx context.Context, tid uuid.UUID) ([]constitution.Subject, error) {
	return []constitution.Subject{{Kind: "test", ID: "x"}}, nil
}
func (f *fakeEvaluator) Evaluate(ctx context.Context, s constitution.Subject, b constitution.BundleHash) (constitution.Result, error) {
	return constitution.Result{Decision: f.verdict, Evidence: f.reason}, nil
}

func TestPolicyGate_EvaluatorAllow_Continues(t *testing.T) {
	reg := constitution.NewRegistry()
	reg.Register(&fakeEvaluator{
		ruleID: "11.4.99", severity: constitution.SeverityLow,
		triggers: []string{"digital.vasic.herald.test"},
		verdict:  constitution.DecisionPass,
	})
	g := &PolicyGate{Registry: reg}
	rc := &RunCtx{Event: cloudEventStub("e1", "digital.vasic.herald.test")}
	if err := g.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if rc.PolicyDecision != constitution.DecisionPass {
		t.Errorf("DecisionPass expected; got %s", rc.PolicyDecision)
	}
}

func TestPolicyGate_EvaluatorFail_DenyShortCircuit(t *testing.T) {
	reg := constitution.NewRegistry()
	reg.Register(&fakeEvaluator{
		ruleID: "11.4.10", severity: constitution.SeverityCritical,
		triggers: []string{"digital.vasic.herald.test"},
		verdict:  constitution.DecisionFail,
		reason:   "credential-leak detected in body",
	})
	g := &PolicyGate{Registry: reg}
	rc := &RunCtx{Event: cloudEventStub("e1", "digital.vasic.herald.test")}
	if err := g.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if rc.PolicyDecision != constitution.DecisionFail {
		t.Errorf("DecisionFail expected; got %s", rc.PolicyDecision)
	}
	if rc.PolicyReason == "" {
		t.Errorf("PolicyReason empty on Fail (operator can't see why)")
	}
}
