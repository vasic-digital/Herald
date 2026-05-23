<div align="center">

<img src="../../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# qaherald-auto — 15-Scenario Automated Lifecycle Test Framework

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` per Universal Constitution §11.4.70. Each task below dispatches to its own implementer subagent. Steps use checkbox (`- [ ]`) syntax. Run each task's commit only AFTER its self-review block passes.

**Goal:** Close the manual operator-typing loop in `tests/test_wave6.5_lifecycle.sh`. Today S1..S15 require a human to type each scenario's input into Telegram, press ENTER, and watch the response. After this plan lands, the operator runs a single command — `qaherald lifecycle --qa-bot-token=... --chat-id=... --pherald-bot-username=atmosphere_worker_bot --out=docs/qa/HRD-101-lifecycle-<id>/` — and the binary drives all 15 scenarios autonomously, posting inputs via a SECOND Telegram bot account (`@herald_qa_bot`), reading pherald-bot's replies via `getUpdates`, asserting wire-byte evidence, and emitting a full bidirectional transcript + Markdown report under `docs/qa/<run-id>/`. Targets tag **`v0.5.1`** after T8 lands.

**Architecture (locked):** 2nd Telegram bot account approach (NOT MTProto). Token in env `HERALD_QA_BOT_TOKEN` (distinct from pherald-bot's `HERALD_TGRAM_BOT_TOKEN`). Both bots are members of the same group (`HERALD_TGRAM_CHAT_ID`). qa-bot has Privacy Mode DISABLED via @BotFather (`/setprivacy` → Disable) so its `getUpdates` returns ALL group traffic — including pherald-bot's outbound replies. pherald's self-filter is `Sender.IsBot && Sender.Username == bot.Me.Username` — qa-bot has a DIFFERENT Username so pherald processes qa-bot's messages (cross-bot allowed per Wave 6 T4). `Sender.IsBot=true` does not affect classification or routing; only username-based self-filter matters.

**Tech Stack:** Go 1.25+; reuse `gopkg.in/telebot.v3` (already vendored at `submodules/telebot/`); reuse existing qaherald packages (`internal/transcript/`, `internal/tgram/`, `internal/herald/`); `github.com/spf13/cobra` for subcommand wiring; `net/http/httptest` for unit tests. NO new external dependencies. NO MTProto (Wave 7 territory). NO GUI.

**Spec reference:** `docs/specs/mvp/specification.V3.md` §32.6 (command vocabulary), §18.1.1 (command-prefix replies — `Help:`, `Status:`, `Continue:`, `Bug:`, `Task:`, `Query:`, `Done:`, `Reopen:`), §11.0 (Channel contract), §43 (commands catalogue). `docs/guides/HERALD_CONSTITUTION.md` §107 (anti-bluff covenant), §107.x (docs/qa evidence mandate — this is the §107.x instrument), §107.y (working-tree quiescence).

**Substrate already landed (do NOT re-implement):**

- `qaherald/cmd/qaherald/{main,run}.go` — Cobra root with `qaherald version` + `qaherald run` (Wave 5 T1–T7)
- `qaherald/internal/transcript/` — JSONL writer + content-addressed `attachments/<sha256>.<ext>` (Wave 5 T2)
- `qaherald/internal/tgram/` — telebot.v3 wrapper: `Send`, `Upload`, `Download`, `WaitForMessage`, `WaitForReply`, `getUpdates` long-poll (Wave 5 T3)
- `qaherald/internal/herald/` — Herald REST client with JWT + TOON + h3 attempt (Wave 5 T4 — kept around for future re-use even though lifecycle scenarios do NOT post directly to `/v1/events`; they go via Telegram so pherald listen ingests)
- `qaherald/internal/scenario/` — 8 Wave 5 scenarios + Orchestrator (NOT used by lifecycle; lifecycle gets its own package — see T3)
- `qaherald/internal/report/` — Markdown report generator (Wave 5 T6 — re-used by lifecycle report)
- `tests/test_wave6.5_lifecycle.sh` — current manual-prompt driver (will be repointed in T8)

Stash@{0} contains partial T8 work (URL-fix + `--scenarios` comma-flag + SKIP_OVERSIZED + `herald.Client` extensions). The lifecycle work does NOT depend on that stash — it lives in NEW packages (`internal/messenger/`, `internal/lifecycle/`) so the stash stays orthogonal and can be popped independently.

**§107 anti-bluff watershed (CRITICAL):** qaherald-auto is itself a §107 instrument — its OUTPUT (`docs/qa/HRD-101-lifecycle-<run-id>/`) is the auditable evidence for the 15-scenario lifecycle. A bluff here is a meta-bluff. Three anchors:

1. **Real-network bytes only.** Every transcript event backed by a real socket round-trip — qa-bot's `sendMessage` MUST cross the wire to api.telegram.org and pherald's response MUST appear in qa-bot's `getUpdates`. Unit tests use `httptest` to simulate the wire; production runs cross it for real.
2. **message_id chain proof.** Every PASS cites (a) the inbound `message_id` from qa-bot→Telegram, (b) the outbound `reply_to_message_id` linking pherald's reply back to (a), and (c) the fs-mutation diff hunk (Issues.md / Fixed.md) where lifecycle actions actually mutated state.
3. **Pre-flight strictness.** Validator REFUSES to start without all gates green — no "best effort" silent degradation. Missing token → exit code 2. qa-bot privacy enabled → exit code 3. qa-bot not in chat → exit code 4.

**Operator-locked decisions (recorded 2026-05-23, verbatim — DO NOT re-decide):**

| Decision | Value |
|---|---|
| 2nd bot path | Bot API only (telebot.v3). NOT MTProto. NOT user-account impersonation. |
| Tag | `v0.5.1` after T8 lands (NOT pull/retag v0.5.0). Increments after Wave 5's `v0.5.0`. |
| Reusability | Single CLI invocation regenerates full evidence for every re-test round. |
| Interface | `MessengerClient` interface — Telegram is first impl, Slack/Email adapters in Wave 7. |
| Scope cap | 8 tasks total. HARD STOP. Do not grow to T9/T10. |
| Run-ID format | ISO timestamp + UUIDv7 short suffix: `2026-05-23T14-22-00-9f2c` (colon → hyphen; trailing 4 hex of UUIDv7 for uniqueness within a single second) |
| Out-dir | `docs/qa/HRD-101-lifecycle-<run-id>/` (HRD-101 is the lifecycle HRD per `docs/Issues.md`) |
| Transcript format | Reuse Wave 5 JSONL — one event per line: `{ts, direction, kind, payload, attachments[]}`. Existing tooling (e2e_bluff_hunt E71-E80) continues to grep this format. |
| Attachment storage | Reuse Wave 5 content-addressing: `docs/qa/<run-id>/attachments/<sha256>.<ext>`. |
| QA-bot env var name | `HERALD_QA_BOT_TOKEN`. MUST be distinct from `HERALD_TGRAM_BOT_TOKEN` (pherald-bot). Validator FAILs if both are equal. |

**Scenarios (15 from `tests/test_wave6.5_lifecycle.sh`):**

| # | Name | Input (qa-bot → group) | Expected classification | Expected action | Expected reply | Expected fs |
|---|---|---|---|---|---|---|
| S1 | `plain-greeting-query-fallthrough` | `Hello, can you help with debugging?` | `query` | `cc.dispatch` | CC echo or CC error | no-op |
| S2 | `help-fastpath` | `Help: lifecycle` | `command_prefix:help` | `builtin.help` | docs/guides/COMMANDS.md excerpt | no-op |
| S3 | `status-fastpath` | `Status:` | `command_prefix:status` | `builtin.status` | docs/Status.md prose | no-op |
| S4 | `continue-fastpath` | `Continue:` | `command_prefix:continue` | `builtin.continue` | docs/CONTINUATION.md prose | no-op |
| S5 | `bug-prefix-cc-issue-open` | `Bug: pherald listen crashes on empty Accept header` | `command_prefix:bug` | `cc.issue.open` | `Opened HRD-NNN ...` | Issues.md +1 HRD row |
| S6 | `task-prefix-cc-issue-open` | `Task: implement S15 emoji fallthrough` | `command_prefix:task` | `cc.issue.open` | `Opened HRD-NNN ...` | Issues.md +1 HRD row |
| S7 | `query-prefix-cc-research` | `Query: what is the TOON byte savings on /v1/events?` | `command_prefix:query` | `cc.research` | CC research summary | no-op |
| S8 | `done-operator-migrate` | `Done: HRD-NNN` (from S5/S6) | `command_prefix:done` | `lifecycle.done` | `Migrated HRD-NNN to Fixed ...` | Issues.md −1, Fixed.md +1 |
| S9 | `done-non-operator-reject` | `Done: HRD-NNN` (from a non-allowlisted qa-bot identity) | `command_prefix:done` | `lifecycle.done.reject` | `Forbidden — operator role required` | no-op |
| S10 | `reopen-operator-migrate` | `Reopen: HRD-NNN` (from S8) | `command_prefix:reopen` | `lifecycle.reopen` | `Migrated HRD-NNN to Issues ...` | Fixed.md −1, Issues.md +1 |
| S11 | `inbound-photo-bug-caption` | photo + caption `Bug: red error banner` | `command_prefix:bug` + attachment | `cc.issue.open` w/ attachment | `Opened HRD-NNN with attachment <sha256>` | Issues.md +1 row, attachments/<sha256>.jpg |
| S12 | `inbound-document-task-caption` | document.pdf + caption `Task: review the API spec` | `command_prefix:task` + attachment | `cc.issue.open` w/ attachment | `Opened HRD-NNN with attachment <sha256>` | Issues.md +1 row, attachments/<sha256>.pdf |
| S13 | `inbound-voice-audio` | voice message (ogg/opus from Telegram voice recorder) | classified as voice → fallthrough query | `cc.dispatch` w/ audio | CC summary or transcription | attachments/<sha256>.ogg |
| S14 | `outbound-attachment-fanout` | `Status:` then assert pherald's outbound reply attaches a file (e.g. logo PNG via SendReply API) | `command_prefix:status` | `builtin.status` + outbound attachment | reply contains an attachment whose sha256 matches a known file | attachments/<sha256>.png recorded as `direction=outbound` |
| S15 | `natural-language-emoji-fallthrough` | `Hey 👋 quick question about 🚀 perf 🤔` | `query` | `cc.dispatch` | CC echo | no-op |

S9 (non-operator-reject) is the trickiest — qa-bot's user-id is by definition a single Telegram bot account, but `HERALD_OPERATOR_IDS` is the allowlist pherald checks. T3 handles this by configuring the lifecycle scenarios with TWO qa-bot configurations: `qa-bot-A` (in `HERALD_OPERATOR_IDS`) and `qa-bot-B` (NOT in `HERALD_OPERATOR_IDS`). If only one qa-bot token is supplied, S9 emits SKIP-with-reason. Operator can supply a second token via `HERALD_QA_BOT_TOKEN_NON_OPERATOR` to exercise S9.

---

## File Structure

### CREATE

| Path | Responsibility |
|---|---|
| `qaherald/internal/messenger/messenger.go` | `MessengerClient` interface + supporting types (`Reply`, `Predicate`, errors) |
| `qaherald/internal/messenger/telegram.go` | Telegram impl — wraps existing `qaherald/internal/tgram/` adapter |
| `qaherald/internal/messenger/telegram_test.go` | Unit tests with `httptest` server impersonating api.telegram.org |
| `qaherald/internal/lifecycle/scenario.go` | `Scenario` type + `Registry` + helpers |
| `qaherald/internal/lifecycle/lifecycle.go` | Orchestrator: loads scenarios, runs serially, captures transcript+report |
| `qaherald/internal/lifecycle/s01_greeting.go` ... `s15_emoji.go` | 15 scenarios, one file each |
| `qaherald/internal/lifecycle/report.go` | Markdown report generator (delegates to Wave 5 `internal/report/` for shared bits) |
| `qaherald/internal/lifecycle/preflight.go` | Pre-flight validator: env, getMe, privacy, getChatMember |
| `qaherald/internal/lifecycle/lifecycle_test.go` | Unit tests: happy path + missing-reply timeout + wrong-classification FAIL + Issues.md no-mutation FAIL + Fixed.md migration check (5–8 tests) |
| `qaherald/cmd/qaherald/lifecycle.go` | Cobra subcommand `qaherald lifecycle ...` |
| `qaherald/cmd/qaherald/lifecycle_test.go` | Cobra-level wiring test (flag binding, env fallback) |

### MODIFY

| Path | Why |
|---|---|
| `qaherald/cmd/qaherald/main.go` | Register the new `lifecycle` subcommand alongside `version` + `run` |
| `tests/test_wave6.5_lifecycle.sh` | Delegate to `qaherald lifecycle` for automated runs; keep `--manual` flag for old prompt-based UX |

### TOUCH (incidental)

| Path | Why |
|---|---|
| `docs/Issues.md` | New HRD-NNN rows for qaherald-auto tasks (one per T1..T8) |
| `docs/Fixed.md` | Closed-HRD rows as each task lands |
| `docs/Status.md` | r-bump after T8 |
| `CLAUDE.md` | r-bump (revision 9) noting qaherald-auto landing |
| `docs/CONTINUATION.md` | New live-test prompt for HERALD_QA_BOT_TOKEN |

---

## Task 1: `qaherald lifecycle` Cobra subcommand skeleton

**Subagent dispatch:** Spawn one implementer subagent per `superpowers:subagent-driven-development`. The subagent inherits this plan and operates ONLY within T1 scope.

### Goal

Add a `qaherald lifecycle` subcommand to the existing qaherald binary with the full flag surface and env fallbacks. No scenarios yet — just the wiring and a version-sanity test.

### Scope

- `qaherald/cmd/qaherald/lifecycle.go` (new file): Cobra command definition.
- `qaherald/cmd/qaherald/main.go` (modify): `rootCmd.AddCommand(newLifecycleCmd())`.
- `qaherald/cmd/qaherald/lifecycle_test.go` (new file): table-driven test of flag parsing + env fallback.

### Flag surface

```go
type lifecycleFlags struct {
    QABotToken              string        // --qa-bot-token; env HERALD_QA_BOT_TOKEN
    QABotTokenNonOp         string        // --qa-bot-token-non-operator; env HERALD_QA_BOT_TOKEN_NON_OPERATOR (optional — S9 SKIPs if missing)
    ChatID                  int64         // --chat-id; env HERALD_TGRAM_CHAT_ID
    PheraldBotUsername      string        // --pherald-bot-username; env HERALD_PHERALD_BOT_USERNAME (must NOT start with @)
    OutDir                  string        // --out; default docs/qa/HRD-101-lifecycle-<run-id>
    RunID                   string        // --run-id; default <ISO-ts>-<uuidv7-4hex>
    DocsDir                 string        // --docs-dir; default docs/ (for Issues.md / Fixed.md fs-mutation assertions)
    PheraldQAOutDir         string        // --pherald-qa-out-dir; env HERALD_QA_OUT_DIR — where pherald listen writes its OWN transcript.jsonl (lifecycle reads it for classification assertions)
    Scenarios               []string      // --scenarios=S1,S2,...; default ALL
    PerScenarioTimeout      time.Duration // --scenario-timeout; default 60s (CC dispatch is slow)
    OverallTimeout          time.Duration // --overall-timeout; default 30m
    SkipPreflight           bool          // --skip-preflight; default FALSE (forbid in prod — only for unit tests)
    Manual                  bool          // --manual; defaults FALSE — if TRUE, prints scenarios and exits (lets the old shell script keep working)
}
```

### Env fallback contract

Resolution order: explicit `--flag` wins → env var fallback → default. If both a flag AND env are unset for a REQUIRED field (token, chat-id, pherald-bot-username), Cobra returns `RequiredFlag` error with HRD-NNN pointer.

### Files

`qaherald/cmd/qaherald/lifecycle.go`:

```go
package main

