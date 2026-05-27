# HRD-021 — bherald CI/test constitution bindings — §107.x QA evidence

| Field | Value |
|---|---|
| HRD | HRD-021 (v1.0.0 Batch B, unit 3) |
| Run ID | HRD-021-20260527T203734Z |
| Flavor | `bherald` (Build / CI Herald) |
| Package | `bherald/internal/bindings` |
| Foundation | Batch-A `commons_constitution` (Evaluator + Registry + ModeLadder + ConstitutionStore + AuditStore + emitter) — HRD-018 |
| Pattern | mirrors HRD-019 (cherald) + HRD-020 (sherald) Pipeline |

## What bherald owns (spec V3 §42.3)

bherald is the BUILD/CI flavor. It registers the 22 §42.3-owned CI/test
gate-result rules as `commons_constitution.Evaluator` impls routed through 3
event classes — `.gate.failed` / `.gate.recovered` (the CI/test gate-result
class), `.policy.violation` (the hygiene rows §11.4.9/.14), and
`.repo.safety.breach` (the shared §11.4.30 build-artifact row with cherald).

3 bespoke pure detectors are the load-bearing vertical slice:

1. **Gate-result classification** (§1 / §11.4.50) — reads a recorded CI gate
   outcome (pass/fail/flaky/error) → verdict. A `flaky` gate is a §11.4.50
   determinism FAIL. A gate with no recognizable outcome refuses to silent-PASS.
2. **Test-tier-verify** (§11.4.27 / §40.2) — reads the present test tiers and
   FAILs when any of the 8 canonical tiers (unit/component/integration/contract/
   e2e_sandbox/e2e_live/mutation/chaos) is missing.
3. **Anti-bluff-PASS detection** (§11.4.2 / §11.4.5) — THE §107 covenant
   detector at the CI layer: a gate that reports `outcome=pass` but has NO
   captured-evidence artefact (`evidence=false`) is a §11.4 PASS-bluff → FAIL.

All detectors are PURE: they CLASSIFY a recorded outcome string; none runs the
build, re-executes the suite, spawns a process, or touches the filesystem
(§12 host-safety).

## Evidence in this directory

| File | What it proves |
|---|---|
| `gate_result_roundtrip_transcript.txt` | A REAL bidirectional round-trip: a failed CI gate (§1) + a PASS-bluff (§11.4.2) + a missing-tier matrix (§11.4.27) detected → emitted as `.gate.failed` on the bus → persisted as state + audit rows → queryable; then a fail→pass recovery emitting `.gate.recovered`. Captures the wire-event IDs observed on the bus (4 published: 3 `.gate.failed` + 1 `.gate.recovered`). NOT a metadata-only assertion. |
| `stress_chaos/concurrent_gate_failures.log` | §11.4.85 stress: 8 workers × 150 = 1200 concurrent distinct failing gate-results through ONE Pipeline → 0 errors, 1200 `.gate.failed` emits, 1200 audit rows (no lost events under contention). Run under `-race`. |
| `stress_chaos/emit_fault_fail_loud.log` | §11.4.85 chaos: emit-fault injection (GateFailed→err) × 800 → 100% of emit faults surface via EvaluateSubject error, 0 swallowed (no §107 distribution-layer bluff). |
| `stress_chaos/latency.json` / `latency_histogram.csv` | captured latency percentiles for the stress scenario. |
| `stress_chaos/summary.md` | the §11.4.85 scenario summary + host-memory headroom (§12.6). |

## Reproduce

```bash
# unit + bespoke-detector + round-trip tests (deterministic, -count=3)
go test -race -count=3 ./bherald/internal/bindings/...

# stress + chaos with persistent artefacts
HERALD_STRESS_QA_DIR=docs/qa/<run-id>/stress_chaos HERALD_STRESS_RUN_ID=run \
  go test -race -count=1 -run 'TestBindings_(Stress|Chaos)' ./bherald/internal/bindings/...
```

## §11.4.92 5-pass

- **Pass 1 (main-task captured-evidence):** TDD RED (build-fail, no impl) → GREEN;
  `go test -race -count=3` deterministic PASS ×3; this transcript + stress/chaos
  artefacts are the captured evidence (no "should work").
- **Pass 2 (regression blast-radius):** new package `bherald/internal/bindings`
  only; `bherald/go.mod` gains `commons_constitution` + `google/uuid` +
  `digital.vasic.database` replace; `bherald/cmd/bherald/main.go` + `internal/stubs`
  untouched. `commons_constitution/` read-only.
- **Pass 3 (cross-feature interaction):** mirrors the HRD-019/020 emit-class
  routing; no shared state with cherald/sherald (each flavor owns a separate
  Pipeline). Composes with §107.y quiescence (no mutation markers) + §11.4.89
  (stress test >30s would background, but this run is <2s).
- **Pass 4 (deep-research):** no external solution — the bindings are bespoke
  Helix-governance Evaluator impls; the emit/persist/audit path is 100% Batch-A
  wiring (§11.4.74 catalogue-first: reuse `commons_constitution`).
- **Pass 5 (anti-bluff):** real MemoryBus + real Memory store/ladder/audit +
  real emitter; every PASS carries a persisted state row + audit row + a wire
  event — NOT metadata-only / config-only / grep-only.

## Proposed e2e invariant

**E92** (conductor to wire into `scripts/e2e_bluff_hunt.sh`): asserts a bherald
gate-result FAIL Subject (`§1`, `test-suite|outcome=fail`) → `.gate.failed`
detected + persisted + audited via the bindings Pipeline, AND a §11.4.2
PASS-without-evidence Subject → FAIL (the anti-bluff-PASS detector). Anchor:
`bherald/internal/bindings` + this evidence directory.
