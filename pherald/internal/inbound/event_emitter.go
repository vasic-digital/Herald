// Package inbound — event_emitter.go: concrete EventEmitter that calls
// runner.Runner.Run directly per Wave 6.5 (no HTTP loopback).
//
// §107 anchor: actually invokes Runner.Run. Nil-Runner returns
// errNoRunner sentinel — no silent drop. Wave 6.5 mutation gate (T11)
// can plant a no-op Emit; the stub-Runner unit test catches it via
// call-count assertion (calls == 1).
package inbound

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/pherald/internal/runner"
)

// errNoRunner is returned by RunnerEventEmitter.Emit when the Runner
// dependency was not injected at construction time. Callers can match
// it with errors.Is for explicit "no-runner" branching.
var errNoRunner = errors.New("inbound: EventEmitter has nil Runner")

// runnerLike is a defensive seam — concrete *runner.Runner satisfies it
// implicitly; tests inject a stub. Keeping the interface in this file
// (rather than the runner package) avoids a cyclic import and lets the
// unit test live inside package inbound.
type runnerLike interface {
	Run(ctx context.Context, raw []byte, claims map[string]any) (*runner.Receipt, error)
}

// RunnerEventEmitter constructs a structured-mode CloudEvent envelope
// from an EventPayload and dispatches it directly into the in-process
// runner.Runner pipeline. This bypasses the HTTP loopback (no
// net/http round-trip back through /v1/events) — saving a network hop
// and avoiding a self-deadlock if pherald listen runs without
// pherald serve attached.
//
// Claims carries the JWT-like claim map the Runner needs for RLS
// scoping. At minimum, claims["tenant"] MUST be a UUID string;
// runner.extractTenant rejects anything else with an explicit error.
// Construct Claims from operator-supplied env vars or pherald's own
// startup-configured tenant UUID.
type RunnerEventEmitter struct {
	R      runnerLike
	Claims map[string]any
	Clock  commons.Clock
}

// Emit builds a CloudEvent JSON object from p and calls R.Run with the
// raw bytes + Claims. Returns errNoRunner if R is nil; wraps any
// downstream error from json.Marshal or R.Run with stage context.
func (e *RunnerEventEmitter) Emit(ctx context.Context, p EventPayload) error {
	if e.R == nil {
		return errNoRunner
	}
	if e.Clock == nil {
		e.Clock = commons.RealClock{}
	}
	ce := map[string]any{
		"specversion":     "1.0",
		"id":              uuid.New().String(),
		"source":          "herald/pherald-listen",
		"type":            p.CloudEventType,
		"subject":         p.Subject,
		"time":            e.Clock.Now().UTC().Format(time.RFC3339Nano),
		"datacontenttype": "application/json",
		"data":            p.Data,
	}
	raw, err := json.Marshal(ce)
	if err != nil {
		return fmt.Errorf("EventEmitter marshal CloudEvent: %w", err)
	}
	if _, err := e.R.Run(ctx, raw, e.Claims); err != nil {
		return fmt.Errorf("EventEmitter Runner.Run: %w", err)
	}
	return nil
}
