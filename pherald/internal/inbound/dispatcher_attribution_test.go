package inbound_test

import (
	"context"
	"testing"

	"github.com/vasic-digital/herald/commons"
	channels "github.com/vasic-digital/herald/commons_messaging/channels"
	"github.com/vasic-digital/herald/pherald/internal/inbound"
)

// This test reuses the package-level recordingMutator (item_mutator_test.go) —
// the DB-boundary fake that records the exact (atmID, location, fields) map the
// dispatcher built. Asserting lastFields["created_by"]/["assigned_to"] proves
// the attribution wiring reaches the ItemMutator boundary (not a metadata-only
// PASS).

// evWithSender builds an inbound item.update event from a Telegram sender,
// stamping the sender's @username into Raw exactly as tgram.Subscribe does
// (channels.StampSender with IdentityUsername). body is the message text
// (carries any `assign:@x` directive).
func evWithSender(username, chatID, body string) commons.InboundEvent {
	ev := commons.InboundEvent{
		EventID: "01HEVT",
		Sender: commons.Recipient{
			Channel:       "tgram",
			ChannelUserID: chatID,
		},
		Body: commons.Body{Plain: body},
		Raw:  map[string]any{"message_id": 7},
	}
	channels.StampSender(ev.Raw, false, channels.IdentityUsername, username)
	return ev
}

// newAttribDispatcher wires a Dispatcher whose item.update action routes to
// the recording mutator, with a MemoryResolver carrying the operator + a known
// participant @bob. The CC stub returns an item.update reply (system=false:
// no created_by in the payload, so attribution fills it from the sender).
func newAttribDispatcher(t *testing.T, mut inbound.ItemMutator, resolver commons.IdentityResolver, stdout string) *inbound.Dispatcher {
	t.Helper()
	d, err := inbound.NewDispatcher(inbound.Config{
		ProjectName: "ATTRIB",
		Code:        stubCode{stdout: stdout},
		Reply:       &recordingReplier{},
		Items:       mut,
		Resolver:    resolver,
	})
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	return d
}

const itemUpdateStdout = `<<<HERALD-REPLY>>> {"action":"item.update","item_update":{"atm_id":"ATM-238","location":"Issues","fields":{"status":"In progress"}}}`

// systemItemUpdateStdout is the System/Claude-opened path: the LLM payload
// declares created_by="Claude" explicitly, so attribution must leave it as
// "Claude" (never overwrite it with the sender) per §2.
const systemItemUpdateStdout = `<<<HERALD-REPLY>>> {"action":"item.update","item_update":{"atm_id":"ATM-238","location":"Issues","fields":{"status":"In progress","created_by":"Claude"}}}`

func TestInboundAttribution_SubscriberOpen_CreatedByIsSender(t *testing.T) {
	resolver := commons.NewMemoryResolver("@milos85vasic", nil)
	mut := &recordingMutator{}
	d := newAttribDispatcher(t, mut, resolver, itemUpdateStdout)

	// @someuser is an unknown participant → ResolveSender returns the raw
	// normalized @username, so the item is still attributable.
	ev := evWithSender("someuser", "555", "please bump the status")
	if err := d.Handle(context.Background(), ev); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if mut.updateCalls != 1 {
		t.Fatalf("item.update reached the mutator %d times, want 1", mut.updateCalls)
	}
	if got := mut.lastFields["created_by"]; got != "@someuser" {
		t.Errorf("created_by = %q, want %q (the message sender)", got, "@someuser")
	}
	// Default assignee = operator handle.
	if got := mut.lastFields["assigned_to"]; got != "@milos85vasic" {
		t.Errorf("assigned_to = %q, want operator %q", got, "@milos85vasic")
	}
}

func TestInboundAttribution_SystemOpen_CreatedByIsClaude(t *testing.T) {
	resolver := commons.NewMemoryResolver("@milos85vasic", nil)
	mut := &recordingMutator{}
	d := newAttribDispatcher(t, mut, resolver, systemItemUpdateStdout)

	ev := evWithSender("someuser", "555", "claude detected a missing feature")
	if err := d.Handle(context.Background(), ev); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got := mut.lastFields["created_by"]; got != commons.SystemAgentHandle {
		t.Errorf("created_by = %q, want %q (system/Claude-opened item)", got, commons.SystemAgentHandle)
	}
	if got := mut.lastFields["assigned_to"]; got != "@milos85vasic" {
		t.Errorf("assigned_to = %q, want operator %q", got, "@milos85vasic")
	}
}

func TestInboundAttribution_DefaultAssignee_IsOperator(t *testing.T) {
	resolver := commons.NewMemoryResolver("@theoperator", nil)
	mut := &recordingMutator{}
	d := newAttribDispatcher(t, mut, resolver, itemUpdateStdout)

	ev := evWithSender("carol", "777", "no explicit assignment here")
	if err := d.Handle(context.Background(), ev); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got := mut.lastFields["assigned_to"]; got != "@theoperator" {
		t.Errorf("assigned_to = %q, want OperatorHandle() %q", got, "@theoperator")
	}
}

func TestInboundAttribution_ExplicitAssign_OverridesDefault(t *testing.T) {
	resolver := commons.NewMemoryResolver("@milos85vasic", nil)
	mut := &recordingMutator{}
	d := newAttribDispatcher(t, mut, resolver, itemUpdateStdout)

	// `assign:@bob` directive in the body overrides the operator default.
	ev := evWithSender("carol", "777", "looks broken assign:@bob please fix")
	if err := d.Handle(context.Background(), ev); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got := mut.lastFields["assigned_to"]; got != "@bob" {
		t.Errorf("assigned_to = %q, want explicit assignee %q", got, "@bob")
	}
	// created_by is still the sender (carol), unaffected by the assignment.
	if got := mut.lastFields["created_by"]; got != "@carol" {
		t.Errorf("created_by = %q, want sender %q", got, "@carol")
	}
}

// TestInboundAttribution_ParseAssign_SplitForm proves the "assign @bob"
// (space-separated) spelling is also recognised — a wrong parser that only
// matched "assign:@bob" would FAIL this (mutation note: drop the split-form
// branch in parseAssign and this assertion goes red).
func TestInboundAttribution_ParseAssign_SplitForm(t *testing.T) {
	resolver := commons.NewMemoryResolver("@op", nil)
	mut := &recordingMutator{}
	d := newAttribDispatcher(t, mut, resolver, itemUpdateStdout)

	ev := evWithSender("carol", "777", "assign @dave look into it")
	if err := d.Handle(context.Background(), ev); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got := mut.lastFields["assigned_to"]; got != "@dave" {
		t.Errorf("assigned_to = %q, want %q (split-form assign)", got, "@dave")
	}
}
