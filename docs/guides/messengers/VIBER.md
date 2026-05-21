<div align="center">

![Herald](../../../assets/logo/herald_logo_square_128.png){width=96px height=96px}

</div>

# Herald — Viber Channel Setup Guide

| Field | Value |
|---|---|
| Status | **PLANNED** — HRD-NNN (later iteration) |
| Spec ref | V3 §11 (later iteration) |
| Env vars (reserved) | `HERALD_VIBER_AUTH_TOKEN`, `HERALD_VIBER_SENDER_NAME`, `HERALD_VIBER_SENDER_AVATAR` |
| Code | (not yet implemented) |

Viber is reserved for a later iteration. Uses a single auth token per bot account from the Viber Bots admin panel.

This guide is a **placeholder**. When the Viber adapter lands as an HRD, this file will expand to the same step-by-step structure as [`TELEGRAM.md`](TELEGRAM.md): obtain credentials → set env vars → verify HealthCheck → verify Send → optional Subscribe → troubleshooting.

## What you can do now

- Reserve the env-var names in your `.env` (commented out, per [`../OPERATOR_CREDENTIALS.md`](../OPERATOR_CREDENTIALS.md)).
- Watch the HRD for status updates — it will move from `open` to `code complete; awaiting live evidence` to `Fixed` over its implementation lifecycle.
- If you have an urgent need for the Viber channel, open an issue against the HRD asking for priority — operator-driven escalation is honored per the constitution.

## Related

- [`OPERATOR_CREDENTIALS.md`](../OPERATOR_CREDENTIALS.md) — the umbrella credentials guide with reserved env-var blocks for every planned channel
- [`TELEGRAM.md`](TELEGRAM.md) — the canonical live-channel guide shape (this file will mirror that structure when Viber lands)
- Spec: V3 §11 (later iteration)
