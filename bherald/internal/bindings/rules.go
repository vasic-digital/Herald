package bindings

// rules.go — the bherald-owned §42.3 CI/test gate-result constitution-rule
// catalogue (HRD-021).
//
// Per spec §42.3 master binding table, bherald owns (sole or shared) the 22
// CI/test rows enumerated here. bherald is the BUILD / CI flavor; it carries
// the gate-result / recorded-evidence / test-tier / determinism / fakes-ban
// bindings. Rows where bherald is a SHARED owner (§1.1 with cherald, §11.4.24
// with sherald, §11.4.30 with cherald, §11.4.43 with cherald) still register
// here — the OTHER owner registers its own binding in its own flavor package.
//
// Three bespoke detection hooks are the load-bearing vertical slice (the task's
// core deliverable). Each carries a REAL, deterministic, PURE classifier:
//
//   - GATE-RESULT classification (§1 / §11.4.50): reads a recorded CI gate
//     outcome (pass/fail/flaky/error) and returns the verdict. A "flaky" gate
//     is a §11.4.50 determinism FAIL. Routes through .gate.failed/.gate.recovered.
//   - TEST-TIER-VERIFY (§11.4.27 / §40.2): reads the present test tiers and
//     FAILs when any of the 8 canonical tiers (unit/component/integration/
//     contract/e2e_sandbox/e2e_live/mutation/chaos) is missing.
//   - ANTI-BLUFF-PASS detection (§11.4.2 / §11.4.5): a gate that reports
//     outcome=pass but has NO captured-evidence artefact (evidence=false) is a
//     §11.4 PASS-bluff and is FAILed. This is the constitutional enforcement of
//     the §107 end-user-quality covenant at the CI layer.
//
// The remaining rows (§11.4.3/.4/.7/.13/.39/.43/.46/.48/.49/.52/.67 gate rows;
// §11.4.9/.14 policy rows; §11.4.24 build-stats; §11.4.30 build-artifact) use
// outcome-marker detectors of the same PURE shape: the verdict is encoded in
// the Subject.ID by the CI gate / §43 command body that already observed the
// outcome, and the binding records the verdict + routes the emit through the
// correct class. The framework + registration + emit-routing are complete for
// ALL rows; deepening any remaining detector with bespoke parsing is per-rule
// follow-up.
//
// PURE detectors (CRITICAL §12 host-safety). Every CheckFunc CLASSIFIES a
// Subject string description of an already-observed CI/test outcome. NONE runs
// the build, re-executes the test suite, spawns a process, or touches the
// filesystem. The §43 command bodies supply the live CI integration upstream.

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
// gate-name / test-id / pkg description). PURE string parsing only — never
// touches the filesystem, network, process table, or test runner.
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

// ---------------------------------------------------------------------------
// Bespoke detection hooks — the HRD-021 vertical-slice core. All PURE.
// ---------------------------------------------------------------------------

// checkGateResult implements §1 (test coverage mandatory) + §11.4.50
// (deterministic consistency). It CLASSIFIES a recorded CI gate outcome:
//
//	outcome=pass  → PASS
//	outcome=fail  → FAIL (the gate failed)
//	outcome=flaky → FAIL (non-deterministic — §11.4.50)
//	outcome=error → FAIL (the gate errored before producing a verdict)
//	(missing / unknown outcome) → FAIL (a gate with no recorded outcome MUST
//	  NOT silently PASS — that would be the inverse §11.4.1 fail-bluff)
//
// PURE: it reads the recorded outcome; it NEVER re-runs the gate.
func checkGateResult(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	gate, kv := subjectFields(s.ID)
	if gate == "" {
		gate = "gate"
	}
	switch strings.ToLower(kv["outcome"]) {
	case "pass", "passed", "ok", "green":
		return pass("§1/§11.4.50: gate " + gate + " outcome=pass"), nil
	case "fail", "failed", "red":
		return fail("§1: gate " + gate + " FAILED"), nil
	case "flaky", "flake":
		return fail("§11.4.50: gate " + gate + " is FLAKY (non-deterministic — same input, different result)"), nil
	case "error", "errored":
		return fail("§1: gate " + gate + " ERRORED before producing a verdict"), nil
	default:
		return fail("§11.4.1-inverse: gate " + gate + " has no recognizable outcome field (refusing to silent-PASS)"), nil
	}
}

