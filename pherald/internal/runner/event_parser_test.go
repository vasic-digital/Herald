package runner

import (
	"context"
	"strings"
	"testing"
)

func TestEventParser_StructuredMode_HappyPath(t *testing.T) {
	body := `{
		"specversion":"1.0",
		"id":"01923456-789a-7bcd-abcd-ef0123456789",
		"source":"//test/source",
		"type":"digital.vasic.herald.test",
		"datacontenttype":"application/json",
		"time":"2026-05-22T12:00:00Z",
		"data":{"hello":"world"}
	}`
	rc := &RunCtx{Raw: []byte(body)}
	p := &EventParser{}
	if err := p.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process error: %v", err)
	}
	if rc.Event.ID != "01923456-789a-7bcd-abcd-ef0123456789" {
		t.Errorf("Event.ID = %q", rc.Event.ID)
	}
	if rc.Event.Type != "digital.vasic.herald.test" {
		t.Errorf("Event.Type = %q", rc.Event.Type)
	}
	if rc.IdemKey == "" {
		t.Errorf("IdemKey empty — should derive from EventID")
	}
	if !strings.Contains(string(rc.Event.Data), "hello") {
		t.Errorf("Event.Data lost: %q", rc.Event.Data)
	}
}

func TestEventParser_MalformedJSON_Errors(t *testing.T) {
	rc := &RunCtx{Raw: []byte("not json")}
	p := &EventParser{}
	if err := p.Process(context.Background(), rc); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestEventParser_MissingRequiredField_Errors(t *testing.T) {
	body := `{"specversion":"1.0","id":"abc","source":"//x"}` // missing "type"
	rc := &RunCtx{Raw: []byte(body)}
	p := &EventParser{}
	if err := p.Process(context.Background(), rc); err == nil {
		t.Fatal("expected error for missing 'type' field")
	}
}

func TestEventParser_ExplicitIdempotencyKey_Honored(t *testing.T) {
	body := `{
		"specversion":"1.0",
		"id":"01923456-789a-7bcd-abcd-ef0123456789",
		"source":"//s","type":"x",
		"heraldidempotencykey":"explicit-key-42"
	}`
	rc := &RunCtx{Raw: []byte(body)}
	p := &EventParser{}
	if err := p.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if rc.IdemKey != "explicit-key-42" {
		t.Errorf("IdemKey = %q, want 'explicit-key-42' from heraldidempotencykey extension", rc.IdemKey)
	}
}
