package bindings

// rules.go — the iherald-owned §42.3 incident/escalation constitution-rule
// catalogue (HRD-024).
//
// Per spec §42.3 master binding table + §42.5 step 7, iherald owns (sole or
// shared) the four escalation rows enumerated here. iherald is the INCIDENT
// flavor; it carries the credential-leak page-out + operator-blocked escalation
// + blocker-clarification bindings. Rows where iherald is a SHARED owner
// (§11.4.10 + §11.4.10.A with cherald; §11.4.21 + §11.4.66 with pherald) still
// register here — the OTHER owner registers its own binding in its own flavor
// package; the §6 channel router fans the SAME emitted event to both owners'
// subscribers via the heraldconstitutionrule extension attribute (§42.5 note).
//
// The bespoke detection hooks are the load-bearing vertical slice (the task's
// core deliverable). Each carries a REAL, deterministic, PURE classifier:
//
//   - CREDENTIAL-LEAK-PAGEOUT (§11.4.10 — CRITICAL): reads a recorded
//     credential-detection signal ("leaked=<bool>[|kind=<env|source|...>]") and
//     FAILs (→ critical page-out) when a plaintext credential / tracked .env was
//     detected. Routes through .credential.leak.
//   - PRE-STORE-LEAK-AUDIT (§11.4.10.A — CRITICAL): reads a recorded
//     credential-storage commit audit ("audited=<bool>[|leaked=<bool>]") and
//     FAILs when the pre-store leak audit was SKIPPED, OR the audit itself found
//     a leak. A credential-storage commit that skipped its gate is itself the
//     §11.4.10.A breach. Routes through .credential.leak.
//   - OPERATOR-BLOCKED-ESCALATION (§11.4.21 — HIGH): reads a recorded item
//     status transition ("status=<status>[|oncall_paged=<bool>]") and FAILs when
//     an item entered operator-blocked WITHOUT the mandatory on-call page-out.
//     Routes through .policy.violation.
//   - BLOCKER-RESOLUTION-CLARIFICATION (§11.4.66 — HIGH): reads a recorded
//     blocker state ("blocked=<bool>|clarification=<bool>") and FAILs when an
//     operator-blocked item has NO clarification prompt queued (the operator was
//     never asked the unblock question). Routes through .policy.violation.
//   - INCIDENT-SEVERITY-ROUTING (bespoke iherald, HIGH): reads a recorded
//     incident severity + paging outcome ("severity=<sev1..sev4>[|paged=<bool>]")
//     and FAILs when a high-severity incident (sev1/sev2) was NOT paged out.
//     Routes through .policy.violation. This is the iherald-specific escalation
//     detector beyond the four §42.3 rows — it enforces the §18.8 incident
//     command-room paging discipline.
//
// PURE detectors (CRITICAL §12 host-safety). Every CheckFunc CLASSIFIES a
// Subject string description of an already-observed incident signal. NONE scans
// a real .env, reads a real secret, pages a real on-call, runs git, or touches
// the filesystem. The live paging integration (/v1/webhooks/page handler body +
// §43 escalation command bodies) is scope-locked to the HRD-024-paging
// follow-ups — those supply these detectors their Subjects upstream.

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
// item / location / incident description). PURE string parsing only — never
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

// hasField reports whether the key is present at all (used to refuse a
// silent-PASS when the prerequisite signal was never recorded).
func hasField(kv map[string]string, key string) bool {
	_, ok := kv[key]
	return ok
}

// highSeverityIncidents is the closed set of incident-severity tokens that MUST
// trigger an immediate page-out per §18.8 incident command-room discipline.
var highSeverityIncidents = map[string]bool{"sev1": true, "sev2": true}

// ---------------------------------------------------------------------------
// Bespoke detection hooks — the HRD-024 vertical-slice core. All PURE.
// ---------------------------------------------------------------------------

// checkCredentialLeak implements §11.4.10 (credentials-handling page-out) — a
// CRITICAL escalation. It CLASSIFIES a recorded credential-detection signal:
//
//	leaked=true              → FAIL (a plaintext credential / tracked .env detected → page out)
//	leaked=false             → PASS (no credential leak)
//	(missing leaked field)   → FAIL (no leak-detection evidence — refuse to silent-PASS)
//
// PURE: it reads the recorded detection outcome; it NEVER scans a real .env or
// reads a real secret.
func checkCredentialLeak(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	loc, kv := subjectFields(s.ID)
	if loc == "" {
		loc = "location"
	}
	if !hasField(kv, "leaked") {
		return fail("§11.4.10: " + loc + " has NO credential-leak detection evidence (refusing to silent-PASS)"), nil
	}
	if boolField(kv, "leaked") {
		kind := kv["kind"]
		if kind == "" {
			kind = "credential"
		}
		return fail("§11.4.10: " + kind + " credential LEAK detected at " + loc + " — paging on-call (critical)"), nil
	}
	return pass("§11.4.10: " + loc + " clean — no plaintext credential / tracked .env detected"), nil
}

