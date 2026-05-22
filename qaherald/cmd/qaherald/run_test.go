// Wave 5 Task 7 — integration test for the qaherald run wiring.
//
// Goal: exercise runScenarios end-to-end against a fake TGSession + an
// httptest-backed *herald.Client + a real transcript writer rooted at
// t.TempDir(), then assert:
//
//  1. A canned-PASS scenario returns a nil error from runScenarios.
//  2. A canned-FAIL scenario returns a non-nil error (so Cobra's RunE
//     contract propagates os.Exit(1)).
//  3. The transcript file + results.json + report.md are written to
//     disk for the canned-PASS path AND the canned-FAIL path — partial
//     evidence is still required for §107.x audit.
//  4. The generated report.md contains the canonical PASS/FAIL tokens
//     matching the scenario verdict — the report is not allowed to
//     coerce a FAIL into a PASS.
//
// Hermetic design rationale (operator-locked):
//   - We DO NOT mock Cobra flags or environment. flagOutDirRoot points
//     at t.TempDir() via direct package-var assignment per-subtest;
//     this is brittle if used everywhere, but for an integration test
//     scoped to a single package it is the simplest pattern that still
//     exercises the runScenarios function under realistic conditions.
//   - We DO NOT register the canned scenarios into the package-level
//     scenario.Registry. The test constructs scenario.Scenario values
//     inline and passes them to runScenarios — runScenarios is exactly
//     the extraction point that lets us bypass Registry coupling.
//   - We DO NOT use the live Telegram client (paid surface). A local
//     fakeTGSession satisfies the scenario.TGSession interface (copied
//     small from scenario_test.go's pattern; the fake is package-private
//     to its package so we duplicate the minimum needed here).
//
// §107 anti-bluff hook: the canned-FAIL scenario returns a sentinel
// error from its Run body. runRun's contract is that this error
// propagates verbatim through runScenarios → into the runRun return
// value → into Cobra's os.Exit(1). The test asserts the wrap message
// ("N scenario(s) FAILed") AND counts the failures via failCount.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	toon "digital.vasic.toon/pkg/toon"
	tele "gopkg.in/telebot.v3"

	"github.com/vasic-digital/herald/qaherald/internal/herald"
	"github.com/vasic-digital/herald/qaherald/internal/report"
	"github.com/vasic-digital/herald/qaherald/internal/scenario"
	"github.com/vasic-digital/herald/qaherald/internal/transcript"
)

// fakeTGSession is a minimal in-memory TGSession satisfying the
// scenario.TGSession interface. Mirrors the fake in
// qaherald/internal/scenario/scenario_test.go (which is package-private
// to that test file). Duplicated here rather than promoted to a
// testing helper package because the duplication keeps the integration
// test self-contained.
type fakeTGSession struct {
	mu        sync.Mutex
	sent      []string
	uploads   []fakeUpload
	downloads map[string][]byte
	inbox     chan tele.Message
	nextMsgID int
}

type fakeUpload struct {
	contentType string
	filename    string
	body        []byte
}

func newFakeTGSession() *fakeTGSession {
	return &fakeTGSession{
		inbox:     make(chan tele.Message, 16),
		downloads: map[string][]byte{},
		nextMsgID: 2000,
	}
}

func (f *fakeTGSession) Send(text string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, text)
	id := f.nextMsgID
	f.nextMsgID++
	return id, nil
}

func (f *fakeTGSession) Upload(r io.Reader, contentType, filename string) (int, string, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return 0, "", err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uploads = append(f.uploads, fakeUpload{contentType: contentType, filename: filename, body: body})
	id := f.nextMsgID
	f.nextMsgID++
	fileID := "fake-file-" + filename
	f.downloads[fileID] = body
	return id, fileID, nil
}

func (f *fakeTGSession) Download(fileID string) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, ok := f.downloads[fileID]
	if !ok {
		return nil, errors.New("fakeTGSession: unknown fileID " + fileID)
	}
	return io.NopCloser(strings.NewReader(string(body))), nil
}

func (f *fakeTGSession) WaitForMessage(timeout time.Duration, predicate func(tele.Message) bool) (tele.Message, error) {
	deadline := time.After(timeout)
	for {
		select {
		case m := <-f.inbox:
			if predicate(m) {
				return m, nil
			}
		case <-deadline:
			return tele.Message{}, context.DeadlineExceeded
		}
	}
}

