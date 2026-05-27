package constitution_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	constitution "github.com/vasic-digital/herald/commons_constitution"
	"github.com/vasic-digital/herald/commons_constitution/ladder"
	"github.com/vasic-digital/herald/commons_constitution/state"
)

// TestRunner_WritesAuditRowOnEnforce is the load-bearing anti-bluff proof
// for HRD-018: prior to this, RunOutcome.Audited was set true but NOTHING
// was persisted. A real durable audit row MUST land for every CHANGED
// transition whose mode is ModeWarn or ModeEnforce.
func TestRunner_WritesAuditRowOnEnforce(t *testing.T) {
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
	au := state.NewMemoryAudit()
	runner, err := constitution.NewRunner(reg, la, st, em, au)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	tenant := uuid.New()
	bundle := constitution.CaptureBytes([]byte("rev-audit"))
	subject := constitution.Subject{Kind: "file", ID: "/etc/x"}

	ev := &evalForTest{id: "§audit", sev: constitution.SeverityHigh, result: makeResult(constitution.DecisionFail, "boom")}
	reg.Register(ev)
	out, err := runner.Run(ctx, ev, tenant, subject, bundle)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !out.Audited {
		t.Fatalf("ModeEnforce + transition must audit; out=%+v", out)
	}

	rows, err := au.ListAudit(ctx, tenant, constitution.AuditQuery{})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 audit row, got %d", len(rows))
	}
	r := rows[0]
	if r.RuleID != "§audit" || r.Subject != subject.ID {
		t.Errorf("audit row rule/subject mismatch: %+v", r)
	}
	if r.NewDecision != constitution.DecisionFail {
		t.Errorf("audit NewDecision = %v; want fail", r.NewDecision)
	}
	if r.ModeAtEmission != constitution.ModeEnforce {
		t.Errorf("audit ModeAtEmission = %v; want enforce", r.ModeAtEmission)
	}
	// Enforce emits → EmittedEventID MUST be set (non-Nil).
	if r.EmittedEventID == uuid.Nil {
		t.Errorf("ModeEnforce audit row must carry the emitted event ID; got Nil")
	}
	if r.BundleHash != bundle {
		t.Errorf("audit BundleHash mismatch")
	}
	// First sight → OldDecision nil.
	if r.OldDecision != nil {
		t.Errorf("first-seen audit row should have nil OldDecision; got %v", *r.OldDecision)
	}
}

// TestRunner_WarnAuditsButNoEmittedID proves ModeWarn writes an audit row
// with a NIL EmittedEventID (audit-only, no channel push).
func TestRunner_WarnAuditsButNoEmittedID(t *testing.T) {
	ctx := context.Background()
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{BufferSize: 64})
	defer bus.Close()
	em, _ := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "test"})
	reg := constitution.NewRegistry()
	st := state.NewMemory()
	la := ladder.NewMemory()
	au := state.NewMemoryAudit()
	runner, _ := constitution.NewRunner(reg, la, st, em, au)

	tenant := uuid.New()
	bundle := constitution.CaptureBytes([]byte("rev-warn"))
	subject := constitution.Subject{Kind: "file", ID: "/etc/y"}
	if err := la.Set(ctx, tenant, "§warn", constitution.ModeWarn, "ops"); err != nil {
		t.Fatalf("Set warn: %v", err)
	}

	ev := &evalForTest{id: "§warn", sev: constitution.SeverityMiddle, result: makeResult(constitution.DecisionFail, "warnable")}
	reg.Register(ev)
	out, err := runner.Run(ctx, ev, tenant, subject, bundle)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !out.Audited || out.Emitted {
		t.Fatalf("ModeWarn must audit but not emit; out=%+v", out)
	}
	rows, err := au.ListAudit(ctx, tenant, constitution.AuditQuery{})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 warn audit row, got %d", len(rows))
	}
	if rows[0].EmittedEventID != uuid.Nil {
		t.Errorf("ModeWarn audit row must have Nil EmittedEventID; got %v", rows[0].EmittedEventID)
	}
	if rows[0].ModeAtEmission != constitution.ModeWarn {
		t.Errorf("ModeAtEmission = %v; want warn", rows[0].ModeAtEmission)
	}
}

