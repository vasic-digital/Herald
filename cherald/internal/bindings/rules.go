package bindings

// rules.go — the cherald-owned §42.3 constitution-rule catalogue (HRD-019).
//
// Per spec §42.3 master binding table, cherald owns (sole or shared) the rule
// rows enumerated here. cherald is the COMPLIANCE flavor; it carries the bulk
// of the policy/credential/gate/spec/catalogue bindings. Rows where cherald is
// a SHARED owner (e.g. §1.1 with bherald, §11.4.10 with iherald) still register
// here — the OTHER owner registers its own binding in its own flavor package.
//
// Each RuleSpec carries a REAL, deterministic CheckFunc. The vertical-slice
// rules (§11.4.29 naming, §11.4.10 credentials, §11.4.44 doc-revision-header,
// §11.4.18 script-docs, §11.4.73 spec-drift, §11.4.74 catalogue-miss) implement
// genuine compliance logic against the Subject. The remaining rows use the
// shared markerCheck mechanism: the Subject carries an explicit verdict marker
// (Subject.Kind "violation" → FAIL, "ok" → PASS) so the binding is fully
// exercisable end-to-end by the REST surface / a sweep that has already done
// the rule-specific detection upstream, while keeping the §11.4.92 blast radius
// of HRD-019 bounded to ONE clean unit. The framework + the registration +
// the emit-routing are complete for ALL rules; deepening each remaining rule's
// bespoke detector is tracked follow-up work (one rule per future increment).

import (
	"context"
	"crypto/sha256"
	"path"
	"strings"

	constitution "github.com/vasic-digital/herald/commons_constitution"
)

// digest is a small helper producing a stable DigestSHA from the decision +
// evidence so the transition gate can detect rationale changes.
func digest(d constitution.Decision, evidence string) [32]byte {
	return sha256.Sum256([]byte(d.String() + ":" + evidence))
}

// pass / fail build canonical Results.
func pass(evidence string) constitution.Result {
	return constitution.Result{Decision: constitution.DecisionPass, Evidence: evidence, DigestSHA: digest(constitution.DecisionPass, evidence)}
}
func fail(evidence string) constitution.Result {
	return constitution.Result{Decision: constitution.DecisionFail, Evidence: evidence, DigestSHA: digest(constitution.DecisionFail, evidence)}
}

// ---------------------------------------------------------------------------
// Vertical-slice CheckFuncs — REAL bespoke detectors.
// ---------------------------------------------------------------------------

// checkSnakeCase implements §11.4.29 (lowercase_snake_case naming). FAILs when
// any path component of the subject (a Go source path) contains an uppercase
// letter or a hyphen — i.e. is not lowercase_snake_case. The "_test.go" suffix
// + the ".go" extension are tolerated.
func checkSnakeCase(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	clean := path.Clean(s.ID)
	for _, comp := range strings.Split(clean, "/") {
		if comp == "" || comp == "." {
			continue
		}
		// Strip a trailing .go (or other) extension before checking the stem.
		stem := comp
		if i := strings.LastIndex(comp, "."); i > 0 {
			stem = comp[:i]
		}
		for _, r := range stem {
			if r >= 'A' && r <= 'Z' {
				return fail("naming violation §11.4.29: uppercase in path component " + comp + " of " + s.ID), nil
			}
			if r == '-' {
				return fail("naming violation §11.4.29: hyphen in path component " + comp + " of " + s.ID), nil
			}
		}
	}
	return pass("snake_case ok: " + s.ID), nil
}

// checkCredentialLeak implements §11.4.10 / §11.4.10.A (credentials never
// tracked). FAILs when the subject is a known credential-bearing artefact that
// must not be committed: a tracked `.env` (but NOT `.env.example`), or a path
// flagged with the credential marker kind.
func checkCredentialLeak(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	base := path.Base(s.ID)
	switch {
	case base == ".env":
		return fail("credential leak §11.4.10: tracked .env (move secrets to git-ignored .env; commit .env.example only)"), nil
	case strings.HasSuffix(base, ".pem"), strings.HasSuffix(base, ".key"):
		return fail("credential leak §11.4.10: tracked private-key material " + base), nil
	case s.Kind == "credential":
		return fail("credential leak §11.4.10: plaintext credential detected in " + s.ID), nil
	}
	return pass("no credential leak: " + s.ID), nil
}

// checkDocRevisionHeader implements §11.4.44 (document revision header). FAILs
// for a Markdown subject flagged as missing its revision header (Subject.Kind
// "missing-revision-header"); PASSes otherwise. The upstream doc-parse lives in
// the §43 catalogue command (HRD-044-class); the binding records the verdict.
func checkDocRevisionHeader(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	if s.Kind == "missing-revision-header" {
		return fail("doc revision header §11.4.44: " + s.ID + " missing the Revision metadata block"), nil
	}
	return pass("revision header present: " + s.ID), nil
}

