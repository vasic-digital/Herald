// Package claude_code is the Claude Code LLM dispatcher per spec §33.
//
// Status: LIVE (HRD-012 step 6). Session-resolution + envelope formatter
// remain in this file; live `claude --resume <UUID> --print "<envelope>"`
// invocation + <<<HERALD-REPLY>>> parsing live in dispatch.go. Session
// bootstrap (auto-spawn on uuid.Nil + PersistSession write-back) is
// HRD-012 step 7; until then Dispatch returns an explicit error when
// the anchor is missing rather than fabricating a session.
//
// The resolution algorithm (spec §33.2):
//
//  1. compute working_dir = config[herald.session_workdir] (default:
//     consuming-project root via parent-walk).
//  2. compute session_anchor_path = working_dir/.herald/claude-code/
//     sessions/<project_name>.session.
//  3. if session_anchor_path exists AND contains a valid session UUID
//     AND `claude --resume <uuid> --print "ping"` returns 0:
//     → return that UUID.
//  4. else:
//     spawn `claude --print "Initializing Herald session for project:
//     <project_name>"` in working_dir.
//     capture the new session UUID from Claude Code stdout.
//     write UUID to session_anchor_path.
//     → return new UUID.
package claude_code

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	db "digital.vasic.database/pkg/database"
	"github.com/google/uuid"
	"github.com/vasic-digital/herald/commons"
)

// DefaultBootstrapTimeout caps the wall-clock cost of a single
// claude --session-id <new-uuid> --print "<bootstrap-prompt>" invocation
// (HRD-012 step 7). 60s mirrors the spec §33.2 step-4 budget; longer
// budgets would mask the more-common "claude binary hangs on auth" case.
const DefaultBootstrapTimeout = 60 * time.Second

// DefaultDispatchTimeout caps the wall-clock cost of a single live
// `claude --resume <UUID> --print "<envelope>"` invocation (HRD-146).
//
// The production caller — pherald listen → Dispatcher.Handle → Dispatch
// — passes the long-poll runtime ctx, which has NO per-message deadline.
// Without a bound, a hung `claude` invocation blocks the inbound
// subscriber goroutine indefinitely (a §11.4 / §107 resilience bluff: the
// runtime claims to process messages but silently wedges on the first
// hang). Dispatch therefore imposes this bound itself, derived as
// min(caller-ctx-deadline, DispatchTimeout) so a tighter caller deadline
// still wins. 120s is generous for a real Opus reply yet short enough that
// an auth/network hang is surfaced promptly rather than wedging the loop.
const DefaultDispatchTimeout = 120 * time.Second

// DispatchTimeoutEnv overrides DefaultDispatchTimeout at construction time
// via a Go duration string (e.g. "90s", "3m"). An empty/unset/invalid
// value falls back to DefaultDispatchTimeout. Resolution lives in New so
// the bound is fixed once per Dispatcher, not re-parsed per message.
const DispatchTimeoutEnv = "HERALD_CLAUDE_DISPATCH_TIMEOUT"

// HeraldSystemTenant is a fixed UUID that scopes Herald's internal
// (operator-shared, non-customer-tenant) data. Sessions, configs, and
// other infra-level state live under this tenant.
//
// Per §16 + §44.6 this is NOT a customer tenant — it's the operator-shared
// bucket. The RLS infrastructure treats it like any other tenant_id, but
// application-level interpretation differs (rows here are operator state,
// not subscriber state).
var HeraldSystemTenant = uuid.MustParse("00000000-0000-0000-0000-000000000001")

// Dispatcher is the Claude Code LLM dispatcher.
type Dispatcher struct {
	binaryPath       string        // typically "claude"
	workingDir       string        // consuming-project root
	projectName      string        // e.g. "ATMOSphere"
	pool             db.Database   // optional; nil = persistence disabled (Dispatch-only)
	bootstrapTimeout time.Duration // §33.2 step-4 budget; default DefaultBootstrapTimeout
	dispatchTimeout  time.Duration // HRD-146 per-message live-dispatch budget; default DefaultDispatchTimeout
}

