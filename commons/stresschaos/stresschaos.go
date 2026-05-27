// Package stresschaos is Herald's shared, dependency-light scaffold for the
// §11.4.85 stress + chaos test suite (GAP-3, plan 2026-05-27-stress-chaos-suite).
//
// It is the reusable substrate every per-surface stress/chaos test in Herald
// builds on (Runner pipeline, /v1/events, listen inbound, claude_code dispatch,
// container flows). It lives in commons (Herald's L0 foundation layer) so every
// higher-layer module (pherald, cherald, sherald, …) can import it without a
// new external dependency: the whole package is Go stdlib only (sync, sort,
// time, encoding/json, os/exec, runtime). No golang.org/x/sync, no stats lib.
//
// What it provides, per plan §2:
//
//   - RunLoad: a bounded concurrent load-runner — fan out N workers, each
//     running the same closure M times, capturing per-iteration latency +
//     error so the caller can prove sustained/concurrent load (§11.4.85
//     "N ≥ 10 parallel, no deadlock/leak/race" + "p50/p95/p99 recorded").
//   - Percentiles: sorted-slice percentile math → LatencyStats{p50,p95,p99,
//     max,count}.
//   - Evidence writers: NewRun creates docs/qa/<run-id>/stress_chaos/<surface>/
//     and writes latency.json + latency_histogram.csv + arbitrary files, all
//     real captured runtime output (never metadata-only PASS lines).
//   - HostMemHeadroom: best-effort host-memory probe (vm_stat on darwin,
//     /proc/meminfo on linux) so resource-exhaustion tests can prove the
//     §12.6 60%-host-memory ceiling was never crossed. Probe failure is
//     recorded as "probe-unavailable", never a hard test failure.
//
// §107 anti-bluff: this scaffold MUST NOT manufacture green. The load-runner
// records every error verbatim; the percentile math is exercised by its own
// unit tests against known inputs; the evidence writers write files the e2e
// invariants grep for *specific values*, so a present-but-empty artefact fails.
package stresschaos

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// LoadResult is the per-iteration outcome captured by RunLoad. Latency is
// the wall-clock duration of the closure; Err is whatever the closure
// returned (nil on success). WorkerID + Iter identify the call so a caller
// can correlate a failure back to a specific fan-out slot.
type LoadResult struct {
	WorkerID int
	Iter     int
	Latency  time.Duration
	Err      error
}

// LoadSummary aggregates a RunLoad invocation: total calls, error count, the
// latency distribution, and the wall-clock window the whole fan-out occupied
// (so a caller can compute throughput = Count/Elapsed).
type LoadSummary struct {
	Workers      int           `json:"workers"`
	IterPerWork  int           `json:"iterations_per_worker"`
	Count        int           `json:"count"`
	Errors       int           `json:"errors"`
	Elapsed      time.Duration `json:"-"`
	ElapsedMS    float64       `json:"elapsed_ms"`
	ThroughputPS float64       `json:"throughput_per_sec"`
	Latency      LatencyStats  `json:"latency"`
	Results      []LoadResult  `json:"-"`
}

// LatencyStats is the percentile summary emitted to latency.json. Durations
// are reported in milliseconds (float) so the JSON is human-readable and the
// e2e invariant can grep a numeric p99.
type LatencyStats struct {
	Count int     `json:"count"`
	P50MS float64 `json:"p50_ms"`
	P95MS float64 `json:"p95_ms"`
	P99MS float64 `json:"p99_ms"`
	MaxMS float64 `json:"max_ms"`
	MinMS float64 `json:"min_ms"`
}

