package inbound_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/pherald/internal/inbound"
)

// recordingReplier captures the SendReply call so the test can assert
// the action was routed here (and the reply_to_message_id arrived intact).
//
// Wave 7 (HRD-114): the generic inbound.Replier signature carries a
// commons.Recipient + string replyToID. lastChatID / lastReplyTo are the
// decoded numeric forms (the dispatcher converts the Telegram chat_id /
// message_id to/from strings) so the existing numeric assertions stay
// meaningful.
type recordingReplier struct {
	called      bool
	lastChatID  int64
	lastReplyTo int
	lastText    string
}

func (r *recordingReplier) SendReply(_ context.Context, recipient commons.Recipient, body, replyToID string, _ []commons.Attachment) (string, error) {
	r.called = true
	if recipient.ChannelUserID != "" {
		chatID, _ := strconv.ParseInt(recipient.ChannelUserID, 10, 64)
		r.lastChatID = chatID
	}
	if replyToID != "" {
		rt, _ := strconv.Atoi(replyToID)
		r.lastReplyTo = rt
	} else {
		r.lastReplyTo = 0
	}
	r.lastText = body
	return "1", nil
}

type recordingOpener struct {
	called    bool
	lastTitle string
}

func (r *recordingOpener) OpenIssue(_ context.Context, p inbound.IssuePayload) error {
	r.called = true
	r.lastTitle = p.Title
	return nil
}

type recordingEmitter struct {
	called   bool
	lastType string
}

func (r *recordingEmitter) Emit(_ context.Context, p inbound.EventPayload) error {
	r.called = true
	r.lastType = p.CloudEventType
	return nil
}

// stubCode returns canned stdout for the action under test, so the
// Dispatcher's switch routes deterministically.
type stubCode struct {
	stdout string
	err    error
}

func (s stubCode) Dispatch(_ context.Context, _ inbound.CodeRequest) (inbound.CodeResponse, error) {
	if s.err != nil {
		return inbound.CodeResponse{}, s.err
	}
	return inbound.CodeResponse{Stdout: []byte(s.stdout)}, nil
}

// countingStubCode is the Wave 6.5 T7 fast-path test seam: tracks the
// number of Dispatch invocations so the fast-path assertion ("Help:
// bypasses CC") can verify calls==0, and the non-fast-path assertion
// ("plain text reaches CC") can verify calls==1.
type countingStubCode struct {
	calls  int32
	stdout string
}

func (s *countingStubCode) Dispatch(_ context.Context, _ inbound.CodeRequest) (inbound.CodeResponse, error) {
	atomic.AddInt32(&s.calls, 1)
	return inbound.CodeResponse{Stdout: []byte(s.stdout)}, nil
}

func (s *countingStubCode) Calls() int32 { return atomic.LoadInt32(&s.calls) }

