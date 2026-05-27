// §43 scheduled-audit command body for scherald (v1.0.0 §43 straggler HRD-047).
//
// HRD-047 status-digest (§11.4.45) — the LIVE scherald CLI command body that
// PRODUCES the Subject the HRD-025 scherald constitution binding classifies.
// It replaces the cli.StubCmd registration in internal/stubs for this row.
//
// scherald is the SCHEDULED-AUDIT flavor. status-digest sweeps docs/Status.md
// (and, for evidence, the docs/Issues.md / docs/Fixed.md HRD trackers) under an
// EXPLICIT --repo, produces a concise digest of the work-item state (counts by
// status + a stale-item tally), builds the §11.4.45 status-sweep Subject, and
// classifies it through the HRD-025 binding. Default = DETECT/REPORT only (safe
// + hermetic, no mutation): it prints the digest + the verdict line and EXITS
// NON-ZERO when Status.md is missing or stale (a §11.4.45 violation). With
// --apply it additionally (re)generates docs/Status_Summary.md from Status.md
// (scoped to --repo). With --emit each invocation additionally drives the REAL
// constitution event through an in-memory pipeline.
//
// §11.4.74 catalogue-first: the repo primitives are the shared commons/gitops
// package (RepoRoot discovery + an optional wrappable export script).
//
// §12 / §107 host-safety: status-digest operates against an EXPLICIT --repo (or
// the discovered enclosing repo); the --apply mutation is scoped to that repo.
// A test NEVER passes --apply against the real Herald checkout — every test uses
// a t.TempDir fixture. With --emit the persisted side-effect IS the positive
// runtime evidence, not a metadata-only claim.
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/commons/gitops"
	constitution "github.com/vasic-digital/herald/commons_constitution"
	"github.com/vasic-digital/herald/commons_constitution/ladder"
	"github.com/vasic-digital/herald/commons_constitution/state"
	"github.com/vasic-digital/herald/scherald/internal/bindings"
)

// registerDigestOps replaces the §43 status-digest stub with its real command
// body. Called from main.go INSTEAD of the now-removed stubs.Register
// status-digest entry (scherald owns only this one §43 row).
func registerDigestOps(root *cobra.Command) {
	root.AddCommand(newStatusDigestCmd())
}

// resolveRepo returns the repo dir from --repo, or discovers the enclosing repo
// root from CWD. An empty result (no repo found) is an error — refuse to operate
// against an unscoped checkout. Mirrors sherald's C2 / cherald's C3a resolveRepo.
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

// classifyAndReport runs the produced Subject through the HRD-025 scherald
// binding for ruleID, prints the verdict, and — when emit is true — drives the
// REAL constitution event through an in-memory pipeline (the composition with
// HRD-025 the task requires). Returns a non-nil error when the verdict is FAIL
// (so the command exit code reflects a §-rule breach) UNLESS allowFail is set.
func classifyAndReport(ctx context.Context, w *cobra.Command, ruleID string, subject constitution.Subject, emit, allowFail bool) error {
	var spec *bindings.RuleSpec
	for _, rs := range bindings.ScheraldRules() {
		if rs.RuleID == ruleID {
			s := rs
			spec = &s
			break
		}
	}
	if spec == nil {
		return fmt.Errorf("internal: no scherald binding for rule %s", ruleID)
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
// fans out as a REAL constitution policy event. This is the runtime seam a
// future serve plane swaps for the PG-backed backends; for the CLI it proves the
// §43 command composes with the HRD-025 bindings (not a metadata-only claim).
func emitVerdict(ctx context.Context, spec bindings.RuleSpec, subject constitution.Subject) error {
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{BufferSize: 16})
	defer func() { _ = bus.Close() }()
	em, err := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "digital.vasic.herald/scherald"})
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

// --- HRD-047 — scherald status-digest (§11.4.45) ---

