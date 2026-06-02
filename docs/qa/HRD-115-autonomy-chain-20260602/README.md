# HRD-115 — Slack inbound autonomy chain (live) + HRD-159 reply-delivery gap

**Run-id:** `HRD-115-autonomy-chain-20260602`
**Covenant:** §107 / §11.4.98. Tokens redacted; the channel ID (`C0B76L5BPDM`) is configuration, not a credential.

## What is LIVE-PROVEN (real journal evidence)

`roundtrip-autonomy-chain.txt` is the verbatim, redacted output of the fully-automated `TestSlack_LiveRoundTrip` harness. It proves the §11.4.98 **autonomy chain** end-to-end with zero human action:

1. The harness builds `pherald`, starts `pherald listen --channels slack` (real Slack Socket Mode) under a dedicated test project name.
2. A **QA user identity** (the operator's user OAuth token) posts a unique nonce-bearing probe into the channel.
3. The pherald journal then shows, keyed on that exact nonce:
   - `✓ inbound message carrying our probe text (channel:slack)` — the bot **received** the user's message over Socket Mode,
   - `✓ cc.dispatch out referencing our probe text` — it **dispatched** to Claude Code,
   - `✓ cc.reply in — Claude responded`.

This is genuine, nonce-keyed proof that a Slack user message drives the bot → Claude pipeline automatically.

## What is NOT proven here — HRD-159 (honest SKIP, not a bluff)

The final **reply-DELIVERY** leg (Claude's reply text actually posted back into Slack) is **NOT** proven, and the test SKIPs-with-reason rather than false-passing:

```
bot reply NOT observed in Slack within 40s: context deadline exceeded
SKIP: §11.4.98 autonomy chain PROVEN; reply-DELIVERY to Slack NOT proven —
fresh bootstrap session returns empty reply text (HRD-159, cross-channel)
```

Root cause (`pherald.log`): `inbound: reply skipped — empty reply text`. In this harness `pherald listen` bootstraps a **fresh, context-less Claude session** (per §11.4.98 rule-2, to avoid colliding with the dev conductor's session), and such a session returns an **empty reply text**, so nothing is posted back. This is a **pre-existing, cross-channel** condition — the Telegram round-trip (`mtproto_wave6_loop_test.go:255`) has the identical gap, masked there by a soft "BONUS" log; the Slack harness was hardened (a `ts > probe` freshness gate so a stale earlier message can't false-satisfy it) which is what surfaced it. Tracked by **HRD-159**.

## Related robustness fix (committed)

An empty reply previously **crashed the entire inbound listener** (`slack.SendReply: empty body` → fail-loud). Fixed in `pherald/internal/inbound/dispatcher.go` (`actReply` empty-text guard: log + skip, never crash), locked by `TestDispatcherEmptyReplyDoesNotCrash` (3 cases, `-race`).

## Relationship to the send-side evidence

The **outbound** Slack path is independently live-proven under `docs/qa/HRD-115-LIVE-20260602T074249Z/` (real `Send` ts, `TestSlack_Live_Send` PASS, app-token Socket-Mode validity, `pherald listen` boot). Together: send works live; inbound autonomy chain works live; reply-delivery is the one remaining leg, tracked by HRD-159. **HRD-115 stays OPEN.**