// SetDispatchTimeout overrides the per-message live-dispatch subprocess
// timeout (HRD-146). A non-positive value resets to DefaultDispatchTimeout.
// Exposed so the inbound runtime + tests can tighten or stretch the budget
// without rebuilding the binary or setting the env var.
func (d *Dispatcher) SetDispatchTimeout(t time.Duration) {
	if t <= 0 {
		d.dispatchTimeout = DefaultDispatchTimeout
		return
	}
	d.dispatchTimeout = t
}

// dispatchTimeoutOrDefault returns the active per-message dispatch budget.
func (d *Dispatcher) dispatchTimeoutOrDefault() time.Duration {
	if d.dispatchTimeout <= 0 {
		return DefaultDispatchTimeout
	}
	return d.dispatchTimeout
}

// resolveDispatchTimeout reads DispatchTimeoutEnv and parses it as a Go
// duration, falling back to DefaultDispatchTimeout when unset/empty/invalid.
// Invalid values fall back silently to the default rather than failing
// construction — a malformed env var must never wedge the runtime, and the
// default is a safe bound. Kept as a free function so New stays readable.
func resolveDispatchTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv(DispatchTimeoutEnv))
	if raw == "" {
		return DefaultDispatchTimeout
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return DefaultDispatchTimeout
	}
	return d
}

// SetBootstrapTimeout overrides the default bootstrap subprocess timeout.
// A non-positive value resets to DefaultBootstrapTimeout. Exposed so the
// inbound runtime + tests can stretch the budget on slow CI runners
// without rebuilding the binary.
func (d *Dispatcher) SetBootstrapTimeout(t time.Duration) {
	if t <= 0 {
		d.bootstrapTimeout = DefaultBootstrapTimeout
		return
	}
	d.bootstrapTimeout = t
}

// New constructs a Dispatcher.
//
// binaryPath is the path to the `claude` CLI (default: lookup via $PATH).
// workingDir is the consuming-project root that anchors the session.
// projectName is the [herald].project_name config value.
func New(binaryPath, workingDir, projectName string) (*Dispatcher, error) {
	if projectName == "" {
		return nil, errors.New("claude_code: project_name MUST NOT be empty (spec §18.2.5)")
	}
	if binaryPath == "" {
		binaryPath = "claude"
	}
	if workingDir == "" {
		var err error
		workingDir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("claude_code: resolving working_dir: %w", err)
		}
	}
	return &Dispatcher{
		binaryPath:       binaryPath,
		workingDir:       workingDir,
		projectName:      projectName,
		bootstrapTimeout: DefaultBootstrapTimeout,
		dispatchTimeout:  resolveDispatchTimeout(),
	}, nil
}

// NewWithStorage constructs a Dispatcher that persists session state to
// the given pool after each successful Dispatch. Persistence is
// best-effort: if the upsert fails AFTER `claude` has already produced a
// reply, Dispatch returns the error to the caller but does NOT roll back
// the dispatch — Claude has already responded and re-issuing would
// double-spend the model invocation. Per §107 we prefer dropping a
// persistence row over re-issuing.
//
// Callers wanting persistence get it automatically through Dispatch (no
// SendForTenant-style separate entry point — the projectName is already
// owned by the Dispatcher, and Herald system-tenant scoping is implicit).
func NewWithStorage(binaryPath, workingDir, projectName string, pool db.Database) (*Dispatcher, error) {
	d, err := New(binaryPath, workingDir, projectName)
	if err != nil {
		return nil, err
	}
	d.pool = pool
	return d, nil
}

// Name identifies this dispatcher.
func (d *Dispatcher) Name() string { return "claude-code" }

