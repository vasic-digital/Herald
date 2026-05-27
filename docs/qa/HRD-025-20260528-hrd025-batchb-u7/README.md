<div align="center">

<img src="../../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# HRD-025 — scherald scheduled-audit constitution bindings — QA evidence

| Field | Value |
|---|---|
| HRD | HRD-025 (v1.0.0 Batch B, unit 7) |
| Run-id | 20260528-hrd025-batchb-u7 |
| Date | 2026-05-28 |
| Flavor | `scherald` (Scheduled-audit Herald) |
| Spec anchors | V3 §42.3 master binding table (scherald owns §11.4.45), §42.5 step 8, §18.7 digest cadence, §43 HRD-047 row |
| Constitutional anchors | Helix §11.4.85 (stress+chaos), Herald §107 / §107.x (docs/qa evidence mandate) |
| Package under test | `scherald/internal/bindings` |
| Verdict | PASS — TDD RED→GREEN, `-race -count=3` deterministic green |

## What shipped

HRD-025 wires scherald's §42.3-owned scheduled-audit rules into the Batch-A
`commons_constitution` foundation (Evaluator + Registry + ModeLadder +
ConstitutionStore + AuditStore + typed event emitter), mirroring the proven
HRD-019/020/021/022 Pipeline pattern. scherald is the SCHEDULED-AUDIT flavor:
its sole §42.3 master-table row is **§11.4.45 Integration-Status-Doc
Maintenance** (`.policy.violation`, warn, low). HRD-025 lands that row plus two
scheduled-audit-specific bespoke facets the §43 / HRD-047 `scherald status
digest [--cadence=daily|weekly|monthly]` command surfaces:

| Rule ID | Detector | Routes through | What it catches |
|---|---|---|---|
| `§11.4.45` | `checkStatusSweep` | `.policy.violation` | periodic Status.md sweep flagged drift, OR Status_Summary.md out of sync (§11.4.56 composition) |
| `§11.4.45.digest` | `checkDigestCadence` | `.policy.violation` | a scheduled daily/weekly/monthly compliance digest fell DUE but was never emitted (§18.7 contract) |
| `§11.4.45.stale` | `checkStaleItem` | `.policy.violation` | open work-item count with no status movement exceeds the configured threshold |

All three detectors are PURE (§12 host-safety): each CLASSIFIES a recorded
sweep / digest-cadence / staleness outcome string. NONE reads Status.md, runs a
cron tick, regenerates a digest, walks the HRD trackers, or touches the
filesystem / process table / network. The live cron / scheduler integration that
feeds these detectors their Subjects is scope-locked to §43 / HRD-047
(`scherald status digest` + `POST /v1/schedule/status-digest`).

## Bidirectional round-trip (the §107 anti-bluff bar)

`roundtrip_transcript.txt` is the verbose `go test -v` capture of the full
evaluate→record→gate→emit→audit round-trip. The load-bearing proof
(`TestPipeline_StatusSweepPolicyViolationRoundTrip`):

```
INBOUND  (scheduler → binding):  Subject{Kind:"status-sweep", ID:"Status.md|sweep=stale|stale_items=4"}
                                  ladder mode = enforce
DETECT   (checkStatusSweep):     Decision=FAIL  "§11.4.45: Status.md is STALE (periodic sweep flagged drift; 4 stale items)"
EMIT     (bus):                  1× digital.vasic.herald.constitution.policy.violation  (observed by a live subscriber goroutine)
PERSIST  (ConstitutionStore):    1× constitution_state row, decision=fail, subject="Status.md|sweep=stale|stale_items=4"
AUDIT    (AuditStore):           1× constitution_audit row, ModeAtEmission=enforce, EmittedEventID != Nil
```

The recovery direction (`TestPipeline_PolicyClearedOnRecovery`) proves a
digest cadence that transitions FAIL→PASS emits `.policy.cleared` (NOT another
`.policy.violation`) — the recovery signal subscribers key on.

## TDD RED→GREEN

- **RED** (demonstrated in the work session, not committed): `checkStatusSweep`
  was temporarily mutated to `return pass(...)` unconditionally (the silent-pass
  bluff). 4 tests FAILed and named the exact bluff:
  `TestPipeline_StatusSweepPolicyViolationRoundTrip`,
  `TestPipeline_LadderGatesEmit`, `TestDetector_StatusSweep`,
  `TestPipeline_MalformedSubjectDoesNotPass`. The mutation was restored from a
  private backup (NOT git) and `grep RED-DEMO` confirms zero residue (§107.y
  quiescence).
- **GREEN**: `go test -race -count=3 ./scherald/internal/bindings/...` → `ok`
  (deterministic, 3 consecutive iterations, race detector clean). See
  `roundtrip_transcript.txt`.

## §11.4.85 stress + chaos

`stress_chaos/` carries the resilience-layer evidence (run under `-race
-count=3`, bounded N=8 workers × M≤150 per §12 host-safety):

| scenario | evidence | key numbers |
|---|---|---|
| `stress_concurrent_status_sweeps` | `concurrent_status_sweeps.log`, `latency.json`, `latency_histogram.csv` | 1200 evals, 0 errors, 1200 `.policy.violation` emits + 1200 audit rows (no lost events under contention), p50=0.154ms p95=0.399ms p99=1.206ms, ~39k eval/s |
| `chaos_emit_fault_fail_loud` | `emit_fault_fail_loud.log` | 800 evals → 100% of emit faults surface via `EvaluateSubject` error, 0 swallowed (fail-loud, no §107 distribution bluff) |

See `stress_chaos/summary.md` for the host-mem headroom probe + full posture.

## Scope-locked follow-ups

- Live cron / scheduler that produces real sweep / digest / staleness Subjects →
  §43 HRD-047 (`scherald status digest` CLI + `POST /v1/schedule/status-digest`).
- e2e_bluff_hunt invariant **E96** (proposed; NOT wired in this unit — conductor
  owns `scripts/e2e_bluff_hunt.sh`): build scherald + run
  `go test ./scherald/internal/bindings/...` + assert the §11.4.45 status-sweep
  round-trip emits a real `.policy.violation` with a queryable state row + audit
  row; anchor on this `docs/qa/HRD-025-<run-id>/` directory per §11.4.2.

## Files

```
roundtrip_transcript.txt              verbose go test -v of the full round-trip + all detector cases
stress_chaos/stress_chaos_run.txt     verbose go test -v of both stress + chaos scenarios
stress_chaos/concurrent_status_sweeps.log  stress scenario captured metrics
stress_chaos/emit_fault_fail_loud.log      chaos scenario captured metrics
stress_chaos/latency.json                  latency percentiles (machine-readable)
stress_chaos/latency_histogram.csv         per-sample latency histogram
stress_chaos/summary.md                    stress+chaos summary + host-mem probe
```