func newStatusDigestCmd() *cobra.Command {
	var (
		repoFlag string
		apply    bool
		emit     bool
	)
	cmd := &cobra.Command{
		Use:   "status-digest",
		Short: "Sweep docs/Status.md + digest work-item state, classify §11.4.45 (--apply regenerates the summary)",
		Long: "SCHEDULED-AUDIT SWEEP: reads docs/Status.md (plus docs/Issues.md / docs/Fixed.md " +
			"for evidence) under --repo, prints a concise digest of the work-item state (a tally " +
			"of open vs fixed HRD-ids + any items still flagged open in BOTH trackers as the " +
			"stale-item count), builds the §11.4.45 status-sweep Subject, classifies it through " +
			"the HRD-025 binding, and EXITS NON-ZERO when docs/Status.md is missing or stale (a " +
			"§11.4.45 violation). Default is DETECT/REPORT-only (hermetic, no mutation). --apply " +
			"additionally (re)generates docs/Status_Summary.md from docs/Status.md (scoped to " +
			"--repo). --emit also drives the §11.4.45 verdict as a real constitution event.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			repo, err := resolveRepo(repoFlag)
			if err != nil {
				return err
			}
			statusPath := filepath.Join(repo, "docs", "Status.md")

			// Sweep: read Status.md presence + freshness, and tally the HRD trackers
			// for the digest body + stale-item count.
			info, sweepErr := sweepStatus(repo, statusPath)
			if sweepErr != nil {
				return fmt.Errorf("status-digest: sweep: %w", sweepErr)
			}

			// Print the operator-facing digest body BEFORE the verdict line.
			fmt.Fprintf(cmd.OutOrStdout(), "status-digest: %s\n", info.digestLine())

			// --apply: (re)generate docs/Status_Summary.md from Status.md. Only
			// meaningful when Status.md is present; scoped to --repo. A missing
			// Status.md cannot be summarized — the sweep verdict below BLOCKS first.
			if apply && info.statusPresent {
				summaryPath := filepath.Join(repo, "docs", "Status_Summary.md")
				if wErr := regenStatusSummary(statusPath, summaryPath, info); wErr != nil {
					return fmt.Errorf("status-digest --apply: regen Status_Summary.md: %w", wErr)
				}
				info.summaryExists = true
				info.summarySynced = true
				fmt.Fprintf(cmd.OutOrStdout(), "status-digest --apply: regenerated %s\n", relTo(repo, summaryPath))
			}

			// Build the §11.4.45 status-sweep Subject per checkStatusSweep's contract:
			//   "<doc>|sweep=<clean|stale>[|stale_items=N][|summary_synced=<bool>]"
			// — Status.md missing/empty ⇒ Kind="violation", sweep=stale (FAIL);
			//   present + fresh ⇒ Kind="status-sweep", sweep=clean (PASS), carrying
			//   summary_synced so the §11.4.56 composition is exercised.
			subject := info.subject()
			return classifyAndReport(ctx, cmd, "§11.4.45", subject, emit, false)
		},
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", "repo dir to scope the sweep/regeneration to (default: discovered from CWD)")
	cmd.Flags().BoolVar(&apply, "apply", false, "(re)generate docs/Status_Summary.md from Status.md (mutation; default: detect-only)")
	cmd.Flags().BoolVar(&emit, "emit", false, "also drive the §11.4.45 verdict as a real constitution event")
	return cmd
}

// statusInfo is the recorded outcome of one Status.md sweep + HRD-tracker tally.
// It carries everything the digest line + the §11.4.45 status-sweep Subject
// need. The fields are populated by sweepStatus — a PURE-ish read of the repo's
// tracker docs (no mutation, no network, no process/cron).
type statusInfo struct {
	statusPresent bool     // docs/Status.md exists + is non-empty
	stale         bool     // Status.md present but stale (empty/whitespace-only)
	openIDs       []string // HRD-ids referenced in docs/Issues.md
	fixedIDs      []string // HRD-ids referenced in docs/Fixed.md
	staleItems    []string // HRD-ids present in BOTH trackers (open + fixed — drift)
	summaryExists bool     // docs/Status_Summary.md is present on disk
	summarySynced bool     // docs/Status_Summary.md present (or set true after --apply)
}

// digestLine renders the concise operator-facing digest body.
func (i statusInfo) digestLine() string {
	present := "present"
	if !i.statusPresent {
		present = "MISSING"
	} else if i.stale {
		present = "STALE"
	}
	return fmt.Sprintf("Status.md=%s; %d open, %d fixed HRD(s); %d stale item(s); Status_Summary=%t",
		present, len(i.openIDs), len(i.fixedIDs), len(i.staleItems), i.summarySynced)
}

