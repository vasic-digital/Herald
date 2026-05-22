<div align="center">

![Herald](../assets/logo/herald_logo_square_128.png){width=96px height=96px}

</div>

# Herald — Fixed

| Field | Value |
|---|---|
| Revision | 10 |
| Created | 2026-05-20 |
| Last modified | 2026-05-22 |
| Status | active |
| Status summary | r10 closes **HRD-011 (Telegram live integration)** atomically with live operator-session evidence: real bot delivery `message_id=5` from `@atmosphere_worker_bot` to chat `2057253161` (validated via `getChat`) using the extended wizard's `--bot-token` / `--chat-id` / `--non-interactive` CLI args. Token→getMe→getChat→sendMessage chain confirmed end-to-end in commit `140a2f1` session 2026-05-22. Prior r9 doc-cleanup: renumbered Wave 3a commons_auth scaffold HRD-093 → **HRD-099**. r8 captured Wave 3a close-outs: HRD-028 (cherald `/v1/compliance` live), HRD-098 (sherald `/v1/safety_state` live), HRD-099 (commons_auth scaffold). 8 new e2e invariants landed in Wave 3a; paired §1.1 mutation gate `tests/test_wave3_mutation_meta.sh` (3/3 PASS). |
| Issues | see `Issues.md` — r11 removes HRD-011 from open (atomically Issues→Fixed in this r10 commit). |
| Issues summary | HRD-008/-015/-016/-018 (in_progress) + HRD-019..HRD-027 + HRD-029..HRD-056 + HRD-081 + HRD-085..HRD-090 still open (48 items). |
| Fixed | HRD-001..HRD-007, HRD-009, HRD-009b, HRD-010, HRD-011, HRD-012, HRD-013, HRD-014, HRD-017, HRD-028, HRD-080, HRD-092, HRD-093, HRD-094, HRD-095, HRD-096, HRD-097, HRD-098, HRD-099 (and HRD-018 partial — M1 + M2 components landed) |
| Fixed summary | spec V1→V3 r8; Go module foundation + Foundation M1/M2/M3; HRD-010 commons_storage live wiring; HRD-011 Telegram live (live `message_id=5` evidence 2026-05-22); HRD-012 Claude Code dispatcher live; universal §11.4.73 + §11.4.74 mandates propagated; I6 gate refined; Wave 2 flavor scaffolds (HRD-092..097) closed atomically; Wave 3a substrate (HRD-099 commons_auth JWT verifier) + 2 live REST routes (HRD-028, HRD-098) closed atomically. |
| Continuation | see `CONTINUATION.md`. |

## Table of contents

