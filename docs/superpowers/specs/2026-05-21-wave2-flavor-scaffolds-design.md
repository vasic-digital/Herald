<div align="center">

<img src="../../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Wave 2 — Flavor Scaffolds Design

| Field | Value |
|---|---|
| Revision | 1 |
| Created | 2026-05-21 |
| Last modified | 2026-05-21 |
| Status | active |
| Status summary | Wave 2 of the master roadmap — 6 new flavor binaries (sherald, cherald, bherald, rherald, iherald, scherald) + pherald refactor. Shared CLI scaffold at `commons/cli/`. Three serving flavors (cherald + sherald + iherald). 28 §43 command stubs distributed across the 7 flavors. e2e_bluff_hunt invariants grow from 18 → ~33. Spec V3 r7 → r8 captures the design decisions. |
| Issues | none |
| Issues summary | — |
| Fixed | (n/a — design doc) |
| Continuation | After approval + spec update, invoke `superpowers:writing-plans` to author the Wave 2 implementation plan, then `superpowers:subagent-driven-development` per Universal §11.4.70 to dispatch task subagents. |

## Constitutional anchors

- **§107 end-user-usability covenant** — every Wave 2 flavor binary's "done" is observable in `e2e_bluff_hunt.sh` (binary builds + version returns JSON + serve binds + healthz 200 + stub returns 501 with HRD pointer). Compile-only PASS is forbidden.
- **§11.4.70 subagent-driven default** — implementation proceeds via `superpowers:subagent-driven-development` (one subagent per task; spec + code review per task).
- **§11.4.74 catalogue-check** — before writing `commons/cli/`, search `vasic-digital` + `HelixDevelopment` for an existing Cobra-scaffold module. Disposition recorded in the implementation plan.
- **§11.4.73 spec versioning** — Wave 2 design changes ARE spec changes; V3 r7 → r8 captures them in the same commit cycle.
- **§3 + §11.4.79** — each new flavor is an own-org submodule conceptually (lives under Herald's tree); included in CodeGraph index per the recently-landed mandate.

## Table of contents

- [Section 1 — Repo + module layout](#section-1--repo--module-layout)
- [Section 2 — Per-flavor identity, ports, serve surface, §43 stub mapping](#section-2--per-flavor-identity-ports-serve-surface-43-stub-mapping)
- [Section 3 — `commons/cli/` package API surface](#section-3--commonscli-package-api-surface)
- [Section 4 — e2e_bluff_hunt invariants (E19-E33)](#section-4--e2e_bluff_hunt-invariants-e19-e33)
- [Section 5 — §107 evidence contract per flavor](#section-5--107-evidence-contract-per-flavor)
- [Section 6 — Test + build strategy](#section-6--test--build-strategy)
- [Spec impact (V3 r7 → r8)](#spec-impact-v3-r7--r8)
- [Implementation phases](#implementation-phases)
- [Open questions deferred to implementation](#open-questions-deferred-to-implementation)

## Section 1 — Repo + module layout

```
Herald/
├── commons/                         (existing — extend with cli/ package)
│   └── cli/                         NEW — shared scaffold:
│       ├── root.go                  NewRootCmd(branding, opts) — Cobra root
│       ├── version.go               VersionCmd(branding) — JSON build-info
│       ├── serve.go                 ServeCmd(opts) — Gin server with healthz/readyz/metrics
│       ├── routes.go                Route + HealthzHandler + ReadyzHandler + MetricsHandler
│       ├── stubs.go                 StubCmd(name, hrd, description) — 501-style CLI
│       └── cli_test.go              unit tests (table-driven)
├── pherald/                         (existing — REFACTORED to use commons/cli)
├── sherald/                         NEW — System Herald (serving daemon)
│   ├── go.mod  · cmd/sherald/main.go (~30 LOC: brand + import flavor stubs)
│   └── internal/stubs/stubs.go      5 §43 stubs
├── cherald/                         NEW — Constitution Herald (serving)
│   ├── go.mod  · cmd/cherald/main.go
│   └── internal/stubs/stubs.go      11 §43 stubs
│   └── internal/http/routes.go      /v1/compliance 501 stub (→ HRD-028)
├── bherald/                         NEW — Build Herald (CLI-only)
│   ├── go.mod  · cmd/bherald/main.go
│   └── internal/stubs/stubs.go      3 §43 stubs
├── rherald/                         NEW — Release Herald (CLI-only)
├── iherald/                         NEW — Incident Herald (serving)
│   ├── go.mod  · cmd/iherald/main.go
│   └── internal/http/routes.go      /v1/webhooks/page 501 stub (→ HRD-024)
└── scherald/                        NEW — Scheduled-audit Herald (CLI-only)
```

**Module count**: `go.work` grows 7 → 14 (+commons unchanged + 6 new flavors; pherald refactor stays in pherald/).

**Layout rules**:
- `cmd/<flavor>herald/main.go` is ~30 LOC: declare branding, build root via `cli.NewRootCmd(branding)`, register stubs, optionally call `cli.ServeCmd(opts)`.
- `internal/stubs/stubs.go` registers every §43 command targeted at the flavor as a `cli.StubCmd(...)`.
- `internal/http/routes.go` (only for serving flavors) declares additional 501-stub routes per flavor.

## Section 2 — Per-flavor identity, ports, serve surface, §43 stub mapping

| Flavor | Mission | Serves? | Default port | §43 stubs (count) |
|---|---|---|---|---|
| **pherald** | Project Herald — multi-mirror push + submodule propagation + project bindings | ✓ | 24791 | 6 (029 commit-push, 030 submodule-propagate, 043 install-upstreams, 044 fetch-guard, 049 reopen, 053 pre-push) |
| **sherald** | System Herald — destructive-op intercept + force-push gate + mem-budget watcher | ✓ daemon | 24793 | 5 (033 destructive-guard, 034 backup-snapshot, 040 constitution-pull, 046 force-push-gate, 056 mem-budget-watch) |
| **cherald** | Constitution Herald — policy evaluator + creds scan + docs sync + composite gate | ✓ | 24792 | 11 (036 creds-scan, 037 docs-sync, 038 script-docs-check, 039 fixed-align, 042 submanifest-verify, 048 fixed-summary-sync, 050 readme-sync, 051 composite-gate, 052 export, 054 spec-version-check, 055 catalogue-check) |
| **bherald** | Build Herald — CI/test bindings + test-tier verifier + evidence captureer | ✗ | n/a | 3 (035 evidence-capture, 041 test-tier-verify, 045 gate-retest — *045 owned by rherald; bherald gets `gate-retest` as cross-flavor alias*) |
| **rherald** | Release Herald — tag mirroring + changelog + installable-asset evidence | ✗ | n/a | 3 (031 tag-mirror, 032 changelog-generate, 045 gate-retest) |
| **iherald** | Incident Herald — credential-leak page-out + operator-blocked escalation | ✓ paging webhooks | 24794 | 0 (HRD-024 is §42 binding; its operator-visible surface is `/v1/webhooks/page` on the serve port) |
| **scherald** | Scheduled-audit Herald — periodic Status.md sweep + compliance digest | ✗ cron-driven | n/a | 1 (047 status-digest) |

**Stub total**: 28 §43 stubs + 1 cross-flavor alias (045 shared between bherald + rherald) = 29 stub registrations. Matches §43 catalogue (HRD-029..HRD-056 = 28 unique HRDs).

**Port allocation**: `2479X` block. Reserved: pherald 24791, cherald 24792, sherald 24793, iherald 24794. Free: 24795..24799 for future flavors.

### Serve-surface routes per serving flavor

Common base (provided by `commons/cli/routes.go`):
- `GET /v1/healthz` — 200 with `{status:"ok",build:{version,go_version}}`
- `GET /v1/readyz` — 200 with `{status:"ready"}`
- `GET /metrics` — Prometheus text + `<flavor>_build_info` gauge

Flavor-specific routes (501 stubs in Wave 2):

| Flavor | Additional routes | Target HRD |
|---|---|---|
| pherald | `POST /v1/events` (existing 501) | HRD-016 |
| cherald | `GET /v1/compliance` | HRD-028 |
| sherald | `GET /v1/safety_state` (daemon status — open events, current mem%, last destructive-op log) | HRD-NNN follow-up (will track) |
| iherald | `POST /v1/webhooks/page` (PagerDuty/Opsgenie-compatible inbound) | HRD-024 |

### Branding (per spec §11.0 + §8.2)

Each flavor's `main.go` constructs a `commons.Branding{}` instance:

```go
commons.Branding{
    Flavor:      "sherald",
    Prefix:      "s",                       // §8.2 per-flavor 3-letter prefix anchor
    DisplayName: "System Herald",
    DefaultPort: 24793,
    Mission:     "Host safety + destructive-op intercept + mem-budget watcher",
}
```

The Branding struct already exists in `commons/types.go` (HRD-009). `commons/cli.NewRootCmd(branding)` consumes Branding to populate Cobra's Use/Short/Long/Version.

## Section 3 — `commons/cli/` package API surface

```go
// Package cli is the shared scaffold for every Herald flavor binary.
// Per Universal §11.4.74 catalogue-check: extends nothing in the
// vasic-digital / HelixDevelopment orgs (no-match → vendor as
// Herald-internal package).
package cli

// NewRootCmd returns the top-level Cobra command for a flavor. The
// branding drives Use / Short / Long / Version. opts are functional
// options for callers that need to override defaults.
func NewRootCmd(branding commons.Branding, opts ...RootOpt) *cobra.Command

// VersionCmd is the `<flavor>herald version --json` subcommand. Returns
// canonical JSON with: binary, flavor, version, go_version, os, arch,
// commit_sha (from -ldflags), build_time. §107 anti-bluff: an empty
// version field is explicitly rejected by the e2e probe.
func VersionCmd(branding commons.Branding) *cobra.Command

// ServeCmd is the `<flavor>herald serve` subcommand for serving flavors.
// Builds a Gin engine with healthz + readyz + metrics + every flavor-
// specific route, binds on opts.Port (default = branding.DefaultPort),
// graceful-shutdown on SIGTERM.
func ServeCmd(opts ServeOpts) *cobra.Command

type ServeOpts struct {
    Branding commons.Branding
    Routes   []Route               // flavor-specific routes appended after base
    Logger   *logrus.Logger        // optional; defaults to a no-op logger
}

// Route declares one HTTP route. Wave 2 stubs return 501 with HRD pointer.
type Route struct {
    Method      string                // "GET", "POST", ...
    Path        string                // "/v1/compliance"
    Handler     gin.HandlerFunc
    Description string                // operator-readable summary
    HRD         string                // "HRD-028" for 501 stubs; "" for live routes
}

// StubCmd returns a Cobra subcommand that always returns 501-style error
// with the HRD pointer. Used by every flavor's internal/stubs/ to register
// §43 commands not yet implemented.
func StubCmd(name, hrd, description string) *cobra.Command

// HealthzHandler / ReadyzHandler / MetricsHandler are exposed so flavor-
// specific routes can compose them when needed.
func HealthzHandler(branding commons.Branding) gin.HandlerFunc
func ReadyzHandler(branding commons.Branding) gin.HandlerFunc
func MetricsHandler(branding commons.Branding) gin.HandlerFunc
```

**Sub-package** (only if needed): `commons/cli/internal/build/` for build-info ldflags wiring (commit SHA + build time). Otherwise build info lives in `commons/cli/version.go`.

**Dependencies**: `commons/cli/` imports `commons/` (for Branding + types) + `github.com/spf13/cobra` + `github.com/gin-gonic/gin` + `github.com/sirupsen/logrus`. All already in `pherald/go.mod`; the refactor moves them into `commons/go.mod`.

## Section 4 — e2e_bluff_hunt invariants (E19-E33)

Wave 2 adds **15 new invariants** to `scripts/e2e_bluff_hunt.sh`. Pre-Wave-2: 18 PASS. Post-Wave-2: **33 PASS** (with creds + container runtime), or 18 PASS + 15 SKIP cleanly.

| Invariant | Probe | Target |
|---|---|---|
| **E19** | `sherald version --json` returns valid JSON with flavor=sherald + non-empty version/go_version | flavor build |
| **E20** | `cherald version --json` | flavor build |
| **E21** | `bherald version --json` | flavor build |
| **E22** | `rherald version --json` | flavor build |
| **E23** | `iherald version --json` | flavor build |
| **E24** | `scherald version --json` | flavor build |
| **E25** | `sherald serve` binds on :24793 + /v1/healthz returns 200 + status:ok + version | sherald serve |
| **E26** | `cherald serve` binds on :24792 + /v1/healthz returns 200 | cherald serve |
| **E27** | `iherald serve` binds on :24794 + /v1/healthz returns 200 | iherald serve |
| **E28** | cherald `GET /v1/compliance` returns 501 with HRD-028 pointer | cherald stub route |
| **E29** | sherald `GET /v1/safety_state` returns 501 with HRD-NNN pointer | sherald stub route |
| **E30** | iherald `POST /v1/webhooks/page` returns 501 with HRD-024 pointer | iherald stub route |
| **E31** | One representative §43 stub per flavor (e.g. `sherald destructive-guard` returns exit≠0 with "HRD-033" in stderr) | stub CLI exec |
| **E32** | All 3 serving flavors graceful-shutdown on SIGTERM (exit 0) | serve teardown |
| **E33** | pherald refactor regression — E1..E13 still PASS on the refactored binary | pherald continuity |

**Anti-bluff posture**: each E19-E24 asserts non-empty `version` + `go_version` fields in the JSON. A `version: ""` PASS is explicitly rejected. E28-E30 assert the response body contains the literal `HRD-NNN` string (not just status 501). E31 asserts exit code ≠ 0 AND stderr contains the HRD reference (catches a stub that exits 0 with "not implemented" silently).

## Section 5 — §107 evidence contract per flavor

For each new flavor binary, the §107 PASS contract:

1. **Binary builds** — `go build ./<flavor>/...` produces an executable, captured via `file <binary>` MIME check.
2. **`version --json` returns canonical JSON** — parsed by python3 in the e2e probe, required fields asserted non-empty.
3. **`<flavor>` (no args) prints usage + exits 0** — proves Cobra root wires up correctly.
4. **For serving flavors**:
   - `serve` binds within 10s of invocation; subsequent `GET /v1/healthz` returns 200.
   - `GET /v1/readyz` returns 200 with status:ready.
   - `GET /metrics` returns text/plain with `<flavor>_build_info` gauge.
   - Flavor-specific 501 routes return body containing the literal HRD-NNN.
   - SIGTERM → graceful shutdown → exit 0 within 5s.
5. **Stub commands return 501-style error** — one representative stub per flavor exercised; exit code ≠ 0; stderr contains HRD-NNN reference.

**For the pherald refactor**: all of E1-E13 (15 invariants from Plan 1 era) MUST stay green. The refactor introduces NO new e2e invariants — just changes the internals.

## Section 6 — Test + build strategy

**Unit tests** (default `go test`):
- `commons/cli/cli_test.go` — table-driven tests for `StubCmd` (exit code + stderr containment), `VersionCmd` (JSON shape), Route registration, branding plumbing.
- Per-flavor `cmd/<flavor>herald/main_test.go` — minimal smoke (root command builds, has expected subcommands).
- Per-flavor `internal/stubs/stubs_test.go` — verifies the expected §43 commands are registered with the right HRDs.

**Integration tests** (`//go:build integration`):
- Per-serving-flavor: boot the binary as a subprocess, hit endpoints, verify response bodies. Same pattern as pherald's existing E7-E12.
- Cross-flavor smoke: run `<flavor>herald version --json` for each of 6 flavors, verify exits + non-empty fields.

**Build verification**:
- `go build ./...` across all 14 modules — must be clean before committing.
- `gofmt -l` must be empty across all modified packages.
- `go vet` must be clean.

**Anti-bluff §1.1 paired mutations** (new gates):
- Strip the HRD pointer from one stub's error message → e2e_bluff_hunt E31 MUST FAIL.
- Make `version --json` return an empty version string → E19 MUST FAIL.
- Make sherald's serve bind on a random port instead of 24793 → E25 MUST FAIL (with explicit port mismatch).

Each new gate gets its mutation test in `tests/test_wave2_mutation_meta.sh` per §1.1.

## Spec impact (V3 r7 → r8)

Wave 2 changes to capture in spec V3 r8:

1. **§18 Flavors** — confirm 3-serving / 4-CLI-only split. Document default ports 24791..24794. Document the §43 stub distribution per flavor.
2. **§41 REST API surface** — add cherald `/v1/compliance` + sherald `/v1/safety_state` + iherald `/v1/webhooks/page` to the route catalogue (501 stubs in Wave 2; live in Waves 3/4).
3. **§44 Foundation implementation contract** — extend with a §44.M Wave 2 milestone section capturing the 6-flavor + pherald-refactor scope + the §107 evidence contract.
4. **§3.5 Branding + per-flavor identity** — formalize the per-flavor Branding instance pattern (Flavor, Prefix, DisplayName, DefaultPort, Mission).
5. **§11.4.74 Catalogue-Check** — document the `commons/cli/` no-match disposition in the §11.4.74 catalogue-checks subdirectory.

All five edits are mandatory per §11.4.73 spec-change rule.

## Implementation phases

Per the master roadmap + this design:

1. **Phase 2a** — `commons/cli/` package + unit tests + Catalogue-Check doc (1 day).
2. **Phase 2b** — Refactor pherald to consume `commons/cli/` + verify E1-E13 still PASS (1 day).
3. **Phase 2c** — Scaffold 6 new flavors in parallel (1 day each, can run in 1-3 days with parallel subagents).
4. **Phase 2d** — Add E19-E33 invariants to `e2e_bluff_hunt.sh` + paired §1.1 mutation tests (half day).
5. **Phase 2e** — Update spec V3 r7 → r8 + regen siblings (half day).
6. **Phase 2f** — Atomic Issues→Fixed for the flavor-scaffold sub-HRDs (one per flavor — open HRD-092..097) + multi-mirror push (half day).

Total: 5-7 days with parallel subagents.

## Open questions deferred to implementation

These are NOT resolved in this design; the implementation plan resolves them.

1. **Exact `commons.Branding` struct fields** — the existing struct from HRD-009 may not have `DefaultPort` or `Mission`. The implementation plan checks the current shape and either adds fields or carries Wave-2-specific fields in a `cli.FlavorMeta` wrapper.
2. **Whether `commons/cli/` lives at `commons/cli/` or `commons/internal/cli/`** — the latter prevents external Herald-consumer projects from importing it. Default: `commons/cli/` (exported) because the same scaffold pattern may be useful to flavors built by future Helix-stack consumers.
3. **sherald `/v1/safety_state` HRD assignment** — currently TBD. The plan opens HRD-098 (or appropriate number) for the live implementation of that route.
4. **Per-flavor smoke test pattern** — Cobra's `cmd.Execute()` from inside a `go test` requires capturing stdout/stderr via `cmd.SetOut()` / `cmd.SetErr()`. Plan documents the exact pattern.

## Next step

After this design doc is committed + pushed:
1. Spec V3 r7 → r8 update lands in the same commit cycle per §11.4.73.
2. `superpowers:writing-plans` skill is invoked to author the Wave 2 implementation plan with bite-sized tasks.
3. `superpowers:subagent-driven-development` dispatches task subagents per Universal §11.4.70.
