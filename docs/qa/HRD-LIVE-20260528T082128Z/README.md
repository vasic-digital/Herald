# Herald 2026-05-28 Live-Evidence Run

**Run ID:** `HRD-LIVE-20260528T082128Z`
**Bot:** `@atmosphere_worker_bot` (id=8823384001, can_read_all_group_messages=true)
**Chat:** group `-4946584787`
**Claude Code:** v2.1.153 at `/Users/milosvasic/.local/bin/claude`
**Claude session UUID:** `19c3b53a-5114-499d-890d-3e1b53a39d7d` (file: `.herald/claude-code/sessions/Herald.session`)
**Conductor session:** Claude Opus 4.7 (autonomous loop)
**Sanitizer guard:** HRD-133 `sanitizeTgramError` — token-shape regex `[0-9]{8,}:[A-Za-z0-9_-]{30,}` matched **0 times** across ALL transcripts in this dir (verified).

## §11.4.98 classification of each sub-dir

Per the Helix Universal Constitution §11.4.98 (Full-Automation Anti-Bluff Mandate, anchored 2026-05-28): a test that requires manual human action during its execution is by definition a §11.4 PASS-bluff at the automation layer, regardless of how thorough the manual run is. Re-runnability + zero-human-action are the binding requirements.

| Sub-dir | Test | Result | §11.4.98 status | Notes |
|---|---|---|---|---|
| `01_outbound/` | `TestHealthCheck_LiveBotAPI` | PASS (0.58s) | **COMPLIANT** | Pure outbound `getMe` + `sendMessage` — fully autonomous, re-runnable. Real Bot API contact. |
| `01_outbound/` | `TestSend_LiveBotAPI` | PASS (0.32s) | **COMPLIANT** | `Send` delivered message_id=76 to chat `-4946584787` in 178ms. Fully autonomous. |
| `02_claude_code/` | `TestBootstrapSession_LiveClaudeInvocation` | PASS (12.10s) | **COMPLIANT** | `claude --resume <UUID> --print "..."` round-trip + JSONL transcript created in temp workdir + recursive-fallback finder located it. Fully autonomous. |
| `02_claude_code/` | `TestDispatch_LiveClaudeInvocation` | SKIP (no UUID) | **COMPLIANT** | Honest SKIP-with-reason — needed `HERALD_CLAUDE_SESSION_UUID`, which the auto-bootstrap path supplies but isn't set in the unit-test env. NOT a bluff (correctly reported as SKIP, not PASS). |
| `03_inbound/` | `TestSubscribe_LiveBotAPI` | PASS (60.0s, 2nd try) | **NON-COMPLIANT (manual-dep)** | Required operator to hand-send a Telegram message during execution. First attempt FAILed (60s window closed before send). Second attempt PASSed because the pending update from the user's prior send was still queued. THIS TEST CANNOT RUN IN CI without a human at the keyboard — by §11.4.98 definition a PASS-bluff at the automation layer. To be **rewritten** under Track B (MTProto user-account harness) or marked §11.4.90 Obsolete. |
| `04_wave6_closed_loop/` | `test_wave6_live_loop.sh` | FAIL (×2) | **NON-COMPLIANT (manual-dep + structural)** | 60s operator-send window closed before the human reaction time on both attempts. Even if the hand-send timing had worked, the structural Claude-session collision (see 05) would have blocked the dispatch leg. Wave 6 stages 1 + 2 **were proven live** via the journal (`transcript.jsonl` shows `tgram.message in` + `cc.dispatch out`); stage 3 (SendReply) never executed because Claude never returned. Per §11.4.98: rewrite end-to-end via MTProto + dedicated test-session UUID. |
| `05_dispatch_live/` | `TestDispatch_LiveClaudeInvocation` (with UUID) | FAIL (180.02s, exit -1) | **STRUCTURAL FAILURE — NOT a bluff** | Test correctly reported FAIL when `claude --resume <UUID>` exited -1 with empty stdout. Root cause: same UUID was being resumed by THIS conductor's active dev session → silent collision. §11.4.98 rule (2): test runs MUST use a dedicated test-only session UUID. Lesson encoded into §11.4.98. |

