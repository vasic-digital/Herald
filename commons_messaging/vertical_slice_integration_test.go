//go:build integration

// HRD-011 + HRD-012 Task 8 — §107 E19 evidence: the full vertical slice
// Telegram → Claude Code → Telegram round-trip live against real services.
//
// Run with (all required, otherwise SKIPs per §11.4.3 hardware_not_present):
//
//	HERALD_TGRAM_BOT_TOKEN=...           \
//	HERALD_TGRAM_CHAT_ID=...             \
//	HERALD_TGRAM_LIVE_INBOUND=1          \
//	HERALD_CLAUDE_PROJECT_NAME=Herald    \
//	HERALD_CLAUDE_SESSION_UUID=<uuid>    \
//	HERALD_CLAUDE_BIN=claude             \
//	HERALD_CLAUDE_WORKDIR=/Users/milosvasic/Projects/Herald \
//	  go test -tags=integration -timeout 10m -count=1 \
//	    -run TestVerticalSlice_TelegramClaudeRoundTrip \
//	    ./commons_messaging/
//
// The operator MUST hand-send a Telegram message to the configured chat
// within the 150s window; the handler will then invoke Claude Code and
// post the reply back via the same bot.
//
// Anti-bluff per §107 + §11.4.5: three independent positive checks across
// the slice — none can be faked individually without faking all three:
//
//   - inboundCount > 0   — proves getUpdates actually pulled a real
//                          operator-hand-sent update.
//   - dispatchOK         — proves `claude --resume` ran AND returned a
//                          parseable §33.3 envelope with non-empty Outcome.
//   - outboundOK         — proves the reply was actually accepted by the
//                          Telegram Bot API (non-empty ChannelMsgID).
//
// A bluff handler that just returns nil would FAIL all three.

package messaging_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/vasic-digital/herald/commons"
	infra "github.com/vasic-digital/herald/commons_infra"
	"github.com/vasic-digital/herald/commons_messaging/channels/tgram"
	"github.com/vasic-digital/herald/commons_messaging/dispatch/claude_code"
)

// handlerFunc adapts a function to commons.InboundHandler — same pattern
// as the Task 4 subscribe_integration_test in commons_messaging/channels/tgram.
type handlerFunc func(context.Context, commons.InboundEvent) error

func (h handlerFunc) Handle(ctx context.Context, ev commons.InboundEvent) error {
	return h(ctx, ev)
}

