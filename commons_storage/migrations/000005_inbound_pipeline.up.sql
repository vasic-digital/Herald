-- 000005_inbound_pipeline.up.sql — spec V3 §32 + §15.5 + §5.4 + §12 + §8.3.
--
-- This single migration bundles the four inbound-pipeline tables so
-- §32 ingest can land atomically: webhook_sources (§5.5),
-- inbound_messages (§32.3), thread_refs (§12), quarantined_messages
-- (§15.2), dead_letters (§5.4), workable_items (§8.3),
-- outbound_dedup (§5.4.1), email_suppressions (§11.9),
-- report_publications (§35).

-- §5.5 webhook_sources
CREATE TABLE IF NOT EXISTS webhook_sources (
    id                UUID PRIMARY KEY DEFAULT uuidv7(),
    tenant_id         UUID NOT NULL,
    name              TEXT NOT NULL,
    signature_kind    TEXT NOT NULL,
    signature_header  TEXT NOT NULL,
    secret_encrypted  BYTEA NOT NULL,
    secret_rotated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ip_allowlist      INET[],
    replay_window_s   INTEGER NOT NULL DEFAULT 300,
    enabled           BOOLEAN NOT NULL DEFAULT true,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, name)
);
CREATE INDEX IF NOT EXISTS webhook_sources_tenant_idx
    ON webhook_sources (tenant_id) WHERE enabled = true;
ALTER TABLE webhook_sources ENABLE ROW LEVEL SECURITY;
ALTER TABLE webhook_sources FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS ws_isolation ON webhook_sources;
CREATE POLICY ws_isolation ON webhook_sources
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

-- §32.3 inbound_messages
CREATE TABLE IF NOT EXISTS inbound_messages (
    id                     UUID PRIMARY KEY DEFAULT uuidv7(),
    tenant_id              UUID NOT NULL,
    channel                TEXT NOT NULL,
    channel_address_id     UUID NOT NULL REFERENCES channel_addresses(id),
    sender_channel_user_id TEXT NOT NULL,
    received_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    cloudevent_jsonb       JSONB NOT NULL,
    attachments_jsonb      JSONB NOT NULL DEFAULT '[]'::jsonb,
    stage                  TEXT NOT NULL DEFAULT 'queued',
    stage_started_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    classification         JSONB,
    workable_item_id       TEXT,
    last_reply_at          TIMESTAMPTZ,
    failure_reason         TEXT,
    failure_details        JSONB
);
CREATE INDEX IF NOT EXISTS inbound_fifo_idx
    ON inbound_messages (tenant_id, channel_address_id, received_at)
    WHERE stage NOT IN ('completed', 'failed');
ALTER TABLE inbound_messages ENABLE ROW LEVEL SECURITY;
ALTER TABLE inbound_messages FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS inb_isolation ON inbound_messages;
CREATE POLICY inb_isolation ON inbound_messages
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

-- §12 thread_refs
CREATE TABLE IF NOT EXISTS thread_refs (
    tenant_id         UUID NOT NULL,
    logical_thread_id UUID NOT NULL,
    channel           TEXT NOT NULL,
    thread_id         TEXT,
    parent_message_id TEXT,
    root_message_id   TEXT,
    last_activity_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, logical_thread_id, channel)
);
CREATE INDEX IF NOT EXISTS thread_refs_recent_idx
    ON thread_refs (tenant_id, last_activity_at DESC);
ALTER TABLE thread_refs ENABLE ROW LEVEL SECURITY;
ALTER TABLE thread_refs FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tr_isolation ON thread_refs;
CREATE POLICY tr_isolation ON thread_refs
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

-- §15.2 quarantined_messages
CREATE TABLE IF NOT EXISTS quarantined_messages (
    id                     UUID PRIMARY KEY DEFAULT uuidv7(),
    tenant_id              UUID NOT NULL,
    received_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    channel                TEXT NOT NULL,
    sender_channel_user_id TEXT NOT NULL,
    sender_display         TEXT,
    reason                 TEXT NOT NULL,
    payload_jsonb          JSONB NOT NULL,
    triage_status          TEXT NOT NULL DEFAULT 'pending',
    triaged_at             TIMESTAMPTZ,
    triaged_by             UUID
);
CREATE INDEX IF NOT EXISTS qm_pending_idx
    ON quarantined_messages (tenant_id, received_at DESC)
    WHERE triage_status = 'pending';
