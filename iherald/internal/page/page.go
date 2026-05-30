// Package page implements iherald's HRD-024 live POST /v1/webhooks/page
// handler — the PagerDuty/Opsgenie-compatible inbound escalation surface
// (spec §42 / §18.8). It flips the Wave 2 r1 501-stub
// (cli.Route{HRD:"HRD-024"}) to a real handler that drives the iherald
// HRD-024 bindings.Pipeline (the §42.3 credential-leak page-out +
// operator-blocked escalation + §18.8 incident-severity routing detectors)
// end-to-end:
//
//	commons_auth.GinMiddleware → Handler → bindings.Pipeline.EvaluateSubject →
//	  { classify → record(state) → gate(ladder) → emit(EventClass) → audit }
//
// A page webhook carries a recorded escalation signal (the same Subject string
// the §43 escalation command bodies record). The handler classifies it through
// the matching iherald escalation rule, drives the REAL emitter + REAL store +
// REAL audit, and returns a Receipt acknowledging the page-out. The persisted
// constitution_state + constitution_audit rows ARE the positive runtime
// evidence (§107 anti-bluff) — a queryable, audited escalation verdict, not a
// metadata-only PASS.
//
// HTTP contract (mirrors pherald POST /v1/events + cherald POST
// /v1/compliance/evaluate):
//
//   - Fresh escalation page accepted     → 202 Accepted + Receipt JSON
//   - Auth claim missing / malformed      → 401 Unauthorized
//   - Malformed / empty body              → 400 Bad Request, error tagged
//                                           `event_parser:` (the pherald
//                                           /v1/events convention)
//   - Unknown rule_id                     → 400 Bad Request `event_parser:`
//   - operator_id not in HERALD_OPERATOR_IDS allow-list (when the list is
//     configured AND the rule is an operator-gated escalation)
//                                         → 403 Forbidden
//   - Pipeline (store/emit/audit) failure → 502 Bad Gateway
//
// §107 anti-bluff: the handler NEVER fabricates a success. A pipeline error
// surfaces as a typed 502; a malformed body as a tagged 400. The Receipt
// reports the REAL Decision/Mode/Emitted/Audited the pipeline produced — so an
// operator reading the body knows whether the page actually fanned out.
//
// EXTERNAL-DELIVERY SEAM (documented follow-up, HRD-024-paging). This handler
// implements the FULL in-Herald escalation path: classify → emit the
// .credential.leak / .policy.violation CloudEvent onto the constitution
// EventBus → persist + audit. The actual outbound delivery to a third-party
// pager (PagerDuty Events API v2 / Opsgenie Alert API) is a SEPARATE channel
// subscriber that consumes those emitted events — it is NOT part of this
// handler and is intentionally out of scope (it requires operator-supplied
// PagerDuty/Opsgenie credentials + a live HTTP egress that cannot run
// hermetically). The emitted event IS the page-out within Herald's own bus;
// wiring a real PagerDuty/Opsgenie egress subscriber is tracked as the
// HRD-024-paging follow-up. This is honest: we do not return a fake 200
// claiming a third-party pager was notified.
package page

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/vasic-digital/herald/commons_auth"
	constitution "github.com/vasic-digital/herald/commons_constitution"
	"github.com/vasic-digital/herald/iherald/internal/bindings"
)

// operatorEnvVar is the env var carrying the comma/whitespace-separated
// allow-list of operator IDs authorised to file operator-gated escalation
// pages (§11.4.21 operator-blocked + §11.4.66 blocker-clarification). When
// UNSET, the operator gate is OPEN (no restriction) — matching the existing
// HERALD_*_IDS resolution convention (env is the only source; a missing var
// means "no allow-list configured", not "deny all"). When SET, an operator_id
// outside the list is rejected 403 for operator-gated rules only.
const operatorEnvVar = "HERALD_OPERATOR_IDS"

// operatorGatedRules is the closed set of rule IDs whose page requests are
// subject to the HERALD_OPERATOR_IDS allow-list. These are the operator-driven
// escalations (an operator is the actor filing them). The credential-leak +
// incident-severity rows are machine-detected signals, not operator-filed, so
// they are NOT operator-gated.
var operatorGatedRules = map[string]bool{
	"§11.4.21": true, // operator-blocked escalation
	"§11.4.66": true, // blocker-resolution clarification
}