func (f *fakeTGSession) WaitForReply(timeout time.Duration, toMsgID int, innerPredicate func(tele.Message) bool) (tele.Message, error) {
	return f.WaitForMessage(timeout, func(m tele.Message) bool {
		if m.ReplyTo == nil || m.ReplyTo.ID != toMsgID {
			return false
		}
		if innerPredicate == nil {
			return true
		}
		return innerPredicate(m)
	})
}

// newPheraldStub stands in for a real pherald. Accepts any POST,
// returns a canned TOON-encoded Receipt with Recipients=1, and (when
// non-nil) pushes a synthetic Telegram message onto tgInbox so a
// hypothetical scenario waiting on WaitForMessage gets unblocked.
func newPheraldStub(t *testing.T, tgInbox chan<- tele.Message) *httptest.Server {
	t.Helper()
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read", http.StatusInternalServerError)
			return
		}
		// Decode generically so both JSON and TOON inputs work.
		var generic map[string]any
		ct := r.Header.Get("Content-Type")
		if toon.IsTOONContentType(ct) {
			if err := toon.Unmarshal(bodyBytes, &generic); err != nil {
				http.Error(w, "toon", http.StatusBadRequest)
				return
			}
		} else {
			if err := json.Unmarshal(bodyBytes, &generic); err != nil {
				http.Error(w, "json", http.StatusBadRequest)
				return
			}
		}
		idAny := generic["id"]
		if idAny == nil {
			idAny = generic["ID"]
		}
		idStr, _ := idAny.(string)
		if tgInbox != nil && idStr != "" {
			select {
			case tgInbox <- tele.Message{
				ID:   42,
				Chat: &tele.Chat{ID: 12345},
				Text: "qaherald delivery for " + idStr,
			}:
			default:
			}
		}
		respBody, err := toon.Marshal(herald.Receipt{
			EventID:    idStr,
			Recipients: 1,
			Status:     "accepted",
		})
		if err != nil {
			http.Error(w, "marshal", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", herald.AcceptTOON)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write(respBody)
	}))
}

// makeCannedPassScenario constructs a §107-compliant canned scenario
// that emits one KindHeraldPost (via Orchestrator.Herald.PostEvent)
// and one KindTGReceive (via Orchestrator.TG.WaitForMessage). Returns
// nil from Run on success.
func makeCannedPassScenario() scenario.Scenario {
	return scenario.Scenario{
		Name:        "qa-test-pass",
		Description: "canned-PASS scenario for runScenarios integration test",
		Run: func(ctx context.Context, o *scenario.Orchestrator) error {
			// One PostEvent — satisfies the herald.* bidirectional half.
			ce := herald.CloudEvent{
				SpecVersion: "1.0",
				ID:          fmt.Sprintf("qa-test-pass-%d", time.Now().UnixNano()),
				Source:      "qa-test",
				Type:        "qa.test.pass",
				Time:        time.Now().UTC(),
				Data:        json.RawMessage(`{"canned":"pass"}`),
			}
			cePayload, _ := json.Marshal(ce)
			_ = o.Transcript.Append(transcript.Event{
				Direction: transcript.DirectionOut,
				Kind:      transcript.KindHeraldPost,
				Scenario:  "qa-test-pass",
				Payload:   cePayload,
			})
			receipt, status, _, err := o.Herald.PostEvent(ctx, ce, herald.AcceptTOON)
			if err != nil {
				return fmt.Errorf("PostEvent: %w", err)
			}
			if status != http.StatusAccepted {
				return fmt.Errorf("expected 202, got %d", status)
			}
			receiptBytes, _ := json.Marshal(receipt)
			_ = o.Transcript.Append(transcript.Event{
				Direction: transcript.DirectionIn,
				Kind:      transcript.KindHeraldResponse,
				Scenario:  "qa-test-pass",
				Payload:   receiptBytes,
			})
			// Wait for the Telegram-side delivery the pherald stub
			// pushed onto the inbox via its handler — satisfies the
			// tg.* bidirectional half.
			msg, err := o.TG.WaitForMessage(2*time.Second, func(m tele.Message) bool {
				return strings.Contains(m.Text, ce.ID)
			})
			if err != nil {
				return fmt.Errorf("WaitForMessage: %w", err)
			}
			msgBytes, _ := json.Marshal(map[string]any{
				"message_id": msg.ID,
				"chat_id":    msg.Chat.ID,
				"text":       msg.Text,
			})
			_ = o.Transcript.Append(transcript.Event{
				Direction: transcript.DirectionIn,
				Kind:      transcript.KindTGReceive,
				Scenario:  "qa-test-pass",
				Payload:   msgBytes,
			})
			return nil
		},
	}
}

