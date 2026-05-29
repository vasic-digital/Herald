package constitution

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestDeadLetterEmitsQueueClass is the RED test for the HRD-090 .queue.dead_letter
// operational event class. It asserts that the DeadLetter emit helper publishes
// exactly one event on the canonical CloudEvents type
// digital.vasic.herald.constitution.queue.dead_letter, subjected to the task,
// carrying the failure reason + count in the payload — the load-bearing
// anti-bluff evidence that a dead-lettered task actually fans out a governance
// event rather than vanishing silently (§107).
func TestDeadLetterEmitsQueueClass(t *testing.T) {
	bus := NewMemoryBus(MemoryBusConfig{})
	defer bus.Close()
	em := newTestEmitter(t, bus)
	sub, _ := bus.Subscribe("*")
	defer sub.Cancel()

	tenant := uuid.New()
	bundle := CaptureBytes([]byte("v1"))
	tr := Transition{Changed: true, NewDecision: DecisionFail, OldDecision: DecisionPass}
	ctx := context.Background()

	err := em.DeadLetter(ctx, QueueEvent{
		TenantID:      tenant,
		RuleID:        "§42.1",
		Severity:      SeverityHigh,
		TaskID:        "task-abc-123",
		FailureReason: "exhausted retries",
		FailureCount:  5,
		Bundle:        bundle,
		Transition:    tr,
	})
	if err != nil {
		t.Fatalf("DeadLetter emit returned error: %v", err)
	}

	select {
	case e := <-sub.Channel:
		wantType := EventNamespace + "." + ClassQueueDeadLetter
		if e.Type != wantType {
			t.Errorf("event type = %q; want %q", e.Type, wantType)
		}
		if e.Subject != "task:task-abc-123" {
			t.Errorf("event subject = %q; want %q", e.Subject, "task:task-abc-123")
		}
		var body struct {
			Payload struct {
				TaskID        string `json:"TaskID"`
				FailureReason string `json:"FailureReason"`
				FailureCount  int    `json:"FailureCount"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(e.Data, &body); err != nil {
			t.Fatalf("unmarshal event data: %v", err)
		}
		if body.Payload.TaskID != "task-abc-123" {
			t.Errorf("payload TaskID = %q; want %q", body.Payload.TaskID, "task-abc-123")
		}
		if body.Payload.FailureReason != "exhausted retries" {
			t.Errorf("payload FailureReason = %q; want %q", body.Payload.FailureReason, "exhausted retries")
		}
		if body.Payload.FailureCount != 5 {
			t.Errorf("payload FailureCount = %d; want 5", body.Payload.FailureCount)
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive a .queue.dead_letter event within 1s")
	}
}

// TestQueueDeadLetterInAllClasses pins the new class into the closed set so
// boot-time validation + metrics-cardinality bounds include it.
func TestQueueDeadLetterInAllClasses(t *testing.T) {
	found := false
	for _, c := range AllClasses() {
		if c == ClassQueueDeadLetter {
			found = true
		}
	}
	if !found {
		t.Errorf("AllClasses() does not contain ClassQueueDeadLetter (%q)", ClassQueueDeadLetter)
	}
}
