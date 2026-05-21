<div align="center">

![Herald](../assets/logo/herald_logo_square_128.png){width=96px height=96px}

</div>

# Herald — Status

| Field | Value |
|---|---|
| Revision | 9 |
| Created | 2026-05-20 |
| Last modified | 2026-05-21 |
| Status | active |
| Status summary | **Wave 2 flavor scaffolds + spec V3 r8 landed (commits 7e0a614..eef606b + 24b96f2 logo branding parallel work).** Shared `commons/cli/` package + 6 new flavor binaries (sherald, cherald, bherald, rherald, iherald, scherald) + pherald refactor (consumes cli.ServeCmd with RequestIDMiddleware hook). Branding struct extended with 5 per-flavor fields (Flavor/Prefix/DisplayName/DefaultPort/Mission); DefaultBranding factory populates them. e2e_bluff_hunt grew 18 → 33 invariants; paired mutation gate `tests/test_wave2_mutation_meta.sh` (4/4 PASS) proves the new invariants catch the regressions they claim. Spec V3 r7 → r8 captured §6.3 Branding extension + §18.0 Wave 2 catalogue + §41 REST routes + §44.M milestone + §43.2 catalogue-check addendum. HRD-092..097 atomically Issues→Fixed in r8 Issues / r7 Fixed; HRD-098 opened as Wave 3+ deferred (sherald /v1/safety_state live). |
| Issues | HRD-008, HRD-011, HRD-015, HRD-016, HRD-018 (in_progress), HRD-019..HRD-028, HRD-029..HRD-056, HRD-081, HRD-085..HRD-090, HRD-098 |
| Issues summary | see `Issues.md` — 51 open items spanning live channel integrations (HRD-011), REST surface (HRD-016), §42 constitution bindings (HRD-018..028), §43 command-catalogue (HRD-029..056), HRD-081 (containers podman-compose adapter refinement), HRD-085..090 (queue-repository stub-completion surface opened during HRD-010), and HRD-098 (sherald /v1/safety_state live impl, Wave 3+). |
| Fixed | HRD-001..HRD-007, HRD-009, HRD-009b, HRD-010, HRD-012, HRD-013, HRD-014, HRD-017, HRD-080, HRD-092, HRD-093, HRD-094, HRD-095, HRD-096, HRD-097 |
| Fixed summary | see `Fixed.md` |
| Continuation | Wave 3 priorities: HRD-016 (pherald Runner wiring), HRD-024 (iherald paging live), HRD-028 (cherald compliance live), HRD-098 (sherald safety_state live), and the 28 §43 command bodies. Operator priorities remain: HRD-008 (quickstart e2e validation) and HRD-011 (Telegram live evidence pending operator creds). Each milestone followed by multi-mirror push + full anti-bluff verification (audit + gate + e2e bluff-hunt). |

## Table of contents

