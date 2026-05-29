# HRD-090 — Dead-letter move + `.queue.dead_letter` event — QA evidence

| Field | Value |
|---|---|
| HRD | HRD-090 (`commons_infra.pgxTaskRepository.MoveToDeadLetter`) |
| Run ID | HRD-090-20260529T052228Z |
| Date | 2026-05-29 |
| Runtime | real Postgres (herald-postgres :24100, podman) booted on-demand via `QuickstartBoot`; migration `000014_dead_letter_tasks` applied automatically |
| Result | **PASS** (`TestRepoMoveToDeadLetter_RoundTripAndEmit`, 2.36s) |

## What this proves (§107 / §11.4.5 / §11.4.68 sink-side evidence)

`MoveToDeadLetter` was exercised against a REAL task row, then **three independent
downstream sinks** were verified — never config-only, never metadata-only:

1. **Source row terminal state** — raw `SELECT status, last_error, completed_at
   FROM background_tasks` confirms `status='dead_letter'`, `last_error` = the
   failure reason, and `completed_at` stamped.
2. **Dead-letter snapshot row** — raw `SELECT ... FROM dead_letter_tasks` confirms
   the new row carries `original_task_id`, `failure_reason`, `failure_count=3`,
   `reprocessed=false`, and a `task_data` JSONB snapshot that round-trips the
   original task id.
3. **Governance event delivery** — a REAL `MemoryBus` subscriber received the
   `digital.vasic.herald.constitution.queue.dead_letter` event with subject
   `task:<id>` — proving the emit path actually fans out, not merely that a
   counter incremented.

Plus the negative case: `MoveToDeadLetter("ghost-id-never-existed", …)` errors
loudly (no silent success-bluff).

## Full-automation note (§11.4.98)

The test is fully self-driving end-to-end: it boots its own Postgres container,
applies migrations, seeds the task, performs the move, and verifies all sinks
with zero manual action during execution. The one-time `podman machine start`
was runtime bootstrap OUTSIDE test execution (operator-authorized 2026-05-29),
which §11.4.98 explicitly permits.

## Reproduce

```bash
podman machine start            # one-time runtime bootstrap (outside the test)
go test -tags=integration -timeout 8m -count=1 \
  -run 'TestRepoMoveToDeadLetter' -v ./commons_infra/...
```

## Artefacts

- `integration_realpg.log` — verbose `go test` transcript of the PASS.

## Related

- Code: `commons_infra/task_repository.go` (`MoveToDeadLetter`), migration
  `commons_storage/migrations/000014_dead_letter_tasks.{up,down}.sql`,
  event class `commons_constitution/{doc.go,emit.go}` (`ClassQueueDeadLetter` /
  `QueueEvent` / `DeadLetter`).
- Unit evidence: `commons_infra/task_repository_test.go`
  (`TestMoveToDeadLetter_*`), `commons_constitution/queue_emit_test.go`.
- Follow-up: governance-plane wiring (a durable subscriber draining
  `.queue.dead_letter` into the `constitution_audit` sink) — boot.go documents
  the deliberate nil-emitter choice in the Foundation plane.
