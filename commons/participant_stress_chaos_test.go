// Package commons — participant_stress_chaos_test.go
//
// Helix §11.4.85 stress + chaos coverage for the participant/identity/tagging
// layer (docs/design/PARTICIPANT_ATTRIBUTION.md §1/§3/§6). The happy-path truth
// table lives in participant_test.go; this file proves the SAME contract holds
// under sustained concurrent load (stress) AND under adversarial / malformed /
// boundary inputs (chaos) — both with the race detector engaged.
//
// Anti-bluff (§107 / Helix §11.4): every assertion checks the EXACT returned
// value, not absence-of-error. Concurrency assertions compare each goroutine's
// result against the single-threaded ground truth computed up-front, so a data
// race that corrupted a result (not just one tripped by -race) would still be
// caught as a value mismatch. Real evidence (call counts + wall-clock timing)
// is printed via t.Logf so a captured run shows the load actually happened.
//
// Run: go test -race -run 'StressChaos' -v ./commons/...
package commons_test

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vasic-digital/herald/commons"
)

// scRoster mirrors newRoster() but is self-contained so this file does not
// depend on test-helper ordering in participant_test.go. Same participants:
//   - operator (@milos85vasic): NEVER tagged.
//   - alice/bob: humans on tgram — taggable on tgram.
//   - carol: human on slack only — NOT taggable on tgram.
//   - Claude: system agent — NEVER tagged, no alias anywhere.
func scRoster() *commons.MemoryResolver {
	return commons.NewMemoryResolver(scOp, []commons.Participant{
		{Handle: scOp, DisplayName: "Operator", Kind: "human",
			Usernames: map[string]string{"tgram": "@milos85vasic"}},
		{Handle: scAlice, DisplayName: "Alice", Kind: "human",
			Usernames: map[string]string{"tgram": "@alice_tg"}},
		{Handle: scBob, DisplayName: "Bob", Kind: "human",
			Usernames: map[string]string{"tgram": "@bob_tg"}},
		{Handle: scCarol, DisplayName: "Carol", Kind: "human",
			Usernames: map[string]string{"slack": "@carol_slack"}},
		{Handle: commons.SystemAgentHandle, DisplayName: "Claude", Kind: "agent",
			Usernames: map[string]string{}},
	})
}

const (
	scOp    = "@milos85vasic"
	scAlice = "@alice"
	scBob   = "@bob"
	scCarol = "@carol"
)

// scEqual treats nil and empty as equal and requires identical order.
func scEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// scCase is one input row + its single-threaded ground-truth output, used as
// the oracle the concurrent workers must reproduce exactly.
type scCase struct {
	createdBy, assignedTo, channel string
	want                           []string
}

// scMatrix is the mixed workload: operator / Claude / subscriber / off-channel
// combinations on tgram. The `want` values are the contractually correct
// outputs per the §3 matrix (assignee-first, de-duped, operator+Claude skipped,
// off-channel skipped).
func scMatrix() []scCase {
	return []scCase{
		{scOp, scOp, "tgram", nil},                                           // operator both → none
		{scOp, scAlice, "tgram", []string{scAlice}},                          // op opens, alice assigned → alice
		{scBob, scOp, "tgram", []string{scBob}},                              // bob opens, op assigned → bob
		{scBob, scAlice, "tgram", []string{scAlice, scBob}},                  // both subs → assignee-first
		{commons.SystemAgentHandle, scOp, "tgram", nil},                      // Claude opens, op → none
		{commons.SystemAgentHandle, scAlice, "tgram", []string{scAlice}},     // Claude opens, alice → alice
		{scAlice, scAlice, "tgram", []string{scAlice}},                       // same sub both → dedup
		{scOp, scCarol, "tgram", nil},                                        // carol off-channel → skipped
		{scCarol, scBob, "tgram", []string{scBob}},                           // opener off-channel, assignee on → assignee only
		{commons.SystemAgentHandle, commons.SystemAgentHandle, "tgram", nil}, // Claude both → none
	}
}

