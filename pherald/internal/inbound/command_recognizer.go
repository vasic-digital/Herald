// Package inbound — command_recognizer.go is TIER 1 of the three-tier intent
// resolution discipline (docs/design/INTENT_RECOGNITION.md §1/§2).
//
// CommandRecognizer is a CONSERVATIVE, deterministic matcher that maps clear
// natural-language commands to a structured inbound action WITHOUT an LLM
// round-trip — no "COMMAND:" prefix required. Users speak plain language; the
// recognizer only fast-paths a match it is confident about (a clear imperative
// verb + a resolvable target). Everything else returns NO-MATCH and falls
// through to Tier 2 (the LLM) — a false command-match is worse than a deferral,
// so when in doubt the recognizer declines (§2 contract: "False command-matches
// are worse than a deferral to the LLM").
//
// The recognizer returns (action, fields, matched). The action string is one of
// the EXISTING inbound actions ("item.update" / "issue.open" /
// "investigation.start" / "reply"); fields carries the per-action parameters
// (status / assigned_to / type / title / atm_id / query) the dispatcher uses to
// build the concrete Reply. matched=false means Tier 1 declined.
package inbound

import (
	"regexp"
	"strings"
)

// Recognized field keys produced by RecognizeCommand. The dispatcher reads
// these to build the concrete Reply payload (reply.go types).
const (
	cmdFieldStatus     = "status"      // item.update target status
	cmdFieldAssignedTo = "assigned_to" // item.update target assignee
	cmdFieldType       = "type"        // issue.open item type (bug|task|feature|...)
	cmdFieldTitle      = "title"       // issue.open title
	cmdFieldAtmID      = "atm_id"      // item id target (ATM-NNN)
	cmdFieldQuery      = "query"       // reply(query) — the item the question is about
)

// atmIDPattern matches an item id like "ATM-123" (case-insensitive on the
// prefix). The prefix is fixed to the ATMOSphere "ATM-" convention per the §2
// table; the numeric suffix is required (a bare "ATM" is not a target).
var atmIDPattern = regexp.MustCompile(`(?i)\b(ATM-\d+)\b`)

// CommandRecognizer is the Tier 1 deterministic command matcher. It carries no
// state today (all matching is pure on the message text) — it is a type so the
// dispatcher can hold it as a field and so future per-project command tables
// can be injected without changing call sites.
type CommandRecognizer struct{}

// NewCommandRecognizer returns the default conservative recognizer.
func NewCommandRecognizer() *CommandRecognizer { return &CommandRecognizer{} }

// RecognizeCommand maps a plain-language message to a structured action.
//
// Returns (action, fields, true) on a CONFIDENT match; ("", nil, false)
// otherwise. The matcher is case-insensitive and tolerant of surrounding
// phrasing, but it only commits when both a clear imperative AND (where the
// action needs one) a resolvable target are present. Ambiguous, vague, or
// purely conversational messages return no-match by design.
func (CommandRecognizer) RecognizeCommand(message string) (string, map[string]string, bool) {
	raw := strings.TrimSpace(message)
	if raw == "" {
		return "", nil, false
	}
	lower := strings.ToLower(raw)
	atmID := firstATMID(raw)

	// ---- issue.open: "open a bug: <title>" / "create a task: <title>" ----
	// These do NOT reference an ATM id — they CREATE a new item. The title is
	// whatever follows the colon. We require a non-empty title after the colon
	// (a bare "open a bug:" with nothing after it is not actionable → no-match).
	if itemType, title, ok := recognizeIssueOpen(raw, lower); ok {
		return "issue.open", map[string]string{
			cmdFieldType:  itemType,
			cmdFieldTitle: title,
		}, true
	}

	// Every remaining command form needs a concrete ATM target. Without one,
	// decline (defer to the LLM) — "close it" / "mark that fixed" are exactly
	// the ambiguous targets §2 says must NOT be guessed.
	if atmID == "" {
		return "", nil, false
	}

	// ---- assign: "assign ATM-5 to @bob" / "give ATM-5 to @bob" ----
	if assignee, ok := recognizeAssign(raw, lower); ok {
		return "item.update", map[string]string{
			cmdFieldAtmID:      atmID,
			cmdFieldAssignedTo: assignee,
		}, true
	}

	// ---- investigation: "investigate ATM-7" / "look into ATM-7" ----
	if recognizeInvestigate(lower) {
		return "investigation.start", map[string]string{
			cmdFieldAtmID: atmID,
		}, true
	}

	// ---- status query: "status of ATM-9?" / "what's ATM-9?" ----
	// Checked BEFORE the close/set verbs so an interrogative ("what is the
	// status of ATM-9?") is never mis-read as a mutating "set status" command.
	if recognizeStatusQuery(lower) {
		return "reply", map[string]string{
			cmdFieldAtmID: atmID,
			cmdFieldQuery: "status",
		}, true
	}

	// ---- close/resolve: "close ATM-123" / "mark ATM-5 fixed/done/resolved" ----
	if recognizeClose(lower) {
		return "item.update", map[string]string{
			cmdFieldAtmID:  atmID,
			cmdFieldStatus: "closed",
		}, true
	}

	// ---- set status: "set ATM-9 to in progress" / "ATM-9 is blocked" ----
	if status, ok := recognizeSetStatus(raw, lower); ok {
		return "item.update", map[string]string{
			cmdFieldAtmID:  atmID,
			cmdFieldStatus: status,
		}, true
	}

	return "", nil, false
}

