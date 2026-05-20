// Package null implements the §11.14 sandbox/no-op channel adapter.
//
// `null://` is the in-process equivalent of /dev/null with full
// instrumentation. Every Send call records the OutboundMessage to an
// in-memory ring buffer, increments per-tag counters, and returns the
// configured DeliveryEvidence ceiling. Used by:
//
//   - Unit tests for commons_messaging routing.
//   - Load tests measuring router/queue throughput without upstream
//     rate-limit interference.
//   - Quickstart/training so operators can send test events before
//     configuring real channel credentials.
//   - Chaos testing — configure fail_rate to exercise retry / DLQ paths.
//
// URL grammar (spec §11.14):
//
//	null://[?seed=<int>&fail_rate=<0..1>&latency_ms=<int>&ceiling=<Accepted|Routed|Delivered|Read>&tags=<csv>]
//
// MUST NOT be enabled in production deployments — the planned operator
// gate CM-NULL-CHANNEL-DISABLED-IN-PROD verifies absence from
// channel_addresses when [herald].environment=production.
package null

import (
	"context"
	"errors"
	"math/rand"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vasic-digital/herald/commons"
)

// Adapter is the §11.14 sandbox channel adapter.
//
// Construct via New(url) so URL params seed the configuration; the
// zero-value Adapter is also valid (defaults: no failures, no latency,
// Routed ceiling, no tags).
type Adapter struct {
	failRate  float64
	latency   time.Duration
	ceiling   commons.DeliveryEvidence
	tagsCSV   string
	rng       *rand.Rand

	mu       sync.Mutex
	ring     []commons.OutboundMessage // bounded; head is oldest
	stats    map[string]int            // outcome → count
	tagStats map[string]int            // tag → count
	cap      int
}

// New parses the adapter URL and returns a configured Adapter.
func New(rawURL string) (*Adapter, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "null" {
		return nil, errors.New("null adapter requires null:// URL scheme")
	}
	q := u.Query()
	a := &Adapter{
		ceiling:  commons.DeliveryRouted,
		cap:      1000,
		stats:    map[string]int{},
		tagStats: map[string]int{},
	}
	if v := q.Get("fail_rate"); v != "" {
		fr, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, err
		}
		if fr < 0 || fr > 1 {
			return nil, errors.New("fail_rate must be in [0,1]")
		}
		a.failRate = fr
	}
	if v := q.Get("latency_ms"); v != "" {
		ms, err := strconv.Atoi(v)
		if err != nil {
			return nil, err
		}
		a.latency = time.Duration(ms) * time.Millisecond
	}
	if v := q.Get("ceiling"); v != "" {
		switch strings.ToLower(v) {
		case "accepted":
			a.ceiling = commons.DeliveryAccepted
		case "routed":
			a.ceiling = commons.DeliveryRouted
		case "delivered":
			a.ceiling = commons.DeliveryDelivered
		case "read":
			a.ceiling = commons.DeliveryRead
		default:
			return nil, errors.New("ceiling must be one of: accepted|routed|delivered|read")
		}
	}
	if v := q.Get("tags"); v != "" {
		a.tagsCSV = v
	}
	seed := time.Now().UnixNano()
	if v := q.Get("seed"); v != "" {
		s, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, err
		}
		seed = s
	}
	a.rng = rand.New(rand.NewSource(seed))
	return a, nil
}

// Name returns the canonical channel ID.
func (a *Adapter) Name() string { return string(commons.ChannelNull) }

// Capabilities advertises what null:// supports.
func (a *Adapter) Capabilities() commons.Capabilities {
	return commons.Capabilities{
		Text:             true,
		Markdown:         true,
		HTML:             true,
		Attachments:      true,
		AttachmentMaxMiB: 100,
		Threads:          true,
		InteractiveURL:   true,
		InteractiveCall:  true,
		DeliveryCeiling:  a.ceiling,
	}
}

// Send records the OutboundMessage to the ring buffer, optionally
// fails per fail_rate, optionally sleeps per latency_ms, and returns
// the configured ceiling as DeliveryEvidence.
func (a *Adapter) Send(ctx context.Context, msg commons.OutboundMessage) (commons.Receipt, error) {
	if a.latency > 0 {
		select {
		case <-ctx.Done():
			return commons.Receipt{}, ctx.Err()
		case <-time.After(a.latency):
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.failRate > 0 && a.rng.Float64() < a.failRate {
		a.stats["fail"]++
		return commons.Receipt{}, errors.New("null adapter: synthetic failure (fail_rate)")
	}

	// Record into the bounded ring buffer.
	if len(a.ring) >= a.cap {
		a.ring = a.ring[1:]
	}
	a.ring = append(a.ring, msg)
	a.stats["ok"]++

	// Tag counter (URL ?tags=...).
	if a.tagsCSV != "" {
		for _, t := range strings.Split(a.tagsCSV, ",") {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			a.tagStats[t]++
		}
	}

	return commons.Receipt{
		Evidence:      a.ceiling,
		ChannelMsgID:  "null-" + msg.EventID,
		SentAt:        commons.Default.Now(),
		LatencyMillis: a.latency.Milliseconds(),
		Native:        map[string]any{"sink": "null", "tags": a.tagsCSV},
	}, nil
}

// Subscribe is a no-op for null:// — there's no upstream to poll.
// Returns immediately so callers can compose it into a fan-out without
// special-casing.
func (a *Adapter) Subscribe(ctx context.Context, h commons.InboundHandler) error {
	<-ctx.Done()
	return ctx.Err()
}

// HealthCheck always succeeds for null://.
func (a *Adapter) HealthCheck(ctx context.Context) error { return nil }

// --- inspector API (test-only) -----------------------------------------

// Messages returns a snapshot copy of the ring buffer.
func (a *Adapter) Messages() []commons.OutboundMessage {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]commons.OutboundMessage, len(a.ring))
	copy(out, a.ring)
	return out
}

// Stats returns a copy of the per-outcome counters.
func (a *Adapter) Stats() map[string]int {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make(map[string]int, len(a.stats))
	for k, v := range a.stats {
		out[k] = v
	}
	return out
}

// TagStats returns a copy of the per-tag counters.
func (a *Adapter) TagStats() map[string]int {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make(map[string]int, len(a.tagStats))
	for k, v := range a.tagStats {
		out[k] = v
	}
	return out
}

// Clear empties the ring buffer + resets stats. Used by tests.
func (a *Adapter) Clear() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ring = a.ring[:0]
	a.stats = map[string]int{}
	a.tagStats = map[string]int{}
}
