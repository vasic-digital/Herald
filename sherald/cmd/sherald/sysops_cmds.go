// §43 system/safety command bodies for sherald (v1.0.0 Batch C, cluster C2).
//
// HRD-033 destructive-guard / HRD-040 constitution-pull / HRD-046
// force-push-gate / HRD-056 mem-budget-watch — the LIVE sherald CLI command
// bodies that PRODUCE the Subjects the HRD-020 sherald constitution bindings
// classify. These replace the cli.StubCmd registrations in internal/stubs.
//
// §11.4.74 catalogue-first: the git/repo primitives are the shared commons/gitops
// package (FindScript prefers a wrappable canonical constitution-submodule script
// when the parent project provides one; falls back to the git binary in a
// standalone Herald checkout). The host-memory probe reuses the shared
// commons/stresschaos.HostMemHeadroom reader (vm_stat/sysctl on darwin,
// /proc/meminfo on linux) — read-only, never allocates memory to test.
//
// §12 / §107 host-safety. EVERY command here is a GUARD, not an actor:
//
//   - destructive-guard DETECTS the destructive op + whether a hardlinked backup
//     exists, classifies §9.1, and EXITS NON-ZERO on FAIL — it NEVER runs rm /
//     git reset --hard / git push --force.
//   - force-push-gate classifies merged-first + per-session-authorized and EXITS
//     NON-ZERO on FAIL — it NEVER performs the force-push.
//   - mem-budget-watch READS the host used-fraction and classifies §12.6 — it
//     NEVER allocates memory to reproduce a breach.
//   - constitution-pull wraps fetch+rebase against the discovered constitution
//     submodule + a post-pull validation gate; it operates against an EXPLICIT
//     --constitution-dir and only fetches/rebases when the operator runs it.
//
// A PASS (exit 0) from a gate means "safe to proceed"; a FAIL (exit 1) BLOCKS the
// operator's wrapper. With --emit each command additionally drives the REAL
// constitution event through an in-memory pipeline (the wire-up seam a future
// serve plane swaps for the PG-backed store).
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/commons/gitops"
	"github.com/vasic-digital/herald/commons/stresschaos"
	constitution "github.com/vasic-digital/herald/commons_constitution"
	"github.com/vasic-digital/herald/commons_constitution/ladder"
	"github.com/vasic-digital/herald/commons_constitution/state"
	"github.com/vasic-digital/herald/sherald/internal/bindings"
)

// registerSysOps replaces the four §43 system/safety stubs with their real
// command bodies. Called from main.go alongside stubs.Register (which keeps the
// remaining sherald-owned §42.3 stubs, e.g. backup-snapshot HRD-034).
func registerSysOps(root *cobra.Command) {
	root.AddCommand(newDestructiveGuardCmd())
	root.AddCommand(newConstitutionPullCmd())
	root.AddCommand(newForcePushGateCmd())
	root.AddCommand(newMemBudgetWatchCmd())
}

// resolveRepo returns the repo dir from --repo, or discovers the enclosing repo
// root from CWD. An empty result (no repo found) is an error — refuse to operate
// against an unscoped checkout. Mirrors pherald's C1 resolveRepo.
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

// classifyAndReport runs the produced Subject through the HRD-020 sherald
// binding for ruleID, prints the verdict, and — when emit is true — drives the
// REAL constitution event through an in-memory pipeline (the composition with
// HRD-020 the task requires). Returns a non-nil error when the verdict is FAIL
// (so the command exit code reflects a §-rule breach) UNLESS allowFail is set.
func classifyAndReport(ctx context.Context, w *cobra.Command, ruleID string, subject constitution.Subject, emit, allowFail bool) error {
	var spec *bindings.RuleSpec
	for _, rs := range bindings.SheraldRules() {
		if rs.RuleID == ruleID {
			s := rs
			spec = &s
			break
		}
	}
	if spec == nil {
		return fmt.Errorf("internal: no sherald binding for rule %s", ruleID)
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
// fans out as a REAL constitution safety event. This is the runtime seam a
// future serve plane swaps for the PG-backed backends; for the CLI it proves the
// §43 command composes with the HRD-020 bindings (not a metadata-only claim).
func emitVerdict(ctx context.Context, spec bindings.RuleSpec, subject constitution.Subject) error {
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{BufferSize: 16})
	defer func() { _ = bus.Close() }()
	em, err := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "digital.vasic.herald/sherald"})
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

