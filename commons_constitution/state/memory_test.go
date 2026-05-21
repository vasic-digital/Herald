package state

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vasic-digital/herald/commons_constitution"
)

func mkResult(d constitution.Decision, evidence string) constitution.Result {
	return constitution.Result{
		Decision:  d,
		Evidence:  evidence,
		DigestSHA: sha256.Sum256([]byte(d.String() + ":" + evidence)),
	}
}

func TestMemory_FirstSightIsTransition(t *testing.T) {
	m := NewMemory()
	tenant := uuid.New()
	bundle := constitution.CaptureBytes([]byte("v1"))

	trans, err := m.Record(context.Background(), tenant, "§A", "subj-1",
		mkResult(constitution.DecisionFail, "missing"), bundle, "evidence://x")
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if !trans.FirstSeen {
		t.Error("first Record must set FirstSeen=true")
	}
	if !trans.Changed {
		t.Error("first Record must set Changed=true")
	}
	if trans.NewDecision != constitution.DecisionFail {
		t.Errorf("NewDecision = %v; want fail", trans.NewDecision)
	}
}

func TestMemory_NoChangeMeansNoTransition(t *testing.T) {
	m := NewMemory()
	tenant := uuid.New()
	bundle := constitution.CaptureBytes([]byte("v1"))
	ctx := context.Background()
	r := mkResult(constitution.DecisionPass, "ok")

	_, err := m.Record(ctx, tenant, "§A", "subj-1", r, bundle, "evidence://x")
	if err != nil {
		t.Fatalf("Record 1: %v", err)
	}
	trans, err := m.Record(ctx, tenant, "§A", "subj-1", r, bundle, "evidence://x")
	if err != nil {
		t.Fatalf("Record 2: %v", err)
	}
	if trans.Changed {
		t.Error("identical Record should report Changed=false (transitions-only discipline)")
	}
	if trans.FirstSeen {
		t.Error("second Record should report FirstSeen=false")
	}
}

func TestMemory_DecisionChangeIsTransition(t *testing.T) {
	m := NewMemory()
	tenant := uuid.New()
	bundle := constitution.CaptureBytes([]byte("v1"))
	ctx := context.Background()

	_, _ = m.Record(ctx, tenant, "§A", "subj-1",
		mkResult(constitution.DecisionPass, "ok"), bundle, "")
	trans, _ := m.Record(ctx, tenant, "§A", "subj-1",
		mkResult(constitution.DecisionFail, "regress"), bundle, "")

	if !trans.Changed {
		t.Error("Decision pass→fail should report Changed=true")
	}
	if trans.OldDecision != constitution.DecisionPass || trans.NewDecision != constitution.DecisionFail {
		t.Errorf("transition direction wrong: %v→%v", trans.OldDecision, trans.NewDecision)
	}
}

func TestMemory_DigestChangeAtSameDecisionIsTransition(t *testing.T) {
	m := NewMemory()
	tenant := uuid.New()
	bundle := constitution.CaptureBytes([]byte("v1"))
	ctx := context.Background()

	_, _ = m.Record(ctx, tenant, "§A", "subj-1",
		mkResult(constitution.DecisionFail, "missing attestation"), bundle, "")
	trans, _ := m.Record(ctx, tenant, "§A", "subj-1",
		mkResult(constitution.DecisionFail, "wrong-issuer"), bundle, "") // same decision, different evidence

	if !trans.Changed {
		t.Error("Decision unchanged but evidence-digest changed → Changed must be true (rationale changed)")
	}
}

func TestMemory_BundleChangeAtSameDecisionAndDigestIsTransition(t *testing.T) {
	// Anti-bluff: must construct a Result whose DigestSHA equals an earlier
	// Result's DigestSHA. Use identical inputs to mkResult.
	m := NewMemory()
	tenant := uuid.New()
	ctx := context.Background()
	res := mkResult(constitution.DecisionPass, "ok")

	_, _ = m.Record(ctx, tenant, "§A", "subj-1", res, constitution.CaptureBytes([]byte("v1")), "")
	trans, _ := m.Record(ctx, tenant, "§A", "subj-1", res, constitution.CaptureBytes([]byte("v2")), "")

	if !trans.Changed {
		t.Error("Bundle rev changed at same Decision+Digest → Changed must be true (.bundle-updated discipline)")
	}
	if trans.NewBundleHash == trans.OldBundleHash {
		t.Errorf("bundle hashes should differ between v1/v2: old=%s new=%s", trans.OldBundleHash, trans.NewBundleHash)
	}
}

func TestMemory_TenantIsolation(t *testing.T) {
	m := NewMemory()
	a, b := uuid.New(), uuid.New()
	bundle := constitution.CaptureBytes([]byte("v1"))
	ctx := context.Background()

	_, _ = m.Record(ctx, a, "§A", "subj-1", mkResult(constitution.DecisionFail, "x"), bundle, "")

	rowB, ok, err := m.Get(ctx, b, "§A", "subj-1")
	if err != nil {
		t.Fatalf("Get B: %v", err)
	}
	if ok {
		t.Errorf("tenant B saw tenant A's row: %+v", rowB)
	}
}

