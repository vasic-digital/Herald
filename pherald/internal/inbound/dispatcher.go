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
	"strings"

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

	// ThreadContext carries the prior messages of the thread this event
	// belongs to (oldest→newest, excludes the current message), copied
	// verbatim from commons.InboundEvent.ThreadContext in Handle. cc_adapter.go
	// forwards it to claude_code.DispatchRequest so the LLM's reply is bound
	// by the thread's meaning (operator mandate 2026-06-02).
	ThreadContext []commons.ThreadMessage
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

	// Items is the workable-item CRUD boundary (WS-4 / HRD-152). When
	// non-nil it backs the item.update / item.delete actions and the
	// confirmed-investigation execution path. When nil, those actions
	// return an explicit error (no silent drop).
	Items ItemMutator

	// ConfirmToken mints the confirmation token for a deferred
	// investigation proposal. Injectable so tests are deterministic;
	// production leaves it nil and a UUIDv7-derived default is used.
	// The argument is the inbound event id (stable per message).
	ConfirmToken func(eventID string) string

	// Resolver is the participant identity resolver (PARTICIPANT_ATTRIBUTION
	// §1/§2/§5). When non-nil, item.update injects created_by (from the
	// message sender via ResolveSender) + assigned_to (OperatorHandle by
	// default, or an explicit `assign:@x` directive) into the mutated item.
	// When nil, attribution is skipped (Wave 6 behaviour preserved) — the
	// fields are simply not touched.
	Resolver commons.IdentityResolver

	Fake bool // listen_test.go opt-in; production callers leave false
}

// Dispatcher is pherald's production InboundHandler.
type Dispatcher struct {
	projectName string
	code        CodeDispatcher
	reply       Replier
	opener      IssueOpener
	emit        EventEmitter
	commands    *CommandsConfig
	items       ItemMutator
	confirmTok  func(string) string
	resolver    commons.IdentityResolver
	pending     *pendingStore

	// recognizer is the TIER 1 deterministic command matcher
	// (docs/design/INTENT_RECOGNITION.md §1/§2). Handle tries it BEFORE the
	// LLM dispatch: on a confident match the action is built directly (no LLM
	// round-trip); otherwise the existing Claude Code dispatch (Tier 2) runs,
	// which may itself return action=clarify (Tier 3). Always non-nil — set
	// in NewDispatcher.
	recognizer *CommandRecognizer

	// actions is the action→handler registry (WS-4 / HRD-152). The
	// Wave 6 switch is refactored into this map so new actions are
	// added by registration, not by growing a switch. Each handler
	// receives the dispatch context — the routed Reply plus the
	// originating event — and is responsible for its own sink call +
	// §107 log line.
	actions map[string]actionHandler
}

// actionHandler routes one parsed Reply action to its sink. It returns
// an error verbatim to Handle (success or failure both terminate the
// dispatch). dctx bundles the event + reply so handlers share one
// signature regardless of which fields they consume.
type actionHandler func(ctx context.Context, dctx dispatchCtx) error

// dispatchCtx is the per-message routing context passed to every
// actionHandler — the originating event, the parsed reply, and the
// pre-computed reply-to id (so handlers don't each re-extract it).
type dispatchCtx struct {
	ev        commons.InboundEvent
	reply     *Reply
	replyToID string
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
	confirmTok := cfg.ConfirmToken
	if confirmTok == nil {
		confirmTok = defaultConfirmToken
	}
	d := &Dispatcher{
		projectName: cfg.ProjectName,
		code:        cfg.Code,
		reply:       cfg.Reply,
		opener:      cfg.Issues,
		emit:        cfg.Events,
		commands:    cfg.Commands,
		items:       cfg.Items,
		confirmTok:  confirmTok,
		resolver:    cfg.Resolver,
		pending:     newPendingStore(),
		recognizer:  NewCommandRecognizer(),
	}
	d.actions = map[string]actionHandler{
		"reply":               d.actReply,
		"issue.open":          d.actIssueOpen,
		"event.emit":          d.actEventEmit,
		"item.update":         d.actItemUpdate,
		"item.delete":         d.actItemDelete,
		"investigation.start": d.actInvestigationStart,
		"clarify":             d.actClarify,
	}
	return d, nil
}

