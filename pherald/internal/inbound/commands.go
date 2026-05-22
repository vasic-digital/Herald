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
	"strconv"
	"strings"
	"time"

	"github.com/vasic-digital/herald/commons"
)

// ErrNotOperator is returned by HandleDone / HandleReopen when the
// invoking sender's channel_user_id is not in c.OperatorIDs. It is the
// pre-file-I/O guard — the §107 "non-operator rejected BEFORE any file
// is touched" anchor checks for this exact error type.
//
// Wave 6.5 ships a stub allowlist (HERALD_OPERATOR_IDS env var, parsed
// in cmd/pherald/listen.go); Wave 7 replaces this with the V3 §32.10
// subscriber-role mapping (full RBAC).
var ErrNotOperator = errors.New("sender not in HERALD_OPERATOR_IDS — Done:/Reopen: rejected")

// hrdRefRE captures the first occurrence of HRD-NNN (one or more
// decimal digits) anywhere in the body. Matches "Done: HRD-042",
// "Done: HRD-042 — fixed it", "Reopen: HRD-007 because reason",
// etc. ParseHRDRef returns ("", error) if no match.
var hrdRefRE = regexp.MustCompile(`HRD-\d+`)

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

// --- T5: Done / Reopen — atomic Issues.md ↔ Fixed.md migration ---
//
// The two-file migration uses a tempfile + double-rename pattern.
// Both tempfiles live in the SAME directory as their destinations so
// the rename is a POSIX-atomic inode swap rather than a cross-filesystem
// copy. The source file is moved to a `.bak.<nanostamp>` backup BEFORE
// the new content lands; if the second rename (destination) fails, the
// backup is renamed back into place — the source file is restored to
// its pre-call bytes. The destination file is untouched on rollback
// because the rollback path runs BEFORE the dst-tmp → dst rename.
//
// §107 anchor: TestHandleDone_RollbackOnSecondRenameFailure injects a
// c.Rename that fails on its 2nd invocation and asserts the source
// file's bytes match the original — proves the rollback path runs.

// requireOperator returns ErrNotOperator if senderID is not in
// c.OperatorIDs. It is the FIRST line of HandleDone / HandleReopen —
// no file I/O happens until the role check passes.
//
// An empty / nil OperatorIDs map means "no operators configured" and
// rejects ALL Done:/Reopen: calls — the explicit-fail-closed posture
// per spec §32.6.
func (c *CommandsConfig) requireOperator(senderID string) error {
	if c.OperatorIDs == nil || !c.OperatorIDs[senderID] {
		return ErrNotOperator
	}
	return nil
}

// parseHRDRef extracts the first HRD-NNN token from body. Accepts any
// surrounding text — "Done: HRD-042", "Done: HRD-042 — reason text",
// "Reopen: HRD-7 because foo" all return "HRD-042" / "HRD-7" / etc.
// Returns an error when no HRD-NNN substring is present.
func parseHRDRef(body string) (string, error) {
	if m := hrdRefRE.FindString(body); m != "" {
		return m, nil
	}
	return "", fmt.Errorf("parseHRDRef: no HRD-NNN reference found in %q", body)
}

// HandleDone migrates the HRD-NNN row from c.IssuesPath into
// c.FixedPath, annotating the migrated row's status cell with
// "Closed (Wave 6.5)" + today's date. Operator-gated; non-operators
// receive ErrNotOperator before any file is touched.
func (c *CommandsConfig) HandleDone(ctx context.Context, body, senderID string) (string, []commons.Attachment, error) {
	if err := c.requireOperator(senderID); err != nil {
		return "", nil, err
	}
	ref, err := parseHRDRef(body)
	if err != nil {
		return "", nil, err
	}
	if err := c.migrateRow(ctx, c.IssuesPath, c.FixedPath, ref, "Closed (Wave 6.5)"); err != nil {
		return "", nil, err
	}
	return fmt.Sprintf("Done: %s migrated Issues.md → Fixed.md", ref), nil, nil
}

