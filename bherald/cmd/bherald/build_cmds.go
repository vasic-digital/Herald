// §43 build/test command bodies for bherald (v1.0.0 Batch C, cluster C5).
//
// HRD-041 test-tier-verify (§11.4.27 / §40.2) + HRD-035 evidence-capture
// (§11.4.2 / §11.4.5) — the LIVE bherald CLI command bodies that PRODUCE the
// Subjects the HRD-021 bherald constitution bindings classify. These replace the
// cli.StubCmd registrations in internal/stubs (the gate-retest HRD-045 alias stub
// stays — rherald owns the real impl; bherald keeps only the alias).
//
// bherald is the BUILD / CI flavor. Unlike pherald (C1) / sherald (C2) these two
// commands are NOT git-centric: they CLASSIFY an already-observed CI/test outcome
// (a tier-coverage description / an evidence bundle). The detectors are PURE
// (commons_constitution §12 host-safety) — they NEVER run the build, re-execute a
// suite, or spawn a process. The command bodies OBSERVE the tier/evidence state
// (from flags, a results file, or a directory probe), build the Subject the
// HRD-021 binding parses, classify it, and EXIT NON-ZERO on a FAIL so a CI
// wrapper sees the breach.
//
// A PASS (exit 0) means "this gate's prerequisite is satisfied"; a FAIL (exit 1)
// BLOCKS the wrapper. With --emit each command additionally drives the REAL
// constitution event through an in-memory pipeline (the wire-up seam a future
// serve plane swaps for the PG-backed store).
//
// NOTE: bherald's main.go does NOT build a JWT verifier (CLI-only, no §107 eager-
// verifier path) — these commands add no auth surface.
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/bherald/internal/bindings"
	constitution "github.com/vasic-digital/herald/commons_constitution"
	"github.com/vasic-digital/herald/commons_constitution/ladder"
	"github.com/vasic-digital/herald/commons_constitution/state"
)

// registerBuildOps replaces the two §43 build/test stubs (HRD-041 test-tier-
// verify + HRD-035 evidence-capture) with their real command bodies. Called from
// main.go alongside stubs.Register (which keeps the gate-retest HRD-045 alias).
func registerBuildOps(root *cobra.Command) {
	root.AddCommand(newTestTierVerifyCmd())
	root.AddCommand(newEvidenceCaptureCmd())
}

// classifyAndReport runs the produced Subject through the HRD-021 bherald binding
// for ruleID, prints the verdict, and — when emit is true — drives the REAL
// constitution event through an in-memory pipeline (the composition with HRD-021
// the task requires). Returns a non-nil error when the verdict is FAIL (so the
// command exit code reflects a §-rule breach) UNLESS allowFail is set. Mirrors
// C1/C2 classifyAndReport exactly, but over the bherald rule catalogue.
func classifyAndReport(ctx context.Context, w *cobra.Command, ruleID string, subject constitution.Subject, emit, allowFail bool) error {
	var spec *bindings.RuleSpec
	for _, rs := range bindings.BheraldRules() {
		if rs.RuleID == ruleID {
			s := rs
			spec = &s
			break
		}
	}
	if spec == nil {
		return fmt.Errorf("internal: no bherald binding for rule %s", ruleID)
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
// ModeLadder + MemoryAudit) and drives the subject through it so the verdict fans
// out as a REAL constitution gate event. This is the runtime seam a future serve
// plane swaps for the PG-backed backends; for the CLI it proves the §43 command
// composes with the HRD-021 bindings (not a metadata-only claim).
func emitVerdict(ctx context.Context, spec bindings.RuleSpec, subject constitution.Subject) error {
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{BufferSize: 16})
	defer func() { _ = bus.Close() }()
	em, err := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "digital.vasic.herald/bherald"})
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

// --- HRD-041 — bherald test-tier-verify (§11.4.27 / §40.2) ---