// defaultConfirmToken derives a confirmation token from the event id.
// Production callers leave Config.ConfirmToken nil; tests inject a
// deterministic stub. The event id is already a UUIDv7-style stable id,
// so a short prefix is sufficient to disambiguate concurrent proposals.
func defaultConfirmToken(eventID string) string {
	if eventID == "" {
		return "CONFIRM"
	}
	if len(eventID) > 8 {
		return eventID[:8]
	}
	return eventID
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
	// CONFIRM fast-path (WS-4 / HRD-152): "CONFIRM <token>" executes a
	// pending investigation-proposed mutation WITHOUT a CC round-trip.
	// Checked before classification so the confirm flow is deterministic
	// (a confirm message is never reinterpreted as a query).
	if tok, ok := parseConfirm(ev.Body.Plain); ok {
		return d.handleConfirm(ctx, ev, tok)
	}

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

	rt := extractReplyToID(ev.Raw)

	// TIER 1 (docs/design/INTENT_RECOGNITION.md §1/§2): the deterministic
	// CommandRecognizer is tried BEFORE the LLM dispatch. On a confident match
	// we build the action directly and route it — skipping the LLM round-trip.
	// A no-match falls through to the existing Claude Code dispatch (Tier 2),
	// which may itself return action=clarify (Tier 3).
	if action, fields, ok := d.recognizer.RecognizeCommand(ev.Body.Plain); ok {
		reply := buildRecognizedReply(action, fields)
		handler, known := d.actions[reply.Action]
		if !known {
			return fmt.Errorf("inbound: recognizer produced unknown action %q", reply.Action)
		}
		log.Printf("inbound recognized (tier1): action=%s (event=%s)", reply.Action, ev.EventID)
		return handler(ctx, dispatchCtx{ev: ev, reply: reply, replyToID: rt})
	}

	req := CodeRequest{
		InboundID:      ev.EventID,
		Sender:         fmt.Sprintf("%s:%s", ev.Sender.Channel, ev.Sender.ChannelUserID),
		Channel:        commons.ChannelID(ev.Sender.Channel),
		Conversation:   ev.Body.Plain,
		Attachments:    ev.Attachments,
		UserMessage:    ev.Body.Plain,
		Classification: classification,
		ThreadContext:  ev.ThreadContext,
	}
	resp, err := d.code.Dispatch(ctx, req)
	if err != nil {
		return fmt.Errorf("inbound: CC dispatch: %w", err)
	}
	reply, err := ParseReply(resp.Stdout)
	if err != nil {
		return fmt.Errorf("inbound: parse reply: %w", err)
	}

	handler, ok := d.actions[reply.Action]
	if !ok {
		return fmt.Errorf("inbound: unknown action %q", reply.Action)
	}
	return handler(ctx, dispatchCtx{ev: ev, reply: reply, replyToID: rt})
}

// recipientOf builds the reply recipient from the originating event.
func recipientOf(ev commons.InboundEvent) commons.Recipient {
	return commons.Recipient{Channel: ev.Sender.Channel, ChannelUserID: ev.Sender.ChannelUserID}
}

// actReply routes action=reply to the Replier. Default action; behaviour
// identical to the Wave 6 switch case.
func (d *Dispatcher) actReply(ctx context.Context, dc dispatchCtx) error {
	rcpt := recipientOf(dc.ev)
	// An empty reply text must NOT be sent (adapters reject an empty body) and
	// must NOT crash the inbound runtime: a single malformed/empty Claude reply
	// would otherwise bubble a fatal error up through Subscribe → fail-loud →
	// take down the entire multi-channel listener. Treat it as a no-op-with-log
	// (the message was processed; Claude simply produced no text to send back).
	if strings.TrimSpace(dc.reply.Text) == "" {
		log.Printf("inbound: reply skipped — empty reply text (event=%s channel=%s user=%s); message processed, nothing to send back",
			dc.ev.EventID, dc.ev.Sender.Channel, dc.ev.Sender.ChannelUserID)
		return nil
	}
	if _, err := d.reply.SendReply(ctx, rcpt, dc.reply.Text, dc.replyToID, nil); err != nil {
		return fmt.Errorf("inbound: send reply: %w", err)
	}
	log.Printf("inbound dispatched: reply (event=%s channel=%s user=%s replyTo=%s)", dc.ev.EventID, dc.ev.Sender.Channel, dc.ev.Sender.ChannelUserID, dc.replyToID)
	return nil
}

