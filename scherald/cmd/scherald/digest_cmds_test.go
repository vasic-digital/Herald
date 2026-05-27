package main

// HRD-047 — §43 scheduled-audit command body (v1.0.0 §43 straggler).
//
// EVERY test is 100% HERMETIC. The repo + docs/Status.md / docs/Issues.md /
// docs/Fixed.md fixtures are built under t.TempDir(); NO test runs against the
// real Herald checkout and NO test passes --apply against it.
//
// §107 anti-bluff: each test asserts the REAL command behaviour — the verdict
// line AND the exit code (PASS ⇒ nil error; FAIL ⇒ non-nil error carrying the
// §11.4.45 breach). A command that prints FAIL but exits 0 would be a §107
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

// healthyStatus is a well-formed docs/Status.md body.
const healthyStatus = "# Status\n\n| Field | Value |\n|---|---|\n| Revision | 13 |\n\n## Table of contents\n\nAll work-items current; periodic sweep clean.\n"

// --- HRD-047 status-digest (§11.4.45) ---

func TestStatusDigest_HealthyStatus_Allows(t *testing.T) {
	repo := t.TempDir()
	writeDoc(t, repo, "docs/Status.md", healthyStatus)
	// Disjoint trackers ⇒ zero stale items ⇒ a clean §11.4.45 sweep.
	writeDoc(t, repo, "docs/Issues.md", "## HRD-100\nopen.\n\n## HRD-101\nopen.\n")
	writeDoc(t, repo, "docs/Fixed.md", "## HRD-001\nfixed.\n\n## HRD-002\nfixed.\n")

	out, err := runCmd(t, newStatusDigestCmd(), "--repo", repo)
	if err != nil {
		t.Fatalf("status-digest with healthy Status.md should PASS (exit 0), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[PASS] §11.4.45") {
		t.Fatalf("expected §11.4.45 PASS verdict line:\n%s", out)
	}
	if !strings.Contains(out, "Status.md=present") {
		t.Fatalf("expected digest body reporting Status.md=present:\n%s", out)
	}
}

func TestStatusDigest_MissingStatus_Blocks(t *testing.T) {
	repo := t.TempDir()
	// No docs/Status.md at all ⇒ a §11.4.45 maintenance violation.
	writeDoc(t, repo, "docs/Issues.md", "## HRD-100\nopen.\n")

	out, err := runCmd(t, newStatusDigestCmd(), "--repo", repo)
	if err == nil || !strings.Contains(err.Error(), "§11.4.45") {
		t.Fatalf("status-digest with missing Status.md should BLOCK (§11.4.45 breach), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[FAIL] §11.4.45") {
		t.Fatalf("expected §11.4.45 FAIL verdict line:\n%s", out)
	}
	if !strings.Contains(out, "Status.md=MISSING") {
		t.Fatalf("expected digest body reporting Status.md=MISSING:\n%s", out)
	}
	if !strings.Contains(out, "STALE") {
		t.Fatalf("expected sweep evidence flagging Status.md STALE:\n%s", out)
	}
}

func TestStatusDigest_StaleItemsDrift_Blocks(t *testing.T) {
	repo := t.TempDir()
	writeDoc(t, repo, "docs/Status.md", healthyStatus)
	// HRD-002 present in BOTH trackers — a closure never removed from Issues.md
	// (the §11.4.19 drift the periodic audit surfaces as a stale item). With the
	// default stale-item threshold of 0, any stale item flags the sweep stale,
	// so the §11.4.45 status-sweep subject FAILs.
	writeDoc(t, repo, "docs/Issues.md", "## HRD-100\nopen.\n\n## HRD-002\nstill listed open.\n")
	writeDoc(t, repo, "docs/Fixed.md", "## HRD-001\nfixed.\n\n## HRD-002\nfixed.\n")

	out, err := runCmd(t, newStatusDigestCmd(), "--repo", repo)
	if err == nil || !strings.Contains(err.Error(), "§11.4.45") {
		t.Fatalf("status-digest with cross-tracker drift should BLOCK (§11.4.45 breach), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[FAIL] §11.4.45") {
		t.Fatalf("expected §11.4.45 FAIL verdict line:\n%s", out)
	}
	if !strings.Contains(out, "1 stale item") {
		t.Fatalf("expected digest body reporting 1 stale item:\n%s", out)
	}
}

func TestStatusDigest_Apply_RegeneratesSummary(t *testing.T) {
	repo := t.TempDir()
	writeDoc(t, repo, "docs/Status.md", healthyStatus)
	writeDoc(t, repo, "docs/Issues.md", "## HRD-100\nopen.\n")
	writeDoc(t, repo, "docs/Fixed.md", "## HRD-001\nfixed.\n")

	out, err := runCmd(t, newStatusDigestCmd(), "--repo", repo, "--apply")
	if err != nil {
		t.Fatalf("status-digest --apply (healthy) should PASS, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "regenerated docs/Status_Summary.md") {
		t.Fatalf("expected --apply to report Status_Summary.md regeneration:\n%s", out)
	}
	summary := filepath.Join(repo, "docs", "Status_Summary.md")
	data, rErr := os.ReadFile(summary)
	if rErr != nil {
		t.Fatalf("expected docs/Status_Summary.md to be written: %v", rErr)
	}
	if !strings.Contains(string(data), "Open work-items: 1") {
		t.Fatalf("expected regenerated summary to tally 1 open HRD:\n%s", data)
	}
	if !strings.Contains(out, "[PASS] §11.4.45") {
		t.Fatalf("expected §11.4.45 PASS after --apply:\n%s", out)
	}
}

func TestStatusDigest_Emit_DrivesEvent(t *testing.T) {
	repo := t.TempDir()
	writeDoc(t, repo, "docs/Status.md", healthyStatus)
	writeDoc(t, repo, "docs/Issues.md", "## HRD-100\nopen.\n")
	writeDoc(t, repo, "docs/Fixed.md", "## HRD-001\nfixed.\n")

	out, err := runCmd(t, newStatusDigestCmd(), "--repo", repo, "--emit")
	if err != nil {
		t.Fatalf("status-digest --emit (healthy) should PASS, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[emit] §11.4.45 drove a constitution event") {
		t.Fatalf("expected emit line:\n%s", out)
	}
}