ALTER TABLE quarantined_messages ENABLE ROW LEVEL SECURITY;
ALTER TABLE quarantined_messages FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS qm_isolation ON quarantined_messages;
CREATE POLICY qm_isolation ON quarantined_messages
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

-- §5.4 dead_letters
CREATE TABLE IF NOT EXISTS dead_letters (
    id                UUID PRIMARY KEY DEFAULT uuidv7(),
    tenant_id         UUID NOT NULL,
    original_event_id UUID NOT NULL,
    channel           TEXT NOT NULL,
    attempt_count     INTEGER NOT NULL,
    last_error        TEXT,
    payload_jsonb     JSONB NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    triaged_at        TIMESTAMPTZ,
    triage_status     TEXT
);
ALTER TABLE dead_letters ENABLE ROW LEVEL SECURITY;
ALTER TABLE dead_letters FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS dl_isolation ON dead_letters;
CREATE POLICY dl_isolation ON dead_letters
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

-- §8.3 workable_items
CREATE TABLE IF NOT EXISTS workable_items (
    tenant_id     UUID NOT NULL,
    item_id       TEXT NOT NULL,
    prefix        TEXT NOT NULL,
    sequence      INTEGER NOT NULL,
    item_type     TEXT NOT NULL,
    status        TEXT NOT NULL,
    opened_by     UUID,
    opened_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at   TIMESTAMPTZ,
    source_thread JSONB,
    PRIMARY KEY (tenant_id, item_id),
    UNIQUE (tenant_id, prefix, sequence)
);
CREATE INDEX IF NOT EXISTS wi_status_idx
    ON workable_items (tenant_id, status)
    WHERE status <> 'resolved';
ALTER TABLE workable_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE workable_items FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS wi_isolation ON workable_items;
CREATE POLICY wi_isolation ON workable_items
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

-- §5.4.1 outbound_dedup
CREATE TABLE IF NOT EXISTS outbound_dedup (
    tenant_id      UUID NOT NULL,
    outbound_key   TEXT NOT NULL,
    sent_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    channel_msg_id TEXT,
    expires_at     TIMESTAMPTZ NOT NULL DEFAULT (now() + interval '24 hours'),
    PRIMARY KEY (tenant_id, outbound_key)
);
ALTER TABLE outbound_dedup ENABLE ROW LEVEL SECURITY;
ALTER TABLE outbound_dedup FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS od_isolation ON outbound_dedup;
CREATE POLICY od_isolation ON outbound_dedup
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

-- §11.9 email_suppressions
CREATE TABLE IF NOT EXISTS email_suppressions (
    tenant_id    UUID NOT NULL,
    address      TEXT NOT NULL,
    reason       TEXT NOT NULL,
    source_event UUID,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, address)
);
ALTER TABLE email_suppressions ENABLE ROW LEVEL SECURITY;
ALTER TABLE email_suppressions FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS es_isolation ON email_suppressions;
CREATE POLICY es_isolation ON email_suppressions
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

-- §35 report_publications
CREATE TABLE IF NOT EXISTS report_publications (
    id                 UUID PRIMARY KEY DEFAULT uuidv7(),
    tenant_id          UUID NOT NULL,
    update_key         TEXT NOT NULL,
    channel            TEXT NOT NULL,
    channel_address_id UUID NOT NULL REFERENCES channel_addresses(id),
    channel_msg_id     TEXT,
    content_sha256     BYTEA NOT NULL,
    git_commit_sha     TEXT,
    published_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, update_key, channel, channel_address_id)
);
ALTER TABLE report_publications ENABLE ROW LEVEL SECURITY;
ALTER TABLE report_publications FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS rp_isolation ON report_publications;
CREATE POLICY rp_isolation ON report_publications
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
