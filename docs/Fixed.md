# Herald — Fixed

| Field | Value |
|---|---|
| Revision | 3 |
| Created | 2026-05-20 |
| Last modified | 2026-05-20 |
| Status | active |
| Status summary | First-implementation cycle r1 landed (5 Go modules, null:// adapter working, migrations + Cobra CLI). Spec V3 r5: §37–§41 + §42 constitution-flavor binding catalogue. HRD-017 closed (universal §11.4.73 + §11.4.74 mandates landed in constitution commit 34a82b3). |
| Issues | see `Issues.md` |
| Issues summary | HRD-008/-010/-011/-012/-015/-016/-018..-028 open. |
| Fixed | HRD-001..HRD-007, HRD-009, HRD-009b, HRD-013, HRD-014, HRD-017 |
| Fixed summary | spec V1→V3 r5; Go module foundation; universal §11.4.73 + §11.4.74 mandates propagated into constitution submodule. |
| Continuation | see `CONTINUATION.md`. |

## Table of contents

- [Recently fixed](#recently-fixed)

## Recently fixed

| ID | Type | Criticality | Title | Closed | Commit | Reference |
|---|---|---|---|---|---|---|
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
