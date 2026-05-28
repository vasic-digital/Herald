package mtproto

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Validate_Rejects_BadConfigs ensures every required-field omission is caught
// + every malformed value is rejected, with NO credential bytes echoed in
// the error (HRD-133 parity).
func TestConfig_Validate_Rejects_BadConfigs(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{"missing AppID", Config{AppHash: "0123456789abcdef0123456789abcdef", Phone: "+12025551234"}},
		{"negative AppID", Config{AppID: -1, AppHash: "0123456789abcdef0123456789abcdef", Phone: "+12025551234"}},
		{"missing AppHash", Config{AppID: 12345, Phone: "+12025551234"}},
		{"wrong-length AppHash", Config{AppID: 12345, AppHash: "abcd", Phone: "+12025551234"}},
		{"non-hex AppHash", Config{AppID: 12345, AppHash: "ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ", Phone: "+12025551234"}},
		{"missing Phone", Config{AppID: 12345, AppHash: "0123456789abcdef0123456789abcdef"}},
		{"Phone without +", Config{AppID: 12345, AppHash: "0123456789abcdef0123456789abcdef", Phone: "12025551234"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if err == nil {
				t.Fatalf("Validate() returned nil for %s; want error", tc.name)
			}
			if ContainsSecret(err.Error()) {
				t.Errorf("Validate() error LEAKED credential bytes: %q", err.Error())
			}
		})
	}
}

func TestConfig_Validate_AcceptsValid(t *testing.T) {
	cfg := Config{
		AppID:    12345678,
		AppHash:  "0123456789abcdef0123456789abcdef",
		Phone:    "+12025551234",
		Password: "",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() rejected valid config: %v", err)
	}
}

// TestNew_ScaffoldRejectsBadConfig confirms the constructor surfaces
// ErrInvalidConfig (so the Track-B-incomplete state never returns a
// silently-broken Client). §107 anti-bluff: never construct an unusable
// Client.
func TestNew_ScaffoldRejectsBadConfig(t *testing.T) {
	_, err := New(Config{AppID: 0})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("want ErrInvalidConfig, got %v", err)
	}
}

// TestNew_ScaffoldReturnsErrNotImplemented confirms that during the Track-B
// scaffold phase (no gotd/td wiring yet) every runtime method returns
// ErrNotImplemented — loudly visible, never silent no-op. §107 anti-bluff.
func TestNew_ScaffoldReturnsErrNotImplemented(t *testing.T) {
	cfg := Config{
		AppID:   12345678,
		AppHash: "0123456789abcdef0123456789abcdef",
		Phone:   "+12025551234",
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	ctx := context.Background()

	if err := c.Connect(ctx); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("Connect: want ErrNotImplemented, got %v", err)
	}
	if _, err := c.SendMessage(ctx, -100123456, "test"); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("SendMessage: want ErrNotImplemented, got %v", err)
	}
	if _, err := c.WaitForReply(ctx, -100123456, func(Message) bool { return true }); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("WaitForReply: want ErrNotImplemented, got %v", err)
	}
	if _, _, err := c.WhoAmI(ctx); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("WhoAmI: want ErrNotImplemented, got %v", err)
	}
}

// TestSanitizeString_RedactsApiHashShape — HRD-133 parity guard.
func TestSanitizeString_RedactsApiHashShape(t *testing.T) {
	input := "MTProto error: api_hash 0123456789abcdef0123456789abcdef was rejected"
	out := sanitizeString(input)
	if strings.Contains(out, "0123456789abcdef0123456789abcdef") {
		t.Errorf("api_hash NOT redacted: %q", out)
	}
	if !strings.Contains(out, "<redacted-api-hash>") {
		t.Errorf("redaction marker missing: %q", out)
	}
}

// TestSanitizeString_RedactsBotTokenShape — defense in depth: if an MTProto
// error transitively includes a bot token (e.g. through a wrapped tgram
// error), it MUST still be redacted.
func TestSanitizeString_RedactsBotTokenShape(t *testing.T) {
	input := "wrapped: send failed using 1234567890:ABCdefGHIjklMNOpqrSTUvwxYZ_abcdefghij"
	out := sanitizeString(input)
	if strings.Contains(out, "1234567890:ABCdefGHIjkl") {
		t.Errorf("bot token NOT redacted: %q", out)
	}
}

// TestSanitizeString_RedactsSessionTokenShape — base64-ish session bytes
// MUST be redacted.
func TestSanitizeString_RedactsSessionTokenShape(t *testing.T) {
	// 80-char base64-ish string
	sessionBytes := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	input := "session restore failed: " + sessionBytes + " is corrupt"
	out := sanitizeString(input)
	if strings.Contains(out, sessionBytes) {
		t.Errorf("session token NOT redacted: %q", out)
	}
}

// TestSanitizeString_LeavesNonSecretsAlone — chat_ids and error codes are
// useful for debugging and MUST NOT be redacted.
func TestSanitizeString_LeavesNonSecretsAlone(t *testing.T) {
	input := "FLOOD_WAIT_30: rate-limited on chat -1001234567890 for user 987654321"
	out := sanitizeString(input)
	if out != input {
		t.Errorf("non-secret content was redacted:\n  in:  %q\n  out: %q", input, out)
	}
}

