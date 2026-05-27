package main

// HRD-041 test-tier-verify + HRD-035 evidence-capture — §43 build/test command
// bodies (v1.0.0 Batch C, cluster C5).
//
// EVERY test is 100% HERMETIC. No test runs a build, re-executes a suite, spawns
// a process, or touches the real Herald repo. test-tier-verify drives the
// deterministic --tier / --all-tiers / --tiers-file seams; evidence-capture
// drives --has-evidence or a t.TempDir() artefact (present / empty / absent) so
// the PASS-bluff branch is exercised without any external state.
//
// §107 anti-bluff: each test asserts the REAL gate behaviour — the verdict line
// AND the exit code (PASS ⇒ nil error / "satisfied"; FAIL ⇒ non-nil error
// carrying the §-rule breach). A gate that prints FAIL but exits 0 would itself
// be a §107 PASS-bluff; these tests assert the error is returned on a FAIL.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// run executes a freshly-built command with args, capturing combined output.
func run(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

// --- HRD-041 test-tier-verify (§11.4.27 / §40.2) ---

func TestTestTierVerify_AllTiersPresent_Allows(t *testing.T) {
	// --all-tiers asserts the full §40.2 8-tier matrix ⇒ PASS / exit 0.
	out, err := run(t, newTestTierVerifyCmd(), "--pkg", "commons", "--all-tiers", "--emit")
	if err != nil {
		t.Fatalf("test-tier-verify --all-tiers should ALLOW (exit 0), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[PASS] §11.4.27") {
		t.Fatalf("expected §11.4.27 PASS verdict line:\n%s", out)
	}
	if !strings.Contains(out, "all 8 canonical test tiers") {
		t.Fatalf("expected full-matrix evidence:\n%s", out)
	}
	if !strings.Contains(out, "[emit] §11.4.27 drove a constitution event") {
		t.Fatalf("expected emit confirmation:\n%s", out)
	}
}

func TestTestTierVerify_AllTiersViaFlags_Allows(t *testing.T) {
	// Supply all 8 tiers explicitly via repeated/comma-separated --tier ⇒ PASS.
	out, err := run(t, newTestTierVerifyCmd(), "--pkg", "pherald",
		"--tier", "unit,component,integration,contract",
		"--tier", "e2e_sandbox,e2e_live,mutation,chaos")
	if err != nil {
		t.Fatalf("test-tier-verify with all 8 tiers via --tier should ALLOW, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[PASS] §11.4.27") {
		t.Fatalf("expected §11.4.27 PASS verdict:\n%s", out)
	}
}

func TestTestTierVerify_MissingTier_Blocks(t *testing.T) {
	// Only the cheap tiers present (no mutation/chaos/e2e_live) ⇒ FAIL / non-zero.
	out, err := run(t, newTestTierVerifyCmd(), "--pkg", "commons_messaging",
		"--tier", "unit,component,integration")
	if err == nil || !strings.Contains(err.Error(), "§11.4.27") {
		t.Fatalf("test-tier-verify with missing tiers should BLOCK (§11.4.27 breach), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[FAIL] §11.4.27") {
		t.Fatalf("expected §11.4.27 FAIL verdict line:\n%s", out)
	}
	if !strings.Contains(out, "missing test tiers") {
		t.Fatalf("expected missing-tier evidence:\n%s", out)
	}
	// The named missing tiers MUST surface so the operator knows what to add.
	for _, want := range []string{"contract", "e2e_sandbox", "e2e_live", "mutation", "chaos"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected missing tier %q named in evidence:\n%s", want, out)
		}
	}
}

func TestTestTierVerify_TiersFile_Allows(t *testing.T) {
	// A --tiers-file listing all 8 tiers (with comments + blank lines) ⇒ PASS.
	dir := t.TempDir()
	path := filepath.Join(dir, "tiers.txt")
	body := "# present test tiers for this package\n" +
		"unit\ncomponent\nintegration\ncontract\n" +
		"\n# e2e + advanced\n" +
		"e2e_sandbox, e2e_live\n" +
		"mutation\nchaos  # full matrix\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, newTestTierVerifyCmd(), "--pkg", "commons_constitution", "--tiers-file", path)
	if err != nil {
		t.Fatalf("test-tier-verify --tiers-file (full matrix) should ALLOW, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[PASS] §11.4.27") {
		t.Fatalf("expected §11.4.27 PASS verdict:\n%s", out)
	}
}

// --- HRD-035 evidence-capture (§11.4.2 / §11.4.5) ---

func TestEvidenceCapture_WithEvidence_Allows(t *testing.T) {
	// outcome=pass + a present, non-empty artefact ⇒ PASS / exit 0.
	dir := t.TempDir()
	artefact := filepath.Join(dir, "transcript.txt")
	if err := os.WriteFile(artefact, []byte("real e2e transcript\nverdict=pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, newEvidenceCaptureCmd(),
		"--test-id", "E89", "--outcome", "pass", "--evidence-path", artefact, "--emit")
	if err != nil {
		t.Fatalf("evidence-capture with captured evidence should ALLOW, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "evidence=true") {
		t.Fatalf("expected evidence=true:\n%s", out)
	}
	if !strings.Contains(out, "[PASS] §11.4.2") {
		t.Fatalf("expected §11.4.2 PASS verdict line:\n%s", out)
	}
	if !strings.Contains(out, "[emit] §11.4.2 drove a constitution event") {
		t.Fatalf("expected emit confirmation:\n%s", out)
	}
}

func TestEvidenceCapture_DirArtefact_Allows(t *testing.T) {
	// outcome=pass + an evidence DIRECTORY holding a non-empty file (a
	// docs/qa/<run-id>/ bundle) ⇒ PASS.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "transcript.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, newEvidenceCaptureCmd(), "--test-id", "HRD-100", "--evidence-path", dir)
	if err != nil {
		t.Fatalf("evidence-capture with a non-empty evidence dir should ALLOW, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[PASS] §11.4.2") {
		t.Fatalf("expected §11.4.2 PASS verdict:\n%s", out)
	}
}

func TestEvidenceCapture_MetadataOnly_Blocks(t *testing.T) {
	// outcome=pass but NO evidence (no --evidence-path, no --has-evidence) ⇒
	// PASS-bluff ⇒ FAIL / non-zero exit. This is the §107 covenant enforcement.
	out, err := run(t, newEvidenceCaptureCmd(), "--test-id", "fake-green", "--outcome", "pass")
	if err == nil || !strings.Contains(err.Error(), "§11.4.2") {
		t.Fatalf("evidence-capture PASS with no evidence should BLOCK (§11.4.2 breach), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[FAIL] §11.4.2") {
		t.Fatalf("expected §11.4.2 FAIL verdict line:\n%s", out)
	}
	if !strings.Contains(out, "PASS-BLUFF") {
		t.Fatalf("expected PASS-BLUFF evidence:\n%s", out)
	}
}

func TestEvidenceCapture_EmptyArtefact_Blocks(t *testing.T) {
	// outcome=pass + a touch-only EMPTY file is NOT evidence ⇒ FAIL.
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, newEvidenceCaptureCmd(), "--test-id", "touch-only", "--evidence-path", empty)
	if err == nil || !strings.Contains(err.Error(), "§11.4.2") {
		t.Fatalf("evidence-capture PASS with an empty artefact should BLOCK, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "evidence=false") || !strings.Contains(out, "[FAIL] §11.4.2") {
		t.Fatalf("expected empty-artefact FAIL:\n%s", out)
	}
}

func TestEvidenceCapture_HonestFail_Allows(t *testing.T) {
	// outcome=fail is honest (not a bluff) ⇒ the anti-bluff check itself PASSes,
	// even without an evidence artefact.
	out, err := run(t, newEvidenceCaptureCmd(), "--test-id", "broken", "--outcome", "fail")
	if err != nil {
		t.Fatalf("evidence-capture of an honest FAIL should pass the anti-bluff check, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[PASS] §11.4.2") {
		t.Fatalf("expected §11.4.2 PASS (honest fail is not a bluff):\n%s", out)
	}
	if !strings.Contains(out, "not a PASS-bluff") {
		t.Fatalf("expected honest-fail evidence:\n%s", out)
	}
}

// --- registration coverage: the two §43 commands are wired, no stubs remain ---

func TestRegisterBuildOps_BothCommandsLive(t *testing.T) {
	root := &cobra.Command{Use: "bherald-test"}
	registerBuildOps(root)
	got := map[string]bool{}
	for _, c := range root.Commands() {
		got[c.Name()] = true
		// §107 anti-bluff: neither command may be a StubCmd anymore.
		if strings.Contains(c.Short, "not yet implemented") {
			t.Errorf("§43 command %q is still a stub: %q", c.Name(), c.Short)
		}
	}
	for _, want := range []string{"test-tier-verify", "evidence-capture"} {
		if !got[want] {
			t.Errorf("missing live §43 command %q", want)
		}
	}
}
