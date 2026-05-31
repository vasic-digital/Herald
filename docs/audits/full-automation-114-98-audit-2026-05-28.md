<div align="center">
<img src="../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />
</div>

# Herald — §11.4.98 Full-Automation Anti-Bluff Audit

| Field | Value |
|---|---|
| Audit date | 2026-05-28 |
| Authority | HelixConstitution §11.4.98 (commit `6828ff2`) + Herald §108.m (commit `bbf03c8`) |
| Scope | All Herald test surfaces (integration tests, shell scripts, mutation gates, e2e invariants) |
| Release-gate posture | NON-COMPLIANT items have 30 days from this audit before graduating to §11.4.90 Obsolete |
| Deadline | 2026-06-27 (T+30 days) |
| Methodology | Static analysis only — no test executed, no source modified |
| Total tests audited | 794 (783 Go test/bench/example funcs + 11 shell test/e2e scripts) |
| COMPLIANT (hermetic) | 750 Go funcs + 8 shell scripts = **758** |
| COMPLIANT-with-creds-bootstrap | 32 Go funcs + 1 shell script = **33** |
| NON-COMPLIANT-manual-dep | 2 Go funcs + 1 shell script + 1 shell script-mode = **4** |
| STRUCTURALLY-BROKEN | **0** (the former item — `TestDispatch_LiveClaudeInvocation` — was reclassified COMPLIANT-with-creds-bootstrap on 2026-05-31 after its dedicated-test-only-session-UUID fix landed; see the resolution note below) |
| OBSOLETE-CANDIDATE | **0** (no removed code-path tests detected — all NON-COMPLIANT items are rewrite-targets, not deletion-targets) |
| PLAN-ONLY (out of scope) | **0** (no `docs/challenges/` dir exists; `docs/research/` contains 4 prose-only docs counted out of scope) |

## Summary

Herald's test surface is overwhelmingly COMPLIANT under §11.4.98: 758 of the 794 audited tests are fully self-driving — they run hermetically with no external dependency or no human action of any kind. A further 33 tests are `COMPLIANT-with-creds-bootstrap` per the §11.4.98(B) explicit exception: they require one-time credential setup (Telegram bot token + chat id in `.env`, `claude` on PATH, Postgres/Redis containers via Podman/Docker) — but once configured, every subsequent run is fully autonomous. This is the §11.4.98(B) permitted carve-out, NOT a manual-dep.

The audit identifies **exactly 4 NON-COMPLIANT items** that require human action during test execution itself — every one of them is a Telegram-inbound-driven test that polls `getUpdates` for a real operator-typed message. These 4 cannot run in unattended CI and are the entire §11.4.98 release-gate debt. They are:

1. **`TestSubscribe_LiveBotAPI`** (`commons_messaging/channels/tgram/subscribe_integration_test.go`) — 60s window for operator hand-send.
2. **`TestVerticalSlice_TelegramClaudeRoundTrip`** (`commons_messaging/vertical_slice_integration_test.go`) — 150s window for operator hand-send, drives the full Telegram → Claude → Telegram round-trip.
3. **`tests/test_wave6_live_loop.sh`** — 60s window for operator hand-send into the configured chat.
4. **`tests/test_wave6.5_lifecycle.sh --manual` (legacy mode)** — 15 narrated scenarios with `read -r _` prompts between each one. Note: the DEFAULT mode of the same script delegates to `qaherald lifecycle` (a second Telegram bot drives the scenarios) and IS fully self-driving; only the explicit `--manual` legacy path is NON-COMPLIANT.

**Update 2026-05-31 — the former STRUCTURALLY-BROKEN item is RESOLVED.** `TestDispatch_LiveClaudeInvocation` (`commons_messaging/dispatch/claude_code/dispatch_integration_test.go:39-139`) no longer accepts the conductor-colliding `HERALD_CLAUDE_SESSION_UUID`. The 2026-05-28 audit predicted the fix (dedicated test-only session UUID); the code now implements it: the test hard-rejects `HERALD_CLAUDE_SESSION_UUID` with `t.Fatalf` (lines 55-56), requires a DEDICATED `HERALD_CLAUDE_TEST_SESSION_UUID` (SKIP-with-reason when absent, lines 58-60), and hard-fails if that test UUID collides with the conductor's live anchored session (the §11.4.98 rule-2 collision guard via `d.ResolveSession()`, lines 83-84). It is therefore reclassified **COMPLIANT-with-creds-bootstrap** — once a throwaway test-only session UUID is supplied (one-time bootstrap, not human action during execution), the test is fully self-driving and cannot collide with the dev conductor's session.

