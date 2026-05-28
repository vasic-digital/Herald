# QA evidence — v1.0.0 Batch D (HRD-085..089)

Run-id: `HRD-085-089-20260528T070000Z`
Feature: the 17 previously-`ErrUnsupported` methods of Herald's
`pgxTaskRepository` (`commons_infra/task_repository.go`), implemented against
real Postgres, plus migration `000013_task_resource_snapshots`.

Authority: §107 / §11.4.83 docs/qa evidence mandate; §11.4.68 positive
sink-side evidence; CONST-050(A) no-fakes-beyond-unit-tests.

## What landed

| HRD | Methods | Migration |
|-----|---------|-----------|
| HRD-085 | `GetByID`, `Update`, `Delete` (soft) | — |
| HRD-086 | `UpdateStatus`, `UpdateProgress`, `UpdateHeartbeat`, `SaveCheckpoint` | — (checkpoint column already in 000009) |
| HRD-087 | `GetByStatus`, `GetPendingTasks`, `CountByStatus`, `GetTaskHistory` | — |
| HRD-088 | `GetStaleTasks`, `GetByWorkerID` | — |
| HRD-089 | `SaveResourceSnapshot`, `GetResourceSnapshots` | **000013_task_resource_snapshots** |

`MoveToDeadLetter` remains `ErrUnsupported` by design (HRD-090, out of Batch D
scope). `Create` / `Dequeue` / `LogEvent` were already implemented.

## Evidence in this directory

- `build_vet.log` — `go build` + `go vet` + `go vet -tags=integration` (all exit 0).
- `unit_tests.log` — full `-v` run of the hermetic unit suite (recording-fake
  `db.Database`; asserts SQL shape + arg construction + validation guards). All PASS.
- `migration_test.log` — `TestMigrationsBundle` confirming 000013 is registered
  (13 migrations × {up,down} = 26 files). PASS.

## Layer disclosure (anti-bluff, §107)

The unit tests are the CHEAP layer. They prove we send the right SQL with the
right args and reject bad input — they do NOT prove the SQL round-trips through
Postgres (they use a recording fake, permitted only in unit tests per
CONST-050(A)). The LOAD-BEARING proof is the live-Postgres integration suite in
`commons_infra/task_repository_integration_test.go` (`//go:build integration`),
which for every method Creates a real row, exercises the method, and reads the
DB state back via an INDEPENDENT raw SELECT with EXACT assertions
(exact-count, exact-id, exact-field) — not `>= 1`, not `non-nil`.

## PG-PENDING

At capture time, Postgres on host port 24100 (spec §9.4) was **NOT reachable**
(`nc -z 127.0.0.1 24100` → unreachable). The integration suite is therefore
WRITTEN + COMPILE-VERIFIED (`go vet -tags=integration` exit 0) but NOT YET RUN
against live PG. The conductor MUST run it once Postgres is up:

```bash
# from repo root, with a podman/docker runtime available:
go test -tags=integration -timeout 5m -count=1 -run 'TestRepo' ./commons_infra/...
```

(The test boots its own Postgres container via QuickstartBoot and sets throwaway
`HERALD_DB_PASSWORD` etc. itself, mirroring the existing
`clients_integration_test.go` pattern. Migration 000013 is applied automatically
by `boot.Up()` → `storage.RunMigrations`.)

Until that run completes with captured output, the Batch D PASS line is
PG-PENDING, not §107-proven. Append the live-run log to this directory and
mark each HRD's integration evidence as captured.

## task_resource_snapshots DDL (migration 000013)

```sql
CREATE TABLE IF NOT EXISTS task_resource_snapshots (
    id              TEXT PRIMARY KEY,
    task_id         TEXT NOT NULL REFERENCES background_tasks(id) ON DELETE CASCADE,
    cpu_percent     DOUBLE PRECISION NOT NULL DEFAULT 0,
    cpu_user_time   DOUBLE PRECISION NOT NULL DEFAULT 0,
    cpu_system_time DOUBLE PRECISION NOT NULL DEFAULT 0,
    memory_rss_bytes BIGINT NOT NULL DEFAULT 0,
    memory_vms_bytes BIGINT NOT NULL DEFAULT 0,
    memory_percent   DOUBLE PRECISION NOT NULL DEFAULT 0,
    io_read_bytes   BIGINT NOT NULL DEFAULT 0,
    io_write_bytes  BIGINT NOT NULL DEFAULT 0,
    io_read_count   BIGINT NOT NULL DEFAULT 0,
    io_write_count  BIGINT NOT NULL DEFAULT 0,
    net_bytes_sent  BIGINT NOT NULL DEFAULT 0,
    net_bytes_recv  BIGINT NOT NULL DEFAULT 0,
    net_connections INTEGER NOT NULL DEFAULT 0,
    open_files      INTEGER NOT NULL DEFAULT 0,
    open_fds        INTEGER NOT NULL DEFAULT 0,
    process_state   TEXT NOT NULL DEFAULT '',
    thread_count    INTEGER NOT NULL DEFAULT 0,
    sampled_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS task_resource_snapshots_task_idx
    ON task_resource_snapshots (task_id, sampled_at DESC);
```

Columns map 1:1 to `models.ResourceSnapshot` `db:` tags
(`submodules/Models/background_task.go`). FK ON DELETE CASCADE mirrors
`background_task_events`. No tenant_id/RLS (dispatcher-internal queue infra,
same as `background_tasks`).
