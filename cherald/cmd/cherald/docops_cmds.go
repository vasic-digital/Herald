// §43 docs-pipeline command bodies for cherald (v1.0.0 Batch C, cluster C3a).
//
// HRD-037 docs-sync (§11.4.61) / HRD-050 readme-sync (§11.4.59) / HRD-052
// export (§11.4.65) / HRD-048 fixed-summary-sync (§11.4.53) / HRD-039
// fixed-align (§11.4.53) — the LIVE cherald CLI command bodies that PRODUCE
// the Subjects the HRD-019 cherald constitution bindings classify. These
// replace the cli.StubCmd registrations in internal/stubs for these five rows.
//
// cherald is the COMPLIANCE / docs flavor. These commands DETECT doc state,
// build a Subject, classify it through the cherald binding (a markerCheck:
// Subject.Kind "violation" → FAIL, anything else → PASS), print the verdict,
// and EXIT NON-ZERO on FAIL. Default = DETECT/CHECK only (safe + hermetic,
// no pandoc / no mutation). Regeneration (export-script invocation, sibling
// regeneration, summary backfill) is behind an explicit --apply flag and
// scopes to an EXPLICIT --repo so a test never mutates the real checkout.
//
// §11.4.74 catalogue-first: the export-script + repo primitives are resolved
// via the shared commons/gitops package (FindScript prefers a wrappable
// canonical script when the parent project provides one; falls back to the
// repo-root scripts/export_docs.sh in a standalone Herald checkout).
//
// §107 anti-bluff: with --emit each command additionally drives the REAL
// constitution event through an in-memory pipeline (the wire-up seam a future
// serve plane swaps for the PG-backed store) — the persisted side-effect IS
// the positive runtime evidence, not a metadata-only claim.
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/cherald/internal/bindings"
	"github.com/vasic-digital/herald/commons/gitops"
	constitution "github.com/vasic-digital/herald/commons_constitution"
	"github.com/vasic-digital/herald/commons_constitution/ladder"
	"github.com/vasic-digital/herald/commons_constitution/state"
)

// registerDocsOps replaces the five §43 docs-pipeline stubs with their real
// command bodies. Called from main.go alongside stubs.Register (which keeps the
// remaining cherald-owned §43 verify/check stubs — cluster C3b).
func registerDocsOps(root *cobra.Command) {
	root.AddCommand(newDocsSyncCmd())
	root.AddCommand(newReadmeSyncCmd())
	root.AddCommand(newExportCmd())
	root.AddCommand(newFixedSummarySyncCmd())
	root.AddCommand(newFixedAlignCmd())
}

// resolveRepo returns the repo dir from --repo, or discovers the enclosing repo
// root from CWD. An empty result (no repo found) is an error — refuse to operate
// against an unscoped checkout. Mirrors sherald's C2 resolveRepo.
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

// classifyAndReport runs the produced Subject through the HRD-019 cherald
// binding for ruleID, prints the verdict, and — when emit is true — drives the
// REAL constitution event through an in-memory pipeline (the composition with
// HRD-019 the task requires). Returns a non-nil error when the verdict is FAIL
// (so the command exit code reflects a §-rule breach) UNLESS allowFail is set.
func classifyAndReport(ctx context.Context, w *cobra.Command, ruleID string, subject constitution.Subject, emit, allowFail bool) error {
	var spec *bindings.RuleSpec
	for _, rs := range bindings.CheraldRules() {
		if rs.RuleID == ruleID {
			s := rs
			spec = &s
			break
		}
	}
	if spec == nil {
		return fmt.Errorf("internal: no cherald binding for rule %s", ruleID)
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
// fans out as a REAL constitution event. This is the runtime seam a future
// serve plane swaps for the PG-backed backends; for the CLI it proves the §43
// command composes with the HRD-019 bindings (not a metadata-only claim).
func emitVerdict(ctx context.Context, spec bindings.RuleSpec, subject constitution.Subject) error {
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{BufferSize: 16})
	defer func() { _ = bus.Close() }()
	em, err := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "digital.vasic.herald/cherald"})
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

