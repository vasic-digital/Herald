package workable

import (
	"strings"
	"testing"
)

// representativeTracker reproduces ATMOSphere's REAL tracker format: a
// section header (## A. ...), a §-prefixed CRITICAL item with an
// [ATM-NNN] id, a plain prefix item, and an item with no [ATM-NNN] id
// (id must be derived from the heading).
const representativeTracker = `# Issues

## A. Global blockers

Some prose describing the section.

## §GL CRITICAL — [ATM-238] Netflix login failure on D3

**Status:** Operator-blocked
**Type:** Bug
**Severity:** Critical

The Netflix login flow returns 500 on device D3.

## SYS — [ATM-101] Disk pressure alerting

**Status:** In progress
**Type:** Feature
**Severity:** High

Add alerting when disk usage crosses 90%.

## §UX — Tidy the onboarding copy

**Status:** Queued
**Type:** Task
**Severity:** Low

No ATM id on this one.
`

func TestParseTracker_RepresentativeItem(t *testing.T) {
	items, err := ParseTracker(representativeTracker, "Issues")
	if err != nil {
		t.Fatalf("ParseTracker() error = %v", err)
	}

	byID := map[string]Item{}
	for _, it := range items {
		byID[it.AtmID] = it
	}

	gl, ok := byID["ATM-238"]
	if !ok {
		t.Fatalf("ATM-238 not parsed; got ids %v", keys(byID))
	}
	if gl.Title != "Netflix login failure on D3" {
		t.Fatalf("ATM-238 title = %q", gl.Title)
	}
	if gl.Status != "Operator-blocked" {
		t.Fatalf("ATM-238 status = %q, want Operator-blocked", gl.Status)
	}
	if gl.Type != "Bug" {
		t.Fatalf("ATM-238 type = %q, want Bug", gl.Type)
	}
	if gl.Severity != "Critical" {
		t.Fatalf("ATM-238 severity = %q, want Critical", gl.Severity)
	}
	if gl.CurrentLocation != "Issues" {
		t.Fatalf("ATM-238 location = %q, want Issues", gl.CurrentLocation)
	}
	if !strings.Contains(gl.BodyMd, "returns 500 on device D3") {
		t.Fatalf("ATM-238 body_md missing prose: %q", gl.BodyMd)
	}
	// body_md must NOT bleed into the next heading.
	if strings.Contains(gl.BodyMd, "Disk pressure alerting") {
		t.Fatalf("ATM-238 body_md bled into next item: %q", gl.BodyMd)
	}
}

func TestParseTracker_PlainPrefixItem(t *testing.T) {
	items, err := ParseTracker(representativeTracker, "Issues")
	if err != nil {
		t.Fatalf("ParseTracker() error = %v", err)
	}
	var found bool
	for _, it := range items {
		if it.AtmID == "ATM-101" {
			found = true
			if it.Title != "Disk pressure alerting" {
				t.Fatalf("ATM-101 title = %q", it.Title)
			}
			if it.Status != "In progress" || it.Type != "Feature" || it.Severity != "High" {
				t.Fatalf("ATM-101 metadata wrong: %+v", it)
			}
		}
	}
	if !found {
		t.Fatal("ATM-101 not parsed")
	}
}

func TestParseTracker_SectionHeaderSkipped(t *testing.T) {
	items, err := ParseTracker(representativeTracker, "Issues")
	if err != nil {
		t.Fatalf("ParseTracker() error = %v", err)
	}
	// "## A. Global blockers" has no **Status:** block -> not an item.
	for _, it := range items {
		if strings.Contains(it.Title, "Global blockers") {
			t.Fatalf("section header was parsed as item: %+v", it)
		}
	}
}

