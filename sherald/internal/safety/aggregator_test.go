package safety_test

import (
	"sync"
	"testing"
	"time"

	"github.com/vasic-digital/herald/sherald/internal/safety"
)

// TestAggregator_FreshSnapshot proves NewAggregator returns reasonable defaults
// — OpenEvents=0, LastDestructiveOp=nil, UptimeSeconds>=0. §107 anti-bluff: a
// fresh aggregator must NOT have phantom destructive ops or negative uptime.
func TestAggregator_FreshSnapshot(t *testing.T) {
	a := safety.NewAggregator()
	s := a.Snapshot()
	if s.OpenEvents != 0 {
		t.Errorf("OpenEvents = %d, want 0", s.OpenEvents)
	}
	if s.LastDestructiveOp != nil {
		t.Errorf("LastDestructiveOp not nil on fresh aggregator")
	}
	if s.UptimeSeconds < 0 {
		t.Errorf("UptimeSeconds negative: %d", s.UptimeSeconds)
	}
	if s.Binary != "sherald" {
		t.Errorf("Binary = %q, want %q", s.Binary, "sherald")
	}
}

// TestAggregator_RecordDestructiveOp proves the destructive_op pointer is
// actually stored AND OpenEvents increments. §107 anti-bluff: fetch Snapshot
// AFTER Record and inspect the returned struct — a passing-by-metadata test
// (e.g. asserting only the counter) would still let a broken pointer-copy slip.
func TestAggregator_RecordDestructiveOp(t *testing.T) {
	a := safety.NewAggregator()
	op := safety.DestructiveOp{
		Op:        "git-push-force",
		Path:      "/tmp/repo.git",
		Operator:  "m@m",
		Blocked:   true,
		BlockedAt: time.Now(),
		HRDRule:   "HRD-046",
	}
	a.RecordDestructiveOp(op)
	s := a.Snapshot()
	if s.LastDestructiveOp == nil {
		t.Fatal("LastDestructiveOp nil after RecordDestructiveOp")
	}
	if s.LastDestructiveOp.Op != "git-push-force" {
		t.Errorf("Op = %s", s.LastDestructiveOp.Op)
	}
	if s.LastDestructiveOp.Path != "/tmp/repo.git" {
		t.Errorf("Path = %s", s.LastDestructiveOp.Path)
	}
	if s.LastDestructiveOp.HRDRule != "HRD-046" {
		t.Errorf("HRDRule = %s", s.LastDestructiveOp.HRDRule)
	}
	if s.OpenEvents != 1 {
		t.Errorf("OpenEvents = %d, want 1 (RecordDestructiveOp increments)", s.OpenEvents)
	}
}

// TestAggregator_UpdateMemPercent proves the percent AND timestamp both update.
// §107 anti-bluff: a stub that updated only the percent (or only the timestamp)
// would silently rot the /v1/safety_state response.
func TestAggregator_UpdateMemPercent(t *testing.T) {
	a := safety.NewAggregator()
	a.UpdateMemPercent(42.5)
	s := a.Snapshot()
	if s.CurrentMemPercent != 42.5 {
		t.Errorf("CurrentMemPercent = %v, want 42.5", s.CurrentMemPercent)
	}
	if s.LastMemSampleAt.IsZero() {
		t.Errorf("LastMemSampleAt not set after UpdateMemPercent")
	}
}

// TestAggregator_ConcurrentSnapshot runs 100 goroutines doing UpdateMemPercent
// + Snapshot concurrently. §107 anti-bluff: `-race` MUST pass — otherwise the
// RWMutex is broken (writer-under-RLock or pointer-without-lock).
func TestAggregator_ConcurrentSnapshot(t *testing.T) {
	a := safety.NewAggregator()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.UpdateMemPercent(50.0)
			_ = a.Snapshot()
		}()
	}
	wg.Wait()
	// Race detector + no panic = pass.
}
