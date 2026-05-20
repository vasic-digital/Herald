-- migration 000007: constitution_bindings (V3 §42.1.4)
--
-- Per-tenant per-rule mode-ladder source of truth. Read-cached at M3 by
-- Redis (60s TTL) per Foundation design §3.1 step [6.b].

BEGIN;

CREATE TABLE constitution_bindings (
    tenant_id   UUID         NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    rule_id     TEXT         NOT NULL,
    mode        SMALLINT     NOT NULL CHECK (mode BETWEEN 0 AND 2),   -- 0 allow / 1 warn / 2 enforce
    mutated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    mutated_by  TEXT         NOT NULL,                                -- operator identity for audit
    PRIMARY KEY (tenant_id, rule_id)
);

COMMENT ON TABLE constitution_bindings IS
'Per-tenant per-rule mode ladder (0=allow, 1=warn, 2=enforce). UNBOUND rules default to enforce — see commons_constitution.ModeLadder semantics.';

CREATE INDEX constitution_bindings_mutated_idx
    ON constitution_bindings (tenant_id, mutated_at DESC);

ALTER TABLE constitution_bindings ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON constitution_bindings
    USING (tenant_id = current_setting('app.tenant_id')::uuid);

COMMIT;
