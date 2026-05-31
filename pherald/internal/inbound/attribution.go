// Package inbound — attribution.go wires PARTICIPANT_ATTRIBUTION §2/§5
// (inbound) into the action router: when an inbound message opens or
// updates a workable item, it sets `created_by` from the message sender
// and a default `assigned_to` of the operator handle (overridable by an
// explicit `assign:@someone` directive in the message body).
//
// The function is pure (no I/O) so dispatcher_attribution_test.go can drive
// the full §2 matrix directly:
//   - a message from @someuser opening an item → created_by == "@someuser"
//   - a Claude/system-opened item             → created_by == "Claude"
//   - default                                  → assigned_to == OperatorHandle()
//   - explicit "assign:@bob"                   → assigned_to == "@bob"
//
// The dispatcher threads an commons.IdentityResolver (built from env +
// known participants) into the item.update action and injects the resolved
// created_by / assigned_to into the field map applied by ItemMutator.Update.
package inbound

import (
	"strings"

	"github.com/vasic-digital/herald/commons"
	channels "github.com/vasic-digital/herald/commons_messaging/channels"
)

// attributionFields is the canonical column names the ItemMutator understands
// for the two attribution columns. They match commons_workable.Item and the
// applyFields switch.
const (
	fieldCreatedBy  = "created_by"
	fieldAssignedTo = "assigned_to"
)

// resolveAttribution computes the (created_by, assigned_to) canonical handles
// for an inbound item open/update per §2.
//
//   - system==true  → created_by = commons.SystemAgentHandle ("Claude") — the
//     System/Claude path (an item Claude opened detecting an issue/task).
//   - system==false → created_by = r.ResolveSender(channel, channelUserID,
//     username) from the message sender (the §2 "received through Herald" path).
//
// assigned_to defaults to r.OperatorHandle(); an explicit `assign:@handle`
// directive anywhere in body overrides it (parseAssign). When r is nil the
// function degrades gracefully: created_by falls back to the raw sender id /
// "Claude", assigned_to falls back to the explicit assignee (or "").
func resolveAttribution(ev commons.InboundEvent, r commons.IdentityResolver, system bool) (createdBy, assignedTo string) {
	if system {
		createdBy = commons.SystemAgentHandle
	} else {
		username := senderUsername(ev)
		if r != nil {
			createdBy = r.ResolveSender(ev.Sender.Channel, ev.Sender.ChannelUserID, username)
		} else if username != "" {
			createdBy = normalizeAt(username)
		} else {
			createdBy = ev.Sender.ChannelUserID
		}
	}

	if assignee, ok := parseAssign(ev.Body.Plain); ok {
		assignedTo = assignee
	} else if r != nil {
		assignedTo = r.OperatorHandle()
	}
	return createdBy, assignedTo
}

// senderUsername extracts the sender's native @username stamped into ev.Raw by
// the channel adapter (channels.StampSender with IdentityUsername — Telegram's
// @handle). Returns "" when no username was stamped (e.g. a sender with no
// public @username), in which case ResolveSender falls back to the channel
// user id.
func senderUsername(ev commons.InboundEvent) string {
	if ev.Raw == nil {
		return ""
	}
	kind, _ := ev.Raw[channels.RawSenderIdentityKnd].(string)
	if channels.IdentityKind(kind) != channels.IdentityUsername {
		return ""
	}
	v, _ := ev.Raw[channels.RawSenderIdentity].(string)
	return v
}

// parseAssign recognises an explicit assignment directive of the form
// `assign:@handle` (case-insensitive on the keyword; the @handle is the next
// non-space token). It scans whitespace-delimited tokens so the directive can
// appear anywhere in the message. Returns (normalizedHandle, true) on a match.
//
// Two accepted spellings:
//
//	assign:@bob   (no space — single token)
//	assign: @bob  / assign @bob   (keyword token followed by the @handle token)
func parseAssign(body string) (string, bool) {
	fields := strings.Fields(body)
	for i, tok := range fields {
		low := strings.ToLower(tok)
		// Combined form: assign:@bob
		if strings.HasPrefix(low, "assign:") {
			rest := tok[len("assign:"):]
			if rest != "" {
				return normalizeAt(rest), true
			}
			// "assign:" alone — assignee is the next token.
			if i+1 < len(fields) {
				return normalizeAt(fields[i+1]), true
			}
		}
		// Split form: "assign" then "@bob".
		if low == "assign" && i+1 < len(fields) {
			return normalizeAt(fields[i+1]), true
		}
	}
	return "", false
}

// normalizeAt guarantees exactly one leading "@" on a non-empty handle (mirrors
// commons.normalizeUsername, which is unexported). Empty input returns "".
func normalizeAt(h string) string {
	h = strings.TrimSpace(h)
	if h == "" {
		return ""
	}
	if h == commons.SystemAgentHandle {
		return h
	}
	return "@" + strings.TrimLeft(h, "@")
}