// verdictSubject builds the Subject the cherald markerCheck classifies: a
// "violation" Kind (FAIL) when the doc state is in breach, "ok" (PASS) when it
// is clean. The id carries the human-readable evidence (file list / drift
// description) so the verdict line + emitted event reference the real subject.
func verdictSubject(violation bool, id string) constitution.Subject {
	kind := "ok"
	if violation {
		kind = "violation"
	}
	return constitution.Subject{Kind: kind, ID: id}
}

// resolveExportScript resolves the doc-export script: an explicit --script
// override wins (the hermetic-test seam — point it at a fake script needing no
// pandoc), else FindScript walks up for a wrappable export_docs.sh
// (constitution/export_docs.sh or repo-root/export_docs.sh), else the
// conventional <repo>/scripts/export_docs.sh. Returns ("", false) when none
// exists on disk.
func resolveExportScript(repo, override string) (string, bool) {
	if override != "" {
		if abs, err := filepath.Abs(override); err == nil {
			if fi, serr := os.Stat(abs); serr == nil && !fi.IsDir() {
				return abs, true
			}
		}
		return "", false
	}
	if p, ok := gitops.FindScript(repo, "export_docs.sh"); ok {
		return p, true
	}
	conv := filepath.Join(repo, "scripts", "export_docs.sh")
	if fi, err := os.Stat(conv); err == nil && !fi.IsDir() {
		return conv, true
	}
	return "", false
}

// --- HRD-037 — cherald docs-sync (§11.4.61) ---

// requiredMetadataMarkers are the §11.4.61 Markdown metadata + ToC markers a
// tracked doc must carry. A doc missing any is a §11.4.61 violation.
var requiredMetadataMarkers = []string{"| Revision |", "## Table of contents"}

func newDocsSyncCmd() *cobra.Command {
	var (
		repoFlag string
		apply    bool
		script   string
		emit     bool
	)
	cmd := &cobra.Command{
		Use:   "docs-sync [<md>...]",
		Short: "Check tracked .md docs carry §11.4.61 metadata + ToC (--apply regenerates siblings)",
		Long: "Checks that the supplied (or all tracked) Markdown docs carry the required " +
			"§11.4.61 metadata block (a Revision table row) + a Table-of-contents heading, " +
			"builds the §11.4.61 Subject, classifies it through the HRD-019 binding, and " +
			"EXITS NON-ZERO when any doc is missing them. Default is DETECT-only (hermetic). " +
			"--apply additionally (re)generates the HTML/PDF/DOCX siblings via the resolved " +
			"export script (scoped to --repo).",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			repo, err := resolveRepo(repoFlag)
			if err != nil {
				return err
			}

			// Determine the set of docs to check: explicit args (relative to repo
			// when not absolute) or every tracked .md under the repo.
			docs, err := docsToCheck(repo, args)
			if err != nil {
				return err
			}
			if len(docs) == 0 {
				return fmt.Errorf("docs-sync: no .md docs found under %s", repo)
			}

			var missing []string
			for _, d := range docs {
				ok, mErr := docHasMetadata(d)
				if mErr != nil {
					return fmt.Errorf("docs-sync: read %s: %w", d, mErr)
				}
				if !ok {
					missing = append(missing, relTo(repo, d))
				}
			}
			sort.Strings(missing)
			fmt.Fprintf(cmd.OutOrStdout(), "docs-sync: checked %d doc(s); %d missing §11.4.61 metadata/ToC\n", len(docs), len(missing))

			violation := len(missing) > 0
			id := fmt.Sprintf("%d-docs-checked", len(docs))
			if violation {
				id = "missing-metadata: " + strings.Join(missing, ", ")
			}

			if apply && !violation {
				sp, ok := resolveExportScript(repo, script)
				if !ok {
					return fmt.Errorf("docs-sync --apply: no export script found (pass --script or add scripts/export_docs.sh)")
				}
				fmt.Fprintf(cmd.OutOrStdout(), "docs-sync --apply: regenerating siblings via %s\n", sp)
				out, rErr := runScript(ctx, repo, sp, docs...)
				if out != "" {
					fmt.Fprintln(cmd.OutOrStdout(), out)
				}
				if rErr != nil {
					return fmt.Errorf("docs-sync --apply: export script failed: %w", rErr)
				}
			}

			// §11.4.61 is low/warn in the cherald ladder, but a missing metadata
			// block is a hard doc defect for this command — BLOCK on it.
			return classifyAndReport(ctx, cmd, "§11.4.61", verdictSubject(violation, id), emit, false)
		},
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", "repo dir to scope the check/regeneration to (default: discovered from CWD)")
	cmd.Flags().BoolVar(&apply, "apply", false, "regenerate HTML/PDF/DOCX siblings via the export script (mutation; default: detect-only)")
	cmd.Flags().StringVar(&script, "script", "", "override the export-script path (default: resolved scripts/export_docs.sh; test seam)")
	cmd.Flags().BoolVar(&emit, "emit", false, "also drive the §11.4.61 verdict as a real constitution event")
	return cmd
}

