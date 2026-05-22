package runner

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// IdempotencyChecker is Stage 2 of the §32 pipeline. It implements the
// "Redis-lies-PG-truths" arbitration semantics from the Wave 3 design
// §4: a Redis SETNX miss means "maybe a duplicate"; the verdict is
// confirmed by PG `events_processed` table lookup. If PG has no row
// (race window: SETNX succeeded but archive hasn't caught up yet),
// the event is treated as FRESH — the alternative (block until PG row
// appears) would lock up ingest if the archive writer is down.
//
// Per §107: a no-op stage that always returns Duplicate=false would
// be a §11.4 bluff (every event would dispatch even when duplicate).
type IdempotencyChecker struct {
	Redis idempotencyRedis
	PG    idempotencyPG
	TTL   time.Duration
}

// idempotencyRedis is the subset of redis.Cmdable this stage uses.
// Used here as an interface so the test fake can satisfy it without
// pulling in the real Redis client.
type idempotencyRedis interface {
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	Get(ctx context.Context, key string) (string, error)
}

// idempotencyPG is the subset of PG access this stage uses.
type idempotencyPG interface {
	Lookup(ctx context.Context, tenantID uuid.UUID, idemKey string) (*eventsProcessedRow, bool)
}

// eventsProcessedRow mirrors the PG `events_processed` table row that
// archives every fresh-accepted event for replay-receipt + duplicate
// arbitration. The real PG-backed implementation lives in commons_storage;
// this struct is the in-process projection the runner stages exchange.
type eventsProcessedRow struct {
	TenantID    uuid.UUID
	IdemKey     string
	EventID     string
	FirstSeenAt time.Time
	Receipt     *Receipt
}

func (c *IdempotencyChecker) Process(ctx context.Context, rc *RunCtx) error {
	key := "herald:idem:" + rc.TenantID.String() + ":" + rc.IdemKey
	set, err := c.Redis.SetNX(ctx, key, rc.Event.ID, c.TTL)
	if err != nil {
		return fmt.Errorf("idempotency: redis SETNX: %w", err)
	}
	if set {
		// SETNX succeeded → key wasn't there → fresh event.
		rc.Duplicate = false
		return nil
	}
	// SETNX missed → key already present → potential duplicate.
	// Confirm via PG: if the events_processed row exists, it's a real
	// duplicate; if not, we're in the Redis-lies-PG-truths race window
	// and should treat as fresh.
	row, found := c.PG.Lookup(ctx, rc.TenantID, rc.IdemKey)
	if !found {
		rc.Duplicate = false
		return nil
	}
	rc.Duplicate = true
	rc.CachedRcpt = row.Receipt
	return nil
}

var errIdemNotFound = errors.New("idempotency: events_processed row not found")
