// Package inbound — item_mutator.go (WS-4 / HRD-152): the ItemMutator
// interface + a concrete RepoMutator backed by commons_workable.Repo.
//
// ItemMutator is the workable-item CRUD boundary for the inbound action
// router (parallel to IssueOpener). Unit tests fake it (recording the
// calls) so the router logic is tested in isolation; the concrete
// RepoMutator is exercised against a REAL SQLite Store in
// repo_mutator_test.go (no metadata-only PASS).
//
// §107 anchor: RepoMutator.Update reads the existing row, applies the
// field deltas, and writes it back through the real Repo.Update — a
// missing row surfaces as workable.ErrNotFound, an invalid status is
// rejected by Repo.Update's closed-set check.
package inbound

import (
	"context"
	"fmt"

	workable "github.com/vasic-digital/herald/commons_workable"
)

// ItemMutator mutates workable items from the LLM's item.update /
// item.delete action triggers (and from confirmed investigation
// proposals). It is the DB boundary the router depends on — production
// binds *RepoMutator; unit tests bind a recording fake.
type ItemMutator interface {
	Update(ctx context.Context, atmID, location string, fields map[string]string) error
	Delete(ctx context.Context, atmID, location string) error
}

// RepoMutator is the concrete ItemMutator backed by a
// commons_workable.Repo (SQLite SSoT).
type RepoMutator struct {
	repo *workable.Repo
}

// NewRepoMutator binds a RepoMutator to an open Repo.
func NewRepoMutator(repo *workable.Repo) *RepoMutator {
	return &RepoMutator{repo: repo}
}

// Update reads the existing (atmID, location) item, applies the
// field→value deltas, and writes it back. A missing row surfaces as
// workable.ErrNotFound; an unknown column name is rejected loudly so a
// typo never silently no-ops (§107 anti-bluff).
func (m *RepoMutator) Update(ctx context.Context, atmID, location string, fields map[string]string) error {
	if m.repo == nil {
		return fmt.Errorf("inbound.RepoMutator: nil repo")
	}
	it, err := m.repo.GetByID(ctx, atmID, location)
	if err != nil {
		return err
	}
	if it == nil {
		return fmt.Errorf("%w: %s/%s", workable.ErrNotFound, atmID, location)
	}
	if err := applyFields(it, fields); err != nil {
		return err
	}
	return m.repo.Update(ctx, *it)
}

// Delete removes the (atmID, location) item via the real Repo.Delete.
func (m *RepoMutator) Delete(ctx context.Context, atmID, location string) error {
	if m.repo == nil {
		return fmt.Errorf("inbound.RepoMutator: nil repo")
	}
	return m.repo.Delete(ctx, atmID, location)
}

// applyFields mutates it in place from the column→value map. Unknown
// columns are an error (no silent drop). The composite-key columns
// (atm_id / current_location) are NOT updatable via this path — a
// move-between-locations is a delete+create, not an in-place field edit.
func applyFields(it *workable.Item, fields map[string]string) error {
	for k, v := range fields {
		switch k {
		case "type":
			it.Type = v
		case "status":
			it.Status = v
		case "severity":
			it.Severity = v
		case "title":
			it.Title = v
		case "description":
			it.Description = v
		case "forensic_anchor":
			it.ForensicAnchor = v
		case "closure_criteria":
			it.ClosureCriteria = v
		case "composes_with":
			it.ComposesWith = v
		case "body_md":
			it.BodyMd = v
		case "last_modified":
			it.LastModified = v
		case "created_by":
			// PARTICIPANT_ATTRIBUTION §4b: canonical handle of who opened
			// the item. Injected by the inbound attribution wiring.
			it.CreatedBy = v
		case "assigned_to":
			// PARTICIPANT_ATTRIBUTION §4b: canonical handle of the assignee.
			it.AssignedTo = v
		default:
			return fmt.Errorf("inbound.RepoMutator: unknown/unupdatable field %q", k)
		}
	}
	return nil
}