// checkPreStoreAudit implements §11.4.10.A (pre-store leak audit) — a CRITICAL
// escalation. It CLASSIFIES a recorded credential-storage commit audit:
//
//	audited=false            → FAIL (storage commit SKIPPED its pre-store leak audit → page out)
//	audited=true & leaked=true → FAIL (the audit RAN and found a leak → page out)
//	audited=true & leaked=false → PASS (gate ran clean)
//	(missing audited field)  → FAIL (no audit evidence — refuse to silent-PASS)
//
// PURE: it reads the recorded audit outcome; it NEVER inspects a real commit.
func checkPreStoreAudit(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	op, kv := subjectFields(s.ID)
	if op == "" {
		op = "credential-store-op"
	}
	if !hasField(kv, "audited") {
		return fail("§11.4.10.A: " + op + " has NO pre-store leak-audit evidence (refusing to silent-PASS)"), nil
	}
	if !boolField(kv, "audited") {
		return fail("§11.4.10.A: " + op + " committed credential storage WITHOUT the pre-store leak audit — paging on-call (critical)"), nil
	}
	if boolField(kv, "leaked") {
		return fail("§11.4.10.A: pre-store audit for " + op + " FOUND a leak — paging on-call (critical)"), nil
	}
	return pass("§11.4.10.A: pre-store leak audit for " + op + " ran and found no leak"), nil
}

// checkOperatorBlocked implements §11.4.21 (operator-blocked escalation) — HIGH.
// It CLASSIFIES a recorded workable-item status transition:
//
//	status=operator-blocked & oncall_paged=false → FAIL (escalation gap — no page)
//	status=operator-blocked & oncall_paged=true  → PASS (escalation engaged)
//	status != operator-blocked                   → PASS (not an escalation event)
//	(operator-blocked but missing oncall_paged)  → FAIL (refuse to silent-PASS the page)
//
// PURE: it reads the recorded status; it NEVER pages a real on-call.
func checkOperatorBlocked(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	item, kv := subjectFields(s.ID)
	if item == "" {
		item = "item"
	}
	status := strings.ToLower(kv["status"])
	if status != "operator-blocked" && status != "operator_blocked" && status != "blocked" {
		return pass("§11.4.21: " + item + " status=" + statusOrUnknown(kv) + " is not an operator-blocked escalation event"), nil
	}
	if !hasField(kv, "oncall_paged") {
		return fail("§11.4.21: " + item + " entered operator-blocked but has NO on-call paging evidence (refusing to silent-PASS)"), nil
	}
	if !boolField(kv, "oncall_paged") {
		return fail("§11.4.21: " + item + " entered operator-blocked WITHOUT paging the on-call tag — escalation gap"), nil
	}
	return pass("§11.4.21: " + item + " operator-blocked escalation engaged (on-call paged)"), nil
}

// checkBlockerClarification implements §11.4.66 (blocker-resolution
// clarification) — HIGH. It CLASSIFIES a recorded blocker state:
//
//	blocked=true  & clarification=false → FAIL (operator never asked the unblock question)
//	blocked=true  & clarification=true  → PASS (clarification prompt queued)
//	blocked=false                       → PASS (not a blocker event)
//	(blocked but missing clarification) → FAIL (refuse to silent-PASS)
//
// PURE: it reads the recorded blocker state; it NEVER prompts a real operator.
func checkBlockerClarification(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	item, kv := subjectFields(s.ID)
	if item == "" {
		item = "item"
	}
	if !boolField(kv, "blocked") {
		return pass("§11.4.66: " + item + " is not blocked — no clarification prompt required"), nil
	}
	if !hasField(kv, "clarification") {
		return fail("§11.4.66: " + item + " is blocked but has NO clarification-prompt evidence (refusing to silent-PASS)"), nil
	}
	if !boolField(kv, "clarification") {
		return fail("§11.4.66: " + item + " operator-blocked WITHOUT a clarification prompt — the unblock question was never asked"), nil
	}
	return pass("§11.4.66: " + item + " operator-blocked WITH a queued clarification prompt"), nil
}

