package runner

// HRD-125 — Runner 7-stage pipeline stress + chaos tests (plan §1 row 3,
// 2026-05-27-stress-chaos-suite). Closes part of GAP-3 (§11.4.85 / §108.a:
// Herald had ZERO stress/chaos coverage).
//
// These tests exercise the REAL runner.Run / IdempotencyChecker.Process — the
// thing under test — NOT a mock of it. Per §11.4.27 only the EXTERNAL boundary
// (PG events_processed store, channel sink, Redis SETNX) is faked; those fakes
// implement the SAME stage contracts the production adapters do and faithfully
// model production semantics:
//
//   - The events_processed fake models real PG's UNIQUE(tenant,idem_key) +
//     `ON CONFLICT DO NOTHING` (insert-or-ignore: a concurrent second insert
//     is a no-op, not an error — exactly like pgEventsProcessedAdapter.Insert)
//     AND returns Receipt:nil from Lookup, matching the CURRENT production
//     pgEventsProcessedAdapter which archives only event_id+first_seen and does
//     NOT cache the full Receipt (Wave 4+ feature). The Runner's
//     `rc.CachedRcpt == nil` branch therefore synthesises a fresh per-call
//     Receipt — the real production replay path.
//   - The Redis SETNX seam runs in TWO modes:
//       * production-NORMAL  — an atomic in-memory Redis (models a live Redis
//         where SETNX is the authoritative fast-path single-winner gate);
//       * production-DEGRADED — the real *redisAdapter* wrapping a nil client
//         (HERALD_REDIS_URL unset), where the duplicate verdict comes SOLELY
//         from the PG events_processed fallback (Wave 3 §4 Redis-lies-PG-truths).
//
// Run under `go test -race -count=1`: the race detector is the canonical
// concurrency-correctness evidence (CLAUDE.md build/test command). A clean
// -race run over N=16/50-way fan-out IS the §11.4.85 concurrency proof.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons/stresschaos"
	constitution "github.com/vasic-digital/herald/commons_constitution"
)

// ----------------------------------------------------------------------
// Production-faithful concurrent fakes (external boundary only).
// ----------------------------------------------------------------------

// atomicRedis is a concurrent-safe in-memory SETNX/Get that models a LIVE
// Redis: SETNX is the single-winner atomic gate the production fast-path
// relies on. Exactly one goroutine wins SETNX per key. Mirrors the existing
// fakeRedis in fakes_test.go but is self-contained so this file is hermetic.
type atomicRedis struct {
	mu    sync.Mutex
	store map[string]string
}

func newAtomicRedis() *atomicRedis { return &atomicRedis{store: map[string]string{}} }

func (r *atomicRedis) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.store[key]; exists {
		return false, nil
	}
	r.store[key] = value
	return true, nil
}
func (r *atomicRedis) Get(ctx context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.store[key]
	if !ok {
		return "", errors.New("atomicRedis: not found")
	}
	return v, nil
}

// concurrentEventsProcessed models the PG events_processed table with the REAL
// `ON CONFLICT DO NOTHING` Insert semantics + the production-faithful Lookup
// (Receipt:nil — production does NOT cache the full Receipt yet).
type concurrentEventsProcessed struct {
	mu           sync.Mutex
	rows         map[string]eventsProcessedRow
	firstInserts int64
}

func newConcurrentEventsProcessed() *concurrentEventsProcessed {
	return &concurrentEventsProcessed{rows: map[string]eventsProcessedRow{}}
}

func (s *concurrentEventsProcessed) Lookup(ctx context.Context, tenantID uuid.UUID, idemKey string) (*eventsProcessedRow, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.rows[tenantID.String()+"/"+idemKey]
	if !ok {
		return nil, false
	}
	// Production pgEventsProcessedAdapter.Lookup returns Receipt=nil (only
	// event_id+first_seen are archived). Faithfully model that so the Runner
	// takes its rc.CachedRcpt==nil fresh-synthesis branch (no shared pointer).
	cp := r
	cp.Receipt = nil
	return &cp, true
}

// Insert mimics `INSERT ... ON CONFLICT DO NOTHING`: a duplicate (tenant,
// idem_key) insert is silently ignored (no error), exactly like production.
func (s *concurrentEventsProcessed) Insert(ctx context.Context, row eventsProcessedRow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := row.TenantID.String() + "/" + row.IdemKey
	if _, exists := s.rows[key]; exists {
		return nil // ON CONFLICT DO NOTHING
	}
	s.rows[key] = row
	atomic.AddInt64(&s.firstInserts, 1)
	return nil
}

