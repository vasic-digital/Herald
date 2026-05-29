// watch_test.go — HRD-153 (ATMOSphere integration WS-2/WS-7).
//
// End-to-end anti-bluff proof for `pherald watch`: a REAL temp SQLite DB
// (commons_workable.Open) + a REAL commons_watch.Watcher (fsnotify + WAL
// poll) + the REAL commons_workable.Diff + the REAL workflow.Notifier
// feeding the REAL runner.ChannelDispatcher into a RECORDING commons.Channel.
// No part of the watcher→diff→notify pipeline is mocked. The only injected
// seam is the recording channel sink (so we can read back what was dispatched)
// and the temp DB path.
package main

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/vasic-digital/herald/commons"
	workable "github.com/vasic-digital/herald/commons_workable"
	"github.com/vasic-digital/herald/pherald/internal/runner"
	"github.com/vasic-digital/herald/pherald/internal/workflow"
)

// recordingChannel is a real commons.Channel that records every dispatched
// OutboundMessage so the test can assert the rendered diff text actually
// reached the production fan-out.
type recordingChannel struct {
	mu       sync.Mutex
	received []commons.OutboundMessage
}

func (c *recordingChannel) Name() string                       { return string(commons.ChannelNull) }
func (c *recordingChannel) Capabilities() commons.Capabilities { return commons.Capabilities{Text: true} }
func (c *recordingChannel) HealthCheck(ctx context.Context) error {
	return nil
}
func (c *recordingChannel) Subscribe(ctx context.Context, h commons.InboundHandler) error {
	<-ctx.Done()
	return ctx.Err()
}
func (c *recordingChannel) Send(ctx context.Context, msg commons.OutboundMessage) (commons.Receipt, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.received = append(c.received, msg)
	return commons.Receipt{Evidence: commons.DeliveryRouted, ChannelMsgID: "rec-" + msg.EventID}, nil
}
func (c *recordingChannel) bodies() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.received))
	for i, m := range c.received {
		out[i] = m.Body.Plain
	}
	return out
}

// waitForBody blocks until the recording channel has received a message whose
// Body.Plain equals want, or the deadline elapses. Returns the full set of
// bodies seen (for diagnostics on failure).
func waitForBody(t *testing.T, rec *recordingChannel, want string, deadline time.Duration) (bool, []string) {
	t.Helper()
	stop := time.After(deadline)
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		bodies := rec.bodies()
		for _, b := range bodies {
			if b == want {
				return true, bodies
			}
		}
		select {
		case <-stop:
			return false, rec.bodies()
		case <-tick.C:
		}
	}
}

func TestRunWatch_EndToEndOutbound(t *testing.T) {
	dir := t.TempDir()
	dbPath := dir + "/workable_items.db"

	store, err := workable.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	repo := workable.NewRepo(store)

	// Seed one pre-existing item so the prev-snapshot baseline is non-empty
	// (proves create/update/delete are detected against a real baseline, not
	// just an empty-DB special case).
	seed := workable.Item{
		AtmID:           "HRD-900",
		Type:            "Task",
		Status:          "Queued",
		Title:           "seed",
		CurrentLocation: "Issues",
	}
	if err := repo.Create(context.Background(), seed); err != nil {
		t.Fatalf("seed create: %v", err)
	}

	// Recording sink wired into the REAL ChannelDispatcher + REAL Notifier.
	rec := &recordingChannel{}
	dispatcher := &runner.ChannelDispatcher{
		Channels: map[commons.ChannelID]commons.Channel{commons.ChannelNull: rec},
	}
	recipients := []commons.Recipient{{Channel: string(commons.ChannelNull), ChannelUserID: "chat-1", DisplayName: "QA"}}
	notifier := workflow.NewNotifier(dispatcher, recipients)

	ctx, cancel := context.WithCancel(context.Background())

	goroutinesBefore := runtime.NumGoroutine()

	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- runWatch(ctx, watchDeps{
			Repo:         repo,
			Locations:    []string{"Issues", "Fixed"},
			Paths:        []string{dbPath},
			Notifier:     notifier,
			PollInterval: 100 * time.Millisecond,
			Debounce:     50 * time.Millisecond,
			Ready:        ready,
		})
	}()

	// Wait until runWatch has snapshotted its baseline + started the watcher,
	// so every mutation below is guaranteed to be observed (no boot race).
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatalf("runWatch did not signal Ready")
	}

	// (a) Create a NEW item → expect "🆕 … created".
	newItem := workable.Item{
		AtmID:           "HRD-901",
		Type:            "Feature",
		Status:          "Queued",
		Title:           "watch-test new item",
		CurrentLocation: "Issues",
	}
	if err := repo.Create(ctx, newItem); err != nil {
		t.Fatalf("create new item: %v", err)
	}
	if ok, seen := waitForBody(t, rec, "🆕 HRD-901 created", 3*time.Second); !ok {
		cancel()
		t.Fatalf("did not observe created message; saw: %#v", seen)
	}

	// (b) Update its status → expect "🔄 … status: A → B".
	newItem.Status = "In progress"
	if err := repo.Update(ctx, newItem); err != nil {
		t.Fatalf("update status: %v", err)
	}
	if ok, seen := waitForBody(t, rec, "🔄 HRD-901 status: Queued → In progress", 3*time.Second); !ok {
		cancel()
		t.Fatalf("did not observe status-change message; saw: %#v", seen)
	}

	// (c) Delete it → expect "🗑️ … removed".
	if err := repo.Delete(ctx, "HRD-901", "Issues"); err != nil {
		t.Fatalf("delete item: %v", err)
	}
	if ok, seen := waitForBody(t, rec, "🗑️ HRD-901 removed", 3*time.Second); !ok {
		cancel()
		t.Fatalf("did not observe removed message; saw: %#v", seen)
	}

	// Clean shutdown: cancel → runWatch returns promptly, no goroutine leak.
	cancel()
	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Fatalf("runWatch returned non-cancel error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("runWatch did not return after ctx cancel")
	}

	// Allow the watcher's child goroutines to fully unwind.
	for i := 0; i < 50; i++ {
		if runtime.NumGoroutine() <= goroutinesBefore+1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if delta := runtime.NumGoroutine() - goroutinesBefore; delta > 1 {
		t.Errorf("goroutine leak: delta=%d (before=%d after=%d)", delta, goroutinesBefore, runtime.NumGoroutine())
	}
}

func TestWatchCmd_Registered(t *testing.T) {
	cmd := newWatchCmd()
	if cmd.Name() != "watch" {
		t.Fatalf("newWatchCmd().Name() = %q, want %q", cmd.Name(), "watch")
	}
	// --help must exit 0 on the constructed command.
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("watch --help: %v", err)
	}
}
