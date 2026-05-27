package bindings

// rules.go — the scherald-owned §42.3 scheduled-audit constitution-rule
// catalogue (HRD-025).
//
// Per spec §42.3 master binding table, scherald is the default owner of the
// §11.4.45 Integration-Status-Doc Maintenance row (`.policy.violation`, warn,
// low — "Status.md stale; composes with §37"). HRD-025 lands that row PLUS two
// scheduled-audit-specific bespoke rules that the §43 / HRD-047 `scherald status
// digest [--cadence=daily|weekly|monthly]` command surfaces: the periodic
// compliance-digest cadence guard (§18.7 digest contract) and the stale-work-
// item detector (the periodic audit sweep facet of §11.4.45 + §11.4.56).
//
// Three bespoke detection hooks are the load-bearing vertical slice (the task's
// core deliverable). Each carries a REAL, deterministic, PURE classifier:
//
//   - STATUS-SWEEP (§11.4.45): reads a recorded Status.md sweep finding
//     ("sweep=<clean|stale>[|stale_items=N][|summary_synced=<bool>]") and FAILs
//     when the sweep found Status.md stale OR its Status_Summary.md derivative
//     out of sync (the §11.4.45 + §11.4.56 composition). Routes through
//     .policy.violation.
//   - DIGEST-CADENCE (§18.7 digest contract): reads a recorded
//     compliance-digest cadence tally ("due=<bool>|emitted=<bool>[|overdue_by_h=N]")
//     and FAILs when a digest fell DUE but was never emitted (a missed
//     scheduled digest). Routes through .policy.violation.
//   - STALE-ITEM (§11.4.45 periodic audit): reads a recorded stale-item count
//     ("stale_items=N[|threshold=M]") and FAILs when the count exceeds the
//     configured threshold (open work-items with no status movement). Routes
//     through .policy.violation.
//
// PURE detectors (CRITICAL §12 host-safety). Every CheckFunc CLASSIFIES a
// Subject string description of an already-observed scheduled-audit outcome.
// NONE reads Status.md, runs a cron tick, regenerates a digest, walks the HRD
// trackers, or touches the filesystem. The §43 command body (HRD-047) +
// scheduler supply the live integration upstream — the live cron / scheduler
// interception is scope-locked to that §43 follow-up.

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
// doc / cadence / scope description). PURE string parsing only — never touches
// the filesystem, network, process table, cron, or a doc.
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

// defaultStaleThreshold is the §11.4.45 periodic-audit default stale-item
// ceiling: the sweep tolerates UP TO this many open items with no recent status
// movement before it raises a policy violation. A subject may override it with
// an explicit "threshold=N" field.
const defaultStaleThreshold = 0

// ---------------------------------------------------------------------------
// Bespoke detection hooks — the HRD-025 vertical-slice core. All PURE.
// ---------------------------------------------------------------------------

// checkStatusSweep implements §11.4.45 (Integration-Status-Doc Maintenance). It
// CLASSIFIES a recorded periodic Status.md sweep finding:
//
//	sweep=stale                          → FAIL (Status.md is stale)
//	sweep=clean but summary_synced=false → FAIL (Status_Summary.md out of sync — §11.4.56 composition)
//	sweep=clean (summary synced or absent) → PASS
//	(missing sweep field)                → FAIL (refuse to silent-PASS — §11.4.1 inverse)
//
// PURE: it reads the recorded sweep result; it NEVER reads Status.md or runs the
// sweep.
func checkStatusSweep(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	doc, kv := subjectFields(s.ID)
	if doc == "" {
		doc = "Status.md"
	}
	sweep, ok := kv["sweep"]
	if !ok {
		return fail("§11.4.45: " + doc + " sweep has no recorded result (refusing to silent-PASS)"), nil
	}
	switch strings.ToLower(sweep) {
	case "stale", "drift", "out-of-date":
		evidence := "§11.4.45: " + doc + " is STALE (periodic sweep flagged drift"
		if n, okN := intField(kv, "stale_items"); okN {
			evidence += "; " + strconv.Itoa(n) + " stale items"
		}
		return fail(evidence + ")"), nil
	case "clean", "fresh", "in-sync":
		// §11.4.56 composition: a clean Status.md whose Status_Summary.md is out
		// of sync is still a maintenance violation.
		if v, present := kv["summary_synced"]; present && !boolField(kv, "summary_synced") {
			_ = v
			return fail("§11.4.45/.56: " + doc + " is clean but Status_Summary.md is OUT OF SYNC"), nil
		}
		return pass("§11.4.45: " + doc + " is current (periodic sweep clean, summary in sync)"), nil
	default:
		return fail("§11.4.45: " + doc + " sweep has an unrecognized result (refusing to silent-PASS)"), nil
	}
}

