package claude_code

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestBootstrapTimeoutDefaultsCorrectly proves the default budget is
// what claude_code.go documents, so a future refactor can't silently
// shrink it below the spec §33.2 step-4 budget.
func TestBootstrapTimeoutDefaultsCorrectly(t *testing.T) {
	d, err := New("claude", t.TempDir(), "TestProj")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got, want := d.bootstrapTimeoutOrDefault(), DefaultBootstrapTimeout; got != want {
		t.Fatalf("bootstrapTimeoutOrDefault = %s, want %s", got, want)
	}
	if DefaultBootstrapTimeout != 60*time.Second {
		t.Fatalf("DefaultBootstrapTimeout = %s, want 60s (spec §33.2 step-4)", DefaultBootstrapTimeout)
	}
}

// TestSetBootstrapTimeoutRoundTrip proves the setter accepts a positive
// override and resets-to-default on a non-positive value (so callers
// can clear an override by passing 0).
func TestSetBootstrapTimeoutRoundTrip(t *testing.T) {
	d, _ := New("claude", t.TempDir(), "TestProj")
	d.SetBootstrapTimeout(15 * time.Second)
	if got := d.bootstrapTimeoutOrDefault(); got != 15*time.Second {
		t.Errorf("after SetBootstrapTimeout(15s): %s, want 15s", got)
	}
	d.SetBootstrapTimeout(0)
	if got := d.bootstrapTimeoutOrDefault(); got != DefaultBootstrapTimeout {
		t.Errorf("after SetBootstrapTimeout(0): %s, want default %s", got, DefaultBootstrapTimeout)
	}
	d.SetBootstrapTimeout(-1 * time.Second)
	if got := d.bootstrapTimeoutOrDefault(); got != DefaultBootstrapTimeout {
		t.Errorf("after SetBootstrapTimeout(-1s): %s, want default %s", got, DefaultBootstrapTimeout)
	}
}

// TestBootstrapPersistsAnchorOnSuccess uses a fake binary that prints a
// non-empty stdout body and exits 0, then verifies bootstrapSession
// persisted the anchor file under the dispatcher's working dir with
// the returned UUID. This is the unit-level §107 evidence that
// bootstrap actually writes through PersistSession (no parallel
// write path).
func TestBootstrapPersistsAnchorOnSuccess(t *testing.T) {
	fakeBin := writeFakeClaudeBinary(t, 0, "<<<HERALD-REPLY>>> ack\n")
	workdir := t.TempDir()
	d, err := New(fakeBin, workdir, "BootstrapProj")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, anchor, _ := d.ResolveSession()
	if _, err := os.Stat(anchor); !os.IsNotExist(err) {
		t.Fatalf("anchor must not exist before bootstrap; got stat err=%v", err)
	}

	gotUUID, err := d.bootstrapSession(t.Context(), anchor)
	if err != nil {
		t.Fatalf("bootstrapSession: %v", err)
	}
	if gotUUID == uuid.Nil {
		t.Fatal("bootstrapSession returned uuid.Nil with err=nil — invariant violation")
	}

	raw, err := os.ReadFile(anchor)
	if err != nil {
		t.Fatalf("read anchor: %v", err)
	}
	parsed, err := uuid.Parse(string(raw[:36]))
	if err != nil {
		t.Fatalf("anchor body not a UUID: %v (raw=%q)", err, string(raw))
	}
	if parsed != gotUUID {
		t.Fatalf("anchor UUID %s != returned UUID %s", parsed, gotUUID)
	}

	// Re-resolution should now return the bootstrapped UUID without
	// touching the binary — proves PersistSession + ResolveSession
	// agree on the on-disk format.
	resolved, _, err := d.ResolveSession()
	if err != nil {
		t.Fatalf("ResolveSession after bootstrap: %v", err)
	}
	if resolved != gotUUID {
		t.Fatalf("ResolveSession returned %s, want %s", resolved, gotUUID)
	}
}

// TestBootstrapFailsOnEmptyStdout proves the §107 bluff guard fires
// when the fake binary exits 0 but emits nothing — without this guard,
// a silently-broken claude install would produce a "successful"
// bootstrap with an orphan anchor and a session that resume cannot
// recover.
func TestBootstrapFailsOnEmptyStdout(t *testing.T) {
	fakeBin := writeFakeClaudeBinary(t, 0, "" /* empty stdout */)
	d, err := New(fakeBin, t.TempDir(), "EmptyStdoutProj")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, anchor, _ := d.ResolveSession()
	if _, err := d.bootstrapSession(t.Context(), anchor); err == nil {
		t.Fatal("bootstrapSession with empty stdout MUST return error (§107 bluff guard)")
	}
	if _, err := os.Stat(anchor); !os.IsNotExist(err) {
		t.Errorf("anchor MUST NOT be persisted when bootstrap fails; stat err=%v", err)
	}
}

