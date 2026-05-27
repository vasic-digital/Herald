package main

// HRD-031/032/045 — §43 release-lifecycle command bodies (v1.0.0 Batch C, C4).
//
// EVERY test is 100% HERMETIC. Real git repos are built under t.TempDir() with
// `git init` + file:// fake remotes (bare repos in other t.TempDir() dirs). The
// tag-mirror tests push the tag to SOME-but-not-all bare remotes to drive the §4
// parity-miss BLOCK path, and to ALL remotes to drive the parity ALLOW path. The
// gate-retest tests drive the deterministic --retest-result seam so the real test
// suite is NEVER run (§12/§107 host-safety). NO test touches the real Herald
// origin/mirrors, creates a real release tag on a real remote, or runs the suite.
//
// §107 anti-bluff: each test asserts the REAL gate behaviour — the verdict line
// AND the exit code (PASS ⇒ nil error / "safe to release"; FAIL ⇒ non-nil error
// carrying the §-rule breach). A gate that prints FAIL but exits 0 would be a §107
// PASS-bluff; these tests assert the error is returned on FAIL.

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

// --- HRD-031 tag-mirror ---

// newTagMirrorRepo builds a work repo wired to n bare mirror remotes (named
// mirror0..mirror{n-1}), pushes the seed branch to each, and creates the tag
// locally. Pushing the tag to specific mirrors is left to the caller. Returns
// the work dir + the remote names.
func newTagMirrorRepo(t *testing.T, tag string, nMirrors int) (work string, remotes []string) {
	t.Helper()
	work = newWorkRepo(t)
	for i := 0; i < nMirrors; i++ {
		name := "mirror" + itoa(i)
		bare := newBareRemote(t)
		tgit(t, work, "remote", "add", name, "file://"+bare)
		tgit(t, work, "push", "-q", name, "main")
		remotes = append(remotes, name)
	}
	tgit(t, work, "tag", tag)
	return work, remotes
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

func TestTagMirror_AllMirrorsHaveTag_Allows(t *testing.T) {
	tag := "v9.9.9"
	work, remotes := newTagMirrorRepo(t, tag, 2)
	// Push the tag to EVERY mirror ⇒ full parity ⇒ §4 PASS.
	for _, rem := range remotes {
		tgit(t, work, "push", "-q", rem, tag)
	}
	args := []string{"--repo", work, "--emit"}
	for _, rem := range remotes {
		args = append(args, "--remote", rem)
	}
	args = append(args, tag)
	out, err := run(t, newTagMirrorCmd(), args...)
	if err != nil {
		t.Fatalf("tag-mirror with full parity should ALLOW (exit 0), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[PASS] §4") {
		t.Fatalf("expected §4 PASS verdict:\n%s", out)
	}
	if !strings.Contains(out, "parity=2/2") {
		t.Fatalf("expected 2/2 parity line:\n%s", out)
	}
	if !strings.Contains(out, "[emit] §4 drove a constitution event") {
		t.Fatalf("expected emit confirmation:\n%s", out)
	}
}

func TestTagMirror_MissingOnMirror_Blocks(t *testing.T) {
	tag := "v9.9.9"
	work, remotes := newTagMirrorRepo(t, tag, 2)
	// Push the tag to only the FIRST mirror ⇒ parity miss ⇒ §4 BLOCK.
	tgit(t, work, "push", "-q", remotes[0], tag)
	args := []string{"--repo", work}
	for _, rem := range remotes {
		args = append(args, "--remote", rem)
	}
	args = append(args, tag)
	out, err := run(t, newTagMirrorCmd(), args...)
	if err == nil || !strings.Contains(err.Error(), "§4") {
		t.Fatalf("tag-mirror with a parity miss should BLOCK (§4 breach), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[FAIL] §4") {
		t.Fatalf("expected §4 FAIL verdict:\n%s", out)
	}
	if !strings.Contains(out, "parity=1/2") {
		t.Fatalf("expected 1/2 parity line:\n%s", out)
	}
}

func TestTagMirror_UpstreamsDeclarationsDiscovered(t *testing.T) {
	// No --remote flags: the command must read the mirror set from upstreams/*.sh.
	tag := "v1.2.3"
	work := newWorkRepo(t)
	bare := newBareRemote(t)
	// Declare ONE mirror named "github" (RemoteNameFor lowercases the filename).
	upDir := filepath.Join(work, "upstreams")
	if err := os.MkdirAll(upDir, 0o755); err != nil {
		t.Fatal(err)
	}
	decl := "#!/bin/bash\n\nexport UPSTREAMABLE_REPOSITORY=\"file://" + bare + "\"\n"
	if err := os.WriteFile(filepath.Join(upDir, "GitHub.sh"), []byte(decl), 0o644); err != nil {
		t.Fatal(err)
	}
	tgit(t, work, "remote", "add", "github", "file://"+bare)
	tgit(t, work, "push", "-q", "github", "main")
	tgit(t, work, "tag", tag)
	tgit(t, work, "push", "-q", "github", tag)

	out, err := run(t, newTagMirrorCmd(), "--repo", work, tag)
	if err != nil {
		t.Fatalf("tag-mirror via upstreams declaration should ALLOW, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[PASS] §4") || !strings.Contains(out, "parity=1/1") {
		t.Fatalf("expected §4 PASS 1/1 via upstreams discovery:\n%s", out)
	}
}

// --- HRD-032 changelog-generate ---

func TestChangelogGenerate_WritesConventionalCommits(t *testing.T) {
	work := newWorkRepo(t) // seed commit subject is "seed" (non-conventional)
	// Layer a few conventional commits on top.
	commit := func(msg, file string) {
		if err := os.WriteFile(filepath.Join(work, file), []byte(msg+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		tgit(t, work, "add", file)
		tgit(t, work, "commit", "-q", "-m", msg)
	}
	commit("feat: add tag-mirror command", "a.txt")
	commit("fix(bindings): correct parity tally", "b.txt")
	commit("docs: changelog notes", "c.txt")

	outDir := filepath.Join(t.TempDir(), "changelogs")
	out, err := run(t, newChangelogGenerateCmd(),
		"--repo", work, "--out-dir", outDir, "--emit", "v1.0.0")
	if err != nil {
		t.Fatalf("changelog-generate should not error (§5 warn-tier), got %v\n%s", err, out)
	}
	wrote := filepath.Join(outDir, "v1.0.0.md")
	if !strings.Contains(out, "wrote "+wrote) {
		t.Fatalf("expected wrote-line for %s:\n%s", wrote, out)
	}
	data, rerr := os.ReadFile(wrote)
	if rerr != nil {
		t.Fatalf("read generated changelog: %v", rerr)
	}
	body := string(data)
	for _, want := range []string{
		"# Changelog — v1.0.0",
		"## Features",
		"- feat: add tag-mirror command",
		"## Bug Fixes",
		"- fix(bindings): correct parity tally",
		"## Documentation",
		"- docs: changelog notes",
		"## Other", // the non-conventional "seed" commit lands here
		"- seed",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("generated changelog missing %q:\n%s", want, body)
		}
	}
	// "seed" is non-conventional ⇒ conforming=false ⇒ §5 WARN (no hard exit).
	if !strings.Contains(out, "conforming=false") {
		t.Fatalf("expected conforming=false (seed is non-conventional):\n%s", out)
	}
	if !strings.Contains(out, "[emit] §5 drove a constitution event") {
		t.Fatalf("expected §5 emit confirmation:\n%s", out)
	}
}

func TestChangelogGenerate_SinceBound_AllConforming(t *testing.T) {
	work := newWorkRepo(t)
	tgit(t, work, "tag", "v0.1.0") // anchor a "since" boundary AFTER the seed commit
	commit := func(msg, file string) {
		if err := os.WriteFile(filepath.Join(work, file), []byte(msg+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		tgit(t, work, "add", file)
		tgit(t, work, "commit", "-q", "-m", msg)
	}
	commit("feat: only-conventional change", "x.txt")
	commit("fix: another conventional change", "y.txt")

	outDir := filepath.Join(t.TempDir(), "cl")
	out, err := run(t, newChangelogGenerateCmd(),
		"--repo", work, "--since", "v0.1.0", "--out-dir", outDir, "v0.2.0")
	if err != nil {
		t.Fatalf("changelog-generate --since should not error, got %v\n%s", err, out)
	}
	// Only the two conventional commits are in range ⇒ conforming=true ⇒ §5 PASS.
	if !strings.Contains(out, "conforming=true") || !strings.Contains(out, "[PASS] §5") {
		t.Fatalf("expected conforming=true / §5 PASS for a since-bounded conventional range:\n%s", out)
	}
	data, _ := os.ReadFile(filepath.Join(outDir, "v0.2.0.md"))
	if !strings.Contains(string(data), "Changes since `v0.1.0`") {
		t.Fatalf("expected since-bound header:\n%s", data)
	}
}

// --- HRD-045 gate-retest ---

func TestGateRetest_RetestPassed_Allows(t *testing.T) {
	out, err := run(t, newGateRetestCmd(), "--retest-result", "pass", "--tiers", "8", "--emit")
	if err != nil {
		t.Fatalf("gate-retest with a green all-tier retest should ALLOW, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[PASS] §11.4.40") {
		t.Fatalf("expected §11.4.40 PASS verdict:\n%s", out)
	}
	if !strings.Contains(out, "retest=green") {
		t.Fatalf("expected normalized retest=green:\n%s", out)
	}
	if !strings.Contains(out, "[emit] §11.4.40 drove a constitution event") {
		t.Fatalf("expected emit confirmation:\n%s", out)
	}
}

func TestGateRetest_RetestFailed_Blocks(t *testing.T) {
	out, err := run(t, newGateRetestCmd(), "--retest-result", "fail", "--tiers", "8")
	if err == nil || !strings.Contains(err.Error(), "§11.4.40") {
		t.Fatalf("gate-retest with a RED retest should BLOCK (§11.4.40 breach), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[FAIL] §11.4.40") {
		t.Fatalf("expected §11.4.40 FAIL verdict:\n%s", out)
	}
	if !strings.Contains(out, "retest=red") {
		t.Fatalf("expected normalized retest=red:\n%s", out)
	}
}

func TestGateRetest_DefaultSkipped_Blocks(t *testing.T) {
	// No --retest-result and no --results-file ⇒ "skipped" ⇒ §11.4.40 BLOCK.
	out, err := run(t, newGateRetestCmd())
	if err == nil || !strings.Contains(err.Error(), "§11.4.40") {
		t.Fatalf("gate-retest with no recorded retest should BLOCK, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "retest=skipped") || !strings.Contains(out, "[FAIL] §11.4.40") {
		t.Fatalf("expected skipped FAIL:\n%s", out)
	}
}

func TestGateRetest_IncompleteTiers_Blocks(t *testing.T) {
	// Green but only 5 of the 8 §40.2 tiers ⇒ incomplete coverage ⇒ §11.4.40 BLOCK.
	out, err := run(t, newGateRetestCmd(), "--retest-result", "green", "--tiers", "5")
	if err == nil || !strings.Contains(err.Error(), "§11.4.40") {
		t.Fatalf("gate-retest with incomplete tier coverage should BLOCK, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[FAIL] §11.4.40") {
		t.Fatalf("expected §11.4.40 FAIL for incomplete tiers:\n%s", out)
	}
}

func TestGateRetest_ResultsFileSeam_Passes(t *testing.T) {
	rf := filepath.Join(t.TempDir(), "retest.txt")
	if err := os.WriteFile(rf, []byte("green\n(suite details on subsequent lines)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, newGateRetestCmd(), "--results-file", rf, "--tiers", "8")
	if err != nil {
		t.Fatalf("gate-retest reading a green results-file should ALLOW, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "[PASS] §11.4.40") {
		t.Fatalf("expected §11.4.40 PASS from results-file seam:\n%s", out)
	}
}

// --- registration coverage: the three §43 commands are wired, no stubs remain ---

func TestRegisterReleaseOps_AllThreeCommandsLive(t *testing.T) {
	root := &cobra.Command{Use: "rherald-test"}
	registerReleaseOps(root)
	got := map[string]bool{}
	for _, c := range root.Commands() {
		got[c.Name()] = true
		// §107 anti-bluff: none of the three may be a StubCmd anymore.
		if strings.Contains(c.Short, "not yet implemented") {
			t.Errorf("§43 command %q is still a stub: %q", c.Name(), c.Short)
		}
	}
	for _, want := range []string{"tag-mirror", "changelog-generate", "gate-retest"} {
		if !got[want] {
			t.Errorf("missing live §43 command %q", want)
		}
	}
}
