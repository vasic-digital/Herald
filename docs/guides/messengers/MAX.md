<div align="center">

<img src="../../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Herald — Max Channel Setup Guide

| Field | Value |
|---|---|
| Status | **PLANNED** — HRD-NNN (planned V2) |
| Spec ref | V3 §11.3 |
| Env vars (reserved) | `HERALD_MAX_BOT_TOKEN`, `HERALD_MAX_CHAT_ID` |
| Code | (not yet implemented) |

Max is the Russian-market messenger; Bot API similar in shape to Telegram's. Planned for V2 alongside Telegram + Slack. Detailed bot-creation steps (analogous to Telegram's @BotFather flow) will land when the HRD opens.

This guide is a **placeholder**. When the Max adapter lands as an HRD, this file will expand to the same step-by-step structure as [`TELEGRAM.md`](TELEGRAM.md): obtain credentials → set env vars → verify HealthCheck → verify Send → optional Subscribe → troubleshooting.

## What you can do now

- Reserve the env-var names in your `.env` (commented out, per [`../OPERATOR_CREDENTIALS.md`](../OPERATOR_CREDENTIALS.md)).
- Watch the HRD for status updates — it will move from `open` to `code complete; awaiting live evidence` to `Fixed` over its implementation lifecycle.
- If you have an urgent need for the Max channel, open an issue against the HRD asking for priority — operator-driven escalation is honored per the constitution.

## Related

- [`OPERATOR_CREDENTIALS.md`](../OPERATOR_CREDENTIALS.md) — the umbrella credentials guide with reserved env-var blocks for every planned channel
- [`TELEGRAM.md`](TELEGRAM.md) — the canonical live-channel guide shape (this file will mirror that structure when Max lands)
- Spec: V3 §11.3
