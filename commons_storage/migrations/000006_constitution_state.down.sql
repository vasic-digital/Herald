-- migration 000006: rollback constitution_state + constitution_audit.
BEGIN;

DROP TABLE IF EXISTS constitution_audit CASCADE;
DROP TABLE IF EXISTS constitution_state CASCADE;

COMMIT;
