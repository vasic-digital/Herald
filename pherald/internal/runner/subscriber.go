package runner

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/vasic-digital/herald/commons"
)

// SubscriberResolver is Stage 5 — reads subscribers + aliases under
// the resolved tenant and emits commons.Recipient entries.
//
// Wave 3b implementation: ALL subscribers in the tenant receive ALL
// events. Per-event preference filtering (CategoryPref / WorkflowPref /
// QuietHours per spec §7.2-§7.3) is a follow-up HRD; for now this
// stage's contract is "tenant-scoped fan-out to every registered
// recipient" — adequate for the consumer-project integration use case
// where one tenant maps to one Telegram chat.
//
// Per §107: returning zero recipients on an empty tenant is NOT an
// error — empty fan-out is a valid (and common) outcome. The
// OutcomeRecorder still writes an events_processed row so the event
// is deduped on replay.
type SubscriberResolver struct {
	Subscribers subscribersStore
}

// subscribersStore is the subset of PG access this stage uses. The
// real PG adapter (T9, commons_storage) and the in-memory test fake
// (fakeSubscribersStore in fakes_test.go) both satisfy it.
type subscribersStore interface {
	ListByTenant(ctx context.Context) ([]subscriberRow, error)
}

// subscriberRow mirrors the PG `subscribers` table row with its
// per-row `subscriber_aliases` rows joined in. Lives in production
// code (not the test file) so the T9 PG adapter
// (pgSubscribersAdapter.ListByTenant) can return the same type —
// mirrors the eventsProcessedRow precedent in idempotency.go.
type subscriberRow struct {
	ID          uuid.UUID
	TenantID    uuid.UUID
	Handle      string
	DisplayName string
	Aliases     []subscriberAliasRow
}

// subscriberAliasRow mirrors the PG `subscriber_aliases` table row
// — the (channel, channel_user_id) pair that maps a subscriber to a
// concrete delivery endpoint (Telegram chat ID, Slack user ID, …).
type subscriberAliasRow struct {
	Channel       string
	ChannelUserID string
}

func (r *SubscriberResolver) Process(ctx context.Context, rc *RunCtx) error {
	if rc.TenantPGCtx == nil {
		return fmt.Errorf("subscriber_resolver: TenantPGCtx not set by stage 3")
	}
	rows, err := r.Subscribers.ListByTenant(rc.TenantPGCtx)
	if err != nil {
		return fmt.Errorf("subscriber_resolver: list: %w", err)
	}
	var recips []commons.Recipient
	for _, row := range rows {
		for _, alias := range row.Aliases {
			recips = append(recips, commons.Recipient{
				Channel:       alias.Channel,
				ChannelUserID: alias.ChannelUserID,
				DisplayName:   row.DisplayName,
			})
		}
	}
	rc.Recipients = recips
	return nil
}
