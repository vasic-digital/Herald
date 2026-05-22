// Package inbound — commands_test.go: §107 anti-bluff coverage for the
// Wave 6.5 T4 fast-path handlers. Every test writes a canned Markdown
// fixture into t.TempDir() with sentinel content and asserts the
// handler returns bytes drawn from that fixture — a hard-coded reply
// would fail because the sentinels are test-unique.
package inbound

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vasic-digital/herald/commons"
)

// fixtureStatusMd mirrors the docs/Status.md leading-header-table
// layout (5 metadata rows + Status summary + Issues/Fixed rows +
// Continuation). The Status summary value carries a sentinel string
// the test asserts on byte-for-byte.
const fixtureStatusMd = `# Herald — Status

| Field | Value |
|---|---|
| Revision | 99 |
| Created | 2026-05-22 |
| Last modified | 2026-05-22 |
| Status | active |
| Status summary | sentinel-status-XYZ-Wave-6.5-T4 lands fast-path Help/Status/Continue handlers per plan T4 |
| Issues | none |
| Fixed | none |
| Continuation | sentinel-continuation-XYZ T5 next |

## Body

Lorem ipsum dolor sit amet, consectetur adipiscing elit.
`

// fixtureContinueMd mirrors docs/CONTINUATION.md's header-table
// layout. The Continuation field carries the sentinel the test
// asserts on.
const fixtureContinueMd = `# Herald — Continuation

| Field | Value |
|---|---|
| Revision | 99 |
| Created | 2026-05-22 |
| Last modified | 2026-05-22 |
| Status | active |
| Status summary | placeholder |
| Continuation | sentinel-continue-ABC-T5-next: extend CommandsConfig with HandleDone+HandleReopen |

## §0. How to use this document

Lorem ipsum.
`

func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

// TestHandleHelp_BuiltinFallback — when HelpPath is empty, the handler
// returns the BuiltinHelp const. Sentinel: "Command catalogue (§32.6)"
// header is part of the const but NOT any of the file fixtures.
func TestHandleHelp_BuiltinFallback(t *testing.T) {
	c := &CommandsConfig{}
	reply, atts, err := c.HandleHelp(context.Background())
	if err != nil {
		t.Fatalf("HandleHelp: %v", err)
	}
	if atts != nil {
		t.Errorf("expected nil attachments, got %d", len(atts))
	}
	if !strings.Contains(reply, "Command catalogue (§32.6)") {
		t.Errorf("BuiltinHelp sentinel missing; got first 200 bytes: %q", reply[:min(200, len(reply))])
	}
	if !strings.Contains(reply, "Bug: <title>") {
		t.Errorf("BuiltinHelp Bug-row sentinel missing")
	}
}

// TestHandleHelp_BuiltinFallback_MissingPath — a configured-but-missing
// HelpPath silently falls back to BuiltinHelp (no error). Models the
// canonical pherald listen deployment that ships without docs/Help.md.
func TestHandleHelp_BuiltinFallback_MissingPath(t *testing.T) {
	dir := t.TempDir()
	c := &CommandsConfig{HelpPath: filepath.Join(dir, "Help.md")} // does not exist
	reply, _, err := c.HandleHelp(context.Background())
	if err != nil {
		t.Fatalf("HandleHelp: %v", err)
	}
	if !strings.Contains(reply, "Command catalogue (§32.6)") {
		t.Errorf("expected BuiltinHelp fallback on missing file")
	}
}

// TestHandleHelp_CustomPath — when HelpPath points at a real file,
// the handler returns its CONTENT verbatim. Sentinel "delta-help-sentinel"
// is not in BuiltinHelp.
func TestHandleHelp_CustomPath(t *testing.T) {
	dir := t.TempDir()
	custom := "# Custom Herald Help\n\ndelta-help-sentinel — operator override.\n"
	hp := writeTempFile(t, dir, "Help.md", custom)
	c := &CommandsConfig{HelpPath: hp}
	reply, _, err := c.HandleHelp(context.Background())
	if err != nil {
		t.Fatalf("HandleHelp: %v", err)
	}
	if !strings.Contains(reply, "delta-help-sentinel") {
		t.Errorf("expected operator-supplied Help.md content; got: %q", reply)
	}
	if strings.Contains(reply, "Command catalogue (§32.6)") {
		t.Errorf("operator Help.md should NOT include BuiltinHelp marker; got: %q", reply)
	}
}