// ResolveSession returns the session UUID anchored at
// <working_dir>/.herald/claude-code/sessions/<project_name>.session.
// If the anchor file exists and contains a valid UUID, that's returned;
// otherwise an empty UUID is returned and the caller should spawn
// `claude` to create a new session (logic kept separate so live
// invocation stays unit-testable in isolation).
func (d *Dispatcher) ResolveSession() (uuid.UUID, string, error) {
	anchor := filepath.Join(d.workingDir, ".herald", "claude-code", "sessions", d.projectName+".session")
	raw, err := os.ReadFile(anchor)
	if err != nil {
		if os.IsNotExist(err) {
			return uuid.Nil, anchor, nil
		}
		return uuid.Nil, anchor, err
	}
	s := strings.TrimSpace(string(raw))
	u, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, anchor, fmt.Errorf("claude_code: anchor file contains non-UUID %q: %w", s, err)
	}
	return u, anchor, nil
}

// PersistSession writes the session UUID to the anchor file, creating
// parent directories as needed.
func (d *Dispatcher) PersistSession(u uuid.UUID, anchorPath string) error {
	if err := os.MkdirAll(filepath.Dir(anchorPath), 0o755); err != nil {
		return fmt.Errorf("claude_code: mkdir anchor parent: %w", err)
	}
	return os.WriteFile(anchorPath, []byte(u.String()+"\n"), 0o644)
}

// DispatchRequest carries the structured fields the §33.3 envelope is
// built from.
type DispatchRequest struct {
	InboundID      string
	Sender         string // formatted "<channel>:<handle>"
	Channel        commons.ChannelID
	Classification Classification
	Conversation   string // full thread bottom-to-top
	Attachments    []commons.Attachment
	UserMessage    string

	// ThreadContext carries the PRIOR messages of the thread this message
	// belongs to (oldest→newest), excluding the current message. When
	// non-empty, FormatEnvelope renders a delimited THREAD CONTEXT block
	// before the user message so the LLM's reply is bound by the thread's
	// MEANING and only contributed when the context warrants one (operator
	// mandate 2026-06-02). Empty → the envelope is byte-identical to its
	// pre-thread-context form.
	ThreadContext []commons.ThreadMessage
}

// threadContextMaxMessages caps how many of the most-recent prior messages
// are rendered into the THREAD CONTEXT block — older messages beyond this
// are summarised as a single elision line so the envelope cannot grow
// unbounded on a long thread.
const threadContextMaxMessages = 20

// threadContextMaxTextLen caps each prior message's text length in the
// rendered block; longer texts are truncated with an ellipsis so one
// verbose prior message cannot dominate the envelope.
const threadContextMaxTextLen = 500

// threadSubjectRefRe matches Herald/ATMOSphere workable-item references
// (HRD-123, ATM-9) anywhere in a thread's text, so the dispatcher can tell the
// LLM which EXISTING item a thread is about. Compiled once; a *regexp.Regexp is
// safe for concurrent use, so this introduces no data race.
var threadSubjectRefRe = regexp.MustCompile(`(?i)\b((?:HRD|ATM)-\d+)\b`)

