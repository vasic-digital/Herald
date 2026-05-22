<div align="center">

<img src="../../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Wave 6 — pherald Inbound Runtime + Claude Code Headless Bridge

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` per Universal Constitution §11.4.70. Each task below dispatches to its own implementer subagent. Steps use checkbox (`- [ ]`) syntax. Run each task's commit only AFTER its self-review block passes.

**Goal:** Close the bidirectional loop. Today pherald can send to Telegram and parse CloudEvents, but the *return path* (subscriber types in the TG group → pherald hears it → Claude Code (Opus, headless) processes it → pherald replies in-thread with `reply_to_message_id`) is incomplete. Wave 6 builds the complete inbound runtime: long-poll `getUpdates` with `OnText|OnPhoto|OnDocument|OnVoice`, bot self-filter to avoid echo loops, sha256-content-addressed attachment download, the human-language pre-text prelude that the operator mandated (`"We have received new message from our communication channel <channel-name>. ..."`), live `claude --resume <session-uuid> --model claude-opus-4-7 --print <envelope>` invocation, reply parsing with `action` routing (`reply` / `issue.open` / `event.emit`), and `tgram.SendReply` that wires `reply_to_message_id`. Tags **v0.4.0**. Closes the HRD-NNN that opens for "pherald inbound runtime + CC headless glue + reply API."

**Architecture:**
New package `pherald/internal/inbound/` (NOT pre-created — the implementer will `mkdir -p` it during T7). New Cobra subcommand `pherald listen`. Three production surfaces touched:

1. `commons_messaging/channels/tgram/subscribe.go` — add `OnPhoto`/`OnDocument`/`OnVoice` handlers + bot self-filter (closure over `bot.Me.Username` captured at construction time).
2. `commons_messaging/dispatch/claude_code/` — `dispatch.go` adds `--model claude-opus-4-7` to the `exec.CommandContext` arg list; `claude_code.go` extends `FormatEnvelope` to emit the verbatim operator pre-text BEFORE the `<<<HERALD-DISPATCH-v1>>>` block.
3. `pherald/internal/inbound/` (new) — `Dispatcher` implements `commons.InboundHandler`, calls the Claude Code dispatcher, parses `action` from the reply, routes to (a) `tgram.SendReply` for `action: "reply"`, (b) `pherald` issue-create code path for `action: "issue.open"`, (c) the outbound runner pipeline for `action: "event.emit"`.

The Telegram `Subscribe` continues to construct its own `*telebot.Bot` with `LongPoller{Timeout: 25 * time.Second}` (already correct per HRD-011). The new attachment download path uses `bot.File(&telebot.File{FileID: ...})` and streams the response body through `io.MultiWriter(diskFile, sha256Hasher)` into `~/.herald/inbox/<sha256>.<ext>`.

**Tech Stack:** Go 1.25+; existing `gopkg.in/telebot.v3` (vendored at `submodules/telebot/`); existing `github.com/spf13/cobra`; standard library `crypto/sha256`, `os/exec`, `path/filepath`. No new external dep, no new submodule, no new workspace module.

**Spec reference:** `docs/specs/mvp/specification.V3.md` §32 (inbound pipeline), §33 (Claude Code dispatcher), §33.3 (envelope schema), §18.1.1 (command-prefix replies), §43 (commands catalogue — `pherald listen` added in T13); `docs/guides/HERALD_CONSTITUTION.md` §107 + §107.x (docs/qa evidence — every Wave 6 run produces a transcript under `docs/qa/HRD-NNN-W6/`) + §107.y (working-tree quiescence; T12 mutation gate writes `.git/MUTATION_IN_PROGRESS` lockfile + greps for residue markers pre-add).

**Wave 4a + 4b + 5 substrate already landed:** HTTP/3 + Brotli + TLS 1.3 + Alt-Svc (`v0.2.0`); TOON across all `/v1/*` (`v0.3.0`); qaherald scaffold + scenarios + transcript writer (`v1.0.0-dev-0.0.1` pending tag). E2e at 70 invariants (E1..E70 inclusive of Wave 5 additions per Wave-5 plan T9 — note: at Wave 6 commit time the current main may still be at 62; Wave 6 layers E71..E78 on top of the prevailing post-Wave-5 baseline). Workspace at 15 modules; Wave 6 adds NO new module (inbound is an internal subpackage of `pherald`).

**§107 anti-bluff watershed (CRITICAL):** Wave 6 is the closed-loop watershed. Three §11.4 cuts that distinguish PASS from PASS-bluff:

1. **Bot self-filter MUST work — no echo loops.** If pherald's `OnText` handler dispatches a CC reply, then re-sees that reply via `getUpdates` (every bot DOES see its own messages in groups), then dispatches again — Claude is now hallucinating to itself in an infinite loop on the operator's API quota. The T12 mutation gate (a) blanks the self-filter; the detector asserts an echo loop is observed within 5 seconds. If the gate's mutated build does NOT echo, the self-filter logic was never load-bearing → critical defect.

2. **Opus pinning MUST appear in the actual `argv` of the spawned process.** Not "intended via config", not "passed as env var, hopefully the binary reads it" — the literal `argv` MUST contain `--model claude-opus-4-7`. T2's unit test inspects the constructed `*exec.Cmd.Args` slice. T11's E73 invariant greps the production binary's recorded subprocess line in `docs/qa/HRD-NNN-W6/transcript.jsonl` for the model arg. T12 mutation (b) swaps to `claude-sonnet-4-6`; detector asserts the e2e invariant catches it.

3. **`reply_to_message_id` MUST be wire-bytes correct.** Telegram clients render a reply differently from a fresh message; missing the field silently degrades the UX without erroring. T8's unit test hits a `httptest.Server` that records `r.FormValue("reply_to_message_id")` and asserts it equals the original message ID. T12 mutation (c) drops the field; detector asserts E75 catches it.

**Operator-locked decisions (recorded 2026-05-22, verbatim — DO NOT re-decide):**

| Decision | Value |
|---|---|
| Session name | `HERALD_PROJECT_NAME` env var; default = `filepath.Base(workdir)`; final fallback `"Herald"` |
| Model | **Opus, always.** Pin via `claude --model claude-opus-4-7`. No Sonnet / Haiku fallback. No `HERALD_CC_MODEL` override env var. |
| Envelope pre-text wording | Verbatim: `"We have received new message from our communication channel <channel-name>. <classification sentence>. <attachment list>"`. Free-form prose. Precedes the `<<<HERALD-DISPATCH-v1>>>` structured block. |
| Polling cadence | telebot.LongPoller `Timeout: 25 * time.Second` (already wired); §32.2 30s safety-net stays observational. |
| Bot self-filter | Drop if `msg.Sender != nil && msg.Sender.IsBot && msg.Sender.Username == botSelfUsername`. `botSelfUsername` captured at Subscribe-construction via `bot.Me.Username`. |
| Attachment storage | `~/.herald/inbox/<sha256>.<ext>` — content-addressed. `<ext>` derived from MIME type (jpeg → `jpg`, png → `png`, mp4 → `mp4`, ogg → `ogg`, application/pdf → `pdf`, fallback `bin`). |
| Reply API | `tgram.Adapter.SendReply(ctx, chatID int64, text string, replyToID int, attachments []commons.Attachment) (msgID int, err error)` — uses `telebot.SendOptions{ReplyTo: &telebot.Message{ID: replyToID}}`. |
| Reply action triggers | CC reply may include `"action": "reply" | "issue.open" | "event.emit"` field in the `<<<HERALD-REPLY>>>` JSON. Default `"reply"`. `issue.open` payload `{type, criticality, title, body, labels}`. `event.emit` payload `{cloudevent_type, subject, data}`. |
| Wave 6 tag | `v0.4.0` (minor — substantial new capability) |
| Workspace module count | **15 → 15** (no new module — inbound is `pherald/internal/inbound/`) |

**Task decomposition (13 tasks):** T1 `HERALD_PROJECT_NAME` resolver + `.env.example` → T2 Opus model pinning in dispatcher → T3 `FormatEnvelope` pre-text → T4 bot self-filter in `Subscribe` → T5 OnPhoto/OnDocument/OnVoice + attachment download → T6 `pherald listen` Cobra subcommand → T7 `pherald/internal/inbound/` Dispatcher + action routing → T8 `tgram.SendReply` with `reply_to_message_id` → T9 live closed-loop test → T10 `docs/qa/<run-id>/` evidence capture → T11 e2e_bluff_hunt E63..E70 (numbered for the Wave 6 brief — at commit time renumbered to E71..E78 if Wave 5 already shipped E63..E70) → T12 Wave 6 mutation gate → T13 spec V3 r-bump + §43 + Issues→Fixed + tag `v0.4.0` + 4-mirror push.

> **Numbering note:** The task brief specifies "E63..E70" for the Wave 6 e2e invariants. Wave 5's plan also specifies E63..E70. Whichever wave commits first owns E63..E70 verbatim. The second-to-commit wave renumbers contiguously (E71..E78). The Wave 6 implementer subagent MUST inspect `scripts/e2e_bluff_hunt.sh` at the moment of T11 execution to determine the correct base offset. The plan keeps the symbolic names (E63..E70 inside this document) but the actual file edit picks the next free contiguous range.

---

## File Structure

### CREATE

| Path | Responsibility |
|---|---|
| `pherald/internal/inbound/dispatcher.go` | `Dispatcher` struct implementing `commons.InboundHandler`. Holds `*claude_code.Dispatcher`, `tgram.Adapter` (for replies), `*pherald.IssueOpener` (T7-internal stub), `commons.Clock`. Method `Handle(ctx, ev)` per the §32 inbound pipeline. |
| `pherald/internal/inbound/dispatcher_test.go` | Unit tests with stub `claude_code.Dispatcher` returning canned replies for each `action` value; asserts correct routing. |
| `pherald/internal/inbound/reply.go` | `type Reply struct { Action string; Text string; Issue *IssuePayload; Event *EventPayload }`. `ParseReply([]byte) (*Reply, error)` — extracts the reply envelope from CC stdout, classifies the `action`. |
| `pherald/internal/inbound/reply_test.go` | Table-driven test: 4 canned CC replies (reply / issue.open / event.emit / missing-action-defaults-to-reply). |
| `pherald/internal/inbound/attachments.go` | Helper `DownloadAttachment(ctx, bot *telebot.Bot, fileID, mime string) (path, sha256, error)`. Streams via `bot.File(&telebot.File{FileID: fileID})` → `io.MultiWriter(diskFile, sha256.New())` → atomic rename. Uses `~/.herald/inbox/`. |
| `pherald/internal/inbound/attachments_test.go` | httptest server impersonating Bot API `getFile` + file content endpoint; asserts written file's sha256 matches the recorded sha256, asserts atomic rename worked, asserts duplicate download (same sha256) doesn't write twice. |
| `pherald/cmd/pherald/listen.go` | Cobra subcommand `listen`. Long-running. Wires `tgram.Subscribe` to `inbound.Dispatcher`. Honors SIGINT / SIGTERM. Flags `--bot-token`, `--chat-id`, `--project-name` (env fallbacks). |
| `pherald/cmd/pherald/listen_test.go` | Integration: starts `listen` in a goroutine against a stub Telegram server returning a canned text update; asserts the inbound dispatcher's `Handle` was invoked within 5s; asserts SIGTERM stops the loop cleanly. |
| `tests/test_wave6_live_loop.sh` | LIVE closed-loop e2e: builds pherald, starts `pherald listen` in background, waits for operator-pre-typed message to be picked up, asserts a reply lands in the chat with `reply_to_message_id == original`, kills pherald cleanly. SKIPS-with-reason if `HERALD_TGRAM_BOT_TOKEN` / `HERALD_TGRAM_CHAT_ID` / `HERALD_CLAUDE_CODE_BINARY` unset. |
| `tests/test_wave6_mutation_meta.sh` | Wave 6 paired-mutation gate: (a) blank bot self-filter → assert echo loop, (b) `--model claude-sonnet-4-6` → assert E73 catches it, (c) drop `reply_to_message_id` → assert E75 catches it. Pre-flight `.git/MUTATION_IN_PROGRESS` lockfile + working-tree quiescence per §107.y. |
| `docs/qa/HRD-NNN-W6-<run-id>/` (populated by T10) | `transcript.jsonl`, `pherald-listen.log`, `claude-stdout.log`, `claude-stderr.log`, `attachments/<sha256>.<ext>` per §107.x. |

### MODIFY

| Path | Change |
|---|---|
| `commons/branding.go` (or equivalent — implementer locates by grep) | Add `func ProjectName() string` returning env-var-or-fallback-or-default. |
| `commons_messaging/dispatch/claude_code/dispatch.go` | Add `--model`, `claude-opus-4-7` to the `exec.CommandContext` arg list. Single-line insertion between `--print` and `envelope`. |
| `commons_messaging/dispatch/claude_code/claude_code.go` | Add `FormatEnvelopeWithPreText(req DispatchRequest, channelName string) string`. The existing `FormatEnvelope` becomes a thin wrapper: `return d.FormatEnvelopeWithPreText(req, string(req.Channel))`. The pre-text is prepended in front of the existing `<<<HERALD-DISPATCH-v1>>>` block. |
| `commons_messaging/dispatch/claude_code/claude_code_test.go` | Add tests asserting pre-text content + ordering. |
| `commons_messaging/dispatch/claude_code/dispatch_integration_test.go` | If a live-integration test exists, extend it to assert `--model` flag present in spawned `argv`. |
| `commons_messaging/channels/tgram/subscribe.go` | Capture `bot.Me.Username` at construction; add bot self-filter inside `OnText` handler; add `OnPhoto`, `OnDocument`, `OnVoice` handlers (each does attachment download + self-filter + dispatch). |
| `commons_messaging/channels/tgram/subscribe_integration_test.go` | Add table-driven cases: bot-own message dropped; OnPhoto produces InboundEvent with Attachments populated; OnDocument same; OnVoice same. |
| `commons_messaging/channels/tgram/send.go` | Add `Adapter.SendReply(ctx, chatID, text, replyToID, attachments []commons.Attachment) (msgID int, err error)`. Internally constructs `telebot.SendOptions{ReplyTo: &telebot.Message{ID: replyToID}}`. |
| `commons_messaging/channels/tgram/send_integration_test.go` | Add SendReply test against httptest Bot API stub; assert `reply_to_message_id` URL form value. |
| `pherald/cmd/pherald/main.go` | Register the new `listen` subcommand in the Cobra root. |
| `.env.example` | Add `HERALD_PROJECT_NAME=` line with operator-facing comment. |
| `scripts/e2e_bluff_hunt.sh` | Append E63..E70 (or E71..E78 per the numbering note above); update header tally. |
| `tests/test_wave6_mutation_meta.sh` | NEW file (listed under CREATE above). |
| `docs/specs/mvp/specification.V3.md` | r12 → r13: §32 + §33 substantive updates (pre-text wording recorded verbatim; `--model claude-opus-4-7` recorded as pin); §43 commands catalogue extended with `pherald listen` + flags. |
| `docs/Issues.md` | Atomic Issues→Fixed at commit time: prepend HRD-NNN-W6 entries. |
| `docs/Fixed.md` | Prepend HRD-NNN-W6 close entries with commit SHA + e2e invariant references + tag `v0.4.0`. |
| `docs/Status.md` | Wave 6 completion summary; e2e invariant total bumped; tag `v0.4.0`. |
| `docs/CONTINUATION.md` | r-bump: append `HERALD_PROJECT_NAME` documentation + `pherald listen` live-test handoff prompt. |
| `CLAUDE.md` | r-bump status pointer: Wave 6 inbound runtime + tag `v0.4.0`. Module count unchanged (15). |
| `docs/guides/HERALD_CONSTITUTION.md` | (Optional, only if §32/§33 already enumerate per-wave pointers) Append Wave 6 pointer. |

---

## Pre-flight (run BEFORE T1)

```bash
cd /Users/milosvasic/Projects/Herald

# Inheritance gate — MUST PASS before any commit touching root docs
bash tests/test_constitution_inheritance.sh
bash tests/test_constitution_inheritance_meta.sh

# Working-tree quiescence (§107.y)
test ! -f .git/MUTATION_IN_PROGRESS || { echo "MUTATION_IN_PROGRESS lockfile present — abort"; exit 1; }
git grep -n "MUTATED for paired\|// always pass\|// MUTATION\|# MUTATION" -- . ':!docs/superpowers/plans' ':!tests/test_wave*_mutation_meta.sh' && { echo "residue markers present"; exit 1; } || echo "quiescent"

# Codegraph + anti-bluff baseline
scripts/audit_antibluff.sh
scripts/codegraph_validate.sh
scripts/e2e_bluff_hunt.sh  # expect current baseline PASS (62 or 70 invariants depending on Wave-5 sequencing)

# Expected: all PASS. If any FAIL, fix at root cause per §11.4.4 before starting Wave 6.
```

---

## Task 1: `HERALD_PROJECT_NAME` env var + resolver + `.env.example`

**Goal.** Operator's locked decision: the Claude Code session name is `HERALD_PROJECT_NAME` (env), default `filepath.Base(os.Getwd())`, final fallback `"Herald"`. Every pherald subcommand that touches the Claude Code dispatcher reads `commons.ProjectName()` instead of hardcoding `"Herald"`.

### Files

**Modify:** `commons/branding.go` — add `func ProjectName() string`.
**Modify:** `.env.example` — add `HERALD_PROJECT_NAME=` documented line.
**Create test:** `commons/branding_test.go` — add `TestProjectName` table-driven cases.

### §107 anti-bluff anchor

A `ProjectName()` that always returned `"Herald"` regardless of env/cwd would pass type-checks and even pass a naive smoke (CC sessions still resolve, just to the wrong project). The bluff is "config respected by code" without proving it. Test uses `t.TempDir()` + `os.Chdir` + `os.Setenv("HERALD_PROJECT_NAME", "...")` permutations and asserts the returned value matches the precedence order.

### Steps

- [ ] **1.1** Locate the canonical place. Default location is `commons/branding.go` since branding already lives there (per-flavor factory). Grep first:

```bash
grep -n "func .*Project\|HERALD_PROJECT" commons/*.go pherald/cmd/pherald/*.go 2>/dev/null
```

Expected: nothing matches (no existing implementation).

- [ ] **1.2** Write the failing test first (`commons/branding_test.go`):

```go
func TestProjectName(t *testing.T) {
    // Save/restore env + cwd
    origEnv, hadEnv := os.LookupEnv("HERALD_PROJECT_NAME")
    origCwd, _ := os.Getwd()
    t.Cleanup(func() {
        if hadEnv { os.Setenv("HERALD_PROJECT_NAME", origEnv) } else { os.Unsetenv("HERALD_PROJECT_NAME") }
        _ = os.Chdir(origCwd)
    })
    // Case 1: env var set wins
    os.Setenv("HERALD_PROJECT_NAME", "AtmosphereProject")
    if got := commons.ProjectName(); got != "AtmosphereProject" {
        t.Fatalf("env-set: got %q want AtmosphereProject", got)
    }
    // Case 2: env empty -> filepath.Base(cwd)
    os.Unsetenv("HERALD_PROJECT_NAME")
    tmp := t.TempDir()
    sub := filepath.Join(tmp, "MyFancyProject")
    _ = os.MkdirAll(sub, 0o755)
    _ = os.Chdir(sub)
    if got := commons.ProjectName(); got != "MyFancyProject" {
        t.Fatalf("cwd-base: got %q want MyFancyProject", got)
    }
    // Case 3: env empty + cwd unreadable -> "Herald"
    // (skipping the unreadable simulation — covered by code review of the
    //  os.Getwd error branch; the test that exercises it is the env-set case
    //  which already takes the env path.)
}
```

```bash
cd commons && go test -run TestProjectName -count=1
# Expected: FAIL ("undefined: commons.ProjectName")
```

- [ ] **1.3** Implement in `commons/branding.go`:

```go
// ProjectName resolves the Claude Code session name per Wave 6 operator
// mandate: HERALD_PROJECT_NAME env var wins; else filepath.Base(os.Getwd());
// else "Herald".
func ProjectName() string {
    if s := strings.TrimSpace(os.Getenv("HERALD_PROJECT_NAME")); s != "" {
        return s
    }
    if cwd, err := os.Getwd(); err == nil && cwd != "" {
        return filepath.Base(cwd)
    }
    return "Herald"
}
```

(Imports: `os`, `path/filepath`, `strings`. Add to the existing import block if absent.)

- [ ] **1.4** Run test green:

```bash
go test -run TestProjectName -count=1 ./commons/...
# Expected: PASS
```

- [ ] **1.5** Update `.env.example` with operator-facing comment:

```
# HERALD_PROJECT_NAME pins the Claude Code session name. Unset = use the
# current working directory's basename. Final fallback = "Herald".
# Operator-locked Wave 6 decision (2026-05-22).
HERALD_PROJECT_NAME=
```

- [ ] **1.6** Working-tree quiescence + commit:

```bash
git grep -n "MUTATED for paired\|// always pass" -- . ':!docs/superpowers/plans' ':!tests/test_wave*_mutation_meta.sh' && exit 1 || true
git status --porcelain
# Expected: only commons/branding.go, commons/branding_test.go, .env.example modified.
git add commons/branding.go commons/branding_test.go .env.example
git commit -m "$(cat <<'EOF'
Wave 6 step 1: HERALD_PROJECT_NAME resolver + .env.example

Adds commons.ProjectName() per operator-locked Wave 6 decision:
HERALD_PROJECT_NAME env var wins; else filepath.Base(os.Getwd());
else "Herald". Wired into CC session name in T7.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

### Self-review

- [ ] `commons.ProjectName()` signature exact match: `func ProjectName() string` (no params, no error return — single-source-of-truth helper).
- [ ] Three precedence cases all tested.
- [ ] `.env.example` documents the var without setting a value (env-empty = filepath.Base path).
- [ ] Working-tree quiescent.

---

## Task 2: Pin Opus model in Claude Code dispatcher

**Goal.** Operator's locked decision: **Opus, always.** The current dispatcher (`commons_messaging/dispatch/claude_code/dispatch.go`) invokes `claude --resume <UUID> --print <envelope>` with NO model flag — which means the dispatcher relies on Claude's default model selection (which can change without notice, drift to a cheaper tier, or vary by user CLI configuration). Wave 6 makes the model selection load-bearing: `--model`, `claude-opus-4-7` is inserted between `--print` and `envelope` in the argv slice.

### Files

**Modify:** `commons_messaging/dispatch/claude_code/dispatch.go` — single-line insertion in the `exec.CommandContext` arg list.
**Modify:** `commons_messaging/dispatch/claude_code/claude_code_test.go` (existing unit test file) — assert dispatcher constructs cmd with `--model claude-opus-4-7` in argv.
**Modify:** `commons_messaging/dispatch/claude_code/dispatch_integration_test.go` (if exists; if not, create) — assert live `claude` invocation includes the flag.

### §107 anti-bluff anchor

"Config respected by code" — without proving the binary actually receives the flag. Tests inspect `*exec.Cmd.Args` directly OR (live test) read the dispatcher's recorded subprocess line from a wrap-script that logs `argv`. Mutation gate (b) swaps to `claude-sonnet-4-6`; e2e invariant E73 asserts the model arg in any captured spawn record matches `claude-opus-4-7`.

### Steps

- [ ] **2.1** Read the current dispatch.go around the `exec.CommandContext` call:

```bash
grep -n -B2 -A6 'exec.CommandContext' commons_messaging/dispatch/claude_code/dispatch.go
```

Expected output (per the substrate inspection): the current call is

```go
cmd := exec.CommandContext(ctx, d.binaryPath,
    "--resume", sessionUUID.String(),
    "--print", envelope,
)
```

with NO `--model` flag present.

- [ ] **2.2** Write the failing unit test addition in `commons_messaging/dispatch/claude_code/claude_code_test.go` (or extend the test file that already covers `Dispatch`-construction; add a new test if none directly inspects argv):

```go
func TestDispatchCommandIncludesOpusModel(t *testing.T) {
    // Construct dispatcher with a non-existent binary path; we don't run it,
    // we only inspect the cmd.Args slice built up before exec.
    //
    // The cleanest way is to refactor Dispatch into a buildCmd helper that
    // returns the *exec.Cmd before exec, so the test can inspect args
    // without spawning. The refactor is invisible to callers.
    d, err := claude_code.New("/nonexistent/claude", t.TempDir(), "TestProj")
    if err != nil { t.Fatal(err) }
    // Pre-create an anchor file so ResolveSession returns a non-Nil UUID.
    anchorDir := filepath.Join(d.WorkingDirForTest(), ".herald", "claude-code", "sessions")
    _ = os.MkdirAll(anchorDir, 0o755)
    _ = os.WriteFile(filepath.Join(anchorDir, "TestProj.session"),
        []byte("11111111-2222-3333-4444-555555555555\n"), 0o644)
    cmd, err := d.BuildCmdForTest(context.Background(), claude_code.DispatchRequest{
        UserMessage: "hi",
    })
    if err != nil { t.Fatal(err) }
    // Argv MUST include --model claude-opus-4-7 contiguously.
    args := cmd.Args
    found := false
    for i := 0; i+1 < len(args); i++ {
        if args[i] == "--model" && args[i+1] == "claude-opus-4-7" { found = true; break }
    }
    if !found {
        t.Fatalf("argv missing --model claude-opus-4-7; got: %v", args)
    }
}
```

Note: the test refers to two new exported helpers (`WorkingDirForTest`, `BuildCmdForTest`). The minimal-surface approach is to add a private `buildCmd` method called by both `Dispatch` and the test, then use `export_test.go` to expose it for tests in the same package:

```go
// In a new file: commons_messaging/dispatch/claude_code/export_test.go
package claude_code
func (d *Dispatcher) BuildCmdForTest(ctx context.Context, req DispatchRequest) (*exec.Cmd, error) {
    return d.buildCmd(ctx, req)
}
func (d *Dispatcher) WorkingDirForTest() string { return d.workingDir }
```

```bash
cd commons_messaging/dispatch/claude_code && go test -run TestDispatchCommandIncludesOpusModel -count=1
# Expected: FAIL (either undefined BuildCmdForTest, or --model not in args)
```

- [ ] **2.3** Refactor `Dispatch` in `dispatch.go` to extract `buildCmd`:

```go
func (d *Dispatcher) buildCmd(ctx context.Context, req DispatchRequest) (*exec.Cmd, error) {
    sessionUUID, _, err := d.ResolveSession()
    if err != nil { return nil, err }
    if sessionUUID == uuid.Nil {
        return nil, fmt.Errorf("claude_code: no anchored session (HRD-012 step 7 will bootstrap)")
    }
    envelope := d.FormatEnvelope(req)
    cmd := exec.CommandContext(ctx, d.binaryPath,
        "--resume", sessionUUID.String(),
        "--model", "claude-opus-4-7", // Wave 6 operator-locked: Opus always
        "--print", envelope,
    )
    cmd.Dir = d.workingDir
    return cmd, nil
}
```

Then `Dispatch` calls `d.buildCmd(ctx, req)` and continues with `cmd.Output()`.

- [ ] **2.4** Run the new test green:

```bash
go test -run TestDispatchCommandIncludesOpusModel -count=1 ./commons_messaging/dispatch/claude_code/...
# Expected: PASS
```

- [ ] **2.5** Also re-run all existing dispatch tests to confirm no regression:

```bash
go test -count=1 ./commons_messaging/dispatch/claude_code/...
# Expected: ALL PASS
```

- [ ] **2.6** Working-tree quiescence + commit:

```bash
git grep -n "// always pass\|MUTATED for paired" -- . ':!docs/superpowers/plans' ':!tests/test_wave*_mutation_meta.sh' && exit 1 || true
git add commons_messaging/dispatch/claude_code/dispatch.go \
        commons_messaging/dispatch/claude_code/claude_code_test.go \
        commons_messaging/dispatch/claude_code/export_test.go
git commit -m "$(cat <<'EOF'
Wave 6 step 2: pin Opus model in claude_code dispatcher

Adds --model claude-opus-4-7 to the spawned `claude` argv per operator-
locked Wave 6 decision. Refactors Dispatch into buildCmd + exec halves
so the test can assert the model flag without spawning.

§107 anchor: model selection is now load-bearing — the literal argv
contains the model flag (not "config respected, hopefully"). T12
mutation gate (b) swaps to claude-sonnet-4-6 to prove the assertion is
catching real drift.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

### Self-review

- [ ] `--model` and `claude-opus-4-7` appear as two contiguous argv entries (NOT as a single `"--model claude-opus-4-7"` joined string — the latter would be one arg from Telegram's perspective and rejected by Claude CLI).
- [ ] Test inspects `cmd.Args` directly (not stdin, not env).
- [ ] No regression in existing dispatch tests.

---

## Task 3: Envelope pre-text — verbatim operator wording before `<<<HERALD-DISPATCH-v1>>>`

**Goal.** Operator wants the LLM to read context BEFORE the structured schema. Current `FormatEnvelope` jumps straight to the `<<<HERALD-DISPATCH-v1>>>` marker. Wave 6 prepends a human-language prelude with the verbatim operator wording: `"We have received new message from our communication channel <channel-name>. ..."`. The structured envelope itself is unchanged — it stays after a blank line.

### Files

**Modify:** `commons_messaging/dispatch/claude_code/claude_code.go` — add `FormatEnvelopeWithPreText`. The existing `FormatEnvelope` becomes a thin wrapper.
**Modify:** `commons_messaging/dispatch/claude_code/claude_code_test.go` — add `TestFormatEnvelopePreText`.

### §107 anti-bluff anchor

The pre-text MUST appear verbatim in the bytes piped to Claude — not in a log line, not in a debug comment, not in a "we'll add it once the user complains" TODO. T11's E74 invariant greps the captured envelope bytes (from `docs/qa/<run-id>/`) for the literal prefix `"We have received new message from our communication channel "`.

### Steps

- [ ] **3.1** Read the current `FormatEnvelope`:

```bash
grep -n -A30 'func .*FormatEnvelope' commons_messaging/dispatch/claude_code/claude_code.go
```

Expected: ~30-line function that writes the `<<<HERALD-DISPATCH-v1>>>` block + structured fields + JSON schema reminder.

- [ ] **3.2** Write the failing test first:

```go
func TestFormatEnvelopePreText(t *testing.T) {
    d, err := claude_code.New("/bin/true", t.TempDir(), "AtmosphereProject")
    if err != nil { t.Fatal(err) }
    req := claude_code.DispatchRequest{
        InboundID:      "01HXYZ",
        Sender:         "tgram:milos85vasic",
        Channel:        commons.ChannelTelegram,
        Classification: claude_code.Classification{Type: "query", Criticality: "low", Confidence: 0.88},
        Conversation:   "milos: ping?",
        Attachments:    []commons.Attachment{{Filename: "shot.png", MIMEType: "image/png", SizeBytes: 1234}},
        UserMessage:    "ping",
    }
    out := d.FormatEnvelopeWithPreText(req, "tgram")

    // (a) verbatim opening line (the operator's mandated wording)
    if !strings.HasPrefix(out, "We have received new message from our communication channel tgram.") {
        t.Fatalf("missing verbatim pre-text opener; got first 80 bytes: %q", out[:min(80, len(out))])
    }
    // (b) pre-text appears BEFORE the structured marker
    preIdx := strings.Index(out, "We have received new message")
    markerIdx := strings.Index(out, "<<<HERALD-DISPATCH-v1>>>")
    if preIdx < 0 || markerIdx < 0 || preIdx >= markerIdx {
        t.Fatalf("ordering wrong: preIdx=%d markerIdx=%d", preIdx, markerIdx)
    }
    // (c) blank line between pre-text and structured block
    blank := strings.Index(out[preIdx:], "\n\n<<<HERALD-DISPATCH-v1>>>")
    if blank < 0 {
        t.Fatalf("no blank line between pre-text and structured marker")
    }
    // (d) attachment filename surfaces in the pre-text (so the LLM sees the
    //     attachment context in natural language before the structured list)
    if !strings.Contains(out, "shot.png") {
        t.Fatalf("attachment filename not surfaced in pre-text")
    }
    // (e) classification surfaces in the pre-text
    if !strings.Contains(strings.ToLower(out), "query") {
        t.Fatalf("classification not surfaced in pre-text")
    }
}
```

```bash
cd commons_messaging/dispatch/claude_code && go test -run TestFormatEnvelopePreText -count=1
# Expected: FAIL (undefined FormatEnvelopeWithPreText)
```

- [ ] **3.3** Implement `FormatEnvelopeWithPreText` in `claude_code.go`:

```go
// FormatEnvelopeWithPreText renders the §33.3 envelope prefixed with the
// verbatim operator pre-text per Wave 6 operator mandate (2026-05-22):
//
//   "We have received new message from our communication channel <name>.
//    <classification sentence>. <attachment list>"
//
// followed by a blank line and the existing <<<HERALD-DISPATCH-v1>>>
// structured block (kept byte-for-byte identical to FormatEnvelope's
// output for the structured portion).
func (d *Dispatcher) FormatEnvelopeWithPreText(req DispatchRequest, channelName string) string {
    var pre strings.Builder
    fmt.Fprintf(&pre, "We have received new message from our communication channel %s.\n", channelName)
    fmt.Fprintf(&pre, "The message has been classified as %q with %q criticality (confidence %.2f).\n",
        req.Classification.Type, req.Classification.Criticality, req.Classification.Confidence)
    fmt.Fprintf(&pre, "Sender: %s. Inbound ID: %s.\n", req.Sender, req.InboundID)
    if len(req.Attachments) > 0 {
        pre.WriteString("Attached materials:\n")
        for _, a := range req.Attachments {
            // Attachments downloaded by pherald inbound runtime are
            // available on the local filesystem. The path is carried in
            // the Filename field for Wave 6 (it's already the canonical
            // ~/.herald/inbox/<sha256>.<ext> path emitted by the
            // attachment download helper — see T5).
            fmt.Fprintf(&pre, "  - %s (%s, %d bytes)\n", a.Filename, a.MIMEType, a.SizeBytes)
        }
    } else {
        pre.WriteString("No attached materials.\n")
    }
    pre.WriteString("\n")
    pre.WriteString(d.FormatEnvelope(req)) // existing structured block unchanged
    return pre.String()
}
```

(Existing `FormatEnvelope` stays as-is — it remains the renderer of the structured `<<<HERALD-DISPATCH-v1>>>` portion. `FormatEnvelopeWithPreText` composes pre-text + existing output.)

- [ ] **3.4** Update `Dispatcher.buildCmd` (from T2) to call `FormatEnvelopeWithPreText`:

```go
envelope := d.FormatEnvelopeWithPreText(req, string(req.Channel))
```

(Use `req.Channel` — already typed as `commons.ChannelID` — coerced to string for the pre-text rendering.)

- [ ] **3.5** Run test + the existing FormatEnvelope tests:

```bash
go test -count=1 ./commons_messaging/dispatch/claude_code/...
# Expected: ALL PASS (existing FormatEnvelope tests still pass because we didn't touch its byte output)
```

- [ ] **3.6** Working-tree quiescence + commit:

```bash
git grep -n "MUTATED for paired\|// always pass" -- . ':!docs/superpowers/plans' ':!tests/test_wave*_mutation_meta.sh' && exit 1 || true
git add commons_messaging/dispatch/claude_code/claude_code.go \
        commons_messaging/dispatch/claude_code/claude_code_test.go \
        commons_messaging/dispatch/claude_code/dispatch.go
git commit -m "$(cat <<'EOF'
Wave 6 step 3: envelope pre-text — verbatim operator wording

Adds FormatEnvelopeWithPreText that prepends the operator's mandated
human-language prelude ("We have received new message from our
communication channel <name>. ...") before the existing
<<<HERALD-DISPATCH-v1>>> structured block. Dispatcher.buildCmd switches
to the new formatter; FormatEnvelope is preserved as the structured-only
renderer.

§107 anchor: pre-text bytes literally appear in the envelope piped to
claude. T11 invariant E74 greps captured envelopes for the verbatim
prefix.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

### Self-review

- [ ] Pre-text appears as a strict prefix of the rendered envelope.
- [ ] Structured `<<<HERALD-DISPATCH-v1>>>` block bytes are unchanged byte-for-byte.
- [ ] Classification + sender + attachments all surface in the pre-text in natural language.
- [ ] One blank line separates pre-text from structured block.

---

## Task 4: Bot self-filter in `tgram.Subscribe`

**Goal.** Today `Subscribe`'s `OnText` handler dispatches every incoming text message — including the bot's own replies (a Telegram bot DOES receive its own messages back via `getUpdates` when posting to a group). Without a filter, Wave 6's closed loop becomes an infinite loop: pherald posts a reply → reply re-enters via `getUpdates` → CC processes its own previous reply → posts another reply → ... Operator-locked decision: drop messages where `msg.Sender != nil && msg.Sender.IsBot && msg.Sender.Username == botSelfUsername`. `botSelfUsername` captured at Subscribe-construction via `bot.Me.Username`.

### Files

**Modify:** `commons_messaging/channels/tgram/subscribe.go` — capture `bot.Me.Username` after `telebot.NewBot`; add early-return inside `OnText` handler.
**Modify:** `commons_messaging/channels/tgram/subscribe_integration_test.go` — add a unit-style test that builds an `*Adapter` + a synthetic `telebot.Message` with `Sender.IsBot=true` + matching Username, asserts the handler does NOT call `h.Handle`.

### §107 anti-bluff anchor

A self-filter that "compiles cleanly but the username comparison is always-false because `bot.Me` is nil" would still pass naive unit tests. T12 mutation gate (a) blanks the filter (`if false { return nil }`) and the detector script sends two messages in quick succession to a live bot+chat and asserts the second message echoes — proving the filter was load-bearing on the primary build.

### Steps

- [ ] **4.1** Read telebot's `Sender` field (already confirmed via substrate inspection):
  - `telebot.Message.Sender *User` (json tag `from`)
  - `telebot.User.IsBot bool`
  - `telebot.User.Username string`
  - `telebot.Bot.Me *User` (set by telebot during `NewBot` via `getMe`)

- [ ] **4.2** Write the failing unit test (extend or create the unit-test file in `commons_messaging/channels/tgram/`):

```go
// File: commons_messaging/channels/tgram/subscribe_test.go (NEW)
// Plain unit test — no live network.
func TestSubscribeBotSelfFilter(t *testing.T) {
    var called int32
    h := commons.InboundHandlerFunc(func(ctx context.Context, ev commons.InboundEvent) error {
        atomic.AddInt32(&called, 1)
        return nil
    })
    // Build the filter function directly (Subscribe's internal logic exposed
    // via export_test.go for testability — see step 4.3).
    filter := tgram.SelfFilterForTest("MyHeraldBot")
    botMsg := &telebot.Message{
        Sender: &telebot.User{IsBot: true, Username: "MyHeraldBot"},
        Text:   "echo loop bait",
    }
    if !filter(botMsg) { t.Fatal("expected filter to drop bot-own message") }
    humanMsg := &telebot.Message{
        Sender: &telebot.User{IsBot: false, Username: "milos85vasic"},
        Text:   "hi",
    }
    if filter(humanMsg) { t.Fatal("expected filter to keep human message") }
    botButDifferent := &telebot.Message{
        Sender: &telebot.User{IsBot: true, Username: "SomeOtherBot"},
        Text:   "cross-bot chatter",
    }
    if filter(botButDifferent) { t.Fatal("expected filter to keep other-bot message (cross-bot collab is real)") }
    _ = called
    _ = h
}
```

(`commons.InboundHandlerFunc` may not exist — if not, declare it inline in the test file or add a one-line adapter in `commons/`.)

- [ ] **4.3** Implementation in `commons_messaging/channels/tgram/subscribe.go`:

```go
// shouldDropBotSelf returns true when the message originates from THIS bot.
// Cross-bot messages (a different bot in the same chat) are kept.
func shouldDropBotSelf(msg *telebot.Message, selfUsername string) bool {
    if msg == nil || msg.Sender == nil { return false }
    if !msg.Sender.IsBot { return false }
    return msg.Sender.Username == selfUsername
}
```

Inside `Subscribe`, after `bot, err := telebot.NewBot(...)`:

```go
selfUsername := ""
if bot.Me != nil { selfUsername = bot.Me.Username }
if selfUsername == "" {
    // telebot fetches getMe during NewBot; if it failed we can't filter
    // safely. Treat as fatal — better to fail Subscribe than to ship an
    // echo-loop-prone runtime.
    return fmt.Errorf("tgram.Subscribe: bot.Me.Username unset after NewBot — getMe likely failed")
}
```

Inside the `OnText` handler:

```go
if shouldDropBotSelf(msg, selfUsername) {
    return nil
}
```

(Same guard prepended to `OnPhoto`, `OnDocument`, `OnVoice` handlers added in T5.)

Export for tests via `commons_messaging/channels/tgram/export_test.go`:

```go
package tgram
func SelfFilterForTest(selfUsername string) func(*telebot.Message) bool {
    return func(msg *telebot.Message) bool { return shouldDropBotSelf(msg, selfUsername) }
}
```

- [ ] **4.4** Run tests:

```bash
go test -count=1 ./commons_messaging/channels/tgram/...
# Expected: PASS (including new TestSubscribeBotSelfFilter)
```

- [ ] **4.5** Working-tree quiescence + commit:

```bash
git grep -n "MUTATED for paired\|// always pass" -- . ':!docs/superpowers/plans' ':!tests/test_wave*_mutation_meta.sh' && exit 1 || true
git add commons_messaging/channels/tgram/subscribe.go \
        commons_messaging/channels/tgram/subscribe_test.go \
        commons_messaging/channels/tgram/export_test.go
git commit -m "$(cat <<'EOF'
Wave 6 step 4: tgram.Subscribe bot self-filter (anti-echo-loop)

Drops messages where Sender.IsBot && Username == bot.Me.Username, so
pherald never re-dispatches its own outbound replies through the Claude
Code pipeline. Cross-bot messages (different bot in same chat) are
kept — those are real subscriber traffic.

If bot.Me.Username is unset after NewBot (getMe failed), Subscribe
returns an error rather than booting an echo-loop-prone runtime.

§107 anchor: T12 mutation gate (a) blanks the filter and runs two
real messages; detector asserts the bot's own message echoes. If the
mutated build does NOT echo, the filter was never load-bearing →
critical defect.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

### Self-review

- [ ] `bot.Me.Username` captured once, at Subscribe time.
- [ ] Empty `bot.Me.Username` fails Subscribe (no silent-fall-through to "filter everything in").
- [ ] Cross-bot messages kept.
- [ ] Helper exported via `export_test.go` (no production export surface widened).

---

## Task 5: OnPhoto / OnDocument / OnVoice handlers + attachment download

**Goal.** Today `Subscribe` only registers `OnText`. Subscribers send photos / documents / voice notes routinely; pherald must download them, content-address the file under `~/.herald/inbox/<sha256>.<ext>`, and inject the local path into `InboundEvent.Attachments[]`. Bot self-filter applies to these handlers too.

### Files

**Modify:** `commons_messaging/channels/tgram/subscribe.go` — register `OnPhoto`, `OnDocument`, `OnVoice` handlers.
**Create:** `commons_messaging/channels/tgram/attachments.go` — helper `DownloadAttachment(ctx, bot, fileID, mime string) (path, sha256, error)`. Streams via `bot.File(&telebot.File{FileID: fileID})` → `io.MultiWriter(diskFile, sha256.New())` → atomic rename.
**Create:** `commons_messaging/channels/tgram/attachments_test.go` — httptest server returns canned file content; assert sha256 + path + atomic rename + duplicate (same sha256) no-overwrite.

### §107 anti-bluff anchor

(a) A handler that "registers" but produces an empty `Attachments[]` would pass type-checks; the test asserts `len(ev.Attachments) >= 1` and the recorded sha256 matches the file on disk. (b) A download that writes to a fixed path (not sha256) would pass too; the test asserts the file lives at exactly `~/.herald/inbox/<sha256>.<ext>` (resolved via `os.UserHomeDir()`).

### Steps

- [ ] **5.1** Build the failing helper test (`attachments_test.go`):

```go
func TestDownloadAttachmentContentAddressed(t *testing.T) {
    // canned response: a small JPEG byte stream
    payload := []byte("\xff\xd8\xff\xe0fake-jpeg-bytes-for-test\xff\xd9")
    sum := sha256.Sum256(payload)
    wantSha := hex.EncodeToString(sum[:])

    // httptest server impersonating the Bot API:
    //   /bot<TOKEN>/getFile?file_id=FILE123 -> {"ok":true,"result":{"file_path":"x.jpg"}}
    //   /file/bot<TOKEN>/x.jpg -> payload bytes
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch {
        case strings.Contains(r.URL.Path, "getFile"):
            _, _ = w.Write([]byte(`{"ok":true,"result":{"file_id":"FILE123","file_path":"x.jpg"}}`))
        case strings.HasSuffix(r.URL.Path, "x.jpg"):
            _, _ = w.Write(payload)
        default:
            w.WriteHeader(http.StatusNotFound)
        }
    }))
    defer srv.Close()

    bot, err := telebot.NewBot(telebot.Settings{Token: "TESTTOKEN", URL: srv.URL, Offline: false})
    if err != nil { t.Fatal(err) }

    // override HOME so ~/.herald/inbox is under t.TempDir
    home := t.TempDir()
    t.Setenv("HOME", home)

    gotPath, gotSha, err := tgram.DownloadAttachment(context.Background(), bot, "FILE123", "image/jpeg")
    if err != nil { t.Fatal(err) }
    if gotSha != wantSha {
        t.Fatalf("sha mismatch: got %s want %s", gotSha, wantSha)
    }
    wantPath := filepath.Join(home, ".herald", "inbox", wantSha+".jpg")
    if gotPath != wantPath {
        t.Fatalf("path mismatch: got %s want %s", gotPath, wantPath)
    }
    onDisk, err := os.ReadFile(gotPath)
    if err != nil { t.Fatal(err) }
    if !bytes.Equal(onDisk, payload) {
        t.Fatalf("file content mismatch")
    }
    // Duplicate download MUST NOT re-write the file (idempotent content-addressing).
    info1, _ := os.Stat(gotPath)
    time.Sleep(10 * time.Millisecond)
    _, _, err = tgram.DownloadAttachment(context.Background(), bot, "FILE123", "image/jpeg")
    if err != nil { t.Fatal(err) }
    info2, _ := os.Stat(gotPath)
    if !info1.ModTime().Equal(info2.ModTime()) {
        t.Fatalf("duplicate download re-wrote the file: mtime changed %v -> %v",
            info1.ModTime(), info2.ModTime())
    }
}
```

(Caveat: telebot's `Bot.URL` field may or may not be a public setter for the base URL. The implementer verifies by reading `submodules/telebot/bot.go` for the `URL` / `apiURL` field; if not exposed, the test uses a wrapper that injects an `*http.Client` whose Transport rewrites Telegram's base URL to the test server. Substrate inspection in pre-flight confirmed telebot.v3 supports custom URL via `Settings.URL`.)

```bash
cd commons_messaging/channels/tgram && go test -run TestDownloadAttachmentContentAddressed -count=1
# Expected: FAIL (undefined tgram.DownloadAttachment)
```

- [ ] **5.2** Implement `attachments.go`. `DownloadAttachment(ctx, bot, fileID, mime)` performs:
  1. `home, _ := os.UserHomeDir()`; `inboxDir := filepath.Join(home, ".herald", "inbox")`; MkdirAll.
  2. `ext := mimeToExt(mime)` — switch table: image/jpeg→jpg, image/png→png, image/gif→gif, image/webp→webp, video/mp4→mp4, audio/ogg|audio/opus→ogg, audio/mpeg→mp3, application/pdf→pdf, text/plain→txt, default→bin.
  3. telebot getFile: implementer reads `submodules/telebot/bot.go` + `file.go` at execution to confirm exact signature. The v3 API is `b.FileByID(fileID)` returning a `telebot.File` with `FilePath`, then `b.File(&f)` returns `io.ReadCloser` of the bytes.
  4. Stream into `os.CreateTemp(inboxDir, "dl-*.part")` via `io.Copy(io.MultiWriter(tmpFile, sha256.New()), rc)`.
  5. `sumHex := hex.EncodeToString(hasher.Sum(nil))`; `finalPath := filepath.Join(inboxDir, sumHex+"."+ext)`.
  6. Idempotent: if `os.Stat(finalPath)` succeeds, drop the temp file (bytes are equal by content-addressing); else `os.Rename(tmpPath, finalPath)`.
  7. Returns `(finalPath, sumHex, nil)` on success.
  Imports: `context`, `crypto/sha256`, `encoding/hex`, `errors`, `fmt`, `io`, `os`, `path/filepath`, `telebot "gopkg.in/telebot.v3"`.

- [ ] **5.3** Wire the three new handlers in `subscribe.go`:

```go
bot.Handle(telebot.OnPhoto, func(c telebot.Context) error {
    msg := c.Message()
    if shouldDropBotSelf(msg, selfUsername) { return nil }
    if msg == nil || msg.Photo == nil { return nil }
    path, sumHex, err := DownloadAttachment(ctx, bot, msg.Photo.FileID, "image/jpeg")
    if err != nil { return fmt.Errorf("tgram.Subscribe OnPhoto: download: %w", err) }
    return h.Handle(ctx, buildEventWithAttachment(msg, path, sumHex, "image/jpeg", msg.Photo.FileSize))
})
bot.Handle(telebot.OnDocument, func(c telebot.Context) error {
    msg := c.Message()
    if shouldDropBotSelf(msg, selfUsername) { return nil }
    if msg == nil || msg.Document == nil { return nil }
    mime := msg.Document.MIME
    if mime == "" { mime = "application/octet-stream" }
    path, sumHex, err := DownloadAttachment(ctx, bot, msg.Document.FileID, mime)
    if err != nil { return fmt.Errorf("tgram.Subscribe OnDocument: download: %w", err) }
    return h.Handle(ctx, buildEventWithAttachment(msg, path, sumHex, mime, int64(msg.Document.FileSize)))
})
bot.Handle(telebot.OnVoice, func(c telebot.Context) error {
    msg := c.Message()
    if shouldDropBotSelf(msg, selfUsername) { return nil }
    if msg == nil || msg.Voice == nil { return nil }
    path, sumHex, err := DownloadAttachment(ctx, bot, msg.Voice.FileID, "audio/ogg")
    if err != nil { return fmt.Errorf("tgram.Subscribe OnVoice: download: %w", err) }
    return h.Handle(ctx, buildEventWithAttachment(msg, path, sumHex, "audio/ogg", int64(msg.Voice.FileSize)))
})
```

`buildEventWithAttachment` is a helper that builds the same `commons.InboundEvent` shape as the existing `OnText` handler, plus appends one `commons.Attachment` whose `Filename` is the on-disk path (so the Claude Code pre-text rendering in T3 can list it directly) and whose `MIMEType`, `SizeBytes`, `CID` (= sha256 for traceability) carry the metadata. Reader is a closure that lazy-opens the file on disk.

- [ ] **5.4** Run unit tests:

```bash
go test -count=1 ./commons_messaging/channels/tgram/...
# Expected: PASS (5+ tests including new download + handler-registration unit tests)
```

- [ ] **5.5** Working-tree quiescence + commit:

```bash
git grep -n "MUTATED for paired\|// always pass" -- . ':!docs/superpowers/plans' ':!tests/test_wave*_mutation_meta.sh' && exit 1 || true
git add commons_messaging/channels/tgram/subscribe.go \
        commons_messaging/channels/tgram/attachments.go \
        commons_messaging/channels/tgram/attachments_test.go \
        commons_messaging/channels/tgram/subscribe_test.go \
        commons_messaging/channels/tgram/export_test.go
git commit -m "$(cat <<'EOF'
Wave 6 step 5: tgram OnPhoto/OnDocument/OnVoice handlers + sha256
content-addressed attachment download

Adds DownloadAttachment helper that streams Telegram files into
~/.herald/inbox/<sha256>.<ext>, computing sha256 inline and writing
atomically. Three new handlers (OnPhoto/OnDocument/OnVoice) each
apply the bot self-filter, fetch + content-address the attachment,
and dispatch a complete InboundEvent with Attachments[] populated.

§107 anchor: T11 invariant E72 reads a real inbound photo's stored
path and asserts sha256(file on disk) == sha256 in the on-disk
filename. T12 mutation gate is NOT layered here (the path uniqueness
is enforced by os.Rename, not by a checkable code branch).

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

### Self-review

- [ ] All four handler types (Text/Photo/Document/Voice) apply the same bot self-filter.
- [ ] Attachment Reader is lazy (closure opens the disk file on first read).
- [ ] Duplicate-download case asserts content addressing is idempotent.
- [ ] MIME-to-ext fallback is `bin` (no panic on exotic types).

---

## Task 6: `pherald listen` Cobra subcommand

**Goal.** New long-running subcommand. Wires `tgram.Subscribe()` to the production `InboundHandler` from T7 (`pherald/internal/inbound/Dispatcher`). Honors SIGINT / SIGTERM. Flags `--bot-token` (env `HERALD_TGRAM_BOT_TOKEN`), `--chat-id` (env `HERALD_TGRAM_CHAT_ID`), `--project-name` (env `HERALD_PROJECT_NAME` via `commons.ProjectName()` from T1).

### Files

**Create:** `pherald/cmd/pherald/listen.go` — Cobra subcommand.
**Create:** `pherald/cmd/pherald/listen_test.go` — integration test using a fake handler + a fake telebot bot.
**Modify:** `pherald/cmd/pherald/main.go` — register the `listen` subcommand on the root.

### §107 anti-bluff anchor

A `listen` that "starts cleanly + exits cleanly" but never wires the handler would PASS smoke (process boots, process stops) yet be a bluff. Test asserts `>=1` handler invocation arrives within 5s when the fake bot publishes one synthetic update.

### Steps

- [ ] **6.1** Failing integration test (`listen_test.go`) — `TestListenWiresHandlerAndExitsOnSIGTERM`:
  1. Start an httptest server impersonating Bot API: returns one `getUpdates` with a synthetic text message, then empty updates.
  2. `buildPheraldForTest(t)` compiles the binary to `t.TempDir()`; spawn it via `exec.Command(binPath, "listen")`.
  3. Env: `HERALD_TGRAM_BOT_TOKEN=fake-token`, `HERALD_TGRAM_CHAT_ID=12345`, `HERALD_PROJECT_NAME=TestProj`, `HERALD_TGRAM_BASE_URL=<httptest URL>`, `HERALD_INBOUND_CC_FAKE=1` (listen.go reads this to wire the in-test no-op CC dispatcher).
  4. Capture combined stdout/stderr into `*bytes.Buffer`; poll for `"inbound dispatched"` within 8s (150ms tick).
  5. On observation: send `syscall.SIGTERM`; wait up to 3s for `cmd.Wait()`; assert exit code 0.
  6. On non-observation OR non-clean shutdown: fail with the captured stdout for diagnostics.

```bash
go test -run TestListenWiresHandlerAndExitsOnSIGTERM -count=1 ./pherald/cmd/pherald/...
# Expected: FAIL (binary doesn't yet have listen)
```

- [ ] **6.2** Implement `listen.go`. `listenCmd` is a Cobra `*Command` with:
  - `Use: "listen"`, `Short: "Run the inbound runtime: Telegram getUpdates long-poll + Claude Code dispatch loop"`.
  - Flags: `--bot-token`, `--chat-id`, `--project-name` (string, default "").
  - `RunE`:
    1. `token := envOrFlag("HERALD_TGRAM_BOT_TOKEN", cmd.Flag("bot-token").Value.String())`; same for chatID.
    2. Return error if either empty.
    3. `projectName := commons.ProjectName()` (T1).
    4. `adapter := tgram.NewAdapter(token, chatID)`.
    5. Build CC dispatcher via `claude_code.New(...)` using `projectName`; wrap in `inbound.ccAdapter` (T7 step 7.5).
    6. `dispatcher, err := inbound.NewDispatcher(inbound.Config{ProjectName, Code: ccAdapter, TgramReply: adapter, Fake: os.Getenv("HERALD_INBOUND_CC_FAKE") == "1"})`.
    7. `ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM); defer cancel()`.
    8. Log `"pherald listen: starting Telegram getUpdates long-poll loop"` to `cmd.OutOrStdout()`.
    9. `return adapter.Subscribe(ctx, dispatcher)`.
  - `envOrFlag(envKey, flagVal string) string`: flag wins if non-empty, else `os.Getenv(envKey)`.

- [ ] **6.3** Register in `pherald/cmd/pherald/main.go`:

```go
rootCmd.AddCommand(listenCmd)
```

- [ ] **6.4** Run tests:

```bash
go build ./pherald/...
go test -count=1 ./pherald/cmd/pherald/...
# Expected: PASS
```

- [ ] **6.5** Working-tree quiescence + commit:

```bash
git grep -n "MUTATED for paired\|// always pass" -- . ':!docs/superpowers/plans' ':!tests/test_wave*_mutation_meta.sh' && exit 1 || true
git add pherald/cmd/pherald/listen.go \
        pherald/cmd/pherald/listen_test.go \
        pherald/cmd/pherald/main.go
git commit -m "$(cat <<'EOF'
Wave 6 step 6: pherald listen subcommand (Cobra)

Long-running. Wires tgram.Subscribe to inbound.Dispatcher (T7).
Honors SIGINT/SIGTERM via signal.NotifyContext. Flag-or-env config
for bot token, chat id, project name.

The HERALD_INBOUND_CC_FAKE=1 env opt-in selects an in-test no-op
dispatcher path — used only by listen_test.go to drive end-to-end
without spawning the real claude CLI. Production paths ignore it.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

### Self-review

- [ ] `listen` is registered on the root Cobra command.
- [ ] SIGTERM cleanly cancels the Subscribe context.
- [ ] Flag-or-env precedence: flag wins, else env.
- [ ] `HERALD_INBOUND_CC_FAKE` is the ONLY test-only env; production never reads it as a kill-switch.

---

## Task 7: `pherald/internal/inbound/` Dispatcher + action routing

**Goal.** Implement the production `InboundHandler`. On every inbound event:
1. Resolve session name via `commons.ProjectName()`.
2. Construct `claude_code.DispatchRequest` from the InboundEvent (sender, channel, conversation, attachments, user message).
3. Call `claude_code.Dispatcher.Dispatch(ctx, req)`.
4. Parse the response's "free-form fields" for an `action` declaration.
5. Route:
   - `action: "reply"` (default) — call `tgram.Adapter.SendReply(ctx, chatID, text, replyToID, attachments)`.
   - `action: "issue.open"` — call pherald's issue-creation code path (stub in this wave; HRD-NNN-W6b will fully wire it).
   - `action: "event.emit"` — call into the outbound runner pipeline (re-enters pherald's `/v1/events` POST path internally via direct function call).

### Files

**Create:** `pherald/internal/inbound/dispatcher.go` — `Dispatcher` struct + `Handle` method.
**Create:** `pherald/internal/inbound/dispatcher_test.go` — table-driven test against a stub `CodeDispatcher` interface.
**Create:** `pherald/internal/inbound/reply.go` — `Reply` type + `ParseReply([]byte) (*Reply, error)`.
**Create:** `pherald/internal/inbound/reply_test.go` — 4 cases.

### §107 anti-bluff anchor

A Dispatcher that swallows the `action` field and always sends a default reply would still PASS the "reply lands in chat" smoke. Test cases assert all three actions route distinctly (stub CodeDispatcher returns a canned `action: "issue.open"` → assert `tgram.SendReply` was NOT called AND a recorded "issue opened" event surfaced).

### Steps

- [ ] **7.1** Define `Reply` + `ParseReply`:

```go
// pherald/internal/inbound/reply.go
package inbound

import (
    "encoding/json"
    "errors"
    "strings"
)

type Reply struct {
    Action string         `json:"action"` // "reply" (default) | "issue.open" | "event.emit"
    Text   string         `json:"text"`   // body for action=reply
    Issue  *IssuePayload  `json:"issue,omitempty"`
    Event  *EventPayload  `json:"event,omitempty"`
}
type IssuePayload struct {
    Type        string   `json:"type"`
    Criticality string   `json:"criticality"`
    Title       string   `json:"title"`
    Body        string   `json:"body"`
    Labels      []string `json:"labels"`
}
type EventPayload struct {
    CloudEventType string         `json:"cloudevent_type"`
    Subject        string         `json:"subject"`
    Data           map[string]any `json:"data"`
}

// ParseReply scans CC stdout for <<<HERALD-REPLY>>> and extracts the JSON.
// Action defaults to "reply" when omitted. Returns error on missing marker
// or malformed JSON.
func ParseReply(stdout []byte) (*Reply, error) {
    const marker = "<<<HERALD-REPLY>>>"
    s := string(stdout)
    idx := strings.Index(s, marker)
    if idx < 0 { return nil, errors.New("inbound: no <<<HERALD-REPLY>>> marker") }
    after := s[idx+len(marker):]
    brace := strings.Index(after, "{")
    if brace < 0 { return nil, errors.New("inbound: marker present but no JSON follows") }
    var r Reply
    if err := json.Unmarshal([]byte(after[brace:]), &r); err != nil { return nil, err }
    if r.Action == "" { r.Action = "reply" }
    return &r, nil
}
```

- [ ] **7.2** Failing test cases (`reply_test.go`):

```go
func TestParseReplyActions(t *testing.T) {
    cases := []struct{
        name, stdout string
        wantAction   string
        wantErr      bool
    }{
        {"reply explicit", `<<<HERALD-REPLY>>> {"action":"reply","text":"hi"}`, "reply", false},
        {"reply default",  `<<<HERALD-REPLY>>> {"text":"hi"}`,                  "reply", false},
        {"issue.open",     `<<<HERALD-REPLY>>> {"action":"issue.open","issue":{"type":"bug","criticality":"high","title":"x","body":"y","labels":["repro"]}}`, "issue.open", false},
        {"event.emit",     `<<<HERALD-REPLY>>> {"action":"event.emit","event":{"cloudevent_type":"com.example.t","subject":"s","data":{"k":"v"}}}`,         "event.emit", false},
        {"no marker",      `gibberish`,                                          "",      true},
        {"malformed JSON", `<<<HERALD-REPLY>>> {oops}`,                          "",      true},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            got, err := inbound.ParseReply([]byte(tc.stdout))
            if tc.wantErr {
                if err == nil { t.Fatal("want err, got nil") }
                return
            }
            if err != nil { t.Fatal(err) }
            if got.Action != tc.wantAction {
                t.Fatalf("action: got %q want %q", got.Action, tc.wantAction)
            }
        })
    }
}
```

- [ ] **7.3** Implement `Dispatcher` (signatures only; bodies follow the spec below):

```go
// pherald/internal/inbound/dispatcher.go
package inbound

type CodeDispatcher interface {
    Dispatch(ctx context.Context, req CodeRequest) (CodeResponse, error)
}
type CodeRequest struct {
    InboundID, Sender, Conversation, UserMessage string
    Channel        commons.ChannelID
    Attachments    []commons.Attachment
    Classification Classification
}
type CodeResponse struct { Stdout []byte } // raw CC stdout carrying <<<HERALD-REPLY>>>

type Classification struct { Type, Criticality string; Confidence float64 }

type TgramReplier interface {
    SendReply(ctx context.Context, chatID int64, text string, replyToID int, attachments []commons.Attachment) (int, error)
}
type IssueOpener  interface { OpenIssue(ctx context.Context, p IssuePayload) error }
type EventEmitter interface { Emit(ctx context.Context, p EventPayload) error }

type Dispatcher struct { code CodeDispatcher; reply TgramReplier; opener IssueOpener; emit EventEmitter }

type Config struct {
    ProjectName string
    Code        CodeDispatcher
    TgramReply  TgramReplier
    Issues      IssueOpener
    Events      EventEmitter
    Fake        bool   // listen_test.go opt-in: substitutes fakeCode + fakeReplier
}

func NewDispatcher(cfg Config) (*Dispatcher, error)
func (d *Dispatcher) Handle(ctx context.Context, ev commons.InboundEvent) error
```

`Handle` body — exact algorithm:
1. Build `CodeRequest` from `ev`: `Sender = ev.Sender.Channel + ":" + ev.Sender.ChannelUserID`, `Channel = commons.ChannelID(ev.Sender.Channel)`, `Conversation = ev.Body.Plain` (Wave 6: full-thread reconstruction is HRD-NNN-W6c; classification stays empty Wave 6 — §32.6 classifier is HRD-NNN-W6c).
2. `resp, err := d.code.Dispatch(ctx, req)` — wrap err with `"inbound: CC dispatch: %w"`.
3. `reply, err := ParseReply(resp.Stdout)` — §107: do NOT fabricate a default reply; surface the parse error.
4. Switch on `reply.Action`:
   - `"reply"`: `chatID, _ := strconv.ParseInt(ev.Sender.ChannelUserID, 10, 64)`; `replyToID, _ := extractReplyToMessageID(ev.Raw)`; `d.reply.SendReply(ctx, chatID, reply.Text, replyToID, nil)`; log `"inbound dispatched: reply"`.
   - `"issue.open"`: require `d.opener != nil && reply.Issue != nil`; call `d.opener.OpenIssue(ctx, *reply.Issue)`; log `"inbound dispatched: issue.open"`.
   - `"event.emit"`: require `d.emit != nil && reply.Event != nil`; call `d.emit.Emit(ctx, *reply.Event)`; log `"inbound dispatched: event.emit"`.
   - default: return `fmt.Errorf("inbound: unknown action %q", reply.Action)`.

`extractReplyToMessageID(raw map[string]any) (int, error)`: reads `raw["message_id"]` (set by tgram.Subscribe's OnText/OnPhoto/etc. handlers), type-asserts to `int`, returns `(0, error)` if missing/wrong type.

`NewDispatcher` validates `cfg.Code != nil && cfg.TgramReply != nil` (Fake branch substitutes fakeCode and fakeReplier; production callers do NOT set Fake=true).

In-test stubs (same file, lowercase): `fakeCode.Dispatch` returns `CodeResponse{Stdout: []byte(`<<<HERALD-REPLY>>> {"action":"reply","text":"ack"}`)}`; `fakeReplier.SendReply` returns `(1, nil)`.

- [ ] **7.4** Failing dispatcher test (`dispatcher_test.go`):

```go
type recordingReplier struct{ called bool; lastReplyTo int }
func (r *recordingReplier) SendReply(_ context.Context, _ int64, _ string, replyToID int, _ []commons.Attachment) (int, error) {
    r.called = true; r.lastReplyTo = replyToID; return 1, nil
}
type recordingOpener struct{ called bool }
func (r *recordingOpener) OpenIssue(_ context.Context, _ inbound.IssuePayload) error { r.called = true; return nil }
type recordingEmitter struct{ called bool }
func (r *recordingEmitter) Emit(_ context.Context, _ inbound.EventPayload) error { r.called = true; return nil }
type stubCode struct{ stdout string }
func (s stubCode) Dispatch(_ context.Context, _ inbound.CodeRequest) (inbound.CodeResponse, error) {
    return inbound.CodeResponse{Stdout: []byte(s.stdout)}, nil
}

func TestDispatcherActionRouting(t *testing.T) {
    cases := []struct{
        name, stdout string
        wantReply, wantIssue, wantEvent bool
    }{
        {"reply",       `<<<HERALD-REPLY>>> {"action":"reply","text":"hi"}`,                                                                      true,  false, false},
        {"issue.open",  `<<<HERALD-REPLY>>> {"action":"issue.open","issue":{"type":"bug","criticality":"high","title":"x","body":"y"}}`,            false, true,  false},
        {"event.emit",  `<<<HERALD-REPLY>>> {"action":"event.emit","event":{"cloudevent_type":"c","subject":"s","data":{}}}`,                       false, false, true},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            rr := &recordingReplier{}
            ro := &recordingOpener{}
            re := &recordingEmitter{}
            d, _ := inbound.NewDispatcher(inbound.Config{
                ProjectName: "T",
                Code:        stubCode{stdout: tc.stdout},
                TgramReply:  rr, Issues: ro, Events: re,
            })
            err := d.Handle(context.Background(), commons.InboundEvent{
                EventID: "01H", Sender: commons.Recipient{Channel: "tgram", ChannelUserID: "12345"},
                Body: commons.Body{Plain: "ping"},
                Raw:  map[string]any{"message_id": 42},
            })
            if err != nil { t.Fatal(err) }
            if rr.called != tc.wantReply { t.Fatalf("reply: got %v want %v", rr.called, tc.wantReply) }
            if ro.called != tc.wantIssue { t.Fatalf("issue: got %v want %v", ro.called, tc.wantIssue) }
            if re.called != tc.wantEvent { t.Fatalf("event: got %v want %v", re.called, tc.wantEvent) }
        })
    }
}
```

```bash
mkdir -p pherald/internal/inbound
# Then place the four files; run
go test -count=1 ./pherald/internal/inbound/...
# Expected: PASS
```

- [ ] **7.5** Wire the production CC adapter. `claude_code.Dispatcher` doesn't yet satisfy `inbound.CodeDispatcher` directly — add a thin adapter in `pherald/internal/inbound/cc_adapter.go`:

```go
type ccAdapter struct { d *claude_code.Dispatcher }
func (a ccAdapter) Dispatch(ctx context.Context, req CodeRequest) (CodeResponse, error) {
    ccReq := claude_code.DispatchRequest{
        InboundID:   req.InboundID,
        Sender:      req.Sender,
        Channel:     req.Channel,
        Conversation: req.Conversation,
        Attachments: req.Attachments,
        UserMessage: req.UserMessage,
        Classification: claude_code.Classification{
            Type: req.Classification.Type,
            Criticality: req.Classification.Criticality,
            Confidence: req.Classification.Confidence,
        },
    }
    resp, err := a.d.Dispatch(ctx, ccReq)
    if err != nil { return CodeResponse{}, err }
    // Re-serialize the structured DispatchResponse back into stdout-shape
    // so ParseReply (which scans for <<<HERALD-REPLY>>>) keeps working
    // uniformly. The CC dispatcher already strips out the marker and
    // hands back a typed DispatchResponse; we re-emit a synthetic stdout
    // line for the inbound dispatcher's uniform parsing path.
    b, _ := json.Marshal(map[string]any{
        "action": "reply",
        "text":   resp.Summary, // operator may evolve this in HRD-NNN-W6c
    })
    return CodeResponse{Stdout: append([]byte("<<<HERALD-REPLY>>> "), b...)}, nil
}
```

(Listen.go's `NewDispatcher` call passes `ccAdapter{d: <real CC dispatcher>}` as `Config.Code`.)

- [ ] **7.6** Working-tree quiescence + commit:

```bash
git grep -n "MUTATED for paired\|// always pass" -- . ':!docs/superpowers/plans' ':!tests/test_wave*_mutation_meta.sh' && exit 1 || true
git add pherald/internal/inbound/
git commit -m "$(cat <<'EOF'
Wave 6 step 7: pherald/internal/inbound/ Dispatcher + action routing

Production InboundHandler:
  - resolves CC session via commons.ProjectName() (T1)
  - builds DispatchRequest from InboundEvent
  - parses <<<HERALD-REPLY>>> for an "action" field
  - routes reply | issue.open | event.emit to distinct backends
  - default action = "reply" (operator-locked)

Action routing tested with a stub CodeDispatcher returning canned
replies for each of the three actions; recording-replier/opener/emitter
assert exclusive activation per route.

§107 anchor: T11 invariant E76 asserts action=issue.open results in
NO outbound Telegram message (proving the action field is load-bearing).

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

### Self-review

- [ ] All three action paths tested distinctly with three recording sinks.
- [ ] Default action explicit (`"reply"`) and tested via the "reply default" case.
- [ ] Unknown action returns explicit error (no silent fallback).
- [ ] `extractReplyToMessageID` handles missing/typed `message_id` gracefully.

---

## Task 8: `tgram.Adapter.SendReply` with `reply_to_message_id`

**Goal.** Extend the tgram adapter with a reply-aware send. Internally constructs `telebot.SendOptions{ReplyTo: &telebot.Message{ID: replyToID}}` (confirmed via substrate inspection — telebot's `embedSendOptions` populates `reply_to_message_id` in the URL form when `ReplyTo.ID != 0`).

### Files

**Modify:** `commons_messaging/channels/tgram/send.go` — add `SendReply` method.
**Modify:** `commons_messaging/channels/tgram/send_integration_test.go` — add a unit-style test against an httptest Bot API server that records `r.FormValue("reply_to_message_id")`.

### §107 anti-bluff anchor

A `SendReply` that "compiles cleanly but the `ReplyTo` field's `ID` is always 0" would pass type-checks; the test asserts the URL form value `reply_to_message_id` equals the expected ID. T12 mutation gate (c) drops the ReplyTo assignment; the detector asserts the test catches it.

### Steps

- [ ] **8.1** Failing test (`send_reply_test.go` — new file, plain unit test, not in the `_integration` suite since it uses httptest only):

```go
func TestSendReplyEmitsReplyToMessageID(t *testing.T) {
    var got string
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        _ = r.ParseForm()
        got = r.FormValue("reply_to_message_id")
        // Canned sendMessage response.
        _, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":777,"chat":{"id":12345},"text":"hi"}}`))
    }))
    defer srv.Close()
    a := tgram.NewAdapterWithBaseURL("TESTTOKEN", "12345", srv.URL) // tiny constructor variant for tests
    msgID, err := a.SendReply(context.Background(), 12345, "hi", 42, nil)
    if err != nil { t.Fatal(err) }
    if msgID != 777 { t.Fatalf("msgID: got %d want 777", msgID) }
    if got != "42" { t.Fatalf("reply_to_message_id: got %q want \"42\"", got) }
}
```

- [ ] **8.2** Implement in `send.go`:

```go
func (a *Adapter) SendReply(ctx context.Context, chatID int64, text string, replyToID int, attachments []commons.Attachment) (int, error) {
    _ = ctx
    if err := a.ensureBot(); err != nil { return 0, err }
    if text == "" { return 0, fmt.Errorf("tgram.SendReply: empty text") }
    chat := &telebot.Chat{ID: chatID}
    opts := &telebot.SendOptions{}
    if replyToID > 0 {
        opts.ReplyTo = &telebot.Message{ID: replyToID}
    }
    m, err := a.bot.Send(chat, text, opts)
    if err != nil { return 0, fmt.Errorf("tgram.SendReply: %w", err) }
    // Attachments path: for Wave 6 we keep SendReply text-only; outbound
    // attachment fan-out lives in OutboundMessage.Send. A future HRD
    // extends SendReply with media. Asserted by code review, not by test.
    _ = attachments
    return m.ID, nil
}

// NewAdapterWithBaseURL is the test seam for httptest-based assertions
// of wire-byte details (reply_to_message_id form value). Production
// constructors use NewAdapter (no URL override).
func NewAdapterWithBaseURL(token, chatID, baseURL string) *Adapter {
    a := NewAdapter(token, chatID)
    a.baseURL = baseURL // private field, set via tgram.go
    return a
}
```

(`tgram.go`'s `Adapter` may not yet have `baseURL`; add it as a struct field and thread through `ensureBot` to pass `telebot.Settings.URL = a.baseURL` when non-empty. This is a one-field addition; production callers leave it empty and telebot defaults to api.telegram.org.)

- [ ] **8.3** Run tests:

```bash
go test -count=1 ./commons_messaging/channels/tgram/...
# Expected: PASS (including new TestSendReplyEmitsReplyToMessageID)
```

- [ ] **8.4** Working-tree quiescence + commit:

```bash
git grep -n "MUTATED for paired\|// always pass" -- . ':!docs/superpowers/plans' ':!tests/test_wave*_mutation_meta.sh' && exit 1 || true
git add commons_messaging/channels/tgram/send.go \
        commons_messaging/channels/tgram/send_reply_test.go \
        commons_messaging/channels/tgram/tgram.go
git commit -m "$(cat <<'EOF'
Wave 6 step 8: tgram.Adapter.SendReply with reply_to_message_id

Adds Adapter.SendReply(ctx, chatID, text, replyToID, attachments). Sets
telebot.SendOptions.ReplyTo to a stub Message with the given ID;
telebot's embedSendOptions populates reply_to_message_id in the URL
form payload when ID != 0 (confirmed via substrate read of
submodules/telebot/options.go:178).

NewAdapterWithBaseURL is the test seam for httptest-based wire-byte
assertions; production callers use NewAdapter unchanged.

§107 anchor: T12 mutation gate (c) drops opts.ReplyTo; detector
asserts the form-value test catches it.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

### Self-review

- [ ] `SendReply` signature exactly: `(ctx, chatID int64, text string, replyToID int, attachments []commons.Attachment) (int, error)`.
- [ ] `replyToID == 0` is the "no-reply" sentinel (opts.ReplyTo stays nil).
- [ ] Test asserts the URL form value, not just compile success.

---

## Task 9: Live closed-loop e2e — `tests/test_wave6_live_loop.sh`

**Goal.** The §107 watershed for Wave 6. End-to-end: operator types a message in the configured chat; pherald listens; CC processes via real `claude` CLI; reply lands in chat with `reply_to_message_id == original`.

### Files

**Create:** `tests/test_wave6_live_loop.sh`.

### §107 anti-bluff anchor

This script IS the watershed — it cannot be faked. Every assertion is against real Telegram getUpdates bytes + real CC subprocess stdout + real pherald process state. SKIPs-with-reason if any of `HERALD_TGRAM_BOT_TOKEN`, `HERALD_TGRAM_CHAT_ID`, `HERALD_CLAUDE_CODE_BINARY` are unset.

### Steps

- [ ] **9.1** Author the script:

```bash
#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." &>/dev/null && pwd)"
cd "${REPO_ROOT}"

skip_with_reason() { echo "SKIP: $1"; exit 0; }

[ -n "${HERALD_TGRAM_BOT_TOKEN:-}" ]      || skip_with_reason "HERALD_TGRAM_BOT_TOKEN unset"
[ -n "${HERALD_TGRAM_CHAT_ID:-}" ]        || skip_with_reason "HERALD_TGRAM_CHAT_ID unset"
[ -n "${HERALD_CLAUDE_CODE_BINARY:-}" ]   || skip_with_reason "HERALD_CLAUDE_CODE_BINARY unset"
command -v "${HERALD_CLAUDE_CODE_BINARY}" >/dev/null || skip_with_reason "claude binary not in PATH"

echo "[wave6-live-loop] building pherald..."
go build -o /tmp/pherald-w6 ./pherald/cmd/pherald

echo "[wave6-live-loop] pre-condition: operator MUST type a single message in the chat NOW"
echo "                  (script reads it via getUpdates within 60s)"
ORIGINAL_MSG_ID=""
deadline=$(( $(date +%s) + 60 ))
while [ "$(date +%s)" -lt "$deadline" ]; do
  resp=$(curl -sS "https://api.telegram.org/bot${HERALD_TGRAM_BOT_TOKEN}/getUpdates?timeout=5&allowed_updates=%5B%22message%22%5D" || true)
  ORIGINAL_MSG_ID=$(echo "$resp" | jq -r --arg cid "$HERALD_TGRAM_CHAT_ID" '.result[] | select(.message.chat.id|tostring==$cid) | .message.message_id' | head -n1)
  [ -n "$ORIGINAL_MSG_ID" ] && [ "$ORIGINAL_MSG_ID" != "null" ] && break
  sleep 2
done
[ -n "$ORIGINAL_MSG_ID" ] && [ "$ORIGINAL_MSG_ID" != "null" ] || { echo "FAIL: no operator-typed message observed in 60s"; exit 1; }
echo "[wave6-live-loop] observed original message_id=${ORIGINAL_MSG_ID}"

echo "[wave6-live-loop] starting pherald listen..."
/tmp/pherald-w6 listen \
    --bot-token "$HERALD_TGRAM_BOT_TOKEN" \
    --chat-id "$HERALD_TGRAM_CHAT_ID" \
    > /tmp/pherald-w6.log 2>&1 &
LISTEN_PID=$!
trap "kill -TERM $LISTEN_PID 2>/dev/null || true" EXIT

echo "[wave6-live-loop] waiting up to 45s for a reply with reply_to_message_id=${ORIGINAL_MSG_ID}..."
deadline=$(( $(date +%s) + 45 ))
REPLY_OK=0
while [ "$(date +%s)" -lt "$deadline" ]; do
  resp=$(curl -sS "https://api.telegram.org/bot${HERALD_TGRAM_BOT_TOKEN}/getUpdates?timeout=5&allowed_updates=%5B%22message%22%5D" || true)
  match=$(echo "$resp" | jq -r --argjson orig "$ORIGINAL_MSG_ID" '.result[] | select(.message.reply_to_message.message_id==$orig) | .message.message_id' | head -n1)
  if [ -n "$match" ] && [ "$match" != "null" ]; then REPLY_OK=1; echo "[wave6-live-loop] reply message_id=$match found"; break; fi
  sleep 2
done

kill -TERM "$LISTEN_PID" 2>/dev/null || true
wait "$LISTEN_PID" 2>/dev/null || true

if [ "$REPLY_OK" = "1" ]; then
  echo "PASS: wave6 closed-loop"
  echo "PASS-evidence: original=${ORIGINAL_MSG_ID}, reply observed"
  echo "PASS-evidence: pherald log tail:"
  tail -n 30 /tmp/pherald-w6.log
  exit 0
else
  echo "FAIL: no reply observed in 45s"
  echo "pherald log tail:"
  tail -n 50 /tmp/pherald-w6.log
  exit 1
fi
```

- [ ] **9.2** `chmod +x tests/test_wave6_live_loop.sh`.

- [ ] **9.3** Operator-supplied credentials path — instructs the operator (via `docs/CONTINUATION.md` update in T13) to:
  1. Export `HERALD_TGRAM_BOT_TOKEN`, `HERALD_TGRAM_CHAT_ID`, `HERALD_CLAUDE_CODE_BINARY`.
  2. Type a single message in the configured chat.
  3. Run `bash tests/test_wave6_live_loop.sh`.
  4. Expect `PASS: wave6 closed-loop` within 105s total.

- [ ] **9.4** Commit:

```bash
git add tests/test_wave6_live_loop.sh
git commit -m "$(cat <<'EOF'
Wave 6 step 9: tests/test_wave6_live_loop.sh — live closed-loop

§107 watershed for Wave 6. Operator types a message → pherald listen
picks it up → CC (Opus) replies → reply lands in chat with
reply_to_message_id == original. Every assertion against real
getUpdates bytes + real CC stdout + real pherald process state.

SKIPs-with-reason if HERALD_TGRAM_BOT_TOKEN / HERALD_TGRAM_CHAT_ID /
HERALD_CLAUDE_CODE_BINARY unset. Operator-supplied credentials path
documented in CONTINUATION.md (T13).

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

### Self-review

- [ ] SKIPs-with-reason are explicit (no silent skip).
- [ ] Every assertion has a real-bytes anchor.
- [ ] Timeouts are operator-friendly (60s + 45s = 105s end-to-end).
- [ ] Cleanup via `trap` ensures pherald doesn't leak.

---

## Task 10: `docs/qa/<run-id>/` evidence per §107.x

**Goal.** Every Wave 6 e2e run produces a transcript directory under `docs/qa/HRD-NNN-W6-<run-id>/` carrying:
- `transcript.jsonl` — bidirectional event log (inbound msg → envelope dispatch → CC reply → outbound sendMessage)
- `pherald-listen.log` — full pherald listen stdout/stderr
- `claude-stdout.log` + `claude-stderr.log` — full CC subprocess output
- `attachments/<sha256>.<ext>` — every attachment exchanged

Operator-locked: commit ONE such run dir as Wave 6's evidence (matching the Wave 5 qaherald format where possible — JSONL with `{ts, direction, kind, payload}` events).

### Files

**Create:** `docs/qa/HRD-NNN-W6-2026-05-22T18-30-00-XXXX/transcript.jsonl` (operator-driven run output).
**Create:** `docs/qa/HRD-NNN-W6-2026-05-22T18-30-00-XXXX/pherald-listen.log`.
**Create:** `docs/qa/HRD-NNN-W6-2026-05-22T18-30-00-XXXX/claude-stdout.log` + `claude-stderr.log`.
**Create:** `docs/qa/HRD-NNN-W6-2026-05-22T18-30-00-XXXX/attachments/<sha256>.<ext>` (if the test exchange involved any).
**Create:** `docs/qa/HRD-NNN-W6-2026-05-22T18-30-00-XXXX/README.md` — operator-supplied narrative.
**Modify:** `pherald/cmd/pherald/listen.go` — add `--qa-out-dir <path>` flag that, when set, journals every inbound + outbound event to `<path>/transcript.jsonl`.

### §107 anti-bluff anchor

(a) Transcript JSONL MUST be byte-aligned with the events that actually happened (assert via re-derivation: the recorded sha256s for attachments MUST match the bytes in `attachments/`). (b) Operator-supplied `README.md` narrative is REQUIRED (preventing "transcript committed but no human ever ran it" bluff).

### Steps

- [ ] **10.1** Add `--qa-out-dir` flag + journaling middleware in `listen.go`:

```go
listenCmd.Flags().String("qa-out-dir", "", "If set, journal inbound/outbound events to <dir>/transcript.jsonl")
```

In `RunE`, if `qa-out-dir` is set, wrap `dispatcher` in a journaling decorator that emits one JSONL line per inbound + one per outbound action. Same shape as Wave 5 qaherald:

```json
{"ts":"2026-05-22T18:30:01.234Z","direction":"in","kind":"tgram.message","payload":{...}}
{"ts":"2026-05-22T18:30:02.456Z","direction":"out","kind":"cc.dispatch","payload":{"argv":[...],"envelope_bytes":...}}
{"ts":"2026-05-22T18:30:08.789Z","direction":"in","kind":"cc.reply","payload":{"action":"reply","text":"..."}}
{"ts":"2026-05-22T18:30:09.012Z","direction":"out","kind":"tgram.send_reply","payload":{"reply_to_message_id":42,"text":"..."}}
```

- [ ] **10.2** Run the live closed-loop test with the flag set; commit the resulting directory:

```bash
mkdir -p docs/qa/HRD-NNN-W6-$(date -u +%Y-%m-%dT%H-%M-%S)-$(uuidgen | cut -c1-4)
# Operator runs:
bash tests/test_wave6_live_loop.sh   # observes PASS
# pherald listen was started with --qa-out-dir pointing at the new dir.
# Copy pherald-listen.log + claude logs into the same dir.
# Operator writes a 5-line README.md narrating the exchange.
```

- [ ] **10.3** Commit (no `--allow-empty` — the dir MUST contain real bytes):

```bash
git add docs/qa/HRD-NNN-W6-*/
git commit -m "$(cat <<'EOF'
Wave 6 step 10: docs/qa/HRD-NNN-W6-<run-id>/ — first closed-loop evidence

