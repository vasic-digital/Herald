package workable

import (
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
