// Package gitops provides the REAL git/repo primitives the Herald §43 flavor
// command bodies build on. It lives in the L0 commons module (pure stdlib, zero
// external deps) so every flavor binary can share it: pherald
// (HRD-029/030/043/044/049/053 — commit-push / submodule-propagate /
// install-upstreams / fetch-guard / reopen / pre-push) and the §43 git-bearing
// commands of sherald (constitution-pull / force-push-gate / destructive-guard),
// rherald (tag-mirror), etc. Promoted here from pherald/internal/gitops so the
// primitives are not duplicated across flavor modules (§11.4.74 catalogue-first).
//
// Why a dedicated package (§11.4.74 catalogue-first posture): the parent
// project's constitution submodule ships canonical shell scripts
// (commit_all.sh / push_all.sh / install_upstreams.sh / sync_issues_docs.sh)
// that these commands SHOULD wrap when present. In a standalone Herald
// checkout (no parent project ⇒ no discovered constitution/ submodule) those
// scripts are absent, so the command bodies fall back to the git binary
// directly. gitops centralises both paths:
//
//   - FindScript walks up from a start dir looking for a named wrappable
//     script (constitution/<name> or repo-root/<name>); commands prefer it.
//   - Run* helpers invoke the `git` binary inside an explicit repo dir so
//     every operation is scoped to a caller-supplied checkout — the seam
//     that makes the command tests 100% HERMETIC (t.TempDir + git init +
//     file:// fake remotes; NEVER the real origin/mirrors).
//
// §107 / §12 host-safety: every operation runs against an EXPLICIT repo dir
// passed by the caller. There is no implicit "current repo" default that
// could touch this checkout's real remotes during a test. The command layer
// (commit-push / submodule-propagate / install-upstreams / fetch-guard /
// pre-push) is the ONLY place a real-remote default is resolved, and only
// from operator-supplied flags/env at runtime.
package gitops

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Runner runs git commands against an explicit repo working directory.
// Construct one per command invocation with the caller-resolved repo dir.
// A zero Runner is invalid — use NewRunner.
type Runner struct {
	// Dir is the git working directory every command runs in. Required.
	Dir string
	// GitBin is the git executable. Empty ⇒ "git" resolved from $PATH.
	GitBin string
}

// NewRunner returns a Runner bound to repoDir. repoDir MUST be a real
// directory; callers resolve it from a --repo flag or RepoRoot discovery.
func NewRunner(repoDir string) *Runner {
	return &Runner{Dir: repoDir}
}

func (r *Runner) bin() string {
	if r.GitBin != "" {
		return r.GitBin
	}
	return "git"
}

