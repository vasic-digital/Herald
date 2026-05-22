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
	Action string        `json:"action"` // "reply" (default) | "issue.open" | "event.emit"
	Text   string        `json:"text"`   // body for action=reply
	Issue  *IssuePayload `json:"issue,omitempty"`
	Event  *EventPayload `json:"event,omitempty"`
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
