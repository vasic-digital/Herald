package main

// HRD-037/039/048/050/052 — §43 docs-pipeline command bodies (v1.0.0 Batch C, C3a).
//
// EVERY test is 100% HERMETIC. Doc fixtures (.md trees, Issues.md/Fixed.md/
// Fixed_Summary.md, fake export scripts) are built under t.TempDir(); the
// export/docs-sync tests point --script at a tiny fake shell script that needs
// NO pandoc (it echoes a marker + touches output siblings). NO test runs
// --apply against the real Herald checkout or invokes the real export pipeline.
//
// §107 anti-bluff: each test asserts the REAL command behaviour — the verdict
// line AND the exit code (PASS ⇒ nil error; FAIL ⇒ non-nil error carrying the
// §-rule breach). A command that prints FAIL but exits 0 would be a §107
// PASS-bluff; these tests assert the error is returned on FAIL.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// runCmd executes a freshly-built command with args, capturing combined output.
func runCmd(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

// writeDoc writes a doc at repo/rel, creating parent dirs.
func writeDoc(t *testing.T, repo, rel, content string) string {
	t.Helper()
	p := filepath.Join(repo, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// metadataDoc is a §11.4.61-compliant doc (Revision row + ToC heading).
const metadataDoc = "# Doc\n\n| Field | Value |\n|---|---|\n| Revision | 1 |\n\n## Table of contents\n\n- intro\n"

// barDoc is a doc MISSING the §11.4.61 metadata block + ToC.
const barDoc = "# Doc\n\nbody only, no metadata, no ToC\n"

// fakeExportScript writes an executable shell script that echoes a marker and
// touches a sentinel file (proving invocation) — no pandoc needed.
func fakeExportScript(t *testing.T, repo string) (scriptPath, sentinel string) {
	t.Helper()
	sentinel = filepath.Join(repo, "export-ran.sentinel")
	scriptPath = filepath.Join(repo, "fake_export.sh")
	body := "#!/bin/sh\necho 'FAKE-EXPORT-INVOKED args:' \"$@\"\ntouch '" + sentinel + "'\n"
	if err := os.WriteFile(scriptPath, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return scriptPath, sentinel
}

// failingExportScript writes an executable shell script that exits non-zero.
func failingExportScript(t *testing.T, repo string) string {
	t.Helper()
	scriptPath := filepath.Join(repo, "fail_export.sh")
	body := "#!/bin/sh\necho 'FAKE-EXPORT-FAILING' >&2\nexit 7\n"
	if err := os.WriteFile(scriptPath, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return scriptPath
}

// --- HRD-037 docs-sync (§11.4.61) ---

func TestDocsSync_MetadataPresent_Allows(t *testing.T) {
	repo := t.TempDir()
	writeDoc(t, repo, "docs/good.md", metadataDoc)
	out, err := runCmd(t, newDocsSyncCmd(), "--repo", repo, "docs/good.md")
	if err != nil {
		t.Fatalf("docs-sync with metadata should PASS (exit 0), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[PASS] §11.4.61") {
		t.Fatalf("expected §11.4.61 PASS verdict line:\n%s", out)
	}
}

func TestDocsSync_MissingMetadata_Blocks(t *testing.T) {
	repo := t.TempDir()
	writeDoc(t, repo, "docs/bad.md", barDoc)
	out, err := runCmd(t, newDocsSyncCmd(), "--repo", repo, "docs/bad.md")
	if err == nil || !strings.Contains(err.Error(), "§11.4.61") {
		t.Fatalf("docs-sync missing metadata should BLOCK (§11.4.61 breach), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[FAIL] §11.4.61") {
		t.Fatalf("expected §11.4.61 FAIL verdict line:\n%s", out)
	}
	if !strings.Contains(out, "missing-metadata") {
		t.Fatalf("expected missing-metadata evidence:\n%s", out)
	}
}

func TestDocsSync_Emit_DrivesEvent(t *testing.T) {
	repo := t.TempDir()
	writeDoc(t, repo, "docs/good.md", metadataDoc)
	out, err := runCmd(t, newDocsSyncCmd(), "--repo", repo, "--emit", "docs/good.md")
	if err != nil {
		t.Fatalf("docs-sync --emit PASS should exit 0, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[emit] §11.4.61 drove a constitution event") {
		t.Fatalf("expected emit line:\n%s", out)
	}
}

func TestDocsSync_ApplyInvokesScript(t *testing.T) {
	repo := t.TempDir()
	writeDoc(t, repo, "docs/good.md", metadataDoc)
	script, sentinel := fakeExportScript(t, repo)
	out, err := runCmd(t, newDocsSyncCmd(), "--repo", repo, "--apply", "--script", script, "docs/good.md")
	if err != nil {
		t.Fatalf("docs-sync --apply (clean docs) should PASS, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "FAKE-EXPORT-INVOKED") {
		t.Fatalf("expected fake export script to be invoked:\n%s", out)
	}
	if _, sErr := os.Stat(sentinel); sErr != nil {
		t.Fatalf("expected export sentinel %s to exist (script ran)", sentinel)
	}
}

// --- HRD-050 readme-sync (§11.4.59) ---

func TestReadmeSync_InSync_Allows(t *testing.T) {
	repo := t.TempDir()
	writeDoc(t, repo, "docs/guide.md", metadataDoc)
	writeDoc(t, repo, "README.md", "# Herald\n\nSee [the guide](docs/guide.md) for details.\n")
	out, err := runCmd(t, newReadmeSyncCmd(), "--repo", repo)
	if err != nil {
		t.Fatalf("readme-sync in-sync should PASS (exit 0), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[PASS] §11.4.59") {
		t.Fatalf("expected §11.4.59 PASS verdict line:\n%s", out)
	}
}

func TestReadmeSync_Drift_Blocks(t *testing.T) {
	repo := t.TempDir()
	writeDoc(t, repo, "README.md", "# Herald\n\nSee [the guide](docs/missing.md) for details.\n")
	out, err := runCmd(t, newReadmeSyncCmd(), "--repo", repo)
	if err == nil || !strings.Contains(err.Error(), "§11.4.59") {
		t.Fatalf("readme-sync with dangling link should BLOCK (§11.4.59 breach), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[FAIL] §11.4.59") {
		t.Fatalf("expected §11.4.59 FAIL verdict line:\n%s", out)
	}
	if !strings.Contains(out, "docs/missing.md") {
		t.Fatalf("expected dangling-link evidence:\n%s", out)
	}
}

// --- HRD-052 export (§11.4.65) ---

func TestExport_InvokesScript_Allows(t *testing.T) {
	repo := t.TempDir()
	writeDoc(t, repo, "docs/x.md", metadataDoc)
	script, sentinel := fakeExportScript(t, repo)
	out, err := runCmd(t, newExportCmd(), "--repo", repo, "--script", script, "docs/x.md")
	if err != nil {
		t.Fatalf("export with fake script should PASS (exit 0), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[PASS] §11.4.65") {
		t.Fatalf("expected §11.4.65 PASS verdict line:\n%s", out)
	}
	if !strings.Contains(out, "FAKE-EXPORT-INVOKED") {
		t.Fatalf("expected fake export script to be invoked:\n%s", out)
	}
	if _, sErr := os.Stat(sentinel); sErr != nil {
		t.Fatalf("expected export sentinel %s to exist (script ran)", sentinel)
	}
}

func TestExport_ScriptFails_Blocks(t *testing.T) {
	repo := t.TempDir()
	writeDoc(t, repo, "docs/x.md", metadataDoc)
	script := failingExportScript(t, repo)
	out, err := runCmd(t, newExportCmd(), "--repo", repo, "--script", script, "docs/x.md")
	if err == nil || !strings.Contains(err.Error(), "§11.4.65") {
		t.Fatalf("export with failing script should BLOCK (§11.4.65 breach), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[FAIL] §11.4.65") {
		t.Fatalf("expected §11.4.65 FAIL verdict line:\n%s", out)
	}
}

func TestExport_NoScript_Errors(t *testing.T) {
	repo := t.TempDir()
	out, err := runCmd(t, newExportCmd(), "--repo", repo)
	if err == nil || !strings.Contains(err.Error(), "no export script") {
		t.Fatalf("export with no resolvable script should error, got %v\n%s", err, out)
	}
}

// --- HRD-048 fixed-summary-sync (§11.4.53) ---

func TestFixedSummarySync_Parity_Allows(t *testing.T) {
	repo := t.TempDir()
	writeDoc(t, repo, "docs/Fixed.md", "## HRD-001\nfixed.\n\n## HRD-002\nfixed.\n")
	writeDoc(t, repo, "docs/Fixed_Summary.md", "- HRD-001 — closed the first thing.\n- HRD-002 — closed the second thing.\n")
	out, err := runCmd(t, newFixedSummarySyncCmd(), "--repo", repo)
	if err != nil {
		t.Fatalf("fixed-summary-sync in parity should PASS (exit 0), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[PASS] §11.4.53") {
		t.Fatalf("expected §11.4.53 PASS verdict line:\n%s", out)
	}
}

func TestFixedSummarySync_Drift_Blocks(t *testing.T) {
	repo := t.TempDir()
	writeDoc(t, repo, "docs/Fixed.md", "## HRD-001\nfixed.\n\n## HRD-002\nfixed.\n")
	writeDoc(t, repo, "docs/Fixed_Summary.md", "- HRD-001 — closed the first thing.\n")
	out, err := runCmd(t, newFixedSummarySyncCmd(), "--repo", repo)
	if err == nil || !strings.Contains(err.Error(), "§11.4.53") {
		t.Fatalf("fixed-summary-sync with missing summary should BLOCK (§11.4.53 breach), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[FAIL] §11.4.53") {
		t.Fatalf("expected §11.4.53 FAIL verdict line:\n%s", out)
	}
	if !strings.Contains(out, "HRD-002") {
		t.Fatalf("expected missing HRD-002 evidence:\n%s", out)
	}
}

func TestFixedSummarySync_ApplyBackfills(t *testing.T) {
	repo := t.TempDir()
	writeDoc(t, repo, "docs/Fixed.md", "## HRD-001\nfixed.\n\n## HRD-002\nfixed.\n")
	summary := writeDoc(t, repo, "docs/Fixed_Summary.md", "- HRD-001 — closed the first thing.\n")
	out, err := runCmd(t, newFixedSummarySyncCmd(), "--repo", repo, "--apply")
	if err != nil {
		t.Fatalf("fixed-summary-sync --apply should restore parity and PASS, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "backfilled 1 summary line") {
		t.Fatalf("expected backfill report:\n%s", out)
	}
	data, _ := os.ReadFile(summary)
	if !strings.Contains(string(data), "HRD-002") {
		t.Fatalf("expected HRD-002 backfilled into Fixed_Summary.md:\n%s", data)
	}
	if !strings.Contains(out, "[PASS] §11.4.53") {
		t.Fatalf("expected §11.4.53 PASS after backfill:\n%s", out)
	}
}

// --- HRD-039 fixed-align (§11.4.53 — closest cherald rule; §11.4.55 is pherald's) ---

func TestFixedAlign_NoDrift_Allows(t *testing.T) {
	repo := t.TempDir()
	writeDoc(t, repo, "docs/Issues.md", "## HRD-010\nopen.\n\n## HRD-011\nopen.\n")
	writeDoc(t, repo, "docs/Fixed.md", "## HRD-001\nfixed.\n\n## HRD-002\nfixed.\n")
	out, err := runCmd(t, newFixedAlignCmd(), "--repo", repo)
	if err != nil {
		t.Fatalf("fixed-align with disjoint trackers should PASS (exit 0), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[PASS] §11.4.53") {
		t.Fatalf("expected §11.4.53 PASS verdict line:\n%s", out)
	}
}

func TestFixedAlign_Drift_Blocks(t *testing.T) {
	repo := t.TempDir()
	// HRD-002 is present in BOTH trackers — a closure that was never removed
	// from Issues.md (the §11.4.19 atomic-migration breach fixed-align detects).
	writeDoc(t, repo, "docs/Issues.md", "## HRD-010\nopen.\n\n## HRD-002\nstill listed open.\n")
	writeDoc(t, repo, "docs/Fixed.md", "## HRD-001\nfixed.\n\n## HRD-002\nfixed.\n")
	out, err := runCmd(t, newFixedAlignCmd(), "--repo", repo)
	if err == nil || !strings.Contains(err.Error(), "§11.4.53") {
		t.Fatalf("fixed-align with cross-tracker drift should BLOCK (§11.4.53 breach), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[FAIL] §11.4.53") {
		t.Fatalf("expected §11.4.53 FAIL verdict line:\n%s", out)
	}
	if !strings.Contains(out, "HRD-002") {
		t.Fatalf("expected HRD-002 drift evidence:\n%s", out)
	}
}

func TestFixedAlign_Emit_DrivesEvent(t *testing.T) {
	repo := t.TempDir()
	writeDoc(t, repo, "docs/Issues.md", "## HRD-010\nopen.\n")
	writeDoc(t, repo, "docs/Fixed.md", "## HRD-001\nfixed.\n")
	out, err := runCmd(t, newFixedAlignCmd(), "--repo", repo, "--emit")
	if err != nil {
		t.Fatalf("fixed-align --emit (aligned) should PASS, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[emit] §11.4.53 drove a constitution event") {
		t.Fatalf("expected emit line:\n%s", out)
	}
}