// actIssueOpen routes action=issue.open to the IssueOpener. Behaviour
// identical to the Wave 6 switch case.
func (d *Dispatcher) actIssueOpen(ctx context.Context, dc dispatchCtx) error {
	if d.opener == nil {
		return errors.New("inbound: action=issue.open but no IssueOpener configured")
	}
	if dc.reply.Issue == nil {
		return errors.New("inbound: action=issue.open but reply.Issue is nil")
	}
	if err := d.opener.OpenIssue(ctx, *dc.reply.Issue); err != nil {
		return fmt.Errorf("inbound: open issue: %w", err)
	}
	log.Printf("inbound dispatched: issue.open (event=%s title=%q)", dc.ev.EventID, dc.reply.Issue.Title)
	return nil
}

// actEventEmit routes action=event.emit to the EventEmitter. Behaviour
// identical to the Wave 6 switch case.
func (d *Dispatcher) actEventEmit(ctx context.Context, dc dispatchCtx) error {
	if d.emit == nil {
		return errors.New("inbound: action=event.emit but no EventEmitter configured")
	}
	if dc.reply.Event == nil {
		return errors.New("inbound: action=event.emit but reply.Event is nil")
	}
	if err := d.emit.Emit(ctx, *dc.reply.Event); err != nil {
		return fmt.Errorf("inbound: emit event: %w", err)
	}
	log.Printf("inbound dispatched: event.emit (event=%s type=%s)", dc.ev.EventID, dc.reply.Event.CloudEventType)
	return nil
}

// actItemUpdate routes action=item.update to the ItemMutator. A nil
// mutator or nil payload is an explicit error (no silent drop).
func (d *Dispatcher) actItemUpdate(ctx context.Context, dc dispatchCtx) error {
	if d.items == nil {
		return errors.New("inbound: action=item.update but no ItemMutator configured")
	}
	if dc.reply.ItemUpdate == nil {
		return errors.New("inbound: action=item.update but reply.ItemUpdate is nil")
	}
	p := dc.reply.ItemUpdate
	fields := d.withAttribution(dc.ev, p.Fields)
	if err := d.items.Update(ctx, p.AtmID, p.Location, fields); err != nil {
		return fmt.Errorf("inbound: item.update %s/%s: %w", p.AtmID, p.Location, err)
	}
	log.Printf("inbound dispatched: item.update (event=%s atm=%s location=%s fields=%d created_by=%q assigned_to=%q)", dc.ev.EventID, p.AtmID, p.Location, len(fields), fields[fieldCreatedBy], fields[fieldAssignedTo])
	return nil
}

// withAttribution returns a copy of the LLM-supplied item.update fields with
// created_by / assigned_to injected per PARTICIPANT_ATTRIBUTION §2/§5.
//
//   - When the resolver is nil, the fields pass through unchanged (Wave 6
//     behaviour preserved).
//   - When the LLM payload already carries created_by == "Claude", this is the
//     System/Claude-opened path (§2) — created_by is left as "Claude" and only
//     a missing assigned_to is defaulted.
//   - Otherwise it is a subscriber message routed through Herald: created_by is
//     set from the message sender via ResolveSender, and assigned_to defaults
//     to OperatorHandle() (overridable by an explicit `assign:@x` in the body).
//
// An explicit created_by / assigned_to already present in the LLM fields is
// NEVER overwritten — the LLM's deliberate attribution wins over the default.
func (d *Dispatcher) withAttribution(ev commons.InboundEvent, in map[string]string) map[string]string {
	if d.resolver == nil {
		return in
	}
	out := make(map[string]string, len(in)+2)
	for k, v := range in {
		out[k] = v
	}
	system := out[fieldCreatedBy] == commons.SystemAgentHandle
	createdBy, assignedTo := resolveAttribution(ev, d.resolver, system)
	if _, ok := out[fieldCreatedBy]; !ok && createdBy != "" {
		out[fieldCreatedBy] = createdBy
	}
	if _, ok := out[fieldAssignedTo]; !ok && assignedTo != "" {
		out[fieldAssignedTo] = assignedTo
	}
	return out
}

