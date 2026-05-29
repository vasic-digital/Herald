package workable

import "sort"

// Change is one per-property delta between a previous and current
// snapshot of the workable items. For lifecycle Kinds (created/deleted)
// Field/Old/New are empty; for the per-property Kinds they carry the
// changed field name and its Old->New values.
type Change struct {
	AtmID    string
	Location string
	Kind     string // see the closed Kind set below
	Field    string
	Old      string
	New      string
}

// Change Kinds.
const (
	KindCreated        = "item.created"
	KindDeleted        = "item.deleted"
	KindStatusChanged  = "item.status.changed"
	KindFieldChanged   = "item.field.changed"
	KindContentUpdated = "item.content.updated"
)

// Diff computes the deterministic, per-property change set transforming
// prev into curr. Items are keyed on their composite (atm_id,
// current_location):
//
//   - present in curr but not prev      -> item.created
//   - present in prev but not curr      -> item.deleted
//   - status differs                    -> item.status.changed
//   - title/severity/type differ        -> one item.field.changed each
//   - body_md/description differ        -> one item.content.updated each
//
// Output is sorted by atm_id, then by Kind-group (created/deleted first,
// then the property changes), then by field name — fully deterministic.
func Diff(prev, curr []Item) []Change {
	prevByKey := indexByKey(prev)
	currByKey := indexByKey(curr)

	var changes []Change

	// Deletions: in prev, absent from curr.
	for key, p := range prevByKey {
		if _, ok := currByKey[key]; !ok {
			changes = append(changes, Change{
				AtmID: p.AtmID, Location: p.CurrentLocation, Kind: KindDeleted,
			})
		}
	}

	for key, c := range currByKey {
		p, existed := prevByKey[key]
		if !existed {
			changes = append(changes, Change{
				AtmID: c.AtmID, Location: c.CurrentLocation, Kind: KindCreated,
			})
			continue
		}
		changes = append(changes, propertyChanges(p, c)...)
	}

	sort.SliceStable(changes, func(i, j int) bool {
		a, b := changes[i], changes[j]
		if a.AtmID != b.AtmID {
			return a.AtmID < b.AtmID
		}
		if a.Location != b.Location {
			return a.Location < b.Location
		}
		if rank(a.Kind) != rank(b.Kind) {
			return rank(a.Kind) < rank(b.Kind)
		}
		return a.Field < b.Field
	})

	return changes
}

// propertyChanges emits one Change per differing tracked property.
func propertyChanges(p, c Item) []Change {
	var out []Change
	loc := c.CurrentLocation

	if p.Status != c.Status {
		out = append(out, Change{
			AtmID: c.AtmID, Location: loc, Kind: KindStatusChanged,
			Field: "status", Old: p.Status, New: c.Status,
		})
	}

	type fieldDelta struct{ field, old, new string }
	fields := []fieldDelta{
		{"severity", p.Severity, c.Severity},
		{"title", p.Title, c.Title},
		{"type", p.Type, c.Type},
	}
	for _, f := range fields {
		if f.old != f.new {
			out = append(out, Change{
				AtmID: c.AtmID, Location: loc, Kind: KindFieldChanged,
				Field: f.field, Old: f.old, New: f.new,
			})
		}
	}

	content := []fieldDelta{
		{"body_md", p.BodyMd, c.BodyMd},
		{"description", p.Description, c.Description},
	}
	for _, f := range content {
		if f.old != f.new {
			out = append(out, Change{
				AtmID: c.AtmID, Location: loc, Kind: KindContentUpdated,
				Field: f.field, Old: f.old, New: f.new,
			})
		}
	}

	return out
}

type itemKey struct {
	atmID    string
	location string
}

func indexByKey(items []Item) map[itemKey]Item {
	m := make(map[itemKey]Item, len(items))
	for _, it := range items {
		m[itemKey{it.AtmID, it.CurrentLocation}] = it
	}
	return m
}

// rank groups Kinds for stable secondary ordering within one item.
func rank(kind string) int {
	switch kind {
	case KindCreated:
		return 0
	case KindDeleted:
		return 1
	case KindStatusChanged:
		return 2
	case KindFieldChanged:
		return 3
	case KindContentUpdated:
		return 4
	default:
		return 5
	}
}
