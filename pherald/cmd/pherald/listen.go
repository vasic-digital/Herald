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
//   HERALD_OPERATOR_IDS    — Wave 6.5 T7: comma-separated channel_user_id
//                            allowlist for the §32.6 Done:/Reopen:
//                            commands. Empty / unset → both commands are
//                            rejected with ErrNotOperator. Whitespace
//                            tolerant; empty segments skipped. Stub for
//                            V3 §32.10 subscriber-role mapping (Wave 7).
//
// Flags:
//   --docs-dir <path>      — Wave 6.5 T7: docs root containing Issues.md,
//                            Fixed.md, Status.md, CONTINUATION.md, Help.md.
//                            Default "docs" (relative to cwd). docs/Issues.md
//                            MUST exist; the command refuses to boot
//                            otherwise (§107 fail-loud — silent degradation
//                            to "everything goes to CC" would mask a real
//                            misconfiguration).
//   --qa-out-dir <path>    — Wave 6 T10a: JSONL journal directory.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
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
//
// Wave 6 T10a (2026-05-22) adds ONE flag — `--qa-out-dir <path>` — which,
// when set, journals every bidirectional inbound/CC/outbound event into
// `<path>/transcript.jsonl` per the docs/qa/ §107.x evidence mandate.
// Attachments are copied (content-addressed) under `<path>/attachments/`.
func newListenCmd() *cobra.Command {
	cmd := &cobra.Command{
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

Optional flags:
  --qa-out-dir <path>      Journal every inbound/CC/outbound event to
                           <path>/transcript.jsonl and copy attachments to
                           <path>/attachments/<sha256>.<ext>. Wave 6 T10a
                           §107.x evidence primitive — feature ships with
                           the resulting run dir under docs/qa/<run-id>/.
  --docs-dir <path>        Docs root containing Issues.md / Fixed.md /
                           Status.md / CONTINUATION.md / Help.md. Default
                           "docs" (relative to cwd). Issues.md MUST exist;
                           startup fails otherwise (Wave 6.5 T7 §107
                           fail-loud).

Signal handling: SIGINT/SIGTERM cancels the long-poll cleanly via
signal.NotifyContext.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadListenConfigFromEnv()
			if err != nil {
				return err
			}
			cfg.QAOutDir, _ = cmd.Flags().GetString("qa-out-dir")
			cfg.DocsDir, _ = cmd.Flags().GetString("docs-dir")
			if cfg.DocsDir == "" {
				cfg.DocsDir = "docs"
			}
			cfg.OperatorIDs = parseOperatorIDs(os.Getenv("HERALD_OPERATOR_IDS"))
			if err := wireDocsAndCommands(&cfg); err != nil {
				return err
			}
			ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()
			fmt.Fprintln(cmd.OutOrStdout(), "pherald listen: starting Telegram getUpdates long-poll loop")
			if cfg.QAOutDir != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "pherald listen: QA journaling enabled — %s\n", cfg.QAOutDir)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "pherald listen: docs dir %s; %d operator(s) authorised for Done:/Reopen:\n",
				cfg.DocsDir, len(cfg.OperatorIDs))
			return runListen(ctx, cfg)
		},
	}
	cmd.Flags().String("qa-out-dir", "",
		"If set, journal inbound/CC/outbound events to <dir>/transcript.jsonl + copy attachments to <dir>/attachments/<sha256>.<ext> (Wave 6 T10a §107.x evidence)")
	cmd.Flags().String("docs-dir", "docs",
		"Docs root containing Issues.md / Fixed.md / Status.md / CONTINUATION.md / Help.md. Issues.md MUST exist (Wave 6.5 T7).")
	return cmd
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
	// QAOutDir, when non-empty, enables JSONL journaling of every
	// bidirectional event to <QAOutDir>/transcript.jsonl plus content-
	// addressed attachment copies under <QAOutDir>/attachments/ per the
	// Wave 6 T10a §107.x evidence primitive.
	QAOutDir string
	// DocsDir is the operator-supplied docs root (Wave 6.5 T7). Default
	// "docs"; --docs-dir overrides. Issues.md must exist inside it.
	DocsDir string
	// OperatorIDs is the parsed HERALD_OPERATOR_IDS env (Wave 6.5 T7).
	// Empty / nil → all Done:/Reopen: commands rejected.
	OperatorIDs map[string]bool
	// IssueOpener (Wave 6.5 T7) wires action=issue.open through to
	// docs/Issues.md. Constructed in wireDocsAndCommands when the
	// docs dir is valid; nil in hermetic tests that don't exercise
	// the issue.open action.
	IssueOpener inbound.IssueOpener
	// Commands (Wave 6.5 T7) wires the §32.6 fast-path command handlers.
	// Constructed in wireDocsAndCommands; nil in hermetic tests that
	// don't exercise the command fast-path.
	Commands *inbound.CommandsConfig
	// EventEmitter (Wave 6.5 T7) wires action=event.emit through to
	// runner.Runner.Run. Wave 6.5 ships this nil — the operator runs
	// pherald listen + pherald serve in separate processes today, so
	// the listen-side Runner is unavailable. The dispatcher returns
	// an explicit "no EventEmitter configured" error for event.emit
	// replies when this is nil (§107 fail-loud).
	EventEmitter inbound.EventEmitter
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
	code := cfg.Code
	replier := cfg.Replier
	var jrn *journal
	if cfg.QAOutDir != "" {
		var jerr error
		jrn, jerr = newJournal(cfg.QAOutDir)
		if jerr != nil {
			return fmt.Errorf("pherald listen: open journal: %w", jerr)
		}
		defer jrn.Close()
		code = &journalingCode{j: jrn, inner: code}
		replier = &journalingReplier{j: jrn, inner: replier}
	}
	dispatcher, err := inbound.NewDispatcher(inbound.Config{
		ProjectName: cfg.ProjectName,
		Code:        code,
		TgramReply:  replier,
		Issues:      cfg.IssueOpener,
		Events:      cfg.EventEmitter,
		Commands:    cfg.Commands,
	})
	if err != nil {
		return fmt.Errorf("pherald listen: build inbound dispatcher: %w", err)
	}
	var handler commons.InboundHandler = dispatcher
	if jrn != nil {
		handler = &journalingHandler{j: jrn, inner: handler}
	}
	if err := cfg.Subscriber(ctx, handler); err != nil {
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

// parseOperatorIDs is the §32.6 Done:/Reopen: operator-role allowlist
// parser (Wave 6.5 T7). It accepts a comma-separated string of
// channel_user_id values (Telegram numeric chat IDs) and returns the
// set as a map[string]bool. Whitespace around each segment is trimmed;
// empty segments (including ",,", trailing/leading commas) are skipped.
//
// Empty / unset HERALD_OPERATOR_IDS env → empty map → all Done:/Reopen:
// commands rejected with inbound.ErrNotOperator. The pherald listen
// startup line prints the count so the operator sees the count before
// any inbound message arrives.
func parseOperatorIDs(s string) map[string]bool {
	m := map[string]bool{}
	if s == "" {
		return m
	}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		m[p] = true
	}
	return m
}

