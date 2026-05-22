<div align="center">

<img src="../../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Herald — OpenCode Dispatcher Setup Guide

| Field | Value |
|---|---|
| Status | **PLANNED** — HRD-NNN (planned) |
| Spec ref | V3 §33 (pluggable Dispatcher) |
| Env vars (reserved) | `HERALD_OPENCODE_BIN`, `HERALD_OPENCODE_PROJECT_NAME`, `HERALD_OPENCODE_SESSION_UUID` |
| Code | (not yet implemented) |

OpenCode is an Anthropic-API-compatible local CLI agent. Planned for V2/V3 — uses the same session+envelope pattern as Claude Code.

This guide is a **placeholder**. When the OpenCode dispatcher lands as an HRD, this file will expand to the same step-by-step structure as [`CLAUDE_CODE.md`](CLAUDE_CODE.md): install the CLI/SDK → set env vars → verify session resolution → verify Dispatch round-trip → troubleshooting.

## What you can do now

- Reserve the env-var names in your `.env` per [`../OPERATOR_CREDENTIALS.md`](../OPERATOR_CREDENTIALS.md).
- The `Dispatcher` interface in `commons_messaging/dispatch/` is the seam every new dispatcher implements — see the Claude Code implementation as the reference shape.

## Related

- [`OPERATOR_CREDENTIALS.md`](../OPERATOR_CREDENTIALS.md) §"Alternate LLM dispatchers"
- [`CLAUDE_CODE.md`](CLAUDE_CODE.md) — canonical live-dispatcher guide shape
- Spec: V3 §33 (pluggable Dispatcher)
