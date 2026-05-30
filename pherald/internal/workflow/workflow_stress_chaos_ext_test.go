package workflow

// §11.4.85 stress + chaos — EXTENDED coverage for the workable-item
// change→notification path. Complements workflow_stress_chaos_test.go (which
// proves N=50 SSoT-write stress + subset-fault + recovery) with the heavier,
// fault-injection-shaped scenarios the §11.4.85 mandate enumerates:
//
//   STRESS:
//     - Stress_5kChanges_ManyRecipients_Concurrent: 5,000 distinct rendered
//       messages fanned out to many recipients, with many Notifiers driven
//       CONCURRENTLY (one per recipient) against the SAME real dispatcher +
//       SAME real counting/recording sink, under -race. Asserts EXACTLY-ONCE
//       delivery (total Send count == 5000 * recipients), per-(recipient,msg)
//       uniqueness (no dup, no drop), full content fidelity (every rendered
//       body present), and a bounded wall-clock budget.
//
//   CHAOS (fault injection, deterministic-by-index — no Math.random):
//     - Chaos_EveryNthSendFails_SurfacesLoudly: a channel that fails every
//       Nth Send keyed off a monotonic send index. Notify MUST return an
//       error naming the FIRST undelivered AtmID (the §107 C1 guarantee holds
//       under intermittent fault), never a silent swallow.
//     - Chaos_PartialBatchFailureMidStream: the channel delivers a prefix of
//       the batch then fails mid-stream; Notify stops at and names the exact
//       failing change, and the count of delivered-before-failure is exactly
//       the prefix that preceded it.
//     - Chaos_SlowChannel_ContextDeadline: a channel that blocks past a short
//       context deadline; Notify surfaces the deadline as a loud error naming
//       the undelivered change rather than hanging or silently dropping.
//
// QA-ANCHOR: HRD-156-STRESS-CHAOS-EXT-20260530
//
// ANTI-BLUFF: every assertion checks a real captured count / real recorded
// body / real error string — no metadata-only or absence-of-error PASS. The
// dispatcher, the Notifier, and the commons.Channel sink are all REAL; only
// the transport-failure injection is synthetic (that IS the chaos).

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vasic-digital/herald/commons"
	workable "github.com/vasic-digital/herald/commons_workable"
	"github.com/vasic-digital/herald/pherald/internal/runner"
)

// countingRecorder is a REAL commons.Channel that records every delivered
// message body keyed by the recipient it was sent to, with a monotonic total
// Send counter. Safe for concurrent Send from many Notifiers at once. It is
// the exactly-once oracle for the 5k stress fan-out.
type countingRecorder struct {
	mu    sync.Mutex
	total int64
	// byRecipient[chatID] = multiset of bodies delivered to that recipient.
	byRecipient map[string]map[string]int
}

func newCountingRecorder() *countingRecorder {
	return &countingRecorder{byRecipient: map[string]map[string]int{}}
}

func (c *countingRecorder) Name() string                       { return string(commons.ChannelNull) }
func (c *countingRecorder) Capabilities() commons.Capabilities { return commons.Capabilities{Text: true} }
func (c *countingRecorder) HealthCheck(ctx context.Context) error {
	return nil
}
func (c *countingRecorder) Subscribe(ctx context.Context, h commons.InboundHandler) error {
	<-ctx.Done()
	return ctx.Err()
}
func (c *countingRecorder) Send(ctx context.Context, msg commons.OutboundMessage) (commons.Receipt, error) {
	atomic.AddInt64(&c.total, 1)
	// The recipient is carried in msg.To (one per dispatch).
	chat := ""
	if len(msg.To) > 0 {
		chat = msg.To[0].ChannelUserID
	}
	c.mu.Lock()
	m := c.byRecipient[chat]
	if m == nil {
		m = map[string]int{}
		c.byRecipient[chat] = m
	}
	m[msg.Body.Plain]++
	c.mu.Unlock()
	return commons.Receipt{Evidence: commons.DeliveryRouted, ChannelMsgID: "rec-" + msg.EventID}, nil
}

func (c *countingRecorder) totalSends() int64 { return atomic.LoadInt64(&c.total) }

