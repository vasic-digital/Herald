package bindings

// rules.go — the pherald-owned §42.3 PROJECT-lifecycle constitution-rule
// catalogue (HRD-023).
//
// Per spec §42.3 master binding table, pherald owns (sole) the project-lifecycle
// rows enumerated here. pherald is the PROJECT flavor; it carries the
// commit-push / submodule-propagation / install-upstreams / fetch-before-edit /
// reopens-history / pre-push bindings.
//
// Four bespoke detection hooks are the load-bearing vertical slice (the task's
// core deliverable). Each carries a REAL, deterministic, PURE classifier:
//
//   - COMMIT-PUSH-DISCIPLINE (§2): reads a recorded commit state
//     ("entrypoint=<bool>|lock_held=<bool>") and FAILs when the commit was made
//     OUTSIDE the single locked entrypoint OR without the commit-lock held.
//     Routes through .repo.safety.breach.
//   - SUBMODULE-PROPAGATION-ORDER (§3): reads a recorded propagation order
//     ("order=<inner-first|parent-first>|inner_pushed=<bool>") and FAILs when the
//     parent was committed before the inner submodules OR the inner SHA was not
//     pushed (the parent would pin a dangling SHA). Routes through
//     .repo.safety.breach.
//   - PRE-PUSH-FETCH-GUARD (§11.4.71): reads a recorded pre-push state
//     ("fetched=<bool>|integrated=<bool>") and FAILs when a push was attempted
//     WITHOUT the pre-push fetch OR without integrating incoming changes. Routes
//     through .repo.safety.breach.
//   - FETCH-BEFORE-EDIT (§11.4.37): reads a recorded rebase state
//     ("rebased=<bool>") and FAILs when an edit was made on a tree NOT rebased on
//     origin. Routes through .repo.safety.breach.
//
// Two policy rows complete the §42.3 pherald set:
//   - INSTALL-UPSTREAMS (§11.4.36): reads a recorded mirror tally
//     ("configured=<n>|declared=<n>") and FAILs when fewer mirror remotes are
//     configured than declared in Upstreams/*.sh. Routes through .policy.violation.
//   - REOPENS-HISTORY (§11.4.55): reads a recorded reopen state
//     ("recorded=<bool>") and FAILs when an Issues←Fixed reversal lacks its
//     docs/Reopens/<HRD>.md record. Routes through .policy.violation.
//
// PURE detectors (CRITICAL §12 host-safety). Every CheckFunc CLASSIFIES a
// Subject string description of an already-observed project-op outcome. NONE
// commits, pushes, force-pushes, runs git, configures remotes, or touches the
// filesystem. The §43 command bodies (HRD-029 commit-push, HRD-030 submodule-
// propagate, HRD-043 install-upstreams, HRD-044 fetch-guard, HRD-049 reopen,
// HRD-053 pre-push) supply the live project integration upstream — the live
// project-op interception is scope-locked to those §43 follow-ups.

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
// sha / HRD / branch / mirror-set description). PURE string parsing only — never
// touches the filesystem, network, process table, or git.
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