// canonicalTiers is the §40.2 8-tier test matrix every shipped Herald package
// MUST carry (per Universal §11.4.27 no-fakes + §11.4.39 on-device validation).
var canonicalTiers = []string{
	"unit", "component", "integration", "contract",
	"e2e_sandbox", "e2e_live", "mutation", "chaos",
}

// CanonicalTiers returns a fresh copy of the §40.2 8-tier canonical matrix the
// checkTestTierVerify detector enforces. The HRD-041 test-tier-verify §43 command
// body uses it to build the full-matrix Subject (--all-tiers) and to order the
// observed tiers deterministically — so the detector and the command body cite
// the SAME source of truth (no drift). A fresh slice is returned so callers
// cannot mutate the package-level matrix.
func CanonicalTiers() []string {
	out := make([]string, len(canonicalTiers))
	copy(out, canonicalTiers)
	return out
}

// checkTestTierVerify implements §11.4.27 (no-fakes + 100% test-type coverage)
// via the §40.2 8-tier matrix. It reads the present tiers from the Subject
// ("tiers=unit,component,...") and FAILs when ANY canonical tier is missing.
// PURE: it compares the recorded tier list; it NEVER runs the suites.
func checkTestTierVerify(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	pkg, kv := subjectFields(s.ID)
	if pkg == "" {
		pkg = "pkg"
	}
	present := map[string]bool{}
	for _, t := range strings.Split(kv["tiers"], ",") {
		t = strings.TrimSpace(strings.ToLower(t))
		if t != "" {
			present[t] = true
		}
	}
	var missing []string
	for _, want := range canonicalTiers {
		if !present[want] {
			missing = append(missing, want)
		}
	}
	if len(missing) > 0 {
		return fail("§11.4.27/§40.2: " + pkg + " missing test tiers: " + strings.Join(missing, ",")), nil
	}
	return pass("§11.4.27/§40.2: " + pkg + " carries all 8 canonical test tiers"), nil
}

// checkAntiBluffPASS implements §11.4.2 (recorded-evidence requirement) +
// §11.4.5 (captured-evidence quality). THE §107 covenant detector at the CI
// layer: a gate/test that reports outcome=pass but has NO captured-evidence
// artefact (evidence=false) is a §11.4 PASS-bluff and MUST be FAILed — it
// claims to work but has no auditable runtime evidence. A genuine PASS carries
// evidence=true. A reported FAIL is honest (no bluff) and passes the
// anti-bluff check itself (the gate correctly reported failure). PURE: it reads
// the recorded outcome + evidence flag; it NEVER inspects the artefact bytes.
func checkAntiBluffPASS(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	testID, kv := subjectFields(s.ID)
	if testID == "" {
		testID = "test"
	}
	outcome := strings.ToLower(kv["outcome"])
	hasEvidence := boolField(kv, "evidence")
	switch outcome {
	case "pass", "passed", "ok", "green":
		if !hasEvidence {
			return fail("§11.4.2/§11.4.5 PASS-BLUFF: " + testID + " reports PASS with NO captured-evidence artefact (metadata-only PASS is a §11.4 bluff)"), nil
		}
		return pass("§11.4.2: " + testID + " PASS with captured evidence"), nil
	case "":
		return fail("§11.4.2: " + testID + " has no recorded outcome (cannot verify evidence)"), nil
	default:
		// An honest FAIL/error is not a bluff — the gate correctly reported it.
		return pass("§11.4.2: " + testID + " honestly reported outcome=" + outcome + " (not a PASS-bluff)"), nil
	}
}

// ---------------------------------------------------------------------------
// Remaining bherald rows — PURE outcome-marker detectors.
// ---------------------------------------------------------------------------