// TestHandleStatus_ExtractsSummary — Status.md with the canonical
// header table yields the "Status summary" cell. Asserts the sentinel
// byte-string lands verbatim AND the "Last modified" leading line
// is present.
func TestHandleStatus_ExtractsSummary(t *testing.T) {
	dir := t.TempDir()
	sp := writeTempFile(t, dir, "Status.md", fixtureStatusMd)
	c := &CommandsConfig{StatusPath: sp}
	reply, atts, err := c.HandleStatus(context.Background())
	if err != nil {
		t.Fatalf("HandleStatus: %v", err)
	}
	if atts != nil {
		t.Errorf("expected nil attachments, got %d", len(atts))
	}
	if !strings.Contains(reply, "sentinel-status-XYZ-Wave-6.5-T4") {
		t.Errorf("expected Status-summary sentinel in reply; got: %q", reply)
	}
	if !strings.Contains(reply, "Last modified: 2026-05-22") {
		t.Errorf("expected leading 'Last modified: ...' line; got: %q", reply)
	}
	// Critically: the BODY section ("Lorem ipsum") must NOT leak —
	// otherwise the handler is returning whole-file (which would blow
	// past Telegram's 4096-char ceiling for real Status.md sizes).
	if strings.Contains(reply, "Lorem ipsum") {
		t.Errorf("HandleStatus should extract summary, not return whole file; got: %q", reply)
	}
}

// TestHandleStatus_MissingFile — unreadable Status.md surfaces an
// error. The pherald listen wiring is explicit; a missing file is a
// real misconfiguration, not a silent-fallback case.
func TestHandleStatus_MissingFile(t *testing.T) {
	dir := t.TempDir()
	c := &CommandsConfig{StatusPath: filepath.Join(dir, "Status.md")} // does not exist
	_, _, err := c.HandleStatus(context.Background())
	if err == nil {
		t.Fatal("expected error on missing Status.md, got nil")
	}
	if !strings.Contains(err.Error(), "HandleStatus") {
		t.Errorf("error should be tagged 'HandleStatus'; got: %v", err)
	}
}

// TestHandleStatus_EmptyPath — unconfigured StatusPath is also an
// error (explicit no-silent-default per plan T4 self-review).
func TestHandleStatus_EmptyPath(t *testing.T) {
	c := &CommandsConfig{}
	_, _, err := c.HandleStatus(context.Background())
	if err == nil {
		t.Fatal("expected error on empty StatusPath")
	}
}

// TestHandleStatus_MissingSummaryField — Status.md without the
// "Status summary" row surfaces an explicit error. (Without this
// guard, an operator-corrupted Status.md would silently return an
// empty reply.)
func TestHandleStatus_MissingSummaryField(t *testing.T) {
	dir := t.TempDir()
	bogus := "# Herald — Status\n\n| Field | Value |\n|---|---|\n| Revision | 1 |\n"
	sp := writeTempFile(t, dir, "Status.md", bogus)
	c := &CommandsConfig{StatusPath: sp}
	_, _, err := c.HandleStatus(context.Background())
	if err == nil {
		t.Fatal("expected error on missing 'Status summary' field")
	}
	if !strings.Contains(err.Error(), "Status summary") {
		t.Errorf("error should mention the missing field; got: %v", err)
	}
}

