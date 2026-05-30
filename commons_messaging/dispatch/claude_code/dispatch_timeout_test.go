package claude_code

import (
	"context"
	"strings"
	"testing"
	"time"
)

// HRD-146 — bounded per-message dispatch timeout.
//
// The production caller (pherald listen → Dispatcher.Handle → Dispatch)
// passes the long-poll runtime ctx, which has NO per-message deadline. The
// pre-existing CHAOS(b) test (dispatch_stress_chaos_test.go) proves the
// exec.CommandContext kill mechanism works WHEN THE CALLER supplies a
// bounded ctx — but production never does. These tests prove Dispatch now
// imposes the bound ITSELF: a hung `claude` is killed within the
// configured budget even when the caller's ctx is unbounded
// (context.Background()).
//
// All tests drive a REAL subprocess (testdata/fake-claude-sleep.sh, which
// `exec sleep 30`) — not a mock — so the SIGKILL path is genuinely
// exercised and the bounded wall-clock is measured, not asserted on faith.

// TestDispatch_DefaultTimeout_KillsHungSubprocess is the core HRD-146
// regression: the caller passes an UNBOUNDED context.Background(), the
// dispatcher's OWN per-message timeout (set tiny here) must kill the
// sleeping `claude` and return a timeout-tagged error far below the shim's
// 30s sleep.
func TestDispatch_DefaultTimeout_KillsHungSubprocess(t *testing.T) {
	fakeBin := shimPath(t, "fake-claude-sleep.sh")
	d, err := New(fakeBin, t.TempDir(), "TimeoutOwnerProj")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	seedAnchor(t, d) // steady-state --resume path.

	// The dispatcher imposes the bound — NOT the caller. This is exactly
	// the production posture: pherald listen hands Dispatch an unbounded
	// runtime ctx.
	const budget = 700 * time.Millisecond
	d.SetDispatchTimeout(budget)

	start := time.Now()
	done := make(chan struct{})
	var dispErr error
	go func() {
		_, dispErr = d.Dispatch(context.Background(), stressReq(0)) // UNBOUNDED caller ctx.
		close(done)
	}()

	// Watchdog: generously above the budget, FAR below the shim's 30s sleep.
	// If the dispatcher's own timeout did NOT fire, Dispatch would block on
	// the 30s sleep and this would catch the wedge — i.e. the production
	// hang HRD-146 fixes.
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("Dispatch did NOT honour its OWN per-message timeout with an unbounded caller ctx — the inbound runtime would wedge on a hung claude (HRD-146 FAIL)")
	}
	elapsed := time.Since(start)

	if dispErr == nil {
		t.Fatal("Dispatch returned nil error despite the per-message budget expiring before the subprocess replied")
	}
	// Bounded wall-clock: must be clearly the cancellation, not the 30s sleep.
	if elapsed >= 5*time.Second {
		t.Fatalf("Dispatch took %s — own-timeout kill did not fire promptly (shim sleeps 30s)", elapsed)
	}
	// Sanity: it should not return implausibly faster than the budget either
	// (that would mean it never actually launched the subprocess).
	if elapsed < budget/2 {
		t.Fatalf("Dispatch returned in %s, well under the %s budget — subprocess may not have been launched", elapsed, budget)
	}
	es := dispErr.Error()
	// §107: the error MUST name the timeout so the operator distinguishes a
	// hang from a genuine claude failure.
	if !strings.Contains(es, "timed out after") || !strings.Contains(es, "HERALD_CLAUDE_DISPATCH_TIMEOUT") {
		t.Errorf("timeout error does not name the timeout/env knob: %q", es)
	}
	if !strings.Contains(es, "claude_code: dispatch") {
		t.Errorf("error not tagged with the dispatch stage: %q", es)
	}

	t.Logf("HRD-146 own-timeout: unbounded caller ctx; budget=%s → Dispatch returned in %s (shim would sleep 30s); err=%q",
		budget, elapsed.Round(time.Millisecond), es)
}

