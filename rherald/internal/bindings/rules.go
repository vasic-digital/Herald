package bindings

// rules.go — the rherald-owned §42.3 release gate constitution-rule catalogue
// (HRD-022).
//
// Per spec §42.3 master binding table, rherald owns (sole or shared) the four
// release-lifecycle rows enumerated here. rherald is the RELEASE flavor; it
// carries the tag-mirroring / changelog / installable-asset / pre-tag-retest
// bindings. The §5 changelog row is SHARED with cherald — rherald registers its
// own binding here (the conformance/export-staleness facet); cherald registers
// its own §5 binding in its own flavor package.
//
// Four bespoke detection hooks are the load-bearing vertical slice (the task's
// core deliverable). Each carries a REAL, deterministic, PURE classifier:
//
//   - TAG-MIRROR-PARITY (§4): reads a recorded tag-mirror tally
//     ("tag=present|mirrors=N|with_tag=M") and FAILs when the parent tag is
//     absent OR any owned mirror is missing the tag (M < N). Routes through
//     .release.gate.blocked.
//   - CHANGELOG-CONFORMANCE (§5): reads the recorded conformance flag
//     ("conforming=<bool>[|export_stale=<bool>]") and FAILs when the changelog
//     is non-conforming (not Conventional-Commits-derived) OR its multi-format
//     export is stale. Routes through .policy.violation (shared with cherald).
//   - PRE-TAG-RETEST-GATE (§11.4.40 — CRITICAL): reads the recorded retest
//     outcome ("retest=<green|red|skipped>[|tiers=N]") and FAILs when the
//     full-suite retest was skipped, RED, or did not cover all 8 §40.2 tiers.
//     This is the highest-severity release gate — a tag attempted without a
//     green all-tier retest. Routes through .release.gate.blocked.
//   - INSTALLABLE-ASSET-EVIDENCE (§11.4.38): reads the recorded install-check
//     outcome ("installed=<bool>") and FAILs when a release asset fails its
//     installability check. Routes through .release.gate.blocked.
//
// PURE detectors (CRITICAL §12 host-safety). Every CheckFunc CLASSIFIES a
// Subject string description of an already-observed release-op outcome. NONE
// tags, pushes, force-pushes, runs git, downloads an asset, or touches the
// filesystem. The §43 command bodies (HRD-031 tag-mirror, HRD-032 changelog,
// HRD-045 gate-retest) supply the live release integration upstream — the live
// release-op interception is scope-locked to those §43 follow-ups.

import (
	"context"
	"crypto/sha256"
	"strconv"
	"strings"

	constitution "github.com/vasic-digital/herald/commons_constitution"
)

// digest produces a stable DigestSHA from the decision + evidence so the
// transition gate can detect rationale changes.
func digest(d constitution.Decision, evidence string) [32]byte {
	return sha256.Sum256([]byte(d.String() + ":" + evidence))
}

func pass(evidence string) constitution.Result {
	return constitution.Result{Decision: constitution.DecisionPass, Evidence: evidence, DigestSHA: digest(constitution.DecisionPass, evidence)}
}
func fail(evidence string) constitution.Result {
	return constitution.Result{Decision: constitution.DecisionFail, Evidence: evidence, DigestSHA: digest(constitution.DecisionFail, evidence)}
}

// subjectFields parses a Subject.ID of the form "<head>|k=v|k2=v2..." into the
// head segment + a key→value map. A first segment WITHOUT "=" is the head (the
// tag / version / asset description). PURE string parsing only — never touches
// the filesystem, network, process table, or git.
func subjectFields(id string) (head string, kv map[string]string) {
	kv = map[string]string{}
	for i, seg := range strings.Split(id, "|") {
		if eq := strings.IndexByte(seg, '='); eq >= 0 {
			kv[strings.TrimSpace(seg[:eq])] = strings.TrimSpace(seg[eq+1:])
			continue
		}
		if i == 0 {
			head = seg
		}
	}
	return head, kv
}

// boolField reads a "true"/"false" field; missing or unparseable → false (the
// safe default — an unproven prerequisite is treated as NOT satisfied).
func boolField(kv map[string]string, key string) bool {
	v, ok := kv[key]
	if !ok {
		return false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false
	}
	return b
}

