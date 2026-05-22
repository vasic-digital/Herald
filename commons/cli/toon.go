// Wave 4b Task 4 — Gin TOON encode/decode middleware.
//
// This file is the consumer-facing seam between Herald's flavor
// handlers (which still write `c.JSON(status, obj)` like every other
// Gin app) and the content-negotiation primitive in
// commons/cli/contentnego.go. The middleware swaps Gin's
// ResponseWriter for a buffering wrapper so that, after the handler
// finishes, we can:
//
//  1. Inspect Content-Type written by c.JSON (`application/json; charset=utf-8`).
//  2. Compare against the negotiated response codec (driven by the
//     request's Accept header + HERALD_DEFAULT_RESPONSE_CODEC env).
//  3. If the client preferred TOON, JSON-decode the buffered body into
//     `interface{}` and re-encode via MarshalChosen("application/toon").
//  4. Rewrite the Content-Type header AND the body before flushing to
//     the underlying ResponseWriter.
//
// BindNegotiated is the symmetric request-side helper handlers can use
// instead of c.BindJSON when they need to honour an inbound
// Content-Type of application/toon.
//
// §107 anti-bluff watershed (load-bearing):
//
//   - The middleware NEVER writes JSON bytes under an `application/toon`
//     Content-Type. The transcode step is the structural guarantee:
//     either MarshalChosen succeeds and we emit real TOON bytes (the
//     paired test asserts byte-0 is not `{`) OR we keep the original
//     JSON body AND restore the `application/json` Content-Type header.
//     There is no third path. The 2026-05-17 PASS-bluff (JSON bytes
//     wearing an application/toon CT) is structurally impossible here.
//   - The round-trip test (TestTOONMiddleware_RoundTrip_StructEqual)
//     asserts reflect.DeepEqual against the original Receipt struct,
//     not "no error". The size test
//     (TestTOONMiddleware_RoundTrip_SizeSmaller) asserts TOON bytes are
//     strictly smaller than JSON for the same struct — proving real
//     codec benefit, not a header swap.
//
// Constitutional anchors: Universal §11.4 (PASS-bluff prevention),
// Herald §107 (end-user-usability covenant), W4b plan Task 4.
package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
)

// TOONMiddleware returns a Gin handler that intercepts response bodies
// emitted via c.JSON (or c.Render with render.JSON) and transcodes them
// to TOON when the client's Accept header (or the Herald-default codec
// env) prefers application/toon.
//
// Flow:
//
//  1. Negotiate response codec from Accept + HERALD_DEFAULT_RESPONSE_CODEC.
//  2. Wrap c.Writer in toonResponseWriter; handlers see no difference —
//     they still call c.JSON(status, obj).
//  3. After c.Next(), inspect the buffered body's Content-Type:
//       - If the writer recorded application/json AND the negotiated
//         codec is application/toon, JSON-decode the body into a
//         generic interface{}, then MarshalChosen → TOON bytes.
//       - Otherwise, pass through unchanged.
//  4. Flush to the underlying ResponseWriter with the correct
//     Content-Type + status + bytes.
//
// Why this design over a Respond(c, status, obj) helper?
//
//   - It is a drop-in: every existing handler that calls c.JSON keeps
//     working. The W4b plan's original sketch (Respond helper) requires
//     editing every handler; the middleware approach narrows the blast
//     radius of W4b T5/T6 to "add the middleware to the chain".
//   - The transcode is bounded by JSON round-trip semantics (numbers
//     become float64, etc.) — for Herald's payloads (Receipts, Compliance
//     decisions, Safety states) those round-trip losslessly. If a flavor
//     ever ships a payload that doesn't round-trip cleanly, the per-
//     handler escape hatch is to call MarshalChosen directly + c.Data.
//
// Non-goals (deliberate):
//
//   - This middleware does NOT compress (Brotli is a separate layer).
//   - It does NOT touch responses that were never set as JSON
//     (e.g., MetricsHandler emits text/plain; the middleware passes
//     those through unchanged).
//   - It does NOT short-circuit when negotiation returns JSON — the
//     pass-through path is identical, just no transcode.
func TOONMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		responseCT, _ := NegotiateContentType(c.GetHeader("Accept"), c.GetHeader("Content-Type"))
		original := c.Writer
		tw := &toonResponseWriter{
			ResponseWriter: original,
			buf:            &bytes.Buffer{},
			negotiatedCT:   responseCT,
		}
		c.Writer = tw
		defer func() {
			// Restore even if the handler panics so downstream
			// recovery middleware sees the original writer.
			c.Writer = original
		}()
		c.Next()
		tw.finish()
	}
}

