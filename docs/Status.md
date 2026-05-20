# Herald — Status

| Field | Value |
|---|---|
| Revision | 1 |
| Created | 2026-05-20 |
| Last modified | 2026-05-20 |
| Status | active |
| Status summary | Pre-implementation. Spec complete at V3 r3. Tracking docs (Issues / Fixed / Status / CONTINUATION) established this commit. Next milestone: first-implementation cycle. |
| Issues | HRD-007 (closing this commit) |
| Issues summary | see `Issues.md` |
| Fixed | HRD-001..HRD-006 |
| Fixed summary | see `Fixed.md` |
| Continuation | scaffold `commons` + `commons_messaging` + `pherald` shim against §11.0 type contract; verify §26.5 quickstart compose works end-to-end. |

## Table of contents

- [Project phase](#project-phase)
- [Specification](#specification)
- [Implementation](#implementation)
- [Operations](#operations)
- [Risk surface](#risk-surface)

## Project phase

**Pre-implementation.** The spec set (V1 → V2 → V3) is complete and stable; no Go source code, `go.mod`, build pipeline, or container compose stack exists yet. The next major milestone is the first-implementation cycle.

## Specification

- **Active spec:** `docs/specs/mvp/specification.V3.md` (Revision 3, ~3900 lines).
- **Archived specs:** V1 + V2 in `docs/specs/mvp/archive/` for traceability.
- **Spec-change rule:** per V3 §23 + HERALD_CONSTITUTION §106 + CLAUDE.md + AGENTS.md — every edit under `docs/specs/` triggers comprehensive planning (gate I7a/b/c).
- **Outstanding spec work:** none for V3. V4 would only be triggered by post-implementation feedback (see §30.7.3 plus V3 r2 metadata Continuation).

## Implementation

- **`commons`** (L0) — not started.
- **`commons_messaging`** (L1) — not started.
- **`commons_storage`** (L1) — not started.
- **`commons_security`** (L1) — not started.
- **`commons_observability`** (L1) — not started.
- **Flavor binaries** (`pherald`, `sherald`, `bherald`, `dherald`, `aherald`, `scherald`, `iherald`, `rherald`, `cherald`) — not started.
- **Channel adapters** — none yet; spec mandates Telegram + Slack + Max + Email + Diary for V1-tier, with eight more channels documented for V2-tier (§11).

## Operations

- **Repo hygiene:** main is the only branch; four-mirror fan-out via `origin` (GitHub + GitLab + GitFlic + GitVerse) is configured and proven.
- **Constitution inheritance gate:** 12 PASS / 0 FAIL.
- **Submodules:** none yet (`containers` planned per spec §9.5; SDK submodules planned per §11 once first channel adapter lands).

## Risk surface

- **Spec-vs-implementation gap.** The longer the spec evolves without code, the higher the risk of accidentally documenting something un-buildable. Mitigation: §26.5 Operator quickstart is the first thing to actually build and verify end-to-end; it'll surface invalid assumptions fast.
- **Claude Code session model assumption.** V3 §33.2 commits to a project-named-session anchor pattern that hasn't been validated against Claude Code in a real test. Mitigation: in HRD-008+ scaffolding work, the Claude Code dispatcher MUST be the first integration with a captured-evidence end-to-end test.
- **Channel-credential rotation.** V3 spec mandates rotation paths but no operator tooling exists. Mitigation: `<flavor>herald doctor` (§17.6) will be one of the first CLI subcommands implemented so operators catch credential drift before it bites.