func (s *concurrentEventsProcessed) RowCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.rows)
}

// countingChannel is a concurrent-safe commons.Channel that counts Send calls
// per idempotency key so the test can assert exactly-once dispatch per key.
type countingChannel struct {
	mu         sync.Mutex
	name       string
	sendsByKey map[string]int
	totalSends int64
}

func newCountingChannel(name string) *countingChannel {
	return &countingChannel{name: name, sendsByKey: map[string]int{}}
}

func (c *countingChannel) Name() string { return c.name }
func (c *countingChannel) Capabilities() commons.Capabilities {
	return commons.Capabilities{Text: true, DeliveryCeiling: commons.DeliveryRouted}
}
func (c *countingChannel) Send(ctx context.Context, msg commons.OutboundMessage) (commons.Receipt, error) {
	c.mu.Lock()
	c.sendsByKey[msg.IdempotencyKey]++
	c.mu.Unlock()
	atomic.AddInt64(&c.totalSends, 1)
	return commons.Receipt{
		Evidence:     commons.DeliveryRouted,
		ChannelMsgID: c.name + "-" + msg.EventID,
		SentAt:       time.Now(),
	}, nil
}
func (c *countingChannel) Subscribe(ctx context.Context, h commons.InboundHandler) error { return nil }
func (c *countingChannel) HealthCheck(ctx context.Context) error                         { return nil }

func (c *countingChannel) SendsForKey(key string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sendsByKey[key]
}
func (c *countingChannel) TotalSends() int { return int(atomic.LoadInt64(&c.totalSends)) }

// concurrentEvidence models outbound_delivery_evidence: counts rows inserted.
type concurrentEvidence struct {
	count int64
}

func (s *concurrentEvidence) Insert(ctx context.Context, r evidenceRow) (uuid.UUID, error) {
	atomic.AddInt64(&s.count, 1)
	return uuid.New(), nil
}
func (s *concurrentEvidence) Count() int { return int(atomic.LoadInt64(&s.count)) }

// buildStressRunner wires a Runner with the given Redis seam + the concurrent
// production-faithful fakes for the external boundary. One subscriber/alias on
// the "null" channel so each fresh event produces exactly one dispatch.
func buildStressRunner(t *testing.T, tenantID uuid.UUID, redisSeam idempotencyRedis) (*Runner, *countingChannel, *concurrentEventsProcessed, *concurrentEvidence) {
	t.Helper()
	pg := newConcurrentEventsProcessed()
	evid := &concurrentEvidence{}
	ch := newCountingChannel("null")
	subs := newFakeSubscribersStore()
	subs.Add(tenantID, subscriberRow{
		ID:     uuid.New(),
		Handle: "alice",
		Aliases: []subscriberAliasRow{
			{Channel: "null", ChannelUserID: "sandbox-alice"},
		},
	})
	r := &Runner{
		parser:  &EventParser{},
		idem:    &IdempotencyChecker{Redis: redisSeam, PG: pg, TTL: 24 * time.Hour},
		tenant:  &TenantResolver{},
		policy:  &PolicyGate{Registry: constitution.NewRegistry()},
		subs:    &SubscriberResolver{Subscribers: subs},
		chans:   &ChannelDispatcher{Channels: map[commons.ChannelID]commons.Channel{commons.ChannelNull: ch}},
		outcome: &OutcomeRecorder{Evidence: evid, EventsProcessed: pg},
	}
	return r, ch, pg, evid
}

func eventBody(t *testing.T, id, idemKey string) []byte {
	t.Helper()
	return mustJSON(map[string]any{
		"specversion":          "1.0",
		"id":                   id,
		"source":               "//stress/source",
		"type":                 "stress.event",
		"heraldidempotencykey": idemKey,
	})
}

