// Package inbound is the pherald production InboundHandler runtime.
//
// Reply parsing (this file) extracts the structured <<<HERALD-REPLY>>> block
// from Claude Code stdout into a typed Reply per Wave 6 operator-locked
// action triggers (2026-05-22). Action defaults to "reply" when omitted.
//
// §107 anchor: no marker → explicit error (not a fabricated default reply).
// Malformed JSON → explicit error (not a silent empty Reply{}). Unknown
// action → caller-level error (dispatcher.go's Handle switch returns it).
package inbound

import (
	"encoding/json"
	"errors"
	"strings"
)

// Reply is the structured projection of the <<<HERALD-REPLY>>> JSON payload
// per Wave 6 operator-locked action triggers (2026-05-22).
//
// Action defaults to "reply" when the field is omitted in the JSON payload —
// this is set inside ParseReply, not at decode-time, so callers can
// distinguish "missing action → defaulted" from "explicit action".
type Reply struct {
	Action string        `json:"action"` // "reply" (default) | "issue.open" | "event.emit" | "item.update" | "item.delete" | "investigation.start" | "clarify"
	Text   string        `json:"text"`   // body for action=reply
	Issue  *IssuePayload `json:"issue,omitempty"`
	Event  *EventPayload `json:"event,omitempty"`

	// WS-4 (HRD-152) workable-item CRUD + investigation payloads.
	ItemUpdate    *ItemUpdatePayload   `json:"item_update,omitempty"`
	ItemDelete    *ItemDeletePayload   `json:"item_delete,omitempty"`
	Investigation *InvestigationPayload `json:"investigation,omitempty"`

	// Question is the precise clarifying question for action=clarify (TIER 3,
	// docs/design/INTENT_RECOGNITION.md §3). The clarifyHandler sends a reply
	// whose body is `@<sender-username> <Question>`, tagging the original
	// sender (resolved via the IdentityResolver) and naming the candidate
	// intents — the anti-annoyance guarantee that the user is never ignored
	// and never has to learn syntax. The LLM is instructed to return
	// action=clarify with a specific question rather than guess an action
	// (§11.4.6 no-guessing: a wrong action is worse than a clarifying question).
	Question string `json:"question,omitempty"`
}

// ItemUpdatePayload is the action=item.update payload. Fields holds the
// column→new-value pairs to apply to the (AtmID, Location) workable item.
type ItemUpdatePayload struct {
	AtmID    string            `json:"atm_id"`
	Location string            `json:"location"`
	Fields   map[string]string `json:"fields"`
}

// ItemDeletePayload is the action=item.delete payload — the composite
// key of the workable item to remove.
type ItemDeletePayload struct {
	AtmID    string `json:"atm_id"`
	Location string `json:"location"`
}

// ProposedAction is a single mutating action an investigation may
// propose. It is NOT executed immediately — investigation.start is
// ACT-WITH-CONFIRMATION (operator decision 2026-05-29): the dispatcher
// records it as pending and emits a confirmation prompt; a subsequent
// CONFIRM <token> message executes it.
//
// Kind is "update" or "delete". For "update", Fields carries the
// column→value pairs; for "delete", Fields is ignored.
type ProposedAction struct {
	Kind     string            `json:"kind"` // "update" | "delete"
	AtmID    string            `json:"atm_id"`
	Location string            `json:"location"`
	Fields   map[string]string `json:"fields,omitempty"`
}

// InvestigationPayload is the action=investigation.start payload. Topic
// is the investigation subject (returned to the requester in the
// report). ProposedActions is the human-readable list of suggestions
// surfaced in the report. ProposedAction (singular) is the ONE machine-
// executable mutation the investigation wants applied — deferred behind
// a confirmation prompt. When nil, the investigation is report-only (no
// confirmation prompt, no pending action).
type InvestigationPayload struct {
	Topic           string          `json:"topic"`
	ProposedActions []string        `json:"proposed_actions,omitempty"`
	ProposedAction  *ProposedAction `json:"proposed_action,omitempty"`
}

// IssuePayload is the action=issue.open payload per spec §32 issue triggers.
type IssuePayload struct {
	Type        string   `json:"type"`
	Criticality string   `json:"criticality"`
	Title       string   `json:"title"`
	Body        string   `json:"body"`
	Labels      []string `json:"labels"`
}

// EventPayload is the action=event.emit payload per spec §32 event triggers.
type EventPayload struct {
	CloudEventType string         `json:"cloudevent_type"`
	Subject        string         `json:"subject"`
	Data           map[string]any `json:"data"`
}

// ParseReply scans CC stdout for the <<<HERALD-REPLY>>> marker and decodes
// the first JSON object that follows it into a Reply. Action defaults to
// "reply" when omitted from the JSON payload (operator-locked Wave 6
// default).
//
// §107 anchor: no marker → error (no fabricated default reply). Marker
// present but no JSON object → error. Malformed JSON → error.
func ParseReply(stdout []byte) (*Reply, error) {
	const marker = "<<<HERALD-REPLY>>>"
	s := string(stdout)
	idx := strings.Index(s, marker)
	if idx < 0 {
		return nil, errors.New("inbound: no <<<HERALD-REPLY>>> marker in CC stdout")
	}
	after := s[idx+len(marker):]
	brace := strings.Index(after, "{")
	if brace < 0 {
		return nil, errors.New("inbound: <<<HERALD-REPLY>>> marker present but no JSON object follows")
	}
	dec := json.NewDecoder(strings.NewReader(after[brace:]))
	var r Reply
	if err := dec.Decode(&r); err != nil {
		return nil, err
	}
	if r.Action == "" {
		r.Action = "reply"
	}
	return &r, nil
}