// TestHandleContinue_ExtractsContinuation — CONTINUATION.md with the
// canonical header table yields the "Continuation" cell verbatim.
func TestHandleContinue_ExtractsContinuation(t *testing.T) {
	dir := t.TempDir()
	cp := writeTempFile(t, dir, "CONTINUATION.md", fixtureContinueMd)
	c := &CommandsConfig{ContinuePath: cp}
	reply, atts, err := c.HandleContinue(context.Background())
	if err != nil {
		t.Fatalf("HandleContinue: %v", err)
	}
	if atts != nil {
		t.Errorf("expected nil attachments, got %d", len(atts))
	}
	if !strings.Contains(reply, "sentinel-continue-ABC-T5-next") {
		t.Errorf("expected Continuation sentinel in reply; got: %q", reply)
	}
	// Body section must not leak.
	if strings.Contains(reply, "How to use this document") {
		t.Errorf("HandleContinue should extract field, not return whole file; got: %q", reply)
	}
}

// TestHandleContinue_MissingFile — error path for an unreadable file.
func TestHandleContinue_MissingFile(t *testing.T) {
	dir := t.TempDir()
	c := &CommandsConfig{ContinuePath: filepath.Join(dir, "CONTINUATION.md")}
	_, _, err := c.HandleContinue(context.Background())
	if err == nil {
		t.Fatal("expected error on missing CONTINUATION.md, got nil")
	}
}

// TestHandleContinue_EmptyPath — unconfigured ContinuePath is an error.
func TestHandleContinue_EmptyPath(t *testing.T) {
	c := &CommandsConfig{}
	_, _, err := c.HandleContinue(context.Background())
	if err == nil {
		t.Fatal("expected error on empty ContinuePath")
	}
}

// TestTruncateReply_HonorsLimit — input ≤ maxReplyChars passes
// through verbatim; input over the limit is trimmed and tagged with
// the truncation marker. The total returned length MUST NOT exceed
// maxReplyChars (the trim is inclusive of the marker length).
func TestTruncateReply_HonorsLimit(t *testing.T) {
	short := "hello world"
	if got := truncateReply(short); got != short {
		t.Errorf("short input modified: got %q want %q", got, short)
	}
	long := strings.Repeat("x", maxReplyChars+500)
	out := truncateReply(long)
	if len(out) > maxReplyChars {
		t.Errorf("truncated output too long: got %d want ≤ %d", len(out), maxReplyChars)
	}
	if !strings.Contains(out, "[...truncated") {
		t.Errorf("expected truncation marker in output")
	}
}

// TestHandleStatus_Truncation — a Status.md whose Status-summary
// cell is itself > 2000 bytes must come back trimmed + marker.
// (Models the real docs/Status.md where the summary cell currently
// runs ~4 KB.)
func TestHandleStatus_Truncation(t *testing.T) {
	dir := t.TempDir()
	huge := strings.Repeat("X", maxReplyChars+800)
	body := "# Status\n\n| Field | Value |\n|---|---|\n| Last modified | 2026-05-22 |\n| Status summary | " + huge + " |\n"
	sp := writeTempFile(t, dir, "Status.md", body)
	c := &CommandsConfig{StatusPath: sp}
	reply, _, err := c.HandleStatus(context.Background())
	if err != nil {
		t.Fatalf("HandleStatus: %v", err)
	}
	if len(reply) > maxReplyChars {
		t.Errorf("reply exceeds maxReplyChars=%d: got %d bytes", maxReplyChars, len(reply))
	}
	if !strings.Contains(reply, "[...truncated") {
		t.Errorf("expected truncation marker in oversized reply")
	}
}

// TestExtractTableField_CaseInsensitive — the extractor matches the
// field name case-insensitively (so | status summary | x | works the
// same as | Status summary | x |).
func TestExtractTableField_CaseInsensitive(t *testing.T) {
	body := "| status summary | lower-case-row |\n"
	v, ok := extractTableField(body, "Status summary")
	if !ok {
		t.Fatal("expected match on case-insensitive lookup")
	}
	if v != "lower-case-row" {
		t.Errorf("got %q want lower-case-row", v)
	}
}

