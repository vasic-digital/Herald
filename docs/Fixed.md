# Herald — Fixed

| Field | Value |
|---|---|
| Revision | 2 |
| Created | 2026-05-20 |
| Last modified | 2026-05-20 |
| Status | active |
| Status summary | First-implementation cycle r1 landed: Go scaffold across 5 modules, null:// adapter fully working, migrations + types complete, pherald CLI compiles + `version` subcommand works end-to-end. Spec V3 r4 with five new operator-mandated sections (§37–§41). |
| Issues | see `Issues.md` |
| Issues summary | HRD-008/-010/-011/-012/-015/-016/-017 still open or in-progress. |
| Fixed | HRD-001..HRD-007 (prior); HRD-009, HRD-009b, HRD-013, HRD-014 (this commit) |
| Fixed summary | spec evolution V1→V3 r4 complete; Go module foundation (commons + commons_prefix + commons_messaging + commons_storage + pherald) landed with passing unit tests. |
| Continuation | see `CONTINUATION.md` for the live HRD-008 quickstart-validation path. |

## Table of contents

- [Recently fixed](#recently-fixed)

## Recently fixed

| ID | Type | Criticality | Title | Closed | Commit | Reference |
|---|---|---|---|---|---|---|
| HRD-014 | task | middle | pherald CLI scaffold — Cobra root + version + 5 stubbed subcommands; `pherald version --json` returns canonical build info | 2026-05-20 | (this commit) | spec V3 §3 + §18.2 |
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