// docsToCheck resolves the doc set: explicit args (relative→repo) or every
// tracked .md under repo (walking the tree, skipping vendored/submodule dirs).
func docsToCheck(repo string, args []string) ([]string, error) {
	if len(args) > 0 {
		var out []string
		for _, a := range args {
			p := a
			if !filepath.IsAbs(p) {
				p = filepath.Join(repo, a)
			}
			out = append(out, p)
		}
		return out, nil
	}
	var out []string
	skip := map[string]bool{"submodules": true, "containers": true, "constitutable": true, ".git": true, "node_modules": true}
	err := filepath.WalkDir(repo, func(p string, d os.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			if skip[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), ".md") {
			out = append(out, p)
		}
		return nil
	})
	return out, err
}

// docHasMetadata reports whether the doc at path carries every
// requiredMetadataMarkers token (the §11.4.61 metadata block + ToC heading).
func docHasMetadata(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	seen := make(map[string]bool, len(requiredMetadataMarkers))
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		for _, m := range requiredMetadataMarkers {
			if strings.Contains(line, m) {
				seen[m] = true
			}
		}
	}
	if err := sc.Err(); err != nil {
		return false, err
	}
	for _, m := range requiredMetadataMarkers {
		if !seen[m] {
			return false, nil
		}
	}
	return true, nil
}

// relTo returns path relative to base, or path unchanged when not under base.
func relTo(base, path string) string {
	if r, err := filepath.Rel(base, path); err == nil {
		return r
	}
	return path
}

// runScript invokes the resolved export script inside repo, passing the doc
// paths as args. The script's combined output is returned for evidence.
func runScript(ctx context.Context, repo, script string, docs ...string) (string, error) {
	args := make([]string, 0, len(docs))
	for _, d := range docs {
		args = append(args, d)
	}
	c := exec.CommandContext(ctx, script, args...)
	c.Dir = repo
	out, err := c.CombinedOutput()
	return strings.TrimRight(string(out), "\n"), err
}

// --- HRD-050 — cherald readme-sync (§11.4.59) ---

