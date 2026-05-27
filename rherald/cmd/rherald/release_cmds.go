// §43 release-lifecycle command bodies for rherald (v1.0.0 Batch C, cluster C4).
//
// HRD-031 tag-mirror / HRD-032 changelog-generate / HRD-045 gate-retest — the
// LIVE rherald CLI command bodies that PRODUCE the Subjects the HRD-022 rherald
// constitution bindings classify. These replace the cli.StubCmd registrations in
// internal/stubs.
//
// §11.4.74 catalogue-first: the git/repo primitives are the shared commons/gitops
// package (Runner.TagExists / Runner.RemoteHasTag observe tag-mirror parity
// read-only; Runner.LogSubjects reads the commit graph for changelog generation;
// gitops.ParseUpstreams reads the upstreams/*.sh mirror declarations without
// sourcing them). Promoted into commons/gitops so the §43 git-bearing commands of
// every flavor share one implementation (no per-flavor duplication).
//
// §12 / §107 host-safety. The release commands are GATES + a read-only generator:
//
//   - tag-mirror OBSERVES tag parity (local tag + each remote's ls-remote tags),
//     classifies §4, and EXITS NON-ZERO on FAIL — it NEVER creates or pushes a
//     tag. It only reads tag state via ls-remote.
//   - changelog-generate READS the commit graph (git log --pretty=%s), GROUPS by
//     Conventional-Commits type, and WRITES the changelog file under --out-dir —
//     the only write it performs, scoped to the operator-supplied repo. It then
//     classifies §5 conformance (Warn-tier — a non-conforming changelog is a
//     warning, not a hard exit).
//   - gate-retest classifies whether the composite pre-tag full-suite retest
//     PASSED (a hermetic --retest-result / results-file seam supplies the recorded
//     outcome) and EXITS NON-ZERO on FAIL — it NEVER runs the real test suite.
//
// A PASS (exit 0) from a gate means "safe to release"; a FAIL (exit 1) BLOCKS the
// operator's release wrapper. With --emit each command additionally drives the
// REAL constitution event through an in-memory pipeline (the wire-up seam a future
// serve plane swaps for the PG-backed store).
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/commons/gitops"
	constitution "github.com/vasic-digital/herald/commons_constitution"
	"github.com/vasic-digital/herald/commons_constitution/ladder"
	"github.com/vasic-digital/herald/commons_constitution/state"
	"github.com/vasic-digital/herald/rherald/internal/bindings"
)

// registerReleaseOps replaces the three §43 release-lifecycle stubs with their
// real command bodies. Called from main.go in place of stubs.Register for the
// rherald-owned §42.3 release rows.
func registerReleaseOps(root *cobra.Command) {
	root.AddCommand(newTagMirrorCmd())
	root.AddCommand(newChangelogGenerateCmd())
	root.AddCommand(newGateRetestCmd())
}

// resolveRepo returns the repo dir from --repo, or discovers the enclosing repo
// root from CWD. An empty result (no repo found) is an error — refuse to operate
// against an unscoped checkout. Mirrors pherald's C1 / sherald's C2 resolveRepo.
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

// classifyAndReport runs the produced Subject through the HRD-022 rherald
// binding for ruleID, prints the verdict, and — when emit is true — drives the
// REAL constitution event through an in-memory pipeline (the composition with
// HRD-022 the task requires). Returns a non-nil error when the verdict is FAIL
// (so the command exit code reflects a §-rule breach) UNLESS allowFail is set.
func classifyAndReport(ctx context.Context, w *cobra.Command, ruleID string, subject constitution.Subject, emit, allowFail bool) error {
	var spec *bindings.RuleSpec
	for _, rs := range bindings.RheraldRules() {
		if rs.RuleID == ruleID {
			s := rs
			spec = &s
			break
		}
	}
	if spec == nil {
		return fmt.Errorf("internal: no rherald binding for rule %s", ruleID)
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
// fans out as a REAL constitution release event. This is the runtime seam a
// future serve plane swaps for the PG-backed backends; for the CLI it proves the
// §43 command composes with the HRD-022 bindings (not a metadata-only claim).
func emitVerdict(ctx context.Context, spec bindings.RuleSpec, subject constitution.Subject) error {
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{BufferSize: 16})
	defer func() { _ = bus.Close() }()
	em, err := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "digital.vasic.herald/rherald"})
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

// --- HRD-031 — rherald tag-mirror <tag> (§4) ---

