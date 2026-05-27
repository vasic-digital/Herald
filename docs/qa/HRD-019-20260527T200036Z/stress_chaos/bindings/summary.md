# Stress + Chaos — cherald constitution bindings (HRD-019)

Constitutional anchor: Helix §11.4.85 (stress + chaos mandate) / Herald §107.x.
Captured: 2026-05-28T01:01:23+05:00  (persistent=true)

## Scenarios

| scenario | verdict | evidence file | key numbers |
|---|---|---|---|
| stress_concurrent_violations | PASS | concurrent_violations.log, latency.json | 1200 evals, 0 errors, every emit + audit accounted (no lost events), p50=0.154ms p95=0.502ms p99=0.917ms tput=39179/s |
| chaos_emit_fault_fail_loud | PASS | emit_fault_fail_loud.log | bus-drop → 100% of emit faults surface via EvaluateSubject error, 0 swallowed |

## Host-safety (§12 / §12.6)

Bounded in-process load only: N=8 workers × M≤150 evaluations per scenario, small subjects, no fork/GB-alloc/host-net. Race detector is the concurrency-correctness evidence (run under -race -count=3).
host_mem used_fraction=0.588 total_bytes=19327352832 crosses_60pct_ceiling=false platform=darwin

## Anti-bluff posture (§107 / §11.4.27)

Real bindings.Pipeline over a real MemoryBus + real Memory store/ladder/audit. Only the EXTERNAL boundary is faulted (faultEmitter for the bus-drop chaos). No pipeline logic is mocked; all numbers are captured runtime output.
