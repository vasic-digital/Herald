# Stress + Chaos — scherald scheduled-audit constitution bindings (HRD-025)

Constitutional anchor: Helix §11.4.85 (stress + chaos mandate) / Herald §107.x.
Captured: 2026-05-28T02:05:21+05:00  (persistent=true)

## Scenarios

| scenario | verdict | evidence file | key numbers |
|---|---|---|---|
| stress_concurrent_status_sweeps | PASS | concurrent_status_sweeps.log, latency.json | 1200 evals, 0 errors, every .policy.violation emit + audit accounted (no lost events), p50=0.154ms p95=0.399ms p99=1.206ms tput=39244/s |
| chaos_emit_fault_fail_loud | PASS | emit_fault_fail_loud.log | bus-drop → 100% of emit faults surface via EvaluateSubject error, 0 swallowed |

## Host-safety (§12 / §12.6)

Bounded in-process load only: N=8 workers × M≤150 evaluations per scenario, small fabricated scheduled-audit subjects, NO real Status.md read / cron tick / digest regen — the detectors DETECT/classify only. No fork/GB-alloc/host-net. Race detector is the concurrency-correctness evidence (run under -race -count=3).
host_mem used_fraction=0.618 total_bytes=19327352832 crosses_60pct_ceiling=true platform=darwin

## Anti-bluff posture (§107 / §11.4.27)

Real bindings.Pipeline over a real MemoryBus + real Memory store/ladder/audit. Only the EXTERNAL boundary is faulted (faultEmitter for the bus-drop chaos). No pipeline logic is mocked; all numbers are captured runtime output.
