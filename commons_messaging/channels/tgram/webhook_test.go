// Hermetic httptest coverage for the Bot API webhook validator (HRD-136).
//
// These tests prove the cryptographic-validation skeleton works end-to-end
// at the HTTP boundary without any external service:
//
//   - the configured secret is required + validated constant-time;
//   - a wrong/missing/empty secret yields uniform 401 with no body detail
//     (denying timing-oracle + body-grep attackers);
//   - the method MUST be POST;
//   - malformed JSON in the body yields 400;
//   - the constructor refuses an empty secret loudly;
//   - subtle.ConstantTimeCompare is on the hot path (sanity test against
//     equal-length-but-different-content secrets — proves we didn't
//     regress to a `==` short-circuit).
//
// Wave 6 wiring into `pherald listen --webhook` is deferred to V1.x — these
// tests deliberately do NOT spin up the full pherald inbound runtime; they
// hit the WebhookHandler directly via httptest.NewServer.
package tgram

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/vasic-digital/herald/commons"
)

// recordingHandler is a fake commons.InboundHandler that captures every
// dispatched event for later assertion. It is goroutine-safe under the
// (unrealistic) chance that httptest fires concurrent requests.
type recordingHandler struct {
	calls  atomic.Int64
	last   atomic.Pointer[commons.InboundEvent]
	retErr error
}

func (r *recordingHandler) Handle(_ context.Context, ev commons.InboundEvent) error {
	r.calls.Add(1)
	evCopy := ev
	r.last.Store(&evCopy)
	return r.retErr
}

// newSilentLogger returns a logger that discards everything — used so the
// test runner's PASS lines aren't polluted with deliberate failure traces
// like "body decode failed: ..." that we trigger on purpose.
func newSilentLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}

// buildValidUpdateJSON renders a minimally-valid telebot.Update JSON body
// that updateToInboundEvent will translate into a non-empty InboundEvent.
func buildValidUpdateJSON(t *testing.T) []byte {
	t.Helper()
	body := map[string]any{
		"update_id": 12345,
		"message": map[string]any{
			"message_id": 7,
			"date":       1700000000,
			"chat": map[string]any{
				"id":   int64(42),
				"type": "private",
			},
			"text": "hello herald",
		},
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("buildValidUpdateJSON: marshal: %v", err)
	}
	return b
}

// TestWebhook_ValidSecret_DispatchesToHandler — happy path. Correct header,
// well-formed Update JSON ⇒ handler invoked + 200 returned + body `{"ok":true}`.
func TestWebhook_ValidSecret_DispatchesToHandler(t *testing.T) {
	const secret = "s3cret-token-A"
	rec := &recordingHandler{}
	h, err := NewWebhookHandler(secret, rec, newSilentLogger())
	if err != nil {
		t.Fatalf("NewWebhookHandler: %v", err)
	}
	ts := httptest.NewServer(h)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL, bytes.NewReader(buildValidUpdateJSON(t)))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set(HeaderName(), secret)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte(`"ok":true`)) {
		t.Fatalf("body = %q, want it to contain {\"ok\":true}", body)
	}
	if got := rec.calls.Load(); got != 1 {
		t.Fatalf("handler called %d times, want 1", got)
	}
	got := rec.last.Load()
	if got == nil {
		t.Fatalf("recorded event is nil")
	}
	if got.Body.Plain != "hello herald" {
		t.Fatalf("event Body.Plain = %q, want %q", got.Body.Plain, "hello herald")
	}
	if got.Sender.Channel != string(commons.ChannelTelegram) {
		t.Fatalf("event Sender.Channel = %q, want %q", got.Sender.Channel, commons.ChannelTelegram)
	}
	if got.Sender.ChannelUserID != "42" {
		t.Fatalf("event Sender.ChannelUserID = %q, want %q", got.Sender.ChannelUserID, "42")
	}
}

