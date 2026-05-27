package compliance

// evaluate.go — HRD-019: the binding-evaluate write surface for cherald.
//
// POST /v1/compliance/evaluate drives one cherald constitution binding against
// a caller-supplied subject through the bindings.Pipeline (the Batch-A
// Evaluator + Runner-gate + emit + audit foundation). A FAIL transition in
// enforce mode emits the rule's event class AND persists a constitution_state
// row, which immediately becomes visible on GET /v1/compliance — closing the
// detect→emit→persist→query loop the §107 covenant requires.
//
// Request body (JSON):
//
//	{ "rule_id": "§11.4.29", "subject_kind": "file", "subject_id": "a/B.go" }
//
// Response (200):
//
//	{ "tenant_id", "rule_id", "subject", "decision", "mode",
//	  "emitted", "audited", "changed", "first_seen", "transition_to" }
//
// §107 anti-bluff: 401 on missing/bad auth, 400 on a malformed body / empty
// fields / unknown rule_id, 502 on a pipeline (store/emit/audit) failure. The
// handler NEVER fabricates a success — a pipeline error surfaces as a typed 502.

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/vasic-digital/herald/cherald/internal/bindings"
	"github.com/vasic-digital/herald/commons_auth"
	constitution "github.com/vasic-digital/herald/commons_constitution"
)

// evaluateBody is the POST /v1/compliance/evaluate request body.
type evaluateBody struct {
	RuleID      string `json:"rule_id"`
	SubjectKind string `json:"subject_kind"`
	SubjectID   string `json:"subject_id"`
}

// EvaluateHandler returns the gin.HandlerFunc serving POST
// /v1/compliance/evaluate, backed by a bindings.Pipeline.
func EvaluateHandler(pipeline *bindings.Pipeline) gin.HandlerFunc {
	return func(c *gin.Context) {
		claimsAny, ok := c.Get(commons_auth.ContextKeyClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "no auth claims"})
			return
		}
		claims, ok := claimsAny.(map[string]any)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "claims have wrong shape"})
			return
		}
		tenantID, err := commons_auth.TenantFromClaims(claims)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "bad tenant claim", "detail": err.Error()})
			return
		}

		var body evaluateBody
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body", "detail": err.Error()})
			return
		}
		body.RuleID = strings.TrimSpace(body.RuleID)
		body.SubjectKind = strings.TrimSpace(body.SubjectKind)
		body.SubjectID = strings.TrimSpace(body.SubjectID)
		if body.RuleID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "empty rule_id", "field": "rule_id"})
			return
		}
		if body.SubjectID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "empty subject_id", "field": "subject_id"})
			return
		}
		if body.SubjectKind == "" {
			body.SubjectKind = "file" // default kind
		}

		subject := constitution.Subject{Kind: body.SubjectKind, ID: body.SubjectID}
		out, err := pipeline.EvaluateSubject(c.Request.Context(), body.RuleID, tenantID, subject)
		if err != nil {
			// An unknown-rule error is the operator's fault (400); any other
			// pipeline error (store/emit/audit) is a dependency failure (502).
			if strings.Contains(err.Error(), "unknown rule") {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":  "unknown rule_id",
					"field":  "rule_id",
					"detail": err.Error(),
				})
				return
			}
			c.JSON(http.StatusBadGateway, gin.H{"error": "evaluate failed", "detail": err.Error()})
			return
		}

		resp := gin.H{
			"tenant_id":     tenantID.String(),
			"rule_id":       body.RuleID,
			"subject":       subject.String(),
			"decision":      decisionString(out.Decision),
			"mode":          out.Mode.String(),
			"emitted":       out.Emitted,
			"audited":       out.Audited,
			"changed":       out.Transition.Changed,
			"first_seen":    out.Transition.FirstSeen,
			"transition_to": out.Transition.NewDecision.String(),
		}
		if out.PanicValue != "" {
			resp["panic"] = out.PanicValue
		}
		c.JSON(http.StatusOK, resp)
	}
}

// errEvaluate is retained for symmetry with the package error vocabulary; the
// handler maps pipeline errors inline. Kept so future typed-error routing has
// a home without an import churn.
var errEvaluate = errors.New("compliance: evaluate")

var _ = errEvaluate
