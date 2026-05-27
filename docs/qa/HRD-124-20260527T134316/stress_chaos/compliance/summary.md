# Stress + Chaos — cherald GET /v1/compliance (HRD-124)

Plan: docs/superpowers/plans/2026-05-27-stress-chaos-suite.md §1 row 2.
Constitutional anchor: Helix §11.4.85 (stress + chaos mandate) / Herald §107.x.
Captured: 2026-05-27T13:43:17+05:00  (persistent=true)

## Scenarios

| scenario | verdict | evidence file | key numbers |
|---|---|---|---|
| stress_healthy_path | PASS | throughput.csv, latency.json, latency_histogram.csv | 2000 req, 0 errors, 0 5xx, p50=1.160ms p95=1.996ms p99=2.587ms tput=8378/s |
| content_negotiation_under_load | PASS | throughput.csv | every Accept:toon → toon CT + real TOON wire bytes; every Accept:json → json CT; 0 codec-bleed under 10-worker concurrency |
| chaos_pg_drop_fail_loud | PASS | pg_drop_fail_loud.log | store-down → 100% 5xx naming the dependency, 0 fabricated 200 |
| chaos_pg_drop_live | SKIP-with-reason | pg_drop_live.log | container-pause requires HERALD_STRESS_LIVE_PG + runtime; hermetic errStore variant proves fail-loud |
| chaos_auth_storm | PASS | auth_storm.log | ~1000 random bearer tokens → 100% 401 via REAL HMAC verifier, 0 bypass |

## Host-safety (§12 / §12.6)

Bounded load only: N=10 workers × M=200 = 2000 req per scenario, small GET requests, no fork/GB-alloc/host-net-change. Race detector is the concurrency-correctness evidence (run under -race -count=3).
host_mem used_fraction=0.735 total_bytes=19327352832 crosses_60pct_ceiling=true platform=darwin

## Anti-bluff posture (§107 / §11.4.27)

Real compliance.Handler over a real httptest server + net/http.Client. Only the EXTERNAL boundary is faked: state.NewMemory() (healthy) or errStore (PG-drop). The auth storm drives the REAL commons_auth HMAC verifier. No handler is mocked; all evidence is captured runtime output.
