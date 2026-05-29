package inbound_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/pherald/internal/inbound"
)

// recordingMutator is the DB-boundary fake for ItemMutator. It records
// every Update / Delete call so the router tests can assert exact
// arguments. It is the ONLY stub in these tests — every other path
// (parser, router, pending-action store, confirmation flow) is real
// inbound package code.
type recordingMutator struct {
	updateCalls  int
	deleteCalls  int
	lastAtmID    string
	lastLocation string
	lastFields   map[string]string
	updateErr    error
	deleteErr    error
}

func (m *recordingMutator) Update(_ context.Context, atmID, location string, fields map[string]string) error {
	m.updateCalls++
	m.lastAtmID = atmID
	m.lastLocation = location
	m.lastFields = fields
	return m.updateErr
}

func (m *recordingMutator) Delete(_ context.Context, atmID, location string) error {
	m.deleteCalls++
	m.lastAtmID = atmID
	m.lastLocation = location
	return m.deleteErr
}

// --- Parser tests -----------------------------------------------------

func TestParseReplyItemUpdatePayload(t *testing.T) {
	stdout := `<<<HERALD-REPLY>>> {"action":"item.update","item_update":{"atm_id":"ATM-238","location":"Issues","fields":{"status":"In progress","title":"new title"}}}`
	r, err := inbound.ParseReply([]byte(stdout))
	if err != nil {
		t.Fatal(err)
	}
	if r.Action != "item.update" {
		t.Fatalf("action: got %q want item.update", r.Action)
	}
	if r.ItemUpdate == nil {
		t.Fatalf("ItemUpdate payload nil")
	}
	if r.ItemUpdate.AtmID != "ATM-238" || r.ItemUpdate.Location != "Issues" {
		t.Fatalf("ItemUpdate key wrong: %+v", r.ItemUpdate)
	}
	if r.ItemUpdate.Fields["status"] != "In progress" || r.ItemUpdate.Fields["title"] != "new title" {
		t.Fatalf("ItemUpdate fields wrong: %+v", r.ItemUpdate.Fields)
	}
}

func TestParseReplyItemDeletePayload(t *testing.T) {
	stdout := `<<<HERALD-REPLY>>> {"action":"item.delete","item_delete":{"atm_id":"ATM-7","location":"Fixed"}}`
	r, err := inbound.ParseReply([]byte(stdout))
	if err != nil {
		t.Fatal(err)
	}
	if r.Action != "item.delete" {
		t.Fatalf("action: got %q want item.delete", r.Action)
	}
	if r.ItemDelete == nil {
		t.Fatalf("ItemDelete payload nil")
	}
	if r.ItemDelete.AtmID != "ATM-7" || r.ItemDelete.Location != "Fixed" {
		t.Fatalf("ItemDelete wrong: %+v", r.ItemDelete)
	}
}

func TestParseReplyInvestigationPayload(t *testing.T) {
	stdout := `<<<HERALD-REPLY>>> {"action":"investigation.start","investigation":{"topic":"flaky tgram poll","proposed_actions":["item.update ATM-238 status=In progress","item.delete ATM-9"]}}`
	r, err := inbound.ParseReply([]byte(stdout))
	if err != nil {
		t.Fatal(err)
	}
	if r.Action != "investigation.start" {
		t.Fatalf("action: got %q want investigation.start", r.Action)
	}
	if r.Investigation == nil {
		t.Fatalf("Investigation payload nil")
	}
	if r.Investigation.Topic != "flaky tgram poll" {
		t.Fatalf("Investigation topic wrong: %+v", r.Investigation)
	}
	if len(r.Investigation.ProposedActions) != 2 {
		t.Fatalf("Investigation proposed actions wrong: %+v", r.Investigation.ProposedActions)
	}
}

// --- Router tests -----------------------------------------------------

