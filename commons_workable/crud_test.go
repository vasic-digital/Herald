package workable

import (
	"context"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "crud.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestCRUD_RoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	repo := NewRepo(s)

	in := Item{
		AtmID:           "ATM-238",
		Type:            "Bug",
		Status:          "Operator-blocked",
		Severity:        "Critical",
		Title:           "Netflix login failure on D3",
		Description:     "login flow 500s",
		ForensicAnchor:  "operator 2026-05-28",
		ClosureCriteria: "login succeeds on D3",
		ComposesWith:    `["ATM-100"]`,
		CurrentLocation: "Issues",
		BodyMd:          "## body",
		CreatedAt:       "2026-05-28",
		LastModified:    "2026-05-28",
	}

	if err := repo.Create(ctx, in); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := repo.GetByID(ctx, "ATM-238", "Issues")
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetByID() = nil, want item")
	}
	if *got != in {
		t.Fatalf("GetByID() = %+v, want %+v", *got, in)
	}

	// Absent row -> (nil, nil).
	missing, err := repo.GetByID(ctx, "ATM-999", "Issues")
	if err != nil {
		t.Fatalf("GetByID(absent) error = %v", err)
	}
	if missing != nil {
		t.Fatalf("GetByID(absent) = %+v, want nil", missing)
	}

	// Update.
	in.Status = "In progress"
	in.Title = "Netflix login failure on D3 (triaging)"
	if err := repo.Update(ctx, in); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	got, err = repo.GetByID(ctx, "ATM-238", "Issues")
	if err != nil {
		t.Fatalf("GetByID(after update) error = %v", err)
	}
	if got.Status != "In progress" || got.Title != in.Title {
		t.Fatalf("after Update got = %+v", *got)
	}

	// List.
	items, err := repo.List(ctx, "Issues")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 1 || items[0].AtmID != "ATM-238" {
		t.Fatalf("List() = %+v", items)
	}

	// Delete.
	if err := repo.Delete(ctx, "ATM-238", "Issues"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	got, err = repo.GetByID(ctx, "ATM-238", "Issues")
	if err != nil {
		t.Fatalf("GetByID(after delete) error = %v", err)
	}
	if got != nil {
		t.Fatalf("GetByID(after delete) = %+v, want nil", *got)
	}
}

func TestCreate_RejectsUnknownStatus(t *testing.T) {
	ctx := context.Background()
	repo := NewRepo(newTestStore(t))

	err := repo.Create(ctx, Item{
		AtmID:           "ATM-1",
		Type:            "Task",
		Status:          "Totally-Bogus-Status",
		CurrentLocation: "Issues",
	})
	if err == nil {
		t.Fatal("Create() with garbage status: expected error, got nil")
	}
}

func TestCreate_RejectsEmptyStatus(t *testing.T) {
	ctx := context.Background()
	repo := NewRepo(newTestStore(t))

	if err := repo.Create(ctx, Item{AtmID: "ATM-2", Status: "", CurrentLocation: "Issues"}); err == nil {
		t.Fatal("Create() with empty status: expected error, got nil")
	}
}

func TestUpdate_LoudOnMissing(t *testing.T) {
	ctx := context.Background()
	repo := NewRepo(newTestStore(t))

	err := repo.Update(ctx, Item{AtmID: "ATM-404", Status: "Queued", CurrentLocation: "Issues"})
	if err == nil {
		t.Fatal("Update(missing) expected error, got nil")
	}
}

func TestDelete_LoudOnMissing(t *testing.T) {
	ctx := context.Background()
	repo := NewRepo(newTestStore(t))

	if err := repo.Delete(ctx, "ATM-404", "Issues"); err == nil {
		t.Fatal("Delete(missing) expected error, got nil")
	}
}
