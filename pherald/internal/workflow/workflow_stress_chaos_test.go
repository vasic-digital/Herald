package workflow

// §11.4.85 stress + chaos evidence for HRD-156 (workable-items → notify
// outbound flow). ANTI-BLUFF: every component under load is REAL — a real
// commons_workable SQLite Store (temp file), a real workflow.Notifier over a
// real runner.ChannelDispatcher, fanning out to a real commons.Channel sink
// (recording for stress, faulting for chaos). NOTHING about the unit under
// test (Diff → Notify → dispatch → Channel.Send) is mocked.
//
// QA-ANCHOR: HRD-156-STRESS-CHAOS-20260530
//
// STRESS (TestNotifier_StressChaos/Stress_NoLossNoDup):
//   - Seed N=50 items into a real SQLite SSoT, snapshot, then concurrently
//     mutate every one (create + status-change) using commons/stresschaos.RunLoad
//     (§11.4.74 reuse) under -race so the SSoT writes are exercised at realistic
//     volume with the race detector live.
//   - Re-snapshot, compute the REAL commons_workable.Diff over prev→curr, then
//     Notify through the real dispatcher into a recording channel.
//   - Assert NO LOSS  : every Diff change produced exactly one rendered message.
//   - Assert NO DUP   : message count == change count (no double-send).
//   - Assert FIDELITY : each rendered body equals RenderChange(change), in order.
//   Deterministic under `-race -count=3`.
//
// CHAOS:
//   - Faulting_SurfacesFailure: a faulting channel that fails on a subset of
//     sends → Notify MUST return an error AND that error MUST identify the
//     specific undelivered change (no silent swallow — the §107/C1 contract:
//     the watch path has no Stage-7 OutcomeRecorder, so the Notifier itself
//     fails loud on the first undelivered notification).
//   - Recovers_AfterFault: once the channel stops faulting, a fresh Notify of
//     the same changes delivers every one — the failure was transport-transient,
//     not a poison-pill that wedges the notifier.

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons/stresschaos"
	workable "github.com/vasic-digital/herald/commons_workable"
	"github.com/vasic-digital/herald/pherald/internal/runner"
)

// subsetFaultingChannel is a REAL commons.Channel (not a mock of the unit
// under test) whose Send fails for any message whose plain body contains one
// of the configured needles, and otherwise records the message. It models a
// channel that drops a subset of sends — the realistic partial-failure chaos
// case. After Heal() it stops faulting so the recovery path can be proven.
type subsetFaultingChannel struct {
	mu       sync.Mutex
	failOn   map[string]struct{} // substrings that trigger a send failure
	healed   bool
	received []commons.OutboundMessage
}

func newSubsetFaultingChannel(failOn ...string) *subsetFaultingChannel {
	m := make(map[string]struct{}, len(failOn))
	for _, s := range failOn {
		m[s] = struct{}{}
	}
	return &subsetFaultingChannel{failOn: m}
}

func (c *subsetFaultingChannel) Name() string { return string(commons.ChannelNull) }
func (c *subsetFaultingChannel) Capabilities() commons.Capabilities {
	return commons.Capabilities{Text: true}
}
func (c *subsetFaultingChannel) HealthCheck(ctx context.Context) error { return nil }
func (c *subsetFaultingChannel) Subscribe(ctx context.Context, h commons.InboundHandler) error {
	<-ctx.Done()
	return ctx.Err()
}

func (c *subsetFaultingChannel) Heal() {
	c.mu.Lock()
	c.healed = true
	c.mu.Unlock()
}

func (c *subsetFaultingChannel) Send(ctx context.Context, msg commons.OutboundMessage) (commons.Receipt, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.healed {
		for needle := range c.failOn {
			if contains(msg.Body.Plain, needle) {
				return commons.Receipt{}, fmt.Errorf("telegram 502: refused %q", msg.Body.Plain)
			}
		}
	}
	c.received = append(c.received, msg)
	return commons.Receipt{Evidence: commons.DeliveryRouted, ChannelMsgID: "rec-" + msg.EventID}, nil
}

func (c *subsetFaultingChannel) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.received)
}

