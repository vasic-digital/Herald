package workable

// §11.4.85 stress + chaos for the change-feed (Diff) + tracker parser.
//
//   STRESS:
//     - TestDiff_Stress_ThousandsOfItems: Diff over a large synthetic
//       Issues+Fixed tracker (thousands of items, every change Kind present)
//       MUST produce a deterministic, correctly-ordered, complete Change set —
//       run repeatedly to prove the output is byte-identical every time (the
//       map-iteration inside Diff is sorted away; this is the regression
//       tripwire that proves it stays sorted at scale).
//
//   CHAOS (input corruption / truncation):
//     - TestParseTracker_Chaos_CorruptedInput: ParseTracker fed a battery of
//       malformed / truncated / adversarial Markdown MUST NOT panic, MUST
//       return either an error or a well-formed (possibly partial) result —
//       never a half-built item or a goroutine crash.
//     - TestDiff_Chaos_AdversarialItems: Diff over items with empty / dup /
//       weird-unicode ids and mixed locations MUST NOT panic and MUST stay
//       internally consistent (sorted, no duplicate identical changes).
//
// QA-ANCHOR: HRD-156-CHANGEFEED-STRESS-CHAOS-20260530
//
// ANTI-BLUFF: assertions check real Change counts, real ordering, real
// recovered-from-panic state — no metadata-only PASS.

import (
	"fmt"
	"sort"
	"testing"
)

// buildSnapshots constructs a large prev/curr pair that, when Diff'd, exercises
// EVERY Kind at volume:
//
//   - 0..nCreate-1      : created   (in curr only)
//   - nCreate..2n-1     : deleted   (in prev only)
//   - 2n..3n-1          : status.changed  (status differs)
//   - 3n..4n-1          : field.changed   (severity+title+type differ => 3 each)
//   - 4n..5n-1          : content.updated (body_md+description differ => 2 each)
//   - 5n..6n-1          : relocated Issues->Fixed (+ status delta => 1 reloc +1 status)
//
// Returns prev, curr, and the exact expected total Change count.
func buildSnapshots(group int) (prev, curr []Item, wantChanges int) {
	mk := func(id, status, sev, title, typ, body, desc, loc string) Item {
		return Item{
			AtmID: id, Status: status, Severity: sev, Title: title, Type: typ,
			BodyMd: body, Description: desc, CurrentLocation: loc,
		}
	}

	// created
	for i := 0; i < group; i++ {
		id := fmt.Sprintf("ATM-C%05d", i)
		curr = append(curr, mk(id, "Queued", "Low", "t", "Bug", "b", "d", "Issues"))
		wantChanges++ // 1 created
	}
	// deleted
	for i := 0; i < group; i++ {
		id := fmt.Sprintf("ATM-D%05d", i)
		prev = append(prev, mk(id, "Queued", "Low", "t", "Bug", "b", "d", "Issues"))
		wantChanges++ // 1 deleted
	}
	// status.changed
	for i := 0; i < group; i++ {
		id := fmt.Sprintf("ATM-S%05d", i)
		prev = append(prev, mk(id, "Queued", "Low", "t", "Bug", "b", "d", "Issues"))
		curr = append(curr, mk(id, "In progress", "Low", "t", "Bug", "b", "d", "Issues"))
		wantChanges++ // 1 status.changed
	}
	// field.changed (severity+title+type all differ => 3 changes each)
	for i := 0; i < group; i++ {
		id := fmt.Sprintf("ATM-F%05d", i)
		prev = append(prev, mk(id, "Queued", "Low", "old", "Bug", "b", "d", "Issues"))
		curr = append(curr, mk(id, "Queued", "High", "new", "Feature", "b", "d", "Issues"))
		wantChanges += 3
	}
	// content.updated (body_md+description differ => 2 changes each)
	for i := 0; i < group; i++ {
		id := fmt.Sprintf("ATM-U%05d", i)
		prev = append(prev, mk(id, "Queued", "Low", "t", "Bug", "ob", "od", "Issues"))
		curr = append(curr, mk(id, "Queued", "Low", "t", "Bug", "nb", "nd", "Issues"))
		wantChanges += 2
	}
	// relocated Issues->Fixed with a status delta (=> 1 relocated + 1 status)
	for i := 0; i < group; i++ {
		id := fmt.Sprintf("ATM-R%05d", i)
		prev = append(prev, mk(id, "In progress", "Low", "t", "Bug", "b", "d", "Issues"))
		curr = append(curr, mk(id, "Fixed (→ Fixed.md)", "Low", "t", "Bug", "b", "d", "Fixed"))
		wantChanges += 2
	}
	return prev, curr, wantChanges
}

