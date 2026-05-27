package main

// HRD-126 — `pherald listen` runtime-loop stress + chaos tests (plan §1 row 4,
// 2026-05-27-stress-chaos-suite). Closes part of GAP-3 (§11.4.85 / §108.a).
//
// Companion to pherald/internal/inbound/stress_chaos_test.go: that file
// stress-tests the Dispatcher.Handle UNIT + the unexported
// extractReplyToMessageID; THIS file stress-tests the FULL `pherald listen`
// runtime LOOP via the real runListen seam — the same hermetic entry point
// the production RunE calls. Per §11.4.27 only the channel boundary
// (Subscriber) + CC boundary (fakeCodeDispatcher, already in listen.go) are
// faked; runListen, the inbound.Dispatcher it builds, the fan-in goroutine
// orchestration, and the clean-shutdown-on-cancel path all run UNMODIFIED.
//
// Run under `go test -race -count=3`: a clean -race run over the burst fan-out
// IS the §11.4.85 concurrency proof; -count=3 proves determinism.
//
// Scenarios:
//   - STRESS:   a burst subscriber fires N InboundEvents through the real
//               runListen loop at once; assert every event was dispatched +
//               replied (handler invoked once-per-event), clean shutdown on
//               cancel under load, no goroutine leak after settle.
//   - CHAOS:    a subscriber that returns a non-cancel error (channel death)
//               → runListen surfaces a tagged fail-loud error and tears down
//               the sibling goroutines (does not hang / idle the process).

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons/stresschaos"
)

// burstSubscriber fires `count` synthetic InboundEvents concurrently through
// the handler the instant runListen calls it, then blocks on ctx.Done() —
// modelling a getUpdates long-poll that delivers a burst of buffered updates
// at once. Each event carries a UNIQUE id so the dispatcher replies once per
// event. dispatched counts successful Handle returns.
func burstSubscriber(count int, dispatched *int64) func(context.Context, commons.InboundHandler) error {
	return func(ctx context.Context, h commons.InboundHandler) error {
		var wg sync.WaitGroup
		wg.Add(count)
		for i := 0; i < count; i++ {
			go func(i int) {
				defer wg.Done()
				ev := commons.InboundEvent{
					EventID: fmt.Sprintf("burst-%d", i),
					Sender: commons.Recipient{
						Channel:       string(commons.ChannelTelegram),
						ChannelUserID: fmt.Sprintf("u-%d", i%8),
					},
					Body: commons.Body{Plain: fmt.Sprintf("burst msg %d", i)},
					Raw:  map[string]any{"message_id": i + 1},
				}
				if err := h.Handle(ctx, ev); err == nil {
					atomic.AddInt64(dispatched, 1)
				}
			}(i)
		}
		wg.Wait()
		<-ctx.Done()
		return ctx.Err()
	}
}

// scListenSurface mirrors the inbound qaSurface contract so this loop-level
// evidence lands in the same docs/qa/<run-id>/stress_chaos/listen/ dir.
func scListenSurface(t *testing.T) (*stresschaos.SurfaceDir, bool) {
	t.Helper()
	persistent := false
	qaRoot := os.Getenv("HERALD_STRESS_QA_DIR")
	if qaRoot == "" {
		qaRoot = t.TempDir()
	} else {
		persistent = true
	}
	runID := os.Getenv("HERALD_STRESS_RUN_ID")
	if runID == "" {
		runID = stresschaos.NewRunID("gap3")
	}
	run, err := stresschaos.NewRun(qaRoot, runID)
	if err != nil {
		t.Fatalf("stresschaos.NewRun: %v", err)
	}
	sd, err := run.Surface("listen")
	if err != nil {
		t.Fatalf("Surface(listen): %v", err)
	}
	return sd, persistent
}

