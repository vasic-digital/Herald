package gitops_test

// HRD-029/030/043/044/049/053 — gitops primitives, written TDD-first.
//
// EVERY test here is 100% HERMETIC: it builds throwaway git repos under
// t.TempDir() with `git init` and file:// fake remotes. NO test touches the
// real Herald origin/mirrors, runs a real push to this checkout's remotes,
// or relies on any ambient git state. The Runner is always bound to a
// t.TempDir() repo dir — the gitops package has no implicit "current repo"
// default.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/vasic-digital/herald/commons/gitops"
)

// initRepo creates a fresh git repo in a t.TempDir() subdir with one commit
// on main, and returns the repo dir. Identity is set LOCALLY (repo-scoped)
// so it never reads/writes the developer's global git config.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustGit(t, dir, "init", "-q", "-b", "main")
	mustGit(t, dir, "config", "user.email", "test@herald.local")
	mustGit(t, dir, "config", "user.name", "Herald Test")
	mustGit(t, dir, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# hermetic\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	mustGit(t, dir, "add", "README.md")
	mustGit(t, dir, "commit", "-q", "-m", "seed")
	return dir
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func TestRunner_IsRepoAndBranch(t *testing.T) {
	dir := initRepo(t)
	r := gitops.NewRunner(dir)
	ctx := context.Background()
	if !r.IsRepo(ctx) {
		t.Fatalf("IsRepo(%s) = false, want true", dir)
	}
	br, err := r.CurrentBranch(ctx)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if br != "main" {
		t.Fatalf("CurrentBranch = %q, want main", br)
	}
}

func TestRunner_IsRepo_False_OnNonRepo(t *testing.T) {
	r := gitops.NewRunner(t.TempDir())
	if r.IsRepo(context.Background()) {
		t.Fatalf("IsRepo on a non-repo temp dir = true, want false")
	}
}

func TestRunner_EmptyDir_Errors(t *testing.T) {
	r := gitops.NewRunner("")
	if _, err := r.Git(context.Background(), "status"); err == nil {
		t.Fatalf("Git on empty-dir Runner = nil error, want refusal")
	}
}

func TestRunner_HasStagedChanges(t *testing.T) {
	dir := initRepo(t)
	r := gitops.NewRunner(dir)
	ctx := context.Background()
	if r.HasStagedChanges(ctx) {
		t.Fatalf("HasStagedChanges on a clean tree = true")
	}
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", "new.txt")
	if !r.HasStagedChanges(ctx) {
		t.Fatalf("HasStagedChanges after staging = false, want true")
	}
}

func TestRunner_AheadBehind(t *testing.T) {
	// Build a bare "remote" repo + a clone, then make the clone 2 ahead.
	remote := t.TempDir()
	mustGit(t, remote, "init", "-q", "--bare", "-b", "main")

	work := initRepo(t)
	r := gitops.NewRunner(work)
	ctx := context.Background()
	if err := r.SetRemote(ctx, "origin", "file://"+remote); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}
	mustGit(t, work, "push", "-q", "origin", "main")
	// 2 local commits ahead.
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(work, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
		mustGit(t, work, "add", name)
		mustGit(t, work, "commit", "-q", "-m", "add "+name)
	}
	mustGit(t, work, "fetch", "-q", "origin")
	ahead, behind, err := r.AheadBehind(ctx, "origin/main")
	if err != nil {
		t.Fatalf("AheadBehind: %v", err)
	}
	if ahead != 2 || behind != 0 {
		t.Fatalf("AheadBehind = (%d,%d), want (2,0)", ahead, behind)
	}
}

func TestRunner_SetRemote_Idempotent(t *testing.T) {
	dir := initRepo(t)
	r := gitops.NewRunner(dir)
	ctx := context.Background()
	if got := r.RemoteURL(ctx, "github"); got != "" {
		t.Fatalf("RemoteURL on fresh repo = %q, want empty", got)
	}
	if err := r.SetRemote(ctx, "github", "file:///tmp/fake1"); err != nil {
		t.Fatalf("SetRemote add: %v", err)
	}
	if got := r.RemoteURL(ctx, "github"); got != "file:///tmp/fake1" {
		t.Fatalf("RemoteURL after add = %q", got)
	}
	// Re-set updates the URL (idempotent add-or-update).
	if err := r.SetRemote(ctx, "github", "file:///tmp/fake2"); err != nil {
		t.Fatalf("SetRemote update: %v", err)
	}
	if got := r.RemoteURL(ctx, "github"); got != "file:///tmp/fake2" {
		t.Fatalf("RemoteURL after update = %q", got)
	}
}

func TestRepoRoot(t *testing.T) {
	dir := initRepo(t)
	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := gitops.RepoRoot(sub); got != dir {
		t.Fatalf("RepoRoot(%s) = %q, want %q", sub, got, dir)
	}
	if got := gitops.RepoRoot(t.TempDir()); got != "" {
		t.Fatalf("RepoRoot on non-repo = %q, want empty", got)
	}
}

func TestFindScript(t *testing.T) {
	root := t.TempDir()
	// constitution/<name> preferred over repo-root/<name>.
	constDir := filepath.Join(root, "constitution")
	if err := os.MkdirAll(constDir, 0o755); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(constDir, "commit_all.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/bash\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "x", "y")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got, ok := gitops.FindScript(sub, "commit_all.sh")
	if !ok {
		t.Fatalf("FindScript did not find commit_all.sh from %s", sub)
	}
	absWant, _ := filepath.Abs(scriptPath)
	if got != absWant {
		t.Fatalf("FindScript = %q, want %q", got, absWant)
	}
	if _, ok := gitops.FindScript(t.TempDir(), "nonexistent.sh"); ok {
		t.Fatalf("FindScript found a nonexistent script")
	}
}

func TestParseUpstreams(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"GitHub.sh": "#!/bin/bash\n\nexport UPSTREAMABLE_REPOSITORY=\"git@github.com:vasic-digital/Herald.git\"\n",
		"GitLab.sh": "#!/bin/bash\n\nexport UPSTREAMABLE_REPOSITORY=\"git@gitlab.com:vasic-digital/herald.git\"\n",
		"notes.txt": "ignored — not .sh\n",
		"helper.sh": "#!/bin/bash\necho no upstream here\n", // no assignment ⇒ skipped
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mirrors, err := gitops.ParseUpstreams(dir)
	if err != nil {
		t.Fatalf("ParseUpstreams: %v", err)
	}
	if len(mirrors) != 2 {
		t.Fatalf("ParseUpstreams found %d mirrors, want 2: %+v", len(mirrors), mirrors)
	}
	// Sorted by name: GitHub, GitLab.
	if mirrors[0].Name != "GitHub" || mirrors[0].URL != "git@github.com:vasic-digital/Herald.git" {
		t.Fatalf("mirror[0] = %+v", mirrors[0])
	}
	if mirrors[1].Name != "GitLab" {
		t.Fatalf("mirror[1] = %+v", mirrors[1])
	}
	if mirrors[0].RemoteNameFor() != "github" {
		t.Fatalf("RemoteNameFor = %q, want github", mirrors[0].RemoteNameFor())
	}
}

func TestParseUpstreams_MissingDir(t *testing.T) {
	mirrors, err := gitops.ParseUpstreams(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("ParseUpstreams on missing dir = %v, want nil error", err)
	}
	if mirrors != nil {
		t.Fatalf("ParseUpstreams on missing dir = %+v, want nil", mirrors)
	}
}
