<div align="center">

![Herald](assets/logo/herald_logo_square_128.png){width=96px height=96px}

</div>

# Herald — AGENTS.md

| Field | Value |
|---|---|
| Revision | 6 |
| Created | 2026-05-15 |
| Last modified | 2026-05-20 |
| Status | active |
| Status summary | r5: added the End-user-usability covenant section restating the verbatim operator mandate at Herald agent-rule level; binds every CLI agent (Claude Code, Codex, Cursor, Gemini, Aider, subagents) to the §11.4 anti-bluff enforcement; ties to HERALD_CONSTITUTION.md §107 + inheritance-gate invariant I8b. |
| Issues | none |
| Issues summary | — |
| Fixed | R-14 (V2), V3-path-sync (V3 r3), Go-scaffold-status-update (V3 r4), §107 mandate restatement + I8b anchor (r5) |
| Fixed summary | aligned with HRD-009/HRD-009b/HRD-013/HRD-014 landing in the same commit; r5 closes the Herald-level explicit-restatement gap identified by the 2026-05-20 audit. |
| Continuation | bump again when first-implementation cycle completes HRD-010..HRD-012/HRD-016 live integrations. |

## Table of contents

- [Critical base rules restated (for agents that don't follow @imports)](#critical-base-rules-restated-for-agents-that-dont-follow-imports)
- [Herald-specific agent rules](#herald-specific-agent-rules)
  - [Project status (load-bearing for every task)](#project-status-load-bearing-for-every-task)
  - [End-user-usability covenant (Herald §107 / Helix §11.4 — MANDATORY ANTI-BLUFF)](#end-user-usability-covenant-herald-107--helix-114--mandatory-anti-bluff)
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
