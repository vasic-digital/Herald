package main

// HRD-036/038/042/051/054/055 — §43 verify/check command bodies (v1.0.0 Batch C, C3b).
//
// EVERY test is 100% HERMETIC: all fixtures (.gitmodules, scripts/*.sh, planted
// fake secrets, spec docs, PR bodies) are built under t.TempDir(). No test
// touches the real Herald checkout or its remotes. Tests that exercise the
// spec-drift git path init a throwaway repo under t.TempDir().
//
// §107 anti-bluff: each test asserts the REAL command behaviour — the verdict
// line AND the exit code (PASS ⇒ nil error; BLOCK ⇒ non-nil error carrying the
// §-rule breach). creds-scan additionally asserts the planted secret VALUE is
// NEVER present in the command output (REDACTION proof).
//
// (runCmd / writeDoc are defined in docops_cmds_test.go — same package.)

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// --- HRD-042 submanifest-verify (§11.4.31) ---

func TestSubmanifestVerify_Present_Allows(t *testing.T) {
	repo := t.TempDir()
	gm := "[submodule \"containers\"]\n\tpath = containers\n\turl = git@github.com:vasic-digital/containers.git\n"
	if err := os.WriteFile(filepath.Join(repo, ".gitmodules"), []byte(gm), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runCmd(t, newSubmanifestVerifyCmd(), "--repo", repo)
	if err != nil {
		t.Fatalf("well-formed .gitmodules should PASS (exit 0), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[PASS] §11.4.31") {
		t.Fatalf("expected §11.4.31 PASS verdict line:\n%s", out)
	}
}

func TestSubmanifestVerify_NoSubmodules_Allows(t *testing.T) {
	repo := t.TempDir() // no .gitmodules ⇒ trivially compliant
	out, err := runCmd(t, newSubmanifestVerifyCmd(), "--repo", repo)
	if err != nil {
		t.Fatalf("repo without submodules should PASS (exit 0), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[PASS] §11.4.31") {
		t.Fatalf("expected §11.4.31 PASS verdict line:\n%s", out)
	}
}

func TestSubmanifestVerify_Missing_Blocks(t *testing.T) {
	repo := t.TempDir()
	// Malformed: a stanza with no `url =`.
	gm := "[submodule \"broken\"]\n\tpath = broken\n"
	if err := os.WriteFile(filepath.Join(repo, ".gitmodules"), []byte(gm), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runCmd(t, newSubmanifestVerifyCmd(), "--repo", repo)
	if err == nil || !strings.Contains(err.Error(), "§11.4.31") {
		t.Fatalf("malformed manifest should BLOCK (§11.4.31 breach), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[FAIL] §11.4.31") {
		t.Fatalf("expected §11.4.31 FAIL verdict line:\n%s", out)
	}
	if !strings.Contains(out, "missing `url =`") {
		t.Fatalf("expected missing-url evidence:\n%s", out)
	}
}

func TestSubmanifestVerify_Emit_DrivesEvent(t *testing.T) {
	repo := t.TempDir()
	out, err := runCmd(t, newSubmanifestVerifyCmd(), "--repo", repo, "--emit")
	if err != nil {
		t.Fatalf("submanifest-verify --emit (compliant) should PASS, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[emit] §11.4.31 drove a constitution event") {
		t.Fatalf("expected emit line:\n%s", out)
	}
}

// --- HRD-051 composite-gate (§11.4.60) ---

func TestCompositeGate_AllPass_Allows(t *testing.T) {
	repo := t.TempDir()
	writeDoc(t, repo, "docs/guide.md", metadataDoc)
	writeDoc(t, repo, "README.md", "# Herald\n\n| Field | Value |\n|---|---|\n| Revision | 1 |\n\n## Table of contents\n\nSee [the guide](docs/guide.md).\n")
	writeDoc(t, repo, "docs/Fixed.md", "## HRD-001\nfixed.\n")
	writeDoc(t, repo, "docs/Fixed_Summary.md", "- HRD-001 — closed the first thing.\n")
	out, err := runCmd(t, newCompositeGateCmd(), "--repo", repo)
	if err != nil {
		t.Fatalf("composite-gate all-pass should PASS (exit 0), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[PASS] §11.4.60") {
		t.Fatalf("expected §11.4.60 PASS verdict line:\n%s", out)
	}
}

func TestCompositeGate_Violation_Blocks(t *testing.T) {
	repo := t.TempDir()
	// README dangling link + Fixed_Summary parity gap ⇒ composite breach.
	writeDoc(t, repo, "README.md", "# Herald\n\n| Field | Value |\n|---|---|\n| Revision | 1 |\n\n## Table of contents\n\nSee [missing](docs/missing.md).\n")
	writeDoc(t, repo, "docs/Fixed.md", "## HRD-001\nfixed.\n\n## HRD-002\nfixed.\n")
	writeDoc(t, repo, "docs/Fixed_Summary.md", "- HRD-001 — closed the first thing.\n")
	out, err := runCmd(t, newCompositeGateCmd(), "--repo", repo)
	if err == nil || !strings.Contains(err.Error(), "§11.4.60") {
		t.Fatalf("composite-gate with breaches should BLOCK (§11.4.60), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[FAIL] §11.4.60") {
		t.Fatalf("expected §11.4.60 FAIL verdict line:\n%s", out)
	}
	if !strings.Contains(out, "docs/missing.md") || !strings.Contains(out, "HRD-002") {
		t.Fatalf("expected README + Fixed_Summary breach evidence:\n%s", out)
	}
}

// --- HRD-054 spec-version-check (§11.4.73) ---

// initRepo initialises a throwaway git repo under dir + commits the given file.
func initRepo(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "t@example.com"},
		{"config", "user.name", "t"},
		{"config", "commit.gpgsign", "false"},
	} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if b, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, b)
		}
	}
}

func gitCommitAll(t *testing.T, dir, msg string) {
	t.Helper()
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", msg}} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if b, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, b)
		}
	}
}

const specDocRev1 = "# Spec\n\n| Field | Value |\n|---|---|\n| Revision | 1 |\n\nbody one\n"

func TestSpecVersionCheck_NoDrift_Allows(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	writeDoc(t, repo, "docs/spec.md", specDocRev1)
	gitCommitAll(t, repo, "spec rev1")
	// Working tree matches HEAD exactly ⇒ no content change ⇒ no drift.
	out, err := runCmd(t, newSpecVersionCheckCmd(), "--repo", repo, "--spec", "docs/spec.md")
	if err != nil {
		t.Fatalf("unmodified spec should PASS (exit 0), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[PASS] §11.4.73") {
		t.Fatalf("expected §11.4.73 PASS verdict line:\n%s", out)
	}
}

func TestSpecVersionCheck_Drift_Blocks(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	specPath := writeDoc(t, repo, "docs/spec.md", specDocRev1)
	gitCommitAll(t, repo, "spec rev1")
	// Modify the BODY in the working tree but leave the Revision at 1 ⇒ §11.4.73 drift.
	modified := "# Spec\n\n| Field | Value |\n|---|---|\n| Revision | 1 |\n\nbody one EDITED without revision bump\n"
	if err := os.WriteFile(specPath, []byte(modified), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runCmd(t, newSpecVersionCheckCmd(), "--repo", repo, "--spec", "docs/spec.md")
	if err == nil || !strings.Contains(err.Error(), "§11.4.73") {
		t.Fatalf("edited-without-revision-bump should BLOCK (§11.4.73), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[FAIL] §11.4.73") {
		t.Fatalf("expected §11.4.73 FAIL verdict line:\n%s", out)
	}
}

func TestSpecVersionCheck_RevisionBumped_Allows(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	specPath := writeDoc(t, repo, "docs/spec.md", specDocRev1)
	gitCommitAll(t, repo, "spec rev1")
	// Edit the body AND bump the Revision ⇒ no drift (the discipline was followed).
	bumped := "# Spec\n\n| Field | Value |\n|---|---|\n| Revision | 2 |\n\nbody one EDITED with revision bump\n"
	if err := os.WriteFile(specPath, []byte(bumped), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runCmd(t, newSpecVersionCheckCmd(), "--repo", repo, "--spec", "docs/spec.md")
	if err != nil {
		t.Fatalf("edit-with-revision-bump should PASS (exit 0), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[PASS] §11.4.73") {
		t.Fatalf("expected §11.4.73 PASS verdict line:\n%s", out)
	}
}

// --- HRD-055 catalogue-check (§11.4.74) ---

func TestCatalogueCheck_HasCatalogueLine_Allows(t *testing.T) {
	body := "Adds a new submodule.\n\nCatalogue-Check: searched the catalogue; no existing primitive fits.\n"
	out, err := runCmd(t, newCatalogueCheckCmd(), "--pr-body", body, "PR-42")
	if err != nil {
		t.Fatalf("PR with Catalogue-Check line should PASS (exit 0), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[PASS] §11.4.74") {
		t.Fatalf("expected §11.4.74 PASS verdict line:\n%s", out)
	}
}

func TestCatalogueCheck_Missing_Blocks(t *testing.T) {
	body := "Adds a brand new vendored dependency without checking the catalogue first.\n"
	out, err := runCmd(t, newCatalogueCheckCmd(), "--pr-body", body, "PR-43")
	if err == nil || !strings.Contains(err.Error(), "§11.4.74") {
		t.Fatalf("non-trivial PR with NO Catalogue-Check line should BLOCK (§11.4.74), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[FAIL] §11.4.74") {
		t.Fatalf("expected §11.4.74 FAIL verdict line:\n%s", out)
	}
}

func TestCatalogueCheck_DiffFile_Allows(t *testing.T) {
	repo := t.TempDir()
	writeDoc(t, repo, "diff.txt", "M commons/foo.go\nA commons/bar.go\nCatalogue-Check: reused commons/gitops, no new dep.\n")
	out, err := runCmd(t, newCatalogueCheckCmd(), "--repo", repo, "--diff-file", "diff.txt")
	if err != nil {
		t.Fatalf("diff-file with Catalogue-Check should PASS (exit 0), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[PASS] §11.4.74") {
		t.Fatalf("expected §11.4.74 PASS verdict line:\n%s", out)
	}
}

// --- HRD-038 script-docs-check (§11.4.62 → §11.4.60) ---

func writeScript(t *testing.T, repo, name, body string) {
	t.Helper()
	dir := filepath.Join(repo, "scripts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestScriptDocsCheck_AllDocumented_Allows(t *testing.T) {
	repo := t.TempDir()
	writeScript(t, repo, "good.sh", "#!/bin/bash\n# good.sh — does a documented thing.\nset -e\necho hi\n")
	writeScript(t, repo, "also_good.sh", "# also_good.sh — no shebang but a header docstring.\necho ok\n")
	out, err := runCmd(t, newScriptDocsCheckCmd(), "--repo", repo)
	if err != nil {
		t.Fatalf("all-documented scripts should PASS (exit 0), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[PASS] §11.4.60") {
		t.Fatalf("expected §11.4.60 PASS verdict line:\n%s", out)
	}
}

func TestScriptDocsCheck_Undocumented_Blocks(t *testing.T) {
	repo := t.TempDir()
	writeScript(t, repo, "good.sh", "#!/bin/bash\n# good.sh — documented.\necho hi\n")
	writeScript(t, repo, "bad.sh", "#!/bin/bash\nset -e\necho 'no docstring after the shebang'\n")
	out, err := runCmd(t, newScriptDocsCheckCmd(), "--repo", repo)
	if err == nil || !strings.Contains(err.Error(), "§11.4.60") {
		t.Fatalf("undocumented script should BLOCK (§11.4.60), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[FAIL] §11.4.60") {
		t.Fatalf("expected §11.4.60 FAIL verdict line:\n%s", out)
	}
	if !strings.Contains(out, "scripts/bad.sh") {
		t.Fatalf("expected undocumented bad.sh evidence:\n%s", out)
	}
}

// --- HRD-036 creds-scan (§16.2 → §11.4.10) ---

func TestCredsScan_Clean_Allows(t *testing.T) {
	repo := t.TempDir()
	writeDoc(t, repo, "src/app.go", "package app\n\nfunc Hello() string { return \"world\" }\n")
	out, err := runCmd(t, newCredsScanCmd(), "--path", repo)
	if err != nil {
		t.Fatalf("clean dir should PASS (exit 0), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[PASS] §11.4.10") {
		t.Fatalf("expected §11.4.10 PASS verdict line:\n%s", out)
	}
}

func TestCredsScan_PlantedSecret_Blocks(t *testing.T) {
	repo := t.TempDir()
	// A dummy AWS access key id (fake — not a real credential).
	const plantedSecret = "AKIAIOSFODNN7EXAMPLE"
	writeDoc(t, repo, "config/leak.txt", "aws_access_key_id = "+plantedSecret+"\n")
	out, err := runCmd(t, newCredsScanCmd(), "--path", repo)
	if err == nil || !strings.Contains(err.Error(), "§11.4.10") {
		t.Fatalf("planted secret should BLOCK (§11.4.10), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[FAIL] §11.4.10") {
		t.Fatalf("expected §11.4.10 FAIL verdict line:\n%s", out)
	}
	if !strings.Contains(out, "[LEAK]") || !strings.Contains(out, "aws-access-key-id") {
		t.Fatalf("expected a redacted [LEAK] finding line:\n%s", out)
	}
	// REDACTION PROOF: the actual secret value must NEVER appear in the output.
	if strings.Contains(out, plantedSecret) {
		t.Fatalf("SECURITY: planted secret value leaked into output (must be redacted):\n%s", out)
	}
	if !strings.Contains(out, "REDACTED") {
		t.Fatalf("expected the REDACTED mask in the finding line:\n%s", out)
	}
}

func TestCredsScan_PemPrivateKey_Blocks(t *testing.T) {
	repo := t.TempDir()
	writeDoc(t, repo, "id_rsa", "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA...redactme...\n-----END RSA PRIVATE KEY-----\n")
	out, err := runCmd(t, newCredsScanCmd(), "--path", repo)
	if err == nil || !strings.Contains(err.Error(), "§11.4.10") {
		t.Fatalf("planted PEM private key should BLOCK (§11.4.10), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "pem-private-key") {
		t.Fatalf("expected pem-private-key finding:\n%s", out)
	}
	// The PEM header itself is the match; assert the secret body never prints raw.
	if strings.Contains(out, "MIIEpAIBAAKCAQEA") {
		t.Fatalf("SECURITY: PEM body leaked into output:\n%s", out)
	}
}
