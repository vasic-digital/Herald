-- 000009_background_tasks.up.sql — HRD-010 Task 5 (E15 evidence).
--
-- Tables that back the upstream digital.vasic.background TaskQueue +
-- TaskRepository contract (see submodules/Models/background_task.go for
-- the canonical struct, submodules/background/interfaces.go for the
-- repository interface).
--
-- The columns map 1:1 to the BackgroundTask struct's `db:` tags so
-- Herald's pgxTaskRepository (commons_infra/task_repository.go) can
-- Scan rows directly into *models.BackgroundTask without translation.
--
-- This is queue/scheduling infrastructure — it deliberately does NOT
-- carry tenant_id + RLS. Queue tasks are dispatcher-internal: they
-- carry tenant context inside the JSON payload, not as a row-level
-- column. The tenant boundary is enforced one level up by Herald
-- service code that constructs+enqueues the task, not by the queue
-- table itself.

CREATE TABLE IF NOT EXISTS background_tasks (
    id                          TEXT PRIMARY KEY,
    task_type                   TEXT NOT NULL,
    task_name                   TEXT NOT NULL,
    correlation_id              TEXT,
    parent_task_id              TEXT,

    -- Configuration
    payload                     JSONB NOT NULL DEFAULT '{}'::jsonb,
    config                      JSONB NOT NULL DEFAULT '{}'::jsonb,
    priority                    TEXT NOT NULL DEFAULT 'normal',

    -- State
    status                      TEXT NOT NULL DEFAULT 'pending',
    progress                    DOUBLE PRECISION NOT NULL DEFAULT 0,
    progress_message            TEXT,
    checkpoint                  JSONB,

    -- Retry
    max_retries                 INTEGER NOT NULL DEFAULT 3,
    retry_count                 INTEGER NOT NULL DEFAULT 0,
    retry_delay_seconds         INTEGER NOT NULL DEFAULT 60,
    last_error                  TEXT,
    error_history               JSONB NOT NULL DEFAULT '[]'::jsonb,

    -- Execution
    worker_id                   TEXT,
    process_pid                 INTEGER,
    started_at                  TIMESTAMPTZ,
    completed_at                TIMESTAMPTZ,
    last_heartbeat              TIMESTAMPTZ,
    deadline                    TIMESTAMPTZ,

    -- Resources
    required_cpu_cores          INTEGER NOT NULL DEFAULT 1,
    required_memory_mb          INTEGER NOT NULL DEFAULT 512,
    estimated_duration_seconds  INTEGER,
    actual_duration_seconds     INTEGER,

    -- Notifications
    notification_config         JSONB NOT NULL DEFAULT '{}'::jsonb,

    -- User association
    user_id                     TEXT,
    session_id                  TEXT,
    tags                        JSONB NOT NULL DEFAULT '[]'::jsonb,
    metadata                    JSONB NOT NULL DEFAULT '{}'::jsonb,

    -- Timestamps
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    scheduled_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at                  TIMESTAMPTZ
);

-- Pending-task dispatch index. The Dequeue path filters by status =
-- 'pending' + scheduled_at <= now and orders by priority + created_at,
-- so this partial index matches the hot path.
CREATE INDEX IF NOT EXISTS background_tasks_pending_idx
    ON background_tasks (priority, created_at)
    WHERE status = 'pending' AND deleted_at IS NULL;

-- Worker-claim lookup (for stale-task scans, worker recovery).
CREATE INDEX IF NOT EXISTS background_tasks_worker_idx
    ON background_tasks (worker_id)
    WHERE worker_id IS NOT NULL;

-- Execution-history events for tasks (LogEvent target).
CREATE TABLE IF NOT EXISTS background_task_events (
    id          UUID PRIMARY KEY DEFAULT uuidv7(),
    task_id     TEXT NOT NULL REFERENCES background_tasks(id) ON DELETE CASCADE,
    event_type  TEXT NOT NULL,
    event_data  JSONB NOT NULL DEFAULT '{}'::jsonb,
    worker_id   TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS background_task_events_task_idx
    ON background_task_events (task_id, created_at);