import (
    "context"
    "fmt"
    "os"
    "time"

    "github.com/spf13/cobra"
    "qaherald/internal/lifecycle"
)

func newLifecycleCmd() *cobra.Command {
    var f lifecycleFlags
    cmd := &cobra.Command{
        Use:   "lifecycle",
        Short: "Run the 15-scenario lifecycle test against pherald listen via a 2nd Telegram bot",
        Long: `qaherald lifecycle automates the S1..S15 scenarios from
tests/test_wave6.5_lifecycle.sh by posting each input via a 2nd Telegram bot
(HERALD_QA_BOT_TOKEN) and asserting pherald's reply + fs mutation.

PRE-REQS:
- pherald listen is running with --qa-out-dir set
- qa-bot is in the same group as pherald-bot
- qa-bot's Privacy Mode is DISABLED (talk to @BotFather → /setprivacy → Disable)
- HERALD_OPERATOR_IDS contains qa-bot's user-id (so S5/S6/S8/S10 succeed)

OUTPUT:
- docs/qa/HRD-101-lifecycle-<run-id>/transcript.jsonl — bidirectional events
- docs/qa/HRD-101-lifecycle-<run-id>/report.md — per-scenario PASS/FAIL
- docs/qa/HRD-101-lifecycle-<run-id>/attachments/<sha256>.<ext> — content-addressed
`,
        RunE: func(cmd *cobra.Command, args []string) error {
            resolveEnvFallbacks(&f)
            if err := validateRequired(&f); err != nil {
                return err
            }
            if f.RunID == "" {
                f.RunID = lifecycle.GenerateRunID()
            }
            if f.OutDir == "" {
                f.OutDir = fmt.Sprintf("docs/qa/HRD-101-lifecycle-%s", f.RunID)
            }
            ctx, cancel := context.WithTimeout(cmd.Context(), f.OverallTimeout)
            defer cancel()
            if f.Manual {
                return lifecycle.PrintScenariosOnly(os.Stdout)
            }
            return lifecycle.Run(ctx, lifecycle.Config{
                QABotToken:         f.QABotToken,
                QABotTokenNonOp:    f.QABotTokenNonOp,
                ChatID:             f.ChatID,
                PheraldBotUsername: f.PheraldBotUsername,
                OutDir:             f.OutDir,
                RunID:              f.RunID,
                DocsDir:            f.DocsDir,
                PheraldQAOutDir:    f.PheraldQAOutDir,
                Scenarios:          f.Scenarios,
                PerScenarioTimeout: f.PerScenarioTimeout,
                SkipPreflight:      f.SkipPreflight,
            })
        },
    }
    cmd.Flags().StringVar(&f.QABotToken, "qa-bot-token", "", "Telegram QA bot token (env HERALD_QA_BOT_TOKEN)")
    cmd.Flags().StringVar(&f.QABotTokenNonOp, "qa-bot-token-non-operator", "", "Optional 2nd QA bot token NOT in HERALD_OPERATOR_IDS (env HERALD_QA_BOT_TOKEN_NON_OPERATOR) — exercises S9")
    cmd.Flags().Int64Var(&f.ChatID, "chat-id", 0, "Telegram group chat-id (env HERALD_TGRAM_CHAT_ID)")
    cmd.Flags().StringVar(&f.PheraldBotUsername, "pherald-bot-username", "", "pherald-bot username, no @ prefix (env HERALD_PHERALD_BOT_USERNAME)")
    cmd.Flags().StringVar(&f.OutDir, "out", "", "Output directory; default docs/qa/HRD-101-lifecycle-<run-id>")
    cmd.Flags().StringVar(&f.RunID, "run-id", "", "Run ID; default auto-generated")
    cmd.Flags().StringVar(&f.DocsDir, "docs-dir", "docs", "Docs directory (for Issues.md / Fixed.md fs-mutation assertions)")
    cmd.Flags().StringVar(&f.PheraldQAOutDir, "pherald-qa-out-dir", "", "Where pherald listen writes its own transcript (env HERALD_QA_OUT_DIR)")
    cmd.Flags().StringSliceVar(&f.Scenarios, "scenarios", nil, "Comma-separated subset of scenarios (default ALL = S1..S15)")
    cmd.Flags().DurationVar(&f.PerScenarioTimeout, "scenario-timeout", 60*time.Second, "Per-scenario timeout")
    cmd.Flags().DurationVar(&f.OverallTimeout, "overall-timeout", 30*time.Minute, "Overall lifecycle timeout")
    cmd.Flags().BoolVar(&f.SkipPreflight, "skip-preflight", false, "Skip pre-flight validation (DEV ONLY)")
    cmd.Flags().BoolVar(&f.Manual, "manual", false, "Print scenarios and exit; old shell script delegates to this for the manual UX")
    return cmd
}

