<div align="center">

<img src="../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Herald — `rherald` Flavor Guide (Operator)

| Field | Value |
|---|---|
| Revision | 1 |
| Created | 2026-05-30 |
| Last modified | 2026-05-30 |
| Status | active |
| Status summary | Nano-detail operator reference for `rherald` (Release Herald) — a CLI-only flavor (DefaultPort=0) for the release path. Documents `changelog-generate` (§5 Conventional-Commits changelog), `tag-mirror` (§4 cross-mirror tag-parity gate), `gate-retest` (§11.4.40 pre-tag full-suite retest gate), and `version`. ANTI-BLUFF: derived from the built `rherald` binary (`rherald --help`, per-subcommand `--help`, `version --json`) + `commons/branding.go`. The gates read recorded state and exit non-zero on a breach; they never create/push tags or run the suites themselves (§12/§107 host-safety). |
| Issues | (none specific to this guide) |
| Continuation | Bump as the release path gains commands. |

## Table of contents

- [§1. What `rherald` is](#1-what-rherald-is)
- [§2. The subcommand surface](#2-the-subcommand-surface)
- [§3. `version`](#3-version)
- [§4. `changelog-generate`](#4-changelog-generate)
- [§5. `tag-mirror`](#5-tag-mirror)
- [§6. `gate-retest`](#6-gate-retest)
- [§7. References](#7-references)

---

## §1. What `rherald` is

`rherald` is **Release Herald** — flavor key `r`, prefix `RHR`, **CLI-only** (DefaultPort=0, no serve mode). Per `commons/branding.go` its mission is "Tag mirroring + changelog + installable-asset evidence". It is invoked at release time to (1) generate a Conventional-Commits changelog, (2) gate that a release tag has full parity across every owned mirror, and (3) gate that a green full-suite retest preceded the tag. Per §12/§107 host-safety, `tag-mirror` and `gate-retest` are read-only/observe-only — they never create or push a tag, and never run the real test suite.

Build it:

```bash
go build -o /tmp/rherald ./rherald/cmd/rherald
```

## §2. The subcommand surface

Verbatim from `rherald --help`:

| Subcommand | What it does |
|---|---|
| `changelog-generate` | Generate a Conventional-Commits changelog for a release version (§5) |
| `tag-mirror` | Assert a release tag has full parity across every owned mirror (§4) |
| `gate-retest` | Pre-tag full-suite retest GATE: classify the recorded retest outcome (§11.4.40) |
| `version` | Print Release Herald version + build info |
| `completion` | Generate shell autocompletion (Cobra built-in) |

All three classifying commands accept `--emit` (drive the verdict as a real constitution event) and `--repo string` (default: discovered from CWD).

## §3. `version`

```bash
$ rherald version --json
{"arch":"arm64","binary":"rherald","build_time":"unknown","commit":"unknown","flavor":"r","go_version":"go1.26.2","os":"darwin","version":"0.0.0-dev"}
```

## §4. `changelog-generate`

`changelog-generate <version>` reads the commit graph (optionally bounded by `--since <previous-tag>`), groups each commit subject by its Conventional-Commits type (`feat` / `fix` / `docs` / …), writes the grouped changelog to `<out-dir>/<version>.md`, then classifies §5 conformance through the HRD-022 binding. **§5 is Warn-tier** — a non-conforming changelog is a warning, not a hard exit. The only write is the changelog file, scoped to the operator-supplied repo.

| Flag | Meaning |
|---|---|
| `--out-dir string` | Output dir for `<version>.md` (default `<repo>/docs/changelogs`) |
| `--since string` | Previous tag/ref bounding the range (default: full history) |
| `--repo string` | Repo dir (default: discovered from CWD) |

```bash
rherald changelog-generate v0.4.0 --since v0.3.0
```

## §5. `tag-mirror`

`tag-mirror <tag>` observes whether release tag `<tag>` is present locally AND on every configured mirror remote, builds the §4 tag-mirror-parity Subject from the observed tally, and **exits non-zero when the verdict is FAIL** — a tag present on the parent but missing on any owned mirror is a §4 violation. Mirrors are read from the repeatable `--remote` flag or (default) from the `upstreams/*.sh` declarations. It **never creates or pushes a tag** — it reads tag state only via `ls-remote`.

| Flag | Meaning |
|---|---|
| `--remote strings` | Mirror remote to check (repeatable; default: `upstreams/*.sh`) |
| `--upstreams-dir string` | Dir holding `*.sh` mirror declarations (default `<repo>/upstreams`) |
| `--repo string` | Repo dir (default: discovered from CWD) |

```bash
rherald tag-mirror v0.4.0 || echo "MIRROR PARITY MISSING"
```

## §6. `gate-retest`

PRE-TAG GATE: classifies whether the composite full-suite retest PASSED before a release tag (the highest-severity release gate) through the HRD-022 §11.4.40 binding, and **exits non-zero when the verdict is FAIL** — a tag attempted without a green all-tier retest is BLOCKED. The retest outcome is supplied via the `--retest-result` seam (or read from `--results-file`) so the gate is hermetically testable and **never runs the real test suite itself**.

| Flag | Meaning |
|---|---|
| `--retest-result string` | Recorded composite retest outcome: `pass`/`fail`/`green`/`red`/`skipped` (test/CI seam) |
| `--results-file string` | File whose first line is the retest outcome (alternative to `--retest-result`) |
| `--tiers int` | Number of §40.2 test tiers the retest covered (must be ≥ 8 to pass) (default 8) |
| `--repo string` | Repo dir (default: discovered from CWD) |

> NOTE: `gate-retest` is the alias shared with `bherald gate-retest`; on `rherald` it is **live**, whereas the `bherald`-side body is still open (HRD-045).

## §7. References

- Source: `rherald/cmd/rherald/main.go` + `release_cmds.go`.
- Branding: `commons/branding.go` (flavor `r`, DefaultPort=0).
- Mirror declarations: `upstreams/{GitHub,GitLab,GitFlic,GitVerse}.sh`.
- Constitution bindings: HRD-022 §4 / §5 / §11.4.40; §40.2 8-tier matrix.
- Companion flavor guides: `docs/guides/{PHERALD,SHERALD,CHERALD,BHERALD,IHERALD,SCHERALD,QAHERALD}.md`.

## Sources verified

**Verified 2026-05-30:** internal doc — no external online sources. Every subcommand, flag, default, and gate-behaviour claim above was derived by running the built `rherald` binary (`rherald --help`; `rherald changelog-generate|tag-mirror|gate-retest --help`; `rherald version --json`) and reading `commons/branding.go` on 2026-05-30. No flags were invented. Re-verify whenever the `rherald` Cobra surface changes.
