package inbound_test

import (
	"testing"

	"github.com/vasic-digital/herald/pherald/internal/inbound"
)

// TestCommandRecognizer_TruthTable is the TIER 1 truth-table
// (docs/design/INTENT_RECOGNITION.md §2 + §6). It drives a table of plain-
// language messages → expected (action, fields), INCLUDING the conservative
// negatives that MUST return no-match (false matches are worse than a deferral
// to the LLM, so when in doubt the recognizer declines).
func TestCommandRecognizer_TruthTable(t *testing.T) {
	r := inbound.NewCommandRecognizer()

	cases := []struct {
		name       string
		message    string
		wantMatch  bool
		wantAction string
		wantFields map[string]string // subset asserted (only listed keys checked)
	}{
		// ---- item.update: close / resolve ----
		{
			name:       "close ATM-N",
			message:    "close ATM-123",
			wantMatch:  true,
			wantAction: "item.update",
			wantFields: map[string]string{"atm_id": "ATM-123", "status": "closed"},
		},
		{
			name:       "mark ATM-N fixed",
			message:    "please mark ATM-5 fixed",
			wantMatch:  true,
			wantAction: "item.update",
			wantFields: map[string]string{"atm_id": "ATM-5", "status": "closed"},
		},
		{
			name:       "ATM-N is done (state assertion closes)",
			message:    "ATM-7 is done now",
			wantMatch:  true,
			wantAction: "item.update",
			wantFields: map[string]string{"atm_id": "ATM-7", "status": "closed"},
		},
		// ---- item.update: set status ----
		{
			name:       "set ATM-N to in progress",
			message:    "set ATM-9 to in progress",
			wantMatch:  true,
			wantAction: "item.update",
			wantFields: map[string]string{"atm_id": "ATM-9", "status": "In progress"},
		},
		{
			name:       "ATM-N is blocked",
			message:    "ATM-9 is blocked on the API team",
			wantMatch:  true,
			wantAction: "item.update",
			wantFields: map[string]string{"atm_id": "ATM-9", "status": "Blocked"},
		},
		// ---- item.update: assign ----
		{
			name:       "assign ATM-N to @x",
			message:    "assign ATM-5 to @bob",
			wantMatch:  true,
			wantAction: "item.update",
			wantFields: map[string]string{"atm_id": "ATM-5", "assigned_to": "@bob"},
		},
		{
			name:       "give ATM-N to @x",
			message:    "give ATM-42 to alice please",
			wantMatch:  true,
			wantAction: "item.update",
			wantFields: map[string]string{"atm_id": "ATM-42", "assigned_to": "alice"},
		},
		// ---- issue.open ----
		{
			name:       "open a bug: <title>",
			message:    "open a bug: login button does nothing on Safari",
			wantMatch:  true,
			wantAction: "issue.open",
			wantFields: map[string]string{"type": "bug", "title": "login button does nothing on Safari"},
		},
		{
			name:       "create a task: <title>",
			message:    "create a task: migrate the cron to systemd timers",
			wantMatch:  true,
			wantAction: "issue.open",
			wantFields: map[string]string{"type": "task", "title": "migrate the cron to systemd timers"},
		},
		{
			name:       "new feature request: <title>",
			message:    "new feature request: dark mode for the dashboard",
			wantMatch:  true,
			wantAction: "issue.open",
			wantFields: map[string]string{"type": "feature", "title": "dark mode for the dashboard"},
		},
		// ---- investigation.start ----
		{
			name:       "investigate ATM-N",
			message:    "investigate ATM-7",
			wantMatch:  true,
			wantAction: "investigation.start",
			wantFields: map[string]string{"atm_id": "ATM-7"},
		},
		{
			name:       "look into ATM-N",
			message:    "can you look into ATM-7 when you get a sec",
			wantMatch:  true,
			wantAction: "investigation.start",
			wantFields: map[string]string{"atm_id": "ATM-7"},
		},
		// ---- reply: status query ----
		{
			name:       "status of ATM-N?",
			message:    "status of ATM-9?",
			wantMatch:  true,
			wantAction: "reply",
			wantFields: map[string]string{"atm_id": "ATM-9", "query": "status"},
		},
		{
			name:       "what's ATM-N?",
			message:    "what's the status of ATM-9?",
			wantMatch:  true,
			wantAction: "reply",
			wantFields: map[string]string{"atm_id": "ATM-9"},
		},

		// ---- CONSERVATIVE NEGATIVES — MUST return no-match ----
		{
			name:      "vague: hey can you look at the thing",
			message:   "hey can you look at the thing",
			wantMatch: false,
		},
		{
			name:      "vague: close it (no target)",
			message:   "ok go ahead and close it",
			wantMatch: false,
		},
		{
			name:      "conversational: thanks!",
			message:   "thanks, that's super helpful!",
			wantMatch: false,
		},
		{
			name:      "set to unknown status (declines, LLM handles)",
			message:   "set ATM-9 to marinating",
			wantMatch: false,
		},
		{
			name:      "open a bug with no title after colon",
			message:   "open a bug:",
			wantMatch: false,
		},
		{
			name:      "bare mention of an ATM id (no imperative)",
			message:   "I was reading ATM-9 earlier and it's interesting",
			wantMatch: false,
		},
		{
			name:      "empty message",
			message:   "   ",
			wantMatch: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			action, fields, matched := r.RecognizeCommand(tc.message)
			if matched != tc.wantMatch {
				t.Fatalf("RecognizeCommand(%q) matched=%v, want %v (action=%q fields=%v)",
					tc.message, matched, tc.wantMatch, action, fields)
			}
			if !tc.wantMatch {
				return // negatives: nothing more to assert
			}
			if action != tc.wantAction {
				t.Errorf("RecognizeCommand(%q) action=%q, want %q", tc.message, action, tc.wantAction)
			}
			for k, want := range tc.wantFields {
				if got := fields[k]; got != want {
					t.Errorf("RecognizeCommand(%q) fields[%q]=%q, want %q (full=%v)",
						tc.message, k, got, want, fields)
				}
			}
		})
	}
}

// TestCommandRecognizer_CaseInsensitive proves phrasing tolerance: the same
// command in different casing recognizes identically.
func TestCommandRecognizer_CaseInsensitive(t *testing.T) {
	r := inbound.NewCommandRecognizer()
	for _, msg := range []string{"CLOSE ATM-9", "Close atm-9", "close ATM-9"} {
		action, fields, ok := r.RecognizeCommand(msg)
		if !ok || action != "item.update" || fields["atm_id"] != "ATM-9" || fields["status"] != "closed" {
			t.Errorf("RecognizeCommand(%q) = (%q, %v, %v); want item.update ATM-9 closed", msg, action, fields, ok)
		}
	}
}
