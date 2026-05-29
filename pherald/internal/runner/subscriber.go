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
// HRD-154 (ATMOSphere integration WS-3): Stage 5 now enforces
// per-subscriber preference + quiet-hours routing via
// filterByPreferences (prefs.go). A subscriber with NO PreferenceSet
// preserves the pre-HRD-154 "resolve all aliases" behaviour so existing
// single-tenant→single-chat callers do not regress; a subscriber WITH a
// PreferenceSet is filtered by mute > quiet-hours > workflow opt-in >
// category opt-in (see prefs.go for the full precedence contract).
//
// Quiet-hours evaluation is timezone-aware and clock-injected: the
// Clock field (defaulting to commons.RealClock) supplies "now" so tests
// inject a deterministic instant. Stage 5 never calls time.Now().
//
// Per §107: returning zero recipients on an empty tenant is NOT an
// error — empty fan-out is a valid (and common) outcome. The
// OutcomeRecorder still writes an events_processed row so the event
// is deduped on replay.
type SubscriberResolver struct {
	Subscribers subscribersStore
	// Clock supplies the current instant for quiet-hours evaluation.
	// Nil ⇒ commons.RealClock{} (production default).
	Clock commons.Clock
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
	// Timezone is the subscriber's IANA TZ (e.g. "Europe/Belgrade"),
	// used by HRD-154 quiet-hours evaluation when QuietHours.TZ is empty.
	Timezone string
	// Preferences is the per-subscriber PreferenceSet (HRD-154). Nil ⇒
	// the subscriber has no configured preferences ⇒ resolve all aliases
	// (pre-HRD-154 behaviour preserved). The T9 PG adapter decodes this
	// from subscribers.metadata.preferences (spec §7.2 / §11.0).
	Preferences *commons.PreferenceSet
	Aliases     []subscriberAliasRow
}

// toSubscriber projects a PG-shaped subscriberRow into the canonical
// commons.Subscriber that filterByPreferences (prefs.go) operates on.
func (row subscriberRow) toSubscriber() commons.Subscriber {
	aliases := make([]commons.SubscriberAlias, 0, len(row.Aliases))
	for _, a := range row.Aliases {
		aliases = append(aliases, commons.SubscriberAlias{
			Channel:       a.Channel,
			ChannelUserID: a.ChannelUserID,
		})
	}
	return commons.Subscriber{
		ID:          row.ID,
		TenantID:    row.TenantID,
		Handle:      row.Handle,
		DisplayName: row.DisplayName,
		Timezone:    row.Timezone,
		Preferences: row.Preferences,
		Aliases:     aliases,
	}
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
	subs := make([]commons.Subscriber, 0, len(rows))
	for _, row := range rows {
		subs = append(subs, row.toSubscriber())
	}
	clk := r.Clock
	if clk == nil {
		clk = commons.RealClock{}
	}
	rc.Recipients = filterByPreferences(subs, rc.Event, clk.Now())
	return nil
}
