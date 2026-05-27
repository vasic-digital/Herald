# Stress + Chaos — sherald GET /v1/safety_state (HRD-124)

Plan: docs/superpowers/plans/2026-05-27-stress-chaos-suite.md §1 row 2.
Constitutional anchor: Helix §11.4.85 (stress + chaos mandate) / Herald §107.x.
Captured: 2026-05-27T13:43:17+05:00  (persistent=true)

## Surface note (honest contract)

/v1/safety_state is process-local in-memory BY DESIGN (Wave 3 design §3) — the
handler reads an Aggregator snapshot and does NO PG read on the hot path. The
plan-row "PG-drop → fail-loud 5xx" variant therefore has no DB dependency to
drop on this surface; it is SKIP-not-applicable (rationale in
pg_drop_not_applicable.log). The fail-loud-on-store-error property is proven on
cherald /v1/compliance (the surface that DOES have a store). The code-true
fault-injection here is concurrent state mutation under load.

## Scenarios

| scenario | verdict | evidence file | key numbers |
|---|---|---|---|
| stress_healthy_path | PASS | throughput.csv, latency.json, latency_histogram.csv | 2000 req, 0 errors, 0 5xx, p50=0.490ms p95=1.428ms p99=1.903ms tput=16064/s |
| content_negotiation_under_load | PASS | throughput.csv | every Accept:toon → toon CT + real TOON wire bytes; every Accept:json → json CT; 0 codec-bleed under 10-worker concurrency |
| chaos_concurrent_mutation_under_load | PASS | concurrent_mutation.log | 2000 GETs under 4-writer storm → all 200 + well-formed snapshot, 0 torn read, 0 5xx, race-clean |
| chaos_pg_drop | SKIP-not-applicable | pg_drop_not_applicable.log | no DB dependency on this in-memory surface; fail-loud proven on cherald /v1/compliance |
| chaos_auth_storm | PASS | auth_storm.log | ~1000 random bearer tokens → 100% 401 via REAL HMAC verifier, 0 bypass |

## Host-safety (§12 / §12.6)

Bounded load only: N=10 workers × M=200 = 2000 req per scenario + 4 bounded background writer goroutines, small GET requests, no fork/GB-alloc/host-net-change. Race detector is the concurrency-correctness evidence (run under -race -count=3).
host_mem used_fraction=0.736 total_bytes=19327352832 crosses_60pct_ceiling=true platform=darwin

## Anti-bluff posture (§107 / §11.4.27)

Real safety.Handler over a real httptest server + net/http.Client. Only the EXTERNAL boundary is faked (the auth claims-injector). The auth storm drives the REAL commons_auth HMAC verifier. No handler is mocked; all evidence is captured runtime output.
