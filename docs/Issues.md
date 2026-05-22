<div align="center">

<img src="../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Herald — Issues

| Field | Value |
|---|---|
| Revision | 12 |
| Created | 2026-05-20 |
| Last modified | 2026-05-22 |
| Status | active |
| Status summary | r12 closes **HRD-016 (pherald `/v1/events` Runner wiring)** atomically — Wave 3b lands the §32 7-stage Runner orchestrator end-to-end + the live HTTP handler at `pherald/internal/http/events.go`. 23/23 runner package tests green; 6 new e2e invariants E37-E42 (and revised E10) cover the live path under PG :24100 reachability; mutation gate gains M2/M3/M4 (M3 SKIP-with-reason pending Wave 3c deny-evaluator). Consumer-project integration is now unlocked. Prior r11 closed HRD-011 (Telegram live integration) atomically with live evidence (message_id=5). r10 renumbered HRD-093 commons_auth scaffold → HRD-099. r9 captured Wave 3a close-out (HRD-028 cherald + HRD-098 sherald). |
| Issues | HRD-008, HRD-015, HRD-018..HRD-027, HRD-029..HRD-056, HRD-081, HRD-085..HRD-090 |
| Issues summary | 47 open / in-progress workable items. r12: HRD-016 closed atomically (Wave 3b pherald Runner live + e2e E37-E42 PASS or honest SKIP-with-reason when PG :24100 absent). r11: HRD-011 (Telegram live) closed atomically with live message_id=5 evidence. r10 doc-cleanup renumbered HRD-093 → HRD-099. r9: HRD-028 + HRD-098 closed (Wave 3a). Carry-over to Wave 3c: HRD-024 (iherald paging), HRD-033 (destructive-guard body), HRD-018..027 flavor-specific bindings, plus the rest of the §43 command catalogue. |
| Fixed | HRD-001..HRD-007, HRD-009, HRD-009b, HRD-010, HRD-011, HRD-012, HRD-013, HRD-014, HRD-016, HRD-017, HRD-028, HRD-080, HRD-092, HRD-093, HRD-094, HRD-095, HRD-096, HRD-097, HRD-098, HRD-099 |
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
| HRD-015 | task | open | low | Inheritance gate I8 invariants for Go scaffold (go.work + commons/types.go + null adapter test passes) | 2026-05-20 | 2026-05-20 | spec V3 §40 + gate I7 pattern; Catalogue-Check: no-match (2026-05-20) |
| HRD-018 | task | in_progress | high | `commons_constitution` Go package: Evaluator interface + 12 event-class emit helpers + constitution_state + constitution_bindings table migrations + bundle-hash captureer + mode-ladder runtime config | 2026-05-20 | 2026-05-20 | spec V3 §42.1 / §42.5 step 1 + Foundation design `docs/superpowers/specs/2026-05-20-foundation-design.md`; Catalogue-Check: **extend** (evidence: `docs/catalogue-checks/HRD-018-foundation.md`, 2026-05-20). 9/12 caps map to existing Helix-stack modules: River→`digital.vasic.background`; Watermill→`digital.vasic.eventbus`; raw Gin/JWT/OTel→`digital.vasic.middleware`+`auth`+`observability`; raw pgx→`digital.vasic.database`; raw redis→`digital.vasic.cache`; config→`digital.vasic.config`; recovery→`digital.vasic.recovery`. Bespoke (no-match → write new): Evaluator framework, BundleHash captureer, ModeLadder semantics. |
| HRD-019 | task | open | high | `cherald` constitution bindings — bulk implementation: ~30 `.policy.violation` rules + `.gate.failed`/`.gate.recovered`/`.credential.leak`/`.spec.revision_drift`/`.catalogue.miss` handlers; `/v1/compliance` pull surface | 2026-05-20 | 2026-05-20 | spec V3 §42.3 cherald rows + §42.5 step 2; Catalogue-Check: TBD |
| HRD-020 | task | open | high | `sherald` host-safety + repo-safety bindings (§9.1/.2/.3, §12.1/.2/.3/.6, §11.4.32, §11.4.36, §11.4.41, §11.4.71) — destructive-op detection hook, force-push interceptor, mem-budget watcher | 2026-05-20 | 2026-05-20 | spec V3 §42.3 sherald rows + §42.5 step 3; Catalogue-Check: TBD |
| HRD-021 | task | open | middle | `bherald` CI/test bindings (§1, §11.4.2/.3/.4/.5/.7/.13/.14/.24/.27/.39/.43/.46/.48–.52/.67) — gate-result event emitters; integrates with the consuming project's CI workflow | 2026-05-20 | 2026-05-20 | spec V3 §42.3 bherald rows + §42.5 step 4; Catalogue-Check: TBD |
| HRD-022 | task | open | middle | `rherald` release bindings (§4 tag mirroring, §5 changelog, §11.4.38 installable-asset evidence, §11.4.40 full-suite retest) | 2026-05-20 | 2026-05-20 | spec V3 §42.3 rherald rows + §42.5 step 5; Catalogue-Check: TBD |
| HRD-023 | task | open | middle | `pherald` project bindings (§2 commit+push, §3 submodule propagation, §11.4.11/.15/.21/.22/.34/.37/.42/.55/.66/.71/.74) | 2026-05-20 | 2026-05-20 | spec V3 §42.3 pherald rows + §42.5 step 6; Catalogue-Check: TBD |
| HRD-024 | task | open | middle | `iherald` constitution-rule escalation bindings (§11.4.10/.10.A credential-leak page-out, §11.4.21 + §11.4.66 operator-blocked escalation) | 2026-05-20 | 2026-05-20 | spec V3 §42.3 iherald rows + §42.5 step 7; Catalogue-Check: TBD |
| HRD-025 | task | open | low | `scherald` scheduled-audit bindings (§11.4.45 periodic Status.md sweep + daily/weekly/monthly compliance digest) | 2026-05-20 | 2026-05-20 | spec V3 §42.3 scherald rows + §42.5 step 8; Catalogue-Check: TBD |
| HRD-026 | task | open | middle | Constitution-bundle hash captureer — computes SHA-256 of rendered Constitution.md at evaluation time; persists on every emitted event for replayability | 2026-05-20 | 2026-05-20 | spec V3 §42.1.3; Catalogue-Check: TBD |
| HRD-027 | task | open | middle | Mode-ladder runtime config (`constitution_bindings` table + admin REST endpoints to flip allow/warn/enforce per binding per tenant without redeploy) | 2026-05-20 | 2026-05-20 | spec V3 §42.1.4; Catalogue-Check: TBD |
| HRD-029 | task | open | middle | §2 `pherald commit-push` — single-entrypoint locked commit + multi-mirror push | 2026-05-20 | 2026-05-20 | spec §43.2 / §2; Catalogue-Check: TBD (likely existing as constitution-submodule script) |
| HRD-030 | task | open | middle | §3 `pherald submodule propagate` — owned-submodule walk in propagation order | 2026-05-20 | 2026-05-20 | spec §43.2 / §3; Catalogue-Check: TBD |
| HRD-031 | task | open | middle | §4 `rherald tag mirror` — assert tag exists on every owned submodule | 2026-05-20 | 2026-05-20 | spec §43.2 / §4; Catalogue-Check: TBD |
| HRD-032 | task | open | low | §5 `rherald changelog generate` — Conventional Commits → `docs/changelogs/<v>.md` + multi-format export | 2026-05-20 | 2026-05-20 | spec §43.2 / §5 / §36; Catalogue-Check: TBD |
| HRD-033 | task | open | high | §9.1 `sherald destructive guard <op>` — wrap rm/git-reset/git-push-force with prerequisite checks | 2026-05-20 | 2026-05-20 | spec §43.2 / §9.1; Catalogue-Check: TBD |
| HRD-034 | task | open | middle | §9.3 `sherald backup snapshot <path>` — hardlinked snapshot helper | 2026-05-20 | 2026-05-20 | spec §43.2 / §9.3; Catalogue-Check: TBD |
| HRD-035 | task | open | middle | §11.4.2 `bherald evidence capture <test_id>` — captured-evidence recorder | 2026-05-20 | 2026-05-20 | spec §43.2 / §11.4.2 + §11.4.5; Catalogue-Check: TBD |
| HRD-036 | task | open | high | §11.4.10 `cherald creds scan` — gitleaks/trufflehog integration, emits `.credential.leak` | 2026-05-20 | 2026-05-20 | spec §43.2 / §11.4.10; Catalogue-Check: TBD — likely an existing OSS scanner |
| HRD-037 | task | open | low | §11.4.12 `cherald docs sync` — regen Issues_Summary / Fixed_Summary / Status_Summary | 2026-05-20 | 2026-05-20 | spec §43.2 / §11.4.12; Catalogue-Check: TBD (likely existing constitution-submodule script) |
| HRD-038 | task | open | low | §11.4.18 `cherald script-docs check` — assert sibling .md for every scripts/**/*.sh | 2026-05-20 | 2026-05-20 | spec §43.2 / §11.4.18; Catalogue-Check: TBD |
| HRD-039 | task | open | low | §11.4.19 / .23 `cherald fixed align` + `cherald colorize` — Issues/Fixed format + HTML colorizer | 2026-05-20 | 2026-05-20 | spec §43.2 / §11.4.19 + §11.4.23; Catalogue-Check: TBD |
| HRD-040 | task | open | high | §11.4.26 `sherald constitution pull` — wrap fetch + rebase + validation gate, emits `.bundle.updated` | 2026-05-20 | 2026-05-20 | spec §43.2 / §11.4.26 + §11.4.32; Catalogue-Check: TBD |
| HRD-041 | task | open | middle | §11.4.27 `bherald test-tier verify` — 8-tier matrix verification | 2026-05-20 | 2026-05-20 | spec §43.2 / §11.4.27 + §40.2; Catalogue-Check: TBD |
| HRD-042 | task | open | low | §11.4.31 `cherald submanifest verify` — Submodule-Dependency-Manifest gate | 2026-05-20 | 2026-05-20 | spec §43.2 / §11.4.31; Catalogue-Check: TBD |
| HRD-043 | task | open | high | §11.4.36 `pherald install-upstreams` — install_upstreams wrapper | 2026-05-20 | 2026-05-20 | spec §43.2 / §11.4.36; Catalogue-Check: extend constitution submodule's install_upstreams.sh |
| HRD-044 | task | open | middle | §11.4.37 `pherald fetch-guard` — pre-edit fetch + rebase enforcement | 2026-05-20 | 2026-05-20 | spec §43.2 / §11.4.37; Catalogue-Check: TBD |
| HRD-045 | task | open | high | §11.4.40 `rherald gate retest` — pre-tag full-suite retest gate | 2026-05-20 | 2026-05-20 | spec §43.2 / §11.4.40; Catalogue-Check: TBD |
| HRD-046 | task | open | high | §11.4.41 `sherald force-push gate` — merge-first + per-session-auth enforcement | 2026-05-20 | 2026-05-20 | spec §43.2 / §11.4.41 + §9.2; Catalogue-Check: TBD |
| HRD-047 | task | open | middle | §11.4.45 / .56 `scherald status digest` — periodic Status.md sweep + Status_Summary regen | 2026-05-20 | 2026-05-20 | spec §43.2 / §11.4.45 + §11.4.56; Catalogue-Check: TBD |
| HRD-048 | task | open | low | §11.4.53 `cherald fixed-summary sync` — standalone Fixed_Summary backfill | 2026-05-20 | 2026-05-20 | spec §43.2 / §11.4.53; Catalogue-Check: TBD (likely existing script) |
| HRD-049 | task | open | middle | §11.4.55 `pherald reopen <HRD-NNN>` — Issues→Fixed reversal + Reopens history | 2026-05-20 | 2026-05-20 | spec §43.2 / §11.4.55; Catalogue-Check: no-match (Herald-specific HRD flow) |
| HRD-050 | task | open | middle | §11.4.59 `cherald readme sync` — README doc-link regen + multi-format re-export | 2026-05-20 | 2026-05-20 | spec §43.2 / §11.4.57 + §11.4.59; Catalogue-Check: TBD |
| HRD-051 | task | open | high | §11.4.60 `cherald composite-gate` — canonical implementation of CM-DOCS-COMPOSITE-SYNC | 2026-05-20 | 2026-05-20 | spec §43.2 / §11.4.60; Catalogue-Check: TBD |
| HRD-052 | task | open | middle | §11.4.65 `cherald export` — bulk Markdown export wrapper (md/html/pdf/docx) | 2026-05-20 | 2026-05-20 | spec §43.2 / §11.4.65 + §36; Catalogue-Check: extend (built atop Pandoc + WeasyPrint) |
| HRD-053 | task | open | high | §11.4.71 `pherald pre-push` — fetch + investigate + integrate hook | 2026-05-20 | 2026-05-20 | spec §43.2 / §11.4.71; Catalogue-Check: TBD |
| HRD-054 | task | open | middle | §11.4.73 `cherald spec-version check` — Revision-vs-edits drift detection | 2026-05-20 | 2026-05-20 | spec §43.2 / §11.4.73; Catalogue-Check: no-match (very-new mandate) |
| HRD-055 | task | open | middle | §11.4.74 `cherald catalogue-check <pr>` — scan PR for Catalogue-Check + survey runner over vasic-digital + HelixDevelopment | 2026-05-20 | 2026-05-20 | spec §43.2 / §11.4.74; Catalogue-Check: no-match (very-new mandate) |
| HRD-056 | task | open | high | §12.6 `sherald mem-budget watch` — daemon-mode 60% threshold watcher emitting `.host.safety_breach` | 2026-05-20 | 2026-05-20 | spec §43.2 / §12.6; Catalogue-Check: TBD |
| HRD-081 | task | open | low | Extend `digital.vasic.containers/pkg/compose` to detect podman-compose vs docker compose runtime and either emit runtime-appropriate flags (no `--wait` for podman-compose) or fall back to host-side healthcheck polling. Composes with §11.4.76 — required so `compose.WithWait(true)` works across both backends. Also fix `Status()` parsing for podman-compose ps output (returns 0 services even when containers are visible to podman directly). Per §11.4.76: extend upstream submodule, never reimplement. | 2026-05-20 | 2026-05-20 | spec V3 §44 + Universal §11.4.76; Catalogue-Check: extend `vasic-digital/containers` (workaround applied in `commons_infra/boot.go` — dropped `WithWait`, TCP-probe used for healthcheck). |
| HRD-085 | task | open | middle | Implement single-task admin operations on `commons_infra.pgxTaskRepository` — `GetByID`, `Update`, `Delete`. Required for Requeue + MoveToDeadLetter call chains and operator tooling. Currently all three return `ErrUnsupported (HRD-085)`. Scope: read one BackgroundTask row by ID; full-row Update including JSONB columns; soft- or hard-Delete decision per §107. | 2026-05-20 | 2026-05-20 | spec V3 §44 + upstream interface `TaskRepository` in `submodules/background/interfaces.go`; Catalogue-Check: extend `digital.vasic.background.TaskRepository@2d46dd60b2ffcb9d3b584b029b711a6fbc71b296` (Herald already implements Enqueue/Dequeue/LogEvent on the same interface; these three rows finish the single-task surface). |
| HRD-086 | task | open | middle | Implement worker-side running-task state mutations on `commons_infra.pgxTaskRepository` — `UpdateStatus`, `UpdateProgress`, `UpdateHeartbeat`, `SaveCheckpoint`. Required by River workers + the stuck-detector reaper. Currently all four return `ErrUnsupported (HRD-086)`. Scope: status transitions guarded by valid-state-machine check; progress as `(float64, message)`; heartbeat as `updated_at = now()` writeback; checkpoint as opaque `[]byte` blob persisted alongside the task row. | 2026-05-20 | 2026-05-20 | spec V3 §44 + upstream interface `TaskRepository` in `submodules/background/interfaces.go`; Catalogue-Check: extend `digital.vasic.background.TaskRepository@2d46dd60b2ffcb9d3b584b029b711a6fbc71b296` (worker-side write surface — the hot path for long-running task progress reporting). |
| HRD-087 | task | open | low | Implement admin / stats / audit reads on `commons_infra.pgxTaskRepository` — `GetByStatus`, `GetPendingTasks`, `CountByStatus`, `GetTaskHistory`. Drives the `/v1/queue/*` introspection endpoints + Peek delegate. Currently all four return `ErrUnsupported (HRD-087)`. Scope: status-filtered paginated reads; pending-task slice for Peek; status-bucketed counts; per-task execution-history rows from the `task_execution_history` table. | 2026-05-20 | 2026-05-20 | spec V3 §44 + upstream interface `TaskRepository` in `submodules/background/interfaces.go`; Catalogue-Check: extend `digital.vasic.background.TaskRepository@2d46dd60b2ffcb9d3b584b029b711a6fbc71b296` (read-only introspection — composes with HRD-016 `/v1/queue/*` Gin handlers). |
| HRD-088 | task | open | middle | Implement queue-recovery reads on `commons_infra.pgxTaskRepository` — `GetStaleTasks`, `GetByWorkerID`. Drives the stuck-task reaper + worker-crash recovery loop. Currently both return `ErrUnsupported (HRD-088)`. Scope: stale-threshold filter against `updated_at`; per-worker assignment lookup for re-dispatch after a worker disappears. | 2026-05-20 | 2026-05-20 | spec V3 §44 + upstream interface `TaskRepository` in `submodules/background/interfaces.go`; Catalogue-Check: extend `digital.vasic.background.TaskRepository@2d46dd60b2ffcb9d3b584b029b711a6fbc71b296` (recovery-loop surface — required before any worker is allowed to crash without operator intervention). |
| HRD-089 | task | open | low | Implement resource-monitor I/O on `commons_infra.pgxTaskRepository` — `SaveResourceSnapshot`, `GetResourceSnapshots`. Persists per-task CPU/memory/IO samples for capacity-planning + post-mortem. Currently both return `ErrUnsupported (HRD-089)`. Scope: append-only insert keyed by `(task_id, captured_at)`; per-task paginated read in reverse-chronological order. Schema target: a `task_resource_snapshots` table not yet in the §9.6 migration set — adding it is part of this HRD. | 2026-05-20 | 2026-05-20 | spec V3 §44 + upstream interface `TaskRepository` in `submodules/background/interfaces.go`; Catalogue-Check: extend `digital.vasic.background.TaskRepository@2d46dd60b2ffcb9d3b584b029b711a6fbc71b296` (resource telemetry — composes with `digital.vasic.observability` OTel pipeline but is durable, not stream-only). |
| HRD-090 | task | open | middle | Implement dead-letter handling on `commons_infra.pgxTaskRepository` — `MoveToDeadLetter`. Required failure path when a task exhausts retries or fails a §107 invariant. Currently returns `ErrUnsupported (HRD-090)`. Scope: atomically move the BackgroundTask row to a `dead_letter_tasks` table with the failure-reason string + original-row JSONB snapshot; emit a `.queue.dead_letter` event via `commons_constitution` per §42.1. Schema target: a `dead_letter_tasks` table not yet in the §9.6 migration set — adding it is part of this HRD. | 2026-05-20 | 2026-05-20 | spec V3 §44 + upstream interface `TaskRepository` in `submodules/background/interfaces.go`; Catalogue-Check: extend `digital.vasic.background.TaskRepository@2d46dd60b2ffcb9d3b584b029b711a6fbc71b296` (failure-terminal surface — composes with HRD-085 GetByID + HRD-018 cherald event emission). |

## In progress

| ID | Type | Status | Criticality | Title | Opened | Last update | References |
|---|---|---|---|---|---|---|---|
| HRD-008 | task | in_progress | middle | Operator-side quickstart compose validation (Postgres + Redis + OTel + pherald container) | 2026-05-20 | 2026-05-20 | spec V3 §26.5 — scaffold shipped, live end-to-end run pending operator; Catalogue-Check: no-match (2026-05-20) — bespoke to Herald |

## Blocked

(none)

## Conventions

See [`Fixed.md`](Fixed.md) for closed items + Universal §11.4.12/.15/.16/.19/.33/.55/.73/.74 composition rules.
