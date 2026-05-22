// Wave 4a Task 5 — Brotli Gin middleware.
//
// BrotliMiddleware is a Gin-native Brotli compression middleware that
// shares its policy surface (Config, compressible-types list, MinLength
// default) with digital.vasic.middleware/pkg/brotli. The upstream
// middleware ships as a net/http handler; we cannot adapt it via the
// digital.vasic.middleware/pkg/gin.Wrap helper because Wrap does NOT
// propagate the wrapped ResponseWriter to Gin's c.Next() chain — Gin
// handlers write directly to c.Writer, bypassing the brotli buffer.
// (The upstream Wrap helper is correct for header-only middleware like
// CORS, RequestID, AltSvc — those set headers BEFORE c.Next() runs and
// the headers live on the shared c.Writer.Header() map. It cannot
// handle response-body-rewriting middleware.)
//
// Per CONST-051(B): this file does NOT introduce Herald-specific
// opinion into the digital.vasic.middleware submodule. Instead, it
// composes the upstream Config + andybalholm/brotli encoder into a
// Gin-native middleware that respects Gin's ResponseWriter contract
// (status, size, written, hijacker etc.). Future work could lift the
// Gin adapter into the middleware submodule (as a sibling to pkg/gin)
// — flagged for §11.4.35 promotion review when the second consumer
// appears.
//
// Per §107 / Sixth Law: the load-bearing assertion in brotli_test.go is
// NOT just "Content-Encoding: br header is present" — it is a real
// round-trip through github.com/andybalholm/brotli.NewReader decoder
// followed by a byte-comparison against the source payload. A buggy
// implementation that sets the header but writes gibberish (or even
// shorter gibberish) would slip past a header-only assertion; it CANNOT
// slip past decode-and-compare.

package cli

import (
	"bufio"
	"bytes"
	"errors"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/andybalholm/brotli"
	brotlimw "digital.vasic.middleware/pkg/brotli"
	"github.com/gin-gonic/gin"
)

// HeraldBrotliDefaultLevel is the operator-locked default Brotli quality
// used by all Herald flavors when no operator override is supplied. 6 is
// one notch above the andybalholm/brotli library default (5); balanced
// CPU/ratio for Herald's small responses (256B-10KiB).
//
// The full quality range per RFC 7932 is 0 (no compression — store
// only) through 11 (maximum compression). Higher = slower encode, lower
// = faster encode + larger output. Decode cost is roughly constant.
const HeraldBrotliDefaultLevel = 6

// brotliMaxLevel is the upper bound of the valid Brotli quality range
// per the andybalholm/brotli library + RFC 7932. Operator-supplied env
// values outside [0, 11] fall back to the operator-locked default.
const brotliMaxLevel = 11

// BrotliMiddleware returns a Gin handler that compresses response
// bodies with Brotli (quality `quality`) when the client's
// Accept-Encoding includes "br" AND the response body is at least 256
// bytes AND the Content-Type is in the upstream's compressible-types
// list (application/json + text/* + the standard upstream defaults).
//
// Quality resolution:
//
//   - quality > 0: use that level directly (clamped to [0, 11] —
//     out-of-range falls back to the operator-locked default).
//   - quality == 0: read HERALD_BROTLI_LEVEL env var; parse + clamp;
//     if env unset OR unparseable OR out-of-range, default to 6.
//
// Per §107: the test brotli_test.go MUST decode the response body via
// github.com/andybalholm/brotli.NewReader and byte-compare it against
// the source. An implementation that sets Content-Encoding: br but
// writes identity bytes (a real failure mode if a middleware bug
// short-circuits the encoder) would pass a header-only assertion but
// fail decode-and-compare.
func BrotliMiddleware(quality int) gin.HandlerFunc {
	cfg := brotlimw.DefaultConfig()
	cfg.Level = resolveBrotliLevel(quality)
	return func(c *gin.Context) {
		// Identity passthrough when the client did not advertise "br".
		if !clientAcceptsBrotli(c.Request) {
			c.Next()
			return
		}
		// Swap the Gin ResponseWriter with a buffering wrapper. Gin
		// handlers will write into the buffer; we decide on c.Next()
		// completion whether to flush identity (sub-MinLength /
		// non-compressible-type) or Brotli-compressed.
		original := c.Writer
		bw := &brotliResponseWriter{
			ResponseWriter: original,
			cfg:            cfg,
			buf:            &bytes.Buffer{},
		}
		c.Writer = bw
		defer func() {
			// Restore even if the handler panics so downstream
			// recovery middleware sees the original writer.
			c.Writer = original
		}()
		c.Next()
		bw.finish()
	}
}

// brotliResponseWriter implements gin.ResponseWriter by buffering all
// writes until finish() is called. On finish(), it inspects the body
// size + content-type and either flushes identity bytes through the
// original writer or Brotli-compresses then writes the compressed form
// with the appropriate Content-Encoding / Vary headers.
type brotliResponseWriter struct {
	gin.ResponseWriter
	cfg         *brotlimw.Config
	buf         *bytes.Buffer
	wroteHeader bool
	statusCode  int
}

