# Herald — Cursor Dispatcher Setup Guide

| Field | Value |
|---|---|
| Status | **PLANNED** — HRD-NNN (planned) |
| Spec ref | V3 §33 (pluggable Dispatcher) |
| Env vars (reserved) | `HERALD_CURSOR_BIN`, `HERALD_CURSOR_PROJECT_NAME` |
| Code | (not yet implemented) |

Cursor is the AI-first IDE. Its CLI surface is evolving; integration will follow when Cursor exposes a stable headless API or CLI.

This guide is a **placeholder**. When the Cursor dispatcher lands as an HRD, this file will expand to the same step-by-step structure as [`CLAUDE_CODE.md`](CLAUDE_CODE.md): install the CLI/SDK → set env vars → verify session resolution → verify Dispatch round-trip → troubleshooting.

## What you can do now

- Reserve the env-var names in your `.env` per [`../OPERATOR_CREDENTIALS.md`](../OPERATOR_CREDENTIALS.md).
- The `Dispatcher` interface in `commons_messaging/dispatch/` is the seam every new dispatcher implements — see the Claude Code implementation as the reference shape.

## Related

- [`OPERATOR_CREDENTIALS.md`](../OPERATOR_CREDENTIALS.md) §"Alternate LLM dispatchers"
- [`CLAUDE_CODE.md`](CLAUDE_CODE.md) — canonical live-dispatcher guide shape
- Spec: V3 §33 (pluggable Dispatcher)
