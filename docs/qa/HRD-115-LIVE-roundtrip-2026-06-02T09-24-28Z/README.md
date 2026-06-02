# HRD-115 — Slack inbound round-trip + THREADING, LIVE PASS (§11.4.98)

**Run-id:** `HRD-115-LIVE-roundtrip-2026-06-02T09-24-28Z`
**Result:** `--- PASS: TestSlack_LiveRoundTrip` (fully automated, zero human action).
**Covenant:** §107 / §11.4.98. Tokens redacted; the channel ID is configuration, not a credential.

Live proof that a Slack **user** message drives the pherald bot end-to-end, a reply is **delivered back in-thread**, and a Subscriber's **reply within an existing thread** is processed and answered **in the same thread** (operator mandates 2026-06-02). `TRANSCRIPT.md` + `transcript.jsonl` + `pherald.log` are the verbatim, redacted run artefacts.

## Leg 1 — inbound autonomy chain (Tier-2, Claude Code) — PROVEN
Nonce-keyed journal: `inbound (channel:slack)` → `cc.dispatch` → `cc.reply`. The bot received the user message over Socket Mode, dispatched to Claude Code, and Claude responded. (CC reply-DELIVERY from a fresh bootstrap session returns empty text — the cross-channel **HRD-159** — recorded as a note, never bluffed.)

## Leg 2 — reply-DELIVERY + THREADING — PROVEN (hard)
The QA user posts `"What is the status of ATM-14937?"`; pherald answers `"Looking up the status of ATM-14937…"` via `slack.SendReply`, and the QA user reads it back **from inside the thread** via `conversations.replies`:
```
Reply DELIVERED : true
In-thread under : 1780392261.919539  (thread_ts == the status-query ts ⇒ the reply is threaded)
```
Proven deterministically via the Tier-1 fast-path (no dependency on CC session fidelity). The reply is verified to carry `thread_ts` == the probe ts (in-thread, not top-level).

## Leg 3 — INBOUND threaded message — PROVEN (hard) — the operator's 2026-06-02 mandate
The QA user then posts a SECOND query **as a reply inside the existing thread** (`thread_ts` set):
```
Subscriber in-thread msg : And the status of ATM-14938?   (thread_ts=1780392261.919539)
Processed + answered     : true
In-thread reply          : "Looking up the status of ATM-14938…"
Stayed in SAME thread    : 1780392261.919539  (thread_ts == the original thread root)
```
This proves pherald **processes messages when a Subscriber replies within an existing thread** and **replies back into that same thread** — because the inbound message carries `thread_ts` and the dispatcher's `extractReplyToID` prefers it. (The "reply to a single message, creating a thread" scenario is the same mechanism and is additionally unit-covered by `TestDispatcherRepliesInThread`.)

## Clean shutdown — PROVEN
`pherald listen exited cleanly (status 0)` — the §107 robustness fix holds (an empty Claude reply no longer crashes the listener).

## Code that makes this work
- `pherald/internal/inbound/dispatcher.go` `extractReplyToID` — channel-agnostic thread-parent extraction (Slack `thread_ts`/ts string; Telegram int `message_id` → `reply_to_message_id`). This REPLACED the int-only extractor that silently dropped Slack threading.
- Unit: `TestDispatcherRepliesInThread` (tgram int/float, slack top-level ts, slack in-thread thread_ts). Adapter: `TestSlackSendReplyUsesThreadTS` (Slack) + `send_reply_test.go` (Telegram `reply_to_message_id`).
