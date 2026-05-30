<div align="center">

<img src="../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Herald — `cherald` Flavor Guide (Operator)

| Field | Value |
|---|---|
| Revision | 1 |
| Created | 2026-05-30 |
| Last modified | 2026-05-30 |
| Status | active |
| Status summary | Nano-detail operator reference for `cherald` (Constitution Herald) — the policy-evaluator flavor. Documents `serve` (the `/v1/compliance` HTTP plane) and the full §11.4 compliance-check command catalogue (`creds-scan`, `docs-sync`, `composite-gate`, `export`, `catalogue-check`, `spec-version-check`, `readme-sync`, `fixed-align`, `fixed-summary-sync`, `script-docs-check`, `submanifest-verify`). ANTI-BLUFF: derived from the built `cherald` binary (`cherald --help`, per-subcommand `--help`, `version --json`) + `commons/branding.go`. Most checks are DETECT-only and exit non-zero on breach; the mutating ones are explicitly flagged (`--apply`). |
| Issues | (none specific to this guide) |
| Continuation | Bump as new §11.4 doc-composite checks are added to the catalogue. |

## Table of contents

- [§1. What `cherald` is](#1-what-cherald-is)
- [§2. The subcommand surface](#2-the-subcommand-surface)
- [§3. `version`](#3-version)
- [§4. `serve` — the compliance HTTP plane](#4-serve--the-compliance-http-plane)
- [§5. The compliance-check commands](#5-the-compliance-check-commands)
- [§6. References](#6-references)

---

## §1. What `cherald` is

`cherald` is **Constitution Herald** — flavor key `c`, prefix `CHR`, default serving port **24792**. Per `commons/branding.go` its mission is "Policy evaluator + creds scan + docs sync + composite gate". It is the flavor that classifies a repository's state against the inherited Helix Constitution rules: each subcommand observes the tree (or a PR/diff), builds the relevant §-Subject, classifies it through an HRD-019 binding, and **exits non-zero on a FAIL/breach verdict** so a CI gate can key off the exit code. Almost everything is **DETECT-only** — `cherald` mutates the tree only where an explicit `--apply` flag is passed.

Build it:

```bash
go build -o /tmp/cherald ./cherald/cmd/cherald
```

## §2. The subcommand surface

Verbatim from `cherald --help`:

| Subcommand | What it does | Mutates? |
|---|---|---|
| `serve` | Start the Constitution Herald HTTP server (`/v1/compliance`) | n/a |
| `creds-scan` | Scan for leaked-credential patterns; REDACT every finding (§16.2 → §11.4.10) | no |
| `docs-sync` | Check tracked `.md` docs carry §11.4.61 metadata + ToC (`--apply` regenerates siblings) | only with `--apply` |
| `composite-gate` | CM-DOCS-COMPOSITE-SYNC: aggregate the §11.4.60 doc-composite checks (detect-only) | no |
| `export` | (Re)generate md→html/pdf/docx siblings via the export script (§11.4.65) | yes |
| `catalogue-check` | Scan a PR description / diff for the §11.4.74 Catalogue-Check line | no |
| `spec-version-check` | Audit a spec doc's Revision header vs its git-modified state (§11.4.73) | no |
| `readme-sync` | Check README doc-link rows are in sync with on-disk docs (§11.4.59) | no |
| `fixed-align` | Reconcile `docs/Issues.md` ↔ `docs/Fixed.md` for drift (detect-only; §11.4.53) | no |
| `fixed-summary-sync` | Check `docs/Fixed_Summary.md` parity vs `docs/Fixed.md` (§11.4.53) | no |
| `script-docs-check` | Audit that every `scripts/*.sh` carries a leading header docstring (§11.4.62 → §11.4.60) | no |
| `submanifest-verify` | Verify the §11.4.31 Submodule-Dependency-Manifest is present + well-formed | no |
| `version` | Print Constitution Herald version + build info | n/a |
| `completion` | Generate shell autocompletion (Cobra built-in) | n/a |

Every classifying subcommand accepts `--emit` to also drive its verdict as a real constitution event, and (where it scopes a repo) `--repo string` (default: discovered from CWD).

## §3. `version`

```bash
$ cherald version --json
{"arch":"arm64","binary":"cherald","build_time":"unknown","commit":"unknown","flavor":"c","go_version":"go1.26.2","os":"darwin","version":"0.0.0-dev"}
```

## §4. `serve` — the compliance HTTP plane

`cherald serve` starts the Constitution Herald HTTP server. Per `docs/INTEGRATION.md` §10 it exposes `GET /v1/compliance`, returning paginated `constitution_state` rows (the audit trail of every policy decision). Default bind port 24792.

```bash
cherald serve --http-port 24792
```

## §5. The compliance-check commands

### `creds-scan`

Scans `--path` (default `--repo`) for leaked-credential patterns — AWS access keys (`AKIA…`), `AWS_SECRET` assignments, PEM private-key blocks, `password=` / `token=` assignments, Telegram bot tokens. **SECURITY:** every finding is reported with the secret value **REDACTED** — the actual secret is never printed. Classifies against §11.4.10 (a hit ⇒ FAIL). Exits non-zero on any hit. DETECT-only (never edits the scanned tree).

| Flag | Meaning |
|---|---|
| `--path string` | Explicit path to scan (file or dir); overrides `--repo` |
| `--repo string` | Repo dir to scan (default: discovered from CWD) |

```bash
cherald creds-scan --path ./src || echo "LEAK DETECTED"
```

### `docs-sync [<md>...]`

Checks the supplied (or all tracked) Markdown docs carry the required §11.4.61 metadata block (a Revision table row) + a ToC heading; exits non-zero when any doc is missing them. DETECT-only by default; **`--apply` additionally regenerates the HTML/PDF/DOCX siblings** via the resolved export script.

| Flag | Meaning |
|---|---|
| `--apply` | Regenerate HTML/PDF/DOCX siblings (mutation; default: detect-only) |
| `--script string` | Override the export-script path (default: `scripts/export_docs.sh`; test seam) |

### `composite-gate`

The canonical CM-DOCS-COMPOSITE-SYNC entrypoint: aggregates three checks under one §11.4.60 verdict — (1) every tracked `.md` carries §11.4.61 metadata + ToC, (2) `README.md` doc-links all resolve on disk, (3) `docs/Fixed_Summary.md` is in parity with `docs/Fixed.md`. DETECT-only; exits non-zero when any constituent check is in breach.

### `export [<md>...]`

Wraps the canonical doc-export script to regenerate HTML/PDF/DOCX siblings of the supplied docs (or every tracked doc when none given); exits non-zero when the export fails. The script is resolved from `--script` or `scripts/export_docs.sh` under `--repo` (§11.4.74 catalogue-first).

### `catalogue-check [<pr-ref>]`

Observes a PR description (`--pr-body`, inline or `@file`) or a changed-files/diff list (`--diff-file`) and verifies it carries a `Catalogue-Check:` line (the §11.4.74 attestation). A trivial (empty) change is compliant; a non-trivial change with no line is a miss. Exits non-zero on a miss.

| Flag | Meaning |
|---|---|
| `--pr-body string` | PR description (literal text, or `@path`) |
| `--diff-file string` | Path to a changed-files/diff list to scan instead of `--pr-body` |

### `spec-version-check`

Observes a spec doc (`--spec`, default `docs/specs/mvp/specification.V3.md`): whether its content was modified in the working tree without its `Revision` header bumped. Modified-but-unbumped ⇒ §11.4.73 drift ⇒ FAIL. DETECT-only; exits non-zero on drift.

| Flag | Meaning |
|---|---|
| `--spec string` | Spec doc path relative to `--repo` |
| `--modified` | Force-treat the content as modified (test seam / non-git checkout) |

### The remaining detect-only checks

- `readme-sync` — checks README doc-link rows are in sync with on-disk docs (§11.4.59).
- `fixed-align` — reconciles `docs/Issues.md` ↔ `docs/Fixed.md` for drift (§11.4.53).
- `fixed-summary-sync` — checks `docs/Fixed_Summary.md` parity vs `docs/Fixed.md` (§11.4.53).
- `script-docs-check` — audits that every `scripts/*.sh` carries a leading header docstring (§11.4.62 → §11.4.60).
- `submanifest-verify` — verifies the §11.4.31 Submodule-Dependency-Manifest is present + well-formed.

Each shares the `--repo` / `--emit` flag shape; run `cherald <command> --help` for the precise set.

## §6. References

- Source: `cherald/cmd/cherald/main.go` + `docops_cmds.go`.
- Branding: `commons/branding.go` (flavor `c`).
- Integration: `docs/INTEGRATION.md` §10 (`GET /v1/compliance`).
- Constitution bindings: HRD-019 §11.4.10 / §11.4.53 / §11.4.59 / §11.4.60 / §11.4.61 / §11.4.62 / §11.4.65 / §11.4.73 / §11.4.74 / §11.4.31.
- Companion flavor guides: `docs/guides/{PHERALD,SHERALD,BHERALD,RHERALD,IHERALD,SCHERALD,QAHERALD}.md`.

## Sources verified

**Verified 2026-05-30:** internal doc — no external online sources. Every subcommand, flag, default, and detect-vs-mutate claim above was derived by running the built `cherald` binary (`cherald --help`; `cherald serve|creds-scan|docs-sync|composite-gate|export|catalogue-check|spec-version-check --help`; `cherald version --json`) and reading `commons/branding.go` on 2026-05-30. No flags were invented. Re-verify whenever the `cherald` Cobra surface changes.
