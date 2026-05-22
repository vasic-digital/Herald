// Package inbound — event_emitter_test.go: stub-Runner unit tests per
// Wave 6.5 T3.
//
// §107 anchor: a no-op Emit would FAIL because stubRunner.calls would
// stay at 0. Sentinel errNoRunner is matched via errors.Is.
package inbound

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/pherald/internal/runner"
)

// stubRunner records each Run invocation + the raw bytes / claims
// passed. nextErr (if set) is returned from Run; otherwise Run returns
// a zero Receipt + nil.
type stubRunner struct {
	calls   int
	lastRaw []byte
	lastClaims map[string]any
	nextErr error
}

func (s *stubRunner) Run(ctx context.Context, raw []byte, claims map[string]any) (*runner.Receipt, error) {
	s.calls++
	s.lastRaw = append([]byte(nil), raw...)
	s.lastClaims = claims
	if s.nextErr != nil {
		return nil, s.nextErr
	}
	return &runner.Receipt{EventID: "stub-event"}, nil
}

func TestRunnerEventEmitter_NilRunnerSentinel(t *testing.T) {
	e := &RunnerEventEmitter{R: nil}
	err := e.Emit(context.Background(), EventPayload{CloudEventType: "x", Subject: "y", Data: map[string]any{"k": "v"}})
	if err == nil {
		t.Fatal("expected error for nil Runner, got nil")
	}
	if !errors.Is(err, errNoRunner) {
		t.Errorf("expected errNoRunner sentinel, got %v", err)
	}
}

func TestRunnerEventEmitter_HappyPath(t *testing.T) {
	stub := &stubRunner{}
	claims := map[string]any{"tenant": "00000000-0000-0000-0000-000000000001"}
	e := &RunnerEventEmitter{R: stub, Claims: claims, Clock: commons.RealClock{}}

	p := EventPayload{
		CloudEventType: "io.herald.test.event",
		Subject:        "subject-abc",
		Data:           map[string]any{"foo": "bar", "n": float64(42)},
	}
	if err := e.Emit(context.Background(), p); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	// §107: a no-op Emit (`return nil`) would fail this — calls would be 0.
	if stub.calls != 1 {
		t.Fatalf("Run call count: got %d want 1", stub.calls)
	}

	// Round-trip the raw bytes and assert CloudEvent fields.
	var ce map[string]any
	if err := json.Unmarshal(stub.lastRaw, &ce); err != nil {
		t.Fatalf("unmarshal CloudEvent: %v\nraw: %s", err, string(stub.lastRaw))
	}
	if got := ce["type"]; got != "io.herald.test.event" {
		t.Errorf("CloudEvent type: got %v want %q", got, "io.herald.test.event")
	}
	if got := ce["subject"]; got != "subject-abc" {
		t.Errorf("CloudEvent subject: got %v want %q", got, "subject-abc")
	}
	if got := ce["specversion"]; got != "1.0" {
		t.Errorf("CloudEvent specversion: got %v want %q", got, "1.0")
	}
	if got := ce["source"]; got != "herald/pherald-listen" {
		t.Errorf("CloudEvent source: got %v want %q", got, "herald/pherald-listen")
	}
	if got := ce["datacontenttype"]; got != "application/json" {
		t.Errorf("CloudEvent datacontenttype: got %v want %q", got, "application/json")
	}
	if ce["id"] == nil || ce["id"] == "" {
		t.Errorf("CloudEvent id: expected non-empty UUID, got %v", ce["id"])
	}
	if ce["time"] == nil || ce["time"] == "" {
		t.Errorf("CloudEvent time: expected non-empty RFC3339Nano, got %v", ce["time"])
	}

	// Data round-trip.
	data, ok := ce["data"].(map[string]any)
	if !ok {
		t.Fatalf("CloudEvent data not a map: %T %v", ce["data"], ce["data"])
	}
	if got := data["foo"]; got != "bar" {
		t.Errorf("data.foo: got %v want %q", got, "bar")
	}
	if got := data["n"]; got != float64(42) {
		t.Errorf("data.n: got %v want 42", got)
	}

	// Claims passed through verbatim.
	if stub.lastClaims == nil {
		t.Fatal("claims not propagated to Runner")
	}
	if got := stub.lastClaims["tenant"]; got != "00000000-0000-0000-0000-000000000001" {
		t.Errorf("claims.tenant: got %v", got)
	}

	// Raw bytes must literally contain the expected JSON fragments —
	// guards against an Emit that builds an unrelated payload.
	rawStr := string(stub.lastRaw)
	for _, want := range []string{
		`"type":"io.herald.test.event"`,
		`"subject":"subject-abc"`,
		`"specversion":"1.0"`,
	} {
		if !containsSubstr(rawStr, want) {
			t.Errorf("raw bytes missing %q\nraw: %s", want, rawStr)
		}
	}
}

func TestRunnerEventEmitter_RunErrorPropagated(t *testing.T) {
	sentinel := errors.New("runner: stage-3 boom")
	stub := &stubRunner{nextErr: sentinel}
	e := &RunnerEventEmitter{R: stub, Claims: map[string]any{"tenant": "00000000-0000-0000-0000-000000000001"}}
	err := e.Emit(context.Background(), EventPayload{CloudEventType: "x", Subject: "y", Data: map[string]any{}})
	if err == nil {
		t.Fatal("expected error from stub.nextErr, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected wrapped sentinel; got %v", err)
	}
	if stub.calls != 1 {
		t.Errorf("expected 1 Run call even on error; got %d", stub.calls)
	}
}

// containsSubstr — tiny helper; avoids pulling strings into the test
// just for one assertion.
func containsSubstr(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
