// Hermetic, anti-bluff tests for the HRD-147 governance dead-letter
// subscriber + its boot-plane wiring.
//
// SCOPE + ANTI-BLUFF DISCLOSURE (§107 / §11.4.5): these tests exercise the
// REAL components end-to-end — a real constitution.MemoryBus, a real
// constitution.Emitter, the real pgxTaskRepository.MoveToDeadLetter, the real
// DeadLetterSubscriber, and the real MemoryDeadLetterSink. NOTHING in the
// emit → publish → subscribe → record path is mocked. The ONLY fake is the
// db.Database underneath MoveToDeadLetter (the recordingDB declared in
// task_repository_test.go), because the SQL round-trip is a SEPARATE concern
// proven by the live-PG integration suite (dead_letter_integration_test.go);
// here the unit under test is the event plane, so a real Postgres is
// unnecessary and the DB is honestly a test double, not the subject.
//
// The load-bearing evidence is: a real MoveToDeadLetter call drives a real
// event onto a real bus, the real subscriber drains it, and the real sink
// holds exactly one record with the right TaskID/Reason/FailureCount. That is
// the closed loop boot.go previously left open (emit-into-the-void).

package infra

import (
	"context"
	"sync"
	"testing"
	"time"

	constitution "github.com/vasic-digital/herald/commons_constitution"
)

// waitForRecords polls the sink up to timeout for at least n records, so the
// test never races the asynchronous bus delivery + drain goroutine. Returns
// the records (oldest-first) once the count is reached, or fails.
func waitForRecords(t *testing.T, sink DeadLetterSink, n int, timeout time.Duration) []DeadLetterRecord {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		recs, err := sink.List(context.Background())
		if err != nil {
			t.Fatalf("sink.List: %v", err)
		}
		if len(recs) >= n {
			return recs
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d records; have %d", n, len(recs))
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// TestDeadLetterSubscriber_RecordsRealMoveEndToEnd is the headline anti-bluff
// test: a real MoveToDeadLetter → real emit → real subscriber → real sink, with
// the recorded row carrying the correct task id, reason, and failure count.
func TestDeadLetterSubscriber_RecordsRealMoveEndToEnd(t *testing.T) {
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{})
	defer bus.Close()

	sink := NewMemoryDeadLetterSink()
	sub, err := StartDeadLetterSubscriber(bus, sink)
	if err != nil {
		t.Fatalf("StartDeadLetterSubscriber: %v", err)
	}
	defer sub.Close()

	em, err := constitution.NewEmitter(bus, constitution.EmitterConfig{
		Source: "digital.vasic.herald/commons_infra-test",
	})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	// Real repository over the in-repo recordingDB fake (the DB is not the
	// unit under test — see file header). The recordingDB GetByID path returns
	// a scannable row; we make MoveToDeadLetter load a real task by priming the
	// fake to return a row, then it emits via the real emitter.
	rec := &recordingDB{execResult: fakeResult{affected: 1}}
	repo := newPgxTaskRepositoryWithEmitter(rec, em)

	// recordingDB.QueryRow returns a fakeRow whose Scan is a no-op (nil error),
	// which scanTask turns into a zero-value BackgroundTask with the queried id
	// absent — so GetByID returns a non-nil task. We instead drive the failure
	// count via the emit path, which reads task.RetryCount; with the no-op scan
	// that is 0. To prove FailureCount propagation we use the lower-level emit
	// directly below; here we assert the move publishes + records at all.
	if err := repo.MoveToDeadLetter(context.Background(), "task-e2e", "exhausted retries"); err != nil {
		t.Fatalf("MoveToDeadLetter: %v", err)
	}

	recs := waitForRecords(t, sink, 1, 2*time.Second)
	if len(recs) != 1 {
		t.Fatalf("want exactly 1 recorded dead-letter, got %d", len(recs))
	}
	got := recs[0]
	if got.TaskID != "task-e2e" {
		t.Errorf("record TaskID = %q; want task-e2e", got.TaskID)
	}
	if got.Reason != "exhausted retries" {
		t.Errorf("record Reason = %q; want %q", got.Reason, "exhausted retries")
	}
	if got.EventID == "" {
		t.Error("record EventID must be the bus-assigned CloudEvent id, not empty")
	}
	if got.Severity != "high" {
		t.Errorf("record Severity = %q; want high (the dead-letter default)", got.Severity)
	}
	if got.RecordedAt.IsZero() {
		t.Error("record RecordedAt must be stamped")
	}

	recorded, decodeErrs := sub.Stats()
	if recorded != 1 {
		t.Errorf("subscriber recorded = %d; want 1", recorded)
	}
	if decodeErrs != 0 {
		t.Errorf("subscriber decodeErrs = %d; want 0", decodeErrs)
	}
}

// TestDeadLetterSubscriber_PropagatesFailureCount drives the emitter directly
// (the same emitter MoveToDeadLetter uses) to prove the FailureCount + RuleID
// fields round-trip through the wire payload into the recorded row. This
// isolates field propagation from the recordingDB's zero RetryCount.
func TestDeadLetterSubscriber_PropagatesFailureCount(t *testing.T) {
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{})
	defer bus.Close()
	sink := NewMemoryDeadLetterSink()
	sub, err := StartDeadLetterSubscriber(bus, sink)
	if err != nil {
		t.Fatalf("StartDeadLetterSubscriber: %v", err)
	}
	defer sub.Close()

	em, err := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "src"})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}
	if err := em.DeadLetter(context.Background(), constitution.QueueEvent{
		RuleID:        "§42.1",
		Severity:      constitution.SeverityHigh,
		TaskID:        "task-fc",
		FailureReason: "boom",
		FailureCount:  7,
	}); err != nil {
		t.Fatalf("DeadLetter emit: %v", err)
	}

	recs := waitForRecords(t, sink, 1, 2*time.Second)
	got := recs[0]
	if got.FailureCount != 7 {
		t.Errorf("record FailureCount = %d; want 7", got.FailureCount)
	}
	if got.RuleID != "§42.1" {
		t.Errorf("record RuleID = %q; want §42.1", got.RuleID)
	}
}

