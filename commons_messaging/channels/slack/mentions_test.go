// Package slack — mentions_test.go (PARTICIPANT_ATTRIBUTION §5 + §6, Slack).
//
// Anti-bluff: drives RenderMention / RenderMentions / PrependMentions through a
// REAL commons.MemoryResolver. The rendered "cc: <@U…>" line and the prepended
// body are asserted byte-for-byte — a stub that returned "" unconditionally, or
// that failed to skip an off-channel handle, or that emitted bare @usernames
// instead of Slack's <@Uxxxxxx> angle-bracket form, would fail here. Mirrors
// tgram/mentions_test.go so the §109 contract is proven identically on both
// channels.
package slack_test

import (
	"testing"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons_messaging/channels/slack"
)

// newResolver wires three participants. On Slack, UsernameFor returns the Slack
// USER ID (Uxxxxxx) — that is the per-channel "username" used for the
// <@Uxxxxxx> mention token (distinct from a human-readable handle).
func newResolver() *commons.MemoryResolver {
	return commons.NewMemoryResolver("@milos85vasic", []commons.Participant{
		{Handle: "@alice", Kind: "human", Usernames: map[string]string{"slack": "U0ALICE"}},
		{Handle: "@bob", Kind: "human", Usernames: map[string]string{"slack": "U0BOB"}},
		// Dora: tgram-only — must be skipped when rendering on slack.
		{Handle: "@dora", Kind: "human", Usernames: map[string]string{"tgram": "@dora_tg"}},
	})
}

func TestRenderMention(t *testing.T) {
	if got := slack.RenderMention("U0ALICE"); got != "<@U0ALICE>" {
		t.Fatalf("RenderMention = %q; want %q", got, "<@U0ALICE>")
	}
	if got := slack.RenderMention(""); got != "" {
		t.Fatalf("RenderMention empty = %q; want empty", got)
	}
}

func TestRenderMentions(t *testing.T) {
	r := newResolver()

	if got := slack.RenderMentions([]string{"@alice", "@bob"}, r); got != "cc: <@U0ALICE> <@U0BOB>" {
		t.Fatalf("RenderMentions two = %q; want %q", got, "cc: <@U0ALICE> <@U0BOB>")
	}
	// Duplicate handle collapses to a single rendered mention.
	if got := slack.RenderMentions([]string{"@alice", "@alice"}, r); got != "cc: <@U0ALICE>" {
		t.Fatalf("RenderMentions dedup = %q; want %q", got, "cc: <@U0ALICE>")
	}
	// Off-channel handle (@dora) is skipped; only the resolvable one renders.
	if got := slack.RenderMentions([]string{"@dora", "@alice"}, r); got != "cc: <@U0ALICE>" {
		t.Fatalf("RenderMentions skip-off-channel = %q; want %q", got, "cc: <@U0ALICE>")
	}
	// All off-channel → empty line (nothing to tag).
	if got := slack.RenderMentions([]string{"@dora"}, r); got != "" {
		t.Fatalf("RenderMentions all-off-channel = %q; want empty", got)
	}
	// Empty input → empty.
	if got := slack.RenderMentions(nil, r); got != "" {
		t.Fatalf("RenderMentions nil = %q; want empty", got)
	}
	// Nil resolver → empty (no panic).
	if got := slack.RenderMentions([]string{"@alice"}, nil); got != "" {
		t.Fatalf("RenderMentions nil-resolver = %q; want empty", got)
	}
	// Unknown handle → skipped.
	if got := slack.RenderMentions([]string{"@nobody"}, r); got != "" {
		t.Fatalf("RenderMentions unknown = %q; want empty", got)
	}
}

func TestPrependMentions(t *testing.T) {
	r := newResolver()

	const body = "Issue HRD-200 was reopened."
	got := slack.PrependMentions(body, []string{"@alice"}, r)
	want := "cc: <@U0ALICE>\n" + body
	if got != want {
		t.Fatalf("PrependMentions = %q; want %q", got, want)
	}
	// Nothing to tag → body unchanged.
	if got := slack.PrependMentions(body, []string{"@dora"}, r); got != body {
		t.Fatalf("PrependMentions nothing-to-tag = %q; want unchanged %q", got, body)
	}
	if got := slack.PrependMentions(body, nil, r); got != body {
		t.Fatalf("PrependMentions nil-handles = %q; want unchanged %q", got, body)
	}
	// Empty body with a mention → just the line.
	if got := slack.PrependMentions("", []string{"@bob"}, r); got != "cc: <@U0BOB>" {
		t.Fatalf("PrependMentions empty-body = %q; want %q", got, "cc: <@U0BOB>")
	}
}

// TestRenderMentions_EndToEndWithMatrix proves the §3 → §5 seam: MentionsFor
// produces the canonical handles, RenderMentions turns them into the real Slack
// <@U…> cc-line. Operator is dropped by MentionsFor and so never appears.
func TestRenderMentions_EndToEndWithMatrix(t *testing.T) {
	r := newResolver()
	const op = "@milos85vasic"

	// Opened by operator, assigned to alice → matrix yields [@alice] → "cc: <@U0ALICE>".
	handles := commons.MentionsFor(op, "@alice", op, slack.Channel, r)
	if got := slack.RenderMentions(handles, r); got != "cc: <@U0ALICE>" {
		t.Fatalf("e2e operator->alice = %q; want %q", got, "cc: <@U0ALICE>")
	}
	// Assigned to operator → matrix yields nothing → empty cc-line (NO self-ping).
	handles = commons.MentionsFor(op, op, op, slack.Channel, r)
	if got := slack.RenderMentions(handles, r); got != "" {
		t.Fatalf("e2e operator self-assign = %q; want empty (operator never tagged)", got)
	}
}