The next-step plan: **Wave 8 Track B — MTProto user-client harness**. Telegram bots cannot see other bots' messages (proven blocker, docs/research/telegram-bot-to-bot-constraint.md), so a 2nd-bot automation driver is impossible. The remaining path is a `gotd/td` user-client that posts messages on behalf of a real Telegram user account; this drives all 4 NON-COMPLIANT items in one fix. Operator-pending: see `docs/requirements/blockers/missing_env_variables.md` for the credentials needed (`TG_API_ID`, `TG_API_HASH`, user phone, session file).

## COMPLIANT tests (hermetic — no creds, no human action)

### Go tests — fully hermetic (sampled by package)

These are the default-hermetic majority. Every test below runs with `go test ./...` (no `-tags=integration`), no env vars, no human input, no external services. The audit confirms this by absence: each file uses only Go stdlib, `httptest`, in-process loopback, mocks, or the `commons/stresschaos` scaffold. Total: ~750 funcs across the 129 non-integration `_test.go` files.

| Test surface | Module / file pattern | Why COMPLIANT |
|---|---|---|
| Commons L0 | `commons/*_test.go`, `commons/cli/*_test.go`, `commons/gitops/*_test.go`, `commons/stresschaos/*_test.go` | Pure-Go unit tests; `commons/cli/serve_dual_test.go` + `h3_test.go` etc. use `httptest` against in-process Gin server — no real network. |
| Prefix | `commons_prefix/prefix_test.go` | Algorithm test, no I/O. |
| Auth | `commons_auth/verifier_test.go` | JWT verifier exercised against in-test-generated keys. |
| TLS | `commons_tls/cert_test.go` | Self-signed cert generation + parse round-trip. |
| Storage | `commons_storage/migrations_test.go`, `storage_test.go`, `resource_stress_chaos_test.go` | Migration parsing + offline checks; no DB. |
| Infra (default) | `commons_infra/boot_test.go`, `clients_test.go`, `task_repository_test.go` | Mock-Pool tests; `_integration_test.go` siblings are tagged-out by default. |
| Constitution | `commons_constitution/audit_test.go`, `bundle_test.go`, `cloudevents_test.go`, `emit_test.go`, `evaluator_test.go`, `eventbus_test.go`, `integration_test.go`, `ladder/memory_test.go`, `state/memory_test.go`, plus `audit_stress_chaos_test.go` | In-memory store implementations + `commons/stresschaos` scaffold. |
| Messaging — channels (default) | `commons_messaging/channels/channel_test.go`, `inbox_test.go`, `registry_test.go`, `selffilter_test.go`, `null/null_test.go`, `slack/*_test.go`, `tgram/attachments_test.go`, `tgram/export_test.go`, `tgram/send_ratelimit_test.go`, `tgram/send_reply_test.go`, `tgram/send_security_test.go`, `tgram/subscribe_test.go`, `tgram/subscribe_lifecycle_test.go`, `tgram/webhook_test.go`, `tgram/stress_chaos_test.go` | Hermetic: in-process httptest fault-injector for the stress/chaos suite; no live API. |
| Messaging — dispatch (default) | `commons_messaging/dispatch/claude_code/bootstrap_test.go`, `claude_code_test.go`, `dispatch_stress_chaos_test.go`, `export_test.go` | Mock `claude` subprocess via fake binary in `t.TempDir()`. |
| pherald | all `pherald/cmd/pherald/*_test.go`, `pherald/internal/{bindings,http,inbound,runner,wizard}/**/*_test.go` (43 funcs) | Cobra unit tests, httptest, in-memory dispatcher fakes, fake redis/PG via `fakes_test.go`. |
| qaherald | all `qaherald/cmd/qaherald/*_test.go`, `qaherald/internal/{herald,lifecycle,messenger,report,scenario,transcript}/**/*_test.go` | Unit tests + builder/preflight harness; the live-bot leg is gated behind `_test.go` env checks. |
| Flavor binaries | `{bherald,cherald,iherald,rherald,scherald,sherald}/cmd/*/*_test.go`, `{...}/internal/**/*_test.go` | Cobra unit tests + safety/compliance/bindings unit tests, all hermetic. |

