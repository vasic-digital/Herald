// §43 verify/check command bodies for cherald (v1.0.0 Batch C, cluster C3b).
//
// HRD-042 submanifest-verify (§11.4.31) / HRD-051 composite-gate (§11.4.60) /
// HRD-054 spec-version-check (§11.4.73) / HRD-055 catalogue-check (§11.4.74) /
// HRD-038 script-docs-check (§11.4.62 → routed §11.4.60) / HRD-036 creds-scan
// (§16.2 → routed §11.4.10) — the LIVE cherald CLI command bodies that PRODUCE
// the Subjects the HRD-019 cherald constitution bindings classify. These
// replace the cli.StubCmd registrations in internal/stubs for these six rows.
//
// cherald is the COMPLIANCE / docs flavor. Each command DETECTs repo state,
// builds the Subject the matching binding expects (see per-command notes —
// most use the C3a verdictSubject "violation"/"ok" markerCheck Subject; the
// spec-drift + catalogue-miss bindings parse a bespoke Subject.Kind), classifies
// it through the cherald binding, prints the verdict, and EXITs NON-ZERO on
// FAIL. Every command is DETECT-only (no mutation): a CHECK the operator's
// wrapper / a CI gate consults; a PASS (exit 0) means "compliant", a FAIL (exit
// 1) BLOCKs.
//
// Rule-routing notes for the two rows whose nominal §-anchor is NOT a
// cherald-owned rule (stated per the task):
//
//   - HRD-038 script-docs-check nominally cites §11.4.62. §11.4.62 is NOT in the
//     cherald §42.3 catalogue (CheraldRules). The closest cherald-owned rule for
//     a docs/script-documentation defect is §11.4.60 "Documentation composite
//     covenant" (High/Enforce, markerCheck). script-docs-check therefore builds
//     the verdictSubject ("violation" when any script lacks a header docstring)
//     and classifies it against §11.4.60. (§11.4.18 "Script documentation
//     mandate" IS a cherald rule, but its detector keys on a "missing-companion-md"
//     Subject for a missing companion .md — a DIFFERENT defect than a missing
//     in-script header docstring; §11.4.60 is the cleaner composite anchor for
//     this command's check.)
//
//   - HRD-036 creds-scan nominally cites §16.2. §16.2 is NOT in the cherald
//     catalogue. The semantically-correct cherald-owned rule is §11.4.10
//     "Credentials-handling" (Critical/Enforce, checkCredentialLeak), whose
//     detector FAILs a Subject with Kind "credential". creds-scan therefore
//     builds a Subject{Kind:"credential"} on any leak hit (hard BLOCK) and a
//     clean non-credential Subject otherwise.
//
// §107 anti-bluff: with --emit each command additionally drives the REAL
// constitution event through the same in-memory pipeline C3a wires (emitVerdict)
// — the persisted side-effect IS the positive runtime evidence.
//
// Security (operator mandate): creds-scan NEVER prints a matched secret value.
// Every finding is reported with the secret REDACTED (redactSecret) so no
// sensitive data leaks through the command's stdout / the emitted event.
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

	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/commons/gitops"
	constitution "github.com/vasic-digital/herald/commons_constitution"
)

// registerCheckOps replaces the six §43 verify/check stubs with their real
// command bodies. Called from main.go alongside registerDocsOps (cluster C3a)
// and stubs.Register (which now registers nothing for cluster C3b).
func registerCheckOps(root *cobra.Command) {
	root.AddCommand(newSubmanifestVerifyCmd())
	root.AddCommand(newCompositeGateCmd())
	root.AddCommand(newSpecVersionCheckCmd())
	root.AddCommand(newCatalogueCheckCmd())
	root.AddCommand(newScriptDocsCheckCmd())
	root.AddCommand(newCredsScanCmd())
}

// --- HRD-042 — cherald submanifest-verify (§11.4.31) ---

