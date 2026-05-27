package constitution_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/vasic-digital/herald/commons/stresschaos"
	constitution "github.com/vasic-digital/herald/commons_constitution"
	"github.com/vasic-digital/herald/commons_constitution/ladder"
	"github.com/vasic-digital/herald/commons_constitution/state"
)

// qaRoot returns the repo-root docs/qa directory for §107.x / §11.4.85
// evidence. Tests run from the package dir, so go up one level.
func qaRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	return filepath.Join(filepath.Dir(wd), "docs", "qa")
}

// TestStress_EmitPersistNoLostWrites is the §11.4.85 stress proof for the
// HRD-018 audit write-through: N concurrent goroutines each drive Runner.Run
// with a UNIQUE (rule, subject) so every call is a CHANGED FirstSeen
// transition in ModeEnforce → exactly one audit row each. We then assert the
// audit store holds EXACTLY N rows (no lost writes, no double writes) and the
// run is -race clean. Evidence lands under docs/qa/HRD-018-<run>/stress_chaos.
func TestStress_EmitPersistNoLostWrites(t *testing.T) {
	ctx := context.Background()
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{BufferSize: 1024})
	defer bus.Close()
	em, err := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "stress"})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}
	reg := constitution.NewRegistry()
	st := state.NewMemory()
	la := ladder.NewMemory()
	au := state.NewMemoryAudit()
	runner, err := constitution.NewRunner(reg, la, st, em, au)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	tenant := uuid.New()
	bundle := constitution.CaptureBytes([]byte("stress-bundle"))

	const workers, iters = 32, 16 // 512 unique emit→persist round-trips, bounded per §12.6
	summary := stresschaos.RunLoad(workers, iters, func(w, i int) error {
		rule := fmt.Sprintf("§stress-%d-%d", w, i)
		subj := constitution.Subject{Kind: "file", ID: fmt.Sprintf("/s/%d/%d", w, i)}
		ev := &evalForTest{id: rule, sev: constitution.SeverityHigh,
			result: makeResult(constitution.DecisionFail, "stress-violation")}
		out, rerr := runner.Run(ctx, ev, tenant, subj, bundle)
		if rerr != nil {
			return rerr
		}
		if !out.Audited {
			return fmt.Errorf("worker %d iter %d: expected Audited", w, i)
		}
		return nil
	})

	if summary.Errors != 0 {
		t.Fatalf("stress run had %d errors out of %d calls", summary.Errors, summary.Count)
	}

	rows, err := au.ListAudit(ctx, tenant, constitution.AuditQuery{})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	want := workers * iters
	if len(rows) != want {
		t.Fatalf("LOST/DUP WRITE: audit rows = %d; want exactly %d (one per unique transition)", len(rows), want)
	}
	// Every row must carry the emitted event ID (enforce) + the bundle hash.
	for _, r := range rows {
		if r.EmittedEventID == uuid.Nil {
			t.Fatalf("audit row %s/%s missing EmittedEventID under load", r.RuleID, r.Subject)
		}
		if r.BundleHash != bundle {
			t.Fatalf("audit row %s/%s bundle hash mismatch under load", r.RuleID, r.Subject)
		}
	}

	// Capture evidence into the stable canonical HRD-018 run dir (re-runs
	// overwrite rather than accumulate timestamped dirs).
	run, err := stresschaos.NewRun(qaRoot(t), "HRD-018-20260527T142556Z")
	if err != nil {
		t.Fatalf("NewRun: %v", err)
	}
	sd, err := run.Surface("emit_persist")
	if err != nil {
		t.Fatalf("Surface: %v", err)
	}
	if _, err := sd.WriteLatencyJSON(summary); err != nil {
		t.Fatalf("WriteLatencyJSON: %v", err)
	}
	if _, err := sd.WriteHistogramCSV(summary); err != nil {
		t.Fatalf("WriteHistogramCSV: %v", err)
	}
	if _, err := sd.WriteFile("assertion.txt", fmt.Sprintf(
		"workers=%d iters=%d calls=%d errors=%d audit_rows=%d (want %d) throughput=%.1f/s p99=%.2fms\nNO LOST WRITES: PASS\n",
		workers, iters, summary.Count, summary.Errors, len(rows), want, summary.ThroughputPS, summary.Latency.P99MS,
	)); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Logf("stress emit→persist: %d calls, 0 errors, %d audit rows, throughput=%.1f/s p99=%.2fms",
		summary.Count, len(rows), summary.ThroughputPS, summary.Latency.P99MS)
}

// TestChaos_ConcurrentLadderFlips is the §11.4.85 chaos proof for the admin
// mode-flip path: many goroutines flip the SAME (tenant, rule) binding
// concurrently to randomly-chosen modes (state-corruption injection). The
// ladder must remain internally consistent — the final Get must return one of
// the written modes (never a torn / impossible value), and the run is -race
// clean. This exercises the Set/Get contention that the admin REST PUT sits
// on top of.
func TestChaos_ConcurrentLadderFlips(t *testing.T) {
	ctx := context.Background()
	la := ladder.NewMemory()
	tenant := uuid.New()
	rule := "§contended"
	modesCycle := []constitution.Mode{constitution.ModeAllow, constitution.ModeWarn, constitution.ModeEnforce}

	const workers, iters = 24, 40
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				m := modesCycle[(w+i)%len(modesCycle)]
				if err := la.Set(ctx, tenant, rule, m, fmt.Sprintf("ops-%d", w)); err != nil {
					t.Errorf("Set under contention: %v", err)
					return
				}
				// Interleave reads to surface any read/write race.
				if _, err := la.Get(ctx, tenant, rule); err != nil {
					t.Errorf("Get under contention: %v", err)
					return
				}
			}
		}(w)
	}
	wg.Wait()

	final, err := la.Get(ctx, tenant, rule)
	if err != nil {
		t.Fatalf("final Get: %v", err)
	}
	ok := false
	for _, m := range modesCycle {
		if final == m {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("CHAOS CORRUPTION: final mode %v is not one of the written modes", final)
	}

	run, err := stresschaos.NewRun(qaRoot(t), "HRD-018-20260527T142556Z")
	if err != nil {
		t.Fatalf("NewRun: %v", err)
	}
	sd, err := run.Surface("ladder_flips")
	if err != nil {
		t.Fatalf("Surface: %v", err)
	}
	if _, err := sd.WriteFile("assertion.txt", fmt.Sprintf(
		"workers=%d iters=%d total_flips=%d final_mode=%s\nNO TORN STATE under concurrent flips: PASS\n",
		workers, iters, workers*iters, final.String(),
	)); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Logf("chaos ladder flips: %d concurrent flips, final mode=%s, no corruption", workers*iters, final)
}
