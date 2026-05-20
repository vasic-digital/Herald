# Herald — Status

| Field | Value |
|---|---|
| Revision | 6 |
| Created | 2026-05-20 |
| Last modified | 2026-05-20 |
| Status | active |
| Status summary | **Foundation M1 + M2 + M3 all landed.** M1: `commons_constitution` (14 files, ~2.9k LOC) — Evaluator + Registry + 12 emit helpers + BundleHash + ModeLadder + ConstitutionStore + MemoryBus + Runner + CloudEvents v1.0 adapter. M2 (commit c8eed7e): Postgres backends with live RLS-tenant-isolation proof. M3 (commit 21593e6): Gin REST surface + 4 M3 submodules + LIVE `pherald serve` accepting HTTP on :24791. Anti-bluff regime now complete: e2e_bluff_hunt.sh 14/14 PASS against real services (commit 92ecdc6); audit_antibluff.sh 14/14 PASS (was 13 — added I8 meta-test for §107 covenant on 2026-05-20); inheritance gate 15/15 PASS (was 12 — added I8a/b/c). |
| Issues | HRD-008, HRD-010, HRD-011, HRD-012, HRD-015, HRD-016, HRD-018 (in_progress), HRD-019..HRD-028, HRD-029..HRD-056, HRD-081 |
| Issues summary | see `Issues.md` — 45 open items spanning live channel integrations (HRD-010..012), REST surface (HRD-016), §42 constitution bindings (HRD-018..028), §43 command-catalogue (HRD-029..056), and HRD-081 (containers podman-compose adapter refinement). |
| Fixed | HRD-001..HRD-007, HRD-009, HRD-009b, HRD-013, HRD-014, HRD-017 |
| Fixed summary | see `Fixed.md` |
| Continuation | Foundation complete; live integrations next. Priority order: HRD-008 (operator quickstart e2e validation) → HRD-011/012 (Telegram + Claude Code live) → HRD-010 (commons_storage live pgx/River/Redis wiring) → §42 constitution-binding rollout (HRD-018..028) → §43 command catalogue (HRD-029..056). Each milestone followed by multi-mirror push + full anti-bluff verification (audit + gate + e2e bluff-hunt). |

## Table of contents

