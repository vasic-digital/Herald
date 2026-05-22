package runner

import (
	"context"
	"log/slog"

	"github.com/vasic-digital/herald/commons"
)

// ChannelDispatcher is Stage 6 — looks up a commons.Channel by the
// recipient's channel ID and calls .Send for each recipient. Wave 3b
// fan-out is sequential (one recipient at a time) — parallel fan-out
// within a single event is a Wave 4+ optimization.
//
// Per §107: a recipient whose channel isn't registered MUST produce
// an Evidence=Unknown result with an explanatory Error field —
// silently dropping unroutable recipients would be a §11.4 bluff.
// Channel.Send errors are similarly captured as Evidence=Unknown +
// Error so the OutcomeRecorder (Stage 7) can persist them for the
// audit trail.
type ChannelDispatcher struct {
	Channels map[commons.ChannelID]commons.Channel
	Logger   *slog.Logger
}

func (d *ChannelDispatcher) Process(ctx context.Context, rc *RunCtx) error {
	for _, rcpt := range rc.Recipients {
		channel, ok := d.Channels[commons.ChannelID(rcpt.Channel)]
		if !ok {
			rc.Receipts = append(rc.Receipts, ChannelDispatchResult{
				ChannelID:     rcpt.Channel,
				ChannelUserID: rcpt.ChannelUserID,
				Evidence:      commons.DeliveryUnknown,
				Error:         "channel '" + rcpt.Channel + "' not registered in Runner.Deps.Channels",
			})
			continue
		}
		msg := commons.OutboundMessage{
			EventID:        rc.Event.ID,
			IdempotencyKey: rc.IdemKey,
			TenantID:       rc.TenantID.String(),
			To:             []commons.Recipient{rcpt},
			Body: commons.Body{
				Plain: string(rc.Event.Data),
			},
			Trace: rc.Trace,
		}
		receipt, err := channel.Send(ctx, msg)
		if err != nil {
			rc.Receipts = append(rc.Receipts, ChannelDispatchResult{
				ChannelID:     rcpt.Channel,
				ChannelUserID: rcpt.ChannelUserID,
				Evidence:      commons.DeliveryUnknown,
				Error:         err.Error(),
			})
			continue
		}
		rc.Receipts = append(rc.Receipts, ChannelDispatchResult{
			ChannelID:     rcpt.Channel,
			ChannelUserID: rcpt.ChannelUserID,
			Evidence:      receipt.Evidence,
			ChannelMsgID:  receipt.ChannelMsgID,
		})
	}
	return nil
}