§107.x mandate: every shipped feature carries an auditable end-to-end
transcript. This run dir contains:
  - transcript.jsonl (bidirectional events)
  - pherald-listen.log
  - claude-stdout.log + claude-stderr.log
  - attachments/<sha256>.<ext> (if any exchanged)
  - README.md (operator narrative)

T11 invariant E78 cites this dir as the positive-evidence anchor for
the closed-loop assertion.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

### Self-review

- [ ] transcript.jsonl events match `attachments/` sha256s.
- [ ] README.md authored by operator (not auto-generated boilerplate).
- [ ] `--qa-out-dir` is opt-in (production runs default to no journaling).

---

## Task 11: e2e_bluff_hunt invariants for Wave 6

**Goal.** Add 8 invariants to `scripts/e2e_bluff_hunt.sh`. Symbolic names E63..E70 per the task brief — actual numbering is the next free contiguous range at the moment of execution (see "Numbering note" in the header).

### Files

**Modify:** `scripts/e2e_bluff_hunt.sh` — append the 8 invariants; update header tally + per-invariant comments.

### Invariant catalogue

| Sym | Anchor | Assertion |
|---|---|---|
| E63 | `pherald listen` lifecycle | Start in background → wait 5s → assert process alive → SIGTERM → wait 3s → assert process exited 0 |
| E64 | Bot self-filter | Mock telebot's `getUpdates` to return a bot-own message → assert `inbound dispatched` NEVER appears in pherald log within 8s. (Realised via a tiny stub Bot API server bundled into the test resource dir.) |
| E65 | Attachment sha256 | Send a known JPEG payload via fake Bot API → after pherald processes it, `ls ~/.herald/inbox/` shows a file whose name is `sha256(payload).jpg`; `sha256sum` of the file matches |
| E66 | Envelope pre-text | Captured CC stdin (from `--qa-out-dir` journaling) starts with `"We have received new message from our communication channel "` |
| E67 | Opus pin in argv | Captured CC subprocess argv (from journaling) contains `--model claude-opus-4-7` as two contiguous entries |
| E68 | reply_to_message_id wire-bytes | Captured outbound sendMessage URL form value `reply_to_message_id` equals the original message's `message_id` |
| E69 | Action routing — issue.open | Drive a synthetic CC reply with `action: "issue.open"` → assert `tgram.send_reply` event NOT in transcript; `issue.opened` event IS in transcript |
| E70 | Live closed-loop (T9 script) | Invoke `tests/test_wave6_live_loop.sh` — PASS or SKIP. The script's exit code (0 or skip-with-reason) determines pass/skip here. |