func newSubmanifestVerifyCmd() *cobra.Command {
	var (
		repoFlag string
		emit     bool
	)
	cmd := &cobra.Command{
		Use:   "submanifest-verify",
		Short: "Verify the §11.4.31 Submodule-Dependency-Manifest is present + well-formed",
		Long: "Verifies that the §11.4.31 Submodule-Dependency-Manifest exists and is " +
			"well-formed under --repo: when a .gitmodules file is present, every [submodule] " +
			"stanza MUST carry both a `path =` and a `url =` line. A repo with NO submodules " +
			"(no .gitmodules) is trivially compliant. Builds the §11.4.31 Subject, classifies " +
			"it through the HRD-019 binding (markerCheck: Subject.Kind \"violation\" → FAIL), " +
			"and EXITS NON-ZERO when the manifest is missing/malformed. DETECT-only.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmdCtx(cmd)
			repo, err := resolveRepo(repoFlag)
			if err != nil {
				return err
			}
			problems, err := submanifestProblems(repo)
			if err != nil {
				return fmt.Errorf("submanifest-verify: %w", err)
			}
			sort.Strings(problems)
			fmt.Fprintf(cmd.OutOrStdout(), "submanifest-verify: %d submodule-manifest problem(s) under %s\n", len(problems), repo)

			violation := len(problems) > 0
			id := "submodule manifest well-formed"
			if violation {
				id = "submodule manifest problems: " + strings.Join(problems, "; ")
			}
			return classifyAndReport(ctx, cmd, "§11.4.31", verdictSubject(violation, id), emit, false)
		},
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", "repo dir to scope the manifest check to (default: discovered from CWD)")
	cmd.Flags().BoolVar(&emit, "emit", false, "also drive the §11.4.31 verdict as a real constitution event")
	return cmd
}

// gitmodulesPathRe / gitmodulesURLRe match the `path =` / `url =` lines inside a
// .gitmodules stanza. gitmodulesStanzaRe matches a [submodule "name"] header.
var (
	gitmodulesStanzaRe = regexp.MustCompile(`(?m)^\s*\[submodule\s+"([^"]*)"\]`)
	gitmodulesPathRe   = regexp.MustCompile(`(?m)^\s*path\s*=`)
	gitmodulesURLRe    = regexp.MustCompile(`(?m)^\s*url\s*=`)
)

// submanifestProblems returns the list of §11.4.31 manifest problems under repo.
// No .gitmodules ⇒ no problems (a repo without submodules is compliant). A
// present .gitmodules MUST declare at least one stanza, and every stanza MUST
// carry both `path =` and `url =`.
func submanifestProblems(repo string) ([]string, error) {
	gm := filepath.Join(repo, ".gitmodules")
	data, err := os.ReadFile(gm)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no submodules ⇒ compliant
		}
		return nil, err
	}
	content := string(data)
	stanzas := gitmodulesStanzaRe.FindAllStringSubmatchIndex(content, -1)
	if len(stanzas) == 0 {
		return []string{".gitmodules present but declares no [submodule] stanza"}, nil
	}
	var problems []string
	for i, loc := range stanzas {
		name := content[loc[2]:loc[3]]
		// The stanza body runs from the end of this header to the start of the
		// next header (or EOF for the last stanza).
		bodyStart := loc[1]
		bodyEnd := len(content)
		if i+1 < len(stanzas) {
			bodyEnd = stanzas[i+1][0]
		}
		body := content[bodyStart:bodyEnd]
		if !gitmodulesPathRe.MatchString(body) {
			problems = append(problems, fmt.Sprintf("submodule %q missing `path =`", name))
		}
		if !gitmodulesURLRe.MatchString(body) {
			problems = append(problems, fmt.Sprintf("submodule %q missing `url =`", name))
		}
	}
	return problems, nil
}

// --- HRD-051 — cherald composite-gate (§11.4.60) ---

