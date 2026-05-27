<div align="center">

<img src="../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Herald — Continuation

| Field | Value |
|---|---|
| Revision | 16 |
| Created | 2026-05-20 |
| Last modified | 2026-05-28 |
| Status | active |
| Status summary | **2026-05-28 (day-2) — v1.0.0 Batch B COMPLETE + constitution sync + 3 fixes (HEAD `74649bb`; constitution pulled `3c9c4e9`→`15cd4bc`). (a) Constitution sync — pulled HelixConstitution `3c9c4e9`→`15cd4bc` (gained §11.4.95 workable-items-DB-tracked-not-gitignored / §11.4.96 safe-parallel-work-with-long-build catalogue / §11.4.97 max-idle + progress-update cadence) and propagated all three into CLAUDE.md r13 / AGENTS.md r9 / HERALD_CONSTITUTION.md r7 §108.j-l (commit `d5b9895`); Herald already complied with §11.4.95 (DB version-controlled per the 2026-05-27 operator divergence); inheritance gate 15 PASS / 0 FAIL. (b) Three real fixes — #194 `audit_stress_chaos_test.go` now gates its qaRoot on `HERALD_STRESS_QA_DIR` via `t.TempDir()` (commit `a8ad4f7`, stops the suite littering `docs/qa/` on every run); `commons_constitution/eventbus.go` `PublishedByType` data race → `sync.Map.LoadOrStore` (commit `f5d1367` — a REAL `-race` regression on E3 latent since Batch A, surfaced by the Batch-B bindings stress test); E17 e2e SKIP-guard now requires a live Postgres (commit `75e7a49` — E17 was FAILing instead of honestly SKIPping when PG is absent). (c) v1.0.0 Batch B COMPLETE — all 7 §42 flavor constitution bindings (HRD-019..025) landed, wiring each flavor to emit through the Batch-A `commons_constitution` Evaluator+audit foundation: HRD-019 cherald (`f33b5e4`, e2e E90 `c3e3cff`), HRD-020 sherald (`5ebad4f`, E91 `75e7a49`), HRD-021 bherald (`46ca77e`, E92 `f31615f`), HRD-022 rherald (`f73195f`, E93 `0206f8f`), HRD-023/024/025 pherald/iherald/scherald (`3fb753a`, landed in one parallel pass; E94/E95/E96 `74649bb`). Each flavor: `<flavor>/internal/bindings/` Pipeline + rule catalogue + bespoke PURE detectors, `-race -count=3` green, §107.x evidence under `docs/qa/HRD-0XX-*/`, §11.4.85 stress (1200 concurrent → 1200 emits+audit rows). All HRD-019..025 remain **in_progress** — mechanism + rules + detectors are done, but live operation-interception (the Subjects fed to the detectors) is Batch C §43 command bodies. e2e invariants now run through E96 (E89 Batch-A foundation + E90-E96 Batch B). Prior r13 (still valid history):** **2026-05-27 v1.0.0 Batch A complete checkpoint (HEAD `854d301`; constitution at `3c9c4e9`). v1.0.0 readiness workstream (workstream 3) STARTED — plan `docs/superpowers/plans/2026-05-27-v1.0.0-execution.md` (`9da90ba`), 39 HRDs + Wave 7 batched A→E with tag cadence v0.5→v1.0.0. Batch A DONE (commits `b19cb43` + `854d301`): HRD-018 / HRD-026 / HRD-027 closed atomically Issues→Fixed (Issues.md r16→r17, 40→37 open). Closed the `commons_constitution` `constitution_audit` write-through §107 PASS-bluff — the table existed in migration 000006 but the Runner never wrote a row (`RunOutcome.Audited=true` persisting 0 rows); FIXED with an `AuditStore` interface + `state.MemoryAudit` + `state.PostgresAudit` (RLS-guarded append-only INSERT) + a new `IDEmitter` (emit returns the generated event ID) so `Runner.Run` writes a durable audit row for every CHANGED Warn/Enforce transition carrying old/new decision+digest, bundle_hash, mode, and the EXACT emitted event ID for Enforce. HRD-026 bundle-hash now persisted on every emitted event AND audit row (replay §42.1.3). HRD-027 mode-ladder runtime config wired to 3 JWT-gated admin REST endpoints on cherald (`GET`/`GET-one`/`PUT /v1/compliance/modes[/:rule_id]`, flip allow/warn/enforce per tenant, no redeploy). Conductor-verified: `-race -count=3` deterministic, real-PG integration green, full `scripts/e2e_bluff_hunt.sh` **81 PASS / 0 FAIL** (was 78; +3 = E89 wired). Evidence `docs/qa/HRD-018-20260527T142556Z/` (real-PG emit→persist + audit DB-level `SELECT` proof + admin-REST flip round-trip + §11.4.85 stress 512 concurrent emit→persist = exactly 512 audit rows no-lost-writes + chaos 960 concurrent flips). **§11.4.93 SQLite-SSoT workstream 2 DEFERRED by operator (2026-05-27 decision):** the constitution's workable-items migration Go binary (`constitution/scripts/workable-items/`) is an unimplemented scaffold (0/7 subcommands) AND schema-incompatible with Herald's HRD-NNN pipe-table format; per §11.4.74 catalogue-first Herald can NOT reimplement it — the fix is a constitution-repo task affecting all consumers. Assessment `docs/research/workable-items-phase2-assessment-2026-05-27.md` (`36fa90b`). HRD-131 stays OPEN, blocked-on-constitution-binary. Prior r12 (still valid history):** **2026-05-27 GAP-3 + HRD-132 close-out checkpoint (HEAD `4f45f26`; constitution at `3c9c4e9`). GAP-3 §11.4.85 stress+chaos suite (HRD-122..HRD-130) AND the HRD-132 Runner idempotency-window fix are COMPLETE and now atomically migrated Issues→Fixed (Issues.md r16 / Fixed.md r15, §11.4.19): HRD-122..HRD-130 Task→Completed, HRD-132 Bug→Fixed. HRD-132 fix `354b883` lands claim-before-dispatch at Runner Stage 2 — `INSERT events_processed … ON CONFLICT DO NOTHING` is the AUTHORITATIVE exactly-once dispatch gate (the Stage-2→Stage-7 duplicate-dispatch window HRD-125 surfaced is closed); latent `runner.go:132` `CachedRcpt.WasReplay` race fixed; HRD-125 assertion upgraded to `sends==1` (`docs/qa/HRD-125-stress-chaos-2026-05-27T131251-gap3/` shows `dispatch_exactly_once=1`, `shared_key_sends=1 want=1`); wave3 mutation-gate M4 retired (`4f45f26`) — Stage-7 Insert now redundant. Full `scripts/e2e_bluff_hunt.sh` 78 PASS / 0 FAIL; HRD-130 gate 6 PASS / 0 FAIL (5 load-bearing mutations). HRD-131 (SQLite SSoT migration Phase 1) remains OPEN; Phases 2-6 follow-on. Prior GAP-3 checkpoint (HEAD `d2a60d3`, all 4 mirrors synced): GAP-3 §11.4.85 stress+chaos suite COMPLETE (commits `02d272d`→`d2a60d3`, HRD-122..130): HRD-122 scaffold `commons/stresschaos/`, HRD-123 Gin `/v1/events`, HRD-124 `/v1/compliance`+`/v1/safety_state`, HRD-125 Runner, HRD-126 `pherald listen`, HRD-127 claude_code (testdata fake-claude shims), HRD-128 container/resource-exhaustion (§12.6-safe, live OOM SKIP-with-reason), HRD-129 e2e E81-E88, HRD-130 paired §1.1 mutation gate `tests/test_stress_chaos_mutation_meta.sh`. Full `scripts/e2e_bluff_hunt.sh` now reports **78 PASS / 0 FAIL** (was 60; +18 = E81-E88 + anchor sub-checks); HRD-130 gate **6 PASS / 0 FAIL** (5 mutations load-bearing). Evidence committed under `docs/qa/HRD-12{3,4,5,7,8}-*` + `docs/qa/2026-05-27T135301-gap3-listen-HRD-126/`. **NOTE — HRD-122..130 are now MIGRATED to Fixed.md (Completed)** (suite landed + e2e green + atomic Issues→Fixed migration done in Issues.md r16 / Fixed.md r15). Two anti-bluff issues caught + fixed mid-session by the conductor: HRD-123 flaky oversized-body assertion (§11.4.50 — transport-rejection of an 8 MiB body is valid, not a failure) and E82 stale-anchor (re-captured committed evidence with `HERALD_STRESS_QA_DIR`; assertion `all_4xx_no_panic`→`all_malformed_rejected_no_5xx`). **HRD-132 (FIXED `354b883`, now in Fixed.md):** Runner idempotency window — concurrent same-key replay duplicate-dispatched (2-4 of a 1000× flood) because `events_processed` was archived at Stage 7 while `idempotency.go:64-72` treated SETNX-miss+no-PG-row as fresh (documented §32.1 "Redis-lies-PG-truths"). FIX landed claim-before-dispatch (`INSERT events_processed … ON CONFLICT DO NOTHING` at Stage 2 = authoritative exactly-once dispatch gate) + closed the latent `runner.go:132` `CachedRcpt.WasReplay` race + upgraded the HRD-125 assertion to `sends==1` (evidence `dispatch_exactly_once=1`); wave3 M4 retired (`4f45f26`). Prior r10 (still valid history) — a constitution-compliance + SSoT-hygiene sweep: (1) pulled HelixConstitution `3a085b9`→`3c9c4e9` (gained §11.4.89 background-test / §11.4.90 Obsolete-status / §11.4.91 summary-clarity / §11.4.92 multi-pass-eval / §11.4.93 SQLite-SSoT / §11.4.94 zero-idle-parallel); (2) propagated §11.4.89-94 into CLAUDE.md r12 / AGENTS.md r8 / HERALD_CONSTITUTION.md r6 §108.d-i (commit `83b5e2f`) — inheritance gate 15 PASS / 0 FAIL; (3) built `scripts/mutation_residue_audit.sh` §107.y pre-push residue scanner (`e23933c`) + back-filled §107.y quiescence + `.git/MUTATION_IN_PROGRESS` lockfile into wave2/3/4 gates (`c19a092`); (4) re-verified ALL mutation gates green one-at-a-time — wave2 5/0, wave3 6/0, wave4 5/0, wave4b 5/0, wave6.5 6/0 (wave6 already 4/4), each with Quiescence assertion PASS; (5) §11.4.91 summary-clarity — 7 one-liners fixed (`d24e829`); (6) operator correction — `docs/.workable_items.db` is VERSION-CONTROLLED, NOT gitignored (Herald divergence from parent §11.4.93, `30f9c6d`); (7) GAP-3 §11.4.85 stress+chaos PLAN committed (`a422c53`, HRD-122..130); (8) read-only audits committed (`9c09496` constitution-compliance, `ee63ce6` HRD-obsolescence + qa-coverage); (9) HRD-131 filed (SQLite SSoT Phase 1) in Issues.md. RESOLVED debts: Wave 6.5 mutation-gate M3 stale anchor (`opts.ReplyTo`→`textOpts.ReplyTo`) FIXED; Postgres-SASL e2e carry-over RESOLVED (e2e now 60 PASS / 0 FAIL / 24 SKIP); HRD-101 now filed as a real Fixed.md row. Prior session: v0.4.0 (Wave 6 closed-loop) + v0.5.0 (Wave 6.5 lifecycle) tagged; Wave 7 T1-T5 closed (HRD-110..114). PENDING-OPERATOR: MTProto real-channel automation (Telegram never relays bot-to-bot in groups — `docs/research/telegram-bot-to-bot-constraint.md`, `5267f14`; needs my.telegram.org app_id+app_hash + `/revoke @pherald_qa_bot`). |
| Issues | HRD-008, HRD-015, HRD-019..HRD-025 (Batch B bindings landed — **in_progress**, live-op deferred to Batch C), HRD-029..HRD-056, HRD-081, HRD-085..HRD-090, HRD-115..HRD-121 (Wave 7 T6-T12 pending), HRD-131 (SQLite SSoT Phase 1, OPEN-deferred — blocked on constitution binary) — **37 open** |
| Issues summary | see `Issues.md`. v1.0.0 Batch A migrated HRD-018 / HRD-026 / HRD-027 Issues→Fixed (§11.4.19 atomic). v1.0.0 Batch B (HRD-019..025, §42 flavor constitution bindings) is **in_progress** — each flavor's `internal/bindings/` Pipeline + rule catalogue + PURE detectors + emit-through-audit foundation + §107.x evidence + §11.4.85 stress all landed (`f33b5e4`/`5ebad4f`/`46ca77e`/`f73195f`/`3fb753a`), but live operation-interception (the Subjects the detectors classify) is deferred to Batch C §43 command bodies, so these stay open not Fixed. Wave 7 closed HRD-110/111/112/113/114; HRD-115..121 are the remaining Wave 7 tasks T6-T12. HRD-122..HRD-130 (§11.4.85 stress+chaos suite, GAP-3) + HRD-132 (Runner idempotency-window bug) MIGRATED to Fixed.md. HRD-131 is §11.4.93 SQLite workable-items SSoT migration Phase 1 — **OPEN-deferred** per operator 2026-05-27: Phase 2 adoption blocked on the unimplemented + schema-incompatible constitution migration binary (assessment `36fa90b`); a §11.4.74 constitution-repo task, not a Herald reimplementation. |
| Fixed | HRD-001..HRD-018 (less open), HRD-026, HRD-027, HRD-028, HRD-080, HRD-092..HRD-101, HRD-110..HRD-114, HRD-122..HRD-130, HRD-132 |
| Fixed summary | see `Fixed.md` |
| Continuation | **Resume — v1.0.0 readiness (workstream 3) per plan `docs/superpowers/plans/2026-05-27-v1.0.0-execution.md`; Batch A + Batch B DONE, next is Batch C:** **(1) Batch C — §43 command catalogue HRD-029..056 (IMMEDIATE NEXT UNIT)** — the live CLI command bodies that PRODUCE the Subjects the Batch-B detectors classify (pherald commit-push, sherald destructive-guard, cherald creds-scan, rherald tag-mirror, bherald evidence-capture, etc.), grouped into 5 flavor clusters per the v1.0.0 plan. Closing Batch C is what flips HRD-019..025 from in_progress→Fixed (their bindings have no live Subjects until the command surface lands). **NOTE — the e2e invariant counter is at E96; next free is E97+.** **(2) Batch D — commons_infra HRD-081 + HRD-085..090.** **(3) Batch E — Wave 7 T6-T12 Slack adapter HRD-115..121** (vendored slack-go submodule — **verify the I6 inheritance-gate check after the `.gitmodules` entry**; confirm slack-go v0.16.0 field names `inner.Files`/`socketmode.EventTypeEventsAPI`/`UploadFileV2Parameters`; then qaherald Slack MessengerClient, spec §11.0/§32.2/§43, e2e, mutation gate, Issues→Fixed/v0.6.0/4-mirror push; T5 pre-staged `perChannelConfig("slack")` + blank-import in listen.go — `HERALD_CHANNELS=tgram,slack` currently errors `ErrUnknownChannel` until T6 registers slack, by design; plan `docs/superpowers/plans/2026-05-27-wave7-generic-messenger.md` line 980). **PLAN GOTCHAS:** Wave 7 e2e invariants MUST renumber off E81-E89 — use **E90+** (E89 is now consumed by Batch A's audit write-through); the slack-go submodule addition requires re-running the I6 inheritance-gate check (`tests/test_constitution_inheritance.sh`). Tag cadence: v0.5→v1.0.0 across the batches. **(5) GAP-4 docs/qa back-fill + docs-audit #147** — features lacking `docs/qa/<run-id>/` evidence per the `ee63ce6` qa-coverage audit + comprehensive documentation audit. **DEFERRED / GATED ITEMS:** **§11.4.93 SQLite SSoT migration (HRD-131) — OPERATOR-DEFERRED 2026-05-27:** Phase 2 adoption is blocked on the constitution's workable-items migration binary, which is an unimplemented scaffold (0/7 subcommands) + schema-incompatible with Herald's HRD-NNN pipe-table format; per §11.4.74 Herald may NOT reimplement it — the fix is a constitution-repo task affecting all consumers (assessment `docs/research/workable-items-phase2-assessment-2026-05-27.md`, `36fa90b`). HRD-131 stays OPEN, blocked-on-constitution-binary. MTProto real-channel automation — OPERATOR-DECISION: provide my.telegram.org app_id+app_hash (folds into a qaherald MessengerClient MTProto impl reusing the entire scenario engine; Telegram never relays bot-to-bot in groups — see `docs/research/telegram-bot-to-bot-constraint.md`); operator should also `/revoke @pherald_qa_bot` (leaked plaintext token). §11.4.90 obsolescence — zero current Obsolete rows; convention in place, periodic re-audit pending. (RESOLVED this session (2026-05-28 day-2): v1.0.0 Batch B COMPLETE — all 7 §42 flavor constitution bindings HRD-019..025 landed (`f33b5e4`/`5ebad4f`/`46ca77e`/`f73195f`/`3fb753a`), each `internal/bindings/` Pipeline + rule catalogue + PURE detectors + §107.x evidence + §11.4.85 stress 1200-concurrent, e2e E90-E96 wired (HRD-019..025 stay in_progress — live-op interception is Batch C); constitution pulled `3c9c4e9`→`15cd4bc` + §11.4.95-97 propagated (`d5b9895`, gate 15/0); 3 fixes — #194 audit-test qaRoot t.TempDir guard (`a8ad4f7`), eventbus `PublishedByType` data race → LoadOrStore (`f5d1367`, a real E3 -race regression latent since Batch A), E17 e2e SKIP-guard now requires live PG (`75e7a49`). Prior r13: v1.0.0 Batch A — HRD-018/026/027 closed atomically Issues→Fixed, e2e **81 PASS / 0 FAIL** (E89 wired), `commons_constitution` audit write-through §107 bluff FIXED + admin mode-flip REST; conductor-verified `-race -count=3` + real-PG + §11.4.85 stress 512-row no-lost-writes. Earlier: GAP-3 §11.4.85 stress+chaos suite IMPLEMENTED — e2e 78 PASS / 0 FAIL — AND atomically migrated Issues→Fixed (HRD-122..130 Completed); HRD-132 Runner idempotency-window bug FIXED `354b883` (claim-before-dispatch, exactly-once `sends==1`) + migrated to Fixed.md; constitution §11.4.89-94 propagation + residue scanner; HRD-123 flaky oversized-body assertion + E82 stale-anchor anti-bluff issues fixed. Prior r10 RESOLVED: Wave 6.5 mutation-gate M3 stale anchor; Postgres-SASL e2e FAILs; HRD-101 filed as a real Fixed.md row.) §11.4.87 endless-loop: continue until Issues.md zero-active + CONTINUATION §3 empty + no subagent in flight. NO BLUFF. Push 4 mirrors every 2-3 commits. |

## Table of contents

- [§0. How to use this document](#0-how-to-use-this-document)
- [§1. Snapshot](#1-snapshot)
- [§2. Last commit landed](#2-last-commit-landed)
- [§3. Active work](#3-active-work)
- [§4. Next concrete steps](#4-next-concrete-steps)
- [§5. Long-form pointers](#5-long-form-pointers)

## §0. How to use this document

Paste the following block into any CLI agent (Claude Code / OpenCode / Cursor / Aider / Gemini CLI) to resume Herald work exactly where it was left:

> You are working on the Herald project at `~/Projects/Herald` (also reachable as the `Herald/` submodule of a consuming project). The Helix Universal Constitution lives at `<ancestor>/constitution/` (parent-walk discovery). Read in this order: `CLAUDE.md`, `AGENTS.md`, `README.md`, `docs/guides/HERALD_CONSTITUTION.md`, `docs/guides/CONSTITUTION_INHERITANCE.md`, `docs/specs/mvp/specification.V3.md`. Then read `docs/CONTINUATION.md` (this file) for live state, `docs/Issues.md` for open work, `docs/Status.md` for current phase, `docs/Fixed.md` for closed history. Go workspace builds via `go test ./commons/... ./commons_prefix/... ./commons_messaging/... ./commons_storage/...`. Inheritance gate `tests/test_constitution_inheritance.sh` MUST exit 0 before any commit. Multi-mirror fan-out push to four hosts (GitHub + GitLab + GitFlic + GitVerse) is mandatory per Constitution §103.

## §1. Snapshot

- **Active spec:** `docs/specs/mvp/specification.V3.md` Revision 4 (~4300 lines).
- **Archived specs:** V1 + V2 in `docs/specs/mvp/archive/` (frozen).
- **Go modules:** `commons`, `commons_prefix`, `commons_messaging`, `commons_storage`, `pherald` (all compile + unit tests pass).
- **Container scaffold:** `quickstart/{docker-compose.quickstart.yml, Dockerfile.pherald, otel-config.yaml, .env.example}` for §26.5 quickstart.
- **CLI:** `pherald version --json` returns canonical build info; `serve`/`send`/`doctor`/`migrate`/`subscriber`/`deadletter` stubbed with HRD-NNN pointers.
- **Inheritance gate:** 15 PASS / 0 FAIL. Meta-test ✓.
- **e2e_bluff_hunt:** invariants now run through **E120** (E89 Batch-A; E90-E96 Batch-B bindings; **E97-E120 = Batch C §43 command catalogue** — E97-E100 pherald, E101-E104 sherald, E105-E107 rherald, E108-E109 bherald, E110-E120 cherald; prior E81-E88 GAP-3 stress+chaos). Full e2e CONFIRMED 2026-05-28 = **128 PASS / 0 FAIL** (E3 -race spans 12 packages incl `commons/gitops`; E17 SKIP-with-reason on absent live-PG; E53 flake root-caused + fixed — was a cross-call byte-diff on the dynamic `/v1/safety_state` body). Plus SKIP-with-reason for hardware/credential-absent invariants (live OOM, MTProto, Wave-6 live-loop). Next free invariant id is **E121+**.
- **Stress+chaos mutation gate:** `tests/test_stress_chaos_mutation_meta.sh` (HRD-130) 6 PASS / 0 FAIL (5 load-bearing mutations).
- **Phase:** implementation r1.

## §2. Last commit landed

This commit (V3 r4 + Go scaffold + tracking-doc refresh) closes HRD-009/HRD-009b/HRD-013/HRD-014 (with a Universal §11.4.19 atomic Issues.md → Fixed.md migration) and lands spec V3 §37–§41 (tracker events, workable-item announcement contract, message presentation + Herald Canonical Template, docs/tests completeness, REST API surface). Builds + tests:

```
$ go test ./commons/... ./commons_prefix/... ./commons_messaging/... ./commons_storage/...
ok  	github.com/vasic-digital/herald/commons	1.135s
ok  	github.com/vasic-digital/herald/commons_prefix	0.639s
ok  	github.com/vasic-digital/herald/commons_messaging/channels/null	0.890s
ok  	github.com/vasic-digital/herald/commons_messaging/dispatch/claude_code	1.381s
ok  	github.com/vasic-digital/herald/commons_storage	1.630s
```

## §3. Active work

| ID | Status | What |
|---|---|---|
| HRD-008 | in_progress | Operator-side Quickstart compose validation (Postgres + Redis + OTel + pherald container) — scaffold shipped this commit; live end-to-end run pending operator. |
| HRD-010 | open | commons_storage live (pgx pool + golang-migrate driver + River queue + Redis ACL). |
| HRD-011 | open | Telegram adapter live (telebot SDK + getUpdates long-poll + webhook secret_token). |
| HRD-012 | open | Claude Code dispatcher live (`claude --resume` + parse `<<<HERALD-REPLY>>>`). |
| HRD-015 | open | Inheritance gate I8 invariants for Go scaffold (go.work + commons/types.go + null adapter test passes). |
| HRD-016 | open | REST API surface via Gin Gonic per spec §41. |
| HRD-017 | open | Propagate Universal §11.4.6X spec-versioning + submodule-catalogue-first mandates into the parent constitution. |
| HRD-122..130 | Completed | GAP-3 §11.4.85 stress+chaos suite — IMPLEMENTED + e2e-green (`d2a60d3`); MIGRATED Issues→Fixed (Fixed.md r15). DONE. |
| HRD-132 | Fixed | Runner idempotency-window bug — FIXED `354b883` (claim-before-dispatch at Stage 2, exactly-once `sends==1`, `CachedRcpt.WasReplay` race closed); MIGRATED Issues→Fixed (Fixed.md r15). DONE. |
| HRD-018/026/027 | Fixed | **v1.0.0 Batch A** (`b19cb43`+`854d301`): `commons_constitution` audit write-through (closed the §107 PASS-bluff — table existed, no row written) via `AuditStore`+Memory/Postgres backends + `IDEmitter`; bundle-hash persisted on every event+audit row (026); cherald admin mode-flip REST `GET`/`GET-one`/`PUT /v1/compliance/modes[/:rule_id]` (027). e2e 78→**81/0** (E89). MIGRATED Issues→Fixed (Issues.md r17, 40→37 open). DONE. |
| HRD-131 | OPEN-deferred | §11.4.93 SQLite workable-items SSoT migration Phase 1 filed; **Phase 2 OPERATOR-DEFERRED 2026-05-27** — blocked on the constitution's migration binary (0/7 subcommands + schema-incompatible; §11.4.74 constitution-repo task, not a Herald reimplementation; assessment `36fa90b`). |
| HRD-019..025 | in_progress | **v1.0.0 Batch B COMPLETE (bindings)** — all 7 §42 flavor constitution bindings landed: HRD-019 cherald (`f33b5e4`, E90 `c3e3cff`), HRD-020 sherald (`5ebad4f`, E91 `75e7a49`), HRD-021 bherald (`46ca77e`, E92 `f31615f`), HRD-022 rherald (`f73195f`, E93 `0206f8f`), HRD-023/024/025 pherald/iherald/scherald (`3fb753a`, E94/E95/E96 `74649bb`). Each: `<flavor>/internal/bindings/` Pipeline + rule catalogue + PURE detectors, emit-through Batch-A audit foundation, `-race -count=3` green, §107.x evidence `docs/qa/HRD-0XX-*/`, §11.4.85 stress 1200-concurrent → 1200 emits+audit rows. **Stay in_progress** — live operation-interception (the Subjects the detectors classify) is Batch C §43 command bodies; these flip →Fixed once Batch C lands. |
| #194 / eventbus race / E17 | Fixed | 3 day-2 fixes: #194 `audit_stress_chaos_test.go` qaRoot gated on `HERALD_STRESS_QA_DIR` via `t.TempDir()` (`a8ad4f7`, stops docs/qa noise); `commons_constitution/eventbus.go` `PublishedByType` data race → `sync.Map.LoadOrStore` (`f5d1367`, real E3 -race regression latent since Batch A); E17 e2e SKIP-guard now requires live PG (`75e7a49`, was FAILing instead of SKIPping when PG absent). |
| Batch C (HRD-029..056) | in_progress (C1-C5 IMPLEMENTED 2026-05-28) | **§43 command catalogue — all 5 flavor clusters landed**, each a live CLI command feeding its Batch-B binding (DETECT+classify+exit-non-zero): C1 pherald `commit-push`/`submodule-propagate`/`install-upstreams`/`fetch-guard`/`reopen`/`pre-push` (`502ebf6`,`65c8362`, E97-E100); C2 sherald `destructive-guard`/`constitution-pull`/`force-push-gate`/`mem-budget-watch` (`ed6c675`, E31+E101-E104); C4 rherald `tag-mirror`/`changelog-generate`/`gate-retest` + C5 bherald `test-tier-verify`/`evidence-capture` (`130ec19`, E105-E109); C3 cherald 11 docs/compliance cmds (`aef0207`, E110-E120). Remaining §43: **HRD-034** (sherald backup-snapshot §9.3) + **HRD-047** (scherald status-digest §11.4.45/.56) — not yet implemented. Rows are in_progress (hermetic+e2e proven; flip →Fixed at an operator-live close-out pass, together with HRD-019..025). |
| commons/gitops promotion | Fixed (`4a6b0ec`) | Promoted `pherald/internal/gitops` → shared `commons/gitops` (L0, pure-stdlib: Runner/RepoRoot/FindScript/ParseUpstreams + TagExists/RemoteHasTag/LogSubjects) so all flavor modules reuse the git primitives (§11.4.74). No go.work/go.mod change (every flavor already requires commons). |
| §107 lazy-verifier fix (sherald+cherald) | Fixed (`ed6c675`,`aef0207`) | sherald + cherald `main.go` built the JWT verifier eagerly + `os.Exit(1)` before `root.Execute()` → EVERY subcommand (even `version`) died without `HERALD_AUTH_MODE`. The §43 CLI commands are for bare CI/cron invocation. Fixed by building the verifier LAZILY inside a flavor-local `newServeCmd` (cli.BindServeFlags+RunServe), mirroring pherald. serve still refuses anonymous. Binary-verified. See memory `feedback_verify_subagent_cli_on_binary`. |
| E53 Brotli flake | Fixed (`dfca0be`) | e2e E53 byte-compared two SEPARATE `/v1/safety_state` responses, but the body carries live `current_mem_percent`/`last_mem_sample_at`/`uptime_seconds` → false FAIL. Replaced cross-call diff with a codec-agnostic `grep uptime_seconds` on the (decoded) body; kept the `Content-Encoding:br` policy check. |
| §43 stragglers (HRD-034/047) | in_progress (IMPLEMENTED 2026-05-28, `ba27a0e`) | **§43 catalogue NOW COMPLETE** — `sherald backup-snapshot` (§9.3 os.Link hardlinks → `.gate.recovered`, E121) + `scherald status-digest` (§11.4.45 Status.md sweep + `--apply` Status_Summary regen, E122). All 30 §43 commands (HRD-029..056) across pherald/sherald/cherald/rherald/bherald/scherald implemented + e2e-proven (E97-E122). |
| RESUME-AT (next units) | next | (1) **Batch D** commons_infra `pgxTaskRepository` (`commons_infra/task_repository.go`) — 17 methods currently `ErrUnsupported`: HRD-085 GetByID/Update/Delete, HRD-086 UpdateStatus/UpdateProgress/UpdateHeartbeat/SaveCheckpoint, HRD-087 GetByStatus/GetPendingTasks/CountByStatus/GetTaskHistory, HRD-088 GetStaleTasks/GetByWorkerID, HRD-089 SaveResourceSnapshot/GetResourceSnapshots (+new `task_resource_snapshots` migration). **Needs live PG :24100** (boot via containers submodule + podman socket) for §107 anti-bluff integration evidence — NOT hermetic. HRD-081 is a containers-submodule podman/docker detect (already worked around in boot.go). (2) **Batch E** Wave 7 T6-T12 Slack HRD-115..121 (slack-go submodule + I6 gate; e2e off E123+). (3) operator-live close-out flips HRD-019..025 + §43 rows →Fixed. (4) §11.4.93 SQLite (HRD-131, deferred). Plan `docs/superpowers/plans/2026-05-27-v1.0.0-execution.md`. HEAD=`ba27a0e`. |

## §4. Next concrete steps

0. **~~HRD-018 / 026 / 027 — DONE (v1.0.0 Batch A, `b19cb43`+`854d301`).~~** The `commons_constitution` foundation (Evaluator interface, 12 event-class emit helpers, `constitution_state`+`constitution_bindings` migrations, bundle-hash captureer, mode-ladder) is complete + the `constitution_audit` write-through §107 bluff is closed + cherald admin mode-flip REST is wired; e2e 81/0. **Next §42 unit is Batch B (HRD-019..025, flavor constitution bindings)** — wire each flavor's per-binding evaluation onto the now-complete foundation. Per §11.4.74, record `Catalogue-Check: reuse|extend|no-match` on each Batch B HRD before any Go code lands.

1. **HRD-008 quickstart validation** — On a fresh laptop with Podman or Docker:
   ```
   git clone <Herald repo>
   git submodule update --init
   cd quickstart
   podman build -t herald/pherald:dev -f Dockerfile.pherald ../..
   cp .env.example ../../.env && $EDITOR ../../.env
   podman-compose -f docker-compose.quickstart.yml up -d
   curl --retry 30 --retry-delay 2 http://localhost:24090/readyz
   ```
   The current `pherald serve` returns "not implemented" — HRD-010/HRD-011/HRD-012 must land first to make the live `curl POST /v1/events` succeed end-to-end. Validation reveals which of the spec's assumptions (port ranges, Compose syntax, OTel collector config, Postgres healthcheck) hold against real infrastructure.
2. **HRD-010** — wire `commons_storage/storage.go`'s `MigrationDriver` to golang-migrate; bring pgx + River + Redis client up; add integration tests under `//go:build integration`.
3. **HRD-011** — replace `commons_messaging/channels/tgram/tgram.go` stub with a live implementation against `gopkg.in/telebot.v3` or `github.com/mymmrac/telego`; recorded HTTP fixtures under `testdata/`.
4. **HRD-012** — replace `commons_messaging/dispatch/claude_code/claude_code.go`'s `Dispatch` stub with a real `claude --resume` invocation; capture session UUID; parse `<<<HERALD-REPLY>>>`.
5. **HRD-016** — scaffold `pherald/internal/http/` with Gin routes per V3 §41; wire `pherald serve` to mount the Gin router on `http_port`.
6. **HRD-017** — propagate Universal §11.4.6X new mandates (spec-versioning + submodule-catalogue-first) into the constitution submodule.

## §4a. Wave 6 live-test handoff (T10b — gating tag v0.4.0)

**Operator-supplied credentials required** — Wave 6's e2e invariants E63-E70 land as honest SKIPs until the live closed-loop runs against real Telegram + real Claude Code with real chat messages exchanged.

**Step-by-step.**

1. Export credentials in the shell that will drive the test:
   ```bash
   export HERALD_TGRAM_BOT_TOKEN="<your-bot-token-from-BotFather>"
   export HERALD_TGRAM_CHAT_ID="<numeric-chat-id-where-bot-is-admin>"
   export HERALD_CLAUDE_BIN="$(command -v claude)"   # path to the claude CLI binary
   export HERALD_PROJECT_NAME="Herald"                # or any operator-chosen session name
   ```
2. Pick a `<run-id>` (timestamp + 4-char nonce):
   ```bash
   RUN_ID="$(date -u +%Y-%m-%dT%H-%M-%S)-$(uuidgen | cut -c1-4)"
   QA_DIR="docs/qa/HRD-100-${RUN_ID}"
   mkdir -p "${QA_DIR}/attachments"
   ```
3. Type ONE message in the configured Telegram chat (script reads it via `getUpdates` within 60s).
4. Run the live closed-loop test:
   ```bash
   bash tests/test_wave6_live_loop.sh
   ```
   The script builds `pherald`, observes the original `message_id`, starts `pherald listen --bot-token "$HERALD_TGRAM_BOT_TOKEN" --chat-id "$HERALD_TGRAM_CHAT_ID" --qa-out-dir "$QA_DIR"` in the background, waits up to 45s for a reply with `reply_to_message_id == original`, and exits 0 on PASS.
5. Capture supplementary logs:
   ```bash
   cp /tmp/pherald-w6.log "${QA_DIR}/pherald-listen.log"
   # claude stdout/stderr — paths depend on the journaling setup. Copy whatever
   # the test produced into ${QA_DIR}/claude-stdout.log + claude-stderr.log.
   ```
6. Author a brief `${QA_DIR}/README.md` (5 lines minimum) narrating: who ran it, when, what message was sent, what reply came back, any anomalies. This is NOT auto-generated — operator-written narrative is the §107.x evidence anchor.
7. `git add ${QA_DIR}/ && git commit -m "Wave 6 step 10b: docs/qa/HRD-100-${RUN_ID}/ live closed-loop evidence"` then proceed to T13b (`v0.4.0` tag + 4-mirror push).

**If the script SKIPs** (any env var unset): the test prints `SKIP: <reason>` and exits 0. That is honest §11.4.3 hardware-absent SKIP-with-reason, not a PASS. Tag `v0.4.0` MUST NOT be created until at least one PASS run is committed to `docs/qa/HRD-100-<run-id>/`.

## §4b. qaherald-auto resume runbook (next session — gating tag v0.5.1)

**Status as of 2026-05-23 session end:**

- HEAD `8ad4cb9` on `main`; all 4 mirrors in sync (`github.com:vasic-digital/Herald.git`, `gitlab.com:vasic-digital/herald.git`, `gitflic.ru:vasic-digital/herald.git`, `gitverse.ru:vasic-digital/Herald.git`).
- Tag `v0.4.0` shipped (Wave 6 closed-loop). Tag `v0.5.0` shipped (Wave 6.5 lifecycle implementations — premature: only S1+S2 live evidence + carries the pre-`53a7ad3` envelope-action-guidance bug). Tag `v0.5.1` is the target after qaherald-auto T6-T8 land + full 15-scenario evidence committed.
- Plan: `docs/superpowers/plans/2026-05-23-qaherald-auto.md` (commit `52ce679`).
- Done: T1 skeleton + T2 messenger + T3 scenarios + T4 orchestrator + T5 report (combined commit `8ad4cb9` for T3-T5).
- TODO: **T6 pre-flight + T7 hermetic tests + T8 shell-adapter** then live run + tag.

### Resume prerequisites

The following are **already provisioned** in `~/.zshrc` (export-on-shell-start):

```bash
HERALD_TGRAM_BOT_TOKEN     # @atmosphere_worker_bot — pherald-bot (Telegram bot 8823384001)
HERALD_TGRAM_CHAT_ID       # -4946584787 (ATMOSphere Development group)
HERALD_OPERATOR_IDS        # 2057253161 (the operator's user-id)
HERALD_QA_BOT_TOKEN        # @pherald_qa_bot — 2nd bot (8971749017, privacy DISABLED, member of the group)
```

**The qa-bot token is plaintext in `~/.zshrc`** — operator should `/revoke` via @BotFather after Wave 6.5 close-out and regenerate (the token leaked into one prior session's bash output earlier today, was never committed to git but lives in `/private/tmp/claude-501/...` operator-local files).

The containers `herald-postgres` (port 24100) and `herald-redis` (port 24200) are LIVE — both have been Up since the start of this session; data persists across podman restarts. Schema is at migration v12 (all 12 applied).

### Resume sequence (suggested next-session order)

1. **Pull state + verify**:
   ```bash
   cd ~/Projects/Herald && git pull origin main && git log --oneline -10
   git status --short  # expect clean
   git stash list      # 2 stashes preserved: T8 Wave-5 salvage + premature S3-S5 transcript WIP
   ```

2. **Spawn T6 (pre-flight)** via a fresh subagent — read `docs/superpowers/plans/2026-05-23-qaherald-auto.md` Task 6 in full. Implementation lands at `qaherald/internal/lifecycle/preflight.go` + `preflight_test.go`. 10 gates: pherald-bot reachable via getMe, qa-bot reachable, qa-bot privacy disabled, qa-bot is group member, OPERATOR_IDS contains qa-bot, etc. Each gate has distinct exit code for diagnostics.

3. **Spawn T7 (full hermetic tests)** — extends T3-T5's 12 tests with httptest-based scenario simulation. Coverage targets: every scenario PASS path + every FAIL diagnostic + S9 SKIP path + S14 outbound-attachment sha256 round-trip + S11/S12/S13 inbound-attachment download + Issues.md/Fixed.md fs-mutation assertion.

4. **Spawn T8 (shell-script adapter)** — rewrites `tests/test_wave6.5_lifecycle.sh` to delegate to `qaherald lifecycle` for automated runs. Keeps the original `--manual` flag for the operator-typing interactive UX as a fallback.

5. **Run the live e2e (the §107 watershed)**:
   ```bash
   cd ~/Projects/Herald
   # Add qa-bot user-id to OPERATOR_IDS so S5/S6/S8/S10 succeed
   export HERALD_OPERATOR_IDS="${HERALD_OPERATOR_IDS},8971749017"
   # Re-export from .zshrc (or just open a new shell)
   source ~/.zshrc

   # Build pherald + qaherald
   go build -o /tmp/pherald ./pherald/cmd/pherald
   go build -o /tmp/qaherald ./qaherald/cmd/qaherald

   # Start pherald listen in background with QA journaling
   RUN_ID="$(date -u +%Y-%m-%dT%H-%M-%S)-w6.5live"
   PHERALD_QA_DIR="docs/qa/HRD-101-lifecycle-${RUN_ID}-pherald"
   QAUTO_QA_DIR="docs/qa/HRD-101-lifecycle-${RUN_ID}"
   mkdir -p "${PHERALD_QA_DIR}/attachments" "${QAUTO_QA_DIR}/attachments"
   /tmp/pherald listen --qa-out-dir "${PHERALD_QA_DIR}" --docs-dir docs &
   PHERALD_PID=$!
   trap 'kill -TERM $PHERALD_PID 2>/dev/null' EXIT
   sleep 5

   # Run qaherald lifecycle (T6-T8 must be done first)
   /tmp/qaherald lifecycle \
     --pherald-bot-username=atmosphere_worker_bot \
     --pherald-qa-out-dir="${PHERALD_QA_DIR}" \
     --out="${QAUTO_QA_DIR}" \
     --run-id="${RUN_ID}"
   # Exit 0 on all-PASS or all-PASS-with-S9-SKIP. Non-zero on any FAIL.
   ```

6. **Capture evidence**:
   - `${PHERALD_QA_DIR}/transcript.jsonl` — pherald's view (classifications, dispatch, replies)
   - `${QAUTO_QA_DIR}/transcript.jsonl` — qaherald-auto's view (sends, assertions)
   - `${QAUTO_QA_DIR}/report.md` — Markdown summary
   - `${QAUTO_QA_DIR}/attachments/<sha256>.<ext>` — content-addressed inbound + outbound attachments
   - `docs/Issues.md` + `docs/Fixed.md` — mutated by S5/S6/S8/S10 (and reverted by S10 + S15 cleanups; both files end in valid state)

7. **Author operator README + commit**:
   ```bash
   cat > "${QAUTO_QA_DIR}/README.md" <<EOF
   # HRD-101 Wave 6.5 lifecycle evidence — <run-id>

   <5+ lines narrating: who ran it, when, which scenarios PASSed/FAILed/SKIPped,
   bot reply quality observations, any anomalies. Operator-written, NOT auto-generated.>
   EOF
   git add docs/qa/HRD-101-lifecycle-${RUN_ID}*
   git commit -m "Wave 6.5 step 13: HRD-101 live 15-scenario lifecycle evidence (qaherald-auto)"
   ```

8. **Tag v0.5.1 + 4-mirror push**:
   ```bash
   git tag -a v0.5.1 -m "Wave 6.5 close: full lifecycle live evidence + envelope action-guidance fix"
   git push origin main
   git push origin v0.5.1
   # Verify all 4 mirrors converged on the same SHA + tag
   for mirror in github.com:vasic-digital/Herald.git gitlab.com:vasic-digital/herald.git gitflic.ru:vasic-digital/herald.git gitverse.ru:vasic-digital/Herald.git; do
     git ls-remote "git@${mirror}" v0.5.1
   done
   ```

### Open / known constraints for tomorrow

- **S9 (non-operator Done: rejection)**: requires `HERALD_QA_BOT_TOKEN_NON_OPERATOR` env (a THIRD bot account whose user-id is NOT in OPERATOR_IDS). Without it, S9 emits a SKIP-with-reason. Operator may register a third bot OR accept the SKIP (S9's logic is already unit-tested hermetically in T5's mutation gate).
- **First-scenario CC bootstrap**: ~30s on first inbound message because the Claude Code session must spawn. Subsequent scenarios use the cached session (~5-15s typical).
- **Issues.md mutation race**: if you run `qaherald lifecycle` while ANOTHER pherald listen is also processing in the same checkout, Issues.md may receive double-writes. Single-pherald-listen invocation is the safe default.
- **Postgres-SASL carry-over (RESOLVED 2026-05-27)**: the e2e_bluff_hunt FAILs on Postgres-SASL invariants (E7-E12 + E14-E16 + E37-E42) are FIXED — `9b166c4` (auth env + PG password self-heal + Redis/tenant wiring + honest SKIP-with-reason) + `bdbe9f1` (Runner nil-Redis graceful PG-only fallback). At resolution e2e_bluff_hunt reported 60 PASS / 0 FAIL / 24 SKIP; the later GAP-3 session (`d2a60d3`) raised it to **78 PASS / 0 FAIL** with E81-E88 wired in.

### Safe-shutdown checklist (HISTORICAL — end of 2026-05-23 session; superseded — current HEAD is `d2a60d3`)

| Check | State |
|---|---|
| All committed work pushed to 4 mirrors | YES — HEAD `8ad4cb9` confirmed on github+gitlab+gitflic+gitverse |
| Local working tree clean | YES — `git status --short` empty |
| Stashes preserved | 2 stashes: T8 Wave-5 salvage (older) + premature mid-test S3-S5 transcript WIP (newer, will be regenerated cleanly by qaherald-auto in next session) |
| Background pherald/qaherald processes | NONE running (`ps aux \| grep pherald` empty) |
| Container state | `herald-postgres` + `herald-redis` Up 17 hours — persist data across `podman stop && podman start`; can be left running OR stopped (`podman stop herald-postgres herald-redis`); next session restart is `podman start herald-postgres herald-redis` |
| `/tmp/*.log` files | one operator-token leak shred-deleted earlier today; remaining transient logs contain no credentials (verified) |
| `.zshrc` | contains 4 HERALD_* exports including `HERALD_QA_BOT_TOKEN` plaintext — operator should `/revoke` + regenerate via @BotFather post-Wave-6.5 closure |
| Memory entries | All session findings persisted under `/Users/milosvasic/.claude/projects/-Users-milosvasic-Projects-Herald/memory/` — survive across sessions |

**Machine is safe to shut down.** Resume tomorrow by `cd ~/Projects/Herald && git pull origin main` and start with qaherald-auto T6.

## §5. Long-form pointers

- `docs/specs/mvp/specification.V3.md` — full active spec (Revision 4).
- `docs/specs/mvp/specification.V3.md#30-v2-self-review-log` — every review pass.
- `docs/guides/HERALD_CONSTITUTION.md` — §101..§106 extending Universal.
- `docs/guides/CONSTITUTION_INHERITANCE.md` — parent-discovery + gate.
- `tests/test_constitution_inheritance.sh` — the gate.
- `quickstart/` — HRD-008 scaffold.
- `commons/types.go` — the §11.0 type contract reference.