## Live-evidence proven by this run (anti-bluff posture)

These code paths are **proven against real Telegram Bot API + real Claude Code** (no fixtures, no mocks):

1. ✅ `commons_messaging/channels/tgram.HealthCheck` — `getMe` round-trip.
2. ✅ `commons_messaging/channels/tgram.Send` — `sendMessage` round-trip + Telegram-side message_id assignment (=76 in this run).
3. ✅ `commons_messaging/dispatch/claude_code.BootstrapSession` — `claude --resume <UUID> --print` produces a JSONL transcript at the expected path; recursive-fallback finder works.
4. ✅ `commons_messaging/channels/tgram.Subscribe` (poller-half) — long-poll `getUpdates` mechanism, bot self-filter, InboundEvent delivery (proven via operator-sent "test" message; mechanism PASSes but **manual driving** is the §11.4.98 violation, not the mechanism itself).
5. ✅ `pherald/internal/inbound.Dispatcher` — receives tgram message, classifies (`Type=query Criticality=middle`), dispatches `cc.dispatch` to Claude Code subprocess. **Journal-confirmed** (`04_wave6_closed_loop/pherald_journal/transcript.jsonl`).
6. ✅ Claude session received the envelope (`/Users/milosvasic/.claude/projects/-Users-milosvasic-Projects-Herald/19c3b53a-5114-499d-890d-3e1b53a39d7d.jsonl` line 171, ts=2026-05-28T08:58:50.903 — user turn carrying pherald's verbatim envelope text). Confirms `claude_code.Dispatcher`'s subprocess invocation works on the wire.

## Live-evidence NOT proven by this run (next-track work)

7. ❌ `claude_code.Dispatcher.Dispatch` end-to-end with assistant-turn reply parsed — Claude never returned a turn before pherald was killed (Wave 6) or before context timeout (dispatch test 05). Same-session-UUID collision blocked it.
8. ❌ `tgram.SendReply` with live `reply_to_message_id` referencing a recent inbound — never reached because (7) never produced an assistant turn.
9. ❌ End-to-end closed-loop e2e PASS — same blocker as (7) + (8).

These are the work for Track B (MTProto harness + dedicated test-only Claude session UUID).

## Sanitizer audit summary (HRD-133)

All 5 transcripts (`01_outbound/`, `02_claude_code/`, `03_inbound/`, `04_wave6_closed_loop/pherald_listen.log` and `pherald_journal/transcript.jsonl`, `05_dispatch_live/`) scanned for the token-shape regex `[0-9]{8,}:[A-Za-z0-9_-]{30,}`. **0 matches**. `sanitizeTgramError` worked end-to-end; bot token NEVER appears in any committed artefact.

## Re-run instructions (for the §11.4.98 compliant subset)

The COMPLIANT tests in this dir can re-run fully autonomously, **endlessly**, with no human action:

```bash
set -a; source .env; set +a
go test -tags=integration -count=1 -v -timeout=60s \
    -run 'TestHealthCheck_LiveBotAPI|TestSend_LiveBotAPI' \
    ./commons_messaging/channels/tgram/...

go test -tags=integration -count=1 -v -timeout=120s \
    -run 'TestBootstrapSession_LiveClaudeInvocation' \
    ./commons_messaging/dispatch/claude_code/...
```

The NON-COMPLIANT tests are **disabled** until Track B (MTProto harness) lands.

## Cross-references

- HelixConstitution §11.4.98 (anchor for this audit)
- Herald §107 / §107.x (anti-bluff covenant family)
- Herald §107.y (working-tree quiescence)
- HRD-133 (token-redaction sanitizer)
- HRD-100 (Wave 6 pherald inbound runtime — code-doc closed, live-evidence partial per §11.4.98)
- HRD-011 (Telegram live — outbound proven, inbound proven-via-manual-which-is-NON-COMPLIANT)
- HRD-012 (Claude Code dispatch — bootstrap proven, dispatch round-trip blocked by session-UUID collision)
- Task #221 (Track B — MTProto harness)
- Task #223 (full §11.4.98 audit of existing test surface)