func newTagMirrorCmd() *cobra.Command {
	var (
		repoFlag      string
		remotes       []string
		upstreamsFlag string
		emit          bool
	)
	cmd := &cobra.Command{
		Use:   "tag-mirror <tag>",
		Short: "Assert a release tag has full parity across every owned mirror (§4)",
		Long: "OBSERVES whether release tag <tag> is present locally AND on every " +
			"configured mirror remote, builds the §4 tag-mirror-parity Subject from the " +
			"observed tally, classifies it through the HRD-022 §4 binding, and EXITS " +
			"NON-ZERO when the verdict is FAIL — a tag present on the parent but missing " +
			"on any owned mirror is a §4 violation. Mirrors are read from the repeatable " +
			"--remote flag, or (default) from the upstreams/*.sh declarations. It NEVER " +
			"creates or pushes a tag (§12/§107 host-safety): it reads tag state only via " +
			"ls-remote.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			tag := strings.TrimSpace(args[0])
			if tag == "" {
				return fmt.Errorf("tag-mirror: empty tag")
			}
			repo, err := resolveRepo(repoFlag)
			if err != nil {
				return err
			}
			r := gitops.NewRunner(repo)
			if !r.IsRepo(ctx) {
				return fmt.Errorf("tag-mirror: %s is not a git repo", repo)
			}

			// Resolve the set of mirror remotes to check: explicit --remote flags
			// take precedence; else read the upstreams/*.sh declarations and map
			// each to its configured git-remote name.
			remoteNames := remotes
			if len(remoteNames) == 0 {
				upstreamsDir := upstreamsFlag
				if upstreamsDir == "" {
					upstreamsDir = filepath.Join(repo, "upstreams")
				}
				mirrors, perr := gitops.ParseUpstreams(upstreamsDir)
				if perr != nil {
					return fmt.Errorf("tag-mirror: %w", perr)
				}
				for _, m := range mirrors {
					remoteNames = append(remoteNames, m.RemoteNameFor())
				}
			}
			if len(remoteNames) == 0 {
				return fmt.Errorf("tag-mirror: no mirror remotes (pass --remote or declare upstreams/*.sh)")
			}

			// §4 parity observation: is the tag present locally (the parent), and
			// on how many of the owned mirrors?
			localPresent := r.TagExists(ctx, tag)
			tagState := "present"
			if !localPresent {
				tagState = "absent"
			}
			mirrorCount := len(remoteNames)
			withTag := 0
			for _, rem := range remoteNames {
				if r.RemoteHasTag(ctx, rem, tag) {
					withTag++
					fmt.Fprintf(cmd.OutOrStdout(), "tag-mirror: %s present on mirror %s\n", tag, rem)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "tag-mirror: %s MISSING on mirror %s\n", tag, rem)
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "tag-mirror: %s local=%t parity=%d/%d owned mirrors\n",
				tag, localPresent, withTag, mirrorCount)

			subject := constitution.Subject{
				Kind: bindings.SubjectTagMirror,
				ID:   fmt.Sprintf("%s|tag=%s|mirrors=%d|with_tag=%d", tag, tagState, mirrorCount, withTag),
			}
			// §4 is Enforce/High: a parity miss is a hard release-gate BLOCK.
			return classifyAndReport(ctx, cmd, "§4", subject, emit, false)
		},
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", "repo dir (default: discovered from CWD)")
	cmd.Flags().StringSliceVar(&remotes, "remote", nil, "mirror remote to check (repeatable; default: upstreams/*.sh)")
	cmd.Flags().StringVar(&upstreamsFlag, "upstreams-dir", "", "dir holding *.sh mirror declarations (default: <repo>/upstreams)")
	cmd.Flags().BoolVar(&emit, "emit", false, "also drive the §4 verdict as a real constitution event")
	return cmd
}

// --- HRD-032 — rherald changelog-generate <version> (§5) ---

// conventionalTypes is the ordered set of Conventional-Commits types the
// changelog groups under. The order is the §5 / Keep-a-Changelog presentation
// order; commits whose subject does not start with a recognized "<type>:" /
// "<type>(scope):" prefix land in the "Other" bucket.
var conventionalTypes = []struct {
	prefix  string // the conventional type token, e.g. "feat"
	heading string // the section heading in the generated changelog
}{
	{"feat", "Features"},
	{"fix", "Bug Fixes"},
	{"perf", "Performance"},
	{"refactor", "Refactoring"},
	{"docs", "Documentation"},
	{"test", "Tests"},
	{"build", "Build"},
	{"ci", "CI"},
	{"chore", "Chores"},
	{"style", "Style"},
	{"revert", "Reverts"},
}

