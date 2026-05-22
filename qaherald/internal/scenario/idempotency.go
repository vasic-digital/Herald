// Wave 5 Task 5 Scenario #3 — idempotency-replay.
//
// POST the same event_id twice and assert:
//   - First response: 202 Accepted + recipients ≥ 1 + no
//     X-Herald-Replay header.
//   - Second response: 200 OK + recipients == 0 + X-Herald-Replay:
//     true.
//
// The X-Herald-Replay contract lives in pherald's idempotency layer
// (HRD-xxx — see specification.V3.md). Scenarios assert the wire
// header verbatim; a header rename would surface here immediately.
//
// §107 bidirectional invariant: emits 2 KindHeraldPost + 2
// KindHeraldResponse + 1 KindTGReceive (first POST's delivery only;
// the replay has zero recipients). Bidirectional minimum holds.
package scenario

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	tele "gopkg.in/telebot.v3"

	"github.com/vasic-digital/herald/qaherald/internal/herald"
	"github.com/vasic-digital/herald/qaherald/internal/transcript"
)

func init() {
	Register(Scenario{
		Name:        "idempotency-replay",
		Description: "Same event_id POSTed twice → 202+recipients then 200+X-Herald-Replay:true+recipients=0",
		Run:         runIdempotency,
	})
}

func runIdempotency(ctx context.Context, o *Orchestrator) error {
	eventID := fmt.Sprintf("qaherald-idem-%d", o.now().UnixNano())
	ce := herald.CloudEvent{
		SpecVersion: "1.0",
		ID:          eventID,
		Source:      "qaherald",
		Type:        "qa.idempotency",
		Time:        o.now().UTC(),
		Data:        json.RawMessage(`{"message":"qaherald idempotency"}`),
	}

	// --- First POST: expect 202 + recipients ≥ 1 + no replay header.
	cePayload, _ := json.Marshal(ce)
	_ = o.Transcript.Append(transcript.Event{
		Direction: transcript.DirectionOut,
		Kind:      transcript.KindHeraldPost,
		Scenario:  "idempotency-replay",
		Payload:   cePayload,
		Note:      "first POST (Accept: application/toon)",
	})
	receipt1, status1, hdr1, err := o.Herald.PostEvent(ctx, ce, herald.AcceptTOON)
	receipt1Payload, _ := json.Marshal(receipt1)
	_ = o.Transcript.Append(transcript.Event{
		Direction: transcript.DirectionIn,
		Kind:      transcript.KindHeraldResponse,
		Scenario:  "idempotency-replay",
		Payload:   receipt1Payload,
		Note:      fmt.Sprintf("first POST: status=%d X-Herald-Replay=%q recipients=%d", status1, hdr1.Get("X-Herald-Replay"), receipt1.Recipients),
	})
	if err != nil {
		return fmt.Errorf("first PostEvent: %w", err)
	}
	if err := assertStatus(http.StatusAccepted, status1, "idempotency first POST"); err != nil {
		return err
	}
	if receipt1.Recipients < 1 {
		return fmt.Errorf("idempotency first POST: expected recipients>=1, got %d", receipt1.Recipients)
	}
	if got := hdr1.Get("X-Herald-Replay"); got != "" {
		return fmt.Errorf("idempotency first POST: expected absent X-Herald-Replay, got %q", got)
	}

	// Cross-check the Telegram-side delivery of the first POST.
	msg, err := o.TG.WaitForMessage(30*time.Second, func(m tele.Message) bool {
		return containsCloudEventID(m.Text, m.Caption, ce.ID)
	})
	if err != nil {
		return fmt.Errorf("WaitForMessage (first POST delivery): %w", err)
	}
	msgPayload, _ := json.Marshal(map[string]any{
		"message_id": msg.ID,
		"chat_id":    chatIDOf(msg),
		"text":       msg.Text,
	})
	_ = o.Transcript.Append(transcript.Event{
		Direction: transcript.DirectionIn,
		Kind:      transcript.KindTGReceive,
		Scenario:  "idempotency-replay",
		Payload:   msgPayload,
		Note:      "first POST delivery",
	})

	// --- Second POST (same event_id): expect 200 + recipients == 0
	//     + X-Herald-Replay: true.
	_ = o.Transcript.Append(transcript.Event{
		Direction: transcript.DirectionOut,
		Kind:      transcript.KindHeraldPost,
		Scenario:  "idempotency-replay",
		Payload:   cePayload,
		Note:      "second POST (replay)",
	})
	receipt2, status2, hdr2, err := o.Herald.PostEvent(ctx, ce, herald.AcceptTOON)
	receipt2Payload, _ := json.Marshal(receipt2)
	_ = o.Transcript.Append(transcript.Event{
		Direction: transcript.DirectionIn,
		Kind:      transcript.KindHeraldResponse,
		Scenario:  "idempotency-replay",
		Payload:   receipt2Payload,
		Note:      fmt.Sprintf("second POST: status=%d X-Herald-Replay=%q recipients=%d", status2, hdr2.Get("X-Herald-Replay"), receipt2.Recipients),
	})
	if err != nil {
		return fmt.Errorf("second PostEvent: %w", err)
	}
	if err := assertStatus(http.StatusOK, status2, "idempotency second POST"); err != nil {
		return err
	}
	if receipt2.Recipients != 0 {
		return fmt.Errorf("idempotency second POST: expected recipients==0, got %d", receipt2.Recipients)
	}
	if err := assertHeader("true", hdr2.Get("X-Herald-Replay"), "idempotency second POST"); err != nil {
		return err
	}
	return nil
}