// firstATMID returns the first ATM-NNN id in the message, normalized to
// upper-case ("ATM-9"), or "" when none is present.
func firstATMID(message string) string {
	m := atmIDPattern.FindStringSubmatch(message)
	if len(m) < 2 {
		return ""
	}
	return strings.ToUpper(m[1])
}

// issueOpenPattern matches the create-item imperatives. The leading verb +
// noun is required ("open a bug", "create a task", "new feature request",
// "file a bug", "raise an issue"); the title is whatever follows the FIRST
// colon. CONSERVATIVE: a colon is required — it is the unambiguous separator
// between the verb phrase and the title.
var issueOpenPattern = regexp.MustCompile(`(?i)\b(?:open|create|file|raise|new|log)\b[^:]*\b(bug|task|feature|issue|defect|request|story)\b[^:]*:\s*(.+)$`)

// recognizeIssueOpen detects "open a bug: <title>" style commands and returns
// the normalized item type + the title. The title MUST be non-empty after the
// colon.
func recognizeIssueOpen(raw, _ string) (itemType, title string, ok bool) {
	m := issueOpenPattern.FindStringSubmatch(raw)
	if len(m) < 3 {
		return "", "", false
	}
	title = strings.TrimSpace(m[2])
	if title == "" {
		return "", "", false
	}
	itemType = normalizeIssueType(strings.ToLower(m[1]))
	return itemType, title, true
}

// normalizeIssueType maps the recognized noun to the canonical item type set.
func normalizeIssueType(noun string) string {
	switch noun {
	case "bug", "defect":
		return "bug"
	case "feature", "story", "request":
		return "feature"
	case "issue":
		return "issue"
	default: // "task"
		return "task"
	}
}

// assignPattern matches "assign|give ATM-N to @who". The assignee token is
// captured verbatim (with or without a leading @); the dispatcher normalizes it.
var assignPattern = regexp.MustCompile(`(?i)\b(?:assign|give|hand(?:\s+off)?)\b.*\bto\s+(@?\w[\w.\-]*)`)

// recognizeAssign detects an assignment imperative and returns the (un-
// normalized) assignee handle. It only fires when an explicit assign/give verb
// is present AND a "to <who>" target follows — "give me ATM-5" (no "to") does
// not match.
func recognizeAssign(raw, lower string) (assignee string, ok bool) {
	if !strings.Contains(lower, "assign") && !strings.Contains(lower, "give") && !strings.Contains(lower, "hand") {
		return "", false
	}
	m := assignPattern.FindStringSubmatch(raw)
	if len(m) < 2 {
		return "", false
	}
	a := strings.TrimSpace(m[1])
	if a == "" {
		return "", false
	}
	return a, true
}