// renderThreadContext builds the THREAD CONTEXT block: WHO is participating, the
// PRIOR messages, and WHAT the thread is about (an existing workable item, or —
// for the LLM to classify — a newly-reported issue / existing item / system or
// project event), so the reply is a contribution BOUND to that subject and made
// only when the thread's context warrants one (operator mandate 2026-06-02).
// Returns the empty string when there are no prior messages — so the envelope is
// byte-identical to its pre-thread-context form for the no-context case.
func renderThreadContext(req DispatchRequest) string {
	msgs := req.ThreadContext
	if len(msgs) == 0 {
		return ""
	}
	// Render only the last threadContextMaxMessages, oldest-first within
	// that window; note any elided older messages on a leading line.
	start := 0
	elided := 0
	if len(msgs) > threadContextMaxMessages {
		start = len(msgs) - threadContextMaxMessages
		elided = start
	}
	var sb strings.Builder
	sb.WriteString("THREAD CONTEXT — this message is part of an existing thread; it has a MEANING and a SUBJECT, and\n")
	sb.WriteString("your reply is a contribution bound to that subject, not an isolated answer.\n")

	// WHO — the distinct participants of the thread (incl. the current sender).
	fmt.Fprintf(&sb, "Participants: %s\n", threadParticipants(req))

	// The prior messages.
	sb.WriteString("Prior messages (oldest first):\n")
	if elided > 0 {
		fmt.Fprintf(&sb, "  [… %d earlier message(s) elided …]\n", elided)
	}
	for i := start; i < len(msgs); i++ {
		m := msgs[i]
		who := m.SenderHandle
		if who == "" {
			who = "unknown"
		}
		if m.SenderIsBot {
			who += " | bot"
		}
		fmt.Fprintf(&sb, "  [%d] %s said: %s\n", (i-start)+1, who, truncateThreadText(m.Text))
	}

	// WHAT — the subject the thread is about.
	if refs := detectWorkableRefs(req); len(refs) > 0 {
		fmt.Fprintf(&sb, "SUBJECT: this thread references existing workable item(s): %s — treat the thread as\n", strings.Join(refs, ", "))
		sb.WriteString("concerning them; consider their current state and make your reply relevant to that item (status,\n")
		sb.WriteString("progress, a follow-up), not a generic answer.\n")
	} else {
		sb.WriteString("SUBJECT: determine from the messages above what this thread is about — a NEWLY-REPORTED issue, an\n")
		sb.WriteString("EXISTING ticket / workable item (HRD-NNN / ATM-N), or a SYSTEM / PROJECT event — and make your\n")
		sb.WriteString("reply relevant to that subject.\n")
	}
	sb.WriteString("Reply ONLY when the thread's context warrants a contribution regarding its subject; do not answer\n")
	sb.WriteString("out of context.\n\n")
	return sb.String()
}

// threadParticipants returns a de-duplicated, human-readable list of the
// distinct senders in the thread (incl. the current sender, parsed from
// req.Sender's "<channel>:<handle>" form), each tagged human/bot, so the LLM
// knows WHO is participating in the thread.
func threadParticipants(req DispatchRequest) string {
	seen := map[string]bool{}
	var out []string
	add := func(handle string, isBot bool) {
		h := strings.TrimSpace(handle)
		if h == "" {
			return
		}
		key := strings.ToLower(h)
		if seen[key] {
			return
		}
		seen[key] = true
		tag := "human"
		if isBot {
			tag = "bot"
		}
		out = append(out, fmt.Sprintf("%s (%s)", h, tag))
	}
	for _, m := range req.ThreadContext {
		add(m.SenderHandle, m.SenderIsBot)
	}
	cur := req.Sender // "<channel>:<handle>"
	if i := strings.LastIndex(cur, ":"); i >= 0 && i+1 <= len(cur) {
		cur = cur[i+1:]
	}
	add(cur, false)
	if len(out) == 0 {
		return "(unknown)"
	}
	return strings.Join(out, ", ")
}

// detectWorkableRefs scans the thread messages + the current user message for
// Herald/ATMOSphere workable-item references (HRD-NNN / ATM-N), de-duplicated +
// upper-cased, so the LLM is told which EXISTING item the thread concerns.
func detectWorkableRefs(req DispatchRequest) []string {
	seen := map[string]bool{}
	var out []string
	scan := func(s string) {
		for _, m := range threadSubjectRefRe.FindAllString(s, -1) {
			u := strings.ToUpper(m)
			if !seen[u] {
				seen[u] = true
				out = append(out, u)
			}
		}
	}
	for _, m := range req.ThreadContext {
		scan(m.Text)
	}
	scan(req.UserMessage)
	return out
}

// truncateThreadText caps a single prior message's text and collapses
// newlines so a multi-line prior message renders on one bullet line.
func truncateThreadText(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > threadContextMaxTextLen {
		return s[:threadContextMaxTextLen] + "…"
	}
	return s
}

// Classification is the spec §32.6 classifier output.
type Classification struct {
	Type        string  // "bug"|"task"|"implementation"|"query"|...
	Criticality string  // "critical"|"high"|"middle"|"low"
	Confidence  float64 // [0,1]
}