// actItemDelete routes action=item.delete to the ItemMutator.
func (d *Dispatcher) actItemDelete(ctx context.Context, dc dispatchCtx) error {
	if d.items == nil {
		return errors.New("inbound: action=item.delete but no ItemMutator configured")
	}
	if dc.reply.ItemDelete == nil {
		return errors.New("inbound: action=item.delete but reply.ItemDelete is nil")
	}
	p := dc.reply.ItemDelete
	if err := d.items.Delete(ctx, p.AtmID, p.Location); err != nil {
		return fmt.Errorf("inbound: item.delete %s/%s: %w", p.AtmID, p.Location, err)
	}
	log.Printf("inbound dispatched: item.delete (event=%s atm=%s location=%s)", dc.ev.EventID, p.AtmID, p.Location)
	return nil
}

// actInvestigationStart implements ACT-WITH-CONFIRMATION (operator
// decision 2026-05-29). The investigation report is returned to the
// requester. Any machine-executable proposed mutation is NOT executed
// immediately — it is recorded as pending under a (test-injectable)
// token and the reply carries a "Reply CONFIRM <token> to apply: …"
// prompt. The mutation runs only when a subsequent "CONFIRM <token>"
// message arrives (see handleConfirm).
//
// §107 anchor: the mutator is NOT called on this path — the
// TestInvestigationProposedMutationDeferred test asserts zero mutator
// calls here, and TestInvestigationConfirmExecutesPendingMutation
// asserts the call happens only after confirm.
func (d *Dispatcher) actInvestigationStart(ctx context.Context, dc dispatchCtx) error {
	if dc.reply.Investigation == nil {
		return errors.New("inbound: action=investigation.start but reply.Investigation is nil")
	}
	inv := dc.reply.Investigation
	rcpt := recipientOf(dc.ev)

	var report strings.Builder
	fmt.Fprintf(&report, "Investigation: %s\n", inv.Topic)
	for _, a := range inv.ProposedActions {
		fmt.Fprintf(&report, "  - %s\n", a)
	}

	if inv.ProposedAction != nil {
		if d.items == nil {
			return errors.New("inbound: investigation proposes a mutation but no ItemMutator configured")
		}
		token := d.confirmTok(dc.ev.EventID)
		d.pending.put(token, *inv.ProposedAction)
		fmt.Fprintf(&report, "\nReply CONFIRM %s to apply: %s %s/%s",
			token, inv.ProposedAction.Kind, inv.ProposedAction.AtmID, inv.ProposedAction.Location)
	}

	if _, err := d.reply.SendReply(ctx, rcpt, report.String(), dc.replyToID, nil); err != nil {
		return fmt.Errorf("inbound: investigation reply: %w", err)
	}
	log.Printf("inbound dispatched: investigation.start (event=%s topic=%q pending=%v)", dc.ev.EventID, inv.Topic, inv.ProposedAction != nil)
	return nil
}

// defaultItemLocation is the workable-item location Tier 1 assumes when the
// recognizer fast-paths an item.update / investigation against a bare ATM id —
// open items live in the "Issues" tracker. (The LLM path supplies an explicit
// location in its payload; Tier 1 cannot know it, so it defaults to the
// primary open-items location.)
const defaultItemLocation = "Issues"