func resolveEnvFallbacks(f *lifecycleFlags) {
    if f.QABotToken == "" {
        f.QABotToken = os.Getenv("HERALD_QA_BOT_TOKEN")
    }
    if f.QABotTokenNonOp == "" {
        f.QABotTokenNonOp = os.Getenv("HERALD_QA_BOT_TOKEN_NON_OPERATOR")
    }
    if f.ChatID == 0 {
        if s := os.Getenv("HERALD_TGRAM_CHAT_ID"); s != "" {
            fmt.Sscanf(s, "%d", &f.ChatID)
        }
    }
    if f.PheraldBotUsername == "" {
        f.PheraldBotUsername = os.Getenv("HERALD_PHERALD_BOT_USERNAME")
    }
    if f.PheraldQAOutDir == "" {
        f.PheraldQAOutDir = os.Getenv("HERALD_QA_OUT_DIR")
    }
}

func validateRequired(f *lifecycleFlags) error {
    if f.QABotToken == "" {
        return fmt.Errorf("HRD-101: --qa-bot-token or env HERALD_QA_BOT_TOKEN is required")
    }
    if f.ChatID == 0 {
        return fmt.Errorf("HRD-101: --chat-id or env HERALD_TGRAM_CHAT_ID is required")
    }
    if f.PheraldBotUsername == "" {
        return fmt.Errorf("HRD-101: --pherald-bot-username or env HERALD_PHERALD_BOT_USERNAME is required")
    }
    return nil
}
```

`qaherald/cmd/qaherald/main.go` — add line:

```go
rootCmd.AddCommand(newLifecycleCmd())
```

`qaherald/cmd/qaherald/lifecycle_test.go`:

- Table-driven: 6 cases — all-flags-set; env-fallback for each required field; missing token; missing chat-id; missing pherald-bot-username; `--manual` short-circuits.
- Uses `cobra.Command.SetArgs(...)` + `t.Setenv(...)` for hermetic env injection.

### Implementation steps

- [ ] Subagent creates `qaherald/internal/lifecycle/` package with placeholder `lifecycle.go`, `scenario.go` (just compile-able stubs: `Run`, `Config`, `GenerateRunID`, `PrintScenariosOnly`).
- [ ] Subagent creates `qaherald/cmd/qaherald/lifecycle.go` exactly as above.
- [ ] Subagent registers `newLifecycleCmd()` in `main.go`.
- [ ] Subagent writes `qaherald/cmd/qaherald/lifecycle_test.go` (6 cases).
- [ ] Subagent runs `go build ./qaherald/...` from repo root → must compile.
- [ ] Subagent runs `go test ./qaherald/cmd/qaherald/...` → all 6 cases PASS.
- [ ] Subagent runs `/tmp/qaherald lifecycle --help` → prints flag surface + Long description.
- [ ] Subagent runs `/tmp/qaherald lifecycle` (no env, no flags) → exits non-zero with the HRD-101 RequiredFlag error.

### Self-review

- [ ] Flag count matches table (12 flags).
- [ ] Env-fallback order is flag-wins-over-env-wins-over-default.
- [ ] No scenarios implemented yet (those are T3).
- [ ] `main.go` registration is one line, idempotent.

### Commit

```
qaherald lifecycle T1: Cobra subcommand skeleton + env-fallback flag surface

Wires `qaherald lifecycle` alongside `qaherald run` (Wave 5). 12 flags with
env fallbacks (HERALD_QA_BOT_TOKEN, HERALD_TGRAM_CHAT_ID, HERALD_PHERALD_BOT_USERNAME,
HERALD_QA_BOT_TOKEN_NON_OPERATOR, HERALD_QA_OUT_DIR). --manual short-circuits
for the old shell-script UX. No scenarios yet (T3).

Closes HRD-101 T1.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
```

---

## Task 2: `qaherald/internal/messenger/` — `MessengerClient` interface + Telegram impl

**Subagent dispatch:** Fresh subagent for T2. Inherits this plan; operates ONLY within T2 scope.

### Goal

Introduce the messenger abstraction so Slack/Email adapters can plug in during Wave 7 without rewriting scenario code. Telegram impl wraps the existing `qaherald/internal/tgram/` adapter.

### Files

`qaherald/internal/messenger/messenger.go`:

```go
// Package messenger is qaherald's messenger-agnostic interface for lifecycle
// scenarios. Telegram is the first impl (T2); Slack and Email come in Wave 7
// without changing scenario code.
package messenger

import (
    "context"
    "io"
    "time"
)

// MessageID is whatever opaque string identifies a message in the underlying
// messenger. Telegram uses int64 → string; Slack uses "1654.123456".
type MessageID string

// Reply is a captured inbound message (from the messenger to qaherald).
type Reply struct {
    MessageID         MessageID         // unique per messenger
    ReplyToMessageID  MessageID         // empty if this is not a reply
    SenderUsername    string            // bot or user username
    SenderIsBot       bool              // true if sender is a bot account
    Text              string            // text content (empty if attachment-only)
    Attachments       []Attachment      // file ids + mime types
    Timestamp         time.Time         // messenger-side timestamp
    Raw               []byte            // raw JSON for forensic anchor in transcript
}

type Attachment struct {
    FileID       string // messenger's file-id (Telegram file_id / Slack file id)
    ContentType  string // "image/jpeg", "application/pdf", "audio/ogg", etc.
    SizeBytes    int64
    OriginalName string // best-effort; some messengers don't carry filename
}

// Predicate selects a reply from the stream. Returning true accepts.
type Predicate func(Reply) bool

// MessengerClient is the lifecycle-facing interface. Implementations MUST be
// safe for sequential use; concurrency is the orchestrator's responsibility.
type MessengerClient interface {
    // Bot identity — for self-filter logic and report attribution.
    Me() (username string, userID int64, err error)

    // Send a text message into the configured chat. Returns the messenger's
    // message-id. The implementation MUST cross the network — no caching.
    Send(ctx context.Context, text string) (MessageID, error)

    // Send a photo attachment with an optional caption.
    SendPhoto(ctx context.Context, path string, caption string) (MessageID, error)

    // Send a document (any non-photo binary).
    SendDocument(ctx context.Context, path string, caption string) (MessageID, error)

    // Send a voice/audio attachment.
    SendVoice(ctx context.Context, path string) (MessageID, error)

    // Wait for the first reply that satisfies the predicate. Returns
    // context.DeadlineExceeded on timeout. Does NOT consume non-matching
    // messages; they remain in the update queue for the next call.
    WaitForReply(ctx context.Context, toMsgID MessageID, pred Predicate, timeout time.Duration) (Reply, error)

    // Download an attachment from the messenger by file-id. Used to verify
    // sha256 round-trip on outbound attachments (S14).
    Download(ctx context.Context, fileID string) (io.ReadCloser, error)

    // GetUpdates exposes the raw update-fetch loop for the orchestrator to
    // pull all pending messages (used at scenario teardown to drain residue).
    GetUpdates(ctx context.Context, offset int64) ([]Reply, int64, error)

    // Pre-flight self-check — returns a structured report consumed by
    // lifecycle/preflight.go.
    Preflight(ctx context.Context, expectedChatID int64) (PreflightReport, error)
}

type PreflightReport struct {
    Username                  string
    UserID                    int64
    CanReadAllGroupMessages   bool   // Telegram: getMe.can_read_all_group_messages
    InChat                    bool   // Telegram: getChatMember returns non-error
    ChatType                  string // "group", "supergroup", "channel"; lifecycle requires group|supergroup
    PheraldBotPresent         bool   // best-effort: getChatAdministrators contains pherald-bot-username
}
```

`qaherald/internal/messenger/telegram.go`:

- `TelegramClient` struct wraps `*tgram.Client` (Wave 5).
- Implements every interface method.
- `Send` → `tgram.Client.Send`.
- `SendPhoto`/`SendDocument`/`SendVoice` → `tgram.Client.Upload` with the right `telebot.SendOptions`.
- `WaitForReply` → wraps `tgram.Client.WaitForReply` (already exists from Wave 5 T3) — predicates re-expressed as `func(*telebot.Message) bool` inside the wrapper.
- `Download` → `tgram.Client.Download`.
- `GetUpdates` → direct `bot.Updates()` channel drain with offset bookkeeping.
- `Preflight` → `bot.Me()`, parse `can_read_all_group_messages` via reflection (telebot.v3 exposes it), `bot.ChatByID(chatID)` for `ChatType`, `bot.ChatMemberOf(chat, &telebot.User{ID: int64(<pherald-bot-username-resolved>)})` for `PheraldBotPresent`.

**telebot.v3 privacy-flag access:** telebot's `Bot.Me.CanReadAllGroupMessages` exists (added telebot v3.2+). If the vendored telebot lacks the field, fall back to a raw `/getMe` HTTP call against `https://api.telegram.org/bot<TOKEN>/getMe` and parse `result.can_read_all_group_messages` from the JSON response. The fallback also documents which telebot version the operator's vendored copy is at; T2 self-review verifies.

`qaherald/internal/messenger/telegram_test.go`:

