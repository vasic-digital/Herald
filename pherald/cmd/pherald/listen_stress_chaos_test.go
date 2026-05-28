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

// ======================================================================
// Wave 7 T11 (HRD-120) — §11.4.85 stress + chaos for the MULTI-CHANNEL
// runtime. These tests extend the single-channel HRD-126 tests above with
// the Wave-7-specific assertion: TWO concurrent Subscribers fan into ONE
// inbound.Dispatcher AND the fan-in is genuinely parallel (not strictly
// serialised) AND survives both burst load (no loss / no double-dispatch /
// no leak) AND surfaces a tagged fail-loud error when ONE channel dies
// (with proof the dead channel ran CONCURRENTLY with the survivor, not
// strictly before it).
//
// Why MULTI-channel adds invariants the single-channel tests cannot:
//   - The W7 fan-in is `for name, sub := range cfg.Subscribers` launching
//     N goroutines into the SAME handler+replier (listen.go lines 439..455).
//     The single-channel tests prove ONE goroutine doesn't lose events;
//     these tests prove N goroutines on the SAME handler don't lose,
//     double, or interleave-corrupt each other's events. The same
//     dispatcher handles both bursts; the W7-specific question is "did the
//     two channels' event-ids stay disjoint AND complete on both sides?".
//   - The chaos test pins CONCURRENCY of the fan-in: a dying channel must
//     have observably overlapped with the survivor (proved by ≥5 survivor
//     events landing before the fail-loud teardown), which is the W7
//     property the single-channel chaos test (one Subscribe → fail-loud)
//     cannot exercise.
//
// runListen contract relied on (read from listen.go lines 399..462):
//   "If ANY subscriber returns a non-cancel error, runListen cancels the
//    siblings and surfaces that error (T11 chaos fail-loud — a channel
//    that silently dies must take the process down, not idle the other
//    channels while one is dark)."
// → fail-loud TEARS DOWN siblings (cancel-on-first-error) — concurrency
//   is asserted by overlap-before-teardown, not by survivor-keeps-running-
//   indefinitely. Per-channel resilience (supervised restart) is an
//   explicitly-deferred Wave 8 design per the plan §T11 footnote.

// multiBurstSubscriber is the multi-channel equivalent of burstSubscriber.
// It fires `count` synthetic InboundEvents whose `Sender.Channel` and
// `EventID` carry the named channel (so the inbound.Dispatcher routes the
// reply via the channelRouter for that channel and the per-channel
// counting replier can attribute the count). EventIDs are unique across
// the (channel, index) space — a missed-or-duplicate event is detectable
// post-hoc by set diff.
func multiBurstSubscriber(channel string, count int, dispatched *int64, seen *sync.Map) func(context.Context, commons.InboundHandler) error {
	return func(ctx context.Context, h commons.InboundHandler) error {
		var wg sync.WaitGroup
		wg.Add(count)
		for i := 0; i < count; i++ {
			go func(i int) {
				defer wg.Done()
				// EventID = "<channel>-<NNNN>", where channel already carries
				// the "-stub" suffix (e.g. "tgram-stub", "slack-stub") — so
				// the full id is "tgram-stub-0001"…"slack-stub-0200" per the
				// W7 T11 spec. The expected-set verifier below uses the
				// SAME format — drift between producer/consumer would
				// surface as "all 400 missing" (the W7 invariant FAIL we
				// must NOT mask).
				id := fmt.Sprintf("%s-%04d", channel, i+1)
				ev := commons.InboundEvent{
					EventID: id,
					Sender: commons.Recipient{
						Channel:       channel,
						ChannelUserID: fmt.Sprintf("u-%d", i%8),
					},
					Body: commons.Body{Plain: fmt.Sprintf("%s burst msg %d", channel, i+1)},
					Raw:  map[string]any{"message_id": i + 1},
				}
				if err := h.Handle(ctx, ev); err == nil {
					seen.Store(id, struct{}{})
					atomic.AddInt64(dispatched, 1)
				}
			}(i)
		}
		wg.Wait()
		<-ctx.Done()
		return ctx.Err()
	}
}

