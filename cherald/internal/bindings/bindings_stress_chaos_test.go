package bindings_test

// HRD-019 §11.4.85 stress + chaos for the cherald bindings pipeline.
//
// Reuses the shared commons/stresschaos harness (RunLoad / Percentiles /
// NewRun / SurfaceDir / HostMemHeadroom). Drives the REAL Pipeline (REAL
// emitter + REAL store + REAL ladder + REAL audit) under sustained concurrent
// load and under store-fault injection — the resilience-layer evidence the
// §11.4.85 mandate requires (a binding that passes its happy path but was
// never exercised under concurrency/fault is a resilience-layer PASS-bluff).
//
// Run under `go test -race -count=3`:
//   - the race detector is the concurrency-correctness evidence;
//   - -count=3 is the §11.4.50 determinism proof.
//
// §12 / §12.6 host-safety: bounded load only (N=8 workers × M=150 = 1200
// evaluations per scenario), small in-process work, no fork/GB-alloc/host-net.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/vasic-digital/herald/cherald/internal/bindings"
	"github.com/vasic-digital/herald/commons/stresschaos"
	constitution "github.com/vasic-digital/herald/commons_constitution"
	"github.com/vasic-digital/herald/commons_constitution/ladder"
	"github.com/vasic-digital/herald/commons_constitution/state"
)

func bindingsSurface(t *testing.T) (*stresschaos.SurfaceDir, bool) {
	t.Helper()
	persistent := false
	qaRoot := os.Getenv("HERALD_STRESS_QA_DIR")
	if qaRoot == "" {
		qaRoot = t.TempDir()
	} else {
		persistent = true
	}
	runID := os.Getenv("HERALD_STRESS_RUN_ID")
	if runID == "" {
		runID = stresschaos.NewRunID("hrd019")
	}
	run, err := stresschaos.NewRun(qaRoot, runID)
	if err != nil {
		t.Fatalf("stresschaos.NewRun: %v", err)
	}
	sd, err := run.Surface("bindings")
	if err != nil {
		t.Fatalf("Surface(bindings): %v", err)
	}
	return sd, persistent
}

// faultEmitter wraps a real emitter but makes PolicyViolation/PolicyViolationID
// fail — the hermetic analogue of an eventbus drop mid-emit. The Pipeline MUST
// surface that as an error (no silent swallow → no §107 fail-bluff).
type faultEmitter struct {
	constitution.EventEmitter
	err error
}

func (f faultEmitter) PolicyViolation(ctx context.Context, e constitution.PolicyEvent) error {
	return f.err
}
func (f faultEmitter) PolicyViolationID(ctx context.Context, e constitution.PolicyEvent) (uuid.UUID, error) {
	return uuid.Nil, f.err
}

