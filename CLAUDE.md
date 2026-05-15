# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## INHERITED FROM Helix Constitution (parent-discovery)

Herald is consumed as a submodule of a parent project that already carries the Helix Constitution submodule at `<parent>/constitution/`. Herald therefore does **NOT** keep its own copy. Locate the constitution from any nested depth by walking up parents until you find `<ancestor>/constitution/Constitution.md`, or by running the canonical helper:

```bash
CONST_DIR="$(bash "$(find . -type d -name constitution -print -quit 2>/dev/null)/find_constitution.sh")"
# or, more robustly, from any starting directory:
CONST_DIR="$(bash <ancestor>/constitution/find_constitution.sh)"
```

For standalone development of Herald (no parent project), clone the constitution alongside Herald:

```bash
git clone git@github.com:HelixDevelopment/HelixConstitution.git \
    $(dirname "$PWD")/constitution
```

Once located, all rules in `<discovered>/CLAUDE.md` and the `<discovered>/Constitution.md` it references apply unconditionally. Herald-specific rules below extend them — they MUST NOT weaken any inherited rule. When this file disagrees with the discovered constitution, the constitution wins.

Canonical: <https://github.com/HelixDevelopment/HelixConstitution>

> **Read order on a cold start:**
> 1. `<discovered-constitution>/CLAUDE.md` + `Constitution.md` — universal Helix rules. Inherited unconditionally.
> 2. `<discovered-constitution>/AGENTS.md` — agent guardrails (anti-bluff, no-guessing, paired mutations).
> 3. `README.md` — Herald overview + how-to.
> 4. This file (Herald-specific notes below).
> 5. `docs/guides/HERALD_CONSTITUTION.md` — Herald's project-specific constitutional extensions.
> 6. `docs/guides/CONSTITUTION_INHERITANCE.md` — operator/agent guide for the discovery contract + the inheritance gate.
> 7. `docs/specs/mvp/specification.md` — mission spec (mostly TBD).

## Project status

Herald is **pre-implementation**. As of this writing the repo contains:

- `README.md` — mission, deployment model, inheritance contract, quickstart.
- `docs/specs/mvp/specification.md` — MVP spec stub (substantive sections TBD).
- `docs/guides/HERALD_CONSTITUTION.md` — Herald's project constitution (extends Helix).
- `docs/guides/CONSTITUTION_INHERITANCE.md` — operator/agent guide for parent-discovery + gate semantics.
- `upstreams/` — Herald's mirror declarations (see below).
- `tests/test_constitution_inheritance.sh` + `_meta.sh` — paired inheritance gate (§1.1).
- `.gitignore` tuned for Go + `.DS_Store`.

Herald does **not** ship a `constitution/` submodule of its own; the parent project provides it. See `docs/guides/CONSTITUTION_INHERITANCE.md`.

There is no `go.mod`, no source code, and no build/test/lint tooling yet. When the user asks you to "add a feature" or "fix" something, first confirm whether they want you to **scaffold the project** (init the module, lay out packages) or **fill in the spec** — the answer determines whether you're writing Go or Markdown.

Do not invent build/test commands or pretend infrastructure exists. If a command is needed but the supporting file (`go.mod`, `Makefile`, CI config) isn't present, say so and ask before scaffolding it.

## Mission (from the spec)

> Ingesting system events and reliably fanning them out to multiple notification channels so every alert reaches the right destination without confusion.

The spec mentions "Integration into the Constitution" as a planned section — Herald is intended to plug into a larger governance/policy system that lives outside this repo. Treat that as a real constraint when designing interfaces, even though the details are TBD.

## Intended stack

Go, inferred from `.gitignore` (`*.test`, `go.work`, `go.work.sum`, `coverage.*`, `profile.cov`). When you scaffold, default to standard `cmd/` + `internal/` layout unless the user asks otherwise, and use `go test ./...` / `go build ./...` / `go vet ./...` — there is no project-specific tooling to override these yet.

## Multi-host mirror convention

`upstreams/` contains one script per mirror host:

- `GitHub.sh` — primary: `git@github.com:vasic-digital/Herald.git`
- `GitLab.sh` — `git@gitlab.com:vasic-digital/herald.git`
- `GitFlic.sh` — `git@gitflic.ru:vasic-digital/herald.git`
- `GitVerse.sh` — `git@gitverse.ru:vasic-digital/Herald.git`

Each script exports a single `UPSTREAMABLE_REPOSITORY` variable and is meant to be **sourced** (`. upstreams/GitHub.sh`), not executed for its output. The naming is intentional — capitalization matches the host's brand (GitFlic, GitVerse). Don't normalize these to lowercase or collapse them into one file; the per-host split is the interface other tooling (likely external mirror-push scripts) keys on.

When adding a new mirror, copy an existing script and change only the URL — keep the `#!/bin/bash` shebang, blank line, and `export` form identical so any consumer that greps these files keeps working.

## Inheritance gate (run before any commit touching root docs)

```bash
bash tests/test_constitution_inheritance.sh        # 9 invariants, must pass
bash tests/test_constitution_inheritance_meta.sh   # paired §1.1 mutation proof
```

If either fails, fix at root cause per Universal §11.4.4. Never silently accept the FAIL.

## Notes for future scaffolding

- The repo is in `main` branch and committed under "Milos Vasic" — no other contributors yet.
- `.claude/` exists but is empty; project-local Claude config can go there.
- `LICENSE` is present (do not overwrite without asking).
- `.DS_Store` is now git-ignored; do not re-add the previously-stray files.
