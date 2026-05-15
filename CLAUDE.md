# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## INHERITED FROM constitution/CLAUDE.md

All rules in `constitution/CLAUDE.md` (and the `constitution/Constitution.md` it references) apply unconditionally. Herald-specific rules below extend them — they MUST NOT weaken any inherited rule. When this file disagrees with the constitution submodule, the constitution wins.

The HelixConstitution submodule is pinned at `1b0b03a2223fffcd71eb7fc8a9620d0c7b4ed8f3` (incorporated 2026-05-15). Re-read it from the submodule directly — do not paraphrase it here.

@constitution/CLAUDE.md

> **Read order on a cold start:**
> 1. `constitution/CLAUDE.md` + `constitution/Constitution.md` — universal Helix rules. Inherited unconditionally.
> 2. `constitution/AGENTS.md` — agent guardrails (anti-bluff, no-guessing, paired mutations).
> 3. This file (Herald-specific notes below).
> 4. `docs/guides/HERALD_CONSTITUTION.md` — Herald's project-specific constitutional extensions (currently mostly TBD, matches the spec).
> 5. `docs/specs/mvp/specification.md` — mission spec (mostly TBD).

## Project status

Herald is **pre-implementation**. As of this writing the repo contains only:

- `README.md` — one-line mission statement.
- `docs/specs/mvp/specification.md` — section headings only; every substantive section (capabilities, design, Constitution integration, notes) is still `Tbd`.
- `upstreams/` — mirror configuration scripts (see below).
- `.gitignore` tuned for Go.

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

## Notes for future scaffolding

- The repo is in `main` branch and committed under "Milos Vasic" — no other contributors yet.
- `.claude/` exists but is empty; project-local Claude config can go there.
- `LICENSE` is present (do not overwrite without asking).
- `docs/` and `docs/specs/` contain stray `.DS_Store` files; don't commit more of them, and consider adding `.DS_Store` to `.gitignore` if the user wants.