// HandleReopen migrates the HRD-NNN row from c.FixedPath into
// c.IssuesPath, annotating the migrated row's status cell with
// "Reopened (Wave 6.5)" + today's date. Symmetric to HandleDone:
// operator-gated, ErrNotOperator before any I/O.
func (c *CommandsConfig) HandleReopen(ctx context.Context, body, senderID string) (string, []commons.Attachment, error) {
	if err := c.requireOperator(senderID); err != nil {
		return "", nil, err
	}
	ref, err := parseHRDRef(body)
	if err != nil {
		return "", nil, err
	}
	if err := c.migrateRow(ctx, c.FixedPath, c.IssuesPath, ref, "Reopened (Wave 6.5)"); err != nil {
		return "", nil, err
	}
	return fmt.Sprintf("Reopen: %s migrated Fixed.md → Issues.md", ref), nil, nil
}

// migrateRow is the atomic two-file move. ctx is currently unused (no
// long-running ops) but kept on the signature so future cancellation
// during rename retries is a non-breaking change.
//
// Failure modes (in order of progress through the function):
//
//  1. ReadFile srcPath fails → return error, no fs change.
//  2. ReadFile dstPath fails → return error, no fs change.
//  3. removeRow(src) fails (HRD ref not found) → return error, no fs change.
//  4. WriteFile srcTmp fails → return error, no fs change.
//  5. WriteFile dstTmp fails → cleanup srcTmp, no other fs change.
//  6. Rename src → srcBackup fails → cleanup both tmps.
//  7. Rename srcTmp → src fails → restore srcBackup → src, cleanup dstTmp.
//  8. Rename dstTmp → dst fails → restore srcBackup → src, cleanup dstTmp.
//     (THE rollback path; §107 anchor. dst is untouched.)
//  9. Success → remove srcBackup.
//
//nolint:revive // ctx reserved for future cancellation
func (c *CommandsConfig) migrateRow(ctx context.Context, srcPath, dstPath, hrdRef, dstStatus string) error {
	_ = ctx
	srcData, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", srcPath, err)
	}
	dstData, err := os.ReadFile(dstPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", dstPath, err)
	}

	row, newSrc, err := removeRow(string(srcData), hrdRef)
	if err != nil {
		return fmt.Errorf("remove row from %s: %w", srcPath, err)
	}
	newDst := prependRow(string(dstData), row, dstStatus, c.Clock)

	stamp := nanoStamp()
	srcTmp := srcPath + ".tmp." + stamp
	dstTmp := dstPath + ".tmp." + stamp
	srcBackup := srcPath + ".bak." + stamp

	// POSIX atomicity: temp files live alongside their destinations so
	// rename is a same-filesystem inode swap.
	if err := os.WriteFile(srcTmp, []byte(newSrc), 0o644); err != nil {
		return fmt.Errorf("write src tmp: %w", err)
	}
	if err := os.WriteFile(dstTmp, []byte(newDst), 0o644); err != nil {
		_ = os.Remove(srcTmp)
		return fmt.Errorf("write dst tmp: %w", err)
	}

	rename := c.renameFn()
	if err := rename(srcPath, srcBackup); err != nil {
		_ = os.Remove(srcTmp)
		_ = os.Remove(dstTmp)
		return fmt.Errorf("backup src: %w", err)
	}
	if err := rename(srcTmp, srcPath); err != nil {
		_ = rename(srcBackup, srcPath)
		_ = os.Remove(srcTmp)
		_ = os.Remove(dstTmp)
		return fmt.Errorf("rename src: %w", err)
	}
	if err := rename(dstTmp, dstPath); err != nil {
		// ROLLBACK path: undo the src rename by restoring the backup.
		// We must REMOVE the post-rename src first because Rename on
		// many filesystems will not overwrite an existing target.
		_ = os.Remove(srcPath)
		_ = rename(srcBackup, srcPath)
		_ = os.Remove(dstTmp)
		return fmt.Errorf("rename dst (rolled back src): %w", err)
	}
	_ = os.Remove(srcBackup)
	return nil
}