- `httptest.Server` impersonates `api.telegram.org`.
- 6 cases:
  1. `Send` posts to `/sendMessage` with the expected payload → returns message-id from JSON response.
  2. `SendPhoto` uploads via multipart → asserts the file bytes match what was read from disk.
  3. `SendDocument` same, with `Content-Type: application/octet-stream`.
  4. `WaitForReply` returns the first reply whose `reply_to_message_id` matches → ignores unrelated messages.
  5. `Preflight` returns `CanReadAllGroupMessages=true` when stub returns it; `=false` otherwise.
  6. `Download` round-trips bytes from `getFile` + `file/<token>/<path>`.

### Implementation steps

- [ ] Subagent creates `qaherald/internal/messenger/messenger.go` with the interface + types above.
- [ ] Subagent creates `qaherald/internal/messenger/telegram.go` wrapping `qaherald/internal/tgram/`.
- [ ] Subagent verifies telebot.v3 field access; falls back to raw HTTP if needed.
- [ ] Subagent writes `qaherald/internal/messenger/telegram_test.go` (6 cases) using `httptest.Server` to impersonate api.telegram.org.
- [ ] Subagent runs `go test ./qaherald/internal/messenger/... -race -count=1` — all 6 PASS.
- [ ] Subagent runs `go vet ./qaherald/internal/messenger/...` — clean.

### Self-review

- [ ] No business logic — pure adapter.
- [ ] No global state — `TelegramClient` carries its own `*tgram.Client`.
- [ ] Real-network behaviour is the impl's job; tests use `httptest`.
- [ ] Future Slack impl will be a sibling file `slack.go` that fills the same interface — no scenario changes.
- [ ] `Preflight` returns ALL the fields the validator (T6) needs.

### Commit

```
qaherald lifecycle T2: MessengerClient interface + Telegram impl

Introduces messenger-agnostic interface so Wave 7 can plug Slack/Email
adapters without changing scenario code. Telegram impl wraps the existing
qaherald/internal/tgram/ adapter. 6 unit tests via httptest impersonating
api.telegram.org.

Closes HRD-101 T2.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
```

---

## Task 3: `qaherald/internal/lifecycle/` — Scenario package + 15 scenario files

**Subagent dispatch:** Fresh subagent for T3. T3 is the largest task — budget 2 hours.

### Goal

Implement the `Scenario` type, the 15 scenario files (S1..S15), and the helpers each scenario needs. Each scenario is a standalone Go func that uses `MessengerClient` for IO and asserts on:

1. The captured `Reply` from pherald-bot (text + attachments).
2. The `transcript.jsonl` emitted by pherald listen (read live from `--pherald-qa-out-dir`) — proves classification + action are what we expect.
3. The fs state of `docs/Issues.md` + `docs/Fixed.md` (diff before/after) — proves lifecycle actions actually mutated state.

### Files

`qaherald/internal/lifecycle/scenario.go`:

```go
package lifecycle

import (
    "context"
    "time"

    "qaherald/internal/messenger"
    "qaherald/internal/transcript"
)

// Scenario is a single lifecycle test case. Each scenario does its own IO via
// the MessengerClient and reports a Result.
type Scenario struct {
    ID                       string                  // "S01", "S02", ...
    Name                     string                  // "plain-greeting-query-fallthrough"
    Description              string                  // human-readable purpose
    Run                      ScenarioRun             // the actual function
    RequiresOperatorRole     bool                    // S5/S6/S8/S10 — qa-bot user-id must be in HERALD_OPERATOR_IDS
    RequiresNonOperatorBot   bool                    // S9 only — uses QABotTokenNonOp
}

type ScenarioRun func(ctx context.Context, env *Env) Result

// Env is the per-scenario environment.
type Env struct {
    Msgr            messenger.MessengerClient
    MsgrNonOp       messenger.MessengerClient // nil if HERALD_QA_BOT_TOKEN_NON_OPERATOR unset
    Transcript      *transcript.Writer
    PheraldBotUser  string
    DocsDir         string
    PheraldQADir    string
    ChatID          int64
    PerTimeout      time.Duration
}

// Result is the per-scenario outcome consumed by the report.
type Result struct {
    ScenarioID         string
    PASS               bool
    FailureReason      string
    InboundMessageID   messenger.MessageID
    ReplyMessageID     messenger.MessageID
    ClassificationSeen string  // from pherald's transcript
    ActionSeen         string  // from pherald's transcript
    Evidence           []EvidenceFragment
    StartedAt          time.Time
    Duration           time.Duration
}

type EvidenceFragment struct {
    Kind    string // "reply-text", "issues-diff", "fixed-diff", "attachment-sha256", "transcript-jsonl-line"
    Content string // raw bytes for the report
}

// Registry returns the 15 scenarios in order.
func Registry() []Scenario {
    return []Scenario{
        {ID: "S01", Name: "plain-greeting-query-fallthrough", Run: runS01},
        {ID: "S02", Name: "help-fastpath", Run: runS02},
        {ID: "S03", Name: "status-fastpath", Run: runS03},
        {ID: "S04", Name: "continue-fastpath", Run: runS04},
        {ID: "S05", Name: "bug-prefix-cc-issue-open", Run: runS05, RequiresOperatorRole: true},
        {ID: "S06", Name: "task-prefix-cc-issue-open", Run: runS06, RequiresOperatorRole: true},
        {ID: "S07", Name: "query-prefix-cc-research", Run: runS07},
        {ID: "S08", Name: "done-operator-migrate", Run: runS08, RequiresOperatorRole: true},
        {ID: "S09", Name: "done-non-operator-reject", Run: runS09, RequiresNonOperatorBot: true},
        {ID: "S10", Name: "reopen-operator-migrate", Run: runS10, RequiresOperatorRole: true},
        {ID: "S11", Name: "inbound-photo-bug-caption", Run: runS11, RequiresOperatorRole: true},
        {ID: "S12", Name: "inbound-document-task-caption", Run: runS12, RequiresOperatorRole: true},
        {ID: "S13", Name: "inbound-voice-audio", Run: runS13},
        {ID: "S14", Name: "outbound-attachment-fanout", Run: runS14},
        {ID: "S15", Name: "natural-language-emoji-fallthrough", Run: runS15},
    }
}
```

### Helpers (in `lifecycle/helpers.go`)

```go
// awaitReplyWithSubstring posts text via msgr.Send, then waits for a reply from
// pherald-bot whose text contains the substring. Returns the captured Reply.
func awaitReplyWithSubstring(ctx context.Context, env *Env, text string, substr string) (messenger.Reply, messenger.MessageID, error)

// awaitClassification reads pherald's transcript.jsonl (env.PheraldQADir) and
// blocks until a line appears with `classification == expected` and
// `inbound.message_id == myMsgID`. Returns the matching JSON line.
func awaitClassification(ctx context.Context, env *Env, myMsgID messenger.MessageID, expected string) ([]byte, error)

// snapshotIssuesFixed reads docs/Issues.md + docs/Fixed.md and returns their
// content for before/after diffing.
func snapshotIssuesFixed(env *Env) (issues, fixed []byte, err error)

// assertIssuesDelta diffs the snapshots and asserts a +N or -N row delta in
// the matching file. Returns the diff hunk as evidence on success.
func assertIssuesDelta(before, after []byte, expectedDelta int) (hunk string, err error)

// extractHRDID parses "Opened HRD-NNN ..." from a reply and returns "HRD-NNN".
func extractHRDID(replyText string) (string, error)
```

### Example scenario implementation — `s01_greeting.go`

```go
package lifecycle

import (
    "context"
    "fmt"
    "time"
)

func runS01(ctx context.Context, env *Env) Result {
    r := Result{ScenarioID: "S01", StartedAt: time.Now()}
    defer func() { r.Duration = time.Since(r.StartedAt) }()

    const input = "Hello, can you help with debugging?"

    issuesBefore, fixedBefore, err := snapshotIssuesFixed(env)
    if err != nil {
        r.FailureReason = fmt.Sprintf("snapshot: %v", err)
        return r
    }

    inID, err := env.Msgr.Send(ctx, input)
    if err != nil {
        r.FailureReason = fmt.Sprintf("send: %v", err)
        return r
    }
    r.InboundMessageID = inID

    reply, _, err := awaitReplyWithSubstring(ctx, env, "", "")
    _ = reply // S01 just asserts SOME reply arrives within timeout
    if err != nil {
        r.FailureReason = fmt.Sprintf("await-reply: %v", err)
        return r
    }
    r.ReplyMessageID = reply.MessageID

    cline, err := awaitClassification(ctx, env, inID, "query")
    if err != nil {
        r.FailureReason = fmt.Sprintf("await-classification: %v", err)
        return r
    }
    r.ClassificationSeen = "query"
    r.ActionSeen = "cc.dispatch"

    issuesAfter, fixedAfter, _ := snapshotIssuesFixed(env)
    if _, err := assertIssuesDelta(issuesBefore, issuesAfter, 0); err != nil {
        r.FailureReason = "Issues.md mutated on a fallthrough query"
        return r
    }
    if _, err := assertIssuesDelta(fixedBefore, fixedAfter, 0); err != nil {
        r.FailureReason = "Fixed.md mutated on a fallthrough query"
        return r
    }

    r.PASS = true
    r.Evidence = []EvidenceFragment{
        {Kind: "reply-text", Content: reply.Text},
        {Kind: "transcript-jsonl-line", Content: string(cline)},
    }
    return r
}
```

The remaining 14 scenarios follow the same shape — input via `env.Msgr.Send` (or `SendPhoto`/`SendDocument`/`SendVoice`), `awaitReplyWithSubstring`, `awaitClassification` with the scenario's expected classification, optional `assertIssuesDelta` for fs-mutation scenarios.

### Scenario-specific notes

