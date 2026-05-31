package workflow

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons_messaging/channels/tgram"
	workable "github.com/vasic-digital/herald/commons_workable"
	"github.com/vasic-digital/herald/pherald/internal/runner"
)

// TestNotifier_OutboundTagging_E2E drives real item-change events through the
// REAL runner.ChannelDispatcher → recordingChannel (a real commons.Channel
// sink, NOT a mock of the unit under test) and asserts the dispatched message
// BODY contains exactly the @username(s) required by the PARTICIPANT_ATTRIBUTION
// §3 tagging matrix. Each assertion inspects the captured body string — no
// metadata-only PASS.
//
// The matrix (operator = @milos85vasic):
//
//	(a) assigned to Operator                       → NO @-mention  (negative case)
//	(b) opened by Operator, assigned to @bob       → body has @bob
//	(c) opened by @carol (subscriber), assigned Op → body has @carol
//	(d) assignee @dave has NO tgram alias          → NO mention for dave (skip)
//
// Mutation note: a wrong matrix would FAIL here. E.g. if MentionsFor tagged the
// operator (dropping the `handle == operatorHandle` skip), case (a)+(c) would
// gain an unwanted "@milos85vasic" and the `wantAbsent` assertions would go red;
// if it tagged "Claude", or tagged a participant with no tgram alias (case d),
// the @dave-absent assertion would fail. The golden assertions below are the
// truth table; flipping any cell breaks a real captured-body check.
func TestNotifier_OutboundTagging_E2E(t *testing.T) {
	const operator = "@milos85vasic"

	// Real in-memory resolver: bob + carol have tgram aliases; dave does NOT
	// (so he cannot be @-tagged on tgram — the skip-if-not-on-channel case).
	resolver := commons.NewMemoryResolver(operator, []commons.Participant{
		{Handle: operator, Kind: "human", Usernames: map[string]string{tgram.Channel: operator}},
		{Handle: "@bob", Kind: "human", Usernames: map[string]string{tgram.Channel: "@bob"}},
		{Handle: "@carol", Kind: "human", Usernames: map[string]string{tgram.Channel: "@carol"}},
		// @dave: human, but NO tgram alias → not taggable on Telegram.
		{Handle: "@dave", Kind: "human", Usernames: map[string]string{"slack": "@dave_slack"}},
	})

	// Per-item attribution table keyed by AtmID — what MentionsFor consults.
	attribution := map[string][2]string{
		"ATM-A": {operator, operator}, // (a) opened+assigned to operator
		"ATM-B": {operator, "@bob"},   // (b) opened by operator, assigned @bob
		"ATM-C": {"@carol", operator}, // (c) opened by @carol, assigned operator
		"ATM-D": {operator, "@dave"},  // (d) assigned @dave (no tgram alias)
	}
	attrFn := func(atmID, _ string) (string, string) {
		v := attribution[atmID]
		return v[0], v[1]
	}

	// REAL ChannelDispatcher with the recording sink registered under the
	// tgram ChannelID so the tgram mention renderer is exercised.
	rec := &recordingChannel{}
	dispatcher := &runner.ChannelDispatcher{
		Channels: map[commons.ChannelID]commons.Channel{commons.ChannelTelegram: rec},
		Logger:   slog.Default(),
	}
	recipients := []commons.Recipient{
		{Channel: string(commons.ChannelTelegram), ChannelUserID: "group-1", DisplayName: "Team"},
	}

	notifier := NewTaggingNotifier(dispatcher, recipients, resolver, operator, attrFn)

	changes := []workable.Change{
		{AtmID: "ATM-A", Kind: workable.KindStatusChanged, Field: "status", Old: "Queued", New: "In progress"},
		{AtmID: "ATM-B", Kind: workable.KindStatusChanged, Field: "status", Old: "Queued", New: "In progress"},
		{AtmID: "ATM-C", Kind: workable.KindCreated},
		{AtmID: "ATM-D", Kind: workable.KindStatusChanged, Field: "status", Old: "Queued", New: "In progress"},
	}

	if err := notifier.Notify(context.Background(), changes); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	bodies := rec.bodies()
	if len(bodies) != len(changes) {
		t.Fatalf("recording channel received %d messages, want %d: %#v", len(bodies), len(changes), bodies)
	}

	// Print the captured bodies so the test log IS the §107.x evidence.
	for i, b := range bodies {
		t.Logf("dispatched body[%d] (%s): %q", i, changes[i].AtmID, b)
	}

	type expect struct {
		atm        string
		wantTagged []string // @usernames that MUST appear
		wantAbsent []string // @usernames that MUST NOT appear
	}
	cases := []expect{
		// (a) operator-only → no mention at all; operator never self-pinged.
		{atm: "ATM-A", wantTagged: nil, wantAbsent: []string{operator, "@bob", "@carol", "@dave", "cc:"}},
		// (b) assignee @bob is tagged; operator (opener) is not.
		{atm: "ATM-B", wantTagged: []string{"@bob"}, wantAbsent: []string{operator, "@carol", "@dave"}},
		// (c) opener @carol is tagged; operator (assignee) is not.
		{atm: "ATM-C", wantTagged: []string{"@carol"}, wantAbsent: []string{operator, "@bob", "@dave"}},
		// (d) @dave has no tgram alias → not tagged anywhere; no mention line.
		{atm: "ATM-D", wantTagged: nil, wantAbsent: []string{operator, "@bob", "@carol", "@dave", "cc:"}},
	}

	for i, c := range cases {
		body := bodies[i]
		for _, want := range c.wantTagged {
			if !strings.Contains(body, want) {
				t.Errorf("[%s] body %q MISSING required mention %q", c.atm, body, want)
			}
		}
		for _, absent := range c.wantAbsent {
			if strings.Contains(body, absent) {
				t.Errorf("[%s] body %q contains FORBIDDEN token %q (matrix violation)", c.atm, body, absent)
			}
		}
		// The change diff text must always survive the prefixing.
		if !strings.Contains(body, c.atm) {
			t.Errorf("[%s] body %q dropped the rendered change text", c.atm, body)
		}
	}
}

// TestNotifier_NoTagging_BodiesVerbatim proves the un-tagged constructor
// (NewNotifier) dispatches bodies WITHOUT any "cc:" prefix — the attribution
// feature is opt-in and does not regress the Wave 6 verbatim path.
func TestNotifier_NoTagging_BodiesVerbatim(t *testing.T) {
	rec := &recordingChannel{}
	dispatcher := &runner.ChannelDispatcher{
		Channels: map[commons.ChannelID]commons.Channel{commons.ChannelTelegram: rec},
		Logger:   slog.Default(),
	}
	recipients := []commons.Recipient{{Channel: string(commons.ChannelTelegram), ChannelUserID: "g1"}}
	notifier := NewNotifier(dispatcher, recipients)

	if err := notifier.Notify(context.Background(), []workable.Change{{AtmID: "ATM-Z", Kind: workable.KindCreated}}); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	bodies := rec.bodies()
	if len(bodies) != 1 {
		t.Fatalf("want 1 body, got %d", len(bodies))
	}
	if strings.Contains(bodies[0], "cc:") || strings.Contains(bodies[0], "@") {
		t.Errorf("untagged notifier leaked a mention prefix: %q", bodies[0])
	}
}