// FormatEnvelope renders the §33.3 <<<HERALD-DISPATCH-v1>>> envelope
// that gets piped to `claude --print`. The format is stable; changing
// it is a spec change (HRD-012 follow-up).
func (d *Dispatcher) FormatEnvelope(req DispatchRequest) string {
	var sb strings.Builder
	sb.WriteString("<<<HERALD-DISPATCH-v1>>>\n")
	fmt.Fprintf(&sb, "Project:        %s\n", d.projectName)
	fmt.Fprintf(&sb, "Inbound ID:     %s\n", req.InboundID)
	fmt.Fprintf(&sb, "Sender:         %s\n", req.Sender)
	fmt.Fprintf(&sb, "Channel:        %s\n", req.Channel)
	fmt.Fprintf(&sb, "Classification: type=%s criticality=%s confidence=%.2f\n",
		req.Classification.Type, req.Classification.Criticality, req.Classification.Confidence)
	fmt.Fprintf(&sb, "Conversation:   %s\n", req.Conversation)
	sb.WriteString("Attachments:    [")
	for i, a := range req.Attachments {
		if i > 0 {
			sb.WriteString(", ")
		}
		fmt.Fprintf(&sb, "%s:%s:%d", a.Filename, a.MIMEType, a.SizeBytes)
	}
	sb.WriteString("]\n\n")
	sb.WriteString(renderThreadContext(req)) // empty when no prior thread → envelope unchanged
	sb.WriteString("User message:\n\n")
	sb.WriteString(req.UserMessage)
	sb.WriteString("\n\n────────────────────────────────────────────────────────────\n")
	sb.WriteString("HERALD TASK (run in background along with mainstream work):\n\n")
	sb.WriteString(taskVerbFor(req.Classification.Type))
	sb.WriteString("\n\nReply with a JSON object on a single line, prefixed with `<<<HERALD-REPLY>>>`:\n")
	sb.WriteString(replyJSONSchema)
	sb.WriteString("\n\nDO NOT modify project files unless the subscriber explicitly asked you to.\n")
	sb.WriteString("DO NOT commit. DO NOT push.\n")
	sb.WriteString("<<<END-HERALD-DISPATCH>>>\n")
	return sb.String()
}

