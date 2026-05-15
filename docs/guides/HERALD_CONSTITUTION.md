# Herald Constitution

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

`tests/test_constitution_inheritance_meta.sh` delegates to the discovered constitution's `meta_test_inheritance.sh`, which strips the §11.4 anchor from `Constitution.md`, runs the gate, and asserts FAIL — proving the gate is not a bluff (Universal §1.1).

Both scripts run as a precondition to any commit that touches root docs or the discovery contract.

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

Most substantive sections of Herald's spec (`docs/specs/mvp/specification.md`) are still `Tbd`. As they fill in, project-specific articles (§106+) may be added here for invariants that are universal-grade but not yet promoted to the Helix Constitution. Promotion requires the §11.4 universal-vs-project audit; default to keeping rules in this file until that audit clears.
