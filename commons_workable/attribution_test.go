package workable

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestCRUD_AttributionRoundTrip proves the new created_by / assigned_to
// columns survive a real SQLite Create -> GetByID -> Update -> List
// round-trip with exact values (not just struct equality), and that an
// item left with empty attribution reads back as "" (the DEFAULT).
func TestCRUD_AttributionRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := NewRepo(newTestStore(t))

	in := Item{
		AtmID:           "ATM-900",
		Type:            "Task",
		Status:          "Queued",
		Severity:        "Low",
		Title:           "Attribution round-trip",
		CurrentLocation: "Issues",
		CreatedBy:       "@alice",
		AssignedTo:      "@bob",
	}
	if err := repo.Create(ctx, in); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := repo.GetByID(ctx, "ATM-900", "Issues")
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetByID() = nil, want item")
	}
	if got.CreatedBy != "@alice" {
		t.Fatalf("CreatedBy = %q, want @alice", got.CreatedBy)
	}
	if got.AssignedTo != "@bob" {
		t.Fatalf("AssignedTo = %q, want @bob", got.AssignedTo)
	}

	// Update both attribution fields and confirm via List.
	in.AssignedTo = "Claude"
	in.CreatedBy = "@carol"
	if err := repo.Update(ctx, in); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	items, err := repo.List(ctx, "Issues")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("List() = %d items, want 1", len(items))
	}
	if items[0].CreatedBy != "@carol" || items[0].AssignedTo != "Claude" {
		t.Fatalf("after Update via List, attribution = (%q,%q), want (@carol,Claude)",
			items[0].CreatedBy, items[0].AssignedTo)
	}

	// An item created WITHOUT attribution must read back as "".
	bare := Item{AtmID: "ATM-901", Type: "Bug", Status: "Queued", CurrentLocation: "Issues"}
	if err := repo.Create(ctx, bare); err != nil {
		t.Fatalf("Create(bare) error = %v", err)
	}
	gotBare, err := repo.GetByID(ctx, "ATM-901", "Issues")
	if err != nil {
		t.Fatalf("GetByID(bare) error = %v", err)
	}
	if gotBare.CreatedBy != "" || gotBare.AssignedTo != "" {
		t.Fatalf("bare item attribution = (%q,%q), want both empty",
			gotBare.CreatedBy, gotBare.AssignedTo)
	}
}

// TestOpen_MigratesLegacyDB proves the forward-migration: a DB created
// under the PRE-attribution `items` schema (no created_by/assigned_to
// columns) opens cleanly via Open(), gains the two columns, preserves the
// legacy row's data, and reads the new columns back as "".
func TestOpen_MigratesLegacyDB(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")

	// Hand-build the legacy schema (verbatim pre-attribution columns) and
	// insert a row WITHOUT created_by/assigned_to.
	legacyDDL := `
CREATE TABLE items (
    atm_id           TEXT NOT NULL,
    type             TEXT,
    status           TEXT,
    severity         TEXT,
    title            TEXT,
    description      TEXT,
    forensic_anchor  TEXT,
    closure_criteria TEXT,
    composes_with    TEXT,
    current_location TEXT DEFAULT 'Issues',
    body_md          TEXT,
    created_at       TEXT,
    last_modified    TEXT,
    PRIMARY KEY (atm_id, current_location)
);`
	{
		raw, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatalf("raw open: %v", err)
		}
		raw.SetMaxOpenConns(1)
		if _, err := raw.Exec(legacyDDL); err != nil {
			t.Fatalf("legacy DDL: %v", err)
		}
		// Populate every legacy column (the production scan targets are
		// non-nullable Go strings, so NULLs would fail Scan independently of
		// the migration under test).
		if _, err := raw.Exec(
			`INSERT INTO items (atm_id, type, status, severity, title, description,
			    forensic_anchor, closure_criteria, composes_with, current_location,
			    body_md, created_at, last_modified)
			 VALUES ('ATM-1', 'Bug', 'In progress', 'High', 'Legacy row', 'legacy desc',
			    '', '', '', 'Issues', '', '2026-05-30', '2026-05-30')`); err != nil {
			t.Fatalf("legacy insert: %v", err)
		}
		// Confirm the legacy table genuinely lacks the new columns.
		var n int
		if err := raw.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('items') WHERE name IN ('created_by','assigned_to')`).
			Scan(&n); err != nil {
			t.Fatalf("legacy pragma: %v", err)
		}
		if n != 0 {
			t.Fatalf("legacy DB unexpectedly already had %d attribution columns", n)
		}
		if err := raw.Close(); err != nil {
			t.Fatalf("raw close: %v", err)
		}
	}

	// Open() via the production code path must migrate in-place.
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open(legacy) error = %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// Both columns now exist.
	var n int
	if err := s.DB().QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('items') WHERE name IN ('created_by','assigned_to')`).
		Scan(&n); err != nil {
		t.Fatalf("post-migrate pragma: %v", err)
	}
	if n != 2 {
		t.Fatalf("post-migrate column count = %d, want 2", n)
	}

	// The legacy row's data is intact and the new columns default to "".
	repo := NewRepo(s)
	got, err := repo.GetByID(ctx, "ATM-1", "Issues")
	if err != nil {
		t.Fatalf("GetByID(legacy) error = %v", err)
	}
	if got == nil {
		t.Fatal("legacy row vanished after migration")
	}
	if got.Title != "Legacy row" || got.Status != "In progress" || got.Severity != "High" {
		t.Fatalf("legacy data not preserved: %+v", *got)
	}
	if got.CreatedBy != "" || got.AssignedTo != "" {
		t.Fatalf("migrated columns = (%q,%q), want both empty",
			got.CreatedBy, got.AssignedTo)
	}

	// Idempotency: re-Open must not error (columns already present).
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("re-Open(legacy) error = %v", err)
	}
	if err := s2.Close(); err != nil {
		t.Fatalf("re-Close error = %v", err)
	}
}

