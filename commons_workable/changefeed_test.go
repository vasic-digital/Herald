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
