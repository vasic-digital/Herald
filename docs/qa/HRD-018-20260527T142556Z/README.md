<div align="center">

<img src="../../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# HRD-018 / HRD-026 / HRD-027 — commons_constitution persistence: §107.x QA evidence

Run ID: `HRD-018-20260527T142556Z` · Captured 2026-05-27 (UTC) · against real Postgres `herald-postgres:24100`.

This directory is the §107.x docs/qa evidence for the v1.0.0 Batch-A closure of:

- **HRD-018** — commons_constitution package: Evaluator + 12 emit helpers + state/bindings migrations + bundle-hash + mode-ladder. The Evaluator framework, emit helpers, bundle-hash, mode-ladder, and the `constitution_state` / `constitution_bindings` migrations were already coded in r1. The remaining gap closed here is the **`constitution_audit` write-through**: the table existed in migration 000006 but the Runner never wrote a row — `RunOutcome.Audited` was set `true` with nothing persisted (a §107 PASS-bluff at the audit layer). The Runner now writes a durable audit row for every CHANGED transition in ModeWarn/ModeEnforce.
- **HRD-026** — bundle-hash captureer: the SHA-256 bundle hash is now persisted on every emitted event AND every audit row (see `bundle_hash_hex` column in `03_*.txt`), enabling replay correlation per §42.1.3.
- **HRD-027** — mode-ladder runtime config: `constitution_bindings` is read at evaluation time by the ladder, and three admin REST endpoints (`GET`/`PUT /v1/compliance/modes[/:rule_id]`) flip allow/warn/enforce per binding per tenant without redeploy.

## Artefacts

| File | What it proves |
|---|---|
| `01_emit_persist_realpg.txt` | Real-PG emit→persist round-trip: `TestPostgresAudit_RecordAndList` (audit row durable + RLS-scoped + emitted_event_id round-trips for enforce / NULL for warn) and `TestPostgresRunner_EndToEndAuditPersist` (Runner.Run → constitution_state UPSERT + constitution_audit INSERT, bundle hash carried for replay). Both PASS. |
| `02_admin_rest_flip.txt` | Admin-REST flip round-trip: `TestModes_FlipReflectsImmediately` (PUT warn → GET reflects warn → ladder.Get on the eval hot path reflects warn → no redeploy), `TestModes_List`, and the §107 negative paths (bad mode → 400, no auth → 401, bad tenant → 401). All PASS. |
| `03_schema_migrations_applied.txt` | DB-level ground truth: 12 migrations applied (incl. 000006 constitution_state+audit, 000007 constitution_bindings, 000008 force_rls); `\d constitution_audit` showing the append-only RLS policies (no-update / no-delete) + the bundle_hash 32-byte CHECK; and a live `SELECT` showing real rows — enforce rows carry `emitted_event_id` (has_event_id=t), warn rows have NULL (f), `bundle_hash_hex` is a 64-hex SHA-256 on every row. |
| `stress_chaos/emit_persist/` | §11.4.85 stress: 512 concurrent emit→persist round-trips (32 workers × 16 iters), `-race` clean, **exactly 512 audit rows — no lost / double writes**. `latency.json` + `latency_histogram.csv` + `assertion.txt`. |
| `stress_chaos/ladder_flips/` | §11.4.85 chaos: 960 concurrent flips of the same binding (24 workers × 40 iters), `-race` clean, final mode is one of the written modes — no torn state. `assertion.txt`. |

## Reproduce

```bash
export DOCKER_HOST="unix:///var/folders/t3/dmp1fb1d61xbl27trnjtr0_c0000gn/T/podman/podman-machine-default-api.sock"
export HERALD_DB_PASSWORD=herald_dev
# emit→persist + admin-flip + stress/chaos (unit + memory):
go test -race -count=1 ./commons_constitution/... ./cherald/internal/modes/...
# real-PG durability (requires herald-postgres up at :24100):
go test -tags integration -race -count=1 ./commons_constitution/...
```

§107 anti-bluff posture: every PASS above carries positive runtime evidence — a real DB row, a real HTTP round-trip, or a load-test row count — not a metadata/absence-of-error PASS.