// removeRow scans src line-by-line for the FIRST Markdown table row
// whose first cell trims to "HRD-NNN" (i.e. the row beginning with
// "| HRD-NNN |"). Returns the removed row verbatim (including the
// trailing newline) and the source with that line excised. Errors
// when no matching row is found.
func removeRow(src, hrdRef string) (string, string, error) {
	lines := strings.SplitAfter(src, "\n")
	for i, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "| "+hrdRef+" |") {
			row := ln
			out := append([]string{}, lines[:i]...)
			out = append(out, lines[i+1:]...)
			return row, strings.Join(out, ""), nil
		}
	}
	return "", "", fmt.Errorf("row containing %q not found", hrdRef)
}

// prependRow inserts row (after annotateRowStatus mutates its status
// cell) immediately AFTER the Markdown table header separator (the
// `|---` line). Tables without a separator append the row to EOF as
// a safe-default. clk supplies the date stamp (RealClock in production,
// FakeClock in tests).
func prependRow(dst, row, status string, clk commons.Clock) string {
	lines := strings.SplitAfter(dst, "\n")
	insertAt := -1
	for i, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "|---") {
			insertAt = i + 1
			break
		}
	}
	annotated := annotateRowStatus(row, status, clk)
	if insertAt < 0 {
		return dst + annotated
	}
	out := append([]string{}, lines[:insertAt]...)
	out = append(out, annotated)
	out = append(out, lines[insertAt:]...)
	return strings.Join(out, "")
}

// annotateRowStatus best-effort rewrites a 4-column Issues/Fixed table
// row of shape `| HRD-NNN | <title> | <status> | <date> |` so the
// status cell carries the new status + today's date. If the regex does
// NOT match (operator-corrupted row shape, or a future table layout),
// the row passes through verbatim — §107 anchor: never silently
// drop / scramble bytes we can't reason about.
func annotateRowStatus(row, status string, clk commons.Clock) string {
	if clk == nil {
		clk = commons.RealClock{}
	}
	today := clk.Now().UTC().Format("2006-01-02")
	// Match `| <c1> | <c2> | <c3> | <c4-and-rest>` — 4-column rows of
	// the canonical Issues/Fixed shape. Replace cell 3 (status) with
	// the new status; leave c1/c2/c4 intact.
	m := rowStatusRE.FindStringSubmatch(row)
	if m == nil {
		return row
	}
	return fmt.Sprintf("| %s | %s | %s — %s | %s",
		strings.TrimSpace(m[1]),
		strings.TrimSpace(m[2]),
		status,
		today,
		strings.TrimSpace(m[3]),
	) + "\n"
}

// rowStatusRE matches the leading 3 cells of a 4-column Markdown row,
// capturing them individually. Cell 1 is HRD-NNN; cell 2 the title;
// cell 3 the prior status; the remainder (date + closing pipe + any
// trailing newline) is what we keep verbatim.
var rowStatusRE = regexp.MustCompile(`^\|\s*([^|]+?)\s*\|\s*([^|]+?)\s*\|\s*[^|]+?\s*\|\s*(.+?)\s*$`)

// renameFn returns c.Rename if set (the test-injection point for the
// rollback assertion) else os.Rename.
func (c *CommandsConfig) renameFn() func(string, string) error {
	if c.Rename != nil {
		return c.Rename
	}
	return os.Rename
}

// nanoStamp returns the current nanosecond Unix epoch as a base-10
// string. Used to mint unique tempfile / backup suffixes that won't
// collide even under concurrent migrations (Wave 6.5 is single-tenant
// per pherald listen; suffix collision is still impossible).
func nanoStamp() string {
	return strconv.FormatInt(time.Now().UnixNano(), 10)
}
