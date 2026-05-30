package workable

import (
	"context"
	"path/filepath"
	"testing"
)

// TestOpen_CreatesSchemaIdempotently asserts Open creates the three
// canonical tables (items / item_history / meta) and is safe to call
// twice against the same file (CREATE TABLE IF NOT EXISTS).
func TestOpen_CreatesSchemaIdempotently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workable.db")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	wantTables := []string{"items", "item_history", "meta"}
	for _, tbl := range wantTables {
		var name string
		row := s.DB().QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl)
		if err := row.Scan(&name); err != nil {
			t.Fatalf("expected table %q to exist: %v", tbl, err)
		}
		if name != tbl {
			t.Fatalf("table name = %q, want %q", name, tbl)
		}
	}

	// WAL journal mode must be active.
	var jmode string
	if err := s.DB().QueryRow(`PRAGMA journal_mode`).Scan(&jmode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if jmode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", jmode)
	}

	// foreign_keys must be ON.
	var fk int
	if err := s.DB().QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Fatalf("foreign_keys = %d, want 1", fk)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Re-open the same file: schema application must be idempotent.
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("re-Open() error = %v", err)
	}
	if err := s2.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

// TestCrossConnectionVisibility proves the WATCHER premise the ATMOSphere
// integration depends on: "ATMOSphere writes, Herald reads" — i.e. a change
// committed by one Store connection becomes visible to a SEPARATE,
// independently-opened Store connection on the same db path via a fresh
// Repo.List. The current e2e writes+reads on a single pinned connection and
// therefore does NOT prove cross-connection (cross-process) WAL visibility.
//
// The reader is opened FIRST (simulating a long-lived watcher already
// running before the writer touches the file), then must see both an INSERT
// and a subsequent UPDATE that the writer commits afterwards.
func TestCrossConnectionVisibility(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "watch.db")

	// Reader opened first — the watcher is already live before any write.
	reader, err := Open(path)
	if err != nil {
		t.Fatalf("Open(reader) error = %v", err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	readerRepo := NewRepo(reader)

	// Sanity: reader sees an empty Issues location before the writer acts.
	if items, err := readerRepo.List(ctx, "Issues"); err != nil {
		t.Fatalf("reader List(pre-write) error = %v", err)
	} else if len(items) != 0 {
		t.Fatalf("reader List(pre-write) = %d items, want 0", len(items))
	}

	// Writer is a SEPARATE Open() on the SAME path = a distinct connection.
	writer, err := Open(path)
	if err != nil {
		t.Fatalf("Open(writer) error = %v", err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	writerRepo := NewRepo(writer)

	in := Item{
		AtmID:           "ATM-555",
		Type:            "Bug",
		Status:          "In progress",
		Severity:        "High",
		Title:           "Cross-connection visibility",
		CurrentLocation: "Issues",
	}
	if err := writerRepo.Create(ctx, in); err != nil {
		t.Fatalf("writer Create() error = %v", err)
	}

	// The reader (different connection) must now see the committed INSERT.
	items, err := readerRepo.List(ctx, "Issues")
	if err != nil {
		t.Fatalf("reader List(after insert) error = %v", err)
	}
	if len(items) != 1 || items[0].AtmID != "ATM-555" {
		t.Fatalf("reader did not see writer's INSERT across connections: %+v", items)
	}
	if items[0].Status != "In progress" {
		t.Fatalf("reader saw stale status = %q, want In progress", items[0].Status)
	}

	// And a subsequent UPDATE committed by the writer must also be visible.
	in.Status = "Fixed (→ Fixed.md)"
	if err := writerRepo.Update(ctx, in); err != nil {
		t.Fatalf("writer Update() error = %v", err)
	}
	items, err = readerRepo.List(ctx, "Issues")
	if err != nil {
		t.Fatalf("reader List(after update) error = %v", err)
	}
	if len(items) != 1 || items[0].Status != "Fixed (→ Fixed.md)" {
		t.Fatalf("reader did not see writer's UPDATE across connections: %+v", items)
	}
}