func newItemEvent(stdout string) (commons.InboundEvent, inbound.Config, *recordingMutator, *recordingReplier) {
	rr := &recordingReplier{}
	rm := &recordingMutator{}
	cfg := inbound.Config{
		ProjectName: "T",
		Code:        stubCode{stdout: stdout},
		Reply:       rr,
		Items:       rm,
		// deterministic-for-test confirmation token
		ConfirmToken: func(string) string { return "TOKEN1" },
	}
	ev := commons.InboundEvent{
		EventID: "01HITEM",
		Sender:  commons.Recipient{Channel: "tgram", ChannelUserID: "12345"},
		Body:    commons.Body{Plain: "x"},
		Raw:     map[string]any{"message_id": 5},
	}
	return ev, cfg, rm, rr
}

func TestRouterItemUpdateCallsMutator(t *testing.T) {
	stdout := `<<<HERALD-REPLY>>> {"action":"item.update","item_update":{"atm_id":"ATM-238","location":"Issues","fields":{"status":"In progress"}}}`
	ev, cfg, rm, _ := newItemEvent(stdout)
	d, err := inbound.NewDispatcher(cfg)
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	if err := d.Handle(context.Background(), ev); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if rm.updateCalls != 1 {
		t.Fatalf("Update called %d times, want 1", rm.updateCalls)
	}
	if rm.lastAtmID != "ATM-238" || rm.lastLocation != "Issues" {
		t.Fatalf("Update key wrong: atmID=%q location=%q", rm.lastAtmID, rm.lastLocation)
	}
	if !reflect.DeepEqual(rm.lastFields, map[string]string{"status": "In progress"}) {
		t.Fatalf("Update fields wrong: %+v", rm.lastFields)
	}
}

func TestRouterItemDeleteCallsMutator(t *testing.T) {
	stdout := `<<<HERALD-REPLY>>> {"action":"item.delete","item_delete":{"atm_id":"ATM-7","location":"Fixed"}}`
	ev, cfg, rm, _ := newItemEvent(stdout)
	d, _ := inbound.NewDispatcher(cfg)
	if err := d.Handle(context.Background(), ev); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if rm.deleteCalls != 1 {
		t.Fatalf("Delete called %d times, want 1", rm.deleteCalls)
	}
	if rm.lastAtmID != "ATM-7" || rm.lastLocation != "Fixed" {
		t.Fatalf("Delete key wrong: atmID=%q location=%q", rm.lastAtmID, rm.lastLocation)
	}
}

func TestRouterItemUpdateMissingMutatorErrors(t *testing.T) {
	rr := &recordingReplier{}
	d, _ := inbound.NewDispatcher(inbound.Config{
		ProjectName: "T",
		Code:        stubCode{stdout: `<<<HERALD-REPLY>>> {"action":"item.update","item_update":{"atm_id":"ATM-1","location":"Issues","fields":{"status":"Queued"}}}`},
		Reply:       rr,
		// Items: nil intentionally
	})
	err := d.Handle(context.Background(), commons.InboundEvent{
		Sender: commons.Recipient{Channel: "tgram", ChannelUserID: "1"},
		Raw:    map[string]any{"message_id": 1},
	})
	if err == nil {
		t.Fatal("want error when ItemMutator nil, got nil")
	}
}

func TestRouterItemUpdateNilPayloadErrors(t *testing.T) {
	ev, cfg, rm, _ := newItemEvent(`<<<HERALD-REPLY>>> {"action":"item.update"}`)
	d, _ := inbound.NewDispatcher(cfg)
	err := d.Handle(context.Background(), ev)
	if err == nil {
		t.Fatal("want error when item_update payload missing, got nil")
	}
	if rm.updateCalls != 0 {
		t.Fatal("mutator MUST NOT fire with nil payload")
	}
}

// --- Regression: existing actions still route correctly ---------------

