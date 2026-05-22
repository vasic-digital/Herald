// Package inbound — commands_test.go: §107 anti-bluff coverage for the
// Wave 6.5 T4 fast-path handlers. Every test writes a canned Markdown
// fixture into t.TempDir() with sentinel content and asserts the
// handler returns bytes drawn from that fixture — a hard-coded reply
// would fail because the sentinels are test-unique.
package inbound

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
