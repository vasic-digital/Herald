-- 000003_subscribers.up.sql — spec V3 §7.1 + §7.5.
CREATE TABLE IF NOT EXISTS subscribers (
    id           UUID PRIMARY KEY DEFAULT uuidv7(),
    tenant_id    UUID NOT NULL,
    handle       TEXT,
    display_name TEXT,
    locale       TEXT DEFAULT 'en-US',
    timezone     TEXT DEFAULT 'UTC',
    kind         TEXT NOT NULL DEFAULT 'human',  -- 'human'|'agent'|'service' (§7.5)
    roles        TEXT[] NOT NULL DEFAULT '{}',
    metadata     JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, handle)
);
CREATE INDEX IF NOT EXISTS subscribers_tenant_idx ON subscribers (tenant_id);
CREATE INDEX IF NOT EXISTS subscribers_kind_idx ON subscribers (tenant_id, kind);

ALTER TABLE subscribers ENABLE ROW LEVEL SECURITY;
ALTER TABLE subscribers FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS sub_isolation ON subscribers;
CREATE POLICY sub_isolation ON subscribers
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

CREATE TABLE IF NOT EXISTS subscriber_aliases (
    subscriber_id   UUID NOT NULL REFERENCES subscribers(id) ON DELETE CASCADE,
    channel         TEXT NOT NULL,
    channel_user_id TEXT NOT NULL,
    verified_at     TIMESTAMPTZ,
    last_seen_at    TIMESTAMPTZ,
    UNIQUE (channel, channel_user_id)
);
CREATE INDEX IF NOT EXISTS subscriber_aliases_sub_idx ON subscriber_aliases (subscriber_id);

-- §7.5 agent tokens
CREATE TABLE IF NOT EXISTS agent_tokens (
    id                 UUID PRIMARY KEY DEFAULT uuidv7(),
    tenant_id          UUID NOT NULL,
    subscriber_id      UUID NOT NULL REFERENCES subscribers(id) ON DELETE CASCADE,
    token_hash         BYTEA NOT NULL,
    name               TEXT NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at       TIMESTAMPTZ,
    expires_at         TIMESTAMPTZ,
    revoked_at         TIMESTAMPTZ,
    rate_limit_per_min INTEGER NOT NULL DEFAULT 60,
    UNIQUE (tenant_id, name)
);
ALTER TABLE agent_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_tokens FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS at_isolation ON agent_tokens;
CREATE POLICY at_isolation ON agent_tokens
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