### Shell scripts — fully hermetic (no creds, no human action)

| Script | Why COMPLIANT |
|---|---|
| `tests/test_constitution_inheritance.sh` | Parent-walk + grep against checked-in files. |
| `tests/test_constitution_inheritance_meta.sh` | Paired §1.1 mutation gate — snapshots → mutates Constitution.md → asserts gate FAILs → restores. Autonomous. |
| `tests/test_i6_refinement_meta.sh` | Paired §1.1 mutation gate against `.gitmodules`. Autonomous. |
| `tests/test_i8_usability_meta.sh` | Paired §1.1 mutation gate against CLAUDE.md / AGENTS.md / HERALD_CONSTITUTION.md. Autonomous. |
| `tests/test_wave2_mutation_meta.sh` | Paired §1.1 mutation gate against flavor binaries (sherald/cherald/iherald/bherald/rherald/scherald) + e2e_bluff_hunt invariants E19-E33. Autonomous. |
| `tests/test_wave3_mutation_meta.sh` | Paired §1.1 mutation gate for Wave 3a JWT verifier + pherald Runner pipeline (E35-E47). Autonomous. |
| `tests/test_wave4_mutation_meta.sh` | Paired §1.1 mutation gate for Wave 4a HTTP/3 + Brotli + Alt-Svc + TLS-1.3 (E49-E55). Autonomous. |
| `tests/test_wave4b_mutation_meta.sh` | Paired §1.1 mutation gate for Wave 4b TOON content negotiation (E56-E62). Autonomous. |
| `tests/test_wave6_mutation_meta.sh` | Paired §1.1 mutation gate for Wave 6 inbound runtime + CC headless bridge. Hermetic detectors only (per file comment: "no live Telegram / claude binary needed"). Autonomous. |
| `tests/test_wave6.5_mutation_meta.sh` | Paired §1.1 mutation gate for Wave 6.5 ticket-lifecycle (T1..T6). Hermetic. Autonomous. |
| `tests/test_wave7_mutation_meta.sh` | Paired §1.1 mutation gate for Wave 7 generic messenger framework (registry + selffilter + Slack adapter). Hermetic. Autonomous. |
| `tests/test_stress_chaos_mutation_meta.sh` | Paired §1.1 mutation gate for the GAP-3 §11.4.85 stress + chaos suite (HRD-130, 5 mutations). Hermetic detectors. Autonomous. |

## COMPLIANT-with-creds-bootstrap tests (§11.4.98(B) explicit exception)

These require one-time credential bootstrap OUTSIDE test execution. Once configured (`.env` + `~/.bashrc` exports + `claude` on PATH + container runtime up), every subsequent run is fully autonomous. This is the §11.4.98(B) permitted exception and is **COMPLIANT** per the rule.

### Go integration tests (creds-bootstrap, fully autonomous once configured)

