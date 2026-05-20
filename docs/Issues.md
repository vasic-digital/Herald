# Herald — Issues

| Field | Value |
|---|---|
| Revision | 1 |
| Created | 2026-05-20 |
| Last modified | 2026-05-20 |
| Status | active |
| Status summary | Pre-implementation tracker established 2026-05-20 (V3 r3). Workable-item prefix `HRD-` per spec §8.1. First six spec-revision items retroactively recorded in `Fixed.md`. |
| Issues | HRD-007 |
| Issues summary | one open: V3 r3 cross-doc sync — closed by the same commit that introduces this file. |
| Fixed | HRD-001..HRD-006 (see `Fixed.md`) |
| Fixed summary | V1 + V2 (r1/r2/r3) + V3 (r1/r2) spec-revision work landed across commits b421fe1 → f8b8073. |
| Continuation | First-implementation cycle starts here. New items (HRD-008+) opened as work begins; per Universal §11.4.12, every commit that introduces a non-trivial change MUST reference its `HRD-` id. |

## Table of contents

- [Format and scope](#format-and-scope)
- [Open](#open)
- [In progress](#in-progress)
- [Blocked](#blocked)
- [Conventions](#conventions)

## Format and scope

`Issues.md` is the canonical open-work tracker for the Herald repository (per Universal Constitution §11.4.12 + §11.4.15 + §11.4.16). Every entry has:

- **ID** — `HRD-NNN`, zero-padded, monotonic per project (per spec §8.1).
- **Type** — one of `bug | issue | task | implementation | investigation | query | request` (per Universal §11.4.16; spec §32.6).
- **Status** — `open | in_progress | blocked | operator-blocked` (per Universal §11.4.15 / §11.4.21).
- **Criticality** — `critical | high | middle | low` (per spec §18.2.2).
- **Opened** — ISO 8601 date.
- **Last update** — ISO 8601 date; the auto-bump script updates this whenever the row changes.
- **Reference** — link to investigation doc / PR / external tracker.

When an item is resolved its row migrates atomically (per Universal §11.4.19) from this file into `Fixed.md`.

## Open

(none — first-implementation cycle has not started)

## In progress

| ID | Type | Status | Criticality | Title | Opened | Last update | Reference |
|---|---|---|---|---|---|---|---|
| HRD-007 | task | in_progress | middle | V3 r3 cross-doc sync (this file + parent-doc path refs + full re-export) | 2026-05-20 | 2026-05-20 | spec §30.8 |

## Blocked

(none)

## Conventions

- New items: append to the **Open** section; allocate next sequence via the `workable_items` schema (spec §8.3) once that schema exists; for now, manually increment.
- Status transitions: edit row in place; `Last update` MUST move; status vocabulary follows Universal §11.4.15 + §11.4.33.
- Closure: move the row atomically (one commit) from `Issues.md` to `Fixed.md` (per Universal §11.4.19); record closure date + outcome.
- Reopens: per Universal §11.4.55 — move back to `Issues.md` with `status=in_progress` and create / append to `Reopens/<HRD-NNN>.md`.
- Sync to summary: `Issues_Summary.md` is regenerated whenever this file changes (per Universal §11.4.12).
- Diary/exports: `.html` + `.pdf` siblings stay in sync via Pandoc + WeasyPrint (per Universal §11.4.61 + §11.4.65).
