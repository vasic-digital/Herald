# Herald Constitution

This constitution **extends** the Helix Universal Constitution at `constitution/Constitution.md`. All clauses there apply unless explicitly overridden below with an explicit `Override §X.Y` section.

The HelixConstitution submodule is pinned at `1b0b03a2223fffcd71eb7fc8a9620d0c7b4ed8f3` (incorporated 2026-05-15).

## Project Articles

### §101. Pre-implementation status

Herald is pre-implementation. Until a `go.mod` is committed, no clause below may be interpreted as authorizing the agent to fabricate build/test infrastructure that doesn't yet exist. Confirm the disambiguation (scaffold vs. fill spec) with the operator before writing code, per Universal §11.4.6 (no-guessing).

### §102. Mission boundary

Herald ingests system events and fans them out to multiple notification channels. Anything outside event ingestion or notification fan-out is **out of scope** for this repo and belongs in a different submodule of the consuming project.

### §103. Mirror parity (extends Universal §2.1)

Every commit on `main` MUST land on all four upstream hosts (GitHub, GitLab, GitFlic, GitVerse) in a single fan-out push. The repo's `origin` remote already aggregates the four push URLs; do not bypass `origin` with per-host pushes unless rebuilding the fan-out configuration.

### §104. Inheritance gate (extends Universal §1.1)

`tests/test_constitution_inheritance.sh` asserts that the constitution submodule is checked out, anchored, and referenced from the root docs. `tests/test_constitution_inheritance_meta.sh` mutates the §11.4 anchor in the submodule's `Constitution.md`, invokes the gate, and asserts FAIL — proving the gate is not a bluff. Both run as a precondition to any commit that touches root docs or the submodule pointer.

---

## Overrides of Universal Constitution

(none — Herald has no exceptions to universal clauses at pre-implementation stage)

---

## Owned-submodule set (per Universal §4)

```
constitution/  (third-party — HelixDevelopment/HelixConstitution; NOT owned by Herald)
```

Herald owns no submodules. The `constitution/` submodule is read-only from Herald's side; changes go through the HelixConstitution repo directly.

---

## Project-specific remotes

| Repo | Remotes |
|---|---|
| Herald (this repo) | `github`, `gitlab`, `gitflic`, `gitverse` + `origin` (fan-out push to all four) |
| `constitution/` | same naming, pointing at `HelixDevelopment/HelixConstitution` mirrors |

---

## Notes

Most substantive sections of Herald's spec (`docs/specs/mvp/specification.md`) are still `Tbd`. As they fill in, project-specific articles (§105+) may be added here for invariants that are universal-grade but not yet promoted to the Helix Constitution. Promotion requires the §11.4 universal-vs-project audit; default to keeping rules in this file until that audit clears.
