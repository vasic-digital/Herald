package constitution_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	constitution "github.com/vasic-digital/herald/commons_constitution"
	"github.com/vasic-digital/herald/commons_constitution/ladder"
	"github.com/vasic-digital/herald/commons_constitution/state"
)

// evalForTest is the test scaffolding for M1 smoke: a deterministic evaluator
// whose Result + transition are caller-controlled per-call.
type evalForTest struct {
	id       string
	sev      constitution.Severity
	result   constitution.Result
	panicNow bool
	errNow   bool
}

func (e *evalForTest) RuleID() string                                                         { return e.id }
func (e *evalForTest) Severity() constitution.Severity                                        { return e.sev }
func (e *evalForTest) PushTriggers() []string                                                 { return nil }
func (e *evalForTest) Subjects(_ context.Context, _ uuid.UUID) ([]constitution.Subject, error) {
	return []constitution.Subject{{Kind: "file", ID: "/test"}}, nil
}
func (e *evalForTest) Evaluate(_ context.Context, _ constitution.Subject, _ constitution.BundleHash) (constitution.Result, error) {
	if e.panicNow {
		panic("evalForTest: forced panic")
	}
	if e.errNow {
		return constitution.Result{}, errors.New("evalForTest: forced error")
	}
	return e.result, nil
}

// makeResult is a small helper to build a Result with a stable digest.
func makeResult(d constitution.Decision, evidence string) constitution.Result {
	return constitution.Result{
		Decision:  d,
		Evidence:  evidence,
		DigestSHA: sha256.Sum256([]byte(d.String() + ":" + evidence)),
	}
}

// TestM1Smoke_EndToEnd is the M1 milestone-closure proof.
//
// Per Foundation design §1: "M1 smoke = go test ./commons_constitution/...
// proves an in-memory evaluator detects a transition → emits a .policy-violation
// → memory-pubsub listener counts it."
//
// This test exercises THE WHOLE M1 FLOW in-process — every code path from
// Runner.Run down to the EventBus listener — including the §3.1 step [7]
// transition gate and the mode-ladder enforcement-mode dispatch.
//
// Anti-bluff: every assertion checks an observable side-effect of REAL code
// running. No mocks-of-mocks. The Bus is real; the Store is real; the Ladder
// is real; the Emitter is real; the Runner is real.
func TestM1Smoke_EndToEnd(t *testing.T) {
	ctx := context.Background()

	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{BufferSize: 64})
	defer bus.Close()
	em, err := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "test"})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}
	reg := constitution.NewRegistry()
	st := state.NewMemory()
	la := ladder.NewMemory()
	runner, err := constitution.NewRunner(reg, la, st, em)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	// Listener tallying policy-violation receipt — this is the anti-bluff
	// observation that proves the emit actually reached the bus.
	violationCh, err := bus.Subscribe(constitution.EventNamespace + ".policy.violation")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer violationCh.Cancel()

	var violationCount int64
	go func() {
		for range violationCh.Channel {
			atomic.AddInt64(&violationCount, 1)
		}
	}()

	tenant := uuid.New()
	bundle := constitution.CaptureBytes([]byte("rev-1"))
	subject := constitution.Subject{Kind: "file", ID: "/etc/secrets"}

	// 1) First run: Pass — no transition emit because first sight + decision pass.
	//    Wait — first sight IS a transition (FirstSeen=true). Per §3.1 step [7],
	//    a transition with Pass+ModeEnforce results in a PolicyCleared emit, not
	//    a PolicyViolation. So we expect 0 violations after this run.
	pass := &evalForTest{id: "§11.4.10", sev: constitution.SeverityCritical, result: makeResult(constitution.DecisionPass, "ok")}
	reg.Register(pass)
	out, err := runner.Run(ctx, pass, tenant, subject, bundle)
	if err != nil {
		t.Fatalf("Run pass: %v", err)
	}
	if !out.Transition.FirstSeen || !out.Transition.Changed {
		t.Errorf("first run should mark FirstSeen+Changed, got %+v", out.Transition)
	}
	if out.Mode != constitution.ModeEnforce {
		t.Errorf("default mode = %v; want ModeEnforce", out.Mode)
	}
	if !out.Emitted {
		t.Errorf("first run with default-enforce should emit (PolicyCleared)")
	}

	// 2) Swap the evaluator's verdict to Fail + re-run. Expect:
	//    - Transition.Changed=true (pass→fail)
	//    - Mode=Enforce (no Set yet)
	//    - Emitted=true (PolicyViolation)
	pass.result = makeResult(constitution.DecisionFail, "regression")
	out, err = runner.Run(ctx, pass, tenant, subject, bundle)
	if err != nil {
		t.Fatalf("Run fail: %v", err)
	}
	if !out.Transition.Changed || out.Transition.OldDecision != constitution.DecisionPass {
		t.Errorf("expected pass→fail transition; got %+v", out.Transition)
	}
	if !out.Emitted {
		t.Errorf("ModeEnforce + transition must emit; out=%+v", out)
	}

	// 3) Wait for the listener to actually observe the violation event.
	deadline := time.After(2 * time.Second)
	for atomic.LoadInt64(&violationCount) < 1 {
		select {
		case <-deadline:
			t.Fatalf("listener did not receive .policy.violation event; bus metrics=%+v", bus.Metrics())
		case <-time.After(10 * time.Millisecond):
		}
	}

	// 4) Repeat the same Fail result without changes — Transition.Changed=false,
	//    Emitted=false (transitions-only discipline).
	emitsBefore := bus.Metrics().PublishedByType[constitution.EventNamespace+".policy.violation"]
	out, err = runner.Run(ctx, pass, tenant, subject, bundle)
	if err != nil {
		t.Fatalf("Run no-change: %v", err)
	}
	if out.Transition.Changed {
		t.Errorf("identical re-run should report Changed=false; got %+v", out.Transition)
	}
	if out.Emitted {
		t.Errorf("no-change run must not emit; out=%+v", out)
	}
	emitsAfter := bus.Metrics().PublishedByType[constitution.EventNamespace+".policy.violation"]
	if emitsAfter != emitsBefore {
		t.Errorf("PublishedByType[.policy.violation] grew on no-change run: %d→%d", emitsBefore, emitsAfter)
	}

	// 5) Flip mode to Warn and re-evaluate at a NEW evidence (different digest).
	//    Expect: Transition.Changed=true, Audited=true, Emitted=false.
	if err := la.Set(ctx, tenant, "§11.4.10", constitution.ModeWarn, "test"); err != nil {
		t.Fatalf("ladder Set warn: %v", err)
	}
	pass.result = makeResult(constitution.DecisionFail, "different-evidence")
	out, err = runner.Run(ctx, pass, tenant, subject, bundle)
	if err != nil {
		t.Fatalf("Run warn: %v", err)
	}
	if !out.Transition.Changed {
		t.Errorf("digest change should mark Changed=true even at same decision")
	}
	if !out.Audited {
		t.Errorf("ModeWarn should audit")
	}
	if out.Emitted {
		t.Errorf("ModeWarn must NOT emit on channels")
	}

	// 6) Flip mode to Allow. Expect: no audit, no emit, but state still recorded.
	if err := la.Set(ctx, tenant, "§11.4.10", constitution.ModeAllow, "test"); err != nil {
		t.Fatalf("ladder Set allow: %v", err)
	}
	pass.result = makeResult(constitution.DecisionPass, "recovered")
	out, err = runner.Run(ctx, pass, tenant, subject, bundle)
	if err != nil {
		t.Fatalf("Run allow: %v", err)
	}
	if !out.Transition.Changed {
		t.Errorf("fail→pass should be a transition")
	}
	if out.Audited || out.Emitted {
		t.Errorf("ModeAllow must neither audit nor emit; got out=%+v", out)
	}
	// State recorded?
	row, ok, err := st.Get(ctx, tenant, "§11.4.10", subject.ID)
	if err != nil || !ok {
		t.Fatalf("Store.Get after allow-run: ok=%v err=%v", ok, err)
	}
	if row.Decision != constitution.DecisionPass {
		t.Errorf("persisted Decision = %v; want pass (ModeAllow still records state)", row.Decision)
	}
}

