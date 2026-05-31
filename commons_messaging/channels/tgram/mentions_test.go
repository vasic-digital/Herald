// Package tgram — mentions_test.go (PARTICIPANT_ATTRIBUTION §5 + §6).
//
// Anti-bluff: drives RenderMentions / PrependMentions through a REAL
// commons.MemoryResolver. The rendered "cc: @a @b" line and the prepended body
// are asserted byte-for-byte — a stub that returned "" unconditionally, or that
// failed to skip an off-channel handle, would fail here.
package tgram_test

import (
	"testing"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons_messaging/channels/tgram"
)

func newResolver() *commons.MemoryResolver {
	return commons.NewMemoryResolver("@milos85vasic", []commons.Participant{
		{Handle: "@alice", Kind: "human", Usernames: map[string]string{"tgram": "@alice_tg"}},
		{Handle: "@bob", Kind: "human", Usernames: map[string]string{"tgram": "@bob_tg"}},
		// Carol: slack-only — must be skipped when rendering on tgram.
		{Handle: "@carol", Kind: "human", Usernames: map[string]string{"slack": "@carol_slack"}},
	})
}

func TestRenderMentions(t *testing.T) {
	r := newResolver()

	if got := tgram.RenderMentions([]string{"@alice", "@bob"}, r); got != "cc: @alice_tg @bob_tg" {
		t.Fatalf("RenderMentions two = %q; want %q", got, "cc: @alice_tg @bob_tg")
	}
	// Off-channel handle (@carol) is skipped; only the resolvable one renders.
	if got := tgram.RenderMentions([]string{"@carol", "@alice"}, r); got != "cc: @alice_tg" {
		t.Fatalf("RenderMentions skip-off-channel = %q; want %q", got, "cc: @alice_tg")
	}
	// All off-channel → empty line (nothing to tag).
	if got := tgram.RenderMentions([]string{"@carol"}, r); got != "" {
		t.Fatalf("RenderMentions all-off-channel = %q; want empty", got)
	}
	// Empty input → empty.
	if got := tgram.RenderMentions(nil, r); got != "" {
		t.Fatalf("RenderMentions nil = %q; want empty", got)
	}
	// Nil resolver → empty (no panic).
	if got := tgram.RenderMentions([]string{"@alice"}, nil); got != "" {
		t.Fatalf("RenderMentions nil-resolver = %q; want empty", got)
	}
	// Unknown handle → skipped.
	if got := tgram.RenderMentions([]string{"@nobody"}, r); got != "" {
		t.Fatalf("RenderMentions unknown = %q; want empty", got)
	}
}

func TestPrependMentions(t *testing.T) {
	r := newResolver()

	const body = "Issue HRD-200 was reopened."
	got := tgram.PrependMentions(body, []string{"@alice"}, r)
	want := "cc: @alice_tg\n" + body
	if got != want {
		t.Fatalf("PrependMentions = %q; want %q", got, want)
	}
	// Nothing to tag → body unchanged.
	if got := tgram.PrependMentions(body, []string{"@carol"}, r); got != body {
		t.Fatalf("PrependMentions nothing-to-tag = %q; want unchanged %q", got, body)
	}
	if got := tgram.PrependMentions(body, nil, r); got != body {
		t.Fatalf("PrependMentions nil-handles = %q; want unchanged %q", got, body)
	}
	// Empty body with a mention → just the line.
	if got := tgram.PrependMentions("", []string{"@bob"}, r); got != "cc: @bob_tg" {
		t.Fatalf("PrependMentions empty-body = %q; want %q", got, "cc: @bob_tg")
	}
}

// TestRenderMentions_EndToEndWithMatrix proves the §3 → §5 seam: MentionsFor
// produces the canonical handles, RenderMentions turns them into the real
// Telegram cc-line. Operator is dropped by MentionsFor and so never appears.
func TestRenderMentions_EndToEndWithMatrix(t *testing.T) {
	r := newResolver()
	const op = "@milos85vasic"

	// Opened by operator, assigned to alice → matrix yields [@alice] → "cc: @alice_tg".
	handles := commons.MentionsFor(op, "@alice", op, tgram.Channel, r)
	if got := tgram.RenderMentions(handles, r); got != "cc: @alice_tg" {
		t.Fatalf("e2e operator->alice = %q; want %q", got, "cc: @alice_tg")
	}
	// Assigned to operator → matrix yields nothing → empty cc-line (NO self-ping).
	handles = commons.MentionsFor(op, op, op, tgram.Channel, r)
	if got := tgram.RenderMentions(handles, r); got != "" {
		t.Fatalf("e2e operator self-assign = %q; want empty (operator never tagged)", got)
	}
}
