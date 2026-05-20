DROP TABLE IF EXISTS tenants;
DROP FUNCTION IF EXISTS uuidv7();
-- Roles intentionally NOT dropped so multiple migrations can compose
-- safely. Operators MAY drop them manually:
--   DROP ROLE IF EXISTS herald_app;
--   DROP ROLE IF EXISTS herald_migrator;
