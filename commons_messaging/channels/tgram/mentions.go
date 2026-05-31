package tgram

import (
	"strings"

	"github.com/vasic-digital/herald/commons"
)

// Channel is the channel name this adapter resolves mentions for. Used as the
// `channel` argument to commons.IdentityResolver / commons.MentionsFor.
const Channel = "tgram"

// RenderMentions resolves a list of canonical handles (typically the output of
// commons.MentionsFor) to their Telegram @usernames on this channel and returns
// the space-joined "cc: @a @b" prefix line. A Telegram @username mention reaches
// the corresponding group member, so prepending this line to an outbound
// notification body delivers the per-§3 tags.
//
// Returns "" when there is nothing to tag (empty input, nil resolver, or no
// handle has a tgram alias) — callers prepend nothing in that case. Handles
// without a tgram alias are skipped (you cannot tag someone not on Telegram).
// Order is preserved and duplicate resolved @usernames are collapsed.
func RenderMentions(handles []string, r commons.IdentityResolver) string {
	if len(handles) == 0 || r == nil {
		return ""
	}
	usernames := make([]string, 0, len(handles))
	seen := make(map[string]struct{}, len(handles))
	for _, h := range handles {
		username, ok := r.UsernameFor(h, Channel)
		if !ok || username == "" {
			continue
		}
		if _, dup := seen[username]; dup {
			continue
		}
		seen[username] = struct{}{}
		usernames = append(usernames, username)
	}
	if len(usernames) == 0 {
		return ""
	}
	return "cc: " + strings.Join(usernames, " ")
}

// PrependMentions builds the final outbound body by placing the rendered
// "cc: @a @b" line ahead of body (separated by a newline). When there is
// nothing to tag the body is returned unchanged, so this is safe to call
// unconditionally where the outbound text is assembled.
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