func TestMemory_List_FilterByDecision(t *testing.T) {
	m := NewMemory()
	tenant := uuid.New()
	bundle := constitution.CaptureBytes([]byte("v1"))
	ctx := context.Background()

	_, _ = m.Record(ctx, tenant, "§A", "subj-1", mkResult(constitution.DecisionPass, "ok"), bundle, "")
	_, _ = m.Record(ctx, tenant, "§A", "subj-2", mkResult(constitution.DecisionFail, "boom"), bundle, "")
	_, _ = m.Record(ctx, tenant, "§B", "subj-3", mkResult(constitution.DecisionFail, "boom"), bundle, "")

	fail := constitution.DecisionFail
	rows, err := m.List(ctx, tenant, constitution.ListQuery{Decision: &fail})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("List(decision=fail) returned %d; want 2", len(rows))
	}
	for _, r := range rows {
		if r.Decision != constitution.DecisionFail {
			t.Errorf("List returned wrong-decision row: %+v", r)
		}
	}
}

func TestMemory_List_HonorsSinceUntilOffset(t *testing.T) {
	// Seed 5 rows at known, monotonically increasing times via the
	// injectable clock. Since each row has a unique (rule, subject) key
	// the rows are first-sights → TransitionedAt is the clock value at
	// Record time, giving us deterministic test data.
	tid := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ctx := context.Background()
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)

	// Build a clock that advances by one hour on each Record call so the
	// 5 seeded rows land at 12:00, 13:00, 14:00, 15:00, 16:00 UTC
	// regardless of the order we walk the rules slice.
	rules := []string{"11.4.1", "11.4.2", "11.4.3", "11.4.4", "11.4.5"}
	clockI := 0
	store := NewMemory().WithClock(func() time.Time {
		t := now.Add(time.Duration(clockI) * time.Hour)
		clockI++
		return t
	})

	bundle := constitution.CaptureBytes([]byte("v1"))
	for _, rule := range rules {
		_, err := store.Record(ctx, tid, rule, "subj-"+rule,
			mkResult(constitution.DecisionWarn, "ok-"+rule), bundle, "")
		if err != nil {
			t.Fatalf("Record(%s): %v", rule, err)
		}
	}

	// Since 13:00 → expect rules 11.4.2..11.4.5 (4 rows).
	since := now.Add(1 * time.Hour)
	rows, err := store.List(ctx, tid, constitution.ListQuery{Since: since})
	if err != nil {
		t.Fatalf("List(Since): %v", err)
	}
	if len(rows) != 4 {
		t.Errorf("Since filter: got %d rows, want 4", len(rows))
	}

	// Until 15:00 (inclusive) → expect rules 11.4.1..11.4.4 (4 rows).
	until := now.Add(3 * time.Hour)
	rows, err = store.List(ctx, tid, constitution.ListQuery{Until: until})
	if err != nil {
		t.Fatalf("List(Until): %v", err)
	}
	if len(rows) != 4 {
		t.Errorf("Until filter: got %d rows, want 4 (inclusive)", len(rows))
	}

	// Offset 2, Limit 2 → expect rows 3 + 4 in deterministic ASC order by
	// TransitionedAt (i.e., the rules seeded at 14:00 + 15:00).
	rows, err = store.List(ctx, tid, constitution.ListQuery{Offset: 2, Limit: 2})
	if err != nil {
		t.Fatalf("List(Offset+Limit): %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("Offset+Limit: got %d rows, want 2", len(rows))
	}
	if len(rows) == 2 {
		want0 := now.Add(2 * time.Hour)
		want1 := now.Add(3 * time.Hour)
		if !rows[0].TransitionedAt.Equal(want0) {
			t.Errorf("Offset+Limit row[0].TransitionedAt = %s; want %s",
				rows[0].TransitionedAt, want0)
		}
		if !rows[1].TransitionedAt.Equal(want1) {
			t.Errorf("Offset+Limit row[1].TransitionedAt = %s; want %s",
				rows[1].TransitionedAt, want1)
		}
	}
}

func TestMemory_NoChangePreservesTransitionedAt(t *testing.T) {
	frozen := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	m := NewMemory().WithClock(func() time.Time { return frozen })
	tenant := uuid.New()
	bundle := constitution.CaptureBytes([]byte("v1"))
	ctx := context.Background()
	r := mkResult(constitution.DecisionPass, "ok")

	_, _ = m.Record(ctx, tenant, "§A", "subj-1", r, bundle, "")

	// Advance clock; re-record same result; the persisted TransitionedAt
	// should NOT advance — the verdict-occurrence-timestamp is preserved.
	advanced := frozen.Add(1 * time.Hour)
	m.WithClock(func() time.Time { return advanced })
	_, _ = m.Record(ctx, tenant, "§A", "subj-1", r, bundle, "")

	row, _, _ := m.Get(ctx, tenant, "§A", "subj-1")
	if !row.TransitionedAt.Equal(frozen) {
		t.Errorf("TransitionedAt advanced on no-change re-record: got %s; want %s", row.TransitionedAt, frozen)
	}
}