func TestDispatcherActionRouting(t *testing.T) {
	cases := []struct {
		name      string
		stdout    string
		wantReply bool
		wantIssue bool
		wantEvent bool
	}{
		{
			name:      "reply",
			stdout:    `<<<HERALD-REPLY>>> {"action":"reply","text":"hi"}`,
			wantReply: true,
		},
		{
			name:      "reply default (action omitted)",
			stdout:    `<<<HERALD-REPLY>>> {"text":"hi"}`,
			wantReply: true,
		},
		{
			name:      "issue.open",
			stdout:    `<<<HERALD-REPLY>>> {"action":"issue.open","issue":{"type":"bug","criticality":"high","title":"X","body":"Y","labels":["repro"]}}`,
			wantIssue: true,
		},
		{
			name:      "event.emit",
			stdout:    `<<<HERALD-REPLY>>> {"action":"event.emit","event":{"cloudevent_type":"c","subject":"s","data":{"k":"v"}}}`,
			wantEvent: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := &recordingReplier{}
			ro := &recordingOpener{}
			re := &recordingEmitter{}
			d, err := inbound.NewDispatcher(inbound.Config{
				ProjectName: "T",
				Code:        stubCode{stdout: tc.stdout},
				Reply:       rr,
				Issues:      ro,
				Events:      re,
			})
			if err != nil {
				t.Fatalf("NewDispatcher: %v", err)
			}
			ev := commons.InboundEvent{
				EventID: "01H",
				Sender: commons.Recipient{
					Channel:       "tgram",
					ChannelUserID: "12345",
				},
				Body: commons.Body{Plain: "ping"},
				Raw:  map[string]any{"message_id": 42},
			}
			if err := d.Handle(context.Background(), ev); err != nil {
				t.Fatalf("Handle: %v", err)
			}
			if rr.called != tc.wantReply {
				t.Fatalf("reply called: got %v want %v", rr.called, tc.wantReply)
			}
			if ro.called != tc.wantIssue {
				t.Fatalf("issue called: got %v want %v", ro.called, tc.wantIssue)
			}
			if re.called != tc.wantEvent {
				t.Fatalf("event called: got %v want %v", re.called, tc.wantEvent)
			}
			// §107: mutual exclusion — at most one sink fires per event.
			fired := 0
			if rr.called {
				fired++
			}
			if ro.called {
				fired++
			}
			if re.called {
				fired++
			}
			if fired != 1 {
				t.Fatalf("expected exactly one sink fired; got %d (reply=%v issue=%v event=%v)",
					fired, rr.called, ro.called, re.called)
			}
		})
	}
}