func newCompositeGateCmd() *cobra.Command {
	var (
		repoFlag string
		emit     bool
	)
	cmd := &cobra.Command{
		Use:   "composite-gate",
		Short: "CM-DOCS-COMPOSITE-SYNC: aggregate the §11.4.60 doc-composite checks (detect-only)",
		Long: "The canonical CM-DOCS-COMPOSITE-SYNC entrypoint: aggregates the C3a " +
			"doc-composite checks under one §11.4.60 verdict — (1) every tracked .md carries " +
			"its §11.4.61 metadata + ToC, (2) README.md doc-links all resolve on disk, (3) " +
			"docs/Fixed_Summary.md is in parity with docs/Fixed.md. Builds the §11.4.60 " +
			"Subject, classifies it through the HRD-019 binding (markerCheck), and EXITS " +
			"NON-ZERO when ANY constituent check is in breach. DETECT-only — it never mutates " +
			"(unlike the individual --apply commands).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmdCtx(cmd)
			repo, err := resolveRepo(repoFlag)
			if err != nil {
				return err
			}

			var breaches []string

			// (1) metadata + ToC on every tracked .md — EXCEPT the derived
			// tracker docs (Fixed.md / Fixed_Summary.md / Issues.md / Status.md +
			// their *_Summary siblings), which are machine-maintained and do not
			// carry the §11.4.61 Revision/ToC block by design. Checking them here
			// would be a false positive.
			docs, derr := docsToCheck(repo, nil)
			if derr != nil {
				return fmt.Errorf("composite-gate: enumerate docs: %w", derr)
			}
			var missingMeta []string
			for _, d := range docs {
				if isTrackerDoc(d) {
					continue
				}
				ok, mErr := docHasMetadata(d)
				if mErr != nil {
					return fmt.Errorf("composite-gate: read %s: %w", d, mErr)
				}
				if !ok {
					missingMeta = append(missingMeta, relTo(repo, d))
				}
			}
			sort.Strings(missingMeta)
			if len(missingMeta) > 0 {
				breaches = append(breaches, fmt.Sprintf("§11.4.61 metadata/ToC missing in %d doc(s): %s", len(missingMeta), strings.Join(missingMeta, ", ")))
			}

			// (2) README doc-links resolve.
			readme := filepath.Join(repo, "README.md")
			if _, sErr := os.Stat(readme); sErr == nil {
				dangling, rerr := readmeDanglingLinks(repo, readme)
				if rerr != nil {
					return fmt.Errorf("composite-gate: readme: %w", rerr)
				}
				sort.Strings(dangling)
				if len(dangling) > 0 {
					breaches = append(breaches, fmt.Sprintf("§11.4.59 README dangling links: %s", strings.Join(dangling, ", ")))
				}
			}

			// (3) Fixed_Summary parity vs Fixed.md.
			fixedIDs, ferr := hrdIDsIn(filepath.Join(repo, "docs", "Fixed.md"))
			if ferr != nil {
				return fmt.Errorf("composite-gate: Fixed.md: %w", ferr)
			}
			summaryIDs, serr := hrdIDsIn(filepath.Join(repo, "docs", "Fixed_Summary.md"))
			if serr != nil {
				return fmt.Errorf("composite-gate: Fixed_Summary.md: %w", serr)
			}
			var summaryGap []string
			for _, id := range sortedKeys(fixedIDs) {
				if !summaryIDs[id] {
					summaryGap = append(summaryGap, id)
				}
			}
			if len(summaryGap) > 0 {
				breaches = append(breaches, fmt.Sprintf("§11.4.53 Fixed_Summary missing: %s", strings.Join(summaryGap, ", ")))
			}

			fmt.Fprintf(cmd.OutOrStdout(), "composite-gate: checked %d doc(s); %d composite breach(es)\n", len(docs), len(breaches))

			violation := len(breaches) > 0
			id := "doc-composite covenant holds"
			if violation {
				id = "composite breaches: " + strings.Join(breaches, " | ")
			}
			return classifyAndReport(ctx, cmd, "§11.4.60", verdictSubject(violation, id), emit, false)
		},
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", "repo dir to scope the composite check to (default: discovered from CWD)")
	cmd.Flags().BoolVar(&emit, "emit", false, "also drive the §11.4.60 verdict as a real constitution event")
	return cmd
}

