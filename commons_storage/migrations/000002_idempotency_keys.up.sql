-- 000002_idempotency_keys.up.sql — spec V3 §4.3.
CREATE TABLE IF NOT EXISTS idempotency_keys (
    tenant_id       UUID  NOT NULL,
    idempotency_key TEXT  NOT NULL,
    request_hash    BYTEA NOT NULL,
    response_id     UUID,
    locked_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL DEFAULT (now() + interval '24 hours'),
    PRIMARY KEY (tenant_id, idempotency_key)
) WITH (FILLFACTOR = 80);

CREATE INDEX IF NOT EXISTS idempotency_expires_idx
    ON idempotency_keys (expires_at);

ALTER TABLE idempotency_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE idempotency_keys FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS idem_isolation ON idempotency_keys;
CREATE POLICY idem_isolation ON idempotency_keys
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
