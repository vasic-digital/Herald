<div align="center">

<img src="../../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# HRD-081 — podman-compose runtime detection fix (§107.x evidence)

| Field | Value |
|---|---|
| HRD | HRD-081 |
| Closed | 2026-06-02 |
| Created-By | Claude |
| Assigned-To | @milos85vasic |
| Upstream commit | `222fb90` (`digital.vasic.containers/pkg/compose`) |
| Herald pointer | `af48c97` (containers submodule bump) |
| Mandate | §11.4.76 — extend the upstream submodule, never reimplement |

## The bug

The shared `digital.vasic.containers/pkg/compose` package was written against the
Docker Compose CLI and made two podman-compose-incompatible assumptions:

1. **`--wait` forwarded unconditionally.** `compose.WithWait(true)` appended the
   `--wait` flag to the `up` invocation. `podman-compose` does not accept `--wait`
   the way `docker compose up --wait` does, so the boot path errored (or required the
   caller to drop `WithWait`, which Herald's `commons_infra/boot.go` had worked around
   by dropping `WithWait` and TCP-probing instead).

2. **`Status()` used a Docker-style ps template.** The parser invoked
   `ps --format {{.Name}}` (Docker's Go-template `ps` output). Run against
   `podman ps` that template errored / produced nothing parseable — the field name
   differs (`Names` vs `Name`) and the template engine is not honored the same way —
   so `Status()` returned **0 services even when the containers were plainly visible
   to `podman` directly**. A caller polling `Status()` for readiness would never see
   the running service.

## The fix (upstream `222fb90`)

- **Runtime classification.** The compose layer now detects whether the active
  backend is `podman-compose` vs `docker compose` and emits runtime-appropriate
  flags: it **omits `--wait` for podman-compose** and falls back to **host-side
  healthcheck polling** so `WithWait(true)` semantics hold across both backends.
- **`Status()` ps JSON parser.** Replaced the brittle Docker `{{.Name}}` Go-template
  path with a `ps --format json` parser that reads the structured podman output and
  correctly enumerates running services.

Per §11.4.76 the fix lands **in the upstream submodule** (`vasic-digital/containers`)
and Herald simply bumps the submodule pointer (`af48c97`) — no reimplementation in
Herald.

## Deterministic tests (`containers/pkg/compose/podman_compose_test.go`)

- **argv-capture `--wait` matrix** — asserts the constructed `up` argv includes
  `--wait` for docker compose and **excludes** it for podman-compose.
- **ps JSON parser vs real fixture** — feeds a captured real `podman ps --format json`
  fixture and asserts the parsed service list is non-empty / correct.
- **paired negative `TestParsePodmanStatusJSON_NegativeOldParserMissesServices`** —
  proves the OLD docker-style template path would have parsed **0 services** from the
  same fixture, locking in that the fix is load-bearing (not a no-op).

## Live verification (executed during implementation, then torn down)

Run against a real local stack: **podman-compose 1.5.0 / podman 5.8.2**.

- Old code path: `Status()` parsed **0 services** from the running stack (the bug
  reproduced live — containers were up under `podman` but invisible to the parser).
- New JSON path: `Status()` parsed **1 running service** with port mapping
  `0.0.0.0:18080->80/tcp`, matching what `podman ps` showed directly.

The live stack was a throwaway brought up solely for this verification and **torn
down after the run** — no persistent container or host state remains.

## Cross-references

- §11.4.76 (catalogue-first / extend-upstream, never reimplement)
- Upstream: `digital.vasic.containers/pkg/compose` @ `222fb90`
- Herald submodule pointer bump: `af48c97`
- Tests: `containers/pkg/compose/podman_compose_test.go`