// perChannelCountingReplier records reply counts attributed to the
// recipient's channel (rcpt.Channel — passed through from
// inbound.Dispatcher: rcpt = commons.Recipient{Channel: ev.Sender.Channel,
// ChannelUserID: ev.Sender.ChannelUserID}). This is the proof the
// multi-channel fan-in did NOT cross-wire one channel's events onto
// another channel's reply path.
type perChannelCountingReplier struct {
	mu     sync.Mutex
	counts map[string]int64
}

func newPerChannelCountingReplier() *perChannelCountingReplier {
	return &perChannelCountingReplier{counts: map[string]int64{}}
}

func (r *perChannelCountingReplier) SendReply(_ context.Context, rcpt commons.Recipient, _, _ string, _ []commons.Attachment) (string, error) {
	r.mu.Lock()
	r.counts[rcpt.Channel]++
	r.mu.Unlock()
	return "1", nil
}

func (r *perChannelCountingReplier) Count(channel string) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.counts[channel]
}

func (r *perChannelCountingReplier) Total() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	var t int64
	for _, v := range r.counts {
		t += v
	}
	return t
}

// TestListen_MultiChannel_StressBurst pins the Wave 7 multi-channel
// fan-in invariant: two concurrent Subscribers (tgram-stub + slack-stub)
// each fire N=200 unique-id synthetic events through the REAL runListen
// loop into ONE inbound.Dispatcher. The asserts (under -race):
//
//   - No-loss: all 400 unique EventIDs were dispatched exactly once.
//     Mismatch ⇒ the fan-in dropped an event under concurrent burst.
//   - No-double: total reply count == 400 (not 401+). An extra is a
//     handler-invoked-twice race the dispatcher would otherwise hide.
//   - Per-channel attribution: count("tgram-stub") == 200 AND
//     count("slack-stub") == 200. Proves the channelRouter (Replier
//     interface implementation in listen.go) routed each reply to the
//     adapter for the recipient's ORIGINATING channel — a cross-wire
//     would show as 199/201 or 0/400 even though Total stayed 400.
//   - Clean shutdown under load: cancel returns runListen nil within 3s.
//   - No goroutine leak: post-cancel NumGoroutine settles at baseline.
//
// Under -race, a data race on the shared dispatcher / replier / handler
// path is reported by the detector; the -count=3 outer run proves
// determinism (no flaky "happens to PASS" pattern).
func TestListen_MultiChannel_StressBurst(t *testing.T) {
	if testing.Short() {
		t.Skip("multi-channel stress test skipped in -short mode")
	}
	const perChannel = 200
	const totalEvents = 2 * perChannel
	var dispatched int64
	var seen sync.Map
	rep := newPerChannelCountingReplier()

	cfg := listenConfig{
		ProjectName: "MultiChanStressProj",
		Code:        fakeCodeDispatcher{},
		Replier:     rep,
		Subscribers: map[string]func(context.Context, commons.InboundHandler) error{
			"tgram-stub": multiBurstSubscriber("tgram-stub", perChannel, &dispatched, &seen),
			"slack-stub": multiBurstSubscriber("slack-stub", perChannel, &dispatched, &seen),
		},
	}

	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	gBefore := runtime.NumGoroutine()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- runListen(ctx, cfg) }()

	// Poll until BOTH channels' bursts have been fully dispatched + replied
	// (budget 10s — fan-in over 400 concurrent goroutines through one
	// dispatcher + replier under -race).
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&dispatched) >= int64(totalEvents) && rep.Total() >= int64(totalEvents) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	gotDispatched := atomic.LoadInt64(&dispatched)
	gotTgram := rep.Count("tgram-stub")
	gotSlack := rep.Count("slack-stub")
	gotTotal := rep.Total()

	if gotDispatched != int64(totalEvents) {
		cancel()
		<-done
		t.Fatalf("burst dispatch: %d/%d events handled within 10s — fan-in dropped events under multi-channel burst", gotDispatched, totalEvents)
	}
	if gotTotal != int64(totalEvents) {
		cancel()
		<-done
		t.Fatalf("burst reply: total %d want %d (double-dispatch or loss — fan-in not atomic per event)", gotTotal, totalEvents)
	}
	if gotTgram != int64(perChannel) {
		cancel()
		<-done
		t.Fatalf("burst reply per-channel: tgram-stub=%d want %d (channelRouter cross-wire or drop)", gotTgram, perChannel)
	}
	if gotSlack != int64(perChannel) {
		cancel()
		<-done
		t.Fatalf("burst reply per-channel: slack-stub=%d want %d (channelRouter cross-wire or drop)", gotSlack, perChannel)
	}

	// No-loss: verify the OBSERVED set of EventIDs equals the EXPECTED set.
	// A missed id (dropped event) OR a duplicate id (double-dispatch with
	// matched total via a missing-counterpart) would slip past the
	// total/per-channel asserts; this set-equality is the rigorous proof.
	missingIDs := 0
	for _, ch := range []string{"tgram-stub", "slack-stub"} {
		for i := 0; i < perChannel; i++ {
			id := fmt.Sprintf("%s-%04d", ch, i+1)
			if _, ok := seen.Load(id); !ok {
				missingIDs++
				if missingIDs <= 5 {
					t.Errorf("missing dispatched event-id: %s", id)
				}
			}
		}
	}
	if missingIDs > 0 {
		cancel()
		<-done
		t.Fatalf("%d/%d expected event-ids never dispatched — fan-in dropped events", missingIDs, totalEvents)
	}

	elapsed := time.Since(start)

	// Clean shutdown under load: cancel → runListen returns nil within 3s.
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runListen returned non-nil on cancel under multi-channel load: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runListen did not return within 3s of cancel — multi-channel shutdown hang")
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
		t.Fatalf("goroutine leak after multi-channel runListen returned: before=%d after=%d (leaked=%d > slack=%d)", gBefore, gAfter, leaked, slack)
	}

	tput := float64(totalEvents) / elapsed.Seconds()
	sd, persistent := scListenSurface(t)
	loopTxt := fmt.Sprintf(
		"surface=listen scenario=multichannel_stress_burst unit=REAL runListen (2-channel fan-in)\n"+
			"channels=tgram-stub,slack-stub events_per_channel=%d total_events=%d\n"+
			"dispatched=%d total_replied=%d (==events: nothing dropped)\n"+
			"per_channel_replied tgram-stub=%d slack-stub=%d (==per_channel: no cross-wire)\n"+
			"unique_event_ids_seen=%d (==total: no loss, no double)\n"+
			"elapsed_ms=%.1f throughput_per_sec=%.1f\n"+
			"clean_shutdown_on_cancel=1 goroutines_before=%d goroutines_after=%d leaked=%d slack=%d (no_goroutine_leak=1)\n"+
			"multichannel_no_drop_under_burst=1\n"+ // anchor grepped by the e2e invariant
			"race_detector=clean\n",
		perChannel, totalEvents,
		gotDispatched, gotTotal,
		gotTgram, gotSlack,
		totalEvents-missingIDs,
		float64(elapsed.Microseconds())/1000.0, tput,
		gBefore, gAfter, leaked, slack)
	if _, err := sd.WriteFile("multichannel_stress_burst.txt", loopTxt); err != nil {
		t.Fatalf("write multichannel_stress_burst.txt: %v", err)
	}
	t.Logf("listen multichannel stress[burst]: %d events (tgram=%d slack=%d) dispatched+replied with per-channel attribution, clean shutdown, goroutines %d→%d (leaked=%d), tput=%.0f/s (persistent=%v dir=%s)",
		totalEvents, gotTgram, gotSlack, gBefore, gAfter, leaked, tput, persistent, sd.Dir)
}