// buildRecognizedReply translates a TIER 1 recognizer result (action string +
// fields map, from CommandRecognizer.RecognizeCommand) into the concrete Reply
// the action handlers consume. The shape mirrors what ParseReply would have
// produced from an LLM <<<HERALD-REPLY>>> payload, so the SAME handler routes
// both Tier 1 and Tier 2 actions (no parallel routing path).
func buildRecognizedReply(action string, fields map[string]string) *Reply {
	r := &Reply{Action: action}
	switch action {
	case "item.update":
		f := map[string]string{}
		if s, ok := fields[cmdFieldStatus]; ok && s != "" {
			f["status"] = s
		}
		if a, ok := fields[cmdFieldAssignedTo]; ok && a != "" {
			f[fieldAssignedTo] = normalizeAt(a)
		}
		r.ItemUpdate = &ItemUpdatePayload{
			AtmID:    fields[cmdFieldAtmID],
			Location: defaultItemLocation,
			Fields:   f,
		}
	case "issue.open":
		r.Issue = &IssuePayload{
			Type:  fields[cmdFieldType],
			Title: fields[cmdFieldTitle],
		}
	case "investigation.start":
		r.Investigation = &InvestigationPayload{
			Topic: fields[cmdFieldAtmID],
		}
	case "reply":
		// A recognized status query — answer is produced downstream; Tier 1
		// only routes it to the reply sink with a short acknowledgement naming
		// the item so the subscriber sees their query was understood.
		atm := fields[cmdFieldAtmID]
		r.Text = "Looking up the status of " + atm + "…"
	}
	return r
}

// actClarify implements TIER 3 (docs/design/INTENT_RECOGNITION.md §3): when
// neither a Tier-1 command nor a confident Tier-2 intent could be determined,
// the LLM returns action=clarify with a precise Question. This handler replies
// to the original message (threaded) with a body of:
//
//	@<sender-username> <question>
//
// The sender is resolved to their per-channel @username via the §11.4.104
// IdentityResolver — ResolveSender maps the inbound sender to a canonical
// handle, then UsernameFor maps that handle to the @username on the originating
// channel. When no alias is known (or no resolver is configured) it falls back
// to the raw sender handle so the user is STILL tagged (never ignored, the
// anti-annoyance guarantee).
//
// §107 anchor: the recording-sink E2E test asserts the captured reply body is
// EXACTLY "@<sender> <question>" — proving the user is tagged + asked, not
// silently dropped.
func (d *Dispatcher) actClarify(ctx context.Context, dc dispatchCtx) error {
	q := strings.TrimSpace(dc.reply.Question)
	if q == "" {
		return errors.New("inbound: action=clarify but reply.question is empty (a clarify with no question is a §107 bluff)")
	}
	tag := d.clarifyTag(dc.ev)
	body := tag + " " + q
	rcpt := recipientOf(dc.ev)
	if _, err := d.reply.SendReply(ctx, rcpt, body, dc.replyToID, nil); err != nil {
		return fmt.Errorf("inbound: clarify reply: %w", err)
	}
	log.Printf("inbound dispatched: clarify (event=%s tag=%s)", dc.ev.EventID, tag)
	return nil
}

// clarifyTag resolves the originating sender to the @username we tag in the
// clarify reply. Resolution order (§11.4.104 IdentityResolver):
//
//  1. resolver.ResolveSender(channel, channelUserID, rawUsername) → canonical handle
//  2. resolver.UsernameFor(handle, channel) → per-channel @username (preferred tag)
//  3. fall back to the canonical handle itself (it is already an @username for
//     unknown first-contact senders)
//  4. fall back to the raw stamped @username from the event
//  5. last resort: "@" + channelUserID so the user is STILL tagged
//
// Every path returns a non-empty @-prefixed tag — the user is never left
// untagged.
func (d *Dispatcher) clarifyTag(ev commons.InboundEvent) string {
	rawUsername := senderUsername(ev)
	if d.resolver != nil {
		handle := d.resolver.ResolveSender(ev.Sender.Channel, ev.Sender.ChannelUserID, rawUsername)
		if uname, ok := d.resolver.UsernameFor(handle, ev.Sender.Channel); ok && uname != "" {
			return normalizeAt(uname)
		}
		if handle != "" {
			return normalizeAt(handle)
		}
	}
	if rawUsername != "" {
		return normalizeAt(rawUsername)
	}
	if ev.Sender.ChannelUserID != "" {
		return normalizeAt(ev.Sender.ChannelUserID)
	}
	return "@there"
}