// TestNotifier_StressChaos_Ext is the extended §11.4.85 suite.
func TestNotifier_StressChaos_Ext(t *testing.T) {
	t.Run("Stress_5kChanges_ManyRecipients_Concurrent", func(t *testing.T) {
		const nChanges = 5000
		const nRecipients = 8

		// Build 5k DISTINCT changes (unique AtmID => unique rendered body) so
		// loss/dup are detectable by content, not just count.
		changes := make([]workable.Change, nChanges)
		for i := range changes {
			changes[i] = workable.Change{
				AtmID: fmt.Sprintf("ATM-%05d", i),
				Kind:  workable.KindCreated,
			}
		}
		// Precompute the expected rendered body set for fidelity checking.
		wantBodies := make(map[string]struct{}, nChanges)
		for _, c := range changes {
			wantBodies[RenderChange(c)] = struct{}{}
		}

		rec := newCountingRecorder()
		dispatcher := &runner.ChannelDispatcher{
			Channels: map[commons.ChannelID]commons.Channel{commons.ChannelNull: rec},
			Logger:   nil,
		}

		// One Notifier per recipient, each fanning the SAME 5k changes — driven
		// CONCURRENTLY so the shared dispatcher + shared sink are exercised under
		// genuine parallel load (the -race evidence). Each recipient is distinct
		// so exactly-once is per-(recipient,message).
		recipients := make([]string, nRecipients)
		for r := range recipients {
			recipients[r] = fmt.Sprintf("chat-%02d", r)
		}

		start := time.Now()
		var wg sync.WaitGroup
		errs := make([]error, nRecipients)
		wg.Add(nRecipients)
		for r := 0; r < nRecipients; r++ {
			go func(r int) {
				defer wg.Done()
				n := NewNotifier(dispatcher, []commons.Recipient{
					{Channel: string(commons.ChannelNull), ChannelUserID: recipients[r]},
				})
				errs[r] = n.Notify(context.Background(), changes)
			}(r)
		}
		wg.Wait()
		elapsed := time.Since(start)

		for r, err := range errs {
			if err != nil {
				t.Fatalf("recipient %d Notify failed under stress: %v", r, err)
			}
		}

		// EXACTLY-ONCE (count): total Send calls == changes * recipients.
		wantTotal := int64(nChanges * nRecipients)
		if got := rec.totalSends(); got != wantTotal {
			t.Fatalf("EXACTLY-ONCE violated: %d total Send calls, want %d (%d changes × %d recipients)",
				got, wantTotal, nChanges, nRecipients)
		}

		// Per-recipient: every one of the 5k distinct bodies delivered EXACTLY
		// once — proves no drop and no duplicate, by content.
		rec.mu.Lock()
		defer rec.mu.Unlock()
		if len(rec.byRecipient) != nRecipients {
			t.Fatalf("delivered to %d distinct recipients, want %d", len(rec.byRecipient), nRecipients)
		}
		for chat, bodies := range rec.byRecipient {
			if len(bodies) != nChanges {
				t.Fatalf("recipient %s received %d distinct bodies, want %d (drop or collapse)", chat, len(bodies), nChanges)
			}
			for body, cnt := range bodies {
				if cnt != 1 {
					t.Fatalf("recipient %s received body %q %d times, want exactly 1 (duplicate fan-out)", chat, body, cnt)
				}
				if _, ok := wantBodies[body]; !ok {
					t.Fatalf("recipient %s received an unexpected body %q (corruption)", chat, body)
				}
			}
		}

		// Bounded wall-clock: 8×5k = 40k in-process Send calls must finish well
		// under a generous ceiling. This is a regression tripwire for an
		// accidental O(n²) or lock-contention blowup, not a microbenchmark.
		const budget = 30 * time.Second
		if elapsed > budget {
			t.Fatalf("fan-out of %d×%d took %v, exceeds %v budget (perf regression)", nChanges, nRecipients, elapsed, budget)
		}
		t.Logf("stress evidence: %d total Send calls across %d recipients in %v (%.0f msg/s)",
			rec.totalSends(), nRecipients, elapsed, float64(wantTotal)/elapsed.Seconds())
	})

	t.Run("Chaos_EveryNthSendFails_SurfacesLoudly", func(t *testing.T) {
		const everyN = 4
		fault := newIndexFaultingChannel(everyN, 0)
		dispatcher := &runner.ChannelDispatcher{
			Channels: map[commons.ChannelID]commons.Channel{commons.ChannelNull: fault},
			Logger:   nil,
		}
		recipients := []commons.Recipient{{Channel: string(commons.ChannelNull), ChannelUserID: "chat-1"}}
		notifier := NewNotifier(dispatcher, recipients)

		// 10 changes; send indices 0..9. everyN=4 => index 3,7 fail (1-based
		// every 4th). Notifier processes in order and fails loud on the FIRST
		// undelivered change. The first failing send is index 3 => the 4th
		// change, ATM-00003.
		changes := make([]workable.Change, 10)
		for i := range changes {
			changes[i] = workable.Change{AtmID: fmt.Sprintf("ATM-%05d", i), Kind: workable.KindCreated}
		}

		err := notifier.Notify(context.Background(), changes)
		if err == nil {
			t.Fatal("Notify swallowed an every-Nth send failure — §107 C1 distribution bluff")
		}
		// Deterministic-by-index: the FIRST failure is the 4th send (0-based
		// index 3) => ATM-00003. The error must name it.
		if !strings.Contains(err.Error(), "ATM-00003") {
			t.Fatalf("error did not name the first undelivered change ATM-00003; got: %v", err)
		}
		// And must NOT have processed past the failure (ATM-00004 was never
		// attempted, so must not appear in the error).
		if strings.Contains(err.Error(), "ATM-00004") {
			t.Fatalf("error misattributed to a change after the failure point; got: %v", err)
		}
		// Positive evidence: the 3 sends before the failure (indices 0,1,2)
		// were delivered.
		if got := fault.delivered(); got != 3 {
			t.Fatalf("expected exactly 3 deliveries before the first fault (index 3), got %d", got)
		}
	})

	t.Run("Chaos_PartialBatchFailureMidStream", func(t *testing.T) {
		// Fail starting at the 6th send (0-based index 5). Indices 0..4 deliver,
		// index 5 fails — a clean mid-stream partial-batch failure.
		fault := newIndexFaultingChannel(0, 5) // everyN=0 disables modulo; failFrom=5
		dispatcher := &runner.ChannelDispatcher{
			Channels: map[commons.ChannelID]commons.Channel{commons.ChannelNull: fault},
			Logger:   nil,
		}
		recipients := []commons.Recipient{{Channel: string(commons.ChannelNull), ChannelUserID: "chat-1"}}
		notifier := NewNotifier(dispatcher, recipients)

		changes := make([]workable.Change, 12)
		for i := range changes {
			changes[i] = workable.Change{AtmID: fmt.Sprintf("ATM-%05d", i), Kind: workable.KindCreated}
		}

		err := notifier.Notify(context.Background(), changes)
		if err == nil {
			t.Fatal("Notify swallowed a mid-stream partial-batch failure — §107 C1 bluff")
		}
		if !strings.Contains(err.Error(), "ATM-00005") {
			t.Fatalf("error did not name the mid-stream failing change ATM-00005; got: %v", err)
		}
		// Exactly the 5-message prefix delivered before the failure.
		if got := fault.delivered(); got != 5 {
			t.Fatalf("partial-batch: expected 5 delivered before mid-stream failure, got %d", got)
		}
	})

	t.Run("Chaos_SlowChannel_ContextDeadline", func(t *testing.T) {
		// A channel whose Send blocks 200ms; a 50ms context deadline must trip
		// first. The dispatcher's Channel.Send honours ctx, returns ctx.Err(),
		// which the Notifier surfaces loudly with the undelivered AtmID.
		slow := &slowChannel{block: 200 * time.Millisecond}
		dispatcher := &runner.ChannelDispatcher{
			Channels: map[commons.ChannelID]commons.Channel{commons.ChannelNull: slow},
			Logger:   nil,
		}
		recipients := []commons.Recipient{{Channel: string(commons.ChannelNull), ChannelUserID: "chat-1"}}
		notifier := NewNotifier(dispatcher, recipients)

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		start := time.Now()
		err := notifier.Notify(ctx, []workable.Change{{AtmID: "ATM-SLOW", Kind: workable.KindCreated}})
		elapsed := time.Since(start)

		if err == nil {
			t.Fatal("Notify swallowed a context-deadline timeout — §107 C1 bluff: the operator silently misses a slow-channel drop")
		}
		if !strings.Contains(err.Error(), "ATM-SLOW") {
			t.Fatalf("timeout error did not name the undelivered change ATM-SLOW; got: %v", err)
		}
		// It must have returned PROMPTLY at the deadline, not after the full
		// 200ms block — proves the deadline actually interrupted the send.
		if elapsed > 150*time.Millisecond {
			t.Fatalf("Notify took %v — did not honour the 50ms deadline (hung on the slow channel)", elapsed)
		}
		t.Logf("deadline evidence: slow-channel Notify aborted in %v (< 200ms block) with loud error", elapsed)
	})
}