| Test | File | Bootstrap requirement (one-time) | Why no human action during execution |
|---|---|---|---|
| `TestHealthCheck_LiveBotAPI` | `commons_messaging/channels/tgram/healthcheck_integration_test.go` | `HERALD_TGRAM_BOT_TOKEN` + `HERALD_TGRAM_CHAT_ID` in env | Pure outbound `getMe` round-trip. PROVEN compliant in `docs/qa/HRD-LIVE-20260528T082128Z/01_outbound/` (PASS 0.58s, autonomous). |
| `TestSend_LiveBotAPI` | `commons_messaging/channels/tgram/send_integration_test.go` | `HERALD_TGRAM_BOT_TOKEN` + `HERALD_TGRAM_CHAT_ID` | Pure outbound `sendMessage`. PROVEN compliant 2026-05-28 (PASS 0.32s, message_id=76 returned by Telegram). |
| `TestSend_PersistsDeliveryEvidence` | `commons_messaging/channels/tgram/persist_integration_test.go` | `HERALD_TGRAM_BOT_TOKEN` + `HERALD_TGRAM_CHAT_ID` + Podman/Docker | Outbound + DB persistence round-trip. No human action. |
| `TestBootstrapSession_LiveClaudeInvocation` | `commons_messaging/dispatch/claude_code/bootstrap_integration_test.go` | `claude` (or `HERALD_CLAUDE_BIN`) on PATH | `claude --resume <freshly-bootstrapped-UUID>` round-trip. PROVEN compliant 2026-05-28 (PASS 12.10s). |
| `TestDispatch_LiveClaudeInvocation` | `commons_messaging/dispatch/claude_code/dispatch_integration_test.go:39-139` | `claude` (or `HERALD_CLAUDE_BIN`) on PATH + `HERALD_CLAUDE_PROJECT_NAME` + a DEDICATED `HERALD_CLAUDE_TEST_SESSION_UUID` | Reclassified from STRUCTURALLY-BROKEN on 2026-05-31. Rejects `HERALD_CLAUDE_SESSION_UUID` (lines 55-56 `t.Fatalf`), requires a throwaway test-only session UUID (lines 58-60), and hard-fails on conductor-session collision via `d.ResolveSession()` (lines 83-84, §11.4.98 rule-2). Once a dedicated test UUID is supplied, fully self-driving — no human action, no conductor collision. |
| `TestDispatch_PersistsSessionState` | `commons_messaging/dispatch/claude_code/persist_integration_test.go` | `HERALD_CLAUDE_BIN` + `HERALD_CLAUDE_PROJECT_NAME` + `HERALD_CLAUDE_SESSION_UUID` + Podman/Docker | Dispatch + DB persistence round-trip. No human action. (Historical caveat: the prior UUID-collision risk was specific to `TestDispatch_LiveClaudeInvocation`, now resolved via the dedicated test-only UUID — see its row above.) |
| `TestLiveBoot_PostgresOnly` | `commons_infra/boot_integration_test.go` | Podman/Docker daemon reachable | Boots quickstart Postgres container + TCP-probes 127.0.0.1:24100. Fully autonomous. |
| `TestUp_PopulatesPool` | `commons_infra/clients_integration_test.go` | Podman/Docker | Boots PG + opens pgx pool. Autonomous. |
| `TestUp_PopulatesRedis_TTLRoundTrip` | `commons_infra/clients_integration_test.go` | Podman/Docker | Boots PG+Redis + Set/Get/TTL-expire/Exists=false round-trip. Autonomous. |
| `TestUp_PopulatesQueue_EnqueueDequeueRoundTrip` | `commons_infra/clients_integration_test.go` | Podman/Docker | Boots PG + Enqueue/Dequeue background_tasks round-trip. TRUNCATEs queue for isolation. Autonomous. |
| `TestRepo*_RoundTrip` (×14) | `commons_infra/task_repository_integration_test.go` | Podman/Docker | 14 funcs exercising pgxTaskRepository methods (HRD-085..089) against live PG. Autonomous. |
| `TestRLS_TenantIsolation_RoundTrip` | `commons_storage/storage_integration_test.go` | Podman/Docker | RLS isolation exact-1-row read-back across 2 tenants. Autonomous. |
| `TestPostgresStore_RecordAndGet` + 5 siblings | `commons_constitution/postgres_integration_test.go` | Podman/Docker | 6 funcs: constitution state + audit + ladder Postgres backends. Autonomous. |
| `TestClient_LiveGetMe` | `qaherald/internal/tgram/client_test.go` | `HERALD_TGRAM_BOT_TOKEN` | qaherald's own `getMe` round-trip. Autonomous. |
| `TestTgram_Chaos_GetUpdatesPollerResilience` | `commons_messaging/channels/tgram/stress_chaos_test.go` | None (uses `httptest` server — NOT live Telegram) | Hermetic; listed here for completeness because the file mentions stress/chaos. Actually default-hermetic COMPLIANT. |

### Shell script (creds-bootstrap, fully autonomous once configured)

| Script | Bootstrap requirement | Why COMPLIANT |
|---|---|---|
| `tests/test_resource_stress_chaos.sh` | Podman/Docker + opt-in `HERALD_STRESS_LIVE_OOM=1` | The opt-in flag is a host-safety gate (§12.6) — not a manual-dep during execution. Once set, the script runs container OOM + connection-churn scenarios autonomously. SKIPs-with-reason if creds/flag absent (per §11.4.3). |
| `tests/test_wave6.5_lifecycle.sh` (DEFAULT mode) | `HERALD_TGRAM_BOT_TOKEN` + `HERALD_TGRAM_CHAT_ID` + `HERALD_QA_BOT_TOKEN` (2nd Telegram bot) + `HERALD_OPERATOR_IDS` + `claude` on PATH | DEFAULT mode delegates the 15-scenario lifecycle to `qaherald lifecycle` — the qa-bot posts each scenario's input via Telegram, reads pherald-bot's replies, asserts wire-byte evidence, all without human typing. The `--manual` flag flips to the legacy operator-typing path (see NON-COMPLIANT below). The `--manual` flag flips to the legacy operator-typing path (see NON-COMPLIANT row below). |

