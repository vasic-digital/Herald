// Package scenario is the qaherald scenario engine.
//
// A Scenario is a typed Go function with a registered name. The
// Orchestrator runs one scenario at a time, threads the shared
// Telegram + Herald clients + transcript writer through it, and emits
// scenario.start / scenario.end events around the user-supplied Run
// body. Each scenario records its bidirectional steps to the
// transcript — every PASS carries auditable runtime evidence per
// Herald §107.x.
//
// §107 anti-bluff anchor (load-bearing for the whole package):
//
//   - Every scenario MUST emit ≥1 KindHeraldPost / KindHeraldGet event
//     AND ≥1 KindTGSend / KindTGReceive / KindTGUpload / KindTGDownload
//     event to the transcript before returning nil. The
//     ValidateScenarioBidirectional helper (registry.go) walks the
//     transcript on disk and asserts the invariant — the T10 Wave 5
//     mutation gate (a) blanks Writer.Append so every scenario's
//     post-condition check fails when ValidateScenarioBidirectional is
//     re-run after the mutation.
//
//   - The TGSession + HeraldSession interfaces below allow the unit
//     test in scenario_test.go to stand the scenario up against an
//     httptest TLS server + an in-memory tgram fake without burning
//     live Telegram API quota. The concrete *tgram.Client and
//     *herald.Client implicitly satisfy both interfaces — see
//     scenario_test.go for the fake implementations.
package scenario

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	tele "gopkg.in/telebot.v3"

	"github.com/vasic-digital/herald/qaherald/internal/herald"
	"github.com/vasic-digital/herald/qaherald/internal/transcript"
)

// TGSession is the subset of *tgram.Client methods scenarios use.
// Defining it as an interface lets the unit test inject an in-memory
// fake. The real *tgram.Client (qaherald/internal/tgram/client.go)
// implicitly satisfies this contract — verified by go build.
type TGSession interface {
	// Send delivers a text message to the configured chat and returns
	// the Telegram-assigned message_id.
	Send(text string) (int, error)
	// Upload sends a file (photo/voice/document picked from
	// contentType) and returns the chat-side message_id + Telegram
	// fileID handle.
	Upload(r io.Reader, contentType, filename string) (msgID int, fileID string, err error)
	// Download fetches a Telegram-side file by its fileID. The caller
	// MUST close the returned ReadCloser.
	Download(fileID string) (io.ReadCloser, error)
	// WaitForMessage drains the inbox until a message matching
	// predicate arrives or timeout elapses.
	WaitForMessage(timeout time.Duration, predicate func(tele.Message) bool) (tele.Message, error)
	// WaitForReply waits for a message whose ReplyTo.ID equals
	// toMsgID and satisfies innerPredicate.
	WaitForReply(timeout time.Duration, toMsgID int, innerPredicate func(tele.Message) bool) (tele.Message, error)
}

// HeraldSession is the subset of *herald.Client methods scenarios use.
// The real *herald.Client (qaherald/internal/herald/client.go)
// implicitly satisfies this contract.
type HeraldSession interface {
	PostEvent(ctx context.Context, ce herald.CloudEvent, accept string) (herald.Receipt, int, http.Header, error)
	GetCompliance(ctx context.Context, accept string) (json.RawMessage, int, http.Header, error)
	GetSafety(ctx context.Context, accept string) (json.RawMessage, int, http.Header, error)
}

// Orchestrator is the shared context every scenario receives. It holds
// the Telegram client (real or fake), the Herald REST client (real or
// httptest-backed), the transcript writer (always real — even unit
// tests record into a t.TempDir transcript), the chat ID for outbound
// Telegram delivery, and a Now() time provider so deterministic tests
// can pin timestamps.
type Orchestrator struct {
	TG         TGSession
	Herald     HeraldSession
	Transcript *transcript.Writer
	ChatID     int64
	Now        func() time.Time
}

// Scenario is a named Go function registered into the package-level
// registry (see registry.go). Run receives the orchestrator + a
// context the caller can cancel; it returns nil on PASS, a
// descriptive error on FAIL.
type Scenario struct {
	Name        string
	Description string
	Run         func(ctx context.Context, o *Orchestrator) error
}

// Result is what RunScenario emits to the transcript at scenario end.
// The report generator (T6) keys off the JSON-tagged fields when
// rendering the PASS/FAIL summary table.
type Result struct {
	Scenario  string        `json:"scenario"`
	PASS      bool          `json:"pass"`
	Duration  time.Duration `json:"duration"`
	ErrorText string        `json:"error,omitempty"`
}

// RunScenario executes a single Scenario inside the orchestrator
// context, emitting scenario.start / scenario.end transcript events
// around the user body. PASS/FAIL is keyed on whether the user's Run
// returned nil; the err is propagated verbatim into Result.ErrorText.
//
// Every transcript Append is best-effort — Append failures do not
// abort the scenario (they would mask the actual scenario error). The
// §107 anti-bluff guarantee is preserved by the post-run
// ValidateScenarioBidirectional check in registry.go, which the
// caller (T7 runner or the unit test) MAY invoke after RunScenario
// returns.
func (o *Orchestrator) RunScenario(ctx context.Context, s Scenario) Result {
	start := o.now()
	_ = o.Transcript.Append(transcript.Event{
		TS:        start,
		Direction: transcript.DirectionInternal,
		Kind:      transcript.KindScenarioStart,
		Scenario:  s.Name,
		Note:      s.Description,
	})
	err := s.Run(ctx, o)
	end := o.now()
	res := Result{
		Scenario: s.Name,
		PASS:     err == nil,
		Duration: end.Sub(start),
	}
	if err != nil {
		res.ErrorText = err.Error()
	}
	_ = o.Transcript.Append(transcript.Event{
		TS:        end,
		Direction: transcript.DirectionInternal,
		Kind:      transcript.KindScenarioEnd,
		Scenario:  s.Name,
		Note:      fmt.Sprintf("PASS=%v err=%v", res.PASS, err),
	})
	return res
}

// now returns o.Now() if set, otherwise time.Now().UTC(). Keeps the
// orchestrator usable from production callers (T7) without forcing
// every caller to wire a clock.
func (o *Orchestrator) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now().UTC()
}

// Wait sleeps for d but returns early if ctx is cancelled. Scenarios
// use this in place of time.Sleep so a parent-cancelled context
// (e.g. test timeout) propagates immediately.
func (o *Orchestrator) Wait(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
