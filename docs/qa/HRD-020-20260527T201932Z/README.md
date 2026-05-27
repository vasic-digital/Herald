# HRD-020 — sherald host-safety + repo-safety constitution bindings (QA evidence)

| Field | Value |
|---|---|
| HRD | HRD-020 (v1.0.0 Batch B, unit 2) |
| Run ID | HRD-020-20260527T201932Z |
| Captured | 2026-05-27 (UTC) |
| Subject | `sherald/internal/bindings` — host/repo-safety constitution bindings Pipeline |
| Constitutional anchors | Herald §107.x (docs/qa evidence mandate) · Helix §11.4.85 (stress+chaos) · §12 / §12.6 (host-safety) |

## What this proves (§107 anti-bluff bar)

sherald is the SYSTEM / SAFETY flavor. HRD-020 wires its §42.3-owned host-safety
+ repo-safety constitution rules into the Batch-A `commons_constitution`
Evaluator + ModeLadder + ConstitutionStore + AuditStore + typed safety-event
emitter foundation, mirroring the proven cherald HRD-019 pattern but routing the
emit through the SAFETY event classes (`.host.safety.breach`,
`.repo.safety.breach`, `.gate.recovered`, `.bundle.updated`) that cherald never
touched.

The load-bearing round-trip: a SAFETY rule violation (a destructive op, a
force-push without merge-first, a 60%-mem-budget breach, a forbidden host op) is
DETECTED by a registered sherald binding → EMITTED as the rule's safety event
class on the bus → PERSISTED as a `constitution_state` row AND a
`constitution_audit` row.

## §12 / §12.6 host-safety attestation — NO real destructive op performed

CRITICAL for this unit: the detection hooks (destructive-op detector, force-push
interceptor, mem-budget watcher, forbidden-host-op detector) are PURE
classifiers. Each reads a `Subject` whose ID encodes the already-observed op
parameters (e.g. `"git reset --hard HEAD~5|backup=false"`,
`"used_fraction=0.83"`) and returns a verdict. They GUARD — they NEVER:

- execute `rm` / `git reset` / `git clean`,
- run `git push --force`,
- suspend/logout/shutdown the host,
- allocate memory to reproduce a mem-budget breach.

The `TestDetectors_NeverPerformOps` unit test + the structural witness (the
test harness + host survive the full suite, including `used_fraction=0.99` and
`rm -rf /` subject strings) are the §12-safety proof. The mem-budget stress
scenario feeds fabricated `used_fraction` strings — `NO_HOST_MEMORY_ALLOCATED=1`.

## Evidence files

| File | Content |
|---|---|
| `safety_roundtrip_evidence.txt` | §107.x detect→emit→persist→audit transcript for §9.1 / §11.4.41 / §12.6 / §12.1 — REAL emitted wire-event type + envelope + `SafetyEvent` payload (incl. `BreachKind`) + persisted state row + audit row. Confirms the SIMULATED op was classified, not executed (`emitted=true err=<nil>`, evidence string names the unmet prerequisite). |
| `roundtrip_transcript.txt` | `go test -race -v` PASS transcript for every round-trip + ladder-gate + detector-purity unit test. |
| `stress_chaos/bindings/concurrent_breaches.log` | §11.4.85 stress: 8 workers × 150 = 1200 concurrent §9.1 destructive-op breaches, 0 errors, 1200 delivered + 1200 audit rows (no lost breaches under load), p99≈0.86ms, race-clean. |
| `stress_chaos/bindings/concurrent_mem_budget.log` | §11.4.85 stress: 960 concurrent §12.6 mem-budget classifications (480 breach / 480 clean), 480 host.safety.breach delivered, NO host memory allocated. |
| `stress_chaos/bindings/safety_emit_fault_fail_loud.log` | §11.4.85 chaos: faultEmitter drops Host/RepoSafetyBreach → all 800 emit faults surface via `EvaluateSubject` error, 0 swallowed (no §107 distribution bluff). |
| `stress_chaos/bindings/latency.json`, `latency_histogram.csv`, `summary.md` | captured latency percentiles + summary. |

## Reproduce

```bash
# unit + round-trip (deterministic, 3×):
go test -race -count=3 ./sherald/internal/bindings/...

# capture persistent stress evidence:
HERALD_STRESS_QA_DIR="$PWD/docs/qa/<run-id>/stress_chaos" \
HERALD_STRESS_RUN_ID="<run-id>" \
  go test -race -count=3 ./sherald/internal/bindings/...
```

## Note for conductor — latent eventbus bug surfaced

The stress tests count delivered events via a REAL subscriber + race-free
`atomic.AddInt64`, deliberately NOT `bus.Metrics().PublishedByType`. That bus
counter is maintained by a check-then-act `sync.Map` Load/Store in
`commons_constitution/eventbus.go` `Publish` (lines ~127-133) which RACES under
concurrency and under-counts by 1 on the first-publish window (observed
1199/1200, 479/480 here). `eventbus.go` is out of this unit's edit scope; the
subscriber-side count is the stronger anti-bluff measure regardless. Flagged for
a follow-up fix (LoadOrStore-with-loaded, or atomic counter init at Subscribe).