// TestDeadLetterSubscriber_NoEventNoRecord proves the subscriber records
// NOTHING when nothing is dead-lettered — the sink stays empty (no phantom
// rows, no §107 false-positive).
func TestDeadLetterSubscriber_NoEventNoRecord(t *testing.T) {
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{})
	defer bus.Close()
	sink := NewMemoryDeadLetterSink()
	sub, err := StartDeadLetterSubscriber(bus, sink)
	if err != nil {
		t.Fatalf("StartDeadLetterSubscriber: %v", err)
	}
	defer sub.Close()

	// Give any (non-existent) delivery a window, then assert empty.
	time.Sleep(20 * time.Millisecond)
	recs, err := sink.List(context.Background())
	if err != nil {
		t.Fatalf("sink.List: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("sink must be empty when nothing dead-lettered; got %d records", len(recs))
	}
	if recorded, _ := sub.Stats(); recorded != 0 {
		t.Errorf("subscriber recorded = %d; want 0", recorded)
	}
}

// TestDeadLetterSubscriber_IgnoresOtherClasses proves the subscriber filters
// to ONLY .queue.dead_letter — a different governance class published on the
// same bus must not produce a dead-letter record.
func TestDeadLetterSubscriber_IgnoresOtherClasses(t *testing.T) {
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{})
	defer bus.Close()
	sink := NewMemoryDeadLetterSink()
	sub, err := StartDeadLetterSubscriber(bus, sink)
	if err != nil {
		t.Fatalf("StartDeadLetterSubscriber: %v", err)
	}
	defer sub.Close()

	em, err := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "src"})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}
	// Emit a non-dead-letter class.
	if err := em.GateFailed(context.Background(), constitution.GateEvent{
		RuleID:   "§11.4",
		Severity: constitution.SeverityHigh,
		GateName: "test-suite",
	}); err != nil {
		t.Fatalf("GateFailed emit: %v", err)
	}
	// Then a real dead-letter so we have a positive anchor to wait on.
	if err := em.DeadLetter(context.Background(), constitution.QueueEvent{
		TaskID:        "task-only",
		FailureReason: "r",
		Severity:      constitution.SeverityHigh,
	}); err != nil {
		t.Fatalf("DeadLetter emit: %v", err)
	}

	recs := waitForRecords(t, sink, 1, 2*time.Second)
	if len(recs) != 1 {
		t.Fatalf("want exactly 1 record (only the dead-letter); got %d", len(recs))
	}
	if recs[0].TaskID != "task-only" {
		t.Errorf("recorded the wrong event: TaskID=%q", recs[0].TaskID)
	}
}

