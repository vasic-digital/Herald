package constitution

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
)

// stubEvaluator is a minimal Evaluator used by registry tests.
type stubEvaluator struct {
	ruleID   string
	severity Severity
	triggers []string
	subjects []Subject
	result   Result
	panicNow bool
	calls    int
	mu       sync.Mutex
}

func (s *stubEvaluator) RuleID() string         { return s.ruleID }
func (s *stubEvaluator) Severity() Severity     { return s.severity }
func (s *stubEvaluator) PushTriggers() []string { return s.triggers }
func (s *stubEvaluator) Subjects(_ context.Context, _ uuid.UUID) ([]Subject, error) {
	return s.subjects, nil
}
func (s *stubEvaluator) Evaluate(_ context.Context, _ Subject, _ BundleHash) (Result, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	if s.panicNow {
		panic("stubEvaluator: forced panic for test")
	}
	return s.result, nil
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	e := &stubEvaluator{ruleID: "§11.4.10", severity: SeverityCritical}
	r.Register(e)

	got, ok := r.Get("§11.4.10")
	if !ok || got != e {
		t.Fatalf("Get(§11.4.10) = (%v, %v); want (%v, true)", got, ok, e)
	}

	if r.Len() != 1 {
		t.Errorf("Len() = %d; want 1", r.Len())
	}

	_, ok = r.Get("§11.4.99")
	if ok {
		t.Errorf("Get(missing) returned ok=true")
	}
}

func TestRegistry_DuplicateRegisterPanics(t *testing.T) {
	r := NewRegistry()
	a := &stubEvaluator{ruleID: "§X"}
	b := &stubEvaluator{ruleID: "§X"}
	r.Register(a)

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on duplicate Register, got none")
		}
	}()
	r.Register(b)
}

func TestRegistry_RegisterSameInstanceIsIdempotent(t *testing.T) {
	r := NewRegistry()
	e := &stubEvaluator{ruleID: "§X"}
	r.Register(e)
	r.Register(e) // same instance — must not panic
	if r.Len() != 1 {
		t.Errorf("Len() = %d; want 1 after idempotent re-register", r.Len())
	}
}

func TestRegistry_NilOrEmptyRulePanics(t *testing.T) {
	r := NewRegistry()

	mustPanic := func(fn func()) {
		defer func() {
			if recover() == nil {
				t.Errorf("expected panic, got none")
			}
		}()
		fn()
	}
	mustPanic(func() { r.Register(nil) })
	mustPanic(func() { r.Register(&stubEvaluator{ruleID: ""}) })
}

func TestRegistry_IterByMaxSeverity(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubEvaluator{ruleID: "§A", severity: SeverityLow})
	r.Register(&stubEvaluator{ruleID: "§B", severity: SeverityMiddle})
	r.Register(&stubEvaluator{ruleID: "§C", severity: SeverityHigh})
	r.Register(&stubEvaluator{ruleID: "§D", severity: SeverityCritical})

	mid := r.IterByMaxSeverity(SeverityMiddle)
	if len(mid) != 2 {
		t.Fatalf("IterByMaxSeverity(middle) returned %d evaluators; want 2", len(mid))
	}
	// Must be sorted by RuleID for deterministic iteration.
	if mid[0].RuleID() != "§A" || mid[1].RuleID() != "§B" {
		t.Errorf("expected sorted [§A §B]; got [%s %s]", mid[0].RuleID(), mid[1].RuleID())
	}

	all := r.IterByMaxSeverity(SeverityCritical)
	if len(all) != 4 {
		t.Errorf("IterByMaxSeverity(critical) returned %d; want 4", len(all))
	}
}

func TestRegistry_ConcurrentSafe(t *testing.T) {
	// Anti-bluff: actually exercise the RWMutex by hammering reads + writes
	// from many goroutines. -race would fire if our locking was wrong.
	r := NewRegistry()
	var wg sync.WaitGroup
	const writers, readers = 8, 32
	for i := 0; i < writers; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Register(&stubEvaluator{ruleID: ruleNum(i)})
		}()
	}
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.Len()
			_ = r.IterByMaxSeverity(SeverityCritical)
			_, _ = r.Get(ruleNum(0))
		}()
	}
	wg.Wait()
	if r.Len() != writers {
		t.Errorf("after %d concurrent Registers, Len() = %d; want %d", writers, r.Len(), writers)
	}
}

func ruleNum(i int) string {
	const digits = "0123456789"
	return "§R-" + string(digits[i%10])
}

func TestSeverity_String(t *testing.T) {
	cases := map[Severity]string{
		SeverityLow: "low", SeverityMiddle: "middle",
		SeverityHigh: "high", SeverityCritical: "critical",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("Severity(%d).String() = %q; want %q", int(k), got, want)
		}
	}
}

func TestDecision_String(t *testing.T) {
	cases := map[Decision]string{
		DecisionPass: "pass", DecisionWarn: "warn", DecisionFail: "fail",
		DecisionError: "error", DecisionSkip: "skip",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("Decision(%d).String() = %q; want %q", int(k), got, want)
		}
	}
}

func TestAllClassesCardinality(t *testing.T) {
	got := AllClasses()
	if len(got) != 13 {
		t.Errorf("AllClasses() returned %d entries; want 13 (12 governance per spec §42.2 + 1 operational queue.dead_letter per HRD-090)", len(got))
	}
	seen := make(map[string]bool, len(got))
	for _, c := range got {
		if seen[c] {
			t.Errorf("duplicate class %q in AllClasses()", c)
		}
		seen[c] = true
	}
}