// isTrackerDoc reports whether path is a machine-maintained tracker/summary doc
// (Fixed.md / Fixed_Summary.md / Issues.md / Issues_Summary.md / Status.md /
// Status_Summary.md / CONTINUATION.md) that by design does NOT carry the
// §11.4.61 Revision/ToC metadata block, so the composite metadata sub-check
// skips it.
func isTrackerDoc(path string) bool {
	switch filepath.Base(path) {
	case "Fixed.md", "Fixed_Summary.md",
		"Issues.md", "Issues_Summary.md",
		"Status.md", "Status_Summary.md",
		"CONTINUATION.md":
		return true
	}
	return false
}

// --- HRD-054 — cherald spec-version-check (§11.4.73, detector checkSpecDrift) ---

func newSpecVersionCheckCmd() *cobra.Command {
	var (
		repoFlag string
		specFlag string
		modified bool
		emit     bool
	)
	cmd := &cobra.Command{
		Use:   "spec-version-check",
		Short: "Audit a spec doc's Revision header vs its git-modified state (§11.4.73)",
		Long: "Observes a spec doc (--spec, default docs/specs/mvp/specification.V4.md) under " +
			"--repo: whether its content has been MODIFIED in the working tree without its " +
			"`Revision` header being bumped. When the doc is git-modified (or --modified is " +
			"forced) but its committed Revision value still matches HEAD, that is §11.4.73 " +
			"drift. Builds the Subject checkSpecDrift parses (Kind \"revision-unchanged\" → " +
			"FAIL, \"spec-doc\" → PASS), classifies it, and EXITS NON-ZERO on drift. DETECT-only.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmdCtx(cmd)
			repo, err := resolveRepo(repoFlag)
			if err != nil {
				return err
			}
			spec := specFlag
			if spec == "" {
				spec = filepath.Join("docs", "specs", "mvp", "specification.V4.md")
			}
			specAbs := spec
			if !filepath.IsAbs(specAbs) {
				specAbs = filepath.Join(repo, spec)
			}
			if _, sErr := os.Stat(specAbs); sErr != nil {
				return fmt.Errorf("spec-version-check: spec doc %s not found", specAbs)
			}

			// Drift detection: the spec content changed (working-tree modified vs
			// HEAD, OR the --modified override) while the committed Revision value
			// is unchanged vs HEAD. We compute both edges deterministically.
			contentChanged, revisionChanged, detail, derr := specDrift(ctx, repo, spec, modified)
			if derr != nil {
				return fmt.Errorf("spec-version-check: %w", derr)
			}
			drift := contentChanged && !revisionChanged
			fmt.Fprintf(cmd.OutOrStdout(), "spec-version-check: %s (content-changed=%t, revision-changed=%t)\n", detail, contentChanged, revisionChanged)

			// Build the Subject checkSpecDrift expects: Kind "revision-unchanged" on
			// drift (→ FAIL), Kind "spec-doc" otherwise (→ PASS).
			subject := constitution.Subject{Kind: "spec-doc", ID: spec}
			if drift {
				subject.Kind = "revision-unchanged"
			}
			return classifyAndReport(ctx, cmd, "§11.4.73", subject, emit, false)
		},
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", "repo dir to scope the check to (default: discovered from CWD)")
	cmd.Flags().StringVar(&specFlag, "spec", "", "spec doc path relative to --repo (default: docs/specs/mvp/specification.V4.md)")
	cmd.Flags().BoolVar(&modified, "modified", false, "force-treat the spec content as modified (test seam / non-git checkout)")
	cmd.Flags().BoolVar(&emit, "emit", false, "also drive the §11.4.73 verdict as a real constitution event")
	return cmd
}

// revisionHeaderRe matches a `| Revision | <n> |` table row (the §11.4.44 / spec
// Revision header). Captures the value for HEAD-vs-worktree comparison.
var revisionHeaderRe = regexp.MustCompile(`(?m)^\s*\|\s*Revision\s*\|\s*([^|]+?)\s*\|`)

