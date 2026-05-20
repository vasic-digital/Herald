-- migration 000006: constitution_state + constitution_audit (V3 §42.1.2)
--
-- Lands the per-(tenant, rule, subject) state table that the transition
-- gate UPSERTs into, plus the append-only audit table that records every
-- transition the gate detected.
--
-- Both tables are RLS-guarded by app.tenant_id GUC (per §16). Foundation
-- M2 wires the GUC via WithTenantContext in commons_storage/tenant_context.go.

BEGIN;

CREATE TABLE constitution_state (
    tenant_id       UUID         NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    rule_id         TEXT         NOT NULL,
    subject         TEXT         NOT NULL,
    decision        SMALLINT     NOT NULL CHECK (decision BETWEEN 0 AND 4),
    digest_sha      BYTEA        NOT NULL CHECK (octet_length(digest_sha) = 32),
    bundle_hash     BYTEA        NOT NULL CHECK (octet_length(bundle_hash) = 32),
    evidence_uri    TEXT         NOT NULL DEFAULT '',
    transitioned_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, rule_id, subject)
);

COMMENT ON TABLE constitution_state IS
'Per-(tenant, rule, subject) verdict-of-record. UPSERTed by Runner.Run via ConstitutionStore.Record. RLS-guarded — every read MUST run inside WithTenantContext(ctx, tenant_id).';

CREATE INDEX constitution_state_decision_idx
    ON constitution_state (tenant_id, decision, transitioned_at DESC);

CREATE INDEX constitution_state_rule_idx
    ON constitution_state (tenant_id, rule_id, transitioned_at DESC);

ALTER TABLE constitution_state ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON constitution_state
    USING (tenant_id = current_setting('app.tenant_id')::uuid);

-- ---- constitution_audit (append-only) ---------------------------------

CREATE TABLE constitution_audit (
    id                UUID         PRIMARY KEY DEFAULT uuidv7(),
    tenant_id         UUID         NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    rule_id           TEXT         NOT NULL,
    subject           TEXT         NOT NULL,
    old_decision      SMALLINT     CHECK (old_decision IS NULL OR old_decision BETWEEN 0 AND 4),
    new_decision      SMALLINT     NOT NULL CHECK (new_decision BETWEEN 0 AND 4),
    old_digest_sha    BYTEA        CHECK (old_digest_sha IS NULL OR octet_length(old_digest_sha) = 32),
    new_digest_sha    BYTEA        NOT NULL CHECK (octet_length(new_digest_sha) = 32),
    bundle_hash       BYTEA        NOT NULL CHECK (octet_length(bundle_hash) = 32),
    evidence_uri      TEXT         NOT NULL DEFAULT '',
    emitted_event_id  UUID,                       -- NULL if mode=warn (audit-only)
    mode_at_emission  SMALLINT     NOT NULL CHECK (mode_at_emission BETWEEN 0 AND 2),
    audited_at        TIMESTAMPTZ  NOT NULL DEFAULT now()
);

COMMENT ON TABLE constitution_audit IS
'Append-only audit trail of every transition emitted (mode=warn or enforce) by the Runner. RLS-guarded. emitted_event_id is NULL when mode=warn (audit-only, no channel emit).';

CREATE INDEX constitution_audit_lookup_idx
    ON constitution_audit (tenant_id, rule_id, audited_at DESC);

CREATE INDEX constitution_audit_event_idx
    ON constitution_audit (emitted_event_id) WHERE emitted_event_id IS NOT NULL;

ALTER TABLE constitution_audit ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON constitution_audit
    USING (tenant_id = current_setting('app.tenant_id')::uuid);

-- Audit table is append-only — forbid UPDATE + DELETE at the row level.
-- A separate role MAY purge by tenant deletion (cascades via FK) for GDPR.
CREATE POLICY tenant_audit_no_update ON constitution_audit
    FOR UPDATE USING (false);
CREATE POLICY tenant_audit_no_delete ON constitution_audit
    FOR DELETE USING (false);

COMMIT;