// outcomeMarker is the shared mechanism for gate rows whose bespoke detector
// lives upstream (the CI gate / §43 command observed the outcome): the Subject
// carries "outcome=<pass|fail>". A "fail" → FAIL, a "pass" → PASS, anything
// else → FAIL (refuse to silent-PASS — §11.4.1 inverse). ruleID is closed over
// for the evidence string. Used for the gate rows that route through
// .gate.failed / .gate.recovered but whose detection is the CI gate's job.
func outcomeMarker(ruleID string) CheckFunc {
	return func(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
		head, kv := subjectFields(s.ID)
		switch strings.ToLower(kv["outcome"]) {
		case "pass", "passed", "ok", "green":
			return pass(ruleID + ": " + head + " outcome=pass"), nil
		case "fail", "failed", "red", "error", "errored":
			return fail(ruleID + ": " + head + " gate FAILED"), nil
		default:
			return fail(ruleID + ": " + head + " has no recognizable outcome (refusing to silent-PASS)"), nil
		}
	}
}

// violationMarker is the shared mechanism for the hygiene / policy rows whose
// bespoke detector lives upstream: the Subject.Kind carries the already-decided
// verdict. "violation" → FAIL, anything else → PASS. Used for the
// .policy.violation rows (§11.4.9 batch-source-fixes, §11.4.14 playback-cleanup)
// + the §11.4.30 build-artifact row.
func violationMarker(ruleID string) CheckFunc {
	return func(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
		if s.Kind == "violation" {
			return fail(ruleID + " violation on " + s.ID), nil
		}
		return pass(ruleID + " ok on " + s.ID), nil
	}
}

// ---------------------------------------------------------------------------
// The bherald-owned §42.3 CI/test catalogue.
// ---------------------------------------------------------------------------

