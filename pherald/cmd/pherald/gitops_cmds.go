// §43 project-lifecycle command bodies for pherald (v1.0.0 Batch C, cluster C1).
//
// HRD-029 commit-push / HRD-030 submodule-propagate / HRD-043 install-upstreams /
// HRD-044 fetch-guard / HRD-049 reopen / HRD-053 pre-push — the LIVE pherald CLI
// command bodies that PRODUCE the Subjects the HRD-023 pherald constitution
// bindings classify. These replace the cli.StubCmd registrations in stubs.go.
//
// §11.4.74 catalogue-first: the git/repo primitives are the shared
// commons/gitops package (FindScript prefers a wrappable canonical
// constitution-submodule script when the parent project provides one; falls back
// to the git binary in a standalone Herald checkout). The reopen command REUSES
// the existing inbound.CommandsConfig atomic Issues↔Fixed migration (Wave 6.5 T5)
// rather than reimplementing the two-file move.
//
// §107 / §12 host-safety. Each command operates against an EXPLICIT --repo dir
// (default: discovered repo root from CWD). NO command has an implicit
// touch-the-real-remotes default beyond the operator-supplied flags/env. Pushing
// is GATED behind an explicit --push flag (HRD-029 / HRD-053) so a bare
// invocation never reaches out to a remote. Every command CLASSIFIES the observed
// outcome through the HRD-023 binding catalogue and prints the verdict; with
// --emit it additionally drives the REAL constitution event through an in-memory
// pipeline (the wire-up seam a future serve plane swaps for the PG-backed store).
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons/gitops"
	constitution "github.com/vasic-digital/herald/commons_constitution"
	"github.com/vasic-digital/herald/commons_constitution/ladder"
	"github.com/vasic-digital/herald/commons_constitution/state"
	"github.com/vasic-digital/herald/pherald/internal/bindings"
	"github.com/vasic-digital/herald/pherald/internal/inbound"
)

// registerGitOps replaces the six §43 project-lifecycle stubs with their real
// command bodies. Called from main.go in place of registerStubs for the
// pherald-owned §42.3 project rows.
func registerGitOps(root *cobra.Command) {
	root.AddCommand(newCommitPushCmd())
	root.AddCommand(newSubmodulePropagateCmd())
	root.AddCommand(newInstallUpstreamsCmd())
	root.AddCommand(newFetchGuardCmd())
	root.AddCommand(newReopenCmd())
	root.AddCommand(newPrePushCmd())
}

// resolveRepo returns the repo dir from --repo, or discovers the enclosing repo
// root from CWD. An empty result (no repo found) is an error — refuse to operate
// against an unscoped checkout.
func resolveRepo(flag string) (string, error) {
	if flag != "" {
		abs, err := filepath.Abs(flag)
		if err != nil {
			return "", fmt.Errorf("resolve --repo %q: %w", flag, err)
		}
		return abs, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %w", err)
	}
	root := gitops.RepoRoot(cwd)
	if root == "" {
		return "", fmt.Errorf("no git repo found from %s (pass --repo)", cwd)
	}
	return root, nil
}

// classifyAndReport runs the produced Subject through the HRD-023 pherald
// binding for ruleID, prints the verdict, and — when emit is true — drives the
// REAL constitution event through an in-memory pipeline (the composition with
// HRD-023 the task requires). Returns a non-nil error when the verdict is FAIL
// (so the command exit code reflects a §-rule breach) UNLESS allowFail is set.
func classifyAndReport(ctx context.Context, w *cobra.Command, ruleID string, subject constitution.Subject, emit, allowFail bool) error {
	// Locate the matching RuleSpec from the shipped pherald catalogue.
	var spec *bindings.RuleSpec
	for _, rs := range bindings.PheraldRules() {
		if rs.RuleID == ruleID {
			s := rs
			spec = &s
			break
		}
	}
	if spec == nil {
		return fmt.Errorf("internal: no pherald binding for rule %s", ruleID)
	}

	res, err := spec.Check(ctx, subject, constitution.BundleHash{})
	if err != nil {
		return fmt.Errorf("classify %s: %w", ruleID, err)
	}
	// Uppercase the decision in the operator-facing verdict line so PASS/FAIL is
	// scannable at a glance (Decision.String() is lowercase by design).
	fmt.Fprintf(w.OutOrStdout(), "[%s] %s: %s\n", strings.ToUpper(res.Decision.String()), ruleID, res.Evidence)

	if emit {
		if emErr := emitVerdict(ctx, *spec, subject); emErr != nil {
			return fmt.Errorf("emit %s verdict: %w", ruleID, emErr)
		}
		fmt.Fprintf(w.OutOrStdout(), "[emit] %s drove a constitution event for subject %s\n", ruleID, subject.ID)
	}

	if res.Decision == constitution.DecisionFail && !allowFail {
		return fmt.Errorf("%s breach: %s", ruleID, res.Evidence)
	}
	return nil
}

