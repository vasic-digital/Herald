# Herald — Issues

| Field | Value |
|---|---|
| Revision | 3 |
| Created | 2026-05-20 |
| Last modified | 2026-05-20 |
| Status | active |
| Status summary | Constitution-flavor binding plan landed in spec V3 §42 (65 rule→flavor bindings); HRD-018..HRD-028 opened for the implementation rollout. HRD-017 closed (universal §11.4.73 + §11.4.74 mandates landed in constitution commit 34a82b3). |
| Issues | HRD-008, HRD-010, HRD-011, HRD-012, HRD-015, HRD-016, HRD-018..HRD-028 |
| Issues summary | 17 open / in-progress workable items spanning quickstart validation, live channel integrations, REST surface, and the §42 constitution-binding rollout. |
| Fixed | HRD-001..HRD-007, HRD-009, HRD-009b, HRD-013, HRD-014, HRD-017 |
| Fixed summary | see `Fixed.md`. |
| Continuation | see `CONTINUATION.md`. |

## Table of contents

- [Open](#open)
- [In progress](#in-progress)
- [Blocked](#blocked)
- [Conventions](#conventions)

## Open

Per Universal §11.4.74 every new row carries a `Catalogue-Check` line in its References cell (one of `reuse|extend|no-match <org/repo>@<sha>`). For HRDs whose catalogue-check is still pending, the row records `no-match (2026-05-20)` — operators reviewing the implementation PR will re-verify.

| ID | Type | Status | Criticality | Title | Opened | Last update | References |
|---|---|---|---|---|---|---|---|
| HRD-010 | task | open | middle | commons_storage live wiring (golang-migrate driver, pgx pool, River queue, Redis ACL) | 2026-05-20 | 2026-05-20 | spec V3 §9.6 + §16; Catalogue-Check: no-match (2026-05-20) — golang-migrate is the standard, not a vasic/Helix-owned thing |
| HRD-011 | task | open | middle | Telegram channel adapter live integration (telebot SDK + getUpdates long-poll + webhook secret_token) | 2026-05-20 | 2026-05-20 | spec V3 §11.1; Catalogue-Check: no-match (2026-05-20) |
| HRD-012 | task | open | middle | Claude Code dispatcher live integration (`claude --resume` + parse `<<<HERALD-REPLY>>>`) | 2026-05-20 | 2026-05-20 | spec V3 §33; Catalogue-Check: no-match (2026-05-20) |
| HRD-015 | task | open | low | Inheritance gate I8 invariants for Go scaffold (go.work + commons/types.go + null adapter test passes) | 2026-05-20 | 2026-05-20 | spec V3 §40 + gate I7 pattern; Catalogue-Check: no-match (2026-05-20) |
| HRD-016 | task | open | middle | REST API surface via Gin Gonic per spec §41 — pherald/internal/http/ with /v1/* routes + JWT auth + OpenAPI tags | 2026-05-20 | 2026-05-20 | spec V3 §41; Catalogue-Check: no-match (2026-05-20) — Gin is external |
| HRD-018 | task | open | high | `commons_constitution` Go package: Evaluator interface + 12 event-class emit helpers + constitution_state + constitution_bindings table migrations + bundle-hash captureer + mode-ladder runtime config | 2026-05-20 | 2026-05-20 | spec V3 §42.1 / §42.5 step 1; Catalogue-Check: TBD — agent MUST survey vasic-digital + HelixDevelopment for existing rule-evaluation/audit modules before scaffolding |
| HRD-019 | task | open | high | `cherald` constitution bindings — bulk implementation: ~30 `.policy.violation` rules + `.gate.failed`/`.gate.recovered`/`.credential.leak`/`.spec.revision_drift`/`.catalogue.miss` handlers; `/v1/compliance` pull surface | 2026-05-20 | 2026-05-20 | spec V3 §42.3 cherald rows + §42.5 step 2; Catalogue-Check: TBD |
| HRD-020 | task | open | high | `sherald` host-safety + repo-safety bindings (§9.1/.2/.3, §12.1/.2/.3/.6, §11.4.32, §11.4.36, §11.4.41, §11.4.71) — destructive-op detection hook, force-push interceptor, mem-budget watcher | 2026-05-20 | 2026-05-20 | spec V3 §42.3 sherald rows + §42.5 step 3; Catalogue-Check: TBD |
| HRD-021 | task | open | middle | `bherald` CI/test bindings (§1, §11.4.2/.3/.4/.5/.7/.13/.14/.24/.27/.39/.43/.46/.48–.52/.67) — gate-result event emitters; integrates with the consuming project's CI workflow | 2026-05-20 | 2026-05-20 | spec V3 §42.3 bherald rows + §42.5 step 4; Catalogue-Check: TBD |
| HRD-022 | task | open | middle | `rherald` release bindings (§4 tag mirroring, §5 changelog, §11.4.38 installable-asset evidence, §11.4.40 full-suite retest) | 2026-05-20 | 2026-05-20 | spec V3 §42.3 rherald rows + §42.5 step 5; Catalogue-Check: TBD |
| HRD-023 | task | open | middle | `pherald` project bindings (§2 commit+push, §3 submodule propagation, §11.4.11/.15/.21/.22/.34/.37/.42/.55/.66/.71/.74) | 2026-05-20 | 2026-05-20 | spec V3 §42.3 pherald rows + §42.5 step 6; Catalogue-Check: TBD |
| HRD-024 | task | open | middle | `iherald` constitution-rule escalation bindings (§11.4.10/.10.A credential-leak page-out, §11.4.21 + §11.4.66 operator-blocked escalation) | 2026-05-20 | 2026-05-20 | spec V3 §42.3 iherald rows + §42.5 step 7; Catalogue-Check: TBD |
| HRD-025 | task | open | low | `scherald` scheduled-audit bindings (§11.4.45 periodic Status.md sweep + daily/weekly/monthly compliance digest) | 2026-05-20 | 2026-05-20 | spec V3 §42.3 scherald rows + §42.5 step 8; Catalogue-Check: TBD |
| HRD-026 | task | open | middle | Constitution-bundle hash captureer — computes SHA-256 of rendered Constitution.md at evaluation time; persists on every emitted event for replayability | 2026-05-20 | 2026-05-20 | spec V3 §42.1.3; Catalogue-Check: TBD |
| HRD-027 | task | open | middle | Mode-ladder runtime config (`constitution_bindings` table + admin REST endpoints to flip allow/warn/enforce per binding per tenant without redeploy) | 2026-05-20 | 2026-05-20 | spec V3 §42.1.4; Catalogue-Check: TBD |
| HRD-028 | task | open | low | `/v1/compliance` pull surface — Gin handler returning `constitution_state` rows filtered by rule / subject / decision; paginated | 2026-05-20 | 2026-05-20 | spec V3 §42.1.5 + §41; Catalogue-Check: TBD |

## In progress

| ID | Type | Status | Criticality | Title | Opened | Last update | References |
|---|---|---|---|---|---|---|---|
| HRD-008 | task | in_progress | middle | Operator-side quickstart compose validation (Postgres + Redis + OTel + pherald container) | 2026-05-20 | 2026-05-20 | spec V3 §26.5 — scaffold shipped, live end-to-end run pending operator; Catalogue-Check: no-match (2026-05-20) — bespoke to Herald |

## Blocked

(none)

## Conventions

See [`Fixed.md`](Fixed.md) for closed items + Universal §11.4.12/.15/.16/.19/.33/.55/.73/.74 composition rules.