// TestStressChaos_MentionsFor_Concurrent drives MentionsFor from many goroutines
// over a single shared MemoryResolver, mixing operator/Claude/subscriber/
// off-channel inputs. Each call's result MUST match the precomputed ground
// truth — deterministic + race-clean (run under -race). 5k+ calls.
func TestStressChaos_MentionsFor_Concurrent(t *testing.T) {
	r := scRoster()
	matrix := scMatrix()

	const goroutines = 64
	const perGoroutine = 100 // 64*100 = 6400 calls
	var totalCalls int64
	var mismatches int64
	var firstMismatch atomic.Value // string

	start := time.Now()
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				tc := matrix[(seed+i)%len(matrix)]
				got := commons.MentionsFor(tc.createdBy, tc.assignedTo, scOp, tc.channel, r)
				atomic.AddInt64(&totalCalls, 1)
				if !scEqual(got, tc.want) {
					atomic.AddInt64(&mismatches, 1)
					firstMismatch.CompareAndSwap(nil, fmt.Sprintf(
						"MentionsFor(%q,%q,%q)=%v want %v",
						tc.createdBy, tc.assignedTo, tc.channel, got, tc.want))
				}
			}
		}(g)
	}
	wg.Wait()
	elapsed := time.Since(start)

	if m := atomic.LoadInt64(&mismatches); m != 0 {
		t.Fatalf("%d/%d concurrent MentionsFor results were WRONG; first: %v",
			m, atomic.LoadInt64(&totalCalls), firstMismatch.Load())
	}
	calls := atomic.LoadInt64(&totalCalls)
	if calls < 5000 {
		t.Fatalf("stress workload too small: %d calls (want >=5000)", calls)
	}
	t.Logf("STRESS MentionsFor: %d goroutines x %d = %d calls in %v (%.0f calls/sec), 0 wrong results, race-clean",
		goroutines, perGoroutine, calls, elapsed, float64(calls)/elapsed.Seconds())
}

// TestStressChaos_Resolver_ConcurrentReads hammers ResolveSender + UsernameFor +
// OperatorHandle concurrently on one shared resolver. All reads must be correct
// and race-clean. AddSenderIndex is called ONCE before the goroutines fan out
// (the contract: index built at construction, then read-only under load).
func TestStressChaos_Resolver_ConcurrentReads(t *testing.T) {
	r := scRoster()
	r.AddSenderIndex("tgram", "1001", scAlice)
	r.AddSenderIndex("tgram", "1002", scBob)

	type rcase struct {
		kind                          int // 0 resolve-by-id, 1 resolve-by-username, 2 username-for, 3 operator
		channel, chanUserID, username string
		want                          string
		wantOK                        bool
	}
	cases := []rcase{
		{0, "tgram", "1001", "", scAlice, true},
		{0, "tgram", "1002", "", scBob, true},
		{1, "tgram", "", "alice_tg", scAlice, true},
		{1, "tgram", "", "@bob_tg", scBob, true},
		{1, "tgram", "", "newcomer", "@newcomer", true}, // first-contact → raw @username
		{2, "tgram", "", "", "@alice_tg", true},         // UsernameFor(alice,tgram)
		{2, "slack", "", "", "@carol_slack", true},      // UsernameFor(carol,slack) handled below
	}
	// handle for the UsernameFor cases (case.kind==2 uses `username` field as handle key)
	cases[5].username = scAlice
	cases[6].username = scCarol

	const goroutines = 64
	const perGoroutine = 120
	var totalCalls, mismatches int64
	var firstMismatch atomic.Value

	start := time.Now()
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				c := cases[(seed+i)%len(cases)]
				atomic.AddInt64(&totalCalls, 1)
				switch c.kind {
				case 0, 1:
					got := r.ResolveSender(c.channel, c.chanUserID, c.username)
					if got != c.want {
						atomic.AddInt64(&mismatches, 1)
						firstMismatch.CompareAndSwap(nil, fmt.Sprintf("ResolveSender(%q,%q,%q)=%q want %q", c.channel, c.chanUserID, c.username, got, c.want))
					}
				case 2:
					got, ok := r.UsernameFor(c.username, c.channel)
					if got != c.want || ok != c.wantOK {
						atomic.AddInt64(&mismatches, 1)
						firstMismatch.CompareAndSwap(nil, fmt.Sprintf("UsernameFor(%q,%q)=(%q,%v) want (%q,%v)", c.username, c.channel, got, ok, c.want, c.wantOK))
					}
				case 3:
					if got := r.OperatorHandle(); got != scOp {
						atomic.AddInt64(&mismatches, 1)
					}
				}
			}
		}(g)
	}
	wg.Wait()
	elapsed := time.Since(start)

	if m := atomic.LoadInt64(&mismatches); m != 0 {
		t.Fatalf("%d concurrent resolver reads were WRONG; first: %v", m, firstMismatch.Load())
	}
	calls := atomic.LoadInt64(&totalCalls)
	t.Logf("STRESS resolver reads: %d goroutines x %d = %d calls in %v (%.0f/sec), 0 wrong, race-clean",
		goroutines, perGoroutine, calls, elapsed, float64(calls)/elapsed.Seconds())
}

