package bindings

// rules.go — the sherald-owned §42.3 host-safety + repo-safety constitution
// rule catalogue (HRD-020).
//
// Per spec §42.3 master binding table, sherald owns (sole or shared) the
// host/repo-safety rows enumerated here. sherald is the SAFETY flavor; it
// carries the destructive-op / force-push / mem-budget / host-op / container /
// bundle-validation bindings. Rows where sherald is a SHARED owner (e.g. §9.2 /
// §11.4.41 / §11.4.71 with pherald, §11.4.24 with bherald, §12.3 with dherald)
// still register here — the OTHER owner registers its own binding in its own
// flavor package.
//
// Three named detection hooks are the load-bearing vertical slice (the task's
// core deliverable): the DESTRUCTIVE-OP detector (§9.1), the FORCE-PUSH
// interceptor (§11.4.41, shared by §9.2), and the MEM-BUDGET watcher (§12.6).
// Each carries a REAL, deterministic, PURE detector:
//
//   - it CLASSIFIES a Subject whose ID encodes the already-observed op
//     parameters (e.g. "git reset --hard|backup=false");
//   - it returns FAIL when the §-rule prerequisite is unmet, PASS otherwise;
//   - it NEVER executes the op, force-pushes, suspends the host, or allocates
//     memory (§12 / §12.6 host-safety — the detector GUARDS, it does not act).
//
// The remaining rows (§9.3 backup, §12.1/.2/.3 host ops, §11.4.32/.36/.71/.26,
// §2 commit, §11.4.24 build-stats, §11.4.47 firebase) use parameter-marker
// detectors of the same PURE shape: the boolean prerequisite is encoded in the
// Subject.ID by the §43 command body / sweep that already observed the op, and
// the binding records the verdict + routes the emit through the correct safety
// class. The framework + registration + emit-routing are complete for ALL rows;
// deepening any detector with bespoke parsing is per-rule follow-up.

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

// subjectFields parses a Subject.ID of the form "<head>|k=v|k2=v2..." (or a
// bare "k=v" with no leading head) into the head segment + a key→value map.
// EVERY "|"-separated segment that contains an "=" is parsed as a k=v pair —
// including the first segment, so a bare "used_fraction=0.40" populates kv
// (and leaves head empty). A first segment WITHOUT "=" is treated as the head
// (the op/ref/path description). PURE string parsing only — never touches the
// filesystem, network, process table, or memory allocator.
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
// safe default — an unproven prerequisite is treated as NOT satisfied, so the
// guard FAILs rather than silently allowing a dangerous op).
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
// Named detection hooks — the HRD-020 vertical-slice core. All PURE detectors.
// ---------------------------------------------------------------------------

// checkDestructiveOp implements §9.1 (destructive-op protocol). FAILs a
// destructive op (rm -rf / git reset --hard / git clean) attempted WITHOUT a
// preceding hardlinked backup (backup=true). The detector reads the recorded
// `backup` prerequisite from the Subject; it NEVER runs the op.
func checkDestructiveOp(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	op, kv := subjectFields(s.ID)
	if op == "" {
		return fail("destructive-op §9.1: empty op description (cannot verify backup prerequisite)"), nil
	}
	if boolField(kv, "backup") {
		return pass("destructive-op §9.1: backup recorded before " + op), nil
	}
	return fail("destructive-op §9.1: " + op + " attempted WITHOUT a preceding hardlinked backup"), nil
}

// checkForcePush implements §11.4.41 (pre-force-push merge-first) + §9.2
// (force-push authorization). FAILs a force-push attempted WITHOUT a preceding
// merge (merged=true) OR WITHOUT explicit per-session authorization
// (authorized=true). The interceptor classifies the attempt; it NEVER pushes.
func checkForcePush(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	ref, kv := subjectFields(s.ID)
	merged := boolField(kv, "merged")
	authorized := boolField(kv, "authorized")
	switch {
	case !merged && !authorized:
		return fail("force-push §11.4.41/§9.2: " + ref + " attempted without merge-first AND without session authorization"), nil
	case !merged:
		return fail("force-push §11.4.41: " + ref + " attempted without a preceding merge (merge-first violated)"), nil
	case !authorized:
		return fail("force-push §9.2: " + ref + " attempted without explicit per-session authorization"), nil
	}
	return pass("force-push §11.4.41/§9.2: " + ref + " merge-first + authorized"), nil
}