// BheraldRules returns the catalogue of every bherald-owned (sole or shared)
// §42.3 CI/test gate-result rule, with its default severity + mode + event
// class + GateName + a real PURE CheckFunc. The slice is freshly built on each
// call (callers may mutate their copy). Order is deterministic (source order ≈
// section order).
func BheraldRules() []RuleSpec {
	return []RuleSpec{
		// --- enforce / gate-result core (the bherald build/CI value-add) ------
		{RuleID: "§1", Title: "Test coverage mandatory", Severity: constitution.SeverityHigh, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassGateFailed, GateName: "test-coverage", Check: checkGateResult, SubjectKinds: []string{SubjectGateResult}},
		{RuleID: "§1.1", Title: "False-positive immunity (paired mutation)", Severity: constitution.SeverityCritical, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassGateFailed, GateName: "paired-mutation", Check: checkGateResult, SubjectKinds: []string{SubjectGateResult}},
		{RuleID: "§11.4.2", Title: "Recorded-evidence requirement", Severity: constitution.SeverityHigh, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassGateFailed, GateName: "evidence-capture", Check: checkAntiBluffPASS, SubjectKinds: []string{SubjectEvidence}},
		{RuleID: "§11.4.5", Title: "Captured-evidence quality", Severity: constitution.SeverityMiddle, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassGateFailed, GateName: "evidence-quality", Check: checkAntiBluffPASS, SubjectKinds: []string{SubjectEvidence}},
		{RuleID: "§11.4.27", Title: "No-Fakes-Beyond-Unit-Tests + test-tier matrix", Severity: constitution.SeverityHigh, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassGateFailed, GateName: "test-tier", Check: checkTestTierVerify, SubjectKinds: []string{SubjectTestTier}},
		{RuleID: "§11.4.50", Title: "Deterministic Consistency", Severity: constitution.SeverityHigh, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassGateFailed, GateName: "determinism", Check: checkGateResult, SubjectKinds: []string{SubjectGateResult}},

		// --- enforce / high: the remaining enforce gate rows ------------------
		{RuleID: "§11.4.39", Title: "Per-Feature On-Device Validation", Severity: constitution.SeverityHigh, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassGateFailed, GateName: "on-device-validation", Check: outcomeMarker("§11.4.39"), SubjectKinds: []string{SubjectGateResult}},
		{RuleID: "§11.4.43", Title: "TDD-Fix-Discipline", Severity: constitution.SeverityHigh, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassGateFailed, GateName: "tdd-fix", Check: outcomeMarker("§11.4.43"), SubjectKinds: []string{SubjectGateResult}},

		// --- enforce / high: shared §11.4.30 build-artifact (with cherald) ----
		{RuleID: "§11.4.30", Title: "No-Versioned-Build-Artifacts", Severity: constitution.SeverityHigh, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassRepoSafetyBreach, GateName: "build-artifact", Check: violationMarker("§11.4.30"), SubjectKinds: []string{"file"}},

		// --- warn / middle: the warn gate rows --------------------------------
		{RuleID: "§11.4.3", Title: "Per-environment-topology dispatch", Severity: constitution.SeverityMiddle, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassGateFailed, GateName: "env-topology", Check: outcomeMarker("§11.4.3"), SubjectKinds: []string{SubjectGateResult}},
		{RuleID: "§11.4.4", Title: "Test-interrupt-on-discovery", Severity: constitution.SeverityMiddle, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassGateFailed, GateName: "interrupt-on-discovery", Check: outcomeMarker("§11.4.4"), SubjectKinds: []string{SubjectGateResult}},
		{RuleID: "§11.4.7", Title: "Demotion-evidence", Severity: constitution.SeverityMiddle, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassGateFailed, GateName: "demotion-evidence", Check: outcomeMarker("§11.4.7"), SubjectKinds: []string{SubjectGateResult}},
		{RuleID: "§11.4.13", Title: "Out-of-band sink-side evidence", Severity: constitution.SeverityMiddle, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassGateFailed, GateName: "sink-side-evidence", Check: outcomeMarker("§11.4.13"), SubjectKinds: []string{SubjectGateResult}},
		{RuleID: "§11.4.24", Title: "Build-resource stats tracking", Severity: constitution.SeverityMiddle, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassGateFailed, GateName: "build-stats", Check: outcomeMarker("§11.4.24"), SubjectKinds: []string{SubjectGateResult}},
		{RuleID: "§11.4.46", Title: "Validate-recent-work-before-post-flash", Severity: constitution.SeverityMiddle, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassGateFailed, GateName: "post-flash-validate", Check: outcomeMarker("§11.4.46"), SubjectKinds: []string{SubjectGateResult}},
		{RuleID: "§11.4.48", Title: "UI-Driven Video Testing", Severity: constitution.SeverityMiddle, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassGateFailed, GateName: "ui-video", Check: outcomeMarker("§11.4.48"), SubjectKinds: []string{SubjectGateResult}},
		{RuleID: "§11.4.49", Title: "Dual-Approach Testing", Severity: constitution.SeverityMiddle, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassGateFailed, GateName: "dual-approach", Check: outcomeMarker("§11.4.49"), SubjectKinds: []string{SubjectGateResult}},
		{RuleID: "§11.4.52", Title: "Autonomous-Validation", Severity: constitution.SeverityMiddle, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassGateFailed, GateName: "autonomous-validation", Check: outcomeMarker("§11.4.52"), SubjectKinds: []string{SubjectGateResult}},

		// --- warn / low: shell-script parseability gate -----------------------
		{RuleID: "§11.4.67", Title: "Shell-script parseability", Severity: constitution.SeverityLow, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassGateFailed, GateName: "shell-parse", Check: outcomeMarker("§11.4.67"), SubjectKinds: []string{SubjectGateResult}},

		// --- warn / low: the .policy.violation hygiene rows -------------------
		{RuleID: "§11.4.9", Title: "Batch-source-fixes-before-rebuild", Severity: constitution.SeverityLow, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassPolicyViolation, Check: violationMarker("§11.4.9"), SubjectKinds: []string{"file"}},
		{RuleID: "§11.4.14", Title: "Test playback cleanup", Severity: constitution.SeverityLow, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassPolicyViolation, Check: violationMarker("§11.4.14"), SubjectKinds: []string{"file"}},
	}
}
