package state

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

// Postgres is the RLS-guarded ConstitutionStore backend for M2/M3.
//
// Per Foundation design §3.1 step [6] + §44.6 three-axis transition gate:
// every Record call (a) runs inside WithTenantContext (caller's job — this
// type holds NO tenant state itself), (b) reads the prior row inside the
// same transaction for transition computation, (c) UPSERTs the new row,
// (d) returns a fully-populated Transition.
//
// Concurrent-safe: each call uses its own transaction. The DB enforces
// (tenant_id, rule_id, subject) PK uniqueness at the storage layer.
type Postgres struct {
	store db.Database
}

// NewPostgres wraps a digital.vasic.database/pkg/database.Database with
// the ConstitutionStore semantics. The caller is responsible for the
// underlying connection lifecycle (Open + Close).
func NewPostgres(database db.Database) *Postgres {
	return &Postgres{store: database}
}

// Record implements constitution.ConstitutionStore.
//
// IMPORTANT: callers MUST wrap this in WithTenantContext from
// commons_storage so the RLS app.tenant_id GUC is set before the
// SELECT + UPSERT execute. Calling Record outside a tenant-scoped
// transaction will silently return zero rows from the prior-row SELECT
// (RLS hides them) and produce wrong Transition data.
func (p *Postgres) Record(
	ctx context.Context,
	tenantID uuid.UUID,
	ruleID, subject string,
	r constitution.Result,
	bundle constitution.BundleHash,
	evidenceURI string,
) (constitution.Transition, error) {
	tx, err := p.store.Begin(ctx)
	if err != nil {
		return constitution.Transition{}, fmt.Errorf("state/postgres: Begin: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()
	// Drop superuser privileges so RLS policies apply (the Postgres
	// connecting user from POSTGRES_USER is typically SUPERUSER which
	// bypasses RLS regardless of FORCE). herald_app role is NOBYPASSRLS
	// per migration 000001 + has grants from migration 000008.
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE herald_app"); err != nil {
		return constitution.Transition{}, fmt.Errorf("state/postgres: SET LOCAL ROLE: %w", err)
	}
	// Set RLS context inside this transaction so prior-row read is tenant-scoped.
	if _, err := tx.Exec(ctx, "SET LOCAL app.tenant_id = '"+tenantID.String()+"'"); err != nil {
		return constitution.Transition{}, fmt.Errorf("state/postgres: SET LOCAL: %w", err)
	}

	now := time.Now().UTC()
	trans := constitution.Transition{
		NewDecision:   r.Decision,
		NewDigest:     r.DigestSHA,
		NewBundleHash: bundle,
		At:            now,
	}

	// Read prior row (if any) inside the same Tx for transition computation.
	var (
		oldDecision   int16
		oldDigest     []byte
		oldBundleHash []byte
		oldTransAt    time.Time
	)
	row := tx.QueryRow(ctx,
		`SELECT decision, digest_sha, bundle_hash, transitioned_at
		 FROM constitution_state
		 WHERE tenant_id = $1 AND rule_id = $2 AND subject = $3`,
		tenantID, ruleID, subject,
	)
	if err := row.Scan(&oldDecision, &oldDigest, &oldBundleHash, &oldTransAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) || isPgxNoRows(err) {
			trans.FirstSeen = true
			trans.Changed = true
		} else {
			return constitution.Transition{}, fmt.Errorf("state/postgres: prior-row scan: %w", err)
		}
	} else {
		trans.OldDecision = constitution.Decision(oldDecision)
		copy(trans.OldDigest[:], oldDigest)
		copy(trans.OldBundleHash[:], oldBundleHash)
		trans.Changed = trans.OldDecision != r.Decision ||
			!equal32(trans.OldDigest, r.DigestSHA) ||
			!equalBundle(trans.OldBundleHash, bundle)
	}

	// Determine effective transitioned_at:
	//   - first sight OR changed: now
	//   - no change: preserve prior timestamp (don't lie about "when did
	//     this verdict first occur")
	transAt := now
	if !trans.FirstSeen && !trans.Changed {
		transAt = oldTransAt
		trans.At = transAt
	}

	// UPSERT — INSERT ... ON CONFLICT DO UPDATE.
	_, err = tx.Exec(ctx,
		`INSERT INTO constitution_state
		   (tenant_id, rule_id, subject, decision, digest_sha, bundle_hash, evidence_uri, transitioned_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (tenant_id, rule_id, subject) DO UPDATE
		 SET decision        = EXCLUDED.decision,
		     digest_sha      = EXCLUDED.digest_sha,
		     bundle_hash     = EXCLUDED.bundle_hash,
		     evidence_uri    = EXCLUDED.evidence_uri,
		     transitioned_at = EXCLUDED.transitioned_at`,
		tenantID, ruleID, subject,
		int16(r.Decision), r.DigestSHA[:], bundle[:], evidenceURI, transAt,
	)
	if err != nil {
		return constitution.Transition{}, fmt.Errorf("state/postgres: UPSERT: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return constitution.Transition{}, fmt.Errorf("state/postgres: commit: %w", err)
	}
	committed = true
	return trans, nil
}

// Get implements constitution.ConstitutionStore.
func (p *Postgres) Get(ctx context.Context, tenantID uuid.UUID, ruleID, subject string) (constitution.StateRow, bool, error) {
	tx, err := p.store.Begin(ctx)
	if err != nil {
		return constitution.StateRow{}, false, fmt.Errorf("state/postgres: Begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE herald_app"); err != nil {
		return constitution.StateRow{}, false, fmt.Errorf("state/postgres: SET LOCAL ROLE: %w", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL app.tenant_id = '"+tenantID.String()+"'"); err != nil {
		return constitution.StateRow{}, false, fmt.Errorf("state/postgres: SET LOCAL: %w", err)
	}

	row := tx.QueryRow(ctx,
		`SELECT decision, digest_sha, bundle_hash, evidence_uri, transitioned_at
		 FROM constitution_state
		 WHERE tenant_id = $1 AND rule_id = $2 AND subject = $3`,
		tenantID, ruleID, subject,
	)
	var (
		decision    int16
		digest      []byte
		bundleHash  []byte
		evidenceURI string
		transAt     time.Time
	)
	if err := row.Scan(&decision, &digest, &bundleHash, &evidenceURI, &transAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) || isPgxNoRows(err) {
			return constitution.StateRow{}, false, nil
		}
		return constitution.StateRow{}, false, fmt.Errorf("state/postgres: Get scan: %w", err)
	}
	out := constitution.StateRow{
		TenantID:       tenantID,
		RuleID:         ruleID,
		Subject:        subject,
		Decision:       constitution.Decision(decision),
		EvidenceURI:    evidenceURI,
		TransitionedAt: transAt,
	}
	copy(out.Digest[:], digest)
	copy(out.BundleHash[:], bundleHash)
	return out, true, nil
}

// List implements constitution.ConstitutionStore.
//
// Filter columns are appended only when non-zero so the query plan stays
// index-friendly on (tenant_id, decision, transitioned_at DESC).
func (p *Postgres) List(ctx context.Context, tenantID uuid.UUID, q constitution.ListQuery) ([]constitution.StateRow, error) {
	tx, err := p.store.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("state/postgres: Begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE herald_app"); err != nil {
		return nil, fmt.Errorf("state/postgres: SET LOCAL ROLE: %w", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL app.tenant_id = '"+tenantID.String()+"'"); err != nil {
		return nil, fmt.Errorf("state/postgres: SET LOCAL: %w", err)
	}

	// Build dynamic WHERE — RLS handles tenant_id; we add explicit filters.
	query := `SELECT rule_id, subject, decision, digest_sha, bundle_hash, evidence_uri, transitioned_at FROM constitution_state WHERE TRUE`
	args := []any{}
	if q.RuleID != "" {
		query += ` AND rule_id = $` + intToStr(len(args)+1)
		args = append(args, q.RuleID)
	}
	if q.Subject != "" {
		query += ` AND subject = $` + intToStr(len(args)+1)
		args = append(args, q.Subject)
	}
	if q.Decision != nil {
		query += ` AND decision = $` + intToStr(len(args)+1)
		args = append(args, int16(*q.Decision))
	}
	if !q.Since.IsZero() {
		query += ` AND transitioned_at >= $` + intToStr(len(args)+1)
		args = append(args, q.Since)
	}
	if !q.Until.IsZero() {
		query += ` AND transitioned_at <= $` + intToStr(len(args)+1)
		args = append(args, q.Until)
	}
	// ASC so OFFSET pagination is meaningful — without an explicit
	// ORDER BY, Postgres row order is undefined and Offset becomes
	// non-deterministic. Wave 3a /v1/compliance walks audit windows
	// oldest-first which matches this ordering naturally.
	query += ` ORDER BY transitioned_at ASC`
	if q.Limit > 0 {
		query += ` LIMIT $` + intToStr(len(args)+1)
		args = append(args, q.Limit)
	}
	if q.Offset > 0 {
		query += ` OFFSET $` + intToStr(len(args)+1)
		args = append(args, q.Offset)
	}

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("state/postgres: List query: %w", err)
	}
	defer rows.Close()

	out := []constitution.StateRow{}
	for rows.Next() {
		var (
			ruleID, subject, evidenceURI string
			decision                     int16
			digest, bundleHash           []byte
			transAt                      time.Time
		)
		if err := rows.Scan(&ruleID, &subject, &decision, &digest, &bundleHash, &evidenceURI, &transAt); err != nil {
			return nil, fmt.Errorf("state/postgres: List scan: %w", err)
		}
		row := constitution.StateRow{
			TenantID:       tenantID,
			RuleID:         ruleID,
			Subject:        subject,
			Decision:       constitution.Decision(decision),
			EvidenceURI:    evidenceURI,
			TransitionedAt: transAt,
		}
		copy(row.Digest[:], digest)
		copy(row.BundleHash[:], bundleHash)
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("state/postgres: List iter: %w", err)
	}
	return out, nil
}

// isPgxNoRows checks if err wraps a pgx-specific "no rows" sentinel.
// The Database interface returns sql.ErrNoRows in the canonical case;
// pgx's implementation may surface a different sentinel — detection is
// best-effort string match to avoid the pgx import here.
func isPgxNoRows(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return s == "no rows in result set" ||
		s == "pgx: no rows in result set" ||
		s == "scanning row: no rows in result set"
}

func equal32(a [32]byte, b [32]byte) bool {
	for i := 0; i < 32; i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalBundle(a, b constitution.BundleHash) bool {
	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func intToStr(n int) string {
	// Minimal positive-int → decimal string converter; we only use 1–10 here.
	if n < 10 {
		return string('0' + byte(n))
	}
	// Two-digit case (queries with >10 args are unrealistic here).
	return string('0'+byte(n/10)) + string('0'+byte(n%10))
}
