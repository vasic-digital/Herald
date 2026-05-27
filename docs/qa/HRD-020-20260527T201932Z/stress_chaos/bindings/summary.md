# Stress + Chaos — sherald host/repo-safety constitution bindings (HRD-020)

Constitutional anchor: Helix §11.4.85 (stress + chaos mandate) / Herald §107.x.
Captured: 2026-05-28T01:19:33+05:00  (persistent=true)

## Scenarios

| scenario | verdict | evidence file | key numbers |
|---|---|---|---|
| stress_concurrent_breaches (§9.1 destructive-op) | PASS | concurrent_breaches.log, latency.json | 1200 evals, 0 errors, every emit + audit accounted (no lost breaches), p50=0.173ms p95=0.586ms p99=0.861ms tput=33027/s |
| stress_concurrent_mem_budget (§12.6) | PASS | concurrent_mem_budget.log | simulated used_fraction readings classified under load; NO host memory allocated (§12.6-safe) |
| chaos_safety_emit_fault_fail_loud (§12.1) | PASS | safety_emit_fault_fail_loud.log | bus-drop → 100% of safety-breach emit faults surface via EvaluateSubject error, 0 swallowed |

## Host-safety (§12 / §12.6) — CRITICAL for this unit

Bounded in-process load only: N=8 workers × M≤150 per scenario, small string subjects, no fork/GB-alloc/host-net. The detectors are PURE classifiers — they read a Subject string describing an attempted op and return a verdict; they NEVER execute rm/reset/force-push/suspend nor allocate memory to reach a real mem-budget breach. The mem-budget scenario feeds fabricated used_fraction strings. Race detector is the concurrency-correctness evidence (run under -race -count=3).
host_mem used_fraction=0.617 total_bytes=19327352832 crosses_60pct_ceiling=true platform=darwin

## Anti-bluff posture (§107 / §11.4.27)

Real bindings.Pipeline over a real MemoryBus + real Memory store/ladder/audit. Only the EXTERNAL emit boundary is faulted (faultEmitter for the bus-drop chaos). No pipeline logic is mocked; all numbers are captured runtime output.