func TestDiff_Stress_ThousandsOfItems(t *testing.T) {
	const group = 500 // 6 groups => ~3000 items; field/content groups multiply changes
	prev, curr, want := buildSnapshots(group)
	t.Logf("stress input: %d prev items, %d curr items, %d expected changes", len(prev), len(curr), want)

	// First Diff: assert count + ordering + completeness.
	first := Diff(prev, curr)
	if len(first) != want {
		t.Fatalf("Diff produced %d changes, want %d (lost or spurious change at scale)", len(first), want)
	}

	// Ordering invariant: the documented sort is (AtmID, Location, rank(Kind),
	// Field). Assert the WHOLE slice is non-decreasing under that key — proves
	// the sorted-output contract holds at scale, not just for 2 items.
	less := func(a, b Change) bool {
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
	}
	for i := 1; i < len(first); i++ {
		if less(first[i], first[i-1]) {
			t.Fatalf("ordering violated at %d: %+v sorts before %+v", i, first[i], first[i-1])
		}
	}
	if !sort.SliceIsSorted(first, func(i, j int) bool { return less(first[i], first[j]) }) {
		t.Fatal("Diff output is not fully sorted at scale")
	}

	// DETERMINISM: re-running Diff on the SAME input must yield a byte-identical
	// sequence every time (the map-iteration nondeterminism inside Diff is the
	// real risk; this is the tripwire). Repeat several times.
	for rep := 0; rep < 5; rep++ {
		again := Diff(prev, curr)
		if len(again) != len(first) {
			t.Fatalf("rep %d: Diff length changed (%d vs %d) — nondeterministic", rep, len(again), len(first))
		}
		for i := range again {
			if again[i] != first[i] {
				t.Fatalf("rep %d: change[%d] differs across runs: %+v vs %+v — nondeterministic Diff", rep, i, again[i], first[i])
			}
		}
	}

	// Completeness by Kind: assert each Kind appears the expected number of times.
	byKind := map[string]int{}
	for _, c := range first {
		byKind[c.Kind]++
	}
	checks := map[string]int{
		KindCreated:        group,
		KindDeleted:        group,
		KindStatusChanged:  group + group, // status group + relocation status deltas
		KindFieldChanged:   group * 3,
		KindContentUpdated: group * 2,
		KindRelocated:      group,
	}
	for kind, exp := range checks {
		if byKind[kind] != exp {
			t.Fatalf("Kind %s appeared %d times, want %d", kind, byKind[kind], exp)
		}
	}
}

func TestParseTracker_Chaos_CorruptedInput(t *testing.T) {
	// Each input is adversarial: truncated, unbalanced fences, binary, huge,
	// deeply nested, control chars. NONE may panic; each must return cleanly.
	cases := map[string]string{
		"empty":                  "",
		"only_whitespace":        "   \n\t\n   ",
		"heading_no_body":        "## SYS — [ATM-1] dangling",
		"truncated_mid_heading":  "## SYS — [ATM-",
		"unterminated_fence":     "## SYS — [ATM-2] x\n**Status:** Open\n```go\nfunc(){",
		"fence_only":             "```",
		"nested_fences":          "```\n~~~\n## not a heading [ATM-9]\n~~~\n```",
		"status_inside_fence":    "## A. section\n```\n**Status:** Open\n```",
		"bracket_no_close":       "## SYS — [ATM-3 missing close\n**Status:** Open",
		"multiple_brackets":      "## SYS — [ATM-4] [ATM-5] [ATM-6]\n**Status:** Open",
		"crlf_line_endings":      "## SYS — [ATM-7] x\r\n**Status:** Open\r\n",
		"null_bytes":             "## SYS — [ATM-8]\x00 x\n**Status:**\x00 Open",
		"unicode_heading":        "## 🔥 — [ATM-9] café façade ünîcødé\n**Status:** Open",
		"giant_single_line":      "## SYS — [ATM-10] " + repeat("z", 100000) + "\n**Status:** Open",
		"many_blank_headings":    repeat("## \n", 5000),
		"status_without_heading": "**Status:** Open\nno heading at all",
		"deeply_indented":        repeat(" ", 4000) + "## SYS — [ATM-11]\n" + repeat(" ", 4000) + "**Status:** Open",
	}

	for name, md := range cases {
		t.Run(name, func(t *testing.T) {
			// Wrap in a recover so a panic is reported as a FAIL with the input,
			// not a test-binary crash that hides which input broke it.
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("ParseTracker PANICKED on %q input: %v", name, r)
				}
			}()
			items, err := ParseTracker(md, "Issues")
			// Contract: either a clean error OR a well-formed slice. Never both
			// a nil error and a malformed (empty-AtmID) item.
			if err != nil {
				return // a returned error is an acceptable outcome
			}
			for i, it := range items {
				if it.AtmID == "" {
					t.Fatalf("ParseTracker returned an item with empty AtmID at %d for input %q: %+v", i, name, it)
				}
				if it.CurrentLocation != "Issues" {
					t.Fatalf("ParseTracker mis-tagged location %q at %d for input %q", it.CurrentLocation, i, name)
				}
			}
		})
	}
}