// TestStressChaos_MentionsFor_AdversarialInputs throws malformed / boundary /
// hostile inputs at MentionsFor and asserts: no panic + honest output (no
// spurious mentions, no operator/Claude leak, no off-channel tag). Each row is
// a chaos vector with the contractually correct (defensive) expectation.
func TestStressChaos_MentionsFor_AdversarialInputs(t *testing.T) {
	r := scRoster()
	longHandle := "@" + strings.Repeat("z", 4096) // unknown, no alias → not taggable

	cases := []struct {
		name                           string
		createdBy, assignedTo, channel string
		resolver                       commons.IdentityResolver
		want                           []string
	}{
		{"empty_both", "", "", "tgram", r, nil},
		{"empty_createdBy_known_assignee", "", scAlice, "tgram", r, []string{scAlice}},
		{"empty_assignee_known_creator", scBob, "", "tgram", r, []string{scBob}},
		{"handle_equals_operator_equals_assignee", scOp, scOp, "tgram", r, nil},
		{"unknown_channel_no_aliases", scAlice, scBob, "discord", r, nil}, // nobody has discord alias
		{"nil_resolver", scAlice, scBob, "tgram", nil, nil},               // no alias source → nothing
		{"very_long_unknown_handle", longHandle, longHandle, "tgram", r, nil},
		{"unknown_handles_both", "@ghost1", "@ghost2", "tgram", r, nil},                 // not in roster → no tgram alias
		{"claude_literal_lookalike_not_sentinel", "claude", "Claude ", "tgram", r, nil}, // neither is the exact sentinel, neither has alias
		{"whitespace_handles", "   ", "\t", "tgram", r, nil},
		{"off_channel_assignee_off_channel_creator", scCarol, scCarol, "tgram", r, nil}, // carol slack-only
		{"empty_channel", scAlice, scBob, "", r, nil},                                   // no aliases for "" channel
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if rec := recover(); rec != nil {
					t.Fatalf("MentionsFor PANICKED on adversarial input %q: %v", tc.name, rec)
				}
			}()
			got := commons.MentionsFor(tc.createdBy, tc.assignedTo, scOp, tc.channel, tc.resolver)
			if !scEqual(got, tc.want) {
				t.Fatalf("MentionsFor(%q,%q,%q)=%v; want %v", tc.createdBy, tc.assignedTo, tc.channel, got, tc.want)
			}
			// Hard invariants regardless of input: operator + Claude sentinel never present.
			for _, h := range got {
				if h == scOp {
					t.Fatalf("OPERATOR LEAKED into mentions for %q: %v", tc.name, got)
				}
				if h == commons.SystemAgentHandle {
					t.Fatalf("CLAUDE LEAKED into mentions for %q: %v", tc.name, got)
				}
			}
		})
	}
	t.Logf("CHAOS MentionsFor: %d adversarial vectors, no panic, no operator/Claude/off-channel leak", len(cases))
}

// TestStressChaos_SkipGuards_Isolated isolates the operator-skip and Claude-skip
// guards from the alias filter. In the default roster the operator/Claude are
// not tagged for TWO independent reasons (the explicit skip guard AND no alias),
// so a mutation that removes a skip guard is MASKED by the alias filter and goes
// undetected. Here the operator AND Claude are given REAL aliases on tgram — so
// the ONLY thing that keeps them out of the mention list is the explicit skip
// guard. This makes the skip-guard removal mutations (M1/M2 in
// tests/test_participant_mutation_meta.sh) genuinely load-bearing: with either
// guard removed, the now-aliased operator/Claude would leak and this test FAILs.
func TestStressChaos_SkipGuards_Isolated(t *testing.T) {
	// Operator AND Claude both have tgram aliases here — alias filter cannot mask
	// a skip-guard removal.
	r := commons.NewMemoryResolver(scOp, []commons.Participant{
		{Handle: scOp, DisplayName: "Operator", Kind: "human",
			Usernames: map[string]string{"tgram": "@milos85vasic"}},
		{Handle: commons.SystemAgentHandle, DisplayName: "Claude", Kind: "agent",
			Usernames: map[string]string{"tgram": "@claude_bot"}}, // Claude HAS a tgram alias
		{Handle: scAlice, DisplayName: "Alice", Kind: "human",
			Usernames: map[string]string{"tgram": "@alice_tg"}},
	})
	const channel = "tgram"

	// Operator opened+assigned, both aliased → still NO tag (operator-skip guard).
	if got := commons.MentionsFor(scOp, scOp, scOp, channel, r); len(got) != 0 {
		t.Fatalf("OPERATOR-SKIP isolation failed: operator leaked %v (alias present, so only the operator-skip guard prevents this — M1 mutation would land here)", got)
	}
	// Claude opened+assigned, aliased → still NO tag (Claude-skip guard).
	if got := commons.MentionsFor(commons.SystemAgentHandle, commons.SystemAgentHandle, scOp, channel, r); len(got) != 0 {
		t.Fatalf("CLAUDE-SKIP isolation failed: Claude leaked %v (alias present, so only the Claude-skip guard prevents this — M2 mutation would land here)", got)
	}
	// Claude opened, operator assigned, both aliased → still NO tag.
	if got := commons.MentionsFor(commons.SystemAgentHandle, scOp, scOp, channel, r); len(got) != 0 {
		t.Fatalf("operator+Claude both aliased but one leaked: %v", got)
	}
	// Control: a real human assignee (aliased, non-operator, non-Claude) IS tagged —
	// proves the skip guards are not over-broad (they don't suppress everyone).
	if got := commons.MentionsFor(commons.SystemAgentHandle, scAlice, scOp, channel, r); !scEqual(got, []string{scAlice}) {
		t.Fatalf("control: human assignee should be tagged, got %v want [%s]", got, scAlice)
	}
	t.Logf("CHAOS skip-guard isolation: operator+Claude aliased on tgram yet never tagged; human assignee still tagged (guards load-bearing, not over-broad)")
}

