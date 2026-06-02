package claude_code

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/vasic-digital/herald/commons"
)

// TestFormatEnvelope_ThreadContextRendered proves that a DispatchRequest WITH
// ThreadContext renders a delimited THREAD CONTEXT block containing each prior
// message's text + the bound-by-meaning instruction, and that bot-authored
// prior messages are labelled distinctly.
func TestFormatEnvelope_ThreadContextRendered(t *testing.T) {
	d, _ := New("claude", t.TempDir(), "ATMOSphere")
	req := DispatchRequest{
		InboundID:   "INB-9",
		Sender:      "tgram:bob",
		Channel:     commons.ChannelTelegram,
		UserMessage: "so what's the next step?",
		Classification: Classification{
			Type: "query", Criticality: "low", Confidence: 0.5,
		},
		Conversation: "(thread)",
		ThreadContext: []commons.ThreadMessage{
			{SenderHandle: "@bob", SenderIsBot: false, Text: "the telemetry pipe is dropping events", Timestamp: time.Unix(1, 0)},
			{SenderHandle: "Claude", SenderIsBot: true, Text: "I opened ATM-42 to track it", Timestamp: time.Unix(2, 0)},
		},
	}
	out := d.FormatEnvelope(req)

	checks := []string{
		"THREAD CONTEXT — this message is part of an existing thread; it has a MEANING and a SUBJECT",
		"Prior messages (oldest first):",
		"the telemetry pipe is dropping events",
		"I opened ATM-42 to track it",
		"context warrants a contribution",
		"@bob said:",         // human prior message, NOT labelled bot
		"Claude | bot said:", // bot-authored prior message labelled distinctly
		// WHO — participants are listed (incl. the current sender's handle).
		"Participants:",
		"@bob (human)",
		"Claude (bot)",
		// WHAT — the thread references an existing workable item (ATM-42), so the
		// SUBJECT line names it and binds the reply to that item.
		"SUBJECT: this thread references existing workable item(s): ATM-42",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("envelope missing %q\n---ENVELOPE---\n%s", want, out)
		}
	}

	// The block must appear BEFORE the user message (it is context for it).
	tcIdx := strings.Index(out, "THREAD CONTEXT")
	umIdx := strings.Index(out, "User message:")
	if tcIdx < 0 || umIdx < 0 || tcIdx > umIdx {
		t.Fatalf("THREAD CONTEXT block must precede the user message (tc=%d um=%d)", tcIdx, umIdx)
	}

	// The bound-by-meaning instruction must be present in the WithPreText
	// variant too (it composes FormatEnvelope).
	pre := d.FormatEnvelopeWithPreText(req, "tgram")
	if !strings.Contains(pre, "THREAD CONTEXT") {
		t.Errorf("FormatEnvelopeWithPreText dropped the THREAD CONTEXT block\n%s", pre)
	}
}

// TestThreadContext_SubjectClassificationNoRef proves that when the thread has
// NO workable-item reference, the envelope instructs the LLM to CLASSIFY the
// subject (newly-reported issue / existing item / system-or-project event)
// rather than naming a specific item (operator clarification 2026-06-02).
func TestThreadContext_SubjectClassificationNoRef(t *testing.T) {
	d, _ := New("claude", t.TempDir(), "ATMOSphere")
	req := DispatchRequest{
		InboundID:   "INB-10",
		Sender:      "slack:carol",
		Channel:     commons.ChannelSlack,
		UserMessage: "and is the dashboard affected too?",
		ThreadContext: []commons.ThreadMessage{
			{SenderHandle: "carol", SenderIsBot: false, Text: "the login page is slow this morning", Timestamp: time.Unix(1, 0)},
			{SenderHandle: "dave", SenderIsBot: false, Text: "yeah I saw 500s as well", Timestamp: time.Unix(2, 0)},
		},
	}
	out := d.FormatEnvelope(req)
	// No HRD-/ATM- ref anywhere → the classify instruction, NOT a "references
	// existing workable item(s)" line.
	if strings.Contains(out, "references existing workable item(s)") {
		t.Errorf("no workable-item ref present, but the envelope named one:\n%s", out)
	}
	for _, want := range []string{
		"NEWLY-REPORTED issue",
		"EXISTING ticket / workable item",
		"SYSTEM / PROJECT event",
		"carol (human)", "dave (human)", // both prior participants listed
	} {
		if !strings.Contains(out, want) {
			t.Errorf("envelope missing %q\n%s", want, out)
		}
	}
}

