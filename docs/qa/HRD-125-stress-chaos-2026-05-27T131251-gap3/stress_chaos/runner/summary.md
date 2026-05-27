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
- Shared key dispatched shared_key_sends=3 - bounded (>=1, <=workers=16), NOT exactly-once (see FINDING).
- Latency (fresh-key Run): p50=0.252ms p95=1.075ms p99=2.966ms max=3.228ms, throughput ~35.2k Run/s.

### 2. CHAOS (a) - duplicate flood, 1000x same key @ 50 parallel (live atomic Redis SETNX)
TestRunner_Chaos_DuplicateFlood - PASS

- 1000 runner.Run calls, all the SAME idempotency key, 0 errors.
- Archival exactly-once: PASS - events_processed_rows=1 (1000x flood collapsed to a single archive row).
- Dispatch collapsed to channel_sends=4 - bounded (>=1, <=workers=50), NOT the full 1000. Evidence rows == sends.
- p99=2.266ms max=4.345ms over the 1000-call flood.

### 3. CHAOS (a') - nil-Redis degrade flood, 1000x same key @ 50 parallel
TestRunner_Chaos_DuplicateFlood_NilRedisDegrade - PASS

- Real redisAdapter{client:nil} (HERALD_REDIS_URL unset) -> PG-only fallback (Wave 3 §4 Redis-lies-PG-truths). 0 errors.
- Archival exactly-once: PASS - events_processed_rows=1 via ON CONFLICT DO NOTHING.
- Dispatch channel_sends=2 - bounded by the documented race window, never 1000.

### 4. CHAOS (b) - PG deadlock injection, fail-loud-not-swallow
TestRunner_Chaos_PGDeadlockSurfacedNotSwallowed - PASS

- Hermetic faulty store returns "pgconn: deadlock detected (SQLSTATE 40P01)" on the events_processed Insert.
- runner.Run surfaced the error stage-tagged: "outcome: archive events_processed: pgconn: deadlock detected (SQLSTATE 40P01)".
- Proves Run does NOT silently swallow a PG fault (that would be a §107 PASS-bluff).

## FINDING - HRD-132: dispatch exactly-once is NOT guaranteed under concurrent replay

The Runner archives the events_processed row at Stage 7 (OutcomeRecorder), i.e.
at the END of the pipeline - AFTER the Stage-6 channel dispatch. The idempotency
check (Stage 2) consults Redis SETNX + a PG events_processed Lookup. Therefore
the window between "Stage-2 fresh verdict" and "Stage-7 archive commit" admits
concurrent duplicate dispatch: a live Redis SETNX makes exactly one goroutine
take the fresh fast-path, but losing goroutines fall through to a PG Lookup that
has NOT yet been populated (the winner is still mid-pipeline) -> they too are
judged fresh -> they too dispatch.

Observed: a 1000x-same-key flood @ 50 parallel collapses to a small handful of
dispatches (2-4 across runs), bounded by the concurrent window, never the full
1000 - but not 1. The race detector is clean (this is a logic/ordering gap, not
a memory race).

What IS guaranteed today (and asserted above): archival exactly-once (exactly 1
events_processed row) and bounded dispatch (<= concurrent window). The honest
contract is recorded; asserting sends==1 would be a §107 PASS-bluff because the
code does not deliver it. Closing the gap (e.g. write the archive row inside the
SETNX-winning critical section, or gate dispatch on a committed archive row) is
tracked as HRD-132 - out of scope for this test-only unit (no production edits).

A LATENT data race (runner.go:132 rc.CachedRcpt.WasReplay = true) was also
observed when a store returns a SHARED cached *Receipt pointer to concurrent
replays. The CURRENT production pgEventsProcessedAdapter.Lookup returns
Receipt=nil (it does not cache the Receipt yet), so production takes the
CachedRcpt==nil fresh-synthesis branch and the race does NOT manifest today. It
would activate when the planned Wave 4+ full-Receipt-caching lands - fold that
guard into HRD-132 / the caching HRD.

## Anti-bluff (§11.4 / §11.4.85) statement

Every number above is captured runtime output from go test -race -count=1. The
tests drive the real runner.Run; no mock-of-the-thing-under-test. The clean
-race result (race_clean.log) is the concurrency-correctness evidence. The two
genuine findings (HRD-132 dispatch gap + latent CachedRcpt race) were discovered
BY these tests refusing to assert an aspirational green.