// pageBody is the POST /v1/webhooks/page request body. It is the
// PagerDuty/Opsgenie-compatible inbound escalation envelope, projected onto the
// iherald escalation rule catalogue. rule_id selects which §42.3 / §18.8
// escalation detector classifies the signal; subject_kind + subject_id carry
// the recorded escalation signal (the same Subject string the §43 escalation
// command bodies record — e.g. "leaked=true|kind=env" for a credential leak,
// "status=operator-blocked|oncall_paged=false" for an escalation gap,
// "severity=sev1|paged=false" for an incident-severity routing decision).
type pageBody struct {
	// RuleID selects the escalation detector, e.g. "§11.4.10" (credential
	// leak), "§11.4.21" (operator-blocked), "§18.8" (incident-severity).
	RuleID string `json:"rule_id"`
	// SubjectKind documents the signal kind (one of the bindings.Subject*
	// constants). Defaults to "incident" when omitted.
	SubjectKind string `json:"subject_kind"`
	// SubjectID is the recorded escalation signal string the detector
	// classifies.
	SubjectID string `json:"subject_id"`
	// OperatorID is the actor filing an operator-gated escalation; validated
	// against HERALD_OPERATOR_IDS for the operator-gated rules. Ignored for
	// machine-detected rows.
	OperatorID string `json:"operator_id,omitempty"`
}

// Receipt is the page-out acknowledgement returned on 202. It mirrors the
// shape of cherald's /v1/compliance/evaluate response + pherald's Receipt: it
// reports the REAL pipeline outcome so an operator can confirm the page
// actually fanned out (emitted=true) and was audited (audited=true). A
// metadata-only "accepted" with no decision/emit detail would be a §107
// PASS-bluff.
type Receipt struct {
	TenantID     string `json:"tenant_id"`
	RuleID       string `json:"rule_id"`
	Subject      string `json:"subject"`
	Decision     string `json:"decision"`
	Mode         string `json:"mode"`
	Escalation   string `json:"escalation"`              // the EscalationKind that fired (page-out surface)
	Emitted      bool   `json:"emitted"`                 // true iff the escalation CloudEvent fanned out
	Audited      bool   `json:"audited"`                 // true iff an audit row was written
	Changed      bool   `json:"changed"`                 // true iff this was a state transition
	FirstSeen    bool   `json:"first_seen"`              // true iff first time this (rule,subject) was seen
	TransitionTo string `json:"transition_to"`           // the new decision after the transition
	Panic        string `json:"panic,omitempty"`         // populated iff the detector panicked
	PagerNote    string `json:"pager_delivery,omitempty"` // honest note re: external pager egress seam
}

// pagerSeamNote is the honest external-delivery disclosure carried on every
// emitted page Receipt (see the package-doc EXTERNAL-DELIVERY SEAM section).
const pagerSeamNote = "escalation event emitted on the in-Herald constitution bus; outbound PagerDuty/Opsgenie egress is a subscriber (HRD-024-paging follow-up), not performed by this handler"

