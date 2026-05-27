<div align="center">

<img src="assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Herald — AGENTS.md

| Field | Value |
|---|---|
| Revision | 8 |
| Created | 2026-05-15 |
| Last modified | 2026-05-27 |
| Status | active |
| Status summary | r8: propagated HelixConstitution §11.4.89–§11.4.94 (background-test execution, Obsolete status + obsolescence audit, summary-doc clarity, multi-pass change-evaluation, SQLite workable-items SSoT, zero-idle parallel-by-default) into the agent-rule covenant cluster as short-form restatements citing the literal anchors, inherited per §11.4.35; restated + cited, not redefined. Prior r7: propagated HelixConstitution §11.4.85 (stress + chaos test mandate) + §11.4.87 (endless-loop autonomous work + zero-idle agent dispatch + anti-bluff testing) + §11.4.88 (background-push) into the agent-rule covenant cluster as short-form restatements citing the literal section numbers, inherited per §11.4.35; required by the §11.4.87 `CM-COVENANT-114-87-PROPAGATION` pre-build gate (literal anchor `11.4.87` MUST be present in every CLAUDE.md/AGENTS.md/QWEN.md; same expectation for §11.4.85/§11.4.88 per cascade pattern). Agents restate + cite, never redefine or weaken. Prior r5: added the End-user-usability covenant section restating the verbatim operator mandate at Herald agent-rule level; binds every CLI agent (Claude Code, Codex, Cursor, Gemini, Aider, subagents) to the §11.4 anti-bluff enforcement; ties to HERALD_CONSTITUTION.md §107 + inheritance-gate invariant I8b. |
| Issues | none |
| Issues summary | — |
| Fixed | R-14 (V2), V3-path-sync (V3 r3), Go-scaffold-status-update (V3 r4), §107 mandate restatement + I8b anchor (r5), Helix §11.4.85 + §11.4.87 + §11.4.88 propagation (r7), Helix §11.4.89–§11.4.94 propagation (r8) |
| Fixed summary | aligned with HRD-009/HRD-009b/HRD-013/HRD-014 landing in the same commit; r5 closes the Herald-level explicit-restatement gap identified by the 2026-05-20 audit; r7 propagates the three new inherited HelixConstitution mandates (§11.4.85 stress+chaos, §11.4.87 endless-loop autonomous work + zero-idle dispatch + anti-bluff testing, §11.4.88 background-push) into this agent-rules file per the §11.4.87 `CM-COVENANT-114-87-PROPAGATION` pre-build gate, inherited per §11.4.35; restated + cited, not redefined; r8 propagates the next six inherited HelixConstitution mandates (§11.4.89 background-test execution, §11.4.90 Obsolete status + obsolescence audit, §11.4.91 summary-doc clarity, §11.4.92 multi-pass change-evaluation, §11.4.93 SQLite workable-items SSoT, §11.4.94 zero-idle parallel-by-default) into the same agent-rule covenant cluster as short-form restatements citing the literal anchors `11.4.89`/`11.4.90`/`11.4.91`/`11.4.92`/`11.4.93`/`11.4.94`, inherited per §11.4.35; restated + cited, not redefined. |
| Continuation | bump again when first-implementation cycle completes HRD-010..HRD-012/HRD-016 live integrations. |

## Table of contents

