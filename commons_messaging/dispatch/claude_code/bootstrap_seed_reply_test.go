package claude_code

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vasic-digital/herald/commons"
)

// HRD-159 — bootstrap-session reply-DELIVERY seeding.
//
// THE BUG: `pherald listen` bootstraps a FRESH, context-less Claude session
// per the §11.4.98 no-collision rule (a dedicated session UUID). Such a
// session, lacking any standing instruction, frequently returned an EMPTY
// reply for the first real inbound message; Dispatch then surfaced an empty
// Summary/Details, the listener logged "reply skipped — empty reply text",
// and NOTHING was posted back to the messenger. The autonomy chain (inbound →
// dispatch → reply) was proven end-to-end, but the final reply-DELIVERY leg
// silently no-op'd because there was no content to deliver.
//
// THE FIX (option a): seed the bootstrap turn — which `--resume` replays
// verbatim into every subsequent dispatch — with a STANDING CONTRACT that
// every future inbound message MUST produce a NON-EMPTY <<<HERALD-REPLY>>>.
//
// HOW THESE TESTS BITE (fully self-driving, no live claude / no creds): a fake
// `claude` binary simulates a session with memory. During bootstrap it scans
// the prompt argv for the standing-contract seeding and records a flag file;
// during the later `--resume` dispatch it emits a NON-EMPTY reply IFF the flag
// is set (the seeded contract was honoured) and an EMPTY reply otherwise. This
// mirrors the real-world behaviour: a seeded session produces content, an
// unseeded one produces empty. The positive test proves Dispatch now yields a
// non-empty reply; the paired negative proves that WITHOUT the seeding the
// same path yields empty — so the assertion genuinely depends on the fix.

