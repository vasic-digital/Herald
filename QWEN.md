<div align="center">

![Herald](assets/logo/herald_logo_square_128.png){width=96px height=96px}

</div>

# QWEN.md

This file provides guidance to **Qwen Code** sessions when working with code in this repository, in parity with `CLAUDE.md` and `AGENTS.md`. The constitutional rules in those files apply unchanged to Qwen agents — this file restates the §107 anti-bluff anchor that MUST live verbatim in every root doc per the operator mandate.

For the full Herald-specific ruleset see:
- `CLAUDE.md` (Claude Code session guidance — primary)
- `AGENTS.md` (cross-agent guardrails)
- `docs/guides/HERALD_CONSTITUTION.md` (Herald's project-specific constitutional extensions)
- The parent constitution at `<parent>/constitution/Constitution.md` (Helix Universal Constitution — inherited unconditionally)

## INHERITED FROM Helix Constitution (parent-discovery)

Herald is consumed as a submodule of a parent project that carries the Helix Constitution submodule at `<parent>/constitution/`. Locate it from any nested depth via:

```bash
CONST_DIR="$(bash "$(find . -type d -name constitution -print -quit 2>/dev/null)/find_constitution.sh")"
```

For standalone development of Herald, clone the constitution alongside:

```bash
git clone git@github.com:HelixDevelopment/HelixConstitution.git $(dirname "$PWD")/constitution
```

All rules in `<discovered>/CLAUDE.md` + `<discovered>/Constitution.md` + `<discovered>/QWEN.md` apply unconditionally to Qwen agents.

## §107 — End-user-usability covenant (Herald §107 / Helix §11.4 — MANDATORY ANTI-BLUFF)

**Forensic anchor — verbatim operator mandate:**

> "all existing tests and Challenges do work in anti-bluff manner - they MUST confirm that all tested codebase really works as expected! We had been in position that all tests do execute with success and all Challenges as well, but in reality the most of the features does not work and can't be used! This MUST NOT be the case and execution of tests and Challenges MUST guarantee the quality, the completition and full usability by end users of the product! This MUST BE part of Constitution of our project, its CLAUDE.MD and AGENTS.MD if it is not there already, and to be applied to all Submodules's Constitution, CLAUDE.MD and AGENTS.MD as well (if not there already)!"

The bar for shipping any Herald feature is **NOT** "tests pass" — it is **"the end user of the flavor binary can actually use the feature."** Every PASS (unit, integration, gate, Challenge, smoke, e2e) MUST carry positive runtime evidence that the user-visible behaviour works. Metadata-only / configuration-only / "absence-of-error" / grep-only PASS are §11.4 PASS-bluffs and constitute critical defects regardless of how green the summary line looks.

Canonical Herald authority: `docs/guides/HERALD_CONSTITUTION.md` §107.
Canonical Helix authority: `<discovered>/Constitution.md` §11.4 + §11.4.1..§11.4.16 and `<discovered>/CLAUDE.md` "MANDATORY ANTI-BLUFF COVENANT — END-USER QUALITY GUARANTEE".
Canonical Herald evidence: `scripts/e2e_bluff_hunt.sh` (41 invariants against real services; ALL must PASS).

Inheritance gate invariant **I8a** asserts this covenant anchor is present in CLAUDE.md; **I8b** asserts it in AGENTS.md; **I8c** asserts it in HERALD_CONSTITUTION.md. This QWEN.md restates the anchor for Qwen Code session parity (added 2026-05-22 doc-cleanup pass alongside QWEN.md propagation across all Helix-stack submodules).

## Qwen-specific notes

Qwen agents follow the same disciplines documented for Claude Code:
- Run the anti-bluff battery (`scripts/audit_antibluff.sh`, `scripts/e2e_bluff_hunt.sh`, `tests/test_constitution_inheritance.sh`, `tests/test_wave2_mutation_meta.sh`, `tests/test_wave3_mutation_meta.sh`) before declaring any task done.
- Use the subagent-driven pattern (Universal §11.4.70) for non-trivial work.
- Never commit secrets — see `docs/guides/OPERATOR_CREDENTIALS.md` and the wizard at `pherald wizard credentials`.
- Multi-mirror push via `git push origin <branch>` (origin fans out to github + gitlab + gitflic + gitverse).

For anything not covered here, defer to `CLAUDE.md` — its rules apply verbatim to Qwen sessions.