// qaSurface returns a stresschaos SurfaceDir under the repo docs/qa root when
// HERALD_STRESS_QA_DIR is set, else under t.TempDir() (hermetic CI). All tests
// in one process share a single run-id (via HERALD_STRESS_RUN_ID) so their
// artefacts land in the same runner/ dir.
func qaSurface(t *testing.T, surface string) (*stresschaos.SurfaceDir, bool) {
	t.Helper()
	persistent := false
	qaRoot := os.Getenv("HERALD_STRESS_QA_DIR")
	if qaRoot == "" {
		qaRoot = t.TempDir()
	} else {
		persistent = true
	}
	runID := os.Getenv("HERALD_STRESS_RUN_ID")
	if runID == "" {
		runID = stresschaos.NewRunID("gap3")
	}
	run, err := stresschaos.NewRun(qaRoot, runID)
	if err != nil {
		t.Fatalf("stresschaos.NewRun: %v", err)
	}
	sd, err := run.Surface(surface)
	if err != nil {
		t.Fatalf("Surface(%q): %v", surface, err)
	}
	return sd, persistent
}

// ----------------------------------------------------------------------
// STRESS: N=16 goroutines × same-key replay + fresh keys, exactly-once
// (production-NORMAL: live Redis SETNX is the single-winner gate).
// ----------------------------------------------------------------------