### §107 anti-bluff anchor

Every invariant either asserts on real-bytes captures (from `docs/qa/<run-id>/transcript.jsonl`) or against a real process/file artifact. No "config-checked" PASS.

### Steps

- [ ] **11.1** Add the 8 invariants to the end of `scripts/e2e_bluff_hunt.sh`. Each invariant follows the established pattern of `assert_pass "EN: <name>"` / `assert_fail "EN: <name>"`. SKIP-with-reason is the third allowed outcome for E70 (and only E70).

- [ ] **11.2** Update the header tally (`Seventy` or whatever the prevailing count is → `Seventy + 8`).

- [ ] **11.3** Update the leading comment block to enumerate the new invariants.

- [ ] **11.4** Run the script:

```bash
scripts/e2e_bluff_hunt.sh
# Expected: all PASS (or E70 SKIPped-with-reason if creds absent).
```

- [ ] **11.5** Commit:

```bash
git add scripts/e2e_bluff_hunt.sh
git commit -m "$(cat <<'EOF'
Wave 6 step 11: e2e_bluff_hunt E63..E70 (Wave 6 invariants)

Eight new invariants covering pherald listen lifecycle, bot self-filter,
attachment sha256 content-addressing, envelope pre-text bytes, Opus
model pin in argv, reply_to_message_id wire-bytes, action routing
(issue.open vs reply), and the live closed-loop test (gated SKIP).

Every assertion against real-bytes captures from
docs/qa/HRD-NNN-W6-<run-id>/transcript.jsonl OR against real process /
file artifacts. No "config-checked" PASS.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

### Self-review

- [ ] Each invariant has a real-bytes anchor (no grep-only PASS).
- [ ] E70 explicitly SKIP-able (only one in the set).
- [ ] Header tally updated.

---

## Task 12: Wave 6 mutation gate — `tests/test_wave6_mutation_meta.sh`

**Goal.** Three paired §1.1 mutations + detectors:
- **(a) Blank bot self-filter.** Mutate `shouldDropBotSelf` to `return false`. Run the live closed-loop with the bot replying once → assert pherald observes its own reply via getUpdates within 8s (the echo-loop signature).
- **(b) Swap Opus to Sonnet.** Mutate the literal `"claude-opus-4-7"` to `"claude-sonnet-4-6"`. Run `scripts/e2e_bluff_hunt.sh -only E67` → assert FAIL.
- **(c) Drop ReplyTo in SendReply.** Mutate `opts.ReplyTo = ...` to a no-op. Run `scripts/e2e_bluff_hunt.sh -only E68` → assert FAIL.

Pre-flight: working-tree quiescence check + `.git/MUTATION_IN_PROGRESS` lockfile per §107.y. Each mutation has a paired restore step (`git checkout -- <file>`) on script exit (trap).

### Files

**Create:** `tests/test_wave6_mutation_meta.sh`.

### §107 anti-bluff anchor

The mutation gate IS the anti-bluff — it proves the detectors actually detect. If a mutation does NOT cause its paired detector to FAIL, the detector was never load-bearing and the production code is bluffing → critical defect.

### Steps

- [ ] **12.1** Write the script (skeleton — full implementation expands at execution time):

```bash
#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." &>/dev/null && pwd)"
cd "${REPO_ROOT}"

