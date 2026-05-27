# HRD-125 — Runner 7-stage pipeline stress + chaos evidence (§11.4.85 / GAP-3 row 3)

| Field | Value |
|---|---|
| Run ID | `HRD-125-stress-chaos-2026-05-27T131251-gap3` |
| Surface | `pherald/internal/runner` (the §32 7-stage `runner.Run` pipeline) |
| Host | darwin (Apple Silicon), Go 1.26.2, 18432 MiB RAM |
| Mode | hermetic (in-process Go; in-memory production-faithful fakes; no PG/Redis container required) |
| Command | `go test -race -count=1 ./pherald/internal/runner/...` |
| Race detector | CLEAN (0 data races across all four tests under N=16 and N=50 fan-out) |
| Host-safety | in-process, <=50 goroutines, sub-KiB payloads -> ZERO §12.6 risk (see host_memory_headroom.txt) |

These tests exercise the real `runner.Run` (the thing under test). Per §11.4.27
only the EXTERNAL boundary is faked (PG events_processed store with real
ON CONFLICT DO NOTHING + Receipt=nil Lookup semantics, channel sink, Redis
SETNX seam). The Runner orchestrator and all seven stages run unmodified.

## Assertions + captured results

### 1. STRESS - N=16 goroutines x 40 iters, concurrent replay (live atomic Redis SETNX)
TestRunner_Stress_ConcurrentReplay_ExactlyOnce - PASS

- 1280 runner.Run calls (640 shared-key replays + 640 fresh unique keys), 0 errors.
- Archival exactly-once (load-bearing): PASS - events_processed_rows=641 (1 shared + 640 fresh).
- Fresh keys each dispatched exactly once (640 sends, 0 unexpected replays).
- Shared key dispatched shared_key_sends=1 - DISPATCH exactly-once (HRD-132 claim-before-dispatch FIXED).
- Latency (fresh-key Run): p50~0.35ms p95~1.7ms p99~3.3ms, throughput ~tens of k Run/s.

### 2. CHAOS (a) - duplicate flood, 1000x same key @ 50 parallel (live atomic Redis SETNX)
TestRunner_Chaos_DuplicateFlood - PASS

- 1000 runner.Run calls, all the SAME idempotency key, 0 errors.
- Archival exactly-once: PASS - events_processed_rows=1 (1000x flood collapsed to a single archive row).
- Dispatch EXACTLY-ONCE: channel_sends=1 (1000x flood collapsed to a single dispatch). HRD-132 FIXED.

### 3. CHAOS (a') - nil-Redis degrade flood, 1000x same key @ 50 parallel
TestRunner_Chaos_DuplicateFlood_NilRedisDegrade - PASS

- Real redisAdapter{client:nil} (HERALD_REDIS_URL unset) -> PG-only fallback (Wave 3 §4 Redis-lies-PG-truths). 0 errors.
- Archival exactly-once: PASS - events_processed_rows=1 via ON CONFLICT DO NOTHING.
- Dispatch EXACTLY-ONCE: channel_sends=1 even without Redis - the Stage-2 PG CLAIM is the authoritative gate. HRD-132 FIXED.

### 3b. RACE - shared cached Receipt replay, N=32 x 40 = 1280 concurrent same-key replays
TestRunner_Race_SharedCachedReceiptReplay_NoDataRace - PASS

- Models the planned Wave-4+ Receipt-caching store (Lookup returns a SHARED *Receipt to every replay).
- -race CLEAN: the replay short-circuit returns a COPY (runner.go) instead of mutating the shared pointer.
- fresh=1 dispatch, 1279 replays, shared.WasReplay never mutated. HRD-132 latent race FIXED (load-bearing under -race).

### 4. CHAOS (b) - PG deadlock injection, fail-loud-not-swallow
TestRunner_Chaos_PGDeadlockSurfacedNotSwallowed - PASS

- Hermetic faulty store returns "pgconn: deadlock detected (SQLSTATE 40P01)" on the events_processed Insert.
- runner.Run surfaced the error stage-tagged: "outcome: archive events_processed: pgconn: deadlock detected (SQLSTATE 40P01)".
- Proves Run does NOT silently swallow a PG fault (that would be a §107 PASS-bluff).

## HRD-132 — FIXED (2026-05-27): dispatch exactly-once + latent race closed

This evidence was RE-CAPTURED after the HRD-132 fix landed. The two findings
the original HRD-125 run recorded are now closed:

### (1) Dispatch exactly-once — FIXED via claim-before-dispatch

The events_processed CLAIM moved from Stage 7 (OutcomeRecorder, end of pipeline)
to Stage 2 (IdempotencyChecker). At Stage 2 the Runner performs an atomic
`INSERT INTO events_processed … ON CONFLICT DO NOTHING` and treats the row
insertion itself as the dispatch grant: the caller that inserts the row (1 row
affected) WON the claim and dispatches; every concurrent caller that observes 0
rows affected LOST and short-circuits as a duplicate BEFORE Stage 6. Because the
PG PRIMARY KEY(tenant_id, idempotency_key) serialises the concurrent inserts,
exactly one caller per key is granted dispatch.

Observed post-fix: the 1000x-same-key flood @ 50 parallel now collapses to
EXACTLY 1 dispatch (channel_sends=1), in BOTH the live-Redis posture and the
nil-Redis degrade posture (the PG claim is authoritative regardless of Redis).
The Redis SETNX fast-path is preserved as an optimisation (a confirmed-archived
replay short-circuits via Lookup without racing the claim INSERT).
sends==1 is now a code-true assertion, not a §107 PASS-bluff.

### (2) Latent CachedRcpt.WasReplay data race — FIXED via copy-on-replay

The replay short-circuit (runner.go) no longer mutates `rc.CachedRcpt.WasReplay`
in place. It returns a COPY of the cached Receipt with WasReplay flipped, so a
SHARED cached *Receipt pointer (the planned Wave-4+ Receipt-caching semantics)
is never written concurrently. Proven load-bearing by
TestRunner_Race_SharedCachedReceiptReplay_NoDataRace (1280 concurrent same-key
replays of one shared cached Receipt, -race CLEAN; re-introducing the in-place
mutation makes -race report a DATA RACE and the test FAIL).

Both archival-exactly-once (PG UNIQUE + ON CONFLICT DO NOTHING) AND
dispatch-exactly-once now hold. The Stage-4 DecisionFail → RecordDenied path is
reconciled: the Stage-2 claim writes the events_processed row, so RecordDenied's
(and Process's) Stage-7 archive Insert is an idempotent ON CONFLICT DO NOTHING
no-op on the pre-existing row (no double-insert, no error).

## Anti-bluff (§11.4 / §11.4.85) statement

Every number above is captured runtime output from go test -race -count=1. The
tests drive the real runner.Run; no mock-of-the-thing-under-test. The clean
-race result (race_clean.log) is the concurrency-correctness evidence. The two
genuine findings (HRD-132 dispatch gap + latent CachedRcpt race) were discovered
BY these tests refusing to assert an aspirational green.
