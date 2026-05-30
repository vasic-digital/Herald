<div align="center">

<img src="../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Herald — `scherald` Flavor Guide (Operator)

| Field | Value |
|---|---|
| Revision | 1 |
| Created | 2026-05-30 |
| Last modified | 2026-05-30 |
| Status | active |
| Status summary | Nano-detail operator reference for `scherald` (Scheduled-audit Herald) — a CLI-only flavor (DefaultPort=0) for the periodic status sweep. Documents `status-digest` (§11.4.45 status-sweep + compliance digest) and `version`. ANTI-BLUFF: derived from the built `scherald` binary (`scherald --help`, `scherald status-digest --help`, `version --json`) + `commons/branding.go`. `status-digest` is DETECT/REPORT-only by default and exits non-zero when `docs/Status.md` is missing or stale; `--apply` is the only mutating mode. |
| Issues | (none specific to this guide) |
| Continuation | Bump as the scheduled-audit catalogue gains commands. |

## Table of contents

- [§1. What `scherald` is](#1-what-scherald-is)
- [§2. The subcommand surface](#2-the-subcommand-surface)
- [§3. `version`](#3-version)
- [§4. `status-digest`](#4-status-digest)
- [§5. References](#5-references)

---

## §1. What `scherald` is

`scherald` is **Scheduled-audit Herald** — flavor key `sc`, prefix `SCR`, **CLI-only** (DefaultPort=0, no serve mode). Per `commons/branding.go` its mission is "Periodic Status.md sweep + compliance digest". It is designed to be invoked from cron (or any scheduler) to sweep the work-item trackers and report on their state, exiting non-zero when `docs/Status.md` is missing or stale so the schedule surfaces drift.

Build it:

```bash
go build -o /tmp/scherald ./scherald/cmd/scherald
```

## §2. The subcommand surface

Verbatim from `scherald --help`:

| Subcommand | What it does |
|---|---|
| `status-digest` | Sweep `docs/Status.md` + digest work-item state, classify §11.4.45 (`--apply` regenerates the summary) |
| `version` | Print Scheduled-audit Herald version + build info |
| `completion` | Generate shell autocompletion (Cobra built-in) |

## §3. `version`

```bash
$ scherald version --json
{"arch":"arm64","binary":"scherald","build_time":"unknown","commit":"unknown","flavor":"sc","go_version":"go1.26.2","os":"darwin","version":"0.0.0-dev"}
```

## §4. `status-digest`

SCHEDULED-AUDIT SWEEP: reads `docs/Status.md` (plus `docs/Issues.md` / `docs/Fixed.md` for evidence) under `--repo`, prints a concise digest of the work-item state — a tally of open vs fixed HRD-ids plus any items still flagged open in BOTH trackers as the stale-item count — builds the §11.4.45 status-sweep Subject, classifies it through the HRD-025 binding, and **exits non-zero when `docs/Status.md` is missing or stale** (a §11.4.45 violation). Default is DETECT/REPORT-only (hermetic, no mutation).

| Flag | Meaning |
|---|---|
| `--apply` | (Re)generate `docs/Status_Summary.md` from `Status.md` (mutation; default: detect-only) |
| `--repo string` | Repo dir to scope the sweep/regeneration to (default: discovered from CWD) |
| `--emit` | Also drive the §11.4.45 verdict as a real constitution event |

```bash
# Cron-friendly detect-only sweep (exit non-zero on stale/missing Status.md):
scherald status-digest --repo /path/to/herald

# Regenerate the derived summary as part of the sweep:
scherald status-digest --apply
```

## §5. References

- Source: `scherald/cmd/scherald/main.go` + `digest_cmds.go`.
- Branding: `commons/branding.go` (flavor `sc`, DefaultPort=0).
- Trackers swept: `docs/Status.md`, `docs/Issues.md`, `docs/Fixed.md`; derived: `docs/Status_Summary.md`.
- Constitution binding: HRD-025 §11.4.45 (status-sweep).
- Companion flavor guides: `docs/guides/{PHERALD,SHERALD,CHERALD,BHERALD,RHERALD,IHERALD,QAHERALD}.md`.

## Sources verified

**Verified 2026-05-30:** internal doc — no external online sources. Every subcommand, flag, default, and behaviour claim above was derived by running the built `scherald` binary (`scherald --help`; `scherald status-digest --help`; `scherald version --json`) and reading `commons/branding.go` on 2026-05-30. No flags were invented. Re-verify whenever the `scherald` Cobra surface changes.
