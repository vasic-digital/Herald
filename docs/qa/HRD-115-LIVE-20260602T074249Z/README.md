# HRD-115 — Slack live evidence (Socket Mode adapter, live parity progress)

**Run-id:** `HRD-115-LIVE-20260602T074249Z`
**Feature:** Slack channel at live parity with Telegram in the `pherald listen` inbound runtime.
**Covenant:** §107 / Helix §11.4 anti-bluff + §107.x evidence mandate + §11.4.98 full-automation. Tokens redacted; the Slack channel ID is configuration, not a credential.

## What is LIVE-PROVEN here (real runtime evidence, not metadata)

| # | Path proven | Evidence | File |
|---|-------------|----------|------|
| 1 | **Outbound send** via Slack Web API (bot token) — the `slack` channel adapter | `Send OK ChannelMsgID=1780386173.306249` (real message in the channel) | `live-transcript.txt` §1 |
| 2 | **qaherald MessengerClient send** (`TestSlack_Live_Send` — the exact name the E127 e2e invariant guards on) | `Send OK MessageID(ts)=1780386175.151239` | `live-transcript.txt` §2 |
| 3 | **Inbound credential** valid for Socket Mode (app token) | `apps.connections.open: ok=True has_wss_url=True` (Slack returned a live `wss://` URL) | `live-transcript.txt` §3 |
| 4 | **`pherald listen --channels slack` boots + engages the inbound loop** with real creds | banner `starting inbound runtime for channel(s): slack`; 10 s with no fail-loud error | `listen-boot.txt` |

Both Slack credentials the operator provided are therefore proven functional end-to-end: the **bot token** (outbound Web API — real send) and the **app token** (inbound Socket Mode — live WebSocket grant). The inbound runtime is wired (`HERALD_CHANNELS` + the new `--channels` flag), the §11.4.104 participant Resolver is now active for Slack and Telegram, and Slack `<@U…>` outbound @-tagging (§109) is wired into the notifier.

## What is NOT yet proven — the honest remaining blocker (NOT a bluff)

A fully self-driving §11.4.98 **inbound round-trip** (a message posted into the channel → received by the bot → dispatched → bot reply, with zero human action) is **NOT demonstrated here**, because a Slack bot cannot see its own messages: driving the round-trip automatically requires a **second, independent Slack identity** (a second bot token, or a user token with `chat:write`) for the qaherald driver to post *as someone the listening bot can see*. The four env vars provided (`HERALD_SLACK_BOT_TOKEN`, `HERALD_SLACK_APP_TOKEN`, `HERALD_SLACK_CHANNEL_ID`, `HERALD_SLACK_OPERATOR_USERNAME`) are sufficient for items 1–4 above but **not** for a self-driving round-trip — `HERALD_SLACK_OPERATOR_USERNAME` is a username, not a credential.

**Therefore HRD-115 stays OPEN**, not closed: closing it on send-only evidence would be a §107 PASS-bluff against its "live round-trip" closure criterion. To finish: provide one additional Slack credential (a second bot or a user token) and the existing qaherald `MessengerClient` driver completes the automated round-trip → `docs/qa/HRD-115-LIVE-<TS>/round-trip.txt`.

## Files

- `live-transcript.txt` — items 1–3 verbatim (tokens redacted).
- `listen-boot.txt` — item 4, the real `pherald listen --channels slack` boot transcript.