// intField reads an integer field; missing or unparseable → (0, false).
func intField(kv map[string]string, key string) (int, bool) {
	v, ok := kv[key]
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}

// canonicalTierCount is the §40.2 8-tier test matrix every full-suite retest
// (§11.4.40) MUST cover before a release tag is allowed.
const canonicalTierCount = 8

// ---------------------------------------------------------------------------
// Bespoke detection hooks — the HRD-022 vertical-slice core. All PURE.
// ---------------------------------------------------------------------------

// checkTagMirrorParity implements §4 (tag mirroring). It CLASSIFIES a recorded
// tag-mirror tally:
//
//	tag=absent                         → FAIL (the parent tag itself is missing)
//	with_tag < mirrors                 → FAIL (a §4 violation — tag missing on an owned mirror)
//	with_tag == mirrors (mirrors >= 1) → PASS (full parity)
//	(missing/unparseable fields)       → FAIL (refuse to silent-PASS — §11.4.1 inverse)
//
// PURE: it reads the recorded tally; it NEVER tags, pushes, or runs git.
func checkTagMirrorParity(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	tag, kv := subjectFields(s.ID)
	if tag == "" {
		tag = "tag"
	}
	if strings.ToLower(kv["tag"]) == "absent" {
		return fail("§4: release tag " + tag + " is ABSENT on the parent (nothing to mirror)"), nil
	}
	mirrors, okM := intField(kv, "mirrors")
	withTag, okW := intField(kv, "with_tag")
	if !okM || !okW {
		return fail("§4: tag " + tag + " has no mirror-parity tally (refusing to silent-PASS)"), nil
	}
	if mirrors < 1 {
		return fail("§4: tag " + tag + " reports zero owned mirrors (mis-recorded parity)"), nil
	}
	if withTag < mirrors {
		return fail("§4: tag " + tag + " mirror-parity MISS: " + strconv.Itoa(withTag) + "/" + strconv.Itoa(mirrors) + " owned mirrors carry the tag"), nil
	}
	return pass("§4: tag " + tag + " present on all " + strconv.Itoa(mirrors) + " owned mirrors"), nil
}

// checkChangelogConformance implements §5 (changelog + multi-format export). It
// CLASSIFIES a recorded changelog conformance state:
//
//	conforming=false        → FAIL (not Conventional-Commits-derived)
//	export_stale=true       → FAIL (the .html/.pdf/.docx export is stale)
//	conforming=true (no stale export) → PASS
//	(missing conforming field) → FAIL (refuse to silent-PASS)
//
// PURE: it reads the recorded flags; it NEVER regenerates the changelog or runs
// git-log. Routes through .policy.violation (shared with cherald).
func checkChangelogConformance(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	ver, kv := subjectFields(s.ID)
	if ver == "" {
		ver = "version"
	}
	if _, ok := kv["conforming"]; !ok {
		return fail("§5: changelog for " + ver + " has no conformance state (refusing to silent-PASS)"), nil
	}
	if !boolField(kv, "conforming") {
		return fail("§5: changelog for " + ver + " is NON-CONFORMING (not derived from Conventional Commits)"), nil
	}
	if boolField(kv, "export_stale") {
		return fail("§5: changelog for " + ver + " has a STALE multi-format export (.html/.pdf/.docx out of sync per §36)"), nil
	}
	return pass("§5: changelog for " + ver + " is conforming with fresh multi-format export"), nil
}

