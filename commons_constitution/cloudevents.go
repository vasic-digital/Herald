package constitution

import (
	"errors"
	"fmt"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
)

// normalizeExtKey converts an internal metadata key (e.g. "rule_id")
// to a CloudEvents v1.0 compliant extension name (e.g. "ruleid").
// CE v1.0 §3 attribute names: "MUST consist of lower-case letters,
// upper-case letters, or digits from the ASCII character set."
func normalizeExtKey(k string) string {
	// Strip underscores + hyphens + dots; lowercase.
	out := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		case r >= '0' && r <= '9':
			return r
		default:
			return -1
		}
	}, k)
	return out
}

// ToCloudEvent translates an in-process Event to a CNCF CloudEvent v1.0
// envelope for HTTP egress. The Metadata map becomes CloudEvent extensions
// keyed identically.
func ToCloudEvent(ev Event) (cloudevents.Event, error) {
	ce := cloudevents.NewEvent()
	ce.SetSpecVersion(cloudevents.VersionV1)
	ce.SetID(ev.ID)
	ce.SetType(ev.Type)
	ce.SetSource(ev.Source)
	if !ev.Time.IsZero() {
		ce.SetTime(ev.Time)
	}
	if ev.Subject != "" {
		ce.SetSubject(ev.Subject)
	}
	for k, v := range ev.Metadata {
		ce.SetExtension(normalizeExtKey(k), v)
	}
	if err := ce.SetData(cloudevents.ApplicationJSON, ev.Data); err != nil {
		return cloudevents.Event{}, fmt.Errorf("constitution: ToCloudEvent: SetData: %w", err)
	}
	if err := ce.Validate(); err != nil {
		return cloudevents.Event{}, fmt.Errorf("constitution: ToCloudEvent: validate: %w", err)
	}
	return ce, nil
}

// FromCloudEvent translates a CNCF CloudEvent v1.0 to an in-process Event.
// Validates spec-version + required attributes. Returns ErrUnsupportedSpec
// for unrecognised SpecVersion values.
func FromCloudEvent(ce cloudevents.Event) (Event, error) {
	if ce.SpecVersion() != cloudevents.VersionV1 {
		return Event{}, fmt.Errorf("constitution: FromCloudEvent: %w (got %q)", ErrUnsupportedSpec, ce.SpecVersion())
	}
	if err := ce.Validate(); err != nil {
		return Event{}, fmt.Errorf("constitution: FromCloudEvent: validate: %w", err)
	}
	out := Event{
		ID:       ce.ID(),
		Type:     ce.Type(),
		Source:   ce.Source(),
		Subject:  ce.Subject(),
		Data:     ce.Data(),
		Time:     ce.Time(),
		Metadata: make(map[string]string),
	}
	for k, v := range ce.Extensions() {
		if s, ok := v.(string); ok {
			out.Metadata[k] = s
		} else {
			out.Metadata[k] = fmt.Sprintf("%v", v)
		}
	}
	return out, nil
}

// ErrUnsupportedSpec is returned by FromCloudEvent when the SpecVersion
// is not CloudEvents v1.0.
var ErrUnsupportedSpec = errors.New("unsupported CloudEvents SpecVersion")
