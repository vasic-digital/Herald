<div align="center">

<img src="../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Catalogue-Check — HRD-092 `commons/cli/` shared scaffold

| Field | Value |
|---|---|
| Revision | 1 |
| Created | 2026-05-21 |
| Last modified | 2026-05-21 |
| Status | Fixed (Wave 2, r8) |
| Status summary | Catalogue-Check survey executed per Universal §11.4.74 before the Wave 2 shared CLI scaffold (`commons/cli/`) landed. Verdict: **`no-match → vendor as Herald-internal package`**. Searched `vasic-digital` + `HelixDevelopment` orgs for an existing Cobra + Gin flavor-binary scaffold that would let 7 Herald flavors share root command + version subcommand + serve subcommand + healthz/readyz/metrics handlers + 501-stub registration. No match found. |
| Issues | HRD-092 (closed via this verdict) |
| Issues summary | see `../Issues.md` (Wave 2 closure) |
| Fixed | HRD-092 (Wave 2, r8) — `commons/cli/` package landed in commit range `7e0a614..37e348e` per Wave 2 plan `docs/superpowers/plans/2026-05-21-wave2-flavor-scaffolds.md`. |
| Fixed summary | Shared scaffold (~400 LOC + 7 unit tests) lets each `cmd/<flavor>/main.go` stay at ~25–30 LOC. 7 flavor binaries (pherald, sherald, cherald, iherald, bherald, rherald, scherald) consume it. |
| Continuation | Wave 3 work consumes `cli.ServeCmd` middleware hook to add real Runner wiring per HRD-016, HRD-024, HRD-028, HRD-098. |

## Table of contents

