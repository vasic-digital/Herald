// qaherald lifecycle T1 — Cobra subcommand skeleton.
//
// Wires the 15-scenario lifecycle test driver alongside `qaherald run`
// (Wave 5). T1 ships ONLY the skeleton: flag surface, env-var fallbacks,
// required-field validation, RunID generation, OutDir default + MkdirAll.
// The actual scenarios + messenger + orchestrator land in T2/T3/T4.
//
// Cobra subcommand contract: env-fallback resolution order is
//
//	explicit --flag → env-var fallback → built-in default
//
// REQUIRED fields (token, chat-id, pherald-bot-username) — if both flag
// AND env are unset the RunE returns a structured error citing the
// HRD-101 ticket so operator tooling can grep for "HRD-101:" to
// distinguish lifecycle wiring errors from generic Cobra errors.
//
// Security mandate (operator-locked, 2026-05-23): the QA bot token MUST
// NEVER appear in any log line, error message, or printed output —
// neither at info nor at debug level. The validator + RunE return
// errors that name the flag/env-var but never echo its value.
//
// §107 anti-bluff: lifecycle_test.go drives this command via
// cobra.Command.SetArgs(...) + t.Setenv(...) and asserts on the
// CAPTURED struct fields, not on stdout. The test proves the flag
// surface resolves to the expected values for every fallback path.
//
// HRD-101 T1 closes here.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/qaherald/internal/lifecycle"
)

// lifecycleFlags is the parsed-flag struct for `qaherald lifecycle`.
// Each field has a CLI flag AND (where applicable per the operator-
// locked decision matrix) an env-var fallback. Field comments name the
// flag + env-var so the matrix is greppable from the source.
type lifecycleFlags struct {
	QABotToken         string        // --qa-bot-token; env HERALD_QA_BOT_TOKEN
	QABotTokenNonOp    string        // --qa-bot-token-non-operator; env HERALD_QA_BOT_TOKEN_NON_OPERATOR (optional — S9 SKIPs if missing)
	ChatID             int64         // --chat-id; env HERALD_TGRAM_CHAT_ID
	PheraldBotUsername string        // --pherald-bot-username; env HERALD_PHERALD_BOT_USERNAME (must NOT start with @)
	PheraldBotUserID   int64         // --pherald-bot-user-id; env HERALD_PHERALD_BOT_USER_ID (0 → fall back to admin-scan + warn)
	OutDir             string        // --out; default docs/qa/HRD-101-lifecycle-<run-id>
	RunID              string        // --run-id; default <ISO-ts>-<4 hex chars>
	DocsDir            string        // --docs-dir; default docs (for Issues.md / Fixed.md fs-mutation assertions)
	PheraldQAOutDir    string        // --pherald-qa-out-dir; env HERALD_QA_OUT_DIR — where pherald listen writes its OWN transcript.jsonl
	Scenarios          []string      // --scenarios=S01,S02,...; default ALL
	PerScenarioTimeout time.Duration // --scenario-timeout; default 60s (CC dispatch is slow)
	OverallTimeout     time.Duration // --overall-timeout; default 30m
	SkipPreflight      bool          // --skip-preflight; default FALSE (forbid in prod — only for unit tests)
	Manual             bool          // --manual; default FALSE — if TRUE, prints scenarios and exits
}

// lifecycleCmdFlags is exported via newLifecycleCmd's returned reference
// only — but the cobra-tests in lifecycle_test.go reach into the
// captured *lifecycleFlags directly via the test-only accessor pattern
// below. We do NOT expose a package-level singleton because the run
// subcommand uses package-level flag vars (run.go) and we want to keep
// lifecycle's state local to the command's lifetime.

