package tgram

import (
	"context"
	"fmt"

	db "digital.vasic.database/pkg/database"
	"github.com/google/uuid"

	"github.com/vasic-digital/herald/commons"
	storage "github.com/vasic-digital/herald/commons_storage"
)

// SendForTenant calls Send and persists the delivery-evidence row under
// the given tenant's RLS context. Returns the same Receipt as Send, plus
// an error if EITHER the network send OR the persistence write fails.
//
// Persistence is skipped (no error) when the Adapter was constructed via
// New (a.pool == nil). Use NewWithStorage to opt into persistence.
//
// §107 bluff guard: a persisted row whose channel_message_id doesn't
// reflect the actual chat-side message_id Telegram assigned would be a
// bluff. The integration test asserts exact equality between the persisted
// column and receipt.ChannelMsgID (which Send populates from the Bot API
// sendMessage response — sent.ID, not a Herald UUID).
//
// channel_id column carries the canonical Channel constant
// (commons.ChannelTelegram → "tgram"). channel_message_id carries the
// integer Bot API message_id formatted as decimal text — preserving the
// integer value while staying type-uniform with future channels (Slack ts
// "1701123456.789012", SMTP "<id@host>" queue IDs, ...).
func (a *Adapter) SendForTenant(ctx context.Context, tenantID uuid.UUID, msg commons.OutboundMessage) (commons.Receipt, error) {
	receipt, err := a.Send(ctx, msg)
	if err != nil {
		return receipt, err
	}
	if a.pool == nil {
		return receipt, nil
	}
	if persistErr := storage.WithTenantContext(ctx, a.pool, tenantID, func(tx db.Tx) error {
		_, execErr := tx.Exec(ctx,
			`INSERT INTO outbound_delivery_evidence
			    (id, tenant_id, channel_id, channel_message_id, evidence)
			 VALUES ($1, $2, $3, $4, $5)`,
			uuid.New(),
			tenantID,
			string(commons.ChannelTelegram),
			receipt.ChannelMsgID,
			int(receipt.Evidence),
		)
		return execErr
	}); persistErr != nil {
		return receipt, fmt.Errorf("tgram.SendForTenant: persist evidence: %w", persistErr)
	}
	return receipt, nil
}
