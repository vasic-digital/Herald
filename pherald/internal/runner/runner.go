package runner

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/vasic-digital/herald/commons"
	constitution "github.com/vasic-digital/herald/commons_constitution"
)

// Runner orchestrates the §32 7-stage event-ingest pipeline. Each stage is
// a concrete field; Run threads RunCtx through them in fixed order.
//
// Concurrent-safe: stages are stateless (their deps — pgxpool, Redis
// client, etc. — are themselves concurrent-safe). Same Runner instance
// handles all requests.
//
// Per §107 anti-bluff: the orchestrator MUST NOT silently skip a stage on
// error — every error path either returns to the HTTP handler (with the
// stage-tagged error) or short-circuits into OutcomeRecorder.RecordDenied
// so the events_processed archive row is still written.
type Runner struct {
	parser  *EventParser
	idem    *IdempotencyChecker
	tenant  *TenantResolver
	policy  *PolicyGate
	subs    *SubscriberResolver
	chans   *ChannelDispatcher
	outcome *OutcomeRecorder
	logger  *slog.Logger
}

// Deps carries every external dependency the Runner needs. Constructed
// once at pherald startup; passed to NewRunner.
//
// PG       — pgx connection pool for events_processed + subscribers +
//             outbound_delivery_evidence tables. Wave 3b assumes the pool
//             is RLS-aware (tenant GUC set by commons_storage helpers).
// Redis    — go-redis Cmdable for idempotency SETNX + Get. Production
//             passes a *redis.Client; tests can pass any Cmdable
//             (cluster client, fake, …).
// Evaluator — commons_constitution Registry holding zero or more
//             evaluators. Empty Registry → permissive (no policy
//             enforcement). Wave 3b ships permissive by default.
// Channels  — map from commons.ChannelID to commons.Channel. At minimum
//             null:// must be registered (sandbox). Telegram, Slack, …
//             added if their credentials are present at startup.
// Logger    — structured logger; defaults to slog.Default() if nil.
type Deps struct {
	PG        *pgxpool.Pool
	Redis     redis.Cmdable
	Evaluator *constitution.Registry
	Channels  map[commons.ChannelID]commons.Channel
	Logger    *slog.Logger
}

// NewRunner builds the Runner from Deps. All stage instances are wired
// to the real PG/Redis adapters defined below. Tests that want fakes
// construct *Runner directly with field assignment (see runner_test.go).
func NewRunner(d Deps) *Runner {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return &Runner{
		parser: &EventParser{},
		idem: &IdempotencyChecker{
			Redis: redisAdapter{client: d.Redis},
			PG:    pgEventsProcessedAdapter{pool: d.PG},
			TTL:   24 * time.Hour,
		},
		tenant: &TenantResolver{},
		policy: &PolicyGate{Registry: d.Evaluator},
		subs: &SubscriberResolver{
			Subscribers: pgSubscribersAdapter{pool: d.PG},
		},
		chans: &ChannelDispatcher{
			Channels: d.Channels,
			Logger:   d.Logger,
		},
		outcome: &OutcomeRecorder{
			Evidence:        pgEvidenceAdapter{pool: d.PG},
			EventsProcessed: pgEventsProcessedAdapter{pool: d.PG},
		},
		logger: d.Logger,
	}
}

// Run executes the full 7-stage pipeline for a single inbound event.
// Returns the Receipt on success, or an error if any stage failed.
// Short-circuits on Stage 2 duplicate (returns cached Receipt with
// WasReplay=true) and on Stage 4 DecisionFail (jumps directly to
// OutcomeRecorder.RecordDenied so the events_processed archive row is
// still written for replay protection).
//
// `raw` is the HTTP body bytes (structured-mode CloudEvent JSON).
// `claims` is the JWT claims map from commons_auth — at minimum a
// "tenant" string claim with a UUID value MUST be present.
func (r *Runner) Run(ctx context.Context, raw []byte, claims map[string]any) (*Receipt, error) {
	rc := &RunCtx{Raw: raw, AuthClaims: claims}

	tenantID, err := extractTenant(claims)
	if err != nil {
		return nil, err
	}
	rc.TenantID = tenantID

	if err := r.parser.Process(ctx, rc); err != nil {
		return nil, err
	}
	if err := r.idem.Process(ctx, rc); err != nil {
		return nil, err
	}
	if rc.Duplicate {
		// Replay short-circuit: return a replay Receipt with WasReplay=true.
		// If CachedRcpt is nil (real-PG path before full Receipt-caching
		// lands in Wave 4+), synthesise a minimal one so the client still
		// gets a 200 with WasReplay=true and the event_id echoed.
		if rc.CachedRcpt == nil {
			return &Receipt{
				EventID:        rc.Event.ID,
				IdempotencyKey: rc.IdemKey,
				WasReplay:      true,
			}, nil
		}
		// HRD-132 race fix: the cached *Receipt may be SHARED across
		// concurrent same-key replays (a store that caches the Receipt
		// hands the same pointer to every loser of the claim). Mutating
		// rc.CachedRcpt.WasReplay in place is a data race. Return a COPY
		// with WasReplay flipped instead of mutating the shared original.
		replay := *rc.CachedRcpt
		replay.WasReplay = true
		return &replay, nil
	}
	if err := r.tenant.Process(ctx, rc); err != nil {
		return nil, err
	}
	if err := r.policy.Process(ctx, rc); err != nil {
		return nil, err
	}
	if rc.PolicyDecision == constitution.DecisionFail {
		return r.outcome.RecordDenied(ctx, rc)
	}
	if err := r.subs.Process(ctx, rc); err != nil {
		return nil, err
	}
	if err := r.chans.Process(ctx, rc); err != nil {
		return nil, err
	}
	return r.outcome.Process(ctx, rc)
}

