package prefix

import "testing"

func TestGenerate(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// Rule C — single token.
		{"Herald", "Herald", "HRD"},
		{"Project", "Project", "PRT"},
		// Rule B — 2 tokens via CamelCase split.
		{"HeraldRouter", "HeraldRouter", "HRT"},
		{"HeraldRunner", "HeraldRunner", "HRN"},
		// Rule A — 3 tokens via CamelCase split.
		{"HeraldRouterCore", "HeraldRouterCore", "HRC"},
		// Hyphen-separated.
		{"my-project", "my-project", "MPR"},
		// Underscore-separated, 4 tokens — uses first 3.
		{"my_cool_test_project", "my_cool_test_project", "MCT"},
		// Slash-separated path-style.
		{"foo/bar/baz", "foo/bar/baz", "FBB"},
		// Empty name falls back to HRD (Herald's own prefix).
		{"empty", "", "HRD"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Generate(tc.in)
			if got != tc.want {
				t.Errorf("Generate(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestResolveNoCollision(t *testing.T) {
	got := Resolve("Herald", map[string]string{})
	if got != "HRD" {
		t.Errorf("Resolve(Herald, empty) = %q, want HRD", got)
	}
}

func TestResolveSameOwnerReturnsExisting(t *testing.T) {
	existing := map[string]string{"HRD": "Herald"}
	got := Resolve("Herald", existing)
	if got != "HRD" {
		t.Errorf("Resolve must return the same prefix when name owns it: got %q want HRD", got)
	}
}

func TestResolveCollisionTieBreak(t *testing.T) {
	existing := map[string]string{"HRD": "OtherProject"}
	got := Resolve("Herald", existing)
	if got == "HRD" {
		t.Errorf("Resolve must avoid collision; got HRD which is owned by OtherProject")
	}
	if len(got) != 3 {
		t.Errorf("Resolve must return 3-letter prefix, got %q", got)
	}
	if got[0] != 'H' || got[1] != 'R' {
		t.Errorf("collision-resolution should change only the third letter; got %q", got)
	}
}

func TestResolveDeterministic(t *testing.T) {
	existing := map[string]string{"HRD": "OtherProject"}
	a := Resolve("Herald", existing)
	b := Resolve("Herald", existing)
	if a != b {
		t.Errorf("Resolve must be deterministic across calls: %q vs %q", a, b)
	}
}

func TestCamelCaseEdgeCases(t *testing.T) {
	cases := map[string]string{
		"":              "HRD", // fallback
		"X":             "XXX", // single letter — no internal consonant, no last consonant
		"AB":            "ABB", // 2-char single token — Rule C with edge handling
	}
	for in, want := range cases {
		got := Generate(in)
		if got != want {
			t.Errorf("Generate(%q) = %q, want %q (edge case)", in, got, want)
		}
	}
}