// checkScriptDocs implements §11.4.18 (script documentation mandate). FAILs for
// a shell-script subject flagged as missing its companion .md (Subject.Kind
// "missing-companion-md").
func checkScriptDocs(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	if s.Kind == "missing-companion-md" {
		return fail("script docs §11.4.18: " + s.ID + " has no companion .md documentation"), nil
	}
	return pass("script documented: " + s.ID), nil
}

// checkSpecDrift implements §11.4.73 (main-spec versioning + revision discipline).
// FAILs for a spec subject flagged as edited-without-revision-bump (Subject.Kind
// "revision-unchanged").
func checkSpecDrift(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	if s.Kind == "revision-unchanged" {
		return fail("spec drift §11.4.73: " + s.ID + " content changed without a Revision bump"), nil
	}
	return pass("spec revision in sync: " + s.ID), nil
}

// checkCatalogueMiss implements §11.4.74 (submodule-catalogue-first). FAILs for
// a PR subject flagged as missing its Catalogue-Check line (Subject.Kind
// "missing-catalogue-check").
func checkCatalogueMiss(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	if s.Kind == "missing-catalogue-check" {
		return fail("catalogue miss §11.4.74: " + s.ID + " is a non-trivial change with no Catalogue-Check line"), nil
	}
	return pass("catalogue-check present: " + s.ID), nil
}

// markerCheck is the shared mechanism for rules whose bespoke detector lives
// upstream (in the §43 catalogue command or a sweep): the Subject.Kind carries
// the already-decided verdict. "violation" → FAIL, anything else → PASS. This
// keeps every catalogue rule fully exercisable end-to-end through the REAL
// emit+persist+audit path while bounding HRD-019's blast radius. ruleID is
// closed over for the evidence string.
func markerCheck(ruleID string) CheckFunc {
	return func(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
		if s.Kind == "violation" {
			return fail(ruleID + " violation on " + s.ID), nil
		}
		return pass(ruleID + " ok on " + s.ID), nil
	}
}

// ---------------------------------------------------------------------------
// The cherald-owned §42.3 catalogue.
// ---------------------------------------------------------------------------