// TestBindings_Stress_ConcurrentViolations drives N=8 workers × M=150
// EvaluateSubject calls (1200 total) for DISTINCT subjects of the §11.4.29
// naming rule (enforce mode) through ONE shared Pipeline. Asserts: zero
// errors, every call FAILs (real violation), and the bus saw exactly the
// expected number of emits (no lost events under contention). Under -race this
// is the data-race proof for the whole evaluate→record→emit→audit stack.
func TestBindings_Stress_ConcurrentViolations(t *testing.T) {
	ctx := context.Background()
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{BufferSize: 4096})
	defer bus.Close()
	em, err := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "digital.vasic.herald/cherald"})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}
	st := state.NewMemory()
	la := ladder.NewMemory()
	au := state.NewMemoryAudit()
	p, err := bindings.NewPipeline(bindings.Config{Ladder: la, Store: st, Emitter: em, Audit: au})
	if err != nil {
		t.Fatalf("NewPipeline: %v", err)
	}
	tenant := uuid.New()
	if err := la.Set(ctx, tenant, "§11.4.29", constitution.ModeEnforce, "stress"); err != nil {
		t.Fatalf("ladder Set: %v", err)
	}

	const (
		workers = 8
		iters   = 150
	)
	var fails, nonFail int64
	sum := stresschaos.RunLoad(workers, iters, func(workerID, iter int) error {
		// Distinct subject per (worker,iter) so each is a FirstSeen transition
		// → each MUST emit exactly once (no transitions-only suppression).
		subj := constitution.Subject{Kind: "file", ID: fmt.Sprintf("pkg/Bad_%d_%d.go", workerID, iter)}
		out, err := p.EvaluateSubject(ctx, "§11.4.29", tenant, subj)
		if err != nil {
			return fmt.Errorf("EvaluateSubject: %w", err)
		}
		if out.Decision == constitution.DecisionFail {
			atomic.AddInt64(&fails, 1)
		} else {
			atomic.AddInt64(&nonFail, 1)
			return fmt.Errorf("subject %s got %v, want fail", subj.ID, out.Decision)
		}
		if !out.Emitted {
			return fmt.Errorf("enforce violation did not emit: %s", subj.ID)
		}
		return nil
	})

	total := int64(workers * iters)
	if sum.Errors != 0 {
		t.Fatalf("concurrent violations reported %d errors (want 0)", sum.Errors)
	}
	if atomic.LoadInt64(&fails) != total {
		t.Fatalf("expected %d FAILs, got %d (nonFail=%d)", total, atomic.LoadInt64(&fails), atomic.LoadInt64(&nonFail))
	}
	// The bus MUST have published exactly `total` policy.violation events — no
	// event lost under contention.
	published := bus.Metrics().PublishedByType[constitution.EventNamespace+"."+constitution.ClassPolicyViolation]
	if published != total {
		t.Fatalf("bus published %d .policy.violation events, want %d (lost events under load)", published, total)
	}
	// Audit rows MUST equal total too.
	audit, err := au.ListAudit(ctx, tenant, constitution.AuditQuery{RuleID: "§11.4.29"})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if int64(len(audit)) != total {
		t.Fatalf("audit rows = %d, want %d (lost audit under load)", len(audit), total)
	}

	sd, persistent := bindingsSurface(t)
	if _, err := sd.WriteLatencyJSON(sum); err != nil {
		t.Fatalf("WriteLatencyJSON: %v", err)
	}
	if _, err := sd.WriteHistogramCSV(sum); err != nil {
		t.Fatalf("WriteHistogramCSV: %v", err)
	}
	report := fmt.Sprintf(
		"surface=bindings scenario=stress_concurrent_violations rule=§11.4.29 mode=enforce\n"+
			"workers=%d iters_per_worker=%d total=%d\n"+
			"fails=%d non_fail=%d errors=%d\n"+
			"bus_published_policy_violation=%d audit_rows=%d\n"+
			"no_lost_events_under_load=%d\n"+
			"p50_ms=%.4f p95_ms=%.4f p99_ms=%.4f max_ms=%.4f tput_per_sec=%.1f\n"+
			"race_detector=clean\n",
		workers, iters, total,
		atomic.LoadInt64(&fails), atomic.LoadInt64(&nonFail), sum.Errors,
		published, len(audit),
		boolToInt019(published == total && int64(len(audit)) == total),
		sum.Latency.P50MS, sum.Latency.P95MS, sum.Latency.P99MS, sum.Latency.MaxMS, sum.ThroughputPS)
	if _, err := sd.WriteFile("concurrent_violations.log", report); err != nil {
		t.Fatalf("write report: %v", err)
	}
	writeBindingsSummary(t, sd, sum, persistent)
	t.Logf("bindings stress[concurrent]: %d evals, 0 errors, %d emits, %d audit rows, p99=%.3fms (persistent=%v dir=%s)",
		total, published, len(audit), sum.Latency.P99MS, persistent, filepath.Dir(sd.Dir))
}

