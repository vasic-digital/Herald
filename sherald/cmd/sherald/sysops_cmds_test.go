package main

// HRD-033/040/046/056 — §43 system/safety command bodies (v1.0.0 Batch C, C2).
//
// EVERY test is 100% HERMETIC. Real git repos are built under t.TempDir() with
// `git init` + file:// fake remotes (bare repos in other t.TempDir() dirs). The
// mem-budget-watch tests drive the deterministic --used-fraction test seam so
// no real memory pressure is ever created (§12.6 host-safety: the guard READS
// memory, it MUST NOT allocate to test). NO test touches the real Herald
// origin/mirrors, performs a real destructive op, or force-pushes.
//
// §107 anti-bluff: each test asserts the REAL gate behaviour — the verdict line
// AND the exit code (PASS ⇒ nil error / "safe to proceed"; FAIL ⇒ non-nil error
// carrying the §-rule breach / "BLOCK"). A gate that prints FAIL but exits 0
// would be a §107 PASS-bluff; these tests assert the error is returned on FAIL.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func tgit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return string(out)
}

// newWorkRepo builds a fresh git repo with one seed commit on main + LOCAL
// (repo-scoped) identity. Returns the repo dir.
func newWorkRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	tgit(t, dir, "init", "-q", "-b", "main")
	tgit(t, dir, "config", "user.email", "test@herald.local")
	tgit(t, dir, "config", "user.name", "Herald Test")
	tgit(t, dir, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# hermetic\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tgit(t, dir, "add", "README.md")
	tgit(t, dir, "commit", "-q", "-m", "seed")
	return dir
}

// newBareRemote builds a bare repo (the file:// fake remote). Hermetic.
func newBareRemote(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	tgit(t, dir, "init", "-q", "--bare", "-b", "main")
	return dir
}

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

// --- HRD-033 destructive-guard ---