// memBudgetCeiling is the §12.6 60% used-fraction ceiling.
const memBudgetCeiling = 0.60

// checkMemBudget implements §12.6 (Memory-Budget 60% ceiling). FAILs when the
// REPORTED used-fraction exceeds 60%. CRITICAL §12.6 host-safety: the detector
// reads a fraction the sampler ALREADY measured — it NEVER allocates memory to
// reproduce the condition.
func checkMemBudget(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	_, kv := subjectFields(s.ID)
	raw, ok := kv["used_fraction"]
	if !ok {
		return fail("mem-budget §12.6: no used_fraction reading supplied"), nil
	}
	frac, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fail("mem-budget §12.6: unparseable used_fraction " + raw), nil
	}
	if frac > memBudgetCeiling {
		return fail("mem-budget §12.6: used_fraction=" + raw + " exceeds the 60% ceiling"), nil
	}
	return pass("mem-budget §12.6: used_fraction=" + raw + " within the 60% ceiling"), nil
}

// ---------------------------------------------------------------------------
// Remaining sherald rows — PURE parameter-marker detectors.
// ---------------------------------------------------------------------------

// prereqCheck builds a PURE detector that FAILs when the named boolean
// prerequisite field is unmet. failMsg / passMsg are closed over for evidence.
// Used for the host-op / backup / bundle / upstream / push / commit rows whose
// prerequisite is a single boolean already observed upstream.
func prereqCheck(ruleID, field, failMsg, passMsg string) CheckFunc {
	return func(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
		head, kv := subjectFields(s.ID)
		if boolField(kv, field) {
			return pass(ruleID + ": " + passMsg + " (" + head + ")"), nil
		}
		return fail(ruleID + ": " + failMsg + " (" + head + ")"), nil
	}
}

// forbiddenHostOp implements §12.1 (forbidden host-session operations). The
// PRESENCE of a forbidden host command (suspend/hibernate/logout/shutdown/
// reboot/systemctl) in the Subject head is the violation. PURE: it pattern-
// matches the recorded command string; it NEVER executes it.
func forbiddenHostOp(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	head, _ := subjectFields(s.ID)
	lower := strings.ToLower(head)
	for _, forbidden := range []string{"suspend", "hibernate", "logout", "shutdown", "reboot", "systemctl", "pmset sleepnow", "halt"} {
		if strings.Contains(lower, forbidden) {
			return fail("host-op §12.1: forbidden host-session operation attempted — " + head), nil
		}
	}
	return pass("host-op §12.1: no forbidden host-session operation in " + head), nil
}

// ---------------------------------------------------------------------------
// The sherald-owned §42.3 safety catalogue.
// ---------------------------------------------------------------------------