// newLifecycleCmd returns the `qaherald lifecycle` Cobra subcommand.
// The returned *cobra.Command holds the lifecycleFlags pointer in its
// closure so callers (including tests) can drive it via SetArgs +
// Execute without touching package state.
//
// The second return value is the *lifecycleFlags struct populated by
// Cobra's Flags().*Var bindings. Tests use this pointer to assert
// post-Execute state. Production code (main.go) discards it.
func newLifecycleCmd() (*cobra.Command, *lifecycleFlags) {
	f := &lifecycleFlags{}
	cmd := &cobra.Command{
		Use:   "lifecycle",
		Short: "Run the 15-scenario lifecycle test against pherald listen via a 2nd Telegram bot",
		Long: `qaherald lifecycle automates the S01..S15 scenarios from
tests/test_wave6.5_lifecycle.sh by posting each input via a 2nd Telegram bot
(HERALD_QA_BOT_TOKEN) and asserting pherald's reply + fs mutation.

PRE-REQS:
- pherald listen is running with --qa-out-dir set
- qa-bot is in the same group as pherald-bot
- qa-bot's Privacy Mode is DISABLED (talk to @BotFather → /setprivacy → Disable)
- HERALD_OPERATOR_IDS contains qa-bot's user-id (so S05/S06/S08/S10 succeed)

OUTPUT:
- docs/qa/HRD-101-lifecycle-<run-id>/transcript.jsonl — bidirectional events
- docs/qa/HRD-101-lifecycle-<run-id>/report.md — per-scenario PASS/FAIL
- docs/qa/HRD-101-lifecycle-<run-id>/attachments/<sha256>.<ext> — content-addressed

T1 status: SKELETON only — T2 (messenger) and T3 (scenarios) wire the
actual driver. Running this command today resolves env fallbacks,
validates required flags, creates the OutDir, and exits with a
"not-wired-yet" message.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolveLifecycleEnvFallbacks(f)

			// --manual short-circuits the required-flag validator —
			// the manual catalogue print does not need a token or
			// chat-id.
			if f.Manual {
				return lifecycle.PrintScenarios(cmd.OutOrStdout())
			}

			if err := validateLifecycleRequired(f); err != nil {
				return err
			}
			if f.RunID == "" {
				f.RunID = generateLifecycleRunID()
			}
			if f.OutDir == "" {
				f.OutDir = filepath.Join("docs", "qa", "HRD-101-lifecycle-"+f.RunID)
			}
			if err := os.MkdirAll(filepath.Join(f.OutDir, "attachments"), 0o755); err != nil {
				return fmt.Errorf("HRD-101: mkdir out-dir: %w", err)
			}

			ctx, cancel := context.WithTimeout(cmdContextOrBackground(cmd), f.OverallTimeout)
			defer cancel()

			cfg := lifecycle.Config{
				QABotToken:         f.QABotToken,
				QABotTokenNonOp:    f.QABotTokenNonOp,
				ChatID:             f.ChatID,
				PheraldBotUsername: f.PheraldBotUsername,
				PheraldBotUserID:   f.PheraldBotUserID,
				OutDir:             f.OutDir,
				RunID:              f.RunID,
				DocsDir:            f.DocsDir,
				PheraldQAOutDir:    f.PheraldQAOutDir,
				Scenarios:          f.Scenarios,
				PerScenarioTimeout: f.PerScenarioTimeout,
				SkipPreflight:      f.SkipPreflight,
			}
			return lifecycle.Run(ctx, cfg)
		},
	}

	cmd.Flags().StringVar(&f.QABotToken, "qa-bot-token", "",
		"Telegram QA bot token (env HERALD_QA_BOT_TOKEN)")
	cmd.Flags().StringVar(&f.QABotTokenNonOp, "qa-bot-token-non-operator", "",
		"Optional 2nd QA bot token NOT in HERALD_OPERATOR_IDS (env HERALD_QA_BOT_TOKEN_NON_OPERATOR) — exercises S09")
	cmd.Flags().Int64Var(&f.ChatID, "chat-id", 0,
		"Telegram group chat-id (env HERALD_TGRAM_CHAT_ID)")
	cmd.Flags().StringVar(&f.PheraldBotUsername, "pherald-bot-username", "",
		"pherald-bot username, no @ prefix (env HERALD_PHERALD_BOT_USERNAME)")
	cmd.Flags().Int64Var(&f.PheraldBotUserID, "pherald-bot-user-id", 0,
		"pherald-bot numeric user-id (env HERALD_PHERALD_BOT_USER_ID) — when set, G1 verifies presence via a real getChatMember call (works for non-admin members); when 0, G1 falls back to the best-effort admin-scan + warns")
	cmd.Flags().StringVar(&f.OutDir, "out", "",
		"Output directory; default docs/qa/HRD-101-lifecycle-<run-id>")
	cmd.Flags().StringVar(&f.RunID, "run-id", "",
		"Run ID; default auto-generated <ISO-ts>-<4 hex chars>")
	cmd.Flags().StringVar(&f.DocsDir, "docs-dir", "docs",
		"Docs directory (for Issues.md / Fixed.md fs-mutation assertions)")
	cmd.Flags().StringVar(&f.PheraldQAOutDir, "pherald-qa-out-dir", "",
		"Where pherald listen writes its own transcript (env HERALD_QA_OUT_DIR)")
	cmd.Flags().StringSliceVar(&f.Scenarios, "scenarios", nil,
		"Comma-separated subset of scenarios (default ALL = S01..S15)")
	cmd.Flags().DurationVar(&f.PerScenarioTimeout, "scenario-timeout", 60*time.Second,
		"Per-scenario timeout (CC dispatch can take 30s+; 60s is the default budget)")
	cmd.Flags().DurationVar(&f.OverallTimeout, "overall-timeout", 30*time.Minute,
		"Overall lifecycle timeout")
	cmd.Flags().BoolVar(&f.SkipPreflight, "skip-preflight", false,
		"Skip pre-flight validation (DEV ONLY — forbidden in prod)")
	cmd.Flags().BoolVar(&f.Manual, "manual", false,
		"Print scenarios and exit; old shell script delegates to this for the manual UX")
	return cmd, f
}

// resolveLifecycleEnvFallbacks resolves env vars for any flag that is
// still empty after Cobra's pflag parse. Resolution order is
// flag → env → default; this helper handles the env step.
//
// The QA bot token is read but never logged. errors that mention the
// token MUST cite the env-var NAME or the flag NAME — never the value.
func resolveLifecycleEnvFallbacks(f *lifecycleFlags) {
	if f.QABotToken == "" {
		f.QABotToken = os.Getenv("HERALD_QA_BOT_TOKEN")
	}
	if f.QABotTokenNonOp == "" {
		f.QABotTokenNonOp = os.Getenv("HERALD_QA_BOT_TOKEN_NON_OPERATOR")
	}
	if f.ChatID == 0 {
		if s := os.Getenv("HERALD_TGRAM_CHAT_ID"); s != "" {
			// Sscanf intentionally swallows the err — invalid env yields
			// ChatID=0 which trips validateLifecycleRequired below.
			_, _ = fmt.Sscanf(s, "%d", &f.ChatID)
		}
	}
	if f.PheraldBotUsername == "" {
		f.PheraldBotUsername = os.Getenv("HERALD_PHERALD_BOT_USERNAME")
	}
	if f.PheraldBotUserID == 0 {
		if s := os.Getenv("HERALD_PHERALD_BOT_USER_ID"); s != "" {
			// Sscanf intentionally swallows the err — invalid env yields
			// PheraldBotUserID=0 which trips the admin-scan fallback path.
			_, _ = fmt.Sscanf(s, "%d", &f.PheraldBotUserID)
		}
	}
	if f.PheraldQAOutDir == "" {
		f.PheraldQAOutDir = os.Getenv("HERALD_QA_OUT_DIR")
	}
}

// validateLifecycleRequired returns a non-nil error citing the missing
// required field. The error mentions the FLAG NAME and ENV-VAR NAME but
// NEVER the value (security mandate — token must never appear in any
// error path).
func validateLifecycleRequired(f *lifecycleFlags) error {
	if f.QABotToken == "" {
		return fmt.Errorf("HRD-101: --qa-bot-token or env HERALD_QA_BOT_TOKEN is required")
	}
	if f.ChatID == 0 {
		return fmt.Errorf("HRD-101: --chat-id or env HERALD_TGRAM_CHAT_ID is required (non-zero int64)")
	}
	if f.PheraldBotUsername == "" {
		return fmt.Errorf("HRD-101: --pherald-bot-username or env HERALD_PHERALD_BOT_USERNAME is required")
	}
	return nil
}

// generateLifecycleRunID returns a fresh run-id of the form
//
//	2026-05-23T14-22-00-9f2c
//
// — RFC-3339-like ISO timestamp with colons hyphen-replaced (so the id
// is safe for directory names on all filesystems) plus 4 hex chars of
// crypto/rand entropy so two runs in the same second still get unique
// ids.
//
// T4 will replace this with a UUIDv7-backed helper that reuses
// commons.UUIDv7 (or equivalent) so qaherald's run-ids share their
// generation contract with Herald's CloudEvent ids. For T1 we use
// crypto/rand + 4 hex chars (per the user-task spec).
func generateLifecycleRunID() string {
	ts := time.Now().UTC().Format("2006-01-02T15-04-05")
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing is a system-level catastrophic event;
		// fall back to a fixed marker so the id is still well-formed
		// and the caller's MkdirAll does not error out on an empty
		// suffix. The "0000" suffix is greppable as a diagnostic.
		return ts + "-0000"
	}
	return ts + "-" + hex.EncodeToString(b[:])
}

// cmdContextOrBackground returns cmd.Context() when non-nil and
// context.Background() otherwise. Cobra harnesses without an explicit
// SetContext call (notably tests) return nil; we wrap to keep the
// RunE body branch-free.
func cmdContextOrBackground(cmd *cobra.Command) context.Context {
	if c := cmd.Context(); c != nil {
		return c
	}
	return context.Background()
}