func newChangelogGenerateCmd() *cobra.Command {
	var (
		repoFlag string
		since    string
		outDir   string
		emit     bool
	)
	cmd := &cobra.Command{
		Use:   "changelog-generate <version>",
		Short: "Generate a Conventional-Commits changelog for a release version (§5)",
		Long: "READS the commit graph (optionally bounded by --since <previous-tag>), " +
			"GROUPS each commit subject by its Conventional-Commits type (feat / fix / " +
			"docs / …), WRITES the grouped changelog to <out-dir>/<version>.md, then " +
			"classifies §5 conformance through the HRD-022 binding. §5 is Warn-tier: a " +
			"non-conforming changelog is a warning, not a hard exit. The only write is the " +
			"changelog file, scoped to the operator-supplied repo (§12/§107 host-safety).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			version := strings.TrimSpace(args[0])
			if version == "" {
				return fmt.Errorf("changelog-generate: empty version")
			}
			repo, err := resolveRepo(repoFlag)
			if err != nil {
				return err
			}
			r := gitops.NewRunner(repo)
			if !r.IsRepo(ctx) {
				return fmt.Errorf("changelog-generate: %s is not a git repo", repo)
			}

			subjects, err := r.LogSubjects(ctx, since)
			if err != nil {
				return fmt.Errorf("changelog-generate: read commit graph: %w", err)
			}

			// Group the subjects by Conventional-Commits type. conforming is true
			// when EVERY commit subject carries a recognized conventional prefix
			// (the §5 conformance signal the binding classifies). An empty range
			// is vacuously conforming (nothing to mis-format).
			grouped, conforming := groupConventional(subjects)

			content := renderChangelog(version, since, grouped, subjects)

			if outDir == "" {
				outDir = filepath.Join(repo, "docs", "changelogs")
			}
			if mkErr := os.MkdirAll(outDir, 0o755); mkErr != nil {
				return fmt.Errorf("changelog-generate: mkdir %s: %w", outDir, mkErr)
			}
			outPath := filepath.Join(outDir, version+".md")
			if wErr := os.WriteFile(outPath, []byte(content), 0o644); wErr != nil {
				return fmt.Errorf("changelog-generate: write %s: %w", outPath, wErr)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "changelog-generate: wrote %s (%d commit(s), conforming=%t)\n",
				outPath, len(subjects), conforming)

			subject := constitution.Subject{
				Kind: bindings.SubjectChangelog,
				ID:   fmt.Sprintf("%s|conforming=%t", version, conforming),
			}
			// §5 is Warn-tier: a non-conforming changelog prints the verdict + emits
			// but does NOT hard-exit (allowFail=true).
			return classifyAndReport(ctx, cmd, "§5", subject, emit, true)
		},
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", "repo dir (default: discovered from CWD)")
	cmd.Flags().StringVar(&since, "since", "", "previous tag/ref bounding the range (default: full history)")
	cmd.Flags().StringVar(&outDir, "out-dir", "", "output dir for <version>.md (default: <repo>/docs/changelogs)")
	cmd.Flags().BoolVar(&emit, "emit", false, "also drive the §5 verdict as a real constitution event")
	return cmd
}

// groupConventional buckets commit subjects by their Conventional-Commits type.
// Returns the type→[]subject map keyed by section heading + whether EVERY
// subject carried a recognized conventional prefix (the §5 conformance signal).
// PURE string parsing — never touches git or the filesystem.
func groupConventional(subjects []string) (grouped map[string][]string, conforming bool) {
	grouped = map[string][]string{}
	conforming = true
	for _, s := range subjects {
		heading, ok := conventionalHeading(s)
		if !ok {
			conforming = false
			grouped["Other"] = append(grouped["Other"], s)
			continue
		}
		grouped[heading] = append(grouped[heading], s)
	}
	return grouped, conforming
}

// conventionalHeading returns the section heading for a subject's
// Conventional-Commits type ("feat: x" / "fix(scope): y" / "feat!: z"), and
// whether the subject matched a recognized type. PURE.
func conventionalHeading(subject string) (string, bool) {
	colon := strings.IndexByte(subject, ':')
	if colon <= 0 {
		return "", false
	}
	token := strings.TrimSpace(subject[:colon])
	// Strip a "(scope)" suffix and a trailing "!" (breaking-change marker).
	if paren := strings.IndexByte(token, '('); paren >= 0 {
		token = token[:paren]
	}
	token = strings.TrimSuffix(token, "!")
	token = strings.ToLower(strings.TrimSpace(token))
	for _, ct := range conventionalTypes {
		if token == ct.prefix {
			return ct.heading, true
		}
	}
	return "", false
}