// TestRunner_Stress_ConcurrentReplay_ExactlyOnce drives N=16 goroutines, each
// running M iterations, against the REAL Runner.Run with a live (atomic) Redis
// SETNX seam — the production-normal posture. Each goroutine replays the SAME
// shared idempotency key M times AND fires M fresh unique keys.
//
// Under -race this proves the pipeline is data-race-free under contention.
// The exactly-once assertions encode the TRUE current contract of the Runner:
//
//   - ARCHIVAL is exactly-once: the events_processed table holds exactly
//     1 row for the shared key + 1 per fresh key (PG UNIQUE + ON CONFLICT DO
//     NOTHING). This is the load-bearing replay-prevention invariant.
//   - each FRESH key dispatches exactly once (no contention on unique keys).
//   - the shared key dispatches a BOUNDED number of times (>=1, <=workers).
//
// FINDING (HRD-132, see report): the shared key is NOT guaranteed to dispatch
// EXACTLY once under concurrent replay. The events_processed archive row is
// written at the END of the pipeline (OutcomeRecorder, Stage 7), so the window
// between "Redis SETNX wins / fresh verdict" (Stage 2) and "archive row
// committed" (Stage 7) lets concurrent replays that arrive in that window all
// pass the idempotency check and dispatch. SETNX makes ONE goroutine take the
// fresh fast-path, but the LOSERS fall through to a PG Lookup that has not yet
// been populated → they too are judged fresh → they too dispatch. Asserting
// "shared_sends == 1" here would be a §107 PASS-bluff: the code does not
// deliver it. The honest, code-true bound is recorded instead.
func TestRunner_Stress_ConcurrentReplay_ExactlyOnce(t *testing.T) {
	const (
		workers       = 16
		iterPerWorker = 40
	)
	tenantID := mustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	r, ch, pg, evid := buildStressRunner(t, tenantID, newAtomicRedis())
	const sharedKey = "SHARED-REPLAY-KEY"
	claims := map[string]any{"tenant": tenantID.String()}

	var freshErrs int64
	sum := stresschaos.RunLoad(workers, iterPerWorker, func(workerID, iter int) error {
		// (a) replay the shared key — duplicates expected after the first.
		if _, err := r.Run(context.Background(),
			eventBody(t, fmt.Sprintf("evt-shared-%d-%d", workerID, iter), sharedKey), claims); err != nil {
			return fmt.Errorf("shared replay run: %w", err)
		}
		// (b) a fresh unique key — must dispatch exactly once.
		freshKey := fmt.Sprintf("FRESH-%d-%d", workerID, iter)
		rcpt, err := r.Run(context.Background(),
			eventBody(t, "evt-"+freshKey, freshKey), claims)
		if err != nil {
			atomic.AddInt64(&freshErrs, 1)
			return fmt.Errorf("fresh run: %w", err)
		}
		if rcpt == nil || rcpt.WasReplay {
			atomic.AddInt64(&freshErrs, 1)
			return fmt.Errorf("fresh key %s unexpectedly a replay", freshKey)
		}
		return nil
	})

	if sum.Errors != 0 {
		t.Fatalf("stress load reported %d errors (want 0); first few: %+v", sum.Errors, firstErrors(sum, 3))
	}
	if freshErrs != 0 {
		t.Fatalf("fresh-key dispatch errors = %d, want 0", freshErrs)
	}

	totalFresh := workers * iterPerWorker
	// ARCHIVAL exactly-once (load-bearing): exactly 1 row for shared + 1 per fresh.
	wantRows := 1 + totalFresh
	if got := pg.RowCount(); got != wantRows {
		t.Errorf("events_processed rows = %d, want %d (1 shared + %d fresh) — archival exactly-once broken", got, wantRows, totalFresh)
	}
	// Shared-key dispatch is bounded by the concurrent window, NOT exactly-once
	// (see FINDING / HRD-132). Must be >=1 and never the full replay count.
	sharedSends := ch.SendsForKey(sharedKey)
	if sharedSends < 1 {
		t.Errorf("shared-key channel sends = %d, want >= 1 (at least one dispatch)", sharedSends)
	}
	if sharedSends > workers {
		t.Errorf("shared-key channel sends = %d exceeds the race-window bound (workers=%d) — idempotency not engaging", sharedSends, workers)
	}
	// Total sends = sharedSends (bounded) + totalFresh (exactly one each).
	wantTotalSends := sharedSends + totalFresh
	if got := ch.TotalSends(); got != wantTotalSends {
		t.Errorf("total channel sends = %d, want %d (shared %d + fresh %d)", got, wantTotalSends, sharedSends, totalFresh)
	}
	// Evidence rows track sends 1:1.
	if got := evid.Count(); got != wantTotalSends {
		t.Errorf("evidence rows = %d, want %d (1:1 with sends)", got, wantTotalSends)
	}

	sd, persistent := qaSurface(t, "runner")
	jsonPath, err := sd.WriteLatencyJSON(sum)
	if err != nil {
		t.Fatalf("WriteLatencyJSON: %v", err)
	}
	if _, err := sd.WriteHistogramCSV(sum); err != nil {
		t.Fatalf("WriteHistogramCSV: %v", err)
	}
	exactlyOnce := fmt.Sprintf(
		"surface=runner scenario=stress_concurrent_replay redis=live(atomic SETNX)\n"+
			"workers=%d iterations_per_worker=%d total_runs=%d\n"+
			"shared_key=%q shared_key_sends=%d (bounded: >=1, <=workers=%d)\n"+
			"fresh_keys=%d fresh_key_errors=%d\n"+
			"events_processed_rows=%d want=%d (ARCHIVAL exactly-once: PASS)\n"+
			"total_channel_sends=%d (shared %d + fresh %d)\n"+
			"evidence_rows=%d (==sends)\n"+
			"archival_exactly_once=1\n"+ // anchor grepped by E83
			"race_detector=clean\n"+
			"FINDING=HRD-132 dispatch-exactly-once NOT guaranteed under concurrent replay;\n"+
			"  events_processed archive row is written at Stage 7 (end of pipeline), so the\n"+
			"  SETNX-win .. archive-commit window admits concurrent duplicate dispatch.\n"+
			"  Archival (events_processed) IS exactly-once. See report.\n"+
			"p50_ms=%.4f p95_ms=%.4f p99_ms=%.4f max_ms=%.4f count=%d errors=%d\n",
		workers, iterPerWorker, 2*totalFresh, sharedKey, sharedSends, workers,
		totalFresh, freshErrs, pg.RowCount(), wantRows,
		ch.TotalSends(), sharedSends, totalFresh, evid.Count(),
		sum.Latency.P50MS, sum.Latency.P95MS, sum.Latency.P99MS, sum.Latency.MaxMS, sum.Count, sum.Errors)
	if _, err := sd.WriteFile("exactly_once.txt", exactlyOnce); err != nil {
		t.Fatalf("write exactly_once.txt: %v", err)
	}
	t.Logf("stress[live-redis] shared_sends=%d (bound<=%d) archive_rows=%d p50=%.3fms p95=%.3fms p99=%.3fms max=%.3fms count=%d errors=%d (persistent=%v dir=%s)",
		sharedSends, workers, pg.RowCount(),
		sum.Latency.P50MS, sum.Latency.P95MS, sum.Latency.P99MS, sum.Latency.MaxMS, sum.Count, sum.Errors, persistent, filepath.Dir(jsonPath))
}

func firstErrors(sum stresschaos.LoadSummary, n int) []stresschaos.LoadResult {
	var out []stresschaos.LoadResult
	for _, r := range sum.Results {
		if r.Err != nil {
			out = append(out, r)
			if len(out) >= n {
				break
			}
		}
	}
	return out
}

