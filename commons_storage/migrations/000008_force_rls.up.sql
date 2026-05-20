-- 000008_force_rls.up.sql
--
-- ENABLE ROW LEVEL SECURITY makes RLS policies apply to non-owner roles.
-- Table OWNERS bypass RLS by default unless `FORCE ROW LEVEL SECURITY` is
-- also set. Foundation integration tests connect as `herald` (the DB
-- owner per the quickstart compose) — without FORCE the tenant_isolation
-- policy is silently bypassed.
--
-- Discovered 2026-05-20 by TestPostgresStore_RLSTenantIsolation in
-- commons_constitution/postgres_integration_test.go. The test
-- caught a real anti-bluff gap: tenant B could see tenant A's
-- constitution_state row. Per §44.6 + §16 this is a release blocker
-- that mechanical enforcement must prevent.
--
-- This migration adds FORCE to every RLS-guarded table created so far
-- so RLS applies uniformly regardless of connecting role.

BEGIN;

ALTER TABLE constitution_state    FORCE ROW LEVEL SECURITY;
ALTER TABLE constitution_audit    FORCE ROW LEVEL SECURITY;
ALTER TABLE constitution_bindings FORCE ROW LEVEL SECURITY;

-- Also force RLS on earlier multi-tenant tables (idempotent — no-op if
-- the table either doesn't exist or already has FORCE set).
DO $$
DECLARE
    t TEXT;
BEGIN
    FOREACH t IN ARRAY ARRAY[
        'idempotency_keys',
        'subscribers',
        'subscriber_aliases',
        'agent_tokens',
        'channel_addresses',
        'webhook_sources',
        'inbound_messages',
        'thread_refs',
        'quarantined_messages',
        'dead_letters',
        'workable_items',
        'outbound_dedup',
        'email_suppressions',
        'report_publications'
    ] LOOP
        IF EXISTS (SELECT 1 FROM pg_class WHERE relname = t AND relrowsecurity) THEN
            EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
        END IF;
    END LOOP;
END
$$;

-- Grant the runtime app role the necessary CRUD permissions on the
-- RLS-guarded tables. Without these grants, SET LOCAL ROLE herald_app
-- would have no access at all (the tables are owned by `herald` per
-- the quickstart compose's POSTGRES_USER).
GRANT SELECT, INSERT, UPDATE, DELETE ON constitution_state    TO herald_app;
GRANT SELECT, INSERT                  ON constitution_audit    TO herald_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON constitution_bindings TO herald_app;
GRANT SELECT, INSERT, UPDATE         ON tenants                TO herald_app;
-- Tenants table is operator-managed; herald_app only reads + can refer to it.

-- Grant USAGE on the public schema (needed for table access).
GRANT USAGE ON SCHEMA public TO herald_app;

-- Idempotent grants on the early multi-tenant tables (if they exist).
DO $$
DECLARE
    t TEXT;
BEGIN
    FOREACH t IN ARRAY ARRAY[
        'idempotency_keys', 'subscribers', 'subscriber_aliases',
        'agent_tokens', 'channel_addresses', 'webhook_sources',
        'inbound_messages', 'thread_refs', 'quarantined_messages',
        'dead_letters', 'workable_items', 'outbound_dedup',
        'email_suppressions', 'report_publications'
    ] LOOP
        IF EXISTS (SELECT 1 FROM pg_class WHERE relname = t) THEN
            EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON %I TO herald_app', t);
        END IF;
    END LOOP;
END
$$;

COMMIT;
