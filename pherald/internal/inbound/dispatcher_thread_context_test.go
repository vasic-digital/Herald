package inbound_test

import (
	"context"
	"testing"
	"time"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/pherald/internal/inbound"
)

// recordingStubCode captures the CodeRequest the dispatcher built so the test
// can assert ev.ThreadContext was threaded through to the CC dispatch.
type recordingStubCode struct {
	stdout  string
	lastReq inbound.CodeRequest
	called  bool
}

func (s *recordingStubCode) Dispatch(_ context.Context, req inbound.CodeRequest) (inbound.CodeResponse, error) {
	s.called = true
	s.lastReq = req
	return inbound.CodeResponse{Stdout: []byte(s.stdout)}, nil
}

// TestDispatcherThreadsThreadContextToCC proves that an InboundEvent carrying
// ThreadContext results in a CodeRequest whose ThreadContext carries those
// prior messages verbatim — i.e. the thread context reaches the Claude Code
// dispatch (operator mandate 2026-06-02). The message is plain conversational
// text so it falls through Tier 1 to the CC dispatch path.
func TestDispatcherThreadsThreadContextToCC(t *testing.T) {
	rr := &recordingReplier{}
	cc := &recordingStubCode{stdout: `<<<HERALD-REPLY>>> {"action":"reply","text":"ack"}`}
	d, err := inbound.NewDispatcher(inbound.Config{
		ProjectName: "T",
		Code:        cc,
		Reply:       rr,
	})
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}

	tc := []commons.ThreadMessage{
		{SenderHandle: "@bob", SenderIsBot: false, Text: "the pipe is dropping events", Timestamp: time.Unix(1, 0)},
		{SenderHandle: "Claude", SenderIsBot: true, Text: "opened ATM-42", Timestamp: time.Unix(2, 0)},
	}
	ev := commons.InboundEvent{
		EventID:       "01HTHREAD",
		Sender:        commons.Recipient{Channel: "tgram", ChannelUserID: "12345"},
		Body:          commons.Body{Plain: "so what's the plan now?"},
		Raw:           map[string]any{"message_id": 42},
		ThreadContext: tc,
	}
	if err := d.Handle(context.Background(), ev); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !cc.called {
		t.Fatalf("CC dispatch was not invoked — plain query should reach Tier 2")
	}
	got := cc.lastReq.ThreadContext
	if len(got) != len(tc) {
		t.Fatalf("CodeRequest.ThreadContext len = %d, want %d", len(got), len(tc))
	}
	for i := range tc {
		if got[i].SenderHandle != tc[i].SenderHandle ||
			got[i].SenderIsBot != tc[i].SenderIsBot ||
			got[i].Text != tc[i].Text {
			t.Errorf("ThreadContext[%d] = %+v, want %+v", i, got[i], tc[i])
		}
	}
}

// TestDispatcherEmptyThreadContextThreadsEmpty proves the no-context case:
// an event without ThreadContext yields a CodeRequest with an empty
// ThreadContext (purely additive — nothing fabricated).
func TestDispatcherEmptyThreadContextThreadsEmpty(t *testing.T) {
	rr := &recordingReplier{}
	cc := &recordingStubCode{stdout: `<<<HERALD-REPLY>>> {"action":"reply","text":"ack"}`}
	d, err := inbound.NewDispatcher(inbound.Config{ProjectName: "T", Code: cc, Reply: rr})
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	ev := commons.InboundEvent{
		EventID: "01HNOTHREAD",
		Sender:  commons.Recipient{Channel: "tgram", ChannelUserID: "12345"},
		Body:    commons.Body{Plain: "any update?"},
		Raw:     map[string]any{"message_id": 7},
	}
	if err := d.Handle(context.Background(), ev); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(cc.lastReq.ThreadContext) != 0 {
		t.Fatalf("expected empty ThreadContext, got %d entries", len(cc.lastReq.ThreadContext))
	}
}
