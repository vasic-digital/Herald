# Herald Constitution

| Field | Value |
|---|---|
| Revision | 3 |
| Created | 2026-05-15 |
| Last modified | 2026-05-20 |
| Status | active |
| Status summary | Spec-path references updated to specification.V3.md (active); Notes section reflects V1+V2 archived under docs/specs/mvp/archive/. |
| Issues | none |
| Issues summary | — |
| Fixed | R-14 (V2), V3-path-sync (V3 r3) |
| Fixed summary | §106 spec-change rule retargeted to V3 path; gate-checked anchor 'comprehensive planning and implementation' unchanged so I7 stays green. |
| Continuation | — |

## Table of contents

- [Project Articles](#project-articles)
  - [§101. Pre-implementation status](#101-pre-implementation-status)
  - [§102. Mission boundary](#102-mission-boundary)
  - [§103. Mirror parity (extends Universal §2.1)](#103-mirror-parity-extends-universal-21)
  - [§104. No embedded constitution (extends Universal §3)](#104-no-embedded-constitution-extends-universal-3)
  - [§105. Inheritance gate (extends Universal §1.1)](#105-inheritance-gate-extends-universal-11)
  - [§106. Spec-change rule (extends Universal §11.4)](#106-spec-change-rule-extends-universal-114)
- [Overrides of Universal Constitution](#overrides-of-universal-constitution)
- [Owned-submodule set (per Universal §4)](#owned-submodule-set-per-universal-4)
- [Project-specific remotes](#project-specific-remotes)
- [Notes](#notes)

This constitution **extends** the Helix Universal Constitution provided by the **parent project's** `constitution/` submodule. Herald does not carry its own copy — see `docs/guides/CONSTITUTION_INHERITANCE.md` for the discovery contract.

All clauses in the parent-provided `constitution/Constitution.md` apply unless explicitly overridden below with an explicit `Override §X.Y` section.

Canonical constitution repo: <https://github.com/HelixDevelopment/HelixConstitution>

## Project Articles

### §101. Pre-implementation status

Herald is pre-implementation. Until a `go.mod` is committed, no clause below may be interpreted as authorizing the agent to fabricate build/test infrastructure that doesn't yet exist. Confirm the disambiguation (scaffold vs. fill spec) with the operator before writing code, per Universal §11.4.6 (no-guessing).

### §102. Mission boundary

Herald ingests system events and fans them out to multiple notification channels. Anything outside event ingestion or notification fan-out is **out of scope** for this repo and belongs in a different submodule of the consuming project.

### §103. Mirror parity (extends Universal §2.1)

Every commit on `main` MUST land on all four upstream hosts (GitHub, GitLab, GitFlic, GitVerse) in a single fan-out push. The repo's `origin` remote already aggregates the four push URLs; do not bypass `origin` with per-host pushes unless rebuilding the fan-out configuration.

### §104. No embedded constitution (extends Universal §3)

Herald **MUST NOT** carry its own `constitution/` submodule. Herald is consumed as a submodule of a parent project that already provides the constitution; a duplicated copy would diverge in pinning, cause confusion about which is authoritative, and violate the "submodule commits propagate first" propagation order (Universal §3). The inheritance gate's invariant `I6` enforces this: it FAILs if `<repo-root>/constitution/` or `.gitmodules` reappears.

For standalone development of Herald (no parent project), clone the constitution **alongside** Herald, not inside it:

```bash
git clone git@github.com:HelixDevelopment/HelixConstitution.git \
    $(dirname "$PWD")/constitution
```

### §105. Inheritance gate (extends Universal §1.1)

`tests/test_constitution_inheritance.sh` discovers the parent-provided constitution via inline parent-walk (mirrors `find_constitution.sh` Phase 1), then asserts six invariants:

| # | Invariant |
|---|---|
| I1 | A `constitution/Constitution.md` is reachable by walking up the parent chain from Herald's repo root. |
| I2 | The discovered `Constitution.md` contains the `§11.4 End-user quality guarantee — forensic anchor` line (the exact string the §1.1 mutation removes). |
| I3 | The discovered `CLAUDE.md` contains the `MANDATORY ANTI-BLUFF COVENANT` anchor. |
| I4 | The discovered `AGENTS.md` contains the `Anti-bluff covenant` anchor. |
| I5a–d | Herald's root docs (`CLAUDE.md`, `AGENTS.md`, this file, and `README.md`) all declare the parent-discovery inheritance contract. |
| I6 | No `constitution/` directory or `.gitmodules` file exists at Herald's root (the §104 invariant). |
| I7a–c | Herald's `CLAUDE.md`, `AGENTS.md`, and this file all contain the §106 spec-change rule anchor (per §1.1 propagation, mutation-paired). |

`tests/test_constitution_inheritance_meta.sh` delegates to the discovered constitution's `meta_test_inheritance.sh`, which strips the §11.4 anchor from `Constitution.md`, runs the gate, and asserts FAIL — proving the gate is not a bluff (Universal §1.1).

Both scripts run as a precondition to any commit that touches root docs or the discovery contract.

### §106. Spec-change rule (extends Universal §11.4)

Any modification to `docs/specs/mvp/specification.V3.md` or any file under `docs/specs/` (any depth) triggers **mandatory comprehensive planning and implementation of all changes**. An agent or contributor may not edit the spec in isolation: every change is a project-wide ripple that requires the corresponding code, tests, and downstream doc updates in the same logical work effort.

This rule does NOT apply to creating or renaming files; for those, the operator must explicitly tell the worker (CLI agent or human contributor) what to do with the newly created or copied files.

**Propagation.** The same rule is restated in Herald's `CLAUDE.md` and `AGENTS.md` (per §1.1 multi-file propagation discipline). The inheritance gate's invariant **I7a–c** asserts the rule's anchor literal (`comprehensive planning and implementation`) is present in all three files; a missing copy is a propagation bluff and the gate FAILs.

**Anchor (forensic):** the literal text `Whenever this document (\`docs/specs/mvp/specification.V3.md\`)` MUST appear in `docs/specs/mvp/specification.V3.md` §"Specification documents" — that line is the source of truth that all three propagated copies summarize.

**Paired §1.1 mutation (planned).** Removing the spec-change anchor from any of the three propagation files MUST cause `I7a/b/c` to FAIL; the paired meta-test will be added when `test_constitution_inheritance_meta.sh` is generalised beyond its current single-anchor mutation.

---

## Overrides of Universal Constitution

(none — Herald has no exceptions to universal clauses at pre-implementation stage)

---

## Owned-submodule set (per Universal §4)

```
(none)
```

Herald owns no submodules. The constitution is provided by the parent project, not vendored here.

---

## Project-specific remotes

| Repo | Remotes |
|---|---|
| Herald (this repo) | `github`, `gitlab`, `gitflic`, `gitverse` + `origin` (fan-out push to all four) |

---

## Notes

Herald's spec is now in V3 (`docs/specs/mvp/specification.V3.md`, ~3900 lines, active) — V1 and V2 are preserved under `docs/specs/mvp/archive/` for historical reference. As project-specific articles mature toward universal status they may move into the Helix Constitution; promotion requires the §11.4 universal-vs-project audit. Default is to keep rules here until the audit clears.