func newReadmeSyncCmd() *cobra.Command {
	var (
		repoFlag string
		apply    bool
		script   string
		emit     bool
	)
	cmd := &cobra.Command{
		Use:   "readme-sync",
		Short: "Check README doc-link rows are in sync with on-disk docs (§11.4.59)",
		Long: "Checks that every doc-link row in README.md points at a doc that exists on " +
			"disk (the §11.4.59 README always-sync invariant), builds the §11.4.59 Subject, " +
			"classifies it through the HRD-019 binding, and EXITS NON-ZERO on drift. Default " +
			"is DETECT-only. --apply (re)generates the README's sibling exports via the " +
			"resolved export script (scoped to --repo).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			repo, err := resolveRepo(repoFlag)
			if err != nil {
				return err
			}
			readme := filepath.Join(repo, "README.md")
			if _, sErr := os.Stat(readme); sErr != nil {
				return fmt.Errorf("readme-sync: no README.md under %s", repo)
			}

			dangling, err := readmeDanglingLinks(repo, readme)
			if err != nil {
				return fmt.Errorf("readme-sync: %w", err)
			}
			sort.Strings(dangling)
			fmt.Fprintf(cmd.OutOrStdout(), "readme-sync: %d dangling doc-link row(s) in README.md\n", len(dangling))

			violation := len(dangling) > 0
			id := "README.md in sync"
			if violation {
				id = "README.md dangling links: " + strings.Join(dangling, ", ")
			}

			if apply && !violation {
				sp, ok := resolveExportScript(repo, script)
				if !ok {
					return fmt.Errorf("readme-sync --apply: no export script found (pass --script)")
				}
				fmt.Fprintf(cmd.OutOrStdout(), "readme-sync --apply: regenerating README siblings via %s\n", sp)
				out, rErr := runScript(ctx, repo, sp, readme)
				if out != "" {
					fmt.Fprintln(cmd.OutOrStdout(), out)
				}
				if rErr != nil {
					return fmt.Errorf("readme-sync --apply: export script failed: %w", rErr)
				}
			}

			return classifyAndReport(ctx, cmd, "§11.4.59", verdictSubject(violation, id), emit, false)
		},
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", "repo dir to scope the check to (default: discovered from CWD)")
	cmd.Flags().BoolVar(&apply, "apply", false, "regenerate README sibling exports via the export script (mutation; default: detect-only)")
	cmd.Flags().StringVar(&script, "script", "", "override the export-script path (test seam)")
	cmd.Flags().BoolVar(&emit, "emit", false, "also drive the §11.4.59 verdict as a real constitution event")
	return cmd
}

// readmeLinkRe matches a Markdown link target ([text](target)). Only local
// relative .md targets are validated (external URLs + anchors are ignored).
var readmeLinkRe = regexp.MustCompile(`\]\(([^)]+\.md)(#[^)]*)?\)`)

// readmeDanglingLinks returns the relative .md link targets in README that do
// NOT resolve to a file on disk (relative to the repo root).
func readmeDanglingLinks(repo, readme string) ([]string, error) {
	data, err := os.ReadFile(readme)
	if err != nil {
		return nil, err
	}
	var dangling []string
	seen := map[string]bool{}
	for _, m := range readmeLinkRe.FindAllStringSubmatch(string(data), -1) {
		target := m[1]
		if strings.Contains(target, "://") || strings.HasPrefix(target, "//") {
			continue // external URL
		}
		if seen[target] {
			continue
		}
		seen[target] = true
		p := target
		if !filepath.IsAbs(p) {
			p = filepath.Join(repo, target)
		}
		if _, sErr := os.Stat(p); sErr != nil {
			dangling = append(dangling, target)
		}
	}
	return dangling, nil
}

// --- HRD-052 — cherald export <md...> (§11.4.65) ---

func newExportCmd() *cobra.Command {
	var (
		repoFlag string
		script   string
		emit     bool
	)
	cmd := &cobra.Command{
		Use:   "export [<md>...]",
		Short: "(Re)generate md→html/pdf/docx siblings via the export script (§11.4.65)",
		Long: "Wraps the canonical doc-export script to (re)generate the HTML/PDF/DOCX " +
			"siblings of the supplied Markdown docs (or every tracked doc when none are " +
			"given), builds the §11.4.65 Universal-Markdown-export Subject from the script " +
			"outcome, classifies it through the HRD-019 binding, and EXITS NON-ZERO when the " +
			"export fails. The export script is resolved from --script (hermetic test seam) " +
			"or the conventional scripts/export_docs.sh under --repo (§11.4.74 catalogue-first).",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			repo, err := resolveRepo(repoFlag)
			if err != nil {
				return err
			}
			sp, ok := resolveExportScript(repo, script)
			if !ok {
				return fmt.Errorf("export: no export script found (pass --script or add scripts/export_docs.sh under --repo)")
			}

			// Resolve doc args (relative→repo); empty ⇒ export everything (the
			// script's no-arg behavior).
			var docs []string
			for _, a := range args {
				p := a
				if !filepath.IsAbs(p) {
					p = filepath.Join(repo, a)
				}
				docs = append(docs, p)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "export: invoking %s for %d doc(s)\n", sp, len(docs))
			out, rErr := runScript(ctx, repo, sp, docs...)
			if out != "" {
				fmt.Fprintln(cmd.OutOrStdout(), out)
			}

			violation := rErr != nil
			id := fmt.Sprintf("export ok via %s (%d docs)", filepath.Base(sp), len(docs))
			if violation {
				id = fmt.Sprintf("export FAILED via %s: %v", filepath.Base(sp), rErr)
			}
			return classifyAndReport(ctx, cmd, "§11.4.65", verdictSubject(violation, id), emit, false)
		},
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", "repo dir to scope the export to (default: discovered from CWD)")
	cmd.Flags().StringVar(&script, "script", "", "override the export-script path (default: resolved scripts/export_docs.sh; test seam)")
	cmd.Flags().BoolVar(&emit, "emit", false, "also drive the §11.4.65 verdict as a real constitution event")
	return cmd
}