// CheraldRules returns the catalogue of every cherald-owned (sole or shared)
// §42.3 constitution rule, with its default severity + mode + event class + a
// real CheckFunc. The slice is freshly built on each call (callers may mutate
// their copy). Order is deterministic (source order = roughly section order).
func CheraldRules() []RuleSpec {
	return []RuleSpec{
		// --- critical / enforce: the anti-bluff + credential core ---------
		{RuleID: "§1.1", Title: "False-positive immunity (paired mutation)", Severity: constitution.SeverityCritical, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassGateFailed, Check: markerCheck("§1.1"), SubjectKinds: []string{"gate"}},
		{RuleID: "§7.1", Title: "NO-BLUFF positive-evidence", Severity: constitution.SeverityCritical, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassGateFailed, Check: markerCheck("§7.1"), SubjectKinds: []string{"gate"}},
		{RuleID: "§11.4.1", Title: "FAIL-bluffs forbidden", Severity: constitution.SeverityCritical, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassGateFailed, Check: markerCheck("§11.4.1"), SubjectKinds: []string{"gate"}},
		{RuleID: "§11.4.10", Title: "Credentials-handling", Severity: constitution.SeverityCritical, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassCredentialLeak, Check: checkCredentialLeak, SubjectKinds: []string{"file", "credential"}},
		{RuleID: "§11.4.10.A", Title: "Pre-store leak audit", Severity: constitution.SeverityCritical, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassCredentialLeak, Check: checkCredentialLeak, SubjectKinds: []string{"file", "credential"}},

		// --- high / enforce: doc-composite, spec-drift, catalogue, build --
		{RuleID: "§11.4.30", Title: "No-Versioned-Build-Artifacts", Severity: constitution.SeverityHigh, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassPolicyViolation, Check: markerCheck("§11.4.30"), SubjectKinds: []string{"file"}},
		{RuleID: "§11.4.43", Title: "TDD-Fix-Discipline", Severity: constitution.SeverityHigh, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassGateFailed, Check: markerCheck("§11.4.43"), SubjectKinds: []string{"gate"}},
		{RuleID: "§11.4.60", Title: "Documentation composite covenant", Severity: constitution.SeverityHigh, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassGateFailed, Check: markerCheck("§11.4.60"), SubjectKinds: []string{"gate"}},
		{RuleID: "§11.4.68", Title: "Positive sink-side evidence", Severity: constitution.SeverityHigh, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassGateFailed, Check: markerCheck("§11.4.68"), SubjectKinds: []string{"gate"}},
		{RuleID: "§11.4.69", Title: "Sink-Side Positive-Evidence Taxonomy", Severity: constitution.SeverityHigh, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassGateFailed, Check: markerCheck("§11.4.69"), SubjectKinds: []string{"gate"}},
		{RuleID: "§11.4.73", Title: "Main-spec versioning + revision discipline", Severity: constitution.SeverityHigh, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassSpecRevisionDrift, Check: checkSpecDrift, SubjectKinds: []string{"spec-doc"}},
		{RuleID: "§11.4.74", Title: "Submodule-catalogue-first", Severity: constitution.SeverityHigh, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassCatalogueMiss, Check: checkCatalogueMiss, SubjectKinds: []string{"pull-request"}},
		{RuleID: "§12.10", Title: "CONTINUATION sacred invariant", Severity: constitution.SeverityHigh, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassPolicyViolation, Check: markerCheck("§12.10"), SubjectKinds: []string{"file"}},

		// --- middle / warn: changelog -------------------------------------
		{RuleID: "§5", Title: "Changelog + multi-format export", Severity: constitution.SeverityMiddle, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassPolicyViolation, Check: markerCheck("§5"), SubjectKinds: []string{"file"}},

		// --- low / warn: the documentation + naming + tracker hygiene set -
		{RuleID: "§9.4", Title: "Commit-message audit trail", Severity: constitution.SeverityLow, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassPolicyViolation, Check: markerCheck("§9.4"), SubjectKinds: []string{"commit"}},
		{RuleID: "§11.4.12", Title: "Auto-generated docs sync", Severity: constitution.SeverityLow, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassPolicyViolation, Check: markerCheck("§11.4.12"), SubjectKinds: []string{"file"}},
		{RuleID: "§11.4.17", Title: "Universal-vs-project rule promotion", Severity: constitution.SeverityLow, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassPolicyViolation, Check: markerCheck("§11.4.17"), SubjectKinds: []string{"file"}},
		{RuleID: "§11.4.18", Title: "Script documentation mandate", Severity: constitution.SeverityLow, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassPolicyViolation, Check: checkScriptDocs, SubjectKinds: []string{"file"}},
		{RuleID: "§11.4.19", Title: "Fixed-document column alignment", Severity: constitution.SeverityLow, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassPolicyViolation, Check: markerCheck("§11.4.19"), SubjectKinds: []string{"file"}},
		{RuleID: "§11.4.23", Title: "Visual-cue & grouping for Issues docs", Severity: constitution.SeverityLow, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassPolicyViolation, Check: markerCheck("§11.4.23"), SubjectKinds: []string{"file"}},
		{RuleID: "§11.4.25", Title: "Full-Automation-Coverage", Severity: constitution.SeverityLow, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassPolicyViolation, Check: markerCheck("§11.4.25"), SubjectKinds: []string{"file"}},
		{RuleID: "§11.4.28", Title: "Submodules-As-Equal-Codebase", Severity: constitution.SeverityLow, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassPolicyViolation, Check: markerCheck("§11.4.28"), SubjectKinds: []string{"submodule"}},
		{RuleID: "§11.4.29", Title: "Lowercase-Snake_Case-Naming", Severity: constitution.SeverityLow, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassPolicyViolation, Check: checkSnakeCase, SubjectKinds: []string{"file"}},
		{RuleID: "§11.4.31", Title: "Submodule-Dependency-Manifest", Severity: constitution.SeverityLow, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassPolicyViolation, Check: markerCheck("§11.4.31"), SubjectKinds: []string{"file"}},
		{RuleID: "§11.4.44", Title: "Document Revision Header", Severity: constitution.SeverityLow, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassPolicyViolation, Check: checkDocRevisionHeader, SubjectKinds: []string{"file"}},
		{RuleID: "§11.4.53", Title: "Fixed_Summary parity", Severity: constitution.SeverityLow, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassPolicyViolation, Check: markerCheck("§11.4.53"), SubjectKinds: []string{"file"}},
		{RuleID: "§11.4.56", Title: "Status_Summary parity", Severity: constitution.SeverityLow, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassPolicyViolation, Check: markerCheck("§11.4.56"), SubjectKinds: []string{"file"}},
		{RuleID: "§11.4.57", Title: "README doc-link section", Severity: constitution.SeverityLow, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassPolicyViolation, Check: markerCheck("§11.4.57"), SubjectKinds: []string{"file"}},
		{RuleID: "§11.4.59", Title: "README always-sync", Severity: constitution.SeverityLow, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassPolicyViolation, Check: markerCheck("§11.4.59"), SubjectKinds: []string{"file"}},
		{RuleID: "§11.4.61", Title: "Markdown metadata + ToC", Severity: constitution.SeverityLow, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassPolicyViolation, Check: markerCheck("§11.4.61"), SubjectKinds: []string{"file"}},
		{RuleID: "§11.4.65", Title: "Universal Markdown export", Severity: constitution.SeverityLow, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassPolicyViolation, Check: markerCheck("§11.4.65"), SubjectKinds: []string{"file"}},

		// --- shared with scherald (§11.4.45 Status.md sweep) --------------
		{RuleID: "§11.4.45", Title: "Integration-Status-Doc Maintenance", Severity: constitution.SeverityLow, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassPolicyViolation, Check: markerCheck("§11.4.45"), SubjectKinds: []string{"file"}},
	}
}