// TestDetectWorkableRefs proves the deterministic subject-entity scan finds
// HRD-/ATM- references in BOTH the prior thread messages and the current user
// message, de-duplicated + upper-cased.
func TestDetectWorkableRefs(t *testing.T) {
	req := DispatchRequest{
		UserMessage: "does this also block hrd-7?",
		ThreadContext: []commons.ThreadMessage{
			{Text: "opened ATM-42 for the pipe"},
			{Text: "also see Atm-42 and HRD-7"}, // dupes (case-insensitive)
		},
	}
	got := detectWorkableRefs(req)
	// Expect exactly ATM-42 and HRD-7 (order = first-seen: ATM-42 then HRD-7).
	want := []string{"ATM-42", "HRD-7"}
	if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", want) {
		t.Fatalf("detectWorkableRefs: got %v want %v", got, want)
	}
}

// TestFormatEnvelope_EmptyThreadContextUnchanged is the §107 anti-bluff guard:
// a request with EMPTY ThreadContext renders NO THREAD CONTEXT block, and the
// envelope is BYTE-IDENTICAL to the same request built without the field set.
// This proves the feature is purely additive for the no-context case.
func TestFormatEnvelope_EmptyThreadContextUnchanged(t *testing.T) {
	d, _ := New("claude", t.TempDir(), "ATMOSphere")
	base := DispatchRequest{
		InboundID:   "INB-1",
		Sender:      "tgram:alice",
		Channel:     commons.ChannelTelegram,
		UserMessage: "Bug: telemetry pipe drops every hour",
		Classification: Classification{
			Type: "bug", Criticality: "high", Confidence: 0.92,
		},
		Conversation: "(no prior thread)",
		Attachments:  []commons.Attachment{{Filename: "log.txt", MIMEType: "text/plain", SizeBytes: 1024}},
	}

	withNil := d.FormatEnvelope(base) // ThreadContext is the nil zero value

	withEmpty := base
	withEmpty.ThreadContext = []commons.ThreadMessage{} // non-nil but empty
	gotEmpty := d.FormatEnvelope(withEmpty)

	if withNil != gotEmpty {
		t.Fatalf("empty ThreadContext changed the envelope:\n--- nil ---\n%s\n--- empty ---\n%s", withNil, gotEmpty)
	}
	if strings.Contains(withNil, "THREAD CONTEXT") {
		t.Fatalf("no-context envelope must NOT contain a THREAD CONTEXT block:\n%s", withNil)
	}

	// Pre-text variant: the same no-context invariant holds.
	preNil := d.FormatEnvelopeWithPreText(base, "tgram")
	if strings.Contains(preNil, "THREAD CONTEXT") {
		t.Fatalf("no-context pre-text envelope must NOT contain a THREAD CONTEXT block:\n%s", preNil)
	}
}

// TestFormatEnvelope_ThreadContextTruncatesAndCaps proves the count cap (last
// threadContextMaxMessages rendered + elision line) and per-message text
// truncation behave — so a long/verbose thread cannot blow up the envelope.
func TestFormatEnvelope_ThreadContextTruncatesAndCaps(t *testing.T) {
	d, _ := New("claude", t.TempDir(), "ATMOSphere")

	msgs := make([]commons.ThreadMessage, threadContextMaxMessages+5)
	for i := range msgs {
		msgs[i] = commons.ThreadMessage{SenderHandle: fmt.Sprintf("@u%d", i), Text: fmt.Sprintf("m%d", i)}
	}
	// Make the LAST message very long to exercise truncation.
	msgs[len(msgs)-1].Text = strings.Repeat("x", threadContextMaxTextLen+50)

	out := d.FormatEnvelope(DispatchRequest{UserMessage: "now", ThreadContext: msgs})

	if !strings.Contains(out, "earlier message(s) elided") {
		t.Errorf("expected an elision line for the %d capped messages\n%s", len(msgs)-threadContextMaxMessages, out)
	}
	// The 5 oldest messages must NOT appear (m0..m4 elided).
	if strings.Contains(out, "@u0 said: m0") {
		t.Errorf("oldest message should have been elided\n%s", out)
	}
	// Truncated long text: the ellipsis marker present, the full-length run absent.
	if !strings.Contains(out, "…") {
		t.Errorf("expected truncation ellipsis in rendered block\n%s", out)
	}
	if strings.Contains(out, strings.Repeat("x", threadContextMaxTextLen+50)) {
		t.Errorf("over-length prior text was not truncated\n%s", out)
	}
}