- [§0. Purpose](#0-purpose)
- [§1. Method](#1-method)
- [§2. Capability probed](#2-capability-probed)
- [§3. Verdict + reasoning](#3-verdict-reasoning)
- [§4. As-built evidence](#4-as-built-evidence)
- [§5. Reproducibility](#5-reproducibility)

## §0. Purpose

Per Universal §11.4.74 (submodule-catalogue-first discovery), every new HRD MUST record a `Catalogue-Check: reuse | extend | no-match` verdict per capability *before* Go code lands. This file is the evidence record for **HRD-092 — `commons/cli/` shared flavor-binary scaffold** (Wave 2 Task 1–4).

## §1. Method

Surveyed via `gh repo list` + `gh api repos/<org>/<name>/contents/`:

- `gh repo list vasic-digital --limit 200` — looked for any repo whose `README.md` or `go.mod` name mentioned Cobra, CLI scaffold, flavor binary, command framework.
- `gh repo list HelixDevelopment --limit 200` — same.
- Shortlist of probed candidates: `vasic-digital/middleware` (HTTP only — no Cobra surface), `vasic-digital/config` (configuration only), `vasic-digital/observability` (metrics/tracing only), `HelixDevelopment/HelixConstitution` (rule definitions, no CLI), `HelixDevelopment/HelixAgent` (agent runtime, not a generic CLI scaffold). None expose a `NewRootCmd(branding) + VersionCmd(branding) + ServeCmd(opts) + StubCmd(...)` Cobra surface keyed on Herald's `commons.Branding` shape.

Survey ran 2026-05-21 between 09:30 and 10:00 local time, ahead of Wave 2 Task 1 implementation.

## §2. Capability probed

Wave 2's shared scaffold needs to provide, for every `<prefix>herald` flavor binary:

1. **Root Cobra command** — `cli.NewRootCmd(branding)` returns a `*cobra.Command` with `Use` = `branding.BinaryName`, `Short` = `branding.DisplayName`, `Long` = `branding.Mission`, and the standard `--config`, `--log-level`, `--log-format` persistent flags.
2. **`version` subcommand** — `cli.VersionCmd(branding)` returns a `*cobra.Command` that prints either human-readable (`pherald 1.0 · github.com/vasic-digital/Herald`) or machine-readable (`--json`) output with the full `branding` shape (Flavor, Prefix, DisplayName, DefaultPort, Mission, version, build SHA).
3. **`serve` subcommand** — `cli.ServeCmd(opts)` returns a `*cobra.Command` that builds a Gin router from `opts.Routes`, applies `opts.Middleware`, mounts `/v1/healthz` + `/v1/readyz` + `/v1/metrics`, binds on `opts.Port` (default = `branding.DefaultPort`), and traps SIGTERM for graceful shutdown.
4. **Stub-route handler** — `cli.StubRouteHandler(hrdNumber)` returns a Gin handler that always responds `501 Not Implemented` with a body pointing at the owning HRD (per §11.4.10 root-cause discipline — stubs MUST surface the HRD that will land the real implementation).
5. **Stub Cobra subcommand** — `cli.StubCmd(name, hrdNumber)` for the §43.2 catalogue commands that are scaffolded ahead of their owning HRD.

The capability is **branding-keyed** — every output (Use/Short/Long, default port, version JSON shape) is derived from `commons.Branding`. A generic Cobra scaffold from elsewhere would not consume Herald's `Branding` shape.

## §3. Verdict + reasoning

**Verdict:** `no-match → vendor as Herald-internal package commons/cli/`.

**Reasoning:**

- No external module in either surveyed org exposes a Cobra scaffold keyed on `commons.Branding`. Other Helix-stack modules (`vasic-digital/middleware`, `auth`, `observability`) are *libraries* that the scaffold *composes* — they are not themselves a CLI scaffold.
- Generic Cobra cookie-cutter generators (e.g. `cobra-cli`) emit one-shot boilerplate; they don't carry the Wave 2 contract (branding-keyed JSON version shape, branding-keyed default port, paired Gin `Routes` + `Middleware` hook for serve, HRD-pointer 501 stubs).
- Building the scaffold inside Herald keeps it tightly coupled to the `commons.Branding` shape — when Wave 3 adds new Branding fields, the scaffold updates in lockstep with no cross-repo dependency lag.
- The scaffold is small (~400 LOC + 7 unit tests). Vendoring is the right call vs. spinning up a separate `vasic-digital/cobra-scaffold` repo for a Herald-specific contract.

## §4. As-built evidence

Wave 2 (r8) landed `commons/cli/` in commit range `7e0a614..37e348e`:

- Package files: `commons/cli/root.go`, `version.go`, `serve.go`, `routes.go`, `stub.go` (~400 LOC total).
- Unit tests: 7 tests, all PASS under `-race`.
- Consumed by all 7 flavor binaries: `pherald`, `sherald`, `cherald`, `iherald`, `bherald`, `rherald`, `scherald`. Each `cmd/<flavor>/main.go` is ~25–30 LOC.
- Verified by `scripts/e2e_bluff_hunt.sh` invariants E19–E33 (33 PASS / 0 FAIL / 3 SKIP).
- Paired §1.1 mutation gate: `tests/test_wave2_mutation_meta.sh` (4/4 PASS).
- Inheritance gate: 15/15 PASS.

See `docs/specs/mvp/specification.V3.md` §44.M for the full Wave 2 milestone evidence.

## §5. Reproducibility

To re-run this catalogue-check:

```bash
gh repo list vasic-digital --limit 200 | grep -iE 'cobra|cli|scaffold|flavor'
gh repo list HelixDevelopment --limit 200 | grep -iE 'cobra|cli|scaffold|flavor'
# For each candidate:
gh api repos/<org>/<name>/contents/ --jq '.[].name'  # look for cobra.go / root.go / cli/
gh api repos/<org>/<name>/contents/go.mod --jq '.content' | base64 -d | grep cobra
```

No probe surfaced a Cobra scaffold keyed on a branding shape compatible with Herald's `commons.Branding`. Verdict unchanged: vendor as Herald-internal.
