package claude_code

import (
	"context"
	"encoding/json"
	"fmt"

	db "digital.vasic.database/pkg/database"

	storage "github.com/vasic-digital/herald/commons_storage"
)

// PersistSessionState upserts the session row for this project after a
// successful Dispatch. Best-effort: persistence failure returns an error
// to the caller but does NOT roll back the dispatch (Claude has already
// responded; we'd rather log a persistence failure than re-issue the
// dispatch and double-spend Claude credits — §107 honest mode).
//
// §107 evidence: the persisted row MUST reflect the actual session_uuid
// and last_response Claude emitted — not a Herald-generated default. The
// integration test asserts equality on both fields.
//
// Tenant scoping: writes are scoped to HeraldSystemTenant (operator-shared
// bucket) regardless of customer tenant context — Claude Code sessions
// are Herald-infra state, not subscriber-tenant state. RLS still applies
// uniformly via the GUC.
//
// pgx v5 quirk: JSONB columns reject typed []byte (sent as bytea-cast,
// SQLSTATE 22P02). We pass last_response as string(payload) with an
// explicit ::jsonb cast in the SQL.
func (d *Dispatcher) PersistSessionState(ctx context.Context, resp DispatchResponse) error {
	if d.pool == nil {
		return fmt.Errorf("claude_code.PersistSessionState: no pool wired (use NewWithStorage)")
	}
	payload, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("claude_code.PersistSessionState: marshal response: %w", err)
	}
	return storage.WithTenantContext(ctx, d.pool, HeraldSystemTenant, func(tx db.Tx) error {
		_, execErr := tx.Exec(ctx,
			`INSERT INTO claude_code_sessions
			    (tenant_id, project_name, session_uuid, anchor_path, last_dispatch_at, last_response)
			 VALUES ($1, $2, $3, $4, NOW(), $5::jsonb)
			 ON CONFLICT (tenant_id, project_name) DO UPDATE SET
			   session_uuid = EXCLUDED.session_uuid,
			   anchor_path = EXCLUDED.anchor_path,
			   last_dispatch_at = NOW(),
			   last_response = EXCLUDED.last_response`,
			HeraldSystemTenant,
			d.projectName,
			resp.SessionUUID,
			resp.AnchorPath,
			string(payload),
		)
		return execErr
	})
}
