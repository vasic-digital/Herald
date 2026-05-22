<div align="center">

<img src="../../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Herald — Email (SMTP + Resend) Channel Setup Guide

| Field | Value |
|---|---|
| Status | **PLANNED** — HRD-NNN (planned V2) |
| Spec ref | V3 §11.4 + §11.9 |
| Env vars (reserved) | `HERALD_SMTP_HOST`, `HERALD_SMTP_PORT`, `HERALD_SMTP_USERNAME`, `HERALD_SMTP_PASSWORD`, `HERALD_SMTP_FROM` OR `HERALD_RESEND_API_KEY`, `HERALD_RESEND_FROM` |
| Code | (not yet implemented) |

Email is planned for V2. Two backends will be supported: raw SMTP (any provider — Postmark, SES, etc.) and Resend (transactional email API). Choose ONE based on your infra. The mailto:// adapter URL form encodes the destination address.

This guide is a **placeholder**. When the Email (SMTP + Resend) adapter lands as an HRD, this file will expand to the same step-by-step structure as [`TELEGRAM.md`](TELEGRAM.md): obtain credentials → set env vars → verify HealthCheck → verify Send → optional Subscribe → troubleshooting.

## What you can do now

- Reserve the env-var names in your `.env` (commented out, per [`../OPERATOR_CREDENTIALS.md`](../OPERATOR_CREDENTIALS.md)).
- Watch the HRD for status updates — it will move from `open` to `code complete; awaiting live evidence` to `Fixed` over its implementation lifecycle.
- If you have an urgent need for the Email (SMTP + Resend) channel, open an issue against the HRD asking for priority — operator-driven escalation is honored per the constitution.

## Related

- [`OPERATOR_CREDENTIALS.md`](../OPERATOR_CREDENTIALS.md) — the umbrella credentials guide with reserved env-var blocks for every planned channel
- [`TELEGRAM.md`](TELEGRAM.md) — the canonical live-channel guide shape (this file will mirror that structure when Email (SMTP + Resend) lands)
- Spec: V3 §11.4 + §11.9