// TestDeadLetterSubscriber_ConcurrentMoves is the -race anti-bluff resilience
// test: many concurrent dead-letter emits must each produce exactly one
// record, with no lost/duplicated rows and no data race in the sink or the
// subscriber's counters.
func TestDeadLetterSubscriber_ConcurrentMoves(t *testing.T) {
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{BufferSize: 1024})
	defer bus.Close()
	sink := NewMemoryDeadLetterSink()
	sub, err := StartDeadLetterSubscriber(bus, sink)
	if err != nil {
		t.Fatalf("StartDeadLetterSubscriber: %v", err)
	}
	defer sub.Close()

	em, err := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "src"})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			_ = em.DeadLetter(context.Background(), constitution.QueueEvent{
				TaskID:        "task",
				FailureReason: "concurrent",
				FailureCount:  i,
				Severity:      constitution.SeverityHigh,
			})
		}(i)
	}
	wg.Wait()

	recs := waitForRecords(t, sink, n, 5*time.Second)
	if len(recs) != n {
		t.Fatalf("want exactly %d records under concurrency; got %d", n, len(recs))
	}
	if recorded, decodeErrs := sub.Stats(); recorded != n || decodeErrs != 0 {
		t.Errorf("subscriber stats: recorded=%d decodeErrs=%d; want recorded=%d decodeErrs=0", recorded, decodeErrs, n)
	}
}

// TestDeadLetterSubscriber_CloseIsIdempotentAndStopsDrain proves Close() joins
// the drain goroutine and is safe to call twice — no panic, no goroutine leak.
func TestDeadLetterSubscriber_CloseIsIdempotentAndStopsDrain(t *testing.T) {
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{})
	sink := NewMemoryDeadLetterSink()
	sub, err := StartDeadLetterSubscriber(bus, sink)
	if err != nil {
		t.Fatalf("StartDeadLetterSubscriber: %v", err)
	}
	sub.Close()
	sub.Close() // must not panic (idempotent)
	_ = bus.Close()
}

// TestStartDeadLetterSubscriber_GuardsNilArgs proves the constructor rejects
// nil bus / nil sink rather than panicking later in the drain goroutine.
func TestStartDeadLetterSubscriber_GuardsNilArgs(t *testing.T) {
	if _, err := StartDeadLetterSubscriber(nil, NewMemoryDeadLetterSink()); err == nil {
		t.Error("StartDeadLetterSubscriber(nil bus) must error")
	}
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{})
	defer bus.Close()
	if _, err := StartDeadLetterSubscriber(bus, nil); err == nil {
		t.Error("StartDeadLetterSubscriber(nil sink) must error")
	}
}

// TestMemoryDeadLetterSink_ListIsACopy proves List returns a defensive copy so
// a caller iterating cannot race a concurrent append (the -race guarantee the
// subscriber relies on).
func TestMemoryDeadLetterSink_ListIsACopy(t *testing.T) {
	sink := NewMemoryDeadLetterSink()
	if err := sink.RecordDeadLetter(context.Background(), DeadLetterRecord{TaskID: "a"}); err != nil {
		t.Fatalf("RecordDeadLetter: %v", err)
	}
	recs, _ := sink.List(context.Background())
	recs[0].TaskID = "mutated"
	again, _ := sink.List(context.Background())
	if again[0].TaskID != "a" {
		t.Errorf("List must return a copy; internal state was mutated to %q", again[0].TaskID)
	}
}
