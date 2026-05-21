-- 000011_claude_code_sessions.up.sql
-- Per spec V3 §33.2: Herald persists the Claude Code session anchor file
-- contents (UUID + path) PLUS the most-recent dispatch metadata, so a
-- restart can re-bind to the same Claude session without losing context.
--
-- Scope: NOT a customer-tenant-scoped table — this is Herald operator-shared
-- state. Uses the fixed HeraldSystemTenant UUID (00000000-0000-0000-0000-
-- 000000000001) as the tenant_id so the existing RLS infrastructure
-- (app.tenant_id GUC + FORCE RLS + herald_app role grants) still applies
-- uniformly. Per §16 + §44.6, the operator-shared bucket is just another
-- tenant from the RLS perspective; only the application-level interpretation
-- of who owns the row differs.
--
-- §107 anti-bluff guard: the integration test in
-- commons_messaging/dispatch/claude_code/persist_integration_test.go
-- asserts exact equality between the persisted session_uuid + anchor_path
-- and the DispatchResponse returned by Dispatch (which itself reads from
-- the live `claude --resume` output). A Herald-synthetic UUID stored here
-- would silently bind a wrong/dead session on restart — the equality
-- check is what closes that bluff.
CREATE TABLE IF NOT EXISTS claude_code_sessions (
    tenant_id        UUID NOT NULL,
    project_name     TEXT NOT NULL,
    session_uuid     UUID NOT NULL,
    anchor_path      TEXT NOT NULL,
    last_dispatch_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_response    JSONB NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (tenant_id, project_name)
);

CREATE INDEX IF NOT EXISTS claude_code_sessions_last_dispatch_idx
    ON claude_code_sessions (last_dispatch_at DESC);

-- RLS — same FORCE + herald_app grant pattern as 000008 / 000010.
-- §44.6: FORCE makes RLS apply even to the owning role; without it the
-- bootstrap POSTGRES_USER (often SUPERUSER) bypasses tenant isolation.
ALTER TABLE claude_code_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE claude_code_sessions FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS claude_code_sessions_tenant_isolation ON claude_code_sessions;
CREATE POLICY claude_code_sessions_tenant_isolation
    ON claude_code_sessions
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON claude_code_sessions TO herald_app;
