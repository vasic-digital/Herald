package tgram_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	telebot "gopkg.in/telebot.v3"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons_messaging/channels/tgram"
)

// recordingSleeper is the fake clock injected via Adapter.SetSleeperForTest
// for HRD-134 rate-limit tests. It records every requested duration in
// order and returns immediately so retry loops don't pay real wall-clock
// time. If ctx is already cancelled at call time it returns ctx.Err().
type recordingSleeper struct {
	mu       sync.Mutex
	requests []time.Duration
}

func (r *recordingSleeper) sleep(ctx context.Context, d time.Duration) error {
	r.mu.Lock()
	r.requests = append(r.requests, d)
	r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (r *recordingSleeper) snapshot() []time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]time.Duration, len(r.requests))
	copy(out, r.requests)
	return out
}

// newRateLimitStub returns an httptest server that responds to /getMe
// (always OK so telebot.NewBot succeeds) and a configurable /sendMessage
// handler whose response is decided by the responses slice in order: each
// invocation pops the next responder. After the slice is exhausted the
// server returns 500 to surface a test-bug rather than wedge.
type stubResponse struct {
	// If retryAfter > 0: 429 with parameters.retry_after = retryAfter.
	// If ok=true: 200 + message_id=777 happy path.
	// Otherwise: generic 500.
	retryAfter int
	ok         bool
}

