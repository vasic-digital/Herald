# Herald — Continuation

| Field | Value |
|---|---|
| Revision | 1 |
| Created | 2026-05-20 |
| Last modified | 2026-05-20 |
| Status | active |
| Status summary | Sacred handoff doc per Universal §12.10. Carries the verbatim resumption prompt + concrete next steps so any agent can resume work without conversational context. |
| Issues | HRD-007 (closing this commit) |
| Issues summary | see `Issues_Summary.md` |
| Fixed | HRD-001..HRD-006 |
| Fixed summary | see `Fixed_Summary.md` |
| Continuation | This file IS the continuation pointer. Update on every non-trivial commit (per Universal §12.10). |

## Table of contents

- [§0. How to use this document](#0-how-to-use-this-document)
- [§1. Snapshot](#1-snapshot)
- [§2. Last commit landed](#2-last-commit-landed)
- [§3. Active work](#3-active-work)
- [§4. Next concrete steps](#4-next-concrete-steps)
- [§5. Long-form pointers](#5-long-form-pointers)

## §0. How to use this document

Paste the following block into any CLI agent (Claude Code, OpenCode, Cursor, Aider, Gemini CLI) to resume Herald work exactly where it was left:

> You are working on the Herald project at `~/Projects/Herald` (also reachable as the `Herald/` submodule of a consuming project). The Helix Universal Constitution lives at `<ancestor>/constitution/` (parent-walk discovery). Read in this order: `CLAUDE.md`, `AGENTS.md`, `README.md`, `docs/guides/HERALD_CONSTITUTION.md`, `docs/guides/CONSTITUTION_INHERITANCE.md`, `docs/specs/mvp/specification.V3.md`. Then read `docs/CONTINUATION.md` (this file) for live state, `docs/Issues.md` for open work, `docs/Status.md` for current phase, `docs/Fixed.md` for closed history. The inheritance gate `tests/test_constitution_inheritance.sh` MUST exit 0 before any commit. Multi-mirror fan-out push to all four hosts (GitHub + GitLab + GitFlic + GitVerse) is mandatory on every commit per Constitution §103.

## §1. Snapshot

- **Active spec:** `docs/specs/mvp/specification.V3.md` (Revision 3, ~3900 lines, Status=active).
- **Archived specs:** `docs/specs/mvp/archive/specification.V1.md`, `…/specification.V2.md` (frozen).
- **Tracking docs:** `Issues.md`, `Issues_Summary.md`, `Fixed.md`, `Fixed_Summary.md`, `Status.md`, `Status_Summary.md`, `CONTINUATION.md` (this file) — all established this commit.
- **Inheritance gate:** 12 PASS / 0 FAIL. Meta-test ✓.
- **Phase:** pre-implementation. No `go.mod`, no Go source, no container compose stack yet.
- **Branch:** `main` only. Fan-out push to 4 mirrors is proven.

## §2. Last commit landed

(This commit, when merged) — V3 r3 final polish: cross-doc sync (parent docs point at `specification.V3.md`); tracking docs scaffold (`Issues.md`/`Fixed.md`/`Status.md`/`CONTINUATION.md` + summaries); §30.8 review log added; full re-export of V3 + 4 parent docs to `.html` + `.pdf`. Inheritance gate 12 PASS / 0 FAIL throughout.

Prior commit: V3 r2 at `f8b8073` — refined all nine flavors for richer channel interaction; V2 archived; ToC duplicates cleaned.

## §3. Active work

| ID | Status | What |
|---|---|---|
| HRD-007 | in_progress → resolving in this commit | V3 r3 cross-doc sync + tracking-docs scaffold |

No other items in flight.

## §4. Next concrete steps

1. **Validate the V3 §26.5 quickstart compose** on a fresh laptop. Confirm Postgres + Redis + OTel collector + `pherald` boot cleanly, that `curl` posts a CloudEvent to `/v1/events`, that a Telegram delivery lands, and the diary appends. **Open `HRD-008` for this validation work.**
2. **Scaffold `commons` and `commons_messaging`** against the §11.0 type contract. Goal: empty implementations of `Channel`, `OutboundMessage`, `Receipt`, `InboundEvent`, `Branding`, `ChannelID`, `Subscriber` — plus the `null://` adapter (§11.14) for tests. **Open `HRD-009`.**
3. **Implement the Postgres + River queue layer** per §5.3 + §16. RLS policy template + per-tenant key prefixing in Redis. Migrations under `commons_storage/migrations/000NNN_*.sql` per §9.6. **Open `HRD-010`.**
4. **First channel adapter** — Telegram per §11.1. Bot API client behind `Channel` interface; webhook ingress with §5.5 signature verification. **Open `HRD-011`.**
5. **Claude Code dispatcher** — per §33.2 + §33.3. The session-resolution algorithm needs real-world validation; capture evidence in `HRD-008`'s end-to-end test extension. **Open `HRD-012`.**

After those five, the first-implementation cycle is verifiably complete and Herald is consumable as a submodule by a real project.

## §5. Long-form pointers

- `docs/specs/mvp/specification.V3.md` — full active spec.
- `docs/specs/mvp/specification.V3.md#30-v2-self-review-log` — every review pass (§30.1..§30.8) documented for any future archaeology.
- `docs/guides/HERALD_CONSTITUTION.md` — Herald-specific articles §101..§106 extending Universal Constitution.
- `docs/guides/CONSTITUTION_INHERITANCE.md` — parent-discovery + inheritance-gate contract.
- `tests/test_constitution_inheritance.sh` — the gate. Run before every commit that touches root docs.
- `tests/test_constitution_inheritance_meta.sh` — paired §1.1 mutation proof.
