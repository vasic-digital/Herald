# Herald — AGENTS.md

| Field | Value |
|---|---|
| Revision | 2 |
| Created | 2026-05-15 |
| Last modified | 2026-05-19 |
| Status | active |
| Status summary | added Spec-change rule per R-14 + Constitution §11.4.61 |
| Issues | none |
| Issues summary | — |
| Fixed | R-14 partial |
| Fixed summary | spec-change rule propagated from CLAUDE.md to AGENTS.md |
| Continuation | I7 gate invariant pending in `tests/test_constitution_inheritance.sh` |

## Table of contents

- [Critical base rules restated (for agents that don't follow @imports)](#critical-base-rules-restated-for-agents-that-dont-follow-imports)
- [Herald-specific agent rules](#herald-specific-agent-rules)
  - [Project status (load-bearing for every task)](#project-status-load-bearing-for-every-task)
  - [Inheritance gate (run before any commit that touches root docs or `constitution/`)](#inheritance-gate-run-before-any-commit-that-touches-root-docs-or-constitution)
  - [Spec-change rule (load-bearing — `docs/specs/mvp/specification.md` §"Specification documents")](#spec-change-rule-load-bearing-docsspecsmvpspecificationmd-specification-documents)
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
- **Credentials never tracked.** `.env` git-ignored; runtime-load only. Constitution §11.4.10.
- **Never force-push.** Requires explicit per-session authorization.
- **Hardlinked backup before any destructive op.** Constitution §9.
- **60% RAM cap on heavy work.** Constitution §12.6.
- **Multi-upstream push.** Every commit fans out to all 4 hosts (GitHub + GitLab + GitFlic + GitVerse). Constitution §2.1.

## Herald-specific agent rules

### Project status (load-bearing for every task)

Herald is **pre-implementation**. As of 2026-05-15 the repo contains:

- `README.md` — mission, deployment model, constitution-inheritance contract, how to run.
- `docs/specs/mvp/specification.md` — MVP spec (section headings only; substantive content TBD).
- `docs/guides/HERALD_CONSTITUTION.md` — project-specific constitutional extensions.
- `docs/guides/CONSTITUTION_INHERITANCE.md` — operator/agent guide for the parent-discovery contract + the gate.
- `upstreams/` — Herald's own mirror declarations (lowercase, §11.4.29-compliant).
- `tests/test_constitution_inheritance.sh` — inheritance gate (read-only assertions).
- `tests/test_constitution_inheritance_meta.sh` — paired mutation meta-test (§1.1).
- `.gitignore` tuned for Go (also ignores `.DS_Store`).

Herald **does not** ship a `constitution/` submodule of its own — see `docs/guides/CONSTITUTION_INHERITANCE.md` for the rationale and the discovery mechanism.

There is no `go.mod`, no source code, and no build/lint tooling yet. When asked to "add a feature" or "fix" something, first disambiguate: **scaffold the project** (init the Go module, lay out packages) vs. **fill in the spec**? The answer determines whether you're writing Go or Markdown.

Never invent build/test commands. If a command is needed but the supporting file (`go.mod`, `Makefile`, CI config) isn't present, say so and confirm before scaffolding.

### Inheritance gate (run before any commit that touches root docs or `constitution/`)

```bash
bash tests/test_constitution_inheritance.sh        # the gate
bash tests/test_constitution_inheritance_meta.sh   # paired mutation proof (§1.1)
```

Both MUST return 0. If either fails, fix at root cause per Constitution §11.4.4 — never silently accept the FAIL.

### Spec-change rule (load-bearing — `docs/specs/mvp/specification.md` §"Specification documents")

Whenever `docs/specs/mvp/specification.md` or any file under `docs/specs/` (any depth) is modified, **comprehensive planning and implementation of all changes is MANDATORY** — agents may not edit the spec in isolation. This rule does NOT apply to creating or renaming files; for those, ask the operator what to do with the new path. Treat every spec edit as a project-wide ripple, not a doc tweak.

This rule is mirrored in `CLAUDE.md` and in `docs/guides/HERALD_CONSTITUTION.md` §106. The inheritance gate's invariant **I7a–c** asserts the rule anchor (`comprehensive planning and implementation`) is present in all three files; a missing copy is a §1.1 propagation bluff and the gate FAILs.

### Multi-host mirror convention (Herald's own upstreams)

`upstreams/` contains one script per mirror host (GitHub, GitLab, GitFlic, GitVerse). Each script exports `UPSTREAMABLE_REPOSITORY` and is sourced, not executed. The Herald repo's `origin` remote is already fan-out (1 fetch URL + 4 push URLs) — a single `git push origin <branch>` propagates to all four hosts. Per-host naming intentionally matches each provider's brand capitalization; do not normalize.

### Forbidden in this project

- **Re-adding a `constitution/` submodule inside Herald.** Herald is consumed as a submodule of a parent project that already provides `constitution/`. A duplicate copy inside Herald is a deployment-model violation. The inheritance gate's invariant `I6` enforces this — re-adding it will turn the gate red.
- Promoting Herald-specific values into the parent constitution (universal status must be EARNED; see Constitution §11.4 + §11.4.10).
- Modifying any file under the discovered `constitution/` from Herald commits — the parent's constitution is read-only from Herald's perspective. Constitution changes go through the HelixConstitution repo directly.
- Adding new submodules without re-running `bash tests/test_constitution_inheritance.sh` afterward.