- **S05/S06:** After PASS, capture the HRD-NNN from the reply text — needed by S08/S10.
- **S08:** Looks up the latest HRD-NNN created by S05/S06 (passed via `env.LastOpenedHRD` set by the orchestrator).
- **S09:** Uses `env.MsgrNonOp.Send(...)`. If `MsgrNonOp` is nil → returns Result{PASS: true, SkippedReason: "HERALD_QA_BOT_TOKEN_NON_OPERATOR unset"} per §11.4.5 SKIP-with-reason.
- **S11:** Sends a fixture photo `qaherald/internal/lifecycle/fixtures/s11_redbanner.jpg` (committed; 4KB). Asserts `attachments/<sha256>.jpg` appears in `env.OutDir/attachments/`.
- **S12:** Sends `qaherald/internal/lifecycle/fixtures/s12_spec.pdf` (committed; ~10KB).
- **S13:** Sends `qaherald/internal/lifecycle/fixtures/s13_voice.ogg` (committed; ~5KB ogg/opus).
- **S14:** Posts `Status:` then asserts the reply's `Attachments[0].ContentType` is image/png AND its sha256 matches `assets/logo/herald_logo_square_128.png`. This proves pherald's outbound fan-out wired attachments correctly.

### Implementation steps

- [ ] Subagent creates `qaherald/internal/lifecycle/scenario.go` (types + Registry).
- [ ] Subagent creates `qaherald/internal/lifecycle/helpers.go` (5 helpers).
- [ ] Subagent creates 15 files `s01_greeting.go` ... `s15_emoji.go`, one func per file.
- [ ] Subagent commits 3 fixture binaries under `qaherald/internal/lifecycle/fixtures/` (jpg + pdf + ogg).
- [ ] Subagent runs `go build ./qaherald/internal/lifecycle/...` — clean compile.
- [ ] Subagent runs `go vet ./qaherald/internal/lifecycle/...` — clean.

### Self-review

- [ ] All 15 scenarios in Registry, in S01..S15 order.
- [ ] Each scenario uses `MessengerClient` (NOT direct telebot calls).
- [ ] Each scenario reads `env.PheraldQADir/transcript.jsonl` for classification assertion.
- [ ] Each fs-mutation scenario diffs Issues.md / Fixed.md before/after.
- [ ] S09 properly SKIPs when `MsgrNonOp` is nil.
- [ ] Fixtures are committed and content-addressed identifiable.
- [ ] No scenario depends on `qaherald/internal/scenario/` (the Wave 5 package) — lifecycle is independent.

### Commit

```
qaherald lifecycle T3: lifecycle/ package + 15 scenario implementations

Scenario type + Registry + 5 helpers + 15 Go-func scenarios mapping
S1..S15 from tests/test_wave6.5_lifecycle.sh. Each scenario uses the T2
MessengerClient interface, asserts pherald's reply, reads pherald's own
transcript.jsonl for classification confirmation, and diffs Issues.md /
Fixed.md for fs-mutation evidence. 3 fixture files committed (jpg, pdf,
ogg) for S11/S12/S13.

Closes HRD-101 T3.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
```

---

## Task 4: `qaherald/cmd/qaherald/lifecycle.go` orchestrator — `lifecycle.Run`

**Subagent dispatch:** Fresh subagent for T4.

### Goal

Tie T1 (Cobra wiring), T2 (MessengerClient), T3 (scenarios) together via an orchestrator that:

1. Constructs the MessengerClient(s) from the resolved config.
2. Initializes the transcript writer.
3. Runs preflight (T6 — depends on T6 placeholder; T6 fills it).
4. Iterates the Registry, filters by `--scenarios` flag, runs each, captures the Result, writes to transcript.
5. Generates the Markdown report (T5).
6. Exits non-zero if any scenario FAILed.

### Files

`qaherald/internal/lifecycle/lifecycle.go`:

```go
package lifecycle

import (
    "context"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "strings"
    "time"

    "qaherald/internal/messenger"
    "qaherald/internal/transcript"
)

type Config struct {
    QABotToken         string
    QABotTokenNonOp    string
    ChatID             int64
    PheraldBotUsername string
    OutDir             string
    RunID              string
    DocsDir            string
    PheraldQAOutDir    string
    Scenarios          []string
    PerScenarioTimeout time.Duration
    SkipPreflight      bool
}

// Run executes the lifecycle. Returns nil if every scheduled scenario PASSed.
func Run(ctx context.Context, cfg Config) error {
    if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
        return fmt.Errorf("mkdir out-dir: %w", err)
    }

    msgr, err := buildMessenger(cfg.QABotToken, cfg.ChatID)
    if err != nil {
        return fmt.Errorf("build qa-bot client: %w", err)
    }

    var msgrNonOp messenger.MessengerClient
    if cfg.QABotTokenNonOp != "" {
        msgrNonOp, err = buildMessenger(cfg.QABotTokenNonOp, cfg.ChatID)
        if err != nil {
            return fmt.Errorf("build qa-bot-non-op client: %w", err)
        }
    }

    tw, err := transcript.NewWriter(filepath.Join(cfg.OutDir, "transcript.jsonl"), filepath.Join(cfg.OutDir, "attachments"))
    if err != nil {
        return fmt.Errorf("transcript writer: %w", err)
    }
    defer tw.Close()

    if !cfg.SkipPreflight {
        if err := runPreflight(ctx, msgr, msgrNonOp, cfg); err != nil {
            return fmt.Errorf("preflight: %w", err)
        }
    }

    env := &Env{
        Msgr:           msgr,
        MsgrNonOp:      msgrNonOp,
        Transcript:     tw,
        PheraldBotUser: cfg.PheraldBotUsername,
        DocsDir:        cfg.DocsDir,
        PheraldQADir:   cfg.PheraldQAOutDir,
        ChatID:         cfg.ChatID,
        PerTimeout:     cfg.PerScenarioTimeout,
    }

    scenarios := Registry()
    if len(cfg.Scenarios) > 0 {
        scenarios = filterScenarios(scenarios, cfg.Scenarios)
    }

    results := make([]Result, 0, len(scenarios))
    for _, s := range scenarios {
        sctx, cancel := context.WithTimeout(ctx, cfg.PerScenarioTimeout)
        if s.RequiresNonOperatorBot && msgrNonOp == nil {
            results = append(results, Result{
                ScenarioID:    s.ID,
                PASS:          true,
                FailureReason: "SKIP: HERALD_QA_BOT_TOKEN_NON_OPERATOR unset (§11.4.5)",
            })
            cancel()
            continue
        }
        fmt.Fprintf(os.Stderr, "[lifecycle] running %s — %s\n", s.ID, s.Name)
        result := s.Run(sctx, env)
        cancel()
        results = append(results, result)
        _ = tw.AppendResult(toTranscriptEvent(s, result))
    }

    if err := writeReport(filepath.Join(cfg.OutDir, "report.md"), cfg.RunID, results); err != nil {
        return fmt.Errorf("write report: %w", err)
    }

    failed := 0
    for _, r := range results {
        if !r.PASS {
            failed++
        }
    }
    if failed > 0 {
        return fmt.Errorf("%d/%d scenarios FAILed — see %s/report.md", failed, len(results), cfg.OutDir)
    }
    return nil
}

func filterScenarios(all []Scenario, subset []string) []Scenario {
    keep := map[string]bool{}
    for _, s := range subset {
        keep[strings.ToUpper(strings.TrimSpace(s))] = true
    }
    out := make([]Scenario, 0, len(all))
    for _, s := range all {
        if keep[s.ID] {
            out = append(out, s)
        }
    }
    return out
}

func toTranscriptEvent(s Scenario, r Result) transcript.Event { /* ... */ }
func buildMessenger(token string, chatID int64) (messenger.MessengerClient, error) { /* uses internal/tgram + internal/messenger */ }
func runPreflight(ctx context.Context, msgr, msgrNonOp messenger.MessengerClient, cfg Config) error { /* delegates to T6 preflight.go */ }
func writeReport(path, runID string, results []Result) error { /* delegates to T5 report.go */ }

// GenerateRunID — exported, used by Cobra command.
func GenerateRunID() string {
    return time.Now().UTC().Format("2006-01-02T15-04-05") + "-" + shortUUID()
}

func PrintScenariosOnly(out io.Writer) error {
    for _, s := range Registry() {
        fmt.Fprintf(out, "%s\t%s\n", s.ID, s.Name)
    }
    return nil
}
```

### Implementation steps

- [ ] Subagent creates `qaherald/internal/lifecycle/lifecycle.go` (above).
- [ ] Subagent creates `qaherald/internal/lifecycle/build.go` with `buildMessenger` (constructs `tgram.Client` from token, wraps in `messenger.TelegramClient`).
- [ ] Subagent creates `qaherald/internal/lifecycle/shortid.go` with UUIDv7 short-form helper (4 hex chars).
- [ ] Subagent runs `go build ./qaherald/...` — clean.
- [ ] Subagent runs `/tmp/qaherald lifecycle --skip-preflight --qa-bot-token=fake --chat-id=1 --pherald-bot-username=u --scenarios=S01 --out=/tmp/qa-test` — should compile and run, fail with messenger network error (expected; no real token).

### Self-review

- [ ] `Run` is the single entry-point from Cobra.
- [ ] `runPreflight` is a placeholder until T6.
- [ ] `writeReport` is a placeholder until T5.
- [ ] Filter + SKIP logic is correct.
- [ ] Per-scenario timeout context is cancelled in defer.
- [ ] Exits non-zero when any scenario FAILed.

### Commit

```
qaherald lifecycle T4: orchestrator (lifecycle.Run) + Cobra wire-up

Orchestrator constructs MessengerClient from T2, opens transcript writer
from Wave 5 internal/transcript/, iterates Registry, runs scenarios
serially with per-scenario timeout context, captures Result, writes
transcript event, generates report (T5 placeholder), exits non-zero on
any FAIL. --scenarios=S01,S02 filters subset.

Closes HRD-101 T4.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
```

---

## Task 5: `qaherald/internal/lifecycle/report.go` — Markdown report generator

**Subagent dispatch:** Fresh subagent for T5.

### Goal

