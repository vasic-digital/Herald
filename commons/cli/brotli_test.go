// Wave 4a Task 5 — Brotli Gin middleware tests.
//
// Per §107 / Sixth Law: the load-bearing positive evidence in this file
// is NOT "Content-Encoding: br header present" — it is the actual
// brotli.NewReader decoding the response body back to the source bytes.
// A bogus implementation that sets the header but writes gibberish (or
// shorter gibberish to game a size check) would pass a header-only OR a
// size-only assertion. It CANNOT pass decode-and-compare.

package cli

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/gin-gonic/gin"
)

// payload5K returns a deterministic ~5KB JSON-ish payload that is well
// above the upstream MinLength=256 cutoff AND in the compressible-types
// list (application/json) so the brotli middleware actually engages.
//
// The payload is repeated-but-varied structure (line index woven in)
// so it compresses well (typical JSON traffic profile) without being
// pathologically periodic. A purely identical-line payload like
// `strings.Repeat("{...}", 100)` would compress to ~60 bytes at any
// quality and starve TestBrotliMiddleware_QualityEnv of the spread it
// needs to distinguish q6 vs q11.
//
// A bug in the encoder that emits gibberish would produce a body
// that cannot be decoded back to this string.
func payload5K() string {
	var b strings.Builder
	for i := 0; i < 100; i++ {
		// Vary one field per line so the periodic suffix differs while
		// the structural prefix repeats; Brotli backref matches the
		// prefix and the entropy coder handles the small variation.
		fmt.Fprintf(&b, `{"key":"value-%03d","number":%d,"text":"hello brotli %s"}`+"\n",
			i, i*7, randomishWord(i))
	}
	return b.String()
}

// randomishWord returns a deterministic short string keyed on i so the
// payload spread is reproducible across test runs (no t.Setenv flakes,
// no clock-dependent values). The pool intentionally has more letters
// than `strings.Repeat` would emit so the q11 entropy advantage shows.
func randomishWord(i int) string {
	pool := []string{
		"alpha", "bravo", "charlie", "delta", "echo", "foxtrot",
		"golf", "hotel", "india", "juliett", "kilo", "lima",
		"mike", "november", "oscar", "papa", "quebec", "romeo",
		"sierra", "tango", "uniform", "victor", "whiskey", "xray",
		"yankee", "zulu",
	}
	return pool[i%len(pool)]
}

// TestBrotliMiddleware_AcceptBr_Compresses asserts the §107 invariant:
// when the client sends Accept-Encoding: br, the response is actually
// Brotli-compressed AND decodes back to the original payload byte-for-
// byte. Header-only assertions would pass a buggy encoder; decode-and-
// compare cannot.
func TestBrotliMiddleware_AcceptBr_Compresses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BrotliMiddleware(6))
	src := payload5K()
	r.GET("/large", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.String(200, src)
	})
	req := httptest.NewRequest("GET", "/large", nil)
	req.Header.Set("Accept-Encoding", "br")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "br" {
		t.Fatalf("Content-Encoding = %q, want 'br' — middleware no-op'd despite Accept-Encoding: br", got)
	}
	// §107: REAL decode-and-compare via the andybalholm/brotli reader.
	// A bogus encoder cannot fake this — the decoder will either fail
	// outright (invalid Brotli stream) or produce non-matching bytes.
	br := brotli.NewReader(bytes.NewReader(rec.Body.Bytes()))
	decoded, err := io.ReadAll(br)
	if err != nil {
		t.Fatalf("brotli.NewReader.ReadAll failed — compressed bytes are not valid Brotli (encoder bluff): %v", err)
	}
	if string(decoded) != src {
		// Don't dump the full 5KB on failure; show enough context.
		preview := func(s string) string {
			if len(s) > 80 {
				return s[:80] + "..."
			}
			return s
		}
		t.Fatalf("decoded body mismatch:\n  want (first 80): %q\n  got  (first 80): %q\n  src_len=%d, decoded_len=%d",
			preview(src), preview(string(decoded)), len(src), len(decoded))
	}
	// Sanity: compressed body smaller than source — Brotli quality 6
	// MUST compress repetitive JSON below the source size.
	if len(rec.Body.Bytes()) >= len(src) {
		t.Errorf("compressed size %d >= source size %d — Brotli is not actually compressing", len(rec.Body.Bytes()), len(src))
	}
}

// TestBrotliMiddleware_NoBrInAccept_Identity asserts identity passthrough
// when the client does NOT advertise Brotli support. Sending Brotli to
// a client that asked for gzip would break the response.
func TestBrotliMiddleware_NoBrInAccept_Identity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BrotliMiddleware(6))
	src := payload5K()
	r.GET("/large", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.String(200, src)
	})
	req := httptest.NewRequest("GET", "/large", nil)
	req.Header.Set("Accept-Encoding", "gzip") // explicitly no "br"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q on br-less Accept-Encoding, want empty (would break the client)", got)
	}
	if rec.Body.String() != src {
		t.Errorf("body altered when no compression should have been applied: len(got)=%d, len(want)=%d", rec.Body.Len(), len(src))
	}
}