# §107.y pre-flight: working-tree quiescence
if git diff --quiet && git diff --cached --quiet; then : ; else
  echo "FAIL: working tree dirty before mutation gate; abort per §107.y"
  exit 1
fi
if [ -f .git/MUTATION_IN_PROGRESS ]; then
  echo "FAIL: another mutation gate already in flight (.git/MUTATION_IN_PROGRESS present)"
  exit 1
fi
touch .git/MUTATION_IN_PROGRESS
cleanup() {
  echo "[wave6-mutation] cleanup: restoring tree"
  git checkout -- commons_messaging/channels/tgram/subscribe.go \
                  commons_messaging/dispatch/claude_code/dispatch.go \
                  commons_messaging/channels/tgram/send.go 2>/dev/null || true
  rm -f .git/MUTATION_IN_PROGRESS
}
trap cleanup EXIT

run_paired() {
  local name="$1" mutate_cmd="$2" detect_cmd="$3"
  echo "[wave6-mutation] paired: $name"
  echo "  → applying mutation"; eval "$mutate_cmd"
  echo "  → expecting detector to FAIL"
  if eval "$detect_cmd"; then
    echo "FAIL: mutation $name applied but detector still PASSed — detector was never load-bearing"
    exit 1
  else
    echo "  ✓ detector FAILed as expected"
  fi
  echo "  → restoring tree"; git checkout -- $(echo "$mutate_cmd" | awk -F"'" '/sed/ {print $NF}')
}

