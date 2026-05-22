// Wave 5 Task 5 Scenario #2 — fan-out-multi-subscriber.
//
// Goal: one CloudEvent → ≥2 subscribers each receive the delivery.
// Pre-condition (operator setup, T8): pherald has ≥2 active
// subscribers configured. This scenario asserts a fan-out count by
// reading receipt.Recipients then collecting one KindTGReceive event
// per recipient.
//
// §107 bidirectional invariant: emits ≥1 KindHeraldPost +
// receipt.Recipients KindTGReceive events.
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
		Name:        "fan-out-multi-subscriber",
		Description: "One CloudEvent → ≥2 subscribers each receive the message",
		Run:         runFanout,
	})
}

func runFanout(ctx context.Context, o *Orchestrator) error {
	ce := herald.CloudEvent{
		SpecVersion: "1.0",
		ID:          fmt.Sprintf("qaherald-fanout-%d", o.now().UnixNano()),
		Source:      "qaherald",
		Type:        "qa.fanout",
		Time:        o.now().UTC(),
		Data:        json.RawMessage(`{"message":"qaherald fan-out broadcast"}`),
	}
	cePayload, _ := json.Marshal(ce)

	_ = o.Transcript.Append(transcript.Event{
		Direction: transcript.DirectionOut,
		Kind:      transcript.KindHeraldPost,
		Scenario:  "fan-out-multi-subscriber",
		Payload:   cePayload,
		Note:      "Accept: application/toon",
	})

	receipt, status, headers, err := o.Herald.PostEvent(ctx, ce, herald.AcceptTOON)
	receiptPayload, _ := json.Marshal(receipt)
	_ = o.Transcript.Append(transcript.Event{
		Direction: transcript.DirectionIn,
		Kind:      transcript.KindHeraldResponse,
		Scenario:  "fan-out-multi-subscriber",
		Payload:   receiptPayload,
		Note:      fmt.Sprintf("status=%d content-type=%s recipients=%d", status, headers.Get("Content-Type"), receipt.Recipients),
	})
	if err != nil {
		return fmt.Errorf("PostEvent: %w", err)
	}
	if err := assertStatus(http.StatusAccepted, status, "fan-out PostEvent"); err != nil {
		return err
	}
	if receipt.Recipients < 2 {
		return fmt.Errorf("fan-out: expected recipients>=2, got %d (operator must configure ≥2 subscribers — see T8)", receipt.Recipients)
	}

	// Collect one delivery per recipient. WaitForMessage with a
	// once-per-call seen-set keeps the test honest: a stub that
	// re-emits the same Telegram message would deadlock instead of
	// silently passing.
	seen := map[int]bool{}
	for i := 0; i < receipt.Recipients; i++ {
		msg, err := o.TG.WaitForMessage(30*time.Second, func(m tele.Message) bool {
			if seen[m.ID] {
				return false
			}
			return containsCloudEventID(m.Text, m.Caption, ce.ID)
		})
		if err != nil {
			return fmt.Errorf("WaitForMessage (recipient %d/%d): %w", i+1, receipt.Recipients, err)
		}
		seen[msg.ID] = true
		msgPayload, _ := json.Marshal(map[string]any{
			"message_id":      msg.ID,
			"chat_id":         chatIDOf(msg),
			"text":            msg.Text,
			"recipient_index": i + 1,
		})
		_ = o.Transcript.Append(transcript.Event{
			Direction: transcript.DirectionIn,
			Kind:      transcript.KindTGReceive,
			Scenario:  "fan-out-multi-subscriber",
			Payload:   msgPayload,
			Note:      fmt.Sprintf("recipient %d/%d", i+1, receipt.Recipients),
		})
	}
	return nil
}
