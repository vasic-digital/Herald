<div align="center">

![Herald](../assets/logo/herald_logo_square_128.png){width=96px height=96px}

</div>

# Herald — Fixed Summary

| Field | Value |
|---|---|
| Revision | 1 |
| Created | 2026-05-20 |
| Last modified | 2026-05-20 |
| Status | active |
| Status summary | Auto-summary companion to `Fixed.md` (per Universal §11.4.53). Two-audience format. |
| Issues | n/a |
| Issues summary | — |
| Fixed | 6 items closed retrospectively |
| Fixed summary | V1 + V2 (r1+r2+r3) + V3 (r1+r2) spec-revision work. Spec set grew from 594 lines (V1) to ~3900 lines (V3) over two days, all on the `main` branch with multi-mirror fan-out. |
| Continuation | regenerated whenever `Fixed.md` changes. |

## Table of contents

- [For operators (TL;DR)](#for-operators-tldr)
- [For AI agents (machine-readable)](#for-ai-agents-machine-readable)

## For operators (TL;DR)

Six items closed retroactively in V3 r3 covering the entire spec authoring + revision arc:

- **HRD-001** V1 MVP spec (initial cut).
- **HRD-002** V2 r1 — architectural baseline (CloudEvents / Watermill / OTel / RLS / SLSA + 9 flavors).
- **HRD-003** V2 r2 — closed missing tables + GDPR + quickstart + DR + costs.
- **HRD-004** V2 r3 — defined remaining Go types + migration tooling + workers + hot-reload + ingress URLs + agent subscribers.
- **HRD-005** V3 r1 — operator-product layer (project contract / inbound pipeline / LLM dispatch / reply protocol / versioned reports / multi-format attachments).
- **HRD-006** V3 r2 — refined nine flavors for channel-native interaction.

Zero rollbacks. Inheritance gate stayed 12 PASS / 0 FAIL throughout.

## For AI agents (machine-readable)

```jsonc
{
  "fixed": [
    { "id": "HRD-006", "closed": "2026-05-20", "commit": "f8b8073", "title": "V3 r2 flavor refinement" },
    { "id": "HRD-005", "closed": "2026-05-20", "commit": "e26a8dc", "title": "V3 r1 operator-product layer" },
    { "id": "HRD-004", "closed": "2026-05-20", "commit": "f4ebba1", "title": "V2 r3 self-review" },
    { "id": "HRD-003", "closed": "2026-05-19", "commit": "9648545", "title": "V2 r2 self-review" },
    { "id": "HRD-002", "closed": "2026-05-19", "commit": "96b7cc6", "title": "V2 r1 architecture" },
    { "id": "HRD-001", "closed": "2026-05-19", "commit": "b421fe1", "title": "V1 MVP specification" }
  ],
  "totals": { "fixed": 6, "reopened": 0 }
}
```