- [Project phase](#project-phase)
- [Specification](#specification)
- [Implementation](#implementation)
- [Operations](#operations)
- [Risk surface](#risk-surface)

## Project phase

**Implementation-r1, Foundation complete.** Spec V3 r6 is active, the Go module foundation compiles, all three Foundation milestones (M1/M2/M3) have landed, and the §107 end-user-usability covenant is now explicit in Herald's three root docs + gate (added 2026-05-20). The remaining cycle is wiring live channel integrations and validating the §26.5 quickstart end-to-end on operator hardware.

## Specification

- **Active spec:** `docs/specs/mvp/specification.V3.md` (Revision 7, ~4300 lines).
- **Archived specs:** V1 + V2 in `docs/specs/mvp/archive/` (frozen).
- **Recent sections:** §37 Tracker-doc change events; §38 Workable-item announcement contract; §39 Message presentation + Herald Canonical Template; §40 Documentation + 15 named test challenges; §41 REST API surface (Gin Gonic); §42 Constitution-binding integration; §43 Constitution-derived flavor commands (27 entries → HRD-029..HRD-056); §44 Foundation implementation contract.
- **Spec-change rule:** §23 + HERALD_CONSTITUTION §106 + I7 gate — green throughout.
- **End-user-usability covenant:** HERALD_CONSTITUTION §107 + I8 gate (added 2026-05-20) — green; paired §1.1 mutation meta-test in `tests/test_i8_usability_meta.sh` (5/5 PASS).

## Implementation

| Module | Status | Notes |
|---|---|---|
| `commons` (L0) | ✅ landed | §11.0 types + Clock + UUIDv7 + DefaultBranding; tests pass. |
| `commons_prefix` | ✅ landed | §8.2 algorithm + 5 test functions; tests pass. |
| `commons_constitution` | ✅ landed (M1) | 14 files, ~2.9k LOC — Evaluator + Registry + 12 emit helpers + BundleHash + ModeLadder + ConstitutionStore + MemoryBus + Runner + CloudEvents v1.0 adapter; all green under `go test -race`. |
| `commons_messaging` (L1) | partial | null:// adapter fully working; Telegram + Claude Code stubs in place (HRD-011/012 for live wiring). |
| `commons_storage` (L1) | ✅ landed (M2) | 9 SQL migrations embedded; pgx + RLS-tenant-isolation live (commit c8eed7e); River + Redis still to wire per HRD-010. |
| `commons_infra` | ✅ landed | QuickstartBoot + WithTenantContext + on-demand container orchestration via `containers/`. |
| `pherald` (CLI) | ✅ partial (M3) | Cobra root + version + **`serve` (live HTTP on :24791, /v1/healthz, /v1/readyz, /v1/events, /metrics)**, commit 21593e6. send/doctor/migrate/subscriber/deadletter stubbed with HRD pointers. |
| Other flavors (sherald/bherald/cherald/rherald/iherald/scherald) | not started | scaffold once HRD-018..028 §42 binding rollout begins. |

## Operations

- **Repo hygiene:** `main` only; four-mirror fan-out proven across spec + Foundation M1/M2/M3 + anti-bluff infra + §107 covenant landing.
- **Constitution inheritance gate:** **15 PASS / 0 FAIL** (was 12; I8a/b/c added 2026-05-20 for §107 covenant).
- **Anti-bluff audit:** **14 PASS / 0 FAIL** (was 13; I8 paired meta-test added).
- **CodeGraph validate:** 7 PASS / 0 FAIL.
- **E2E bluff-hunt against real services:** **14 PASS / 0 FAIL** — builds pherald, starts real Gin server, hits live `/v1/*` endpoints, boots real Postgres container via `containers/` submodule, runs M2 RLS integration tests, SIGTERM-graceful-shutdowns. This is the canonical §107 evidence per Herald §107.
- **Submodules:** 10 vendored — 9 Helix-stack capability modules under `submodules/` (`auth`, `background`, `cache`, `config`, `database`, `eventbus`, `middleware`, `observability`, `recovery`) referenced via `replace` directives in consuming `go.mod` files, NOT via `go.work`; plus `containers/` (runtime auto-detection + on-demand container boot, consumed directly by Foundation tests + `pherald doctor`). All 10 carry the §11.4 anti-bluff anchor.
- **Build / test toolchain:** Go 1.25+ (verified on 1.26 per CLAUDE.md), Pandoc 3.9, WeasyPrint 66, Podman 5.8 — verified locally.

## Risk surface

- **Submodule-catalogue discovery (operator mandate 2026-05-20).** Every new HRD MUST carry a `Catalogue-Check: reuse|extend|no-match <org/repo>@<sha>` line per Universal §11.4.74. New HRDs (HRD-029..056 §43 catalogue) currently default to `TBD` pending operator review.
- **Versioning discipline (operator mandate 2026-05-20).** Minor changes bump secondary version, major bump primary. Currently at V3 r6→r7 within the V3 series; a full V4 rewrite is reserved for a primary-version jump.
- **Live channel integrations untested at runtime.** HRD-011 (Telegram telebot live) + HRD-012 (Claude Code `claude --resume` live) require operator-supplied credentials per `docs/CONTINUATION.md`. The §107 covenant blocks claims of "done" on these until end-user evidence is captured.
- **HRD-008 operator-side quickstart not yet validated end-to-end on a fresh laptop.** The compose stack works in dev; operator hardware validation is the next concrete §107 evidence point.
- **No CI yet.** No GitHub Actions / GitLab CI / etc. Operator runs `scripts/e2e_bluff_hunt.sh` manually for now — that script IS the local CI surrogate.
