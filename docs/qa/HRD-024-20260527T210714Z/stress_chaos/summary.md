# Stress + Chaos — iherald incident/escalation constitution bindings (HRD-024)

Constitutional anchor: Helix §11.4.85 (stress + chaos mandate) / Herald §107.x.
Captured: 2026-05-28T02:07:14+05:00  (persistent=true)

## Scenarios

| scenario | verdict | evidence file | key numbers |
|---|---|---|---|
| stress_concurrent_credential_leaks | PASS | concurrent_credential_leaks.log, latency.json | 1200 evals, 0 errors, every .credential.leak page-out + audit accounted (no lost page-outs), p50=0.161ms p95=0.516ms p99=0.786ms tput=38188/s |
| chaos_page_out_fault_fail_loud | PASS | page_out_fault_fail_loud.log | bus-drop → 100% of page-out faults surface via EvaluateSubject error, 0 swallowed |

## No-real-secret attestation (§107.x)

Every credential-leak Subject in this suite is a FABRICATED "fake_leak_<w>_<i>" location plus a boolean detection flag. NO real .env is scanned and NO real secret string appears anywhere in the test file or its captured evidence.

## Host-safety (§12 / §12.6)

Bounded in-process load only: N=8 workers × M≤150 evaluations per scenario, small fabricated credential-leak subjects, NO real credential scans, no fork/GB-alloc/host-net. Race detector is the concurrency-correctness evidence (run under -race -count=3).
host_mem used_fraction=0.622 total_bytes=19327352832 crosses_60pct_ceiling=true platform=darwin

## Anti-bluff posture (§107 / §11.4.27)

Real bindings.Pipeline over a real MemoryBus + real Memory store/ladder/audit. Only the EXTERNAL boundary is faulted (faultEmitter for the bus-drop chaos). No pipeline logic is mocked; all numbers are captured runtime output.
