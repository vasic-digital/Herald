//go:build integration

// HRD-011 Task 5 — §107 E17 evidence: outbound delivery persistence live
// round-trip against real Postgres + real Telegram Bot API.
//
// Run with:
//
//	HERALD_TGRAM_BOT_TOKEN=... HERALD_TGRAM_CHAT_ID=... \
//	  go test -tags=integration -timeout 5m -count=1 \
//	    -run TestSend_PersistsDeliveryEvidence ./commons_messaging/channels/tgram/...
//
// Requires:
//   - A running Podman or Docker runtime on the host.
//   - HERALD_TGRAM_BOT_TOKEN + HERALD_TGRAM_CHAT_ID env vars (the §11.4.3
//     hardware_not_present skip kicks in otherwise — no fakes per §11.4.27).
//
// Anti-bluff per §107 + §11.4.5:
//
//   - Asserts the persisted channel_message_id EXACTLY equals the receipt's
//     ChannelMsgID. A row that exists but with a fake/Herald-synthetic ID
//     would PASS a "row exists" check but FAIL this equality — that's the
//     load-bearing §107 invariant for this table.
//   - Reads back under the SAME tenant's RLS context the insert happened
//     under, exercising the ode_isolation policy's USING clause.
//   - Reads back exactly the tenant's row (ORDER BY sent_at DESC LIMIT 1)
//     rather than any row, so a leaky RLS policy would produce a wrong-
//     tenant row that the equality check would also catch.

package tgram

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	db "digital.vasic.database/pkg/database"
	"github.com/google/uuid"

	"github.com/vasic-digital/herald/commons"
	infra "github.com/vasic-digital/herald/commons_infra"
	storage "github.com/vasic-digital/herald/commons_storage"
)

func TestSend_PersistsDeliveryEvidence(t *testing.T) {
	token := os.Getenv("HERALD_TGRAM_BOT_TOKEN")
	chatID := os.Getenv("HERALD_TGRAM_CHAT_ID")
	if token == "" || chatID == "" {
		t.Skipf("skip: hardware_not_present — HERALD_TGRAM_BOT_TOKEN or HERALD_TGRAM_CHAT_ID absent per §11.4.3")
	}
	if _, err := exec.LookPath("podman"); err != nil {
		if _, err := exec.LookPath("docker"); err != nil {
			t.Skipf("skip: hardware_not_present — no container runtime (podman/docker) on PATH per §11.4.3")
		}
	}

	// Test-scope env: the quickstart compose declares ${HERALD_DB_PASSWORD}
	// (and friends) as required. Pattern lifted from
	// commons_storage/storage_integration_test.go::TestRLS_TenantIsolation_RoundTrip.
	t.Setenv("HERALD_DB_PASSWORD", "test-postgres-password-DO-NOT-USE-IN-PROD")
	t.Setenv("HERALD_REDIS_PASSWORD", "test-redis-password-DO-NOT-USE-IN-PROD")
	t.Setenv("HERALD_PROJECT_NAME", "Herald-Integration-Test")
	t.Setenv("HERALD_TENANT_ID", "00000000-0000-0000-0000-000000000099")

	if os.Getenv("DOCKER_HOST") == "" {
		if sock := os.Getenv("PODMAN_MAC_SOCK"); sock != "" {
			t.Setenv("DOCKER_HOST", "unix://"+sock)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	boot, err := infra.NewQuickstartBoot(infra.Config{
		Services: []string{"postgres"}, // limit blast radius: only postgres
	})
	if err != nil {
		t.Skipf("skip: compose runtime not available (hardware_not_present): %v", err)
	}

	if err := boot.Up(ctx); err != nil {
		t.Fatalf("boot.Up: %v", err)
	}
	t.Cleanup(func() {
		downCtx, downCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer downCancel()
		if err := boot.Down(downCtx); err != nil {
			t.Logf("boot.Down (cleanup): %v", err)
		}
	})

	pool, err := boot.Pool()
	if err != nil {
		t.Fatalf("Pool() after Up(): %v", err)
	}
	if pool == nil {
		t.Fatal("Pool() returned nil without error — §107 PASS-bluff guard")
	}

	tenant := uuid.New()

	// Seed the tenant — same pattern as the §107 E14 RLS test in
	// commons_storage. Without this, an FK or audit trail (if any are
	// added later) would refuse the insert.
	if _, err := pool.Exec(ctx,
		`INSERT INTO tenants (id, name, environment) VALUES ($1, $2, $3)
		 ON CONFLICT (id) DO NOTHING`,
		tenant, "e17-"+tenant.String()[:8], "quickstart",
	); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}

	a, err := NewWithStorage("tgram://"+token+"/"+chatID, pool)
	if err != nil {
		t.Fatalf("NewWithStorage: %v", err)
	}

	msgText := "Herald E17 persist test " + time.Now().Format(time.RFC3339Nano)
	receipt, err := a.SendForTenant(ctx, tenant, commons.OutboundMessage{
		TenantID: tenant.String(),
		Body:     commons.Body{Plain: msgText},
		To: []commons.Recipient{{
			Channel:       string(commons.ChannelTelegram),
			ChannelUserID: chatID,
		}},
	})
	if err != nil {
		t.Fatalf("SendForTenant: %v", err)
	}
	if receipt.ChannelMsgID == "" {
		t.Fatal("receipt.ChannelMsgID empty — §107 bluff guard: Bot API did not return a chat-side message_id")
	}

	var persistedChannelMsgID string
	var persistedChannelID string
	var persistedEvidence int
	err = storage.WithTenantContext(ctx, pool, tenant, func(tx db.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT channel_id, channel_message_id, evidence
			 FROM outbound_delivery_evidence
			 WHERE tenant_id = $1
			 ORDER BY sent_at DESC
			 LIMIT 1`,
			tenant,
		).Scan(&persistedChannelID, &persistedChannelMsgID, &persistedEvidence)
	})
	if err != nil {
		t.Fatalf("read-back: %v", err)
	}

	// §107 load-bearing invariant: the persisted ID must equal the chat-side
	// ID Telegram assigned. A bluff implementation could insert a row with
	// uuid.New().String() here — that would FAIL this check.
	if persistedChannelMsgID != receipt.ChannelMsgID {
		t.Fatalf("persisted channel_message_id mismatch: got %q want %q (§107 bluff guard)",
			persistedChannelMsgID, receipt.ChannelMsgID)
	}
	if persistedChannelID != string(commons.ChannelTelegram) {
		t.Fatalf("persisted channel_id mismatch: got %q want %q",
			persistedChannelID, string(commons.ChannelTelegram))
	}
	if commons.DeliveryEvidence(persistedEvidence) != commons.DeliveryRouted {
		t.Fatalf("persisted evidence mismatch: got %d want %d (DeliveryRouted)",
			persistedEvidence, commons.DeliveryRouted)
	}
}
