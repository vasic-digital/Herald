// Package commons — participant_test.go (PARTICIPANT_ATTRIBUTION §1 + §3 + §6).
//
// Anti-bluff (§107 / Helix §11.4): the MentionsFor truth-table exercises EVERY
// cell of the §3 matrix against a REAL MemoryResolver built from a real
// participant roster — no mocks. The matrix is also proven by a deliberate-
// mutation sibling (TestMentionsFor_MutationWouldFail) that asserts the
// CORRECT-but-wrong-if-flipped expectations: if any cell of the matrix were
// implemented backwards (tag the operator, or tag Claude, or skip the
// assignee), at least one of these named cases would FAIL. Metadata-only PASS
// is impossible here — each case asserts the exact returned handle slice.
package commons_test

import (
	"os"
	"reflect"
	"testing"

	"github.com/vasic-digital/herald/commons"
)

// roster is the shared real participant set used across the truth-table.
//   - operator (@milos85vasic): the operator — NEVER tagged.
//   - alice (@alice_tg): a human subscriber on tgram — taggable.
//   - bob (@bob_tg): a human subscriber on tgram — taggable.
//   - carol (@carol_slack): a human on SLACK ONLY — NOT taggable on tgram.
//   - Claude: the system agent — NEVER tagged (and has no alias anywhere).
const (
	opHandle    = "@milos85vasic"
	aliceHandle = "@alice"
	bobHandle   = "@bob"
	carolHandle = "@carol"
)

func newRoster() *commons.MemoryResolver {
	return commons.NewMemoryResolver(opHandle, []commons.Participant{
		{Handle: opHandle, DisplayName: "Operator", Kind: "human",
			Usernames: map[string]string{"tgram": "@milos85vasic"}},
		{Handle: aliceHandle, DisplayName: "Alice", Kind: "human",
			Usernames: map[string]string{"tgram": "@alice_tg"}},
		{Handle: bobHandle, DisplayName: "Bob", Kind: "human",
			Usernames: map[string]string{"tgram": "@bob_tg"}},
		// Carol is on Slack only — has NO tgram alias, so cannot be tagged on tgram.
		{Handle: carolHandle, DisplayName: "Carol", Kind: "human",
			Usernames: map[string]string{"slack": "@carol_slack"}},
		{Handle: commons.SystemAgentHandle, DisplayName: "Claude", Kind: "agent",
			Usernames: map[string]string{}},
	})
}

