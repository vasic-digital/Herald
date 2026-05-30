// Package infra — governance dead-letter subscriber (HRD-147).
//
// CLOSES THE EMIT-INTO-THE-VOID GAP documented in boot.go: the
// pgxTaskRepository's MoveToDeadLetter already publishes a
// digital.vasic.herald.constitution.queue.dead_letter CloudEvent whenever a
// task is dead-lettered (HRD-090, proven end-to-end in
// task_repository_integration_test.go). Prior to HRD-147 the Foundation boot
// plane deliberately wired the NIL emitter (boot.go) because no subscriber
// drained that class — publishing into an unconsumed bus is itself a §107
// PASS-bluff ("the event fans out" with nobody listening).
//
// This file supplies the missing consumer: a durable subscriber that drains
// .queue.dead_letter events off the in-process EventBus and records each one
// into a durable sink (DeadLetterSink). The boot plane (boot.go) now wires a
// REAL emitter over a MemoryBus + this subscriber, so the emit path has a
// genuine end-to-end consumer.
//
// DESIGN (surgical, mirrors the existing AuditStore pattern in
// commons_constitution/audit.go):
//
//   - DeadLetterSink is the append-only persistence interface — one
//     RecordDeadLetter mutator + one List reader, exactly like AuditStore's
//     RecordAudit / ListAudit shape.
//   - MemoryDeadLetterSink is the in-process backend (slice-backed,
//     mutex-guarded). It is the M1 / test backend, just as
//     constitution.MemoryAudit is. A Postgres backend (INSERT into a
//     dead_letter_audit table) is the M2 follow-up; the interface seam means
//     the subscriber never changes when that lands.
//   - DeadLetterSubscriber owns a drain goroutine that ranges the bus
//     Subscription channel, decodes each event's QueueEvent payload, and
//     calls sink.RecordDeadLetter. It is Close()-able (idempotent), and the
//     drain goroutine exits cleanly when the Subscription channel closes
//     (bus.Close) or Close() is called.
//
// ANTI-BLUFF (§107 / §11.4.5): RecordDeadLetter is the load-bearing durable
// side effect. The subscriber's tests assert a real MoveToDeadLetter →
// real emit → real subscriber → real sink row, end to end, with NO mock of
// the unit under test (the bus, emitter, repository, and sink are all real).
package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	constitution "github.com/vasic-digital/herald/commons_constitution"
)

// DeadLetterRecord is one durably-recorded dead-letter occurrence — the
// projection the subscriber persists from a decoded .queue.dead_letter event.
// Mirrors the shape an eventual dead_letter_audit table would carry.
type DeadLetterRecord struct {
	// EventID is the CloudEvent ID of the .queue.dead_letter event that
	// triggered this record (the bus-assigned UUID). Lets an operator
	// correlate the audit row with the wire event.
	EventID string
	// TaskID is the dead-lettered background task's id.
	TaskID string
	// Reason is why the task was dead-lettered ("exhausted retries" / ...).
	Reason string
	// FailureCount is the retry/attempt count at move-time.
	FailureCount int
	// Severity is the event severity ("high" by default for dead-letters).
	Severity string
	// RuleID is the governance rule the emit cited (e.g. "§42.1").
	RuleID string
	// RecordedAt is when the subscriber persisted this record (UTC).
	RecordedAt time.Time
}

// DeadLetterSink is the append-only persistence interface for recorded
// dead-letter occurrences. Mirrors constitution.AuditStore's append-only
// shape (RecordAudit / ListAudit): RecordDeadLetter is the only mutator and
// List is the only reader. Backends:
//
//   - MemoryDeadLetterSink (this file) — slice-backed, M1 / test.
//   - A future PostgresDeadLetterSink — INSERT into a dead_letter_audit
//     table; the subscriber is unchanged when that lands.
type DeadLetterSink interface {
	// RecordDeadLetter appends one record durably. Must be safe for
	// concurrent callers (the subscriber is single-goroutine today, but the
	// interface contract must not assume that).
	RecordDeadLetter(ctx context.Context, rec DeadLetterRecord) error
	// List returns every recorded dead-letter, oldest-first (insertion
	// order). Returns a non-nil (possibly empty) slice — a nil slice would
	// be a §107 ambiguity ("did the read run?").
	List(ctx context.Context) ([]DeadLetterRecord, error)
}

// MemoryDeadLetterSink is the in-process DeadLetterSink. Slice-backed +
// mutex-guarded so it is safe under -race. Test / M1 backend; the production
// swap is a Postgres-backed sink behind the same interface.
type MemoryDeadLetterSink struct {
	mu      sync.Mutex
	records []DeadLetterRecord
}

// NewMemoryDeadLetterSink returns an empty in-process sink.
func NewMemoryDeadLetterSink() *MemoryDeadLetterSink {
	return &MemoryDeadLetterSink{records: make([]DeadLetterRecord, 0)}
}

// RecordDeadLetter appends rec under the mutex. Stamps RecordedAt = now()
// when the caller left it zero.
func (s *MemoryDeadLetterSink) RecordDeadLetter(_ context.Context, rec DeadLetterRecord) error {
	if rec.RecordedAt.IsZero() {
		rec.RecordedAt = time.Now().UTC()
	}
	s.mu.Lock()
	s.records = append(s.records, rec)
	s.mu.Unlock()
	return nil
}