Generate `docs/qa/HRD-101-lifecycle-<run-id>/report.md` per the §107 anti-bluff requirement: every PASS line cites (a) inbound message_id, (b) outbound reply_to_message_id, (c) fs-mutation diff hunk where applicable.

### Files

`qaherald/internal/lifecycle/report.go`:

```go
package lifecycle

import (
    "fmt"
    "io"
    "os"
    "strings"
    "time"
)

func writeReport(path, runID string, results []Result) error {
    f, err := os.Create(path)
    if err != nil {
        return err
    }
    defer f.Close()

    fmt.Fprintf(f, "# Lifecycle Report — run-id `%s`\n\n", runID)
    fmt.Fprintf(f, "Generated: %s\n\n", time.Now().UTC().Format(time.RFC3339))

    var passN, failN, skipN int
    for _, r := range results {
        switch {
        case strings.HasPrefix(r.FailureReason, "SKIP:"):
            skipN++
        case r.PASS:
            passN++
        default:
            failN++
        }
    }
    fmt.Fprintf(f, "**Summary:** %d PASS / %d FAIL / %d SKIP (of %d scenarios)\n\n", passN, failN, skipN, len(results))

    fmt.Fprintf(f, "## Per-Scenario Detail\n\n")
    for _, r := range results {
        writeScenarioSection(f, r)
    }

    writeAggregate(f, results)
    return nil
}

func writeScenarioSection(w io.Writer, r Result) {
    status := "PASS"
    if !r.PASS && strings.HasPrefix(r.FailureReason, "SKIP:") {
        status = "SKIP"
    } else if !r.PASS {
        status = "FAIL"
    }

    fmt.Fprintf(w, "### %s — %s\n\n", r.ScenarioID, status)
    fmt.Fprintf(w, "- **Duration:** %s\n", r.Duration)
    fmt.Fprintf(w, "- **Inbound message_id:** `%s`\n", r.InboundMessageID)
    fmt.Fprintf(w, "- **Reply message_id:** `%s`\n", r.ReplyMessageID)
    fmt.Fprintf(w, "- **Classification:** `%s`\n", r.ClassificationSeen)
    fmt.Fprintf(w, "- **Action:** `%s`\n", r.ActionSeen)
    if r.FailureReason != "" {
        fmt.Fprintf(w, "- **Reason:** %s\n", r.FailureReason)
    }
    if len(r.Evidence) > 0 {
        fmt.Fprintf(w, "\n#### Evidence\n\n")
        for _, e := range r.Evidence {
            fmt.Fprintf(w, "**%s:**\n\n```\n%s\n```\n\n", e.Kind, truncate(e.Content, 2000))
        }
    }
    fmt.Fprintln(w)
}

func writeAggregate(w io.Writer, results []Result) {
    fmt.Fprintf(w, "## Aggregate\n\n")
    fmt.Fprintf(w, "| Scenario | Status | Duration | Inbound | Reply |\n")
    fmt.Fprintf(w, "|---|---|---|---|---|\n")
    for _, r := range results {
        status := "PASS"
        if !r.PASS && strings.HasPrefix(r.FailureReason, "SKIP:") {
            status = "SKIP"
        } else if !r.PASS {
            status = "FAIL"
        }
        fmt.Fprintf(w, "| %s | %s | %s | `%s` | `%s` |\n", r.ScenarioID, status, r.Duration, r.InboundMessageID, r.ReplyMessageID)
    }
}

func truncate(s string, n int) string {
    if len(s) <= n {
        return s
    }
    return s[:n] + "... (truncated)"
}
```

### Implementation steps

- [ ] Subagent creates `qaherald/internal/lifecycle/report.go` (above).
- [ ] Subagent removes the placeholder `writeReport` from `lifecycle.go` (T4) and uses this real one.
- [ ] Subagent runs `go build ./qaherald/...` — clean.

### Self-review

- [ ] Every scenario gets a `### S0X — STATUS` section.
- [ ] PASS sections include all required evidence kinds.
- [ ] FAIL sections include `FailureReason`.
- [ ] SKIP is distinct from FAIL in the summary count.
- [ ] Aggregate table at the bottom lists all scenarios at a glance.

### Commit

```
qaherald lifecycle T5: Markdown report generator with §107 evidence

Per-scenario sections cite inbound message_id + reply message_id +
classification + action + evidence fragments (reply text, transcript
JSONL line, fs-diff hunk). Aggregate table at the bottom. PASS/FAIL/SKIP
counts distinct.

Closes HRD-101 T5.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
```

---

## Task 6: Pre-flight env validator

**Subagent dispatch:** Fresh subagent for T6.

### Goal

`qaherald lifecycle` REFUSES to start when any precondition is missing. No silent degradation. Failures point to `docs/guides/OPERATOR_CREDENTIALS.md`.

### Validator gates

| Gate | Check | Exit code | Failure message |
|---|---|---|---|
| G1 | pherald listen reachable | open TCP connect to pherald's OTel port (default `7080`) OR call `getMe` against `HERALD_PHERALD_BOT_USERNAME` via the qa-bot token and assert the bot exists | 2 | "pherald listen not running OR pherald-bot username mismatch" |
| G2 | qa-bot token works | qa-bot `getMe` returns 200 + non-empty username | 2 | "HERALD_QA_BOT_TOKEN invalid (getMe failed)" |
| G3 | qa-bot privacy disabled | qa-bot's `getMe.can_read_all_group_messages == true` | 3 | "qa-bot Privacy Mode is enabled — @BotFather → /setprivacy → Disable" |
| G4 | qa-bot in chat | qa-bot `getChat(chatID)` returns 200 + type=`group`/`supergroup` | 4 | "qa-bot is not a member of HERALD_TGRAM_CHAT_ID" |
| G5 | qa-bot ≠ pherald-bot | qa-bot's username MUST differ from `--pherald-bot-username` | 5 | "qa-bot username matches pherald-bot — wrong token" |
| G6 | tokens distinct | `HERALD_QA_BOT_TOKEN != HERALD_TGRAM_BOT_TOKEN` (if both present in env) | 5 | "qa-bot token equals pherald-bot token — use distinct bots" |
| G7 | docs dirs exist | `--docs-dir/Issues.md` AND `--docs-dir/Fixed.md` exist | 6 | "Issues.md or Fixed.md not found under --docs-dir" |
| G8 | pherald-qa-out-dir exists | `--pherald-qa-out-dir/transcript.jsonl` exists OR can be created | 6 | "pherald listen is not configured to write transcript to --pherald-qa-out-dir" |
| G9 | qa-bot's user-id resolved | call `getMe` once and cache the user-id for the report attribution; record for transcript | (no fail — informational) | n/a |
| G10 | non-op-bot distinct | if `--qa-bot-token-non-operator` present, its user-id MUST differ from main qa-bot's user-id; AND its user-id MUST NOT appear in `HERALD_OPERATOR_IDS` | 7 | "non-operator qa-bot user-id is in HERALD_OPERATOR_IDS — defeats the purpose of S9" |

### Files

`qaherald/internal/lifecycle/preflight.go`:

```go
package lifecycle

import (
    "context"
    "fmt"
    "net"
    "os"
    "strconv"
    "strings"
    "time"

    "qaherald/internal/messenger"
)

type PreflightError struct {
    Gate     string
    ExitCode int
    Reason   string
}

func (e *PreflightError) Error() string {
    return fmt.Sprintf("[%s] %s (exit %d) — see docs/guides/OPERATOR_CREDENTIALS.md", e.Gate, e.Reason, e.ExitCode)
}

func runPreflight(ctx context.Context, msgr, msgrNonOp messenger.MessengerClient, cfg Config) error {
    // G2 — qa-bot getMe
    rep, err := msgr.Preflight(ctx, cfg.ChatID)
    if err != nil {
        return &PreflightError{Gate: "G2", ExitCode: 2, Reason: fmt.Sprintf("qa-bot getMe failed: %v", err)}
    }

    // G5 — qa-bot ≠ pherald-bot
    if strings.EqualFold(rep.Username, cfg.PheraldBotUsername) {
        return &PreflightError{Gate: "G5", ExitCode: 5, Reason: fmt.Sprintf("qa-bot username (%s) equals pherald-bot username", rep.Username)}
    }

    // G6 — tokens distinct (if both env vars present)
    if os.Getenv("HERALD_TGRAM_BOT_TOKEN") == cfg.QABotToken && cfg.QABotToken != "" {
        return &PreflightError{Gate: "G6", ExitCode: 5, Reason: "HERALD_QA_BOT_TOKEN equals HERALD_TGRAM_BOT_TOKEN"}
    }

    // G3 — qa-bot privacy disabled
    if !rep.CanReadAllGroupMessages {
        return &PreflightError{Gate: "G3", ExitCode: 3, Reason: "qa-bot Privacy Mode is enabled — talk to @BotFather → /setprivacy → Disable"}
    }

    // G4 — qa-bot in chat
    if !rep.InChat {
        return &PreflightError{Gate: "G4", ExitCode: 4, Reason: fmt.Sprintf("qa-bot is not a member of chat-id %d", cfg.ChatID)}
    }
    if rep.ChatType != "group" && rep.ChatType != "supergroup" {
        return &PreflightError{Gate: "G4", ExitCode: 4, Reason: fmt.Sprintf("chat-id %d is type %q; expected group or supergroup", cfg.ChatID, rep.ChatType)}
    }

    // G7 — docs files exist
    for _, p := range []string{"Issues.md", "Fixed.md"} {
        if _, err := os.Stat(cfg.DocsDir + "/" + p); err != nil {
            return &PreflightError{Gate: "G7", ExitCode: 6, Reason: fmt.Sprintf("%s/%s not found", cfg.DocsDir, p)}
        }
    }

    // G8 — pherald-qa-out-dir writable
    if cfg.PheraldQAOutDir != "" {
        if _, err := os.Stat(cfg.PheraldQAOutDir); err != nil {
            return &PreflightError{Gate: "G8", ExitCode: 6, Reason: fmt.Sprintf("--pherald-qa-out-dir %s does not exist", cfg.PheraldQAOutDir)}
        }
    }

    // G1 — pherald-bot identity (assert by checking that the username in --pherald-bot-username
    // resolves via getChat or getChatMember; we already have rep.PheraldBotPresent from Preflight).
    if !rep.PheraldBotPresent {
        return &PreflightError{Gate: "G1", ExitCode: 2, Reason: fmt.Sprintf("pherald-bot username %q not found in chat", cfg.PheraldBotUsername)}
    }

    // G1.5 — best-effort OTel port liveness (informational; only FAIL if env explicitly sets HERALD_OTEL_PORT)
    if portStr := os.Getenv("HERALD_OTEL_PORT"); portStr != "" {
        port, _ := strconv.Atoi(portStr)
        conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 2*time.Second)
        if err != nil {
            return &PreflightError{Gate: "G1", ExitCode: 2, Reason: fmt.Sprintf("pherald OTel port %d unreachable: %v", port, err)}
        }
        conn.Close()
    }

    // G10 — non-op bot distinctness (if supplied)
    if msgrNonOp != nil {
        repNon, err := msgrNonOp.Preflight(ctx, cfg.ChatID)
        if err != nil {
            return &PreflightError{Gate: "G10", ExitCode: 7, Reason: fmt.Sprintf("non-operator qa-bot getMe failed: %v", err)}
        }
        if repNon.UserID == rep.UserID {
            return &PreflightError{Gate: "G10", ExitCode: 7, Reason: "non-operator qa-bot has same user-id as main qa-bot"}
        }
        // Operator-id allowlist check: read HERALD_OPERATOR_IDS, fail if non-op user-id is in it.
        oplist := os.Getenv("HERALD_OPERATOR_IDS")
        for _, s := range strings.Split(oplist, ",") {
            if strings.TrimSpace(s) == strconv.FormatInt(repNon.UserID, 10) {
                return &PreflightError{Gate: "G10", ExitCode: 7, Reason: fmt.Sprintf("non-operator qa-bot user-id %d is in HERALD_OPERATOR_IDS — defeats S9", repNon.UserID)}
            }
        }
    }

    return nil
}
```