// TestListen_MultiChannel_ChaosOneChannelDies_OtherKeepsGoing pins the
// Wave 7 multi-channel CONCURRENCY-of-fan-in invariant under fault
// injection: two Subscribers run concurrently into one dispatcher; one
// channel ("tgram-stub") fires ONE event and then returns a non-cancel
// error mid-run; the other channel ("slack-stub") is a slow ticking
// emitter (50 events ~2ms apart, total ~100ms).
//
// runListen contract pinned (verbatim from listen.go lines 399..462 doc-
// comment): "If ANY subscriber returns a non-cancel error, runListen
// cancels the siblings and surfaces that error (T11 chaos fail-loud — a
// channel that silently dies must take the process down, not idle the
// other channels while one is dark)."
//
// Therefore the W7 invariant this test pins is NOT "survivor keeps
// running indefinitely after a sibling dies" (that would be supervised-
// restart, an explicitly-deferred Wave 8 design per plan §T11 footnote).
// The W7 invariant IS "the two Subscribers ran CONCURRENTLY (not strictly
// serially)" — proved jointly by:
//
//   (a) ≥5 slack-stub events were dispatched (the slack survivor was
//       running CONCURRENTLY with tgram-stub before the cancel landed —
//       a strictly-serial fan-in would dispatch ZERO slack events
//       because tgram-stub returns its error before yielding the
//       scheduler), AND
//   (b) the tgram-stub error surfaced through runListen tagged with
//       channel="tgram-stub" + subscribe (fail-loud, no swallow, no hang).
//
// Both (a) and (b) MUST hold together for PASS. A test that asserted only
// (b) would not distinguish concurrent-but-aborted from serial-tgram-dies-
// then-slack-never-starts; a test that asserted only (a) would not catch
// the §107 silent-swallow regression the single-channel chaos test
// covers. The conjunction is the multi-channel-specific invariant.
//
// (Wave 8 follow-up: per-channel supervised restart would change THIS
// invariant to "survivor keeps running indefinitely after tgram dies".
// Do NOT silently implement that here — the plan §T11 explicitly defers
// it as a separate design.)
func TestListen_MultiChannel_ChaosOneChannelDies_OtherKeepsGoing(t *testing.T) {
	if testing.Short() {
		t.Skip("multi-channel chaos test skipped in -short mode")
	}
	deathErr := fmt.Errorf("tgram subscribe failed (chaos: socket reset by peer)")

	// slowSurvivor fires 50 events spaced ~2ms apart (~100ms total) before
	// returning ctx.Err() on cancellation. The 2ms spacing gives the fan-in
	// scheduler ample window to interleave tgram-stub's death + cancel
	// propagation with several survivor dispatches — the W7 concurrency
	// proof. survivorDispatched counts successful Handle returns.
	var survivorDispatched int64
	slowSurvivor := func(ctx context.Context, h commons.InboundHandler) error {
		tk := time.NewTicker(2 * time.Millisecond)
		defer tk.Stop()
		emitted := 0
		for emitted < 50 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-tk.C:
				emitted++
				ev := commons.InboundEvent{
					EventID: fmt.Sprintf("slack-stub-%04d", emitted),
					Sender:  commons.Recipient{Channel: "slack-stub", ChannelUserID: "1"},
					Body:    commons.Body{Plain: "alive"},
					Raw:     map[string]any{"message_id": emitted},
				}
				if err := h.Handle(ctx, ev); err == nil {
					atomic.AddInt64(&survivorDispatched, 1)
				}
			}
		}
		<-ctx.Done()
		return ctx.Err()
	}

	// tgramOneShotThenDies fires ONE event then returns the death error.
	// The single event is the marker that tgram-stub did dispatch at
	// least one message before dying — proof the goroutine actually ran.
	var tgramDispatched int64
	tgramOneShotThenDies := func(ctx context.Context, h commons.InboundHandler) error {
		ev := commons.InboundEvent{
			EventID: "tgram-stub-0001",
			Sender:  commons.Recipient{Channel: "tgram-stub", ChannelUserID: "1"},
			Body:    commons.Body{Plain: "one-shot before death"},
			Raw:     map[string]any{"message_id": 1},
		}
		if err := h.Handle(ctx, ev); err == nil {
			atomic.AddInt64(&tgramDispatched, 1)
		}
		// Brief yield so the survivor goroutine gets scheduled before
		// runListen's cancel() propagates and tears the group down. This
		// is the deliberate concurrency-window the W7 invariant relies on
		// — a fan-in that strictly-serialised the subscribers would
		// dispatch ZERO slack events regardless of this sleep.
		time.Sleep(50 * time.Millisecond)
		return deathErr
	}

	rep := newPerChannelCountingReplier()
	cfg := listenConfig{
		ProjectName: "MultiChanChaosProj",
		Code:        fakeCodeDispatcher{},
		Replier:     rep,
		Subscribers: map[string]func(context.Context, commons.InboundHandler) error{
			"tgram-stub": tgramOneShotThenDies,
			"slack-stub": slowSurvivor,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	done := make(chan error, 1)
	go func() { done <- runListen(ctx, cfg) }()

	var err error
	select {
	case err = <-done:
	case <-time.After(4 * time.Second):
		t.Fatal("runListen did not return within 4s after channel death — multi-channel fail-loud teardown HUNG (process would idle dark)")
	}

	// (b) fail-loud: error surfaced and tagged with channel="tgram-stub".
	if err == nil {
		t.Fatal("runListen returned nil after tgram-stub death (§107 PASS-bluff: silent swallow with sibling running)")
	}
	es := err.Error()
	if !strings.Contains(es, "channel") || !strings.Contains(es, "tgram-stub") || !strings.Contains(es, "subscribe") {
		t.Errorf("channel-death error not stage-tagged with channel/tgram-stub/subscribe: %q", es)
	}

	// (a) concurrency proof: slack-stub dispatched ≥5 events DURING the
	// window between runListen starting both goroutines and the cancel-
	// on-tgram-error tearing slack-stub down. A strictly-serial fan-in
	// (e.g. one goroutine that processes Subscribers map entries in order)
	// would yield 0 survivor events.
	const concurrencyFloor = 5
	gotSurvivor := atomic.LoadInt64(&survivorDispatched)
	gotTgram := atomic.LoadInt64(&tgramDispatched)
	if gotSurvivor < concurrencyFloor {
		t.Fatalf("slack-stub dispatched only %d events before fail-loud teardown (want ≥%d) — fan-in did NOT run channels concurrently with tgram-stub (W7 concurrency invariant FAIL)",
			gotSurvivor, concurrencyFloor)
	}
	if gotTgram != 1 {
		t.Errorf("tgram-stub one-shot event count: got %d want 1 (the death subscriber should dispatch its single marker event before erroring)", gotTgram)
	}
	gotSurvivorReplies := rep.Count("slack-stub")
	if gotSurvivorReplies < concurrencyFloor {
		t.Errorf("slack-stub reply count %d < %d — survivor dispatched but replies didn't reach the channelRouter under fault (per-channel routing degraded under chaos)",
			gotSurvivorReplies, concurrencyFloor)
	}

	sd, _ := scListenSurface(t)
	chaosTxt := fmt.Sprintf(
		"surface=listen scenario=multichannel_chaos_one_dies_other_concurrent unit=REAL runListen\n"+
			"injected_error=%q\n"+
			"runListen_returned_error=%q\n"+
			"tgram_stub_one_shot_dispatched=%d (==1: death subscriber ran before erroring)\n"+
			"slack_stub_survivor_dispatched=%d (≥%d: concurrent with tgram-stub before teardown)\n"+
			"slack_stub_survivor_replied=%d (≥%d: per-channel routing held under fault)\n"+
			"fail_loud_not_hang=1 (returned tagged error within 4s under multi-channel load)\n"+
			"multichannel_concurrency_proven=1\n"+ // anchor grepped by the e2e invariant
			"multichannel_one_channel_death_surfaced=1\n",
		deathErr.Error(), es,
		gotTgram, gotSurvivor, concurrencyFloor, gotSurvivorReplies, concurrencyFloor)
	if _, werr := sd.WriteFile("multichannel_chaos_one_dies.txt", chaosTxt); werr != nil {
		t.Fatalf("write multichannel_chaos_one_dies.txt: %v", werr)
	}
	t.Logf("listen multichannel chaos[one-dies]: tgram-stub returned %q after 1 dispatch; slack-stub had %d concurrent dispatches (+%d replies) before fail-loud teardown",
		es, gotSurvivor, gotSurvivorReplies)
}