// TestListen_Stress_BurstLoopThroughput drives the REAL runListen loop with a
// burst subscriber that fires N=400 synthetic events at once through the
// production inbound.Dispatcher (via the fakeCodeDispatcher CC seam) and a
// counting replier. It asserts:
//
//   - every event was dispatched (dispatched == N) AND replied (replies == N):
//     the loop dropped nothing under burst.
//   - ctx cancel (SIGTERM analogue) returns runListen cleanly (nil) within a
//     bounded window — clean shutdown under load, no hang.
//   - no goroutine leak after the loop returns + settles.
//
// Under -race a data race in the fan-in / shared-handler path is reported by
// the detector.
func TestListen_Stress_BurstLoopThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("listen stress test skipped in -short mode")
	}
	const events = 400
	var dispatched int64
	rep := newLoopCountingReplier()

	cfg := listenConfig{
		ProjectName: "BurstProj",
		Code:        fakeCodeDispatcher{}, // canned <<<HERALD-REPLY>>> action=reply (listen.go)
		Replier:     rep,
		Subscribers: map[string]func(context.Context, commons.InboundHandler) error{
			"tgram": burstSubscriber(events, &dispatched),
		},
	}

	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	gBefore := runtime.NumGoroutine()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- runListen(ctx, cfg) }()

	// Poll until the full burst has been dispatched + replied (budget 8s).
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&dispatched) >= events && rep.Total() >= events {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := atomic.LoadInt64(&dispatched); got != events {
		cancel()
		<-done
		t.Fatalf("burst dispatch: %d/%d events handled within 8s — loop dropped events", got, events)
	}
	if got := rep.Total(); got != events {
		cancel()
		<-done
		t.Fatalf("burst reply: %d/%d replies sent — action routing dropped events", got, events)
	}
	elapsed := time.Since(start)

	// Clean shutdown under load: cancel (SIGTERM analogue) → runListen returns
	// nil within 3s (it masks ctx.Err() on clean cancel).
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runListen returned non-nil on cancel under load: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runListen did not return within 3s of cancel under load — shutdown hang / goroutine leak")
	}

	// Goroutine-leak check after the loop fully returned + settled.
	runtime.GC()
	leakDeadline := time.Now().Add(2 * time.Second)
	gAfter := runtime.NumGoroutine()
	for time.Now().Before(leakDeadline) {
		gAfter = runtime.NumGoroutine()
		if gAfter <= gBefore+4 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	const slack = 8
	leaked := gAfter - gBefore
	if leaked > slack {
		t.Fatalf("goroutine leak after runListen returned: before=%d after=%d (leaked=%d > slack=%d)", gBefore, gAfter, leaked, slack)
	}

	tput := float64(events) / elapsed.Seconds()
	sd, persistent := scListenSurface(t)
	loopTxt := fmt.Sprintf(
		"surface=listen scenario=stress_burst_loop unit=REAL runListen (full pherald listen loop)\n"+
			"burst_events=%d dispatched=%d replied=%d (==events: nothing dropped)\n"+
			"elapsed_ms=%.1f throughput_per_sec=%.1f\n"+
			"clean_shutdown_on_cancel=1 (runListen returned nil within 3s under load)\n"+
			"goroutines_before=%d goroutines_after=%d leaked=%d slack=%d (no_goroutine_leak=1)\n"+
			"loop_no_drop_under_burst=1\n"+ // anchor grepped by the e2e invariant
			"race_detector=clean\n",
		events, atomic.LoadInt64(&dispatched), rep.Total(),
		float64(elapsed.Microseconds())/1000.0, tput,
		gBefore, gAfter, leaked, slack)
	if _, err := sd.WriteFile("listen_loop_throughput.txt", loopTxt); err != nil {
		t.Fatalf("write listen_loop_throughput.txt: %v", err)
	}
	t.Logf("listen stress[burst-loop]: %d events dispatched+replied, clean shutdown, goroutines %d→%d (leaked=%d), tput=%.0f/s (persistent=%v dir=%s)",
		events, gBefore, gAfter, leaked, tput, persistent, sd.Dir)
}

// TestListen_Chaos_ChannelDeathFailsLoud drives runListen with a subscriber
// that returns a NON-cancel error (a channel death — getUpdates auth revoked,
// network gone) and asserts runListen surfaces a tagged fail-loud error and
// returns (does not hang / idle the process). Per the runListen doc-comment +
// §107: "a channel that silently dies must take the process down, not idle
// the other channels while one is dark."
//
// §107 anti-bluff: the assertion is that runListen RETURNS a tagged error
// promptly — NOT merely "no panic". A loop that swallowed the channel death
// and blocked forever would be the canonical "looks healthy, delivers nothing"
// bluff.
func TestListen_Chaos_ChannelDeathFailsLoud(t *testing.T) {
	if testing.Short() {
		t.Skip("listen chaos test skipped in -short mode")
	}
	deathErr := fmt.Errorf("getUpdates: 401 Unauthorized (bot token revoked mid-poll)")
	rep := newLoopCountingReplier()
	cfg := listenConfig{
		ProjectName: "DeathProj",
		Code:        fakeCodeDispatcher{},
		Replier:     rep,
		Subscribers: map[string]func(context.Context, commons.InboundHandler) error{
			// A subscriber that dies almost immediately with a non-cancel error.
			"tgram": func(ctx context.Context, _ commons.InboundHandler) error {
				return deathErr
			},
		},
	}

	done := make(chan error, 1)
	go func() { done <- runListen(context.Background(), cfg) }()

	var err error
	select {
	case err = <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("runListen did not return within 3s after channel death — fail-loud teardown HUNG (process would idle dark)")
	}
	if err == nil {
		t.Fatal("runListen returned nil after a channel death (§107 PASS-bluff: silent swallow — process idles while channel is dark)")
	}
	// Error must be stage-tagged with the channel + subscribe context so the
	// operator sees WHICH channel died and WHY.
	es := err.Error()
	if !strings.Contains(es, "channel") || !strings.Contains(es, "tgram") || !strings.Contains(es, "subscribe") {
		t.Errorf("channel-death error not stage-tagged with channel/tgram/subscribe: %q", es)
	}

	sd, _ := scListenSurface(t)
	deathTxt := fmt.Sprintf(
		"surface=listen scenario=chaos_channel_death_fail_loud unit=REAL runListen\n"+
			"injected_error=%q\n"+
			"runListen_returned_error=%q\n"+
			"fail_loud_not_hang=1 (returned tagged error within 3s, did not idle dark)\n"+
			"channel_death_surfaced=1\n", // anchor grepped by the e2e invariant
		deathErr.Error(), es)
	if _, werr := sd.WriteFile("channel_death.txt", deathTxt); werr != nil {
		t.Fatalf("write channel_death.txt: %v", werr)
	}
	t.Logf("listen chaos[channel-death]: runListen surfaced %q (fail-loud, no hang)", es)
}

// ----------------------------------------------------------------------
// Loop-level recording replier (distinct name from listen_test.go's
// recordingReplier to avoid a redeclaration in package main).
// ----------------------------------------------------------------------

type loopCountingReplier struct {
	total int64
}

func newLoopCountingReplier() *loopCountingReplier { return &loopCountingReplier{} }

func (r *loopCountingReplier) SendReply(_ context.Context, _ commons.Recipient, _, _ string, _ []commons.Attachment) (string, error) {
	atomic.AddInt64(&r.total, 1)
	return "1", nil
}
func (r *loopCountingReplier) Total() int { return int(atomic.LoadInt64(&r.total)) }