// makeCannedFailScenario constructs a scenario whose Run body
// unconditionally returns a sentinel error. The test asserts this
// error propagates through runScenarios as a non-nil aggregate.
func makeCannedFailScenario() scenario.Scenario {
	return scenario.Scenario{
		Name:        "qa-test-fail",
		Description: "canned-FAIL scenario for runScenarios integration test",
		Run: func(ctx context.Context, o *scenario.Orchestrator) error {
			// Emit one event so the transcript is non-empty when the
			// report runs over it — the FAIL evidence is still useful.
			_ = o.Transcript.Append(transcript.Event{
				Direction: transcript.DirectionInternal,
				Kind:      transcript.KindAssert,
				Scenario:  "qa-test-fail",
				Note:      "about to FAIL intentionally",
			})
			return errors.New("intentional FAIL")
		},
	}
}

// newOrchestrator wires a fakeTGSession + httptest-backed
// *herald.Client + a real transcript.Writer rooted at outDirRoot.
// Returns the orchestrator, the transcript writer (so the test can
// close + introspect), and a cleanup func.
func newOrchestrator(t *testing.T, outDirRoot string) (*scenario.Orchestrator, *transcript.Writer, func()) {
	t.Helper()

	tw, err := transcript.NewWriter(outDirRoot)
	if err != nil {
		t.Fatalf("transcript.NewWriter: %v", err)
	}
	tg := newFakeTGSession()
	srv := newPheraldStub(t, tg.inbox)
	hc := herald.NewWithClient(srv.URL, []byte("test-secret"), srv.Client())
	orch := &scenario.Orchestrator{
		TG:         tg,
		Herald:     hc,
		Transcript: tw,
		ChatID:     12345,
		Now:        func() time.Time { return time.Now().UTC() },
	}
	cleanup := func() {
		srv.Close()
	}
	return orch, tw, cleanup
}

