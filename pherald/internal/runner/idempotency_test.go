package runner

import (
	"context"
	"testing"
	"time"
)

func TestIdempotencyChecker_FreshEvent_NotDuplicate(t *testing.T) {
	tenantID := mustParse("11111111-1111-1111-1111-111111111111")
	redis := newFakeRedis()
	pg := newFakeEventsProcessedStore()
	c := &IdempotencyChecker{Redis: redis, PG: pg, TTL: 24 * time.Hour}

	rc := &RunCtx{
		TenantID: tenantID,
		IdemKey:  "fresh-1",
		Event:    cloudEventStub("evt-1", "x"),
	}
	if err := c.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if rc.Duplicate {
		t.Errorf("Duplicate=true on fresh event")
	}
	if rc.CachedRcpt != nil {
		t.Errorf("CachedRcpt non-nil on fresh event")
	}
}

func TestIdempotencyChecker_DuplicateRedis_HitsPG(t *testing.T) {
	tenantID := mustParse("11111111-1111-1111-1111-111111111111")
	redis := newFakeRedis()
	pg := newFakeEventsProcessedStore()
	c := &IdempotencyChecker{Redis: redis, PG: pg, TTL: 24 * time.Hour}

	// Seed Redis + PG with a prior event.
	priorReceipt := &Receipt{EventID: "evt-1", IdempotencyKey: "k1", Recipients: 1}
	_, _ = redis.SetNX(context.Background(), "herald:idem:"+tenantID.String()+":k1", "evt-1", 24*time.Hour)
	_ = pg.Insert(context.Background(), eventsProcessedRow{
		TenantID: tenantID, IdemKey: "k1", EventID: "evt-1", FirstSeenAt: time.Now(), Receipt: priorReceipt,
	})

	// Process a "second send" with the same IdemKey.
	rc := &RunCtx{
		TenantID: tenantID,
		IdemKey:  "k1",
		Event:    cloudEventStub("evt-1-retry", "x"),
	}
	if err := c.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if !rc.Duplicate {
		t.Errorf("Duplicate=false on second send with same IdemKey")
	}
	if rc.CachedRcpt == nil {
		t.Fatal("CachedRcpt nil on duplicate (expected prior Receipt)")
	}
	if rc.CachedRcpt.EventID != "evt-1" {
		t.Errorf("CachedRcpt.EventID = %q, want 'evt-1'", rc.CachedRcpt.EventID)
	}
}

func TestIdempotencyChecker_RedisLiesPGTruths_FreshIfPGAbsent(t *testing.T) {
	// Race scenario: Redis says "duplicate" but PG hasn't archived yet.
	// Per Wave 3 design §4, arbitration favors PG truth → treat as fresh.
	tenantID := mustParse("11111111-1111-1111-1111-111111111111")
	redis := newFakeRedis()
	pg := newFakeEventsProcessedStore() // intentionally empty
	c := &IdempotencyChecker{Redis: redis, PG: pg, TTL: 24 * time.Hour}

	// Redis SETNX has the key but PG has no row (mid-race).
	_, _ = redis.SetNX(context.Background(), "herald:idem:"+tenantID.String()+":k1", "evt-1", 24*time.Hour)

	rc := &RunCtx{
		TenantID: tenantID,
		IdemKey:  "k1",
		Event:    cloudEventStub("evt-2", "x"),
	}
	if err := c.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if rc.Duplicate {
		t.Errorf("Redis-lies-PG-truths: should treat as FRESH when PG row absent (got Duplicate=true)")
	}
}
