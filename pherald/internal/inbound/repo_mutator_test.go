package inbound_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	workable "github.com/vasic-digital/herald/commons_workable"
	"github.com/vasic-digital/herald/pherald/internal/inbound"
)

// These tests exercise the concrete RepoMutator against a REAL SQLite
// Store (the DB boundary). They prove the adapter actually mutates rows
// — not a metadata-only PASS.

func newRepo(t *testing.T) *workable.Repo {
	t.Helper()
	s, err := workable.Open(filepath.Join(t.TempDir(), "mutator.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return workable.NewRepo(s)
}

func seedItem(t *testing.T, repo *workable.Repo, atmID, location string) {
	t.Helper()
	err := repo.Create(context.Background(), workable.Item{
		AtmID:           atmID,
		Type:            "Bug",
		Status:          "Queued",
		Title:           "seed " + atmID,
		CurrentLocation: location,
		CreatedAt:       "2026-05-29",
		LastModified:    "2026-05-29",
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestRepoMutatorUpdateAppliesFields(t *testing.T) {
	repo := newRepo(t)
	seedItem(t, repo, "ATM-238", "Issues")
	m := inbound.NewRepoMutator(repo)

	err := m.Update(context.Background(), "ATM-238", "Issues", map[string]string{
		"status": "In progress",
		"title":  "updated title",
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := repo.GetByID(context.Background(), "ATM-238", "Issues")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("row vanished")
	}
	if got.Status != "In progress" {
		t.Fatalf("status not applied: %q", got.Status)
	}
	if got.Title != "updated title" {
		t.Fatalf("title not applied: %q", got.Title)
	}
}

func TestRepoMutatorUpdateMissingRow(t *testing.T) {
	repo := newRepo(t)
	m := inbound.NewRepoMutator(repo)
	err := m.Update(context.Background(), "ATM-404", "Issues", map[string]string{"status": "Queued"})
	if !errors.Is(err, workable.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestRepoMutatorUpdateRejectsInvalidStatus(t *testing.T) {
	repo := newRepo(t)
	seedItem(t, repo, "ATM-1", "Issues")
	m := inbound.NewRepoMutator(repo)
	err := m.Update(context.Background(), "ATM-1", "Issues", map[string]string{"status": "Bogus"})
	if err == nil {
		t.Fatal("want error for invalid status, got nil")
	}
}

func TestRepoMutatorDeleteRemovesRow(t *testing.T) {
	repo := newRepo(t)
	seedItem(t, repo, "ATM-7", "Fixed")
	m := inbound.NewRepoMutator(repo)
	if err := m.Delete(context.Background(), "ATM-7", "Fixed"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err := repo.GetByID(context.Background(), "ATM-7", "Fixed")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got != nil {
		t.Fatal("row not deleted")
	}
}

func TestRepoMutatorDeleteMissingRow(t *testing.T) {
	repo := newRepo(t)
	m := inbound.NewRepoMutator(repo)
	err := m.Delete(context.Background(), "ATM-404", "Fixed")
	if !errors.Is(err, workable.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
