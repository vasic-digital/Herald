// Package compliance implements cherald's GET /v1/compliance handler
// per V3 §41 and the Wave 3 design (Section 3).
//
// The handler queries commons_constitution.ConstitutionStore (Memory in
// tests, Postgres at runtime) with tenant scope extracted from the JWT
// claims that commons_auth.GinMiddleware set on the Gin context.
//
// Query-param surface (all optional):
//   - rule_id:   exact-match filter on rule ID.
//   - decision:  one of pass|allow|warn|deny|fail|all (deny+fail are aliases).
//   - since:     RFC3339 lower bound on TransitionedAt (inclusive).
//   - until:     RFC3339 upper bound on TransitionedAt (inclusive).
//   - page:      1-based page number; default 1.
//   - page_size: rows per page; default 50, max 200 (400 on overflow).
//
// Response shape (200):
//
//	{
//	  "page":      <int>,
//	  "page_size": <int>,
//	  "total":     <int>,     // total rows matching filters (pre-pagination)
//	  "tenant_id": "<uuid>",
//	  "results":   [ { rule_id, subject, decision, digest_sha,
//	                    bundle_hash, evidence_uri, transitioned_at }, ... ]
//	}
//
// §107 anti-bluff: this handler does NOT return success on missing/bad
// auth — 401 if no claims, 401 if tenant claim is malformed, 400 if any
// query param is malformed, 500 on store errors. Operator gets a typed
// error body naming the field that failed.
package compliance

import (
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/vasic-digital/herald/commons_auth"
	constitution "github.com/vasic-digital/herald/commons_constitution"
)

const (
	defaultPageSize = 50
	maxPageSize     = 200
)

// Handler returns the gin.HandlerFunc serving GET /v1/compliance.
// Reads ConstitutionStore via store; expects commons_auth.GinMiddleware
// (or a test-only equivalent) to have populated
// commons_auth.ContextKeyClaims upstream.
func Handler(store constitution.ConstitutionStore) gin.HandlerFunc {
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
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":  "bad tenant claim",
				"detail": err.Error(),
			})
			return
		}

		q := constitution.ListQuery{}
		if rid := c.Query("rule_id"); rid != "" {
			q.RuleID = rid
		}
		if subj := c.Query("subject"); subj != "" {
			q.Subject = subj
		}
		if dec := c.Query("decision"); dec != "" && dec != "all" {
			d, perr := parseDecision(dec)
			if perr != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":  "invalid decision",
					"field":  "decision",
					"detail": perr.Error(),
				})
				return
			}
			q.Decision = &d
		}
		if since := c.Query("since"); since != "" {
			t, perr := time.Parse(time.RFC3339, since)
			if perr != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "invalid time format",
					"field": "since",
				})
				return
			}
			q.Since = t
		}
		if until := c.Query("until"); until != "" {
			t, perr := time.Parse(time.RFC3339, until)
			if perr != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "invalid time format",
					"field": "until",
				})
				return
			}
			q.Until = t
		}
		page := 1
		if p := c.Query("page"); p != "" {
			n, perr := strconv.Atoi(p)
			if perr != nil || n < 1 {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "invalid page",
					"field": "page",
				})
				return
			}
			page = n
		}
		pageSize := defaultPageSize
		if ps := c.Query("page_size"); ps != "" {
			n, perr := strconv.Atoi(ps)
			if perr != nil || n < 1 {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "invalid page_size",
					"field": "page_size",
				})
				return
			}
			if n > maxPageSize {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":    "page_size exceeds maximum",
					"field":    "page_size",
					"max":      maxPageSize,
					"received": n,
				})
				return
			}
			pageSize = n
		}
		q.Limit = pageSize
		q.Offset = (page - 1) * pageSize

		// Note: at runtime cherald main.go will wrap c.Request.Context()
		// with commons_storage.SetTenantContext BEFORE calling the
		// handler chain so RLS is set for the Postgres store. The Memory
		// backend ignores tenant context, so tests pass either way.
		ctx := c.Request.Context()
		rows, err := store.List(ctx, tenantID, q)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":  "store list failed",
				"detail": err.Error(),
			})
			return
		}

		// total count: re-run a List with no limit/offset to get a count.
		// Memory backend is cheap; Postgres should use a COUNT(*) query.
		// For Wave 3a we use the simple approach; optimize in Wave 4 if needed.
		all, err := store.List(ctx, tenantID, constitution.ListQuery{
			RuleID:   q.RuleID,
			Subject:  q.Subject,
			Decision: q.Decision,
			Since:    q.Since,
			Until:    q.Until,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":  "store count failed",
				"detail": err.Error(),
			})
			return
		}

		results := make([]gin.H, 0, len(rows))
		for _, r := range rows {
			results = append(results, gin.H{
				"rule_id":         r.RuleID,
				"subject":         r.Subject,
				"decision":        decisionString(r.Decision),
				"digest_sha":      hex.EncodeToString(r.Digest[:]),
				"bundle_hash":     hex.EncodeToString(r.BundleHash[:]),
				"evidence_uri":    r.EvidenceURI,
				"transitioned_at": r.TransitionedAt.Format(time.RFC3339Nano),
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"page":      page,
			"page_size": pageSize,
			"total":     len(all),
			"tenant_id": tenantID.String(),
			"results":   results,
		})
	}
}

// parseDecision maps the public string vocabulary onto the
// commons_constitution.Decision enum. "allow" is a synonym for "pass";
// "deny" is a synonym for "fail" (the API uses the operator-facing word
// "deny" while the internal enum is DecisionFail — kept aligned with the
// Wave 3 design's external surface and the §41 spec wording).
func parseDecision(s string) (constitution.Decision, error) {
	switch s {
	case "pass", "allow":
		return constitution.DecisionPass, nil
	case "warn":
		return constitution.DecisionWarn, nil
	case "deny", "fail":
		return constitution.DecisionFail, nil
	}
	return 0, errors.New("decision must be one of: pass|allow|warn|deny|fail|all; got " + s)
}

// decisionString is the inverse of parseDecision for the response body.
// We surface "deny" (not "fail") to keep the public vocabulary consistent
// with the query-param surface.
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