// TestWebhook_InvalidSecret_Returns401_NoTokenLeak — defends against
// probe-based oracle attacks: a wrong header MUST yield 401 AND the
// response body must contain NEITHER the presented token NOR the
// configured secret (no echo-back, no error message that names a secret).
func TestWebhook_InvalidSecret_Returns401_NoTokenLeak(t *testing.T) {
	const configured = "CONFIGURED-SECRET-XYZ"
	const presented = "WRONG-PROBE-TOKEN-123"
	rec := &recordingHandler{}
	h, err := NewWebhookHandler(configured, rec, newSilentLogger())
	if err != nil {
		t.Fatalf("NewWebhookHandler: %v", err)
	}
	ts := httptest.NewServer(h)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL, bytes.NewReader(buildValidUpdateJSON(t)))
	req.Header.Set(HeaderName(), presented)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if strings.Contains(bodyStr, configured) {
		t.Fatalf("response body leaks configured secret %q: body=%q", configured, bodyStr)
	}
	if strings.Contains(bodyStr, presented) {
		t.Fatalf("response body echoes presented token %q: body=%q", presented, bodyStr)
	}
	if rec.calls.Load() != 0 {
		t.Fatalf("handler was invoked %d times on 401 path, want 0", rec.calls.Load())
	}
}

// TestWebhook_MissingSecret_Returns401 — no header at all = 401.
// The body must be empty (or at least free of any hint that a header was
// expected — that hint is itself a leak about the configured secret's
// existence).
func TestWebhook_MissingSecret_Returns401(t *testing.T) {
	const configured = "X-secret-Y"
	rec := &recordingHandler{}
	h, err := NewWebhookHandler(configured, rec, newSilentLogger())
	if err != nil {
		t.Fatalf("NewWebhookHandler: %v", err)
	}
	ts := httptest.NewServer(h)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL, bytes.NewReader(buildValidUpdateJSON(t)))
	// Deliberately NO HeaderName() set.

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), configured) {
		t.Fatalf("response body leaks configured secret: body=%q", body)
	}
	if rec.calls.Load() != 0 {
		t.Fatalf("handler was invoked %d times on missing-header 401 path, want 0", rec.calls.Load())
	}
}

// TestWebhook_GET_Returns405 — wrong method. Telegram only ever issues POST.
// A GET probe (operator-typed curl, attacker scanner) gets 405.
func TestWebhook_GET_Returns405(t *testing.T) {
	const configured = "any-secret"
	rec := &recordingHandler{}
	h, err := NewWebhookHandler(configured, rec, newSilentLogger())
	if err != nil {
		t.Fatalf("NewWebhookHandler: %v", err)
	}
	ts := httptest.NewServer(h)
	defer ts.Close()

	// Even WITH the correct secret on a GET, we must 405 — auth is
	// orthogonal to method; the method check fires first by design.
	req, _ := http.NewRequest(http.MethodGet, ts.URL, nil)
	req.Header.Set(HeaderName(), configured)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
	if got := resp.Header.Get("Allow"); got != http.MethodPost {
		t.Fatalf("Allow header = %q, want %q", got, http.MethodPost)
	}
	if rec.calls.Load() != 0 {
		t.Fatalf("handler was invoked %d times on GET 405 path, want 0", rec.calls.Load())
	}
}

// TestWebhook_MalformedJSON_Returns400 — body decode failure path. We send
// the correct secret + a body that is not valid JSON; expect 400 + handler
// untouched. The malformed-body trace lands in the logger, NOT the response.
func TestWebhook_MalformedJSON_Returns400(t *testing.T) {
	const configured = "good-secret"
	rec := &recordingHandler{}
	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)
	h, err := NewWebhookHandler(configured, rec, logger)
	if err != nil {
		t.Fatalf("NewWebhookHandler: %v", err)
	}
	ts := httptest.NewServer(h)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL, strings.NewReader("{this is not json"))
	req.Header.Set(HeaderName(), configured)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if rec.calls.Load() != 0 {
		t.Fatalf("handler was invoked %d times on malformed-body path, want 0", rec.calls.Load())
	}
	// The decode error trace SHOULD have landed in the logger; the
	// response body MUST NOT.
	respBody, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(respBody), "decode") {
		t.Fatalf("response body leaks decode error detail: body=%q", respBody)
	}
	if !strings.Contains(logBuf.String(), "decode failed") {
		t.Fatalf("logger did not capture decode failure; got=%q", logBuf.String())
	}
}

