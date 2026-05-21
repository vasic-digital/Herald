-- migration 000012: events_processed (V3 §32.2 idempotency archive)
--
-- Inbound idempotency archive. The Redis SETNX gate (24h TTL) handles
-- the hot path; this table is the 30-day audit + replay archive.
-- RLS-guarded by app.current_tenant_id GUC (same pattern as 000006).
--
-- Wave 3 OutcomeRecorder writes here AFTER outbound_delivery_evidence
-- so a successful event acceptance leaves both rows; an aborted ingest
-- never appears here (no events_processed row → safe to replay).

BEGIN;

CREATE TABLE events_processed (
    tenant_id        UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    idempotency_key  TEXT        NOT NULL,
    event_id         UUID        NOT NULL,
    first_seen_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at       TIMESTAMPTZ NOT NULL DEFAULT (now() + INTERVAL '30 days'),
    PRIMARY KEY (tenant_id, idempotency_key)
);

COMMENT ON TABLE events_processed IS
'Inbound idempotency archive per V3 §32.2. UPSERTed by Runner.OutcomeRecorder after dispatch. RLS-guarded — every read MUST run inside WithTenantContext(ctx, tenant_id). 30-day retention via expires_at; sweep is HRD-047 (scherald status-digest).';

CREATE INDEX events_processed_expires_idx
    ON events_processed (expires_at);

CREATE INDEX events_processed_event_id_idx
    ON events_processed (event_id);

ALTER TABLE events_processed ENABLE ROW LEVEL SECURITY;
ALTER TABLE events_processed FORCE  ROW LEVEL SECURITY;

CREATE POLICY events_processed_tenant_isolation ON events_processed
    USING (tenant_id::text = current_setting('app.current_tenant_id', true));

COMMIT;
