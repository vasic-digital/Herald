// Package inbound — commands.go: §32.6 fast-path command handlers.
//
// These handlers run BEFORE Claude Code dispatch when the classifier emits
// a §32.6 command-type prefix (help_command / status_request /
// continuation_request / closure / reopen). They read local Markdown
// docs (or, for HandleHelp, an embedded BuiltinHelp constant) and reply
// directly — zero LLM cost, sub-millisecond response.
//
// T4 (Wave 6.5) lands Help/Status/Continue here. T5 will extend the
// CommandsConfig with HandleDone + HandleReopen for the atomic
// Issues.md ↔ Fixed.md migration (operator-role gated).
//
// §107 anchor: each handler reads a REAL file (or named constant).
// Unit tests assert on actual extracted bytes from real files in
// t.TempDir(). A handler that returned a hard-coded string would FAIL
// the tests because the fixtures carry specific sentinel content.
package inbound

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/vasic-digital/herald/commons"
)

// CommandsConfig is the dependency-injection container for the §32.6
// fast-path handlers. It is constructed by pherald listen wiring (T7)
// from operator-supplied paths and env vars; unit tests construct it
// in-line against t.TempDir() fixtures.
//
// T5 will populate the remaining fields (IssuesPath / FixedPath /
// LockPath / OperatorIDs / Clock / Rename) for HandleDone / HandleReopen.
type CommandsConfig struct {
	// DocsDir is the canonical docs root (typically "docs"). Reserved
	// for future helpers — current T4 handlers consume explicit *Path
	// fields so each handler is independently testable without
	// stamping the whole docs tree into t.TempDir().
	DocsDir string

	// HelpPath is the optional path to a docs/Help.md operator override.
	// If empty (or the file does not exist), HandleHelp returns the
	// BuiltinHelp const so pherald listen works on a clean checkout.
	HelpPath string

	// StatusPath is the path to docs/Status.md. HandleStatus reads it
	// and extracts the "Status summary" field from the header table.
	StatusPath string

	// ContinuePath is the path to docs/CONTINUATION.md. HandleContinue
	// reads it and extracts the "Continuation" field from the header
	// table.
	ContinuePath string

	// T5 fields — populated in Wave 6.5 T5 (Done/Reopen). Documented
	// here so callers wire them once and HandleDone/HandleReopen pick
	// them up automatically.
	IssuesPath  string
	FixedPath   string
	LockPath    string
	OperatorIDs map[string]bool
	Clock       commons.Clock
	Rename      func(old, new string) error // optional override for atomicity tests
}

// BuiltinHelp is the §32.6 command catalogue rendered when HandleHelp
// has no HelpPath (or the file is missing). Kept as a code constant so
// pherald listen works out-of-the-box on a fresh checkout.
const BuiltinHelp = `Herald — Command catalogue (§32.6)

  Bug: <title>             — open a bug-type workable item
  Issue: <title>           — alias for Bug:
  Task: <title>            — open a task-type workable item
  Implementation: <title>  — alias for Task:
  Investigation: <title>   — open an investigation (Wave 7)
  Query: <prose>           — ask Herald a question (LLM-driven)
  Question: <prose>        — alias for Query:
  Q: <prose>               — short alias
  Request: <prose>         — alias for Query:
  Help:                    — this message
  Status:                  — current project status
  Continue:                — pointer to CONTINUATION.md
  Done: HRD-NNN            — close item (operator role only)
  Resolve: HRD-NNN         — alias for Done:
  Reopen: HRD-NNN          — re-open (operator role only)
  Override: HRD-NNN type=X — reclassify (operator role only)

Criticality keywords anywhere: critical / urgent / high / important / low / trivial.
Natural language with no prefix → treated as Query:.
`

// maxReplyChars caps the rendered reply at ~2000 bytes so the Telegram
// 4096-char per-message ceiling has ample headroom for framing
// (envelope pre-text, ReplyTo metadata, Markdown escaping). When a
// handler trims, it appends a "[...truncated...]" marker so the
// operator sees the trim explicitly.
const maxReplyChars = 2000

// HandleHelp returns the operator-supplied Help.md content if
// c.HelpPath is configured AND readable, else returns BuiltinHelp.
//
// A configured-but-missing HelpPath falls back to BuiltinHelp silently
// (no error) — the typical pherald listen deployment ships without a
// docs/Help.md and we do NOT want the bot to refuse Help: in that case.
//
// An unreadable file (permission denied, etc.) is surfaced as an
// error — the operator wired the path explicitly, so a failure here
// indicates a real misconfiguration we must not paper over.
func (c *CommandsConfig) HandleHelp(_ context.Context) (string, []commons.Attachment, error) {
	if c.HelpPath != "" {
		data, err := os.ReadFile(c.HelpPath)
		if err == nil {
			return truncateReply(string(data)), nil, nil
		}
		if !os.IsNotExist(err) {
			return "", nil, fmt.Errorf("HandleHelp: read %s: %w", c.HelpPath, err)
		}
	}
	return BuiltinHelp, nil, nil
}

