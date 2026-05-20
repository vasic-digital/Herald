-- migration 000007: rollback constitution_bindings.
BEGIN;
DROP TABLE IF EXISTS constitution_bindings CASCADE;
COMMIT;