// specDrift computes (contentChanged, revisionChanged) for the spec doc.
//   - contentChanged: the working-tree file differs from HEAD (git diff), OR the
//     --modified override is set (the hermetic/non-git test seam).
//   - revisionChanged: the Revision header value in the working tree differs from
//     the value committed at HEAD. When HEAD has no committed copy (a brand-new
//     spec doc not yet committed), revisionChanged is treated as true (a new doc
//     carries its own initial revision — not drift).
func specDrift(ctx context.Context, repo, spec string, forceModified bool) (contentChanged, revisionChanged bool, detail string, err error) {
	specAbs := spec
	if !filepath.IsAbs(specAbs) {
		specAbs = filepath.Join(repo, spec)
	}
	wtData, rerr := os.ReadFile(specAbs)
	if rerr != nil {
		return false, false, "", rerr
	}
	wtRev := firstSubmatch(revisionHeaderRe, string(wtData))

	r := gitops.NewRunner(repo)
	// `git show HEAD:<path>` returns the committed copy of the spec at HEAD.
	headData, headErr := r.Git(ctx, "show", "HEAD:"+filepath.ToSlash(spec))
	if headErr != nil {
		// No committed HEAD copy (uncommitted new doc, or not a git repo). Content
		// is "changed" only when forced; a new doc carries its own revision.
		if forceModified {
			return true, true, "no HEAD copy; --modified forced (new doc carries its own revision)", nil
		}
		return false, true, "no HEAD copy (uncommitted / non-git) — no drift", nil
	}
	headRev := firstSubmatch(revisionHeaderRe, headData)

	// `git show` output is trailing-newline-trimmed by gitops.Git, so compare
	// both sides with trailing whitespace stripped to avoid a spurious diff.
	contentChanged = forceModified || (strings.TrimRight(string(wtData), "\n") != strings.TrimRight(headData, "\n"))
	revisionChanged = wtRev != headRev
	detail = fmt.Sprintf("revision worktree=%q HEAD=%q", wtRev, headRev)
	return contentChanged, revisionChanged, detail, nil
}

