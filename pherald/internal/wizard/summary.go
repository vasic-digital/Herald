package wizard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SummaryEntry records a single credential the wizard configured.
// Values are stored MASKED via MaskValue — the raw secret never reaches
// the summary file. The summary lives at ~/.herald/credentials.md
// (chmod 600) and is git-ignored by convention.
type SummaryEntry struct {
	Service     string    // "telegram" | "claude_code" | ...
	VarName     string    // "HERALD_TGRAM_BOT_TOKEN"
	MaskedValue string    // "882338...jfU" — never the raw value
	ShellFile   string    // "/Users/alice/.zshrc"
	Timestamp   time.Time // when the wizard wrote this entry
	Notes       string    // free-form, e.g. "DM chat_id 2057253161; group setup pending"
}

// SummaryPath returns the canonical path to the operator's credentials
// summary file: $HOME/.herald/credentials.md. Per §11.4.10 this file
// MUST NOT be committed to git. The directory and file are created
// chmod 700 / chmod 600 respectively.
func SummaryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("wizard.SummaryPath: %w", err)
	}
	return filepath.Join(home, ".herald", "credentials.md"), nil
}

// AppendSummary appends one entry to the credentials summary file.
// Creates the directory + file with restrictive permissions if absent.
// §107: re-reads the file after write and confirms the entry is present.
func AppendSummary(e SummaryEntry) error {
	path, err := SummaryPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("wizard.AppendSummary: mkdir: %w", err)
	}

	exists := true
	if _, err := os.Stat(path); os.IsNotExist(err) {
		exists = false
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("wizard.AppendSummary: open: %w", err)
	}
	defer f.Close()

	var sb strings.Builder
	if !exists {
		sb.WriteString("# Herald — Operator Credentials Summary\n\n")
		sb.WriteString("This file lives at `$HOME/.herald/credentials.md`. It is **never** committed to git.\n\n")
		sb.WriteString("Per Universal Constitution §11.4.10: credentials are not tracked. This summary captures only MASKED values + provenance metadata so operators have a single place to recall which env vars are set + when.\n\n")
		sb.WriteString("## Entries\n\n")
	}

	sb.WriteString(fmt.Sprintf("### %s — %s\n\n", e.Service, e.VarName))
	sb.WriteString(fmt.Sprintf("- Masked value: `%s`\n", e.MaskedValue))
	sb.WriteString(fmt.Sprintf("- Written to:   `%s`\n", e.ShellFile))
	sb.WriteString(fmt.Sprintf("- Timestamp:    `%s`\n", e.Timestamp.UTC().Format(time.RFC3339)))
	if e.Notes != "" {
		sb.WriteString(fmt.Sprintf("- Notes:        %s\n", e.Notes))
	}
	sb.WriteString("\n")

	if _, err := f.WriteString(sb.String()); err != nil {
		return fmt.Errorf("wizard.AppendSummary: write: %w", err)
	}

	// §107 verification.
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !strings.Contains(string(data), e.VarName) || !strings.Contains(string(data), e.MaskedValue) {
		return fmt.Errorf("wizard.AppendSummary: post-write verification failed (§107 bluff guard)")
	}
	return nil
}
