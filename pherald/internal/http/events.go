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
//
// Wave 4b Task 5 (2026-05-22) — TOON content-type support:
//
//   - Request side: when the inbound Content-Type is application/toon, the
//     handler transcodes the body to JSON (via cli.UnmarshalChosen +
//     cli.MarshalChosen) BEFORE passing it to the Runner. The Runner stays
//     codec-agnostic (its EventParser still JSON-decodes inside
//     event_parser.go). This is "Option A" in the W4b plan §Task 5.
//   - Response side: the handler continues to emit c.JSON; the
//     TOONMiddleware mounted in the chain (pherald/cmd/pherald/stubs.go)
//     transcodes JSON → TOON when the negotiated response codec is TOON.
//
// §107 anti-bluff: TOON request bodies are decoded via the REAL
// digital.vasic.toon codec — not a string-comparison or grep-fallback. A
// malformed TOON body surfaces as `event_parser: …` after transcode (the
// Runner's existing 400 mapping handles it) OR as a 400 "toon transcode"
// at this layer if UnmarshalChosen rejects the bytes before the Runner.
package http

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/vasic-digital/herald/commons/cli"
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
		// Wave 4b Task 5 — TOON request-body support. If the client sent
		// `Content-Type: application/toon`, transcode the bytes to JSON
		// before handing to the Runner. The Runner's EventParser is
		// JSON-only (event_parser.go) by design — Option A in the W4b plan
		// keeps the Runner codec-agnostic and isolates the codec dispatch
		// to this HTTP adapter layer.
		//
		// §107 anti-bluff: the transcode goes through the REAL TOON codec
		// via cli.UnmarshalChosen + json.Marshal. A "JSON bytes wearing a
		// TOON content-type" body fails at UnmarshalChosen and surfaces as
		// 400 "toon transcode" — never silently treated as JSON.
		body, err = transcodeRequestBody(c, body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "toon transcode", "detail": err.Error()})
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

// transcodeRequestBody returns the request body as JSON bytes. When the
// inbound Content-Type is application/toon, the body is TOON-decoded into
// a generic any via cli.UnmarshalChosen, then re-encoded as JSON. Any
// other Content-Type (including the empty / `*/*` default per the W4b
// negotiation policy) passes through unchanged.
//
// Why a per-handler transcode rather than a request-side middleware
// counterpart to TOONMiddleware? Two reasons:
//
//  1. The Runner's EventParser owns the structuredCloudEvent struct shape
//     (event_parser.go). A request-side middleware that JSON-decoded the
//     TOON body into a fresh map[string]any and then JSON-re-encoded would
//     still need to land in the Runner as JSON bytes — exactly what this
//     helper does, but with the same generic round-trip. Keeping it in
//     the handler keeps the dispatch path local to the file the operator
//     reads when debugging /v1/events.
//  2. Other pherald routes (none today; metrics/healthz served by cli.*)
//     don't need request-body transcode at all. Mounting a wide request
//     middleware would force a no-op buffer copy on every route, which is
//     a §107 paper-cut bluff.
//
// §107 anti-bluff: UnmarshalChosen + json.Marshal is the load-bearing
// transcode path — the wire bytes for a TOON request are decoded via the
// REAL digital.vasic.toon codec (no JSON fallback that would mask a
// malformed body). The empty-body path returns nil bytes unchanged so the
// Runner's "event_parser: empty body" surfaces at its normal exit.
func transcodeRequestBody(c *gin.Context, body []byte) ([]byte, error) {
	if len(body) == 0 {
		return body, nil
	}
	_, requestCT := cli.NegotiateContentType(c.GetHeader("Accept"), c.GetHeader("Content-Type"))
	if requestCT != cli.MediaTypeTOON {
		// JSON (explicit) or default fallback (empty / */*) — pass through.
		return body, nil
	}
	var generic any
	if err := cli.UnmarshalChosen(body, &generic, cli.MediaTypeTOON); err != nil {
		return nil, err
	}
	// Re-encode as JSON so the Runner's EventParser (which is JSON-only
	// by design) sees the same struct shape regardless of the inbound
	// codec.
	transcoded, err := json.Marshal(generic)
	if err != nil {
		return nil, err
	}
	return transcoded, nil
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