func TestRouterRegressionExistingActions(t *testing.T) {
	cases := []struct {
		name      string
		stdout    string
		wantReply bool
		wantIssue bool
		wantEvent bool
	}{
		{"reply", `<<<HERALD-REPLY>>> {"action":"reply","text":"hi"}`, true, false, false},
		{"issue.open", `<<<HERALD-REPLY>>> {"action":"issue.open","issue":{"title":"X"}}`, false, true, false},
		{"event.emit", `<<<HERALD-REPLY>>> {"action":"event.emit","event":{"cloudevent_type":"c"}}`, false, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := &recordingReplier{}
			ro := &recordingOpener{}
			re := &recordingEmitter{}
			rm := &recordingMutator{}
			d, err := inbound.NewDispatcher(inbound.Config{
				ProjectName: "T",
				Code:        stubCode{stdout: tc.stdout},
				Reply:       rr,
				Issues:      ro,
				Events:      re,
				Items:       rm,
			})
			if err != nil {
				t.Fatalf("NewDispatcher: %v", err)
			}
			ev := commons.InboundEvent{
				EventID: "01H",
				Sender:  commons.Recipient{Channel: "tgram", ChannelUserID: "12345"},
				Body:    commons.Body{Plain: "ping"},
				Raw:     map[string]any{"message_id": 42},
			}
			if err := d.Handle(context.Background(), ev); err != nil {
				t.Fatalf("Handle: %v", err)
			}
			if rr.called != tc.wantReply || ro.called != tc.wantIssue || re.called != tc.wantEvent {
				t.Fatalf("routing wrong: reply=%v issue=%v event=%v", rr.called, ro.called, re.called)
			}
			if rm.updateCalls != 0 || rm.deleteCalls != 0 {
				t.Fatalf("mutator MUST NOT fire for non-item action; got update=%d delete=%d", rm.updateCalls, rm.deleteCalls)
			}
		})
	}
}

func TestRouterUnknownActionStillErrors(t *testing.T) {
	rr := &recordingReplier{}
	d, _ := inbound.NewDispatcher(inbound.Config{
		ProjectName: "T",
		Code:        stubCode{stdout: `<<<HERALD-REPLY>>> {"action":"weird.thing"}`},
		Reply:       rr,
		Items:       &recordingMutator{},
	})
	err := d.Handle(context.Background(), commons.InboundEvent{
		Sender: commons.Recipient{Channel: "tgram", ChannelUserID: "1"},
		Raw:    map[string]any{"message_id": 1},
	})
	if err == nil || !strings.Contains(err.Error(), "unknown action") {
		t.Fatalf("want 'unknown action' error, got %v", err)
	}
	if rr.called {
		t.Fatal("§107: replier MUST NOT fire on unknown action")
	}
}

// --- Investigation / confirmation flow --------------------------------

// TestInvestigationProposedMutationDeferred — an investigation.start with
// a proposed mutating action MUST NOT call the mutator immediately. It
// MUST reply with a confirmation prompt carrying the deterministic token.
func TestInvestigationProposedMutationDeferred(t *testing.T) {
	stdout := `<<<HERALD-REPLY>>> {"action":"investigation.start","investigation":{"topic":"stale row","proposed_actions":["item.delete ATM-9 from Fixed"]}}`
	ev, cfg, rm, rr := newItemEvent(stdout)
	// Make the proposed action a real, executable mutation.
	cfg.Code = stubCode{stdout: `<<<HERALD-REPLY>>> {"action":"investigation.start","investigation":{"topic":"stale row","proposed_action":{"kind":"delete","atm_id":"ATM-9","location":"Fixed"}}}`}
	d, err := inbound.NewDispatcher(cfg)
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	if err := d.Handle(context.Background(), ev); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	// The mutator MUST NOT have fired yet — act-with-confirmation.
	if rm.deleteCalls != 0 || rm.updateCalls != 0 {
		t.Fatalf("§ACT-WITH-CONFIRMATION: mutator fired during investigation; update=%d delete=%d", rm.updateCalls, rm.deleteCalls)
	}
	if !rr.called {
		t.Fatal("investigation MUST reply with a report/confirmation prompt")
	}
	if !strings.Contains(rr.lastText, "CONFIRM TOKEN1") {
		t.Fatalf("confirmation prompt missing token; got %q", rr.lastText)
	}
}