// List returns a copy of the recorded slice (oldest-first), so a caller
// iterating cannot race a concurrent RecordDeadLetter append.
func (s *MemoryDeadLetterSink) List(_ context.Context) ([]DeadLetterRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]DeadLetterRecord, len(s.records))
	copy(out, s.records)
	return out, nil
}

// Len returns the number of recorded dead-letters. Convenience for tests +
// health probes that just need the count without copying the slice.
func (s *MemoryDeadLetterSink) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.records)
}

// deadLetterEventType is the CloudEvents type the subscriber filters on:
// the canonical digital.vasic.herald.constitution.queue.dead_letter.
var deadLetterEventType = constitution.EventNamespace + "." + constitution.ClassQueueDeadLetter

// DeadLetterSubscriber drains .queue.dead_letter events off an EventBus and
// records each into a DeadLetterSink. It owns one drain goroutine started by
// StartDeadLetterSubscriber.
type DeadLetterSubscriber struct {
	sub  *constitution.Subscription
	sink DeadLetterSink
	done chan struct{} // closed when the drain goroutine has fully exited

	closeOnce sync.Once

	// decodeErrs counts events that arrived but whose payload failed to
	// decode (so a malformed event surfaces as a metric rather than a silent
	// drop — §107). Guarded by mu.
	mu         sync.Mutex
	recorded   int
	decodeErrs int
}

// StartDeadLetterSubscriber subscribes to the dead-letter event type on bus
// and starts the drain goroutine that records every received event into sink.
// Returns an error if the subscription cannot be created (e.g. the bus is
// already closed). The returned subscriber MUST be Close()d to release the
// subscription + stop the goroutine (boot.Down does this).
func StartDeadLetterSubscriber(bus constitution.EventBus, sink DeadLetterSink) (*DeadLetterSubscriber, error) {
	if bus == nil {
		return nil, fmt.Errorf("commons_infra: StartDeadLetterSubscriber: nil bus")
	}
	if sink == nil {
		return nil, fmt.Errorf("commons_infra: StartDeadLetterSubscriber: nil sink")
	}
	sub, err := bus.Subscribe(deadLetterEventType)
	if err != nil {
		return nil, fmt.Errorf("commons_infra: StartDeadLetterSubscriber: subscribe: %w", err)
	}
	s := &DeadLetterSubscriber{
		sub:  sub,
		sink: sink,
		done: make(chan struct{}),
	}
	go s.drain()
	return s, nil
}

// drain ranges the subscription channel until it closes (bus.Close or
// sub.Cancel), decoding + recording each event. Exits by closing s.done.
func (s *DeadLetterSubscriber) drain() {
	defer close(s.done)
	for ev := range s.sub.Channel {
		s.handle(ev)
	}
}

// busPayload is the on-the-wire body emit() produces: an envelope + the typed
// per-class payload. We only need the QueueEvent payload fields here.
type busPayload struct {
	Payload constitution.QueueEvent `json:"payload"`
}

// handle decodes one event + records it. A decode failure is counted (not
// fatal) so one malformed event cannot wedge the drain loop, but it is never
// silently swallowed — decodeErrs is observable via Stats().
func (s *DeadLetterSubscriber) handle(ev constitution.Event) {
	var body busPayload
	if err := json.Unmarshal(ev.Data, &body); err != nil {
		s.mu.Lock()
		s.decodeErrs++
		s.mu.Unlock()
		return
	}
	rec := DeadLetterRecord{
		EventID:      ev.ID,
		TaskID:       body.Payload.TaskID,
		Reason:       body.Payload.FailureReason,
		FailureCount: body.Payload.FailureCount,
		Severity:     ev.Metadata["severity"],
		RuleID:       ev.Metadata["rule_id"],
		RecordedAt:   time.Now().UTC(),
	}
	// Best-effort record; a sink error increments decodeErrs-adjacent
	// accounting via recorded staying flat. RecordDeadLetter on the memory
	// sink never errors; a Postgres sink that errors is surfaced by the
	// recorded-vs-published gap a health probe checks.
	if err := s.sink.RecordDeadLetter(context.Background(), rec); err != nil {
		s.mu.Lock()
		s.decodeErrs++
		s.mu.Unlock()
		return
	}
	s.mu.Lock()
	s.recorded++
	s.mu.Unlock()
}

// Stats returns the count of successfully-recorded dead-letters and the count
// of events that arrived but could not be decoded/recorded. Used by tests +
// health probes as the anti-bluff evidence that the subscriber actually
// consumed events (a non-zero recorded count proves the end-to-end path).
func (s *DeadLetterSubscriber) Stats() (recorded, decodeErrors int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recorded, s.decodeErrs
}

// Close cancels the subscription and waits for the drain goroutine to exit.
// Idempotent. After Close the subscriber records no further events.
func (s *DeadLetterSubscriber) Close() {
	s.closeOnce.Do(func() {
		s.sub.Cancel() // closes the channel → drain loop ranges to completion
		<-s.done       // wait for the goroutine to fully drain + exit
	})
}