// TestExtractTableField_NoMatch — non-existent field returns ("", false).
func TestExtractTableField_NoMatch(t *testing.T) {
	body := "| Foo | bar |\n"
	v, ok := extractTableField(body, "Status summary")
	if ok {
		t.Errorf("expected !ok for missing field; got %q", v)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// --- T5 fixtures + helpers ---

const fixtureIssuesMd = `# Herald — Issues

| Field | Value |
|---|---|
| Revision | 99 |
| Last modified | 2026-05-22 |

## Open

| HRD | Title | Status | Last update |
|---|---|---|---|
| HRD-042 | Sentinel-issue-row for T5 happy-path | Open | 2026-05-22 |
| HRD-043 | Another open item — must stay in place | Open | 2026-05-22 |
| HRD-044 | Third open item — must stay in place | Open | 2026-05-22 |
`

const fixtureFixedMd = `# Herald — Fixed

| Field | Value |
|---|---|
| Revision | 99 |
| Last modified | 2026-05-22 |

## Closed

| HRD | Title | Status | Last update |
|---|---|---|---|
| HRD-001 | Pre-existing fixed row — must remain | Closed | 2026-05-20 |
| HRD-007 | Sentinel-reopen row for symmetric test | Closed | 2026-05-20 |
`

// sha256OfFile returns a hex digest of the bytes at path; used by
// rollback / non-operator tests to assert files were not mutated.
func sha256OfFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// makeOperatorConfig spins a CommandsConfig with the canned Issues +
// Fixed fixtures in t.TempDir, the senderID "op-1" in the operator
// allowlist, and a deterministic FakeClock.
func makeOperatorConfig(t *testing.T) (*CommandsConfig, string, string) {
	t.Helper()
	dir := t.TempDir()
	issuesPath := writeTempFile(t, dir, "Issues.md", fixtureIssuesMd)
	fixedPath := writeTempFile(t, dir, "Fixed.md", fixtureFixedMd)
	c := &CommandsConfig{
		IssuesPath:  issuesPath,
		FixedPath:   fixedPath,
		OperatorIDs: map[string]bool{"op-1": true},
		Clock:       newFixedClock(),
	}
	return c, issuesPath, fixedPath
}

// fixedClock is a deterministic Clock that always returns
// 2026-05-22T12:00:00Z. We don't reuse commons.NewFakeClock() because
// it anchors to 2026-05-20, and T5's annotateRowStatus emits today's
// date — the fixture timeline is anchored to 2026-05-22 (the date the
// plan landed).
type fixedClock struct{}

func newFixedClock() fixedClock { return fixedClock{} }
func (fixedClock) Now() time.Time                       { return time.Date(2026, time.May, 22, 12, 0, 0, 0, time.UTC) }
func (fixedClock) Since(t time.Time) time.Duration      { return time.Time{}.Sub(t) }
func (fixedClock) Sleep(time.Duration)                  {}
func (fixedClock) After(time.Duration) <-chan time.Time { return nil }
func (fixedClock) NewTimer(time.Duration) commons.Timer {
	return noopTimer{}
}

// noopTimer is a no-op Timer for tests that don't exercise timed paths.
type noopTimer struct{}

func (noopTimer) C() <-chan time.Time { return nil }
func (noopTimer) Stop() bool          { return true }

// TestHandleDone_HappyPath — operator senderID + canned Issues with
// HRD-042 + canned Fixed → HRD-042 row leaves Issues, appears in Fixed
// with "Closed (Wave 6.5)" + 2026-05-22 annotation. Other rows in
// Issues stay verbatim; pre-existing Fixed rows stay verbatim.
func TestHandleDone_HappyPath(t *testing.T) {
	c, issuesPath, fixedPath := makeOperatorConfig(t)
	reply, atts, err := c.HandleDone(context.Background(), "Done: HRD-042 — fixed it", "op-1")
	if err != nil {
		t.Fatalf("HandleDone: %v", err)
	}
	if atts != nil {
		t.Errorf("expected nil attachments, got %d", len(atts))
	}
	if !strings.Contains(reply, "HRD-042") || !strings.Contains(reply, "Issues.md → Fixed.md") {
		t.Errorf("unexpected reply: %q", reply)
	}

	issuesBytes, err := os.ReadFile(issuesPath)
	if err != nil {
		t.Fatalf("read Issues.md: %v", err)
	}
	if strings.Contains(string(issuesBytes), "| HRD-042 |") {
		t.Errorf("HRD-042 row still in Issues.md:\n%s", string(issuesBytes))
	}
	if !strings.Contains(string(issuesBytes), "| HRD-043 |") {
		t.Errorf("HRD-043 row missing from Issues.md (collateral damage):\n%s", string(issuesBytes))
	}
	if !strings.Contains(string(issuesBytes), "| HRD-044 |") {
		t.Errorf("HRD-044 row missing from Issues.md (collateral damage):\n%s", string(issuesBytes))
	}

	fixedBytes, err := os.ReadFile(fixedPath)
	if err != nil {
		t.Fatalf("read Fixed.md: %v", err)
	}
	if !strings.Contains(string(fixedBytes), "HRD-042") {
		t.Errorf("HRD-042 row not in Fixed.md:\n%s", string(fixedBytes))
	}
	if !strings.Contains(string(fixedBytes), "Closed (Wave 6.5)") {
		t.Errorf("expected 'Closed (Wave 6.5)' annotation; got:\n%s", string(fixedBytes))
	}
	if !strings.Contains(string(fixedBytes), "2026-05-22") {
		t.Errorf("expected date stamp 2026-05-22 in Fixed.md; got:\n%s", string(fixedBytes))
	}
	if !strings.Contains(string(fixedBytes), "| HRD-001 |") {
		t.Errorf("pre-existing HRD-001 row missing from Fixed.md (collateral damage):\n%s", string(fixedBytes))
	}

	// §107 anchor: no leftover tmp / bak files (cleanup ran).
	dir := filepath.Dir(issuesPath)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		name := e.Name()
		if strings.Contains(name, ".tmp.") || strings.Contains(name, ".bak.") {
			t.Errorf("leftover transient file after happy-path: %s", name)
		}
	}
}

// TestHandleDone_NonOperatorRejected — non-operator senderID is
// rejected BEFORE any file is touched. We snapshot Issues.md +
// Fixed.md sha256 + mtime before the call and assert nothing changed
// after. §107 anchor: proves the role check ran first.
func TestHandleDone_NonOperatorRejected(t *testing.T) {
	c, issuesPath, fixedPath := makeOperatorConfig(t)

	issuesHashBefore := sha256OfFile(t, issuesPath)
	fixedHashBefore := sha256OfFile(t, fixedPath)
	issuesInfoBefore, err := os.Stat(issuesPath)
	if err != nil {
		t.Fatalf("stat Issues.md: %v", err)
	}
	fixedInfoBefore, err := os.Stat(fixedPath)
	if err != nil {
		t.Fatalf("stat Fixed.md: %v", err)
	}

	_, _, err = c.HandleDone(context.Background(), "Done: HRD-042 — sneaky", "intruder-99")
	if err == nil {
		t.Fatal("expected ErrNotOperator, got nil")
	}
	if !errors.Is(err, ErrNotOperator) {
		t.Errorf("expected ErrNotOperator, got %v", err)
	}

	if got := sha256OfFile(t, issuesPath); got != issuesHashBefore {
		t.Errorf("Issues.md mutated by non-operator call: before=%s after=%s", issuesHashBefore, got)
	}
	if got := sha256OfFile(t, fixedPath); got != fixedHashBefore {
		t.Errorf("Fixed.md mutated by non-operator call: before=%s after=%s", fixedHashBefore, got)
	}
	issuesInfoAfter, _ := os.Stat(issuesPath)
	if !issuesInfoAfter.ModTime().Equal(issuesInfoBefore.ModTime()) {
		t.Errorf("Issues.md mtime changed: before=%s after=%s",
			issuesInfoBefore.ModTime(), issuesInfoAfter.ModTime())
	}
	fixedInfoAfter, _ := os.Stat(fixedPath)
	if !fixedInfoAfter.ModTime().Equal(fixedInfoBefore.ModTime()) {
		t.Errorf("Fixed.md mtime changed: before=%s after=%s",
			fixedInfoBefore.ModTime(), fixedInfoAfter.ModTime())
	}
}

// TestHandleDone_MissingHRDInIssues — HRD-999 is not in the canned
// Issues.md; the migrateRow helper returns an error. Both files MUST
// remain byte-identical to their pre-call state (no half-write).
func TestHandleDone_MissingHRDInIssues(t *testing.T) {
	c, issuesPath, fixedPath := makeOperatorConfig(t)

	issuesHashBefore := sha256OfFile(t, issuesPath)
	fixedHashBefore := sha256OfFile(t, fixedPath)

	_, _, err := c.HandleDone(context.Background(), "Done: HRD-999 — not here", "op-1")
	if err == nil {
		t.Fatal("expected error for missing HRD-999, got nil")
	}
	if !strings.Contains(err.Error(), "HRD-999") {
		t.Errorf("error should mention HRD-999; got: %v", err)
	}

	if got := sha256OfFile(t, issuesPath); got != issuesHashBefore {
		t.Errorf("Issues.md mutated despite missing HRD: before=%s after=%s", issuesHashBefore, got)
	}
	if got := sha256OfFile(t, fixedPath); got != fixedHashBefore {
		t.Errorf("Fixed.md mutated despite missing HRD: before=%s after=%s", fixedHashBefore, got)
	}
}

// TestHandleDone_RollbackOnSecondRenameFailure — inject a c.Rename
// that fails on its 2nd invocation. The migrateRow sequence is:
//
//	1: rename(src, srcBackup)         ← succeeds (1st call)
//	2: rename(srcTmp, src)            ← fails    (2nd call — INJECTED)
//	3: (would be) rename(dstTmp, dst) ← never reached
//
// Rollback path: rename(srcBackup, src) — restores the original
// source bytes. The destination file is never touched (the dst rename
// happens AFTER the src rename, which failed). §107 anchor: assert
// Issues.md hash matches the original.
func TestHandleDone_RollbackOnSecondRenameFailure(t *testing.T) {
	c, issuesPath, fixedPath := makeOperatorConfig(t)

	issuesHashBefore := sha256OfFile(t, issuesPath)
	fixedHashBefore := sha256OfFile(t, fixedPath)

	calls := 0
	c.Rename = func(old, new string) error {
		calls++
		if calls == 2 {
			return errors.New("injected 2nd-call rename failure (T5 rollback test)")
		}
		return os.Rename(old, new)
	}

	_, _, err := c.HandleDone(context.Background(), "Done: HRD-042 — should rollback", "op-1")
	if err == nil {
		t.Fatal("expected injected rename failure, got nil")
	}
	if !strings.Contains(err.Error(), "rename src") {
		t.Errorf("expected 'rename src' tag in error (2nd-call path); got: %v", err)
	}

	if got := sha256OfFile(t, issuesPath); got != issuesHashBefore {
		t.Errorf("ROLLBACK FAILED: Issues.md not restored to original.\n"+
			"before=%s\nafter=%s\ncalls=%d", issuesHashBefore, got, calls)
	}
	if got := sha256OfFile(t, fixedPath); got != fixedHashBefore {
		t.Errorf("Fixed.md mutated despite rollback path: before=%s after=%s", fixedHashBefore, got)
	}

	// No leftover .tmp.* / .bak.* in the directory (cleanup ran).
	dir := filepath.Dir(issuesPath)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".tmp."+name) || strings.Contains(name, ".tmp.") {
			// A .tmp.* file leaked → cleanup path missed it.
			t.Errorf("leftover tmp file after rollback: %s", name)
		}
	}
}

