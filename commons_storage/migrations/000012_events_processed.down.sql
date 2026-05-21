BEGIN;
DROP POLICY IF EXISTS events_processed_tenant_isolation ON events_processed;
DROP TABLE IF EXISTS events_processed;
COMMIT;
