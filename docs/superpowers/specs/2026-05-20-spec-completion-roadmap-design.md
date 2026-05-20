# Spec V3 → fully-implemented Roadmap (Master Design)

| Field | Value |
|---|---|
| Revision | 1 |
| Created | 2026-05-20 |
| Last modified | 2026-05-20 |
| Status | active |
| Status summary | Master roadmap that sequences the 45 currently-open HRDs into 4 waves (Storage+Channels+Dispatcher → Flavor scaffolds → §42 Constitution bindings → §43 command catalogue), each gated by per-wave §107 end-user-evidence captured in `scripts/e2e_bluff_hunt.sh`. Drives Herald from ~35–45% implemented (today) to spec V3 r7 fully implemented. Each wave gets its own implementation plan via `writing-plans` after this master is approved. |
| Issues | none |
| Issues summary | — |
| Fixed | (n/a — design doc) |
| Fixed summary | — |
| Continuation | After approval, invoke `superpowers:writing-plans` to author the Wave 1 implementation plan. Subsequent waves get their own brainstorm + plan cycles. |

## Constitutional anchors

This design treats the **Herald §107 End-user-usability covenant** (commit `2d4e829`, 2026-05-20) as a structural constraint, not a checklist item. Per §107 (which extends Helix Universal Constitution §11.4 + §11.4.1..§11.4.16), every wave's "done" gate is captured runtime evidence that an end user of the relevant `<flavor>herald` binary can actually use the feature.

Verbatim operator mandate this design must honor (first declared 2026-04-28, reasserted multiple times across 2026-05):

> "all existing tests and Challenges do work in anti-bluff manner - they MUST confirm that all tested codebase really works as expected! We had been in position that all tests do execute with success and all Challenges as well, but in reality the most of the features does not work and can't be used! This MUST NOT be the case and execution of tests and Challenges MUST guarantee the quality, the completition and full usability by end users of the product! This MUST BE part of Constitution of our project, its CLAUDE.MD and AGENTS.MD if it is not there already, and to be applied to all Submodules's Constitution, CLAUDE.MD and AGENTS.MD as well (if not there already)!"

The mandate is fully propagated as of commit `2d4e829`:

- Helix Constitution root (read-only from Herald per §104): ✅ verbatim quote present
- All 10 vendored submodules (`submodules/*` + `containers/`): ✅ verbatim quote present (multiple times each)
- Herald root (CLAUDE.md, AGENTS.md, HERALD_CONSTITUTION.md §107): ✅ verbatim quote present (added 2026-05-20)
- Inheritance gate I8a/b/c: ✅ asserts the verbatim anchor in all three Herald root docs
- Paired §1.1 mutation meta-test (`tests/test_i8_usability_meta.sh`): ✅ 5/5 PASS — proves I8 is not itself a bluff

A future regression that silently weakens the mandate WILL turn the inheritance gate red. The roadmap inherits this guarantee.

## Table of contents

