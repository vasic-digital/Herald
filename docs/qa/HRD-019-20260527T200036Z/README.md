# HRD-019 — cherald constitution bindings (§107.x evidence)

| Field | Value |
|---|---|
| HRD | HRD-019 (v1.0.0 Batch B, unit 1) |
| Captured | 2026-05-27 (UTC run-id `HRD-019-20260527T200036Z`) |
| Feature | cherald wires the 32 cherald-owned §42.3 constitution rules into the Batch-A `commons_constitution` Evaluator + emit + persist + audit foundation, plus the `POST /v1/compliance/evaluate` write surface. |
| Anti-bluff anchor | Helix §11.4.85 (stress+chaos) / Herald §107 + §107.x |
| Mode | Hermetic — real `bindings.Pipeline` over real `MemoryBus` + real Memory `ConstitutionStore`/`ModeLadder`/`AuditStore`. Only the JWT-verify boundary is faked (`fakeAuth`, the same §11.4.27 seam the existing handler tests use). |

## What this proves

The §107 bar for HRD-019 is not "the bindings registered" — it is "a constitution rule
violation can actually be detected, emitted, persisted, audited, and queried by an end
user end-to-end." The artefacts here are real captured runtime output of that loop.

## Artefacts

### `rest/binding_roundtrip_transcript.md` — bidirectional REST round-trip

Real `httptest` server + the production `compliance.EvaluateHandler` + `compliance.Handler`:

1. `POST /v1/compliance/evaluate` with a §11.4.29 naming violation (`commons_messaging/BadName.go`)
   → `200` `{decision: deny, emitted: true, audited: true, changed: true}`.
2. `GET /v1/compliance?rule_id=§11.4.29` → `200` showing the persisted `constitution_state`
   row with the evidence URI naming the violation — i.e. the violation is now queryable.
3. `POST` a compliant subject (`good_name.go`) → `200` `{decision: pass}` — no false positive.
4. Event-bus metrics confirm the emit really fanned out (`policy.violation:1`, `policy.cleared:1`).

### `stress_chaos/bindings/` — §11.4.85 stress + chaos

| Scenario | Verdict | Key numbers |
|---|---|---|
| `concurrent_violations.log` (stress) | PASS | N=8 workers × M=150 = 1200 distinct §11.4.29 violations through ONE Pipeline; 0 errors; bus published exactly 1200 `.policy.violation` events AND 1200 audit rows (no lost events under contention); p50≈0.12ms p95≈0.43ms p99≈0.70ms; `-race` clean. |
| `emit_fault_fail_loud.log` (chaos) | PASS | bus-drop injected via `faultEmitter`; 800/800 enforce-mode violations surfaced the emit error from `EvaluateSubject` — 0 swallowed (no distribution-layer §107 bluff). |
| `latency.json`, `latency_histogram.csv`, `summary.md` | — | machine-readable latency + per-iteration histogram + human summary (incl. §12.6 host-mem headroom probe). |

Regenerate: `HERALD_STRESS_QA_DIR=$PWD/docs/qa HERALD_STRESS_RUN_ID=<run-id> go test -race -count=1 -run 'TestBindings_(Stress|Chaos)' ./cherald/internal/bindings/...`

## Proposed e2e invariant (conductor wires into `scripts/e2e_bluff_hunt.sh`)

**E90 — cherald constitution-binding detect→emit→persist→query round-trip.** Build cherald,
start the real Gin server, `POST /v1/compliance/evaluate` a §11.4.29 naming violation,
assert `200` with `decision=deny` + `emitted=true`, then `GET /v1/compliance?rule_id=§11.4.29`
and assert the persisted row is present with the violation evidence URI. Positive-evidence
anchor: this directory's `rest/binding_roundtrip_transcript.md`. (E89 is consumed by Batch-A
HRD-018; E90 is the next free invariant.)