// TestInvestigationConfirmExecutesPendingMutation — after the deferred
// proposal, a subsequent "CONFIRM <token>" message MUST execute the
// pending mutation (the mutator fires exactly once, with the proposed
// args).
func TestInvestigationConfirmExecutesPendingMutation(t *testing.T) {
	rr := &recordingReplier{}
	rm := &recordingMutator{}
	cfg := inbound.Config{
		ProjectName:  "T",
		Reply:        rr,
		Items:        rm,
		ConfirmToken: func(string) string { return "TOKEN1" },
	}
	sender := commons.Recipient{Channel: "tgram", ChannelUserID: "12345"}

	// Step 1: investigation proposes a delete (deferred).
	cfg.Code = stubCode{stdout: `<<<HERALD-REPLY>>> {"action":"investigation.start","investigation":{"topic":"t","proposed_action":{"kind":"delete","atm_id":"ATM-9","location":"Fixed"}}}`}
	d, err := inbound.NewDispatcher(cfg)
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	ev1 := commons.InboundEvent{
		EventID: "01HINV1",
		Sender:  sender,
		Body:    commons.Body{Plain: "Investigation: please clean stale rows"},
		Raw:     map[string]any{"message_id": 1},
	}
	if err := d.Handle(context.Background(), ev1); err != nil {
		t.Fatalf("Handle investigation: %v", err)
	}
	if rm.deleteCalls != 0 {
		t.Fatal("mutator fired before confirm")
	}

	// Step 2: operator confirms. This message must NOT go through CC —
	// it's a confirm fast-path. The mutator fires now.
	ev2 := commons.InboundEvent{
		EventID: "01HINV2",
		Sender:  sender,
		Body:    commons.Body{Plain: "CONFIRM TOKEN1"},
		Raw:     map[string]any{"message_id": 2},
	}
	if err := d.Handle(context.Background(), ev2); err != nil {
		t.Fatalf("Handle confirm: %v", err)
	}
	if rm.deleteCalls != 1 {
		t.Fatalf("§ACT-WITH-CONFIRMATION: pending delete not executed on confirm; deleteCalls=%d", rm.deleteCalls)
	}
	if rm.lastAtmID != "ATM-9" || rm.lastLocation != "Fixed" {
		t.Fatalf("confirmed mutation args wrong: atmID=%q location=%q", rm.lastAtmID, rm.lastLocation)
	}
}

// TestConfirmUnknownTokenErrors — a CONFIRM with no matching pending
// action must NOT silently succeed (§107: no fabricated success).
func TestConfirmUnknownTokenErrors(t *testing.T) {
	rr := &recordingReplier{}
	rm := &recordingMutator{}
	d, _ := inbound.NewDispatcher(inbound.Config{
		ProjectName:  "T",
		Code:         stubCode{stdout: `<<<HERALD-REPLY>>> {"action":"reply","text":"never"}`},
		Reply:        rr,
		Items:        rm,
		ConfirmToken: func(string) string { return "TOKEN1" },
	})
	ev := commons.InboundEvent{
		Sender: commons.Recipient{Channel: "tgram", ChannelUserID: "12345"},
		Body:   commons.Body{Plain: "CONFIRM NONEXISTENT"},
		Raw:    map[string]any{"message_id": 1},
	}
	err := d.Handle(context.Background(), ev)
	if err == nil {
		t.Fatal("want error for unknown confirm token, got nil")
	}
	if rm.deleteCalls != 0 || rm.updateCalls != 0 {
		t.Fatal("mutator MUST NOT fire on unknown confirm token")
	}
}
