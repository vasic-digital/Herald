package workable

import (
	"reflect"
	"testing"
)

func item(id, status, title, sev, typ, body, desc string) Item {
	return Item{
		AtmID:           id,
		Type:            typ,
		Status:          status,
		Severity:        sev,
		Title:           title,
		Description:     desc,
		CurrentLocation: "Issues",
		BodyMd:          body,
	}
}

func TestDiff_Created(t *testing.T) {
	curr := []Item{item("ATM-1", "Queued", "t", "Low", "Bug", "b", "d")}
	got := Diff(nil, curr)
	want := []Change{{AtmID: "ATM-1", Location: "Issues", Kind: "item.created"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Diff created = %+v, want %+v", got, want)
	}
}

func TestDiff_Deleted(t *testing.T) {
	prev := []Item{item("ATM-1", "Queued", "t", "Low", "Bug", "b", "d")}
	got := Diff(prev, nil)
	want := []Change{{AtmID: "ATM-1", Location: "Issues", Kind: "item.deleted"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Diff deleted = %+v, want %+v", got, want)
	}
}

func TestDiff_StatusChanged(t *testing.T) {
	prev := []Item{item("ATM-1", "Queued", "t", "Low", "Bug", "b", "d")}
	curr := []Item{item("ATM-1", "In progress", "t", "Low", "Bug", "b", "d")}
	got := Diff(prev, curr)
	want := []Change{{
		AtmID: "ATM-1", Location: "Issues",
		Kind: "item.status.changed", Field: "status",
		Old: "Queued", New: "In progress",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Diff status = %+v, want %+v", got, want)
	}
}

func TestDiff_FieldChanged_TitleSeverityType(t *testing.T) {
	prev := []Item{item("ATM-1", "Queued", "old-title", "Low", "Bug", "b", "d")}
	curr := []Item{item("ATM-1", "Queued", "new-title", "High", "Feature", "b", "d")}
	got := Diff(prev, curr)
	// Sorted by field: severity, title, type.
	want := []Change{
		{AtmID: "ATM-1", Location: "Issues", Kind: "item.field.changed", Field: "severity", Old: "Low", New: "High"},
		{AtmID: "ATM-1", Location: "Issues", Kind: "item.field.changed", Field: "title", Old: "old-title", New: "new-title"},
		{AtmID: "ATM-1", Location: "Issues", Kind: "item.field.changed", Field: "type", Old: "Bug", New: "Feature"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Diff field = %+v, want %+v", got, want)
	}
}

func TestDiff_ContentUpdated(t *testing.T) {
	prev := []Item{item("ATM-1", "Queued", "t", "Low", "Bug", "old-body", "old-desc")}
	curr := []Item{item("ATM-1", "Queued", "t", "Low", "Bug", "new-body", "new-desc")}
	got := Diff(prev, curr)
	want := []Change{
		{AtmID: "ATM-1", Location: "Issues", Kind: "item.content.updated", Field: "body_md", Old: "old-body", New: "new-body"},
		{AtmID: "ATM-1", Location: "Issues", Kind: "item.content.updated", Field: "description", Old: "old-desc", New: "new-desc"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Diff content = %+v, want %+v", got, want)
	}
}

func TestDiff_DeterministicOrderingAcrossItems(t *testing.T) {
	prev := []Item{
		item("ATM-2", "Queued", "t", "Low", "Bug", "b", "d"),
		item("ATM-1", "Queued", "t", "Low", "Bug", "b", "d"),
	}
	curr := []Item{
		item("ATM-2", "In progress", "t", "Low", "Bug", "b", "d"),
		item("ATM-1", "In progress", "t", "Low", "Bug", "b", "d"),
	}
	got := Diff(prev, curr)
	if len(got) != 2 {
		t.Fatalf("expected 2 changes, got %d: %+v", len(got), got)
	}
	if got[0].AtmID != "ATM-1" || got[1].AtmID != "ATM-2" {
		t.Fatalf("changes not sorted by atm_id: %+v", got)
	}
}

func TestDiff_NoChange(t *testing.T) {
	same := []Item{item("ATM-1", "Queued", "t", "Low", "Bug", "b", "d")}
	if got := Diff(same, same); len(got) != 0 {
		t.Fatalf("Diff(no change) = %+v, want empty", got)
	}
}

// itemAt builds an Item like item() but at an explicit location.
func itemAt(id, status, location string) Item {
	it := item(id, status, "t", "Low", "Bug", "b", "d")
	it.CurrentLocation = location
	return it
}

// TestDiff_Relocated_IssuesToFixed asserts the most important lifecycle
// event — an item moving Issues -> Fixed (it "got Fixed") — emits exactly
// ONE item.relocated Change carrying the from/to locations, and does NOT
// degrade into a spurious item.deleted (Issues) + item.created (Fixed).
func TestDiff_Relocated_IssuesToFixed(t *testing.T) {
	prev := []Item{itemAt("ATM-X", "In progress", "Issues")}
	curr := []Item{itemAt("ATM-X", "Fixed (→ Fixed.md)", "Fixed")}
	got := Diff(prev, curr)

	// No spurious delete/create for the relocated atm_id.
	for _, c := range got {
		if c.AtmID == "ATM-X" && (c.Kind == KindDeleted || c.Kind == KindCreated) {
			t.Fatalf("relocation emitted spurious %s for ATM-X: %+v", c.Kind, got)
		}
	}

	var relocations []Change
	for _, c := range got {
		if c.Kind == KindRelocated {
			relocations = append(relocations, c)
		}
	}
	if len(relocations) != 1 {
		t.Fatalf("expected exactly 1 item.relocated, got %d: %+v", len(relocations), got)
	}
	r := relocations[0]
	if r.AtmID != "ATM-X" {
		t.Fatalf("relocation AtmID = %q, want ATM-X", r.AtmID)
	}
	if r.Field != "current_location" {
		t.Fatalf("relocation Field = %q, want current_location", r.Field)
	}
	if r.Old != "Issues" || r.New != "Fixed" {
		t.Fatalf("relocation Old/New = %q/%q, want Issues/Fixed", r.Old, r.New)
	}
}

// TestDiff_Relocated_AlsoSurfacesStatusChange asserts that when an item
// relocates AND its status differs, the status change is surfaced too
// (alongside the single relocation), so subscribers learn the new status.
func TestDiff_Relocated_AlsoSurfacesStatusChange(t *testing.T) {
	prev := []Item{itemAt("ATM-X", "In progress", "Issues")}
	curr := []Item{itemAt("ATM-X", "Fixed (→ Fixed.md)", "Fixed")}
	got := Diff(prev, curr)

	var sawStatus bool
	for _, c := range got {
		if c.Kind == KindStatusChanged {
			sawStatus = true
			if c.Old != "In progress" || c.New != "Fixed (→ Fixed.md)" {
				t.Fatalf("status change Old/New = %q/%q", c.Old, c.New)
			}
		}
	}
	if !sawStatus {
		t.Fatalf("relocation with status delta did not surface status.changed: %+v", got)
	}
}
