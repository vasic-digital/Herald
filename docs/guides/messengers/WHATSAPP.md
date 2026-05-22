<div align="center">

<img src="../../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Herald — WhatsApp Channel Setup Guide

| Field | Value |
|---|---|
| Status | **PLANNED** — HRD-NNN (later iteration) |
| Spec ref | V3 §11 (later iteration) |
| Env vars (reserved) | `HERALD_WHATSAPP_BUSINESS_TOKEN`, `HERALD_WHATSAPP_PHONE_NUMBER_ID`, `HERALD_WHATSAPP_VERIFY_TOKEN` |
| Code | (not yet implemented) |

WhatsApp Business API is reserved for a later iteration. Requires WhatsApp Business Account approval and a verified phone number; setup is significantly more involved than other messengers.

This guide is a **placeholder**. When the WhatsApp adapter lands as an HRD, this file will expand to the same step-by-step structure as [`TELEGRAM.md`](TELEGRAM.md): obtain credentials → set env vars → verify HealthCheck → verify Send → optional Subscribe → troubleshooting.

## What you can do now

- Reserve the env-var names in your `.env` (commented out, per [`../OPERATOR_CREDENTIALS.md`](../OPERATOR_CREDENTIALS.md)).
- Watch the HRD for status updates — it will move from `open` to `code complete; awaiting live evidence` to `Fixed` over its implementation lifecycle.
- If you have an urgent need for the WhatsApp channel, open an issue against the HRD asking for priority — operator-driven escalation is honored per the constitution.

## Related

- [`OPERATOR_CREDENTIALS.md`](../OPERATOR_CREDENTIALS.md) — the umbrella credentials guide with reserved env-var blocks for every planned channel
- [`TELEGRAM.md`](TELEGRAM.md) — the canonical live-channel guide shape (this file will mirror that structure when WhatsApp lands)
- Spec: V3 §11 (later iteration)