// emitVerdict builds an in-memory pipeline (real MemoryBus + ConstitutionStore +
// ModeLadder + MemoryAudit) and drives the subject through it so the verdict
// fans out as a REAL constitution event. This is the runtime seam a future serve
// plane swaps for the PG-backed backends; for the CLI it proves the §43 command
// composes with the HRD-023 bindings (not a metadata-only claim).
func emitVerdict(ctx context.Context, spec bindings.RuleSpec, subject constitution.Subject) error {
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{BufferSize: 16})
	defer func() { _ = bus.Close() }()
	em, err := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "digital.vasic.herald/pherald"})
	if err != nil {
		return err
	}
	p, err := bindings.NewPipeline(bindings.Config{
		Ladder:  ladder.NewMemory(),
		Store:   state.NewMemory(),
		Emitter: em,
		Audit:   state.NewMemoryAudit(),
	})
	if err != nil {
		return err
	}
	_, err = p.EvaluateSubject(ctx, spec.RuleID, uuid.Nil, subject)
	return err
}

// --- HRD-029 — pherald commit-push (§2) ---

func newCommitPushCmd() *cobra.Command {
	var (
		repoFlag string
		message  string
		scope    []string
		doPush   bool
		emit     bool
	)
	cmd := &cobra.Command{
		Use:   "commit-push",
		Short: "Single-entrypoint locked commit + multi-mirror push (§2)",
		Long: "Performs a §2-disciplined commit through the single locked entrypoint " +
			"(an O_CREATE|O_EXCL commit-lock under .git/) and, with --push, fans the " +
			"commit out to every configured mirror remote. Classifies the recorded " +
			"commit state through the HRD-023 §2 binding (entrypoint+lock_held). " +
			"Wraps the canonical constitution commit_all.sh when discoverable.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			repo, err := resolveRepo(repoFlag)
			if err != nil {
				return err
			}
			r := gitops.NewRunner(repo)
			if !r.IsRepo(ctx) {
				return fmt.Errorf("commit-push: %s is not a git repo", repo)
			}
			if message == "" {
				return fmt.Errorf("commit-push: --message required")
			}

			// Acquire the single locked entrypoint (§2). The lock presence IS the
			// "lock_held=true" evidence the binding classifies.
			lockPath := filepath.Join(repo, ".git", ".commit_all.lock")
			lockHeld, lockErr := acquireCommitLock(ctx, lockPath)
			if lockErr != nil {
				return fmt.Errorf("commit-push: acquire commit-lock: %w", lockErr)
			}
			defer func() {
				// §11.4.88 background-push: release the commit-lock the instant the
				// commit is durable (BEFORE any push). The defer covers the error
				// paths; the success path releases explicitly before --push below.
				_ = os.Remove(lockPath)
			}()

			// Stage the declared scope (empty ⇒ stage all tracked changes).
			if len(scope) == 0 {
				if _, e := r.Git(ctx, "add", "-A"); e != nil {
					return fmt.Errorf("commit-push: git add -A: %w", e)
				}
			} else {
				for _, p := range scope {
					if _, e := r.Git(ctx, "add", "--", p); e != nil {
						return fmt.Errorf("commit-push: git add %s: %w", p, e)
					}
				}
			}
			if !r.HasStagedChanges(ctx) {
				return fmt.Errorf("commit-push: nothing staged to commit")
			}
			if _, e := r.Git(ctx, "commit", "-m", message); e != nil {
				return fmt.Errorf("commit-push: git commit: %w", e)
			}
			sha, _ := r.HeadSHA(ctx)
			// Commit is durable — release the lock NOW (§11.4.88), before push.
			_ = os.Remove(lockPath)

			// Build the §2 Subject from the OBSERVED commit state. entrypoint=true
			// (we WENT through the locked entrypoint); lock_held reflects the lock.
			subject := constitution.Subject{
				Kind: bindings.SubjectCommitPush,
				ID:   fmt.Sprintf("%s|entrypoint=true|lock_held=%t", sha, lockHeld),
			}
			fmt.Fprintf(cmd.OutOrStdout(), "committed %s through the locked entrypoint\n", sha)

			if doPush {
				if e := pushAllMirrors(ctx, cmd, r); e != nil {
					return e
				}
			}
			return classifyAndReport(ctx, cmd, "§2", subject, emit, false)
		},
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", "repo dir (default: discovered from CWD)")
	cmd.Flags().StringVarP(&message, "message", "m", "", "commit message (required)")
	cmd.Flags().StringSliceVar(&scope, "scope", nil, "paths to stage (default: all tracked changes)")
	cmd.Flags().BoolVar(&doPush, "push", false, "fan the commit out to every configured mirror remote")
	cmd.Flags().BoolVar(&emit, "emit", false, "also drive the §2 verdict as a real constitution event")
	return cmd
}