// extractTenant pulls the "tenant" claim out as a uuid.UUID. The HTTP
// handler relies on commons_auth.GinMiddleware to have populated the
// claim map; an empty or non-string claim is a 401-style auth failure.
func extractTenant(claims map[string]any) (uuid.UUID, error) {
	v, ok := claims["tenant"]
	if !ok {
		return uuid.Nil, fmt.Errorf("runner: claims missing 'tenant'")
	}
	s, ok := v.(string)
	if !ok {
		return uuid.Nil, fmt.Errorf("runner: 'tenant' claim not a string (got %T)", v)
	}
	tid, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, fmt.Errorf("runner: parse 'tenant' claim: %w", err)
	}
	return tid, nil
}

// ----------------------------------------------------------------------
// Real-PG / real-Redis adapters. These satisfy the per-stage interfaces
// defined in idempotency.go, subscriber.go, and outcome.go.
//
// Wave 3b implements just enough SQL to make the happy/duplicate/deny
// paths work end-to-end against real Postgres. Schema-level concerns
// (RLS enforcement, indexes, partitioning) live in commons_storage
// migrations 000001..000005.
// ----------------------------------------------------------------------

// redisAdapter wraps a redis.Cmdable so the IdempotencyChecker can call
// SetNX/Get without binding to a specific *redis.Client. Production
// passes a real *redis.Client; tests inject fakeRedis directly into the
// Runner struct (bypassing this adapter).
//
// Graceful degradation (per cmd/pherald/main.go:buildRedisClient): when
// HERALD_REDIS_URL is unset, buildRedisClient returns a nil Cmdable and
// NewRunner wires it in here. A nil client MUST NOT crash the pipeline —
// the adapter short-circuits the Redis fast-path so the IdempotencyChecker
// falls through to the PG events_processed table as the sole duplicate
// detector (Wave 3 design §4 "Redis-lies-PG-truths"; degraded but
// functional). A Redis outage therefore degrades to PG-only idempotency
// rather than panicking pherald.
type redisAdapter struct {
	client redis.Cmdable
}

func (r redisAdapter) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	if r.client == nil {
		// No Redis configured: report "not set" so the IdempotencyChecker
		// proceeds to its PG Lookup confirmation (the §32.1.1 fallback).
		// Returning false (not true) is deliberate — true would mean
		// "fresh, fast-path accept" and skip the PG duplicate check.
		return false, nil
	}
	res, err := r.client.SetNX(ctx, key, value, ttl).Result()
	if err != nil {
		return false, err
	}
	return res, nil
}

func (r redisAdapter) Get(ctx context.Context, key string) (string, error) {
	if r.client == nil {
		// No Redis configured: behave like a cache miss so callers fall
		// back to the PG-truth path rather than nil-dereferencing.
		return "", redis.Nil
	}
	return r.client.Get(ctx, key).Result()
}

// pgEventsProcessedAdapter is the real PG-backed implementation of the
// idempotencyPG + eventsProcessedStore interfaces. Lookup reads the
// archive row's event_id + first_seen_at; Wave 3b does NOT cache the
// full Receipt in PG (a single TEXT/JSONB column per row) — the
// Receipt is reconstituted by the Runner.Run short-circuit when
// rc.CachedRcpt is nil (see Run above). Full Receipt caching is a
// Wave 4+ optimization tracked under a future HRD.
type pgEventsProcessedAdapter struct {
	pool *pgxpool.Pool
}

func (a pgEventsProcessedAdapter) Lookup(ctx context.Context, tenantID uuid.UUID, idemKey string) (*eventsProcessedRow, bool) {
	if a.pool == nil {
		return nil, false
	}
	row := a.pool.QueryRow(ctx,
		`SELECT event_id, first_seen_at
		   FROM events_processed
		  WHERE tenant_id = $1 AND idempotency_key = $2`,
		tenantID, idemKey)
	var eventID string
	var firstSeen time.Time
	if err := row.Scan(&eventID, &firstSeen); err != nil {
		return nil, false
	}
	return &eventsProcessedRow{
		TenantID:    tenantID,
		IdemKey:     idemKey,
		EventID:     eventID,
		FirstSeenAt: firstSeen,
	}, true
}