func TestDispatcherReplyPassesMessageID(t *testing.T) {
	rr := &recordingReplier{}
	d, err := inbound.NewDispatcher(inbound.Config{
		ProjectName: "T",
		Code:        stubCode{stdout: `<<<HERALD-REPLY>>> {"action":"reply","text":"pong"}`},
		Reply:       rr,
	})
	if err != nil {
		t.Fatal(err)
	}
	ev := commons.InboundEvent{
		EventID: "01H",
		Sender:  commons.Recipient{Channel: "tgram", ChannelUserID: "98765"},
		Body:    commons.Body{Plain: "ping"},
		Raw:     map[string]any{"message_id": 777},
	}
	if err := d.Handle(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if !rr.called {
		t.Fatal("replier not called")
	}
	if rr.lastChatID != 98765 {
		t.Fatalf("chatID: got %d want 98765", rr.lastChatID)
	}
	if rr.lastReplyTo != 777 {
		t.Fatalf("replyToID: got %d want 777", rr.lastReplyTo)
	}
	if rr.lastText != "pong" {
		t.Fatalf("text: got %q want pong", rr.lastText)
	}
}

// Telegram getUpdates JSON-decodes numeric fields into float64 when
// unmarshalled into map[string]any. Confirm extractReplyToMessageID
// handles this realistic case (qaherald-flavored ingest path).
func TestDispatcherReplyAcceptsFloat64MessageID(t *testing.T) {
	rr := &recordingReplier{}
	d, _ := inbound.NewDispatcher(inbound.Config{
		ProjectName: "T",
		Code:        stubCode{stdout: `<<<HERALD-REPLY>>> {"action":"reply","text":"ok"}`},
		Reply:       rr,
	})
	ev := commons.InboundEvent{
		Sender: commons.Recipient{Channel: "tgram", ChannelUserID: "1"},
		Raw:    map[string]any{"message_id": float64(123)},
	}
	if err := d.Handle(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if rr.lastReplyTo != 123 {
		t.Fatalf("float64 message_id: got %d want 123", rr.lastReplyTo)
	}
}

func TestDispatcherUnknownActionReturnsError(t *testing.T) {
	rr := &recordingReplier{}
	d, _ := inbound.NewDispatcher(inbound.Config{
		ProjectName: "T",
		Code:        stubCode{stdout: `<<<HERALD-REPLY>>> {"action":"weird.thing"}`},
		Reply:       rr,
	})
	err := d.Handle(context.Background(), commons.InboundEvent{
		Sender: commons.Recipient{Channel: "tgram", ChannelUserID: "1"},
		Raw:    map[string]any{"message_id": 1},
	})
	if err == nil {
		t.Fatal("want error for unknown action, got nil")
	}
	if !strings.Contains(err.Error(), "unknown action") {
		t.Fatalf("error message: got %q want contains 'unknown action'", err.Error())
	}
	if rr.called {
		t.Fatal("§107: replier MUST NOT fire on unknown action (no silent fallback)")
	}
}

func TestDispatcherParseErrorSurfaces(t *testing.T) {
	rr := &recordingReplier{}
	d, _ := inbound.NewDispatcher(inbound.Config{
		ProjectName: "T",
		Code:        stubCode{stdout: `no marker here at all`},
		Reply:       rr,
	})
	err := d.Handle(context.Background(), commons.InboundEvent{
		Sender: commons.Recipient{Channel: "tgram", ChannelUserID: "1"},
		Raw:    map[string]any{"message_id": 1},
	})
	if err == nil {
		t.Fatal("want error for missing marker, got nil")
	}
	if rr.called {
		t.Fatal("§107: replier MUST NOT fire when CC stdout lacks <<<HERALD-REPLY>>> marker")
	}
}

func TestDispatcherIssueOpenMissingOpenerErrors(t *testing.T) {
	rr := &recordingReplier{}
	d, _ := inbound.NewDispatcher(inbound.Config{
		ProjectName: "T",
		Code:        stubCode{stdout: `<<<HERALD-REPLY>>> {"action":"issue.open","issue":{"title":"X"}}`},
		Reply:       rr,
		// Issues: nil intentionally
	})
	err := d.Handle(context.Background(), commons.InboundEvent{
		Sender: commons.Recipient{Channel: "tgram", ChannelUserID: "1"},
		Raw:    map[string]any{"message_id": 1},
	})
	if err == nil {
		t.Fatal("want error when IssueOpener nil, got nil")
	}
}

func TestNewDispatcherValidatesRequiredFields(t *testing.T) {
	if _, err := inbound.NewDispatcher(inbound.Config{}); err == nil {
		t.Fatal("want error for missing Code+Reply")
	}
	if _, err := inbound.NewDispatcher(inbound.Config{Code: stubCode{}}); err == nil {
		t.Fatal("want error for missing Reply")
	}
	if _, err := inbound.NewDispatcher(inbound.Config{Reply: &recordingReplier{}}); err == nil {
		t.Fatal("want error for missing Code")
	}
}

// TestDispatcherFastPathHelpBypassesCC — §107 anti-bluff anchor for
// Wave 6.5 T7. When a Help:-prefixed message arrives AND a non-nil
// Commands is wired, the Dispatcher MUST invoke HandleHelp + reply
// directly WITHOUT touching the CC dispatcher. The counting stub
// records every Dispatch call; the assertion is calls == 0.
//
// A regressed dispatcher that always falls through to CC would FAIL
// this test (calls == 1). A dispatcher that always uses fast-path
// (mis-classifies plain text as a command) would FAIL the companion
// TestDispatcherCCPathTakenForPlainQuery test (calls == 0 when 1 is
// expected). Both directions are pinned.
func TestDispatcherFastPathHelpBypassesCC(t *testing.T) {
	rr := &recordingReplier{}
	cc := &countingStubCode{stdout: `<<<HERALD-REPLY>>> {"action":"reply","text":"should-not-fire"}`}
	cmds := &inbound.CommandsConfig{
		HelpPath: "", // empty → HandleHelp returns BuiltinHelp
	}
	d, err := inbound.NewDispatcher(inbound.Config{
		ProjectName: "T",
		Code:        cc,
		Reply:       rr,
		Commands:    cmds,
	})
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	ev := commons.InboundEvent{
		EventID: "01HFASTHELP",
		Sender:  commons.Recipient{Channel: "tgram", ChannelUserID: "12345"},
		Body:    commons.Body{Plain: "Help:"},
		Raw:     map[string]any{"message_id": 42},
	}
	if err := d.Handle(context.Background(), ev); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got := cc.Calls(); got != 0 {
		t.Fatalf("§107 fast-path: CC dispatcher invoked %d times for Help: — fast-path bypass is broken", got)
	}
	if rr.Called() != 1 {
		t.Fatalf("§107 fast-path: replier called %d times for Help: — want exactly 1", rr.Called())
	}
	if !strings.Contains(rr.LastText(), "Command catalogue") {
		t.Fatalf("§107 fast-path: replier text does not match BuiltinHelp; got %q", rr.LastText())
	}
}

// TestDispatcherCCPathTakenForPlainQuery — §107 companion anchor.
// When a plain-text message arrives (classifier falls through to
// type=query, confidence=0.0), the Dispatcher MUST route to CC. The
// counting stub records calls; the assertion is calls == 1. A
// dispatcher that always uses fast-path would FAIL this test.
func TestDispatcherCCPathTakenForPlainQuery(t *testing.T) {
	rr := &recordingReplier{}
	cc := &countingStubCode{stdout: `<<<HERALD-REPLY>>> {"action":"reply","text":"from CC"}`}
	cmds := &inbound.CommandsConfig{
		HelpPath: "",
	}
	d, err := inbound.NewDispatcher(inbound.Config{
		ProjectName: "T",
		Code:        cc,
		Reply:       rr,
		Commands:    cmds,
	})
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	ev := commons.InboundEvent{
		EventID: "01HPLAINQ",
		Sender:  commons.Recipient{Channel: "tgram", ChannelUserID: "12345"},
		Body:    commons.Body{Plain: "hey what's up over there?"},
		Raw:     map[string]any{"message_id": 42},
	}
	if err := d.Handle(context.Background(), ev); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got := cc.Calls(); got != 1 {
		t.Fatalf("§107 CC-path: CC dispatcher invoked %d times for plain query — want exactly 1", got)
	}
	if rr.Called() != 1 {
		t.Fatalf("§107 CC-path: replier called %d times — want exactly 1 (reply from CC)", rr.Called())
	}
	if !strings.Contains(rr.LastText(), "from CC") {
		t.Fatalf("§107 CC-path: replier text not from CC stub; got %q", rr.LastText())
	}
}

// TestDispatcherFastPathStatusReadsFixture — confirms HandleStatus is
// actually invoked (not no-op'd) on a Status: prefix. Uses a t.TempDir
// fixture; the assertion asserts the sentinel content appears in the
// reply. A fast-path stub that always sent the same canned reply
// would FAIL this test.
func TestDispatcherFastPathStatusReadsFixture(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "Status.md")
	content := `# Status
| Field | Value |
|---|---|
| Status summary | sentinel-status-T7-fast-path |
`
	if err := os.WriteFile(statusPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	rr := &recordingReplier{}
	cc := &countingStubCode{stdout: `<<<HERALD-REPLY>>> {"action":"reply","text":"never"}`}
	cmds := &inbound.CommandsConfig{StatusPath: statusPath}
	d, err := inbound.NewDispatcher(inbound.Config{
		ProjectName: "T",
		Code:        cc,
		Reply:       rr,
		Commands:    cmds,
	})
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	ev := commons.InboundEvent{
		EventID: "01HFASTST",
		Sender:  commons.Recipient{Channel: "tgram", ChannelUserID: "12345"},
		Body:    commons.Body{Plain: "Status:"},
		Raw:     map[string]any{"message_id": 99},
	}
	if err := d.Handle(context.Background(), ev); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got := cc.Calls(); got != 0 {
		t.Fatalf("§107 fast-path Status: CC invoked %d times — bypass broken", got)
	}
	if !strings.Contains(rr.LastText(), "sentinel-status-T7-fast-path") {
		t.Fatalf("§107 fast-path Status: fixture sentinel missing; got %q", rr.LastText())
	}
}

// TestDispatcherFastPathDoneNonOperatorReplies — when a non-operator
// sender invokes Done:, the fast-path MUST reply with an explicit
// error message (operator sees WHY the command failed) and MUST NOT
// fall through to CC. The original error is logged but the operator
// gets a reply, not silence.
func TestDispatcherFastPathDoneNonOperatorReplies(t *testing.T) {
	rr := &recordingReplier{}
	cc := &countingStubCode{stdout: `<<<HERALD-REPLY>>> {"action":"reply","text":"never"}`}
	cmds := &inbound.CommandsConfig{
		IssuesPath:  filepath.Join(t.TempDir(), "Issues.md"),
		FixedPath:   filepath.Join(t.TempDir(), "Fixed.md"),
		OperatorIDs: map[string]bool{"99999": true}, // sender 12345 is NOT in this list
	}
	d, _ := inbound.NewDispatcher(inbound.Config{
		ProjectName: "T",
		Code:        cc,
		Reply:       rr,
		Commands:    cmds,
	})
	ev := commons.InboundEvent{
		EventID: "01HDONEDENY",
		Sender:  commons.Recipient{Channel: "tgram", ChannelUserID: "12345"},
		Body:    commons.Body{Plain: "Done: HRD-042"},
		Raw:     map[string]any{"message_id": 7},
	}
	if err := d.Handle(context.Background(), ev); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got := cc.Calls(); got != 0 {
		t.Fatalf("§107 fast-path Done deny: CC invoked %d times — fall-through wrong", got)
	}
	if rr.Called() != 1 {
		t.Fatalf("§107 fast-path Done deny: replier called %d times — operator MUST see error reply", rr.Called())
	}
	if !strings.Contains(rr.LastText(), "rejected") && !strings.Contains(rr.LastText(), "HERALD_OPERATOR_IDS") {
		t.Fatalf("§107 fast-path Done deny: reply text does not surface the rejection reason; got %q", rr.LastText())
	}
}

// TestDispatcherNilCommandsFallsThrough — when Commands is nil
// (Wave 6 deployment without T7 wiring, OR hermetic tests that never
// exercise the fast-path), a Help:-prefixed message MUST go to CC
// because there's no handler to short-circuit it. This protects
// against accidental regressions where the fast-path branch ignores
// the nil check.
func TestDispatcherNilCommandsFallsThrough(t *testing.T) {
	rr := &recordingReplier{}
	cc := &countingStubCode{stdout: `<<<HERALD-REPLY>>> {"action":"reply","text":"cc-reply"}`}
	d, _ := inbound.NewDispatcher(inbound.Config{
		ProjectName: "T",
		Code:        cc,
		Reply:       rr,
		Commands:    nil, // no T7 wiring
	})
	ev := commons.InboundEvent{
		EventID: "01HFALLBACK",
		Sender:  commons.Recipient{Channel: "tgram", ChannelUserID: "12345"},
		Body:    commons.Body{Plain: "Help:"},
		Raw:     map[string]any{"message_id": 42},
	}
	if err := d.Handle(context.Background(), ev); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if cc.Calls() != 1 {
		t.Fatalf("nil Commands: CC MUST be invoked when no fast-path wired; got %d", cc.Calls())
	}
}

// Augment recordingReplier with the Called / LastText accessors so
// the new fast-path tests can poll without touching the unexported
// fields directly. (The simpler counterpart is to read the fields
// inline; the accessors mirror listen_test.go's existing helpers.)
func (r *recordingReplier) Called() int {
	if r.called {
		return 1
	}
	return 0
}

func (r *recordingReplier) LastText() string { return r.lastText }