func newTestTierVerifyCmd() *cobra.Command {
	var (
		pkg       string
		tiers     []string
		tiersFile string
		allTiers  bool
		emit      bool
	)
	cmd := &cobra.Command{
		Use:   "test-tier-verify",
		Short: "Verify the §40.2 8-tier test matrix for a package (§11.4.27)",
		Long: "Verifies a package carries every tier of the §40.2 canonical 8-tier test " +
			"matrix (unit/component/integration/contract/e2e_sandbox/e2e_live/mutation/chaos) " +
			"per §11.4.27 (no-fakes-beyond-unit + 100% test-type coverage). OBSERVES the " +
			"present tiers from --tier (repeatable), --tiers-file (one tier per line, '#' " +
			"comments allowed), or --all-tiers (assert the full matrix), builds the §11.4.27 " +
			"Subject, classifies it through the HRD-021 binding, and EXITS NON-ZERO when ANY " +
			"canonical tier is missing — BLOCKING the promotion. PURE: it classifies the " +
			"recorded tier coverage; it NEVER runs the suites.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			if pkg == "" {
				pkg = "pkg"
			}

			present, err := collectTiers(pkg, tiers, tiersFile, allTiers)
			if err != nil {
				return fmt.Errorf("test-tier-verify: %w", err)
			}
			ordered := sortedTiers(present)
			fmt.Fprintf(cmd.OutOrStdout(), "test-tier-verify: pkg=%q observed tiers=[%s]\n", pkg, strings.Join(ordered, ","))

			subject := constitution.Subject{
				Kind: bindings.SubjectTestTier,
				ID:   fmt.Sprintf("%s|tiers=%s", pkg, strings.Join(ordered, ",")),
			}
			// §11.4.27 is High/Enforce: a missing tier is a hard BLOCK (allowFail=false).
			return classifyAndReport(ctx, cmd, "§11.4.27", subject, emit, false)
		},
	}
	cmd.Flags().StringVar(&pkg, "pkg", "", "package label recorded in the §11.4.27 Subject (default: pkg)")
	cmd.Flags().StringSliceVar(&tiers, "tier", nil, "a present test tier (repeatable / comma-separated): unit,component,integration,contract,e2e_sandbox,e2e_live,mutation,chaos")
	cmd.Flags().StringVar(&tiersFile, "tiers-file", "", "file listing present tiers (one per line; '#' starts a comment)")
	cmd.Flags().BoolVar(&allTiers, "all-tiers", false, "assert the full §40.2 8-tier matrix is present (test seam / operator override)")
	cmd.Flags().BoolVar(&emit, "emit", false, "also drive the §11.4.27 verdict as a real constitution event")
	return cmd
}

// collectTiers gathers the present tier set from the supplied sources. --all-tiers
// short-circuits to the full canonical matrix. Otherwise --tier flags + a
// --tiers-file are unioned. PURE except the optional file read (a recorded
// results manifest, never a test runner).
func collectTiers(pkg string, tiers []string, tiersFile string, allTiers bool) (map[string]bool, error) {
	present := map[string]bool{}
	if allTiers {
		for _, t := range bindings.CanonicalTiers() {
			present[t] = true
		}
		return present, nil
	}
	for _, t := range tiers {
		t = strings.TrimSpace(strings.ToLower(t))
		if t != "" {
			present[t] = true
		}
	}
	if tiersFile != "" {
		f, err := os.Open(tiersFile)
		if err != nil {
			return nil, fmt.Errorf("open --tiers-file %q: %w", tiersFile, err)
		}
		defer func() { _ = f.Close() }()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if i := strings.IndexByte(line, '#'); i >= 0 {
				line = strings.TrimSpace(line[:i])
			}
			if line == "" {
				continue
			}
			for _, t := range strings.Split(line, ",") {
				t = strings.TrimSpace(strings.ToLower(t))
				if t != "" {
					present[t] = true
				}
			}
		}
		if err := sc.Err(); err != nil {
			return nil, fmt.Errorf("read --tiers-file %q: %w", tiersFile, err)
		}
	}
	return present, nil
}

// sortedTiers returns the present tiers as a deterministic, canonically-ordered
// slice (canonical tiers first in §40.2 order, then any extras alphabetically) so
// the produced Subject.ID is stable across runs.
func sortedTiers(present map[string]bool) []string {
	var ordered []string
	seen := map[string]bool{}
	for _, t := range bindings.CanonicalTiers() {
		if present[t] {
			ordered = append(ordered, t)
			seen[t] = true
		}
	}
	var extra []string
	for t := range present {
		if !seen[t] {
			extra = append(extra, t)
		}
	}
	sort.Strings(extra)
	return append(ordered, extra...)
}

// --- HRD-035 — bherald evidence-capture (§11.4.2 / §11.4.5) ---