// boolField reads a "true"/"false" field; missing or unparseable → (false,
// false). The second return reports whether the field was present + parseable so
// callers can distinguish "absent" (refuse to silent-PASS) from "explicit false".
func boolField(kv map[string]string, key string) (val bool, present bool) {
	v, ok := kv[key]
	if !ok {
		return false, false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, false
	}
	return b, true
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

// ---------------------------------------------------------------------------
// Bespoke detection hooks — the HRD-023 vertical-slice core. All PURE.
// ---------------------------------------------------------------------------

// checkCommitPushDiscipline implements §2 (commit + push mechanics). It
// CLASSIFIES a recorded commit:
//
//	entrypoint=true AND lock_held=true → PASS (through the single locked entrypoint)
//	entrypoint=false                   → FAIL (the §2 "entrypoint bypassed" breach)
//	lock_held=false                    → FAIL (committed without the commit-lock held)
//	(missing fields)                   → FAIL (refuse to silent-PASS — §11.4.1 inverse)
//
// PURE: it reads the recorded commit state; it NEVER commits, pushes, or runs git.
func checkCommitPushDiscipline(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	sha, kv := subjectFields(s.ID)
	if sha == "" {
		sha = "commit"
	}
	ep, epOK := boolField(kv, "entrypoint")
	lock, lockOK := boolField(kv, "lock_held")
	if !epOK || !lockOK {
		return fail("§2: commit " + sha + " has no entrypoint/lock evidence (refusing to silent-PASS)"), nil
	}
	if !ep {
		return fail("§2: commit " + sha + " was made OUTSIDE the single locked entrypoint (§2 repo-safety breach)"), nil
	}
	if !lock {
		return fail("§2: commit " + sha + " was made WITHOUT the commit-lock held (§2 repo-safety breach)"), nil
	}
	return pass("§2: commit " + sha + " went through the single locked entrypoint with the lock held"), nil
}

// checkSubmodulePropagationOrder implements §3 (submodule propagation order). It
// CLASSIFIES a recorded propagation:
//
//	order=inner-first AND inner_pushed=true → PASS (correct §3 order)
//	order=parent-first                      → FAIL (parent committed before inner)
//	inner_pushed=false                      → FAIL (parent would pin a dangling inner SHA)
//	(missing order field)                   → FAIL (refuse to silent-PASS)
//
// PURE: it reads the recorded order; it NEVER commits or runs git submodule.
func checkSubmodulePropagationOrder(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	_, kv := subjectFields(s.ID)
	order, ok := kv["order"]
	if !ok {
		return fail("§3: submodule propagation has no recorded order (refusing to silent-PASS)"), nil
	}
	switch strings.ToLower(order) {
	case "inner-first", "inner", "child-first":
		pushed, pOK := boolField(kv, "inner_pushed")
		if !pOK {
			return fail("§3: inner-first propagation reports no inner-pushed state (cannot confirm the parent pins a pushed SHA)"), nil
		}
		if !pushed {
			return fail("§3: inner-first propagation but the inner SHA was NOT pushed — the parent would pin a dangling SHA"), nil
		}
		return pass("§3: submodule propagation committed inner submodules first then the parent (pushed)"), nil
	case "parent-first", "parent", "outer-first":
		return fail("§3: submodule propagation committed the PARENT before the inner submodules (§3 wrong propagation order)"), nil
	default:
		return fail("§3: submodule propagation has an unrecognized order " + order + " (refusing to silent-PASS)"), nil
	}
}

// checkPrePushFetchGuard implements §11.4.71 (pre-push fetch+investigate+
// integrate). It CLASSIFIES a recorded pre-push state:
//
//	fetched=true AND integrated=true → PASS (incoming changes fetched + integrated)
//	fetched=false                    → FAIL (push WITHOUT the pre-push fetch)
//	integrated=false                 → FAIL (fetched but incoming changes not integrated)
//	(missing fields)                 → FAIL (refuse to silent-PASS)
//
// PURE: it reads the recorded pre-push state; it NEVER fetches or pushes.
func checkPrePushFetchGuard(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	branch, kv := subjectFields(s.ID)
	if branch == "" {
		branch = "branch"
	}
	fetched, fOK := boolField(kv, "fetched")
	integrated, iOK := boolField(kv, "integrated")
	if !fOK || !iOK {
		return fail("§11.4.71: pre-push for " + branch + " has no fetch/integrate evidence (refusing to silent-PASS)"), nil
	}
	if !fetched {
		return fail("§11.4.71: push to " + branch + " attempted WITHOUT the mandatory pre-push fetch (repo-safety breach)"), nil
	}
	if !integrated {
		return fail("§11.4.71: pre-push fetch for " + branch + " done but incoming changes were NOT integrated (repo-safety breach)"), nil
	}
	return pass("§11.4.71: pre-push for " + branch + " fetched + integrated incoming changes before pushing"), nil
}

// checkFetchBeforeEdit implements §11.4.37 (fetch-before-edit). It CLASSIFIES a
// recorded rebase state:
//
//	rebased=true  → PASS (edit on a tree rebased on origin)
//	rebased=false → FAIL (edit on a stale tree — §11.4.37 repo-safety breach)
//	(missing)     → FAIL (refuse to silent-PASS)
//
// PURE: it reads the recorded rebase state; it NEVER fetches.
func checkFetchBeforeEdit(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	branch, kv := subjectFields(s.ID)
	if branch == "" {
		branch = "branch"
	}
	rebased, ok := boolField(kv, "rebased")
	if !ok {
		return fail("§11.4.37: edit on " + branch + " has no rebase evidence (refusing to silent-PASS)"), nil
	}
	if !rebased {
		return fail("§11.4.37: edit on " + branch + " was made on a STALE tree (not rebased on origin — repo-safety breach)"), nil
	}
	return pass("§11.4.37: edit on " + branch + " was made on a tree rebased on origin"), nil
}

// checkInstallUpstreams implements §11.4.36 (install-upstreams). It CLASSIFIES a
// recorded mirror tally:
//
//	configured >= declared (declared >= 1) → PASS (every declared mirror configured)
//	configured < declared                  → FAIL (a declared mirror is not configured)
//	(missing tally)                        → FAIL (refuse to silent-PASS)
//
// PURE: it reads the recorded tally; it NEVER configures a remote or runs git.
func checkInstallUpstreams(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	_, kv := subjectFields(s.ID)
	configured, cOK := intField(kv, "configured")
	declared, dOK := intField(kv, "declared")
	if !cOK || !dOK {
		return fail("§11.4.36: install-upstreams has no configured/declared mirror tally (refusing to silent-PASS)"), nil
	}
	if declared < 1 {
		return fail("§11.4.36: install-upstreams reports zero declared mirrors (mis-recorded — Herald declares 4)"), nil
	}
	if configured < declared {
		return fail("§11.4.36: install-upstreams MISS: only " + strconv.Itoa(configured) + "/" + strconv.Itoa(declared) + " declared mirror remotes configured"), nil
	}
	return pass("§11.4.36: install-upstreams configured all " + strconv.Itoa(declared) + " declared mirror remotes"), nil
}

// checkReopensHistory implements §11.4.55 (reopens-history). It CLASSIFIES a
// recorded reopen:
//
//	recorded=true  → PASS (the docs/Reopens/<HRD>.md record exists)
//	recorded=false → FAIL (Issues←Fixed reversal without its reopens record)
//	(missing)      → FAIL (refuse to silent-PASS)
//
// PURE: it reads the recorded reopen state; it NEVER writes docs or runs git.
func checkReopensHistory(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	hrd, kv := subjectFields(s.ID)
	if hrd == "" {
		hrd = "item"
	}
	recorded, ok := boolField(kv, "recorded")
	if !ok {
		return fail("§11.4.55: reopen of " + hrd + " has no reopens-record evidence (refusing to silent-PASS)"), nil
	}
	if !recorded {
		return fail("§11.4.55: reopen of " + hrd + " has NO docs/Reopens/" + hrd + ".md history record"), nil
	}
	return pass("§11.4.55: reopen of " + hrd + " carries its docs/Reopens/" + hrd + ".md history record"), nil
}

// ---------------------------------------------------------------------------
// The pherald-owned §42.3 project catalogue.
// ---------------------------------------------------------------------------

// PheraldRules returns the catalogue of every pherald-owned (sole) §42.3 project
// rule, with its default severity + mode + event class + BreachKind name + a
// real PURE CheckFunc. The slice is freshly built on each call (callers may
// mutate their copy). Order is deterministic (source order = roughly section
// order).
func PheraldRules() []RuleSpec {
	return []RuleSpec{
		// §2 Commit + push mechanics — enforce / CRITICAL — .repo.safety.breach.
		// The single locked entrypoint is the highest-severity repo-safety gate.
		{RuleID: "§2", Title: "Commit + push mechanics", Severity: constitution.SeverityCritical, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassRepoSafetyBreach, BreachKind: "commit-push", Check: checkCommitPushDiscipline, SubjectKinds: []string{SubjectCommitPush}},

		// §3 Submodule propagation order — enforce / high — .repo.safety.breach.
		{RuleID: "§3", Title: "Submodule propagation order", Severity: constitution.SeverityHigh, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassRepoSafetyBreach, BreachKind: "submodule-order", Check: checkSubmodulePropagationOrder, SubjectKinds: []string{SubjectSubmodulePropagate}},

		// §11.4.36 install-upstreams — warn / middle — .policy.violation.
		{RuleID: "§11.4.36", Title: "install-upstreams", Severity: constitution.SeverityMiddle, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassPolicyViolation, BreachKind: "install-upstreams", Check: checkInstallUpstreams, SubjectKinds: []string{SubjectInstallUpstreams}},

		// §11.4.37 Fetch-before-edit — enforce / high — .repo.safety.breach.
		{RuleID: "§11.4.37", Title: "Fetch-before-edit", Severity: constitution.SeverityHigh, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassRepoSafetyBreach, BreachKind: "fetch-before-edit", Check: checkFetchBeforeEdit, SubjectKinds: []string{SubjectFetchGuard}},

		// §11.4.55 Reopens-history — warn / middle — .policy.violation.
		{RuleID: "§11.4.55", Title: "Reopens-history", Severity: constitution.SeverityMiddle, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassPolicyViolation, BreachKind: "reopens-history", Check: checkReopensHistory, SubjectKinds: []string{SubjectReopen}},

		// §11.4.71 Pre-push fetch+investigate+integrate — enforce / high —
		// .repo.safety.breach.
		{RuleID: "§11.4.71", Title: "Pre-push fetch + investigate + integrate", Severity: constitution.SeverityHigh, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassRepoSafetyBreach, BreachKind: "pre-push", Check: checkPrePushFetchGuard, SubjectKinds: []string{SubjectPrePush}},
	}
}