// indexFaultingChannel is a REAL commons.Channel that fails sends based on a
// monotonic, deterministic send index — NO randomness. Two independent
// triggers compose:
//
//   - everyN>0: the (index+1)%everyN==0 send fails (every Nth, 1-based).
//   - failFrom>=0 with everyN==0: every send with index>=failFrom fails
//     (mid-stream partial-batch failure).
//
// All other sends are delivered and counted.
type indexFaultingChannel struct {
	everyN   int
	failFrom int
	mu       sync.Mutex
	idx      int
	count    int
}

func newIndexFaultingChannel(everyN, failFrom int) *indexFaultingChannel {
	return &indexFaultingChannel{everyN: everyN, failFrom: failFrom}
}

func (c *indexFaultingChannel) Name() string { return string(commons.ChannelNull) }
func (c *indexFaultingChannel) Capabilities() commons.Capabilities {
	return commons.Capabilities{Text: true}
}
func (c *indexFaultingChannel) HealthCheck(ctx context.Context) error { return nil }
func (c *indexFaultingChannel) Subscribe(ctx context.Context, h commons.InboundHandler) error {
	<-ctx.Done()
	return ctx.Err()
}
func (c *indexFaultingChannel) Send(ctx context.Context, msg commons.OutboundMessage) (commons.Receipt, error) {
	c.mu.Lock()
	i := c.idx
	c.idx++
	fail := false
	if c.everyN > 0 {
		fail = (i+1)%c.everyN == 0
	} else if i >= c.failFrom {
		fail = true
	}
	if !fail {
		c.count++
	}
	c.mu.Unlock()
	if fail {
		return commons.Receipt{}, fmt.Errorf("transport fault at send index %d: %q", i, msg.Body.Plain)
	}
	return commons.Receipt{Evidence: commons.DeliveryRouted, ChannelMsgID: "rec-" + msg.EventID}, nil
}

func (c *indexFaultingChannel) delivered() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
}

// slowChannel is a REAL commons.Channel whose Send blocks for `block` or until
// the context is cancelled, whichever comes first — modelling a slow/hung
// transport for the deadline chaos case.
type slowChannel struct{ block time.Duration }

func (c *slowChannel) Name() string                       { return string(commons.ChannelNull) }
func (c *slowChannel) Capabilities() commons.Capabilities { return commons.Capabilities{Text: true} }
func (c *slowChannel) HealthCheck(ctx context.Context) error {
	return nil
}
func (c *slowChannel) Subscribe(ctx context.Context, h commons.InboundHandler) error {
	<-ctx.Done()
	return ctx.Err()
}
func (c *slowChannel) Send(ctx context.Context, msg commons.OutboundMessage) (commons.Receipt, error) {
	select {
	case <-time.After(c.block):
		return commons.Receipt{Evidence: commons.DeliveryRouted, ChannelMsgID: "rec-" + msg.EventID}, nil
	case <-ctx.Done():
		return commons.Receipt{}, ctx.Err()
	}
}
