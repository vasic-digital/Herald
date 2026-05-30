<div align="center">

<img src="../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Herald — `bherald` Flavor Guide (Operator)

| Field | Value |
|---|---|
| Revision | 1 |
| Created | 2026-05-30 |
| Last modified | 2026-05-30 |
| Status | active |
| Status summary | Nano-detail operator reference for `bherald` (Build Herald) — a CLI-only flavor (no HTTP serve mode; DefaultPort=0) for CI/test gates. Documents `evidence-capture` (§11.4.2 anti-bluff evidence requirement), `test-tier-verify` (§40.2 8-tier matrix per §11.4.27), the not-yet-implemented `gate-retest` (HRD-045), and `version`. ANTI-BLUFF: derived from the built `bherald` binary (`bherald --help`, per-subcommand `--help`, `version --json`) + `commons/branding.go`. The gates are PURE classifiers — they observe recorded outcomes and exit non-zero on a breach; they never run the suites themselves. |
| Issues | (none specific to this guide) |
| Continuation | Bump when `gate-retest` (HRD-045) lands. |

## Table of contents

- [§1. What `bherald` is](#1-what-bherald-is)
- [§2. The subcommand surface](#2-the-subcommand-surface)
- [§3. `version`](#3-version)
- [§4. `evidence-capture`](#4-evidence-capture)
- [§5. `test-tier-verify`](#5-test-tier-verify)
- [§6. `gate-retest` (not yet implemented)](#6-gate-retest-not-yet-implemented)
- [§7. References](#7-references)

---

## §1. What `bherald` is

`bherald` is **Build Herald** — flavor key `b`, prefix `BHR`, **CLI-only** (DefaultPort=0, no serve mode). Per `commons/branding.go` its mission is "CI/test bindings + test-tier verifier + evidence capture". It is invoked from a CI pipeline to assert two anti-bluff properties at the build layer: that every PASS carries captured evidence (§11.4.2), and that a package covers the full §40.2 8-tier test matrix (§11.4.27). Both subcommands are **PURE** classifiers — they observe recorded state and exit non-zero on a breach; they never run the test suites.

Build it:

```bash
go build -o /tmp/bherald ./bherald/cmd/bherald
```

## §2. The subcommand surface

Verbatim from `bherald --help`:

| Subcommand | What it does | State |
|---|---|---|
| `evidence-capture` | Validate a CI gate PASS carries a recorded evidence artefact (§11.4.2) | live |
| `test-tier-verify` | Verify the §40.2 8-tier test matrix for a package (§11.4.27) | live |
| `gate-retest` | Re-run composite gate post-fix per §11.4.38 (alias shared with `rherald`) | **not yet implemented — HRD-045** |
| `version` | Print Build Herald version + build info | live |
| `completion` | Generate shell autocompletion (Cobra built-in) | live |

## §3. `version`

```bash
$ bherald version --json
{"arch":"arm64","binary":"bherald","build_time":"unknown","commit":"unknown","flavor":"b","go_version":"go1.26.2","os":"darwin","version":"0.0.0-dev"}
```

## §4. `evidence-capture`

Validates the §11.4.2 recorded-evidence requirement (the §107 anti-bluff covenant at the CI layer): a gate/test reporting `outcome=pass` MUST carry a captured-evidence artefact. It observes the evidence state from `--evidence-path` (a present, non-empty file/dir ⇒ evidence captured) or `--has-evidence`, classifies through the HRD-021 binding, and **exits non-zero on a metadata-only / no-evidence PASS-bluff**. An honest FAIL passes the anti-bluff check (the gate correctly reported failure). PURE: it reads the recorded outcome + the presence of the artefact; it never inspects its bytes.

| Flag | Meaning |
|---|---|
| `--outcome string` | The recorded gate outcome (`pass`/`fail`/`error`) (default `pass`) |
| `--evidence-path string` | Path to the captured-evidence artefact (present + non-empty ⇒ satisfied) |
| `--has-evidence` | Assert a captured-evidence artefact exists (test seam / operator override) |
| `--test-id string` | Test/gate identifier recorded in the §11.4.2 Subject (default `test`) |
| `--emit` | Also drive the §11.4.2 verdict as a real constitution event |

```bash
bherald evidence-capture --outcome pass --evidence-path docs/qa/HRD-100/transcript.jsonl --test-id HRD-100 || echo "PASS-BLUFF"
```

## §5. `test-tier-verify`

Verifies a package carries every tier of the §40.2 canonical 8-tier test matrix (`unit` / `component` / `integration` / `contract` / `e2e_sandbox` / `e2e_live` / `mutation` / `chaos`) per §11.4.27 (no-fakes-beyond-unit + 100% test-type coverage). It observes the present tiers from `--tier` (repeatable), `--tiers-file`, or `--all-tiers`, and **exits non-zero when any canonical tier is missing** — blocking the promotion. PURE: it classifies the recorded tier coverage; it never runs the suites.

| Flag | Meaning |
|---|---|
| `--tier strings` | A present test tier (repeatable / comma-separated) from the 8-tier set |
| `--tiers-file string` | File listing present tiers (one per line; `#` comments allowed) |
| `--all-tiers` | Assert the full §40.2 8-tier matrix is present (test seam / operator override) |
| `--pkg string` | Package label recorded in the §11.4.27 Subject (default `pkg`) |
| `--emit` | Also drive the §11.4.27 verdict as a real constitution event |

```bash
bherald test-tier-verify --pkg commons_messaging \
    --tier unit,component,integration,contract,e2e_sandbox,e2e_live,mutation,chaos
```

## §6. `gate-retest` (not yet implemented)

`bherald gate-retest` is present in the command tree but its help line reads "(not yet implemented — HRD-045)". It is an alias shared with `rherald gate-retest`; on `rherald` it is live (see `docs/guides/RHERALD.md` §6), but the `bherald`-side body is open work under HRD-045. Do not script against `bherald gate-retest` yet.

## §7. References

- Source: `bherald/cmd/bherald/main.go` + `build_cmds.go`.
- Branding: `commons/branding.go` (flavor `b`, DefaultPort=0).
- Constitution bindings: HRD-021 §11.4.2 / §11.4.27; §40.2 8-tier matrix; HRD-045 (`gate-retest`).
- Companion flavor guides: `docs/guides/{PHERALD,SHERALD,CHERALD,RHERALD,IHERALD,SCHERALD,QAHERALD}.md`.

## Sources verified

**Verified 2026-05-30:** internal doc — no external online sources. Every subcommand, flag, default, and live-vs-stub claim above was derived by running the built `bherald` binary (`bherald --help`; `bherald evidence-capture|test-tier-verify|gate-retest --help`; `bherald version --json`) and reading `commons/branding.go` on 2026-05-30. The `gate-retest` not-yet-implemented status is quoted verbatim from the binary's own help line. No flags were invented. Re-verify whenever the `bherald` Cobra surface changes (notably when HRD-045 lands).