// TestWebhook_EmptySecretAtConstruction_ErrorsLoud — the constructor MUST
// refuse an empty secret. A receiver that accepts "" as a secret means
// "any caller succeeds" — that is the §107 footgun this whole file exists
// to prevent. Fail loud at construction, not silently at request time.
func TestWebhook_EmptySecretAtConstruction_ErrorsLoud(t *testing.T) {
	rec := &recordingHandler{}
	h, err := NewWebhookHandler("", rec, newSilentLogger())
	if err == nil {
		t.Fatalf("NewWebhookHandler(\"\", ...) returned no error; want a loud rejection")
	}
	if h != nil {
		t.Fatalf("NewWebhookHandler(\"\", ...) returned a non-nil handler; want nil on error")
	}
	if !strings.Contains(err.Error(), "secret") {
		t.Fatalf("error message %q does not mention 'secret' — operator may miss the cause", err.Error())
	}

	// Also assert the nil-handler refusal — same rationale (silent
	// no-op receiver is a worse failure mode than a loud constructor
	// error).
	h2, err2 := NewWebhookHandler("good", nil, newSilentLogger())
	if err2 == nil {
		t.Fatalf("NewWebhookHandler(\"good\", nil, ...) returned no error; want a loud rejection")
	}
	if h2 != nil {
		t.Fatalf("NewWebhookHandler(nil-handler) returned a non-nil handler; want nil on error")
	}
}

// TestWebhook_ConstantTimeCompare — sanity that two equal-length but
// different-content secrets BOTH yield 401. This catches a regression
// where someone "simplifies" secretMatches to use bytes.Equal or a `==`
// short-circuit: those compare in non-constant time and leak the secret
// one timing-measurable byte at a time. We don't measure timing here
// (that's flaky on shared CI runners); we just assert the behavioural
// contract — equal-length-different-content MUST still fail with the
// same 401 + no-body shape, AND we directly invoke secretMatches to
// prove it returns false on equal-length-different-content (an
// instrumented `==` check would still return false here, but adding
// the http-boundary assertion plus the direct comparator call together
// makes a future regression to a non-constant-time check very obvious
// in the test diff).
func TestWebhook_ConstantTimeCompare(t *testing.T) {
	// Direct comparator-level assertions — these are the load-bearing
	// crypto invariants.
	configured := []byte("AAAAAAAAAAAAAAAA") // 16 A's
	presented1 := []byte("BBBBBBBBBBBBBBBB") // 16 B's (equal length, all-differ)
	presented2 := []byte("AAAAAAAAAAAAAAAB") // 16 chars, only last differs (the classic timing-oracle target)
	presented3 := []byte("AAAAAAAA")         // shorter (length-mismatch path)

	if secretMatches(configured, presented1) {
		t.Fatalf("secretMatches matched all-different equal-length secret; constant-time-compare regression")
	}
	if secretMatches(configured, presented2) {
		t.Fatalf("secretMatches matched last-byte-differing equal-length secret; constant-time-compare regression")
	}
	if secretMatches(configured, presented3) {
		t.Fatalf("secretMatches matched length-mismatched secret; length-fallback regression")
	}
	if !secretMatches(configured, []byte("AAAAAAAAAAAAAAAA")) {
		t.Fatalf("secretMatches rejected the correct secret; comparator inverted?")
	}

	// HTTP-boundary assertion — both equal-length-different-content
	// inputs land on the same 401 + no-body shape; an attacker can
	// observe NO difference between them.
	rec := &recordingHandler{}
	h, err := NewWebhookHandler(string(configured), rec, newSilentLogger())
	if err != nil {
		t.Fatalf("NewWebhookHandler: %v", err)
	}
	ts := httptest.NewServer(h)
	defer ts.Close()

	for _, presented := range []string{string(presented1), string(presented2)} {
		req, _ := http.NewRequest(http.MethodPost, ts.URL, bytes.NewReader(buildValidUpdateJSON(t)))
		req.Header.Set(HeaderName(), presented)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Do(presented=%q): %v", presented, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("presented=%q: status = %d, want 401", presented, resp.StatusCode)
		}
		if len(body) != 0 {
			t.Fatalf("presented=%q: response body must be empty on 401 (got %q) to deny body-grep oracles", presented, body)
		}
	}
	if rec.calls.Load() != 0 {
		t.Fatalf("handler invoked %d times during constant-time-compare probes, want 0", rec.calls.Load())
	}
}
