package wizard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaskValue(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "(empty)"},
		{"abc", "***"},
		{"abcdef", "***"},
		{"abcdefgh", "abc...***"},
		{"abcdefghijklmnop", "abc...***"},
		{"abcdefghijklmnopq", "abcdef...opq"},
		// Synthetic Telegram-token-shaped string (NOT a real token; Bot IDs in
		// this format are reserved for fixtures per §11.4.10 — no leak).
		{"0000000000:XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX", "000000...XXX"},
	}
	for _, c := range cases {
		got := MaskValue(c.in)
		if got != c.want {
			t.Errorf("MaskValue(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHasExportAndAppend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".zshrc")

	// Empty file should report HasExport=false
	has, err := HasExport(path, "HERALD_TGRAM_BOT_TOKEN")
	if err != nil {
		t.Fatalf("HasExport on missing: %v", err)
	}
	if has {
		t.Fatal("HasExport on missing file should return false")
	}

	// Append once
	if err := AppendExport(path, "HERALD_TGRAM_BOT_TOKEN", "test-value-A", "Herald — Telegram (test)"); err != nil {
		t.Fatalf("AppendExport: %v", err)
	}

	// Verify line written + readable
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), "export HERALD_TGRAM_BOT_TOKEN='test-value-A'") {
		t.Fatalf("appended line not found in file:\n%s", string(data))
	}

	// Second append should return ErrAlreadyExported (idempotency)
	err = AppendExport(path, "HERALD_TGRAM_BOT_TOKEN", "test-value-B", "")
	if err != ErrAlreadyExported {
		t.Fatalf("expected ErrAlreadyExported, got %v", err)
	}
}

func TestReplaceExport(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".zshrc")

	// Seed with an existing export
	if err := AppendExport(path, "HERALD_TGRAM_BOT_TOKEN", "old-value", ""); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Replace
	if err := ReplaceExport(path, "HERALD_TGRAM_BOT_TOKEN", "new-value", "Herald — Telegram (replaced)"); err != nil {
		t.Fatalf("ReplaceExport: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(data), "old-value") {
		t.Fatalf("old value still present after Replace:\n%s", string(data))
	}
	if !strings.Contains(string(data), "export HERALD_TGRAM_BOT_TOKEN='new-value'") {
		t.Fatalf("new value not present:\n%s", string(data))
	}
}

func TestReplaceExport_QuoteEscaping(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".zshrc")

	// Value with an embedded single quote — must be escaped via the POSIX trick.
	value := "I'm a tricky value"
	if err := AppendExport(path, "HERALD_TEST_VAR", value, ""); err != nil {
		t.Fatalf("AppendExport: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	want := `export HERALD_TEST_VAR='I'\''m a tricky value'`
	if !strings.Contains(string(data), want) {
		t.Fatalf("expected escaped form %q in file, got:\n%s", want, string(data))
	}
}

func TestDetectShellTargets(t *testing.T) {
	targets, err := DetectShellTargets()
	if err != nil {
		t.Fatalf("DetectShellTargets: %v", err)
	}
	if len(targets) < 2 {
		t.Fatalf("expected at least 2 candidate targets, got %d", len(targets))
	}
	for _, tg := range targets {
		if tg.Path == "" || tg.DisplayName == "" || tg.ShellKind == "" {
			t.Errorf("incomplete target: %+v", tg)
		}
	}
}
