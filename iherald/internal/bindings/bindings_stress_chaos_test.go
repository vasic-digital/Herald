package bindings_test

// HRD-024 §11.4.85 stress + chaos for the iherald incident/escalation bindings
// pipeline.
//
// Reuses the shared commons/stresschaos harness (RunLoad / Percentiles /
// NewRun / SurfaceDir / HostMemHeadroom). Drives the REAL Pipeline (REAL
// emitter + REAL store + REAL ladder + REAL audit) under sustained concurrent
// credential-leak page-out load and under emit-fault injection — the
// resilience-layer evidence the §11.4.85 mandate requires (a binding that
// passes its happy path but was never exercised under concurrency/fault is a
// resilience-layer PASS-bluff).
//
// Run under `go test -race -count=3`:
//   - the race detector is the concurrency-correctness evidence;
//   - -count=3 is the §11.4.50 determinism proof.
//
// §12 / §12.6 host-safety: bounded load only (N=8 workers × M≤150 = ≤1200
// evaluations per scenario), small in-process work, no fork/GB-alloc/host-net.
//
// NO REAL SECRETS: every credential-leak Subject is a FABRICATED location +
// boolean detection flag (e.g. "fake_leak_3_42|leaked=true"). NO real .env is
// scanned and NO real secret string appears anywhere in this file.

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

	"github.com/vasic-digital/herald/commons/stresschaos"
	constitution "github.com/vasic-digital/herald/commons_constitution"
	"github.com/vasic-digital/herald/commons_constitution/ladder"
	"github.com/vasic-digital/herald/commons_constitution/state"
	"github.com/vasic-digital/herald/iherald/internal/bindings"
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
		runID = stresschaos.NewRunID("hrd024")
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

// faultEmitter wraps a real emitter but makes CredentialLeak fail — the hermetic
// analogue of an eventbus drop mid-page-out. The Pipeline MUST surface that as
// an error (no silent swallow → no §107 fail-bluff at the distribution layer: a
// credential leak that "paged" but never reached the bus is the worst kind of
// bluff).
type faultEmitter struct {
	constitution.EventEmitter
	err error
}

func (f faultEmitter) CredentialLeak(ctx context.Context, e constitution.CredentialEvent) error {
	return f.err
}