func TestDestructiveGuard_NoBackup_Blocks(t *testing.T) {
	out, err := run(t, newDestructiveGuardCmd(), "git", "reset", "--hard", "HEAD~1")
	if err == nil || !strings.Contains(err.Error(), "§9.1") {
		t.Fatalf("destructive-guard without backup should BLOCK (§9.1 breach), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[FAIL] §9.1") {
		t.Fatalf("expected §9.1 FAIL verdict line:\n%s", out)
	}
	if !strings.Contains(out, "WITHOUT a preceding hardlinked backup") {
		t.Fatalf("expected backup-missing evidence:\n%s", out)
	}
}

func TestDestructiveGuard_WithBackup_Allows(t *testing.T) {
	out, err := run(t, newDestructiveGuardCmd(), "--backup-exists", "rm", "-rf", "build/")
	if err != nil {
		t.Fatalf("destructive-guard WITH backup should ALLOW (exit 0), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[PASS] §9.1") {
		t.Fatalf("expected §9.1 PASS verdict line:\n%s", out)
	}
}

func TestDestructiveGuard_BackupPathDetection_Allows(t *testing.T) {
	dir := t.TempDir()
	backup := filepath.Join(dir, ".backup-snapshot")
	if err := os.WriteFile(backup, []byte("hardlinked snapshot\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, newDestructiveGuardCmd(), "--backup-path", backup, "git", "clean", "-fd")
	if err != nil {
		t.Fatalf("destructive-guard with present --backup-path should ALLOW, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[PASS] §9.1") {
		t.Fatalf("expected §9.1 PASS with backup-path:\n%s", out)
	}
}

func TestDestructiveGuard_NonDestructiveOp_Allows(t *testing.T) {
	out, err := run(t, newDestructiveGuardCmd(), "git", "status")
	if err != nil {
		t.Fatalf("destructive-guard on a non-destructive op should pass, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "not a recognized destructive op") || !strings.Contains(out, "[PASS] §9.1") {
		t.Fatalf("expected non-destructive PASS:\n%s", out)
	}
}

func TestDestructiveGuard_Emit_DrivesEvent(t *testing.T) {
	out, err := run(t, newDestructiveGuardCmd(), "--backup-exists", "--emit", "git", "reset", "--hard")
	if err != nil {
		t.Fatalf("destructive-guard --emit (with backup) should pass, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[emit] §9.1 drove a constitution event") {
		t.Fatalf("expected emit confirmation:\n%s", out)
	}
}

// --- HRD-040 constitution-pull ---

// newFakeConstitution builds a "constitution" repo cloned from a fake remote, so
// fetch + rebase succeed hermetically and HEAD has a real SHA. Returns the
// clone dir (the --constitution-dir) and the bare remote.
func newFakeConstitution(t *testing.T) (cdir, remote string) {
	t.Helper()
	remote = newBareRemote(t)
	seed := newWorkRepo(t)
	tgit(t, seed, "remote", "add", "origin", "file://"+remote)
	tgit(t, seed, "push", "-q", "origin", "main")
	parent := t.TempDir()
	tgit(t, parent, "clone", "-q", "file://"+remote, "constitution")
	cdir = filepath.Join(parent, "constitution")
	tgit(t, cdir, "config", "user.email", "c@herald.local")
	tgit(t, cdir, "config", "user.name", "Const")
	tgit(t, cdir, "config", "commit.gpgsign", "false")
	return cdir, remote
}

func TestConstitutionPull_FetchRebaseValidate_EmitsBundleUpdated(t *testing.T) {
	cdir, _ := newFakeConstitution(t)
	out, err := run(t, newConstitutionPullCmd(),
		"--constitution-dir", cdir, "--remote", "origin", "--assume-validated", "--emit")
	if err != nil {
		t.Fatalf("constitution-pull (validated) should PASS, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "fetched + rebased") {
		t.Fatalf("expected fetch+rebase line:\n%s", out)
	}
	if !strings.Contains(out, "[PASS] §11.4.26") {
		t.Fatalf("expected §11.4.26 PASS verdict:\n%s", out)
	}
	if !strings.Contains(out, "[PASS] §11.4.32") {
		t.Fatalf("expected §11.4.32 PASS verdict:\n%s", out)
	}
	// .bundle.updated is the §11.4.32 emit class — assert the emit drove.
	if !strings.Contains(out, "[emit] §11.4.32 drove a constitution event") {
		t.Fatalf("expected §11.4.32 emit confirmation (bundle.updated):\n%s", out)
	}
}

func TestConstitutionPull_ValidationFails_Blocks(t *testing.T) {
	cdir, _ := newFakeConstitution(t)
	// --skip-validate ⇒ validated=false ⇒ §11.4.32 FAIL ⇒ non-zero exit.
	out, err := run(t, newConstitutionPullCmd(),
		"--constitution-dir", cdir, "--remote", "origin", "--skip-validate")
	if err == nil || !strings.Contains(err.Error(), "§11.4.32") {
		t.Fatalf("constitution-pull with skipped validation should BLOCK (§11.4.32 breach), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[FAIL] §11.4.32") {
		t.Fatalf("expected §11.4.32 FAIL verdict:\n%s", out)
	}
	// §11.4.26 (the pull itself) still succeeded.
	if !strings.Contains(out, "[PASS] §11.4.26") {
		t.Fatalf("expected §11.4.26 PASS (pull ok even when validation fails):\n%s", out)
	}
}

func TestConstitutionPull_NoConstitutionDir_Errors(t *testing.T) {
	// A bare temp dir that is NOT a git repo and has no discoverable constitution.
	notARepo := t.TempDir()
	out, err := run(t, newConstitutionPullCmd(), "--constitution-dir", notARepo)
	if err == nil || !strings.Contains(err.Error(), "not a git repo") {
		t.Fatalf("constitution-pull against a non-repo dir should error, got %v\n%s", err, out)
	}
}

// --- HRD-046 force-push-gate ---

func TestForcePushGate_MergedAndAuthorized_Allows(t *testing.T) {
	work := newWorkRepo(t)
	remote := newBareRemote(t)
	tgit(t, work, "remote", "add", "origin", "file://"+remote)
	tgit(t, work, "push", "-q", "origin", "main")
	// Local is even with origin/main ⇒ behind==0 ⇒ merge-first satisfied.
	out, err := run(t, newForcePushGateCmd(),
		"--repo", work, "--upstream", "origin/main", "--authorized", "--emit", "main")
	if err != nil {
		t.Fatalf("force-push-gate (merged+authorized) should ALLOW, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "merge-first=true") {
		t.Fatalf("expected merge-first=true:\n%s", out)
	}
	if !strings.Contains(out, "[PASS] §11.4.41") {
		t.Fatalf("expected §11.4.41 PASS verdict:\n%s", out)
	}
	if !strings.Contains(out, "[emit] §11.4.41 drove a constitution event") {
		t.Fatalf("expected emit confirmation:\n%s", out)
	}
}

func TestForcePushGate_NotMerged_Blocks(t *testing.T) {
	// remote advances past the local clone ⇒ behind>0 ⇒ merge-first violated ⇒ BLOCK.
	remote := newBareRemote(t)
	seed := newWorkRepo(t)
	tgit(t, seed, "remote", "add", "origin", "file://"+remote)
	tgit(t, seed, "push", "-q", "origin", "main")
	cloneParent := t.TempDir()
	tgit(t, cloneParent, "clone", "-q", "file://"+remote, "clone")
	clone := filepath.Join(cloneParent, "clone")
	tgit(t, clone, "config", "user.email", "c@herald.local")
	tgit(t, clone, "config", "user.name", "Clone")
	tgit(t, clone, "config", "commit.gpgsign", "false")

	// Advance the remote via seed.
	if err := os.WriteFile(filepath.Join(seed, "u.txt"), []byte("incoming\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tgit(t, seed, "add", "u.txt")
	tgit(t, seed, "commit", "-q", "-m", "incoming")
	tgit(t, seed, "push", "-q", "origin", "main")

	// --fetch so the clone learns about the new upstream commit ⇒ behind>0.
	out, err := run(t, newForcePushGateCmd(),
		"--repo", clone, "--upstream", "origin/main", "--authorized", "--fetch", "main")
	if err == nil || !strings.Contains(err.Error(), "§11.4.41") {
		t.Fatalf("force-push-gate (not merged) should BLOCK (§11.4.41 breach), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[FAIL] §11.4.41") {
		t.Fatalf("expected §11.4.41 FAIL verdict:\n%s", out)
	}
}

func TestForcePushGate_Unauthorized_Blocks(t *testing.T) {
	work := newWorkRepo(t)
	remote := newBareRemote(t)
	tgit(t, work, "remote", "add", "origin", "file://"+remote)
	tgit(t, work, "push", "-q", "origin", "main")
	// merged (behind==0) but NO --authorized ⇒ §9.2 unauthorized ⇒ BLOCK.
	// Ensure the env token is not set so the test is deterministic.
	t.Setenv("HERALD_FORCE_PUSH_AUTHORIZED", "")
	out, err := run(t, newForcePushGateCmd(),
		"--repo", work, "--upstream", "origin/main", "main")
	if err == nil || !strings.Contains(err.Error(), "§11.4.41") {
		t.Fatalf("force-push-gate (unauthorized) should BLOCK, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "authorized=false") || !strings.Contains(out, "[FAIL] §11.4.41") {
		t.Fatalf("expected unauthorized FAIL:\n%s", out)
	}
}

// --- HRD-056 mem-budget-watch ---

func TestMemBudgetWatch_UnderCeiling_Allows(t *testing.T) {
	// One-shot, test seam used_fraction=0.30 < 0.60 ⇒ PASS / exit 0.
	out, err := run(t, newMemBudgetWatchCmd(), "--used-fraction", "0.30", "--emit")
	if err != nil {
		t.Fatalf("mem-budget-watch under ceiling should ALLOW, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[PASS] §12.6") {
		t.Fatalf("expected §12.6 PASS verdict:\n%s", out)
	}
	if !strings.Contains(out, "within the 60% ceiling") {
		t.Fatalf("expected within-ceiling evidence:\n%s", out)
	}
	if !strings.Contains(out, "[emit] §12.6 drove a constitution event") {
		t.Fatalf("expected emit confirmation:\n%s", out)
	}
}

func TestMemBudgetWatch_OverCeiling_Blocks(t *testing.T) {
	// One-shot, test seam used_fraction=0.85 > 0.60 ⇒ FAIL / non-zero exit.
	out, err := run(t, newMemBudgetWatchCmd(), "--used-fraction", "0.85")
	if err == nil || !strings.Contains(err.Error(), "§12.6") {
		t.Fatalf("mem-budget-watch over ceiling should BLOCK (§12.6 breach), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[FAIL] §12.6") {
		t.Fatalf("expected §12.6 FAIL verdict:\n%s", out)
	}
	if !strings.Contains(out, "exceeds the 60% ceiling") {
		t.Fatalf("expected over-ceiling evidence:\n%s", out)
	}
}

func TestMemBudgetWatch_EnvSeam_OverCeiling_Blocks(t *testing.T) {
	t.Setenv("HERALD_MEM_FRACTION", "0.91")
	out, err := run(t, newMemBudgetWatchCmd())
	if err == nil || !strings.Contains(err.Error(), "§12.6") {
		t.Fatalf("mem-budget-watch with HERALD_MEM_FRACTION over ceiling should BLOCK, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "env HERALD_MEM_FRACTION") {
		t.Fatalf("expected env-seam evidence:\n%s", out)
	}
}

// --- registration coverage: the four §43 commands are wired, no stubs remain ---

func TestRegisterSysOps_AllFourCommandsLive(t *testing.T) {
	root := &cobra.Command{Use: "sherald-test"}
	registerSysOps(root)
	got := map[string]bool{}
	for _, c := range root.Commands() {
		got[c.Name()] = true
		// §107 anti-bluff: none of the four may be a StubCmd anymore.
		if strings.Contains(c.Short, "not yet implemented") {
			t.Errorf("§43 command %q is still a stub: %q", c.Name(), c.Short)
		}
	}
	for _, want := range []string{"destructive-guard", "constitution-pull", "force-push-gate", "mem-budget-watch"} {
		if !got[want] {
			t.Errorf("missing live §43 command %q", want)
		}
	}
}
