// Package inbound — Dispatcher implements commons.InboundHandler per Wave 6
// (spec §32 inbound pipeline). On every InboundEvent:
//
//  1. Build a CodeRequest from the event (sender, channel, conversation,
//     attachments, user message, classification).
//  2. Call CodeDispatcher.Dispatch.
//  3. ParseReply on the returned stdout for an action declaration.
//  4. Route the action exclusively to one sink:
//     - "reply"      → TgramReplier.SendReply (default action; operator-locked)
//     - "issue.open" → IssueOpener.OpenIssue
//     - "event.emit" → EventEmitter.Emit
//     - unknown      → explicit error (no silent fallback)
//
// §107 anchor: action routing is mutually exclusive — the
// TestDispatcherActionRouting matrix asserts each action wakes exactly its
// own recording sink, never the others. ParseReply errors surface to the
// caller — we do not fabricate a default reply when the marker is missing.
package inbound

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"

	"github.com/vasic-digital/herald/commons"
)

// CodeDispatcher abstracts the Claude Code LLM dispatcher so the inbound
// runtime can be unit-tested against a stub. The production binding lives
// in cc_adapter.go (wraps *claude_code.Dispatcher).
type CodeDispatcher interface {
	Dispatch(ctx context.Context, req CodeRequest) (CodeResponse, error)
}

// CodeRequest mirrors claude_code.DispatchRequest at the inbound package's
// abstraction boundary. cc_adapter.go does the field-by-field translation
// to/from the concrete claude_code types.
type CodeRequest struct {
	InboundID      string
	Sender         string
	Channel        commons.ChannelID
	Conversation   string
	Attachments    []commons.Attachment
	UserMessage    string
	Classification Classification
}

// CodeResponse carries the raw stdout from the CC invocation so that
// ParseReply (which scans for <<<HERALD-REPLY>>>) sees the bytes Claude
// emitted (or a synthetic projection of them via cc_adapter.go).
type CodeResponse struct {
	Stdout []byte
}

// Classification mirrors claude_code.Classification.
type Classification struct {
	Type        string
	Criticality string
	Confidence  float64
}

// TgramReplier sends a reply that quotes the original message via Telegram's
// reply_to_message_id semantics. T8 (Wave 6) lands the production
// implementation on tgram.Adapter.
type TgramReplier interface {
	SendReply(ctx context.Context, chatID int64, text string, replyToID int, attachments []commons.Attachment) (int, error)
}

// IssueOpener creates a workable-item (HRD-NNN per V3 §8.3) from the
// LLM's action=issue.open trigger. Wave 6 stubs the implementation;
// HRD-NNN-W6b wires it end-to-end.
type IssueOpener interface {
	OpenIssue(ctx context.Context, p IssuePayload) error
}

// EventEmitter re-enters pherald's outbound runner pipeline from the
// LLM's action=event.emit trigger.
type EventEmitter interface {
	Emit(ctx context.Context, p EventPayload) error
}

// Config is NewDispatcher's input. ProjectName surfaces here for
// observability/logging; CC session resolution happens inside the
// CodeDispatcher implementation.
type Config struct {
	ProjectName string
	Code        CodeDispatcher
	TgramReply  TgramReplier
	Issues      IssueOpener
	Events      EventEmitter
	Fake        bool // listen_test.go opt-in; production callers leave false
}

// Dispatcher is pherald's production InboundHandler.
type Dispatcher struct {
	projectName string
	code        CodeDispatcher
	reply       TgramReplier
	opener      IssueOpener
	emit        EventEmitter
}

// NewDispatcher validates cfg and returns a ready Dispatcher. cfg.Code and
// cfg.TgramReply MUST be non-nil — cfg.Issues / cfg.Events may be nil iff
// the corresponding action triggers are never emitted by the LLM (the
// switch returns an explicit error in that case rather than silently
// dropping the trigger).
func NewDispatcher(cfg Config) (*Dispatcher, error) {
	if cfg.Code == nil {
		return nil, errors.New("inbound.NewDispatcher: cfg.Code is required")
	}
	if cfg.TgramReply == nil {
		return nil, errors.New("inbound.NewDispatcher: cfg.TgramReply is required")
	}
	return &Dispatcher{
		projectName: cfg.ProjectName,
		code:        cfg.Code,
		reply:       cfg.TgramReply,
		opener:      cfg.Issues,
		emit:        cfg.Events,
	}, nil
}