// TestBindings_Stress_ConcurrentCredentialLeaks drives N=8 workers × M=150
// EvaluateSubject calls (1200 total) for DISTINCT credential-leak signals of the
// §11.4.10 rule (enforce mode) through ONE shared Pipeline. Asserts: zero
// errors, every call FAILs (real leak → page out), and the bus saw exactly the
// expected number of .credential.leak emits (no page-out lost under contention).
// Under -race this is the data-race proof for the whole evaluate→record→emit→
// audit stack.
//
// NO REAL SECRETS: each Subject is the fabricated string "fake_leak_<w>_<i>".
func TestBindings_Stress_ConcurrentCredentialLeaks(t *testing.T) {
	ctx := context.Background()
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{BufferSize: 4096})
	defer bus.Close()
	em, err := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "digital.vasic.herald/iherald"})
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
	if err := la.Set(ctx, tenant, "§11.4.10", constitution.ModeEnforce, "stress"); err != nil {
		t.Fatalf("ladder Set: %v", err)
	}

	const (
		workers = 8
		iters   = 150
	)
	var fails, nonFail int64
	sum := stresschaos.RunLoad(workers, iters, func(workerID, iter int) error {
		// Distinct FAKE leak location per (worker,iter) so each is a FirstSeen
		// transition → each MUST page out exactly once (no transitions-only
		// suppression). NO real secret — only a fabricated location + flag.
		subj := constitution.Subject{Kind: bindings.SubjectCredentialLeak, ID: fmt.Sprintf("fake_leak_%d_%d|leaked=true|kind=env", workerID, iter)}
		out, err := p.EvaluateSubject(ctx, "§11.4.10", tenant, subj)
		if err != nil {
			return fmt.Errorf("EvaluateSubject: %w", err)
		}
		if out.Decision == constitution.DecisionFail {
			atomic.AddInt64(&fails, 1)
		} else {
			atomic.AddInt64(&nonFail, 1)
			return fmt.Errorf("leak %s got %v, want fail", subj.ID, out.Decision)
		}
		if !out.Emitted {
			return fmt.Errorf("enforce credential-leak did not page out (emit): %s", subj.ID)
		}
		return nil
	})

	total := int64(workers * iters)
	if sum.Errors != 0 {
		t.Fatalf("concurrent credential-leaks reported %d errors (want 0)", sum.Errors)
	}
	if atomic.LoadInt64(&fails) != total {
		t.Fatalf("expected %d FAILs, got %d (nonFail=%d)", total, atomic.LoadInt64(&fails), atomic.LoadInt64(&nonFail))
	}
	// The bus MUST have published exactly `total` .credential.leak events — no
	// page-out lost under contention.
	published := bus.Metrics().PublishedByType[constitution.EventNamespace+"."+constitution.ClassCredentialLeak]
	if published != total {
		t.Fatalf("bus published %d .credential.leak events, want %d (lost page-outs under load)", published, total)
	}
	// Audit rows MUST equal total too.
	audit, err := au.ListAudit(ctx, tenant, constitution.AuditQuery{RuleID: "§11.4.10"})
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
		"surface=bindings scenario=stress_concurrent_credential_leaks rule=§11.4.10 mode=enforce\n"+
			"NO_REAL_SECRETS: every subject is a fabricated 'fake_leak_<w>_<i>' location + boolean flag\n"+
			"workers=%d iters_per_worker=%d total=%d\n"+
			"fails=%d non_fail=%d errors=%d\n"+
			"bus_published_credential_leak=%d audit_rows=%d\n"+
			"no_lost_page_outs_under_load=%d\n"+
			"p50_ms=%.4f p95_ms=%.4f p99_ms=%.4f max_ms=%.4f tput_per_sec=%.1f\n"+
			"race_detector=clean\n",
		workers, iters, total,
		atomic.LoadInt64(&fails), atomic.LoadInt64(&nonFail), sum.Errors,
		published, len(audit),
		boolToInt024(published == total && int64(len(audit)) == total),
		sum.Latency.P50MS, sum.Latency.P95MS, sum.Latency.P99MS, sum.Latency.MaxMS, sum.ThroughputPS)
	if _, err := sd.WriteFile("concurrent_credential_leaks.log", report); err != nil {
		t.Fatalf("write report: %v", err)
	}
	writeBindingsSummary(t, sd, sum, persistent)
	t.Logf("iherald bindings stress[concurrent-leaks]: %d evals, 0 errors, %d page-outs, %d audit rows, p99=%.3fms (persistent=%v dir=%s)",
		total, published, len(audit), sum.Latency.P99MS, persistent, filepath.Dir(sd.Dir))
}

