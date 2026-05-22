// Wave 5 Task 5 Scenario #1 — happy-path-single-channel.
//
// Worked example. All other scenarios mimic this shape:
//   1. Construct a CloudEvent.
//   2. Append a KindHeraldPost transcript event with the outbound
//      payload.
//   3. POST it via Accept: application/toon (exercises Wave 4b).
//   4. Append a KindHeraldResponse transcript event with the receipt
//      + status + content-type.
//   5. Assert status == 202 + receipt.Recipients >= 1.
//   6. WaitForMessage on the Telegram inbox for the pherald-driven
//      delivery (matched by CloudEvent ID substring in the text or
//      caption).
//   7. Append a KindTGReceive transcript event with the matched
//      Telegram message metadata.
//
// §107 bidirectional invariant: this scenario emits one
// KindHeraldPost + one KindHeraldResponse + one KindTGReceive — the
// bidirectional minimum holds.
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
		Name:        "happy-path-single-channel",
		Description: "One CloudEvent → pherald /v1/events (TOON) → Telegram delivery cross-checked",
		Run:         runHappyPath,
	})
}

func runHappyPath(ctx context.Context, o *Orchestrator) error {
	ce := herald.CloudEvent{
		SpecVersion: "1.0",
		ID:          fmt.Sprintf("qaherald-happy-%d", o.now().UnixNano()),
		Source:      "qaherald",
		Type:        "qa.happy-path",
		Time:        o.now().UTC(),
		Data:        json.RawMessage(`{"message":"qaherald says hello"}`),
	}
	cePayload, _ := json.Marshal(ce)

	_ = o.Transcript.Append(transcript.Event{
		Direction: transcript.DirectionOut,
		Kind:      transcript.KindHeraldPost,
		Scenario:  "happy-path-single-channel",
		Payload:   cePayload,
		Note:      "Accept: application/toon",
	})

	receipt, status, headers, err := o.Herald.PostEvent(ctx, ce, herald.AcceptTOON)
	receiptPayload, _ := json.Marshal(receipt)
	_ = o.Transcript.Append(transcript.Event{
		Direction: transcript.DirectionIn,
		Kind:      transcript.KindHeraldResponse,
		Scenario:  "happy-path-single-channel",
		Payload:   receiptPayload,
		Note:      fmt.Sprintf("status=%d content-type=%s", status, headers.Get("Content-Type")),
	})
	if err != nil {
		return fmt.Errorf("PostEvent: %w", err)
	}
	if err := assertStatus(http.StatusAccepted, status, "happy-path PostEvent"); err != nil {
		return err
	}
	if receipt.Recipients < 1 {
		return fmt.Errorf("happy-path: expected recipients>=1, got %d", receipt.Recipients)
	}

	msg, err := o.TG.WaitForMessage(30*time.Second, func(m tele.Message) bool {
		return containsCloudEventID(m.Text, m.Caption, ce.ID)
	})
	if err != nil {
		return fmt.Errorf("WaitForMessage: %w", err)
	}
	msgPayload, _ := json.Marshal(map[string]any{
		"message_id": msg.ID,
		"chat_id":    chatIDOf(msg),
		"text":       msg.Text,
		"caption":    msg.Caption,
	})
	_ = o.Transcript.Append(transcript.Event{
		Direction: transcript.DirectionIn,
		Kind:      transcript.KindTGReceive,
		Scenario:  "happy-path-single-channel",
		Payload:   msgPayload,
		Note:      "delivery from pherald → tgram → chat",
	})
	return nil
}

// chatIDOf safely extracts m.Chat.ID — Telegram populates Chat for
// every delivered message, but the unit-test fake may leave it nil.
// Defending here keeps the JSON marshal deterministic.
func chatIDOf(m tele.Message) int64 {
	if m.Chat == nil {
		return 0
	}
	return m.Chat.ID
}
