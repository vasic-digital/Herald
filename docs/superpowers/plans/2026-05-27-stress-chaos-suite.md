<div align="center">

<img src="../../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Herald §11.4.85 Stress + Chaos Test Suite — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Long-running suites MUST run detached per §108.d / §11.4.89 (`nohup … > qa-results/<id>_<TS>.log 2>&1 &` then `disown`); the mutation gate runs **one-at-a-time, foreground, never backgrounded/concurrent** per §107.y + the 2026-05-27 incident memo.

| Field | Value |
|---|---|
| Plan ID | `2026-05-27-stress-chaos-suite` |
| Created | 2026-05-27 |
| Author | planning agent (read-only design pass) |
| Compliance gap | **GAP-3** (constitution-compliance-audit-2026-05-27): Herald currently has ZERO stress/chaos coverage; §11.4.85 / §108.a is unsatisfied for every shipped feature. |
| Canonical authority | HelixConstitution §11.4.85 (inherited per §11.4.35) → Herald `CLAUDE.md` §11.4.85 restatement → `docs/guides/HERALD_CONSTITUTION.md` §108.a (project-binding scope). |
| Host-safety authority | §12 host-session-safety + §12.6 60%-memory-ceiling (resource-exhaustion chaos MUST run inside bounded scopes that can never sign the host out). |
| HRD range | **HRD-122 … HRD-130** (9 tasks). Highest currently-assigned HRD is HRD-121 (Wave 7); HRD-150 in the wild is only an in-test example string, not an allocation. |
| e2e invariants | **E81 … E88** (next free after E80; E1..E80 are taken). |
| Evidence root | `docs/qa/<run-id>/stress_chaos/` per §108.a + §107.x. |
| Mutation gate | `tests/test_stress_chaos_mutation_meta.sh` (paired §1.1; check_quiescence + `.git/MUTATION_IN_PROGRESS` lockfile per §107.y). |
| Status | PLAN — not yet executed. |

---

## 0. Goal + non-goals

**Goal.** Land an automated, repeatable **stress** AND **chaos** test layer covering every user-visible Herald surface, such that each PASS cites a real captured-evidence artefact under `docs/qa/<run-id>/stress_chaos/` (latency histogram, error-rate-under-fault, recovery-time, OOM-kill-scope-confinement proof). Wire the PASS into `scripts/e2e_bluff_hunt.sh` as E81..E88 and protect every load-bearing concurrency/recovery property with a paired §1.1 mutation gate. This closes GAP-3 and brings Herald into §11.4.85 / §108.a compliance.

§108.a fixes the quantitative bar Herald MUST meet: **stress** = N ≥ 100 iterations or ≥ 30 s sustained, p50/p95/p99 recorded, AND/OR N ≥ 10 parallel with no deadlock / leak / data race, AND/OR boundary conditions (empty / max / off-by-one) each producing a categorised result. **Chaos** = at least one failure-injection category appropriate to the fix-class: process-death, network-fault, input-corruption, resource-exhaustion (disk-full / OOM / fd-exhaustion), or state-corruption (DB-lock-loss / partial-write / cache-invalidation). Cleanup is non-negotiable: `trap '...' EXIT` restores any corrupted `.env`, `rm`s disk-fillers, verifies killed processes restarted, and confirms no container scope outlives the run.

**Non-goals.**
- No new product feature. This is a resilience-test layer over existing surfaces (Wave 1–7).
- No live-credential dependency in CI: the Telegram + Claude-Code chaos scenarios use the EXISTING test seams (`HERALD_INBOUND_CC_FAKE=1`, telebot offline-bot, `subscribe_integration_test.go` skip-without-creds pattern) so the suite is hermetic; the live variants are SKIP-with-reason per §11.4.3 until an operator supplies creds (mirrors E63..E70 / E17 / E18).
- No `--skip-stress` / `--no-chaos` / `--happy-path-suffices` escape hatch (forbidden by §108.a).

---

## 1. Target matrix — surface → stress scenario + chaos scenario