// SheraldRules returns the catalogue of every sherald-owned (sole or shared)
// §42.3 host/repo-safety constitution rule, with its default severity + mode +
// event class + breach kind + a real PURE CheckFunc. Freshly built per call.
// Order is deterministic (source order ≈ section order).
func SheraldRules() []RuleSpec {
	return []RuleSpec{
		// --- §9 codebase-safety (repo.safety.breach + gate.recovered) -------
		{RuleID: "§2", Title: "Commit + push mechanics", Severity: constitution.SeverityHigh, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassRepoSafetyBreach, BreachKind: "commit-mechanics", Check: prereqCheck("§2", "complete", "commit/push via wrong entrypoint OR partial fan-out", "commit/push via canonical entrypoint, full fan-out"), SubjectKinds: []string{SubjectCommit}},
		{RuleID: "§9.1", Title: "Destructive-op protocol", Severity: constitution.SeverityCritical, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassRepoSafetyBreach, BreachKind: "destructive-op", Check: checkDestructiveOp, SubjectKinds: []string{SubjectDestructiveOp}},
		{RuleID: "§9.2", Title: "Force-push authorization", Severity: constitution.SeverityCritical, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassRepoSafetyBreach, BreachKind: "force-push", Check: checkForcePush, SubjectKinds: []string{SubjectForcePush}},
		{RuleID: "§9.3", Title: "Hardlinked backup before destructive", Severity: constitution.SeverityMiddle, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassGateRecovered, BreachKind: "backup", Check: prereqCheck("§9.3", "created", "hardlinked backup NOT created before destructive op", "hardlinked backup created (audit trail)"), SubjectKinds: []string{SubjectBackup}},

		// --- §11.4 repo-safety + bundle workflow ----------------------------
		{RuleID: "§11.4.24", Title: "Build-resource stats tracking", Severity: constitution.SeverityMiddle, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassGateFailed, BreachKind: "build-stats", Check: prereqCheck("§11.4.24", "stats", "build resource stats missing", "build resource stats recorded"), SubjectKinds: []string{SubjectBuildStats}},
		{RuleID: "§11.4.26", Title: "Constitution-submodule update workflow", Severity: constitution.SeverityMiddle, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassBundleUpdated, BreachKind: "constitution-pull", Check: prereqCheck("§11.4.26", "ok", "constitution pull workflow incomplete", "constitution pulled successfully"), SubjectKinds: []string{SubjectConstitutionPull}},
		{RuleID: "§11.4.32", Title: "Post-constitution-pull validation", Severity: constitution.SeverityHigh, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassBundleUpdated, BreachKind: "bundle-validation", Check: prereqCheck("§11.4.32", "validated", "post-pull validation gate FAILED", "post-pull validation gate PASSED"), SubjectKinds: []string{SubjectBundleValidation}},
		{RuleID: "§11.4.36", Title: "Mandatory install_upstreams", Severity: constitution.SeverityMiddle, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassRepoSafetyBreach, BreachKind: "upstreams", Check: prereqCheck("§11.4.36", "configured", "submodule missing upstream mirror configuration", "submodule upstream mirrors configured"), SubjectKinds: []string{SubjectUpstreams}},
		{RuleID: "§11.4.41", Title: "Pre-force-push merge-first", Severity: constitution.SeverityCritical, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassRepoSafetyBreach, BreachKind: "force-push", Check: checkForcePush, SubjectKinds: []string{SubjectForcePush}},
		{RuleID: "§11.4.47", Title: "Firebase data review", Severity: constitution.SeverityLow, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassPolicyViolation, BreachKind: "", Check: prereqCheck("§11.4.47", "reviewed", "Firebase data review skipped (project-specific)", "Firebase data reviewed"), SubjectKinds: []string{SubjectFirebaseReview}},
		{RuleID: "§11.4.71", Title: "Pre-push fetch + investigate + integrate", Severity: constitution.SeverityHigh, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassRepoSafetyBreach, BreachKind: "push", Check: checkPrePushFetch, SubjectKinds: []string{SubjectPush}},

		// --- §12 host-session-safety (host.safety.breach) -------------------
		{RuleID: "§12.1", Title: "Forbidden host-session operations", Severity: constitution.SeverityCritical, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassHostSafetyBreach, BreachKind: "host-op", Check: forbiddenHostOp, SubjectKinds: []string{SubjectHostOp}},
		{RuleID: "§12.2", Title: "Required safeguards", Severity: constitution.SeverityHigh, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassHostSafetyBreach, BreachKind: "safeguard", Check: prereqCheck("§12.2", "bounded", "heavy work without bounded scope (no safeguard)", "heavy work bounded with safeguards"), SubjectKinds: []string{SubjectSafeguard}},
		{RuleID: "§12.3", Title: "Container hygiene", Severity: constitution.SeverityHigh, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassHostSafetyBreach, BreachKind: "container", Check: prereqCheck("§12.3", "mem_limit", "container started WITHOUT a memory limit", "container has a memory limit"), SubjectKinds: []string{SubjectContainer}},
		{RuleID: "§12.6", Title: "Memory-budget 60% ceiling", Severity: constitution.SeverityCritical, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassHostSafetyBreach, BreachKind: "mem-budget", Check: checkMemBudget, SubjectKinds: []string{SubjectMemBudget}},
	}
}

// checkPrePushFetch implements §11.4.71 (pre-push fetch + investigate +
// integrate). FAILs a push attempted without a preceding fetch (fetched=true)
// AND integrate (integrated=true). PURE: classifies the recorded prerequisites.
func checkPrePushFetch(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	ref, kv := subjectFields(s.ID)
	fetched := boolField(kv, "fetched")
	integrated := boolField(kv, "integrated")
	if fetched && integrated {
		return pass("§11.4.71: " + ref + " fetched + integrated before push"), nil
	}
	return fail("§11.4.71: " + ref + " push attempted without preceding fetch+integrate"), nil
}
