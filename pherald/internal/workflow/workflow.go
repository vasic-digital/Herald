// Package workflow is the change→CloudEvent→notification bridge (WS-3 /
// HRD-151). It turns the per-property workable-item deltas produced by
// commons_workable.Diff into CloudEvents and Jira/ClickUp-style human diff
// messages, then fans them out through pherald's existing outbound Stage-6
// ChannelDispatcher — it does NOT reimplement fan-out.
//
// Three components:
//
//   - ChangesToEvents — one commons.CloudEventEnvelope per Change, ce-type
//     namespaced under digital.vasic.herald.workable.<kind>, subject
//     item:<atm_id>, JSON body {atm_id, location, field, old, new}.
//   - RenderChange — deterministic one-line diff message per Change.
//   - Notifier — renders each Change and feeds the message through the real
//     runner.ChannelDispatcher so it reaches every recipient's channel.
//
// Per §107 anti-bluff: the Notifier drives the production dispatch code path
// (runner.ChannelDispatcher.Process → commons.Channel.Send), proven by a
// recording commons.Channel sink in workflow_test.go — no mock of the bridge
// itself.
package workflow

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/vasic-digital/herald/commons"
	workable "github.com/vasic-digital/herald/commons_workable"
	"github.com/vasic-digital/herald/pherald/internal/runner"
)

// ceTypePrefix is the reverse-DNS namespace for workable-item CloudEvents.
// The Change.Kind ("item.status.changed", …) is appended verbatim so the
// full type is e.g. "digital.vasic.herald.workable.item.status.changed".
const ceTypePrefix = "digital.vasic.herald.workable."

// changeBody is the JSON payload carried in CloudEventEnvelope.Data.
type changeBody struct {
	AtmID    string `json:"atm_id"`
	Location string `json:"location"`
	Field    string `json:"field"`
	Old      string `json:"old"`
	New      string `json:"new"`
}

// ChangesToEvents maps each Change to one CloudEvent. Order is preserved
// 1:1 with the input slice (commons_workable.Diff already emits a
// deterministic order). Each event gets a fresh UUIDv7 id, the namespaced
// ce-type, subject "item:<atm_id>", and a JSON body carrying the delta.
func ChangesToEvents(changes []workable.Change) []commons.CloudEventEnvelope {
	events := make([]commons.CloudEventEnvelope, 0, len(changes))
	for _, c := range changes {
		data, _ := json.Marshal(changeBody{
			AtmID:    c.AtmID,
			Location: c.Location,
			Field:    c.Field,
			Old:      c.Old,
			New:      c.New,
		})
		events = append(events, commons.CloudEventEnvelope{
			SpecVersion:     "1.0",
			ID:              commons.MustUUIDv7().String(),
			Source:          "herald/commons_workable",
			Type:            ceTypePrefix + c.Kind,
			Subject:         "item:" + c.AtmID,
			DataContentType: "application/json",
			Data:            data,
		})
	}
	return events
}

// RenderChange returns a deterministic, single-line Jira/ClickUp-style diff
// message for one Change. Unknown Kinds fall back to a generic line so the
// renderer never panics or returns "".
func RenderChange(c workable.Change) string {
	switch c.Kind {
	case workable.KindCreated:
		return "🆕 " + c.AtmID + " created"
	case workable.KindDeleted:
		return "🗑️ " + c.AtmID + " removed"
	case workable.KindStatusChanged:
		return "🔄 " + c.AtmID + " status: " + c.Old + " → " + c.New
	case workable.KindFieldChanged:
		return "✏️ " + c.AtmID + " " + c.Field + ": " + c.Old + " → " + c.New
	case workable.KindContentUpdated:
		return "📝 " + c.AtmID + " content updated"
	case workable.KindRelocated:
		return "📦 " + c.AtmID + " moved: " + c.Old + " → " + c.New
	default:
		return c.AtmID + " " + c.Kind
	}
}

// Notifier fans workable-item changes out to subscribers' channels by
// rendering each change and feeding it through pherald's existing Stage-6
// ChannelDispatcher. It owns NO fan-out logic of its own — the dispatcher
// is the single source of truth for per-recipient delivery + evidence.
type Notifier struct {
	dispatcher *runner.ChannelDispatcher
	recipients []commons.Recipient
}

// NewNotifier wires a Notifier to an existing ChannelDispatcher and an
// explicit recipient list. The explicit recipients deliberately bypass the
// PG-backed runner.SubscriberResolver — the full PG subscriber-resolution
// e2e is HRD-156 (WS-5). Everything downstream of the recipient list is the
// production dispatcher, exercised verbatim.
func NewNotifier(dispatcher *runner.ChannelDispatcher, recipients []commons.Recipient) *Notifier {
	return &Notifier{dispatcher: dispatcher, recipients: recipients}
}

// Notify renders each change and dispatches it to every recipient through
// the real ChannelDispatcher. One dispatch pass per change so each rendered
// message is a distinct outbound message (matching the per-change CloudEvent
// from ChangesToEvents). Returns the first dispatcher error, if any.
func (n *Notifier) Notify(ctx context.Context, changes []workable.Change) error {
	events := ChangesToEvents(changes)
	for i, c := range changes {
		// Carry the rendered diff text as the event payload so the real
		// ChannelDispatcher (which reads rc.Event.Data into Body.Plain)
		// fans out exactly the human message we rendered.
		rc := &runner.RunCtx{
			Event:      events[i],
			Recipients: n.recipients,
		}
		rc.Event.Data = []byte(RenderChange(c))
		if err := n.dispatcher.Process(ctx, rc); err != nil {
			return fmt.Errorf("workflow: dispatch change %d (%s %s): %w", i, c.AtmID, c.Kind, err)
		}
		// §107 anti-bluff (C1): ChannelDispatcher.Process records each
		// per-recipient failure (unregistered channel / Channel.Send error)
		// into rc.Receipts with Evidence=Unknown + Error and returns nil — the
		// FULL Runner persists those at Stage 7 (OutcomeRecorder) for the audit
		// trail. The Notifier has NO Stage 7, so it MUST inspect the receipts
		// itself: a failed delivery is a real undelivered notification the
		// operator would otherwise silently miss. Fail loud on the first one.
		for _, r := range rc.Receipts {
			if r.Error != "" || r.Evidence == commons.DeliveryUnknown {
				return fmt.Errorf("workflow: change %d (%s %s) NOT delivered to %s/%s: evidence=%s error=%q",
					i, c.AtmID, c.Kind, r.ChannelID, r.ChannelUserID, r.Evidence, r.Error)
			}
		}
	}
	return nil
}