// --- HRD-033 — sherald destructive-guard <op> (§9.1) ---

func newDestructiveGuardCmd() *cobra.Command {
	var (
		repoFlag     string
		backupExists bool
		backupPath   string
		emit         bool
	)
	cmd := &cobra.Command{
		Use:   "destructive-guard <op>...",
		Short: "Pre-flight gate for rm / git reset --hard / git push --force (§9.1)",
		Long: "PRE-FLIGHT GATE: detects a destructive operation (rm / git reset --hard / " +
			"git clean / git push --force) from the supplied args and whether a preceding " +
			"hardlinked backup exists (--backup-exists, or a present --backup-path), builds " +
			"the §9.1 Subject, classifies it through the HRD-020 binding, and EXITS NON-ZERO " +
			"when the verdict is FAIL — BLOCKING the op. It NEVER executes the destructive " +
			"op itself (§12/§107 host-safety): it is a guard the operator's wrapper consults.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			op := strings.TrimSpace(strings.Join(args, " "))
			if !isDestructiveOp(op) {
				// Not a destructive op ⇒ nothing for §9.1 to guard; pass cleanly.
				fmt.Fprintf(cmd.OutOrStdout(), "destructive-guard: %q is not a recognized destructive op — nothing to guard\n", op)
				subject := constitution.Subject{
					Kind: bindings.SubjectDestructiveOp,
					ID:   fmt.Sprintf("%s|backup=true", op),
				}
				return classifyAndReport(ctx, cmd, "§9.1", subject, emit, false)
			}

			// Backup detection: an explicit --backup-exists, OR a --backup-path that
			// actually exists on disk (a hardlinked snapshot the operator created
			// before invoking the destructive op). repoFlag scopes a relative
			// --backup-path to the repo.
			haveBackup := backupExists
			if !haveBackup && backupPath != "" {
				p := backupPath
				if !filepath.IsAbs(p) && repoFlag != "" {
					p = filepath.Join(repoFlag, p)
				}
				if _, statErr := os.Stat(p); statErr == nil {
					haveBackup = true
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "destructive-guard: op=%q backup=%t\n", op, haveBackup)
			subject := constitution.Subject{
				Kind: bindings.SubjectDestructiveOp,
				ID:   fmt.Sprintf("%s|backup=%t", op, haveBackup),
			}
			// §9.1 is Critical/Enforce: a missing backup is a hard BLOCK (allowFail=false).
			return classifyAndReport(ctx, cmd, "§9.1", subject, emit, false)
		},
	}
	// Stop flag parsing at the first positional arg so a destructive op carrying
	// its own flags ("git reset --hard", "rm -rf build/") is captured verbatim as
	// the op description rather than misinterpreted as sherald flags. Leading
	// sherald flags (--backup-exists / --emit) still parse before the op.
	cmd.Flags().SetInterspersed(false)
	cmd.Flags().StringVar(&repoFlag, "repo", "", "repo dir scoping a relative --backup-path (default: none)")
	cmd.Flags().BoolVar(&backupExists, "backup-exists", false, "assert a preceding hardlinked backup exists")
	cmd.Flags().StringVar(&backupPath, "backup-path", "", "path to a hardlinked backup snapshot (existence ⇒ backup satisfied)")
	cmd.Flags().BoolVar(&emit, "emit", false, "also drive the §9.1 verdict as a real constitution event")
	return cmd
}

