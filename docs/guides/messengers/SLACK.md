<div align="center">

<img src="../../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Herald — Slack Channel Setup Guide

| Field | Value |
|---|---|
| Status | **PLANNED** — HRD-NNN (planned V2) |
| Spec ref | V3 §11.2 + §11.9 |
| Env vars (reserved) | `HERALD_SLACK_BOT_TOKEN`, `HERALD_SLACK_SIGNING_SECRET`, `HERALD_SLACK_APP_TOKEN`, `HERALD_SLACK_DEFAULT_CHANNEL` |
| Code | (not yet implemented) |

Slack is planned for V2. The adapter will support both Events API (HTTPS webhooks) and Socket Mode (WebSocket — useful for hosts without inbound HTTPS). Channels are addressed by ID (not name) since Slack channels are renamable while IDs are stable.

This guide is a **placeholder**. When the Slack adapter lands as an HRD, this file will expand to the same step-by-step structure as [`TELEGRAM.md`](TELEGRAM.md): obtain credentials → set env vars → verify HealthCheck → verify Send → optional Subscribe → troubleshooting.

## What you can do now

- Reserve the env-var names in your `.env` (commented out, per [`../OPERATOR_CREDENTIALS.md`](../OPERATOR_CREDENTIALS.md)).
- Watch the HRD for status updates — it will move from `open` to `code complete; awaiting live evidence` to `Fixed` over its implementation lifecycle.
- If you have an urgent need for the Slack channel, open an issue against the HRD asking for priority — operator-driven escalation is honored per the constitution.

## Related

- [`OPERATOR_CREDENTIALS.md`](../OPERATOR_CREDENTIALS.md) — the umbrella credentials guide with reserved env-var blocks for every planned channel
- [`TELEGRAM.md`](TELEGRAM.md) — the canonical live-channel guide shape (this file will mirror that structure when Slack lands)
- Spec: V3 §11.2 + §11.9