- [Constitutional anchors](#constitutional-anchors)
- [Starting state (2026-05-20)](#starting-state-2026-05-20)
- [Wave overview](#wave-overview)
- [Wave 1 — Storage + first user-visible feature](#wave-1--storage--first-user-visible-feature)
- [Wave 2 — Flavor scaffolds (6 honest-stub binaries)](#wave-2--flavor-scaffolds-6-honest-stub-binaries)
- [Wave 3 — §42 Constitution binding rollout (HRD-018..028)](#wave-3--§42-constitution-binding-rollout-hrd-018028)
- [Wave 4 — §43 Constitution-derived command catalogue (HRD-029..056)](#wave-4--§43-constitution-derived-command-catalogue-hrd-029056)
- [Cross-cutting design constraints](#cross-cutting-design-constraints)
- [Anti-bluff trap matrix (summary across all waves)](#anti-bluff-trap-matrix-summary-across-all-waves)
- [Roadmap totals](#roadmap-totals)
- [Open questions / decisions deferred to per-wave designs](#open-questions--decisions-deferred-to-per-wave-designs)
- [Next steps after approval](#next-steps-after-approval)

## Starting state (2026-05-20)

Per `docs/Status.md` r6 + `docs/Issues.md` r4:

| Metric | Value |
|---|---|
| Spec lines | 4,758 (V3 r7) |
| Spec sections | 39 (§1..§44 with archive renumbering) |
| Open HRDs | 45 |
| Closed HRDs | 13 |
| Estimated weighted completion | ~35–45% |
| `e2e_bluff_hunt.sh` invariants | 15 PASS / 0 FAIL |
| Inheritance gate invariants | 15 PASS / 0 FAIL |
| Anti-bluff audit | 14 PASS / 0 FAIL |
| Flavor binaries built | 1 (`pherald`) |
| Foundation milestones landed | M1 + M2 + M3 |
| Live user-visible features | 7 (per E1..E13 in `e2e_bluff_hunt.sh`) |

## Wave overview

```
   ┌─────────────────────────────────────────────────────────────────┐
   │ DONE  Foundation M1 + M2 + M3  (commit history through 2d4e829) │
   │       commons_constitution + Postgres+RLS + Gin REST surface    │
   └─────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
   ┌─ WAVE 1: Live persistence + first user feature ───────────────────────────────────┐
   │  HRD-010 commons_storage live (pgx + River + Redis ACL)                          │
   │  HRD-011 Telegram channel live (telebot SDK + webhook secret_token)              │
   │  HRD-012 Claude Code dispatcher live (claude --resume + <<<HERALD-REPLY>>>)      │
   │  HRD-008 Operator quickstart e2e validation (already in_progress)                │
   │  §107: real Telegram msg in → Claude Code reply out → DB-persisted evidence      │
   │  e2e_bluff_hunt: +5 invariants (E14-E18). Total 20.                              │
   └────────────────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
   ┌─ WAVE 2: Flavor scaffolds (6 honest-stub binaries) ───────────────────────────────┐
   │  sherald / cherald / bherald / rherald / iherald / scherald                       │
   │  §107: each version --json works; cherald + iherald serve healthz                 │
   │  e2e_bluff_hunt: +10 invariants (E19-E28). Total 30.                              │
   └────────────────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
   ┌─ WAVE 3: §42 Constitution bindings (HRD-018..028, 11 HRDs) ───────────────────────┐
   │  3a HIGH    HRD-018, 019, 020                                                     │
   │  3b MIDDLE  HRD-021, 022, 023, 024, 026, 027                                      │
   │  3c LOW     HRD-025, 028                                                          │
   │  §107: each binding's trigger condition fires → CloudEvent emitted → DB row       │
   │  e2e_bluff_hunt: +13 invariants (E29-E41). Total 43.                              │
   └────────────────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
   ┌─ WAVE 4: §43 Constitution-derived commands (HRD-029..056, 28 HRDs) ───────────────┐
   │  4a HIGH    9 commands  (destructive guard, creds scan, constitution pull,        │
   │             install-upstreams, gate retest, force-push gate, composite-gate,      │
   │             pre-push, mem-budget watch)                                           │
   │  4b MIDDLE  13 commands                                                           │
   │  4c LOW     6 commands                                                            │
   │  §107: each command's golden-path produces correct output + side-effect           │
   │  e2e_bluff_hunt: +28 invariants (E42-E69). Total 71.                              │
   └────────────────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
   ┌──────────────── DONE: Spec V3 r7 fully implemented ───────────────────────────────┐
   │  All 45 currently-open HRDs closed. e2e_bluff_hunt grew from 15 to 71 invariants. │
   │  Every user-visible feature has positive runtime evidence per §107.               │
   └────────────────────────────────────────────────────────────────────────────────────┘
```

### Hard dependency rules

| Rule | Why |
|---|---|
| Wave N must complete before Wave N+1 starts | Each subsequent wave uses primitives the previous wave introduces |
| Within Wave 3/4, sub-wave Na must complete before Nb starts | Criticality ordering — high-impact safety/policy lands first |
| Within a sub-wave, HRDs may run in parallel | They touch different flavors / different bindings |
| Every wave grows `scripts/e2e_bluff_hunt.sh` BEFORE the wave can be declared "done" | §107 covenant — no PASS without positive runtime evidence |
| No wave may "land" with a §107 PASS-bluff present | Bluff = test green without captured runtime evidence. Per §11.4 and §107, this is a release blocker. |

## Wave 1 — Storage + first user-visible feature

### HRD-010 — `commons_storage` live wiring

| Dimension | Spec |
|---|---|
| **Scope** | Replace embedded-migration-only `commons_storage/` with: pgx pool wired through `commons_infra.QuickstartBoot` + RLS context propagation; River queue (`digital.vasic.background`) bound to a real `river_queue` table; Redis ACL'd via `digital.vasic.cache` with per-tenant key prefix. Migrations 000001..000005 (already embedded) become live. |
| **§107 evidence** | (a) `commons_storage.Pool.Exec` round-trip against live Postgres returns ≥1 row from `herald_subscribers` after a write+read cycle. (b) River enqueue+dequeue round-trip — a fake `outbound_delivery` job is processed by a registered worker, with persisted `attempts`, `state`, `errored_at`. (c) Redis SET + GET round-trip with TTL expiry verified. ALL three captured as `e2e_bluff_hunt.sh` invariants. |
| **New e2e invariants** | **E14** pgx pool against live PG (RLS context propagates). **E15** River worker actually runs (not just enqueue). **E16** Redis TTL respected. |
| **Anti-bluff trap** | A "PASS on connection-only" without round-trip data is forbidden. Each invariant MUST observe a state mutation, not just an OK from the driver. |

### HRD-011 — Telegram channel live integration

| Dimension | Spec |
|---|---|
| **Scope** | Implement `commons_messaging/channels/tgram/` per spec §11.1. Two modes: long-poll (`getUpdates`) for dev and webhook (with `secret_token`) for prod. Outbound via `sendMessage` + attachments via `sendDocument`. Subscriber resolution per §7. Persist `outbound_delivery_evidence` rows to `commons_storage`. |
| **§107 evidence** | A real Telegram bot (operator-supplied token in `.env`, never committed) receives an inbound message → Herald processes it → a known outbound message is dispatched back → the bot's chat shows it. The full round-trip is captured: chat ID, message text in, message text out, `delivery_evidence.delivered_at` row in PG. |
| **New e2e invariants** | **E17** Live Telegram round-trip (skipped with reason if `HERALD_TGRAM_BOT_TOKEN` absent — explicit SKIP-with-reason, NEVER PASS-by-default per §11.4.3 + §107). |
| **Anti-bluff trap** | If credentials absent → SKIP with reason. If credentials present → PASS requires captured Telegram API response body + DB row. A "PASS because we mocked the API" is forbidden. |

### HRD-012 — Claude Code dispatcher live integration

| Dimension | Spec |
|---|---|
| **Scope** | Implement `commons_messaging/dispatch/claude_code/` per spec §33. Resolve a session via the anchor-file pattern (§33.2), invoke `claude --resume <session-id>` with the formatted envelope, capture stdout, parse `<<<HERALD-REPLY>>>...<<<END>>>` markers, emit the reply as an outbound message on the originating channel. |
| **§107 evidence** | A test inbound message routed to the Claude Code dispatcher produces a real `claude` CLI invocation, the reply text contains the marker-delimited payload, and the parsed reply is dispatched back on the source channel (Telegram in this wave's vertical slice). Captured: `claude` exit code, stdout transcript (sanitized), parsed reply body, outbound `delivery_evidence` row. |
| **New e2e invariants** | **E18** Claude Code round-trip (skipped with reason if `HERALD_CLAUDE_SESSION_ID` absent). |
| **Anti-bluff trap** | A "PASS because we exec'd `echo` instead of `claude`" is forbidden. The real `claude` binary or explicit SKIP. |

### HRD-008 — Operator-side quickstart e2e validation (already in_progress)

| Dimension | Spec |
|---|---|
| **Scope** | Operator runs `docker compose -f quickstart/compose.yaml up` on a fresh laptop. Postgres+Redis+OTel+pherald all boot. Validation walks the same e2e_bluff_hunt path against the composed stack rather than ad-hoc containers. |
| **§107 evidence** | Captured terminal transcript + `compose ps` output + first 50 lines of pherald log + healthz/readyz response bodies, signed off by operator in `docs/Fixed.md` HRD-008 row. |
| **New e2e invariants** | **E_OPS** Quickstart-mode invocation of e2e_bluff_hunt against the composed stack (parameterized variant of the existing one). |

### Wave 1 dependency graph

```
   HRD-010 storage live ───┐
                           ├──> Wave 1 done (E14-E18 + E_OPS green)
   HRD-011 Telegram live ──┤    + HRD-008 closed with operator evidence
                           │
   HRD-012 Claude Code ────┘
```

HRD-010 is a hard prereq for HRD-011/012 (they persist delivery evidence). HRD-011 and HRD-012 can proceed in parallel once HRD-010 lands. HRD-008 validates the full stack and runs LAST.

### Wave 1 gate

1. `e2e_bluff_hunt.sh` reports **20 PASS / 0 FAIL** (was 15 — added E14, E15, E16, E17, E18; E17/E18 explicit SKIP-with-reason if credentials absent counts as PASS for the gate, not as bluff).
2. Inheritance gate still 15/15 PASS.
3. Anti-bluff audit still 14/14 PASS.
4. `docs/Issues.md` HRD-010/011/012 rows migrate to `docs/Fixed.md` per §11.4.19 atomic-migration.
5. Operator signs off HRD-008 with captured transcript.

## Wave 2 — Flavor scaffolds (6 honest-stub binaries)

### Per-flavor scope + §107 evidence

| Flavor | Mission (per spec §18) | Serves HTTP? | Subcommands scaffolded | §107 evidence |
|---|---|---|---|---|
| **sherald** | System Herald — host safety, destructive-op guard, force-push interceptor, mem-budget watcher | **No** (CLI + daemon) | `version`, `daemon` (501→HRD-056), `destructive-guard` (501→HRD-033), `force-push-gate` (501→HRD-046), `mem-budget-watch` (501→HRD-056), `backup-snapshot` (501→HRD-034) | `sherald version --json` returns valid JSON with `flavor:"sherald"` |
| **cherald** | Constitution Herald — policy evaluator, creds scan, docs sync, composite-gate | **Yes** (`/v1/compliance` per §42.1.5) | `version`, `serve`, `policy` (501→HRD-019), `creds-scan` (501→HRD-036), `docs-sync` (501→HRD-037), `composite-gate` (501→HRD-051), `export` (501→HRD-052) | `cherald version --json` ✓ + `cherald serve` binds + `/v1/healthz` 200 + `/v1/compliance` returns 501 with HRD-028 pointer (honest stub) |
| **bherald** | Build Herald — CI/test bindings, gate-result emitter, test-tier verifier | **No** (CLI invoked from CI) | `version`, `evidence-capture` (501→HRD-035), `test-tier-verify` (501→HRD-041), `gate-retest` (501→HRD-045) | `bherald version --json` ✓ |
| **rherald** | Release Herald — tag mirroring, changelog, installable-asset evidence | **No** (CLI invoked from release pipeline) | `version`, `tag-mirror` (501→HRD-031), `changelog-generate` (501→HRD-032), `gate-retest` (501→HRD-045) | `rherald version --json` ✓ |
| **iherald** | Incident Herald — credential-leak page-out, operator-blocked escalation | **Yes** (webhook receiver for paging systems) | `version`, `serve`, `escalate` (501→HRD-024) | `iherald version --json` ✓ + `iherald serve` binds + `/v1/healthz` 200 |
| **scherald** | Scheduled-audit Herald — periodic Status.md sweep, daily/weekly/monthly compliance digest | **No** (cron-driven) | `version`, `audit` (501→HRD-025), `status-digest` (501→HRD-047) | `scherald version --json` ✓ |

### Build + ship layout

Each flavor lives in its own Go module under the repo root: `sherald/`, `cherald/`, `bherald/`, `rherald/`, `iherald/`, `scherald/` — mirroring `pherald/`. All 6 added to `go.work`. Shared cobra+version+serve plumbing factored into `commons/cli/` (NEW) so each flavor's `cmd/<flavor>herald/main.go` is ~30 lines.

Per Helix §11.4.74 we **must** Catalogue-Check `vasic-digital`/`HelixDevelopment` for an existing CLI scaffolding module before writing `commons/cli/` — if one exists, extend; if not, the new module is `extend|no-match` documented in the HRD.

### New e2e_bluff_hunt invariants

```
E19-E24  <flavor>herald version --json for each of 6 flavors (parses JSON,
         asserts flavor + version + build fields populated, NOT empty stubs)

E25-E26  cherald serve + iherald serve bind + healthz 200 + readyz 200

E27      cherald /v1/compliance returns 501 with HRD-028 pointer body
         (honest stub — proves the route exists and reports its incompleteness)

E28      iherald /v1/healthz returns 200 with iherald build_info
```

**Wave 2 e2e_bluff_hunt grows from 20 (post-Wave-1) → 30 invariants.**

### Wave 2 anti-bluff traps

1. **"Empty version field" bluff.** A `version --json` that returns `{"flavor":"sherald","version":""}` is forbidden — the e2e probe asserts non-empty version, build, and goVersion fields.
2. **"Serve returns 200 on every route" bluff.** Each flavor's serve MUST return 404 for unknown routes (catch-all), 501 with HRD pointer for declared-but-unimplemented routes, and 200 ONLY for actually-implemented routes (healthz/readyz/metrics).
3. **"Build succeeds, never runs" bluff.** `go build ./<flavor>/...` PASS alone is NOT sufficient. Every E19-E28 invariant invokes the resulting binary and asserts on its output.

### Wave 2 dependency

```
Wave 1 done
   │
   ▼
Wave 2.0  commons/cli/ shared scaffolding (single PR)
   │
   ▼
Wave 2.1  6 flavor scaffolds in parallel (one PR per flavor)
   │      can all proceed concurrently — they don't share state
   ▼
Wave 2 gate green
```

### Wave 2 gate

1. `e2e_bluff_hunt.sh` reports **30 PASS / 0 FAIL**.
2. `go build ./...` clean across all 7 (was 1) flavor binaries.
3. `go.work` lists 13 Herald modules (was 7).
4. `docs/Status.md` Implementation table shows ✅ scaffold landed for all 6 new flavors.
5. CodeGraph re-indexed (`scripts/codegraph_setup.sh`) and validate green for the new symbols.

## Wave 3 — §42 Constitution binding rollout (HRD-018..028)

11 HRDs split into 3 sub-waves by criticality.

### Sub-wave 3a — "high" criticality (3 HRDs)

| HRD | Binding | Trigger condition | Emitted CloudEvent | §107 evidence probe |
|---|---|---|---|---|
| **HRD-018** *(finalize from M1 partial)* | Evaluator framework + emit helpers + bundle-hash captureer + mode-ladder config | Any binding evaluator invoked | Any of 12 event classes via `cherald.Emit*()` | **E29**: Evaluator round-trip — register a fake binding, fire it, assert event lands on memory bus + `constitution_state` row written with `bundle_hash` set + `mode` resolved per ladder |
| **HRD-019** | cherald: ~30 `.policy.violation` rules + `.gate.failed/.recovered` + `.credential.leak` + `.spec.revision_drift` + `.catalogue.miss` handlers; `/v1/compliance` pull surface | Any of the 30 rules trips | `digital.vasic.herald.policy.violation` (CloudEvents v1.0) | **E30**: Inject a synthetic violation, invoke evaluator, assert CloudEvent on cherald's bus + `constitution_state` row + `/v1/compliance` returns the row in JSON |
| **HRD-020** | sherald: §9.1/9.2/9.3 + §12.1-3/.6 + §11.4.32/.36/.41/.71 — destructive-op detection hook, force-push interceptor, mem-budget watcher | OS-level signals: process exec'ing `rm -rf`, `git push --force`, RSS over 60% threshold | `.host.safety_breach` / `.repo.force_push_blocked` / `.host.memory_breach` | **E31**: Spawn a child process that tries `rm -rf` against a sandboxed temp dir under sherald's hook + assert sherald intercepted + emitted + the child exit code is non-zero. **E32**: Same for force-push attempt against a sandboxed test repo. **E33**: Spawn a memory-balloon process + assert sherald emits `.host.memory_breach` when threshold crossed |

**Sub-wave 3a §107 trap:** A "PASS because we mocked the syscall" is forbidden. E31/E32/E33 use real `os/exec` against sandboxed targets, not mocks. The captured-evidence is: child exit code, sherald log line, DB row.

### Sub-wave 3b — "middle" criticality (6 HRDs)

| HRD | Flavor | Binding scope | §107 evidence probe |
|---|---|---|---|
| **HRD-021** | bherald | CI/test bindings — gate-result emitters per §1 + ~20 §11.4.x clauses | **E34**: bherald processes a real `audit_antibluff.sh` result file → emits one `.gate.recovered` per PASS, one `.gate.failed` per FAIL |
| **HRD-022** | rherald | Release bindings — §4 tag mirroring, §5 changelog, §11.4.38 installable-asset evidence, §11.4.40 full-suite retest | **E35**: rherald processes a real `git tag v0.x.y` event → emits `.release.tagged` with the tag SHA + asserts asset evidence captured. Tests against a sandboxed test repo |
| **HRD-023** | pherald | Project bindings — §2 commit+push, §3 submodule propagation, §11.4.11/.15/.21/.22/.34/.37/.42/.55/.66/.71/.74 | **E36**: pherald processes a real commit on a sandboxed test repo → emits `.project.commit_landed` with multi-mirror push status |
| **HRD-024** | iherald | Escalation bindings — §11.4.10/.10.A credential-leak page-out, §11.4.21 + §11.4.66 operator-blocked escalation | **E37**: Synthetic credential-leak event triggers iherald → asserts page-out attempted on a configured channel (real Telegram if creds present, SKIP-with-reason otherwise) |
| **HRD-026** | (shared) | Constitution-bundle hash captureer — SHA-256 of rendered Constitution.md at evaluation time; persists on every emitted event | **E38**: Compute hash of `<discovered>/Constitution.md` → compare against `constitution_state.bundle_hash` on a freshly-emitted event → assert exact match |
| **HRD-027** | (shared) | Mode-ladder runtime config — `constitution_bindings` table + admin REST endpoints to flip allow/warn/enforce per binding per tenant without redeploy | **E39**: Flip a binding via cherald `/v1/compliance/bindings/<id>` PATCH from `warn` → `enforce` → trigger same condition again → assert the second event carries `mode:"enforce"` while the first carries `mode:"warn"` |

**Sub-wave 3b §107 trap:** Each probe operates against a real instance — real bherald reading a real audit file, real rherald against a sandboxed git repo, real pherald against a sandboxed commit. No "we tested the function in isolation, assume it works in context" PASS.

### Sub-wave 3c — "low" criticality (2 HRDs)

| HRD | Flavor | Binding scope | §107 evidence probe |
|---|---|---|---|
| **HRD-025** | scherald | Scheduled-audit bindings — §11.4.45 periodic Status.md sweep + daily/weekly/monthly compliance digest | **E40**: Invoke `scherald audit --once` against a sandboxed Herald-shape repo → assert digest written to `docs/Status_Summary.md` + `.audit.digest_published` event emitted |
| **HRD-028** | cherald | `/v1/compliance` pull surface — Gin handler returning `constitution_state` rows filtered by rule / subject / decision; paginated | **E41**: GET `/v1/compliance?rule=force_push_blocked&limit=10` against a cherald with 11 fixture rows → asserts 10 rows returned + `next_page` token + filter applied |

### Wave 3 shared design constraints

1. **CloudEvents v1.0** — every emission uses the existing `commons_constitution/cloudevents.go` adapter from M1. No new event schema.
2. **In-process bus initially** — `commons_constitution.MemoryBus` is the default consumer for tests. A production `digital.vasic.eventbus` (Watermill) backend can be wired later without changing binding code (interface already exists).
3. **`constitution_state` schema** — already migrated in M2 (migration 000005). No new tables needed for Wave 3.
4. **Catalogue-Check per HRD** — every new binding HRD must declare `reuse|extend|no-match <org/repo>@<sha>` per Universal §11.4.74. Most are `extend digital.vasic.middleware|auth|recovery` for the safety bindings.
5. **Paired §1.1 mutation meta-test per binding.** Every binding HRD ships a paired test (`tests/test_binding_<id>_meta.sh`) that modifies the binding's emission contract to assert the gate catches the regression. 11 new paired meta-tests total.

### Wave 3 dependency graph

```
Wave 2 done (6 flavors scaffolded)
   │
   ▼
3a HIGH (HRD-018 finalize → HRD-019 + HRD-020 in parallel)
   │
   ▼
3b MIDDLE (HRD-021/022/023/024/026/027 — six PRs in parallel after 3a)
   │
   ▼
3c LOW (HRD-025 + HRD-028 in parallel)
   │
   ▼
Wave 3 gate green
```

### Wave 3 gate

1. `e2e_bluff_hunt.sh` reports **43 PASS / 0 FAIL**.
2. **All 11 HRDs migrated** Issues → Fixed (atomic per §11.4.19).
3. Every binding has a paired §1.1 mutation meta-test green.
4. `docs/specs/mvp/specification.V3.md` Revision bumped (per §11.4.73) reflecting binding-rollout completion.

## Wave 4 — §43 Constitution-derived command catalogue (HRD-029..056)

28 HRDs implementing the 27-entry §43 catalogue (HRD-039 bundles §11.4.19 + §11.4.23 into one workable item). Grouped by criticality.

### Sub-wave 4a — "high" criticality (9 commands)

| HRD | Flavor | Command | Description | §107 evidence probe |
|---|---|---|---|---|
| **HRD-033** | sherald | `destructive guard <op>` | Wraps `rm`, `git reset --hard`, `git push --force` with prerequisite checks | **E42**: Invoke against a real `rm -rf <tmpdir>` → assert hardlink backup created BEFORE the rm proceeds + `.host.destructive_op_logged` event emitted |
| **HRD-036** | cherald | `creds scan <path>` | gitleaks/trufflehog integration; emits `.credential.leak` | **E43**: Plant a fake AWS access key in fixture → invoke scan → assert finding reported + event emitted. Plant non-secret → assert clean (no false-positive bluff) |
| **HRD-040** | sherald | `constitution pull` | Wrap fetch + rebase + validation gate, emits `.bundle.updated` | **E44**: Invoke against sandboxed clone of the constitution → assert fetch happened (git reflog) + gate ran + new bundle_hash captured |
| **HRD-043** | pherald | `install-upstreams` | install_upstreams wrapper (extends constitution submodule's existing script per Catalogue-Check) | **E45**: Invoke against fresh sandboxed clone → assert origin has 4 push URLs configured (github + gitlab + gitflic + gitverse) |
| **HRD-045** | rherald | `gate retest` | Pre-tag full-suite retest gate | **E46**: Invoke against sandboxed Herald-shape repo → assert e2e_bluff_hunt + audit + gate all run + result captured + non-zero exit on simulated FAIL |
| **HRD-046** | sherald | `force-push gate` | Merge-first + per-session-auth enforcement | **E47**: Attempt `git push --force` against sandboxed repo without per-session auth token → assert blocked + `.repo.force_push_blocked` emitted. With token → assert allowed |
| **HRD-051** | cherald | `composite-gate` | Canonical CM-DOCS-COMPOSITE-SYNC implementation | **E48**: Invoke against fixture set with intentional doc-out-of-sync condition → assert FAIL + specific files reported. Fix → re-invoke → PASS |
| **HRD-053** | pherald | `pre-push` | Fetch + investigate + integrate hook | **E49**: Wired as git pre-push hook on sandboxed repo → push triggers it → assert fetch happens before push proceeds + diverged history triggers rebase prompt |
| **HRD-056** | sherald | `mem-budget watch` | Daemon-mode 60% threshold watcher emitting `.host.safety_breach` | **E50**: Spawn `mem-budget watch` as background process + spawn balloon → assert event emitted when RSS crosses 60% (already proven in Wave 3 HRD-020 — this E50 wraps the standalone daemon mode) |

### Sub-wave 4b — "middle" criticality (13 commands)

| HRD | Flavor | Command | §107 evidence probe |
|---|---|---|---|
| **HRD-029** | pherald | `commit-push` | **E51**: Real commit on sandboxed repo → assert all 4 mirrors received |
| **HRD-030** | pherald | `submodule propagate` | **E52**: Modify owned submodule → assert propagation order + commits land |
| **HRD-031** | rherald | `tag mirror` | **E53**: Tag on parent → assert tag exists on every owned submodule |
| **HRD-034** | sherald | `backup snapshot <path>` | **E54**: Invoke → assert hardlinks created (`ls -li` shows same inode) |
| **HRD-035** | bherald | `evidence capture <test_id>` | **E55**: Invoke after a test run → assert captured-evidence file written |
| **HRD-041** | bherald | `test-tier verify` | **E56**: 8-tier matrix runs → all tiers reported PASS/SKIP/FAIL with evidence |
| **HRD-044** | pherald | `fetch-guard` | **E57**: Pre-edit hook intercepts non-fetched state → assert rebase prompted |
| **HRD-047** | scherald | `status digest` | **E58**: Sweep + regen → assert `docs/Status_Summary.md` and `.audit.digest_published` event |
| **HRD-049** | pherald | `reopen <HRD-NNN>` | **E59**: Reopen a fixture closed HRD → assert Fixed→Issues migration + Reopens history line |
| **HRD-050** | cherald | `readme sync` | **E60**: Invoke → assert README doc-links regenerated + multi-format re-export ran |
| **HRD-052** | cherald | `export` | **E61**: Bulk export → assert .md / .html / .pdf / .docx siblings updated for fixtures |
| **HRD-054** | cherald | `spec-version check` | **E62**: Plant Revision-vs-edits drift → assert FAIL. Fix → assert PASS |
| **HRD-055** | cherald | `catalogue-check <pr>` | **E63**: Scan a fixture PR → assert Catalogue-Check lines extracted + survey runner produces report |

### Sub-wave 4c — "low" criticality (6 commands)

| HRD | Flavor | Command | §107 evidence probe |
|---|---|---|---|
| **HRD-032** | rherald | `changelog generate` | **E64**: Conventional Commits → `docs/changelogs/<v>.md` + multi-format export |
| **HRD-037** | cherald | `docs sync` | **E65**: Regen Issues_Summary / Fixed_Summary / Status_Summary from sources |
| **HRD-038** | cherald | `script-docs check` | **E66**: Assert sibling .md exists for every `scripts/**/*.sh` (plant a missing one → FAIL) |
| **HRD-039** | cherald | `fixed align` + `colorize` | **E67**: Issues/Fixed format → table-shape valid; HTML colorizer produces colored spans |
| **HRD-042** | cherald | `submanifest verify` | **E68**: Submodule-Dependency-Manifest gate against fixtures |
| **HRD-048** | cherald | `fixed-summary sync` | **E69**: Standalone Fixed_Summary backfill — same evidence as E65 but isolated |

### Wave 4 shared design constraints

1. **Sandbox-first.** Every command's §107 probe runs against a sandboxed working tree (a temp directory the test owns), NEVER against the live Herald repo. Required to prevent test pollution and to make probes hermetic.
2. **Captured outputs in `evidence/` JSON.** Each probe writes its captured evidence (stdout, side-effect proof, DB rows, file hashes) to `evidence/E_NN_<command>.json` so an auditor can re-verify by inspecting the JSON without re-running the test.
3. **Shared `commons/cmdfx/`** (NEW, Catalogue-Check first). Pulls common patterns: sandbox setup/teardown, sandboxed-git-repo helper, hardlink-backup helper, evidence-capture helper. Each command's `main.go` stays small.
4. **Plant-then-detect pattern** for negative tests. For commands that detect drift (HRD-054 spec-version check, HRD-038 script-docs check, HRD-039 fixed align), every probe MUST include a "plant the broken state → expect FAIL" subtest AND a "fix the state → expect PASS" subtest. Half a test (only positive) is a §107 bluff.

### Wave 4 dependency

```
Wave 3 done (§42 bindings firing on real events)
   │
   ▼
Wave 4.0  commons/cmdfx/ shared helpers (single PR, Catalogue-Check first)
   │
   ▼
4a HIGH (9 commands, can mostly run in parallel)
   │
   ▼
4b MIDDLE (13 commands, parallel across flavors)
   │
   ▼
4c LOW (6 commands, all cherald, mostly parallel)
   │
   ▼
Wave 4 gate green ⇒ spec V3 r7 fully implemented
```

### Wave 4 gate

1. `e2e_bluff_hunt.sh` reports **71 PASS / 0 FAIL**.
2. All 28 HRDs migrated Issues → Fixed atomically (§11.4.19).
3. Every command has a "plant + detect + fix + verify" probe pair (positive + negative captured evidence).
4. `evidence/E_NN_<command>.json` corpus committed (sanitized — no real secrets).
5. `docs/specs/mvp/specification.V3.md` Revision bumped — final spec V3 r-final declares all §43 entries implemented.

## Cross-cutting design constraints

These apply to every wave and every HRD inside it.

### C1. §11.4.74 Catalogue-Check is mandatory before writing new code

For every new module, helper, or external integration: search `vasic-digital/*` and `HelixDevelopment/*` for an existing capability. Declare `reuse|extend|no-match <org/repo>@<sha>` in the HRD's References cell. A `TBD` Catalogue-Check is acceptable for opening an HRD but MUST resolve before the HRD closes.

### C2. Multi-mirror push parity on every wave-closing commit

Each wave's closing commit fans out to GitHub + GitLab + GitFlic + GitVerse via the existing `origin` push-fan-out (per HERALD_CONSTITUTION §103). Per-host pushes are forbidden unless rebuilding fan-out configuration.

### C3. Spec revision bump on wave close (§11.4.73)

Spec V3's Revision field bumps on every wave-closing commit that closes spec-impacting HRDs. Minor wave-close = Revision++; the V3→V4 jump is reserved for a true rewrite.

### C4. Hardlinked backup before destructive ops (§9)

Any HRD whose implementation involves rewriting tracker docs (Issues→Fixed migration, Status.md regeneration) MUST take a hardlinked backup first.

### C5. 60% RAM cap on heavy work (§12.6)

Wave 1 (live integrations) and Wave 4 (sandbox-heavy probes) are most at risk. Each new test that spawns child processes (E31/E32/E33/E50) MUST respect the §12.6 cap — Wave 3a's HRD-020 itself implements the watcher; until then, manual operator vigilance.

### C6. End-to-end evidence stays in `scripts/e2e_bluff_hunt.sh`, not in `_test.go`

`go test` proves unit-level correctness. The §107 covenant lives in `e2e_bluff_hunt.sh` because that's what boots real services. New invariants land THERE first, then optionally a `_test.go` mirror for fast iteration. The bluff vector is "I added the test but only to `_test.go` and the e2e script never sees it."

### C7. Honest-stub 501s with HRD pointers (not 200s or panics)

For every declared-but-unimplemented route/subcommand, return HTTP 501 (or CLI exit 1) with a body/stderr line referencing the HRD-NNN that will implement it. This is the §107-faithful "absence of feature" representation. A 200 with a "TODO" body or a panic-on-call are both bluffs.

### C8. SKIP-with-reason for credential-dependent tests

Tests that require operator-supplied credentials (Telegram bot token, Claude Code session ID, real Postgres host) MUST `SKIP-with-reason` when credentials are absent — per §11.4.3. A PASS-by-default with no credential check is a critical bluff.

### C9. Paired §1.1 mutation meta-test per new gate

Every new gate invariant added across Waves 1-4 must ship with a paired mutation meta-test that proves the gate catches the regression it claims to catch. Without the paired test, the gate is itself a §11.4 PASS-bluff.

## Anti-bluff trap matrix (summary across all waves)

| Trap class | Manifestation | Mitigation pattern |
|---|---|---|
| **Mock-driven PASS** | Test mocks the API/syscall and PASSes against the mock | Real `os/exec` against sandboxed targets; real driver against live container; real `claude` CLI not `echo` |
| **Compile-only PASS** | `go build` succeeds, binary never invoked | Every probe invokes the binary and asserts on its output/side-effect |
| **Connection-only PASS** | Driver returns OK on Open(), never reads/writes data | Each probe observes a state mutation (row insert, file write, child exit code) |
| **Positive-only PASS** | Test only covers the happy path | Plant-then-detect pattern; assert FAIL on broken state, then PASS on fixed state |
| **Empty-field PASS** | JSON response returns `{}` or empty strings | Each probe asserts non-empty content of required fields |
| **Catch-all 200 PASS** | Serve returns 200 to every request | Probe asserts 404 for unknown routes, 501 with HRD pointer for declared-unimplemented |
| **Gate-bluff PASS** | Gate reports PASS but doesn't actually check what it claims | Paired §1.1 mutation meta-test required per gate |
| **Skipped-silently PASS** | Test SKIP without explanation, counted as PASS | SKIP-with-reason mandatory; SKIP counted explicitly, never folded into PASS |
| **Sandbox-leakage PASS** | Test mutates the live repo, "PASSes" but pollutes state | Sandbox helper enforces tmpdir paths; CI hook rejects post-test working-tree changes |
| **Anchor-loss PASS** | Constitutional anchor silently removed; gate still green | I8a/b/c invariants + `tests/test_i8_usability_meta.sh` paired mutation |

## Roadmap totals

| Metric | Start (2026-05-20) | End (spec V3 r7 fully implemented) | Δ |
|---|---|---|---|
| Open HRDs | 45 | 0 | −45 |
| Closed HRDs | 13 | 58 (45 + 13) | +45 |
| `e2e_bluff_hunt.sh` invariants | 15 | ~71 | +56 (+373%) |
| Flavor binaries | 1 (`pherald`) | 7 | +6 |
| Inheritance gate invariants | 15 | 15 | 0 (gate is meta) |
| Paired §1.1 mutation meta-tests | 3 | ~14 | +11 (per-binding meta) |
| Modules in `go.work` | 7 | 13+ | +6 flavors (+ commons/cli, commons/cmdfx) |
| Spec sections fully landed | ~9 | 39 | +30 |

## Open questions / decisions deferred to per-wave designs

These are intentionally NOT resolved in the master roadmap. Each per-wave design owns its own answer.

1. **Wave 1**: Should River queue configuration default to in-process worker (simpler) or external pool (more realistic)? Defer to the HRD-010 sub-design.
2. **Wave 1**: How does HRD-011's webhook secret_token rotate? Defer to HRD-011 sub-design.
3. **Wave 2**: Does each flavor need its own `Issues.md`/`Fixed.md`, or share Herald's? Lean toward shared; defer to Wave 2 sub-design.
4. **Wave 3**: When does `digital.vasic.eventbus` (Watermill) replace `MemoryBus` in production? Likely Wave 3b or later; defer.
5. **Wave 4**: Is `evidence/` committed to git or stored externally (e.g. S3-backed)? Sensitivity question — defer.
6. **Wave 4**: Are the 9 high-criticality 4a commands enough to declare a "minimum-viable safety release", or do we need 4b+4c too? Defer to operator preference at Wave 3-close checkpoint.

## Next steps after approval

1. **Commit this design doc** with multi-mirror push (per §103).
2. **Invoke `superpowers:writing-plans`** to author the **Wave 1** implementation plan (HRD-010 → HRD-011/HRD-012 → HRD-008 close-out).
3. **Per-wave brainstorm.** Each subsequent wave (2, 3, 4) gets its own brainstorming + design doc + writing-plans cycle when its predecessor wave closes. Each sub-design references this master doc.
4. **§107 hygiene at every milestone.** Every wave close re-runs the full anti-bluff battery: inheritance gate, I6+I8 meta-tests, audit, codegraph validate, e2e_bluff_hunt. ALL green or the wave is not "done".