// checkPreTagRetest implements §11.4.40 (full-suite retest before release tag) —
// THE critical-severity release gate. It CLASSIFIES a recorded retest outcome:
//
//	retest=skipped / missing → FAIL (tag attempted WITHOUT a full retest)
//	retest=red               → FAIL (the retest ran but FAILed)
//	retest=green but tiers<8  → FAIL (incomplete §40.2 tier coverage)
//	retest=green and tiers>=8 → PASS (full retest, all tiers green)
//
// PURE: it reads the recorded retest outcome; it NEVER tags or re-runs the suite.
func checkPreTagRetest(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	tag, kv := subjectFields(s.ID)
	if tag == "" {
		tag = "tag"
	}
	switch strings.ToLower(kv["retest"]) {
	case "green", "pass", "passed", "ok":
		tiers, ok := intField(kv, "tiers")
		if !ok {
			return fail("§11.4.40: retest for " + tag + " reports green but no tier count (cannot confirm §40.2 8-tier coverage)"), nil
		}
		if tiers < canonicalTierCount {
			return fail("§11.4.40: retest for " + tag + " green but only " + strconv.Itoa(tiers) + "/" + strconv.Itoa(canonicalTierCount) + " §40.2 tiers covered"), nil
		}
		return pass("§11.4.40: full-suite retest for " + tag + " green across all " + strconv.Itoa(canonicalTierCount) + " §40.2 tiers"), nil
	case "red", "fail", "failed":
		return fail("§11.4.40: pre-tag retest for " + tag + " is RED — refusing to allow the tag"), nil
	case "skipped", "skip", "none", "":
		return fail("§11.4.40: tag " + tag + " attempted WITHOUT the mandatory full-suite retest (release gate BLOCKED)"), nil
	default:
		return fail("§11.4.40: tag " + tag + " has an unrecognized retest outcome (refusing to silent-PASS)"), nil
	}
}

// checkInstallableAsset implements §11.4.38 (installable-asset evidence). It
// CLASSIFIES a recorded install-check outcome:
//
//	installed=true  → PASS
//	installed=false → FAIL (the release asset fails its installability check)
//	(missing field) → FAIL (no install evidence — refuse to silent-PASS)
//
// PURE: it reads the recorded install-check outcome; it NEVER downloads or
// installs the asset.
func checkInstallableAsset(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	asset, kv := subjectFields(s.ID)
	if asset == "" {
		asset = "asset"
	}
	if _, ok := kv["installed"]; !ok {
		return fail("§11.4.38: release asset " + asset + " has NO recorded install-check evidence (refusing to silent-PASS)"), nil
	}
	if !boolField(kv, "installed") {
		return fail("§11.4.38: release asset " + asset + " FAILED its installability check"), nil
	}
	return pass("§11.4.38: release asset " + asset + " passed its installability check"), nil
}

// ---------------------------------------------------------------------------
// The rherald-owned §42.3 release catalogue.
// ---------------------------------------------------------------------------

// RheraldRules returns the catalogue of every rherald-owned (sole or shared)
// §42.3 release gate rule, with its default severity + mode + event class +
// ReleaseGate name + a real PURE CheckFunc. The slice is freshly built on each
// call (callers may mutate their copy). Order is deterministic (source order =
// roughly section order).
func RheraldRules() []RuleSpec {
	return []RuleSpec{
		// §4 Tag mirroring — enforce / high — .release.gate.blocked.
		{RuleID: "§4", Title: "Tag mirroring", Severity: constitution.SeverityHigh, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassReleaseGateBlocked, ReleaseGate: "tag-mirror", Check: checkTagMirrorParity, SubjectKinds: []string{SubjectTagMirror}},

		// §5 Changelog + multi-format export — warn / middle — .policy.violation
		// (SHARED with cherald; rherald owns the conformance/export-staleness facet).
		{RuleID: "§5", Title: "Changelog + multi-format export", Severity: constitution.SeverityMiddle, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassPolicyViolation, ReleaseGate: "changelog", Check: checkChangelogConformance, SubjectKinds: []string{SubjectChangelog}},

		// §11.4.38 Installable-Asset Evidence — enforce / high — .release.gate.blocked.
		{RuleID: "§11.4.38", Title: "Installable-Asset Evidence", Severity: constitution.SeverityHigh, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassReleaseGateBlocked, ReleaseGate: "install-asset", Check: checkInstallableAsset, SubjectKinds: []string{SubjectInstallAsset}},

		// §11.4.40 Full-suite retest before release tag — enforce / CRITICAL —
		// .release.gate.blocked. The highest-severity release gate.
		{RuleID: "§11.4.40", Title: "Full-suite retest before release tag", Severity: constitution.SeverityCritical, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassReleaseGateBlocked, ReleaseGate: "pre-tag-retest", Check: checkPreTagRetest, SubjectKinds: []string{SubjectRetestGate}},
	}
}
