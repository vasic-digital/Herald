-- 000014_dead_letter_tasks.down.sql — HRD-090 rollback.
--
-- Drops the dead-letter snapshot table. The indexes are dropped implicitly
-- with the table; the explicit DROP INDEX is defensive for partial-apply
-- states.

DROP INDEX IF EXISTS dead_letter_tasks_original_idx;
DROP INDEX IF EXISTS dead_letter_tasks_reprocess_idx;
DROP TABLE IF EXISTS dead_letter_tasks;
