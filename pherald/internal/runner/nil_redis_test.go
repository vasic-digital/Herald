package runner

import (
	"context"
	"testing"
	"time"

	"github.com/vasic-digital/herald/commons"
	constitution "github.com/vasic-digital/herald/commons_constitution"
)

// These tests exercise the documented graceful-degradation contract from
// pherald/cmd/pherald/main.go:74-78:
//
//	"The Runner's IdempotencyChecker tolerates a nil client (the SetNX
//	 adapter short-circuits to "not seen" and the PG archive table becomes
//	 the sole duplicate detector — degraded but functional)."
//
// They construct the IdempotencyChecker exactly as production wires it
// (NewRunner → idem.Redis = redisAdapter{client: d.Redis}) with a nil
// Redis client, so they hit the real redisAdapter (runner.go), NOT the
// in-memory fakeRedis. Before the fix, redisAdapter.SetNX nil-derefs
// (runner.go:191) → panic. After the fix, the Redis fast-path is skipped
// and idempotency degrades to the PG events_processed fallback.

// TestRunner_NilRedis_FreshEvent_NoPanic_ViaPG is the §107 anchor for the
// FRESH path: nil Redis + an event whose IdemKey is absent from PG →
// MUST NOT panic, MUST be treated as fresh (Duplicate=false), and the
// verdict MUST come from the PG fallback Lookup (not a crash, not a
// blanket "always fresh").
func TestRunner_NilRedis_FreshEvent_NoPanic_ViaPG(t *testing.T) {
	tenantID := mustParse("11111111-1111-1111-1111-111111111111")
	pg := newFakeEventsProcessedStore() // empty: this event was never seen

	// Wire the IdempotencyChecker the way production NewRunner does: the
	// real redisAdapter wrapping a nil client (HERALD_REDIS_URL unset).
	c := &IdempotencyChecker{
		Redis: redisAdapter{client: nil},
		PG:    pg,
		TTL:   24 * time.Hour,
	}

	rc := &RunCtx{
		TenantID: tenantID,
		IdemKey:  "fresh-key",
		Event:    cloudEventStub("evt-fresh", "x"),
	}

	// Pre-fix this PANICS at redisAdapter.SetNX (nil-deref of r.client).
	if err := c.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process with nil Redis: unexpected error: %v", err)
	}
	if rc.Duplicate {
		t.Errorf("nil-Redis fresh event marked Duplicate=true; want false (PG fallback found no prior row)")
	}
	if rc.CachedRcpt != nil {
		t.Errorf("nil-Redis fresh event has non-nil CachedRcpt; want nil")
	}
}

// TestRunner_NilRedis_DuplicateEvent_DetectedViaPG is the §107 anchor for
// the DUPLICATE path: nil Redis + an event whose IdemKey IS already in PG
// events_processed → MUST be detected as a duplicate via the PG fallback
// (Duplicate=true, prior Receipt returned). This proves the fallback
// actually performs idempotency, not merely "doesn't crash" — a no-crash
// stub that always returns fresh would be a §107 PASS-bluff.
func TestRunner_NilRedis_DuplicateEvent_DetectedViaPG(t *testing.T) {
	tenantID := mustParse("11111111-1111-1111-1111-111111111111")
	pg := newFakeEventsProcessedStore()

	// Seed PG as if a prior fresh event already archived this IdemKey.
	priorReceipt := &Receipt{EventID: "evt-orig", IdempotencyKey: "dup-key", Recipients: 1}
	if err := pg.Insert(context.Background(), eventsProcessedRow{
		TenantID:    tenantID,
		IdemKey:     "dup-key",
		EventID:     "evt-orig",
		FirstSeenAt: time.Now(),
		Receipt:     priorReceipt,
	}); err != nil {
		t.Fatalf("seed PG: %v", err)
	}

	c := &IdempotencyChecker{
		Redis: redisAdapter{client: nil},
		PG:    pg,
		TTL:   24 * time.Hour,
	}

	rc := &RunCtx{
		TenantID: tenantID,
		IdemKey:  "dup-key",
		Event:    cloudEventStub("evt-retry", "x"),
	}

	if err := c.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process with nil Redis (duplicate): unexpected error: %v", err)
	}
	if !rc.Duplicate {
		t.Errorf("nil-Redis duplicate event marked Duplicate=false; want true (PG fallback should detect it)")
	}
	if rc.CachedRcpt == nil {
		t.Fatal("nil-Redis duplicate event has nil CachedRcpt; want the prior Receipt from PG")
	}
	if rc.CachedRcpt.EventID != "evt-orig" {
		t.Errorf("CachedRcpt.EventID = %q, want 'evt-orig' (the original archived event)", rc.CachedRcpt.EventID)
	}
}