// TestMentionsFor_TruthTable covers every cell of the §3 matrix.
func TestMentionsFor_TruthTable(t *testing.T) {
	r := newRoster()
	const channel = "tgram"

	cases := []struct {
		name       string
		createdBy  string
		assignedTo string
		want       []string
	}{
		{
			// assigned to Operator (default), opened by Operator → NO tag.
			name:       "assigned_to_operator_opened_by_operator__none",
			createdBy:  opHandle,
			assignedTo: opHandle,
			want:       nil,
		},
		{
			// opened by Operator, assigned to another human → tag the assignee.
			name:       "opened_by_operator_assigned_other__assignee",
			createdBy:  opHandle,
			assignedTo: aliceHandle,
			want:       []string{aliceHandle},
		},
		{
			// opened by a non-operator non-Claude subscriber, assigned to operator
			// (default) → tag the opener only.
			name:       "opened_by_subscriber_assigned_operator__opener",
			createdBy:  bobHandle,
			assignedTo: opHandle,
			want:       []string{bobHandle},
		},
		{
			// opened by a subscriber, assigned to a DIFFERENT subscriber → tag both,
			// assignee first.
			name:       "opened_by_subscriber_assigned_other_subscriber__both",
			createdBy:  bobHandle,
			assignedTo: aliceHandle,
			want:       []string{aliceHandle, bobHandle},
		},
		{
			// opened by Claude (system), assigned to operator → NO tag.
			name:       "opened_by_claude_assigned_operator__none",
			createdBy:  commons.SystemAgentHandle,
			assignedTo: opHandle,
			want:       nil,
		},
		{
			// opened by Claude, assigned to a human → tag only the human assignee
			// (Claude is never tagged).
			name:       "opened_by_claude_assigned_human__assignee_only",
			createdBy:  commons.SystemAgentHandle,
			assignedTo: aliceHandle,
			want:       []string{aliceHandle},
		},
		{
			// both created_by and assigned_to are the SAME subscriber → de-dup to one.
			name:       "same_subscriber_both__deduped",
			createdBy:  aliceHandle,
			assignedTo: aliceHandle,
			want:       []string{aliceHandle},
		},
		{
			// assignee is on Slack only (no tgram alias) → skipped on tgram.
			name:       "assignee_not_on_channel__skipped",
			createdBy:  opHandle,
			assignedTo: carolHandle,
			want:       nil,
		},
		{
			// opener on Slack only, assignee on tgram → tag only the on-channel one.
			name:       "opener_off_channel_assignee_on_channel__assignee_only",
			createdBy:  carolHandle,
			assignedTo: bobHandle,
			want:       []string{bobHandle},
		},
		{
			// Claude assigned and Claude opened → nothing.
			name:       "claude_both__none",
			createdBy:  commons.SystemAgentHandle,
			assignedTo: commons.SystemAgentHandle,
			want:       nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := commons.MentionsFor(tc.createdBy, tc.assignedTo, opHandle, channel, r)
			if !equalSlices(got, tc.want) {
				t.Fatalf("MentionsFor(createdBy=%q, assignedTo=%q) = %v; want %v",
					tc.createdBy, tc.assignedTo, got, tc.want)
			}
		})
	}
}

// TestMentionsFor_MutationWouldFail is the §6 mutation sibling: it asserts the
// NEGATIVE of each rule, proving the test suite would catch an implementation
// that flipped a cell. If MentionsFor were (wrongly) tagging the operator or
// Claude, or (wrongly) NOT tagging the assignee, these assertions would fail —
// which is exactly what we want a mutation to do.
func TestMentionsFor_MutationWouldFail(t *testing.T) {
	r := newRoster()
	const channel = "tgram"

	// Operator must NEVER appear, even when both fields are the operator.
	if got := commons.MentionsFor(opHandle, opHandle, opHandle, channel, r); len(got) != 0 {
		t.Fatalf("operator self-ping leaked: got %v, want empty (a mutation that tagged the operator would land here)", got)
	}
	// Claude must NEVER appear.
	if got := commons.MentionsFor(commons.SystemAgentHandle, commons.SystemAgentHandle, opHandle, channel, r); len(got) != 0 {
		t.Fatalf("Claude tagged: got %v, want empty (a mutation that tagged the system agent would land here)", got)
	}
	// The assignee MUST appear when it is a non-operator human — a mutation that
	// dropped the assigned_to branch would return empty here.
	if got := commons.MentionsFor(opHandle, aliceHandle, opHandle, channel, r); !equalSlices(got, []string{aliceHandle}) {
		t.Fatalf("assignee not tagged: got %v, want [%s] (a mutation that skipped assigned_to would land here)", got, aliceHandle)
	}
	// The opener MUST appear when it is a non-operator non-Claude human — a
	// mutation that dropped the created_by branch would return empty here.
	if got := commons.MentionsFor(bobHandle, opHandle, opHandle, channel, r); !equalSlices(got, []string{bobHandle}) {
		t.Fatalf("opener not tagged: got %v, want [%s] (a mutation that skipped created_by would land here)", got, bobHandle)
	}
}

func TestMentionsFor_NilResolver_NoPanic(t *testing.T) {
	// With a nil resolver nothing is taggable (no alias source) — must not panic.
	if got := commons.MentionsFor(aliceHandle, bobHandle, opHandle, "tgram", nil); len(got) != 0 {
		t.Fatalf("nil resolver should yield no mentions, got %v", got)
	}
}