// Write captures the body into the internal buffer. The actual write
// to the underlying ResponseWriter happens in finish() after we know
// the final size + content-type.
func (bw *brotliResponseWriter) Write(b []byte) (int, error) {
	if !bw.wroteHeader {
		bw.statusCode = http.StatusOK
		bw.wroteHeader = true
	}
	return bw.buf.Write(b)
}

// WriteString matches the gin.ResponseWriter contract.
func (bw *brotliResponseWriter) WriteString(s string) (int, error) {
	return bw.Write([]byte(s))
}

// WriteHeader records the status code; the actual call on the
// underlying writer happens in finish().
func (bw *brotliResponseWriter) WriteHeader(code int) {
	bw.statusCode = code
	bw.wroteHeader = true
}

// Status returns the recorded status so middleware downstream of us
// (e.g., access logging) can observe it through the Gin contract.
func (bw *brotliResponseWriter) Status() int {
	if bw.statusCode == 0 {
		return http.StatusOK
	}
	return bw.statusCode
}

// Size returns the buffered (pre-compression) body size — matches the
// gin.ResponseWriter convention (number of body bytes the application
// wrote, not the on-the-wire size).
func (bw *brotliResponseWriter) Size() int {
	return bw.buf.Len()
}

// Written matches gin.ResponseWriter — true once any body bytes or
// status code have been recorded.
func (bw *brotliResponseWriter) Written() bool {
	return bw.wroteHeader || bw.buf.Len() > 0
}

// Hijack rejects hijacking — Brotli compression is incompatible with
// connection takeover (e.g., websocket upgrade). Callers needing
// hijack should ensure no Brotli middleware is on the path.
func (bw *brotliResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, errors.New("brotli middleware: hijack not supported")
}

// Flush is a no-op while buffering — Brotli compression is final-pass
// only. Gin's default handlers do not Flush mid-response; if a flavor
// route needs streaming, omit BrotliMiddleware from its chain.
func (bw *brotliResponseWriter) Flush() {}

// finish decides identity-vs-compressed and writes through to the
// original ResponseWriter. Skips compression when the body is below
// MinLength OR the Content-Type is not in the configured compressible
// list — same policy as digital.vasic.middleware/pkg/brotli.
func (bw *brotliResponseWriter) finish() {
	body := bw.buf.Bytes()
	ct := bw.ResponseWriter.Header().Get("Content-Type")
	if len(body) < bw.cfg.MinLength || !isCompressibleContentType(ct, bw.cfg.CompressibleTypes) {
		if bw.wroteHeader {
			bw.ResponseWriter.WriteHeader(bw.statusCode)
		}
		_, _ = bw.ResponseWriter.Write(body)
		return
	}
	var compressed bytes.Buffer
	writer := brotli.NewWriterLevel(&compressed, bw.cfg.Level)
	_, _ = writer.Write(body)
	_ = writer.Close()
	bw.ResponseWriter.Header().Set("Content-Encoding", "br")
	bw.ResponseWriter.Header().Add("Vary", "Accept-Encoding")
	bw.ResponseWriter.Header().Del("Content-Length")
	if bw.wroteHeader {
		bw.ResponseWriter.WriteHeader(bw.statusCode)
	}
	_, _ = bw.ResponseWriter.Write(compressed.Bytes())
}

// clientAcceptsBrotli mirrors the upstream's acceptsBrotli — true iff
// the Accept-Encoding header contains "br" as a substring (the upstream
// uses substring match rather than a strict q-value parse; we match
// that policy exactly for consistency).
func clientAcceptsBrotli(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept-Encoding"), "br")
}

// isCompressibleContentType returns true when ct starts with any of
// the configured compressible prefixes. Matches the upstream's
// isCompressible policy (case-insensitive prefix match).
func isCompressibleContentType(ct string, prefixes []string) bool {
	if ct == "" {
		return false
	}
	lower := strings.ToLower(ct)
	for _, p := range prefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

// resolveBrotliLevel implements the quality-resolution policy:
//
//  1. If the caller passed quality > 0 in valid range [0, 11], use it.
//  2. If quality == 0 (or out of range), consult HERALD_BROTLI_LEVEL
//     env var; if set + parseable + in range, use it.
//  3. Otherwise return HeraldBrotliDefaultLevel (6).
//
// Note: callers wanting "no compression" (level 0) MUST pass a special
// sentinel or wrap differently; quality == 0 here means "use default
// resolution" (env-or-default), not "level 0 / store only". This is a
// deliberate API choice — level 0 is rarely useful for an HTTP server.
func resolveBrotliLevel(quality int) int {
	if quality > 0 && quality <= brotliMaxLevel {
		return quality
	}
	if env := os.Getenv("HERALD_BROTLI_LEVEL"); env != "" {
		if n, err := strconv.Atoi(env); err == nil && n >= 0 && n <= brotliMaxLevel {
			return n
		}
	}
	return HeraldBrotliDefaultLevel
}