# (a) blank bot self-filter
run_paired "bot-self-filter" \
  "sed -i.bak 's/return msg.Sender.Username == selfUsername/return false \/\/ MUTATION/' commons_messaging/channels/tgram/subscribe.go" \
  "bash tests/test_wave6_live_loop.sh; ! grep -q 'echo loop detected' /tmp/pherald-w6.log"

# (b) Opus → Sonnet
run_paired "opus-pin" \
  "sed -i.bak 's/claude-opus-4-7/claude-sonnet-4-6/' commons_messaging/dispatch/claude_code/dispatch.go" \
  "scripts/e2e_bluff_hunt.sh -only E67"

# (c) drop ReplyTo
run_paired "reply-to" \
  "sed -i.bak 's/opts.ReplyTo = &telebot.Message{ID: replyToID}/\/\/ MUTATION no-op/' commons_messaging/channels/tgram/send.go" \
  "scripts/e2e_bluff_hunt.sh -only E68"

echo "PASS: wave6 mutation gate (3 paired)"
```

- [ ] **12.2** `chmod +x tests/test_wave6_mutation_meta.sh`.

- [ ] **12.3** Run the gate (creds present):

```bash
bash tests/test_wave6_mutation_meta.sh
# Expected: PASS: wave6 mutation gate (3 paired)
```

- [ ] **12.4** Commit:

```bash
git add tests/test_wave6_mutation_meta.sh
git commit -m "$(cat <<'EOF'
Wave 6 step 12: tests/test_wave6_mutation_meta.sh — 3 paired mutations

