package constitution

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Event is the in-process event-bus envelope. The shape mirrors what
// `digital.vasic.eventbus`/`pkg/event.Event` exposes per the Catalogue-Check
// (docs/catalogue-checks/HRD-018-foundation.md §2.2 + §2.12), so the M2
// swap-in is a straight rename + import path change.
type Event struct {
	ID       string            // unique within the bus lifetime; typically UUIDv7
	Type     string            // dot-notation: "digital.vasic.herald.constitution.policy.violation"
	Source   string            // emitting subsystem: "digital.vasic.herald/cherald"
	Time     time.Time         // emit timestamp (UTC)
	Subject  string            // optional subject ref (e.g. "repo:vasic-digital/Herald#v1.4.0")
	Metadata map[string]string // string-only metadata (mirrors the Helix eventbus shape)
	Data     []byte            // opaque payload; typically JSON-encoded
}

// Subscription is a handle to a subscriber's delivery channel. Cancel
// MUST be called when the consumer no longer wants events — otherwise
// the bus will continue trying to deliver and (per OverflowDrop) drop
// events on a full buffer.
type Subscription struct {
	Channel <-chan Event
	cancel  func()
	closed  atomic.Bool
}

// Cancel detaches the subscriber from the bus. Idempotent.
func (s *Subscription) Cancel() {
	if s.closed.CompareAndSwap(false, true) {
		s.cancel()
	}
}

// EventBus is the in-process pub/sub interface. The signature is the
// minimum surface Foundation needs; the production swap (Helix-stack
// `digital.vasic.eventbus`) is a superset.
type EventBus interface {
	// Publish delivers ev to every matching subscriber. Returns immediately
	// after enqueueing — actual delivery is asynchronous. Drops events for
	// any subscriber whose buffer is full beyond PublishTimeout (logged
	// via the metrics counter).
	Publish(ctx context.Context, ev Event) error

	// Subscribe returns a Subscription receiving every Event whose Type is
	// `t` OR (if t is "*") every event on the bus. Future versions will
	// add glob / prefix / metadata filters per the Helix eventbus shape;
	// Foundation only needs the exact-type and wildcard cases.
	Subscribe(t string) (*Subscription, error)

	// Close stops the bus and cancels every outstanding subscription.
	// Idempotent. After Close, Publish + Subscribe both return ErrBusClosed.
	Close() error

	// Metrics returns a snapshot of bus counters. Used by health probes +
	// anti-bluff tests that assert "the emit actually fired."
	Metrics() BusMetrics
}

// BusMetrics is a snapshot of counters for observability + anti-bluff tests.
type BusMetrics struct {
	Published       int64
	Delivered       int64
	Dropped         int64
	Subscribers     int
	PublishedByType map[string]int64
}

// ErrBusClosed is returned by Publish + Subscribe after Close has been called.
var ErrBusClosed = errors.New("constitution: event bus closed")

// MemoryBus is the in-process EventBus implementation. Production deployments
// swap this for `digital.vasic.eventbus` at M2.
type MemoryBus struct {
	mu              sync.RWMutex
	closed          atomic.Bool
	subs            map[string][]*subEntry // keyed by type, with "*" for catch-all
	publishTimeout  time.Duration
	bufferSize      int
	publishedTotal  atomic.Int64
	deliveredTotal  atomic.Int64
	droppedTotal    atomic.Int64
	publishedByType sync.Map // map[string]*atomic.Int64
}

type subEntry struct {
	ch         chan Event
	cancelFunc func()
	once       sync.Once
}

// MemoryBusConfig configures a MemoryBus. Zero values pick sensible defaults.
type MemoryBusConfig struct {
	PublishTimeout time.Duration // default 100ms
	BufferSize     int           // default 256
}

// NewMemoryBus returns a started in-memory event bus.
func NewMemoryBus(cfg MemoryBusConfig) *MemoryBus {
	if cfg.PublishTimeout == 0 {
		cfg.PublishTimeout = 100 * time.Millisecond
	}
	if cfg.BufferSize == 0 {
		cfg.BufferSize = 256
	}
	return &MemoryBus{
		subs:           make(map[string][]*subEntry),
		publishTimeout: cfg.PublishTimeout,
		bufferSize:     cfg.BufferSize,
	}
}

// Publish enqueues ev to every matching subscriber's channel. Honors
// ctx cancellation. Drops on subscriber buffer overflow rather than blocking.
func (b *MemoryBus) Publish(ctx context.Context, ev Event) error {
	if b.closed.Load() {
		return ErrBusClosed
	}
	b.publishedTotal.Add(1)
	// LoadOrStore atomically resolves the per-type counter: concurrent
	// publishers of a NEW ev.Type would otherwise both Load-miss → both
	// create+Store a fresh counter → one overwrites the other (lost
	// increment + data race on the map entry). LoadOrStore guarantees
	// exactly one counter instance per type with no lost Add.
	counter, _ := b.publishedByType.LoadOrStore(ev.Type, new(atomic.Int64))
	counter.(*atomic.Int64).Add(1)

	b.mu.RLock()
	matches := append([]*subEntry(nil), b.subs[ev.Type]...)
	matches = append(matches, b.subs["*"]...)
	b.mu.RUnlock()

	for _, s := range matches {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case s.ch <- ev:
			b.deliveredTotal.Add(1)
		case <-time.After(b.publishTimeout):
			b.droppedTotal.Add(1)
		}
	}
	return nil
}

// Subscribe registers a subscriber for type t (or "*" for all).
func (b *MemoryBus) Subscribe(t string) (*Subscription, error) {
	if b.closed.Load() {
		return nil, ErrBusClosed
	}
	ch := make(chan Event, b.bufferSize)
	entry := &subEntry{ch: ch}

	cancel := func() {
		entry.once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			arr := b.subs[t]
			for i, s := range arr {
				if s == entry {
					b.subs[t] = append(arr[:i], arr[i+1:]...)
					break
				}
			}
			close(ch)
		})
	}
	entry.cancelFunc = cancel

	b.mu.Lock()
	b.subs[t] = append(b.subs[t], entry)
	b.mu.Unlock()

	return &Subscription{Channel: ch, cancel: cancel}, nil
}

// Close stops the bus and cancels every outstanding subscription.
func (b *MemoryBus) Close() error {
	if !b.closed.CompareAndSwap(false, true) {
		return nil
	}
	b.mu.Lock()
	all := make([]*subEntry, 0)
	for _, arr := range b.subs {
		all = append(all, arr...)
	}
	b.subs = nil
	b.mu.Unlock()
	for _, s := range all {
		s.cancelFunc()
	}
	return nil
}

// Metrics returns a snapshot.
func (b *MemoryBus) Metrics() BusMetrics {
	out := BusMetrics{
		Published:       b.publishedTotal.Load(),
		Delivered:       b.deliveredTotal.Load(),
		Dropped:         b.droppedTotal.Load(),
		PublishedByType: make(map[string]int64),
	}
	b.publishedByType.Range(func(k, v any) bool {
		out.PublishedByType[k.(string)] = v.(*atomic.Int64).Load()
		return true
	})
	b.mu.RLock()
	for _, arr := range b.subs {
		out.Subscribers += len(arr)
	}
	b.mu.RUnlock()
	return out
}
