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
	KindRelocated      = "item.relocated"
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

	// Relocation detection: an atm_id present at exactly one location in
	// prev and at exactly one DIFFERENT location in curr (and at no key
	// shared between the two snapshots) has moved — the most important
	// lifecycle event, "got Fixed" (Issues -> Fixed). Rather than degrade
	// into a spurious delete (old location) + create (new location), emit a
	// single item.relocated Change. relocated[atmID] holds (from, to).
	relocated := detectRelocations(prevByKey, currByKey)

	var changes []Change

	// Deletions: in prev, absent from curr — unless the atm_id relocated
	// (its disappearance at the old location is explained by the move).
	for key, p := range prevByKey {
		if _, ok := currByKey[key]; ok {
			continue
		}
		if rl, moved := relocated[p.AtmID]; moved && rl.from == p.CurrentLocation {
			continue
		}
		changes = append(changes, Change{
			AtmID: p.AtmID, Location: p.CurrentLocation, Kind: KindDeleted,
		})
	}

	for key, c := range currByKey {
		p, existed := prevByKey[key]
		if !existed {
			if rl, moved := relocated[c.AtmID]; moved && rl.to == c.CurrentLocation {
				// Emit the single relocation Change (keyed at the new
				// location) plus a status.changed if the status differs.
				changes = append(changes, Change{
					AtmID: c.AtmID, Location: c.CurrentLocation, Kind: KindRelocated,
					Field: "current_location", Old: rl.from, New: rl.to,
				})
				if rl.fromItem.Status != c.Status {
					changes = append(changes, Change{
						AtmID: c.AtmID, Location: c.CurrentLocation, Kind: KindStatusChanged,
						Field: "status", Old: rl.fromItem.Status, New: c.Status,
					})
				}
				continue
			}
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

// relocation records an atm_id moving from one location to another.
type relocation struct {
	from     string
	to       string
	fromItem Item // the prev-snapshot item, to surface a status delta
}

// detectRelocations finds atm_ids that disappear from exactly one location
// and reappear at exactly one DIFFERENT location across the snapshots. Only
// unambiguous 1->1 moves qualify: an atm_id that is present at the SAME
// location in both snapshots (a normal in-place update), or that appears at
// multiple locations in either snapshot, is not treated as a relocation.
func detectRelocations(prevByKey, currByKey map[itemKey]Item) map[string]relocation {
	type locSet struct {
		locs  []string
		items map[string]Item
	}
	prevLocs := map[string]*locSet{}
	currLocs := map[string]*locSet{}

	collect := func(src map[itemKey]Item, dst map[string]*locSet) {
		for k, it := range src {
			ls := dst[k.atmID]
			if ls == nil {
				ls = &locSet{items: map[string]Item{}}
				dst[k.atmID] = ls
			}
			ls.locs = append(ls.locs, k.location)
			ls.items[k.location] = it
		}
	}
	collect(prevByKey, prevLocs)
	collect(currByKey, currLocs)

	out := map[string]relocation{}
	for atmID, pls := range prevLocs {
		cls, ok := currLocs[atmID]
		if !ok {
			continue // gone entirely -> a real deletion
		}
		// Only unambiguous single-location-on-both-sides moves qualify.
		if len(pls.locs) != 1 || len(cls.locs) != 1 {
			continue
		}
		from, to := pls.locs[0], cls.locs[0]
		if from == to {
			continue // in-place update, handled by propertyChanges
		}
		out[atmID] = relocation{from: from, to: to, fromItem: pls.items[from]}
	}
	return out
}

// rank groups Kinds for stable secondary ordering within one item.
func rank(kind string) int {
	switch kind {
	case KindCreated:
		return 0
	case KindDeleted:
		return 1
	case KindRelocated:
		return 2
	case KindStatusChanged:
		return 3
	case KindFieldChanged:
		return 4
	case KindContentUpdated:
		return 5
	default:
		return 6
	}
}
