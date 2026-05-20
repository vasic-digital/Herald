# Herald — Status

| Field | Value |
|---|---|
| Revision | 2 |
| Created | 2026-05-20 |
| Last modified | 2026-05-20 |
| Status | active |
| Status summary | First-implementation cycle r1 LANDED. Spec V3 r4 (5 new sections §37–§41). Five Go modules compile + tests pass. Quickstart compose scaffolded for operator validation. Live integrations (Postgres+River / Telegram / Claude Code / Gin REST) pending. |
| Issues | HRD-008, HRD-010, HRD-011, HRD-012, HRD-015, HRD-016, HRD-017 |
| Issues summary | see `Issues.md` |
| Fixed | HRD-001..HRD-007 (prior) + HRD-009/HRD-009b/HRD-013/HRD-014 (this commit) |
| Fixed summary | see `Fixed.md` |
| Continuation | next concrete step: HRD-008 live operator-side quickstart compose validation (Postgres + Redis + OTel + pherald container). After that: HRD-010 (live Postgres + River wiring), HRD-011 (Telegram adapter), HRD-012 (Claude Code live), HRD-016 (REST API). |

## Table of contents

- [Project phase](#project-phase)
- [Specification](#specification)
- [Implementation](#implementation)
- [Operations](#operations)
- [Risk surface](#risk-surface)

## Project phase

**Implementation-r1.** Spec V3 r4 is complete + the Go module foundation compiles. The remaining cycle is wiring live integrations and validating the §26.5 quickstart end-to-end.

## Specification

- **Active spec:** `docs/specs/mvp/specification.V3.md` (Revision 4, ~4300 lines).
- **Archived specs:** V1 + V2 in `docs/specs/mvp/archive/` (frozen).
- **New sections this cycle:** §37 Tracker-doc change events; §38 Workable-item announcement contract; §39 Message presentation + Herald Canonical Template; §40 Documentation + 15 named test challenges; §41 REST API surface (Gin Gonic).
- **Spec-change rule:** §23 + HERALD_CONSTITUTION §106 + I7 gate — green throughout.

## Implementation

| Module | Status | Notes |
|---|---|---|
| `commons` (L0) | ✅ landed | §11.0 types + Clock + UUIDv7 + DefaultBranding; tests pass. |
| `commons_prefix` | ✅ landed | §8.2 algorithm + 5 test functions; tests pass. |
| `commons_messaging` (L1) | partial | null:// adapter fully working; Telegram + Claude Code stubs in place. |
| `commons_storage` (L1) | partial | 9 SQL migrations embedded; pgx pool + River + Redis wiring pending HRD-010. |
| `commons_security` (L1) | not started | spec §15 ready. HRD-018 follow-up. |
| `commons_observability` (L1) | not started | OTel wiring per §17. HRD-019 follow-up. |
| `commons_diary` (L1) | not started | spec §19 — fsnotify + Pandoc-WeasyPrint sync. HRD-020 follow-up. |
| `pherald` (CLI) | partial | Cobra root + version subcommand work end-to-end; serve/send/doctor/migrate/subscriber/deadletter stubbed with HRD pointers. |
| Other flavors (sherald/bherald/…) | not started | scaffold once pherald end-to-end runs the quickstart compose. |

## Operations

- **Repo hygiene:** `main` only; four-mirror fan-out proven across spec + Go-scaffold commits.
- **Constitution inheritance gate:** 12 PASS / 0 FAIL.
- **Submodules:** none yet (`containers/` is a local directory in this commit; will migrate to `vasic-digital/containers` submodule when HRD-008 validation passes and the submodule is created).
- **Build / test toolchain:** Go 1.22+, Pandoc 3.9, WeasyPrint 66, Podman 5.8 — verified locally.

## Risk surface

- **Submodule-catalogue discovery (operator mandate 2026-05-20).** Before scaffolding more modules, Herald work MUST audit `vasic-digital` + `HelixDevelopment` orgs (GitHub + GitLab) for reusable Submodules — pending in the upcoming constitution-submodule mandate.
- **Versioning discipline (operator mandate 2026-05-20).** Minor changes bump secondary version, major bump primary. Currently we're at Revision 4 within V3; the next "minor" addition continues incrementing Revision; a full V4 rewrite is reserved for a primary-version jump.
- **Live integrations untested.** HRD-008 quickstart compose hasn't been verified end-to-end on a fresh laptop yet — that's the next concrete operator-driven validation.
- **Claude Code session model.** §33.2 anchor file pattern hasn't been validated against the live `claude` binary yet; HRD-012 will deliver that evidence.
- **No CI yet.** No GitHub Actions / GitLab CI / etc. Operator runs `go test ./...` manually for now.
