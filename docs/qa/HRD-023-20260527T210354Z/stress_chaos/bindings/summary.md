# Stress + Chaos — pherald PROJECT constitution bindings (HRD-023)

Constitutional anchor: Helix §11.4.85 (stress + chaos mandate) / Herald §107.x.
Captured: 2026-05-28T02:04:38+05:00  (persistent=true)

## Scenarios

| scenario | verdict | evidence file | key numbers |
|---|---|---|---|
| stress_concurrent_repo_breaches | PASS | concurrent_repo_breaches.log, latency.json | 1200 evals, 0 errors, every .repo.safety.breach emit + audit accounted (no lost events), p50=0.157ms p95=0.436ms p99=0.905ms tput=39210/s |
| chaos_emit_fault_fail_loud | PASS | emit_fault_fail_loud.log | bus-drop → 100% of emit faults surface via EvaluateSubject error, 0 swallowed |

## Host-safety (§12 / §12.6)

Bounded in-process load only: N=8 workers × M≤150 evaluations per scenario, small fabricated project-result subjects, NO real commit/push/git — the project detectors DETECT/classify only. No fork/GB-alloc/host-net. Race detector is the concurrency-correctness evidence (run under -race -count=3).
host_mem used_fraction=0.611 total_bytes=19327352832 crosses_60pct_ceiling=true platform=darwin

## Anti-bluff posture (§107 / §11.4.27)

Real bindings.Pipeline over a real MemoryBus + real Memory store/ladder/audit. Only the EXTERNAL boundary is faulted (faultEmitter for the bus-drop chaos). No pipeline logic is mocked; all numbers are captured runtime output.