Each row binds one user-visible Herald surface to its stress and chaos scenarios. Column 4 is the concrete kill/fault mechanism. Every row's PASS lands an artefact under `docs/qa/<run-id>/stress_chaos/<surface>/`.

| # | Surface (real path) | Stress scenario | Chaos scenario(s) | Fault mechanism | HRD |
|---|---|---|---|---|---|
| 1 | **Gin `/v1/events`** (`pherald/internal/http/events.go` → `runner.Run`) JWT-gated POST | N=12 workers × M=200 CloudEvents (≥2400 req) against a real dual-listener `RunServe`; record p50/p95/p99 latency + throughput; assert 0 deadlocks + 0 5xx on healthy path | (a) PG connection drop mid-request; (b) Redis down → assert graceful **PG-fallback idempotency** under load (the documented degrade, runner.go redisAdapter nil path); (c) oversized + corrupt TOON+JSON bodies (input-corruption); (d) JWT/HMAC auth storm (10k bad tokens) | (a) `podman/docker pause` the PG container OR kill the pooled conn; (b) start server with `HERALD_REDIS_URL` unset; (c) 8 MiB random + truncated bodies; (d) random 32-byte bearer tokens | HRD-123 |
| 2 | **Gin `/v1/compliance`** (cherald) + **`/v1/safety_state`** (sherald) GET, JWT-gated, TOON-aware | N=10 workers × M=200 GETs each flavor; p50/p95/p99 + content-negotiation correctness under load (Accept: application/toon vs json) | network-fault (PG drop) → assert flavor returns fail-loud 5xx, NOT a fabricated 200; auth storm | shared driver from row 1 retargeted at flavor ports | HRD-124 |
| 3 | **Runner 7-stage pipeline** (`pherald/internal/runner`) in-process, no HTTP | N=16 goroutines replaying the SAME idempotency key + N fresh keys concurrently; assert exactly-once dispatch (idempotency under contention — extends `nil_redis_test.go` to the concurrent case) + `-race` clean | (a) duplicate-event flood (1000× same key, 50 parallel) → assert ≤1 channel send + ≤1 evidence row; (b) PG deadlock injection (two tenants lock-ordering); (c) River queue saturation (enqueue past worker pool) | `-race`; fake PG store that returns `pgx` deadlock error code on a scripted call; bounded queue + backpressure assertion | HRD-125 |
| 4 | **`pherald listen` inbound long-poll** (`pherald/cmd/pherald/listen.go` → `inbound.Dispatcher.Handle`) | N concurrent `InboundEvent`s fed through the `runListen` stub-subscriber seam (the existing `HERALD_INBOUND_CC_FAKE=1` path) at burst rate; assert handler invoked once-per-event, no goroutine leak, clean SIGTERM under load | (a) mid-dispatch process-death (SIGKILL the listen process while events in flight → restart → assert no duplicate side-effects via PG); (b) malformed update payloads (nil Raw, missing message_id, non-UTF8) → assert `extractReplyToMessageID` degrades to replyTo=0, never panics; (c) claude_code subprocess killed mid-reply | SIGKILL via os/exec in a shell harness; table-driven corrupt `commons.InboundEvent`; fake CodeDispatcher that `panic`s / returns truncated stdout | HRD-126 |
| 5 | **claude_code dispatch** (`commons_messaging/dispatch/claude_code`) `Dispatch` / `bootstrapSession` | N parallel Dispatch calls against a fake `claude` shim binary; p50/p95/p99 of the exec round-trip; assert session-resolution is concurrency-safe (no double-bootstrap race, `-race` clean) | (a) process-death: fake `claude` that `exit 137` / SIGKILLs itself mid-write → assert dispatch returns a tagged error, not a hang; (b) timeout: fake `claude` that sleeps past `bootstrapTimeoutOrDefault` → assert `CommandContext` cancellation fires; (c) truncated `<<<HERALD-REPLY>>>` (half a marker) → assert `parseReply` errors explicitly | a committed `testdata/fake-claude*.sh` shim selected via `HERALD_CLAUDE_BIN`; context-deadline harness | HRD-127 |
| 6 | **Container-orchestrated flows** (Postgres + Redis via `containers/` submodule, used by `/v1/events` live + M2 integration) | sustained ≥30 s connection-churn against a real `--memory`-bounded PG container | resource-exhaustion: (a) disk-full on the PG volume (fill a bounded tmpfs/scratch dir, assert pherald reports a tagged write error, then `trap` cleans the filler); (b) **OOM pressure** — boot PG/Redis with a tight `--memory` cap + drive a memory-heavy query/keyspace, assert the OOM-kill is **confined to the container scope** and the host stays well under the §12.6 60% ceiling | container runtime `--memory=<cap>` (podman/docker, already the e2e boot path) + `fallocate`/`dd` disk-filler inside a bounded scratch dir with `trap rm` | HRD-128 |