// TestSanitizeMTProtoError_PreservesErrorsIs — wrapping MUST preserve the
// errors.Is chain so callers can still pattern-match on sentinels.
func TestSanitizeMTProtoError_PreservesErrorsIs(t *testing.T) {
	root := errors.New("inner: 0123456789abcdef0123456789abcdef leaked")
	wrapped := sanitizeMTProtoError(root)

	if wrapped == nil {
		t.Fatal("wrapped is nil")
	}
	if strings.Contains(wrapped.Error(), "0123456789abcdef0123456789abcdef") {
		t.Errorf("redaction failed: %q", wrapped.Error())
	}
	// Unwrap MUST reach the root.
	if !errors.Is(wrapped, root) {
		t.Errorf("errors.Is chain broken")
	}
}

// TestSanitizeMTProtoError_NilSafe — defensive idempotency.
func TestSanitizeMTProtoError_NilSafe(t *testing.T) {
	if got := sanitizeMTProtoError(nil); got != nil {
		t.Errorf("sanitizeMTProtoError(nil) = %v; want nil", got)
	}
}

// TestSanitizeMTProtoError_NoRedactionReturnsOriginal — efficient path:
// if no redaction is needed, return the original error unmodified to
// preserve identity.
func TestSanitizeMTProtoError_NoRedactionReturnsOriginal(t *testing.T) {
	plain := errors.New("connection refused on chat -1001234567890")
	wrapped := sanitizeMTProtoError(plain)
	if wrapped != plain {
		t.Errorf("expected original error preserved when no redaction needed; got %v vs %v", wrapped, plain)
	}
}

// TestFloodWaitError_IsErrFloodWait — typed-error matchability.
func TestFloodWaitError_IsErrFloodWait(t *testing.T) {
	e := &FloodWaitError{RetryAfter: 30 * time.Second}
	if !errors.Is(e, ErrFloodWait) {
		t.Errorf("FloodWaitError does not match ErrFloodWait")
	}
	var fwe *FloodWaitError
	if !errors.As(e, &fwe) {
		t.Errorf("errors.As extraction failed")
	}
	if fwe.RetryAfter != 30*time.Second {
		t.Errorf("RetryAfter lost in As: got %v", fwe.RetryAfter)
	}
}

// TestDefaultSessionFile_RespectsHome — hermetic; uses t.TempDir + sets HOME.
func TestDefaultSessionFile_RespectsHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	got := DefaultSessionFile()
	want := filepath.Join(tmp, ".config", "herald", "mtproto.session")
	if got != want {
		t.Errorf("DefaultSessionFile = %q; want %q", got, want)
	}
}

// TestConfig_ResolvedSessionFile_PrefersExplicit — when Config.SessionFile
// is set, it wins over the default.
func TestConfig_ResolvedSessionFile_PrefersExplicit(t *testing.T) {
	cfg := Config{SessionFile: "/tmp/custom.session"}
	if got := cfg.ResolvedSessionFile(); got != "/tmp/custom.session" {
		t.Errorf("got %q; want /tmp/custom.session", got)
	}
}

// TestConfig_SessionExists_MissingFile — returns (false, nil) for absent
// session.
func TestConfig_SessionExists_MissingFile(t *testing.T) {
	tmp := t.TempDir()
	cfg := Config{SessionFile: filepath.Join(tmp, "does-not-exist.session")}
	exists, err := cfg.SessionExists()
	if err != nil {
		t.Fatalf("SessionExists: %v", err)
	}
	if exists {
		t.Errorf("SessionExists() = true; want false for missing file")
	}
}

// TestConfig_SessionExists_PresentFile — returns (true, nil) for existing
// session file.
func TestConfig_SessionExists_PresentFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "session.bin")
	if err := os.WriteFile(path, []byte("opaque session bytes"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg := Config{SessionFile: path}
	exists, err := cfg.SessionExists()
	if err != nil {
		t.Fatalf("SessionExists: %v", err)
	}
	if !exists {
		t.Errorf("SessionExists() = false; want true for present file")
	}
}

// TestConfig_EnsureSessionDir_CreatesPath — pre-create the ~/.config/herald
// parent path with mode 0700.
func TestConfig_EnsureSessionDir_CreatesPath(t *testing.T) {
	tmp := t.TempDir()
	cfg := Config{SessionFile: filepath.Join(tmp, "deeply", "nested", "session")}
	if err := cfg.EnsureSessionDir(); err != nil {
		t.Fatalf("EnsureSessionDir: %v", err)
	}
	info, err := os.Stat(filepath.Join(tmp, "deeply", "nested"))
	if err != nil {
		t.Fatalf("dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("expected directory, got %v", info.Mode())
	}
	// Permission check is platform-dependent; mode 0700 is the goal.
	if mode := info.Mode().Perm(); mode != 0700 {
		t.Logf("note: parent dir mode = %o; expected 0700 (platform may force umask narrowing)", mode)
	}
}
