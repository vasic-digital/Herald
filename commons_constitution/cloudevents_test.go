package constitution

import (
	"errors"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
)

func TestToCloudEvent_RoundTrip(t *testing.T) {
	in := Event{
		ID:      "01HXM-fixture",
		Type:    EventNamespace + ".policy.violation",
		Source:  "digital.vasic.herald/cherald",
		Time:    time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC),
		Subject: "file:/etc/secrets",
		Metadata: map[string]string{
			"rule_id":  "§11.4.10",
			"severity": "critical",
		},
		Data: []byte(`{"envelope":{},"payload":{}}`),
	}

	ce, err := ToCloudEvent(in)
	if err != nil {
		t.Fatalf("ToCloudEvent: %v", err)
	}
	if ce.ID() != in.ID {
		t.Errorf("ID lost: %q vs %q", ce.ID(), in.ID)
	}
	if ce.Type() != in.Type {
		t.Errorf("Type lost: %q vs %q", ce.Type(), in.Type)
	}
	if ce.Source() != in.Source {
		t.Errorf("Source lost: %q vs %q", ce.Source(), in.Source)
	}
	if !ce.Time().Equal(in.Time) {
		t.Errorf("Time lost: %s vs %s", ce.Time(), in.Time)
	}
	if ce.Subject() != in.Subject {
		t.Errorf("Subject lost: %q vs %q", ce.Subject(), in.Subject)
	}
	// CloudEvents v1.0 extension names MUST be lowercase alphanumeric — our
	// ToCloudEvent normalizes underscored keys (e.g. rule_id → ruleid).
	// The round-trip preserves the NORMALIZED form, which is what HTTP
	// peers will see on the wire.
	for k, want := range in.Metadata {
		nk := normalizeExtKey(k)
		got := ce.Extensions()[nk]
		if got != want {
			t.Errorf("extension %q (normalized %q) lost: got %v want %q", k, nk, got, want)
		}
	}

	out, err := FromCloudEvent(ce)
	if err != nil {
		t.Fatalf("FromCloudEvent: %v", err)
	}
	if out.ID != in.ID || out.Type != in.Type || out.Source != in.Source {
		t.Errorf("round-trip lost fields: %+v vs %+v", out, in)
	}
	for k, want := range in.Metadata {
		nk := normalizeExtKey(k)
		if out.Metadata[nk] != want {
			t.Errorf("round-trip lost metadata %q→%q: %q vs %q", k, nk, out.Metadata[nk], want)
		}
	}
	if string(out.Data) != string(in.Data) {
		t.Errorf("round-trip lost Data: %s vs %s", out.Data, in.Data)
	}
}

func TestFromCloudEvent_RejectsBadSpecVersion(t *testing.T) {
	ce := cloudevents.NewEvent()
	ce.SetSpecVersion("0.3") // legacy
	ce.SetID("x")
	ce.SetType("t")
	ce.SetSource("/s")
	_, err := FromCloudEvent(ce)
	if !errors.Is(err, ErrUnsupportedSpec) {
		t.Errorf("FromCloudEvent on v0.3 returned %v; want errors.Is(ErrUnsupportedSpec) == true", err)
	}
}