// firstSubmatch returns the first capture group of re in s, or "" if no match.
func firstSubmatch(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// --- HRD-055 — cherald catalogue-check <pr> (§11.4.74, detector checkCatalogueMiss) ---

func newCatalogueCheckCmd() *cobra.Command {
	var (
		repoFlag string
		prBody   string
		diffFile string
		emit     bool
	)
	cmd := &cobra.Command{
		Use:   "catalogue-check [<pr-ref>]",
		Short: "Scan a PR description / diff for the §11.4.74 Catalogue-Check line",
		Long: "Observes a PR description (--pr-body, inline or @file) or a changed-files / diff " +
			"list (--diff-file) and verifies it carries a `Catalogue-Check:` line (the §11.4.74 " +
			"submodule-catalogue-first attestation). A trivial change (empty body/diff) is " +
			"compliant. A non-trivial change with NO Catalogue-Check line is a §11.4.74 miss. " +
			"Builds the Subject checkCatalogueMiss parses (Kind \"missing-catalogue-check\" → " +
			"FAIL, \"pull-request\" → PASS), classifies it, and EXITS NON-ZERO on a miss.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmdCtx(cmd)
			prRef := "pr"
			if len(args) == 1 {
				prRef = strings.TrimSpace(args[0])
			}

			// Resolve the body text: --pr-body (literal or @file), else --diff-file.
			body, src, berr := resolveCatalogueBody(repoFlag, prBody, diffFile)
			if berr != nil {
				return fmt.Errorf("catalogue-check: %w", berr)
			}

			trivial := strings.TrimSpace(body) == ""
			hasLine := catalogueCheckLineRe.MatchString(body)
			miss := !trivial && !hasLine
			fmt.Fprintf(cmd.OutOrStdout(), "catalogue-check: %s (trivial=%t, has-catalogue-check=%t) from %s\n", prRef, trivial, hasLine, src)

			// Build the Subject checkCatalogueMiss expects.
			subject := constitution.Subject{Kind: "pull-request", ID: prRef}
			if miss {
				subject.Kind = "missing-catalogue-check"
			}
			return classifyAndReport(ctx, cmd, "§11.4.74", subject, emit, false)
		},
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", "repo dir scoping a relative @file / --diff-file (default: CWD)")
	cmd.Flags().StringVar(&prBody, "pr-body", "", "PR description (literal text, or @path to read from a file)")
	cmd.Flags().StringVar(&diffFile, "diff-file", "", "path to a changed-files / diff list to scan instead of --pr-body")
	cmd.Flags().BoolVar(&emit, "emit", false, "also drive the §11.4.74 verdict as a real constitution event")
	return cmd
}

// catalogueCheckLineRe matches a `Catalogue-Check:` attestation line (§11.4.74),
// case-insensitive, anchored to a line start.
var catalogueCheckLineRe = regexp.MustCompile(`(?mi)^\s*Catalogue-Check\s*:`)

// resolveCatalogueBody returns the body text to scan + a human-readable source
// label. --pr-body wins (literal, or @file when it begins with '@'); else
// --diff-file is read; else empty (trivial).
func resolveCatalogueBody(repo, prBody, diffFile string) (body, src string, err error) {
	if prBody != "" {
		if strings.HasPrefix(prBody, "@") {
			p := strings.TrimPrefix(prBody, "@")
			if !filepath.IsAbs(p) && repo != "" {
				p = filepath.Join(repo, p)
			}
			data, rerr := os.ReadFile(p)
			if rerr != nil {
				return "", "", fmt.Errorf("read --pr-body @%s: %w", p, rerr)
			}
			return string(data), "--pr-body @" + p, nil
		}
		return prBody, "--pr-body (literal)", nil
	}
	if diffFile != "" {
		p := diffFile
		if !filepath.IsAbs(p) && repo != "" {
			p = filepath.Join(repo, p)
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			return "", "", fmt.Errorf("read --diff-file %s: %w", p, rerr)
		}
		return string(data), "--diff-file " + p, nil
	}
	return "", "(no body — trivial)", nil
}

// --- HRD-038 — cherald script-docs-check (§11.4.62 → routed §11.4.60) ---

func newScriptDocsCheckCmd() *cobra.Command {
	var (
		repoFlag string
		emit     bool
	)
	cmd := &cobra.Command{
		Use:   "script-docs-check",
		Short: "Audit that every scripts/*.sh carries a leading header docstring (§11.4.62 → §11.4.60)",
		Long: "Scans <repo>/scripts/*.sh and verifies each carries a leading header docstring " +
			"(a comment block immediately after the optional shebang, before the first " +
			"non-comment line). §11.4.62 is NOT a cherald-owned rule, so the verdict is " +
			"classified against the closest cherald rule — §11.4.60 \"Documentation composite " +
			"covenant\" — via the markerCheck Subject (\"violation\" on any undocumented " +
			"script). EXITS NON-ZERO when any script lacks a header docstring. DETECT-only.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmdCtx(cmd)
			repo, err := resolveRepo(repoFlag)
			if err != nil {
				return err
			}
			undocumented, scanned, err := undocumentedScripts(repo)
			if err != nil {
				return fmt.Errorf("script-docs-check: %w", err)
			}
			sort.Strings(undocumented)
			fmt.Fprintf(cmd.OutOrStdout(), "script-docs-check: scanned %d script(s); %d undocumented\n", scanned, len(undocumented))

			violation := len(undocumented) > 0
			id := "all scripts carry a header docstring"
			if violation {
				id = "undocumented scripts: " + strings.Join(undocumented, ", ")
			}
			// Routed to §11.4.60 (closest cherald-owned rule; §11.4.62 is not a
			// cherald rule). High/Enforce ⇒ a missing docstring is a hard BLOCK.
			return classifyAndReport(ctx, cmd, "§11.4.60", verdictSubject(violation, id), emit, false)
		},
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", "repo dir whose scripts/*.sh are audited (default: discovered from CWD)")
	cmd.Flags().BoolVar(&emit, "emit", false, "also drive the §11.4.60 verdict as a real constitution event")
	return cmd
}

// undocumentedScripts returns the relative paths of scripts/*.sh under repo that
// lack a leading header docstring, plus the count scanned. A script is
// "documented" when, after an optional shebang line, the next non-blank line is
// a comment (#-prefixed). No scripts/ dir ⇒ nothing to audit (compliant).
func undocumentedScripts(repo string) (undocumented []string, scanned int, err error) {
	dir := filepath.Join(repo, "scripts")
	entries, derr := os.ReadDir(dir)
	if derr != nil {
		if os.IsNotExist(derr) {
			return nil, 0, nil
		}
		return nil, 0, derr
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sh") {
			continue
		}
		scanned++
		p := filepath.Join(dir, e.Name())
		ok, herr := scriptHasHeaderDoc(p)
		if herr != nil {
			return nil, scanned, herr
		}
		if !ok {
			undocumented = append(undocumented, filepath.Join("scripts", e.Name()))
		}
	}
	return undocumented, scanned, nil
}

// scriptHasHeaderDoc reports whether the shell script at path opens with a
// header docstring: skipping an optional leading shebang + blank lines, the
// first content line MUST be a comment (#...).
func scriptHasHeaderDoc(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	first := true
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if first {
			first = false
			if strings.HasPrefix(line, "#!") {
				continue // shebang — keep looking for the docstring
			}
		}
		if line == "" {
			continue // tolerate blank lines between shebang and docstring
		}
		return strings.HasPrefix(line, "#"), nil
	}
	if err := sc.Err(); err != nil {
		return false, err
	}
	return false, nil // empty file ⇒ no docstring
}

// --- HRD-036 — cherald creds-scan (§16.2 → routed §11.4.10) ---

func newCredsScanCmd() *cobra.Command {
	var (
		repoFlag string
		pathFlag string
		emit     bool
	)
	cmd := &cobra.Command{
		Use:   "creds-scan",
		Short: "Scan the repo for leaked-credential patterns; REDACT every finding (§16.2 → §11.4.10)",
		Long: "Scans --path (default --repo) for leaked-credential patterns — AWS access keys " +
			"(AKIA…), AWS_SECRET assignments, PEM private-key blocks, password= / token= " +
			"assignments, Telegram bot tokens. SECURITY: every finding is reported with the " +
			"secret value REDACTED — the actual secret is NEVER printed (operator mandate: no " +
			"sensitive data may leak through logs). §16.2 is NOT a cherald-owned rule, so the " +
			"verdict is classified against the semantically-correct §11.4.10 " +
			"\"Credentials-handling\" rule (Subject Kind \"credential\" on a hit → FAIL). EXITS " +
			"NON-ZERO on any hit. DETECT-only (never edits the scanned tree).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmdCtx(cmd)
			scanRoot := pathFlag
			if scanRoot == "" {
				repo, err := resolveRepo(repoFlag)
				if err != nil {
					return err
				}
				scanRoot = repo
			} else {
				abs, err := filepath.Abs(scanRoot)
				if err != nil {
					return fmt.Errorf("creds-scan: resolve --path: %w", err)
				}
				scanRoot = abs
			}

			findings, err := scanCredentials(scanRoot)
			if err != nil {
				return fmt.Errorf("creds-scan: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "creds-scan: scanned %s; %d leaked-credential finding(s)\n", scanRoot, len(findings))
			for _, f := range findings {
				// REDACTED report — the secret value is never printed.
				fmt.Fprintf(cmd.OutOrStdout(), "  [LEAK] %s:%d %s = %s\n", f.RelPath, f.Line, f.Pattern, f.Redacted)
			}

			hit := len(findings) > 0
			// Build the §11.4.10 Subject: a credential-bearing artefact on a hit
			// (checkCredentialLeak FAILs Kind "credential"), a clean file Subject
			// otherwise (PASS). The ID names the scan root (no secret value).
			subject := constitution.Subject{Kind: "file", ID: scanRoot + "/<scan>"}
			if hit {
				subject.Kind = "credential"
				subject.ID = fmt.Sprintf("%d leaked-credential finding(s) under %s (values redacted)", len(findings), scanRoot)
			}
			// §11.4.10 is Critical/Enforce ⇒ any leak is a hard BLOCK.
			return classifyAndReport(ctx, cmd, "§11.4.10", subject, emit, false)
		},
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", "repo dir to scan (default: discovered from CWD; --path overrides)")
	cmd.Flags().StringVar(&pathFlag, "path", "", "explicit path to scan (file or dir); overrides --repo")
	cmd.Flags().BoolVar(&emit, "emit", false, "also drive the §11.4.10 verdict as a real constitution event")
	return cmd
}

// credFinding is one redacted leaked-credential hit.
type credFinding struct {
	RelPath  string
	Line     int
	Pattern  string
	Redacted string // the matched secret, REDACTED (never the real value)
}

// credPattern pairs a human label with a compiled detector regex. The capture
// group (when present) is the secret token that gets redacted.
type credPattern struct {
	Name string
	Re   *regexp.Regexp
}

// credPatterns is the leaked-credential detector set. Each regex captures (group
// 1, when present) the secret token so it can be redacted before reporting.
var credPatterns = []credPattern{
	{"aws-access-key-id", regexp.MustCompile(`\b(AKIA[0-9A-Z]{16})\b`)},
	{"aws-secret", regexp.MustCompile(`(?i)aws_secret(?:_access_key)?\s*[:=]\s*["']?([A-Za-z0-9/+=]{20,})["']?`)},
	{"pem-private-key", regexp.MustCompile(`-----BEGIN(?: RSA| EC| OPENSSH| PGP| DSA)? PRIVATE KEY-----`)},
	{"password-assignment", regexp.MustCompile(`(?i)\bpassword\s*[:=]\s*["']?([^\s"']{6,})["']?`)},
	{"token-assignment", regexp.MustCompile(`(?i)\b(?:api[_-]?token|access[_-]?token|secret[_-]?token)\s*[:=]\s*["']?([^\s"']{8,})["']?`)},
	{"telegram-bot-token", regexp.MustCompile(`\b(\d{6,}:[A-Za-z0-9_-]{30,})\b`)},
}

// scanCredentials walks scanRoot (a file or dir) and returns every redacted
// leaked-credential finding. Binary/vendored/VCS dirs are skipped. PURE read:
// it NEVER writes to the scanned tree.
func scanCredentials(scanRoot string) ([]credFinding, error) {
	var findings []credFinding
	skip := map[string]bool{".git": true, "node_modules": true, "vendor": true, "submodules": true, "containers": true}

	scanFile := func(path, rel string) error {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		ln := 0
		for sc.Scan() {
			ln++
			line := sc.Text()
			for _, cp := range credPatterns {
				m := cp.Re.FindStringSubmatch(line)
				if m == nil {
					continue
				}
				secret := m[0]
				if len(m) > 1 && m[1] != "" {
					secret = m[1]
				}
				findings = append(findings, credFinding{
					RelPath:  rel,
					Line:     ln,
					Pattern:  cp.Name,
					Redacted: redactSecret(secret),
				})
			}
		}
		return sc.Err()
	}

	fi, err := os.Stat(scanRoot)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		if serr := scanFile(scanRoot, filepath.Base(scanRoot)); serr != nil {
			return nil, serr
		}
		return findings, nil
	}

	walkErr := filepath.WalkDir(scanRoot, func(p string, d os.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			if skip[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip obvious binaries by extension to avoid false positives + huge reads.
		if isLikelyBinary(d.Name()) {
			return nil
		}
		rel := relTo(scanRoot, p)
		return scanFile(p, rel)
	})
	return findings, walkErr
}

// isLikelyBinary reports whether name has an extension we should not text-scan.
func isLikelyBinary(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".pdf", ".zip", ".gz", ".tar",
		".ico", ".woff", ".woff2", ".ttf", ".so", ".dylib", ".a", ".o",
		".docx", ".bin", ".exe", ".class", ".jar":
		return true
	}
	return false
}

// redactSecret returns a redacted form of a secret that NEVER exposes the
// sensitive middle. It keeps at most the first 4 chars as a recognisability
// prefix (so an operator can locate the offending value) and masks the rest,
// always printing a fixed-width mask so the length is not leaked for short
// secrets.
func redactSecret(secret string) string {
	const keep = 4
	if len(secret) <= keep {
		return "****REDACTED****"
	}
	return secret[:keep] + "****REDACTED****"
}

// --- shared helpers ---

// cmdCtx returns the command's context or a fresh background context.
func cmdCtx(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}
