package runner

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/vasic-digital/herald/commons"
)

// EventParser parses raw HTTP body bytes into a commons.CloudEventEnvelope
// (Herald's typed projection of cloudevents v1.0). Supports the
// "structured" content mode where the entire envelope is in the body
// as JSON.
//
// Per §107: returns an error on malformed JSON or missing required
// fields (id, source, type). An empty CloudEvent past parsing would
// be a §11.4 bluff (downstream stages would silently no-op).
//
// Binary mode (envelope metadata in HTTP headers, payload in body) is
// also supported when the HTTP handler propagates headers into the
// Raw payload — Wave 3b uses structured mode by default; binary mode
// detection is a follow-up.
type EventParser struct{}

// structuredCloudEvent mirrors the JSON wire format Herald accepts.
// Field tags are CloudEvents 1.0 canonical names; "heraldidempotencykey"
// is the Herald extension carrying an explicit idempotency key (per
// spec §32.2).
type structuredCloudEvent struct {
	SpecVersion          string          `json:"specversion"`
	ID                   string          `json:"id"`
	Source               string          `json:"source"`
	Type                 string          `json:"type"`
	Subject              string          `json:"subject,omitempty"`
	Time                 string          `json:"time,omitempty"`
	DataContentType      string          `json:"datacontenttype,omitempty"`
	Data                 json.RawMessage `json:"data,omitempty"`
	HeraldIdempotencyKey string          `json:"heraldidempotencykey,omitempty"`
	HeraldTenant         string          `json:"heraldtenant,omitempty"`
	HeraldPriority       string          `json:"heraldpriority,omitempty"`
}

func (p *EventParser) Process(ctx context.Context, rc *RunCtx) error {
	if len(rc.Raw) == 0 {
		return fmt.Errorf("event_parser: empty body")
	}
	var s structuredCloudEvent
	if err := json.Unmarshal(rc.Raw, &s); err != nil {
		return fmt.Errorf("event_parser: malformed JSON: %w", err)
	}
	if s.ID == "" {
		return fmt.Errorf("event_parser: missing required field 'id'")
	}
	if s.Source == "" {
		return fmt.Errorf("event_parser: missing required field 'source'")
	}
	if s.Type == "" {
		return fmt.Errorf("event_parser: missing required field 'type'")
	}
	rc.Event = commons.CloudEventEnvelope{
		SpecVersion:     s.SpecVersion,
		ID:              s.ID,
		Source:          s.Source,
		Type:            s.Type,
		Subject:         s.Subject,
		DataContentType: s.DataContentType,
		Data:            []byte(s.Data),
		Extensions: map[string]string{
			"heraldidempotencykey": s.HeraldIdempotencyKey,
			"heraldtenant":         s.HeraldTenant,
			"heraldpriority":       s.HeraldPriority,
		},
	}
	// Derive idempotency key: explicit > event_id.
	if s.HeraldIdempotencyKey != "" {
		rc.IdemKey = s.HeraldIdempotencyKey
	} else {
		rc.IdemKey = s.ID
	}
	return nil
}
