# HRD-115 — Telegram (MTProto) live threading evidence

**Test:** `TestMTProto_Wave6_AutonomousClosedLoop` (build tag `integration_mtproto`), fully automated, zero human action. PASS 2026-06-02.

Proves the operator's threading mandate on **Telegram** (`reply_to_message_id`), mirroring the Slack proof:

1. **Autonomy chain** (journal): an MTProto user message → bot inbound (getUpdates) → `cc.dispatch` → `cc.reply`.
2. **reply-DELIVERY + THREADING** (hard-asserted, deterministic Tier-1 fast-path, no CC-session dependency): the MTProto user posts `"What is the status of ATM-<pid>?"`; the deterministic CommandRecognizer answers `"Looking up the status of ATM-<pid>…"` via `tgram.SendReply` **with `reply_to_message_id`** = our query's id; the MTProto user observes the reply as a **quoted reply** — `reply_to_message_id=211854` == our query's MTProto-id `211854`. The matcher keys on the reply TEXT (unique id token proves causation) and asserts `reply_to_message_id != 0` (threaded), and the run additionally showed exact id equality.

The shared fix is `pherald/internal/inbound/dispatcher.go` `extractReplyToID` (channel-agnostic: Telegram int `message_id` → `reply_to_message_id`; Slack `thread_ts`/ts → `thread_ts`). Unit-covered by `TestDispatcherRepliesInThread`; Slack live-covered by `TestSlack_LiveRoundTrip` (see `docs/qa/HRD-115-LIVE-roundtrip-*`).

Note: the bot's `reply_to` is logged as the Bot-API chat-local id (e.g. 96) in `pherald.log`, while MTProto observes the same message under the MTProto id namespace — they are the same message; Telegram maps between the two.
