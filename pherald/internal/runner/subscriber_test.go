package runner

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestSubscriberResolver_EmptyTenant_ZeroRecipients(t *testing.T) {
	tenantID := mustParse("33333333-3333-3333-3333-333333333333")
	store := newFakeSubscribersStore() // no subs added
	r := &SubscriberResolver{Subscribers: store}

	rc := &RunCtx{TenantID: tenantID}
	rc.TenantPGCtx = withTenantCtx(context.Background(), tenantID)
	if err := r.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(rc.Recipients) != 0 {
		t.Errorf("Recipients = %d, want 0 on empty tenant", len(rc.Recipients))
	}
}

func TestSubscriberResolver_TwoSubscribers_TgramAliases(t *testing.T) {
	tenantID := mustParse("33333333-3333-3333-3333-333333333333")
	store := newFakeSubscribersStore()
	store.Add(tenantID, subscriberRow{
		ID: uuid.New(), Handle: "alice", DisplayName: "Alice",
		Aliases: []subscriberAliasRow{{Channel: "tgram", ChannelUserID: "100"}},
	})
	store.Add(tenantID, subscriberRow{
		ID: uuid.New(), Handle: "bob", DisplayName: "Bob",
		Aliases: []subscriberAliasRow{{Channel: "tgram", ChannelUserID: "200"}},
	})

	r := &SubscriberResolver{Subscribers: store}
	rc := &RunCtx{TenantID: tenantID}
	rc.TenantPGCtx = withTenantCtx(context.Background(), tenantID)
	if err := r.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(rc.Recipients) != 2 {
		t.Fatalf("Recipients = %d, want 2", len(rc.Recipients))
	}
	chats := map[string]bool{}
	for _, rcpt := range rc.Recipients {
		chats[rcpt.ChannelUserID] = true
	}
	if !chats["100"] || !chats["200"] {
		t.Errorf("Recipients missing expected chats: %v", chats)
	}
}

func TestSubscriberResolver_TenantIsolation(t *testing.T) {
	tidA := mustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	tidB := mustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	store := newFakeSubscribersStore()
	store.Add(tidA, subscriberRow{ID: uuid.New(), Handle: "a1",
		Aliases: []subscriberAliasRow{{Channel: "tgram", ChannelUserID: "111"}}})
	store.Add(tidB, subscriberRow{ID: uuid.New(), Handle: "b1",
		Aliases: []subscriberAliasRow{{Channel: "tgram", ChannelUserID: "222"}}})

	r := &SubscriberResolver{Subscribers: store}
	// Resolve as tenant A — should see only their sub.
	rc := &RunCtx{TenantID: tidA}
	rc.TenantPGCtx = withTenantCtx(context.Background(), tidA)
	if err := r.Process(context.Background(), rc); err != nil {
		t.Fatal(err)
	}
	if len(rc.Recipients) != 1 || rc.Recipients[0].ChannelUserID != "111" {
		t.Errorf("Tenant isolation broken: got %v", rc.Recipients)
	}
}

// withTenantCtx mirrors TenantResolver.Process to make sub-tests
// independent of stage 3.
func withTenantCtx(ctx context.Context, tid uuid.UUID) context.Context {
	return context.WithValue(ctx, tenantCtxKey{}, tid)
}