### Implementation steps

- [ ] Subagent creates `qaherald/internal/lifecycle/preflight.go` (above).
- [ ] Subagent ensures G1..G10 all wired.
- [ ] Subagent runs `go build ./qaherald/...` — clean.

### Self-review

- [ ] Every gate has explicit `ExitCode` for shell-script consumption.
- [ ] Every failure message references `docs/guides/OPERATOR_CREDENTIALS.md`.
- [ ] G6 only triggers when BOTH tokens are present.
- [ ] G10 only triggers when `--qa-bot-token-non-operator` supplied.
- [ ] No silent degradation.

### Commit

```
qaherald lifecycle T6: pre-flight validator with 10 gates

Validator REFUSES to start without all preconditions green: qa-bot
getMe, Privacy Mode disabled, qa-bot in chat (group/supergroup), tokens
distinct from pherald-bot, Issues.md / Fixed.md present, pherald
listening on OTel port (if HERALD_OTEL_PORT set), non-operator qa-bot
distinct from main + not in HERALD_OPERATOR_IDS. Each gate carries a
distinct exit code (2..7) for shell-script consumption.

Closes HRD-101 T6.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
```

---

## Task 7: Unit tests — hermetic httptest impersonator

**Subagent dispatch:** Fresh subagent for T7.

### Goal

Cover the orchestrator end-to-end without crossing the network. Use `httptest.Server` to impersonate api.telegram.org + stub pherald's transcript.jsonl file. Stub `MessengerClient` for scenarios that need deterministic verification.

### Files

`qaherald/internal/lifecycle/lifecycle_test.go`:

Test cases (5–8):

1. **TestLifecycleRun_S01_HappyPath_PASS** — stub messenger returns the expected reply on `WaitForReply`; stub pherald transcript.jsonl already contains the `query` classification line; assert `Result.PASS=true`, `Result.ClassificationSeen="query"`.

2. **TestLifecycleRun_S01_MissingReply_FAIL** — stub messenger times out on `WaitForReply`; assert `Result.PASS=false`, `Result.FailureReason` contains "context deadline exceeded".

3. **TestLifecycleRun_S05_BugPrefix_ClassificationMismatch_FAIL** — stub messenger returns a reply but pherald's transcript.jsonl says classification is `query` not `command_prefix:bug`; assert FAIL with reason containing "expected command_prefix:bug".

4. **TestLifecycleRun_S05_BugPrefix_NoIssuesMutation_FAIL** — reply arrives + classification is correct, but `docs/Issues.md` snapshot delta is 0 (expected +1); assert FAIL with reason "Issues.md +1 row expected".

5. **TestLifecycleRun_S08_DoneOperatorMigrate_FixedDelta_PASS** — full pipeline; Issues.md −1, Fixed.md +1; assert PASS + evidence contains both diff hunks.

6. **TestLifecycleRun_S09_NonOpRejection_SKIP_WhenNoNonOpToken** — `MsgrNonOp=nil`; assert `Result.PASS=true`, FailureReason starts with "SKIP:".

7. **TestLifecycleRun_Preflight_PrivacyEnabled_FAIL** — stub `Preflight` returns `CanReadAllGroupMessages=false`; assert error message is `PreflightError` with Gate=G3, ExitCode=3.

8. **TestLifecycleRun_FilterScenarios** — `cfg.Scenarios=["S01","S03"]`; assert only those two are executed.

### Stub MessengerClient

`qaherald/internal/lifecycle/stub_test.go`:

```go
package lifecycle

import (
    "context"
    "io"
    "strings"
    "time"

    "qaherald/internal/messenger"
)

type StubMessenger struct {
    SendFn         func(context.Context, string) (messenger.MessageID, error)
    WaitFn         func(context.Context, messenger.MessageID, messenger.Predicate, time.Duration) (messenger.Reply, error)
    PreflightFn    func(context.Context, int64) (messenger.PreflightReport, error)
    SendPhotoFn    func(context.Context, string, string) (messenger.MessageID, error)
    SendDocFn      func(context.Context, string, string) (messenger.MessageID, error)
    SendVoiceFn    func(context.Context, string) (messenger.MessageID, error)
    DownloadFn     func(context.Context, string) (io.ReadCloser, error)
    GetUpdatesFn   func(context.Context, int64) ([]messenger.Reply, int64, error)
    MeFn           func() (string, int64, error)
}

func (s *StubMessenger) Me() (string, int64, error) {
    if s.MeFn == nil { return "qa_bot", 999, nil }
    return s.MeFn()
}
func (s *StubMessenger) Send(ctx context.Context, t string) (messenger.MessageID, error) {
    if s.SendFn == nil { return "1", nil }
    return s.SendFn(ctx, t)
}
// ... rest of the interface delegations
```

### Stub pherald transcript

Create a per-test `t.TempDir()` containing a `transcript.jsonl` with the exact lines the scenario should observe:

```jsonl
{"ts":"2026-05-23T14:22:01Z","direction":"inbound","kind":"telegram-message","payload":{"message_id":1,"text":"Hello, can you help with debugging?"},"classification":"query","action":"cc.dispatch"}
```

`awaitClassification` reads this file via a polling loop with deadline; tests inject a pre-populated file so the polling sees it immediately.

### Implementation steps

- [ ] Subagent creates `qaherald/internal/lifecycle/stub_test.go`.
- [ ] Subagent creates `qaherald/internal/lifecycle/lifecycle_test.go` with 8 test cases.
- [ ] Subagent runs `go test ./qaherald/internal/lifecycle/... -race -count=1` — all PASS.
- [ ] Subagent runs `go test ./qaherald/internal/lifecycle/... -cover` — coverage ≥ 70% on the lifecycle package.

### Self-review

- [ ] All 8 cases run hermetically — no network, no filesystem outside `t.TempDir()`.
- [ ] Stub honors the full `MessengerClient` interface (compiles).
- [ ] An impl that fakes message_ids without crossing the network FAILs the unit tests because the stub `httptest` server is what defines truth — covered implicitly via T2's tests.
- [ ] No test calls `t.Skip` for "not implemented" — every case has a real assertion.

### Commit

```
qaherald lifecycle T7: hermetic unit tests with stub messenger + temp dirs

8 test cases — happy path + missing-reply + wrong-classification +
no-fs-mutation + Done: migration + S9 SKIP + preflight G3 fail +
scenario filtering. Stub MessengerClient, pre-populated pherald
transcript.jsonl in t.TempDir(). Run with -race -count=1, all PASS.

Closes HRD-101 T7.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
```

---

## Task 8: Update `tests/test_wave6.5_lifecycle.sh` to delegate + tag v0.5.1

**Subagent dispatch:** Fresh subagent for T8. T8 is the wrap-up — final integration + docs + tag.

### Goal

1. Repoint `tests/test_wave6.5_lifecycle.sh` to delegate to `qaherald lifecycle` for automated runs. Keep `--manual` flag for the old prompt-based UX (operator can still drive manually).
2. Tag `v0.5.1` after the lifecycle commits land.

### Shell script changes

`tests/test_wave6.5_lifecycle.sh` — replace the 15-scenario `narrate_scenario` body with:

