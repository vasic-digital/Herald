package constitution

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMemoryBus_PublishDeliversToSubscribers(t *testing.T) {
	bus := NewMemoryBus(MemoryBusConfig{})
	defer bus.Close()

	sub, err := bus.Subscribe("test.type")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Cancel()

	if err := bus.Publish(context.Background(), Event{
		ID: "id-1", Type: "test.type", Source: "test", Time: time.Now(),
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case e := <-sub.Channel:
		if e.ID != "id-1" {
			t.Errorf("delivered event ID = %q; want id-1", e.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive event within 1s")
	}

	// Anti-bluff metrics check.
	m := bus.Metrics()
	if m.Published != 1 {
		t.Errorf("Metrics.Published = %d; want 1", m.Published)
	}
	if m.Delivered != 1 {
		t.Errorf("Metrics.Delivered = %d; want 1", m.Delivered)
	}
	if m.PublishedByType["test.type"] != 1 {
		t.Errorf("Metrics.PublishedByType[test.type] = %d; want 1", m.PublishedByType["test.type"])
	}
}

func TestMemoryBus_WildcardSubscriberReceivesAll(t *testing.T) {
	bus := NewMemoryBus(MemoryBusConfig{})
	defer bus.Close()

	all, err := bus.Subscribe("*")
	if err != nil {
		t.Fatalf("Subscribe(*): %v", err)
	}
	defer all.Cancel()

	bus.Publish(context.Background(), Event{ID: "1", Type: "a"})
	bus.Publish(context.Background(), Event{ID: "2", Type: "b"})

	received := make(map[string]bool)
	for i := 0; i < 2; i++ {
		select {
		case e := <-all.Channel:
			received[e.ID] = true
		case <-time.After(time.Second):
			t.Fatalf("wildcard subscriber missed event after %d delivered", i)
		}
	}
	if !received["1"] || !received["2"] {
		t.Errorf("wildcard subscriber received %v; want both 1 and 2", received)
	}
}

func TestMemoryBus_NonMatchingTypeIsNotDelivered(t *testing.T) {
	bus := NewMemoryBus(MemoryBusConfig{})
	defer bus.Close()

	sub, _ := bus.Subscribe("a")
	defer sub.Cancel()

	bus.Publish(context.Background(), Event{ID: "1", Type: "b"})
	select {
	case e := <-sub.Channel:
		t.Errorf("subscriber to type 'a' received type 'b' event: %+v", e)
	case <-time.After(100 * time.Millisecond):
		// expected
	}
}

func TestMemoryBus_CancelStopsDelivery(t *testing.T) {
	bus := NewMemoryBus(MemoryBusConfig{})
	defer bus.Close()

	sub, _ := bus.Subscribe("x")
	sub.Cancel() // before any publish
	sub.Cancel() // idempotent

	// Channel should be closed.
	if _, ok := <-sub.Channel; ok {
		t.Errorf("subscription channel did not close on Cancel")
	}

	bus.Publish(context.Background(), Event{ID: "1", Type: "x"})
	// Should NOT have delivered (subscriber gone), so Delivered count is 0.
	if d := bus.Metrics().Delivered; d != 0 {
		t.Errorf("Metrics.Delivered after Cancel+Publish = %d; want 0", d)
	}
}

func TestMemoryBus_CloseRejectsFurtherOps(t *testing.T) {
	bus := NewMemoryBus(MemoryBusConfig{})
	_ = bus.Close()
	_ = bus.Close() // idempotent

	if err := bus.Publish(context.Background(), Event{}); err == nil {
		t.Errorf("Publish after Close did not error")
	}
	if _, err := bus.Subscribe("x"); err == nil {
		t.Errorf("Subscribe after Close did not error")
	}
}

func TestMemoryBus_OverflowDrops(t *testing.T) {
	// Tiny buffer + slow consumer = forced drops.
	bus := NewMemoryBus(MemoryBusConfig{
		PublishTimeout: 10 * time.Millisecond,
		BufferSize:     2,
	})
	defer bus.Close()

	sub, _ := bus.Subscribe("x")
	// Don't drain — let buffer fill.

	for i := 0; i < 10; i++ {
		_ = bus.Publish(context.Background(), Event{ID: "id", Type: "x"})
	}
	// Drain after to be polite.
	go func() {
		for range sub.Channel {
		}
	}()
	sub.Cancel()
	time.Sleep(50 * time.Millisecond)

	m := bus.Metrics()
	if m.Dropped == 0 {
		t.Errorf("expected at least 1 drop on overflow; got Dropped=%d (Published=%d, Delivered=%d)",
			m.Dropped, m.Published, m.Delivered)
	}
}

func TestMemoryBus_ConcurrentPublishSafe(t *testing.T) {
	// Race-tested anti-bluff: hammer with concurrent Publish from many goroutines.
	bus := NewMemoryBus(MemoryBusConfig{BufferSize: 1024})
	defer bus.Close()
	sub, _ := bus.Subscribe("*")
	defer sub.Cancel()

	const N = 200
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = bus.Publish(context.Background(), Event{ID: "id", Type: "concurrent", Source: "src"})
			_ = i
		}()
	}
	wg.Wait()

	// Drain.
	count := 0
	done := time.After(500 * time.Millisecond)
	for {
		select {
		case _, ok := <-sub.Channel:
			if !ok {
				goto Done
			}
			count++
			if count == N {
				goto Done
			}
		case <-done:
			goto Done
		}
	}
Done:
	m := bus.Metrics()
	if m.Published != N {
		t.Errorf("Published = %d; want %d", m.Published, N)
	}
	// Allow some drops on tight timeout but most must deliver.
	if m.Delivered+m.Dropped != N {
		t.Errorf("Delivered+Dropped = %d; want %d", m.Delivered+m.Dropped, N)
	}
}