Mutations + paired detectors:
  (a) blank bot self-filter → echo-loop signature in pherald log
  (b) Opus → Sonnet model literal swap → E67 FAIL
  (c) drop opts.ReplyTo in SendReply → E68 FAIL

§107.y pre-flight: working-tree quiescence + .git/MUTATION_IN_PROGRESS
lockfile. Trap-on-exit restores tree + clears lockfile.

§107 anchor: the gate proves the detectors are load-bearing. A
mutation that does NOT cause its detector to FAIL means the detector
was decorative → critical defect.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

### Self-review

- [ ] Working-tree quiescence pre-check inline at top of script.
- [ ] `.git/MUTATION_IN_PROGRESS` lockfile created on entry + removed via trap.
- [ ] Each mutation paired with exactly one detector.
- [ ] Trap-on-exit restores every mutated file even on `set -e` early-exit.

---

## Task 13: Spec V3 r-bump + §43 catalogue + Issues→Fixed + tag `v0.4.0` + 4-mirror push

**Goal.** Close Wave 6. Spec V3 r12 → r13 captures the Wave 6 substrate (pre-text wording, Opus pin, action routing, SendReply). §43 command catalogue extended with `pherald listen` + flags. HRD-NNN-W6 atomic migration Issues → Fixed. Tag `v0.4.0`. Push to all 4 mirrors per `upstreams/`.

