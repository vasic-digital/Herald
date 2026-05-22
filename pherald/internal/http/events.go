// Wave 3b Task 10 — pherald POST /v1/events live handler.
//
// Flips the Wave 2 r1 501-stub (registered via cli.Route{HRD:"HRD-016"}) to
// a real handler that drives the §32 7-stage Runner pipeline. The handler
// extracts JWT claims placed by commons_auth.GinMiddleware, reads the raw
// CloudEvent body, calls Runner.Run, and translates the Receipt into the
// HTTP response per spec §41:
//
//   - Fresh accepted event              → 202 Accepted + Receipt JSON
//   - Replay (idempotency hit)           → 200 OK + Receipt + X-Herald-Replay: true header
//   - Auth claim missing/malformed       → 401 Unauthorized
//   - CloudEvent parse / tenant resolve  → 400 Bad Request
//   - Policy DecisionFail                → handled inside Runner — RecordDenied still
//                                          returns a Receipt; we surface it as 202 so the
//                                          archive row is acknowledged
//                                          (the denial detail lives inside the Receipt)
//   - Any other stage error              → 500 Internal Server Error
//
// §107 anti-bluff: the failure paths return tagged error strings so an
// operator inspecting the body can distinguish a stage-2 idempotency Redis
// outage from a stage-7 PG write failure — silently swallowing errors with
// 500 + "internal error" would mask real bugs.
package http

import (
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/vasic-digital/herald/commons_auth"
	"github.com/vasic-digital/herald/pherald/internal/runner"
)

// EventsHandler returns the gin.HandlerFunc serving POST /v1/events.
//
// Wiring path (post-Wave 3b):
//
//	commons_auth.GinMiddleware → EventsHandler → runner.Runner.Run →
//	{ EventParser, IdempotencyChecker, TenantResolver, PolicyGate,
//	  SubscriberResolver, ChannelDispatcher, OutcomeRecorder }
//
// The handler is the thinnest possible adapter between the HTTP plane and
// the Runner — all business logic lives in runner/.
func EventsHandler(r *runner.Runner) gin.HandlerFunc {
	return func(c *gin.Context) {
		claimsAny, ok := c.Get(commons_auth.ContextKeyClaims)
		if !ok {
			// Should never happen if GinMiddleware ran — but if a route
			// shipped without auth, fail loud rather than process anonymously.
			c.JSON(http.StatusUnauthorized, gin.H{"error": "no auth claims in context"})
			return
		}
		claims, claimsOK := claimsAny.(map[string]any)
		if !claimsOK {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "auth claims wrong shape"})
			return
		}
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "read body", "detail": err.Error()})
			return
		}
		rcpt, err := r.Run(c.Request.Context(), body, claims)
		if err != nil {
			c.JSON(mapErrorToStatus(err), gin.H{"error": err.Error()})
			return
		}
		if rcpt == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "runner returned nil receipt"})
			return
		}
		if rcpt.WasReplay {
			c.Header("X-Herald-Replay", "true")
			c.JSON(http.StatusOK, rcpt)
			return
		}
		c.JSON(http.StatusAccepted, rcpt)
	}
}

// mapErrorToStatus inspects the Runner error and translates it to an HTTP
// status. The Runner tags errors with their stage prefix ("event_parser:",
// "tenant_resolver:", "runner: claims missing 'tenant'") so the mapping is
// purely string-based — no error-type assertions, no fragile sentinel
// values. Unknown errors are 500 (default) so a new error class doesn't
// accidentally leak as 400.
func mapErrorToStatus(err error) int {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "event_parser:"):
		return http.StatusBadRequest
	case strings.Contains(msg, "runner: claims missing 'tenant'"),
		strings.Contains(msg, "'tenant' claim not a string"),
		strings.Contains(msg, "runner: parse 'tenant' claim"):
		return http.StatusUnauthorized
	case strings.Contains(msg, "tenant_resolver:"):
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}