- [Critical base rules restated (for agents that don't follow @imports)](#critical-base-rules-restated-for-agents-that-dont-follow-imports)
- [Herald-specific agent rules](#herald-specific-agent-rules)
  - [Project status (load-bearing for every task)](#project-status-load-bearing-for-every-task)
  - [End-user-usability covenant (Herald §107 / Helix §11.4 — MANDATORY ANTI-BLUFF)](#end-user-usability-covenant-herald-107--helix-114--mandatory-anti-bluff)
  - [Inherited covenant restatements — Helix §11.4.85 / §11.4.87 / §11.4.88 / §11.4.89 / §11.4.90 / §11.4.91 / §11.4.92 / §11.4.93 / §11.4.94](#inherited-covenant-restatements--helix-11485--11487--11488--11489--11490--11491--11492--11493--11494)
  - [Inheritance gate (run before any commit that touches root docs or `constitution/`)](#inheritance-gate-run-before-any-commit-that-touches-root-docs-or-constitution)
  - [Spec-change rule (load-bearing — `docs/specs/mvp/specification.V3.md` §"Specification documents")](#spec-change-rule-load-bearing-docsspecsmvpspecificationmd-specification-documents)
  - [Multi-host mirror convention (Herald's own upstreams)](#multi-host-mirror-convention-heralds-own-upstreams)
  - [Forbidden in this project](#forbidden-in-this-project)

> Base agent rules: the Helix Constitution's `AGENTS.md`, provided by the **parent project's** `constitution/` submodule (Herald does not carry its own copy). **READ IT FIRST.**
>
> Discover the constitution by walking up parent directories until you find `<ancestor>/constitution/Constitution.md`, or by invoking the canonical helper `<discovered>/find_constitution.sh`. For standalone Herald work, clone the constitution alongside Herald:
>
> ```bash
> git clone git@github.com:HelixDevelopment/HelixConstitution.git \
>     $(dirname "$PWD")/constitution
> ```
>
> The base file is authoritative for any topic not covered here. Herald-specific rules below extend them; they never weaken them.
>
> Canonical: <https://github.com/HelixDevelopment/HelixConstitution>

## Critical base rules restated (for agents that don't follow @imports)

- **No bluffing.** Every PASS carries positive evidence. Constitution §11.4 / §1.1.
- **Mutation-paired gates.** Every new gate has a paired mutation proving it catches regressions. Constitution §1.1.
- **No guessing language** (`likely`, `probably`, `maybe`, `seems`, `appears`) when reporting causes. Constitution §11.4.6.
- **Credentials never tracked.** `.env` git-ignored; runtime-load only. Constitution §11.4.10. **Operator step-by-step guide for every supported messenger + dispatcher**: `docs/guides/OPERATOR_CREDENTIALS.md` (covers Postgres / Redis / Telegram HRD-011 / Claude Code HRD-012 + reserved env-var names for planned channels + audit checklist).
- **Never force-push.** Requires explicit per-session authorization.
- **Hardlinked backup before any destructive op.** Constitution §9.
- **60% RAM cap on heavy work.** Constitution §12.6.
- **Multi-upstream push.** Every commit fans out to all 4 hosts (GitHub + GitLab + GitFlic + GitVerse). Constitution §2.1.

## Herald-specific agent rules

### Project status (load-bearing for every task)

Herald is **pre-implementation**. As of 2026-05-15 the repo contains:

- `README.md` — mission, deployment model, constitution-inheritance contract, how to run.
- `docs/specs/mvp/specification.V3.md` — MVP spec (section headings only; substantive content TBD).
- `docs/guides/HERALD_CONSTITUTION.md` — project-specific constitutional extensions.
- `docs/guides/CONSTITUTION_INHERITANCE.md` — operator/agent guide for the parent-discovery contract + the gate.
- `upstreams/` — Herald's own mirror declarations (lowercase, §11.4.29-compliant).
- `tests/test_constitution_inheritance.sh` — inheritance gate (read-only assertions).
- `tests/test_constitution_inheritance_meta.sh` — paired mutation meta-test (§1.1).
- `.gitignore` tuned for Go (also ignores `.DS_Store`).

Herald **does not** ship a `constitution/` submodule of its own — see `docs/guides/CONSTITUTION_INHERITANCE.md` for the rationale and the discovery mechanism.

**As of 2026-05-20** the Go scaffold landed (first-implementation cycle r1). 5 Go modules (`commons`, `commons_prefix`, `commons_messaging`, `commons_storage`, `pherald`) compile + unit tests pass. `pherald version --json` returns build info. The full §11.0 type contract is realized in `commons/types.go`. The `null://` sandbox adapter is fully working with 8 unit tests. SQL migrations 000001..000005 embedded via `//go:embed`. Docker/Podman Compose for §26.5 Quickstart shipped under `quickstart/` (migrated from `containers/quickstart/` when the `containers/` submodule landed). On-demand container orchestration is provided by the `containers/` submodule (`digital.vasic.containers`).

Build + test from repo root:

```
go test ./commons/... ./commons_prefix/... ./commons_messaging/... ./commons_storage/...
go build -o /tmp/pherald-dev ./pherald/cmd/pherald
```

What's NOT yet live (per `docs/Issues.md`):
- HRD-008 operator-side Quickstart compose validation.
- HRD-010 commons_storage live (pgx + River + Redis).
- HRD-011 Telegram adapter live (telebot + getUpdates).
- HRD-012 Claude Code dispatcher live (`claude --resume`).
- HRD-016 REST API per §41 (Gin Gonic).

When asked to "add a feature": find the spec section, open / claim the relevant HRD-NNN in `docs/Issues.md`, write Go + tests, ensure `go test` passes, close the HRD-NNN by migrating its row to `docs/Fixed.md` (per Universal §11.4.19 atomic-migration mandate).

Never invent build / test commands beyond `go test ./<module>/...`. Live-integration tests require operator-supplied bot tokens / Claude sessions / Postgres — `docs/CONTINUATION.md` carries the handoff prompt.

### End-user-usability covenant (Herald §107 / Helix §11.4 — MANDATORY ANTI-BLUFF)

**Forensic anchor — verbatim operator mandate:**

> "all existing tests and Challenges do work in anti-bluff manner - they MUST confirm that all tested codebase really works as expected! We had been in position that all tests do execute with success and all Challenges as well, but in reality the most of the features does not work and can't be used! This MUST NOT be the case and execution of tests and Challenges MUST guarantee the quality, the completition and full usability by end users of the product! This MUST BE part of Constitution of our project, its CLAUDE.MD and AGENTS.MD if it is not there already, and to be applied to all Submodules's Constitution, CLAUDE.MD and AGENTS.MD as well (if not there already)!"

**Agent-binding rule.** No agent — Claude Code, Codex, Cursor, Gemini, Aider, any CLI subagent — may declare a Herald feature "done", "landed", "shipped", or "tests pass" without **positive runtime evidence captured during execution** that an end user of the relevant `<flavor>herald` binary can actually use the feature.

- Unit-test green is necessary but **not sufficient**.
- Integration-test green is necessary but **not sufficient**.
- Compile success is necessary but **not sufficient**.
- A `PASS` line in a Challenge runner is a §11.4 PASS-bluff if it doesn't cross-check against runtime evidence (HTTP response body, DB row, log line, captured screenshot/audio, etc.).

Canonical Herald enforcement: `scripts/e2e_bluff_hunt.sh` — boots real services (Gin server + Postgres container via the `containers/` submodule), hits real `/v1/*` endpoints, asserts response bodies, runs M2 RLS-tenant-isolation integration tests, and SIGTERM-graceful-shutdowns. ALL 14 invariants MUST PASS before any release-tag, risky commit, or "milestone landed" claim. Canonical Helix authority: `<discovered>/Constitution.md` §11.4 + §11.4.1..§11.4.16. Canonical Herald authority: `docs/guides/HERALD_CONSTITUTION.md` §107. Inheritance-gate invariant **I8b** asserts the verbatim covenant anchor is present in this file.

Tests AND Challenges are bound equally: a Challenge that scores PASS on a broken-for-end-user feature is the same defect class as a unit test that does. Both are release blockers.

### §107.x — docs/qa/ Evidence Mandate (operator mandate, 2026-05-22; cascades from Helix §11.4.83)

**Forensic anchor — verbatim operator mandate (2026-05-22):**

> "every feature that ships MUST carry a recorded e2e communication transcript + any attached materials under `docs/qa/<run-id>/` (per-feature subdirectories). A feature with no QA transcript is itself a §107 PASS-bluff — it claims to work but has no auditable runtime evidence. Bot-driven automation (e.g. Herald's planned `qaherald` binary) MUST preserve full bidirectional communication threads as proof."

**Agent-binding rule for Herald.** No agent may declare a Herald feature done without `docs/qa/<run-id>/` evidence committed in the same logical work effort. Telegram-driven features (HRD-011, planned `qaherald`) capture both halves of the bot conversation; Gin `/v1/*` routes capture request + full response body; container-driven features (Postgres, Redis, OTel) capture container logs + healthcheck output. Bot-driven QA (the planned `qaherald` binary) MUST preserve the full conversation thread under `docs/qa/qaherald-<TS>/` — a stored-final-PASS-only mode is itself a §107 bluff.

CI / release gates MUST refuse to tag a release whose feature-shipping commits lack their matching `docs/qa/<run-id>/`. Canonical Helix authority: `<discovered>/Constitution.md` §11.4.83. Canonical Herald authority: `docs/guides/HERALD_CONSTITUTION.md` §107.x.

### §107.y — Working-Tree Quiescence Rule (operator mandate, 2026-05-22; cascades from Helix §11.4.84)

**Short tag:** `working-tree quiescence`.

**Forensic anchor — verbatim operator mandate (2026-05-22):**

> "no subagent commit may proceed while any concurrent mutation gate is in flight in the same checkout. Before `git add`, the committing agent MUST `grep` its own working tree for mutation markers (`MUTATED for paired`, `// always pass`, `return json.Marshal` shortcut paths, etc.). Any unexplained file in the staging area triggers ABORT."

**Lesson (forensic — Herald-internal).** Commit `72e81ab` (logo fix subagent, 2026-05-21) swept a `// always pass` JWT-bypass mutation residue — left mid-cycle by a paired §1.1 Wave 4b mutation gate — into the unrelated commit, which was then pushed to all four mirrors. The SECURITY FIX `d5bd360` ("restore commons_auth/middleware.go JWT verify (mutation residue in 72e81ab)") landed within the hour, but the production-equivalent-binary-with-bypassed-JWT window is a real security-defect window. The rule below is the constitutional outcome.

**Agent-binding rule for Herald.** Every commit flow (main thread + every dispatched subagent) MUST: (1) grep the working tree for mutation markers (`MUTATED for paired`, `// always pass`, `return json.Marshal` shortcut paths, `// MUTATION` / `# MUTATION` annotations, `_mutated_*` filename suffixes, `.git/MUTATION_IN_PROGRESS` lockfile) BEFORE `git add`; (2) cross-check `git status --porcelain` against the declared scope — unaccounted files ABORT; (3) refuse to operate while `.git/MUTATION_IN_PROGRESS` is present (any active mutation gate MUST complete mutate → assert FAIL → restore → assert PASS BEFORE unrelated commits proceed); (4) prefer `git worktree add` per subagent when running concurrent subagent work.

**Prototype.** `tests/test_wave4b_mutation_meta.sh` carries the canonical Herald implementation (`check_quiescence()` at line 92; assertion at line 197). Generalising to all paired-§1.1 gates is open work. The planned universal scanner is `scripts/mutation_residue_audit.sh`.

Canonical Helix authority: `<discovered>/Constitution.md` §11.4.84. Canonical Herald authority: `docs/guides/HERALD_CONSTITUTION.md` §107.y. A mutation marker that lands in a tagged Herald commit is a critical defect regardless of how briefly it persisted — see `72e81ab` / `d5bd360` as proof.

### Inherited covenant restatements — Helix §11.4.85 / §11.4.87 / §11.4.88 / §11.4.89 / §11.4.90 / §11.4.91 / §11.4.92 / §11.4.93 / §11.4.94

These nine mandates are **inherited** from the HelixConstitution via parent-discovery (§11.4.35). Every agent — Claude Code, Codex, Cursor, Gemini, Aider, any CLI subagent — is bound by them; this file **restates + cites**, it does NOT redefine or weaken. The literal anchors are required by the §11.4.87 `CM-COVENANT-114-87-PROPAGATION` pre-build gate (asserts `11.4.85` / `11.4.87` / `11.4.88` present in every CLAUDE.md / AGENTS.md / QWEN.md).

**§11.4.85 — Stress + Chaos Test Mandate (Helix, 2026-05-24).** Agent-binding rule: no agent may declare a Herald fix or improvement done without full-automation **stress** (sustained / concurrent load) AND **chaos** (process-death / network-fault / input-corruption / resource-exhaustion / state-corruption injection) test suites, each PASS citing a captured-evidence artefact under `docs/qa/<run-id>/stress_chaos/` per §11.4.5 + §11.4.69. A happy-path-only PASS is a §11.4 / §107 PASS-bluff at the resilience layer. Binds Herald's flavor binaries (`pherald listen` under concurrent updates, Gin `/v1/*` under load, claude_code dispatch under process-death, container flows under disk/OOM pressure). Canonical authority: HelixConstitution Constitution.md §11.4.85 (inherited per §11.4.35).

**§11.4.87 — Endless-loop autonomous work + zero-idle agent dispatch + anti-bluff testing (Helix, 2026-05-26).** Agent-binding rule: when instructed to "continue in endless loop fully autonomously" (or equivalent), the agent MUST continue until ALL are simultaneously TRUE — Herald's loop checks `docs/Issues.md` Status-column (zero `In progress`/`Ready for testing`/`In testing`/`Reopened` per §11.4.15), `docs/CONTINUATION.md` §3 "Active work" empty, TaskList reports no subagent mid-execution, and no in-flight push/build/sync. The agent MUST dispatch background subagents for parallelisable non-contending work rather than serialise; idle is permitted ONLY while waiting on a result. Every closed item lands four-layer coverage (§11.4.4(b)) with real captured-evidence PASS; tests AND Challenges are bound equally. The loop terminates only on all-clear, explicit operator `STOP`, a §12 host-session-safety demand, or a scheduled wake against a known-future-actionable signal. No `--idle-OK` / `--skip-endless-loop` / `--metadata-only-test-suffices` escape exists. Canonical authority: HelixConstitution Constitution.md §11.4.87 (inherited per §11.4.35).

**§11.4.88 — Background-push mandate (Helix, 2026-05-26).** Agent-binding rule: every Herald commit flow MUST release the commit-lock (`.git/.commit_all.lock`) the instant `git commit` returns 0 — BEFORE any push — then spawn the push **detached** (`nohup ./push_all.sh ... &` + `disown`) with a per-remote flock (`.git/.push.<remote>.lock`) so same-remote pushes serialise while GitHub / GitLab / GitFlic / GitVerse push in parallel. The orchestrator's exit code reports COMMIT success, not push success. Backgrounded push failures land in `qa-results/push_failures/<TS>_<remote>.log` and the next autonomous-loop tick MUST surface them (silent push-failure is a §11.4 distribution-layer PASS-bluff). The ONLY synchronous-push escape is the explicit `--sync-push` flag for §11.4.41 force-push merge-first paths. Canonical authority: HelixConstitution Constitution.md §11.4.88 (inherited per §11.4.35).

**§11.4.89 — Background test execution mandate (Helix, 2026-05-27).** Agent-binding rule: any Herald test cycle the agent expects to exceed ~30s — the `tests/test_wave*_mutation_meta.sh` gates, `scripts/e2e_bluff_hunt.sh`, future stress/chaos suites — MUST be launched detached (`nohup … > qa-results/<test_id>_<TS>.log 2>&1 &` + `disown`); the agent returns immediately to the §11.4.42 priority queue and polls the log / exit-status rather than blocking. Composes with §107.y / §11.4.84: a backgrounded mutation gate still mutates the shared tree, so the agent runs ONLY ONE against the main checkout at a time (serialised by the `.git/MUTATION_IN_PROGRESS` lockfile + `scripts/mutation_residue_audit.sh` pre-push scanner); concurrent gates require separate `git worktree` checkouts. Foreground is permitted only for <30s tests or on explicit operator request. Canonical authority: HelixConstitution Constitution.md §11.4.89 (inherited per §11.4.35).

**§11.4.90 — Obsolete status + per-item obsolescence audit (Helix, 2026-05-27).** Agent-binding rule: Herald's HRD Status closed-set gains a 4th terminal value `Obsolete (→ Fixed.md)` for items no longer valid (Reason closed-set: superseded-by-design-change / superseded-by-later-mandate / feature-removed / duplicate-of / unsupported-topology). The agent MUST add an `**Obsolete-Details:**` line within 8 non-blank lines of every `Obsolete` HRD heading: Since (ISO date), Reason, Superseding-item (§/HRD ref), Triple-check evidence (git-log / grep / runtime path per §11.4.6 — bare assertion forbidden). At every release-gate sweep the agent re-evaluates every open + Fixed HRD for obsolescence; migrations are atomic per §11.4.19. Canonical authority: HelixConstitution Constitution.md §11.4.90 (inherited per §11.4.35).

**§11.4.91 — Summary-doc clarity (Helix, 2026-05-27).** Agent-binding rule: every one-liner the agent writes in `docs/Issues_Summary.md` / `docs/Fixed_Summary.md` / `docs/Status_Summary.md` / README doc-link rows MUST be a self-contained clause (≥6 words OR ≥40 chars) naming the SUBJECT + the PROBLEM/GOAL — never a section-label fragment (`Composes with`, `Closure criteria`), bare metadata (`Critical`, `Bug`), a status restatement, or a bare HRD-id. Each is derived from the source long-form H1/H2 heading, never invented. Canonical authority: HelixConstitution Constitution.md §11.4.91 (inherited per §11.4.35).

**§11.4.92 — Multi-pass change-evaluation discipline (Helix, 2026-05-27).** Agent-binding rule: every non-trivial Herald change MUST pass — and the agent MUST document — a 5-pass evaluation before commit-ready: Pass 1 main-task captured-evidence (§11.4.5, no "should work"); Pass 2 regression-blast-radius (every touched file + every importer/caller audited); Pass 3 cross-feature interaction (shared state / timing / env — e.g. a gate edit checks §107.y quiescence + §11.4.89 backgrounding); Pass 4 deep-research validation (§11.4.8 — external precedent or literal "NO external solution found"); Pass 5 anti-bluff confirmation (no metadata-only / config-only / script-bug PASS). Evidence lands in the commit footer or `docs/qa/` / `qa-results/`. Trivial changes (typo, revision-bump, MD-export regen touching zero source) are exempt only with explicit commit-message citation. Canonical authority: HelixConstitution Constitution.md §11.4.92 (inherited per §11.4.35).

**§11.4.93 — SQLite-backed single-source-of-truth for workable items (Helix, 2026-05-27).** Agent-binding rule: Herald's text-based HRD trackers (`docs/Issues.md` / `Fixed.md` / `*_Summary.md` / `Status.md` / `CONTINUATION.md` §3) migrate to a SQLite single-source-of-truth (`docs/.workable_items.db`, **version-controlled as Herald's authoritative SSoT** per operator mandate 2026-05-27 — a deliberate Herald divergence from the parent §11.4.93 gitignored-with-regeneration default, since for Herald the DB IS the authoritative artefact and MUST be committed, not regenerated-on-clone; only the transient WAL/SHM sidecars stay ignored) with bidirectional MD↔DB regeneration so sync-drift is mechanically impossible. The Go binary lives in the constitution submodule (`constitution/scripts/workable-items/`) — the agent references it per §11.4.74 catalogue-first, never reimplements. Migration is 6-phase; the agent files the tracking HRD (Phase 1) and progresses incrementally. Canonical authority: HelixConstitution Constitution.md §11.4.93 (inherited per §11.4.35).

**§11.4.94 — Zero-idle priority-first parallel-by-default operating mode (Helix, 2026-05-27).** Agent-binding rule (binding always-on contract): Herald work is NEVER idle while a priority-queued item can progress. Before any wake / sleep / "waiting for X", the agent surveys the priority queue, identifies all non-contending items, and dispatches them in parallel (subagent-driven per §11.4.20 / §11.4.70 when non-trivial; background per §11.4.89 when >30s), picking highest-Severity/priority first. The conductor remains the integration + commit + push seam; parallel work MUST NOT compromise stability (composes with §107.y quiescence + §11.4.92 multi-pass + §12 host-safety). Idle is permitted ONLY when every item is genuinely blocked on external dependency, the operator issued STOP, or §12 host-session-safety demands it. Canonical authority: HelixConstitution Constitution.md §11.4.94 (inherited per §11.4.35).

### Inheritance gate (run before any commit that touches root docs or `constitution/`)

```bash
bash tests/test_constitution_inheritance.sh        # the gate
bash tests/test_constitution_inheritance_meta.sh   # paired mutation proof (§1.1)
```

Both MUST return 0. If either fails, fix at root cause per Constitution §11.4.4 — never silently accept the FAIL.

### Spec-change rule (load-bearing — `docs/specs/mvp/specification.V3.md` §"Specification documents")

Whenever `docs/specs/mvp/specification.V3.md` or any file under `docs/specs/` (any depth) is modified, **comprehensive planning and implementation of all changes is MANDATORY** — agents may not edit the spec in isolation. This rule does NOT apply to creating or renaming files; for those, ask the operator what to do with the new path. Treat every spec edit as a project-wide ripple, not a doc tweak.

This rule is mirrored in `CLAUDE.md` and in `docs/guides/HERALD_CONSTITUTION.md` §106. The inheritance gate's invariant **I7a–c** asserts the rule anchor (`comprehensive planning and implementation`) is present in all three files; a missing copy is a §1.1 propagation bluff and the gate FAILs.

### Multi-host mirror convention (Herald's own upstreams)

`upstreams/` contains one script per mirror host (GitHub, GitLab, GitFlic, GitVerse). Each script exports `UPSTREAMABLE_REPOSITORY` and is sourced, not executed. The Herald repo's `origin` remote is already fan-out (1 fetch URL + 4 push URLs) — a single `git push origin <branch>` propagates to all four hosts. Per-host naming intentionally matches each provider's brand capitalization; do not normalize.

### Forbidden in this project

- **Re-adding a `constitution/` submodule inside Herald.** Herald is consumed as a submodule of a parent project that already provides `constitution/`. A duplicate copy inside Herald is a deployment-model violation. The inheritance gate's invariant `I6` enforces this — re-adding it will turn the gate red.
- Promoting Herald-specific values into the parent constitution (universal status must be EARNED; see Constitution §11.4 + §11.4.10).
- Modifying any file under the discovered `constitution/` from Herald commits — the parent's constitution is read-only from Herald's perspective. Constitution changes go through the HelixConstitution repo directly.
- Adding new submodules without re-running `bash tests/test_constitution_inheritance.sh` afterward.