// BindNegotiated reads the request body and decodes it into v using the
// codec implied by the request's Content-Type header. Handlers use this
// instead of c.BindJSON when they need to accept either JSON or TOON
// request bodies.
//
// Behaviour:
//   - Content-Type: application/toon       → TOON decode
//   - Content-Type: application/json       → JSON decode
//   - Content-Type: empty / "*/*"          → JSON decode (curl -d '...'
//                                              compat per W4b operator
//                                              decision 2)
//   - Content-Type: application/xml etc.   → JSON decode (best-effort
//                                              fallback; UnmarshalChosen
//                                              will surface a clear error
//                                              if the bytes are not JSON)
//   - empty body                           → returns nil, leaves v
//                                              untouched (handler decides
//                                              whether absence is OK)
//   - read or decode error                 → returns a wrapped error
//                                              naming the codec for the
//                                              caller to map to 4xx
func BindNegotiated(c *gin.Context, v any) error {
	if v == nil {
		return errors.New("toon middleware: nil target for BindNegotiated")
	}
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return errors.New("toon middleware: nil request body")
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return fmt.Errorf("toon middleware: read body: %w", err)
	}
	if len(body) == 0 {
		return nil
	}
	_, requestCT := NegotiateContentType(c.GetHeader("Accept"), c.GetHeader("Content-Type"))
	if err := UnmarshalChosen(body, v, requestCT); err != nil {
		return fmt.Errorf("toon middleware: unmarshal as %s: %w", requestCT, err)
	}
	return nil
}

// toonResponseWriter buffers writes from upstream handlers (typically
// c.JSON → render.JSON.Render → Writer.Write) so the middleware can
// inspect the Content-Type + bytes after c.Next() completes and decide
// whether to transcode.
//
// Implements the full gin.ResponseWriter interface so it can be
// transparently substituted for the original c.Writer.
type toonResponseWriter struct {
	gin.ResponseWriter

	buf          *bytes.Buffer
	negotiatedCT string
	statusCode   int
	wroteHeader  bool
}

// Write captures the body into the internal buffer. The actual write
// to the underlying ResponseWriter happens in finish() after we know
// the final Content-Type.
func (tw *toonResponseWriter) Write(b []byte) (int, error) {
	if !tw.wroteHeader {
		tw.statusCode = http.StatusOK
		tw.wroteHeader = true
	}
	return tw.buf.Write(b)
}

// WriteString mirrors Write.
func (tw *toonResponseWriter) WriteString(s string) (int, error) {
	return tw.Write([]byte(s))
}

// WriteHeader records the status code; the actual call on the
// underlying writer happens in finish().
func (tw *toonResponseWriter) WriteHeader(code int) {
	tw.statusCode = code
	tw.wroteHeader = true
}

// WriteHeaderNow is a hook Gin uses to force-flush headers (e.g., for
// bodyless statuses). We absorb it; finish() will perform the actual
// flush once we know the final Content-Type.
func (tw *toonResponseWriter) WriteHeaderNow() {
	// Intentional no-op — finish() owns the underlying WriteHeaderNow.
}

