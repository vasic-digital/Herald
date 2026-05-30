package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/vasic-digital/herald/commons"
	workable "github.com/vasic-digital/herald/commons_workable"
	"github.com/vasic-digital/herald/pherald/internal/runner"
)

// recordingChannel is a real commons.Channel implementation (NOT a mock of
// the unit under test) that records every OutboundMessage it receives. It is
// the recording sink used to prove the bridge→format→dispatch path runs
// end-to-end through the real ChannelDispatcher.Send code, not stubbed-out.
type recordingChannel struct {
	mu       sync.Mutex
	received []commons.OutboundMessage
}

func (c *recordingChannel) Name() string                       { return string(commons.ChannelNull) }
func (c *recordingChannel) Capabilities() commons.Capabilities { return commons.Capabilities{Text: true} }
func (c *recordingChannel) HealthCheck(ctx context.Context) error {
	return nil
}
func (c *recordingChannel) Subscribe(ctx context.Context, h commons.InboundHandler) error {
	<-ctx.Done()
	return ctx.Err()
}
func (c *recordingChannel) Send(ctx context.Context, msg commons.OutboundMessage) (commons.Receipt, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.received = append(c.received, msg)
	return commons.Receipt{Evidence: commons.DeliveryRouted, ChannelMsgID: "rec-" + msg.EventID}, nil
}
func (c *recordingChannel) bodies() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.received))
	for i, m := range c.received {
		out[i] = m.Body.Plain
	}
	return out
}

func TestChangesToEvents(t *testing.T) {
	t.Parallel()
	changes := []workable.Change{
		{AtmID: "ATM-238", Location: "Issues", Kind: workable.KindCreated},
		{AtmID: "ATM-238", Location: "Issues", Kind: workable.KindStatusChanged, Field: "status", Old: "In progress", New: "Ready for testing"},
		{AtmID: "ATM-9", Location: "Issues", Kind: workable.KindFieldChanged, Field: "severity", Old: "Critical", New: "Medium"},
		{AtmID: "ATM-9", Location: "Fixed", Kind: workable.KindContentUpdated, Field: "body_md", Old: "a", New: "b"},
		{AtmID: "ATM-1", Location: "Fixed", Kind: workable.KindDeleted},
	}
	events := ChangesToEvents(changes)
	if len(events) != len(changes) {
		t.Fatalf("ChangesToEvents: want %d events, got %d", len(changes), len(events))
	}

	wantTypes := []string{
		"digital.vasic.herald.workable.item.created",
		"digital.vasic.herald.workable.item.status.changed",
		"digital.vasic.herald.workable.item.field.changed",
		"digital.vasic.herald.workable.item.content.updated",
		"digital.vasic.herald.workable.item.deleted",
	}
	wantSubjects := []string{"item:ATM-238", "item:ATM-238", "item:ATM-9", "item:ATM-9", "item:ATM-1"}

	for i, ev := range events {
		if ev.Type != wantTypes[i] {
			t.Errorf("event[%d].Type = %q, want %q", i, ev.Type, wantTypes[i])
		}
		if ev.Subject != wantSubjects[i] {
			t.Errorf("event[%d].Subject = %q, want %q", i, ev.Subject, wantSubjects[i])
		}
		if ev.ID == "" {
			t.Errorf("event[%d].ID is empty, want a UUIDv7", i)
		}
		if ev.DataContentType != "application/json" {
			t.Errorf("event[%d].DataContentType = %q, want application/json", i, ev.DataContentType)
		}
		var body struct {
			AtmID    string `json:"atm_id"`
			Location string `json:"location"`
			Field    string `json:"field"`
			Old      string `json:"old"`
			New      string `json:"new"`
		}
		if err := json.Unmarshal(ev.Data, &body); err != nil {
			t.Fatalf("event[%d].Data not valid JSON: %v", i, err)
		}
		c := changes[i]
		if body.AtmID != c.AtmID || body.Location != c.Location || body.Field != c.Field || body.Old != c.Old || body.New != c.New {
			t.Errorf("event[%d].Data = %+v, want fields from %+v", i, body, c)
		}
	}
}