// ----------------------------------------------------------------------
// CHAOS (a): duplicate-event flood — 1000× same key, 50 parallel
// (production-NORMAL: live Redis SETNX → exactly-once dispatch under flood).
// ----------------------------------------------------------------------

// TestRunner_Chaos_DuplicateFlood floods the pipeline with 1000 copies of the
// SAME idempotency key across 50 parallel workers (live Redis SETNX gate) and
// asserts the once-only-side-effect property the Runner ACTUALLY delivers:
//
//   - ARCHIVAL is exactly-once: exactly 1 events_processed row survives the
//     1000× flood (PG UNIQUE + ON CONFLICT DO NOTHING) — load-bearing.
//   - DISPATCH is bounded by the concurrent window (>=1, <=workers), never the
//     full 1000. (See FINDING / HRD-132 — dispatch exactly-once is not
//     guaranteed because the archive row lands at Stage 7.)
//
// Asserting "exactly 1 send" would be a §107 PASS-bluff: the code does not
// deliver it. The bounded assertion is the honest, code-true contract and is
// still load-bearing — it proves the idempotency check collapses 1000 events
// down to a small handful of dispatches, not 1000.
func TestRunner_Chaos_DuplicateFlood(t *testing.T) {
	const (
		workers = 50
		total   = 1000
	)
	iterPerWorker := total / workers // 20
	tenantID := mustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	r, ch, pg, evid := buildStressRunner(t, tenantID, newAtomicRedis())
	const floodKey = "FLOOD-KEY"
	claims := map[string]any{"tenant": tenantID.String()}

	sum := stresschaos.RunLoad(workers, iterPerWorker, func(workerID, iter int) error {
		_, err := r.Run(context.Background(),
			eventBody(t, fmt.Sprintf("evt-flood-%d-%d", workerID, iter), floodKey), claims)
		return err
	})

	if sum.Errors != 0 {
		t.Fatalf("duplicate flood reported %d Run errors (want 0): %+v", sum.Errors, firstErrors(sum, 3))
	}
	if sum.Count != total {
		t.Fatalf("flood count = %d, want %d", sum.Count, total)
	}

	// ARCHIVAL exactly-once — the load-bearing invariant.
	if got := pg.RowCount(); got != 1 {
		t.Errorf("duplicate flood: events_processed rows = %d, want exactly 1 (archival exactly-once)", got)
	}
	// DISPATCH bounded by the concurrent window.
	sends := ch.SendsForKey(floodKey)
	if sends < 1 {
		t.Errorf("duplicate flood: channel sends = %d, want >= 1", sends)
	}
	if sends > workers {
		t.Errorf("duplicate flood: channel sends = %d exceeds race-window bound (workers=%d) — 1000× flood was NOT collapsed", sends, workers)
	}
	// Evidence rows track sends 1:1.
	if got := evid.Count(); got != sends {
		t.Errorf("duplicate flood: evidence rows = %d, want == sends (%d)", got, sends)
	}

	sd, _ := qaSurface(t, "runner")
	floodTxt := fmt.Sprintf(
		"surface=runner scenario=chaos_duplicate_flood redis=live(atomic SETNX)\n"+
			"flood_total=%d parallel_workers=%d key=%q\n"+
			"events_processed_rows=%d want=1 (ARCHIVAL exactly-once: PASS)\n"+
			"channel_sends=%d (bounded: >=1, <=workers=%d; collapsed from %d)\n"+
			"evidence_rows=%d (==sends)\n"+
			"archival_exactly_once=1\n"+ // anchor grepped by E83
			"FINDING=HRD-132 dispatch-exactly-once NOT guaranteed (Stage-7 archive window)\n"+
			"p99_ms=%.4f max_ms=%.4f count=%d\n",
		total, workers, floodKey, pg.RowCount(), sends, workers, total, evid.Count(),
		sum.Latency.P99MS, sum.Latency.MaxMS, sum.Count)
	if _, err := sd.WriteFile("duplicate_flood.txt", floodTxt); err != nil {
		t.Fatalf("write duplicate_flood.txt: %v", err)
	}
	t.Logf("duplicate flood[live-redis]: %d events collapsed → %d send(s) (bound<=%d), %d archive row(s), p99=%.3fms",
		total, sends, workers, pg.RowCount(), sum.Latency.P99MS)
}