// RunLoad fans out `workers` goroutines, each invoking `fn(workerID, iter)`
// exactly `iterPerWorker` times, capturing per-call latency + error. It is
// the canonical bounded concurrent load-runner for the suite.
//
// Host-safety (§12 / §12.6): the caller is responsible for keeping `workers`
// and the per-call allocation bounded (the suite uses N≤50, small payloads).
// RunLoad itself allocates only workers*iterPerWorker LoadResult structs
// (a few KiB at the suite's scale) and never spawns unbounded goroutines —
// exactly `workers` goroutines run, regardless of total iterations.
//
// The fan-out is the concurrency-correctness evidence when run under
// `go test -race`: a data race in the code under load is reported by the
// race detector, turning a clean RunLoad into positive §11.4.85 evidence.
func RunLoad(workers, iterPerWorker int, fn func(workerID, iter int) error) LoadSummary {
	if workers < 1 {
		workers = 1
	}
	if iterPerWorker < 1 {
		iterPerWorker = 1
	}
	total := workers * iterPerWorker
	results := make([]LoadResult, total)
	var errCount int64

	var wg sync.WaitGroup
	wg.Add(workers)
	start := time.Now()
	for w := 0; w < workers; w++ {
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iterPerWorker; i++ {
				t0 := time.Now()
				err := fn(w, i)
				lat := time.Since(t0)
				idx := w*iterPerWorker + i
				results[idx] = LoadResult{WorkerID: w, Iter: i, Latency: lat, Err: err}
				if err != nil {
					atomic.AddInt64(&errCount, 1)
				}
			}
		}(w)
	}
	wg.Wait()
	elapsed := time.Since(start)

	lats := make([]time.Duration, total)
	for i, r := range results {
		lats[i] = r.Latency
	}
	stats := Percentiles(lats)

	tput := 0.0
	if elapsed > 0 {
		tput = float64(total) / elapsed.Seconds()
	}
	return LoadSummary{
		Workers:      workers,
		IterPerWork:  iterPerWorker,
		Count:        total,
		Errors:       int(errCount),
		Elapsed:      elapsed,
		ElapsedMS:    float64(elapsed.Microseconds()) / 1000.0,
		ThroughputPS: tput,
		Latency:      stats,
		Results:      results,
	}
}

// Percentiles computes p50/p95/p99/max/min from a slice of durations using
// the nearest-rank method on a sorted copy (no interpolation — deterministic
// and trivially unit-testable). An empty input yields a zero-value
// LatencyStats with Count=0.
func Percentiles(durs []time.Duration) LatencyStats {
	n := len(durs)
	if n == 0 {
		return LatencyStats{}
	}
	sorted := make([]time.Duration, n)
	copy(sorted, durs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	ms := func(d time.Duration) float64 { return float64(d.Microseconds()) / 1000.0 }
	return LatencyStats{
		Count: n,
		P50MS: ms(percentile(sorted, 50)),
		P95MS: ms(percentile(sorted, 95)),
		P99MS: ms(percentile(sorted, 99)),
		MaxMS: ms(sorted[n-1]),
		MinMS: ms(sorted[0]),
	}
}

// percentile returns the value at the given percentile (1..100) of a slice
// that is ALREADY sorted ascending, using the nearest-rank method:
// rank = ceil(p/100 * n), 1-indexed, clamped to [1,n].
func percentile(sorted []time.Duration, p int) time.Duration {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[n-1]
	}
	// nearest-rank: ceil(p/100 * n)
	rank := (p*n + 99) / 100 // integer ceil of p*n/100
	if rank < 1 {
		rank = 1
	}
	if rank > n {
		rank = n
	}
	return sorted[rank-1]
}

// ----------------------------------------------------------------------
// Evidence writers — docs/qa/<run-id>/stress_chaos/<surface>/
// ----------------------------------------------------------------------

// Run is a handle to one evidence directory tree under
// docs/qa/<run-id>/stress_chaos/. Surface() returns a per-surface
// subdirectory writer.
type Run struct {
	// RunID is the timestamp-based identifier, e.g.
	// "2026-05-27T143501-gap3".
	RunID string
	// Root is the absolute path to docs/qa/<run-id>/stress_chaos.
	Root string
}

// NewRunID builds a timestamp-based run-id with the given suffix tag, e.g.
// NewRunID("gap3") → "2026-05-27T143501-gap3". Deterministic given the
// clock; safe for filesystem paths (no ':' in the time portion).
func NewRunID(tag string) string {
	ts := time.Now().Format("2006-01-02T150405")
	if tag == "" {
		return ts
	}
	return ts + "-" + tag
}

// NewRun creates docs/qa/<run-id>/stress_chaos/ rooted at qaRoot (the absolute
// path to the repo's docs/qa directory) and returns a handle. It mkdir-alls
// the tree so per-surface writers can drop files immediately.
func NewRun(qaRoot, runID string) (*Run, error) {
	root := filepath.Join(qaRoot, runID, "stress_chaos")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("stresschaos: mkdir evidence root %s: %w", root, err)
	}
	return &Run{RunID: runID, Root: root}, nil
}

// Surface returns a SurfaceDir for the named surface (e.g. "runner"),
// creating docs/qa/<run-id>/stress_chaos/<surface>/.
func (r *Run) Surface(name string) (*SurfaceDir, error) {
	dir := filepath.Join(r.Root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("stresschaos: mkdir surface %s: %w", dir, err)
	}
	return &SurfaceDir{Dir: dir}, nil
}

// SurfaceDir is the per-surface evidence directory. All its writers return
// the absolute path written so a caller (and the e2e invariant) can locate
// the artefact.
type SurfaceDir struct {
	Dir string
}