// checkDigestCadence implements the §18.7 scheduled compliance-digest cadence
// contract. It CLASSIFIES a recorded digest-cadence tally:
//
//	due=true  & emitted=false → FAIL (a scheduled digest fell DUE but was never emitted)
//	due=true  & emitted=true  → PASS (digest fired on schedule)
//	due=false                 → PASS (nothing scheduled this tick — vacuously satisfied)
//	(missing due field)       → FAIL (refuse to silent-PASS)
//
// PURE: it reads the recorded cadence tally; it NEVER regenerates a digest or
// consults a clock / cron.
func checkDigestCadence(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	cadence, kv := subjectFields(s.ID)
	if cadence == "" {
		cadence = "digest"
	}
	if _, ok := kv["due"]; !ok {
		return fail("§18.7: " + cadence + " digest has no recorded cadence state (refusing to silent-PASS)"), nil
	}
	if !boolField(kv, "due") {
		return pass("§18.7: " + cadence + " digest not due this tick (cadence satisfied)"), nil
	}
	// due=true: a digest was scheduled. It MUST have been emitted.
	if !boolField(kv, "emitted") {
		evidence := "§18.7: " + cadence + " digest fell DUE but was NOT emitted (missed scheduled digest"
		if n, okN := intField(kv, "overdue_by_h"); okN {
			evidence += "; overdue by " + strconv.Itoa(n) + "h"
		}
		return fail(evidence + ")"), nil
	}
	return pass("§18.7: " + cadence + " digest emitted on schedule"), nil
}

// checkStaleItem implements the §11.4.45 periodic-audit stale-work-item detector.
// It CLASSIFIES a recorded stale-item count against a threshold:
//
//	stale_items <= threshold → PASS
//	stale_items >  threshold → FAIL (too many items with no recent status movement)
//	(missing stale_items)    → FAIL (no audit evidence — refuse to silent-PASS)
//
// PURE: it reads the recorded count; it NEVER walks the HRD trackers or reads
// any tracker doc.
func checkStaleItem(_ context.Context, s constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	scope, kv := subjectFields(s.ID)
	if scope == "" {
		scope = "trackers"
	}
	n, ok := intField(kv, "stale_items")
	if !ok {
		return fail("§11.4.45: " + scope + " has no recorded stale-item count (refusing to silent-PASS)"), nil
	}
	threshold := defaultStaleThreshold
	if t, okT := intField(kv, "threshold"); okT {
		threshold = t
	}
	if n > threshold {
		return fail("§11.4.45: " + scope + " has " + strconv.Itoa(n) + " stale work-items (>" + strconv.Itoa(threshold) + " threshold — periodic audit raised)"), nil
	}
	return pass("§11.4.45: " + scope + " stale-item count " + strconv.Itoa(n) + " within threshold " + strconv.Itoa(threshold)), nil
}

// ---------------------------------------------------------------------------
// The scherald-owned §42.3 scheduled-audit catalogue.
// ---------------------------------------------------------------------------

// ScheraldRules returns the catalogue of every scherald-owned §42.3
// scheduled-audit rule, with its default severity + mode + event class +
// AuditGate name + a real PURE CheckFunc. The slice is freshly built on each
// call (callers may mutate their copy). Order is deterministic (source order).
//
// Note on rule IDs: §11.4.45 is the canonical spec-table anchor and is bound
// once (the status-sweep facet). The compliance-digest-cadence and stale-item
// rules are scheduled-audit-specific facets that scherald owns as the
// scheduled-audit flavor; they carry distinct anchors so the registry holds no
// duplicate key and each surfaces its own audit/state row. §11.4.45.digest and
// §11.4.45.stale denote the two §11.4.45-derived periodic-audit facets the §43
// `scherald status digest` command surfaces.
func ScheraldRules() []RuleSpec {
	return []RuleSpec{
		// §11.4.45 Integration-Status-Doc Maintenance — warn / low —
		// .policy.violation (scherald is the default owner per §42.3; composes
		// with §37 + the §11.4.56 Status_Summary parity facet).
		{RuleID: "§11.4.45", Title: "Integration-Status-Doc Maintenance", Severity: constitution.SeverityLow, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassPolicyViolation, AuditGate: "status-sweep", Check: checkStatusSweep, SubjectKinds: []string{SubjectStatusSweep}},

		// §11.4.45.digest Periodic compliance-digest cadence — warn / low —
		// .policy.violation (§18.7 digest contract facet of the periodic audit).
		{RuleID: "§11.4.45.digest", Title: "Periodic compliance-digest cadence", Severity: constitution.SeverityLow, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassPolicyViolation, AuditGate: "digest-cadence", Check: checkDigestCadence, SubjectKinds: []string{SubjectDigestCadence}},

		// §11.4.45.stale Stale-work-item detection — warn / low —
		// .policy.violation (periodic-audit-sweep facet of §11.4.45 + §11.4.56).
		{RuleID: "§11.4.45.stale", Title: "Stale-work-item detection", Severity: constitution.SeverityLow, DefaultMode: constitution.ModeWarn, EventClass: constitution.ClassPolicyViolation, AuditGate: "stale-item", Check: checkStaleItem, SubjectKinds: []string{SubjectStaleItem}},
	}
}