// recognizeInvestigate detects "investigate ATM-7" / "look into ATM-7" /
// "dig into ATM-7".
func recognizeInvestigate(lower string) bool {
	return strings.Contains(lower, "investigate") ||
		strings.Contains(lower, "look into") ||
		strings.Contains(lower, "dig into") ||
		strings.Contains(lower, "look at")
}

// statusQueryPattern matches interrogative status requests. We require an
// interrogative cue (a leading question word OR a trailing "?") so a plain
// "set the status" imperative is never captured here.
var statusQueryPattern = regexp.MustCompile(`(?i)^(?:what|what's|whats|status|how|where|when|who|why)\b`)

// recognizeStatusQuery detects "status of ATM-9?" / "what's ATM-9?" /
// "what is the status of ATM-9".
func recognizeStatusQuery(lower string) bool {
	if strings.HasSuffix(strings.TrimSpace(lower), "?") {
		return true
	}
	return statusQueryPattern.MatchString(strings.TrimSpace(lower))
}

// recognizeClose detects close/resolve imperatives: "close ATM-123",
// "mark ATM-5 fixed", "mark ATM-5 as done", "resolve ATM-5", "ATM-5 is fixed".
func recognizeClose(lower string) bool {
	// Explicit close/resolve verbs.
	for _, v := range []string{"close ", "resolve ", "mark "} {
		if strings.Contains(lower, v) {
			// "mark" must be paired with a closed-state word to count as a
			// close (otherwise "mark ATM-5 as blocked" is a set-status, not a
			// close).
			if v == "mark " {
				if containsAny(lower, "fixed", "done", "resolved", "closed", "complete") {
					return true
				}
				continue
			}
			return true
		}
	}
	// "ATM-5 is fixed/done/resolved/closed" — a state assertion that closes.
	if strings.Contains(lower, " is ") || strings.Contains(lower, " as ") {
		if containsAny(lower, "fixed", "done", "resolved", "closed") {
			return true
		}
	}
	return false
}

// setStatusPattern captures the target status from "set ATM-9 to <status>".
var setStatusPattern = regexp.MustCompile(`(?i)\bset\b.*\bto\s+(.+?)\s*$`)

// knownStatuses is the conservative closed-set of statuses the recognizer will
// commit to. A "set ATM-9 to <x>" where <x> is not a known status falls
// through to the LLM rather than writing an arbitrary string into the item.
var knownStatuses = map[string]string{
	"in progress": "In progress",
	"inprogress":  "In progress",
	"in-progress": "In progress",
	"blocked":     "Blocked",
	"open":        "Open",
	"reopened":    "Reopened",
	"in testing":  "In testing",
	"in review":   "In review",
	"ready":       "Ready for testing",
	"on hold":     "On hold",
	"closed":      "closed",
	"done":        "closed",
	"fixed":       "closed",
	"resolved":    "closed",
}

// recognizeSetStatus detects "set ATM-9 to in progress" and "ATM-9 is blocked",
// returning the canonical status. CONSERVATIVE: the parsed status MUST be in
// knownStatuses, else no-match (the LLM handles novel/freeform statuses).
func recognizeSetStatus(raw, lower string) (status string, ok bool) {
	// Form A: "set ... to <status>".
	if strings.Contains(lower, "set ") {
		m := setStatusPattern.FindStringSubmatch(raw)
		if len(m) >= 2 {
			if canon, found := lookupStatus(m[1]); found {
				return canon, true
			}
		}
	}
	// Form B: "ATM-9 is blocked" — a state assertion (non-closing states only;
	// closing states are handled by recognizeClose).
	if strings.Contains(lower, " is ") {
		for k, v := range knownStatuses {
			if v == "closed" {
				continue // closing assertions belong to recognizeClose
			}
			if strings.Contains(lower, " is "+k) {
				return v, true
			}
		}
	}
	return "", false
}

// lookupStatus normalizes a free phrase to a canonical status if it is in the
// known set. Trailing punctuation is trimmed.
func lookupStatus(phrase string) (string, bool) {
	p := strings.ToLower(strings.TrimSpace(phrase))
	p = strings.TrimRight(p, ".!?")
	p = strings.TrimSpace(p)
	if canon, ok := knownStatuses[p]; ok {
		return canon, true
	}
	return "", false
}

// containsAny reports whether s contains any of the given substrings.
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