func contains(haystack, needle string) bool {
	if needle == "" {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// openTempStore opens a real commons_workable SQLite store on a temp file.
func openTempStore(t *testing.T) *workable.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "workable_hrd156.db")
	st, err := workable.Open(path)
	if err != nil {
		t.Fatalf("workable.Open(%s): %v", path, err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func snapshot(t *testing.T, repo *workable.Repo, ctx context.Context) []workable.Item {
	t.Helper()
	items, err := repo.List(ctx, "Issues")
	if err != nil {
		t.Fatalf("repo.List(Issues): %v", err)
	}
	return items
}

func TestNotifier_StressChaos(t *testing.T) {
	t.Run("Stress_NoLossNoDup", func(t *testing.T) {
		const n = 50
		ctx := context.Background()
		store := openTempStore(t)
		repo := workable.NewRepo(store)

		// prev snapshot: empty Issues location.
		prev := snapshot(t, repo, ctx)

		// STRESS: N concurrent creators, each inserting one item then updating
		// its status. RunLoad fans out N workers × 1 iter under -race; the real
		// SQLite SSoT is the shared resource under concurrent write load.
		now := time.Now().UTC().Format(time.RFC3339)
		sum := stresschaos.RunLoad(n, 1, func(worker, iter int) error {
			id := fmt.Sprintf("ATM-%04d", worker)
			if err := repo.Create(ctx, workable.Item{
				AtmID: id, Type: "Task", Status: "Queued",
				Title: "load " + id, CurrentLocation: "Issues",
				CreatedAt: now, LastModified: now,
			}); err != nil {
				return fmt.Errorf("create %s: %w", id, err)
			}
			if err := repo.Update(ctx, workable.Item{
				AtmID: id, Type: "Task", Status: "In progress",
				Title: "load " + id, CurrentLocation: "Issues",
				CreatedAt: now, LastModified: now,
			}); err != nil {
				return fmt.Errorf("update %s: %w", id, err)
			}
			return nil
		})
		if sum.Errors != 0 {
			t.Fatalf("stress load reported %d/%d SSoT errors — lost writes under concurrency", sum.Errors, sum.Count)
		}

		// curr snapshot after the concurrent mutation storm.
		curr := snapshot(t, repo, ctx)
		if len(curr) != n {
			t.Fatalf("after stress: %d items in SSoT, want %d (lost/dup writes)", len(curr), n)
		}

		// REAL Diff over prev→curr. Each of the N new items is one created
		// change; the final status is "In progress" so no spurious status delta
		// versus an empty prev.
		changes := workable.Diff(prev, curr)
		if len(changes) != n {
			t.Fatalf("Diff produced %d changes, want %d (one item.created each)", len(changes), n)
		}

		// Notify through the REAL dispatcher into a recording channel.
		rec := &recordingChannel{}
		dispatcher := &runner.ChannelDispatcher{
			Channels: map[commons.ChannelID]commons.Channel{commons.ChannelNull: rec},
			Logger:   nil,
		}
		recipients := []commons.Recipient{{Channel: string(commons.ChannelNull), ChannelUserID: "chat-1"}}
		notifier := NewNotifier(dispatcher, recipients)

		if err := notifier.Notify(ctx, changes); err != nil {
			t.Fatalf("Notify under stress: %v", err)
		}

		got := rec.bodies()
		// NO LOSS + NO DUP: exactly one rendered message per change.
		if len(got) != len(changes) {
			t.Fatalf("NO-LOSS/NO-DUP violated: %d messages for %d changes", len(got), len(changes))
		}
		// FIDELITY: each message is exactly RenderChange(change), in order.
		for i, c := range changes {
			if want := RenderChange(c); got[i] != want {
				t.Fatalf("message[%d] = %q, want %q (change %s/%s)", i, got[i], want, c.AtmID, c.Kind)
			}
		}
		// Every body is non-empty and unique (no silent collapse to one send).
		seen := make(map[string]struct{}, len(got))
		for i, b := range got {
			if b == "" {
				t.Fatalf("message[%d] is empty — rendered nothing", i)
			}
			if _, dup := seen[b]; dup {
				t.Fatalf("duplicate rendered message %q — fan-out double-sent", b)
			}
			seen[b] = struct{}{}
		}
	})

	t.Run("Chaos_Faulting_SurfacesFailure", func(t *testing.T) {
		ctx := context.Background()
		// Channel fails on any send whose body mentions ATM-0002 (a subset of
		// the batch). The change for that id is a real undelivered notification.
		fault := newSubsetFaultingChannel("ATM-0002")
		dispatcher := &runner.ChannelDispatcher{
			Channels: map[commons.ChannelID]commons.Channel{commons.ChannelNull: fault},
			Logger:   nil,
		}
		recipients := []commons.Recipient{{Channel: string(commons.ChannelNull), ChannelUserID: "chat-1"}}
		notifier := NewNotifier(dispatcher, recipients)

		changes := []workable.Change{
			{AtmID: "ATM-0001", Kind: workable.KindCreated},
			{AtmID: "ATM-0002", Kind: workable.KindCreated},
			{AtmID: "ATM-0003", Kind: workable.KindCreated},
		}

		err := notifier.Notify(ctx, changes)
		if err == nil {
			t.Fatal("Notify swallowed a subset send failure — §107 distribution-layer bluff (C1): the operator silently misses ATM-0002")
		}
		// Attributable: the surfaced error must name the change that failed.
		if !contains(err.Error(), "ATM-0002") {
			t.Fatalf("failure not attributed to the undelivered change ATM-0002; got: %v", err)
		}
		// And it must NOT misattribute a delivered change.
		if contains(err.Error(), "ATM-0003") {
			t.Fatalf("failure misattributed to a delivered change ATM-0003; got: %v", err)
		}
	})

	t.Run("Chaos_Recovers_AfterFault", func(t *testing.T) {
		ctx := context.Background()
		fault := newSubsetFaultingChannel("ATM-0002")
		dispatcher := &runner.ChannelDispatcher{
			Channels: map[commons.ChannelID]commons.Channel{commons.ChannelNull: fault},
			Logger:   nil,
		}
		recipients := []commons.Recipient{{Channel: string(commons.ChannelNull), ChannelUserID: "chat-1"}}
		notifier := NewNotifier(dispatcher, recipients)

		changes := []workable.Change{
			{AtmID: "ATM-0001", Kind: workable.KindCreated},
			{AtmID: "ATM-0002", Kind: workable.KindCreated},
			{AtmID: "ATM-0003", Kind: workable.KindCreated},
		}

		// First pass: fails on ATM-0002.
		if err := notifier.Notify(ctx, changes); err == nil {
			t.Fatal("expected the faulting pass to surface an error")
		}

		// Heal the transport and retry: every change must now deliver.
		fault.Heal()
		before := fault.count()
		if err := notifier.Notify(ctx, changes); err != nil {
			t.Fatalf("Notify after channel recovered: %v (channel should no longer fault)", err)
		}
		delivered := fault.count() - before
		if delivered != len(changes) {
			t.Fatalf("after recovery: %d delivered, want %d (transport was not transient)", delivered, len(changes))
		}
	})
}
