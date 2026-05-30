<div align="center">

<img src="../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Herald — `sherald` Flavor Guide (Operator)

| Field | Value |
|---|---|
| Revision | 1 |
| Created | 2026-05-30 |
| Last modified | 2026-05-30 |
| Status | active |
| Status summary | Nano-detail operator reference for `sherald` (System Herald) — the host-safety flavor. Documents `serve` (the `/v1/safety_state` HTTP plane), the §9/§11.4/§12.6 pre-flight guard commands (`destructive-guard`, `force-push-gate`, `mem-budget-watch`, `backup-snapshot`, `constitution-pull`), and `version`. ANTI-BLUFF: every claim derived from running the built `sherald` binary (`sherald --help`, `sherald <sub> --help`, `sherald version --json`) and `commons/branding.go`. The guard commands NEVER execute the destructive op themselves — they classify + exit non-zero to BLOCK; documented as such below. |
| Issues | (none specific to this guide) |
| Continuation | Bump when additional §43 sysops bindings land or `/v1/safety_state` gains fields. |

## Table of contents

- [§1. What `sherald` is](#1-what-sherald-is)
- [§2. The subcommand surface](#2-the-subcommand-surface)
- [§3. `version`](#3-version)
- [§4. `serve` — the safety-state HTTP plane](#4-serve--the-safety-state-http-plane)
- [§5. The host-safety guard commands](#5-the-host-safety-guard-commands)
- [§6. References](#6-references)

---

## §1. What `sherald` is

`sherald` is **System Herald** — flavor key `s`, prefix `SHR`, default serving port **24793**. Per `commons/branding.go` its mission is "Host-level system events fan-out + safety-state guards". It is the flavor a host-management wrapper consults before any destructive operation: each guard subcommand is a **pre-flight gate** that classifies the requested op through the HRD-020 constitution binding and exits non-zero to BLOCK it. Per §12/§107 host-safety, `sherald` never performs the destructive op itself — it only advises the operator's wrapper.

Build it:

```bash
go build -o /tmp/sherald ./sherald/cmd/sherald
```

## §2. The subcommand surface

Verbatim from `sherald --help`:

| Subcommand | What it does |
|---|---|
| `serve` | Start the System Herald HTTP server (`/v1/safety_state`) |
| `destructive-guard` | Pre-flight gate for `rm` / `git reset --hard` / `git push --force` (§9.1) |
| `force-push-gate` | Pre-flight gate for force-push: merge-first + per-session auth (§11.4.41 / §9.2) |
| `mem-budget-watch` | Sample host memory + enforce the §12.6 60% ceiling (one-shot or `--watch` daemon) |
| `backup-snapshot` | Create a hardlinked snapshot before a destructive op (§9.3) |
| `constitution-pull` | Wrap fetch + rebase + post-pull validation gate (§11.4.26 + §11.4.32) |
| `version` | Print System Herald version + build info |
| `completion` | Generate shell autocompletion (Cobra built-in) |

## §3. `version`

```bash
$ sherald version --json
{"arch":"arm64","binary":"sherald","build_time":"unknown","commit":"unknown","flavor":"s","go_version":"go1.26.2","os":"darwin","version":"0.0.0-dev"}
```

## §4. `serve` — the safety-state HTTP plane

`sherald serve` starts the System Herald HTTP server. Per `docs/INTEGRATION.md` §10 it exposes `GET /v1/safety_state`, returning process-local counters (open events, mem%, last destructive-op log). It shares the `commons/cli` serve scaffold with the other serving flavors; the default bind port is 24793.

```bash
sherald serve --http-port 24793
```

## §5. The host-safety guard commands

Each of these is a **gate**: it inspects the requested operation, builds the relevant §-Subject, classifies it through the HRD-020 binding, and **exits non-zero on a FAIL verdict** so the operator's wrapper aborts. None of them mutate host state beyond the explicit, non-destructive action named. Every guard accepts `--emit` to additionally drive the verdict as a real constitution event.

### `destructive-guard <op>...`

Detects a destructive operation (`rm` / `git reset --hard` / `git clean` / `git push --force`) from the supplied args and whether a preceding hardlinked backup exists, then classifies + exits non-zero on FAIL (BLOCKING the op). It **never executes the op itself**.

| Flag | Meaning |
|---|---|
| `--backup-exists` | Assert a preceding hardlinked backup exists |
| `--backup-path string` | Path to a hardlinked backup snapshot (existence ⇒ backup satisfied) |
| `--repo string` | Repo dir scoping a relative `--backup-path` |

```bash
sherald destructive-guard rm -rf ./build --backup-path ./build.bak-20260530 || echo "BLOCKED"
```

### `force-push-gate <ref>`

Classifies merged-first (`behind upstream == 0`) AND per-session-authorized through the §11.4.41 binding; exits non-zero on FAIL. Never performs the force-push.

| Flag | Meaning |
|---|---|
| `--authorized` | Assert explicit per-session force-push authorization (or `HERALD_FORCE_PUSH_AUTHORIZED`) |
| `--upstream string` | Upstream ref to prove merge-first against (default `origin/main`) |
| `--fetch` | Fetch before computing merge-first (default: use already-fetched state) |
| `--repo string` | Repo dir (default: discovered from CWD) |

### `mem-budget-watch`

Samples host memory used-fraction READ-ONLY (`vm_stat`/`sysctl` on darwin, `/proc/meminfo` on linux — never allocates to test the ceiling), classifies against the §12.6 60% ceiling. One-shot by default (sample → classify → exit non-zero on breach); `--watch` loops emitting on breach-transitions until SIGINT.

| Flag | Meaning |
|---|---|
| `--watch` | Daemon mode: loop sampling, emit on breach-transition until SIGINT |
| `--interval duration` | Sample interval in `--watch` mode (default 10s) |

There is also a hidden `--used-fraction` override (or `HERALD_MEM_FRACTION` env) — the deterministic test seam to drive PASS/FAIL branches without real memory pressure.

```bash
sherald mem-budget-watch              # one-shot
sherald mem-budget-watch --watch --interval 30s
```

### `backup-snapshot <target>`

SAFE CREATE: hardlinks a snapshot of `<target>` (file or dir) at `--dest` (default `<target>.bak-<UTC-timestamp>` alongside the target) — cheap and space-free, the §9.3 "hardlinked backup before destructive op" helper. It **never deletes or overwrites** the target. Exits non-zero if the snapshot could not be created.

| Flag | Meaning |
|---|---|
| `--dest string` | Snapshot destination (default `<target>.bak-<UTC-timestamp>`) |

### `constitution-pull`

Pulls the discovered constitution submodule (fetch + rebase against `--remote`) and runs the §11.4.32 post-pull validation gate, classifying both §11.4.26 (pull ok?) and §11.4.32 (validation passed?). Prefers the canonical `constitution_pull.sh` / `find_constitution.sh` when discoverable.

| Flag | Meaning |
|---|---|
| `--remote string` | Remote to fetch + rebase against (default `origin`) |
| `--constitution-dir string` | Constitution submodule dir (default: discovered) |
| `--repo string` | Repo dir to start discovery from (default: CWD) |
| `--skip-validate` | Do not run the post-pull validation gate (records `validated=false`) |
| `--assume-validated` | Treat the post-pull validation gate as PASSED (test seam / operator override) |

## §6. References

- Source: `sherald/cmd/sherald/main.go` + `sysops_cmds.go`.
- Branding: `commons/branding.go` (flavor `s`).
- Integration: `docs/INTEGRATION.md` §10 (`GET /v1/safety_state`).
- Constitution bindings: HRD-020 §9.1 / §9.2 / §9.3 / §11.4.26 / §11.4.32 / §11.4.41 / §12.6.
- Companion flavor guides: `docs/guides/{PHERALD,CHERALD,BHERALD,RHERALD,IHERALD,SCHERALD,QAHERALD}.md`.

## Sources verified

**Verified 2026-05-30:** internal doc — no external online sources. Every subcommand, flag, default, and gate-behaviour claim above was derived by running the built `sherald` binary (`sherald --help`; `sherald serve|destructive-guard|force-push-gate|mem-budget-watch|backup-snapshot|constitution-pull --help`; `sherald version --json`) and reading `commons/branding.go` on 2026-05-30. No flags were invented. Re-verify whenever the `sherald` Cobra surface changes.
