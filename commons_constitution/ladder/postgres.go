package ladder

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	db "digital.vasic.database/pkg/database"
	"github.com/google/uuid"
	"github.com/vasic-digital/herald/commons_constitution"
)

// Postgres is the RLS-guarded ModeLadder backend for M2/M3.
//
// Per spec §42.1.4 + Foundation design §3.1 step [7]: every Get on the
// hot path (every evaluator call) reads constitution_bindings; M3's
// Redis read-cache wrapper sits in front (60s TTL).
//
// Concurrent-safe via Postgres-side row locking. Each call opens its
// own transaction with RLS tenant-scope.
type Postgres struct {
	store db.Database
}

// NewPostgres wraps a digital.vasic.database/pkg/database.Database with
// ModeLadder semantics. The caller is responsible for connection
// lifecycle (Open + Close).
func NewPostgres(database db.Database) *Postgres {
	return &Postgres{store: database}
}

// Get implements constitution.ModeLadder. Returns ModeEnforce (the safe
// default per §42.1.4) when no binding exists for (tenantID, ruleID).
func (p *Postgres) Get(ctx context.Context, tenantID uuid.UUID, ruleID string) (constitution.Mode, error) {
	tx, err := p.store.Begin(ctx)
	if err != nil {
		return constitution.ModeEnforce, fmt.Errorf("ladder/postgres: Begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE herald_app"); err != nil {
		return constitution.ModeEnforce, fmt.Errorf("ladder/postgres: SET LOCAL ROLE: %w", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL app.tenant_id = '"+tenantID.String()+"'"); err != nil {
		return constitution.ModeEnforce, fmt.Errorf("ladder/postgres: SET LOCAL: %w", err)
	}

	row := tx.QueryRow(ctx,
		`SELECT mode FROM constitution_bindings WHERE tenant_id = $1 AND rule_id = $2`,
		tenantID, ruleID,
	)
	var mode int16
	if err := row.Scan(&mode); err != nil {
		if errors.Is(err, sql.ErrNoRows) || isPgxNoRows(err) {
			// Safe default — unbound rules enforce.
			return constitution.ModeEnforce, nil
		}
		return constitution.ModeEnforce, fmt.Errorf("ladder/postgres: Get scan: %w", err)
	}
	return constitution.Mode(mode), nil
}

// Set implements constitution.ModeLadder. Upserts the binding.
func (p *Postgres) Set(ctx context.Context, tenantID uuid.UUID, ruleID string, m constitution.Mode, by string) error {
	tx, err := p.store.Begin(ctx)
	if err != nil {
		return fmt.Errorf("ladder/postgres: Begin: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE herald_app"); err != nil {
		return fmt.Errorf("ladder/postgres: SET LOCAL ROLE: %w", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL app.tenant_id = '"+tenantID.String()+"'"); err != nil {
		return fmt.Errorf("ladder/postgres: SET LOCAL: %w", err)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO constitution_bindings (tenant_id, rule_id, mode, mutated_at, mutated_by)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (tenant_id, rule_id) DO UPDATE
		 SET mode = EXCLUDED.mode,
		     mutated_at = EXCLUDED.mutated_at,
		     mutated_by = EXCLUDED.mutated_by`,
		tenantID, ruleID, int16(m), time.Now().UTC(), by,
	)
	if err != nil {
		return fmt.Errorf("ladder/postgres: UPSERT: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("ladder/postgres: commit: %w", err)
	}
	committed = true
	return nil
}

// List implements constitution.ModeLadder.
func (p *Postgres) List(ctx context.Context, tenantID uuid.UUID) (map[string]constitution.Mode, error) {
	tx, err := p.store.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("ladder/postgres: Begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE herald_app"); err != nil {
		return nil, fmt.Errorf("ladder/postgres: SET LOCAL ROLE: %w", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL app.tenant_id = '"+tenantID.String()+"'"); err != nil {
		return nil, fmt.Errorf("ladder/postgres: SET LOCAL: %w", err)
	}

	rows, err := tx.Query(ctx,
		`SELECT rule_id, mode FROM constitution_bindings WHERE tenant_id = $1`,
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("ladder/postgres: List query: %w", err)
	}
	defer rows.Close()

	out := map[string]constitution.Mode{}
	for rows.Next() {
		var ruleID string
		var mode int16
		if err := rows.Scan(&ruleID, &mode); err != nil {
			return nil, fmt.Errorf("ladder/postgres: List scan: %w", err)
		}
		out[ruleID] = constitution.Mode(mode)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ladder/postgres: List iter: %w", err)
	}
	return out, nil
}

// isPgxNoRows is the shared "no rows" detector (kept local to avoid
// cross-package coupling between ladder + state packages).
func isPgxNoRows(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return s == "no rows in result set" ||
		s == "pgx: no rows in result set" ||
		s == "scanning row: no rows in result set"
}