// TestDispatch_CallerCancel_Propagates proves a caller cancelling mid-flight
// (e.g. pherald listen shutting down) unblocks Dispatch promptly — the
// caller's cancellation is NOT swallowed by the dispatcher's own timeout
// wrapper. The dispatcher budget is set LARGE so the only thing that can
// end the call early is the caller's cancel.
func TestDispatch_CallerCancel_Propagates(t *testing.T) {
	fakeBin := shimPath(t, "fake-claude-sleep.sh")
	d, err := New(fakeBin, t.TempDir(), "CallerCancelProj")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	seedAnchor(t, d)
	d.SetDispatchTimeout(30 * time.Second) // far larger than the cancel window.

	ctx, cancel := context.WithCancel(context.Background())
	start := time.Now()
	done := make(chan struct{})
	var dispErr error
	go func() {
		_, dispErr = d.Dispatch(ctx, stressReq(0))
		close(done)
	}()

	// Cancel mid-flight after the subprocess has surely launched.
	time.Sleep(400 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("Dispatch did NOT return promptly on caller cancellation — cancellation was swallowed (HRD-146 FAIL)")
	}
	elapsed := time.Since(start)

	if dispErr == nil {
		t.Fatal("Dispatch returned nil error despite caller cancelling mid-flight")
	}
	if elapsed >= 5*time.Second {
		t.Fatalf("Dispatch took %s after caller cancel — did not propagate promptly (shim sleeps 30s)", elapsed)
	}
	t.Logf("HRD-146 caller-cancel: cancelled at ~400ms → Dispatch returned in %s; err=%q",
		elapsed.Round(time.Millisecond), dispErr)
}

// TestDispatch_HappyPath_FastReplyUnaffected proves the timeout wrapper does
// NOT perturb the normal success path: a healthy fake-claude that replies
// immediately still parses into the expected DispatchResponse well within
// the (generous default) budget.
func TestDispatch_HappyPath_FastReplyUnaffected(t *testing.T) {
	fakeBin := shimPath(t, "fake-claude-reply.sh")
	d, err := New(fakeBin, t.TempDir(), "HappyProj")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	seedAnchor(t, d)

	start := time.Now()
	resp, err := d.Dispatch(context.Background(), stressReq(0))
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Dispatch (happy path): %v", err)
	}
	// §107: real parse from the shim's <<<HERALD-REPLY>>> line, not a default.
	if resp.Outcome != "answered" || resp.Summary != "stress ack" {
		t.Fatalf("unexpected parsed reply: outcome=%q summary=%q", resp.Outcome, resp.Summary)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("healthy reply took %s — timeout wrapper introduced latency", elapsed)
	}
	t.Logf("HRD-146 happy-path: healthy reply parsed in %s (default budget %s untouched)",
		elapsed.Round(time.Millisecond), DefaultDispatchTimeout)
}

// TestResolveDispatchTimeout_Env proves the HERALD_CLAUDE_DISPATCH_TIMEOUT
// env knob is honoured at construction, and that unset/invalid values fall
// back to the safe default (never zero / never unbounded).
func TestResolveDispatchTimeout_Env(t *testing.T) {
	cases := []struct {
		name string
		set  bool
		val  string
		want time.Duration
	}{
		{"unset", false, "", DefaultDispatchTimeout},
		{"empty", true, "", DefaultDispatchTimeout},
		{"valid_90s", true, "90s", 90 * time.Second},
		{"valid_3m", true, "3m", 3 * time.Minute},
		{"invalid_garbage", true, "not-a-duration", DefaultDispatchTimeout},
		{"invalid_zero", true, "0s", DefaultDispatchTimeout},
		{"invalid_negative", true, "-5s", DefaultDispatchTimeout},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv(DispatchTimeoutEnv, tc.val)
			} else {
				// Ensure it is unset for this subtest.
				t.Setenv(DispatchTimeoutEnv, "")
			}
			d, err := New("claude", t.TempDir(), "EnvProj")
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if got := d.dispatchTimeoutOrDefault(); got != tc.want {
				t.Fatalf("env=%q → dispatchTimeout=%s, want %s", tc.val, got, tc.want)
			}
		})
	}
}

// TestSetDispatchTimeout_ResetSemantics proves the override + reset contract.
func TestSetDispatchTimeout_ResetSemantics(t *testing.T) {
	d, err := New("claude", t.TempDir(), "SetProj")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	d.SetDispatchTimeout(42 * time.Second)
	if got := d.dispatchTimeoutOrDefault(); got != 42*time.Second {
		t.Fatalf("after Set(42s): %s", got)
	}
	d.SetDispatchTimeout(0) // non-positive → reset to default.
	if got := d.dispatchTimeoutOrDefault(); got != DefaultDispatchTimeout {
		t.Fatalf("after Set(0) reset: %s, want %s", got, DefaultDispatchTimeout)
	}
}