// FormatEnvelopeWithPreText renders the §33.3 envelope prefixed with the
// verbatim operator pre-text per Wave 6 operator mandate (2026-05-22):
//
//	"We have received new message from our communication channel <name>.
//	 <classification sentence>. <attachment list>"
//
// followed by a blank line and the existing <<<HERALD-DISPATCH-v1>>>
// structured block (kept byte-for-byte identical to FormatEnvelope's
// output for the structured portion).
//
// §107 anchor: the opening sentence MUST appear as a strict prefix of
// the rendered output — TestFormatEnvelopePreText asserts via
// strings.HasPrefix, and T11 invariant E74 greps captured envelopes
// (under docs/qa/<run-id>/) for the literal prefix.
func (d *Dispatcher) FormatEnvelopeWithPreText(req DispatchRequest, channelName string) string {
	var pre strings.Builder
	fmt.Fprintf(&pre, "We have received new message from our communication channel %s.\n", channelName)
	fmt.Fprintf(&pre, "The message has been classified as %q with %q criticality (confidence %.2f).\n",
		req.Classification.Type, req.Classification.Criticality, req.Classification.Confidence)
	fmt.Fprintf(&pre, "Sender: %s. Inbound ID: %s.\n", req.Sender, req.InboundID)
	if len(req.Attachments) > 0 {
		pre.WriteString("Attached materials:\n")
		for _, a := range req.Attachments {
			// Attachments downloaded by pherald inbound runtime are
			// available on the local filesystem. The path is carried in
			// the Filename field for Wave 6 (it's already the canonical
			// ~/.herald/inbox/<sha256>.<ext> path emitted by the
			// attachment download helper — see T5).
			fmt.Fprintf(&pre, "  - %s (%s, %d bytes)\n", a.Filename, a.MIMEType, a.SizeBytes)
		}
	} else {
		pre.WriteString("No attached materials.\n")
	}

	// Wave 6.5 §107 fix (2026-05-23): the LLM was never told how to
	// choose between action types — it always defaulted to "reply" even
	// for type=bug messages. Without this guidance the issue.open
	// pipeline (DocsIssueOpener → docs/Issues.md mutation) never fired
	// in production, leaving the bug-ticket lifecycle broken at the
	// prompt-engineering layer. Live evidence: scenario S5 (HRD-101).
	pre.WriteString("\nACTION FORMAT GUIDANCE — your reply MUST end with a single line\n")
	pre.WriteString("starting with <<<HERALD-REPLY>>> followed by JSON. Choose action by\n")
	pre.WriteString("classification.type:\n")
	pre.WriteString("  - type in {bug, task, implementation, investigation}\n")
	pre.WriteString("      action: \"issue.open\" with payload\n")
	pre.WriteString("      {\"action\":\"issue.open\",\"issue\":{\"type\":\"<type>\",\"criticality\":\"<crit>\",\n")
	pre.WriteString("       \"title\":\"<short summary, less than or equal to 80 chars>\",\"body\":\"<longer description>\",\"labels\":[]}}\n")
	pre.WriteString("  - type = query (or empty/unrecognised): action: \"reply\"\n")
	pre.WriteString("      {\"action\":\"reply\",\"text\":\"<natural-language answer>\"}\n")
	pre.WriteString("  - type = event_trigger: action: \"event.emit\"\n")
	pre.WriteString("      {\"action\":\"event.emit\",\"event\":{\"cloudevent_type\":\"<type>\",\n")
	pre.WriteString("       \"subject\":\"<subject>\",\"data\":{}}}\n")
	pre.WriteString("Help/Status/Continue/Done/Reopen never reach you — fast-pathed in pherald.\n")
	pre.WriteString("\n")

	// TIER 2 intent-inference instruction (docs/design/INTENT_RECOGNITION.md
	// §1/§4). The user speaks PLAIN LANGUAGE — there is NO command syntax and
	// no "COMMAND:" prefix. Recognize Herald's command set from natural
	// language and map to the right action. If you CANNOT determine the intent
	// with confidence, return action=clarify with a PRECISE question naming the
	// candidate intents — do NOT guess (§11.4.6: a wrong action is worse than a
	// clarifying question).
	pre.WriteString(intentInferenceInstruction)
	pre.WriteString("\n")

	pre.WriteString(suggestedActionLine(req.Classification.Type))
	pre.WriteString("\n")

	pre.WriteString(d.FormatEnvelope(req)) // existing structured block unchanged
	return pre.String()
}

// intentInferenceInstruction is the TIER 2 envelope block
// (docs/design/INTENT_RECOGNITION.md §4). It is additive to the existing
// ACTION FORMAT GUIDANCE: it tells the LLM that users speak plain language, to
// map natural language onto Herald's command set, and — crucially — to return
// action=clarify with a precise question instead of guessing when the intent
// is not confidently determinable.
//
// §107 anchor: TestFormatEnvelope_IntentInferenceInstruction asserts this block
// (and the literal "action=clarify" token) appears in the rendered envelope, so
// a regression that drops the instruction is caught.
const intentInferenceInstruction = `INTENT RECOGNITION — the user speaks PLAIN LANGUAGE. There is NO command syntax
and NO "COMMAND:" prefix. Recognize Herald's command set from natural language
and map the message to the right action:
  - "close ATM-N" / "mark ATM-N fixed|done|resolved"  → item.update (status)
  - "set ATM-N to <status>" / "ATM-N is blocked"       → item.update (status)
  - "assign ATM-N to @x" / "give ATM-N to @x"          → item.update (assigned_to)
  - "open a bug: <title>" / "create a task: <title>"   → issue.open (type+title)
  - "investigate ATM-N" / "look into ATM-N"            → investigation.start
  - "status of ATM-N?" / "what's ATM-N?"               → reply (status query)
If you CANNOT determine the intent with confidence, DO NOT guess. Return
action=clarify with a PRECISE question naming the candidate intents, e.g.:
  {"action":"clarify","question":"did you want to close ATM-9, reassign it, or just get its status?"}
A wrong action is worse than a clarifying question (§11.4.6 no-guessing).
`