func newEvidenceCaptureCmd() *cobra.Command {
	var (
		testID       string
		outcome      string
		evidencePath string
		hasEvidence  bool
		emit         bool
	)
	cmd := &cobra.Command{
		Use:   "evidence-capture",
		Short: "Validate a CI gate PASS carries a recorded evidence artefact (§11.4.2)",
		Long: "Validates the §11.4.2 recorded-evidence requirement (the §107 anti-bluff " +
			"covenant at the CI layer): a gate/test reporting outcome=pass MUST carry a " +
			"captured-evidence artefact. OBSERVES the evidence state from --evidence-path " +
			"(a present, non-empty file/dir ⇒ evidence captured) or --has-evidence, builds " +
			"the §11.4.2 Subject, classifies it through the HRD-021 binding, and EXITS " +
			"NON-ZERO on a metadata-only / no-evidence PASS-bluff. An honest FAIL passes the " +
			"anti-bluff check (the gate correctly reported failure). PURE: it reads the " +
			"recorded outcome + the presence of the artefact; it NEVER inspects its bytes.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			if testID == "" {
				testID = "test"
			}
			if outcome == "" {
				outcome = "pass"
			}

			// Evidence detection: an explicit --has-evidence, OR a --evidence-path that
			// actually exists on disk and is non-empty (a captured artefact: a JSONL
			// transcript, a coverage report, a docs/qa/<run-id>/ dir, ...).
			haveEvidence := hasEvidence
			detail := "via --has-evidence"
			if !haveEvidence && evidencePath != "" {
				ok, note := evidenceArtefactPresent(evidencePath)
				haveEvidence = ok
				detail = note
			} else if !haveEvidence && evidencePath == "" {
				detail = "no --evidence-path and --has-evidence not set"
			}

			fmt.Fprintf(cmd.OutOrStdout(), "evidence-capture: test=%q outcome=%s evidence=%t (%s)\n", testID, outcome, haveEvidence, detail)
			subject := constitution.Subject{
				Kind: bindings.SubjectEvidence,
				ID:   fmt.Sprintf("%s|outcome=%s|evidence=%t", testID, outcome, haveEvidence),
			}
			// §11.4.2 is High/Enforce: a PASS without captured evidence is a hard BLOCK
			// (allowFail=false) — that is the §107 PASS-bluff this gate exists to catch.
			return classifyAndReport(ctx, cmd, "§11.4.2", subject, emit, false)
		},
	}
	cmd.Flags().StringVar(&testID, "test-id", "", "test/gate identifier recorded in the §11.4.2 Subject (default: test)")
	cmd.Flags().StringVar(&outcome, "outcome", "pass", "the recorded gate outcome (pass|fail|error)")
	cmd.Flags().StringVar(&evidencePath, "evidence-path", "", "path to the captured-evidence artefact (present + non-empty ⇒ evidence satisfied)")
	cmd.Flags().BoolVar(&hasEvidence, "has-evidence", false, "assert a captured-evidence artefact exists (test seam / operator override)")
	cmd.Flags().BoolVar(&emit, "emit", false, "also drive the §11.4.2 verdict as a real constitution event")
	return cmd
}

// evidenceArtefactPresent reports whether evidencePath names a real, non-empty
// captured-evidence artefact: a regular file with size > 0, or a directory that
// contains at least one non-empty regular file (a docs/qa/<run-id>/ bundle). An
// empty file / empty dir is NOT evidence (a touch-only stub is itself a bluff).
// PURE-read: it stats the path; it never reads the artefact's content bytes.
func evidenceArtefactPresent(path string) (bool, string) {
	info, err := os.Stat(path)
	if err != nil {
		return false, fmt.Sprintf("--evidence-path %q does not exist", path)
	}
	if !info.IsDir() {
		if info.Size() == 0 {
			return false, fmt.Sprintf("--evidence-path %q is empty (touch-only stub is not evidence)", path)
		}
		return true, fmt.Sprintf("--evidence-path %q is a %d-byte artefact", path, info.Size())
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, fmt.Sprintf("--evidence-path %q unreadable: %v", path, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fi, ferr := e.Info()
		if ferr == nil && fi.Size() > 0 {
			return true, fmt.Sprintf("--evidence-path %q holds artefact %s (%d bytes)", path, e.Name(), fi.Size())
		}
	}
	return false, fmt.Sprintf("--evidence-path %q has no non-empty artefact", path)
}