// checkIncidentSeverity is the bespoke iherald incident-severity routing
// detector (§18.8 command-room discipline) — HIGH. It CLASSIFIES a recorded
// incident severity + paging outcome:
//
//	severity in {sev1,sev2} & paged=false → FAIL (high-severity incident not paged → routing failure)
//	severity in {sev1,sev2} & paged=true  → PASS (paged correctly)
//	severity in {sev3,sev4}               → PASS (no mandatory page-out)
//	(missing/unknown severity)            → FAIL (refuse to silent-PASS an unclassified incident)
//
// PURE: it reads the recorded incident routing decision; it NEVER pages.
func checkIncidentSeverity(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	inc, kv := subjectFields(s.ID)
	if inc == "" {
		inc = "incident"
	}
	sev := strings.ToLower(kv["severity"])
	if sev == "" {
		return fail("§18.8: incident " + inc + " has NO severity classification (refusing to silent-PASS routing)"), nil
	}
	if !highSeverityIncidents[sev] {
		// sev3 / sev4 (or any explicitly low tier) require no mandatory page-out.
		if sev == "sev3" || sev == "sev4" {
			return pass("§18.8: incident " + inc + " severity=" + sev + " requires no mandatory page-out"), nil
		}
		return fail("§18.8: incident " + inc + " has an unrecognized severity " + sev + " (refusing to silent-PASS routing)"), nil
	}
	if !hasField(kv, "paged") {
		return fail("§18.8: high-severity incident " + inc + " (" + sev + ") has NO paging evidence (refusing to silent-PASS)"), nil
	}
	if !boolField(kv, "paged") {
		return fail("§18.8: high-severity incident " + inc + " (" + sev + ") was NOT paged out — incident-severity routing failure"), nil
	}
	return pass("§18.8: high-severity incident " + inc + " (" + sev + ") paged out correctly"), nil
}

// statusOrUnknown returns the recorded status or "<unset>" for evidence strings.
func statusOrUnknown(kv map[string]string) string {
	if v, ok := kv["status"]; ok && v != "" {
		return v
	}
	return "<unset>"
}

// ---------------------------------------------------------------------------
// The iherald-owned §42.3 incident/escalation catalogue.
// ---------------------------------------------------------------------------

// IheraldRules returns the catalogue of every iherald-owned (sole or shared)
// §42.3 escalation rule plus the bespoke §18.8 incident-severity routing rule,
// each with its default severity + mode + event class + EscalationKind + a real
// PURE CheckFunc. The slice is freshly built on each call (callers may mutate
// their copy). Order is deterministic (source order = roughly section order).
func IheraldRules() []RuleSpec {
	return []RuleSpec{
		// §11.4.10 Credentials-handling — enforce / critical — .credential.leak
		// (SHARED with cherald). Default-owner page-out for a detected plaintext
		// credential / tracked .env.
		{RuleID: "§11.4.10", Title: "Credentials-handling", Severity: constitution.SeverityCritical, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassCredentialLeak, EscalationKind: "credential-leak", Check: checkCredentialLeak, SubjectKinds: []string{SubjectCredentialLeak}},

		// §11.4.10.A Pre-store leak audit — enforce / critical — .credential.leak
		// (SHARED with cherald). Gate before committing credential storage.
		{RuleID: "§11.4.10.A", Title: "Pre-store leak audit", Severity: constitution.SeverityCritical, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassCredentialLeak, EscalationKind: "pre-store-audit", Check: checkPreStoreAudit, SubjectKinds: []string{SubjectPreStoreAudit}},

		// §11.4.21 Operator-blocked status — enforce / high — .policy.violation
		// (SHARED with pherald). Item enters operator-blocked → page on-call.
		{RuleID: "§11.4.21", Title: "Operator-blocked status", Severity: constitution.SeverityHigh, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassPolicyViolation, EscalationKind: "operator-blocked", Check: checkOperatorBlocked, SubjectKinds: []string{SubjectOperatorBlocked}},

		// §11.4.66 Blocker-resolution clarification — enforce / high —
		// .policy.violation (SHARED with pherald). Operator-blocked without a
		// clarification prompt.
		{RuleID: "§11.4.66", Title: "Blocker-resolution clarification", Severity: constitution.SeverityHigh, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassPolicyViolation, EscalationKind: "blocker-clarification", Check: checkBlockerClarification, SubjectKinds: []string{SubjectBlockerClarification}},

		// §18.8 Incident-severity routing — enforce / high — .policy.violation.
		// Bespoke iherald escalation rule beyond the four §42.3 rows: a high-
		// severity incident (sev1/sev2) that was not paged out is a routing failure.
		{RuleID: "§18.8", Title: "Incident-severity routing", Severity: constitution.SeverityHigh, DefaultMode: constitution.ModeEnforce, EventClass: constitution.ClassPolicyViolation, EscalationKind: "incident-severity", Check: checkIncidentSeverity, SubjectKinds: []string{SubjectIncidentSeverity}},
	}
}
