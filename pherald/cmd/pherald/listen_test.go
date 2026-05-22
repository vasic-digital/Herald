// Wave 6 T6 — pherald listen integration test.
//
// Test approach (hermetic, fast): rather than compiling+spawning the
// pherald binary, call runListen(ctx, cfg) directly with stub Subscriber
// + stub CodeDispatcher + recording Replier. The test asserts:
//
//   (a) ≥1 handler invocation arrives within 8s when the stub Subscriber
//       publishes one synthetic InboundEvent.
//   (b) Context cancel (simulating SIGTERM) causes runListen to return
//       cleanly (nil — the helper masks ctx.Err() per its doc-comment).
//   (c) The action routing pipeline reached the SendReply sink (the
//       Dispatcher dispatched action=reply because the stub Code emitted
//       a canned <<<HERALD-REPLY>>> blob — Wave 6 T7 routing).
//
// §107 anchor — every PASS is a positive runtime observation: the
// recordingReplier MUST have its called flag set within 8s. A test that
// only confirmed "no panic" would be a bluff.
package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/pherald/internal/inbound"
)

// stubSubscriber publishes one synthetic InboundEvent (text "ping" from
// chat 12345) on first call, then blocks on ctx.Done(). Mirrors the
// behaviour of tgram.Subscribe (it returns ctx.Err() when the ctx is
// cancelled). The handler invocation count is exposed via the counter
// pointer so the test can poll it.
func stubSubscriber(counter *int32) func(context.Context, commons.InboundHandler) error {
	return func(ctx context.Context, h commons.InboundHandler) error {
		// Dispatch one synthetic event immediately so the test sees the
		// handler get invoked (proving the wiring is load-bearing).
		go func() {
			ev := commons.InboundEvent{
				EventID: "01HXYZTEST",
				Sender: commons.Recipient{
					Channel:       string(commons.ChannelTelegram),
					ChannelUserID: "12345",
				},
				Body: commons.Body{Plain: "ping"},
				Raw: map[string]any{
					"message_id": 42,
					"chat_id":    int64(12345),
					"text":       "ping",
				},
			}
			if err := h.Handle(ctx, ev); err == nil {
				atomic.AddInt32(counter, 1)
			}
		}()
		<-ctx.Done()
		return ctx.Err()
	}
}

// stubCode returns a canned <<<HERALD-REPLY>>> blob so inbound.Dispatcher
// routes deterministically to the "reply" action (which calls SendReply
// on the recording replier — proving the full pipeline ran end-to-end).
type stubCode struct{}

func (stubCode) Dispatch(_ context.Context, _ inbound.CodeRequest) (inbound.CodeResponse, error) {
	return inbound.CodeResponse{
		Stdout: []byte(`<<<HERALD-REPLY>>> {"action":"reply","text":"hi from stub"}`),
	}, nil
}

// recordingReplier records SendReply calls so the test can assert the
// action=reply routing reached this sink.
type recordingReplier struct {
	mu      sync.Mutex
	called  int
	lastTxt string
}

func (r *recordingReplier) SendReply(_ context.Context, _ int64, text string, _ int, _ []commons.Attachment) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.called++
	r.lastTxt = text
	return 1, nil
}

func (r *recordingReplier) Called() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.called
}

func (r *recordingReplier) LastText() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastTxt
}

