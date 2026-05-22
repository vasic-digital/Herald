package runner

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/vasic-digital/herald/commons"
	constitution "github.com/vasic-digital/herald/commons_constitution"
)

// integrationDeps groups the fake dependencies for the orchestrator-level
// integration tests below. Each test builds a fresh Runner via
// newIntegrationRunner so per-test state never leaks.
type integrationDeps struct {
	redis    *fakeRedis
	procd    *fakeEventsProcessedStore
	subs     *fakeSubscribersStore
	chans    map[commons.ChannelID]commons.Channel
	registry *constitution.Registry
	evid     *fakeEvidenceStore
	null     *fakeChannel
}

// newIntegrationRunner builds a Runner with all-fake deps (no real PG,
// no real Redis). Stages are wired by field assignment — bypasses
// NewRunner's real-adapter wiring so the fakes are used directly.
func newIntegrationRunner() (*Runner, *integrationDeps) {
	nullCh := newFakeChannel("null")
	d := &integrationDeps{
		redis:    newFakeRedis(),
		procd:    newFakeEventsProcessedStore(),
		subs:     newFakeSubscribersStore(),
		chans:    map[commons.ChannelID]commons.Channel{commons.ChannelNull: nullCh},
		registry: constitution.NewRegistry(),
		evid:     newFakeEvidenceStore(),
		null:     nullCh,
	}
	r := &Runner{
		parser:  &EventParser{},
		idem:    &IdempotencyChecker{Redis: d.redis, PG: d.procd, TTL: 24 * time.Hour},
		tenant:  &TenantResolver{},
		policy:  &PolicyGate{Registry: d.registry},
		subs:    &SubscriberResolver{Subscribers: d.subs},
		chans:   &ChannelDispatcher{Channels: d.chans},
		outcome: &OutcomeRecorder{Evidence: d.evid, EventsProcessed: d.procd},
	}
	return r, d
}