// TestVerticalSlice_TelegramClaudeRoundTrip is the §107 E19 evidence.
//
// Vertical slice: operator hand-sends a Telegram message → tgram.Subscribe
// receives it → handler invokes claude_code.Dispatch → handler parses the
// reply → handler calls tgram.SendForTenant to send the reply back to the
// same chat.
func TestVerticalSlice_TelegramClaudeRoundTrip(t *testing.T) {
	tgToken := os.Getenv("HERALD_TGRAM_BOT_TOKEN")
	tgChat := os.Getenv("HERALD_TGRAM_CHAT_ID")
	if tgToken == "" || tgChat == "" {
		t.Skipf("skip: hardware_not_present — HERALD_TGRAM_BOT_TOKEN or HERALD_TGRAM_CHAT_ID absent per §11.4.3")
	}
	if os.Getenv("HERALD_TGRAM_LIVE_INBOUND") != "1" {
		t.Skipf("skip: hardware_not_present — HERALD_TGRAM_LIVE_INBOUND=1 not set; operator hand-sent inbound required per §11.4.3")
	}
	claudeBin := os.Getenv("HERALD_CLAUDE_BIN")
	if claudeBin == "" {
		claudeBin = "claude"
	}
	if _, err := exec.LookPath(claudeBin); err != nil {
		t.Skipf("skip: hardware_not_present — %s not on PATH per §11.4.3", claudeBin)
	}
	projectName := os.Getenv("HERALD_CLAUDE_PROJECT_NAME")
	sessionUUIDStr := os.Getenv("HERALD_CLAUDE_SESSION_UUID")
	if projectName == "" || sessionUUIDStr == "" {
		t.Skipf("skip: hardware_not_present — HERALD_CLAUDE_PROJECT_NAME or HERALD_CLAUDE_SESSION_UUID absent per §11.4.3")
	}
	sessionUUID, err := uuid.Parse(sessionUUIDStr)
	if err != nil {
		t.Fatalf("HERALD_CLAUDE_SESSION_UUID %q is not a valid UUID: %v", sessionUUIDStr, err)
	}
	if _, err := exec.LookPath("podman"); err != nil {
		if _, err := exec.LookPath("docker"); err != nil {
			t.Skipf("skip: hardware_not_present — no container runtime (podman/docker) on PATH per §11.4.3")
		}
	}

	// Test-scope env: the quickstart compose declares ${HERALD_DB_PASSWORD}
	// (and friends) as required. Pattern lifted from the persist tests.
	t.Setenv("HERALD_DB_PASSWORD", "test-postgres-password-DO-NOT-USE-IN-PROD")
	t.Setenv("HERALD_REDIS_PASSWORD", "test-redis-password-DO-NOT-USE-IN-PROD")
	t.Setenv("HERALD_PROJECT_NAME", "Herald-Integration-Test")
	t.Setenv("HERALD_TENANT_ID", "00000000-0000-0000-0000-000000000099")

	if os.Getenv("DOCKER_HOST") == "" {
		if sock := os.Getenv("PODMAN_MAC_SOCK"); sock != "" {
			t.Setenv("DOCKER_HOST", "unix://"+sock)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
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

	// Seed a tenant for the outbound persistence write — same pattern as the
	// T5 outbound_delivery_evidence persist test.
	tenant := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO tenants (id, name, environment) VALUES ($1, $2, $3)
		 ON CONFLICT (id) DO NOTHING`,
		tenant, "e19-"+tenant.String()[:8], "quickstart",
	); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}

	// Telegram adapter with persistence.
	tgAdapter, err := tgram.NewWithStorage("tgram://"+tgToken+"/"+tgChat, pool)
	if err != nil {
		t.Fatalf("tgram.NewWithStorage: %v", err)
	}

	// Claude Code dispatcher with persistence.
	workdir := os.Getenv("HERALD_CLAUDE_WORKDIR")
	if workdir == "" {
		workdir = "."
	}
	dispatcher, err := claude_code.NewWithStorage(claudeBin, workdir, projectName, pool)
	if err != nil {
		t.Fatalf("claude_code.NewWithStorage: %v", err)
	}

	// Bootstrap the anchor file from the operator-supplied UUID — same pattern
	// as the T7 claude_code persist test. Without this, ResolveSession returns
	// uuid.Nil and Dispatch fails with "no anchored session".
	_, anchor, _ := dispatcher.ResolveSession()
	if err := dispatcher.PersistSession(sessionUUID, anchor); err != nil {
		t.Fatalf("PersistSession (anchor bootstrap): %v", err)
	}
	t.Cleanup(func() {
		// Best-effort: remove the anchor file and any empty parents we created.
		_ = os.Remove(anchor)
		for dir := filepath.Dir(anchor); strings.HasPrefix(dir, workdir) && dir != workdir; dir = filepath.Dir(dir) {
			if err := os.Remove(dir); err != nil {
				break
			}
		}
	})

	// Three independent positive checks across the slice (§107 bluff guard).
	var inboundCount atomic.Int64
	var dispatchOK atomic.Bool
	var outboundOK atomic.Bool
	var slipperyErr atomic.Value // stores the last error from the handler

	handler := handlerFunc(func(ctx context.Context, ev commons.InboundEvent) error {
		inboundCount.Add(1)

		req := claude_code.DispatchRequest{
			UserMessage: "Herald E19 vertical-slice test " + time.Now().Format(time.RFC3339Nano) +
				" — operator said: " + ev.Body.Plain +
				". Please reply with outcome=answered and a short summary acknowledging receipt.",
		}
		resp, err := dispatcher.Dispatch(ctx, req)
		if err != nil {
			slipperyErr.Store(err)
			return err
		}
		if resp.Outcome == "" {
			err := errors.New("dispatch returned empty Outcome — §107 bluff guard")
			slipperyErr.Store(err)
			return err
		}
		dispatchOK.Store(true)

		// Send the reply back to the same chat via SendForTenant. The
		// persistence write into outbound_delivery_evidence is the third
		// load-bearing artifact.
		outbound := commons.OutboundMessage{
			TenantID: tenant.String(),
			Body: commons.Body{
				Plain: "Herald reply (outcome=" + resp.Outcome + "): " + resp.Summary,
			},
			To: []commons.Recipient{{
				Channel:       string(commons.ChannelTelegram),
				ChannelUserID: tgChat,
			}},
		}
		receipt, err := tgAdapter.SendForTenant(ctx, tenant, outbound)
		if err != nil {
			slipperyErr.Store(err)
			return err
		}
		if receipt.ChannelMsgID == "" {
			err := errors.New("outbound Receipt.ChannelMsgID empty — §107 bluff guard")
			slipperyErr.Store(err)
			return err
		}
		outboundOK.Store(true)
		return nil
	})

	// Subscribe blocks until ctx is cancelled — run it in a goroutine.
	subCtx, subCancel := context.WithCancel(ctx)
	defer subCancel()
	subErr := make(chan error, 1)
	go func() {
		subErr <- tgAdapter.Subscribe(subCtx, handler)
	}()

	// Poll for completion: all three invariants must flip true.
	deadline := time.Now().Add(150 * time.Second)
	for time.Now().Before(deadline) {
		if inboundCount.Load() > 0 && dispatchOK.Load() && outboundOK.Load() {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	subCancel()
	select {
	case err := <-subErr:
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			t.Logf("Subscribe returned: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Log("Subscribe did not exit within 10s of cancel — likely a poller stall (observational)")
	}

	// §107 invariants — three independent positive checks. A bluff would
	// have to fake all three.
	if inboundCount.Load() == 0 {
		t.Fatal("VS: handler never invoked — operator did not hand-send a message within the 150s window? §107 bluff guard")
	}
	if !dispatchOK.Load() {
		if v := slipperyErr.Load(); v != nil {
			t.Fatalf("VS: dispatch failed — %v", v)
		}
		t.Fatal("VS: dispatch did not complete — §107 guard")
	}
	if !outboundOK.Load() {
		if v := slipperyErr.Load(); v != nil {
			t.Fatalf("VS: outbound send failed — %v", v)
		}
		t.Fatal("VS: outbound send did not complete — §107 guard")
	}

	t.Logf("E19 PASS: tenant=%s; inbound_handler_invocations=%d; dispatch ok (outcome non-empty); outbound ok (ChannelMsgID non-empty); chat=%q",
		tenant, inboundCount.Load(), tgChat)
}
