# HRD-100 Wave 6 — Live closed-loop e2e evidence

| Field | Value |
|---|---|
| Run ID | 2026-05-22T18-27-30-w6live |
| Date (UTC) | 2026-05-22 18:46:44 → 18:47:12 |
| Duration | ~28 s end-to-end (incl. CC session bootstrap on first dispatch) |
| Chat | ATMOSphere Development (chat_id `-4946584787`, type `group`) |
| Bot | `@atmosphere_worker_bot` (id `8823384001`, name "ATMOSphere Bot") |
| Operator | milos85vasic (id `2057253161`) |
| Model | `claude-opus-4-7` (pinned per §33.7) |
| CC session UUID | `19c3b53a-5114-499d-890d-3e1b53a39d7d` (anchored at `.herald/claude-code/sessions/Herald.session`) |
| Outcome | **PASS — full bidirectional closed loop validated** |

## What this run proves (§107 anti-bluff)

Every assertion below is backed by bytes-on-wire captured in `transcript.jsonl`. No metadata-only PASS-bluff is possible.

1. **Inbound capture** — a real human subscriber (operator) sent `/start@atmosphere_worker_bot` in the Telegram group; pherald's `getUpdates` long-poll received it (message_id 25).
2. **Bot self-filter passed** — operator's `from.is_bot = false` so the message was NOT dropped by §32.9's anti-echo filter.
3. **Envelope assembled with verbatim operator pre-text** (§33.6) — natural-language prelude precedes the structured `<<<HERALD-DISPATCH-v1>>>` block.
4. **Opus pinned in argv** (§33.7) — `claude --model claude-opus-4-7` literal argv flag in the spawned subprocess (verified in T2's unit test; reproduced live here).
5. **CC session bootstrap-on-uuid.Nil** — first inbound message triggered the HRD-012-step-7 bootstrap (commit `87260ff`); session `19c3b53a-…` created via `claude --session-id <uuid> --print "<bootstrap-prompt>"`, anchored under `.herald/claude-code/sessions/Herald.session`, and persisted for future dispatches.
6. **`<<<HERALD-REPLY>>>` parsed** — Claude Opus returned `{"action":"reply","text":"Welcome to Herald — bot is online and listening."}`. The `inbound.Dispatcher` parsed the marker + JSON correctly.
7. **Action routed to `reply`** — default action; the recording-sink + log line `inbound dispatched: reply (chatID=-4946584787 replyTo=25)` proves the routing branch fired.
8. **`tgram.SendReply` emitted `reply_to_message_id`** (§107.x wire-byte proof) — the bot's outgoing message (id 26) has `reply_to_message_id = 25`, exactly matching the inbound message_id. Telegram rendered it as a threaded reply in the group; the operator visually confirmed the reply in their client.

## Files in this run dir

| File | Purpose |
|---|---|
| `README.md` | This file — operator-attestable narrative of the run. |
| `transcript.jsonl` | 4 JSONL events captured by pherald's `--qa-out-dir` middleware: `tgram.message` → `cc.dispatch` → `cc.reply` → `tgram.send_reply`. |
| `pherald-listen.log` | Raw stdout/stderr of the `pherald listen` process — includes the dispatch log line. |
| `claude-code-session-uuid.txt` | The UUID of the Claude Code session created by the HRD-012-step-7 bootstrap; same UUID anchored at `<workingDir>/.herald/claude-code/sessions/Herald.session`. |
| `claude-session.jsonl` | Full transcript of the Claude Code subprocess (79 lines) — proves the session really exists and the Opus model really processed the envelope. |
| `attachments/` | Empty in this run — no attachments were exchanged. The directory is part of the §107.x layout contract and stays present. |

## Reproduce this run

```bash
# Prereqs:
export HERALD_TGRAM_BOT_TOKEN=<from .zshrc>
export HERALD_TGRAM_CHAT_ID=-4946584787   # group chat_id
export HERALD_CLAUDE_BIN=$(command -v claude)
export HERALD_PROJECT_NAME=Herald
export HERALD_PG_DSN="postgres://herald:<pw>@127.0.0.1:24100/herald?sslmode=disable"
export HERALD_REDIS_URL="redis://127.0.0.1:24200/0"
export HERALD_AUTH_MODE=hmac
export HERALD_JWT_SECRET=$(openssl rand -hex 32)

go build -o .build/pherald ./pherald/cmd/pherald
RUN_ID="$(date -u +%Y-%m-%dT%H-%M-%S)-w6live"
mkdir -p "docs/qa/HRD-100-${RUN_ID}/attachments"

# Start the runtime
.build/pherald listen --qa-out-dir "docs/qa/HRD-100-${RUN_ID}" &
LISTEN_PID=$!

# OPERATOR: in the ATMOSphere Development group, send:
#   /start@atmosphere_worker_bot
# (commands with explicit @bot suffix pass bot-privacy mode; plain @mentions
#  often don't produce a real `mention` entity in mobile clients)

# Watch transcript:
tail -F "docs/qa/HRD-100-${RUN_ID}/transcript.jsonl"

# When 4 events captured (tgram.message → cc.dispatch → cc.reply → tgram.send_reply):
kill -TERM $LISTEN_PID

# Commit the run dir as §107.x evidence:
git add "docs/qa/HRD-100-${RUN_ID}/"
git commit -m "Wave 6 step 10b: HRD-100 live closed-loop evidence"
```

## Root-cause fixes that landed during this run

1. **`78363b3`** — `commons_messaging/channels/tgram/tgram.go`: bot-token URL parser collapse. Real Telegram bot tokens contain a `:` between the numeric id and the alphanumeric HMAC; Go's `url.Parse` interprets that as `host:port` and rejects non-numeric "port" values. Short-circuited the canonical `tgram://<id>:<hmac>/<chat>` shape + added `NewWithCreds(token, chatID)`. Surfaced when `pherald listen` died at boot with the operator's real token in this run; the unit test suite was green because synthetic tokens are colon-free. **Live evidence of §107 working as designed**: in-vivo runtime exposed a bluff that the unit-test surface couldn't.

2. **`87260ff`** — `commons_messaging/dispatch/claude_code/bootstrap.go` (NEW): `bootstrapSession()` helper auto-spawns a fresh Claude Code session on `uuid.Nil`, persists the new UUID via the existing `PersistSession` (no parallel write path), surfaces stderr verbatim on failure. Closed HRD-012 step 7 properly — the future-tense placeholder comment in `dispatch.go:77` was replaced with a real call. Live integration test creates a real claude subprocess and asserts the transcript file appears under `~/.claude/projects/...`. Surfaced this run by the empty `claude_code_sessions` table + missing `.herald/claude-code/sessions/Herald.session` anchor on a fresh repo.

## Gating verdict

- §107.x docs/qa/ evidence mandate: **SATISFIED** by this directory.
- Wave 6 closed-loop architecture: **SATISFIED** — all 4 stages captured with wire-byte truth.
- Tag `v0.4.0` (T13b): **UNBLOCKED** — proceeding with annotated tag + 4-mirror push.

## Operator attestation

> The bot's reply landed in the ATMOSphere Development group as a threaded reply to message_id 25, exactly as the architecture mandates. The closed loop works end-to-end against real Telegram + real Claude Code Opus + real pherald listen. — milos85vasic, 2026-05-22 (confirmed in chat at run time)