func newRateLimitStub(t *testing.T, responses []stubResponse) (*httptest.Server, *int32) {
	t.Helper()
	var idx int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"TestBot","username":"TestBot"}}`))
			return
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			i := atomic.AddInt32(&idx, 1) - 1
			if int(i) >= len(responses) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"ok":false,"error_code":500,"description":"test stub ran out of responses"}`))
				return
			}
			resp := responses[int(i)]
			switch {
			case resp.retryAfter > 0:
				// Per Bot API contract for 429: `parameters.retry_after`
				// is the seconds the client should wait. extractOk
				// (telebot/bot_raw.go:322) maps this to FloodError.
				// Note: telebot's extractOk does not gate on HTTP
				// status; it decodes the JSON body for ok=false +
				// error_code=429. Returning 200 here would still
				// work, but real Bot API sends 429 — match reality.
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests: retry after ` +
					itoaTest(resp.retryAfter) + `","parameters":{"retry_after":` + itoaTest(resp.retryAfter) + `}}`))
			case resp.ok:
				_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":777,"chat":{"id":12345},"text":"hi"}}`))
			default:
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"ok":false,"error_code":500,"description":"stub error"}`))
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &idx
}

// itoaTest is the local integer-to-string helper for the stub responder
// (avoids any name-clash with strconv.Itoa in the test scope).
func itoaTest(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := make([]byte, 0, 6)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}

func sendPlain(a *tgram.Adapter, ctx context.Context) (commons.Receipt, error) {
	return a.Send(ctx, commons.OutboundMessage{Body: commons.Body{Plain: "hi"}})
}

// TestTgram_Send_429_HonorsRetryAfter is the §107 anti-bluff anchor for
// HRD-134's happy retry path: first call returns 429 retry_after=1,
// second call returns 200; Send MUST sleep ~1s and succeed. We use the
// recordingSleeper to assert the sleep WAS requested (the canonical
// behavioural anchor — a regression that dropped withRetry429 would
// either propagate the 429 to the caller or skip the sleep entirely).
func TestTgram_Send_429_HonorsRetryAfter(t *testing.T) {
	srv, _ := newRateLimitStub(t, []stubResponse{
		{retryAfter: 1},
		{ok: true},
	})

	a := tgram.NewAdapterWithBaseURL("TESTTOKEN", "12345", srv.URL)
	sleeper := &recordingSleeper{}
	a.SetSleeperForTest(sleeper.sleep)

	rcpt, err := sendPlain(a, context.Background())
	if err != nil {
		t.Fatalf("Send: expected success after one retry, got %v", err)
	}
	if rcpt.ChannelMsgID != "777" {
		t.Fatalf("ChannelMsgID: got %q want %q", rcpt.ChannelMsgID, "777")
	}
	got := sleeper.snapshot()
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 sleep request; got %d: %v", len(got), got)
	}
	if got[0] != time.Second {
		t.Fatalf("sleep duration: got %v want %v", got[0], time.Second)
	}
}

// TestTgram_Send_429_GivesUpAfter3Attempts: every call returns 429
// retry_after=1; Send MUST give up after 3 attempts (2 sleeps between
// them) and return the final 429 error. The sleeper records exactly 2
// sleep requests — one between attempt 1↔2 and one between 2↔3, NOT
// one between 3↔nothing (the loop exits before the 3rd sleep).
func TestTgram_Send_429_GivesUpAfter3Attempts(t *testing.T) {
	srv, callCount := newRateLimitStub(t, []stubResponse{
		{retryAfter: 1},
		{retryAfter: 1},
		{retryAfter: 1},
	})

	a := tgram.NewAdapterWithBaseURL("TESTTOKEN", "12345", srv.URL)
	sleeper := &recordingSleeper{}
	a.SetSleeperForTest(sleeper.sleep)

	_, err := sendPlain(a, context.Background())
	if err == nil {
		t.Fatal("Send: expected final 429 error after 3 attempts; got nil")
	}
	if atomic.LoadInt32(callCount) != 3 {
		t.Fatalf("sendMessage hit count: got %d want 3", atomic.LoadInt32(callCount))
	}
	got := sleeper.snapshot()
	if len(got) != 2 {
		t.Fatalf("expected exactly 2 sleep requests (between attempt 1↔2 and 2↔3); got %d: %v", len(got), got)
	}
	// Final error must still carry a Telegram 429 trace. We don't pin
	// the exact format (telebot wraps the description) — just confirm
	// it's an error string from the tgram path.
	if !strings.Contains(err.Error(), "tgram.Send") {
		t.Errorf("final error %q does not cite call site tgram.Send", err.Error())
	}
}

// TestTgram_Send_429_RespectsCtxCancel: server returns 429
// retry_after=30 once; ctx is cancelled mid-sleep; Send MUST return
// ctx.Err() promptly (NOT after 30s wall-clock). The fake sleeper
// honors the cancelled ctx and returns its error — proves the
// withRetry429 sleeper integration is ctx-aware.
func TestTgram_Send_429_RespectsCtxCancel(t *testing.T) {
	srv, _ := newRateLimitStub(t, []stubResponse{
		{retryAfter: 30},
	})

	a := tgram.NewAdapterWithBaseURL("TESTTOKEN", "12345", srv.URL)
	// Sleeper that observes a pre-cancelled ctx and returns ctx.Err().
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel BEFORE the call so the sleeper sees Err immediately
	a.SetSleeperForTest(func(c context.Context, d time.Duration) error {
		// Match realSleep semantics — return ctx.Err() if ctx fired.
		if err := c.Err(); err != nil {
			return err
		}
		return nil
	})

	start := time.Now()
	_, err := sendPlain(a, ctx)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("Send: expected ctx.Cancelled error; got nil")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("Send took %v — must return promptly on ctx cancel, not wait 30s", elapsed)
	}
	// The propagated error should mention "context canceled" since that's
	// what ctx.Err() returns. The withRetry429 layer returns it directly
	// (bypasses sanitizeTgramError on ctx-fail paths), and the Send
	// caller wraps with "tgram.Send: sendMessage to chat ...".
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("expected ctx.Cancelled in error; got %q", err.Error())
	}
}

// TestTgram_Send_429_ClampsRetryAfter_60s: server returns 429
// retry_after=300 (5 minutes — well above the maxRetryAfterSeconds
// ceiling). Send MUST request the clamped 60s sleep, NOT the raw
// 300s. Defense-in-depth against a Bot API regression that returns
// runaway retry_after values.
func TestTgram_Send_429_ClampsRetryAfter_60s(t *testing.T) {
	srv, _ := newRateLimitStub(t, []stubResponse{
		{retryAfter: 300},
		{ok: true},
	})

	a := tgram.NewAdapterWithBaseURL("TESTTOKEN", "12345", srv.URL)
	sleeper := &recordingSleeper{}
	a.SetSleeperForTest(sleeper.sleep)

	rcpt, err := sendPlain(a, context.Background())
	if err != nil {
		t.Fatalf("Send: expected success after one retry, got %v", err)
	}
	if rcpt.ChannelMsgID != "777" {
		t.Fatalf("ChannelMsgID: got %q want %q", rcpt.ChannelMsgID, "777")
	}
	got := sleeper.snapshot()
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 sleep request; got %d: %v", len(got), got)
	}
	want := 60 * time.Second
	if got[0] != want {
		t.Fatalf("sleep duration: got %v want %v (clamp must cap at maxRetryAfterSeconds=60)", got[0], want)
	}
}

// TestTgram_ExtractRetryAfter_Unit pins the telebot.FloodError ->
// retry_after mapping in isolation — paired with the live-server tests
// above so a regression in either the FloodError construction
// (telebot/bot_raw.go:329) or the Herald-side extractor surfaces
// independently.
func TestTgram_ExtractRetryAfter_Unit(t *testing.T) {
	// Construct a FloodError the same way telebot does (errors.go:17 +
	// bot_raw.go:329). RetryAfter is the only field we care about.
	fe := telebot.FloodError{RetryAfter: 7}
	got, ok := tgram.ExtractRetryAfterForTest(fe)
	if !ok {
		t.Fatal("extractRetryAfter(FloodError{7}): ok=false; want true")
	}
	if got != 7 {
		t.Errorf("got %d want 7", got)
	}
	// Non-FloodError → (0, false).
	plainErr := &plainErrorForTest{msg: "not a flood"}
	if _, ok := tgram.ExtractRetryAfterForTest(plainErr); ok {
		t.Fatal("non-FloodError path: expected ok=false")
	}
	// nil → (0, false).
	if _, ok := tgram.ExtractRetryAfterForTest(nil); ok {
		t.Fatal("nil path: expected ok=false")
	}
}

// plainErrorForTest is a minimal non-FloodError error used by
// TestTgram_ExtractRetryAfter_Unit's negative-path check.
type plainErrorForTest struct{ msg string }

func (e *plainErrorForTest) Error() string { return e.msg }