### E-invariants in `scripts/e2e_bluff_hunt.sh` (88 total, E1-E88)

`scripts/e2e_bluff_hunt.sh` is the canonical end-to-end smoke. Of its 88 invariants (E1-E88):

- **E1-E12, E19-E33, E35-E48, E49-E62, E81-E88** (~70 invariants): hermetic or creds-bootstrap. COMPLIANT.
- **E13-E18** (6 invariants): creds-bootstrap (live PG / Redis / Telegram outbound / Claude Code). COMPLIANT.
- **E34** (vertical slice): manual-dep — wraps `TestVerticalSlice_TelegramClaudeRoundTrip`. The e2e harness SKIPs E34 unless `HERALD_TGRAM_LIVE_INBOUND=1` is set (signalling an attended session). Classified NON-COMPLIANT via the underlying Go test.
- **E63-E70** (Wave 6 inbound): currently SKIP-with-reason pending T10b live closed-loop evidence; E70 explicitly gated behind `HERALD_W6_LIVE_LOOP=1` because `tests/test_wave6_live_loop.sh` is an ATTENDED test. Classified NON-COMPLIANT via the underlying script.
- **E71-E80** (Wave 6.5 lifecycle): currently SKIP-with-reason pending the full S1..S15 evidence dir. The new automated mode (qaherald lifecycle) will close these once an operator runs it; until then SKIP.

## NON-COMPLIANT-manual-dep tests (require human action during execution)

| Test | File | Manual action required | Rewrite plan | Wave 8 Track ref |
|---|---|---|---|---|
| **`TestSubscribe_LiveBotAPI`** | `commons_messaging/channels/tgram/subscribe_integration_test.go` (lines 30-58) | Operator MUST hand-send a real Telegram message into `HERALD_TGRAM_CHAT_ID` within a 60s `context.WithTimeout` window — line 57 fails the test if `got.Load() == 0`. Source comment line 24: *"requires the operator to hand-send a message"*. | Replace the 60s polling window with an MTProto user-client (`gotd/td`) that programmatically posts a fixture message on behalf of a real Telegram user account; assert the existing `got.Add(1)` invariant inside the handler. | Wave 8 Track B (MTProto user-client harness). Source comment on line 57 already cites this as the load-bearing §107 invariant; the rewrite changes the **driver**, not the assertion. |
| **`TestVerticalSlice_TelegramClaudeRoundTrip`** | `commons_messaging/vertical_slice_integration_test.go` (lines 70-280) | Operator MUST hand-send a Telegram message within a 150s window (line 270 fails: *"VS: handler never invoked — operator did not hand-send a message within the 150s window"*). Source comment lines 19-21: *"The operator MUST hand-send a Telegram message to the configured chat within the 150s window"*. | Same fix: MTProto user-client posts the fixture message, the existing 3-pronged assertion (`inboundCount > 0`, `dispatchOK`, `outboundOK`) runs unchanged. | Wave 8 Track B. The same MTProto driver that fixes `TestSubscribe_LiveBotAPI` automatically closes this one. |
| **`tests/test_wave6_live_loop.sh`** | `tests/test_wave6_live_loop.sh` (lines 53-58 narrate the precondition; lines 64-92 poll for the message) | Operator MUST type a single message in chat `HERALD_TGRAM_CHAT_ID` while the script polls `getUpdates` for 60s; line 94 hard-FAILs if no human-typed message observed: *"no subscriber-typed message observed in 60s"*. | Replace the manual prompt + curl-poll block with the MTProto user-client harness driving the inbound side. `scripts/e2e_bluff_hunt.sh` E70 already gates execution behind `HERALD_W6_LIVE_LOOP=1` to honestly SKIP-with-reason in unattended runs (a §11.4.3 + §11.4.98 honest report). | Wave 8 Track B. Once the harness lands, drop the `HERALD_W6_LIVE_LOOP` opt-in (the test becomes routinely runnable). |
| **`tests/test_wave6.5_lifecycle.sh --manual`** (legacy mode only) | `tests/test_wave6.5_lifecycle.sh` (lines 156-346 — `--manual` mode; line 210 `read -r _` after each `narrate` prompt; 15 such prompts S1..S15) | Operator MUST press ENTER + hand-send a specific Telegram message into the chat for each of 15 scenarios; line 211 `read -r _` blocks until human ENTER. Default mode is COMPLIANT — only the explicit `--manual` flag (or `HERALD_LIFECYCLE_MANUAL=1`) selects this path. | **NO REWRITE NEEDED — already replaced by the default automated mode.** The legacy manual block is preserved verbatim only for forensic / fallback use (e.g. a brand-new operator without the qa-bot set up). Per §11.4.90 the right next step is to mark the `--manual` mode `Obsolete (→ Fixed.md)` once the qa-bot path is validated on a clean checkout; until then it stays as a fallback. | Wave 8 housekeeping: §11.4.90 obsolescence audit of the `--manual` legacy code path once `HERALD_QA_BOT_TOKEN` is the documented default. |

