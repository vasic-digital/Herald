package inbound_test

import (
	"context"
	"strings"
	"testing"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/pherald/internal/inbound"
)

// reply_delivery_test.go — HRD-159 Herald-side reply-DELIVERY plumbing
// regression-guard (full-automation per §11.4.98; no live network/creds).
//
// SCOPE / HONESTY: this test proves the Herald-SIDE pipeline that posts a
// reply BACK to a channel — inbound event → Dispatcher → CC dispatch seam →
// ParseReply(<<<HERALD-REPLY>>>) → SendReply to the channel sink. It does NOT
// prove that a real fresh Claude bootstrap session honours the standing reply
// contract (that requires a live run); the bootstrap-seeding contract itself
// is exercised by commons_messaging/dispatch/claude_code/bootstrap_seed_reply_test.go.
// Here we lock in the leg that USES that reply: WHEN the session yields a
// non-empty reply, Herald actually DELIVERS it; WHEN it yields nothing, Herald
// delivers nothing (the §107 empty-reply guard) rather than crashing or
// posting an empty body.
//
// HOW IT BITES:
//   - Positive: stubCode returns a NON-EMPTY <<<HERALD-REPLY>>>; the test
//     asserts the recordingReplier captured a NON-EMPTY body that EQUALS the
//     stub's reply text. A dispatcher that dropped the reply, posted the wrong
//     text, or never reached the delivery leg FAILS.
//   - Paired negative: stubCode returns an EMPTY reply text; the test asserts
//     NOTHING was posted (the guard path). If the positive assertion were
//     satisfied by some canned/always-on send, this negative would FAIL —
//     proving the positive genuinely depends on real delivered content.

// plainQuery is a conversational message that does NOT match the deterministic
// Tier-1 CommandRecognizer (no imperative verb + resolvable target), so the
// Dispatcher falls through to the Tier-2 CC dispatch seam — the leg under test.
// Mirrors TestDispatcherCCPathTakenForPlainQuery's body, which already asserts
// this phrasing reaches CC (calls == 1).
const plainQuery = "hey what's up over there?"

// TestDispatcherDeliversNonEmptyReplyEndToEnd is the POSITIVE plumbing proof:
// a plain inbound query drives the full inbound→dispatch→parse→deliver pipeline
// and the non-empty reply the (stubbed) session yields is actually POSTED BACK
// to the channel sink, verbatim.
func TestDispatcherDeliversNonEmptyReplyEndToEnd(t *testing.T) {
	const replyText = "Got it — handling your request now."

	rr := &recordingReplier{}
	d, err := inbound.NewDispatcher(inbound.Config{
		ProjectName: "T",
		Code:        stubCode{stdout: `<<<HERALD-REPLY>>> {"action":"reply","text":"` + replyText + `"}`},
		Reply:       rr,
	})
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}

	ev := commons.InboundEvent{
		EventID: "01HREPLYDELIVERY",
		Sender:  commons.Recipient{Channel: "tgram", ChannelUserID: "55555"},
		Body:    commons.Body{Plain: plainQuery},
		Raw:     map[string]any{"message_id": 4242},
	}
	if err := d.Handle(context.Background(), ev); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	// The delivery leg must have fired.
	if !rr.called {
		t.Fatal("reply-DELIVERY: SendReply was never called — the reply was parsed but never posted back to the channel")
	}
	// The DELIVERED body must be non-empty (the HRD-159 failure mode was an
	// empty body that the listener logged-and-dropped).
	if strings.TrimSpace(rr.lastText) == "" {
		t.Fatal("reply-DELIVERY: posted-back body is empty — nothing reached the channel (HRD-159 failure mode)")
	}
	// And it must be EXACTLY the reply the session yielded — proving the
	// content flowed through ParseReply → actReply → SendReply intact.
	if rr.lastText != replyText {
		t.Fatalf("reply-DELIVERY: posted-back body = %q, want %q (content corrupted on the delivery leg)", rr.lastText, replyText)
	}
	// And it threaded onto the originating message (delivery is in-reply-to).
	if rr.lastReplyToRaw != "4242" {
		t.Fatalf("reply-DELIVERY: thread-parent id = %q, want %q", rr.lastReplyToRaw, "4242")
	}
}

// TestDispatcherDeliversNothingWhenSessionYieldsEmpty is the PAIRED NEGATIVE:
// when the (stubbed) session yields an EMPTY reply text, the pipeline must
// deliver NOTHING — the §107 empty-reply guard. This is the control that makes
// the positive test genuinely bite: if the positive PASS were produced by a
// canned/always-on send rather than the real delivered content, this assertion
// would FAIL (something would be posted despite empty content).
func TestDispatcherDeliversNothingWhenSessionYieldsEmpty(t *testing.T) {
	rr := &recordingReplier{}
	d, err := inbound.NewDispatcher(inbound.Config{
		ProjectName: "T",
		Code:        stubCode{stdout: `<<<HERALD-REPLY>>> {"action":"reply","text":""}`},
		Reply:       rr,
	})
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}

	ev := commons.InboundEvent{
		EventID: "01HREPLYEMPTY",
		Sender:  commons.Recipient{Channel: "tgram", ChannelUserID: "55555"},
		Body:    commons.Body{Plain: plainQuery},
		Raw:     map[string]any{"message_id": 4242},
	}
	// Must not error — an empty reply must not fail-loud the listener.
	if err := d.Handle(context.Background(), ev); err != nil {
		t.Fatalf("Handle (empty reply) must not error: %v", err)
	}
	// Must not have posted anything: adapters reject an empty body, so the
	// guard short-circuits delivery.
	if rr.called {
		t.Fatalf("reply-DELIVERY guard: SendReply fired with an empty body (got %q) — empty reply must be dropped, not posted", rr.lastText)
	}
}