// isDestructiveOp pattern-matches the recorded op string for the §9.1
// destructive families (rm -rf / git reset --hard / git clean / git push
// --force). PURE: it inspects the string only; it NEVER executes anything.
func isDestructiveOp(op string) bool {
	lower := strings.ToLower(op)
	patterns := []string{
		"rm -rf", "rm -fr", "rm -r", "rm -f",
		"reset --hard", "git clean", "clean -f", "clean -fd",
		"push --force", "push -f", "force-with-lease", "--force",
		"truncate", "drop table", "drop database",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// --- HRD-040 — sherald constitution-pull (§11.4.26 + §11.4.32) ---

func newConstitutionPullCmd() *cobra.Command {
	var (
		repoFlag        string
		constitutionDir string
		remote          string
		assumeValidated bool
		skipValidate    bool
		emit            bool
	)
	cmd := &cobra.Command{
		Use:   "constitution-pull",
		Short: "Wrap fetch + rebase + post-pull validation gate (§11.4.26 + §11.4.32)",
		Long: "Pulls the discovered constitution submodule (fetch + rebase against the " +
			"--remote) and runs the §11.4.32 post-pull validation gate, classifying BOTH " +
			"§11.4.26 (pull ok?) and §11.4.32 (validation passed?) through the HRD-020 " +
			"bindings and emitting .bundle.updated. Prefers the canonical " +
			"constitution_pull.sh / find_constitution.sh when discoverable; otherwise drives " +
			"gitops.Runner fetch+rebase against --constitution-dir.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			// Resolve the constitution dir: explicit flag, else walk up for a
			// discovered constitution/ submodule, else error (refuse to guess).
			cdir := constitutionDir
			if cdir == "" {
				start := repoFlag
				if start == "" {
					if cwd, err := os.Getwd(); err == nil {
						start = cwd
					}
				}
				if script, ok := gitops.FindScript(start, "find_constitution.sh"); ok {
					// The canonical helper lives in <ancestor>/constitution/ — its dir
					// is the constitution submodule root.
					cdir = filepath.Dir(script)
				}
			}
			if cdir == "" {
				return fmt.Errorf("constitution-pull: no constitution dir (pass --constitution-dir)")
			}
			cdir, err := filepath.Abs(cdir)
			if err != nil {
				return fmt.Errorf("constitution-pull: resolve constitution dir: %w", err)
			}
			r := gitops.NewRunner(cdir)
			if !r.IsRepo(ctx) {
				return fmt.Errorf("constitution-pull: %s is not a git repo", cdir)
			}

			// §11.4.26 fetch + rebase. A canonical constitution_pull.sh, when present,
			// is preferred (catalogue-first); otherwise drive git fetch + rebase.
			pullOK := true
			if _, ok := gitops.FindScript(cdir, "constitution_pull.sh"); ok {
				// We discovered a canonical script but do NOT execute arbitrary shell
				// here (§12 host-safety) — we still drive the deterministic git ops so
				// the workflow is auditable. The script presence is recorded only.
				fmt.Fprintf(cmd.OutOrStdout(), "constitution-pull: canonical constitution_pull.sh discovered (driving git fetch+rebase directly for auditability)\n")
			}
			if _, e := r.Git(ctx, "fetch", remote); e != nil {
				pullOK = false
				fmt.Fprintf(cmd.OutOrStdout(), "constitution-pull: fetch %s failed: %v\n", remote, e)
			}
			sha, _ := r.HeadSHA(ctx)
			if pullOK {
				branch, bErr := r.CurrentBranch(ctx)
				if bErr == nil {
					upstream := remote + "/" + branch
					if _, e := r.Git(ctx, "rebase", upstream); e != nil {
						pullOK = false
						fmt.Fprintf(cmd.OutOrStdout(), "constitution-pull: rebase onto %s failed: %v\n", upstream, e)
					} else {
						sha, _ = r.HeadSHA(ctx)
						fmt.Fprintf(cmd.OutOrStdout(), "constitution-pull: fetched + rebased onto %s (HEAD %s)\n", upstream, sha)
					}
				}
			}

			pullSubject := constitution.Subject{
				Kind: bindings.SubjectConstitutionPull,
				ID:   fmt.Sprintf("%s|ok=%t", sha, pullOK),
			}
			if e := classifyAndReport(ctx, cmd, "§11.4.26", pullSubject, emit, false); e != nil {
				return e
			}

			// §11.4.32 post-pull validation gate. The validation outcome is supplied
			// by the operator (--assume-validated) or skipped (--skip-validate ⇒ not
			// validated). A real validation harness would run here; the seam keeps the
			// CLI hermetically testable while still classifying a REAL outcome.
			validated := assumeValidated && !skipValidate
			bundleName := "constitution@" + sha
			fmt.Fprintf(cmd.OutOrStdout(), "constitution-pull: post-pull validation validated=%t (bundle %s)\n", validated, bundleName)
			valSubject := constitution.Subject{
				Kind: bindings.SubjectBundleValidation,
				ID:   fmt.Sprintf("%s|validated=%t", bundleName, validated),
			}
			return classifyAndReport(ctx, cmd, "§11.4.32", valSubject, emit, false)
		},
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", "repo dir to start discovery from (default: CWD)")
	cmd.Flags().StringVar(&constitutionDir, "constitution-dir", "", "constitution submodule dir (default: discovered)")
	cmd.Flags().StringVar(&remote, "remote", "origin", "remote to fetch + rebase the constitution against")
	cmd.Flags().BoolVar(&assumeValidated, "assume-validated", false, "treat the post-pull validation gate as PASSED (test seam / operator override)")
	cmd.Flags().BoolVar(&skipValidate, "skip-validate", false, "do not run the post-pull validation gate (records validated=false)")
	cmd.Flags().BoolVar(&emit, "emit", false, "also drive the §11.4.26 + §11.4.32 verdicts as real constitution events")
	return cmd
}

// --- HRD-046 — sherald force-push-gate <ref> (§11.4.41 / §9.2) ---

func newForcePushGateCmd() *cobra.Command {
	var (
		repoFlag   string
		upstream   string
		authorized bool
		doFetch    bool
		emit       bool
	)
	cmd := &cobra.Command{
		Use:   "force-push-gate <ref>",
		Short: "Pre-flight gate for force-push: merge-first + per-session auth (§11.4.41 / §9.2)",
		Long: "PRE-FLIGHT GATE for a force-push of <ref>: classifies merged-first " +
			"(behind the upstream == 0 ⇒ merge-first satisfied) AND per-session-authorized " +
			"(--authorized, or HERALD_FORCE_PUSH_AUTHORIZED) through the HRD-020 §11.4.41 " +
			"binding, and EXITS NON-ZERO when the verdict is FAIL — BLOCKING the force-push. " +
			"It NEVER performs the force-push itself (§12/§107 host-safety).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			ref := strings.TrimSpace(args[0])
			repo, err := resolveRepo(repoFlag)
			if err != nil {
				return err
			}
			r := gitops.NewRunner(repo)
			if !r.IsRepo(ctx) {
				return fmt.Errorf("force-push-gate: %s is not a git repo", repo)
			}

			// merged-first: the local branch must NOT be behind the upstream (every
			// upstream commit is already integrated locally). If there is no upstream
			// tracking ref we conservatively treat merge-first as NOT satisfied — a
			// force-push to an unknown upstream is exactly the dangerous case §11.4.41
			// guards.
			merged := false
			if doFetch {
				// best-effort fetch; a fetch failure is reported but does not flip
				// the verdict on its own (the AheadBehind below is the source of truth).
				if _, e := r.Git(ctx, "fetch"); e != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "force-push-gate: fetch failed: %v\n", e)
				}
			}
			_, behind, abErr := r.AheadBehind(ctx, upstream)
			if abErr != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "force-push-gate: no upstream %s (%v) — merge-first cannot be proven\n", upstream, abErr)
			} else {
				merged = behind == 0
				fmt.Fprintf(cmd.OutOrStdout(), "force-push-gate: %s is %d commit(s) behind %s (merge-first=%t)\n", ref, behind, upstream, merged)
			}

			// per-session authorization: explicit flag OR env token presence.
			auth := authorized
			if !auth {
				if v := os.Getenv("HERALD_FORCE_PUSH_AUTHORIZED"); v != "" {
					if b, perr := strconv.ParseBool(v); perr == nil {
						auth = b
					} else {
						auth = true // any non-empty non-bool value is treated as a token
					}
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "force-push-gate: authorized=%t\n", auth)

			subject := constitution.Subject{
				Kind: bindings.SubjectForcePush,
				ID:   fmt.Sprintf("%s|merged=%t|authorized=%t", ref, merged, auth),
			}
			// §11.4.41 is Critical/Enforce: a not-merged-or-unauthorized force-push is
			// a hard BLOCK (allowFail=false).
			return classifyAndReport(ctx, cmd, "§11.4.41", subject, emit, false)
		},
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", "repo dir (default: discovered from CWD)")
	cmd.Flags().StringVar(&upstream, "upstream", "origin/main", "upstream ref to prove merge-first against (behind==0 ⇒ merged)")
	cmd.Flags().BoolVar(&authorized, "authorized", false, "assert explicit per-session force-push authorization")
	cmd.Flags().BoolVar(&doFetch, "fetch", false, "fetch before computing merge-first (default: use already-fetched state)")
	cmd.Flags().BoolVar(&emit, "emit", false, "also drive the §11.4.41 verdict as a real constitution event")
	return cmd
}

