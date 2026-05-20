// Package commons — clock abstraction per spec V3 §3.5.
//
// Herald never calls time.Now() directly outside this file. The Clock
// abstraction lets tests fast-forward time, which is essential for
// quiet-hours, batching windows, retry backoff, idempotency TTLs, and
// escalation chains.
package commons

import (
	"sync"
	"time"
)

// Clock is the time-source abstraction (spec §3.5).
type Clock interface {
	Now() time.Time
	Since(t time.Time) time.Duration
	Sleep(d time.Duration)
	After(d time.Duration) <-chan time.Time
	NewTimer(d time.Duration) Timer
}

// Timer abstracts over time.Timer for FakeClock support.
type Timer interface {
	C() <-chan time.Time
	Stop() bool
}

// RealClock wraps the stdlib time package. Used in production.
type RealClock struct{}

// Now returns the current time (real wall clock).
func (RealClock) Now() time.Time                  { return time.Now() }
// Since returns the elapsed time since t.
func (RealClock) Since(t time.Time) time.Duration { return time.Since(t) }
// Sleep blocks for the given duration.
func (RealClock) Sleep(d time.Duration)            { time.Sleep(d) }
// After returns a channel that fires once d has elapsed.
func (RealClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}
// NewTimer wraps time.NewTimer in the Timer interface.
func (RealClock) NewTimer(d time.Duration) Timer {
	t := time.NewTimer(d)
	return realTimer{t}
}

type realTimer struct{ t *time.Timer }

func (rt realTimer) C() <-chan time.Time { return rt.t.C }
func (rt realTimer) Stop() bool          { return rt.t.Stop() }

// FakeClock is the test implementation. Advance(d) moves the clock
// forward by d and fires any pending After/Timer channels whose
// deadlines the advance crosses (spec §3.5).
type FakeClock struct {
	mu      sync.Mutex
	now     time.Time
	pending []*fakeTimer
}

// NewFakeClock returns a FakeClock anchored at a deterministic instant.
func NewFakeClock() *FakeClock {
	return &FakeClock{now: time.Date(2026, time.May, 20, 12, 0, 0, 0, time.UTC)}
}

// Now returns the fake clock's current instant.
func (f *FakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Since returns the elapsed time since t against the fake clock.
func (f *FakeClock) Since(t time.Time) time.Duration {
	return f.Now().Sub(t)
}

// Sleep advances the fake clock by d (does NOT actually sleep).
func (f *FakeClock) Sleep(d time.Duration) {
	f.Advance(d)
}

// After returns a channel that fires when the fake clock advances by d.
func (f *FakeClock) After(d time.Duration) <-chan time.Time {
	t := f.NewTimer(d)
	return t.C()
}

// NewTimer returns a fake Timer that fires when Advance crosses its deadline.
func (f *FakeClock) NewTimer(d time.Duration) Timer {
	f.mu.Lock()
	defer f.mu.Unlock()
	ft := &fakeTimer{
		deadline: f.now.Add(d),
		ch:       make(chan time.Time, 1),
	}
	f.pending = append(f.pending, ft)
	return ft
}

// Advance moves the fake clock forward by d, firing any timers whose
// deadlines the advance crosses.
func (f *FakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
	remaining := f.pending[:0]
	for _, ft := range f.pending {
		if !f.now.Before(ft.deadline) && !ft.stopped {
			ft.fire(f.now)
		} else {
			remaining = append(remaining, ft)
		}
	}
	f.pending = remaining
}

type fakeTimer struct {
	deadline time.Time
	ch       chan time.Time
	stopped  bool
}

func (ft *fakeTimer) C() <-chan time.Time { return ft.ch }
func (ft *fakeTimer) Stop() bool {
	already := ft.stopped
	ft.stopped = true
	return !already
}
func (ft *fakeTimer) fire(now time.Time) {
	select {
	case ft.ch <- now:
	default:
	}
}

// Default is the process-global Clock. Production main() leaves it as
// RealClock{}; tests swap it in TestMain. Anyone calling time.Now()
// outside this file is a bug — the herald-no-direct-time-now lint rule
// (planned per spec §3.5) flags violations.
var Default Clock = RealClock{}