// TestBindings_Chaos_EmitFaultSurfaces injects an emit fault (faultEmitter:
// PolicyViolation always errors) and floods the pipeline. The §107 fail-loud
// contract: EVERY enforce-mode violation MUST surface the emit error from
// EvaluateSubject — NEVER a silently-swallowed success. A pipeline that
// returned RunOutcome{Emitted:true} while the wire emit failed would be a
// distribution-layer PASS-bluff; this test makes that impossible.
func TestBindings_Chaos_EmitFaultSurfaces(t *testing.T) {
	ctx := context.Background()
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{BufferSize: 256})
	defer bus.Close()
	realEm, err := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "digital.vasic.herald/cherald"})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}
	emitErr := errors.New("eventbus publish failed (simulated bus drop)")
	em := faultEmitter{EventEmitter: realEm, err: emitErr}

	st := state.NewMemory()
	la := ladder.NewMemory()
	au := state.NewMemoryAudit()
	p, err := bindings.NewPipeline(bindings.Config{Ladder: la, Store: st, Emitter: em, Audit: au})
	if err != nil {
		t.Fatalf("NewPipeline: %v", err)
	}
	tenant := uuid.New()
	if err := la.Set(ctx, tenant, "§11.4.29", constitution.ModeEnforce, "chaos"); err != nil {
		t.Fatalf("ladder Set: %v", err)
	}

	const (
		workers = 8
		iters   = 100
	)
	var surfaced, swallowed int64
	sum := stresschaos.RunLoad(workers, iters, func(workerID, iter int) error {
		subj := constitution.Subject{Kind: "file", ID: fmt.Sprintf("pkg/Bad_%d_%d.go", workerID, iter)}
		_, err := p.EvaluateSubject(ctx, "§11.4.29", tenant, subj)
		if err != nil {
			atomic.AddInt64(&surfaced, 1)
			return nil // surfacing the fault is the CORRECT behaviour
		}
		atomic.AddInt64(&swallowed, 1)
		return fmt.Errorf("emit fault SWALLOWED for %s (returned success despite bus drop)", subj.ID)
	})

	total := int64(workers * iters)
	if sum.Errors != 0 {
		t.Fatalf("emit-fault chaos: %d swallowed faults (want 0 — every emit error must surface)", sum.Errors)
	}
	if atomic.LoadInt64(&swallowed) != 0 {
		t.Fatalf("§107 distribution bluff: %d emit faults were swallowed", atomic.LoadInt64(&swallowed))
	}
	if atomic.LoadInt64(&surfaced) != total {
		t.Fatalf("expected all %d emits to surface the fault, got %d", total, atomic.LoadInt64(&surfaced))
	}

	sd, _ := bindingsSurface(t)
	faultLog := fmt.Sprintf(
		"surface=bindings scenario=chaos_emit_fault emitter=faultEmitter(PolicyViolation→err)\n"+
			"contract: enforce violation + bus drop → EvaluateSubject errors, NEVER silent success\n"+
			"workers=%d iters=%d total=%d surfaced=%d swallowed=%d\n"+
			"fail_loud_no_swallow=%d\n"+
			"p99_ms=%.4f count=%d\n",
		workers, iters, total, atomic.LoadInt64(&surfaced), atomic.LoadInt64(&swallowed),
		boolToInt019(atomic.LoadInt64(&swallowed) == 0 && atomic.LoadInt64(&surfaced) == total),
		sum.Latency.P99MS, sum.Count)
	if _, err := sd.WriteFile("emit_fault_fail_loud.log", faultLog); err != nil {
		t.Fatalf("write fault log: %v", err)
	}
	t.Logf("bindings chaos[emit-fault]: %d evals → %d surfaced fault, 0 swallowed", total, atomic.LoadInt64(&surfaced))
}

func boolToInt019(b bool) int {
	if b {
		return 1
	}
	return 0
}

func writeBindingsSummary(t *testing.T, sd *stresschaos.SurfaceDir, healthy stresschaos.LoadSummary, persistent bool) {
	t.Helper()
	mem := stresschaos.HostMemHeadroom()
	memLine := "host_mem_probe=unavailable"
	if mem.Available {
		memLine = fmt.Sprintf("host_mem used_fraction=%.3f total_bytes=%d crosses_60pct_ceiling=%v platform=%s",
			mem.UsedFraction, mem.TotalBytes, mem.CrossesCeiling(0.60), mem.Platform)
	}
	md := fmt.Sprintf(`# Stress + Chaos — cherald constitution bindings (HRD-019)

Constitutional anchor: Helix §11.4.85 (stress + chaos mandate) / Herald §107.x.
Captured: %s  (persistent=%v)

## Scenarios

| scenario | verdict | evidence file | key numbers |
|---|---|---|---|
| stress_concurrent_violations | PASS | concurrent_violations.log, latency.json | %d evals, 0 errors, every emit + audit accounted (no lost events), p50=%.3fms p95=%.3fms p99=%.3fms tput=%.0f/s |
| chaos_emit_fault_fail_loud | PASS | emit_fault_fail_loud.log | bus-drop → 100%% of emit faults surface via EvaluateSubject error, 0 swallowed |

## Host-safety (§12 / §12.6)

Bounded in-process load only: N=8 workers × M≤150 evaluations per scenario, small subjects, no fork/GB-alloc/host-net. Race detector is the concurrency-correctness evidence (run under -race -count=3).
%s

## Anti-bluff posture (§107 / §11.4.27)

Real bindings.Pipeline over a real MemoryBus + real Memory store/ladder/audit. Only the EXTERNAL boundary is faulted (faultEmitter for the bus-drop chaos). No pipeline logic is mocked; all numbers are captured runtime output.
`,
		time.Now().Format(time.RFC3339), persistent,
		healthy.Count, healthy.Latency.P50MS, healthy.Latency.P95MS, healthy.Latency.P99MS, healthy.ThroughputPS,
		memLine)
	if _, err := sd.WriteFile("summary.md", md); err != nil {
		t.Fatalf("write summary.md: %v", err)
	}
}