func TestParseTracker_Chaos_RoundTripStability(t *testing.T) {
	// A valid-but-large tracker parsed twice must yield identical items — a
	// stability check that corruption-tolerance didn't introduce nondeterminism.
	var b []byte
	for i := 0; i < 1000; i++ {
		b = append(b, []byte(fmt.Sprintf("## SYS — [ATM-%05d] item %d\n**Status:** Open\n**Type:** Bug\n**Severity:** Low\n\nbody %d\n\n", i, i, i))...)
	}
	md := string(b)
	a, err := ParseTracker(md, "Issues")
	if err != nil {
		t.Fatalf("ParseTracker on valid large tracker: %v", err)
	}
	if len(a) != 1000 {
		t.Fatalf("parsed %d items, want 1000", len(a))
	}
	c, err := ParseTracker(md, "Issues")
	if err != nil {
		t.Fatalf("second ParseTracker: %v", err)
	}
	for i := range a {
		if a[i] != c[i] {
			t.Fatalf("item[%d] differs across parses: %+v vs %+v", i, a[i], c[i])
		}
	}
}

func TestDiff_Chaos_AdversarialItems(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Diff PANICKED on adversarial items: %v", r)
		}
	}()
	mk := func(id, status, loc string) Item {
		return Item{AtmID: id, Status: status, CurrentLocation: loc, Type: "Bug", Severity: "Low", Title: "t", BodyMd: "b", Description: "d"}
	}
	// Empty ids, unicode ids, same id at multiple locations (ambiguous — must
	// NOT be treated as a relocation), duplicate identical items.
	prev := []Item{
		mk("", "Queued", "Issues"),
		mk("ATM-ünî", "Queued", "Issues"),
		mk("ATM-MULTI", "Queued", "Issues"),
		mk("ATM-MULTI", "Queued", "Fixed"),
		mk("ATM-DUP", "Queued", "Issues"),
		mk("ATM-DUP", "Queued", "Issues"), // exact dup key — collapses in index
		mk("ATM-SAME", "Queued", "Issues"),
	}
	curr := []Item{
		mk("", "In progress", "Issues"),
		mk("ATM-ünî", "In progress", "Issues"),
		mk("ATM-MULTI", "In progress", "Issues"),
		mk("ATM-MULTI", "Queued", "Fixed"),
		mk("ATM-DUP", "Queued", "Issues"), // present unchanged in both => no change
		mk("ATM-SAME", "Queued", "Issues"),
		mk("ATM-NEW", "Queued", "Issues"),
	}
	changes := Diff(prev, curr)

	// Must be sorted + free of exact-duplicate changes (no double-emit).
	less := func(a, b Change) bool {
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
	}
	if !sort.SliceIsSorted(changes, func(i, j int) bool { return less(changes[i], changes[j]) }) {
		t.Fatalf("Diff over adversarial items is not sorted: %+v", changes)
	}
	seen := map[Change]struct{}{}
	for _, c := range changes {
		if _, dup := seen[c]; dup {
			t.Fatalf("Diff emitted a duplicate identical change: %+v", c)
		}
		seen[c] = struct{}{}
	}
	// Positive evidence: ATM-NEW was created, ATM-DUP unchanged (no spurious
	// change), the empty-id item's status change is still surfaced.
	var sawNew, sawEmptyStatus bool
	for _, c := range changes {
		if c.AtmID == "ATM-NEW" && c.Kind == KindCreated {
			sawNew = true
		}
		if c.AtmID == "" && c.Kind == KindStatusChanged {
			sawEmptyStatus = true
		}
		if c.AtmID == "ATM-DUP" {
			t.Fatalf("ATM-DUP (collapsed-dup, unchanged) produced a spurious change: %+v", c)
		}
		if c.AtmID == "ATM-SAME" {
			t.Fatalf("ATM-SAME (unchanged) produced a spurious change: %+v", c)
		}
	}
	if !sawNew {
		t.Fatalf("ATM-NEW create not surfaced: %+v", changes)
	}
	if !sawEmptyStatus {
		t.Fatalf("empty-id item status change not surfaced: %+v", changes)
	}
}

// repeat returns s repeated n times (test-local, avoids importing strings just
// for this).
func repeat(s string, n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		b = append(b, s...)
	}
	return string(b)
}