// TestRunner_NilRedis_FullPipeline_FreshThenDuplicate drives the full
// 7-stage Runner with the production NewRunner-style wiring (real
// redisAdapter, nil client) end-to-end, then a duplicate. This is the
// orchestrator-level §107 evidence that a Redis-less pherald serves
// /v1/events without crashing AND still de-duplicates via PG.
func TestRunner_NilRedis_FullPipeline_FreshThenDuplicate(t *testing.T) {
	tenantID := mustParse("55555555-5555-5555-5555-555555555555")

	// Build a Runner like newIntegrationRunner, but with the real
	// redisAdapter wrapping a nil client (mirrors production with no
	// HERALD_REDIS_URL set). PG/subs/channels/evidence stay fake.
	pg := newFakeEventsProcessedStore()
	subs := newFakeSubscribersStore()
	evid := newFakeEvidenceStore()
	nullCh := newFakeChannel("null")
	subs.Add(tenantID, subscriberRow{
		ID:     mustParse("99999999-9999-9999-9999-999999999999"),
		Handle: "alice",
		Aliases: []subscriberAliasRow{
			{Channel: "null", ChannelUserID: "sandbox-alice"},
		},
	})

	r := &Runner{
		parser:  &EventParser{},
		idem:    &IdempotencyChecker{Redis: redisAdapter{client: nil}, PG: pg, TTL: 24 * time.Hour},
		tenant:  &TenantResolver{},
		policy:  &PolicyGate{Registry: constitution.NewRegistry()},
		subs:    &SubscriberResolver{Subscribers: subs},
		chans:   &ChannelDispatcher{Channels: map[commons.ChannelID]commons.Channel{commons.ChannelNull: nullCh}},
		outcome: &OutcomeRecorder{Evidence: evid, EventsProcessed: pg},
	}

	body := mustJSON(map[string]any{
		"specversion":          "1.0",
		"id":                   "evt-loop-1",
		"source":               "//x",
		"type":                 "x",
		"heraldidempotencykey": "LOOPKEY",
	})

	rcpt1, err := r.Run(context.Background(), body, map[string]any{"tenant": tenantID.String()})
	if err != nil {
		t.Fatalf("Run-1 (fresh, nil Redis): %v", err)
	}
	if rcpt1 == nil || rcpt1.WasReplay {
		t.Fatalf("Run-1 should be a fresh dispatch (WasReplay=false); got %+v", rcpt1)
	}
	if got := len(evid.All()); got != 1 {
		t.Errorf("after fresh run, evidence rows = %d, want 1", got)
	}
	if got := len(nullCh.sends); got != 1 {
		t.Errorf("after fresh run, channel sends = %d, want 1", got)
	}

	// Second send of the same idempotency key — must be detected as a
	// duplicate via PG (no Redis available) and NOT re-dispatch.
	rcpt2, err := r.Run(context.Background(), body, map[string]any{"tenant": tenantID.String()})
	if err != nil {
		t.Fatalf("Run-2 (duplicate, nil Redis): %v", err)
	}
	if rcpt2 == nil || !rcpt2.WasReplay {
		t.Fatalf("Run-2 should be a replay (WasReplay=true) via PG fallback; got %+v", rcpt2)
	}
	if got := len(evid.All()); got != 1 {
		t.Errorf("after duplicate run, evidence rows = %d, want 1 (PG fallback prevented re-dispatch)", got)
	}
	if got := len(nullCh.sends); got != 1 {
		t.Errorf("after duplicate run, channel sends = %d, want 1 (PG fallback prevented re-dispatch)", got)
	}
}
