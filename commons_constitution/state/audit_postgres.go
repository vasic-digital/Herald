package state

import (
	"context"
	"fmt"

	db "digital.vasic.database/pkg/database"
	"github.com/google/uuid"
	"github.com/vasic-digital/herald/commons_constitution"
)

// PostgresAudit is the RLS-guarded constitution.AuditStore backend for
// M2/M3. INSERT-only — the constitution_audit RLS policy from migration
// 000006 forbids UPDATE + DELETE at the row level, so RecordAudit is the
// only mutator and there is no Update/Delete surface by construction.
//
// Concurrent-safe: each call opens its own transaction with RLS tenant
// scope (SET LOCAL ROLE herald_app + SET LOCAL app.tenant_id).
type PostgresAudit struct {
	store db.Database
}

// NewPostgresAudit wraps a digital.vasic.database/pkg/database.Database
// with AuditStore semantics. Caller owns the connection lifecycle.
func NewPostgresAudit(database db.Database) *PostgresAudit {
	return &PostgresAudit{store: database}
}

// RecordAudit appends one constitution_audit row. The DB assigns the id via
// the uuidv7() column default when row.ID is uuid.Nil; the assigned id is
// returned via INSERT ... RETURNING id.
//
// emitted_event_id is stored NULL when row.EmittedEventID is uuid.Nil
// (ModeWarn audit-only rows). old_decision / old_digest_sha are stored NULL
// when their AuditRow pointers are nil (FirstSeen transitions).
func (p *PostgresAudit) RecordAudit(ctx context.Context, row constitution.AuditRow) (uuid.UUID, error) {
	tx, err := p.store.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("state/postgres audit: Begin: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE herald_app"); err != nil {
		return uuid.Nil, fmt.Errorf("state/postgres audit: SET LOCAL ROLE: %w", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL app.tenant_id = '"+row.TenantID.String()+"'"); err != nil {
		return uuid.Nil, fmt.Errorf("state/postgres audit: SET LOCAL: %w", err)
	}

	// NULL-able columns: convert nil pointers + Nil UUID to nil interface so
	// the driver writes SQL NULL.
	var oldDecision any
	if row.OldDecision != nil {
		oldDecision = int16(*row.OldDecision)
	}
	var oldDigest any
	if row.OldDigest != nil {
		oldDigest = row.OldDigest[:]
	}
	var emittedEventID any
	if row.EmittedEventID != uuid.Nil {
		emittedEventID = row.EmittedEventID
	}

	// uuidv7() default fills id when row.ID is Nil; otherwise honor caller's id.
	var assignedID uuid.UUID
	if row.ID != uuid.Nil {
		err = tx.QueryRow(ctx,
			`INSERT INTO constitution_audit
			   (id, tenant_id, rule_id, subject, old_decision, new_decision,
			    old_digest_sha, new_digest_sha, bundle_hash, evidence_uri,
			    emitted_event_id, mode_at_emission)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
			 RETURNING id`,
			row.ID, row.TenantID, row.RuleID, row.Subject,
			oldDecision, int16(row.NewDecision),
			oldDigest, row.NewDigest[:], row.BundleHash[:], row.EvidenceURI,
			emittedEventID, int16(row.ModeAtEmission),
		).Scan(&assignedID)
	} else {
		err = tx.QueryRow(ctx,
			`INSERT INTO constitution_audit
			   (tenant_id, rule_id, subject, old_decision, new_decision,
			    old_digest_sha, new_digest_sha, bundle_hash, evidence_uri,
			    emitted_event_id, mode_at_emission)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			 RETURNING id`,
			row.TenantID, row.RuleID, row.Subject,
			oldDecision, int16(row.NewDecision),
			oldDigest, row.NewDigest[:], row.BundleHash[:], row.EvidenceURI,
			emittedEventID, int16(row.ModeAtEmission),
		).Scan(&assignedID)
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("state/postgres audit: INSERT: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("state/postgres audit: commit: %w", err)
	}
	committed = true
	return assignedID, nil
}

// ListAudit returns audit rows for tenantID matching q, newest-first
// (audited_at DESC). RLS handles tenant scope; the explicit filters keep
// the plan index-friendly on (tenant_id, rule_id, audited_at DESC).
func (p *PostgresAudit) ListAudit(ctx context.Context, tenantID uuid.UUID, q constitution.AuditQuery) ([]constitution.AuditRow, error) {
	tx, err := p.store.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("state/postgres audit: Begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE herald_app"); err != nil {
		return nil, fmt.Errorf("state/postgres audit: SET LOCAL ROLE: %w", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL app.tenant_id = '"+tenantID.String()+"'"); err != nil {
		return nil, fmt.Errorf("state/postgres audit: SET LOCAL: %w", err)
	}

	query := `SELECT id, rule_id, subject, old_decision, new_decision,
	                 old_digest_sha, new_digest_sha, bundle_hash, evidence_uri,
	                 emitted_event_id, mode_at_emission, audited_at
	          FROM constitution_audit WHERE TRUE`
	args := []any{}
	if q.RuleID != "" {
		query += ` AND rule_id = $` + intToStr(len(args)+1)
		args = append(args, q.RuleID)
	}
	if q.Subject != "" {
		query += ` AND subject = $` + intToStr(len(args)+1)
		args = append(args, q.Subject)
	}
	if !q.Since.IsZero() {
		query += ` AND audited_at >= $` + intToStr(len(args)+1)
		args = append(args, q.Since)
	}
	if !q.Until.IsZero() {
		query += ` AND audited_at <= $` + intToStr(len(args)+1)
		args = append(args, q.Until)
	}
	query += ` ORDER BY audited_at DESC`
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
		return nil, fmt.Errorf("state/postgres audit: List query: %w", err)
	}
	defer rows.Close()

	out := []constitution.AuditRow{}
	for rows.Next() {
		var (
			id                          uuid.UUID
			ruleID, subject, evidenceURI string
			newDecision, modeAtEmission int16
			newDigest, bundleHash       []byte
			oldDecision                 *int16
			oldDigest                   []byte
			emittedEventID              *uuid.UUID
			ar                          constitution.AuditRow
		)
		if err := rows.Scan(&id, &ruleID, &subject, &oldDecision, &newDecision,
			&oldDigest, &newDigest, &bundleHash, &evidenceURI,
			&emittedEventID, &modeAtEmission, &ar.AuditedAt); err != nil {
			return nil, fmt.Errorf("state/postgres audit: List scan: %w", err)
		}
		ar.ID = id
		ar.TenantID = tenantID
		ar.RuleID = ruleID
		ar.Subject = subject
		ar.NewDecision = constitution.Decision(newDecision)
		ar.ModeAtEmission = constitution.Mode(modeAtEmission)
		ar.EvidenceURI = evidenceURI
		copy(ar.NewDigest[:], newDigest)
		copy(ar.BundleHash[:], bundleHash)
		if oldDecision != nil {
			d := constitution.Decision(*oldDecision)
			ar.OldDecision = &d
		}
		if oldDigest != nil {
			var od [32]byte
			copy(od[:], oldDigest)
			ar.OldDigest = &od
		}
		if emittedEventID != nil {
			ar.EmittedEventID = *emittedEventID
		}
		out = append(out, ar)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("state/postgres audit: List iter: %w", err)
	}
	return out, nil
}
