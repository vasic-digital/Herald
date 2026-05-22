<div align="center">

<img src="../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Herald — Issues Summary

| Field | Value |
|---|---|
| Revision | 1 |
| Created | 2026-05-20 |
| Last modified | 2026-05-20 |
| Status | active |
| Status summary | Auto-summary companion to `Issues.md` (per Universal §11.4.12 — paired tracker). Two-audience format (operator + AI agent) per Universal §11.4.56. |
| Issues | 1 open / in-progress (see below) |
| Issues summary | HRD-007 V3 r3 cross-doc sync — closing in the same commit. |
| Fixed | n/a (see `Fixed_Summary.md`) |
| Fixed summary | — |
| Continuation | regenerated whenever `Issues.md` changes; manual until the auto-sync wrapper exists |

## Table of contents

- [For operators (TL;DR)](#for-operators-tldr)
- [For AI agents (machine-readable)](#for-ai-agents-machine-readable)

## For operators (TL;DR)

Herald is pre-implementation. The only currently-tracked work is the V3 r3 cross-doc sync (`HRD-007`) which is being closed in the same commit that introduces this file. No engineering work blocked; no incidents in flight. Next milestone: first-implementation cycle (scaffold `commons` + `commons_messaging` + `pherald` shim against the §11.0 type contract).

## For AI agents (machine-readable)

```jsonc
{
  "open": [],
  "in_progress": [
    {
      "id":          "HRD-007",
      "type":        "task",
      "status":      "in_progress",
      "criticality": "middle",
      "title":       "V3 r3 cross-doc sync",
      "opened":      "2026-05-20",
      "reference":   "spec V3 §30.8"
    }
  ],
  "blocked":  [],
  "operator_blocked": [],
  "totals":   { "open": 0, "in_progress": 1, "blocked": 0, "operator_blocked": 0 }
}
```