// parseConfirm recognises a "CONFIRM <token>" message and returns the
// token. Case-insensitive on the CONFIRM keyword; the token is the
// next whitespace-delimited field verbatim.
func parseConfirm(body string) (string, bool) {
	fields := strings.Fields(strings.TrimSpace(body))
	if len(fields) == 2 && strings.EqualFold(fields[0], "CONFIRM") {
		return fields[1], true
	}
	return "", false
}

// handleConfirm looks up the pending proposal for token and executes it
// via the ItemMutator. An unknown token is an explicit error (no
// fabricated success); the entry is consumed on lookup so a replayed
// CONFIRM cannot double-apply.
func (d *Dispatcher) handleConfirm(ctx context.Context, ev commons.InboundEvent, token string) error {
	if d.items == nil {
		return errors.New("inbound: CONFIRM received but no ItemMutator configured")
	}
	a, err := d.pending.take(token)
	if err != nil {
		return fmt.Errorf("inbound: CONFIRM %s: %w", token, err)
	}
	rt := extractReplyToID(ev.Raw)
	var execErr error
	switch a.Kind {
	case "update":
		execErr = d.items.Update(ctx, a.AtmID, a.Location, a.Fields)
	case "delete":
		execErr = d.items.Delete(ctx, a.AtmID, a.Location)
	default:
		return fmt.Errorf("inbound: CONFIRM %s: unknown pending kind %q", token, a.Kind)
	}
	if execErr != nil {
		return fmt.Errorf("inbound: CONFIRM %s execute %s: %w", token, a.Kind, execErr)
	}
	rcpt := recipientOf(ev)
	confirmMsg := fmt.Sprintf("Applied: %s %s/%s", a.Kind, a.AtmID, a.Location)
	if _, err := d.reply.SendReply(ctx, rcpt, confirmMsg, rt, nil); err != nil {
		return fmt.Errorf("inbound: CONFIRM %s reply: %w", token, err)
	}
	log.Printf("inbound dispatched: confirm (event=%s token=%s kind=%s atm=%s location=%s)", ev.EventID, token, a.Kind, a.AtmID, a.Location)
	return nil
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
	rt := extractReplyToID(ev.Raw)
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

// extractReplyToID returns the CHANNEL-AGNOSTIC thread-parent id (as the string
// the inbound.Replier contract expects) so EVERY reply is delivered IN-THREAD on
// every messenger that supports threading. Per-channel semantics:
//
//   - Slack: ev.Raw["message_id"] is the message ts (e.g. "1780389836.066119")
//     and ev.Raw["thread_ts"] is the thread root when the incoming message is
//     already inside a thread. We PREFER thread_ts (reply into the existing
//     thread); otherwise we return the message's own ts so the reply STARTS a
//     thread on it. The slack adapter maps this string straight onto thread_ts.
//   - Telegram: ev.Raw["message_id"] is the integer Bot API message_id (int from
//     in-process Subscribe, float64 from a getUpdates JSON round-trip). We return
//     it as a decimal string; the tgram adapter parses it back to the int
//     reply_to_message_id — Telegram's threading mechanism.
//
// Returns "" only when no usable id is present (no Raw, no message_id, and no
// thread_ts) — callers then send a non-threaded message, the correct degraded
// behaviour. This REPLACES the former int-only extraction at the reply sites,
// which silently dropped Slack threading (a string ts is unparseable as int).
func extractReplyToID(raw map[string]any) string {
	if raw == nil {
		return ""
	}
	// Slack: a message already in a thread carries thread_ts (the thread root).
	// Telegram never sets this key, so this branch is Slack-only.
	if ts, ok := raw["thread_ts"].(string); ok && ts != "" {
		return ts
	}
	v, ok := raw["message_id"]
	if !ok {
		return ""
	}
	switch x := v.(type) {
	case string: // Slack ts ("<epoch>.<seq>") — top-level message; thread on it.
		return x
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case int32:
		return strconv.Itoa(int(x))
	case float64: // Telegram getUpdates JSON number.
		return strconv.FormatInt(int64(x), 10)
	case float32:
		return strconv.FormatInt(int64(x), 10)
	default:
		return ""
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