**Composition note (row 4 ↔ row 1).** The `listen` inbound chaos (process-death) and the `/v1/events` chaos (PG drop) share the once-only-side-effect assertion against the same `events_processed` / `outbound_delivery_evidence` tables — both rely on the idempotency property row 3 stress-proves. Land row 3 (HRD-125) FIRST so rows 1+4 can cite it.

---

## 2. Tooling decision (per surface) + dependency policy

**Guiding principle (§11.4.74 catalogue-check + the spec "vendored SDKs as git submodule, never `go get`" convention + §104/I6 no-embedded-constitution):** prefer Go-native + already-vendored tooling; reach for an external load/fault tool ONLY where Go-native cannot reproduce the fault, and vendor it as a submodule (never `go get`'d) if it is a Go library.

| Need | Recommendation | Justification |
|---|---|---|
| HTTP load generation (rows 1,2) | **Go-native** — `testing` + `golang.org/x/sync/errgroup` + a real `cli.RunServe` listener dialed by `net/http.Client`. NO external load tool. | The dual-listener already starts in-process in `serve_test.go`; an errgroup of N clients hitting a real port gives p50/p95/p99 without a binary dependency. `errgroup` is already an indirect dep via the toolchain; if not in `go.mod` it is `golang.org/x/sync` (stdlib-adjacent, x/-repo) — acceptable as a normal Go dep, NOT a messenger SDK, so no submodule needed. Latency captured with `time.Now()` deltas → sorted slice → percentile. Avoids vegeta/hey/bombardier entirely (no new toolchain, hermetic, CI-friendly). |
| Concurrency / data-race proof (rows 3,5) | **Go-native** — `go test -race -count=1` with `sync.WaitGroup` / `errgroup` fan-out. | The race detector IS the canonical concurrency-correctness evidence; matches the existing `go test -race` build/test command in `CLAUDE.md`. |
| Process-death (rows 4,5) | **Shell-driven** Go-test orchestration — `os/exec` `SIGKILL` in a `tests/test_stress_chaos_*.sh` harness for the binary-level kill; fake-dispatcher `panic`/exit for the in-process kill. | Killing a real `pherald listen` process and asserting clean restart is a binary-level concern best driven from shell (mirrors `test_wave6_live_loop.sh`); the claude subprocess kill is reproduced by a committed `testdata/fake-claude*.sh` shim (no real `claude` needed → hermetic). |
| Network-fault / PG-drop (rows 1,2,6) | **Container runtime `pause`/`stop`** (podman/docker) for the live path; **fake adapter returning conn-reset/deadlock errors** for the hermetic path. NO toxiproxy, NO `tc`/`iptables`. | `tc`/`iptables` need root + risk host-level network state (a §12 host-safety hazard); toxiproxy is a new submodule for marginal gain. `podman pause <pg>` cleanly simulates a mid-request stall and is ALREADY the e2e PG boot path. The hermetic CI path uses a fake `pgEventsProcessedAdapter` that returns a scripted `pgconn` error — no container needed, deterministic. **Recommended: dual-path** (hermetic fake for CI gate + live container-pause variant SKIP-without-runtime). |
| Input-corruption (rows 1,4,5) | **Go-native** — table-driven corrupt payloads (oversized, truncated, non-UTF8, nil-field). | Pure data; no tooling. |
| Resource-exhaustion / OOM (row 6) | **Container `--memory=<cap>`** (podman/docker) — already the e2e boot mechanism; disk-full via `fallocate`/`dd` into a bounded scratch dir. **`systemd-run --scope -p MemoryMax=` is the preferred host-safe wrapper on Linux** where available; fall back to container `--memory` cap on macOS/podman-machine. | Confines the OOM kill to the cgroup scope, never the host (§12.6). gopsutil could *observe* host memory but is a new dep; instead read `/proc/meminfo` (Linux) or `vm_stat` (macOS) in the harness for the pre/post host-memory headroom proof. No new submodule. |
| Percentile / histogram math | **Go-native** — sort + index, emit `latency.json` (`{p50,p95,p99,max,count}`) + `latency_histogram.csv`. | Trivial; avoids a stats-lib dep. |

**New dependency verdict:** **NONE strictly required.** Everything reuses Go stdlib + `golang.org/x/sync/errgroup` (normal Go dep, not a messenger SDK) + the already-vendored `containers/` submodule + podman/docker already on the e2e path. If a future row wants HTTP/3-specific load (QUIC stream multiplexing under stress), the vendored `submodules/http3` client already exists — reuse it, do not add a new tool. **Flag if execution discovers a genuine need:** any Go library added MUST go in as a `submodules/<name>` git submodule with a `replace` directive (per `CLAUDE.md` vendored-SDK rule), NOT `go get`'d; record a "NO Go-native solution found" §11.4.8 note in the task commit if so.

---

## 3. Evidence shape — `docs/qa/<run-id>/stress_chaos/`

`<run-id>` is `HRD-122-stress-chaos-<TS>` (timestamp-prefixed, multiple runs allowed). Layout per §108.a + §107.x. Every artefact is **captured runtime output**, never a metadata-only "PASS" line.

```
docs/qa/HRD-122-stress-chaos-<TS>/stress_chaos/
  README.md                      # run manifest: host, OS, Go version, runtime (podman/docker|none),
                                 #   which rows ran live vs SKIP-with-reason, host-memory headroom pre/post
  events/                        # row 1 (HRD-123)
    latency.json                 # {p50,p95,p99,max,count,errors} for the healthy stress run
    latency_histogram.csv        # bucketed latency for a plottable artefact
    throughput.csv               # req/s over the sustained window
    categorised_errors.txt       # error taxonomy under each chaos sub-scenario (PG-drop / Redis-down / corrupt-body / auth-storm)
    redis_down_fallback.log      # proof PG-fallback idempotency held with Redis absent (run started, dedup observed)
    recovery_trace.log           # time-to-first-success after PG unpause
  compliance_safety/             # row 2 (HRD-124): latency.json + content_negotiation_under_load.txt
  runner/                        # row 3 (HRD-125)
    race_clean.log               # `go test -race` tail proving 0 data races under N=16 fan-out
    exactly_once.txt             # sends=1 evidence_rows=1 across 1000× duplicate flood
    deadlock_recovery.log        # injected-deadlock retry/abort categorised result
  listen_inbound/                # row 4 (HRD-126)
    burst_handled.txt            # events_in=N handled=N leaked_goroutines=0
    process_death_no_dup.log     # kill→restart→ zero duplicate side-effects (PG row counts pre/post)
    malformed_no_panic.txt       # table-driven corrupt-event results (each → degraded, never panic)
  claude_code/                   # row 5 (HRD-127)
    dispatch_latency.json
    subprocess_kill.log          # exit137 / SIGKILL → tagged error, no hang
    timeout_cancel.log           # context-deadline fired
    truncated_reply.txt          # half-marker → explicit parse error
  containers/                    # row 6 (HRD-128)
    oom_scope_confinement.log    # container OOM-killed; host stayed <60% (vm_stat/meminfo pre+post)
    disk_full_tagged_error.log   # write error surfaced + filler cleaned (trap proof)
    host_memory_headroom.txt     # MUST show host free memory never crossed the §12.6 ceiling
  STRESS_CHAOS_SUMMARY.md        # per-row PASS/FAIL/SKIP table + the e2e E-invariant each row anchors
```

**Anti-bluff requirement:** each row's e2e invariant (E81..E88) greps a *specific value* out of these files (e.g. E83 asserts `runner/exactly_once.txt` contains `sends=1`), not merely the file's existence. A present-but-empty artefact is a §11.4 PASS-bluff and MUST FAIL the invariant.

---

## 4. e2e_bluff_hunt integration — E81 … E88

Append to `scripts/e2e_bluff_hunt.sh` after E80. Each follows the existing PASS/FAIL/`fail_names+=(...)` + SKIP-with-reason convention (§11.4.3). The healthy-path stress invariants run hermetically in the gate; the live-container chaos variants SKIP-with-reason when no runtime is present (mirrors E13..E18).

| Invariant | Asserts (greps a real value) | Evidence anchor | Run mode |
|---|---|---|---|
| **E81** | `/v1/events` stress: `latency.json` parses + `p99` numeric + `errors=0` on healthy path; ≥2400 req counted | `events/latency.json` | hermetic (in-process listener) |
| **E82** | `/v1/events` chaos: `redis_down_fallback.log` shows dedup-held-without-Redis AND `categorised_errors.txt` has a PG-drop category | `events/*.log` | hermetic fake + SKIP live-pause variant |
| **E83** | Runner exactly-once under contention: `runner/exactly_once.txt` contains `sends=1` + `evidence_rows=1`; `race_clean.log` shows `-race` 0 races | `runner/*` | hermetic |
| **E84** | `listen` burst: `listen_inbound/burst_handled.txt` shows `handled==events_in` + `leaked_goroutines=0` | `listen_inbound/burst_handled.txt` | hermetic |
| **E85** | `listen` process-death: `process_death_no_dup.log` shows pre==post side-effect counts after kill+restart | `listen_inbound/process_death_no_dup.log` | shell-driven; SKIP if no PG |
| **E86** | claude_code chaos: `claude_code/subprocess_kill.log` shows tagged error (no hang) + `timeout_cancel.log` shows deadline fired | `claude_code/*` | hermetic (fake shim) |
| **E87** | Container OOM confinement: `containers/oom_scope_confinement.log` shows container-scope kill + `host_memory_headroom.txt` proves host stayed <60% | `containers/*` | SKIP-with-reason when no runtime |
| **E88** | Suite manifest sanity: `STRESS_CHAOS_SUMMARY.md` present, every non-SKIP row marked PASS, NO empty artefact in the tree (anti-bluff guard against present-but-empty files) | whole `stress_chaos/` tree | always |

Update the E1..E80 header comment block to E1..E88 and the "Eighty invariants" prose to "Eighty-eight". Bump the `TOTAL`/summary tally lines.

---

## 5. Paired §1.1 mutation gate — `tests/test_stress_chaos_mutation_meta.sh`

Models exactly on `tests/test_wave6.5_mutation_meta.sh`: pre-flight dirty-tree refusal → `.git/MUTATION_IN_PROGRESS` lockfile creation → `trap cleanup_all EXIT` (restore + lockfile removal) → per-mutation `assert_restored` byte-for-byte check → `check_quiescence` final scan for leaked `MUTATED for paired` markers. **Runs one-at-a-time, foreground, NEVER backgrounded/concurrent** (§107.y + the 2026-05-27 concurrent-gate-residue incident memo). The marker convention is the canonical `MUTATED for paired` / `// always pass` set from §107.y.

Each mutation removes a load-bearing concurrency/recovery guard and asserts the matching stress/chaos test FAILs (proving the test is genuinely load-bearing), then restores and asserts PASS.

| Mut | File + property removed | Mutated to | Detector (expect FAIL) |
|---|---|---|---|
| **M1** | `runner.go` redisAdapter nil-guard (`if r.client == nil { return false, nil }`) → make it `return true, nil` (claim "always fresh fast-path") | `// MUTATED for paired — fake fresh` | row-3 duplicate-flood test: exactly-once breaks → `sends>1` → FAIL |
| **M2** | Runner idempotency check short-circuit (`if rc.Duplicate { … }`) → delete the early-return | `// MUTATED for paired — skip dedup` | row-1/row-4 once-only side-effect under concurrent replay → duplicate sends → FAIL |
| **M3** | claude_code `exec.CommandContext` → `exec.Command` (drop ctx cancellation) | `// MUTATED for paired — no ctx` | row-5 timeout chaos: deadline never fires → test hangs/exceeds bound → FAIL (detector wraps in its own timeout) |
| **M4** | `extractReplyToMessageID` `default:` panic-vs-error — change `return 0, fmt.Errorf(...)` to `panic(...)` | `// MUTATED for paired — panic on bad type` | row-4 malformed-payload chaos: handler panics instead of degrading → FAIL |
| **M5** | Container OOM scope cap — in the row-6 harness, remove the `--memory=<cap>` flag (mutate the harness's own cap constant) | `# MUTATED for paired — unbounded mem` | row-6 confinement detector: host-headroom proof shows cap absent → FAIL (and host-safety guard refuses to proceed) |

Gate returns 0 ONLY when every mutation FAILs its detector, every restore is byte-clean, and `check_quiescence` finds zero leaked markers. **M5 caveat:** M5 mutates the *harness's* memory cap, not production source, and the detector MUST assert the cap-absence is *detected and refused* (it must NOT actually run an unbounded OOM scenario — see §7).

---

## 6. Task breakdown — T1 … T9 (HRD-122 … HRD-130)

Each task opens its HRD in `docs/Issues.md` at start and migrates it to `docs/Fixed.md` atomically at close (V3 §8.3 + §11.4.19). Each is independently committable. Long suites run detached (§108.d); the mutation gate runs foreground/one-at-a-time (§107.y).

- [ ] **T1 — HRD-122: Shared stress/chaos harness scaffold.** New package `tests/stresschaos/` (Go) + `tests/stress_chaos.sh` orchestrator. Provides: errgroup load-driver, percentile/histogram emitter (`latency.json` / `*.csv`), `docs/qa/<run-id>/stress_chaos/` evidence-dir creator + manifest writer, host-memory-headroom reader (`/proc/meminfo` | `vm_stat`), committed `testdata/fake-claude*.sh` shim. NO surface tests yet. Scoped: new files only.
- [ ] **T2 — HRD-123: `/v1/events` stress + chaos.** Row 1. In-process `RunServe` + N×M errgroup load → `events/latency.json`; chaos sub-scenarios (PG-drop fake, Redis-down fallback, corrupt bodies, auth storm). Depends on T1.
- [ ] **T3 — HRD-124: `/v1/compliance` + `/v1/safety_state` stress + chaos.** Row 2. Retargets T2's driver at cherald/sherald; content-negotiation-under-load check. Depends on T1, T2.
- [ ] **T4 — HRD-125: Runner exactly-once-under-contention (LAND FIRST among the dependents).** Row 3. Extends `nil_redis_test.go` to concurrent fan-out; `-race`; duplicate-flood; deadlock injection; River saturation. Depends on T1. *Rows 1+4 cite this — schedule before T2 close if possible.*
- [ ] **T5 — HRD-126: `pherald listen` inbound stress + chaos.** Row 4. Burst via `runListen` stub-subscriber seam; process-death kill+restart (shell); malformed-payload table; CC-subprocess-kill. Depends on T1, T4.
- [ ] **T6 — HRD-127: claude_code dispatch stress + chaos.** Row 5. Parallel Dispatch vs fake shim; subprocess-kill / timeout / truncated-reply. Depends on T1.
- [ ] **T7 — HRD-128: container OOM + disk-full chaos (host-safety-gated).** Row 6. `--memory`-capped PG/Redis via `containers/`; disk-filler in bounded scratch dir; host-headroom proof. SKIP-with-reason when no runtime. Depends on T1; MUST implement §7 guardrails. Run detached.
- [ ] **T8 — HRD-129: e2e_bluff_hunt E81..E88 + header bump.** Wire all eight invariants citing the T2..T7 evidence dirs; hermetic vs SKIP-live split. Depends on T2..T7.
- [ ] **T9 — HRD-130: Paired mutation gate `tests/test_stress_chaos_mutation_meta.sh` + docs.** M1..M5 per §5; operator guide stub under `docs/guides/` (+ PDF/HTML/DOCX siblings per the docs-mandatory rule); CLAUDE.md/Status/CONTINUATION r-bumps; spec V3 §-row for the suite. Depends on T2..T8. Mutation gate runs foreground/one-at-a-time.

**Ordering for parallel dispatch (§108.b zero-idle):** T1 first (blocks all). Then T4 (blocks T5) + T6 + T7 can run in parallel subagents (worktree-isolated per §107.y.4) while T2→T3 proceed. T8 + T9 serialise at the end.

---

## 7. Host-safety guardrails (§12 + §12.6 60%-memory-ceiling)

Resource-exhaustion chaos is the ONLY scenario that can endanger the host (an unbounded OOM filler could swap-storm the machine and sign the operator out). HARD rules for HRD-128 (T7) + the M5 mutation:

1. **Bounded scope only.** The OOM scenario MUST run inside a confined cgroup scope: container `--memory=<cap>` (podman/docker, the existing e2e boot path) and, on Linux where available, additionally `systemd-run --scope -p MemoryMax=<cap> -p MemorySwapMax=0`. The memory pressure is generated *inside* that scope (a heavy PG query / Redis keyspace fill), NOT on the host.
2. **Pre-flight headroom gate.** Before launching any resource-exhaustion scenario, read host memory (`/proc/meminfo` MemAvailable on Linux, `vm_stat` on macOS). If current host usage is already ≥ 50%, or the cap would push host usage past the §12.6 **60% ceiling**, the harness REFUSES to run that scenario and emits `SKIP host-safety: would risk §12.6 ceiling` (a real SKIP-with-reason, not a silent pass). The cap is sized so `host_used + cap < 60% × host_total`.
3. **Hard wall-clock + memory cap on every scope.** Every OOM/disk scope carries a `timeout <N>s` wall-clock AND its memory cap; on breach the scope is force-killed (`podman kill` / scope teardown), never left to grow.
4. **Disk-full is scratch-confined.** The disk-full filler writes ONLY into a dedicated bounded scratch dir (e.g. a small `tmpfs` mount or a quota-capped dir), never the real PG data volume or `$HOME`; `trap 'rm -f <filler>' EXIT` guarantees cleanup even on early exit.
5. **Evidence proves confinement.** `host_memory_headroom.txt` records host memory pre/start/peak/post and MUST show the host never crossed 60%; `oom_scope_confinement.log` MUST show the kill landed on the container/scope PID, not a host process. An OOM scenario that cannot prove confinement FAILs (it does not silently pass).
6. **No background OOM.** The OOM/disk scenarios run foreground within their detached suite log but are NEVER themselves spawned as fire-and-forget host processes; the harness owns and reaps every scope.

This satisfies §12 host-session-safety: the suite can never consume enough host memory to trigger the OS to sign the operator out.

---

## 8. Definition of done (§108.a + §107.x compliance)

GAP-3 is closed when ALL hold:
- `tests/stress_chaos.sh` runs green hermetically (no creds, no runtime) with every healthy-path stress row PASS and every live-chaos row a real SKIP-with-reason.
- A full live run (operator-supplied PG/Redis runtime) lands `docs/qa/HRD-122-stress-chaos-<TS>/stress_chaos/` with non-empty, value-bearing artefacts for every row.
- `scripts/e2e_bluff_hunt.sh` reports E81..E88 (hermetic PASS + documented live-SKIPs), header bumped to E1..E88.
- `tests/test_stress_chaos_mutation_meta.sh` returns 0 (M1..M5 each FAIL-then-PASS, quiescence clean), run foreground one-at-a-time.
- HRD-122..HRD-130 migrated Issues→Fixed; CLAUDE.md / HERALD_CONSTITUTION.md §108.a reference the suite as the §11.4.85 evidence anchor; operator guide + PDF/HTML/DOCX siblings committed.
- Host-safety proof (§7) present in every resource-exhaustion artefact.