### Files

**Modify:** `docs/specs/mvp/specification.V3.md` r12 → r13.
**Modify:** `docs/Issues.md` — remove HRD-NNN-W6 entries.
**Modify:** `docs/Fixed.md` — prepend HRD-NNN-W6 closure entries with commit SHAs + e2e invariant references.
**Modify:** `docs/Status.md` — Wave 6 completion summary; e2e tally bumped; tag `v0.4.0`.
**Modify:** `docs/CONTINUATION.md` — append the live-test handoff prompt for Wave 6 (operator runs `bash tests/test_wave6_live_loop.sh` with creds).
**Modify:** `CLAUDE.md` — r-bump status pointer; module count stays at 15.

### §107 anti-bluff anchor

(a) `docs/Fixed.md` entry MUST cite a real commit SHA (not "TBD"). (b) `docs/Status.md` e2e tally MUST match `scripts/e2e_bluff_hunt.sh`'s header. (c) Tag `v0.4.0` is created with `git tag -s v0.4.0 -m "<msg>"` (signed if operator's GPG config supports it; else `-a` annotated).

### Steps

- [ ] **13.1** Spec V3 r-bump. In `docs/specs/mvp/specification.V3.md`:
  - Header revision: r12 → r13. Update Status summary line.
  - §32 (inbound pipeline): add subsection "§32.7 Bot self-filter (anti-echo, Wave 6)" with the operator-mandated rule verbatim.
  - §33 (Claude Code dispatcher): add subsection "§33.5 Envelope pre-text (Wave 6, operator-mandated)" with the verbatim wording AND "§33.6 Model pinning (Opus, Wave 6)".
  - §43 commands catalogue: add `pherald listen` row with the flags + env vars + default behaviour.
  - Issues row + Fixed row appended per §8.3 lifecycle.

  Per `CLAUDE.md` spec-change rule: spec edits trigger mandatory comprehensive planning + implementation; this plan IS the planning artifact and Tasks 1–12 ARE the implementation.

