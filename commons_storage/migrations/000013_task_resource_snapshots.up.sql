-- 000013_task_resource_snapshots.up.sql — HRD-089 (resource-monitor evidence).
--
-- Append-only resource-usage time series for background tasks. Backs the
-- upstream digital.vasic.background.TaskRepository.SaveResourceSnapshot +
-- GetResourceSnapshots contract (submodules/background/interfaces.go) and
-- maps 1:1 to the models.ResourceSnapshot struct's `db:` tags
-- (submodules/Models/background_task.go).
--
-- Each row is one point-in-time sample captured by a ResourceMonitor for a
-- running task. Writes are append-only (never UPDATEd); reads are
-- reverse-chronological per task_id. The FK to background_tasks mirrors the
-- background_task_events pattern (ON DELETE CASCADE) so removing a task
-- cleans up its samples.
--
-- Like background_tasks this is dispatcher-internal queue/monitoring
-- infrastructure — it deliberately carries NO tenant_id + RLS (tenant
-- context lives in the parent task's JSON payload, enforced one level up
-- by Herald service code).

CREATE TABLE IF NOT EXISTS task_resource_snapshots (
    id              TEXT PRIMARY KEY,
    task_id         TEXT NOT NULL REFERENCES background_tasks(id) ON DELETE CASCADE,

    -- CPU metrics
    cpu_percent     DOUBLE PRECISION NOT NULL DEFAULT 0,
    cpu_user_time   DOUBLE PRECISION NOT NULL DEFAULT 0,
    cpu_system_time DOUBLE PRECISION NOT NULL DEFAULT 0,

    -- Memory metrics
    memory_rss_bytes BIGINT NOT NULL DEFAULT 0,
    memory_vms_bytes BIGINT NOT NULL DEFAULT 0,
    memory_percent   DOUBLE PRECISION NOT NULL DEFAULT 0,

    -- I/O metrics
    io_read_bytes   BIGINT NOT NULL DEFAULT 0,
    io_write_bytes  BIGINT NOT NULL DEFAULT 0,
    io_read_count   BIGINT NOT NULL DEFAULT 0,
    io_write_count  BIGINT NOT NULL DEFAULT 0,

    -- Network metrics
    net_bytes_sent  BIGINT NOT NULL DEFAULT 0,
    net_bytes_recv  BIGINT NOT NULL DEFAULT 0,
    net_connections INTEGER NOT NULL DEFAULT 0,

    -- File descriptors
    open_files      INTEGER NOT NULL DEFAULT 0,
    open_fds        INTEGER NOT NULL DEFAULT 0,

    -- Process state
    process_state   TEXT NOT NULL DEFAULT '',
    thread_count    INTEGER NOT NULL DEFAULT 0,

    sampled_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Reverse-chronological-per-task read path: GetResourceSnapshots filters by
-- task_id and orders by sampled_at DESC, so this composite index matches.
CREATE INDEX IF NOT EXISTS task_resource_snapshots_task_idx
    ON task_resource_snapshots (task_id, sampled_at DESC);
