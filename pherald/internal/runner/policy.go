package runner

import (
	"context"
	"strings"

	constitution "github.com/vasic-digital/herald/commons_constitution"
)

// PolicyGate is Stage 4 — runs every Evaluator in the Registry whose
// PushTriggers include the inbound event Type. The first non-Pass
// verdict wins (Fail > Warn > Pass), so a single Fail short-circuits.
//
// Wave 3b default is permissive: if no Evaluator is registered for the
// event type, DecisionPass is set and the pipeline continues. Tightening
// to fail-closed is a future HRD (operators can register a "deny by
// default" evaluator if they want that posture).
type PolicyGate struct {
	Registry *constitution.Registry
}

func (g *PolicyGate) Process(ctx context.Context, rc *RunCtx) error {
	if g.Registry == nil || g.Registry.Len() == 0 {
		rc.PolicyDecision = constitution.DecisionPass
		return nil
	}
	// Find evaluators whose PushTriggers include this event Type.
	worst := constitution.DecisionPass
	var reason string
	for _, ev := range g.Registry.All() {
		triggers := ev.PushTriggers()
		match := false
		for _, t := range triggers {
			if strings.EqualFold(t, rc.Event.Type) {
				match = true
				break
			}
		}
		if !match {
			continue
		}
		subjects, err := ev.Subjects(ctx, rc.TenantID)
		if err != nil {
			// Per §107 honesty: an evaluator error is recorded as DecisionError
			// rather than silently swallowed (a silent swallow would let
			// faulty evaluators bypass enforcement).
			worst = constitution.DecisionError
			reason = "evaluator " + ev.RuleID() + " Subjects() failed: " + err.Error()
			continue
		}
		for _, s := range subjects {
			res, err := ev.Evaluate(ctx, s, constitution.BundleHash{})
			if err != nil {
				worst = constitution.DecisionError
				reason = "evaluator " + ev.RuleID() + " Evaluate() failed: " + err.Error()
				continue
			}
			if res.Decision > worst {
				worst = res.Decision
				reason = ev.RuleID() + ": " + res.Evidence
			}
		}
	}
	rc.PolicyDecision = worst
	rc.PolicyReason = reason
	return nil
}
