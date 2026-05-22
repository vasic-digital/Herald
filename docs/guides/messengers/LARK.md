<div align="center">

<img src="../../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Herald — Lark Channel Setup Guide

| Field | Value |
|---|---|
| Status | **PLANNED** — HRD-NNN (later iteration) |
| Spec ref | V3 §11 (later iteration) |
| Env vars (reserved) | `HERALD_LARK_APP_ID`, `HERALD_LARK_APP_SECRET`, `HERALD_LARK_CHAT_ID` |
| Code | (not yet implemented) |

Lark (Feishu in China) is reserved for a later iteration. Uses an OAuth-style app-id + app-secret flow rather than a single bot token.

This guide is a **placeholder**. When the Lark adapter lands as an HRD, this file will expand to the same step-by-step structure as [`TELEGRAM.md`](TELEGRAM.md): obtain credentials → set env vars → verify HealthCheck → verify Send → optional Subscribe → troubleshooting.

## What you can do now

- Reserve the env-var names in your `.env` (commented out, per [`../OPERATOR_CREDENTIALS.md`](../OPERATOR_CREDENTIALS.md)).
- Watch the HRD for status updates — it will move from `open` to `code complete; awaiting live evidence` to `Fixed` over its implementation lifecycle.
- If you have an urgent need for the Lark channel, open an issue against the HRD asking for priority — operator-driven escalation is honored per the constitution.

## Related

- [`OPERATOR_CREDENTIALS.md`](../OPERATOR_CREDENTIALS.md) — the umbrella credentials guide with reserved env-var blocks for every planned channel
- [`TELEGRAM.md`](TELEGRAM.md) — the canonical live-channel guide shape (this file will mirror that structure when Lark lands)
- Spec: V3 §11 (later iteration)