```bash
# --- Automated mode (default) ---
if [ "${HERALD_LIFECYCLE_MANUAL:-0}" != "1" ]; then
  echo "[w6.5-lifecycle] automated mode — delegating to qaherald lifecycle"

  # Pre-flight: qaherald binary must exist.
  if ! [ -x /tmp/qaherald ]; then
    echo "[w6.5-lifecycle] building qaherald → /tmp/qaherald ..."
    go build -o /tmp/qaherald ./qaherald/cmd/qaherald
  fi

  # Pre-flight: HERALD_QA_BOT_TOKEN required for automated mode.
  if [ -z "${HERALD_QA_BOT_TOKEN:-}" ]; then
    skip_with_reason "HERALD_QA_BOT_TOKEN unset — automated mode requires a 2nd Telegram bot. Set HERALD_LIFECYCLE_MANUAL=1 to run prompt-based mode."
  fi

  PHERALD_BOT_USERNAME="${HERALD_PHERALD_BOT_USERNAME:-atmosphere_worker_bot}"

  /tmp/qaherald lifecycle \
    --qa-bot-token="${HERALD_QA_BOT_TOKEN}" \
    --qa-bot-token-non-operator="${HERALD_QA_BOT_TOKEN_NON_OPERATOR:-}" \
    --chat-id="${HERALD_TGRAM_CHAT_ID}" \
    --pherald-bot-username="${PHERALD_BOT_USERNAME}" \
    --out="${QA_OUT}" \
    --run-id="${HERALD_QA_RUN_ID}" \
    --docs-dir=docs \
    --pherald-qa-out-dir="${QA_OUT}" \
    --scenario-timeout=120s \
    --overall-timeout=45m

  AUTO_EXIT=$?
  echo "[w6.5-lifecycle] qaherald lifecycle exit code: ${AUTO_EXIT}"
  if [ ${AUTO_EXIT} -ne 0 ]; then
    echo "FAIL: qaherald lifecycle reported failures — see ${QA_OUT}/report.md"
    exit 1
  fi
  echo "PASS: lifecycle 15/15 scenarios — evidence in ${QA_OUT}/"
  exit 0
fi

# --- Manual mode (HERALD_LIFECYCLE_MANUAL=1) ---
# (existing prompt-based body kept verbatim below)
echo "[w6.5-lifecycle] manual mode — operator drives each scenario"
# ... rest of the script as before
```

### Tag v0.5.1

After T8 lands and PASSes the gate:

```bash
git tag -a v0.5.1 -m "qaherald-auto: 15-scenario automated lifecycle"
# DO NOT push — operator does this manually after review
```

### Implementation steps

- [ ] Subagent edits `tests/test_wave6.5_lifecycle.sh` per the spec above.
- [ ] Subagent verifies the script still passes `shellcheck`.
- [ ] Subagent runs the shell script with `HERALD_LIFECYCLE_MANUAL=1` to confirm manual path still works (operator prompt sequence intact).
- [ ] Subagent runs the shell script in automated mode IF `HERALD_QA_BOT_TOKEN` is set (will be a live test — needs operator credentials).
- [ ] Subagent updates `docs/Issues.md`: closes HRD-101 entries T1..T8.
- [ ] Subagent updates `docs/Fixed.md`: adds 8 closed-HRD rows with the same scenario tag.
- [ ] Subagent bumps `CLAUDE.md` revision (r8 → r9).
- [ ] Subagent updates `docs/CONTINUATION.md` with the new live-test prompt referencing `HERALD_QA_BOT_TOKEN`.
- [ ] Subagent updates `docs/guides/OPERATOR_CREDENTIALS.md` with the 2nd-bot setup section (BotFather steps for `/setprivacy → Disable`).
- [ ] Subagent runs `scripts/audit_antibluff.sh`, `scripts/codegraph_validate.sh`, `scripts/e2e_bluff_hunt.sh` — all PASS.
- [ ] Subagent commits the lot.
- [ ] Subagent creates `git tag -a v0.5.1 -m "..."`. DOES NOT push (operator decides).

### Self-review

- [ ] Manual mode still works (`HERALD_LIFECYCLE_MANUAL=1`).
- [ ] Automated mode SKIPs cleanly when token missing.
- [ ] HRD-101 entries closed in Issues.md.
- [ ] CLAUDE.md r-bump matches the workspace state.
- [ ] OPERATOR_CREDENTIALS.md has the BotFather privacy-disable steps.
- [ ] Tag `v0.5.1` exists locally but is NOT pushed.

### Commit

```
qaherald lifecycle T8: shell script delegates to qaherald lifecycle + tag v0.5.1

tests/test_wave6.5_lifecycle.sh now delegates to `qaherald lifecycle` by
default; HERALD_LIFECYCLE_MANUAL=1 falls back to the prompt-based UX.
HRD-101 closed; CLAUDE.md r8 → r9; OPERATOR_CREDENTIALS.md gains the
BotFather privacy-disable section. Tag v0.5.1 created locally (operator
pushes).

Closes HRD-101 T8 — wave complete.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
```

---

## Cross-cutting: anti-bluff invariants

Every task above carries a §11.4 / §107 anchor. Recap, for the wave-completion audit:

| Invariant | Anchor | Asserted by |
|---|---|---|
| Real-network bytes only | §11.4.2 | T2 unit tests (httptest) + T8 live run |
| message_id chain proof | §107 | T5 report.md cites inbound + reply_to_message_id per scenario |
| fs-mutation diff | §107.x | T3 helpers + T5 evidence fragments |
| Pre-flight strictness | §11.4.5 | T6 — 10 distinct exit codes, no silent degradation |
| Working-tree quiescence | §107.y | T8 — `scripts/audit_antibluff.sh` must PASS before tag |
| docs/qa evidence mandate | §107.x | T8 — `docs/qa/HRD-101-lifecycle-<run-id>/` committed by live run |
| Test-interrupt on discovery | §11.4.4 | Any T-task FAIL → STOP the whole wave; do NOT proceed to next T |

---

## Wave-completion checklist (run after T8 lands)

- [ ] `go build ./qaherald/...` clean.
- [ ] `go test ./qaherald/... -race -count=1` 100% PASS.
- [ ] `go vet ./qaherald/...` clean.
- [ ] `scripts/audit_antibluff.sh` PASS.
- [ ] `scripts/codegraph_validate.sh` PASS.
- [ ] `scripts/e2e_bluff_hunt.sh` PASS (e2e invariant count unchanged at 62 — qaherald-auto adds no new e2e invariant; it CONSUMES the existing E71-E80 lifecycle invariants).
- [ ] `tests/test_constitution_inheritance.sh` PASS.
- [ ] `tests/test_wave6.5_lifecycle.sh` in automated mode PASS (live, requires HERALD_QA_BOT_TOKEN).
- [ ] `tests/test_wave6.5_lifecycle.sh` in manual mode still works (`HERALD_LIFECYCLE_MANUAL=1`).
- [ ] `docs/qa/HRD-101-lifecycle-<run-id>/` committed with transcript.jsonl + report.md + attachments/.
- [ ] HRD-101 closed in Issues.md → Fixed.md.
- [ ] CLAUDE.md r9 reflects qaherald-auto landing.
- [ ] `v0.5.1` tag created locally.
- [ ] No `MUTATED for paired` / `// always pass` markers in any staged file (§107.y).

---

## Out-of-scope (explicitly DEFERRED — DO NOT add)

- **MTProto / Telegram user-account impersonation** — Wave 7 territory.
- **Slack/Email MessengerClient impls** — Wave 7. The interface lands here; impls are Wave 7's job.
- **GUI / web dashboard for transcripts** — never. Markdown report is the only UI.
- **Multi-channel parallelism** — scenarios run serially. Parallelism is an optimisation that risks ordering bugs in fs-mutation assertions; skip until needed.
- **Scenario authoring DSL / YAML** — Go funcs only, per Wave 5 precedent.
- **Auto-recovery on transient Telegram 500s** — single attempt per scenario; flake-suppression is the operator's job (re-run lifecycle).
- **Real CC dispatch verification** — scenarios assert pherald's classification + action label, NOT that CC's response text is semantically correct. CC quality is out of scope.
- **HTTP/3 + TOON exercise** — Wave 5 `qaherald run` already exercises h3+TOON; lifecycle uses Telegram only.

---

## Self-review (plan-level)

- [x] All 15 scenarios from `tests/test_wave6.5_lifecycle.sh` mapped to a `Scenario` func in Registry().
- [x] `MessengerClient` interface lets a future Slack impl plug in without changing scenario code.
- [x] Orchestrator preserves Wave 5 transcript format (E71-E80 grep paths unchanged).
- [x] 8 tasks total; T9/T10 explicitly forbidden.
- [x] §107 anti-bluff requirements wired in T3 (helpers), T5 (report), T6 (preflight), T7 (tests).
- [x] Anti-overengineering: reuses Wave 5 `transcript/`, `tgram/`, `herald/`, `report/`; adds 2 NEW packages (`messenger/`, `lifecycle/`).
- [x] Stash@{0} (Wave 5 T8 partial work) is orthogonal — does NOT need to be popped first.
- [x] Tag `v0.5.1` after T8; NO push.

---

## Ambiguities surfaced during planning

1. **telebot.v3 `CanReadAllGroupMessages` field availability.** The vendored telebot at `submodules/telebot/` may or may not expose this on `Bot.Me`. T2 implements both paths (direct field access + raw HTTP fallback). Operator confirms which path the vendored copy supports during T2 review.

2. **pherald transcript schema.** This plan assumes pherald writes `{ts, direction, kind, payload, classification, action}` to `--qa-out-dir/transcript.jsonl`. If the actual schema differs, the helpers in T3 (`awaitClassification`) need adjustment — likely a small JSON-path update. Will be caught in T7 unit tests via the stub fixture.

3. **S09 non-operator bot allowlist.** The operator must register `HERALD_QA_BOT_TOKEN_NON_OPERATOR`'s user-id as NOT being in `HERALD_OPERATOR_IDS` before running. T6 gate G10 enforces this; operator credential setup docs in T8 explain it.

4. **Fixture sizes.** S11/S12/S13 fixtures must be small enough to avoid Telegram's 50MB upload cap but large enough to be non-trivial. Targets: 4KB jpg, ~10KB pdf, ~5KB ogg. Operator may swap fixtures freely; sha256 assertions adjust automatically.

5. **OTel port liveness.** G1 best-effort port-check uses `HERALD_OTEL_PORT` env. Operator may run pherald without OTel; in that case G1 falls back to the `pherald-bot username resolves in chat` check.
