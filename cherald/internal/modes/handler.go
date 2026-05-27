// Package modes implements cherald's admin mode-ladder REST surface per
// spec V3 §42.1.4 + HRD-027: operators flip a constitution rule's
// enforcement mode (allow / warn / enforce) per binding per tenant WITHOUT
// a redeploy — the ladder reads constitution_bindings at evaluation time.
//
// Endpoints (all under /v1/compliance/modes, JWT-gated by
// commons_auth.GinMiddleware mounted in cherald serve):
//
//	GET  /v1/compliance/modes            → list every binding for the tenant
//	GET  /v1/compliance/modes/:rule_id   → one binding's mode (enforce default)
//	PUT  /v1/compliance/modes/:rule_id   → flip a binding's mode
//	      body: {"mode": "allow"|"warn"|"enforce"}
//
// §107 anti-bluff: 401 on missing/bad auth, 400 on a malformed rule_id or
// body, 502 on store errors. Mutations are durable before 200 returns — a
// subsequent GET (or a re-evaluation) reflects the new mode. The handler
// records the operator identity (the JWT `sub` claim) in the
// constitution_bindings.mutated_by audit column.
package modes

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/vasic-digital/herald/commons_auth"
	constitution "github.com/vasic-digital/herald/commons_constitution"
)

// ListHandler serves GET /v1/compliance/modes — every binding for the
// tenant, plus the implicit enforce-default note for unbound rules.
func ListHandler(ladder constitution.ModeLadder) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID, ok := tenantOr401(c)
		if !ok {
			return
		}
		bindings, err := ladder.List(c.Request.Context(), tenantID)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "ladder list failed", "detail": err.Error()})
			return
		}
		out := make(map[string]string, len(bindings))
		for ruleID, m := range bindings {
			out[ruleID] = m.String()
		}
		c.JSON(http.StatusOK, gin.H{
			"tenant_id":       tenantID.String(),
			"bindings":        out,
			"unbound_default": constitution.ModeEnforce.String(),
		})
	}
}

// GetHandler serves GET /v1/compliance/modes/:rule_id — one binding's
// effective mode (enforce when unbound, per §42.1.4 safe default).
func GetHandler(ladder constitution.ModeLadder) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID, ok := tenantOr401(c)
		if !ok {
			return
		}
		ruleID := c.Param("rule_id")
		if ruleID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "empty rule_id", "field": "rule_id"})
			return
		}
		m, err := ladder.Get(c.Request.Context(), tenantID, ruleID)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "ladder get failed", "detail": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"tenant_id": tenantID.String(),
			"rule_id":   ruleID,
			"mode":      m.String(),
		})
	}
}

// putBody is the PUT /v1/compliance/modes/:rule_id request body.
type putBody struct {
	Mode string `json:"mode"`
}

// PutHandler serves PUT /v1/compliance/modes/:rule_id — flip a binding's
// mode. The new mode takes effect on the NEXT evaluation with no redeploy.
func PutHandler(ladder constitution.ModeLadder) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID, ok := tenantOr401(c)
		if !ok {
			return
		}
		ruleID := c.Param("rule_id")
		if ruleID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "empty rule_id", "field": "rule_id"})
			return
		}
		var body putBody
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body", "detail": err.Error()})
			return
		}
		mode, perr := constitution.ParseMode(body.Mode)
		if perr != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":    "invalid mode",
				"field":    "mode",
				"detail":   perr.Error(),
				"accepted": []string{"allow", "warn", "enforce"},
			})
			return
		}

		// Operator identity for the constitution_bindings.mutated_by audit
		// column comes from the JWT `sub` claim.
		by := operatorFromClaims(c)

		if err := ladder.Set(c.Request.Context(), tenantID, ruleID, mode, by); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "ladder set failed", "detail": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"tenant_id":  tenantID.String(),
			"rule_id":    ruleID,
			"mode":       mode.String(),
			"mutated_by": by,
		})
	}
}

// tenantOr401 extracts + validates the tenant claim, short-circuiting with a
// typed 401 on any failure (mirrors compliance.Handler's posture).
func tenantOr401(c *gin.Context) (tenant uuid.UUID, ok bool) {
	claimsAny, present := c.Get(commons_auth.ContextKeyClaims)
	if !present {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "no auth claims"})
		return uuid.Nil, false
	}
	claims, isMap := claimsAny.(map[string]any)
	if !isMap {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "claims have wrong shape"})
		return uuid.Nil, false
	}
	tid, err := commons_auth.TenantFromClaims(claims)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "bad tenant claim", "detail": err.Error()})
		return uuid.Nil, false
	}
	return tid, true
}

func operatorFromClaims(c *gin.Context) string {
	claimsAny, ok := c.Get(commons_auth.ContextKeyClaims)
	if !ok {
		return "unknown"
	}
	claims, ok := claimsAny.(map[string]any)
	if !ok {
		return "unknown"
	}
	if sub, err := commons_auth.SubjectFromClaims(claims); err == nil && sub != "" {
		return sub
	}
	return "unknown"
}
