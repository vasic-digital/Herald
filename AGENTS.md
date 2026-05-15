# Herald — AGENTS.md

> Base agent rules: `constitution/AGENTS.md` — **READ IT FIRST.**
> The base file is authoritative for any topic not covered here.
> Herald-specific rules below extend them; they never weaken them.
>
> The constitution submodule is pinned at `1b0b03a2223fffcd71eb7fc8a9620d0c7b4ed8f3`
> (incorporated 2026-05-15). Canonical: https://github.com/HelixDevelopment/HelixConstitution

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

- `README.md` — one-line mission statement.
- `docs/specs/mvp/specification.md` — section headings only (substantive content TBD).
- `docs/guides/HERALD_CONSTITUTION.md` — project-specific constitutional extensions (TBD-heavy).
- `upstreams/` — Herald's own mirror declarations (lowercase, §11.4.29-compliant).
- `constitution/` — Helix Constitution submodule (READ-ONLY from Herald's side).
- `tests/test_constitution_inheritance.sh` — inheritance gate.
- `tests/test_constitution_inheritance_meta.sh` — paired mutation meta-test.
- `.gitignore` tuned for Go.

There is no `go.mod`, no source code, and no build/lint tooling yet. When asked to "add a feature" or "fix" something, first disambiguate: **scaffold the project** (init the Go module, lay out packages) vs. **fill in the spec**? The answer determines whether you're writing Go or Markdown.

Never invent build/test commands. If a command is needed but the supporting file (`go.mod`, `Makefile`, CI config) isn't present, say so and confirm before scaffolding.

### Inheritance gate (run before any commit that touches root docs or `constitution/`)

```bash
bash tests/test_constitution_inheritance.sh        # the gate
bash tests/test_constitution_inheritance_meta.sh   # paired mutation proof (§1.1)
```

Both MUST return 0. If either fails, fix at root cause per Constitution §11.4.4 — never silently accept the FAIL.

### Multi-host mirror convention (Herald's own upstreams)

`upstreams/` contains one script per mirror host (GitHub, GitLab, GitFlic, GitVerse). Each script exports `UPSTREAMABLE_REPOSITORY` and is sourced, not executed. The Herald repo's `origin` remote is already fan-out (1 fetch URL + 4 push URLs) — a single `git push origin <branch>` propagates to all four hosts. Per-host naming intentionally matches each provider's brand capitalization; do not normalize.

### Forbidden in this project

- Promoting Herald-specific values into `constitution/` (universal status must be EARNED; see Constitution §11.4 + §11.4.10).
- Modifying any file under `constitution/` from Herald commits — the submodule is read-only for Herald. Constitution changes go through the HelixConstitution repo directly.
- Adding new submodules without re-running the inheritance audit afterward.
