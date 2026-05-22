<div align="center">

<img src="../../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Herald — Aider Dispatcher Setup Guide

| Field | Value |
|---|---|
| Status | **PLANNED** — HRD-NNN (planned) |
| Spec ref | V3 §33 (pluggable Dispatcher) |
| Env vars (reserved) | `HERALD_AIDER_BIN`, `HERALD_AIDER_MODEL` (e.g. `claude-3-5-sonnet-20241022`), `HERALD_AIDER_API_KEY` |
| Code | (not yet implemented) |

Aider is a popular open-source pair-programming CLI agent. Different envelope+reply format than Claude Code — the adapter will translate.

This guide is a **placeholder**. When the Aider dispatcher lands as an HRD, this file will expand to the same step-by-step structure as [`CLAUDE_CODE.md`](CLAUDE_CODE.md): install the CLI/SDK → set env vars → verify session resolution → verify Dispatch round-trip → troubleshooting.

## What you can do now

- Reserve the env-var names in your `.env` per [`../OPERATOR_CREDENTIALS.md`](../OPERATOR_CREDENTIALS.md).
- The `Dispatcher` interface in `commons_messaging/dispatch/` is the seam every new dispatcher implements — see the Claude Code implementation as the reference shape.

## Related

- [`OPERATOR_CREDENTIALS.md`](../OPERATOR_CREDENTIALS.md) §"Alternate LLM dispatchers"
- [`CLAUDE_CODE.md`](CLAUDE_CODE.md) — canonical live-dispatcher guide shape
- Spec: V3 §33 (pluggable Dispatcher)