// TestHandleReopen_HappyPath — symmetric to HandleDone. HRD-007 in
// the canned Fixed.md moves into Issues.md with the "Reopened (Wave 6.5)"
// annotation + date stamp.
func TestHandleReopen_HappyPath(t *testing.T) {
	c, issuesPath, fixedPath := makeOperatorConfig(t)
	reply, _, err := c.HandleReopen(context.Background(), "Reopen: HRD-007 because regression", "op-1")
	if err != nil {
		t.Fatalf("HandleReopen: %v", err)
	}
	if !strings.Contains(reply, "HRD-007") || !strings.Contains(reply, "Fixed.md → Issues.md") {
		t.Errorf("unexpected reply: %q", reply)
	}

	fixedBytes, err := os.ReadFile(fixedPath)
	if err != nil {
		t.Fatalf("read Fixed.md: %v", err)
	}
	if strings.Contains(string(fixedBytes), "| HRD-007 |") {
		t.Errorf("HRD-007 row still in Fixed.md:\n%s", string(fixedBytes))
	}
	if !strings.Contains(string(fixedBytes), "| HRD-001 |") {
		t.Errorf("HRD-001 row missing from Fixed.md (collateral damage):\n%s", string(fixedBytes))
	}

	issuesBytes, err := os.ReadFile(issuesPath)
	if err != nil {
		t.Fatalf("read Issues.md: %v", err)
	}
	if !strings.Contains(string(issuesBytes), "HRD-007") {
		t.Errorf("HRD-007 row not in Issues.md:\n%s", string(issuesBytes))
	}
	if !strings.Contains(string(issuesBytes), "Reopened (Wave 6.5)") {
		t.Errorf("expected 'Reopened (Wave 6.5)' annotation; got:\n%s", string(issuesBytes))
	}
	if !strings.Contains(string(issuesBytes), "2026-05-22") {
		t.Errorf("expected date stamp 2026-05-22 in Issues.md; got:\n%s", string(issuesBytes))
	}
}