// Status returns the recorded status so downstream middleware (e.g.,
// access logging) can observe it through the Gin contract.
func (tw *toonResponseWriter) Status() int {
	if tw.statusCode == 0 {
		return http.StatusOK
	}
	return tw.statusCode
}

// Size returns the buffered (pre-transcode) body size. Matches the
// gin.ResponseWriter convention (count of body bytes the application
// wrote, not the on-the-wire size after transcode).
func (tw *toonResponseWriter) Size() int {
	return tw.buf.Len()
}

// Written matches gin.ResponseWriter — true once any body bytes or
// status code have been recorded.
func (tw *toonResponseWriter) Written() bool {
	return tw.wroteHeader || tw.buf.Len() > 0
}

// Hijack rejects hijacking — the buffer-then-rewrite design is
// incompatible with connection takeover (e.g., websocket upgrade).
// Callers needing hijack should omit TOONMiddleware from that route's
// chain.
func (tw *toonResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, errors.New("toon middleware: hijack not supported")
}

// Flush is a no-op while buffering — codec transcode is a final-pass
// operation. Gin's default handlers do not Flush mid-response; if a
// flavor route needs streaming, omit TOONMiddleware from its chain.
func (tw *toonResponseWriter) Flush() {}

// finish decides whether to transcode the buffered body and then
// flushes through to the underlying ResponseWriter with the correct
// Content-Type, status, and bytes.
//
// Decision matrix (Content-Type recorded by handler × negotiated CT):
//
//	recorded\negotiated | application/toon            | application/json
//	--------------------+-----------------------------+-----------------
//	application/json    | TRANSCODE: JSON→TOON        | passthrough
//	application/toon    | passthrough (already TOON)  | passthrough  (a)
//	other (text/plain…) | passthrough                 | passthrough
//
// (a) If a handler somehow already emitted TOON but the client preferred
// JSON, we passthrough rather than transcode TOON→JSON. The negotiated
// codec is the SERVER's intent; an explicit handler choice (e.g., a
// handler that always emits TOON for tabular endpoints) is honoured.
// In practice no Herald handler does this today — handlers call c.JSON.
func (tw *toonResponseWriter) finish() {
	body := tw.buf.Bytes()
	if len(body) == 0 {
		// Bodyless response (e.g., a 204 No Content). Flush headers
		// + status only.
		if tw.wroteHeader {
			tw.ResponseWriter.WriteHeader(tw.statusCode)
		}
		return
	}

	recordedCT := canonicalCT(tw.ResponseWriter.Header().Get("Content-Type"))
	out := body
	finalCT := tw.ResponseWriter.Header().Get("Content-Type")

	if recordedCT == MediaTypeJSON && tw.negotiatedCT == MediaTypeTOON {
		// Decode JSON into a codec-neutral representation, then
		// re-encode via the negotiated codec (TOON).
		var generic any
		if err := json.Unmarshal(body, &generic); err == nil {
			if encoded, encErr := MarshalChosen(generic, MediaTypeTOON); encErr == nil && len(encoded) > 0 {
				out = encoded
				finalCT = MediaTypeTOON
			}
			// If MarshalChosen fails, fall through with the original
			// JSON body. This is the §107 honest-fallback path: we
			// MUST NOT write JSON bytes under a `Content-Type:
			// application/toon` header. The Content-Type stays JSON.
		}
		// If json.Unmarshal fails, the body isn't valid JSON despite
		// the Content-Type — flush as-is so the operator can see the
		// real bytes (Universal §11.4 transparency).
	}

	// Update the Content-Type header BEFORE WriteHeader (RFC 7230 —
	// headers must be sent before the body; Gin coalesces them at
	// WriteHeader time).
	tw.ResponseWriter.Header().Set("Content-Type", finalCT)
	tw.ResponseWriter.Header().Del("Content-Length") // length changed; let net/http re-derive

	if tw.wroteHeader {
		tw.ResponseWriter.WriteHeader(tw.statusCode)
	}
	_, _ = tw.ResponseWriter.Write(out)
}