// TestBootstrapFailsOnNonZeroExit proves a non-zero-exit claude
// subprocess is surfaced as an error (with stderr) and does NOT
// persist an anchor.
func TestBootstrapFailsOnNonZeroExit(t *testing.T) {
	fakeBin := writeFakeClaudeBinaryStderr(t, 1, "auth failure: token rejected")
	d, err := New(fakeBin, t.TempDir(), "NonZeroExitProj")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, anchor, _ := d.ResolveSession()
	_, err = d.bootstrapSession(t.Context(), anchor)
	if err == nil {
		t.Fatal("bootstrapSession with non-zero exit MUST return error")
	}
	if !contains(err.Error(), "auth failure: token rejected") {
		t.Errorf("error MUST include stderr verbatim for diagnostics; got: %v", err)
	}
	if _, statErr := os.Stat(anchor); !os.IsNotExist(statErr) {
		t.Errorf("anchor MUST NOT be persisted when bootstrap fails; stat err=%v", statErr)
	}
}

// TestDispatchBootstrapsThenInvokesResume proves the buildCmd happy
// path: when no anchor exists, bootstrap runs THEN buildCmd returns a
// command whose argv contains "--resume <new-uuid>" (not
// "--session-id"). This is the end-to-end glue assertion — the prior
// PASS-bluff was specifically that this glue was missing.
func TestDispatchBootstrapsThenInvokesResume(t *testing.T) {
	fakeBin := writeFakeClaudeBinary(t, 0, "<<<HERALD-REPLY>>> ack\n")
	workdir := t.TempDir()
	d, err := New(fakeBin, workdir, "GlueProj")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, anchor, _ := d.ResolveSession()
	if _, err := os.Stat(anchor); !os.IsNotExist(err) {
		t.Fatalf("anchor must not exist pre-test; got err=%v", err)
	}

	cmd, err := d.BuildCmdForTest(t.Context(), DispatchRequest{
		InboundID:    "INB-GLUE-1",
		Sender:       "tgram:glue-test",
		Channel:      "tgram",
		UserMessage:  "hi",
		Conversation: "(none)",
		Classification: Classification{
			Type: "query", Criticality: "low", Confidence: 0.5,
		},
	})
	if err != nil {
		t.Fatalf("BuildCmdForTest: %v", err)
	}

	args := cmd.Args
	resumeIdx := -1
	for i, a := range args {
		if a == "--resume" {
			resumeIdx = i
			break
		}
	}
	if resumeIdx < 0 || resumeIdx+1 >= len(args) {
		t.Fatalf("expected --resume <uuid> in argv after bootstrap; got: %v", args)
	}
	if _, err := uuid.Parse(args[resumeIdx+1]); err != nil {
		t.Fatalf("argv[%d]=%q is not a UUID: %v", resumeIdx+1, args[resumeIdx+1], err)
	}

	// And the anchor file should now exist with that same UUID.
	raw, err := os.ReadFile(anchor)
	if err != nil {
		t.Fatalf("read anchor after bootstrap: %v", err)
	}
	parsed, err := uuid.Parse(string(raw[:36]))
	if err != nil {
		t.Fatalf("anchor body not a UUID: %v", err)
	}
	if parsed.String() != args[resumeIdx+1] {
		t.Fatalf("argv --resume %s but anchor persisted %s", args[resumeIdx+1], parsed)
	}
}

// --- helpers ---------------------------------------------------------

// writeFakeClaudeBinary writes a shell script that prints `stdoutBody`
// to stdout and exits with `exitCode`. The script is made executable.
// Used by unit tests to exercise bootstrap without spawning real claude.
func writeFakeClaudeBinary(t *testing.T, exitCode int, stdoutBody string) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "fake-claude")
	script := "#!/bin/sh\n"
	if stdoutBody != "" {
		// Use printf to preserve trailing newline semantics exactly.
		script += "printf '%s' " + shellQuote(stdoutBody) + "\n"
	}
	script += "exit " + itoa(exitCode) + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	return bin
}

// writeFakeClaudeBinaryStderr is like writeFakeClaudeBinary but writes
// to stderr and exits non-zero — used to exercise the error path that
// reports stderr verbatim.
func writeFakeClaudeBinaryStderr(t *testing.T, exitCode int, stderrBody string) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "fake-claude-stderr")
	script := "#!/bin/sh\n"
	if stderrBody != "" {
		script += "printf '%s' " + shellQuote(stderrBody) + " 1>&2\n"
	}
	script += "exit " + itoa(exitCode) + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	return bin
}

func shellQuote(s string) string {
	// POSIX single-quote escaping: ' -> '\''
	var b []byte
	b = append(b, '\'')
	for _, c := range []byte(s) {
		if c == '\'' {
			b = append(b, '\'', '\\', '\'', '\'')
		} else {
			b = append(b, c)
		}
	}
	b = append(b, '\'')
	return string(b)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

func contains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