// TestParseHRDRef — the regex-based extractor matches the first
// HRD-NNN substring regardless of surrounding text.
func TestParseHRDRef(t *testing.T) {
	cases := []struct {
		body string
		want string
		ok   bool
	}{
		{"Done: HRD-042", "HRD-042", true},
		{"Done: HRD-042 — fixed it", "HRD-042", true},
		{"Reopen: HRD-7 because reason", "HRD-7", true},
		{"random prose with HRD-12345 buried inside", "HRD-12345", true},
		{"no reference here", "", false},
		{"HRD-X is not a match (no digits)", "", false},
	}
	for _, tc := range cases {
		got, err := parseHRDRef(tc.body)
		if tc.ok {
			if err != nil {
				t.Errorf("parseHRDRef(%q) unexpected error: %v", tc.body, err)
			}
			if got != tc.want {
				t.Errorf("parseHRDRef(%q) = %q, want %q", tc.body, got, tc.want)
			}
		} else {
			if err == nil {
				t.Errorf("parseHRDRef(%q) expected error, got %q", tc.body, got)
			}
		}
	}
}

// TestRequireOperator — operator presence map gates Done/Reopen access.
func TestRequireOperator(t *testing.T) {
	c := &CommandsConfig{OperatorIDs: map[string]bool{"op-1": true}}
	if err := c.requireOperator("op-1"); err != nil {
		t.Errorf("op-1 should be allowed; got %v", err)
	}
	if err := c.requireOperator("nobody"); !errors.Is(err, ErrNotOperator) {
		t.Errorf("'nobody' should be rejected with ErrNotOperator; got %v", err)
	}
	// Nil map → reject all.
	cc := &CommandsConfig{}
	if err := cc.requireOperator("anybody"); !errors.Is(err, ErrNotOperator) {
		t.Errorf("nil OperatorIDs should reject all; got %v", err)
	}
}
