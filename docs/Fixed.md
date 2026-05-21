<div align="center">

![Herald](../assets/logo/herald_logo_square_128.png){width=96px height=96px}

</div>

# Herald — Fixed

| Field | Value |
|---|---|
| Revision | 6 |
| Created | 2026-05-20 |
| Last modified | 2026-05-21 |
| Status | active |
| Status summary | HRD-012 Claude Code dispatcher live integration closed atomically per §107 — Plan 2 Tasks 6 + 7 produced two independent live PASS runs (24s + 36s) against real `claude --resume` with exact-match assertions on session_uuid, anchor_path, Outcome, and Summary. HRD-011 Telegram stays open ("code complete, awaiting live E17/E19 evidence") per §107 — operator credentials needed before live PASS captured. Prior: HRD-010 commons_storage + the E14 RLS-bypass production bug fix; Foundation M1/M2/M3. |
| Issues | see `Issues.md` |
| Issues summary | HRD-008/-011 (code complete pending creds) /-015/-016/-018 (in_progress) + HRD-019..HRD-056 + HRD-081 + HRD-085..HRD-090 still open. |
| Fixed | HRD-001..HRD-007, HRD-009, HRD-009b, HRD-010, HRD-012, HRD-013, HRD-014, HRD-017, HRD-080 (and HRD-018 partial — M1 component landed) |
| Fixed summary | spec V1→V3 r7; Go module foundation + Foundation M1/M2/M3; HRD-010 commons_storage live wiring; HRD-012 Claude Code dispatcher live (live PASS evidence captured); universal §11.4.73 + §11.4.74 mandates propagated; I6 gate refined. |
| Continuation | see `CONTINUATION.md`. |

## Table of contents

- [Recently fixed](#recently-fixed)

## Recently fixed

| ID | Type | Criticality | Title | Closed | Commit | Reference |
|---|---|---|---|---|---|---|
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
