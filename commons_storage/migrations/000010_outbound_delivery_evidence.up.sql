-- 000010_outbound_delivery_evidence.up.sql
-- Per spec V3 §11.0 + §16: per-tenant log of outbound message dispatch.
--
-- Captured at SEND-time so a Herald operator can prove delivery to the
-- recipient channel without re-issuing the request. Distinct from
-- outbound_dedup (§5.4.1) which keys on the idempotency outbound_key
-- to suppress double-sends — this table is the AUDIT/evidence trail.
--
-- §107 anti-bluff guard: channel_message_id MUST be the chat-side ID
-- returned by the channel provider (Telegram message_id, Slack ts, SMTP
-- queue id, ...) — NOT a Herald-generated UUID. A Herald-synthetic ID
-- would falsely prove delivery. The integration test in
-- commons_messaging/channels/tgram/persist_integration_test.go asserts
-- exact equality between the persisted column and the receipt returned
-- by Adapter.Send (which itself reads from the Bot API response).

CREATE TABLE IF NOT EXISTS outbound_delivery_evidence (
    id                 UUID PRIMARY KEY DEFAULT uuidv7(),
    tenant_id          UUID NOT NULL,
    channel_id         TEXT NOT NULL,
    channel_message_id TEXT NOT NULL,
    evidence           INT NOT NULL,
    sent_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS outbound_delivery_evidence_tenant_sent_idx
    ON outbound_delivery_evidence (tenant_id, sent_at DESC);

-- RLS — same FORCE + herald_app grant pattern as 000008 / 000003.
-- §44.6: FORCE makes RLS apply even to the owning role; without it the
-- bootstrap POSTGRES_USER (often SUPERUSER) bypasses tenant isolation.
ALTER TABLE outbound_delivery_evidence ENABLE ROW LEVEL SECURITY;
ALTER TABLE outbound_delivery_evidence FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS ode_isolation ON outbound_delivery_evidence;
CREATE POLICY ode_isolation ON outbound_delivery_evidence
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON outbound_delivery_evidence TO herald_app;
