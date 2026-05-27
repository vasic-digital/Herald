<div align="center">

<img src="../../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# HRD-022 — rherald release constitution bindings — §107.x QA transcript

| Field | Value |
|---|---|
| Run ID | HRD-022-20260528T015202Z |
| Feature | rherald (Release Herald) §42.3 release-gate constitution bindings |
| HRD | HRD-022 (v1.0.0 Batch B, unit 4) |
| Captured | 2026-05-28 |
| Toolchain | go1.26.2 darwin/arm64, `go test -race -count=3` |
| Anchors | spec V3 §42.3 rherald rows (§4 / §5 / §11.4.38 / §11.4.40) + §42.5 step 5; Helix §11.4.85 (stress+chaos); Herald §107 / §107.x |
| Verdict | PASS — 14/14 unit+stress+chaos tests green 3× under -race; no real tag/push/git/install performed |

This transcript is the §107.x positive-runtime-evidence anchor for HRD-022. rherald
is the RELEASE flavor: its bindings wire the four §42.3-owned release-lifecycle
rules THROUGH the Batch-A `commons_constitution` Evaluator + Registry + ModeLadder
+ ConstitutionStore + AuditStore + typed event emitter. Every verdict below is the
captured output of the REAL pipeline driving a REAL `MemoryBus` + REAL Memory
store/ladder/audit — no mocks-of-mocks (§11.4.27).

**§12 host-safety attestation.** Every detector CLASSIFIES a recorded
release-op outcome string (a tag-mirror parity tally, a changelog conformance
flag, a retest outcome, an install-check result). NO real tag, push, force-push,
`git` invocation, asset download, or install was performed at any point. The
release-op interception is scope-locked to the §43 follow-ups (HRD-031
tag-mirror, HRD-032 changelog-generate, HRD-045 gate-retest).

## 1. Release-gate-blocked round-trip (§4 tag mirroring — enforce / high)

The load-bearing HRD-022 anti-bluff proof (`TestPipeline_ReleaseGateBlockedRoundTrip`).

**INBOUND (the §43 release command body records a tag-mirror state):**

```
Subject{Kind: "tag-mirror", ID: "v1.4.0|tag=present|mirrors=4|with_tag=3"}
rule = §4   tenant = <uuid>   ladder mode = enforce
```

Interpretation: the `v1.4.0` tag is present on the parent but only 3 of the 4
owned mirrors (GitHub / GitLab / GitFlic / GitVerse) carry it — a §4 parity miss.

**OUTBOUND (the pipeline's observable side-effects):**

```
RunOutcome.Decision   = fail
RunOutcome.Transition = {Changed:true, FirstSeen:true}
RunOutcome.Mode       = enforce
RunOutcome.Emitted    = true
RunOutcome.Audited    = true

(1) bus event   : digital.vasic.herald.constitution.release.gate.blocked   ×1  (ReleaseRef="v1.4.0", Reason="tag-mirror-block")
(2) state row   : constitution_state{rule=§4, subject="v1.4.0|tag=present|mirrors=4|with_tag=3", decision=fail}  (queryable via future /v1/release)
(3) audit row   : constitution_audit{rule=§4, mode_at_emission=enforce}
```

A green parity (`with_tag=4`) instead transitions the gate back to PASS and emits
`.gate.recovered` (the release class family has no distinct "recovered" leaf, so a
release recovery maps to the shared `.gate.recovered` companion).

## 2. Changelog policy violation (§5 — warn / middle — shared with cherald)

`TestPipeline_ChangelogPolicyViolation` + `TestDetector_ChangelogConformance`.

**INBOUND:** `Subject{Kind:"changelog", ID:"v1.4.0|conforming=false"}`, rule `§5`.

**OUTBOUND:** Decision=fail → `.policy.violation` event (NOT a release-gate class —
changelog is a warn-mode policy concern shared with cherald); the audit row
captures the exact emitted event ID via the `IDEmitter` path. A `conforming=true`
changelog with a fresh multi-format export PASSes; `export_stale=true` FAILs.

## 3. Pre-tag full-suite retest gate (§11.4.40 — enforce / CRITICAL)

`TestDetector_PreTagRetestGate`. The highest-severity release gate.

| INBOUND subject | OUTBOUND decision | rationale |
|---|---|---|
| `v1.4.0\|retest=green\|tiers=8` | PASS | full retest, all 8 §40.2 tiers |
| `v1.4.0\|retest=skipped` | FAIL | tag attempted WITHOUT the mandatory retest — release BLOCKED |
| `v1.4.0\|retest=red` | FAIL | retest ran but RED |
| `v1.4.0\|retest=green\|tiers=5` | FAIL | incomplete tier coverage (5/8) |
| `v1.4.0` (no retest field) | FAIL | refuse to silent-PASS (§11.4.1 inverse) |

## 4. Installable-asset evidence (§11.4.38 — enforce / high)

`TestDetector_InstallableAssetEvidence`. `installed=true` → PASS;
`installed=false` → FAIL; missing field → FAIL (no install evidence).

## 5. TDD RED→GREEN proof

Tests were authored FIRST. RED was demonstrated by injecting an `if true { return
pass(...) }` always-pass bluff into the §11.4.40 retest detector; the suite caught
it immediately:

```
--- FAIL: TestDetector_PreTagRetestGate
    bindings_test.go: retest-gate "v1.4.0|retest=skipped" classified as pass, want fail
    bindings_test.go: retest-gate "v1.4.0|retest=red" classified as pass, want fail
    bindings_test.go: retest-gate "v1.4.0|retest=green|tiers=5" classified as pass, want fail
    bindings_test.go: retest-gate "v1.4.0" classified as pass, want fail
```

The bluff was reverted; the suite returned to GREEN. Full verbose GREEN output:
`roundtrip_test_output.log` (this directory). The full deterministic
`-race -count=3` run is `stress_chaos/go_test_output.log`.

## 6. §11.4.85 stress + chaos evidence

See `stress_chaos/` (captured artefacts):

- `concurrent_release_blocks.log` + `latency.json` + `latency_histogram.csv` — N=8
  workers × M=150 = **1200** distinct §11.4.40 release-blocks through ONE shared
  pipeline: **0 errors, 1200 `.release.gate.blocked` emits, 1200 audit rows** (no
  lost events under contention), p50=0.153ms p95=0.463ms p99=0.796ms,
  tput≈43.7k/s, race detector clean.
- `emit_fault_fail_loud.log` — bus-drop chaos (faultEmitter making
  `ReleaseGateBlocked` error): **800/800 emit faults surface** via
  `EvaluateSubject` error, **0 swallowed** (no §107 distribution-layer fail-bluff).
- `summary.md` — scenario table + host-safety + anti-bluff posture.

## 7. e2e_bluff_hunt anchor (for the conductor)

Proposed next-free invariant **E93** — "rherald release bindings: a §11.4.40
tag-without-retest subject drives a real `.release.gate.blocked` emit + persisted
constitution_state + constitution_audit row through the rherald Pipeline."
Positive-evidence anchor: this `docs/qa/HRD-022-<run-id>/` directory +
`go test -race -count=3 ./rherald/internal/bindings/...`. (The subagent did NOT
edit `scripts/e2e_bluff_hunt.sh` — the conductor wires E93.)