// WriteLatencyJSON writes latency.json carrying the LoadSummary's latency
// stats + count/errors/throughput. This is the §11.4.85 "p50/p95/p99
// recorded" artefact.
func (s *SurfaceDir) WriteLatencyJSON(sum LoadSummary) (string, error) {
	payload := map[string]any{
		"count":              sum.Count,
		"errors":             sum.Errors,
		"workers":            sum.Workers,
		"iterations_each":    sum.IterPerWork,
		"elapsed_ms":         sum.ElapsedMS,
		"throughput_per_sec": sum.ThroughputPS,
		"p50_ms":             sum.Latency.P50MS,
		"p95_ms":             sum.Latency.P95MS,
		"p99_ms":             sum.Latency.P99MS,
		"max_ms":             sum.Latency.MaxMS,
		"min_ms":             sum.Latency.MinMS,
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("stresschaos: marshal latency.json: %w", err)
	}
	b = append(b, '\n')
	path := filepath.Join(s.Dir, "latency.json")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return "", fmt.Errorf("stresschaos: write latency.json: %w", err)
	}
	return path, nil
}

// WriteHistogramCSV writes latency_histogram.csv: one row per iteration with
// worker,iter,latency_ms,error. A plottable, value-bearing artefact (not an
// empty placeholder).
func (s *SurfaceDir) WriteHistogramCSV(sum LoadSummary) (string, error) {
	var sb []byte
	sb = append(sb, "worker,iter,latency_ms,error\n"...)
	for _, r := range sum.Results {
		errStr := ""
		if r.Err != nil {
			errStr = sanitizeCSV(r.Err.Error())
		}
		line := fmt.Sprintf("%d,%d,%.4f,%s\n",
			r.WorkerID, r.Iter, float64(r.Latency.Microseconds())/1000.0, errStr)
		sb = append(sb, line...)
	}
	path := filepath.Join(s.Dir, "latency_histogram.csv")
	if err := os.WriteFile(path, sb, 0o644); err != nil {
		return "", fmt.Errorf("stresschaos: write histogram csv: %w", err)
	}
	return path, nil
}

// WriteFile writes an arbitrary named artefact (e.g. exactly_once.txt,
// summary.md, race_clean.log) into the surface dir and returns its path.
func (s *SurfaceDir) WriteFile(name, content string) (string, error) {
	path := filepath.Join(s.Dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("stresschaos: write %s: %w", name, err)
	}
	return path, nil
}

// sanitizeCSV strips commas/newlines from a field so the CSV stays well-formed.
func sanitizeCSV(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch r {
		case ',', '\n', '\r':
			out = append(out, ' ')
		default:
			out = append(out, r)
		}
	}
	return string(out)
}

// ----------------------------------------------------------------------
// Host-memory headroom probe (§12.6 60%-host-memory ceiling proof)
// ----------------------------------------------------------------------

// MemSnapshot is a best-effort point-in-time host-memory reading. When the
// platform probe is unavailable, Available is false and Note explains why —
// callers MUST treat !Available as "probe-unavailable", never as a failure.
type MemSnapshot struct {
	Available     bool    `json:"available"`
	TotalBytes    uint64  `json:"total_bytes"`
	FreeBytes     uint64  `json:"free_bytes"`
	UsedBytes     uint64  `json:"used_bytes"`
	UsedFraction  float64 `json:"used_fraction"`
	Platform      string  `json:"platform"`
	Note          string  `json:"note,omitempty"`
	CapturedAtRFC string  `json:"captured_at"`
}

// CrossesCeiling reports whether host used-memory is at or above the given
// fraction (e.g. 0.60 for the §12.6 ceiling). When the probe is unavailable
// it returns false (cannot prove a breach), and callers SHOULD record the
// probe-unavailable note rather than proceed with a resource-exhaustion run.
func (m MemSnapshot) CrossesCeiling(fraction float64) bool {
	if !m.Available {
		return false
	}
	return m.UsedFraction >= fraction
}

// HostMemHeadroom reads host memory best-effort: vm_stat + sysctl hw.memsize
// on darwin, /proc/meminfo on linux. Any error → MemSnapshot{Available:false,
// Note: "..."} (never panics, never returns an error to the caller — §12.6
// headroom proof is best-effort by design).
func HostMemHeadroom() MemSnapshot {
	now := time.Now().Format(time.RFC3339)
	switch runtime.GOOS {
	case "darwin":
		return hostMemDarwin(now)
	case "linux":
		return hostMemLinux(now)
	default:
		return MemSnapshot{
			Available:     false,
			Platform:      runtime.GOOS,
			Note:          "probe-unavailable: no host-mem probe for this platform",
			CapturedAtRFC: now,
		}
	}
}
