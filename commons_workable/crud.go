package workable

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrNotFound is returned (wrapped) by Update/Delete when no row matches
// the given (atmID, location) composite key.
var ErrNotFound = errors.New("workable: item not found")

// Repo is a CRUD repository over the `items` table.
type Repo struct {
	s *Store
}

// NewRepo binds a Repo to an open Store.
func NewRepo(s *Store) *Repo { return &Repo{s: s} }

const itemColumns = `atm_id, type, status, severity, title, description,
	forensic_anchor, closure_criteria, composes_with, current_location,
	body_md, created_at, last_modified, created_by, assigned_to`

// Create inserts a new item. The status is validated against the closed
// set before any DB access; an unknown/empty status is rejected loudly.
func (r *Repo) Create(ctx context.Context, it Item) error {
	if !ValidStatus(it.Status) {
		return fmt.Errorf("workable: invalid status %q (not in closed set)", it.Status)
	}
	_, err := r.s.db.ExecContext(ctx,
		`INSERT INTO items (`+itemColumns+`)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		it.AtmID, it.Type, it.Status, it.Severity, it.Title, it.Description,
		it.ForensicAnchor, it.ClosureCriteria, it.ComposesWith, it.CurrentLocation,
		it.BodyMd, it.CreatedAt, it.LastModified, it.CreatedBy, it.AssignedTo)
	if err != nil {
		return fmt.Errorf("workable: create %s/%s: %w", it.AtmID, it.CurrentLocation, err)
	}
	return nil
}

// GetByID returns the item with the given composite key, or (nil, nil)
// when no such row exists.
func (r *Repo) GetByID(ctx context.Context, atmID, location string) (*Item, error) {
	row := r.s.db.QueryRowContext(ctx,
		`SELECT `+itemColumns+` FROM items WHERE atm_id=? AND current_location=?`,
		atmID, location)
	var it Item
	err := row.Scan(
		&it.AtmID, &it.Type, &it.Status, &it.Severity, &it.Title, &it.Description,
		&it.ForensicAnchor, &it.ClosureCriteria, &it.ComposesWith, &it.CurrentLocation,
		&it.BodyMd, &it.CreatedAt, &it.LastModified, &it.CreatedBy, &it.AssignedTo)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("workable: get %s/%s: %w", atmID, location, err)
	}
	return &it, nil
}

// Update mutates an existing item. The status is validated against the
// closed set first; a missing row is reported as ErrNotFound.
func (r *Repo) Update(ctx context.Context, it Item) error {
	if !ValidStatus(it.Status) {
		return fmt.Errorf("workable: invalid status %q (not in closed set)", it.Status)
	}
	res, err := r.s.db.ExecContext(ctx,
		`UPDATE items SET type=?, status=?, severity=?, title=?, description=?,
		 forensic_anchor=?, closure_criteria=?, composes_with=?, body_md=?,
		 created_at=?, last_modified=?, created_by=?, assigned_to=?
		 WHERE atm_id=? AND current_location=?`,
		it.Type, it.Status, it.Severity, it.Title, it.Description,
		it.ForensicAnchor, it.ClosureCriteria, it.ComposesWith, it.BodyMd,
		it.CreatedAt, it.LastModified, it.CreatedBy, it.AssignedTo,
		it.AtmID, it.CurrentLocation)
	if err != nil {
		return fmt.Errorf("workable: update %s/%s: %w", it.AtmID, it.CurrentLocation, err)
	}
	return requireOneRow(res, it.AtmID, it.CurrentLocation)
}

// Delete removes an item. A missing row is reported as ErrNotFound.
func (r *Repo) Delete(ctx context.Context, atmID, location string) error {
	res, err := r.s.db.ExecContext(ctx,
		`DELETE FROM items WHERE atm_id=? AND current_location=?`, atmID, location)
	if err != nil {
		return fmt.Errorf("workable: delete %s/%s: %w", atmID, location, err)
	}
	return requireOneRow(res, atmID, location)
}

// List returns every item at the given location, ordered by atm_id for
// deterministic output.
func (r *Repo) List(ctx context.Context, location string) ([]Item, error) {
	rows, err := r.s.db.QueryContext(ctx,
		`SELECT `+itemColumns+` FROM items WHERE current_location=? ORDER BY atm_id`,
		location)
	if err != nil {
		return nil, fmt.Errorf("workable: list %s: %w", location, err)
	}
	defer rows.Close()

	var out []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(
			&it.AtmID, &it.Type, &it.Status, &it.Severity, &it.Title, &it.Description,
			&it.ForensicAnchor, &it.ClosureCriteria, &it.ComposesWith, &it.CurrentLocation,
			&it.BodyMd, &it.CreatedAt, &it.LastModified, &it.CreatedBy, &it.AssignedTo); err != nil {
			return nil, fmt.Errorf("workable: list scan: %w", err)
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("workable: list rows: %w", err)
	}
	return out, nil
}

func requireOneRow(res sql.Result, atmID, location string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("workable: rows affected for %s/%s: %w", atmID, location, err)
	}
	if n == 0 {
		return fmt.Errorf("%w: %s/%s", ErrNotFound, atmID, location)
	}
	return nil
}
