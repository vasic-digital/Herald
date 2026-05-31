package inbound_test

import (
	"context"
	"strings"
	"testing"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/pherald/internal/inbound"
)

// clarifyStdout is the TIER 3 CC reply: the LLM could not determine the intent
// with confidence, so it returns action=clarify with a precise question naming
// the candidate intents (docs/design/INTENT_RECOGNITION.md §3).
const clarifyStdout = `<<<HERALD-REPLY>>> {"action":"clarify","question":"did you want to close ATM-9, reassign it, or just get its status?"}`

// TestClarify_E2E_TagsSenderAndAsks is the TIER 3 anti-bluff E2E
// (docs/design/INTENT_RECOGNITION.md §6). An ambiguous message drives a REAL
// dispatch (through the production Dispatcher.Handle path, CC stub returning
// action=clarify) whose recording-sink reply body is EXACTLY
// "@<sender> <specific question>" — proving the user is tagged AND asked, not
// ignored. The recording sink is the same recordingReplier the other inbound
// tests use (not a mock that asserts nothing).
func TestClarify_E2E_TagsSenderAndAsks(t *testing.T) {
	// A known participant @carol with a tgram alias — UsernameFor resolves the
	// per-channel @username for the tag.
	resolver := commons.NewMemoryResolver("@milos85vasic", []commons.Participant{
		{
			Handle:    "@carol",
			Kind:      "human",
			Usernames: map[string]string{"tgram": "@carol"},
		},
	})
	rec := &recordingReplier{}
	d, err := inbound.NewDispatcher(inbound.Config{
		ProjectName: "CLARIFY",
		Code:        stubCode{stdout: clarifyStdout},
		Reply:       rec,
		Resolver:    resolver,
	})
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}

	// An ambiguous message that Tier 1 declines (no clear imperative+target)
	// and that the (stubbed) LLM resolves to clarify.
	ev := evWithSender("carol", "555", "hey can you do the ATM-9 thing")
	if err := d.Handle(context.Background(), ev); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if !rec.called {
		t.Fatal("clarify did NOT reach the reply sink — the user was ignored (§107 bluff)")
	}

	const wantQuestion = "did you want to close ATM-9, reassign it, or just get its status?"
	wantBody := "@carol " + wantQuestion
	if rec.lastText != wantBody {
		t.Fatalf("clarify body = %q, want EXACTLY %q", rec.lastText, wantBody)
	}

	// Anti-bluff sub-assertions: the @sender tag is present AND the question is
	// non-generic (names the candidate intents).
	if !strings.HasPrefix(rec.lastText, "@carol ") {
		t.Errorf("clarify body %q must START with the @sender tag '@carol '", rec.lastText)
	}
	if !strings.Contains(rec.lastText, "close ATM-9") || !strings.Contains(rec.lastText, "reassign") {
		t.Errorf("clarify question is generic — must name the candidate intents; got %q", rec.lastText)
	}
	// It is threaded back to the original message (reply_to_message_id intact).
	if rec.lastReplyTo != 7 {
		t.Errorf("clarify reply_to = %d, want 7 (threaded to original)", rec.lastReplyTo)
	}
}

// TestClarify_E2E_UnknownSender_FallsBackToRawHandle proves the fallback: a
// first-contact sender (not in the roster) is STILL tagged via the raw
// normalized @username — never left untagged.
func TestClarify_E2E_UnknownSender_FallsBackToRawHandle(t *testing.T) {
	resolver := commons.NewMemoryResolver("@op", nil) // empty roster
	rec := &recordingReplier{}
	d, err := inbound.NewDispatcher(inbound.Config{
		ProjectName: "CLARIFY",
		Code:        stubCode{stdout: clarifyStdout},
		Reply:       rec,
		Resolver:    resolver,
	})
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	ev := evWithSender("stranger", "999", "do the ATM-9 thing")
	if err := d.Handle(context.Background(), ev); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !strings.HasPrefix(rec.lastText, "@stranger ") {
		t.Fatalf("unknown sender must still be tagged; body=%q want prefix '@stranger '", rec.lastText)
	}
}

// TestClarify_NilResolver_Graceful proves a nil resolver does not panic and
// still tags the user via the raw stamped @username.
func TestClarify_NilResolver_Graceful(t *testing.T) {
	rec := &recordingReplier{}
	d, err := inbound.NewDispatcher(inbound.Config{
		ProjectName: "CLARIFY",
		Code:        stubCode{stdout: clarifyStdout},
		Reply:       rec,
		// Resolver intentionally nil.
	})
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	ev := evWithSender("dave", "111", "the ATM-9 thing")
	if err := d.Handle(context.Background(), ev); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !strings.HasPrefix(rec.lastText, "@dave ") {
		t.Fatalf("nil-resolver clarify must still tag the raw @username; body=%q", rec.lastText)
	}
}

// TestClarify_EmptyQuestion_IsLoudError proves a clarify with no question is an
// explicit error (a clarify with no question is itself a §107 bluff — it claims
// to ask but asks nothing).
func TestClarify_EmptyQuestion_IsLoudError(t *testing.T) {
	rec := &recordingReplier{}
	d, err := inbound.NewDispatcher(inbound.Config{
		ProjectName: "CLARIFY",
		Code:        stubCode{stdout: `<<<HERALD-REPLY>>> {"action":"clarify","question":"   "}`},
		Reply:       rec,
		Resolver:    commons.NewMemoryResolver("@op", nil),
	})
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	ev := evWithSender("dave", "111", "the ATM-9 thing")
	if err := d.Handle(context.Background(), ev); err == nil {
		t.Fatal("clarify with an empty question must be a loud error, got nil")
	}
	if rec.called {
		t.Error("empty-question clarify must NOT send a reply (no @sender + blank ask)")
	}
}

// TestClearCommand_DoesNotTriggerClarify is the TIER-3 NEGATIVE
// (docs/design/INTENT_RECOGNITION.md §6): a clear "close ATM-9" routes to
// item.update via TIER 1 and NEVER triggers clarify. We prove it by giving the
// dispatcher a CC stub that would return clarify if it were reached — and
// asserting the CC path is NOT taken (the item mutator fires, the clarify reply
// does NOT).
func TestClearCommand_DoesNotTriggerClarify(t *testing.T) {
	resolver := commons.NewMemoryResolver("@op", nil)
	rec := &recordingReplier{}
	mut := &recordingMutator{}
	d, err := inbound.NewDispatcher(inbound.Config{
		ProjectName: "CLARIFY",
		// If the LLM were ever reached it would clarify — so any clarify reply
		// proves Tier 1 failed to fast-path the clear command.
		Code:     stubCode{stdout: clarifyStdout},
		Reply:    rec,
		Items:    mut,
		Resolver: resolver,
	})
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	ev := evWithSender("carol", "555", "close ATM-9")
	if err := d.Handle(context.Background(), ev); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if mut.updateCalls != 1 {
		t.Fatalf("clear command must route to item.update (Tier 1); updateCalls=%d", mut.updateCalls)
	}
	if mut.lastAtmID != "ATM-9" || mut.lastFields["status"] != "closed" {
		t.Errorf("Tier-1 close routed wrong: atm=%q fields=%v", mut.lastAtmID, mut.lastFields)
	}
	if rec.called {
		t.Errorf("clear command must NOT trigger a clarify reply; got body=%q", rec.lastText)
	}
}
