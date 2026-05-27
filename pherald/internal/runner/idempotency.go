package runner

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// IdempotencyChecker is Stage 2 of the §32 pipeline. It is the
// authoritative *dispatch gate*: only the caller that wins an atomic
// claim on the (tenant, idempotency_key) slot proceeds to dispatch;
// every other concurrent same-key caller short-circuits as a duplicate.
//
// HRD-132 (claim-before-dispatch). Before HRD-132 the events_processed
// archive row was written only at Stage 7 (OutcomeRecorder), so the
// Stage-2-fresh-verdict → Stage-7-archive-commit window admitted
// concurrent duplicate dispatch (a 1000× same-key flood collapsed to a
// bounded handful of sends, never exactly 1). The fix moves the claim
// to Stage 2: the IdempotencyChecker performs an
// `INSERT … ON CONFLICT DO NOTHING` against events_processed and treats
// the row insertion itself as the dispatch grant. Because the PG
// PRIMARY KEY(tenant_id, idempotency_key) serialises concurrent inserts,
// EXACTLY ONE caller observes "row inserted" (claimed) and proceeds;
// the losers observe "0 rows affected" (already claimed) and are
// duplicates. Dispatch is therefore exactly-once even under concurrency.
//
// The Redis SETNX fast-path is preserved purely as an OPTIMISATION: a
// SETNX miss with a confirming PG Lookup lets a known-duplicate replay
// short-circuit WITHOUT racing the claim INSERT. But the PG claim is the
// AUTHORITATIVE gate — a SETNX *win* no longer grants dispatch on its
// own; it still must win the PG claim (so two callers that both win
// SETNX for distinct keys, or that race a TTL-expired key, cannot both
// dispatch the same idempotency key).
//
// Degraded (nil Redis, HRD-179): with no Redis the fast-path is skipped
// and the PG claim INSERT alone provides exactly-once — the documented
// Wave 3 §4 "Redis-lies-PG-truths" posture, now strengthened from
// "bounded dispatch" to "exactly-once dispatch" by the atomic claim.
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
//
// Lookup confirms a Redis-miss against the archive (fast-path
// optimisation: a known-archived key short-circuits without racing the
// claim INSERT). Claim is the AUTHORITATIVE dispatch gate (HRD-132): it
// performs an `INSERT … ON CONFLICT DO NOTHING` and reports whether THIS
// caller inserted the row (won → fresh → dispatch) or it already existed
// (lost → duplicate → short-circuit). Exactly one concurrent caller per
// (tenant, idempotency_key) gets claimed==true because the PG PRIMARY KEY
// serialises the inserts.
type idempotencyPG interface {
	Lookup(ctx context.Context, tenantID uuid.UUID, idemKey string) (*eventsProcessedRow, bool)
	Claim(ctx context.Context, row eventsProcessedRow) (claimed bool, err error)
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
	if !set {
		// SETNX missed → key already present → likely a duplicate replay.
		// Fast-path OPTIMISATION (HRD-132): confirm via the archive
		// Lookup. If the events_processed row is already committed, this
		// is a confirmed duplicate and we short-circuit WITHOUT racing the
		// claim INSERT. If the row is NOT yet there we are in the
		// Redis-lies-PG-truths window (SETNX winner still mid-pipeline,
		// pre-claim): fall through to the authoritative PG claim below —
		// the PRIMARY KEY serialises us against the winner so at most one
		// of us is granted dispatch.
		if row, found := c.PG.Lookup(ctx, rc.TenantID, rc.IdemKey); found {
			rc.Duplicate = true
			rc.CachedRcpt = row.Receipt
			return nil
		}
	}
	// Authoritative dispatch gate (HRD-132): attempt to CLAIM the
	// (tenant, idempotency_key) slot with an atomic
	// `INSERT … ON CONFLICT DO NOTHING`. Exactly one concurrent caller
	// per key observes claimed==true (it inserted the row) and is granted
	// dispatch; every other caller observes claimed==false (the row
	// already existed) and is a duplicate. This holds whether or not Redis
	// is present (nil-Redis degrade still claims via PG alone).
	claimed, err := c.PG.Claim(ctx, eventsProcessedRow{
		TenantID:    rc.TenantID,
		IdemKey:     rc.IdemKey,
		EventID:     rc.Event.ID,
		FirstSeenAt: time.Now(),
	})
	if err != nil {
		return fmt.Errorf("idempotency: claim events_processed: %w", err)
	}
	if !claimed {
		// Lost the claim → another caller already owns this slot.
		// Surface the prior receipt if the store cached one (production
		// PG returns Receipt=nil today; the Run short-circuit synthesises
		// a minimal replay receipt in that case).
		rc.Duplicate = true
		if row, found := c.PG.Lookup(ctx, rc.TenantID, rc.IdemKey); found {
			rc.CachedRcpt = row.Receipt
		}
		return nil
	}
	// Won the claim → fresh event, this caller dispatches. The
	// events_processed archive row is already durable (we just inserted
	// it), so Stage 7 OutcomeRecorder's archive Insert is an idempotent
	// no-op (ON CONFLICT DO NOTHING) that only refreshes the bookkeeping.
	rc.Duplicate = false
	rc.Claimed = true
	return nil
}

var errIdemNotFound = errors.New("idempotency: events_processed row not found")
