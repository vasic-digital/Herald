package inbound_test

import (
	"context"
	"strings"
	"testing"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/pherald/internal/inbound"
)

// recordingReplier captures the SendReply call so the test can assert
// the action was routed here (and the reply_to_message_id arrived intact).
type recordingReplier struct {
	called      bool
	lastChatID  int64
	lastReplyTo int
	lastText    string
}

func (r *recordingReplier) SendReply(_ context.Context, chatID int64, text string, replyToID int, _ []commons.Attachment) (int, error) {
	r.called = true
	r.lastChatID = chatID
	r.lastReplyTo = replyToID
	r.lastText = text
	return 1, nil
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
				TgramReply:  rr,
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
		TgramReply:  rr,
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
		TgramReply:  rr,
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
		TgramReply:  rr,
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
		TgramReply:  rr,
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
		TgramReply:  rr,
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
		t.Fatal("want error for missing Code+TgramReply")
	}
	if _, err := inbound.NewDispatcher(inbound.Config{Code: stubCode{}}); err == nil {
		t.Fatal("want error for missing TgramReply")
	}
	if _, err := inbound.NewDispatcher(inbound.Config{TgramReply: &recordingReplier{}}); err == nil {
		t.Fatal("want error for missing Code")
	}
}
