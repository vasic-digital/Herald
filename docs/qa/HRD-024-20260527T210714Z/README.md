# HRD-024 — iherald incident/escalation constitution bindings (§107.x QA evidence)

| Field | Value |
|---|---|
| HRD | HRD-024 |
| Run ID | HRD-024-20260527T210714Z |
| Feature | iherald constitution-rule escalation bindings (spec V3 §42.3 iherald rows + §42.5 step 7) |
| Flavor | `iherald` (Incident Herald) |
| Package | `iherald/internal/bindings/` |
| Constitutional anchor | Herald §107.x (docs/qa evidence mandate) / Helix §11.4.85 (stress+chaos) / §11.4.2 + §11.4.5 (captured-evidence) |
| NO-REAL-SECRET attestation | Every credential-leak Subject is a FABRICATED `fake_leak_<w>_<i>` / `config/fake-*` location plus a boolean detection flag. NO real `.env` scanned, NO real secret string in any test or artefact. |

## What this proves

iherald is the INCIDENT/escalation flavor. HRD-024 wires its §42.3-owned escalation
rules into the Batch-A `commons_constitution` Evaluator + Registry + ModeLadder +
ConstitutionStore + AuditStore foundation:

| Rule | Title | Severity | Mode | Event class | Detector |
|---|---|---|---|---|---|
| §11.4.10 | Credentials-handling (page-out) | critical | enforce | `.credential.leak` | `checkCredentialLeak` |
| §11.4.10.A | Pre-store leak audit | critical | enforce | `.credential.leak` | `checkPreStoreAudit` |
| §11.4.21 | Operator-blocked escalation | high | enforce | `.policy.violation` | `checkOperatorBlocked` |
| §11.4.66 | Blocker-resolution clarification | high | enforce | `.policy.violation` | `checkBlockerClarification` |
| §18.8 | Incident-severity routing (bespoke) | high | enforce | `.policy.violation` | `checkIncidentSeverity` |

Each detector is PURE: it CLASSIFIES a recorded incident signal (a credential-leak
detection outcome, an operator-blocked status transition, an incident-severity
routing decision). It never scans a real `.env`, reads a real secret, pages a real
on-call, runs git, or touches the filesystem. Live paging integration
(`/v1/webhooks/page` handler body + the §43 escalation command bodies) is
scope-locked to the HRD-024-paging follow-ups.

## Round-trip evidence (the §107 anti-bluff bar)

`escalation_roundtrip_verbose.log` — full `go test -race -v` transcript. The
load-bearing round-trips:

- **Credential-leak page-out** (`TestPipeline_CredentialLeakRoundTrip`): a detected
  leak (FAKE location + `leaked=true`) → `.credential.leak` event reaches the bus
  (page-out fan-out fired) → `constitution_state` row persisted (decision=fail,
  subject) → `constitution_audit` row written under enforce mode.
- **Operator-blocked escalation** (`TestPipeline_OperatorBlockedRoundTrip`): an item
  entering operator-blocked WITHOUT the on-call page → `.policy.violation` event
  with a captured non-Nil emitted-event-id audit row.
- **Leak-remediation recovery** (`TestPipeline_CredentialLeakRecoveredOnPass`): a
  fail→pass transition emits `.gate.recovered` (shared companion class), NOT another
  page-out (no on-call spam).
- **Ladder gating** (`TestPipeline_LadderGatesEmit`): allow-mode records state but
  does NOT page out / audit.
- All 5 bespoke detectors have per-decision case tables (PASS / FAIL / refuse-to-
  silent-PASS).

15/15 tests PASS, deterministic under `go test -race -count=3`.

## Stress + chaos evidence (§11.4.85) — `stress_chaos/`

- `concurrent_credential_leaks.log` + `latency.json` + `latency_histogram.csv`:
  N=8 workers × M=150 = **1200** distinct FAKE credential-leak page-outs through ONE
  shared Pipeline. 0 errors, all 1200 FAIL→page out, bus published exactly 1200
  `.credential.leak` events, 1200 audit rows — **no lost page-outs under load**.
  p50=0.161ms p95=0.516ms p99=0.786ms, ~38k/s, race-detector clean.
- `page_out_fault_fail_loud.log`: emit-fault injection (`faultEmitter` makes
  `CredentialLeak` always error). 800 page-outs, **800 surfaced the fault, 0
  swallowed** — a credential leak that "paged" but never reached the bus is the worst
  distribution-layer §107 bluff; this makes it impossible.
- `summary.md`: scenario table + host-memory headroom (§12/§12.6) + no-real-secret
  attestation.

## Reproduce

```bash
# hermetic, bounded, no real services / secrets:
go test -race -count=3 ./iherald/internal/bindings/...

# with persisted stress surface:
HERALD_STRESS_QA_DIR="$PWD/docs/qa/<run-id>/stress_chaos" \
HERALD_STRESS_RUN_ID="<run-id>" \
  go test -race -count=1 -v ./iherald/internal/bindings/...
```

## e2e_bluff_hunt anchor (proposed, NOT yet wired)

Next-free invariant **E95** — assert `iherald/internal/bindings` drives a real
credential-leak signal through the emit→state→audit path, anchored on this
`docs/qa/HRD-024-<run-id>/` directory as its §11.4.2/§11.4.5 positive-evidence
artefact. (Conductor wires `scripts/e2e_bluff_hunt.sh`; this subagent did NOT edit it.)
