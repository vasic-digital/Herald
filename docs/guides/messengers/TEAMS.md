<div align="center">

![Herald](../../../assets/logo/herald_logo_square_128.png){width=96px height=96px}

</div>

# Herald — Microsoft Teams Channel Setup Guide

| Field | Value |
|---|---|
| Status | **PLANNED** — HRD-NNN (planned V3) |
| Spec ref | V3 §11 (later iteration) |
| Env vars (reserved) | `HERALD_TEAMS_WEBHOOK_URL` OR `HERALD_TEAMS_BOT_APP_ID` + `HERALD_TEAMS_BOT_APP_PASSWORD` |
| Code | (not yet implemented) |

Microsoft Teams is reserved for V3. Two integration paths: (a) Incoming Webhook (simplest — connector URL only; one-way fan-out only), or (b) Bot Framework registration (full two-way, requires Azure AD app).

This guide is a **placeholder**. When the Microsoft Teams adapter lands as an HRD, this file will expand to the same step-by-step structure as [`TELEGRAM.md`](TELEGRAM.md): obtain credentials → set env vars → verify HealthCheck → verify Send → optional Subscribe → troubleshooting.

## What you can do now

- Reserve the env-var names in your `.env` (commented out, per [`../OPERATOR_CREDENTIALS.md`](../OPERATOR_CREDENTIALS.md)).
- Watch the HRD for status updates — it will move from `open` to `code complete; awaiting live evidence` to `Fixed` over its implementation lifecycle.
- If you have an urgent need for the Microsoft Teams channel, open an issue against the HRD asking for priority — operator-driven escalation is honored per the constitution.

## Related

- [`OPERATOR_CREDENTIALS.md`](../OPERATOR_CREDENTIALS.md) — the umbrella credentials guide with reserved env-var blocks for every planned channel
- [`TELEGRAM.md`](TELEGRAM.md) — the canonical live-channel guide shape (this file will mirror that structure when Microsoft Teams lands)
- Spec: V3 §11 (later iteration)