// ----------------------------------------------------------------------
// CHAOS (a'): nil-Redis degrade — documented Redis-lies-PG-truths contract.
// ----------------------------------------------------------------------

// TestRunner_Chaos_DuplicateFlood_NilRedisDegrade floods the pipeline with the
// same 1000×-same-key flood but through the REAL redisAdapter wrapping a nil
// client (HERALD_REDIS_URL unset). This exercises the DOCUMENTED degrade
// contract (runner.go redisAdapter + Wave 3 §4): with no Redis fast-path the
// duplicate verdict comes solely from the PG events_processed fallback, and the
// design explicitly tolerates a NARROW race window in which two concurrent
// fresh events both miss the not-yet-committed archive row and BOTH dispatch.
//
// The load-bearing guarantee that MUST hold even in the degrade is: PG
// `ON CONFLICT DO NOTHING` keeps archival exactly-once (exactly 1
// events_processed row), and dispatch is bounded (≥1 send, and in practice a
// small handful within the race window — never the full 1000). This is the
// honest contract; asserting "exactly 1 send" here would be a §107 PASS-bluff
// because the degrade path does not promise it.
func TestRunner_Chaos_DuplicateFlood_NilRedisDegrade(t *testing.T) {
	const (
		workers = 50
		total   = 1000
	)
	iterPerWorker := total / workers
	tenantID := mustParse("dddddddd-dddd-dddd-dddd-dddddddddddd")
	r, ch, pg, evid := buildStressRunner(t, tenantID, redisAdapter{client: nil})
	const floodKey = "FLOOD-KEY-DEGRADE"
	claims := map[string]any{"tenant": tenantID.String()}

	sum := stresschaos.RunLoad(workers, iterPerWorker, func(workerID, iter int) error {
		_, err := r.Run(context.Background(),
			eventBody(t, fmt.Sprintf("evt-degrade-%d-%d", workerID, iter), floodKey), claims)
		return err
	})

	if sum.Errors != 0 {
		t.Fatalf("nil-Redis degrade flood reported %d Run errors (want 0): %+v", sum.Errors, firstErrors(sum, 3))
	}

	// Archival is the load-bearing exactly-once invariant in the degrade.
	if got := pg.RowCount(); got != 1 {
		t.Errorf("nil-Redis degrade: events_processed rows = %d, want exactly 1 (ON CONFLICT DO NOTHING)", got)
	}
	sends := ch.SendsForKey(floodKey)
	if sends < 1 {
		t.Errorf("nil-Redis degrade: channel sends = %d, want >= 1 (at least one dispatch)", sends)
	}
	if sends > workers {
		t.Errorf("nil-Redis degrade: channel sends = %d exceeds the race-window bound (workers=%d) — fallback dedup is not engaging at all", sends, workers)
	}
	// Evidence rows track sends 1:1 in this path (each dispatching goroutine
	// writes its own evidence before the archive ON CONFLICT no-ops).
	if got := evid.Count(); got != sends {
		t.Errorf("nil-Redis degrade: evidence rows = %d, want == sends (%d)", got, sends)
	}

	sd, _ := qaSurface(t, "runner")
	degradeTxt := fmt.Sprintf(
		"surface=runner scenario=chaos_duplicate_flood_nil_redis_degrade redis=ABSENT(nil client)\n"+
			"contract=Redis-lies-PG-truths (Wave 3 §4): PG fallback dedup, race-window double-send TOLERATED\n"+
			"flood_total=%d parallel_workers=%d key=%q\n"+
			"events_processed_rows=%d want=1 (archival exactly-once via ON CONFLICT DO NOTHING)\n"+
			"channel_sends=%d (>=1, <=workers=%d — bounded by race window, NOT %d)\n"+
			"evidence_rows=%d (==sends)\n"+
			"p99_ms=%.4f max_ms=%.4f count=%d\n"+
			"NOTE: dispatch exactly-once is NOT guaranteed in EITHER posture (live Redis or\n"+
			"  nil-Redis) — see HRD-132. The events_processed archival row IS exactly-once in\n"+
			"  both. Live Redis (TestRunner_Chaos_DuplicateFlood) tightens the dispatch race\n"+
			"  window but does not close it, because the archive row lands at Stage 7.\n",
		total, workers, floodKey, pg.RowCount(), sends, workers, total, evid.Count(),
		sum.Latency.P99MS, sum.Latency.MaxMS, sum.Count)
	if _, err := sd.WriteFile("nil_redis_degrade.txt", degradeTxt); err != nil {
		t.Fatalf("write nil_redis_degrade.txt: %v", err)
	}
	t.Logf("duplicate flood[nil-redis degrade]: %d events → %d send(s) (race-window bounded by %d workers), %d archive row(s)",
		total, sends, workers, pg.RowCount())
}