// suggestedActionLine emits a single line telling the LLM which action
// to use for this specific message, derived deterministically from
// classification.type. Wave 6.5 fix per §107: removes the LLM's
// freedom to silently default to "reply" for bug/task messages.
func suggestedActionLine(classType string) string {
	switch classType {
	case "bug", "task", "implementation", "investigation":
		return fmt.Sprintf("SUGGESTED ACTION for this message (type=%q): emit issue.open.\n", classType)
	case "query", "":
		return "SUGGESTED ACTION for this message: emit reply with a concise natural-language answer.\n"
	case "event_trigger":
		return "SUGGESTED ACTION for this message: emit event.emit with the appropriate CloudEvent type.\n"
	default:
		return fmt.Sprintf("SUGGESTED ACTION for this message (type=%q): emit reply asking for clarification.\n", classType)
	}
}

// DispatchResponse is the typed projection of the §33.3 JSON reply.
// JSON tags match the snake_case schema declared in replyJSONSchema.
// SessionUUID + AnchorPath are populated by Dispatch post-exec for the
// persistence layer (HRD-012 step 7) and are excluded from the wire JSON.
type DispatchResponse struct {
	Outcome              string                `json:"outcome"` // "validated"|"rejected"|"needs_more_info"|"answered"
	Summary              string                `json:"summary"`
	Details              string                `json:"details"`
	AffectedPaths        []string              `json:"affected_paths"`
	ReproductionSteps    []string              `json:"reproduction_steps"`
	EstimatedEffort      string                `json:"estimated_effort"` // "S"|"M"|"L"|"XL"
	WorkableItemProposed *WorkableItemProposal `json:"workable_item_proposed,omitempty"`
	FollowUpQuestions    []string              `json:"follow_up_questions"`

	// Populated by Dispatch — not part of the wire JSON.
	SessionUUID uuid.UUID `json:"-"`
	AnchorPath  string    `json:"-"`
}

// WorkableItemProposal is the §33.3 nested object the LLM may include
// when outcome=validated.
type WorkableItemProposal struct {
	Type        string   `json:"type"`
	Criticality string   `json:"criticality"`
	Title       string   `json:"title"`
	Labels      []string `json:"labels"`
}

// --- internals ---------------------------------------------------------

func taskVerbFor(itemType string) string {
	switch itemType {
	case "bug", "issue", "investigation":
		return "- For bug/issue/investigation: reproduce + identify affected code paths + classify root-cause area + propose validation steps."
	case "query", "question":
		return "- For query/question: research + answer; cite project docs if relevant; short answers preferred (subscribers see this directly)."
	case "request", "implementation":
		return "- For request/implementation: scope effort + propose approach + flag prerequisites + estimate workable-item dependencies."
	case "spec_change_request":
		return "- For spec_change_request: invoke Herald Constitution §106 spec-change rule."
	default:
		return "- Process per the standard Herald inbound pipeline (§32)."
	}
}

const replyJSONSchema = `
{
  "outcome": "validated|rejected|needs_more_info|answered",
  "summary": "<short summary for the subscriber>",
  "details": "<longer markdown body for the diary>",
  "affected_paths": ["<file>:<line>", "..."],
  "reproduction_steps": ["..."],
  "estimated_effort": "S|M|L|XL",
  "workable_item_proposed": {
    "type": "bug|task|implementation|investigation",
    "criticality": "critical|high|middle|low",
    "title": "...",
    "labels": ["..."]
  },
  "follow_up_questions": ["..."]
}`