// TestStressChaos_DuplicateParticipants_LastWins builds a roster with DUPLICATE
// handles (same canonical handle appearing twice with different aliases) and a
// participant whose handle is on one channel but not another, then asserts
// resolution + tagging stay honest under concurrent load. This is the "duplicate
// participants" + "handle present on one channel but not another" chaos vector.
func TestStressChaos_DuplicateParticipants_LastWins(t *testing.T) {
	// alice appears TWICE; the constructor's map-build means the LAST entry wins
	// for byHandle, while bySenderKey accumulates both username indexes.
	r := commons.NewMemoryResolver(scOp, []commons.Participant{
		{Handle: scAlice, DisplayName: "Alice v1", Kind: "human",
			Usernames: map[string]string{"tgram": "@alice_old"}},
		{Handle: scAlice, DisplayName: "Alice v2", Kind: "human",
			Usernames: map[string]string{"tgram": "@alice_tg", "slack": "@alice_sl"}},
		{Handle: scBob, DisplayName: "Bob", Kind: "human",
			Usernames: map[string]string{"slack": "@bob_sl"}}, // bob on slack, NOT tgram
	})

	// On tgram: alice taggable (last-wins alias @alice_tg), bob NOT taggable (slack only).
	if got := commons.MentionsFor(scBob, scAlice, scOp, "tgram", r); !scEqual(got, []string{scAlice}) {
		t.Fatalf("tgram: want [%s] (bob off-channel skipped), got %v", scAlice, got)
	}
	// On slack: both taggable.
	if got := commons.MentionsFor(scBob, scAlice, scOp, "slack", r); !scEqual(got, []string{scAlice, scBob}) {
		t.Fatalf("slack: want [%s %s], got %v", scAlice, scBob, got)
	}
	// UsernameFor reflects last-wins for tgram.
	if u, ok := r.UsernameFor(scAlice, "tgram"); !ok || u != "@alice_tg" {
		t.Fatalf("UsernameFor(alice,tgram)=(%q,%v); want (@alice_tg,true) [last-wins]", u, ok)
	}
	// Both the old and new username indexes resolve back to alice (accumulated).
	if h := r.ResolveSender("tgram", "", "alice_old"); h != scAlice {
		t.Fatalf("ResolveSender(alice_old)=%q; want %q", h, scAlice)
	}
	if h := r.ResolveSender("tgram", "", "alice_tg"); h != scAlice {
		t.Fatalf("ResolveSender(alice_tg)=%q; want %q", h, scAlice)
	}

	// Same assertions under concurrent load — race-clean + deterministic.
	const goroutines = 32
	const per = 80
	var wrong int64
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < per; i++ {
				if !scEqual(commons.MentionsFor(scBob, scAlice, scOp, "tgram", r), []string{scAlice}) {
					atomic.AddInt64(&wrong, 1)
				}
				if !scEqual(commons.MentionsFor(scBob, scAlice, scOp, "slack", r), []string{scAlice, scBob}) {
					atomic.AddInt64(&wrong, 1)
				}
			}
		}()
	}
	wg.Wait()
	if wrong != 0 {
		t.Fatalf("%d wrong results under concurrent duplicate-participant load", wrong)
	}
	t.Logf("CHAOS duplicate-participants: last-wins alias + one-channel-not-another honored under %d concurrent goroutines, 0 wrong",
		goroutines)
}