func TestM1Smoke_PanicIsolation(t *testing.T) {
	// Anti-bluff for §4 row 1: a panicking evaluator MUST NOT leak the panic
	// through Runner.Run AND MUST result in a DecisionError state row.
	ctx := context.Background()
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{})
	defer bus.Close()
	em, _ := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "t"})
	reg := constitution.NewRegistry()
	st := state.NewMemory()
	la := ladder.NewMemory()
	runner, _ := constitution.NewRunner(reg, la, st, em)

	panicky := &evalForTest{id: "§panicky", panicNow: true}
	reg.Register(panicky)
	out, err := runner.Run(ctx, panicky, uuid.New(), constitution.Subject{Kind: "x", ID: "y"}, constitution.BundleHash{})
	if err != nil {
		t.Fatalf("Run with panic returned error: %v (should be recovered)", err)
	}
	if out.PanicValue == "" {
		t.Error("PanicValue empty after panicking evaluator; recovery not surfaced")
	}
	if out.Decision != constitution.DecisionError {
		t.Errorf("Decision = %v; want DecisionError", out.Decision)
	}
}

func TestM1Smoke_ErrorReturnedByEvaluator(t *testing.T) {
	// Distinct from panic: evaluator returns an error, not a panic. Should
	// still surface as DecisionError but PanicValue stays empty.
	ctx := context.Background()
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{})
	defer bus.Close()
	em, _ := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "t"})
	reg := constitution.NewRegistry()
	st := state.NewMemory()
	la := ladder.NewMemory()
	runner, _ := constitution.NewRunner(reg, la, st, em)

	erry := &evalForTest{id: "§erry", errNow: true}
	reg.Register(erry)
	out, err := runner.Run(ctx, erry, uuid.New(), constitution.Subject{Kind: "x", ID: "y"}, constitution.BundleHash{})
	if err != nil {
		t.Fatalf("Run with err returned outer error: %v", err)
	}
	if out.Decision != constitution.DecisionError {
		t.Errorf("evaluator returned error → Decision = %v; want DecisionError", out.Decision)
	}
	if out.PanicValue != "" {
		t.Errorf("evaluator error (not panic) should leave PanicValue empty; got %q", out.PanicValue)
	}
}
