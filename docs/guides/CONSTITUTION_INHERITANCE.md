# Constitution inheritance — operator & agent guide

| Field | Value |
|---|---|
| Revision | 1 |
| Created | 2026-05-15 |
| Last modified | 2026-05-19 |
| Status | active |
| Status summary | — |
| Issues | none |
| Issues summary | — |
| Fixed | none |
| Fixed summary | — |
| Continuation | — |

## Table of contents

- [Summary](#summary)
- [Why no embedded submodule?](#why-no-embedded-submodule)
- [Discovery contract](#discovery-contract)
  - [Standalone development](#standalone-development)
- [The inheritance gate](#the-inheritance-gate)
  - [Common failure: `FAIL  I1 constitution NOT found …`](#common-failure-fail-i1-constitution-not-found)
  - [Common failure: `FAIL  I6 No constitution submodule embedded in Herald`](#common-failure-fail-i6-no-constitution-submodule-embedded-in-herald)
- [The paired mutation meta-test (§1.1)](#the-paired-mutation-meta-test-11)
- [When to run the gates](#when-to-run-the-gates)
- [Promoting Herald rules into the constitution](#promoting-herald-rules-into-the-constitution)
- [Glossary](#glossary)

This guide explains how Herald inherits from the Helix Universal Constitution, why Herald does **not** keep its own copy, how the runtime discovery contract works, and how the inheritance gate enforces both.

If you only need the rule, read [§Summary](#summary) and stop. If you're operating Herald, building tooling around it, or debugging a `FAIL  I1` from the gate, read on.

## Summary

- Herald **does not** carry a `constitution/` submodule.
- Herald is consumed as a submodule of a parent project that **already provides** `<parent>/constitution/`.
- For standalone Herald work, clone the constitution **alongside** Herald (`$(dirname "$PWD")/constitution`), never inside.
- The inheritance contract is enforced by `tests/test_constitution_inheritance.sh` (gate) and `tests/test_constitution_inheritance_meta.sh` (paired §1.1 mutation proof).
- Discovery uses parent-walk: walk up directories until you find `<ancestor>/constitution/Constitution.md`.

## Why no embedded submodule?

Herald is a building block. It will be used as a submodule by several parent projects. Each parent already includes the Helix Constitution at `<parent>/constitution/`. If Herald also carried its own `constitution/`:

1. **Two pins, one source of truth.** The parent and Herald could pin to different constitution SHAs, silently disagreeing on which clauses apply. Constitution §3 requires submodule commits propagate first — duplicate copies make the propagation order undefined.
2. **Doubled storage** for a 250 KB content set that should be shared.
3. **Confusion about authority.** Two `constitution/CLAUDE.md` files (one at `<parent>/constitution/`, one at `<parent>/Herald/constitution/`) is exactly the kind of "which file wins?" ambiguity the anti-bluff covenant exists to prevent.

Herald therefore inherits from the parent's constitution and provides only a discovery mechanism to locate it.

## Discovery contract

Discovery uses the same algorithm as the canonical `find_constitution.sh` (Phase 1):

1. Start at Herald's repo root.
2. Walk up parent directories.
3. At each step, check whether `<dir>/constitution/Constitution.md` exists.
4. First match wins; print its absolute path and exit 0.
5. If no ancestor matches, exit non-zero with an error.

This works at any nesting depth:

```
SomeBigProject/                       ← parent project, ships constitution/
├── constitution/                     ← provided by the parent
│   ├── Constitution.md
│   ├── CLAUDE.md
│   ├── AGENTS.md
│   └── find_constitution.sh
└── third-party/
    └── notification/
        └── Herald/                   ← Herald lives here; walks up 3 levels
            └── tests/test_constitution_inheritance.sh
```

From Herald's gate, the parent-walk visits `Herald/` → `notification/` → `third-party/` → `SomeBigProject/` and matches at the last step.

### Standalone development

If you cloned just Herald (no parent project), put a clone of the constitution **alongside** Herald — exactly mirroring the `Projects/` layout used during Herald's bootstrap:

```bash
git clone git@github.com:HelixDevelopment/HelixConstitution.git \
    $(dirname "$PWD")/constitution
```

This gives you `Projects/Herald/` (your work) and `Projects/constitution/` (the sibling). Parent-walk from Herald visits `Herald/` → `Projects/` and matches.

**Do not clone the constitution inside Herald.** The gate's invariant `I6` will FAIL if `<repo-root>/constitution/` or `.gitmodules` reappears.

## The inheritance gate

`tests/test_constitution_inheritance.sh` is read-only and idempotent. It asserts:

| Invariant | What it checks |
|---|---|
| **I1** | A `constitution/Constitution.md` is reachable via parent-walk from Herald's repo root. Prints the discovered path. |
| **I2** | The discovered `Constitution.md` contains the exact line `### §11.4 End-user quality guarantee — forensic anchor (User mandate, 2026-04-28)`. This is the line the §1.1 mutation removes; the gate's I2 and the mutation's removal are paired by design. |
| **I3** | The discovered `CLAUDE.md` contains `MANDATORY ANTI-BLUFF COVENANT`. |
| **I4** | The discovered `AGENTS.md` contains `Anti-bluff covenant`. |
| **I5a** | Herald's `CLAUDE.md` declares `INHERITED FROM Helix Constitution` and references `find_constitution.sh`. |
| **I5b** | Herald's `AGENTS.md` references `find_constitution.sh`. |
| **I5c** | `docs/guides/HERALD_CONSTITUTION.md` `extends` the `Helix Universal Constitution`. |
| **I5d** | `README.md` documents the `Helix Constitution` inheritance contract. |
| **I6** | No `constitution/` directory or `.gitmodules` file exists at Herald's root (§104). |

Run:

```bash
bash tests/test_constitution_inheritance.sh
```

Expected output on a healthy tree:

```
PASS  I1 constitution discovered at /Users/<you>/Projects/constitution
PASS  I2 Constitution.md contains §11.4 forensic anchor
PASS  I3 constitution/CLAUDE.md contains anti-bluff covenant anchor
PASS  I4 constitution/AGENTS.md contains anti-bluff covenant anchor
PASS  I5a CLAUDE.md declares Helix Constitution inheritance (parent-discovery)
PASS  I5b AGENTS.md references the parent-provided constitution + find_constitution.sh
PASS  I5c HERALD_CONSTITUTION.md extends the Helix Universal Constitution
PASS  I5d README.md documents the constitution-inheritance contract
PASS  I6 No constitution submodule embedded in Herald
----
Result: 9 PASS / 0 FAIL
```

Non-zero exit means at least one invariant FAILed; fix the offending invariant at root cause per Universal §11.4.4 (test-interrupt-on-discovery).

### Common failure: `FAIL  I1 constitution NOT found …`

Means there's no `<ancestor>/constitution/Constitution.md` reachable above Herald. Either:

- **You're working standalone** — clone the constitution as a sibling (see [Standalone development](#standalone-development)).
- **You're inside a parent project that lacks the constitution submodule** — fix the parent. Herald is not the right place to fix this.

### Common failure: `FAIL  I6 No constitution submodule embedded in Herald`

Means somebody re-added `constitution/` inside Herald (a §104 violation). Remove it cleanly:

```bash
git submodule deinit -f constitution
git rm -f constitution
rm -rf .git/modules/constitution
# If .gitmodules becomes empty, delete it too:
[ ! -s .gitmodules ] && git rm -f .gitmodules
```

## The paired mutation meta-test (§1.1)

`tests/test_constitution_inheritance_meta.sh` is the proof that the gate is not itself a bluff. Per Universal §1.1, every gate must have a paired mutation demonstrating it catches the regression it claims to catch.

The meta-test:

1. Discovers the constitution (same algorithm as the gate).
2. Delegates to `<discovered>/meta_test_inheritance.sh`, passing Herald's gate as the command-under-test.
3. The constitution's meta-runner snapshots `Constitution.md`, strips the §11.4 anchor, runs Herald's gate, asserts the gate exits non-zero, then restores the file.
4. If the gate exited zero with the anchor removed, the meta-test fails with `META-TEST FAIL: gate returned 0 even though the §11.4 anchor was removed`.

Run:

```bash
bash tests/test_constitution_inheritance_meta.sh
```

Expected output on a healthy tree:

```
Mutation applied: §11.4 anchor removed from Constitution.md
Running consuming-project gate: bash '/…/tests/test_constitution_inheritance.sh' > /dev/null 2>&1
✓ META-TEST PASS: gate correctly FAILed (rc=1) on mutated Constitution
```

The restore happens via `trap` inside the constitution-side runner; if you ever see a `.bak` file alongside `Constitution.md` after a meta-test run, the restore was interrupted — restore manually from the `.bak`.

## When to run the gates

- **Before any commit that touches root docs** (`CLAUDE.md`, `AGENTS.md`, `README.md`), `docs/guides/`, or `tests/test_constitution_inheritance*`.
- **After cloning** Herald standalone, immediately after cloning the constitution sibling.
- **In CI** as a precondition to any other test (catches drift early).
- **When integrating Herald into a new parent project** to confirm discovery works at the new nesting depth.

## Promoting Herald rules into the constitution

If Herald grows a rule that genuinely deserves universal status (applies to ≥3 unrelated projects, references no Herald-specific tech, encodes a policy not a configuration value), promote it via the HelixConstitution repo, not by editing the parent's `constitution/` from Herald. Universal §11.4 + §11.4.10 require this audit; the default is to keep rules in `docs/guides/HERALD_CONSTITUTION.md` until the audit clears.

## Glossary

- **Parent project** — the larger project that includes Herald as a submodule and provides `<parent>/constitution/`.
- **Discovered constitution** — the result of parent-walk; the absolute path printed by `bash <ancestor>/constitution/find_constitution.sh` or by `I1` of Herald's gate.
- **Forensic anchor** — the exact line `### §11.4 End-user quality guarantee — forensic anchor (User mandate, 2026-04-28)`. The gate checks for it; the meta-test removes it.
- **Bluff gate** — a gate that returns PASS without actually checking the thing it claims to check. Universal §1.1 forbids them; paired mutations are the mechanism that catches them.