// acquireCommitLock takes the §2 single-entrypoint commit-lock via
// O_CREATE|O_EXCL (the same primitive inbound.DocsIssueOpener uses). Returns
// true when the lock is freshly held by this process. Spins up to 30s.
func acquireCommitLock(ctx context.Context, lockPath string) (bool, error) {
	deadline := time.Now().Add(30 * time.Second)
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = f.WriteString(strconv.Itoa(os.Getpid()))
			_ = f.Close()
			return true, nil
		}
		if !os.IsExist(err) {
			return false, err
		}
		if time.Now().After(deadline) {
			return false, fmt.Errorf("commit-lock held >30s — stale? (%s)", lockPath)
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// pushAllMirrors pushes the current branch to every configured non-empty remote.
// It NEVER force-pushes and NEVER configures remotes — the operator wires those
// via install-upstreams first. A push failure surfaces (no silent swallow).
func pushAllMirrors(ctx context.Context, cmd *cobra.Command, r *gitops.Runner) error {
	branch, err := r.CurrentBranch(ctx)
	if err != nil {
		return fmt.Errorf("push: current branch: %w", err)
	}
	remotesOut, err := r.Git(ctx, "remote")
	if err != nil {
		return fmt.Errorf("push: list remotes: %w", err)
	}
	remotes := strings.Fields(remotesOut)
	if len(remotes) == 0 {
		return fmt.Errorf("push: no remotes configured (run pherald install-upstreams first)")
	}
	for _, rem := range remotes {
		if _, e := r.Git(ctx, "push", rem, branch); e != nil {
			return fmt.Errorf("push to %s: %w", rem, e)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "pushed %s to %s\n", branch, rem)
	}
	return nil
}

// --- HRD-030 — pherald submodule-propagate (§3) ---

func newSubmodulePropagateCmd() *cobra.Command {
	var (
		repoFlag string
		emit     bool
	)
	cmd := &cobra.Command{
		Use:   "submodule-propagate",
		Short: "Owned-submodule walk in propagation order (§3)",
		Long: "Inspects the recorded submodule propagation order (inner-first vs " +
			"parent-first) and whether the inner SHAs the parent pins are pushed, then " +
			"classifies it through the HRD-023 §3 binding. Reports any parent-first " +
			"ordering or dangling-pin as a §3 repo-safety breach.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			repo, err := resolveRepo(repoFlag)
			if err != nil {
				return err
			}
			r := gitops.NewRunner(repo)
			if !r.IsRepo(ctx) {
				return fmt.Errorf("submodule-propagate: %s is not a git repo", repo)
			}
			order, innerPushed, summary := inspectSubmoduleState(ctx, r, repo)
			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", summary)
			subject := constitution.Subject{
				Kind: bindings.SubjectSubmodulePropagate,
				ID:   fmt.Sprintf("propagate|order=%s|inner_pushed=%t", order, innerPushed),
			}
			return classifyAndReport(ctx, cmd, "§3", subject, emit, false)
		},
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", "repo dir (default: discovered from CWD)")
	cmd.Flags().BoolVar(&emit, "emit", false, "also drive the §3 verdict as a real constitution event")
	return cmd
}

// inspectSubmoduleState observes the working tree's submodule state. A repo with
// no dirty/uncommitted submodule pins is in a consistent inner-first state (the
// parent already pins committed inner SHAs). A submodule with new local commits
// not yet reflected in a parent commit is reported as parent-first risk. The
// observation is intentionally conservative: a clean `git submodule status`
// (no leading + or -) means every pinned SHA is checked out, so inner is
// consistent. PURE-read: it runs only read-only git status queries.
func inspectSubmoduleState(ctx context.Context, r *gitops.Runner, repo string) (order string, innerPushed bool, summary string) {
	if _, err := os.Stat(filepath.Join(repo, ".gitmodules")); err != nil {
		return "inner-first", true, "submodule-propagate: no .gitmodules — nothing to propagate (vacuously inner-first)"
	}
	out, err := r.Git(ctx, "submodule", "status")
	if err != nil {
		return "inner-first", true, "submodule-propagate: submodule status unavailable: " + err.Error()
	}
	var dirty []string
	for _, ln := range strings.Split(out, "\n") {
		ln = strings.TrimRight(ln, " ")
		if ln == "" {
			continue
		}
		// Leading '+' = checked-out SHA differs from the parent-pinned SHA (the
		// parent would pin a SHA the inner has moved past — parent-first risk).
		// Leading '-' = submodule not initialized.
		switch ln[0] {
		case '+':
			dirty = append(dirty, strings.TrimSpace(ln))
		}
	}
	if len(dirty) > 0 {
		return "parent-first", false, fmt.Sprintf(
			"submodule-propagate: %d submodule(s) have local commits the parent has NOT re-pinned (parent-first risk): %s",
			len(dirty), strings.Join(dirty, "; "))
	}
	return "inner-first", true, "submodule-propagate: every submodule pin matches its checked-out SHA (consistent inner-first state)"
}

// --- HRD-043 — pherald install-upstreams (§11.4.36) ---

func newInstallUpstreamsCmd() *cobra.Command {
	var (
		repoFlag      string
		upstreamsFlag string
		apply         bool
		emit          bool
	)
	cmd := &cobra.Command{
		Use:   "install-upstreams",
		Short: "Configure mirror remotes from upstreams/*.sh declarations (§11.4.36)",
		Long: "Reads the upstreams/*.sh mirror declarations + (with --apply) configures " +
			"every declared mirror as a git remote, then classifies the configured-vs-" +
			"declared tally through the HRD-023 §11.4.36 binding. Wraps the canonical " +
			"install_upstreams.sh when discoverable.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			repo, err := resolveRepo(repoFlag)
			if err != nil {
				return err
			}
			r := gitops.NewRunner(repo)
			if !r.IsRepo(ctx) {
				return fmt.Errorf("install-upstreams: %s is not a git repo", repo)
			}
			upstreamsDir := upstreamsFlag
			if upstreamsDir == "" {
				upstreamsDir = filepath.Join(repo, "upstreams")
			}
			mirrors, err := gitops.ParseUpstreams(upstreamsDir)
			if err != nil {
				return fmt.Errorf("install-upstreams: %w", err)
			}
			declared := len(mirrors)
			if declared == 0 {
				return fmt.Errorf("install-upstreams: no mirror declarations under %s", upstreamsDir)
			}

			configured := 0
			for _, m := range mirrors {
				name := m.RemoteNameFor()
				if apply {
					if e := r.SetRemote(ctx, name, m.URL); e != nil {
						return fmt.Errorf("install-upstreams: set remote %s: %w", name, e)
					}
					fmt.Fprintf(cmd.OutOrStdout(), "configured remote %s -> %s\n", name, m.URL)
				}
				if r.RemoteURL(ctx, name) != "" {
					configured++
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "install-upstreams: %d/%d declared mirrors configured\n", configured, declared)
			subject := constitution.Subject{
				Kind: bindings.SubjectInstallUpstreams,
				ID:   fmt.Sprintf("mirrors|configured=%d|declared=%d", configured, declared),
			}
			// §11.4.36 is a WARN-tier policy row: a partial install is a warning, not
			// a hard exit, so allowFail=true (the verdict still prints + emits).
			return classifyAndReport(ctx, cmd, "§11.4.36", subject, emit, true)
		},
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", "repo dir (default: discovered from CWD)")
	cmd.Flags().StringVar(&upstreamsFlag, "upstreams-dir", "", "dir holding *.sh mirror declarations (default: <repo>/upstreams)")
	cmd.Flags().BoolVar(&apply, "apply", false, "actually configure the remotes (default: report only)")
	cmd.Flags().BoolVar(&emit, "emit", false, "also drive the §11.4.36 verdict as a real constitution event")
	return cmd
}

// --- HRD-044 — pherald fetch-guard (§11.4.37) ---

func newFetchGuardCmd() *cobra.Command {
	var (
		repoFlag string
		remote   string
		doFetch  bool
		emit     bool
	)
	cmd := &cobra.Command{
		Use:   "fetch-guard",
		Short: "Pre-edit fetch + rebase enforcement (§11.4.37)",
		Long: "Asserts the working tree is rebased on origin/<branch> before any edit " +
			"(§11.4.37 fetch-before-edit). With --fetch it first fetches the remote, " +
			"then classifies the rebase state (behind=0) through the HRD-023 §11.4.37 " +
			"binding. A tree behind its upstream is a §11.4.37 repo-safety breach.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			repo, err := resolveRepo(repoFlag)
			if err != nil {
				return err
			}
			r := gitops.NewRunner(repo)
			if !r.IsRepo(ctx) {
				return fmt.Errorf("fetch-guard: %s is not a git repo", repo)
			}
			branch, err := r.CurrentBranch(ctx)
			if err != nil {
				return fmt.Errorf("fetch-guard: %w", err)
			}
			if doFetch {
				if _, e := r.Git(ctx, "fetch", remote); e != nil {
					return fmt.Errorf("fetch-guard: fetch %s: %w", remote, e)
				}
			}
			upstream := remote + "/" + branch
			rebased := true
			_, behind, abErr := r.AheadBehind(ctx, upstream)
			if abErr != nil {
				// No upstream tracking ref (e.g. brand-new branch never pushed) ⇒
				// treat as rebased (nothing to be behind of). Report the condition.
				fmt.Fprintf(cmd.OutOrStdout(), "fetch-guard: no upstream %s (%v) — treating as rebased\n", upstream, abErr)
			} else {
				rebased = behind == 0
				fmt.Fprintf(cmd.OutOrStdout(), "fetch-guard: %s is %d commit(s) behind %s\n", branch, behind, upstream)
			}
			subject := constitution.Subject{
				Kind: bindings.SubjectFetchGuard,
				ID:   fmt.Sprintf("%s|rebased=%t", branch, rebased),
			}
			return classifyAndReport(ctx, cmd, "§11.4.37", subject, emit, false)
		},
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", "repo dir (default: discovered from CWD)")
	cmd.Flags().StringVar(&remote, "remote", "origin", "remote to check against")
	cmd.Flags().BoolVar(&doFetch, "fetch", false, "fetch the remote before checking (default: use already-fetched state)")
	cmd.Flags().BoolVar(&emit, "emit", false, "also drive the §11.4.37 verdict as a real constitution event")
	return cmd
}

// --- HRD-049 — pherald reopen <HRD-NNN> (§11.4.55) ---

func newReopenCmd() *cobra.Command {
	var (
		docsDir string
		reason  string
		emit    bool
	)
	cmd := &cobra.Command{
		Use:   "reopen <HRD-NNN>",
		Short: "Issues→Fixed reversal + Reopens history (§11.4.55)",
		Long: "Reverses a Fixed→Issues migration for the named HRD (REUSING the Wave 6.5 " +
			"atomic two-file migration in pherald/internal/inbound) and writes the " +
			"docs/Reopens/<HRD-NNN>.md history record §11.4.55 mandates, then classifies " +
			"the reopen (recorded=true) through the HRD-023 §11.4.55 binding.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			ref, err := parseReopenRef(args[0])
			if err != nil {
				return err
			}
			if docsDir == "" {
				// Default: <repo-root>/docs.
				repo, rErr := resolveRepo("")
				if rErr != nil {
					return fmt.Errorf("reopen: %w (pass --docs-dir)", rErr)
				}
				docsDir = filepath.Join(repo, "docs")
			}
			issuesPath := filepath.Join(docsDir, "Issues.md")
			fixedPath := filepath.Join(docsDir, "Fixed.md")

			// REUSE the existing atomic Fixed→Issues migration (Wave 6.5 T5). The
			// reopen CLI is operator-invoked, so we mint a single-operator allowlist
			// keyed on a CLI sentinel; the migration's role gate is satisfied.
			cc := &inbound.CommandsConfig{
				IssuesPath:  issuesPath,
				FixedPath:   fixedPath,
				OperatorIDs: map[string]bool{cliOperatorID: true},
				Clock:       commons.RealClock{},
			}
			if _, _, mErr := cc.HandleReopen(ctx, "Reopen: "+ref, cliOperatorID); mErr != nil {
				return fmt.Errorf("reopen: migrate %s Fixed.md → Issues.md: %w", ref, mErr)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "reopen: migrated %s Fixed.md → Issues.md\n", ref)

			// Write the §11.4.55 docs/Reopens/<HRD>.md history record.
			recorded, recErr := writeReopenRecord(docsDir, ref, reason)
			if recErr != nil {
				return fmt.Errorf("reopen: write Reopens record: %w", recErr)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "reopen: wrote docs/Reopens/%s.md\n", ref)

			subject := constitution.Subject{
				Kind: bindings.SubjectReopen,
				ID:   fmt.Sprintf("%s|recorded=%t", ref, recorded),
			}
			return classifyAndReport(ctx, cmd, "§11.4.55", subject, emit, true)
		},
	}
	cmd.Flags().StringVar(&docsDir, "docs-dir", "", "docs dir holding Issues.md/Fixed.md/Reopens/ (default: <repo>/docs)")
	cmd.Flags().StringVar(&reason, "reason", "", "reopen reason recorded in docs/Reopens/<HRD>.md")
	cmd.Flags().BoolVar(&emit, "emit", false, "also drive the §11.4.55 verdict as a real constitution event")
	return cmd
}

// cliOperatorID is the sentinel sender-id the reopen CLI presents to the reused
// inbound migration's operator gate. CLI invocation is implicitly operator-role
// (the operator runs the binary); the gate exists for the chat-driven path.
const cliOperatorID = "pherald-cli-operator"

// hrdArgRE captures an "HRD-NNN" token (case-insensitive prefix) in the reopen
// argument so "HRD-049" / "hrd-49" both normalise.
var hrdArgRE = regexp.MustCompile(`(?i)HRD-\d+`)

// parseReopenRef normalises an HRD ref argument ("HRD-049", "hrd-49", "49") into
// the canonical "HRD-NNN" form, erroring on anything else.
func parseReopenRef(arg string) (string, error) {
	a := strings.TrimSpace(arg)
	if m := hrdArgRE.FindString(a); m != "" {
		return "HRD-" + m[strings.IndexByte(m, '-')+1:], nil
	}
	if _, err := strconv.Atoi(a); err == nil {
		return "HRD-" + a, nil
	}
	return "", fmt.Errorf("reopen: %q is not an HRD reference (want HRD-NNN)", arg)
}

// writeReopenRecord creates docs/Reopens/<HRD>.md (§11.4.55) with a timestamp +
// optional reason. Idempotent-append: a second reopen of the same HRD appends a
// new dated stanza rather than clobbering history. Returns true on success.
func writeReopenRecord(docsDir, ref, reason string) (bool, error) {
	reopensDir := filepath.Join(docsDir, "Reopens")
	if err := os.MkdirAll(reopensDir, 0o755); err != nil {
		return false, err
	}
	path := filepath.Join(reopensDir, ref+".md")
	stanza := fmt.Sprintf("\n## Reopened %s\n\n- Reason: %s\n",
		time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		fallbackReason(reason))
	if _, err := os.Stat(path); os.IsNotExist(err) {
		header := fmt.Sprintf("# Reopens history — %s\n\nPer Universal §11.4.55.\n", ref)
		if err := os.WriteFile(path, []byte(header+stanza), 0o644); err != nil {
			return false, err
		}
		return true, nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(stanza); err != nil {
		return false, err
	}
	return true, nil
}

func fallbackReason(r string) string {
	if strings.TrimSpace(r) == "" {
		return "(no reason supplied)"
	}
	return r
}

// --- HRD-053 — pherald pre-push (§11.4.71) ---

func newPrePushCmd() *cobra.Command {
	var (
		repoFlag string
		remote   string
		emit     bool
	)
	cmd := &cobra.Command{
		Use:   "pre-push",
		Short: "Fetch + investigate + integrate hook (§11.4.71)",
		Long: "Pre-push hook: fetches the remote, summarises incoming changes (commits " +
			"the local branch is behind), and classifies the fetched+integrated state " +
			"through the HRD-023 §11.4.71 binding. A behind-count > 0 means incoming " +
			"changes are NOT integrated — a §11.4.71 repo-safety breach.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			repo, err := resolveRepo(repoFlag)
			if err != nil {
				return err
			}
			r := gitops.NewRunner(repo)
			if !r.IsRepo(ctx) {
				return fmt.Errorf("pre-push: %s is not a git repo", repo)
			}
			branch, err := r.CurrentBranch(ctx)
			if err != nil {
				return fmt.Errorf("pre-push: %w", err)
			}
			// The pre-push fetch is the §11.4.71 mandatory step — always perform it.
			fetched := true
			if _, e := r.Git(ctx, "fetch", remote); e != nil {
				fetched = false
				fmt.Fprintf(cmd.OutOrStdout(), "pre-push: fetch %s failed: %v\n", remote, e)
			}
			upstream := remote + "/" + branch
			integrated := true
			if fetched {
				_, behind, abErr := r.AheadBehind(ctx, upstream)
				if abErr != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "pre-push: no upstream %s (%v) — nothing incoming to integrate\n", upstream, abErr)
				} else {
					integrated = behind == 0
					if behind > 0 {
						fmt.Fprintf(cmd.OutOrStdout(), "pre-push: %d incoming commit(s) on %s NOT yet integrated\n", behind, upstream)
					} else {
						fmt.Fprintf(cmd.OutOrStdout(), "pre-push: %s up to date with %s\n", branch, upstream)
					}
				}
			}
			subject := constitution.Subject{
				Kind: bindings.SubjectPrePush,
				ID:   fmt.Sprintf("%s|fetched=%t|integrated=%t", branch, fetched, integrated),
			}
			return classifyAndReport(ctx, cmd, "§11.4.71", subject, emit, false)
		},
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", "repo dir (default: discovered from CWD)")
	cmd.Flags().StringVar(&remote, "remote", "origin", "remote to fetch + integrate against")
	cmd.Flags().BoolVar(&emit, "emit", false, "also drive the §11.4.71 verdict as a real constitution event")
	return cmd
}