- [Project phase](#project-phase)
- [Specification](#specification)
- [Implementation](#implementation)
- [Operations](#operations)
- [Risk surface](#risk-surface)

## Project phase

**Implementation-r1, Foundation complete + Wave 2 flavor scaffolds landed.** Spec V3 r8 is active, the Go module foundation compiles, all three Foundation milestones (M1/M2/M3) plus the Wave 2 flavor-scaffold workstream have landed, and the §107 end-user-usability covenant is now explicit in Herald's three root docs + gate (added 2026-05-20). The remaining cycle is wiring live channel integrations (HRD-011, HRD-016, HRD-024, HRD-028, HRD-098), validating the §26.5 quickstart end-to-end on operator hardware, and implementing the 28 §43 command bodies still stubbed.

## Specification

- **Active spec:** `docs/specs/mvp/specification.V3.md` (Revision 8, ~4600 lines after r8 Wave 2 capture).
- **Archived specs:** V1 + V2 in `docs/specs/mvp/archive/` (frozen).
- **Recent sections:** §37 Tracker-doc change events; §38 Workable-item announcement contract; §39 Message presentation + Herald Canonical Template; §40 Documentation + 15 named test challenges; §41 REST API surface (Gin Gonic); §42 Constitution-binding integration; §43 Constitution-derived flavor commands (27 entries → HRD-029..HRD-056) + §43.2 catalogue-check addendum (r8); §44 Foundation implementation contract + §44.M Wave 2 milestone capture (r8); §6.3 Branding extension with per-flavor fields (r8); §18.0 Wave 2 flavor-binary catalogue (r8).
- **Spec-change rule:** §23 + HERALD_CONSTITUTION §106 + I7 gate — green throughout.
- **End-user-usability covenant:** HERALD_CONSTITUTION §107 + I8 gate (added 2026-05-20) — green; paired §1.1 mutation meta-test in `tests/test_i8_usability_meta.sh` (5/5 PASS).

## Implementation

| Module | Status | Notes |
|---|---|---|
| `commons` (L0) | ✅ landed | §11.0 types + Clock + UUIDv7 + DefaultBranding; tests pass. |
| `commons_prefix` | ✅ landed | §8.2 algorithm + 5 test functions; tests pass. |
| `commons_constitution` | ✅ landed (M1) | 14 files, ~2.9k LOC — Evaluator + Registry + 12 emit helpers + BundleHash + ModeLadder + ConstitutionStore + MemoryBus + Runner + CloudEvents v1.0 adapter; all green under `go test -race`. |
| `commons_messaging` (L1) | partial | null:// adapter fully working; Telegram + Claude Code stubs in place (HRD-011/012 for live wiring). |
| `commons_storage` (L1) | ✅ landed (M2 + HRD-010) | 9 SQL migrations embedded; pgx pool + RLS-enforcing WithTenantContext live (commit c8168ec); HRD-010 (commits 82ea82d..13cea69) wired queue (digital.vasic.background → pgxTaskRepository) + Redis (digital.vasic.cache) + the `pherald migrate up/status/down/validate` CLI. The §107 covenant proved its worth here: the E14 RLS falsifiability test caught (and the implementer fixed) a real production RLS-bypass bug where the bootstrap PG superuser silently bypassed `FORCE ROW LEVEL SECURITY`. HRD-085..090 opened to track the remaining 16 pgxTaskRepository methods returning `ErrUnsupported`. |
| `commons_infra` | ✅ landed | QuickstartBoot + WithTenantContext + on-demand container orchestration via `containers/`. |
| `commons/cli` (L0) | ✅ landed (Wave 2 / HRD-092) | Shared CLI scaffold: NewRootCmd + VersionCmd + ServeCmd (with Middleware hook) + StubCmd + healthz/readyz/metrics handlers. Consumed by pherald + 6 new flavor binaries. Catalogue-check no-match → vendored Herald-internal per §11.4.74. |
| `pherald` (CLI) | ✅ partial (M3 + Wave 2 refactor) | Cobra root + version + **`serve` (live HTTP on :24791, /v1/healthz, /v1/readyz, /v1/events, /metrics)** now consumes cli.ServeCmd with RequestIDMiddleware hook (commits 1f81d69, 31562cf). send/doctor/migrate/subscriber/deadletter stubbed with HRD pointers. |
| `sherald` (CLI) | ✅ scaffolded (Wave 2 / HRD-093) | System Herald binary serving :24793 via cli.ServeCmd. 5 §43 stubs (HRD-033/034/040/046/056). `/v1/safety_state` returns honest 501 + HRD-098. |
| `cherald` (CLI) | ✅ scaffolded (Wave 2 / HRD-094) | Constitution Herald binary serving :24792 via cli.ServeCmd. 11 §43 stubs. `/v1/compliance` returns honest 501 + HRD-028. |
| `bherald` (CLI) | ✅ scaffolded (Wave 2 / HRD-095) | Build Herald CLI-only binary. 3 §43 stubs (HRD-035/041/045). |
| `rherald` (CLI) | ✅ scaffolded (Wave 2 / HRD-096) | Release Herald CLI-only binary. 3 §43 stubs (HRD-031/032/045). |
| `iherald` (CLI) | ✅ scaffolded (Wave 2 / HRD-097) | Incident Herald binary serving :24794 via cli.ServeCmd. `/v1/webhooks/page` returns honest 501 + HRD-024. |
| `scherald` (CLI) | ✅ scaffolded (Wave 2 / HRD-097) | Scheduled-audit Herald CLI-only binary. 1 §43 stub (HRD-047). |

## Operations

- **Repo hygiene:** `main` only; four-mirror fan-out proven across spec + Foundation M1/M2/M3 + anti-bluff infra + §107 covenant landing.
- **Constitution inheritance gate:** **15 PASS / 0 FAIL** (was 12; I8a/b/c added 2026-05-20 for §107 covenant).
- **Anti-bluff audit:** **16 PASS / 0 FAIL / 1 SKIP** (was 14; HRD-011 telebot.v3 third-party SKIP-with-reason per §11.4.74; I8 paired meta-test included).
- **CodeGraph validate:** 7 PASS / 0 FAIL / 2 SKIP (HRD-091 documented submodule-traversal gap).
- **E2E bluff-hunt against real services:** **33 PASS / 0 FAIL / 3 SKIP** (was 18; Wave 2 added E19-E33 covering 6 new flavor binaries: version --json shape, serving flavors healthz on :249NN, /v1/safety_state 501, /v1/compliance 501, /v1/webhooks/page 501, sherald SIGTERM graceful-shutdown, §43 stub exit-code, pherald regression sentinel). SKIPs are E17/E18/E34 (Telegram live, claude live, full inbound) requiring operator credentials. Real Gin server, live `/v1/*` endpoints, real Postgres + Redis containers via `containers/` submodule, M2 RLS integration tests + HRD-010 storage tests against the live stack, SIGTERM-graceful-shutdowns. Paired §1.1 mutation gate `tests/test_wave2_mutation_meta.sh` (4/4 PASS) proves the new Wave 2 invariants catch the regressions they claim. This is the canonical §107 evidence per Herald §107.
- **Submodules:** 12 vendored — 11 Helix-stack capability modules under `submodules/` (`auth`, `background`, `cache`, `config`, `database`, `eventbus`, `middleware`, `observability`, `recovery`, plus `Models` + `Concurrency` added during HRD-010) referenced via `replace` directives in consuming `go.mod` files, NOT via `go.work`; plus `containers/` (runtime auto-detection + on-demand container boot, consumed directly by Foundation tests + `pherald doctor`). All 12 carry the §11.4 anti-bluff anchor.
- **Build / test toolchain:** Go 1.25+ (verified on 1.26 per CLAUDE.md), Pandoc 3.9, WeasyPrint 66, Podman 5.8 — verified locally.

## Risk surface

- **Submodule-catalogue discovery (operator mandate 2026-05-20).** Every new HRD MUST carry a `Catalogue-Check: reuse|extend|no-match <org/repo>@<sha>` line per Universal §11.4.74. New HRDs (HRD-029..056 §43 catalogue) currently default to `TBD` pending operator review.
- **Versioning discipline (operator mandate 2026-05-20).** Minor changes bump secondary version, major bump primary. Currently at V3 r7→r8 within the V3 series; a full V4 rewrite is reserved for a primary-version jump.
- **Live channel integrations untested at runtime.** HRD-011 (Telegram telebot live) requires operator-supplied credentials per `docs/CONTINUATION.md`. HRD-012 (Claude Code `claude --resume` live) is closed with live PASS evidence captured. The §107 covenant blocks claims of "done" on these until end-user evidence is captured.
- **HRD-008 operator-side quickstart not yet validated end-to-end on a fresh laptop.** The compose stack works in dev; operator hardware validation is the next concrete §107 evidence point.
- **No CI yet.** No GitHub Actions / GitLab CI / etc. Operator runs `scripts/e2e_bluff_hunt.sh` manually for now — that script IS the local CI surrogate.

### Closed risks

- **Production RLS-bypass bug (closed 2026-05-20 during HRD-010 step 3, commit `5c1c022`).** Until that commit, `commons_infra.WithTenantContext` set the tenant GUC but `ROW LEVEL SECURITY` was not `FORCE`d on the targeted tables — and the bootstrap PG user used in tests was a superuser, which bypasses RLS regardless. The E14 falsifiability test (real Postgres, cross-tenant SELECT must return EXACTLY 0 rows from the other tenant) caught the bluff before any live deployment. Fix: switched the application connection to a non-superuser role + ran `ALTER TABLE … FORCE ROW LEVEL SECURITY` in migration 000003. The §107 covenant directly prevented this from shipping; the lesson is "EXACT-N row count, never >=N" for any isolation test.

## Recent activity

### Wave 2 — flavor-scaffold sweep (closed 2026-05-21)

15-step workstream commits `7e0a614..eef606b` (with parallel logo branding `24b96f2`) shipped:

- **Shared scaffold** — new `commons/cli/` package: `NewRootCmd` (steps 2), `VersionCmd` (step 2), `routes.go` + healthz/readyz/metrics handlers (step 3), `ServeCmd` with Middleware-hook + graceful shutdown (step 4), `StubCmd` honest 501-style stubs (step 1). Vendored Herald-internal per §11.4.74 catalogue-check no-match → see `docs/catalogue-checks/HRD-092-commons-cli.md`.
- **pherald refactor** — main + stubs migrated to consume `commons/cli/` (step 5); `pherald serve` consumes `cli.ServeCmd` with RequestIDMiddleware hook (step 6, commit `31562cf`).
- **6 new flavor binaries** — sherald serving :24793 (step 7, HRD-093), cherald serving :24792 (step 8, HRD-094), iherald serving :24794 (step 9, HRD-097), bherald CLI-only (step 10, HRD-095), rherald CLI-only (step 11, HRD-096), scherald CLI-only (step 12, HRD-097). Each ships honest 501-style §43 command stubs with HRD pointers.
- **Anti-bluff additions** — `scripts/e2e_bluff_hunt.sh` grew from 18 → 33 invariants covering all 6 new binaries (version --json shape, healthz binding, /v1/safety_state + /v1/compliance + /v1/webhooks/page 501 honesty, sherald SIGTERM graceful-shutdown, §43 stub exit-code, pherald regression sentinel). New paired mutation gate `tests/test_wave2_mutation_meta.sh` (4/4 PASS) proves the new invariants catch their claimed regressions.
- **Spec capture** — V3 r7 → r8 added §6.3 Branding extension (per-flavor fields), §18.0 Wave 2 catalogue, §41 REST route additions, §44.M Wave 2 milestone, §43.2 catalogue-check addendum (step 14, commit `eef606b`).
- **Atomic Issues→Fixed** — HRD-092..097 closed in r7 of Fixed.md; HRD-098 opened in r8 of Issues.md as Wave 3+ deferred (sherald /v1/safety_state live impl).

Anti-bluff battery sign-off (this commit): inheritance gate 15/15 PASS; meta-tests (I6, I8, Wave2) all green; `audit_antibluff.sh` 16 PASS / 0 FAIL / 1 SKIP; `codegraph_validate.sh` 7 PASS / 0 FAIL / 2 SKIP; `e2e_bluff_hunt.sh` 33 PASS / 0 FAIL / 3 SKIP.
