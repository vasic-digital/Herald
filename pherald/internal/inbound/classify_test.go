// Package inbound — classify_test.go: table-driven §32.6 classifier tests.
//
// Coverage matrix (per Wave 6.5 plan T1):
//   - Every §32.6 prefix row: bug / issue / task / implementation / impl /
//     investigation / investigate / query / question / q / ? / request /
//     help / /help / status / /status / continue / /continue / done /
//     resolve / reopen / override.
//   - Case-insensitivity: BUG: / Bug: / bug: all classify identically.
//   - Leading-whitespace tolerance: "  Bug: foo" still classifies as bug.
//   - Criticality keyword extraction: critical / urgent / high / important /
//     low / trivial, with precedence (critical > high > low).
//   - Natural-language fallthrough: "hey what's up" → query, confidence 0.0.
//   - Empty input: no panic; returns the default query/middle/0.0 triple.
//
// §107 anchor: every assertion is on the RETURN VALUE of Classify for a
// specific input. A no-op implementation returning Classification{}
// fails every prefix-hit case because the default Type is "" not the
// expected "bug"/"task"/etc., the default Confidence is 0.0 not 1.0,
// and the default Criticality is "" not "middle". M1 mutation (planted
// in Wave 6.5 T11) cannot pass this suite.
package inbound

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		wantType string
		wantCrit string
		wantConf float64
	}{
		// §32.6 prefix table — one case per documented row + aliases.
		{"bug-basic", "Bug: telemetry pipe down", "bug", "middle", 1.0},
		{"bug-lower", "bug: foo", "bug", "middle", 1.0},
		{"bug-upper", "BUG: X", "bug", "middle", 1.0},
		{"bug-leading-ws", "  Bug: leading whitespace", "bug", "middle", 1.0},
		{"issue-alias", "Issue: same thing", "bug", "middle", 1.0},
		{"task", "Task: refactor X", "task", "middle", 1.0},
		{"impl-alias-full", "Implementation: Y", "task", "middle", 1.0},
		{"impl-alias-short", "Impl: Z", "task", "middle", 1.0},
		{"investigation", "Investigation: reproduce flake", "investigation", "middle", 1.0},
		{"investigate-alias", "Investigate: same", "investigation", "middle", 1.0},
		{"query", "Query: what is X?", "query", "middle", 1.0},
		{"question-alias", "Question: explain Y", "query", "middle", 1.0},
		{"q-alias", "Q: short?", "query", "middle", 1.0},
		{"qmark", "? help me", "query", "middle", 1.0},
		{"request", "Request: explain Y", "query", "middle", 1.0},
		{"help", "Help:", "help_command", "middle", 1.0},
		{"help-slash", "/help", "help_command", "middle", 1.0},
		{"status", "Status:", "status_request", "middle", 1.0},
		{"status-slash", "/status", "status_request", "middle", 1.0},
		{"continue", "Continue:", "continuation_request", "middle", 1.0},
		{"continue-slash", "/continue", "continuation_request", "middle", 1.0},
		{"done", "Done: HRD-042", "closure", "middle", 1.0},
		{"resolve-alias", "Resolve: HRD-042", "closure", "middle", 1.0},
		{"reopen", "Reopen: HRD-042", "reopen", "middle", 1.0},
		{"override", "Override: HRD-042 type=task", "override", "middle", 1.0},

		// Natural-language fallthrough → query / confidence 0.0.
		{"plain-fallback-to-query", "hey what's up", "query", "middle", 0.0},
		{"empty-input", "", "query", "middle", 0.0},
		{"whitespace-only", "   \n\t   ", "query", "middle", 0.0},

		// Criticality extraction — combined with prefix.
		{"crit-urgent", "Bug: URGENT please", "bug", "critical", 1.0},
		{"crit-critical", "Bug: critical telemetry down", "bug", "critical", 1.0},
		{"crit-p0", "Bug: P0 outage", "bug", "critical", 1.0},
		{"crit-sev1", "Bug: sev-1 page", "bug", "critical", 1.0},
		{"crit-emergency", "Bug: emergency please look", "bug", "critical", 1.0},
		{"crit-high", "Bug: important to fix", "bug", "high", 1.0},
		{"crit-high-word", "Task: high priority cleanup", "task", "high", 1.0},
		{"crit-p1", "Bug: P1 follow-up", "bug", "high", 1.0},
		{"crit-low-word", "Bug: low priority typo", "bug", "low", 1.0},
		{"crit-trivial", "Bug: trivial typo", "bug", "low", 1.0},
		{"crit-p3", "Bug: P3 nit", "bug", "low", 1.0},

		// Precedence — critical wins over high wins over low.
		{"crit-mixed-prefer-critical", "Bug: critical though low priority", "bug", "critical", 1.0},
		{"crit-mixed-prefer-high-over-low", "Bug: important not trivial", "bug", "high", 1.0},

		// Criticality keywords on natural-language input (no prefix).
		{"crit-on-fallback", "this is urgent please", "query", "critical", 0.0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := Classify(tc.in)
			if c.Type != tc.wantType {
				t.Errorf("Classify(%q).Type = %q; want %q", tc.in, c.Type, tc.wantType)
			}
			if c.Criticality != tc.wantCrit {
				t.Errorf("Classify(%q).Criticality = %q; want %q", tc.in, c.Criticality, tc.wantCrit)
			}
			if c.Confidence != tc.wantConf {
				t.Errorf("Classify(%q).Confidence = %v; want %v", tc.in, c.Confidence, tc.wantConf)
			}
		})
	}
}