// writeSeedAwareFakeClaude writes a fake `claude` that behaves like a
// stateful session:
//
//   - On the bootstrap call (`--session-id ...`): it scans its argv for the
//     marker string `markerSentinel`. If found, it touches <flagFile> to
//     record that the standing-contract seeding was present. It always prints
//     a non-empty bootstrap ack (so the §107 bootstrap bluff-guard passes).
//   - On the dispatch call (`--resume ...`): if <flagFile> exists, it prints a
//     NON-EMPTY <<<HERALD-REPLY>>> whose summary echoes a real message; if not,
//     it prints an EMPTY-summary reply (the pre-fix delivery failure).
//
// The script keys on whether `--session-id` or `--resume` appears in argv,
// exactly like the real claude invocations Herald constructs.
func writeSeedAwareFakeClaude(t *testing.T, markerSentinel, flagFile string) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "fake-claude-seed")
	// POSIX sh. We pass the sentinel + flag path via env (HERALD_SEED_SENTINEL /
	// HERALD_SEED_FLAG) baked into the script so there is no brittle argv-glob
	// quoting. "$*" is the full argv; we test the invocation mode by glob and
	// scan it for the sentinel with `case`. printf keeps newline semantics
	// deterministic.
	script := `#!/bin/sh
SENTINEL=` + shellQuote(markerSentinel) + `
FLAG=` + shellQuote(flagFile) + `
ARGS="$*"
case "$ARGS" in
  *--session-id*)
    # Bootstrap leg: detect the standing-contract seeding in the prompt.
    case "$ARGS" in
      *"$SENTINEL"*) : > "$FLAG" ;;
    esac
    printf '%s\n' '<<<HERALD-REPLY>>> {"outcome":"answered","summary":"bootstrap ack","details":"ok","affected_paths":[],"reproduction_steps":[],"estimated_effort":"S","follow_up_questions":[]}'
    exit 0
    ;;
  *--resume*)
    # Dispatch leg: a seeded session returns NON-EMPTY content; an
    # unseeded session returns an EMPTY summary (the pre-fix bug).
    if [ -f "$FLAG" ]; then
      printf '%s\n' '<<<HERALD-REPLY>>> {"outcome":"answered","summary":"Got it — handling your request now.","details":"seeded session produced a real reply","affected_paths":[],"reproduction_steps":[],"estimated_effort":"S","follow_up_questions":[]}'
    else
      printf '%s\n' '<<<HERALD-REPLY>>> {"outcome":"answered","summary":"","details":"","affected_paths":[],"reproduction_steps":[],"estimated_effort":"S","follow_up_questions":[]}'
    fi
    exit 0
    ;;
esac
exit 0
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write seed-aware fake binary: %v", err)
	}
	return bin
}

func typicalInboundReq() DispatchRequest {
	return DispatchRequest{
		InboundID:    "INB-HRD159-1",
		Sender:       "tgram:subscriber",
		Channel:      commons.ChannelTelegram,
		Conversation: "(no prior thread)",
		UserMessage:  "Hi Herald, can you tell me the status of the deploy?",
		Classification: Classification{
			Type:        "query",
			Criticality: "low",
			Confidence:  0.9,
		},
	}
}

// TestBootstrapSeedingProducesNonEmptyReply is the POSITIVE HRD-159 evidence:
// a fresh (no-anchor) dispatcher bootstraps a session WHOSE BOOTSTRAP PROMPT
// CARRIES the standing-contract seeding, then a typical inbound message is
// dispatched, and the dispatcher returns a NON-EMPTY reply — i.e. the
// reply-DELIVERY leg now has content to post back.
func TestBootstrapSeedingProducesNonEmptyReply(t *testing.T) {
	// The sentinel is a literal substring of the seeded bootstrapPrompt. If a
	// regression strips the standing contract from bootstrapPrompt, this
	// sentinel will no longer be present in the bootstrap argv, the flag file
	// will not be created, and the dispatch leg will return EMPTY — failing
	// the assertion below. So the sentinel choice ties the test to the fix.
	const sentinel = "STANDING CONTRACT FOR EVERY FUTURE INBOUND MESSAGE"
	if !strings.Contains(bootstrapPrompt, sentinel) {
		t.Fatalf("guard: bootstrapPrompt no longer contains the standing-contract seeding %q — HRD-159 fix was removed or reworded; update this test deliberately", sentinel)
	}

	flagFile := filepath.Join(t.TempDir(), "seeded.flag")
	fakeBin := writeSeedAwareFakeClaude(t, sentinel, flagFile)

	workdir := t.TempDir()
	d, err := New(fakeBin, workdir, "HRD159SeedProj")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// No anchor pre-exists → Dispatch will bootstrap first, then resume.
	_, anchor, _ := d.ResolveSession()
	if _, statErr := os.Stat(anchor); !os.IsNotExist(statErr) {
		t.Fatalf("anchor must not exist pre-test; stat err=%v", statErr)
	}

	resp, err := d.Dispatch(t.Context(), typicalInboundReq())
	if err != nil {
		t.Fatalf("Dispatch (with seeding): %v", err)
	}

	// The fake recorded the seeding flag during bootstrap, proving the
	// standing-contract text actually reached the bootstrap argv.
	if _, statErr := os.Stat(flagFile); statErr != nil {
		t.Fatalf("seeding flag not created — the standing-contract block never reached the bootstrap claude argv (stat err=%v)", statErr)
	}

	// Reply-DELIVERY content: Summary is the human-facing field the listener
	// posts back. It MUST be non-empty now.
	if strings.TrimSpace(resp.Summary) == "" {
		t.Fatalf("HRD-159: seeded bootstrap session produced an EMPTY reply summary — reply-DELIVERY leg still has no content to post (resp=%+v)", resp)
	}
	if resp.Summary != "Got it — handling your request now." {
		t.Fatalf("unexpected reply summary %q — fake binary contract drift", resp.Summary)
	}
}

// TestWithoutBootstrapSeeding_ReplyIsEmpty is the PAIRED NEGATIVE: it drives
// the SAME dispatcher/fake-binary path but the fake's bootstrap leg is told to
// look for a sentinel that the seeded prompt does NOT contain — simulating the
// pre-fix world where the bootstrap turn carried no standing reply contract.
// In that world the seeding flag is never set and the dispatch leg returns an
// EMPTY reply. This proves the positive test genuinely bites on the seeding:
// remove the seeding and the delivery leg goes empty again.
func TestWithoutBootstrapSeeding_ReplyIsEmpty(t *testing.T) {
	// A sentinel that is GUARANTEED absent from bootstrapPrompt → models the
	// un-seeded bootstrap.
	const absentSentinel = "THIS-STRING-IS-NEVER-IN-THE-BOOTSTRAP-PROMPT-HRD159"
	if strings.Contains(bootstrapPrompt, absentSentinel) {
		t.Fatalf("test setup error: chosen absent-sentinel unexpectedly present in bootstrapPrompt")
	}

	flagFile := filepath.Join(t.TempDir(), "seeded.flag")
	fakeBin := writeSeedAwareFakeClaude(t, absentSentinel, flagFile)

	d, err := New(fakeBin, t.TempDir(), "HRD159NoSeedProj")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := d.Dispatch(t.Context(), typicalInboundReq())
	if err != nil {
		t.Fatalf("Dispatch (no seeding): %v", err)
	}

	// Flag MUST NOT have been set (sentinel absent from the prompt).
	if _, statErr := os.Stat(flagFile); statErr == nil {
		t.Fatalf("seeding flag was created despite absent sentinel — negative-control invalid")
	}

	// Pre-fix behaviour: empty reply → nothing to deliver.
	if strings.TrimSpace(resp.Summary) != "" {
		t.Fatalf("negative control expected EMPTY reply summary (un-seeded session) but got %q — the positive test would not genuinely bite", resp.Summary)
	}
}