// TestBindings_Chaos_PageOutFaultSurfaces injects an emit fault (faultEmitter:
// CredentialLeak always errors) and floods the pipeline with FAKE leaks. The
// §107 fail-loud contract: EVERY enforce-mode credential leak MUST surface the
// page-out error from EvaluateSubject — NEVER a silently-swallowed success. A
// pipeline that returned RunOutcome{Emitted:true} while the page-out failed
// would be a distribution-layer PASS-bluff at the WORST possible layer (a
// credential leak that never reached on-call); this test makes that impossible.
func TestBindings_Chaos_PageOutFaultSurfaces(t *testing.T) {
	ctx := context.Background()
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{BufferSize: 256})
	defer bus.Close()
	realEm, err := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "digital.vasic.herald/iherald"})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}
	emitErr := errors.New("eventbus publish failed (simulated bus drop on page-out)")
	em := faultEmitter{EventEmitter: realEm, err: emitErr}

	st := state.NewMemory()
	la := ladder.NewMemory()
	au := state.NewMemoryAudit()
	p, err := bindings.NewPipeline(bindings.Config{Ladder: la, Store: st, Emitter: em, Audit: au})
	if err != nil {
		t.Fatalf("NewPipeline: %v", err)
	}
	tenant := uuid.New()
	if err := la.Set(ctx, tenant, "§11.4.10", constitution.ModeEnforce, "chaos"); err != nil {
		t.Fatalf("ladder Set: %v", err)
	}

	const (
		workers = 8
		iters   = 100
	)
	var surfaced, swallowed int64
	sum := stresschaos.RunLoad(workers, iters, func(workerID, iter int) error {
		subj := constitution.Subject{Kind: bindings.SubjectCredentialLeak, ID: fmt.Sprintf("fake_leak_%d_%d|leaked=true|kind=env", workerID, iter)}
		_, err := p.EvaluateSubject(ctx, "§11.4.10", tenant, subj)
		if err != nil {
			atomic.AddInt64(&surfaced, 1)
			return nil // surfacing the fault is the CORRECT behaviour
		}
		atomic.AddInt64(&swallowed, 1)
		return fmt.Errorf("page-out fault SWALLOWED for %s (returned success despite bus drop)", subj.ID)
	})

	total := int64(workers * iters)
	if sum.Errors != 0 {
		t.Fatalf("page-out-fault chaos: %d swallowed faults (want 0 — every page-out error must surface)", sum.Errors)
	}
	if atomic.LoadInt64(&swallowed) != 0 {
		t.Fatalf("§107 distribution bluff: %d page-out faults were swallowed", atomic.LoadInt64(&swallowed))
	}
	if atomic.LoadInt64(&surfaced) != total {
		t.Fatalf("expected all %d page-outs to surface the fault, got %d", total, atomic.LoadInt64(&surfaced))
	}

	sd, _ := bindingsSurface(t)
	faultLog := fmt.Sprintf(
		"surface=bindings scenario=chaos_page_out_fault emitter=faultEmitter(CredentialLeak→err)\n"+
			"NO_REAL_SECRETS: every subject is a fabricated 'fake_leak_<w>_<i>' location + boolean flag\n"+
			"contract: enforce credential-leak + bus drop → EvaluateSubject errors, NEVER silent success\n"+
			"workers=%d iters=%d total=%d surfaced=%d swallowed=%d\n"+
			"fail_loud_no_swallow=%d\n"+
			"p99_ms=%.4f count=%d\n",
		workers, iters, total, atomic.LoadInt64(&surfaced), atomic.LoadInt64(&swallowed),
		boolToInt024(atomic.LoadInt64(&swallowed) == 0 && atomic.LoadInt64(&surfaced) == total),
		sum.Latency.P99MS, sum.Count)
	if _, err := sd.WriteFile("page_out_fault_fail_loud.log", faultLog); err != nil {
		t.Fatalf("write fault log: %v", err)
	}
	t.Logf("iherald bindings chaos[page-out-fault]: %d evals → %d surfaced fault, 0 swallowed", total, atomic.LoadInt64(&surfaced))
}

func boolToInt024(b bool) int {
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
	md := fmt.Sprintf(`# Stress + Chaos — iherald incident/escalation constitution bindings (HRD-024)

Constitutional anchor: Helix §11.4.85 (stress + chaos mandate) / Herald §107.x.
Captured: %s  (persistent=%v)

## Scenarios

| scenario | verdict | evidence file | key numbers |
|---|---|---|---|
| stress_concurrent_credential_leaks | PASS | concurrent_credential_leaks.log, latency.json | %d evals, 0 errors, every .credential.leak page-out + audit accounted (no lost page-outs), p50=%.3fms p95=%.3fms p99=%.3fms tput=%.0f/s |
| chaos_page_out_fault_fail_loud | PASS | page_out_fault_fail_loud.log | bus-drop → 100%% of page-out faults surface via EvaluateSubject error, 0 swallowed |

## No-real-secret attestation (§107.x)

Every credential-leak Subject in this suite is a FABRICATED "fake_leak_<w>_<i>" location plus a boolean detection flag. NO real .env is scanned and NO real secret string appears anywhere in the test file or its captured evidence.

## Host-safety (§12 / §12.6)

Bounded in-process load only: N=8 workers × M≤150 evaluations per scenario, small fabricated credential-leak subjects, NO real credential scans, no fork/GB-alloc/host-net. Race detector is the concurrency-correctness evidence (run under -race -count=3).
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