// Handler returns the gin.HandlerFunc serving POST /v1/webhooks/page, backed
// by the iherald HRD-024 bindings.Pipeline. The handler is the thinnest
// possible adapter between the HTTP plane and the Pipeline — all classify /
// emit / persist / audit logic lives in bindings/.
func Handler(pipeline *bindings.Pipeline) gin.HandlerFunc {
	return func(c *gin.Context) {
		// --- Auth + tenant gate (mirrors cherald EvaluateHandler). ---
		claimsAny, ok := c.Get(commons_auth.ContextKeyClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "no auth claims in context"})
			return
		}
		claims, ok := claimsAny.(map[string]any)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "auth claims wrong shape"})
			return
		}
		tenantID, err := commons_auth.TenantFromClaims(claims)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "bad tenant claim", "detail": err.Error()})
			return
		}

		// --- Parse + validate the webhook body. Malformed → 400 tagged
		// `event_parser:` per the pherald /v1/events convention. ---
		var body pageBody
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "event_parser: malformed page body", "detail": err.Error()})
			return
		}
		body.RuleID = strings.TrimSpace(body.RuleID)
		body.SubjectKind = strings.TrimSpace(body.SubjectKind)
		body.SubjectID = strings.TrimSpace(body.SubjectID)
		body.OperatorID = strings.TrimSpace(body.OperatorID)
		if body.RuleID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "event_parser: missing required field 'rule_id'", "field": "rule_id"})
			return
		}
		if body.SubjectID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "event_parser: missing required field 'subject_id'", "field": "subject_id"})
			return
		}
		if body.SubjectKind == "" {
			body.SubjectKind = "incident"
		}

		// --- Operator allow-list gate (§11.4.21 / §11.4.66). Only enforced
		// when HERALD_OPERATOR_IDS is configured AND the rule is operator-gated.
		// A page filed by an operator outside the allow-list is rejected 403 —
		// it never reaches the pipeline. ---
		if operatorGatedRules[body.RuleID] {
			if allow, configured := operatorAllowList(); configured {
				if body.OperatorID == "" {
					c.JSON(http.StatusForbidden, gin.H{
						"error": "operator_id required for operator-gated escalation when HERALD_OPERATOR_IDS is configured",
						"field": "operator_id",
						"rule":  body.RuleID,
					})
					return
				}
				if !allow[body.OperatorID] {
					c.JSON(http.StatusForbidden, gin.H{
						"error":    "operator_id not in HERALD_OPERATOR_IDS allow-list",
						"field":    "operator_id",
						"rule":     body.RuleID,
						"operator": body.OperatorID,
					})
					return
				}
			}
		}

		// --- Drive the escalation through the pipeline. ---
		subject := constitution.Subject{Kind: body.SubjectKind, ID: body.SubjectID}
		out, err := pipeline.EvaluateSubject(c.Request.Context(), body.RuleID, tenantID, subject)
		if err != nil {
			// Unknown rule is the caller's fault (400, tagged `event_parser:`
			// so the operator can grep it like a pherald parse error); any
			// other pipeline error (store / emit / audit) is a dependency
			// failure (502). The handler never fabricates a success.
			if strings.Contains(err.Error(), "unknown rule") {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":  "event_parser: unknown rule_id",
					"field":  "rule_id",
					"detail": err.Error(),
				})
				return
			}
			c.JSON(http.StatusBadGateway, gin.H{"error": "page escalation failed", "detail": err.Error()})
			return
		}

		rcpt := Receipt{
			TenantID:     tenantID.String(),
			RuleID:       body.RuleID,
			Subject:      subject.String(),
			Decision:     decisionString(out.Decision),
			Mode:         out.Mode.String(),
			Escalation:   escalationKindFor(body.RuleID),
			Emitted:      out.Emitted,
			Audited:      out.Audited,
			Changed:      out.Transition.Changed,
			FirstSeen:    out.Transition.FirstSeen,
			TransitionTo: decisionString(out.Transition.NewDecision),
			PagerNote:    pagerSeamNote,
		}
		if out.PanicValue != "" {
			rcpt.Panic = out.PanicValue
		}
		c.JSON(http.StatusAccepted, rcpt)
	}
}

// operatorAllowList parses HERALD_OPERATOR_IDS into a set. Returns
// (set, configured) where configured is false when the env var is unset/empty
// (the gate is OPEN). Splits on comma or any whitespace so both
// "op1,op2" and "op1 op2" forms work; empty fields are dropped.
func operatorAllowList() (set map[string]bool, configured bool) {
	raw := strings.TrimSpace(os.Getenv(operatorEnvVar))
	if raw == "" {
		return nil, false
	}
	set = map[string]bool{}
	for _, f := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	}) {
		f = strings.TrimSpace(f)
		if f != "" {
			set[f] = true
		}
	}
	// All-whitespace/all-comma content → still "configured" but empty set
	// (deny-all) would be surprising; treat an empty parse result as
	// not-configured (open) so a stray "," doesn't lock everyone out.
	if len(set) == 0 {
		return nil, false
	}
	return set, true
}

// escalationKindFor returns the EscalationKind declared on the rule's spec so
// the Receipt names the page-out surface that fired. Falls back to the rule ID
// when the rule isn't in the catalogue (defensive — EvaluateSubject would have
// already rejected an unknown rule, so this is belt-and-suspenders).
func escalationKindFor(ruleID string) string {
	for _, spec := range bindings.IheraldRules() {
		if spec.RuleID == ruleID {
			if spec.EscalationKind != "" {
				return spec.EscalationKind
			}
			return ruleID
		}
	}
	return ruleID
}

// decisionString maps a constitution.Decision to the public Receipt
// vocabulary. Mirrors cherald/internal/compliance.decisionString — a FAIL
// surfaces as "deny" to keep the public verb consistent across flavors.
func decisionString(d constitution.Decision) string {
	switch d {
	case constitution.DecisionPass:
		return "pass"
	case constitution.DecisionWarn:
		return "warn"
	case constitution.DecisionFail:
		return "deny"
	case constitution.DecisionError:
		return "error"
	case constitution.DecisionSkip:
		return "skip"
	}
	return "unknown"
}
