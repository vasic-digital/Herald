package safety

import (
	"context"
	"os"
	"runtime"
	"strconv"
	"time"
)

// StartMemSampler starts a goroutine that periodically samples the process's
// memory usage and feeds it to agg.UpdateMemPercent. The goroutine exits when
// ctx is cancelled.
//
// Wave 3a uses Go's runtime.MemStats for portability — `HeapAlloc / Sys * 100`
// is the process-heap fraction of process-total memory obtained from the OS.
// This is NOT the same as host-RAM% (which would require gopsutil + a syscall
// per-OS) but is adequate for the 60% threshold §12.6 watcher in the daemon's
// own scope. Operators wanting accurate host-RAM% can swap to gopsutil in a
// follow-up (HRD-056).
//
// Interval default: 10s. Override via HERALD_SAFETY_MEM_SAMPLE_INTERVAL —
// either a Go duration string ("30s", "5s", "2m") or an integer-seconds value
// ("30"). Invalid values silently fall back to the 10s default; the §107
// covenant for this knob is "never block sampler start on env parsing".
//
// The first sample fires synchronously BEFORE the ticker — so the first
// GET /v1/safety_state read right after serve start gets a non-zero value
// rather than the 0% placeholder.
func StartMemSampler(ctx context.Context, agg *Aggregator) {
	interval := 10 * time.Second
	if s := os.Getenv("HERALD_SAFETY_MEM_SAMPLE_INTERVAL"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			interval = d
		} else if n, err := strconv.Atoi(s); err == nil {
			interval = time.Duration(n) * time.Second
		}
	}
	go func() {
		// Sample immediately so the first /v1/safety_state read has a value:
		agg.UpdateMemPercent(samplePercent())
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				agg.UpdateMemPercent(samplePercent())
			}
		}
	}()
}

// samplePercent returns a percentage of the process's heap allocation
// relative to its Sys (total memory obtained from the OS).
//
// Trade-off: this is the process-heap percentage, NOT host-RAM%. For Wave 3a
// it is a portable approximation; the §12.6 daemon-mode 60% threshold (see
// HRD-056) is intended to be compared against the same metric across runs,
// not against absolute host-RAM. A gopsutil-backed sampler in a later wave
// can swap implementations without changing the Aggregator surface.
func samplePercent() float64 {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	if ms.Sys == 0 {
		return 0
	}
	return float64(ms.HeapAlloc) / float64(ms.Sys) * 100.0
}