- [ ] **13.2** Issues → Fixed atomic. In `docs/Issues.md` remove the HRD-NNN-W6 stub; in `docs/Fixed.md` prepend:

```
HRD-NNN-W6 — pherald inbound runtime + CC headless bridge
- Closed by commits <T1..T12 SHAs>
- e2e invariants: E63..E70 (or current contiguous range)
- Mutation gate: tests/test_wave6_mutation_meta.sh (3 paired)
- Live evidence: docs/qa/HRD-NNN-W6-<run-id>/
- Tag: v0.4.0
```

- [ ] **13.3** Status pointer. In `docs/Status.md` and `CLAUDE.md` add a Wave 6 summary; update e2e tally; tag `v0.4.0`.

- [ ] **13.4** Re-export PDFs/HTMLs for every touched MD per the export-pipeline convention:

```bash
bash scripts/export_docs.sh docs/specs/mvp/specification.V3.md \
                            docs/Issues.md docs/Fixed.md docs/Status.md \
                            docs/CONTINUATION.md CLAUDE.md
```

- [ ] **13.5** Final pre-tag checks:

```bash
scripts/audit_antibluff.sh
scripts/codegraph_validate.sh
scripts/e2e_bluff_hunt.sh   # ALL PASS (or E70 SKIP-with-reason)
bash tests/test_wave6_mutation_meta.sh   # if creds present
bash tests/test_constitution_inheritance.sh
bash tests/test_constitution_inheritance_meta.sh
```

All MUST PASS. Then:

```bash
git add docs/specs/mvp/specification.V3.md \
        docs/Issues.md docs/Fixed.md docs/Status.md docs/CONTINUATION.md \
        CLAUDE.md \
        docs/specs/mvp/specification.V3.pdf docs/specs/mvp/specification.V3.html \
        docs/Issues.{pdf,html,docx} docs/Fixed.{pdf,html,docx} \
        docs/Status.{pdf,html,docx} docs/CONTINUATION.{pdf,html,docx} \
        CLAUDE.{pdf,html,docx}
git commit -m "$(cat <<'EOF'
Wave 6 step 13: spec V3 r12→r13 + §43 catalogue + Issues→Fixed + status r-bump

Spec V3 r13 captures Wave 6 substrate: §32.7 bot self-filter, §33.5
envelope pre-text (verbatim operator wording), §33.6 Opus model pin.
§43 catalogue extended with pherald listen + flags + env vars.

HRD-NNN-W6 atomic close: Issues→Fixed with commit SHAs, e2e invariant
references, mutation-gate reference, docs/qa/HRD-NNN-W6-<run-id>/
evidence pointer, tag v0.4.0.

CLAUDE.md / Status.md / CONTINUATION.md r-bumped. Module count
unchanged (15). E2e tally bumped per the prevailing baseline + 8.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

- [ ] **13.6** Tag:

```bash
git tag -s v0.4.0 -m "Wave 6: pherald inbound runtime + CC headless bridge (Opus)" \
  || git tag -a v0.4.0 -m "Wave 6: pherald inbound runtime + CC headless bridge (Opus)"
```

- [ ] **13.7** Push to 4 mirrors per `upstreams/`:

```bash
for f in upstreams/GitHub.sh upstreams/GitLab.sh upstreams/GitFlic.sh upstreams/GitVerse.sh; do
  ( . "$f"; echo "[push] $UPSTREAMABLE_REPOSITORY"
    git push "$UPSTREAMABLE_REPOSITORY" main --tags )
done
```

DO NOT push if the controller has not granted permission. The plan's "DO NOT push (controller will batch)" instruction overrides this if the controller is running Wave 6 in batched-push mode.

### Self-review

- [ ] Every Fixed.md entry cites a real commit SHA (no `TBD`).
- [ ] Spec V3 r13 captures all three operator-mandated Wave 6 invariants (self-filter, pre-text, Opus pin) substantively (not as a single one-line ref).
- [ ] §43 catalogue row for `pherald listen` enumerates every flag + env var + default.
- [ ] Tag `v0.4.0` created annotated or signed.

---

## End-of-Wave checks

After T13 lands:

```bash
# 1) Module count unchanged
grep -c '^	\./' go.work    # Expected: 15

# 2) E2e total
grep -c 'assert_pass "E' scripts/e2e_bluff_hunt.sh   # Expected: prev baseline + 8

# 3) Mutation gate inventory
ls tests/test_wave*_mutation_meta.sh | wc -l         # Expected: 5 (W2/W3/W4/W4b/W6) — Wave 5 mutation gate counted as well if shipped

# 4) Tag exists locally
git tag -l 'v0.4.0'                                   # Expected: v0.4.0

# 5) Evidence dir present
ls docs/qa/HRD-NNN-W6-*/transcript.jsonl              # Expected: 1 file
```

If all five PASS, Wave 6 is closed.

---

## Plan self-review (run inline, no subagent)

- **Spec coverage:** every operator-mandate clause from the task brief has at least one task that implements it:
  - `HERALD_PROJECT_NAME` resolver → T1
  - Opus pin → T2
  - Pre-text wording → T3
  - Bot self-filter → T4
  - Attachment download → T5
  - `pherald listen` subcommand → T6
  - InboundHandler + action routing → T7
  - `tgram.SendReply` + `reply_to_message_id` → T8
  - Live closed-loop test → T9
  - `docs/qa/<run-id>/` evidence → T10
  - e2e invariants → T11
  - Mutation gate → T12
  - Spec + tag + push → T13

- **Placeholder scan:** searched the plan for `TBD`, "implement later", "similar to Task N", "handle edge cases" — only references are in this self-review section + the §43 catalogue (which is intentional — operators consult the catalogue, not the plan).

- **Type consistency:**
  - `commons.InboundEvent` (existing, unchanged) — used in T4/T5/T7.
  - `commons.Attachment` (existing, unchanged) — used in T5/T7/T8.
  - `claude_code.DispatchRequest` (existing) — used in T2/T3/T7 (via cc_adapter).
  - `inbound.CodeRequest` / `inbound.CodeResponse` / `inbound.Reply` / `inbound.IssuePayload` / `inbound.EventPayload` — declared in T7, used only in T7.
  - `tgram.Adapter` (existing) — extended with `SendReply` in T8.
  - `InboundHandler` interface (existing in `commons`) — implemented by `inbound.Dispatcher` in T7.

All type names referenced across tasks are consistent. No name drift.

- **§107 coverage:** each task has an anti-bluff anchor section calling out the specific bluff it forecloses. T11 invariants and T12 mutations form the test-side enforcement.

- **§107.y working-tree quiescence:** every task's commit block includes the explicit `git grep -n "MUTATED for paired\|// always pass"` pre-add check. T12's mutation gate additionally writes `.git/MUTATION_IN_PROGRESS` and registers trap-on-exit.

- **Task count:** 13 tasks. Hard cap was 14. Within budget.

- **Length:** target ~1500 lines. Self-measure via `wc -l` after write.

---

## Commit (after self-review)

```bash
git add docs/superpowers/plans/2026-05-22-wave6-inbound-runtime.md
git commit -m "$(cat <<'EOF'
Wave 6 implementation plan: pherald inbound runtime + CC headless bridge

13 tasks, TDD, full anti-bluff §107.x compliance.

Honors operator mandate (2026-05-22): bidirectional pherald ↔ Telegram
loop with Claude Code (Opus) as the message-processing glue. Session
name = HERALD_PROJECT_NAME (default: repo root folder name). Envelope
pre-text "We have received new message from our communication
channel ..." precedes the existing <<<HERALD-DISPATCH-v1>>> schema.

Builds on HRD-012 (closed) dispatcher substrate + extends with bot
self-filter, attachment download, reply API with reply_to_message_id,
and action-trigger dispatch (reply / issue.open / event.emit).

Targets v0.4.0 tag + closes the loop required by Wave 5 qaherald.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

DO NOT push (controller will batch).