// Handle implements commons.InboundHandler.
//
// §107 anchor: every sink invocation is logged with a stable prefix
// ("inbound dispatched: <action>") so listen_test.go can confirm the
// production handler was wired (not silently swallowed).
func (d *Dispatcher) Handle(ctx context.Context, ev commons.InboundEvent) error {
	req := CodeRequest{
		InboundID:    ev.EventID,
		Sender:       fmt.Sprintf("%s:%s", ev.Sender.Channel, ev.Sender.ChannelUserID),
		Channel:      commons.ChannelID(ev.Sender.Channel),
		Conversation: ev.Body.Plain,
		Attachments:  ev.Attachments,
		UserMessage:  ev.Body.Plain,
		// Classification is left empty for Wave 6; §32.6 classifier is HRD-NNN-W6c.
	}
	resp, err := d.code.Dispatch(ctx, req)
	if err != nil {
		return fmt.Errorf("inbound: CC dispatch: %w", err)
	}
	reply, err := ParseReply(resp.Stdout)
	if err != nil {
		return fmt.Errorf("inbound: parse reply: %w", err)
	}

	switch reply.Action {
	case "reply":
		chatID, _ := strconv.ParseInt(ev.Sender.ChannelUserID, 10, 64)
		replyToID, _ := extractReplyToMessageID(ev.Raw)
		if _, err := d.reply.SendReply(ctx, chatID, reply.Text, replyToID, nil); err != nil {
			return fmt.Errorf("inbound: send reply: %w", err)
		}
		log.Printf("inbound dispatched: reply (event=%s chatID=%d replyTo=%d)", ev.EventID, chatID, replyToID)
		return nil

	case "issue.open":
		if d.opener == nil {
			return errors.New("inbound: action=issue.open but no IssueOpener configured")
		}
		if reply.Issue == nil {
			return errors.New("inbound: action=issue.open but reply.Issue is nil")
		}
		if err := d.opener.OpenIssue(ctx, *reply.Issue); err != nil {
			return fmt.Errorf("inbound: open issue: %w", err)
		}
		log.Printf("inbound dispatched: issue.open (event=%s title=%q)", ev.EventID, reply.Issue.Title)
		return nil

	case "event.emit":
		if d.emit == nil {
			return errors.New("inbound: action=event.emit but no EventEmitter configured")
		}
		if reply.Event == nil {
			return errors.New("inbound: action=event.emit but reply.Event is nil")
		}
		if err := d.emit.Emit(ctx, *reply.Event); err != nil {
			return fmt.Errorf("inbound: emit event: %w", err)
		}
		log.Printf("inbound dispatched: event.emit (event=%s type=%s)", ev.EventID, reply.Event.CloudEventType)
		return nil

	default:
		return fmt.Errorf("inbound: unknown action %q", reply.Action)
	}
}

// extractReplyToMessageID reads ev.Raw["message_id"] — populated by
// tgram.Subscribe's OnText/OnPhoto/OnDocument/OnVoice handlers per Wave 6
// T4+T5. Telegram's message_id arrives as JSON number from getUpdates
// (float64 after json.Unmarshal into map[string]any), but Subscribe
// constructs InboundEvent in-process with a plain int (msg.ID, telebot's
// Message.ID field) — so we accept both. int64 is also accepted for safety.
//
// Returns (0, error) on missing key or unsupported type — callers
// continue with replyToID=0 (no reply_to_message_id field on the wire),
// which Telegram clients render as a fresh message.
func extractReplyToMessageID(raw map[string]any) (int, error) {
	if raw == nil {
		return 0, errors.New("inbound: Raw map is nil")
	}
	v, ok := raw["message_id"]
	if !ok {
		return 0, errors.New("inbound: Raw has no message_id")
	}
	switch x := v.(type) {
	case int:
		return x, nil
	case int64:
		return int(x), nil
	case int32:
		return int(x), nil
	case float64:
		return int(x), nil
	case float32:
		return int(x), nil
	default:
		return 0, fmt.Errorf("inbound: Raw.message_id unsupported type %T", v)
	}
}