- [Recently fixed](#recently-fixed)

## Recently fixed

| ID | Type | Criticality | Title | Closed | Commit | Reference |
|---|---|---|---|---|---|---|
| HRD-011 | task | middle | Telegram channel adapter live integration — telebot.v3 vendored (`submodules/telebot`); HealthCheck + Send + Subscribe + outbound_delivery_evidence persistence all live-wired with §107 bluff guards. Closed atomically with operator-session live evidence 2026-05-22: real bot delivery `message_id=5` from `@atmosphere_worker_bot` to chat `2057253161` (validated via `getChat`) using the extended wizard CLI's `--bot-token` (env: HERALD_TGRAM_BOT_TOKEN) + `--chat-id` (validated via `getChat` before persist) + `--non-interactive` flags. Token→getMe→getChat→sendMessage chain confirmed end-to-end in commit `140a2f1` session. | 2026-05-22 | (this commit) | spec V3 §11.1; Catalogue-Check: no-match → vendor telebot.v3 as submodules/telebot per §11.4.74; live evidence: wizard run 2026-05-22 + Telegram Bot API `sendMessage` returning `ok=true, message_id=5` |
| HRD-099 | task | low | commons_auth/ JWT verifier — HMAC + JWKS hybrid + Gin middleware + claims helpers, vendored Herald-internal per §11.4.74 catalogue-check no-match. Catalogue evidence: `docs/catalogue-checks/HRD-099-commons-auth.md`. Renumbered from HRD-093 (Wave 3a executing-time anchor) in the post-Wave-3a doc-cleanup pass; Wave 3a commits `dbbea95..0cc6fad` keep the old HRD-093 in their commit messages as history. | 2026-05-21 | dbbea95 (impl) + r9 doc-cleanup (renumber) | spec V3 §41 + Wave 3 design §4; docs/catalogue-checks/HRD-099-commons-auth.md |
| HRD-098 | task | middle | sherald `/v1/safety_state` live — process-local Aggregator + background mem-sampler (15s tick) + Handler with JWT gate via commons_auth.GinMiddleware. Replaces the Wave 2 501-stub. §107 evidence: e2e E46 (no-auth → 401), E47 (valid HMAC → 200 + current_mem_percent>0 + last_destructive_op=null + uptime_seconds>=1 + open_events=0). Paired mutation M5 (zero-mem Aggregator) proves E47 catches the regression. E48 SKIP-with-reason — destructive-op trigger awaits HRD-033 body. | 2026-05-21 | (this commit) | spec V3 §18.0 + §41 + §44.M; e2e E46-E47 |
| HRD-028 | task | low | cherald `/v1/compliance` live — paginated + filter-aware `constitution_state` pull surface with JWT gate via commons_auth.GinMiddleware. Backed by ConstitutionStore.ListQuery extended with Since/Until/Offset (commit 6276437). §107 evidence: e2e E43 (no-auth → 401), E44 (valid HMAC + empty tenant → 200 + total=0 + page=1 + page_size=50). E45 SKIP-with-reason — cross-binary cherald-reads-pherald-writes deferred to Wave 3b (Runner not live yet). | 2026-05-21 | (this commit) | spec V3 §42.1.5 + §41 + §44.M; e2e E43-E44 |
| HRD-092 | task | middle | commons/cli/ shared CLI scaffold (Wave 2) — NewRootCmd + VersionCmd + ServeCmd (with Middleware hook) + StubCmd + healthz/readyz/metrics handlers. Vendored as Herald-internal per §11.4.74 catalogue-check no-match. As-built evidence: §44.M of spec V3 r8 + docs/catalogue-checks/HRD-092-commons-cli.md. | 2026-05-21 | (this commit) | spec V3 §44.M; Catalogue-Check: no-match → vendor |
| HRD-093 | task | middle | sherald flavor scaffold — System Herald binary serving :24793 with cli.ServeCmd, 5 §43 stubs (HRD-033/034/040/046/056) + /v1/safety_state 501 stub → HRD-098. | 2026-05-21 | (this commit) | spec V3 §18.0 + §44.M; Catalogue-Check: no-match → vendor |
| HRD-094 | task | middle | cherald flavor scaffold — Constitution Herald binary serving :24792 with cli.ServeCmd, 11 §43 stubs + /v1/compliance 501 stub → HRD-028. | 2026-05-21 | (this commit) | spec V3 §18.0 + §44.M; Catalogue-Check: no-match → vendor |
| HRD-095 | task | low | bherald flavor scaffold — Build Herald CLI-only binary with 3 §43 stubs (HRD-035/041/045). | 2026-05-21 | (this commit) | spec V3 §18.0 + §44.M; Catalogue-Check: no-match → vendor |
| HRD-096 | task | low | rherald flavor scaffold — Release Herald CLI-only binary with 3 §43 stubs (HRD-031/032/045). | 2026-05-21 | (this commit) | spec V3 §18.0 + §44.M; Catalogue-Check: no-match → vendor |
| HRD-097 | task | low | iherald + scherald flavor scaffolds — Incident Herald serving :24794 with /v1/webhooks/page → HRD-024 + Scheduled-audit Herald CLI-only with 1 §43 stub (HRD-047). Paired since iherald has zero §43 stubs. | 2026-05-21 | (this commit) | spec V3 §18.0 + §44.M; Catalogue-Check: no-match → vendor |
| HRD-012 | task | middle | Claude Code dispatcher live integration — `claude --resume <UUID> --print <envelope>` exec + `<<<HERALD-REPLY>>>` JSON parse + `claude_code_sessions` persistence per §33. §107 evidence: Plan 2 Task 6 commit `702b5a3` (live PASS 24.24s — real claude CLI round-trip, structured reply parsed, Outcome+Summary non-empty) + Task 7 commit `4718c0e` (live PASS 36.23s — session_state upsert under HeraldSystemTenant with exact-equality assertions on session_uuid + anchor_path + last_response JSONB round-trip). HRD-085..HRD-090 stay open for upstream-defined TaskRepository methods not exercised by the Dispatch+session hot path. | 2026-05-21 | (this commit) | spec V3 §33 + §33.2; Catalogue-Check: no-match → `claude` is external binary not library; extend digital.vasic.database@<pinned> for live pool. |
| HRD-010 | task | middle | commons_storage live wiring — pgx pool + RLS-enforcing WithTenantContext (discovered + fixed RLS-bypass bug via E14 falsifiability) + 9 migrations + background queue (digital.vasic.background bound via pgxTaskRepository) + Redis ACL (digital.vasic.cache) + pherald migrate up/status/down/validate subcommand + 3 new §107 e2e invariants (E14/E15/E16) + HRD-085..090 registered for queue-repository stubs | 2026-05-20 | (this commit) | spec V3 §9.6 + §16; Catalogue-Check: extend digital.vasic.database@<pinned> + digital.vasic.background@2d46dd60 + digital.vasic.cache@<pinned>; Models + Concurrency submodules added |
| HRD-080 | task | low | Refine I6 inheritance-gate invariant from blanket `.gitmodules`-forbidden to "no `constitution/` entry in `.gitmodules`" — paired meta-test `test_i6_refinement_meta.sh` with 3 anti-bluff subtests. Enables Foundation M2/M3 Helix-stack submodule installs. | 2026-05-20 | (this commit) | spec V3 §44.9 |
| HRD-017 | task | middle | Propagate Universal §11.4.73 (main-spec versioning + revision discipline) and §11.4.74 (submodule-catalogue-first discovery) into parent constitution Constitution.md + CLAUDE.md + AGENTS.md | 2026-05-20 | constitution `34a82b3` | Universal §11.4.73, §11.4.74 |
| HRD-014 | task | middle | pherald CLI scaffold — Cobra root + version + 5 stubbed subcommands; `pherald version --json` returns canonical build info | 2026-05-20 | `e627c76` | spec V3 §3 + §18.2 |
| HRD-013 | task | middle | commons_messaging + null:// adapter — full §11.0 Channel contract impl with ring buffer + fail_rate/latency/ceiling URL params + 8-case unit test suite | 2026-05-20 | (this commit) | spec V3 §11.14 |
| HRD-009b | task | low | commons_prefix module — §8.2 deterministic 3-letter prefix algorithm with CamelCase split + collision-resolution via fnv1a32 + table-driven tests | 2026-05-20 | (this commit) | spec V3 §8.2 |
| HRD-009 | task | middle | commons module — full §11.0 Go type contract (Channel + Capabilities + DeliveryEvidence + OutboundMessage + Body + Attachment + Recipient + Action + Priority + Receipt + InboundHandler + InboundEvent + Subscriber + SubscriberAlias + CloudEventEnvelope + TraceContext + Branding + ChannelID + PreferenceSet + …) + Clock abstraction + UUIDv7 helper + DefaultBranding factory | 2026-05-20 | (this commit) | spec V3 §11.0 + §3.5 + §4.3 + §6.3 |
| HRD-007 | task | middle | V3 r3 cross-doc sync + tracking-doc scaffold (Issues/Fixed/Status/Status_Summary/Issues_Summary/Fixed_Summary/CONTINUATION) | 2026-05-20 | `741cccd` | spec V3 §30.8 |
| HRD-006 | task | middle | V3 r2 flavor refinement — 9 flavors × per-channel interaction tables | 2026-05-20 | `f8b8073` | spec V3 §30.7 |
| HRD-005 | task | middle | V3 r1 operator-product layer (§31..§36 + §18.2 expansion) | 2026-05-20 | `e26a8dc` | spec V3 §30.6 |
| HRD-004 | task | middle | V2 r3 — Go type contract closure + operational ops detail | 2026-05-20 | `f4ebba1` | archived V2 §30.5 |
| HRD-003 | task | middle | V2 r2 — close prose↔definition gaps + add operational guidance | 2026-05-19 | `9648545` | archived V2 §30.1 |
| HRD-002 | task | middle | V2 r1 — architectural authoring (CloudEvents/Watermill/OTel/RLS/SLSA/9 flavors) | 2026-05-19 | `96b7cc6` | archived V2 §29 |
| HRD-001 | task | middle | V1 — initial MVP specification + Review section + Recommendations | 2026-05-19 | `b421fe1` | archive V1 §"Review" |