// TestListenWiresHandlerAndExitsOnCancel exercises runListen end-to-end
// against the hermetic stubs. It asserts (a) handler invoked ≥1, (b)
// SendReply sink reached ≥1, (c) ctx cancel returns runListen cleanly.
func TestListenWiresHandlerAndExitsOnCancel(t *testing.T) {
	if testing.Short() {
		t.Skip("listen integration test skipped in -short mode")
	}

	var handlerCount int32
	replier := &recordingReplier{}

	cfg := listenConfig{
		ProjectName: "TestProj",
		BotToken:    "fake-token",
		ChatID:      "12345",
		Subscriber:  stubSubscriber(&handlerCount),
		Code:        stubCode{},
		Replier:     replier,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run runListen in a goroutine; it blocks on cfg.Subscriber until ctx
	// is cancelled.
	done := make(chan error, 1)
	go func() {
		done <- runListen(ctx, cfg)
	}()

	// Poll for handler invocation. Budget: 8s, 50ms tick (160 iterations
	// max). Reaching ≥1 on handlerCount proves the stub subscriber's
	// synthetic event flowed through inbound.Dispatcher.Handle. Reaching
	// ≥1 on replier.Called proves the action="reply" branch routed to
	// SendReply — the full T7 pipeline ran.
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&handlerCount) >= 1 && replier.Called() >= 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if got := atomic.LoadInt32(&handlerCount); got < 1 {
		cancel()
		<-done
		t.Fatalf("inbound handler not invoked within 8s (handlerCount=%d) — listen runtime is a bluff", got)
	}
	if got := replier.Called(); got < 1 {
		cancel()
		<-done
		t.Fatalf("SendReply sink not reached within 8s (called=%d) — action routing is a bluff", got)
	}
	if txt := replier.LastText(); txt != "hi from stub" {
		cancel()
		<-done
		t.Fatalf("SendReply text mismatch: got %q want %q (stubCode reply not routed verbatim)", txt, "hi from stub")
	}

	// Cancel ctx to simulate SIGTERM; runListen MUST return within 3s.
	cancel()
	select {
	case err := <-done:
		// runListen masks ctx.Err() and returns nil on clean cancel; see
		// the helper's doc-comment for the §107 rationale.
		if err != nil {
			t.Fatalf("runListen returned non-nil on cancel: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runListen did not return within 3s of ctx cancel — goroutine leak")
	}
}

// TestRunListenRejectsBadConfig asserts NewDispatcher's validation
// surfaces through runListen (cfg.Code == nil → error). This is the
// negative-path assertion that prevents an empty-config "boots silently"
// PASS-bluff (e.g. if a future refactor drops the nil check).
func TestRunListenRejectsBadConfig(t *testing.T) {
	cfg := listenConfig{
		ProjectName: "TestProj",
		Subscriber:  func(ctx context.Context, _ commons.InboundHandler) error { <-ctx.Done(); return ctx.Err() },
		// Code and Replier left nil — NewDispatcher MUST reject.
	}
	err := runListen(context.Background(), cfg)
	if err == nil {
		t.Fatal("runListen accepted nil Code/Replier — NewDispatcher validation bypass")
	}
}

// TestParseOperatorIDs — Wave 6.5 T7 §32.6 operator-allowlist parser.
// HERALD_OPERATOR_IDS is comma-separated channel_user_id values
// (Telegram numeric chat IDs); the parser MUST tolerate whitespace
// around segments and skip empty segments (including leading /
// trailing commas, double-commas).
//
// §107 anchor: a parser that silently swallowed valid IDs would
// reject legitimate operators (Done:/Reopen: would always fail
// even when properly configured). The 5 cases below pin the
// happy paths + edge cases.
func TestParseOperatorIDs(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{name: "empty", in: "", want: nil},
		{name: "single", in: "12345", want: []string{"12345"}},
		{name: "two", in: "12345,67890", want: []string{"12345", "67890"}},
		{name: "whitespace", in: "  12345  ,  67890  ", want: []string{"12345", "67890"}},
		{name: "empty-segments", in: ",,12345,,", want: []string{"12345"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseOperatorIDs(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("len mismatch in=%q got=%v want=%v", tc.in, got, tc.want)
			}
			for _, k := range tc.want {
				if !got[k] {
					t.Fatalf("missing %q in result for in=%q (got=%v)", k, tc.in, got)
				}
			}
		})
	}
}

// TestWireDocsAndCommandsRequiresIssuesMd — §107 fail-loud: when
// docs/Issues.md is missing, wireDocsAndCommands MUST return an
// explicit error referencing the path. Silent degradation (e.g.
// nil IssueOpener → action=issue.open errors mid-flight) would be
// a §107 bluff because the operator would see "everything goes to
// CC" without knowing why issue.open never lands.
func TestWireDocsAndCommandsRequiresIssuesMd(t *testing.T) {
	dir := t.TempDir()
	cfg := listenConfig{DocsDir: dir}
	err := wireDocsAndCommands(&cfg)
	if err == nil {
		t.Fatal("wireDocsAndCommands accepted missing Issues.md — §107 fail-loud bypass")
	}
	if !strings.Contains(err.Error(), "Issues.md") {
		t.Fatalf("wireDocsAndCommands err does not reference Issues.md path: %v", err)
	}
}

// TestWireDocsAndCommandsHappyPath — when docs/Issues.md exists,
// wireDocsAndCommands populates cfg.IssueOpener + cfg.Commands +
// leaves cfg.EventEmitter nil (Wave 6.5 ships without runner-attach).
// Optional siblings (Status.md / CONTINUATION.md / Help.md) absent
// emit stderr warnings but do not block.
func TestWireDocsAndCommandsHappyPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Issues.md"), []byte("# Issues\n## Open\n| ID | x |\n|---|---|\n| HRD-001 | x |\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Fixed.md"), []byte("# Fixed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := listenConfig{DocsDir: dir, OperatorIDs: map[string]bool{"123": true}}
	if err := wireDocsAndCommands(&cfg); err != nil {
		t.Fatalf("wireDocsAndCommands: %v", err)
	}
	if cfg.IssueOpener == nil {
		t.Fatal("cfg.IssueOpener nil after happy-path wire")
	}
	if cfg.Commands == nil {
		t.Fatal("cfg.Commands nil after happy-path wire")
	}
	if cfg.Commands.IssuesPath == "" {
		t.Fatal("cfg.Commands.IssuesPath empty")
	}
	if !cfg.Commands.OperatorIDs["123"] {
		t.Fatal("cfg.Commands.OperatorIDs not threaded from cfg")
	}
	if cfg.EventEmitter != nil {
		t.Fatal("cfg.EventEmitter MUST be nil in Wave 6.5 (no runner-attach)")
	}
}
