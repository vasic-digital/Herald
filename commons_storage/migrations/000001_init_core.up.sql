-- 000001_init_core.up.sql — bootstrap Herald's per-tenant core schema
-- per spec V3 §9.2 + §16.

-- Roles. Defined here so subsequent migrations can run as the migrator
-- with BYPASSRLS while the runtime app role obeys the policies.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'herald_migrator') THEN
        CREATE ROLE herald_migrator BYPASSRLS;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'herald_app') THEN
        CREATE ROLE herald_app NOBYPASSRLS;
    END IF;
END
$$;

-- UUIDv7 generator. PostgreSQL doesn't ship one yet (as of v16); use a
-- thin SQL wrapper around the bytea-construction recipe so we get
-- time-ordered primary keys for index locality.
CREATE OR REPLACE FUNCTION uuidv7() RETURNS uuid
LANGUAGE sql VOLATILE PARALLEL SAFE AS $$
    SELECT encode(
        set_bit(
            set_bit(
                overlay(uuid_send(gen_random_uuid())
                        placing substring(int8send((extract(epoch FROM clock_timestamp())*1000)::bigint) from 3)
                        from 1 for 6),
                52, 1),
            53, 1),
        'hex')::uuid;
$$;

-- Tenant table — single canonical source of tenant identity.
CREATE TABLE IF NOT EXISTS tenants (
    id          UUID PRIMARY KEY DEFAULT uuidv7(),
    name        TEXT NOT NULL UNIQUE,
    environment TEXT NOT NULL DEFAULT 'production',  -- 'production'|'staging'|'dev'|'quickstart'
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
