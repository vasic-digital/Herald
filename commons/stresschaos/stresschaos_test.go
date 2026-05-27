package stresschaos

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestPercentiles_KnownInput verifies the nearest-rank percentile math
// against a hand-computed 1..100 ms dataset. With 100 samples (1ms..100ms),
// nearest-rank ceil(p/100*100)=p, so p50→sorted[49]=50ms, p95→sorted[94]=95ms,
// p99→sorted[98]=99ms, max→100ms, min→1ms.
func TestPercentiles_KnownInput(t *testing.T) {
	var durs []time.Duration
	for i := 1; i <= 100; i++ {
		durs = append(durs, time.Duration(i)*time.Millisecond)
	}
	st := Percentiles(durs)
	if st.Count != 100 {
		t.Errorf("count = %d, want 100", st.Count)
	}
	checks := []struct {
		name string
		got  float64
		want float64
	}{
		{"p50", st.P50MS, 50},
		{"p95", st.P95MS, 95},
		{"p99", st.P99MS, 99},
		{"max", st.MaxMS, 100},
		{"min", st.MinMS, 1},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v ms, want %v ms", c.name, c.got, c.want)
		}
	}
}

// TestPercentiles_Unsorted proves the function sorts internally (input order
// must not change the result).
func TestPercentiles_Unsorted(t *testing.T) {
	durs := []time.Duration{
		100 * time.Millisecond,
		1 * time.Millisecond,
		50 * time.Millisecond,
		99 * time.Millisecond,
		2 * time.Millisecond,
	}
	st := Percentiles(durs)
	if st.MinMS != 1 {
		t.Errorf("min = %v, want 1", st.MinMS)
	}
	if st.MaxMS != 100 {
		t.Errorf("max = %v, want 100", st.MaxMS)
	}
}

// TestPercentiles_Empty returns a zero-value LatencyStats, never panics.
func TestPercentiles_Empty(t *testing.T) {
	st := Percentiles(nil)
	if st.Count != 0 || st.P99MS != 0 {
		t.Errorf("empty input should give zero stats, got %+v", st)
	}
}

// TestPercentiles_Single — a single sample is every percentile.
func TestPercentiles_Single(t *testing.T) {
	st := Percentiles([]time.Duration{7 * time.Millisecond})
	if st.P50MS != 7 || st.P95MS != 7 || st.P99MS != 7 || st.MaxMS != 7 || st.MinMS != 7 {
		t.Errorf("single sample percentiles should all equal 7ms, got %+v", st)
	}
}

// TestRunLoad_RunsEveryIteration proves the fan-out invokes the closure
// exactly workers*iterPerWorker times and aggregates count + errors honestly.
func TestRunLoad_RunsEveryIteration(t *testing.T) {
	var calls int64
	sum := RunLoad(8, 25, func(workerID, iter int) error {
		atomic.AddInt64(&calls, 1)
		return nil
	})
	if got := atomic.LoadInt64(&calls); got != 200 {
		t.Errorf("closure invoked %d times, want 200", got)
	}
	if sum.Count != 200 {
		t.Errorf("summary count = %d, want 200", sum.Count)
	}
	if sum.Errors != 0 {
		t.Errorf("summary errors = %d, want 0", sum.Errors)
	}
	if sum.Latency.Count != 200 {
		t.Errorf("latency count = %d, want 200", sum.Latency.Count)
	}
	if sum.ThroughputPS <= 0 {
		t.Errorf("throughput should be positive, got %v", sum.ThroughputPS)
	}
}

// TestRunLoad_CountsErrors proves errors returned by the closure are counted
// (anti-bluff: a load-runner that hides errors is a §11.4 PASS-bluff).
func TestRunLoad_CountsErrors(t *testing.T) {
	sum := RunLoad(4, 10, func(workerID, iter int) error {
		if iter%2 == 0 {
			return errors.New("boom")
		}
		return nil
	})
	// 5 even iters per worker × 4 workers = 20 errors.
	if sum.Errors != 20 {
		t.Errorf("errors = %d, want 20", sum.Errors)
	}
	if sum.Count != 40 {
		t.Errorf("count = %d, want 40", sum.Count)
	}
}

// TestRunLoad_BoundedGoroutines proves RunLoad spawns exactly `workers`
// goroutines, not workers*iterations — host-safety invariant (§12).
func TestRunLoad_BoundedGoroutines(t *testing.T) {
	var maxConcurrent int64
	var current int64
	RunLoad(6, 50, func(workerID, iter int) error {
		c := atomic.AddInt64(&current, 1)
		for {
			m := atomic.LoadInt64(&maxConcurrent)
			if c <= m || atomic.CompareAndSwapInt64(&maxConcurrent, m, c) {
				break
			}
		}
		time.Sleep(time.Microsecond)
		atomic.AddInt64(&current, -1)
		return nil
	})
	if got := atomic.LoadInt64(&maxConcurrent); got > 6 {
		t.Errorf("peak concurrency = %d, want <= 6 (bounded by workers)", got)
	}
}

