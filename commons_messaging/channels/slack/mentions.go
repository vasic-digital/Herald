package slack

import (
	"strings"

	"github.com/vasic-digital/herald/commons"
)

// Channel is the channel name this adapter resolves mentions for. Used as the
// `channel` argument to commons.IdentityResolver / commons.MentionsFor — mirrors
// tgram.Channel so the §109 tagging-matrix consumer (pherald workflow.Notifier)
// can dispatch per-channel without special-casing the resolver key.
const Channel = "slack"

// RenderMention formats a single Slack user mention. Slack's mention syntax is
// the angle-bracket form <@Uxxxxxx> carrying the Slack USER ID (not an
// @username) — Slack resolves the bracketed id to a clickable, notifying
// mention. This differs from Telegram, where a bare @username is the mention
// token. The caller is responsible for having already resolved a canonical
// handle to its Slack user-id (see RenderMentions / IdentityResolver.UsernameFor
// on the "slack" channel). An empty userID yields "".
func RenderMention(userID string) string {
	if userID == "" {
		return ""
	}
	return "<@" + userID + ">"
}

// RenderMentions resolves a list of canonical handles (typically the output of
// commons.MentionsFor) to their Slack user-ids on this channel and returns the
// space-joined "cc: <@U1> <@U2>" prefix line. A Slack <@Uxxxxxx> mention pings
// the corresponding workspace member, so prepending this line to an outbound
// notification body delivers the §109 tags.
//
// Returns "" when there is nothing to tag (empty input, nil resolver, or no
// handle has a slack alias) — callers prepend nothing in that case. Handles
// without a slack alias are skipped (you cannot tag someone not on Slack).
// Order is preserved and duplicate resolved user-ids are collapsed.
//
// Signature + semantics mirror tgram.RenderMentions exactly so the
// pherald workflow.Notifier.tag seam (workflow.go) can route per-channel with a
// uniform call shape.
func RenderMentions(handles []string, r commons.IdentityResolver) string {
	if len(handles) == 0 || r == nil {
		return ""
	}
	ids := make([]string, 0, len(handles))
	seen := make(map[string]struct{}, len(handles))
	for _, h := range handles {
		userID, ok := r.UsernameFor(h, Channel)
		if !ok || userID == "" {
			continue
		}
		if _, dup := seen[userID]; dup {
			continue
		}
		seen[userID] = struct{}{}
		ids = append(ids, RenderMention(userID))
	}
	if len(ids) == 0 {
		return ""
	}
	return "cc: " + strings.Join(ids, " ")
}

// PrependMentions builds the final outbound body by placing the rendered
// "cc: <@U1> <@U2>" line ahead of body (separated by a newline). When there is
// nothing to tag the body is returned unchanged, so this is safe to call
// unconditionally where the outbound text is assembled. Signature matches
// tgram.PrependMentions.
func PrependMentions(body string, handles []string, r commons.IdentityResolver) string {
	line := RenderMentions(handles, r)
	if line == "" {
		return body
	}
	if body == "" {
		return line
	}
	return line + "\n" + body
}