// wireDocsAndCommands constructs the Wave 6.5 T7 docs-aware dependencies
// (DocsIssueOpener + CommandsConfig) from cfg.DocsDir + cfg.OperatorIDs
// and stamps them into cfg in-place. Returns an explicit error if
// docs/Issues.md is missing — §107 fail-loud, never silent-degrade to
// "everything goes to CC because the issue opener can't see the file".
//
// Optional siblings (Status.md / CONTINUATION.md / Help.md) emit
// warnings to stderr but do not block startup; the corresponding
// commands fall back to errors-at-call-time (HandleStatus / HandleContinue
// return an error from os.ReadFile when invoked; HandleHelp falls back
// to BuiltinHelp by design).
//
// EventEmitter is left nil — the operator runs pherald listen +
// pherald serve in separate processes today (Wave 6.5 scope). The
// dispatcher returns an explicit error for action=event.emit replies
// when nil, which the operator sees as a SendReply error.
func wireDocsAndCommands(cfg *listenConfig) error {
	if cfg.DocsDir == "" {
		cfg.DocsDir = "docs"
	}
	issuesPath := filepath.Join(cfg.DocsDir, "Issues.md")
	fixedPath := filepath.Join(cfg.DocsDir, "Fixed.md")
	if _, err := os.Stat(issuesPath); err != nil {
		return fmt.Errorf("pherald listen: docs Issues.md not found at %s — set --docs-dir or chdir to repo root: %w",
			issuesPath, err)
	}
	if _, err := os.Stat(fixedPath); err != nil {
		// Fixed.md absent is a warning, not fatal — nextHRDNumber
		// tolerates the missing path. But Done:/Reopen: will fail
		// when invoked against a missing Fixed.md (with explicit
		// error to the operator), so warn at startup so the
		// operator notices.
		fmt.Fprintf(os.Stderr, "pherald listen: warning — Fixed.md missing at %s (Done:/Reopen: will fail until present)\n", fixedPath)
	}
	statusPath := filepath.Join(cfg.DocsDir, "Status.md")
	continuePath := filepath.Join(cfg.DocsDir, "CONTINUATION.md")
	helpPath := filepath.Join(cfg.DocsDir, "Help.md")
	if _, err := os.Stat(statusPath); err != nil {
		fmt.Fprintf(os.Stderr, "pherald listen: warning — Status.md missing at %s (Status: command will error until present)\n", statusPath)
	}
	if _, err := os.Stat(continuePath); err != nil {
		fmt.Fprintf(os.Stderr, "pherald listen: warning — CONTINUATION.md missing at %s (Continue: command will error until present)\n", continuePath)
	}
	// Help.md is optional by design (BuiltinHelp fallback) — no warning.
	lockPath := filepath.Join(cfg.DocsDir, ".issues.lock")
	cfg.IssueOpener = &inbound.DocsIssueOpener{
		IssuesPath: issuesPath,
		FixedPath:  fixedPath,
		LockPath:   lockPath,
		Clock:      commons.RealClock{},
	}
	cfg.Commands = &inbound.CommandsConfig{
		DocsDir:      cfg.DocsDir,
		IssuesPath:   issuesPath,
		FixedPath:    fixedPath,
		StatusPath:   statusPath,
		ContinuePath: continuePath,
		HelpPath:     helpPath,
		LockPath:     lockPath,
		OperatorIDs:  cfg.OperatorIDs,
		Clock:        commons.RealClock{},
	}
	cfg.EventEmitter = nil
	return nil
}