// TestNewRun_CreatesTree proves NewRun + Surface create the
// docs/qa/<run-id>/stress_chaos/<surface>/ tree on disk.
func TestNewRun_CreatesTree(t *testing.T) {
	qaRoot := t.TempDir()
	runID := NewRunID("unit")
	run, err := NewRun(qaRoot, runID)
	if err != nil {
		t.Fatalf("NewRun: %v", err)
	}
	want := filepath.Join(qaRoot, runID, "stress_chaos")
	if run.Root != want {
		t.Errorf("run.Root = %q, want %q", run.Root, want)
	}
	if _, err := os.Stat(run.Root); err != nil {
		t.Fatalf("evidence root not created: %v", err)
	}
	sd, err := run.Surface("runner")
	if err != nil {
		t.Fatalf("Surface: %v", err)
	}
	if _, err := os.Stat(sd.Dir); err != nil {
		t.Fatalf("surface dir not created: %v", err)
	}
}

// TestSurfaceDir_WritesArtefacts proves the latency.json + histogram.csv
// writers produce non-empty, value-bearing files (anti-bluff: a present-but-
// empty artefact must not happen).
func TestSurfaceDir_WritesArtefacts(t *testing.T) {
	qaRoot := t.TempDir()
	run, err := NewRun(qaRoot, NewRunID("unit"))
	if err != nil {
		t.Fatalf("NewRun: %v", err)
	}
	sd, err := run.Surface("runner")
	if err != nil {
		t.Fatalf("Surface: %v", err)
	}

	sum := RunLoad(2, 5, func(workerID, iter int) error { return nil })

	jsonPath, err := sd.WriteLatencyJSON(sum)
	if err != nil {
		t.Fatalf("WriteLatencyJSON: %v", err)
	}
	b, _ := os.ReadFile(jsonPath)
	if len(b) == 0 {
		t.Fatal("latency.json is empty (PASS-bluff)")
	}
	if !strings.Contains(string(b), "p99_ms") {
		t.Errorf("latency.json missing p99_ms field: %s", b)
	}
	if !strings.Contains(string(b), `"count": 10`) {
		t.Errorf("latency.json count != 10: %s", b)
	}

	csvPath, err := sd.WriteHistogramCSV(sum)
	if err != nil {
		t.Fatalf("WriteHistogramCSV: %v", err)
	}
	cb, _ := os.ReadFile(csvPath)
	lines := strings.Count(string(cb), "\n")
	// 1 header + 10 data rows.
	if lines != 11 {
		t.Errorf("histogram csv has %d lines, want 11 (1 header + 10 rows)", lines)
	}

	mdPath, err := sd.WriteFile("summary.md", "# hello\n")
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mb, _ := os.ReadFile(mdPath)
	if string(mb) != "# hello\n" {
		t.Errorf("summary.md content = %q", mb)
	}
}

// TestHostMemHeadroom_BestEffort proves the probe never panics and returns a
// usable snapshot OR a documented probe-unavailable. On the CI host (darwin
// or linux) it SHOULD be available; on an exotic platform it degrades.
func TestHostMemHeadroom_BestEffort(t *testing.T) {
	snap := HostMemHeadroom()
	if snap.Available {
		if snap.TotalBytes == 0 {
			t.Errorf("available snapshot has zero total bytes: %+v", snap)
		}
		if snap.UsedFraction < 0 || snap.UsedFraction > 1.0001 {
			t.Errorf("used fraction out of range: %v", snap.UsedFraction)
		}
		t.Logf("host mem probe: total=%d MiB used=%.1f%% (%s)",
			snap.TotalBytes/(1024*1024), snap.UsedFraction*100, snap.Platform)
	} else {
		if snap.Note == "" {
			t.Errorf("unavailable snapshot must carry a probe-unavailable Note")
		}
		t.Logf("host mem probe unavailable: %s", snap.Note)
	}
}

// TestMemSnapshot_CrossesCeiling exercises the §12.6 ceiling predicate with
// synthetic snapshots (deterministic, no host dependency).
func TestMemSnapshot_CrossesCeiling(t *testing.T) {
	cases := []struct {
		name     string
		snap     MemSnapshot
		fraction float64
		want     bool
	}{
		{"below", MemSnapshot{Available: true, UsedFraction: 0.40}, 0.60, false},
		{"at", MemSnapshot{Available: true, UsedFraction: 0.60}, 0.60, true},
		{"above", MemSnapshot{Available: true, UsedFraction: 0.75}, 0.60, true},
		{"unavailable", MemSnapshot{Available: false, UsedFraction: 0.99}, 0.60, false},
	}
	for _, c := range cases {
		if got := c.snap.CrossesCeiling(c.fraction); got != c.want {
			t.Errorf("%s: CrossesCeiling(%v) = %v, want %v", c.name, c.fraction, got, c.want)
		}
	}
}