// TestRunScenarios_CannedPass asserts the happy path: a single
// canned-PASS scenario returns a nil error from runScenarios.
func TestRunScenarios_CannedPass(t *testing.T) {
	outRoot := t.TempDir()
	orch, tw, cleanup := newOrchestrator(t, outRoot)
	defer cleanup()
	defer tw.Close()

	results, err := runScenarios(
		context.Background(),
		orch,
		[]scenario.Scenario{makeCannedPassScenario()},
		5*time.Second,
	)
	if err != nil {
		t.Fatalf("runScenarios returned error on canned-PASS: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].PASS {
		t.Fatalf("expected PASS, got FAIL with error %q", results[0].ErrorText)
	}
	if failCount(results) != 0 {
		t.Fatalf("expected failCount=0, got %d", failCount(results))
	}
}

// TestRunScenarios_CannedFail asserts runScenarios returns a non-nil
// error when a scenario FAILs, AND that the FAIL is counted by
// failCount. This is the load-bearing §107 exit-code contract.
func TestRunScenarios_CannedFail(t *testing.T) {
	outRoot := t.TempDir()
	orch, tw, cleanup := newOrchestrator(t, outRoot)
	defer cleanup()
	defer tw.Close()

	results, err := runScenarios(
		context.Background(),
		orch,
		[]scenario.Scenario{makeCannedFailScenario()},
		5*time.Second,
	)
	if err == nil {
		t.Fatal("runScenarios returned nil error on canned-FAIL — exit-code contract violated")
	}
	if !strings.Contains(err.Error(), "FAILed") {
		t.Fatalf("expected error to mention 'FAILed', got: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].PASS {
		t.Fatalf("expected FAIL, got PASS")
	}
	if !strings.Contains(results[0].ErrorText, "intentional FAIL") {
		t.Fatalf("expected error text to surface the scenario error verbatim, got %q", results[0].ErrorText)
	}
	if failCount(results) != 1 {
		t.Fatalf("expected failCount=1, got %d", failCount(results))
	}
}

// TestRunScenarios_PassAndFailMixed asserts opposite outcomes within
// one runScenarios call: one PASS + one FAIL ⇒ non-nil aggregate
// error + failCount=1.
func TestRunScenarios_PassAndFailMixed(t *testing.T) {
	outRoot := t.TempDir()
	orch, tw, cleanup := newOrchestrator(t, outRoot)
	defer cleanup()
	defer tw.Close()

	results, err := runScenarios(
		context.Background(),
		orch,
		[]scenario.Scenario{
			makeCannedPassScenario(),
			makeCannedFailScenario(),
		},
		5*time.Second,
	)
	if err == nil {
		t.Fatal("expected non-nil error when at least one scenario FAILed")
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Order-preserving: results[0] is the PASS, results[1] is the FAIL.
	if !results[0].PASS {
		t.Fatalf("results[0] expected PASS, got FAIL with error %q", results[0].ErrorText)
	}
	if results[1].PASS {
		t.Fatalf("results[1] expected FAIL, got PASS")
	}
	if failCount(results) != 1 {
		t.Fatalf("expected failCount=1, got %d", failCount(results))
	}
}

// TestRunScenarios_ReportEvidence asserts the full evidence chain:
// runScenarios → writeResults → report.Generate. We mimic the runRun
// flow without invoking Cobra/flag wiring. Asserts:
//   - results.json exists on disk and decodes to the expected Result[]
//   - report.md exists on disk and contains a PASS token for the PASS
//     scenario AND a FAIL token for the FAIL scenario
//
// §107: this is the report-correctness anchor — the canned outcomes
// from the scenarios MUST surface verbatim in the on-disk report. A
// report that coerces FAIL→PASS (or omits results.json on FAIL) would
// fail this assertion.
func TestRunScenarios_ReportEvidence(t *testing.T) {
	outRoot := t.TempDir()
	orch, tw, cleanup := newOrchestrator(t, outRoot)
	defer cleanup()

	results, runErr := runScenarios(
		context.Background(),
		orch,
		[]scenario.Scenario{
			makeCannedPassScenario(),
			makeCannedFailScenario(),
		},
		5*time.Second,
	)
	// Close transcript BEFORE report.Generate so the JSONL is flushed
	// and the report scanner sees the final state. This matches the
	// runRun production order: tw.Close → writeResults → report.Generate.
	if err := tw.Close(); err != nil {
		t.Fatalf("transcript.Close: %v", err)
	}
	if err := writeResults(tw.OutDir(), results); err != nil {
		t.Fatalf("writeResults: %v", err)
	}

	// Decode results.json from disk and assert it matches what
	// runScenarios returned. Anti-bluff: a writeResults bluff (e.g.
	// truncating to len=0) is caught here.
	resultsBytes, err := os.ReadFile(filepath.Join(tw.OutDir(), "results.json"))
	if err != nil {
		t.Fatalf("read results.json: %v", err)
	}
	var decoded []scenario.Result
	if err := json.Unmarshal(resultsBytes, &decoded); err != nil {
		t.Fatalf("decode results.json: %v", err)
	}
	if len(decoded) != 2 {
		t.Fatalf("results.json: expected 2 entries, got %d", len(decoded))
	}
	if !decoded[0].PASS {
		t.Fatalf("results.json[0] expected PASS, got FAIL")
	}
	if decoded[1].PASS {
		t.Fatalf("results.json[1] expected FAIL, got PASS")
	}

	// Generate the report against the on-disk transcript + results.
	// We invoke report.Generate directly — the same call runRun makes.
	reportPath := filepath.Join(tw.OutDir(), "report.md")
	if err := report.Generate(
		filepath.Join(tw.OutDir(), "transcript.jsonl"),
		filepath.Join(tw.OutDir(), "results.json"),
		reportPath,
	); err != nil {
		t.Fatalf("report.Generate: %v", err)
	}

	body, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read report.md: %v", err)
	}
	bodyStr := string(body)

	// PASS token — at minimum the summary table row for qa-test-pass
	// should contain "PASS".
	if !strings.Contains(bodyStr, "PASS") {
		t.Fatalf("expected report.md to contain 'PASS' token; got:\n%s", bodyStr)
	}
	// FAIL token — the canned-FAIL scenario's verdict MUST surface.
	if !strings.Contains(bodyStr, "FAIL") {
		t.Fatalf("expected report.md to contain 'FAIL' token; got:\n%s", bodyStr)
	}
	// Scenario names should surface in the summary section.
	if !strings.Contains(bodyStr, "qa-test-pass") {
		t.Fatalf("expected report.md to mention 'qa-test-pass'; got:\n%s", bodyStr)
	}
	if !strings.Contains(bodyStr, "qa-test-fail") {
		t.Fatalf("expected report.md to mention 'qa-test-fail'; got:\n%s", bodyStr)
	}
	// runErr from a mixed-outcome suite MUST be non-nil — assert here
	// so the test fails loudly if a regression silently dropped it.
	if runErr == nil {
		t.Fatal("runScenarios returned nil error from mixed PASS+FAIL suite")
	}
}
