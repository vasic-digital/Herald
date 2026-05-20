package null

import (
	"context"
	"testing"

	"github.com/vasic-digital/herald/commons"
)

func TestNullAdapter_SendRecords(t *testing.T) {
	a, err := New("null://?tags=test,prod")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	r, err := a.Send(context.Background(), commons.OutboundMessage{
		EventID:  "abc123",
		TenantID: "t1",
		Body:     commons.Body{Plain: "hello"},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if r.ChannelMsgID != "null-abc123" {
		t.Errorf("ChannelMsgID = %q, want null-abc123", r.ChannelMsgID)
	}
	if r.Evidence != commons.DeliveryRouted {
		t.Errorf("evidence = %v, want Routed", r.Evidence)
	}
	if got := a.Stats()["ok"]; got != 1 {
		t.Errorf("stats[ok] = %d, want 1", got)
	}
	if got := a.TagStats()["test"]; got != 1 {
		t.Errorf("tagStats[test] = %d, want 1", got)
	}
	if got := a.TagStats()["prod"]; got != 1 {
		t.Errorf("tagStats[prod] = %d, want 1", got)
	}
}

func TestNullAdapter_FailRate(t *testing.T) {
	a, err := New("null://?fail_rate=1.0&seed=42")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := a.Send(context.Background(), commons.OutboundMessage{EventID: "x"}); err == nil {
		t.Errorf("expected error with fail_rate=1.0")
	}
	if got := a.Stats()["fail"]; got != 1 {
		t.Errorf("stats[fail] = %d, want 1", got)
	}
}

func TestNullAdapter_Ceiling(t *testing.T) {
	a, err := New("null://?ceiling=Delivered")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a.Capabilities().DeliveryCeiling != commons.DeliveryDelivered {
		t.Errorf("ceiling = %v, want Delivered", a.Capabilities().DeliveryCeiling)
	}
	r, _ := a.Send(context.Background(), commons.OutboundMessage{EventID: "y"})
	if r.Evidence != commons.DeliveryDelivered {
		t.Errorf("evidence = %v, want Delivered", r.Evidence)
	}
}

func TestNullAdapter_RingBufferBounded(t *testing.T) {
	a, _ := New("null://")
	a.cap = 3 // override for test
	for i := 0; i < 5; i++ {
		_, _ = a.Send(context.Background(), commons.OutboundMessage{EventID: string(rune('a' + i))})
	}
	msgs := a.Messages()
	if len(msgs) != 3 {
		t.Errorf("ring size = %d, want 3 (cap)", len(msgs))
	}
	// Oldest should be the 3rd send ("c"); newest the 5th ("e").
	if msgs[0].EventID != "c" || msgs[2].EventID != "e" {
		t.Errorf("ring contents wrong: %+v", msgs)
	}
}

func TestNullAdapter_Clear(t *testing.T) {
	a, _ := New("null://")
	_, _ = a.Send(context.Background(), commons.OutboundMessage{EventID: "1"})
	a.Clear()
	if len(a.Messages()) != 0 {
		t.Errorf("Clear did not empty the ring")
	}
	if a.Stats()["ok"] != 0 {
		t.Errorf("Clear did not reset stats")
	}
}

func TestNullAdapter_InvalidURL(t *testing.T) {
	cases := []string{
		"http://wrong-scheme",
		"null://?fail_rate=2",
		"null://?fail_rate=-1",
		"null://?ceiling=bogus",
		"null://?latency_ms=notanumber",
	}
	for _, c := range cases {
		if _, err := New(c); err == nil {
			t.Errorf("New(%q) succeeded; expected error", c)
		}
	}
}

func TestNullAdapter_HealthCheck(t *testing.T) {
	a, _ := New("null://")
	if err := a.HealthCheck(context.Background()); err != nil {
		t.Errorf("HealthCheck returned %v, want nil", err)
	}
}

func TestNullAdapter_NameAndCapabilities(t *testing.T) {
	a, _ := New("null://")
	if a.Name() != "null" {
		t.Errorf("Name = %q, want null", a.Name())
	}
	caps := a.Capabilities()
	if !caps.Text || !caps.Attachments || !caps.Threads {
		t.Errorf("null:// should advertise text+attachments+threads support")
	}
}