func TestParseTracker_DerivesStableIDWhenNoBracket(t *testing.T) {
	items, err := ParseTracker(representativeTracker, "Issues")
	if err != nil {
		t.Fatalf("ParseTracker() error = %v", err)
	}
	// The §UX item has no [ATM-NNN]; it must still be captured with a
	// non-empty, stable derived id and its real title/metadata.
	var derived *Item
	for i := range items {
		if items[i].Title == "Tidy the onboarding copy" {
			derived = &items[i]
		}
	}
	if derived == nil {
		t.Fatalf("no-bracket item not parsed; ids %v", idsOf(items))
	}
	if derived.AtmID == "" {
		t.Fatal("derived id is empty")
	}
	if derived.Status != "Queued" || derived.Type != "Task" {
		t.Fatalf("no-bracket item metadata wrong: %+v", *derived)
	}

	// Determinism: parsing again yields the same derived id.
	items2, _ := ParseTracker(representativeTracker, "Issues")
	var id2 string
	for _, it := range items2 {
		if it.Title == "Tidy the onboarding copy" {
			id2 = it.AtmID
		}
	}
	if id2 != derived.AtmID {
		t.Fatalf("derived id not stable: %q vs %q", derived.AtmID, id2)
	}
}

func TestParseTracker_ItemCount(t *testing.T) {
	items, err := ParseTracker(representativeTracker, "Issues")
	if err != nil {
		t.Fatalf("ParseTracker() error = %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("parsed %d items, want 3: %v", len(items), idsOf(items))
	}
}

// fencedCodeTracker embeds a fenced code block whose sample contains a
// `## ` line and a **Status:** line. Neither must be parsed as a real
// item — they live inside ``` ... ``` and are documentation, not headings.
const fencedCodeTracker = "# Issues\n" +
	"\n" +
	"## SYS — [ATM-700] Real item outside the fence\n" +
	"\n" +
	"**Status:** In progress\n" +
	"**Type:** Bug\n" +
	"**Severity:** High\n" +
	"\n" +
	"Here is an example tracker entry an operator might paste:\n" +
	"\n" +
	"```\n" +
	"## §FAKE — [ATM-999] This is only a code sample\n" +
	"\n" +
	"**Status:** Queued\n" +
	"**Type:** Task\n" +
	"**Severity:** Low\n" +
	"```\n" +
	"\n" +
	"More prose after the fence.\n"

func TestParseTracker_FencedCodeNotParsedAsItem(t *testing.T) {
	items, err := ParseTracker(fencedCodeTracker, "Issues")
	if err != nil {
		t.Fatalf("ParseTracker() error = %v", err)
	}

	// The fenced ATM-999 sample MUST NOT become an item.
	for _, it := range items {
		if it.AtmID == "ATM-999" {
			t.Fatalf("phantom item parsed from inside fenced code block: %+v", it)
		}
		if strings.Contains(it.Title, "only a code sample") {
			t.Fatalf("fenced heading parsed as item title: %+v", it)
		}
	}

	// The real item outside the fence MUST still parse — and its own
	// metadata must NOT be corrupted by the fenced sample's **Status:** /
	// **Severity:** lines that fall inside its captured body.
	var found bool
	for _, it := range items {
		if it.AtmID == "ATM-700" {
			found = true
			if it.Status != "In progress" {
				t.Fatalf("ATM-700 status = %q, want In progress (fenced metadata leaked)", it.Status)
			}
			if it.Severity != "High" {
				t.Fatalf("ATM-700 severity = %q, want High (fenced metadata leaked)", it.Severity)
			}
			if it.Type != "Bug" {
				t.Fatalf("ATM-700 type = %q, want Bug (fenced metadata leaked)", it.Type)
			}
		}
	}
	if !found {
		t.Fatalf("real item ATM-700 not parsed; ids %v", idsOf(items))
	}
	if len(items) != 1 {
		t.Fatalf("parsed %d items, want exactly 1 (ATM-700): %v", len(items), idsOf(items))
	}
}

func keys(m map[string]Item) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func idsOf(items []Item) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.AtmID)
	}
	return out
}
