// pherald listen — Wave 6 T6 (2026-05-22).
//
// Long-running subcommand that wires tgram.Subscribe (getUpdates long-poll
// per spec §32.2) to the production inbound.Dispatcher (T7). On every
// Telegram message the bot self-filter (T4) drops self-echoes, attachments
// are content-addressed under ~/.herald/inbox/<sha>.<ext> (T5), the
// dispatcher invokes claude_code with the operator-mandated pre-text (T3)
// + the Opus pin (T2), parses <<<HERALD-REPLY>>>, and routes the action
// (reply | issue.open | event.emit).
//
// §107 posture (anti-bluff): the `listen` command's PASS is NOT "the
// process boots cleanly + exits cleanly" — that's a §11.4 PASS-bluff. The
// listen_test integration test asserts the handler is wired (≥1
// dispatcher.Handle invocation arrives via the stub subscriber within 8s)
// AND that SIGTERM / context cancel actually returns runListen cleanly.
// The runListen helper is the seam: production RunE constructs real
// dependencies and calls runListen; the test constructs stub dependencies
// and calls runListen directly (no binary spawn — fast + deterministic).
//
// Env-only configuration (no flags wired Wave 6; flag-or-env can land in
// HRD-NNN-W6c when an operator requests it):
//   HERALD_TGRAM_BOT_TOKEN — required: Telegram bot token (validated by
//                            tgram.Subscribe via telebot.NewBot getMe).
//   HERALD_TGRAM_CHAT_ID   — required: numeric chat ID this binary serves
//                            (tgram:// URL component; Adapter routing
//                            ignores it for inbound, but the URL parser
//                            requires it).
//   HERALD_PROJECT_NAME    — optional: pinned via commons.ProjectName().
//   HERALD_CLAUDE_BIN      — optional: path to `claude` binary (default
//                            "claude" via $PATH per claude_code.New).
//   HERALD_INBOUND_CC_FAKE — listen_test seam: if "1", the runListen
//                            helper substitutes a no-op CodeDispatcher
//                            that returns a canned reply, so the test
//                            never spawns the real claude CLI. Production
//                            callers do NOT set this.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons_messaging/channels/tgram"
	"github.com/vasic-digital/herald/commons_messaging/dispatch/claude_code"
	"github.com/vasic-digital/herald/pherald/internal/inbound"
)