// TestDiff_AttributionFieldChanged proves the change-feed emits an
// item.field.changed Change for created_by and assigned_to changes, with
// truth-checked Old->New values.
func TestDiff_AttributionFieldChanged(t *testing.T) {
	prev := []Item{{
		AtmID: "ATM-1", Status: "In progress", CurrentLocation: "Issues",
		CreatedBy: "@alice", AssignedTo: "@bob",
	}}
	curr := []Item{{
		AtmID: "ATM-1", Status: "In progress", CurrentLocation: "Issues",
		CreatedBy: "Claude", AssignedTo: "@carol",
	}}

	changes := Diff(prev, curr)

	// Collect the field.changed Changes by field name.
	byField := map[string]Change{}
	for _, ch := range changes {
		if ch.Kind == KindFieldChanged {
			byField[ch.Field] = ch
		}
	}

	cb, ok := byField["created_by"]
	if !ok {
		t.Fatalf("no created_by field.changed emitted; changes = %+v", changes)
	}
	if cb.Old != "@alice" || cb.New != "Claude" {
		t.Fatalf("created_by change = (%q->%q), want (@alice->Claude)", cb.Old, cb.New)
	}
	if cb.AtmID != "ATM-1" || cb.Location != "Issues" {
		t.Fatalf("created_by change key wrong: %+v", cb)
	}

	at, ok := byField["assigned_to"]
	if !ok {
		t.Fatalf("no assigned_to field.changed emitted; changes = %+v", changes)
	}
	if at.Old != "@bob" || at.New != "@carol" {
		t.Fatalf("assigned_to change = (%q->%q), want (@bob->@carol)", at.Old, at.New)
	}
}

// TestDiff_NoAttributionChangeNoChange proves the negative: identical
// attribution emits NO field.changed Change for created_by/assigned_to.
func TestDiff_NoAttributionChangeNoChange(t *testing.T) {
	prev := []Item{{
		AtmID: "ATM-1", Status: "In progress", CurrentLocation: "Issues",
		CreatedBy: "@alice", AssignedTo: "@bob",
	}}
	curr := []Item{{
		AtmID: "ATM-1", Status: "In progress", CurrentLocation: "Issues",
		CreatedBy: "@alice", AssignedTo: "@bob",
	}}

	for _, ch := range Diff(prev, curr) {
		if ch.Kind == KindFieldChanged && (ch.Field == "created_by" || ch.Field == "assigned_to") {
			t.Fatalf("unexpected attribution change emitted for identical items: %+v", ch)
		}
	}
}

// attributionTracker carries the MD attribution fields on a real item
// block. The second item deliberately OMITS them (absent -> "").
const attributionTracker = `# Issues

## §GL — [ATM-238] Netflix login failure on D3

**Status:** Operator-blocked
**Type:** Bug
**Severity:** Critical
**Created-By:** @milos85vasic
**Assigned-To:** Claude

The Netflix login flow returns 500 on device D3.

## SYS — [ATM-101] Disk pressure alerting

**Status:** In progress
**Type:** Feature
**Severity:** High

No attribution fields on this one.
`

// TestParseTracker_ReadsAttributionFields proves the parser reads
// **Created-By:** / **Assigned-To:**, and that an item omitting them
// parses with empty attribution.
func TestParseTracker_ReadsAttributionFields(t *testing.T) {
	items, err := ParseTracker(attributionTracker, "Issues")
	if err != nil {
		t.Fatalf("ParseTracker() error = %v", err)
	}

	byID := map[string]Item{}
	for _, it := range items {
		byID[it.AtmID] = it
	}

	gl, ok := byID["ATM-238"]
	if !ok {
		t.Fatalf("ATM-238 not parsed; ids %v", idsOf(items))
	}
	if gl.CreatedBy != "@milos85vasic" {
		t.Fatalf("ATM-238 CreatedBy = %q, want @milos85vasic", gl.CreatedBy)
	}
	if gl.AssignedTo != "Claude" {
		t.Fatalf("ATM-238 AssignedTo = %q, want Claude", gl.AssignedTo)
	}

	sys, ok := byID["ATM-101"]
	if !ok {
		t.Fatalf("ATM-101 not parsed; ids %v", idsOf(items))
	}
	if sys.CreatedBy != "" || sys.AssignedTo != "" {
		t.Fatalf("ATM-101 attribution = (%q,%q), want both empty (absent fields)",
			sys.CreatedBy, sys.AssignedTo)
	}
}

// TestParseTracker_LegacyFixtureUnchanged proves the existing
// representativeTracker fixture (which has NO attribution fields) still
// parses to exactly its 3 items, each with empty attribution — the
// round-trip is unchanged by the new feature.
func TestParseTracker_LegacyFixtureUnchanged(t *testing.T) {
	items, err := ParseTracker(representativeTracker, "Issues")
	if err != nil {
		t.Fatalf("ParseTracker() error = %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("legacy fixture parsed %d items, want 3: %v", len(items), idsOf(items))
	}
	for _, it := range items {
		if it.CreatedBy != "" || it.AssignedTo != "" {
			t.Fatalf("legacy fixture item %s gained attribution = (%q,%q), want empty",
				it.AtmID, it.CreatedBy, it.AssignedTo)
		}
	}
}