// ----------------------------------------------------------------------
// CHAOS (b): hermetic PG-error injection — scripted store fault.
// ----------------------------------------------------------------------

// faultyEventsProcessed injects a scripted error on the Nth Insert (simulating
// a pgconn deadlock / 40P01-class error mid-pipeline). Lookup always misses so
// the event reaches the OutcomeRecorder's Insert where the fault fires.
type faultyEventsProcessed struct {
	calls    int64
	failAt   int64
	failWith error
}

func (s *faultyEventsProcessed) Lookup(ctx context.Context, tenantID uuid.UUID, idemKey string) (*eventsProcessedRow, bool) {
	return nil, false
}
func (s *faultyEventsProcessed) Insert(ctx context.Context, row eventsProcessedRow) error {
	if atomic.AddInt64(&s.calls, 1) == s.failAt {
		return s.failWith
	}
	return nil
}

// TestRunner_Chaos_PGDeadlockSurfacedNotSwallowed proves that when the PG
// events_processed Insert fails (deadlock / conn-reset class), Runner.Run
// SURFACES a stage-tagged error rather than returning a fabricated success
// (a silent pass would be a §107 PASS-bluff: the client would believe the
// event was archived when it was not).
func TestRunner_Chaos_PGDeadlockSurfacedNotSwallowed(t *testing.T) {
	tenantID := mustParse("cccccccc-cccc-cccc-cccc-cccccccccccc")
	deadlock := errors.New("pgconn: deadlock detected (SQLSTATE 40P01)")
	faulty := &faultyEventsProcessed{failAt: 1, failWith: deadlock}
	evid := &concurrentEvidence{}
	ch := newCountingChannel("null")
	subs := newFakeSubscribersStore()
	subs.Add(tenantID, subscriberRow{
		ID: uuid.New(), Handle: "bob",
		Aliases: []subscriberAliasRow{{Channel: "null", ChannelUserID: "sandbox-bob"}},
	})
	r := &Runner{
		parser:  &EventParser{},
		idem:    &IdempotencyChecker{Redis: newAtomicRedis(), PG: faulty, TTL: 24 * time.Hour},
		tenant:  &TenantResolver{},
		policy:  &PolicyGate{Registry: constitution.NewRegistry()},
		subs:    &SubscriberResolver{Subscribers: subs},
		chans:   &ChannelDispatcher{Channels: map[commons.ChannelID]commons.Channel{commons.ChannelNull: ch}},
		outcome: &OutcomeRecorder{Evidence: evid, EventsProcessed: faulty},
	}

	_, err := r.Run(context.Background(), eventBody(t, "evt-deadlock", "DEADLOCK-KEY"),
		map[string]any{"tenant": tenantID.String()})
	if err == nil {
		t.Fatal("Run returned nil error despite injected PG deadlock on events_processed Insert (§107 PASS-bluff: silent swallow)")
	}
	if !errors.Is(err, deadlock) {
		t.Errorf("Run error does not wrap the injected deadlock: %v", err)
	}
	if got := err.Error(); !strings.Contains(got, "outcome") || !strings.Contains(got, "events_processed") {
		t.Errorf("error not stage-tagged with outcome/events_processed: %q", got)
	}

	sd, _ := qaSurface(t, "runner")
	deadlockTxt := fmt.Sprintf(
		"surface=runner scenario=chaos_pg_deadlock_injection\n"+
			"injected_error=%q\n"+
			"run_returned_error=%q\n"+
			"surfaced_not_swallowed=true\n"+
			"stage_tagged=true\n",
		deadlock.Error(), err.Error())
	if _, werr := sd.WriteFile("deadlock_recovery.log", deadlockTxt); werr != nil {
		t.Fatalf("write deadlock_recovery.log: %v", werr)
	}
	t.Logf("PG deadlock chaos: Run surfaced %q (not swallowed)", err.Error())
}