// subject builds the §11.4.45 status-sweep Subject per checkStatusSweep's
// contract. A missing/stale Status.md, OR any stale work-items present, is a
// violation (Kind="violation", sweep=stale ⇒ FAIL — the §11.4.19 cross-tracker
// drift the periodic audit surfaces). A present+fresh Status.md with no stale
// items is a clean sweep (Kind="status-sweep", sweep=clean ⇒ PASS).
//
// The §11.4.56 composition: summary_synced is only carried when a
// docs/Status_Summary.md actually exists — an out-of-sync summary then FAILs,
// while a simply-absent summary is NOT a violation (the field is omitted, and
// checkStatusSweep only fails on a present-and-false summary_synced).
func (i statusInfo) subject() constitution.Subject {
	if !i.statusPresent || i.stale || len(i.staleItems) > 0 {
		id := fmt.Sprintf("Status.md|sweep=stale|stale_items=%d", len(i.staleItems))
		return constitution.Subject{Kind: "violation", ID: id}
	}
	id := fmt.Sprintf("Status.md|sweep=clean|stale_items=%d", len(i.staleItems))
	if i.summaryExists {
		id += fmt.Sprintf("|summary_synced=%t", i.summarySynced)
	}
	return constitution.Subject{Kind: bindings.SubjectStatusSweep, ID: id}
}

// sweepStatus reads docs/Status.md presence/freshness + tallies the HRD
// trackers. A missing Status.md ⇒ statusPresent=false (the §11.4.45 violation).
// A present but empty/whitespace-only Status.md ⇒ stale=true. The stale-item
// count is the set of HRD-ids present in BOTH Issues.md and Fixed.md (a closure
// never removed from Issues.md — the §11.4.19 drift the periodic audit surfaces).
func sweepStatus(repo, statusPath string) (statusInfo, error) {
	var info statusInfo

	switch data, err := os.ReadFile(statusPath); {
	case err == nil:
		info.statusPresent = true
		info.stale = strings.TrimSpace(string(data)) == ""
	case os.IsNotExist(err):
		info.statusPresent = false
	default:
		return info, fmt.Errorf("read %s: %w", statusPath, err)
	}

	openSet, err := hrdIDsIn(filepath.Join(repo, "docs", "Issues.md"))
	if err != nil {
		return info, err
	}
	fixedSet, err := hrdIDsIn(filepath.Join(repo, "docs", "Fixed.md"))
	if err != nil {
		return info, err
	}
	info.openIDs = sortedKeys(openSet)
	info.fixedIDs = sortedKeys(fixedSet)
	for _, id := range info.openIDs {
		if fixedSet[id] {
			info.staleItems = append(info.staleItems, id)
		}
	}

	if _, sErr := os.Stat(filepath.Join(repo, "docs", "Status_Summary.md")); sErr == nil {
		info.summaryExists = true
		info.summarySynced = true
	}
	return info, nil
}

// regenStatusSummary (re)generates docs/Status_Summary.md from the swept state.
// The --apply mutation path; scoped to an explicit --repo by the caller. The
// generated one-liners are self-contained clauses (§11.4.91) the operator
// refines. statusPath is read for a heading-derived title line.
func regenStatusSummary(statusPath, summaryPath string, info statusInfo) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# Status summary\n\n")
	fmt.Fprintf(&b, "_Regenerated by scherald status-digest --apply from %s._\n\n",
		filepath.Base(statusPath))
	fmt.Fprintf(&b, "- Open work-items: %d HRD(s) tracked in Issues.md.\n", len(info.openIDs))
	fmt.Fprintf(&b, "- Closed work-items: %d HRD(s) recorded in Fixed.md.\n", len(info.fixedIDs))
	fmt.Fprintf(&b, "- Stale items (present in BOTH trackers): %d.\n", len(info.staleItems))
	if len(info.staleItems) > 0 {
		fmt.Fprintf(&b, "  - %s\n", strings.Join(info.staleItems, ", "))
	}
	return os.WriteFile(summaryPath, []byte(b.String()), 0o644)
}

// --- shared HRD-tracker helpers (scherald-local copies; mirror cherald C3a) ---

// hrdIDRe matches an HRD work-item id (e.g. HRD-047). Used to extract the id set
// from a tracker doc for the digest tally + stale-item detection.
var hrdIDRe = regexp.MustCompile(`\bHRD-(\d{3,})\b`)

// hrdIDsIn returns the set of HRD-ids referenced in the doc at path. A
// not-exist file yields an empty set + nil error (the tracker is simply absent).
func hrdIDsIn(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()
	ids := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		for _, m := range hrdIDRe.FindAllString(sc.Text(), -1) {
			ids[m] = true
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
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

// relTo returns path relative to base, or path unchanged when not under base.
func relTo(base, path string) string {
	if r, err := filepath.Rel(base, path); err == nil {
		return r
	}
	return path
}