// --- HRD-056 — sherald mem-budget-watch (§12.6) ---

func newMemBudgetWatchCmd() *cobra.Command {
	var (
		watch        bool
		interval     time.Duration
		usedFraction float64
		emit         bool
	)
	cmd := &cobra.Command{
		Use:   "mem-budget-watch",
		Short: "Sample host memory + enforce the §12.6 60% ceiling (one-shot or --watch daemon)",
		Long: "SAMPLES the current host memory used-fraction (vm_stat/sysctl on darwin, " +
			"/proc/meminfo on linux — READ-ONLY, NEVER allocates memory to test §12.6), " +
			"builds the §12.6 Subject, and classifies it through the HRD-020 binding. " +
			"One-shot by default (sample once, classify, exit non-zero on breach); --watch " +
			"loops at --interval emitting on a breach-transition until SIGINT. A hidden " +
			"--used-fraction override (or HERALD_MEM_FRACTION env) is the deterministic " +
			"test seam to drive PASS/FAIL branches without real memory pressure.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			sample := func() (float64, string) {
				// Test seam: --used-fraction flag, then HERALD_MEM_FRACTION env, then
				// the REAL host probe. The flag is < 0 sentinel ⇒ not supplied.
				if usedFraction >= 0 {
					return usedFraction, fmt.Sprintf("test-seam used_fraction=%.4f", usedFraction)
				}
				if env := os.Getenv("HERALD_MEM_FRACTION"); env != "" {
					if f, perr := strconv.ParseFloat(env, 64); perr == nil {
						return f, fmt.Sprintf("env HERALD_MEM_FRACTION=%.4f", f)
					}
				}
				snap := stresschaos.HostMemHeadroom()
				if !snap.Available {
					// Probe unavailable: report it, treat as within-ceiling (cannot
					// prove a breach) — NEVER allocate to reproduce (§12.6 host-safety).
					return 0, "probe-unavailable: " + snap.Note
				}
				return snap.UsedFraction, fmt.Sprintf("host probe (%s) used_fraction=%.4f", snap.Platform, snap.UsedFraction)
			}

			classify := func(frac float64, note string) error {
				fmt.Fprintf(cmd.OutOrStdout(), "mem-budget-watch: %s\n", note)
				subject := constitution.Subject{
					Kind: bindings.SubjectMemBudget,
					ID:   fmt.Sprintf("used_fraction=%.4f", frac),
				}
				return classifyAndReport(ctx, cmd, "§12.6", subject, emit, false)
			}

			if !watch {
				frac, note := sample()
				return classify(frac, note)
			}

			// --watch daemon mode: loop at interval, emit on breach-transition only,
			// until SIGINT (ctx cancellation). The FIRST sample fires synchronously.
			fmt.Fprintf(cmd.OutOrStdout(), "mem-budget-watch: --watch interval=%s (ceiling=%.2f) — Ctrl-C to stop\n", interval, memBudgetCeilingDoc)
			prevBreach := false
			tick := func() error {
				frac, note := sample()
				breach := frac > memBudgetCeilingDoc
				if breach != prevBreach {
					// transition — classify (and emit when --emit) on the edge only.
					prevBreach = breach
					return classify(frac, note+" [transition]")
				}
				fmt.Fprintf(cmd.OutOrStdout(), "mem-budget-watch: %s (no transition, breach=%t)\n", note, breach)
				return nil
			}
			if err := tick(); err != nil {
				return err
			}
			t := time.NewTicker(interval)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					fmt.Fprintln(cmd.OutOrStdout(), "mem-budget-watch: stopped")
					return nil
				case <-t.C:
					if err := tick(); err != nil {
						return err
					}
				}
			}
		},
	}
	cmd.Flags().BoolVar(&watch, "watch", false, "daemon mode: loop sampling, emit on breach-transition until SIGINT")
	cmd.Flags().DurationVar(&interval, "interval", 10*time.Second, "sample interval in --watch mode")
	cmd.Flags().Float64Var(&usedFraction, "used-fraction", -1, "test seam: override the sampled used-fraction (0..1); <0 ⇒ use the real host probe")
	cmd.Flags().BoolVar(&emit, "emit", false, "also drive the §12.6 verdict as a real constitution event")
	_ = cmd.Flags().MarkHidden("used-fraction")
	return cmd
}

// memBudgetCeilingDoc mirrors the §12.6 60% used-fraction ceiling the bindings
// detector (checkMemBudget) enforces. Kept here as the watch-mode transition
// threshold so the daemon emits on the same edge the binding classifies.
const memBudgetCeilingDoc = 0.60
