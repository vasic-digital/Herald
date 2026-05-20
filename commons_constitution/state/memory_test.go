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
