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
	"strings"

	"github.com/google/uuid"
	"github.com/vasic-digital/herald/commons"
)

// Dispatcher is the Claude Code LLM dispatcher.
type Dispatcher struct {
	binaryPath  string // typically "claude"
	workingDir  string // consuming-project root
	projectName string // e.g. "ATMOSphere"
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
		binaryPath:  binaryPath,
		workingDir:  workingDir,
		projectName: projectName,
	}, nil
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
	sb.WriteString("]\n\nUser message:\n\n")
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
