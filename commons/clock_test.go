package commons

import (
	"testing"
	"time"
)

func TestFakeClockAdvanceFiresTimer(t *testing.T) {
	fc := NewFakeClock()
	tm := fc.NewTimer(5 * time.Second)

	select {
	case <-tm.C():
		t.Fatal("timer fired before Advance")
	default:
	}

	fc.Advance(5 * time.Second)

	select {
	case got := <-tm.C():
		if got.Sub(fc.Now()) != 0 {
			t.Errorf("timer fired at %v, expected fake-now %v", got, fc.Now())
		}
	default:
		t.Fatal("timer did not fire after Advance(5s)")
	}
}

func TestFakeClockNowIsDeterministic(t *testing.T) {
	fc := NewFakeClock()
	a := fc.Now()
	b := fc.Now()
	if !a.Equal(b) {
		t.Errorf("FakeClock.Now drifted between back-to-back calls: %v vs %v", a, b)
	}
	fc.Advance(time.Hour)
	c := fc.Now()
	if c.Sub(a) != time.Hour {
		t.Errorf("after Advance(1h), expected delta 1h, got %v", c.Sub(a))
	}
}

func TestRealClockDoesNotPanic(t *testing.T) {
	// RealClock just delegates to time package; ensure interface is satisfied.
	var c Clock = RealClock{}
	_ = c.Now()
	_ = c.Since(time.Now())
	tm := c.NewTimer(time.Millisecond)
	defer tm.Stop()
}
