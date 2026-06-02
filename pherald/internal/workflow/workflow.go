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
	"github.com/vasic-digital/herald/commons_messaging/channels/slack"
	"github.com/vasic-digital/herald/commons_messaging/channels/tgram"
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

// AttributionFunc returns the (created_by, assigned_to) canonical handles for a
// workable item identified by (atmID, location). The Notifier uses it to drive
// the PARTICIPANT_ATTRIBUTION §3 tagging matrix per change. Returning ("", "")
// (the zero value of an unset func) means "no attribution known" → no mentions.
type AttributionFunc func(atmID, location string) (createdBy, assignedTo string)

// Notifier fans workable-item changes out to subscribers' channels by
// rendering each change and feeding it through pherald's existing Stage-6
// ChannelDispatcher. It owns NO fan-out logic of its own — the dispatcher
// is the single source of truth for per-recipient delivery + evidence.
//
// PARTICIPANT_ATTRIBUTION §3/§5 (outbound tagging): when resolver + attribution
// are wired, each dispatched message body is prefixed with the resolved
// @username(s) for the participants who must be aware of the change (the
// assignee and/or the opener, never the operator, never "Claude"), per the §3
// matrix. Tagging is per-channel: a participant with no alias on the target
// channel is skipped (you cannot tag someone not on that messenger).
type Notifier struct {
	dispatcher  *runner.ChannelDispatcher
	recipients  []commons.Recipient
	resolver    commons.IdentityResolver
	operator    string
	attribution AttributionFunc
}

// NewNotifier wires a Notifier to an existing ChannelDispatcher and an
// explicit recipient list. The explicit recipients deliberately bypass the
// PG-backed runner.SubscriberResolver — the full PG subscriber-resolution
// e2e is HRD-156 (WS-5). Everything downstream of the recipient list is the
// production dispatcher, exercised verbatim.
//
// This constructor leaves tagging OFF (resolver nil) — bodies are dispatched
// verbatim (Wave 6 behaviour). Use NewTaggingNotifier to enable §3 tagging.
func NewNotifier(dispatcher *runner.ChannelDispatcher, recipients []commons.Recipient) *Notifier {
	return &Notifier{dispatcher: dispatcher, recipients: recipients}
}

// NewTaggingNotifier wires a Notifier with PARTICIPANT_ATTRIBUTION §3 outbound
// tagging enabled. resolver resolves canonical handles → per-channel @usernames;
// operator is the canonical operator handle (never tagged); attribution maps a
// change's (atmID, location) to the item's (created_by, assigned_to). A nil
// resolver or nil attribution disables tagging (bodies dispatched verbatim).
func NewTaggingNotifier(dispatcher *runner.ChannelDispatcher, recipients []commons.Recipient, resolver commons.IdentityResolver, operator string, attribution AttributionFunc) *Notifier {
	return &Notifier{
		dispatcher:  dispatcher,
		recipients:  recipients,
		resolver:    resolver,
		operator:    operator,
		attribution: attribution,
	}
}

// Notify renders each change and dispatches it to every recipient through
// the real ChannelDispatcher. One dispatch pass per (change, channel) group so
// each rendered message is a distinct outbound message (matching the per-change
// CloudEvent from ChangesToEvents) AND carries the channel-correct
// @-mention prefix (§3 tagging: a mention is rendered to the recipient
// channel's @username, and a participant with no alias on that channel is
// skipped). Returns the first dispatcher error, if any.
func (n *Notifier) Notify(ctx context.Context, changes []workable.Change) error {
	events := ChangesToEvents(changes)
	byChannel := n.recipientsByChannel()
	for i, c := range changes {
		base := RenderChange(c)
		for _, channel := range byChannel.order {
			recipients := byChannel.m[channel]
			// §3 tagging: prefix the body with the resolved @usernames for the
			// participants who must be aware of this change on THIS channel.
			body := n.tag(base, c, channel)
			rc := &runner.RunCtx{
				Event:      events[i],
				Recipients: recipients,
			}
			// Carry the (possibly tagged) rendered diff text as the event
			// payload so the real ChannelDispatcher (which reads rc.Event.Data
			// into Body.Plain) fans out exactly the human message we rendered.
			rc.Event.Data = []byte(body)
			if err := n.dispatcher.Process(ctx, rc); err != nil {
				return fmt.Errorf("workflow: dispatch change %d (%s %s) on %s: %w", i, c.AtmID, c.Kind, channel, err)
			}
			// §107 anti-bluff (C1): ChannelDispatcher.Process records each
			// per-recipient failure (unregistered channel / Channel.Send error)
			// into rc.Receipts with Evidence=Unknown + Error and returns nil —
			// the FULL Runner persists those at Stage 7 (OutcomeRecorder) for
			// the audit trail. The Notifier has NO Stage 7, so it MUST inspect
			// the receipts itself: a failed delivery is a real undelivered
			// notification the operator would otherwise silently miss. Fail
			// loud on the first one.
			for _, r := range rc.Receipts {
				if r.Error != "" || r.Evidence == commons.DeliveryUnknown {
					return fmt.Errorf("workflow: change %d (%s %s) NOT delivered to %s/%s: evidence=%s error=%q",
						i, c.AtmID, c.Kind, r.ChannelID, r.ChannelUserID, r.Evidence, r.Error)
				}
			}
		}
	}
	return nil
}

// channelGroups is recipients bucketed by channel, with a stable iteration
// order (first-seen) so per-change dispatch is deterministic.
type channelGroups struct {
	m     map[string][]commons.Recipient
	order []string
}

// recipientsByChannel buckets n.recipients by their Channel, preserving
// first-seen order.
func (n *Notifier) recipientsByChannel() channelGroups {
	g := channelGroups{m: make(map[string][]commons.Recipient)}
	for _, r := range n.recipients {
		if _, ok := g.m[r.Channel]; !ok {
			g.order = append(g.order, r.Channel)
		}
		g.m[r.Channel] = append(g.m[r.Channel], r)
	}
	return g
}

// tag prefixes body with the §3 @-mentions for change c on the target channel.
// When tagging is disabled (nil resolver / attribution) or the item has no
// taggable participant on this channel, body is returned unchanged.
func (n *Notifier) tag(body string, c workable.Change, channel string) string {
	if n.resolver == nil || n.attribution == nil {
		return body
	}
	createdBy, assignedTo := n.attribution(c.AtmID, c.Location)
	handles := commons.MentionsFor(createdBy, assignedTo, n.operator, channel, n.resolver)
	if len(handles) == 0 {
		return body
	}
	// Per-channel mention rendering. Each channel resolves the taggable
	// handles to its own alias syntax (tgram → "@username"; slack → "<@U…>")
	// via that channel's PrependMentions, which resolves each handle against
	// the correct per-channel alias table. A channel with no mention renderer
	// dispatches the body untagged rather than mis-resolving handles against
	// the wrong channel's aliases.
	switch channel {
	case tgram.Channel:
		return tgram.PrependMentions(body, handles, n.resolver)
	case slack.Channel:
		return slack.PrependMentions(body, handles, n.resolver)
	default:
		return body
	}
}
