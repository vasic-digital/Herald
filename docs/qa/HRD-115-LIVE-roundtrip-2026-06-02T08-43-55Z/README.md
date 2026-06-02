# HRD-115 — Slack inbound round-trip, LIVE PASS (§11.4.98 self-driving)

**Run-id:** `HRD-115-LIVE-roundtrip-2026-06-02T08-43-55Z`
**Result:** `--- PASS: TestSlack_LiveRoundTrip` (fully automated, zero human action).
**Covenant:** §107 / §11.4.98. Tokens redacted; the channel ID is configuration, not a credential.

This is the live, fully-automated proof that a Slack **user** message drives the pherald bot end-to-end and a reply is **delivered back to Slack** — with no human action during execution. `TRANSCRIPT.md` + `transcript.jsonl` + `pherald.log` are the verbatim, redacted run artefacts.

## Leg 1 — inbound autonomy chain (Tier-2, Claude Code) — PROVEN

A QA user identity (the operator's user OAuth token) posts a unique nonce-bearing probe. The pherald journal then shows, nonce-keyed:
`inbound (channel:slack)` → `cc.dispatch` → `cc.reply`. I.e. the bot **received** the user message over Socket Mode, **dispatched** it to Claude Code, and Claude **responded**.

Honest caveat (HRD-159): in this harness `pherald listen` bootstraps a fresh, context-less Claude session (per §11.4.98 rule-2, no collision with the dev conductor), and such a session returns **empty** reply text — so the *CC* reply is not posted back here. That specific gap is **cross-channel** (the Telegram round-trip has it too) and tracked by **HRD-159**. It is recorded as a note, never bluffed as success.

## Leg 2 — reply-DELIVERY to Slack — PROVEN (hard assertion)

The reply-back-to-Slack leg is proven **deterministically** via the Tier-1 fast-path, which does **not** depend on CC session fidelity:

```
QA user posts:  "What is the status of ATM-77810?"   (ts 1780389834.821619)
bot replies:    "Looking up the status of ATM-77810…" (ts 1780389836.066119)  ← landed in Slack, ts > probe
Reply DELIVERED : true
```

The deterministic `CommandRecognizer` (no Claude round-trip) recognizes the natural-language status query and pherald replies via `slack.SendReply`; the QA user reads that reply back from Slack. The unique id token (`ATM-<pid>`) is echoed in the reply for unambiguous provenance, and a `ts > probe` freshness gate excludes any stale earlier-run message.

## Clean shutdown — PROVEN

`pherald listen exited cleanly (status 0)` — confirming the §107 robustness fix: an empty Claude reply no longer crashes the listener (previously `slack.SendReply: empty body` → fail-loud killed the subscriber; fixed in `dispatcher.go` actReply + `TestDispatcherEmptyReplyDoesNotCrash`).

## Net

A Slack user message → bot (Socket Mode) → processing → **reply delivered back to the user**, fully automated. Combined with the send-side live evidence (`docs/qa/HRD-115-LIVE-20260602T074249Z/`), the Slack channel is at live parity with Telegram. The one residual — CC/Tier-2 reply-delivery from a fresh test session — is the cross-channel **HRD-159**, not a Slack-adapter defect.