// renderChangelog produces the Markdown changelog body: a per-version H2 header
// followed by one H3 section per non-empty Conventional-Commits group (in the
// presentation order), then an "Other" section for non-conforming subjects.
func renderChangelog(version, since string, grouped map[string][]string, all []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Changelog — %s\n\n", version)
	if since != "" {
		fmt.Fprintf(&b, "Changes since `%s` (%d commit(s)).\n\n", since, len(all))
	} else {
		fmt.Fprintf(&b, "Full history (%d commit(s)).\n\n", len(all))
	}
	if len(all) == 0 {
		b.WriteString("_No commits in range._\n")
		return b.String()
	}
	for _, ct := range conventionalTypes {
		items := grouped[ct.heading]
		if len(items) == 0 {
			continue
		}
		fmt.Fprintf(&b, "## %s\n\n", ct.heading)
		for _, it := range items {
			fmt.Fprintf(&b, "- %s\n", it)
		}
		b.WriteString("\n")
	}
	if other := grouped["Other"]; len(other) > 0 {
		// Deterministic: the "Other" bucket preserves git-log (newest-first) order.
		b.WriteString("## Other\n\n")
		for _, it := range other {
			fmt.Fprintf(&b, "- %s\n", it)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// --- HRD-045 — rherald gate-retest (§11.4.40) ---

func newGateRetestCmd() *cobra.Command {
	var (
		repoFlag     string
		retestResult string
		resultsFile  string
		tiers        int
		emit         bool
	)
	cmd := &cobra.Command{
		Use:   "gate-retest",
		Short: "Pre-tag full-suite retest GATE: classify the recorded retest outcome (§11.4.40)",
		Long: "PRE-TAG GATE: classifies whether the composite full-suite retest PASSED " +
			"before a release tag (the highest-severity release gate) through the HRD-022 " +
			"§11.4.40 binding, and EXITS NON-ZERO when the verdict is FAIL — a tag attempted " +
			"without a green all-tier retest is BLOCKED. The retest outcome is supplied via " +
			"the --retest-result pass|fail seam (or read from a --results-file whose first " +
			"line is the outcome) so this gate is hermetically testable and NEVER runs the " +
			"real test suite itself (§12/§107 host-safety).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			// Resolve the recorded retest outcome: explicit --retest-result, else the
			// first line of --results-file, else "skipped" (no evidence ⇒ §11.4.40
			// refuses to allow the tag).
			outcome := strings.ToLower(strings.TrimSpace(retestResult))
			if outcome == "" && resultsFile != "" {
				data, rerr := os.ReadFile(resultsFile)
				if rerr != nil {
					return fmt.Errorf("gate-retest: read results file %s: %w", resultsFile, rerr)
				}
				first := data
				if nl := strings.IndexByte(string(data), '\n'); nl >= 0 {
					first = data[:nl]
				}
				outcome = strings.ToLower(strings.TrimSpace(string(first)))
			}
			if outcome == "" {
				outcome = "skipped"
			}

			// Normalise the operator-facing token to the binding's retest vocabulary
			// (green/red/skipped). pass→green, fail→red; green/red/skipped pass through.
			retest := outcome
			switch outcome {
			case "pass", "passed", "ok", "green":
				retest = "green"
			case "fail", "failed", "red":
				retest = "red"
			case "skip", "skipped", "none":
				retest = "skipped"
			}

			fmt.Fprintf(cmd.OutOrStdout(), "gate-retest: recorded retest=%s tiers=%d\n", retest, tiers)

			subject := constitution.Subject{
				Kind: bindings.SubjectRetestGate,
				ID:   fmt.Sprintf("retest-gate|retest=%s|tiers=%d", retest, tiers),
			}
			// §11.4.40 is Critical/Enforce: a non-green / incomplete-tier retest is a
			// hard release-gate BLOCK (allowFail=false).
			return classifyAndReport(ctx, cmd, "§11.4.40", subject, emit, false)
		},
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", "repo dir (default: discovered from CWD)")
	cmd.Flags().StringVar(&retestResult, "retest-result", "", "recorded composite retest outcome: pass|fail|green|red|skipped (test/CI seam)")
	cmd.Flags().StringVar(&resultsFile, "results-file", "", "file whose first line is the retest outcome (alternative to --retest-result)")
	cmd.Flags().IntVar(&tiers, "tiers", 8, "number of §40.2 test tiers the retest covered (must be >= 8 to pass)")
	cmd.Flags().BoolVar(&emit, "emit", false, "also drive the §11.4.40 verdict as a real constitution event")
	return cmd
}