## STRUCTURALLY-BROKEN (none — the one former item is RESOLVED)

The 2026-05-28 audit carved out a single STRUCTURALLY-BROKEN item, `TestDispatch_LiveClaudeInvocation`. **It was fixed and reclassified COMPLIANT-with-creds-bootstrap on 2026-05-31** — the count above is now 0. The table below is retained as a forensic record of the original finding and its resolution.

| Test | File | Original failure mode (2026-05-28) | Root cause | Resolution (2026-05-31) |
|---|---|---|---|---|
| **`TestDispatch_LiveClaudeInvocation`** | `commons_messaging/dispatch/claude_code/dispatch_integration_test.go:39-139` | FAILed with exit code -1 + empty stdout when operator supplied `HERALD_CLAUDE_SESSION_UUID` equal to the conductor's own active dev-session UUID (proven 2026-05-28 in `docs/qa/HRD-LIVE-20260528T082128Z/05_dispatch_live/` — FAIL after 180.02s). | Two different processes calling `claude --resume <same-UUID>` collide; one wins, the other exits -1. NOT a manual-dep — the test PASSes against a dedicated test-only session UUID. | **DONE.** The test now (a) hard-rejects `HERALD_CLAUDE_SESSION_UUID` via `t.Fatalf` (lines 55-56), (b) requires a DEDICATED `HERALD_CLAUDE_TEST_SESSION_UUID`, SKIP-with-reason when absent (lines 58-60), and (c) hard-fails if that test UUID collides with the conductor's live anchored session via `d.ResolveSession()` (lines 83-84, §11.4.98 rule-2 collision guard). Reclassified COMPLIANT-with-creds-bootstrap — see that table above. |

## OBSOLETE-CANDIDATE (none detected)

No NON-COMPLIANT test exercises a removed or superseded code path. All 4 NON-COMPLIANT items (plus the former STRUCTURALLY-BROKEN item, now resolved + reclassified COMPLIANT-with-creds-bootstrap on 2026-05-31) target production code that is live and load-bearing (`tgram.Subscribe` long-poll loop, `claude_code.Dispatch`, `pherald listen` inbound runtime, the full §32.6 ticket-lifecycle pipeline). The rewrite path is **swap the driver, keep the assertions** — not delete-and-replace.

If, after Wave 8 Track B lands, any of the legacy operator-typing prompts in `tests/test_wave6.5_lifecycle.sh --manual` is decided to be a maintenance burden, it should graduate to §11.4.90 `Obsolete (→ Fixed.md)` with Reason `superseded-by-design-change` and Superseding-item pointing at the qaherald-driven automated mode.

## PLAN-ONLY (out of scope — for completeness)

`docs/research/` carries 4 prose-only research documents with NO executable tests:

- `docs/research/constitution-compliance-audit-2026-05-27.md`
- `docs/research/hrd-obsolescence-and-qa-coverage-audit-2026-05-27.md`
- `docs/research/telegram-bot-to-bot-constraint.md` (the structural blocker cited above)
- `docs/research/workable-items-phase2-assessment-2026-05-27.md`

These emit no PASS/FAIL and are correctly out of §11.4.98 scope. `docs/research/protocols/` (58 sub-docs) is also prose-only and out of scope.