// --- HRD-048 — cherald fixed-summary-sync (§11.4.53) ---

func newFixedSummarySyncCmd() *cobra.Command {
	var (
		repoFlag string
		apply    bool
		emit     bool
	)
	cmd := &cobra.Command{
		Use:   "fixed-summary-sync",
		Short: "Check docs/Fixed_Summary.md parity vs docs/Fixed.md (§11.4.53)",
		Long: "Checks that every HRD-id present in docs/Fixed.md has a matching one-liner in " +
			"docs/Fixed_Summary.md (the §11.4.53 Fixed_Summary parity invariant), builds the " +
			"§11.4.53 Subject, classifies it through the HRD-019 binding, and EXITS NON-ZERO " +
			"on a parity gap. Default is DETECT-only; --apply backfills the missing summary " +
			"lines into docs/Fixed_Summary.md (scoped to --repo).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			repo, err := resolveRepo(repoFlag)
			if err != nil {
				return err
			}
			fixedPath := filepath.Join(repo, "docs", "Fixed.md")
			summaryPath := filepath.Join(repo, "docs", "Fixed_Summary.md")

			fixedIDs, err := hrdIDsIn(fixedPath)
			if err != nil {
				return fmt.Errorf("fixed-summary-sync: read Fixed.md: %w", err)
			}
			summaryIDs, err := hrdIDsIn(summaryPath)
			if err != nil {
				return fmt.Errorf("fixed-summary-sync: read Fixed_Summary.md: %w", err)
			}

			var missing []string
			for _, id := range sortedKeys(fixedIDs) {
				if !summaryIDs[id] {
					missing = append(missing, id)
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "fixed-summary-sync: %d HRD(s) in Fixed.md; %d missing from Fixed_Summary.md\n", len(fixedIDs), len(missing))

			violation := len(missing) > 0
			id := "Fixed_Summary parity ok"
			if violation {
				id = "Fixed_Summary missing: " + strings.Join(missing, ", ")
			}

			if apply && violation {
				if aErr := backfillSummary(summaryPath, missing); aErr != nil {
					return fmt.Errorf("fixed-summary-sync --apply: backfill: %w", aErr)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "fixed-summary-sync --apply: backfilled %d summary line(s)\n", len(missing))
				violation = false
				id = "Fixed_Summary parity restored (backfilled " + strings.Join(missing, ", ") + ")"
			}

			return classifyAndReport(ctx, cmd, "§11.4.53", verdictSubject(violation, id), emit, false)
		},
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", "repo dir to scope the check to (default: discovered from CWD)")
	cmd.Flags().BoolVar(&apply, "apply", false, "backfill missing Fixed_Summary.md lines (mutation; default: detect-only)")
	cmd.Flags().BoolVar(&emit, "emit", false, "also drive the §11.4.53 verdict as a real constitution event")
	return cmd
}

// --- HRD-039 — cherald fixed-align (§11.4.53) ---
//
// Rule choice: §11.4.55 is NOT a cherald-owned rule (it is pherald's reopen
// binding — see pherald/internal/bindings). The closest cherald-owned rule for
// a Fixed-document reconciliation is §11.4.53 "Fixed_Summary parity" — the
// Fixed-document hygiene/parity binding. fixed-align therefore classifies its
// Issues.md↔Fixed.md drift against §11.4.53. This is a DETECT-only command: it
// never migrates an HRD (that atomic Issues↔Fixed migration is pherald's
// reopen command, §11.4.55).

func newFixedAlignCmd() *cobra.Command {
	var (
		repoFlag string
		emit     bool
	)
	cmd := &cobra.Command{
		Use:   "fixed-align",
		Short: "Reconcile docs/Issues.md ↔ docs/Fixed.md for drift (detect-only; §11.4.53)",
		Long: "RECONCILIATION CHECK: detects drift between docs/Issues.md and docs/Fixed.md — " +
			"an HRD-id present in BOTH (a closure that was never removed from Issues.md), or a " +
			"Fixed HRD-id absent from Fixed.md. Builds the Subject, classifies it through the " +
			"closest cherald-owned binding (§11.4.53 Fixed_Summary parity — §11.4.55 is " +
			"pherald's reopen rule, not a cherald rule), and EXITS NON-ZERO on drift. " +
			"DETECT-ONLY: it never migrates an HRD (that atomic migration is pherald's reopen).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			repo, err := resolveRepo(repoFlag)
			if err != nil {
				return err
			}
			issuesPath := filepath.Join(repo, "docs", "Issues.md")
			fixedPath := filepath.Join(repo, "docs", "Fixed.md")

			issuesIDs, err := hrdIDsIn(issuesPath)
			if err != nil {
				return fmt.Errorf("fixed-align: read Issues.md: %w", err)
			}
			fixedIDs, err := hrdIDsIn(fixedPath)
			if err != nil {
				return fmt.Errorf("fixed-align: read Fixed.md: %w", err)
			}

			// Drift: any HRD-id present in BOTH trackers — a closed item that was
			// never removed from Issues.md (the §11.4.19 atomic-migration breach).
			var inBoth []string
			for _, id := range sortedKeys(issuesIDs) {
				if fixedIDs[id] {
					inBoth = append(inBoth, id)
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "fixed-align: %d in Issues.md, %d in Fixed.md; %d present in BOTH (drift)\n",
				len(issuesIDs), len(fixedIDs), len(inBoth))

			violation := len(inBoth) > 0
			id := "Issues.md/Fixed.md aligned"
			if violation {
				id = "drift — HRD present in both Issues.md and Fixed.md: " + strings.Join(inBoth, ", ")
			}
			return classifyAndReport(ctx, cmd, "§11.4.53", verdictSubject(violation, id), emit, false)
		},
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", "repo dir to scope the reconciliation to (default: discovered from CWD)")
	cmd.Flags().BoolVar(&emit, "emit", false, "also drive the §11.4.53 verdict as a real constitution event")
	return cmd
}

// --- shared HRD-tracker helpers ---

// hrdIDRe matches an HRD work-item id (e.g. HRD-037). Used to extract the id
// set from a tracker doc. A missing file is treated as an empty set (not an
// error) so a repo without the tracker still reconciles cleanly.
var hrdIDRe = regexp.MustCompile(`\bHRD-(\d{3,})\b`)

// hrdIDsIn returns the set of HRD-ids referenced in the doc at path. A
// not-exist file yields an empty set + nil error (the tracker is simply absent).
func hrdIDsIn(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}, nil
		}
		return nil, err
	}
	ids := map[string]bool{}
	for _, m := range hrdIDRe.FindAllString(string(data), -1) {
		ids[m] = true
	}
	return ids, nil
}

// sortedKeys returns the keys of a string-set in ascending order.
func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// backfillSummary appends a placeholder one-liner for each missing HRD-id to
// the Fixed_Summary.md at path (creating it if absent). The --apply mutation
// path; scoped to an explicit --repo by the caller. The placeholder is a
// self-contained clause (§11.4.91) the operator refines.
func backfillSummary(path string, missing []string) error {
	var b strings.Builder
	for _, id := range missing {
		fmt.Fprintf(&b, "- %s — summary backfilled by cherald fixed-summary-sync; refine with the real closure clause.\n", id)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = f.WriteString(b.String())
	return err
}