// TestRunner_AllowWritesNoAuditRow proves ModeAllow records state only —
// no audit row, no emit (per §42.1.4 ladder semantics).
func TestRunner_AllowWritesNoAuditRow(t *testing.T) {
	ctx := context.Background()
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{BufferSize: 64})
	defer bus.Close()
	em, _ := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "test"})
	reg := constitution.NewRegistry()
	st := state.NewMemory()
	la := ladder.NewMemory()
	au := state.NewMemoryAudit()
	runner, _ := constitution.NewRunner(reg, la, st, em, au)

	tenant := uuid.New()
	bundle := constitution.CaptureBytes([]byte("rev-allow"))
	subject := constitution.Subject{Kind: "file", ID: "/etc/z"}
	if err := la.Set(ctx, tenant, "§allow", constitution.ModeAllow, "ops"); err != nil {
		t.Fatalf("Set allow: %v", err)
	}

	ev := &evalForTest{id: "§allow", sev: constitution.SeverityLow, result: makeResult(constitution.DecisionFail, "ignored")}
	reg.Register(ev)
	out, err := runner.Run(ctx, ev, tenant, subject, bundle)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.Audited || out.Emitted {
		t.Fatalf("ModeAllow must neither audit nor emit; out=%+v", out)
	}
	rows, err := au.ListAudit(ctx, tenant, constitution.AuditQuery{})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("ModeAllow must write zero audit rows; got %d", len(rows))
	}
}

// TestRunner_NoChangeNoAuditRow proves the transitions-only discipline:
// an unchanged re-run writes no second audit row.
func TestRunner_NoChangeNoAuditRow(t *testing.T) {
	ctx := context.Background()
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{BufferSize: 64})
	defer bus.Close()
	em, _ := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "test"})
	reg := constitution.NewRegistry()
	st := state.NewMemory()
	la := ladder.NewMemory()
	au := state.NewMemoryAudit()
	runner, _ := constitution.NewRunner(reg, la, st, em, au)

	tenant := uuid.New()
	bundle := constitution.CaptureBytes([]byte("rev-nc"))
	subject := constitution.Subject{Kind: "file", ID: "/etc/nc"}
	ev := &evalForTest{id: "§nc", sev: constitution.SeverityHigh, result: makeResult(constitution.DecisionFail, "x")}
	reg.Register(ev)

	if _, err := runner.Run(ctx, ev, tenant, subject, bundle); err != nil {
		t.Fatalf("Run 1: %v", err)
	}
	if _, err := runner.Run(ctx, ev, tenant, subject, bundle); err != nil {
		t.Fatalf("Run 2 (no change): %v", err)
	}
	rows, err := au.ListAudit(ctx, tenant, constitution.AuditQuery{})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("transitions-only: expected 1 audit row after a no-change re-run, got %d", len(rows))
	}
}

// TestMemoryAudit_RLSStyleTenantScope proves ListAudit only returns the
// queried tenant's rows (mirrors the Postgres RLS isolation in-memory).
func TestMemoryAudit_TenantScope(t *testing.T) {
	ctx := context.Background()
	au := state.NewMemoryAudit()
	tA, tB := uuid.New(), uuid.New()
	if _, err := au.RecordAudit(ctx, constitution.AuditRow{TenantID: tA, RuleID: "§r", Subject: "s", NewDecision: constitution.DecisionFail}); err != nil {
		t.Fatalf("RecordAudit A: %v", err)
	}
	rows, err := au.ListAudit(ctx, tB, constitution.AuditQuery{})
	if err != nil {
		t.Fatalf("ListAudit B: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("tenant B must not see tenant A's audit rows; got %d", len(rows))
	}
}