func TestRenderChange(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   workable.Change
		want string
	}{
		{"created", workable.Change{AtmID: "ATM-238", Kind: workable.KindCreated}, "🆕 ATM-238 created"},
		{"deleted", workable.Change{AtmID: "ATM-238", Kind: workable.KindDeleted}, "🗑️ ATM-238 removed"},
		{"status", workable.Change{AtmID: "ATM-238", Kind: workable.KindStatusChanged, Field: "status", Old: "In progress", New: "Ready for testing"}, "🔄 ATM-238 status: In progress → Ready for testing"},
		{"field", workable.Change{AtmID: "ATM-238", Kind: workable.KindFieldChanged, Field: "severity", Old: "Critical", New: "Medium"}, "✏️ ATM-238 severity: Critical → Medium"},
		{"content", workable.Change{AtmID: "ATM-238", Kind: workable.KindContentUpdated, Field: "body_md"}, "📝 ATM-238 content updated"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderChange(tc.in)
			if got != tc.want {
				t.Errorf("RenderChange(%s) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestNotifier_FeedsRealDispatcher(t *testing.T) {
	t.Parallel()
	// Recording sink wired into the REAL runner.ChannelDispatcher — the
	// actual Stage-6 fan-out code path. We bypass the PG SubscriberResolver
	// by supplying explicit recipients (the full PG-backed subscriber
	// resolution path is HRD-156 / WS-5's e2e); everything downstream of the
	// recipient list — message build + per-recipient channel.Send — is the
	// production dispatcher, exercised verbatim.
	rec := &recordingChannel{}
	dispatcher := &runner.ChannelDispatcher{
		Channels: map[commons.ChannelID]commons.Channel{commons.ChannelNull: rec},
		Logger:   slog.Default(),
	}
	recipients := []commons.Recipient{{Channel: string(commons.ChannelNull), ChannelUserID: "chat-1", DisplayName: "QA"}}

	notifier := NewNotifier(dispatcher, recipients)

	changes := []workable.Change{
		{AtmID: "ATM-238", Kind: workable.KindCreated},
		{AtmID: "ATM-238", Kind: workable.KindStatusChanged, Field: "status", Old: "In progress", New: "Ready for testing"},
		{AtmID: "ATM-9", Kind: workable.KindFieldChanged, Field: "severity", Old: "Critical", New: "Medium"},
	}

	if err := notifier.Notify(context.Background(), changes); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	got := rec.bodies()
	want := []string{
		"🆕 ATM-238 created",
		"🔄 ATM-238 status: In progress → Ready for testing",
		"✏️ ATM-9 severity: Critical → Medium",
	}
	if len(got) != len(want) {
		t.Fatalf("recording channel received %d messages, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("dispatched message[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// faultingChannel is a real commons.Channel whose Send always fails — proves
// the Notifier surfaces a transport failure (C1 / §107) rather than silently
// swallowing it (the watch path has no Stage-7 OutcomeRecorder to persist the
// failure receipt, so the Notifier itself must fail loud).
type faultingChannel struct{ err error }

func (c *faultingChannel) Name() string                       { return string(commons.ChannelNull) }
func (c *faultingChannel) Capabilities() commons.Capabilities { return commons.Capabilities{Text: true} }
func (c *faultingChannel) HealthCheck(ctx context.Context) error {
	return nil
}
func (c *faultingChannel) Subscribe(ctx context.Context, h commons.InboundHandler) error {
	<-ctx.Done()
	return ctx.Err()
}
func (c *faultingChannel) Send(ctx context.Context, msg commons.OutboundMessage) (commons.Receipt, error) {
	return commons.Receipt{}, c.err
}

func TestNotifier_SurfacesSendFailure(t *testing.T) {
	t.Parallel()
	fault := &faultingChannel{err: errors.New("telegram 502 bad gateway")}
	dispatcher := &runner.ChannelDispatcher{
		Channels: map[commons.ChannelID]commons.Channel{commons.ChannelNull: fault},
		Logger:   slog.Default(),
	}
	recipients := []commons.Recipient{{Channel: string(commons.ChannelNull), ChannelUserID: "chat-1"}}
	notifier := NewNotifier(dispatcher, recipients)

	err := notifier.Notify(context.Background(), []workable.Change{{AtmID: "ATM-7", Kind: workable.KindCreated}})
	if err == nil {
		t.Fatal("Notify swallowed a real Send failure — §107 distribution-layer bluff (C1): the operator would silently miss the workable-item notification")
	}
	if !strings.Contains(err.Error(), "ATM-7") {
		t.Errorf("Notify error should identify the undelivered change (ATM-7); got: %v", err)
	}
}

func TestNotifier_SurfacesUnregisteredChannel(t *testing.T) {
	t.Parallel()
	// A recipient whose channel isn't registered MUST surface as an error,
	// not a silent drop (the dispatcher records Evidence=Unknown; the Notifier
	// must escalate it since there is no Stage-7 to persist it).
	dispatcher := &runner.ChannelDispatcher{
		Channels: map[commons.ChannelID]commons.Channel{},
		Logger:   slog.Default(),
	}
	recipients := []commons.Recipient{{Channel: "tgram", ChannelUserID: "chat-1"}}
	notifier := NewNotifier(dispatcher, recipients)

	if err := notifier.Notify(context.Background(), []workable.Change{{AtmID: "ATM-8", Kind: workable.KindCreated}}); err == nil {
		t.Fatal("Notify swallowed an unregistered-channel recipient — §107 bluff")
	}
}
