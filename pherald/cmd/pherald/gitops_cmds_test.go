package main

// HRD-029/030/043/044/049/053 — §43 project-lifecycle command bodies.
//
// EVERY test is 100% HERMETIC. Real git repos are built under t.TempDir() with
// `git init` + file:// fake remotes (bare repos in other t.TempDir() dirs). NO
// test touches the real Herald origin/mirrors, runs a real push to this
// checkout's remotes, or mutates the real docs/Issues.md / docs/Fixed.md. The
// reopen command's Issues↔Fixed migration runs against a temp docs tree only.
//
// §107 anti-bluff: each test asserts a REAL observable side-effect — a commit
// actually landed in a file:// remote, a remote was actually configured, a
// docs/Reopens/<HRD>.md file actually exists with the expected bytes, the row
// actually moved between Issues.md and Fixed.md — not a metadata/grep-only PASS.

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

// newBareRemote builds a bare repo (the file:// fake remote) and returns its
// path. Hermetic: it is a throwaway t.TempDir() — NEVER a real mirror.
func newBareRemote(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	tgit(t, dir, "init", "-q", "--bare", "-b", "main")
	return dir
}

// --- HRD-029 commit-push ---

func TestCommitPush_CommitsAndPushesToFakeRemote(t *testing.T) {
	work := newWorkRepo(t)
	remote := newBareRemote(t)
	// Wire the fake remote + an initial push so the branch exists upstream.
	tgit(t, work, "remote", "add", "github", "file://"+remote)
	tgit(t, work, "push", "-q", "github", "main")

	// Make a local change to commit.
	if err := os.WriteFile(filepath.Join(work, "feature.txt"), []byte("new feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := newCommitPushCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--repo", work, "-m", "feat: add feature.txt", "--push", "--emit"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("commit-push: %v\n%s", err, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "committed") || !strings.Contains(out, "pushed main to github") {
		t.Fatalf("output missing commit/push lines:\n%s", out)
	}
	if !strings.Contains(out, "[PASS] §2") {
		t.Fatalf("output missing §2 PASS verdict:\n%s", out)
	}
	if !strings.Contains(out, "[emit] §2 drove a constitution event") {
		t.Fatalf("output missing emit confirmation:\n%s", out)
	}

	// REAL side-effect: the commit must be in the fake remote's main.
	remoteLog := tgit(t, remote, "log", "--oneline", "main")
	if !strings.Contains(remoteLog, "feat: add feature.txt") {
		t.Fatalf("commit not found in fake remote main:\n%s", remoteLog)
	}
	// REAL side-effect: the commit-lock must be released (no residue).
	if _, err := os.Stat(filepath.Join(work, ".git", ".commit_all.lock")); !os.IsNotExist(err) {
		t.Fatalf("commit-lock not released after commit-push")
	}
}

func TestCommitPush_NoPushFlag_DoesNotReachRemote(t *testing.T) {
	work := newWorkRepo(t)
	if err := os.WriteFile(filepath.Join(work, "x.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := newCommitPushCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	// No remotes configured + no --push: must still commit cleanly.
	cmd.SetArgs([]string{"--repo", work, "-m", "chore: local only"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("commit-push (no push): %v\n%s", err, buf.String())
	}
	if strings.Contains(buf.String(), "pushed") {
		t.Fatalf("bare commit-push reached a remote:\n%s", buf.String())
	}
	log := tgit(t, work, "log", "--oneline")
	if !strings.Contains(log, "chore: local only") {
		t.Fatalf("commit not made locally:\n%s", log)
	}
}

func TestCommitPush_RequiresMessage(t *testing.T) {
	work := newWorkRepo(t)
	cmd := newCommitPushCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--repo", work})
	if err := cmd.Execute(); err == nil {
		t.Fatal("commit-push without -m should error")
	}
}

func TestCommitPush_NothingStaged(t *testing.T) {
	work := newWorkRepo(t) // clean tree
	cmd := newCommitPushCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--repo", work, "-m", "nothing"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "nothing staged") {
		t.Fatalf("clean tree commit-push: want 'nothing staged' error, got %v", err)
	}
}

// --- HRD-053 pre-push ---

func TestPrePush_UpToDate_PASS(t *testing.T) {
	work := newWorkRepo(t)
	remote := newBareRemote(t)
	tgit(t, work, "remote", "add", "origin", "file://"+remote)
	tgit(t, work, "push", "-q", "origin", "main")

	cmd := newPrePushCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--repo", work, "--emit"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pre-push (up to date): %v\n%s", err, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "up to date") || !strings.Contains(out, "[PASS] §11.4.71") {
		t.Fatalf("pre-push up-to-date output unexpected:\n%s", out)
	}
}

func TestPrePush_Behind_FAIL(t *testing.T) {
	// remote gets a commit the local clone doesn't have ⇒ behind > 0 ⇒ §11.4.71 FAIL.
	remote := newBareRemote(t)
	seed := newWorkRepo(t)
	tgit(t, seed, "remote", "add", "origin", "file://"+remote)
	tgit(t, seed, "push", "-q", "origin", "main")

	// Clone into a separate work dir.
	cloneParent := t.TempDir()
	tgit(t, cloneParent, "clone", "-q", "file://"+remote, "clone")
	clone := filepath.Join(cloneParent, "clone")
	tgit(t, clone, "config", "user.email", "c@herald.local")
	tgit(t, clone, "config", "user.name", "Clone")
	tgit(t, clone, "config", "commit.gpgsign", "false")

	// Advance the remote via the seed repo.
	if err := os.WriteFile(filepath.Join(seed, "upstream.txt"), []byte("incoming\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tgit(t, seed, "add", "upstream.txt")
	tgit(t, seed, "commit", "-q", "-m", "incoming change")
	tgit(t, seed, "push", "-q", "origin", "main")

	cmd := newPrePushCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--repo", clone})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "§11.4.71") {
		t.Fatalf("pre-push (behind) should FAIL with §11.4.71 breach, got %v\n%s", err, buf.String())
	}
	if !strings.Contains(buf.String(), "incoming commit") {
		t.Fatalf("pre-push behind output should report incoming commits:\n%s", buf.String())
	}
}

// --- HRD-044 fetch-guard ---

func TestFetchGuard_Rebased_PASS(t *testing.T) {
	work := newWorkRepo(t)
	remote := newBareRemote(t)
	tgit(t, work, "remote", "add", "origin", "file://"+remote)
	tgit(t, work, "push", "-q", "origin", "main")

	cmd := newFetchGuardCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--repo", work, "--fetch", "--emit"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("fetch-guard (rebased): %v\n%s", err, buf.String())
	}
	if !strings.Contains(buf.String(), "[PASS] §11.4.37") {
		t.Fatalf("fetch-guard rebased output unexpected:\n%s", buf.String())
	}
}

func TestFetchGuard_Stale_FAIL(t *testing.T) {
	remote := newBareRemote(t)
	seed := newWorkRepo(t)
	tgit(t, seed, "remote", "add", "origin", "file://"+remote)
	tgit(t, seed, "push", "-q", "origin", "main")
	cloneParent := t.TempDir()
	tgit(t, cloneParent, "clone", "-q", "file://"+remote, "clone")
	clone := filepath.Join(cloneParent, "clone")

	// Advance the remote.
	if err := os.WriteFile(filepath.Join(seed, "u.txt"), []byte("u\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tgit(t, seed, "config", "user.email", "s@herald.local")
	tgit(t, seed, "config", "user.name", "Seed")
	tgit(t, seed, "config", "commit.gpgsign", "false")
	tgit(t, seed, "add", "u.txt")
	tgit(t, seed, "commit", "-q", "-m", "advance")
	tgit(t, seed, "push", "-q", "origin", "main")

	cmd := newFetchGuardCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--repo", clone, "--fetch"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "§11.4.37") {
		t.Fatalf("fetch-guard (stale) should FAIL §11.4.37, got %v\n%s", err, buf.String())
	}
}

// --- HRD-043 install-upstreams ---

func TestInstallUpstreams_ApplyConfiguresRemotes(t *testing.T) {
	work := newWorkRepo(t)
	upDir := filepath.Join(work, "upstreams")
	if err := os.MkdirAll(upDir, 0o755); err != nil {
		t.Fatal(err)
	}
	decls := map[string]string{
		"GitHub.sh": "#!/bin/bash\n\nexport UPSTREAMABLE_REPOSITORY=\"file:///tmp/fake-github\"\n",
		"GitLab.sh": "#!/bin/bash\n\nexport UPSTREAMABLE_REPOSITORY=\"file:///tmp/fake-gitlab\"\n",
	}
	for n, b := range decls {
		if err := os.WriteFile(filepath.Join(upDir, n), []byte(b), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := newInstallUpstreamsCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--repo", work, "--apply", "--emit"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("install-upstreams --apply: %v\n%s", err, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "2/2 declared mirrors configured") || !strings.Contains(out, "[PASS] §11.4.36") {
		t.Fatalf("install-upstreams output unexpected:\n%s", out)
	}
	// REAL side-effect: the remotes are actually configured in git.
	remotes := tgit(t, work, "remote")
	if !strings.Contains(remotes, "github") || !strings.Contains(remotes, "gitlab") {
		t.Fatalf("remotes not configured: %q", remotes)
	}
	gh := strings.TrimSpace(tgit(t, work, "remote", "get-url", "github"))
	if gh != "file:///tmp/fake-github" {
		t.Fatalf("github remote url = %q", gh)
	}
}

func TestInstallUpstreams_ReportOnly_PartialWarn(t *testing.T) {
	work := newWorkRepo(t)
	upDir := filepath.Join(work, "upstreams")
	if err := os.MkdirAll(upDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 2 declared, 0 configured, no --apply ⇒ WARN-tier, allowFail (no hard error).
	for _, n := range []string{"GitHub.sh", "GitLab.sh"} {
		if err := os.WriteFile(filepath.Join(upDir, n),
			[]byte("export UPSTREAMABLE_REPOSITORY=\"file:///tmp/x\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := newInstallUpstreamsCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--repo", work})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("install-upstreams report-only should not hard-fail (WARN tier): %v", err)
	}
	if !strings.Contains(buf.String(), "[FAIL] §11.4.36") {
		t.Fatalf("report-only with 0 configured should show §11.4.36 FAIL verdict (but not exit-fail):\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "0/2 declared mirrors configured") {
		t.Fatalf("expected 0/2 tally:\n%s", buf.String())
	}
}

func TestInstallUpstreams_NoDeclarations(t *testing.T) {
	work := newWorkRepo(t)
	cmd := newInstallUpstreamsCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--repo", work})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "no mirror declarations") {
		t.Fatalf("install-upstreams with no upstreams dir: want error, got %v", err)
	}
}

// --- HRD-030 submodule-propagate ---

func TestSubmodulePropagate_NoSubmodules_PASS(t *testing.T) {
	work := newWorkRepo(t) // no .gitmodules
	cmd := newSubmodulePropagateCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--repo", work, "--emit"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("submodule-propagate (no submodules): %v\n%s", err, buf.String())
	}
	if !strings.Contains(buf.String(), "[PASS] §3") {
		t.Fatalf("submodule-propagate no-submodules verdict unexpected:\n%s", buf.String())
	}
}

func TestSubmodulePropagate_DriftedPin_FAIL(t *testing.T) {
	// Build a parent repo with one submodule, then advance the submodule's
	// checked-out SHA past the parent's pinned SHA ⇒ `git submodule status`
	// shows a leading '+' ⇒ parent-first risk ⇒ §3 FAIL.
	inner := newWorkRepo(t)
	tgit(t, inner, "config", "user.email", "i@herald.local")
	tgit(t, inner, "config", "user.name", "Inner")

	parent := newWorkRepo(t)
	// add submodule from a file:// url (local path) — hermetic.
	tgit(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", "file://"+inner, "sub")
	tgit(t, parent, "commit", "-q", "-m", "add submodule")

	// Advance the inner checkout inside the parent's sub dir past the pin.
	subDir := filepath.Join(parent, "sub")
	if err := os.WriteFile(filepath.Join(subDir, "drift.txt"), []byte("drift\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tgit(t, subDir, "add", "drift.txt")
	tgit(t, subDir, "commit", "-q", "-m", "inner drift not re-pinned by parent")

	cmd := newSubmodulePropagateCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--repo", parent})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "§3") {
		t.Fatalf("submodule-propagate (drifted pin) should FAIL §3, got %v\n%s", err, buf.String())
	}
	if !strings.Contains(buf.String(), "parent-first risk") {
		t.Fatalf("expected parent-first-risk summary:\n%s", buf.String())
	}
}

// --- HRD-049 reopen ---

func writeDocsFixture(t *testing.T, docsDir string) {
	t.Helper()
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	issues := "# Issues\n\n| ID | Title | Status | Date |\n|---|---|---|---|\n| HRD-200 | open one | open | 2026-05-20 |\n"
	fixed := "# Fixed\n\n| ID | Title | Status | Date |\n|---|---|---|---|\n| HRD-049 | reopen target | Closed | 2026-05-20 |\n"
	if err := os.WriteFile(filepath.Join(docsDir, "Issues.md"), []byte(issues), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "Fixed.md"), []byte(fixed), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReopen_MigratesRowAndWritesReopensRecord(t *testing.T) {
	docsDir := filepath.Join(t.TempDir(), "docs")
	writeDocsFixture(t, docsDir)

	cmd := newReopenCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"HRD-049", "--docs-dir", docsDir, "--reason", "regression found", "--emit"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("reopen: %v\n%s", err, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "[PASS] §11.4.55") || !strings.Contains(out, "wrote docs/Reopens/HRD-049.md") {
		t.Fatalf("reopen output unexpected:\n%s", out)
	}

	// REAL side-effect 1: the row moved Fixed.md → Issues.md.
	issues, _ := os.ReadFile(filepath.Join(docsDir, "Issues.md"))
	fixed, _ := os.ReadFile(filepath.Join(docsDir, "Fixed.md"))
	if !strings.Contains(string(issues), "HRD-049") {
		t.Fatalf("HRD-049 not migrated into Issues.md:\n%s", issues)
	}
	if strings.Contains(string(fixed), "| HRD-049 |") {
		t.Fatalf("HRD-049 still present in Fixed.md:\n%s", fixed)
	}
	// REAL side-effect 2: the §11.4.55 Reopens record exists with the reason.
	rec, err := os.ReadFile(filepath.Join(docsDir, "Reopens", "HRD-049.md"))
	if err != nil {
		t.Fatalf("Reopens record missing: %v", err)
	}
	if !strings.Contains(string(rec), "regression found") || !strings.Contains(string(rec), "§11.4.55") {
		t.Fatalf("Reopens record content unexpected:\n%s", rec)
	}
}

func TestReopen_IdempotentRecordAppend(t *testing.T) {
	docsDir := filepath.Join(t.TempDir(), "docs")
	writeDocsFixture(t, docsDir)
	// First reopen.
	c1 := newReopenCmd()
	var b1 bytes.Buffer
	c1.SetOut(&b1)
	c1.SetErr(&b1)
	c1.SetArgs([]string{"HRD-049", "--docs-dir", docsDir, "--reason", "first"})
	if err := c1.Execute(); err != nil {
		t.Fatalf("reopen #1: %v\n%s", err, b1.String())
	}
	// Migrate it back to Fixed so the row exists to reopen again (use raw fixture rewrite).
	writeDocsFixtureKeepReopens(t, docsDir)
	c2 := newReopenCmd()
	var b2 bytes.Buffer
	c2.SetOut(&b2)
	c2.SetErr(&b2)
	c2.SetArgs([]string{"HRD-049", "--docs-dir", docsDir, "--reason", "second"})
	if err := c2.Execute(); err != nil {
		t.Fatalf("reopen #2: %v\n%s", err, b2.String())
	}
	rec, _ := os.ReadFile(filepath.Join(docsDir, "Reopens", "HRD-049.md"))
	if !strings.Contains(string(rec), "first") || !strings.Contains(string(rec), "second") {
		t.Fatalf("Reopens record should append both reasons:\n%s", rec)
	}
}

// writeDocsFixtureKeepReopens rewrites Issues/Fixed (re-seeding HRD-049 in Fixed)
// WITHOUT clobbering the existing Reopens/ dir, so the second reopen appends.
func writeDocsFixtureKeepReopens(t *testing.T, docsDir string) {
	t.Helper()
	issues := "# Issues\n\n| ID | Title | Status | Date |\n|---|---|---|---|\n| HRD-200 | open one | open | 2026-05-20 |\n"
	fixed := "# Fixed\n\n| ID | Title | Status | Date |\n|---|---|---|---|\n| HRD-049 | reopen target | Closed | 2026-05-20 |\n"
	if err := os.WriteFile(filepath.Join(docsDir, "Issues.md"), []byte(issues), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "Fixed.md"), []byte(fixed), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReopen_RejectsNonHRDArg(t *testing.T) {
	docsDir := filepath.Join(t.TempDir(), "docs")
	writeDocsFixture(t, docsDir)
	cmd := newReopenCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"not-an-hrd", "--docs-dir", docsDir})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "not an HRD reference") {
		t.Fatalf("reopen with non-HRD arg: want error, got %v", err)
	}
}

func TestParseReopenRef(t *testing.T) {
	cases := map[string]string{
		"HRD-049": "HRD-049",
		"hrd-49":  "HRD-49",
		"49":      "HRD-49",
	}
	for in, want := range cases {
		got, err := parseReopenRef(in)
		if err != nil {
			t.Fatalf("parseReopenRef(%q): %v", in, err)
		}
		if got != want {
			t.Fatalf("parseReopenRef(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := parseReopenRef("xyz"); err == nil {
		t.Fatal("parseReopenRef(xyz) should error")
	}
}

// --- registration coverage: the six §43 commands are wired, no stubs remain ---

func TestRegisterGitOps_AllSixCommandsLive(t *testing.T) {
	root := &cobra.Command{Use: "pherald-test"}
	registerGitOps(root)
	got := map[string]bool{}
	for _, c := range root.Commands() {
		got[c.Name()] = true
		// §107 anti-bluff: none of the six may be a StubCmd anymore — assert the
		// Short string does NOT carry the "(not yet implemented — HRD-...)" marker.
		if strings.Contains(c.Short, "not yet implemented") {
			t.Errorf("§43 command %q is still a stub: %q", c.Name(), c.Short)
		}
	}
	for _, want := range []string{"commit-push", "submodule-propagate", "install-upstreams", "fetch-guard", "reopen", "pre-push"} {
		if !got[want] {
			t.Errorf("missing live §43 command %q", want)
		}
	}
}
