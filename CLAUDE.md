# CLAUDE.md

| Field | Value |
|---|---|
| Revision | 3 |
| Created | 2026-05-15 |
| Last modified | 2026-05-20 |
| Status | active |
| Status summary | "Project status" section updated to reflect that the Go scaffold (5 modules, passing unit tests, Quickstart compose) has landed. New commands documented (go build/test). The pre-implementation language replaced with implementation-r1 reality. |
| Issues | none |
| Issues summary | — |
| Fixed | spec-path references (r2), pre-implementation-language update (r3) |
| Fixed summary | aligned with HRD-009/HRD-009b/HRD-013/HRD-014 landing in the same commit. |
| Continuation | bump again when first-implementation cycle completes HRD-010..HRD-012/HRD-016 live integrations. |

## Table of contents

- [INHERITED FROM Helix Constitution (parent-discovery)](#inherited-from-helix-constitution-parent-discovery)
- [Project status](#project-status)
- [Mission (from the spec)](#mission-from-the-spec)
- [Intended stack](#intended-stack)
- [Multi-host mirror convention](#multi-host-mirror-convention)
- [Inheritance gate (run before any commit touching root docs)](#inheritance-gate-run-before-any-commit-touching-root-docs)
- [Spec-change rule (load-bearing — `docs/specs/mvp/specification.V3.md` §"Specification documents")](#spec-change-rule-load-bearing-docsspecsmvpspecificationmd-specification-documents)
- [Project conventions from the spec (apply when scaffolding)](#project-conventions-from-the-spec-apply-when-scaffolding)
- [`constitutable/` directory (parent-project extension hook)](#constitutable-directory-parent-project-extension-hook)
- [Documentation artefacts (PDF/HTML siblings)](#documentation-artefacts-pdfhtml-siblings)
- [Notes for future scaffolding](#notes-for-future-scaffolding)

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
> 7. `docs/specs/mvp/specification.V3.md` — mission spec (mostly TBD).

## Project status

Herald is **pre-implementation**. As of this writing the repo contains:

- `README.md` — mission, deployment model, inheritance contract, quickstart.
- `docs/specs/mvp/specification.V3.md` — MVP spec stub (substantive sections TBD).
- `docs/guides/HERALD_CONSTITUTION.md` — Herald's project constitution (extends Helix).
- `docs/guides/CONSTITUTION_INHERITANCE.md` — operator/agent guide for parent-discovery + gate semantics.
- `upstreams/` — Herald's mirror declarations (see below).
- `tests/test_constitution_inheritance.sh` + `_meta.sh` — paired inheritance gate (§1.1).
- `.gitignore` tuned for Go + `.DS_Store`.

Herald does **not** ship a `constitution/` submodule of its own; the parent project provides it. See `docs/guides/CONSTITUTION_INHERITANCE.md`.

**As of 2026-05-20** the Go scaffold has landed (first-implementation cycle r1). The repo now contains:

- `go.work` (gitignored locally; check in if the project wants reproducible workspaces — current convention: gitignored per spec §9.1).
- `commons/` (L0) — `commons/types.go` with the full §11.0 Channel contract + Subscriber/CloudEventEnvelope/TraceContext/Branding/ChannelID; `commons/clock.go` Clock abstraction; `commons/uuidv7.go`; `commons/branding.go` per-flavor factory.
- `commons_prefix/` — §8.2 3-letter prefix algorithm.
- `commons_messaging/channels/null/` — full §11.14 `null://` sandbox adapter (working, tested).
- `commons_messaging/channels/tgram/` — Telegram adapter SCAFFOLD (HRD-011 open).
- `commons_messaging/dispatch/claude_code/` — Claude Code session-resolution + envelope formatter; live `claude --resume` invocation pending (HRD-012 open).
- `commons_storage/` — 9 SQL migrations (000001..000005) embedded via `//go:embed`; pgx + River + Redis wiring pending (HRD-010 open).
- `pherald/cmd/pherald/` — Cobra CLI; `pherald version` works end-to-end; other subcommands stubbed with HRD-NNN error pointers.
- `quickstart/` — Herald-specific Docker/Podman Compose + Dockerfile + otel-config + `.env.example` per §26.5 (HRD-008 open for operator validation). Migrated from `containers/quickstart/` 2026-05-20 when the `containers/` submodule was added.
- `containers/` — git submodule pointing at `git@github.com:vasic-digital/containers.git` (the `digital.vasic.containers` Go module — runtime auto-detection + on-demand container boot + lifecycle/health). Imported by Foundation tests + the `pherald doctor` subcommand to bring Postgres + Redis + OTel up on-demand.

**Build + test:** from the repo root:

```bash
go build ./commons/... ./commons_prefix/... ./commons_messaging/... ./commons_storage/... ./pherald/...
go test  ./commons/... ./commons_prefix/... ./commons_messaging/... ./commons_storage/...
```

Tests pass on Go 1.22+ (verified on 1.26). Workspace is configured via `go.work` listing all 5 modules.

When the user asks to "add a feature" the spec is the source of truth — find the relevant §, then the relevant module + package, then the relevant HRD-NNN if one is already open. New work opens a new HRD-NNN in `docs/Issues.md` per V3 §8.3 lifecycle.

Do not invent build/test commands beyond what `go test ./<module>/...` provides. Live-integration tests (Telegram bot, Claude Code session, real Postgres) require operator-supplied credentials — see `docs/CONTINUATION.md` for the live-test handoff prompt.

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
bash tests/test_constitution_inheritance.sh        # 6 invariants (I1–I6), 9 checks
bash tests/test_constitution_inheritance_meta.sh   # paired §1.1 mutation proof
```

The gate inline-walks parents for `<ancestor>/constitution/Constitution.md`. I5 is split into I5a–d (one check per root doc that must declare parent-discovery: `CLAUDE.md`, `AGENTS.md`, `docs/guides/HERALD_CONSTITUTION.md`, `README.md`). I6 forbids re-introducing `<repo-root>/constitution/` or `.gitmodules` — the §104 invariant.

If either script fails, fix at root cause per Universal §11.4.4. Never silently accept the FAIL.

## Spec-change rule (load-bearing — `docs/specs/mvp/specification.V3.md` §"Specification documents")

Any modification to a file under `docs/specs/` (any depth) triggers **mandatory comprehensive planning and implementation** of the implied changes — you may not edit the spec in isolation. This rule does **not** apply to creating or renaming files; for those, ask the operator what to do with the new path. Treat every spec edit as a project-wide ripple, not a doc tweak.

## Project conventions from the spec (apply when scaffolding)

These are declared in `docs/specs/mvp/specification.V3.md` and are easy to miss because no code enforces them yet:

- **Workable-item prefix:** `HRD-` (e.g. `HRD-001`). Use it for issues, status entries, fix logs.
- **Flavor binaries:** each Herald flavor ships as its own CLI binary, named `<prefix>herald` — `pherald` (Project Herald), `sherald` (System Herald), etc. Designed for CI / pipeline / cron / AI-agent invocation.
- **Layered shared code:** `commons` → `commons_messaging` (level 1) → … → flavor. Put new shared code in the **lowest layer that still makes sense**; flavors inherit upward. `commons_messaging` owns the Telegram / Max / Slack / Email / Markdown-export integrations.
- **Messenger integration priority:** Telegram → Max → Slack (then Email, then Markdown/PDF/HTML export). Microsoft Teams, Lark, Discord, WhatsApp, Viber are explicitly later iterations — don't pre-implement.
- **Conversation diary:** every message in/out is appended to `docs/herald/diary/main.md` and re-exported to `main.pdf` + `main.html` in sync. Don't break this invariant when designing channel I/O.
- **Container stack:** Postgres (main DB) + Redis (in-memory) bundled via the `containers` submodule (`https://github.com/vasic-digital/containers`). All container names start with `herald`; all host ports start with `70XXX` (70001, 70002, …) to avoid collisions.
- **Credentials:** `.env` (git-ignored) with a committed `.env.example`. Resolution order: exported shell vars from `.bashrc`/`.zshrc` load first, then `.env` overrides them on key collision.
- **Vendored SDKs:** any official/unofficial messenger SDK or API client we depend on goes in as a **git submodule**, e.g. `commons_messaging/sdk/telegram` or `commons_messaging/api/telegram` — not `go get`'d into `go.mod`.

## `constitutable/` directory (parent-project extension hook)

The empty `constitutable/` directory at the repo root is intentional. Per the spec, a parent project may drop additional `Constitution.md` / `CLAUDE.md` / `AGENTS.md` (in `constitutable/`, `constitutable/<flavor>/`, `constitutable/<flavor>/<variant>/`, etc.) to layer extensions or overrides on top of the discovered `constitution/` submodule. Apply-order: `constitution/` submodule → `constitutable/` extensions → Herald's own docs. Do not delete the directory because it's empty.

## Documentation artefacts (PDF/HTML siblings)

`docs/guides/HERALD_CONSTITUTION.md` and `docs/guides/CONSTITUTION_INHERITANCE.md` each ship with a committed `.pdf` sibling. When you edit one of these Markdown files, the PDF goes stale — flag it; do not regenerate silently unless the operator asks.

## Notes for future scaffolding

- The repo is in `main` branch and committed under "Milos Vasic" — no other contributors yet.
- `.claude/` exists but is empty; project-local Claude config can go there.
- `LICENSE` is present (do not overwrite without asking).
- `.DS_Store` is now git-ignored; do not re-add the previously-stray files.
