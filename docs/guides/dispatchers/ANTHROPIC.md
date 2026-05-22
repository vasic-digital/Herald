<div align="center">

<img src="../../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Herald — Anthropic Managed Agent Dispatcher Setup Guide

| Field | Value |
|---|---|
| Status | **PLANNED** — HRD-NNN (planned) |
| Spec ref | V3 §33 (pluggable Dispatcher) |
| Env vars (reserved) | `ANTHROPIC_API_KEY` (canonical Anthropic env var name) OR `HERALD_ANTHROPIC_API_KEY`; optional: `HERALD_ANTHROPIC_MODEL`, `HERALD_ANTHROPIC_BASE_URL` (for Bedrock / Vertex variants) |
| Code | (not yet implemented) |

Direct Anthropic API integration (no CLI middleman). Differs from Claude Code in that there's no session-anchor file — the dispatcher calls the Messages API directly with the conversation history as context. Useful when the operator wants centralized billing or model selection beyond what Claude Code exposes.

This guide is a **placeholder**. When the Anthropic Managed Agent dispatcher lands as an HRD, this file will expand to the same step-by-step structure as [`CLAUDE_CODE.md`](CLAUDE_CODE.md): install the CLI/SDK → set env vars → verify session resolution → verify Dispatch round-trip → troubleshooting.

## What you can do now

- Reserve the env-var names in your `.env` per [`../OPERATOR_CREDENTIALS.md`](../OPERATOR_CREDENTIALS.md).
- The `Dispatcher` interface in `commons_messaging/dispatch/` is the seam every new dispatcher implements — see the Claude Code implementation as the reference shape.

## Related

- [`OPERATOR_CREDENTIALS.md`](../OPERATOR_CREDENTIALS.md) §"Alternate LLM dispatchers"
- [`CLAUDE_CODE.md`](CLAUDE_CODE.md) — canonical live-dispatcher guide shape
- Spec: V3 §33 (pluggable Dispatcher)