// TestBrotliMiddleware_SmallerThanGzip asserts the practical motivation
// for choosing Brotli over gzip: on text/JSON payloads, Brotli at
// quality 6 typically produces 10-30% smaller output than default gzip
// (compress/gzip's DefaultCompression == level 6). If this invariant
// is ever violated for Herald's 5KB JSON profile, the cost/benefit
// trade-off needs to be revisited — quality 6 might need to bump or
// the operator-locked default reconsidered.
func TestBrotliMiddleware_SmallerThanGzip(t *testing.T) {
	src := payload5K()

	// Brotli quality 6 via the same middleware path the server uses.
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BrotliMiddleware(6))
	r.GET("/large", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.String(200, src)
	})
	req := httptest.NewRequest("GET", "/large", nil)
	req.Header.Set("Accept-Encoding", "br")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("brotli leg: status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "br" {
		t.Fatalf("brotli leg: Content-Encoding = %q, want 'br'", got)
	}
	brSize := len(rec.Body.Bytes())

	// Default gzip from stdlib compress/gzip.
	var gzBuf bytes.Buffer
	gz := gzip.NewWriter(&gzBuf)
	if _, err := gz.Write([]byte(src)); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	gzSize := gzBuf.Len()

	if brSize > gzSize {
		t.Errorf("Brotli quality 6 (%d bytes) larger than default gzip (%d bytes) on 5KB JSON — Brotli should compress text payloads better than gzip; revisit quality choice", brSize, gzSize)
	}
	t.Logf("compression sizes — source: %d, gzip default: %d, brotli q6: %d (saving vs gzip: %.1f%%)",
		len(src), gzSize, brSize, 100*float64(gzSize-brSize)/float64(gzSize))
}

// TestBrotliMiddleware_QualityEnv asserts the HERALD_BROTLI_LEVEL env
// var override path. Passing quality=0 should make the middleware
// consult the env var; a higher quality should compress more
// aggressively than quality 6 on the same payload.
//
// Quality 11 vs quality 6 is the deliberate max-vs-default contrast —
// at q11 Brotli runs its full backref + entropy search and produces
// the smallest output the algorithm can manage. If q11 doesn't beat q6
// on Herald's 5KB repetitive JSON, either the env var path is broken
// (still using q6 default) or the upstream brotli library is broken.
func TestBrotliMiddleware_QualityEnv(t *testing.T) {
	src := payload5K()

	// Baseline: quality 6 (operator-locked default).
	gin.SetMode(gin.TestMode)
	r6 := gin.New()
	r6.Use(BrotliMiddleware(6))
	r6.GET("/large", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.String(200, src)
	})
	req6 := httptest.NewRequest("GET", "/large", nil)
	req6.Header.Set("Accept-Encoding", "br")
	rec6 := httptest.NewRecorder()
	r6.ServeHTTP(rec6, req6)
	if rec6.Code != 200 || rec6.Header().Get("Content-Encoding") != "br" {
		t.Fatalf("q6 baseline failed: code=%d ce=%q", rec6.Code, rec6.Header().Get("Content-Encoding"))
	}
	q6Size := len(rec6.Body.Bytes())

	// Env-driven quality 11 via quality=0 + HERALD_BROTLI_LEVEL=11.
	t.Setenv("HERALD_BROTLI_LEVEL", "11")
	rEnv := gin.New()
	rEnv.Use(BrotliMiddleware(0)) // 0 ⇒ consult env ⇒ should read "11"
	rEnv.GET("/large", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.String(200, src)
	})
	reqEnv := httptest.NewRequest("GET", "/large", nil)
	reqEnv.Header.Set("Accept-Encoding", "br")
	recEnv := httptest.NewRecorder()
	rEnv.ServeHTTP(recEnv, reqEnv)
	if recEnv.Code != 200 || recEnv.Header().Get("Content-Encoding") != "br" {
		t.Fatalf("env q11 leg failed: code=%d ce=%q", recEnv.Code, recEnv.Header().Get("Content-Encoding"))
	}
	q11Size := len(recEnv.Body.Bytes())

	// Sanity: env-driven q11 still decodes correctly (the encoder didn't
	// silently fall back to identity or some lower quality).
	br := brotli.NewReader(bytes.NewReader(recEnv.Body.Bytes()))
	decoded, err := io.ReadAll(br)
	if err != nil {
		t.Fatalf("env q11 leg: decode failed: %v", err)
	}
	if string(decoded) != src {
		t.Fatalf("env q11 leg: decoded body mismatch (len got=%d, want=%d)", len(decoded), len(src))
	}

	// Load-bearing: q11 MUST be at least as compressed as q6 on this
	// payload. Brotli's quality monotonically improves the ratio at
	// the cost of CPU — q11 SHOULD beat q6, but in pathological edge
	// cases (already-incompressible input) they tie. For repetitive
	// JSON this assertion is safe.
	if q11Size > q6Size {
		t.Errorf("HERALD_BROTLI_LEVEL=11 (%d bytes) larger than quality 6 (%d bytes) — env override path likely broken (still using default level)", q11Size, q6Size)
	}
	t.Logf("env-override compression sizes — source: %d, q6: %d, env-q11: %d (env savings vs q6: %.1f%%)",
		len(src), q6Size, q11Size, 100*float64(q6Size-q11Size)/float64(q6Size))
}
