// Package inbound — Dispatcher implements commons.InboundHandler per Wave 6
// (spec §32 inbound pipeline). On every InboundEvent:
//
//  1. Build a CodeRequest from the event (sender, channel, conversation,
//     attachments, user message, classification).
//  2. Call CodeDispatcher.Dispatch.
//  3. ParseReply on the returned stdout for an action declaration.
//  4. Route the action exclusively to one sink:
//     - "reply"      → Replier.SendReply (default action; operator-locked)
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

// Replier sends a reply that quotes the original message. Wave 7 (HRD-114)
// generic signature: recipient + string ids let any channel (Telegram,
// Slack, …) satisfy it without the dispatcher knowing the channel-native
// id types. The production wiring in pherald/cmd/pherald/listen.go binds a
// channelRouter that dispatches each reply to the adapter for
// recipient.Channel (each adapter's *tgram.Adapter.SendReplyGeneric /
// *slack.Adapter.SendReplyGeneric matches this exact shape via a thin
// per-channel wrapper).
//
// Wave 6 history: this interface was named TgramReplier with a Telegram-
// native int64 chatID / int replyToID signature; T5 widened it to the
// generic recipient/string form so the dispatcher is channel-agnostic.
// The method keeps the name SendReply (the inbound package's own name,
// independent of channels.Channel.SendReplyGeneric) — see the Wave 7 plan
// Step 4 method-name resolution.
type Replier interface {
	SendReply(ctx context.Context, recipient commons.Recipient, body string, replyToID string, attachments []commons.Attachment) (string, error)
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
//
// Commands (Wave 6.5 T7) wires the §32.6 fast-path command handlers
// (Help / Status / Continue / Done / Reopen). When non-nil, the
// Dispatcher's Handle method invokes them BEFORE CC dispatch for the
// matching classification.Type values — saving the LLM round-trip for
// deterministic-prefix messages. When nil, every inbound message goes
// to CC regardless of classification (Wave 6 behaviour preserved).
type Config struct {
	ProjectName string
	Code        CodeDispatcher
	Reply       Replier
	Issues      IssueOpener
	Events      EventEmitter
	Commands    *CommandsConfig
	Fake        bool // listen_test.go opt-in; production callers leave false
}

// Dispatcher is pherald's production InboundHandler.
type Dispatcher struct {
	projectName string
	code        CodeDispatcher
	reply       Replier
	opener      IssueOpener
	emit        EventEmitter
	commands    *CommandsConfig
}

// NewDispatcher validates cfg and returns a ready Dispatcher. cfg.Code and
// cfg.Reply MUST be non-nil — cfg.Issues / cfg.Events may be nil iff
// the corresponding action triggers are never emitted by the LLM (the
// switch returns an explicit error in that case rather than silently
// dropping the trigger). cfg.Commands may be nil — when nil, every
// inbound message goes to CC regardless of classification (Wave 6
// behaviour preserved).
func NewDispatcher(cfg Config) (*Dispatcher, error) {
	if cfg.Code == nil {
		return nil, errors.New("inbound.NewDispatcher: cfg.Code is required")
	}
	if cfg.Reply == nil {
		return nil, errors.New("inbound.NewDispatcher: cfg.Reply is required")
	}
	return &Dispatcher{
		projectName: cfg.ProjectName,
		code:        cfg.Code,
		reply:       cfg.Reply,
		opener:      cfg.Issues,
		emit:        cfg.Events,
		commands:    cfg.Commands,
	}, nil
}

// Handle implements commons.InboundHandler.
//
// Wave 6.5 T7: classifier runs unconditionally on ev.Body.Plain so the
// journal always records the §32.6 classification — even when the
// dispatch path falls through to CC (because the classifier hit a free-
// form `query` / `bug` / `task` / `investigation` / `override` type).
// For the fast-path command types (help_command / status_request /
// continuation_request / closure / reopen), the corresponding handler
// from d.commands is invoked BEFORE CC dispatch — saving the LLM
// round-trip for deterministic-prefix messages. If d.commands is nil,
// every inbound message goes to CC regardless of classification (Wave 6
// behaviour preserved).
//
// §107 anchor: every sink invocation is logged with a stable prefix
// ("inbound dispatched: <action>") so listen_test.go can confirm the
// production handler was wired (not silently swallowed).
func (d *Dispatcher) Handle(ctx context.Context, ev commons.InboundEvent) error {
	classification := Classify(ev.Body.Plain)

	// Fast-path: §32.6 command-prefix types are handled locally without a
	// CC round-trip. d.commands must be wired (T7 listen.go) for this to
	// fire — when nil, the classifier still ran (journal sees it) but the
	// dispatcher falls through to CC.
	if d.commands != nil {
		if handled, err := d.fastPath(ctx, ev, classification); handled {
			return err
		}
	}

	req := CodeRequest{
		InboundID:      ev.EventID,
		Sender:         fmt.Sprintf("%s:%s", ev.Sender.Channel, ev.Sender.ChannelUserID),
		Channel:        commons.ChannelID(ev.Sender.Channel),
		Conversation:   ev.Body.Plain,
		Attachments:    ev.Attachments,
		UserMessage:    ev.Body.Plain,
		Classification: classification,
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
		replyToID, _ := extractReplyToMessageID(ev.Raw)
		rt := ""
		if replyToID > 0 {
			rt = strconv.Itoa(replyToID)
		}
		rcpt := commons.Recipient{Channel: ev.Sender.Channel, ChannelUserID: ev.Sender.ChannelUserID}
		if _, err := d.reply.SendReply(ctx, rcpt, reply.Text, rt, nil); err != nil {
			return fmt.Errorf("inbound: send reply: %w", err)
		}
		log.Printf("inbound dispatched: reply (event=%s channel=%s user=%s replyTo=%s)", ev.EventID, ev.Sender.Channel, ev.Sender.ChannelUserID, rt)
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

// fastPath inspects classification.Type and, for the §32.6 command-prefix
// types (help_command / status_request / continuation_request / closure /
// reopen), invokes the matching handler on d.commands and sends the
// resulting reply directly via d.reply.SendReply — bypassing CC entirely.
//
// Returns (handled=true, err) when the type matched a fast-path handler
// — caller MUST return err verbatim (success or failure both terminate
// Handle). Returns (handled=false, nil) when the type is NOT a fast-path
// type (the caller proceeds with CC dispatch).
//
// §107 anchor: each branch logs "inbound dispatched: <command>" before
// returning so listen_test + journal can confirm the command path ran
// (not silently swallowed). A handler that returns an error still
// terminates Handle — we do NOT fall through to CC on fast-path failure
// (the operator should see the explicit error, not a fabricated CC
// retry).
func (d *Dispatcher) fastPath(ctx context.Context, ev commons.InboundEvent, c Classification) (bool, error) {
	var (
		replyText string
		atts      []commons.Attachment
		err       error
		label     string
	)
	switch c.Type {
	case "help_command":
		label = "help"
		replyText, atts, err = d.commands.HandleHelp(ctx)
	case "status_request":
		label = "status"
		replyText, atts, err = d.commands.HandleStatus(ctx)
	case "continuation_request":
		label = "continue"
		replyText, atts, err = d.commands.HandleContinue(ctx)
	case "closure":
		label = "done"
		replyText, atts, err = d.commands.HandleDone(ctx, ev.Body.Plain, ev.Sender.ChannelUserID)
	case "reopen":
		label = "reopen"
		replyText, atts, err = d.commands.HandleReopen(ctx, ev.Body.Plain, ev.Sender.ChannelUserID)
	default:
		return false, nil
	}
	rcpt := commons.Recipient{Channel: ev.Sender.Channel, ChannelUserID: ev.Sender.ChannelUserID}
	replyToID, _ := extractReplyToMessageID(ev.Raw)
	rt := ""
	if replyToID > 0 {
		rt = strconv.Itoa(replyToID)
	}
	if err != nil {
		// Fast-path handler errored (e.g. non-operator rejection on
		// Done:/Reopen:). Surface the message to the operator via the
		// same reply channel so they see WHY the command failed —
		// silent failure here would be a §107 PASS-bluff.
		errText := fmt.Sprintf("%s: %s", label, err.Error())
		if _, sendErr := d.reply.SendReply(ctx, rcpt, errText, rt, nil); sendErr != nil {
			return true, fmt.Errorf("inbound: fast-path %s send err-reply: %w (original: %v)", label, sendErr, err)
		}
		log.Printf("inbound dispatched: %s (event=%s err=%v)", label, ev.EventID, err)
		return true, nil
	}
	if _, err := d.reply.SendReply(ctx, rcpt, replyText, rt, atts); err != nil {
		return true, fmt.Errorf("inbound: fast-path %s send reply: %w", label, err)
	}
	log.Printf("inbound dispatched: %s (event=%s channel=%s user=%s replyTo=%s)", label, ev.EventID, ev.Sender.Channel, ev.Sender.ChannelUserID, rt)
	return true, nil
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
