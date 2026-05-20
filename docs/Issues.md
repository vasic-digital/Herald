# Herald — Issues

| Field | Value |
|---|---|
| Revision | 2 |
| Created | 2026-05-20 |
| Last modified | 2026-05-20 |
| Status | active |
| Status summary | First-implementation cycle in progress: V3 spec extended (§37–§41), Go scaffold landed (commons + commons_prefix + commons_messaging + commons_storage + pherald CLI), Quickstart compose scaffolded, null:// adapter fully working with passing tests. |
| Issues | HRD-008, HRD-010, HRD-011, HRD-012, HRD-015, HRD-016, HRD-017 |
| Issues summary | live integrations + REST API surface + universal versioning rule propagation pending. |
| Fixed | HRD-007, HRD-009, HRD-009b, HRD-013, HRD-014 (this commit) |
| Fixed summary | see `Fixed.md`. |
| Continuation | see `CONTINUATION.md`. |

## Table of contents

- [Open](#open)
- [In progress](#in-progress)
- [Blocked](#blocked)

## Open

| ID | Type | Status | Criticality | Title | Opened | Last update | Reference |
|---|---|---|---|---|---|---|---|
| HRD-010 | task | open | middle | commons_storage live wiring (golang-migrate driver, pgx pool, River queue, Redis ACL) | 2026-05-20 | 2026-05-20 | spec V3 §9.6 + §16; migrations already shipped in this commit |
| HRD-011 | task | open | middle | Telegram channel adapter live integration (telebot SDK + getUpdates long-poll + webhook secret_token) | 2026-05-20 | 2026-05-20 | spec V3 §11.1; stub shipped |
| HRD-012 | task | open | middle | Claude Code dispatcher live integration (resolve_session + `claude --resume` + parse `<<<HERALD-REPLY>>>`) | 2026-05-20 | 2026-05-20 | spec V3 §33; stub + envelope formatter + tests shipped |
| HRD-015 | task | open | low | Add inheritance gate I8 invariants for Go scaffold (go.work present + commons/types.go present + null adapter passes test) | 2026-05-20 | 2026-05-20 | spec V3 §40 + gate I7 pattern |
| HRD-016 | task | open | middle | REST API surface via Gin Gonic per spec §41 — pherald/internal/http/ with /v1/* routes + JWT auth + OpenAPI tags | 2026-05-20 | 2026-05-20 | spec V3 §41 (new this commit) |
| HRD-017 | task | open | low | Propagate the new Universal §11.4.6X spec-versioning mandate into the parent constitution; add corresponding HERALD_CONSTITUTION §107 | 2026-05-20 | 2026-05-20 | pending in the constitution-submodule commit immediately following |

## In progress

| ID | Type | Status | Criticality | Title | Opened | Last update | Reference |
|---|---|---|---|---|---|---|---|
| HRD-008 | task | in_progress | middle | Operator-side quickstart compose validation (Postgres + Redis + OTel + pherald container) | 2026-05-20 | 2026-05-20 | spec V3 §26.5 — scaffold shipped this commit; live end-to-end run pending operator. |

## Blocked

(none)

## Conventions

See [`Fixed.md`](Fixed.md) for closed items + Universal §11.4.12/.15/.16/.19/.33/.55 composition rules.
