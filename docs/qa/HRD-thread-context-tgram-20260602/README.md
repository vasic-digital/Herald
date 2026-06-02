# Telegram live thread-context-awareness evidence

**Test:** `TestMTProto_Wave6_AutonomousClosedLoop` (`-tags integration_mtproto`), fully automated. PASS 2026-06-02.

Proves Telegram thread-context end-to-end: the MTProto user posts a freeform PARENT, then a freeform REPLY that QUOTES it (via the new `mtproto.Client.SendReply` → `reply_to_message_id`). pherald's inbound `msg.ReplyTo` is non-nil → the tgram adapter's `threadContextFromReply` gathers the quoted parent → the dispatcher renders the `THREAD CONTEXT` block. An envelope-capture wrapper (`HERALD_CLAUDE_BIN`=wrapper that tees the real `claude --print` argv) captures the ACTUAL rendered envelope, and the test HARD-asserts it contains `THREAD CONTEXT` + `Participants:` + the parent message's text. (The journal does NOT contain the rendered envelope — it records only the raw user message — so the wrapper capture is the correct, stronger evidence.)

Mirrors the Slack proof (`docs/qa/HRD-115-LIVE-roundtrip-*` LEG 4). Both channels: thread-context reaches the Claude envelope, live.