// TestRunner_HappyPath_FullPipeline drives all 7 stages on a fresh event
// with one registered subscriber. §107 evidence: exactly one
// outbound_delivery_evidence row must be written (proves Stage 7 ran)
// and the Receipt's Recipients count must be 1 (proves Stage 6 saw the
// single registered subscriber).
func TestRunner_HappyPath_FullPipeline(t *testing.T) {
	tenantID := mustParse("55555555-5555-5555-5555-555555555555")
	r, d := newIntegrationRunner()
	d.subs.Add(tenantID, subscriberRow{
		ID:     uuid.New(),
		Handle: "alice",
		Aliases: []subscriberAliasRow{
			{Channel: "null", ChannelUserID: "sandbox-alice"},
		},
	})

	body := mustJSON(map[string]any{
		"specversion": "1.0",
		"id":          "01923456-789a-7bcd-abcd-ef0123456789",
		"source":      "//test",
		"type":        "digital.vasic.herald.test",
		"data":        map[string]string{"hi": "there"},
	})
	rcpt, err := r.Run(context.Background(), body, map[string]any{"tenant": tenantID.String()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rcpt == nil {
		t.Fatalf("Run returned nil Receipt")
	}
	if rcpt.Recipients != 1 {
		t.Errorf("Recipients = %d, want 1", rcpt.Recipients)
	}
	if rcpt.WasReplay {
		t.Errorf("WasReplay=true on fresh event")
	}
	if len(d.evid.All()) != 1 {
		t.Errorf("evidence rows = %d, want 1 (§107: proves Stage 7 wrote)", len(d.evid.All()))
	}
	if len(rcpt.OutboundEvidenceIDs) != 1 {
		t.Errorf("Receipt.OutboundEvidenceIDs len = %d, want 1", len(rcpt.OutboundEvidenceIDs))
	}
	// §107 evidence: the fakeChannel must have recorded exactly one Send.
	if got := len(d.null.sends); got != 1 {
		t.Errorf("fakeChannel sends = %d, want 1 (proves Stage 6 dispatched)", got)
	}
}

// TestRunner_Duplicate_Replay submits the same event twice with an
// explicit IdempotencyKey. §107 evidence: after the second Run, the
// evidence store STILL holds only 1 row (proves the Stage 2 short-circuit
// prevented re-dispatch). The second Receipt must have WasReplay=true.
func TestRunner_Duplicate_Replay(t *testing.T) {
	tenantID := mustParse("55555555-5555-5555-5555-555555555555")
	r, d := newIntegrationRunner()
	d.subs.Add(tenantID, subscriberRow{
		ID:     uuid.New(),
		Handle: "alice",
		Aliases: []subscriberAliasRow{
			{Channel: "null", ChannelUserID: "sandbox-alice"},
		},
	})

	body := mustJSON(map[string]any{
		"specversion":          "1.0",
		"id":                   "evt-1",
		"source":               "//x",
		"type":                 "x",
		"heraldidempotencykey": "K1",
	})
	if _, err := r.Run(context.Background(), body, map[string]any{"tenant": tenantID.String()}); err != nil {
		t.Fatalf("Run-1: %v", err)
	}
	rcpt2, err := r.Run(context.Background(), body, map[string]any{"tenant": tenantID.String()})
	if err != nil {
		t.Fatalf("Run-2: %v", err)
	}
	if rcpt2 == nil {
		t.Fatalf("Run-2 returned nil Receipt")
	}
	if !rcpt2.WasReplay {
		t.Errorf("Run-2 should mark WasReplay=true")
	}
	// §107 evidence: exactly one outbound_delivery_evidence row should
	// exist (the first run); the second run is a no-op dispatch
	// (returns cached Receipt).
	if got := len(d.evid.All()); got != 1 {
		t.Errorf("evidence rows after duplicate = %d, want 1 (§107: proves Stage 2 short-circuit prevented re-dispatch)", got)
	}
	// And the fakeChannel must still show exactly one Send.
	if got := len(d.null.sends); got != 1 {
		t.Errorf("fakeChannel sends after duplicate = %d, want 1", got)
	}
}

// TestRunner_Deny_ShortCircuits registers an evaluator that returns
// DecisionFail on the event type. §107 evidence: exactly one denial
// evidence row written (proves Stage 4 short-circuited into
// RecordDenied); Receipt.Recipients == 0 (proves Stage 5/6 were skipped).
func TestRunner_Deny_ShortCircuits(t *testing.T) {
	tenantID := mustParse("55555555-5555-5555-5555-555555555555")
	r, d := newIntegrationRunner()
	d.subs.Add(tenantID, subscriberRow{
		ID:     uuid.New(),
		Handle: "alice",
		Aliases: []subscriberAliasRow{
			{Channel: "null", ChannelUserID: "sandbox-alice"},
		},
	})
	// Register an evaluator that fails on this event type.
	d.registry.Register(&fakeEvaluator{
		ruleID:   "11.4.10",
		severity: constitution.SeverityCritical,
		triggers: []string{"digital.vasic.herald.test"},
		verdict:  constitution.DecisionFail,
		reason:   "leak detected",
	})

	body := mustJSON(map[string]any{
		"specversion": "1.0",
		"id":          "evt-deny",
		"source":      "//x",
		"type":        "digital.vasic.herald.test",
	})
	rcpt, err := r.Run(context.Background(), body, map[string]any{"tenant": tenantID.String()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rcpt == nil {
		t.Fatalf("Run returned nil Receipt on deny path")
	}
	if rcpt.Recipients != 0 {
		t.Errorf("Recipients = %d on deny path; want 0 (§107: proves Stage 4 short-circuited Stages 5/6)", rcpt.Recipients)
	}
	// §107 evidence: exactly one "denial" evidence row.
	if got := len(d.evid.All()); got != 1 {
		t.Errorf("evidence rows = %d, want 1 (§107: proves Stage 4 short-circuit recorded the denial)", got)
	}
	// And the fakeChannel must show ZERO Sends (deny path bypasses Stage 6).
	if got := len(d.null.sends); got != 0 {
		t.Errorf("fakeChannel sends on deny path = %d, want 0", got)
	}
}

// mustJSON marshals the value or panics. Mirrors mustParse in fakes_test.go.
func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