No `docs/challenges/` directory exists in this repo.

## Release-gate timeline

| Date | Milestone |
|---|---|
| **2026-05-28 (today)** | Audit published. NON-COMPLIANT items: 4. Release-gate clock starts. |
| 2026-06-05 (T+8d) | Wave 8 Track B planning checkpoint — MTProto harness HRD-NNN filed; operator credentials request (`TG_API_ID`, `TG_API_HASH`, user phone) acknowledged. |
| 2026-06-13 (T+16d) | Wave 8 Track B implementation midpoint — harness skeleton + 1 of 4 NON-COMPLIANT items rewritten end-to-end (canonical target: `TestSubscribe_LiveBotAPI`). |
| **2026-05-31 (T+3d, landed early)** | `STRUCTURALLY-BROKEN TestDispatch_LiveClaudeInvocation` FIXED (dedicated test-only session UUID, conductor-collision guard) and reclassified COMPLIANT-with-creds-bootstrap — ahead of the predicted T+23d closeout. STRUCTURALLY-BROKEN count → 0. |
| 2026-06-20 (T+23d) | Wave 8 Track B implementation closeout — all 4 NON-COMPLIANT items rewritten. (The `TestDispatch_LiveClaudeInvocation` fix originally bundled here already landed 2026-05-31, see above.) |
| **2026-06-27 (T+30d)** | **§11.4.98 release-gate deadline.** All 4 NON-COMPLIANT items MUST either: (a) be rewritten autonomous and PASS in CI, OR (b) be migrated to §11.4.90 `Obsolete (→ Fixed.md)` with full forensic detail (Since, Reason, Superseding-item, Triple-check evidence per §11.4.6). No third option exists. |

**Blocker on the timeline:** Wave 8 Track B is blocked on operator-supplied Telegram user-account credentials. The blocker is fully documented at `docs/requirements/blockers/missing_env_variables.md`. Until those credentials land, the §11.4.3 SKIP-with-reason posture holds — the audit honestly reports the items as NON-COMPLIANT-but-honestly-skipped, not as PASS-bluffs.

## Cross-references

| Anchor | Path / commit |
|---|---|
| HelixConstitution §11.4.98 (canonical) | `../constitution/Constitution.md` commit `6828ff2` |
| Herald §108.m (project-binding restatement) | `docs/guides/HERALD_CONSTITUTION.md` r8 §108.m, commit `bbf03c8` |
| Live-evidence README (2026-05-28 run) | `docs/qa/HRD-LIVE-20260528T082128Z/README.md` |
| Missing-env blocker | `docs/requirements/blockers/missing_env_variables.md` |
| Telegram bot-to-bot wall (Wave 7→8 driver constraint) | `docs/research/telegram-bot-to-bot-constraint.md` |
| §107 covenant (Herald) | `CLAUDE.md` §"End-user-usability covenant" + `docs/guides/HERALD_CONSTITUTION.md` §107 |
| §107.x docs/qa evidence mandate | `CLAUDE.md` §107.x + Helix §11.4.83 |
| §107.y working-tree quiescence (mutation-gate safety) | `CLAUDE.md` §107.y + Helix §11.4.84 |
| §11.4.90 Obsolete status mechanism | Helix §11.4.90, restated in `CLAUDE.md` |
| §11.4.4 test-interrupt-on-discovery | Helix §11.4.4 (PASS-bluff discovery halts the cycle) |
| §11.4.3 explicit SKIP-with-reason | Helix §11.4.3 — the only honest non-PASS posture in absence of dependency |

---

**Audit posture statement.** Herald is, to within the 4 NON-COMPLIANT items above, §11.4.98-compliant today (the former 1 STRUCTURALLY-BROKEN item was fixed + reclassified COMPLIANT-with-creds-bootstrap on 2026-05-31). The 4 outstanding items are fully scoped to a single Wave (8 Track B) and a single dependency (operator-supplied Telegram user-account credentials). No structural rearchitecture is required; the rewrites swap drivers and keep assertions. The release-gate clock is honest: 30 days, no extension hatch, two paths (rewrite OR §11.4.90 Obsolete). The DEFAULT path of `tests/test_wave6.5_lifecycle.sh` (automated qaherald-driven) is COMPLIANT today; only its explicit legacy `--manual` mode counts toward the debt.

— end of audit —
