// Wave 5 Task 5 Scenario #4 — deny-path-policy-gate.
//
// Goal: a CloudEvent whose type matches an operator-configured deny
// rule (qa.deny-me) MUST get 403 + zero Telegram deliveries within a
// 5-second window.
//
// Pre-condition (operator setup, T8): cherald compliance or sherald
// safety has a deny rule for type=qa.deny-me. T8 documents the rule;
// T5 ships the scenario as-coded — if the rule is missing, the
// scenario FAILs the status assertion, which is the correct §107
// signal (deny-path bluff would otherwise PASS).
//
// §107 bidirectional invariant: emits 1 KindHeraldPost + 1
// KindHeraldResponse + 1 KindTGReceive — the KindTGReceive is the
// "zero-deliveries-observed" sentinel event (Payload encodes the
// elapsed wait + the observation that no qualifying message arrived).
// That sentinel keeps the bidirectional minimum honest: a deny scenario
// that emits no TG event would silently pass the ≥1 invariant by
// elision; a sentinel event explicitly asserts the absence of a real
// delivery.
package scenario

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	tele "gopkg.in/telebot.v3"

	"github.com/vasic-digital/herald/qaherald/internal/herald"
	"github.com/vasic-digital/herald/qaherald/internal/transcript"
)

func init() {
	Register(Scenario{
		Name:        "deny-path-policy-gate",
		Description: "CloudEvent matching operator deny rule → 403 + zero Telegram deliveries in 5s window",
		Run:         runDenyPath,
	})
}

func runDenyPath(ctx context.Context, o *Orchestrator) error {
	ce := herald.CloudEvent{
		SpecVersion: "1.0",
		ID:          fmt.Sprintf("qaherald-deny-%d", o.now().UnixNano()),
		Source:      "qaherald",
		Type:        "qa.deny-me", // operator-configured deny target (see T8 setup notes)
		Time:        o.now().UTC(),
		Data:        json.RawMessage(`{"message":"qaherald deny-path probe"}`),
	}
	cePayload, _ := json.Marshal(ce)

	_ = o.Transcript.Append(transcript.Event{
		Direction: transcript.DirectionOut,
		Kind:      transcript.KindHeraldPost,
		Scenario:  "deny-path-policy-gate",
		Payload:   cePayload,
		Note:      "Accept: application/toon (expect 403)",
	})
	receipt, status, headers, err := o.Herald.PostEvent(ctx, ce, herald.AcceptTOON)
	receiptPayload, _ := json.Marshal(receipt)
	_ = o.Transcript.Append(transcript.Event{
		Direction: transcript.DirectionIn,
		Kind:      transcript.KindHeraldResponse,
		Scenario:  "deny-path-policy-gate",
		Payload:   receiptPayload,
		Note:      fmt.Sprintf("status=%d content-type=%s", status, headers.Get("Content-Type")),
	})
	// Network-layer errors here are themselves a deny-path FAIL — we
	// expect a real 403 response, not a connection failure.
	if err != nil {
		return fmt.Errorf("PostEvent: %w", err)
	}
	if err := assertStatus(http.StatusForbidden, status, "deny-path PostEvent"); err != nil {
		return err
	}

	// Wait 5 seconds and assert no qualifying Telegram message
	// arrives. We use a short-window WaitForMessage with a CE-ID
	// predicate; context.DeadlineExceeded is the expected success path.
	const window = 5 * time.Second
	startWait := o.now()
	msg, err := o.TG.WaitForMessage(window, func(m tele.Message) bool {
		return containsCloudEventID(m.Text, m.Caption, ce.ID)
	})
	elapsed := o.now().Sub(startWait)
	if err == nil {
		// A matching message arrived inside the window — the deny
		// gate leaked.
		leakPayload, _ := json.Marshal(map[string]any{
			"message_id": msg.ID,
			"chat_id":    chatIDOf(msg),
			"text":       msg.Text,
			"elapsed_ms": elapsed.Milliseconds(),
		})
		_ = o.Transcript.Append(transcript.Event{
			Direction: transcript.DirectionIn,
			Kind:      transcript.KindTGReceive,
			Scenario:  "deny-path-policy-gate",
			Payload:   leakPayload,
			Note:      "DENY-PATH LEAK: matching Telegram message arrived inside the deny window",
		})
		return fmt.Errorf("deny-path leak: matching Telegram delivery for event %q arrived after %s (expected zero)", ce.ID, elapsed)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("WaitForMessage (deny-window): unexpected error %w", err)
	}
	// Zero-delivery sentinel — log the observation so the
	// bidirectional invariant has a real Telegram-side event.
	sentinelPayload, _ := json.Marshal(map[string]any{
		"window_seconds": window.Seconds(),
		"observed":       0,
		"event_id":       ce.ID,
	})
	_ = o.Transcript.Append(transcript.Event{
		Direction: transcript.DirectionIn,
		Kind:      transcript.KindTGReceive,
		Scenario:  "deny-path-policy-gate",
		Payload:   sentinelPayload,
		Note:      "deny-path sentinel: zero qualifying Telegram messages observed within window",
	})
	return nil
}
