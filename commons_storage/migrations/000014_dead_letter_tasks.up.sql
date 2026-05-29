-- 000014_dead_letter_tasks.up.sql — HRD-090 (failure-terminal evidence).
--
-- Dead-letter snapshot table backing the upstream
-- digital.vasic.background.TaskRepository.MoveToDeadLetter contract
-- (submodules/background/interfaces.go) and mapping 1:1 to the
-- models.DeadLetterTask struct's `db:` tags
-- (submodules/Models/background_task.go).
--
-- A row is written when a task exhausts retries or fails a §107 invariant.
-- task_data is a full JSONB snapshot of the BackgroundTask at move-time so
-- the original row can be reconstructed for reprocessing. The FK to
-- background_tasks mirrors the background_task_events / task_resource_snapshots
-- pattern (ON DELETE CASCADE) so removing a task cleans up its dead-letter row.
--
-- Like background_tasks this is dispatcher-internal queue infrastructure —
-- it deliberately carries NO tenant_id + RLS (tenant context lives in the
-- snapshotted JSON payload, enforced one level up by Herald service code).

CREATE TABLE IF NOT EXISTS dead_letter_tasks (
    id               TEXT PRIMARY KEY,
    original_task_id TEXT NOT NULL REFERENCES background_tasks(id) ON DELETE CASCADE,
    task_data        JSONB NOT NULL DEFAULT '{}'::jsonb,
    failure_reason   TEXT NOT NULL DEFAULT '',
    failure_count    INTEGER NOT NULL DEFAULT 0,
    moved_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    reprocess_after  TIMESTAMPTZ,
    reprocessed      BOOLEAN NOT NULL DEFAULT false
);

-- Reprocess-scan path: not-yet-reprocessed dead letters, oldest-first.
CREATE INDEX IF NOT EXISTS dead_letter_tasks_reprocess_idx
    ON dead_letter_tasks (moved_at)
    WHERE reprocessed = false;

-- Lookup-by-original-task path (audit / dedup).
CREATE INDEX IF NOT EXISTS dead_letter_tasks_original_idx
    ON dead_letter_tasks (original_task_id);