// HandleStatus reads c.StatusPath, extracts the "Status summary" row
// from the leading Markdown header table, and returns it preceded by
// a "Last modified: <date>" line.
//
// The whole docs/Status.md file is ~30-50 KB in practice — well over
// Telegram's 4096-char ceiling. The Status summary field is the
// canonical single-row description per Herald §107.x evidence
// convention; that is what we surface to operators.
//
// If c.StatusPath is empty OR the file is unreadable, an error is
// returned (no silent fallback — pherald listen wired this explicitly).
func (c *CommandsConfig) HandleStatus(_ context.Context) (string, []commons.Attachment, error) {
	if c.StatusPath == "" {
		return "", nil, errors.New("HandleStatus: StatusPath not configured")
	}
	data, err := os.ReadFile(c.StatusPath)
	if err != nil {
		return "", nil, fmt.Errorf("HandleStatus: read %s: %w", c.StatusPath, err)
	}
	text := string(data)
	summary, ok := extractTableField(text, "Status summary")
	if !ok {
		return "", nil, fmt.Errorf("HandleStatus: 'Status summary' field not found in %s", c.StatusPath)
	}
	lastMod, _ := extractTableField(text, "Last modified")
	var b strings.Builder
	if lastMod != "" {
		fmt.Fprintf(&b, "Last modified: %s\n\n", lastMod)
	}
	fmt.Fprintf(&b, "Status summary: %s", summary)
	return truncateReply(b.String()), nil, nil
}

// HandleContinue reads c.ContinuePath, extracts the "Continuation"
// row from the leading Markdown header table, and returns it.
//
// As with HandleStatus, the full docs/CONTINUATION.md is too large
// for a single Telegram message — the Continuation field captures
// the operator-curated "next step" pointer that Herald §107.x mandates.
//
// If c.ContinuePath is empty OR the file is unreadable, an error is
// returned (no silent fallback).
func (c *CommandsConfig) HandleContinue(_ context.Context) (string, []commons.Attachment, error) {
	if c.ContinuePath == "" {
		return "", nil, errors.New("HandleContinue: ContinuePath not configured")
	}
	data, err := os.ReadFile(c.ContinuePath)
	if err != nil {
		return "", nil, fmt.Errorf("HandleContinue: read %s: %w", c.ContinuePath, err)
	}
	text := string(data)
	cont, ok := extractTableField(text, "Continuation")
	if !ok {
		return "", nil, fmt.Errorf("HandleContinue: 'Continuation' field not found in %s", c.ContinuePath)
	}
	return truncateReply("Continuation: " + cont), nil, nil
}

// tableFieldRE matches a single Markdown table row of the form
//
//	| <name> | <value> |
//
// where <name> is captured by the caller as a regex prefix (the leading
// `| <name> |` literal). The value is everything between the second
// `|` and the trailing `|` at line end, trimmed of surrounding spaces.
//
// This intentionally tolerates inline `|` only when escaped as `\|`
// (the convention Herald docs follow); a row with an unescaped inline
// `|` in the value would split mid-cell — acceptable for first
// implementation per the plan, since Herald's own header tables do
// not use unescaped pipes in field values.
var tableFieldRE = regexp.MustCompile(`(?m)^\|\s*([^|]+?)\s*\|\s*(.+?)\s*\|\s*$`)

// extractTableField scans body for a Markdown header-table row
// matching `| <fieldName> | <value> |` (case-insensitive on
// fieldName, whitespace-tolerant) and returns the value.
//
// Returns ("", false) when no row matches. Multi-row matches return
// the FIRST occurrence — Herald header tables are documented as
// single-occurrence per field.
func extractTableField(body, fieldName string) (string, bool) {
	want := strings.ToLower(strings.TrimSpace(fieldName))
	for _, m := range tableFieldRE.FindAllStringSubmatch(body, -1) {
		if strings.ToLower(strings.TrimSpace(m[1])) == want {
			return strings.TrimSpace(m[2]), true
		}
	}
	return "", false
}

// truncateReply caps text at maxReplyChars and appends a visible
// "[...truncated...]" marker so the operator sees the trim. Short
// inputs pass through verbatim.
func truncateReply(text string) string {
	if len(text) <= maxReplyChars {
		return text
	}
	const marker = "\n\n[...truncated; see docs/ source for full text...]"
	cutoff := maxReplyChars - len(marker)
	if cutoff < 0 {
		cutoff = 0
	}
	return text[:cutoff] + marker
}

// docsPath joins a relative filename against c.DocsDir if c.DocsDir
// is set; absolute paths pass through. Reserved for T5 helpers that
// resolve siblings of Issues.md / Fixed.md by name.
func (c *CommandsConfig) docsPath(name string) string { // nolint:unused
	if filepath.IsAbs(name) {
		return name
	}
	if c.DocsDir == "" {
		return name
	}
	return filepath.Join(c.DocsDir, name)
}