// newListenCmd wires the `pherald listen` Cobra subcommand. Wave 6 keeps
// the surface env-only to match the operator-locked config story (single
// .env file controls everything); a follow-up HRD can lift values to
// --bot-token / --chat-id / --project-name flags if the workflow demands.
func newListenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "listen",
		Short: "Run the inbound runtime: Telegram getUpdates long-poll + Claude Code dispatch loop",
		Long: `Long-running. Wires tgram.Subscribe (getUpdates long-poll) to the
production inbound.Dispatcher (Wave 6 T7). Every inbound message is
dispatched through Claude Code (Opus, pinned) per the §32 inbound pipeline.

Required environment:
  HERALD_TGRAM_BOT_TOKEN   Telegram bot token (validated via getMe on boot).
  HERALD_TGRAM_CHAT_ID     Numeric chat ID; included in the tgram:// URL.

Optional environment:
  HERALD_PROJECT_NAME      Claude Code session name (else basename(cwd) else "Herald").
  HERALD_CLAUDE_BIN        Path to ` + "`claude`" + ` CLI (default: $PATH lookup).

Signal handling: SIGINT/SIGTERM cancels the long-poll cleanly via
signal.NotifyContext.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadListenConfigFromEnv()
			if err != nil {
				return err
			}
			ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()
			fmt.Fprintln(cmd.OutOrStdout(), "pherald listen: starting Telegram getUpdates long-poll loop")
			return runListen(ctx, cfg)
		},
	}
}

// listenConfig carries the resolved dependencies for runListen. The
// Subscriber + Code fields are the test seams: production code populates
// them with the live tgram.Adapter.Subscribe + claude_code.Dispatcher
// (wrapped by inbound.NewCCAdapter); listen_test populates them with
// stubs so the dispatch loop can be driven hermetically.
type listenConfig struct {
	ProjectName string
	BotToken    string
	ChatID      string
	ClaudeBin   string
	// Subscriber is the long-poll entry point. Production: a closure over
	// tgram.Adapter.Subscribe. Test: a closure that publishes one synthetic
	// InboundEvent and waits for ctx.Done.
	Subscriber func(ctx context.Context, h commons.InboundHandler) error
	// Code is the CC dispatcher. Production: inbound.NewCCAdapter(*claude_code.Dispatcher).
	// Test: a stubCode returning a canned <<<HERALD-REPLY>>> blob.
	Code inbound.CodeDispatcher
	// Replier is the tgram SendReply binding. Production: the live
	// *tgram.Adapter. Test: a recordingReplier.
	Replier inbound.TgramReplier
}

// loadListenConfigFromEnv resolves env into listenConfig + constructs the
// production tgram + claude_code dependencies. Returns a fully wired
// listenConfig on success. Returns a descriptive error if any required
// env var is missing — never a "boots silently with anonymous defaults"
// path (anonymous serve plane is a §107 PASS-bluff per main.go's
// buildVerifier doc-comment).
func loadListenConfigFromEnv() (listenConfig, error) {
	token := os.Getenv("HERALD_TGRAM_BOT_TOKEN")
	if token == "" {
		return listenConfig{}, fmt.Errorf("pherald listen: HERALD_TGRAM_BOT_TOKEN required")
	}
	chatID := os.Getenv("HERALD_TGRAM_CHAT_ID")
	if chatID == "" {
		return listenConfig{}, fmt.Errorf("pherald listen: HERALD_TGRAM_CHAT_ID required")
	}
	projectName := commons.ProjectName()
	claudeBin := os.Getenv("HERALD_CLAUDE_BIN")

	// Build the production tgram adapter. The Wave 6 tgram package
	// constructor takes a tgram://<token>/<chat_id> URL (no NewAdapter
	// helper yet; see commons_messaging/channels/tgram/tgram.go:New).
	tgramAdapter, err := tgram.New("tgram://" + token + "/" + chatID)
	if err != nil {
		return listenConfig{}, fmt.Errorf("pherald listen: build tgram adapter: %w", err)
	}

	// Build the production Claude Code dispatcher.
	ccDispatcher, err := claude_code.New(claudeBin, "", projectName)
	if err != nil {
		return listenConfig{}, fmt.Errorf("pherald listen: build claude_code dispatcher: %w", err)
	}

	cfg := listenConfig{
		ProjectName: projectName,
		BotToken:    token,
		ChatID:      chatID,
		ClaudeBin:   claudeBin,
		Subscriber:  tgramAdapter.Subscribe,
		Code:        inbound.NewCCAdapter(ccDispatcher),
		Replier:     tgramAdapter,
	}

	// HERALD_INBOUND_CC_FAKE is the listen_test seam — see package doc.
	// Production callers do NOT set it; the env-read here is the entire
	// kill-switch surface.
	if os.Getenv("HERALD_INBOUND_CC_FAKE") == "1" {
		cfg.Code = fakeCodeDispatcher{}
	}

	return cfg, nil
}

// runListen wires the inbound.Dispatcher to the configured Subscriber and
// blocks until ctx is cancelled. Extracted from RunE so listen_test can
// drive it hermetically without binary spawn.
//
// §107 anchor: the loop is NOT "boots + exits" — runListen returns the
// error from cfg.Subscriber (which is itself the long-poll loop that
// invokes Handle on every message). Test asserts Handle was invoked and
// the returned err is one of (nil, context.Canceled, context.DeadlineExceeded).
func runListen(ctx context.Context, cfg listenConfig) error {
	dispatcher, err := inbound.NewDispatcher(inbound.Config{
		ProjectName: cfg.ProjectName,
		Code:        cfg.Code,
		TgramReply:  cfg.Replier,
	})
	if err != nil {
		return fmt.Errorf("pherald listen: build inbound dispatcher: %w", err)
	}
	if err := cfg.Subscriber(ctx, dispatcher); err != nil {
		// Subscriber returns ctx.Err() on clean cancellation; bubble
		// that up so the caller (signal.NotifyContext) can exit 0.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil
		}
		return fmt.Errorf("pherald listen: subscribe: %w", err)
	}
	return nil
}

// fakeCodeDispatcher is the HERALD_INBOUND_CC_FAKE=1 seam. Returns a
// canned <<<HERALD-REPLY>>> blob so the inbound.Dispatcher routes to the
// "reply" action without spawning a real claude binary. Production code
// paths never instantiate this type — the only construction site is
// loadListenConfigFromEnv when the env opt-in is set.
type fakeCodeDispatcher struct{}

func (fakeCodeDispatcher) Dispatch(_ context.Context, _ inbound.CodeRequest) (inbound.CodeResponse, error) {
	return inbound.CodeResponse{
		Stdout: []byte(`<<<HERALD-REPLY>>> {"action":"reply","text":"ack (fake)"}`),
	}, nil
}