func TestMemoryResolver_ResolveSender(t *testing.T) {
	r := newRoster()
	r.AddSenderIndex("tgram", "1001", aliceHandle) // chat/user id mapping

	// Known by channel_user_id.
	if got := r.ResolveSender("tgram", "1001", ""); got != aliceHandle {
		t.Fatalf("ResolveSender by id = %q; want %q", got, aliceHandle)
	}
	// Known by @username (alice_tg).
	if got := r.ResolveSender("tgram", "9999", "alice_tg"); got != aliceHandle {
		t.Fatalf("ResolveSender by username = %q; want %q", got, aliceHandle)
	}
	// Username already carrying a leading @ resolves identically.
	if got := r.ResolveSender("tgram", "", "@bob_tg"); got != bobHandle {
		t.Fatalf("ResolveSender by @username = %q; want %q", got, bobHandle)
	}
	// Unknown sender → raw @username (first-contact attribution).
	if got := r.ResolveSender("tgram", "5", "newcomer"); got != "@newcomer" {
		t.Fatalf("ResolveSender unknown = %q; want %q", got, "@newcomer")
	}
	// id match takes precedence over a (potentially different) username.
	if got := r.ResolveSender("tgram", "1001", "bob_tg"); got != aliceHandle {
		t.Fatalf("ResolveSender id-precedence = %q; want %q", got, aliceHandle)
	}
}

func TestMemoryResolver_UsernameFor_RoundTrip(t *testing.T) {
	r := newRoster()
	// Resolve a sender's @username back to its canonical handle, then the
	// canonical handle back to the channel @username — a full round-trip.
	handle := r.ResolveSender("tgram", "", "alice_tg")
	if handle != aliceHandle {
		t.Fatalf("ResolveSender = %q; want %q", handle, aliceHandle)
	}
	username, ok := r.UsernameFor(handle, "tgram")
	if !ok || username != "@alice_tg" {
		t.Fatalf("UsernameFor(%q,tgram) = (%q,%v); want (@alice_tg,true)", handle, username, ok)
	}
	// No alias on a channel → ok=false (carol is slack-only).
	if _, ok := r.UsernameFor(carolHandle, "tgram"); ok {
		t.Fatalf("UsernameFor(carol,tgram) should be ok=false (slack-only participant)")
	}
	// Unknown handle → ok=false.
	if _, ok := r.UsernameFor("@nobody", "tgram"); ok {
		t.Fatalf("UsernameFor(@nobody,tgram) should be ok=false")
	}
}

func TestMemoryResolver_OperatorHandle(t *testing.T) {
	r := newRoster()
	if got := r.OperatorHandle(); got != opHandle {
		t.Fatalf("OperatorHandle = %q; want %q", got, opHandle)
	}
}

func TestOperatorHandleFromEnv(t *testing.T) {
	const key = "HERALD_TGRAM_OPERATOR_USERNAME"
	prev, had := os.LookupEnv(key)
	t.Cleanup(func() {
		if had {
			os.Setenv(key, prev)
		} else {
			os.Unsetenv(key)
		}
	})

	os.Setenv(key, "  @milos85vasic  ")
	if got := commons.OperatorHandleFromEnv("tgram"); got != "@milos85vasic" {
		t.Fatalf("OperatorHandleFromEnv(tgram) = %q; want %q (trimmed)", got, "@milos85vasic")
	}
	// Channel name is upper-cased to form the env key.
	if got := commons.OperatorHandleFromEnv("TGRAM"); got != "@milos85vasic" {
		t.Fatalf("OperatorHandleFromEnv(TGRAM) = %q; want %q", got, "@milos85vasic")
	}
	os.Unsetenv(key)
	if got := commons.OperatorHandleFromEnv("tgram"); got != "" {
		t.Fatalf("OperatorHandleFromEnv with unset env = %q; want empty", got)
	}
	if got := commons.OperatorHandleFromEnv(""); got != "" {
		t.Fatalf("OperatorHandleFromEnv(\"\") = %q; want empty", got)
	}
}

// equalSlices compares two string slices treating nil and empty as equal and
// requiring identical order (order is contractually assignee-before-opener).
func equalSlices(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}