func (a pgEventsProcessedAdapter) Insert(ctx context.Context, row eventsProcessedRow) error {
	if a.pool == nil {
		return fmt.Errorf("pg_events_processed: nil pool (no PG configured)")
	}
	_, err := a.pool.Exec(ctx,
		`INSERT INTO events_processed(tenant_id, idempotency_key, event_id, first_seen_at)
		 VALUES($1, $2, $3, $4)
		 ON CONFLICT DO NOTHING`,
		row.TenantID, row.IdemKey, row.EventID, row.FirstSeenAt)
	return err
}

// Claim is the HRD-132 authoritative dispatch gate. It performs the SAME
// `INSERT … ON CONFLICT DO NOTHING` as Insert but inspects the command
// tag's rows-affected: exactly 1 means THIS caller inserted the row (won
// the claim → fresh → dispatch); 0 means the (tenant, idempotency_key)
// PRIMARY KEY already held a row (lost → duplicate). PG serialises
// concurrent inserts on the PK, so at most one concurrent caller per key
// sees claimed==true. Used by the Stage-2 IdempotencyChecker; the
// Stage-7 OutcomeRecorder.Insert (also ON CONFLICT DO NOTHING) is then an
// idempotent no-op on the row this claim wrote.
func (a pgEventsProcessedAdapter) Claim(ctx context.Context, row eventsProcessedRow) (bool, error) {
	if a.pool == nil {
		return false, fmt.Errorf("pg_events_processed: nil pool (no PG configured)")
	}
	tag, err := a.pool.Exec(ctx,
		`INSERT INTO events_processed(tenant_id, idempotency_key, event_id, first_seen_at)
		 VALUES($1, $2, $3, $4)
		 ON CONFLICT DO NOTHING`,
		row.TenantID, row.IdemKey, row.EventID, row.FirstSeenAt)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// pgSubscribersAdapter is the real PG-backed implementation of the
// subscribersStore interface. Two queries per call (subscribers, then
// aliases-per-subscriber) — Wave 3b prioritizes clarity over the
// JOIN-or-array-agg micro-optimization. Tenant scoping is via the
// $1 placeholder (NOT inlined as a string) and ALSO via RLS (the
// commons_storage tenant GUC), per the defense-in-depth posture.
type pgSubscribersAdapter struct {
	pool *pgxpool.Pool
}

func (a pgSubscribersAdapter) ListByTenant(ctx context.Context) ([]subscriberRow, error) {
	if a.pool == nil {
		return nil, fmt.Errorf("pg_subscribers: nil pool (no PG configured)")
	}
	tid := TenantFromCtx(ctx)
	rows, err := a.pool.Query(ctx,
		`SELECT id, handle, display_name FROM subscribers WHERE tenant_id = $1`,
		tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []subscriberRow
	for rows.Next() {
		var r subscriberRow
		if err := rows.Scan(&r.ID, &r.Handle, &r.DisplayName); err != nil {
			return nil, err
		}
		r.TenantID = tid
		aliasRows, err := a.pool.Query(ctx,
			`SELECT channel, channel_user_id FROM subscriber_aliases WHERE subscriber_id = $1`,
			r.ID)
		if err != nil {
			return nil, err
		}
		for aliasRows.Next() {
			var ar subscriberAliasRow
			if err := aliasRows.Scan(&ar.Channel, &ar.ChannelUserID); err != nil {
				aliasRows.Close()
				return nil, err
			}
			r.Aliases = append(r.Aliases, ar)
		}
		aliasRows.Close()
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// pgEvidenceAdapter is the real PG-backed implementation of the
// evidenceStore interface. Inserts one row per dispatched recipient
// into outbound_delivery_evidence and returns its UUID for inclusion in
// the Receipt's OutboundEvidenceIDs.
type pgEvidenceAdapter struct {
	pool *pgxpool.Pool
}

func (a pgEvidenceAdapter) Insert(ctx context.Context, r evidenceRow) (uuid.UUID, error) {
	if a.pool == nil {
		return uuid.Nil, fmt.Errorf("pg_evidence: nil pool (no PG configured)")
	}
	row := a.pool.QueryRow(ctx,
		`INSERT INTO outbound_delivery_evidence(tenant_id, channel_id, channel_message_id, evidence, sent_at)
		 VALUES($1, $2, $3, $4, $5) RETURNING id`,
		r.TenantID, r.ChannelID, r.ChannelMessageID, r.Evidence, r.SentAt)
	var id uuid.UUID
	if err := row.Scan(&id); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}
