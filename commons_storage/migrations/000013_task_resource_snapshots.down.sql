-- 000013_task_resource_snapshots.down.sql — HRD-089 rollback.
--
-- Drops the append-only resource-snapshot time series. The index is dropped
-- implicitly with the table; the explicit DROP INDEX is defensive for
-- partial-apply states.

DROP INDEX IF EXISTS task_resource_snapshots_task_idx;
DROP TABLE IF EXISTS task_resource_snapshots;
