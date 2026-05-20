-- 000004_channel_addresses.up.sql — spec V3 §6.
CREATE TABLE IF NOT EXISTS channel_addresses (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),
    tenant_id       UUID NOT NULL,
    channel         TEXT NOT NULL,
    address_url     TEXT NOT NULL,
    tags            TEXT[] NOT NULL DEFAULT '{}',
    priority_floor  INTEGER NOT NULL DEFAULT 1,
    enabled         BOOLEAN NOT NULL DEFAULT true,
    last_health_at  TIMESTAMPTZ,
    last_health_ok  BOOLEAN,
    last_health_err TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, address_url)
);
CREATE INDEX IF NOT EXISTS ch_addr_tags_idx ON channel_addresses USING gin (tags) WHERE enabled = true;
CREATE INDEX IF NOT EXISTS ch_addr_tenant_idx ON channel_addresses (tenant_id, channel) WHERE enabled = true;

ALTER TABLE channel_addresses ENABLE ROW LEVEL SECURITY;
ALTER TABLE channel_addresses FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS ca_isolation ON channel_addresses;
CREATE POLICY ca_isolation ON channel_addresses
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