// Git runs `git <args...>` in r.Dir and returns trimmed stdout. A non-zero
// exit surfaces as an error carrying the combined stderr (no silent swallow
// — §11.4.6 no-guessing).
func (r *Runner) Git(ctx context.Context, args ...string) (string, error) {
	if r.Dir == "" {
		return "", fmt.Errorf("gitops: empty repo dir (refusing to run git against an unscoped checkout)")
	}
	cmd := exec.CommandContext(ctx, r.bin(), args...)
	cmd.Dir = r.Dir
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return strings.TrimRight(stdout.String(), "\n"), fmt.Errorf("gitops: git %s: %w: %s",
			strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimRight(stdout.String(), "\n"), nil
}

// IsRepo reports whether r.Dir is inside a git work tree.
func (r *Runner) IsRepo(ctx context.Context) bool {
	out, err := r.Git(ctx, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// CurrentBranch returns the short symbolic ref of HEAD (e.g. "main").
func (r *Runner) CurrentBranch(ctx context.Context) (string, error) {
	return r.Git(ctx, "rev-parse", "--abbrev-ref", "HEAD")
}

// HeadSHA returns the short SHA of HEAD.
func (r *Runner) HeadSHA(ctx context.Context) (string, error) {
	return r.Git(ctx, "rev-parse", "--short", "HEAD")
}

// HasStagedChanges reports whether the index has staged changes (exit-code
// semantics of `git diff --cached --quiet`).
func (r *Runner) HasStagedChanges(ctx context.Context) bool {
	_, err := r.Git(ctx, "diff", "--cached", "--quiet")
	return err != nil // non-zero exit ⇒ there ARE staged changes
}

// AheadBehind returns how many commits the local branch is ahead/behind the
// given upstream ref (e.g. "origin/main"). It does NOT fetch — call Fetch
// first. Returns (ahead, behind, err).
func (r *Runner) AheadBehind(ctx context.Context, upstream string) (ahead, behind int, err error) {
	out, gerr := r.Git(ctx, "rev-list", "--left-right", "--count", "HEAD..."+upstream)
	if gerr != nil {
		return 0, 0, gerr
	}
	// `--left-right --count A...B` prints "<left>\t<right>" where left =
	// commits in HEAD not in upstream (ahead), right = commits in upstream
	// not in HEAD (behind).
	fields := strings.Fields(out)
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("gitops: unexpected rev-list output %q", out)
	}
	if _, e := fmt.Sscanf(fields[0], "%d", &ahead); e != nil {
		return 0, 0, fmt.Errorf("gitops: parse ahead %q: %w", fields[0], e)
	}
	if _, e := fmt.Sscanf(fields[1], "%d", &behind); e != nil {
		return 0, 0, fmt.Errorf("gitops: parse behind %q: %w", fields[1], e)
	}
	return ahead, behind, nil
}

// RemoteURL returns the configured URL for the named remote, or "" if the
// remote is not configured.
func (r *Runner) RemoteURL(ctx context.Context, name string) string {
	out, err := r.Git(ctx, "remote", "get-url", name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// SetRemote idempotently configures (adds or updates) a named remote to url.
func (r *Runner) SetRemote(ctx context.Context, name, url string) error {
	if r.RemoteURL(ctx, name) != "" {
		_, err := r.Git(ctx, "remote", "set-url", name, url)
		return err
	}
	_, err := r.Git(ctx, "remote", "add", name, url)
	return err
}

// TagExists reports whether tag is present in the local repo's tag list
// (`git tag -l <tag>` prints the tag name on a hit, nothing on a miss).
func (r *Runner) TagExists(ctx context.Context, tag string) bool {
	out, err := r.Git(ctx, "tag", "-l", tag)
	return err == nil && strings.TrimSpace(out) == tag
}

// RemoteHasTag reports whether the named remote carries tag, observed via
// `git ls-remote --tags <remote> refs/tags/<tag>`. A non-empty match line ⇒
// the remote has the tag. Read-only — it never pushes or creates the tag.
func (r *Runner) RemoteHasTag(ctx context.Context, remote, tag string) bool {
	out, err := r.Git(ctx, "ls-remote", "--tags", remote, "refs/tags/"+tag)
	if err != nil {
		return false
	}
	return strings.Contains(out, "refs/tags/"+tag)
}

// LogSubjects returns the commit subjects (one per line, newest-first) reachable
// from HEAD, optionally bounded to the range since..HEAD when since is non-empty
// (e.g. a previous tag). Uses `git log --pretty=%s`. Read-only.
func (r *Runner) LogSubjects(ctx context.Context, since string) ([]string, error) {
	args := []string{"log", "--pretty=%s"}
	if since != "" {
		args = append(args, since+"..HEAD")
	}
	out, err := r.Git(ctx, args...)
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// RepoRoot walks up from startDir looking for the enclosing git repo root
// (the dir containing .git). Returns "" if none is found before the
// filesystem root. PURE filesystem walk — never runs git.
func RepoRoot(startDir string) string {
	dir := startDir
	for {
		if fi, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			_ = fi
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// FindScript walks up from startDir looking for a wrappable canonical script
// named name, checking <dir>/constitution/<name> then <dir>/<name> at each
// level (the §11.4.74 catalogue-first preference: wrap the canonical script
// when the parent project provides it). Returns the absolute path + true on
// the first hit, ("", false) when no copy is discoverable. PURE filesystem
// walk — never executes anything.
func FindScript(startDir, name string) (string, bool) {
	dir := startDir
	for {
		for _, cand := range []string{
			filepath.Join(dir, "constitution", name),
			filepath.Join(dir, name),
		} {
			if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
				abs, aerr := filepath.Abs(cand)
				if aerr != nil {
					abs = cand
				}
				return abs, true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
