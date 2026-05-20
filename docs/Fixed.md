# Herald — Fixed

| Field | Value |
|---|---|
| Revision | 1 |
| Created | 2026-05-20 |
| Last modified | 2026-05-20 |
| Status | active |
| Status summary | Retrospective tracker established 2026-05-20 (V3 r3) for spec-revision work HRD-001..HRD-006 already shipped. Going forward every closed item migrates here from `Issues.md` per Universal §11.4.19 atomic-migration mandate. |
| Issues | n/a (see `Issues.md`) |
| Issues summary | — |
| Fixed | HRD-001..HRD-006 (six items retroactively recorded) |
| Fixed summary | V1 MVP authoring → V2 architecture (3 revisions) → V3 operator-product (2 revisions); ~3900 lines of spec across V1+V2+V3; all commits pushed to all four Herald mirrors. |
| Continuation | regenerate `Fixed_Summary.md` whenever this file changes; manual until the auto-sync wrapper exists. |

## Table of contents

- [Recently fixed](#recently-fixed)
- [HRD-001 — V1 MVP specification](#hrd-001--v1-mvp-specification)
- [HRD-002 — V2 r1 architecture](#hrd-002--v2-r1-architecture)
- [HRD-003 — V2 r2 self-review](#hrd-003--v2-r2-self-review)
- [HRD-004 — V2 r3 self-review](#hrd-004--v2-r3-self-review)
- [HRD-005 — V3 r1 operator-product layer](#hrd-005--v3-r1-operator-product-layer)
- [HRD-006 — V3 r2 flavor refinement](#hrd-006--v3-r2-flavor-refinement)

## Recently fixed

| ID | Type | Criticality | Title | Closed | Commit | Reference |
|---|---|---|---|---|---|---|
| HRD-006 | task | middle | V3 r2 — refine 9 flavors for richer channel interaction | 2026-05-20 | `f8b8073` | spec V3 §30.7 |
| HRD-005 | task | middle | V3 r1 — operator-product layer (§31..§36 + §18.2 expansion) | 2026-05-20 | `e26a8dc` | spec V3 §30.6 |
| HRD-004 | task | middle | V2 r3 — define remaining Go types + operational ops (workers, hot-reload, API paths, …) | 2026-05-20 | `f4ebba1` | archived V2 §30.5 |
| HRD-003 | task | middle | V2 r2 — close prose↔definition gaps; add operational guidance (quickstart, DR, retention, costs) | 2026-05-19 | `9648545` | archived V2 §30.1 |
| HRD-002 | task | middle | V2 r1 — architectural authoring (CloudEvents, Watermill, OTel, RLS, SLSA, 9 flavors) | 2026-05-19 | `96b7cc6` | archived V2 §29 |
| HRD-001 | task | middle | V1 — initial MVP specification + Review section + Recommendations | 2026-05-19 | `b421fe1` (and earlier) | archive V1 §"Review" |

## HRD-001 — V1 MVP specification

**Outcome:** authored the initial Herald MVP spec covering Upstreams, Mission, Execution Model, Constitution Integration, Workable-item prefix, Technology stack, Commons + Messaging + Subscribers + APIs, Project Herald + System Herald + Future flavors, Documentation, Testing. Closing Review section with 22 findings (R-01..R-22) catalogued for later passes. Renamed to `specification.V1.md` at the V2 supersession point; preserved in `docs/specs/mvp/archive/`.

## HRD-002 — V2 r1 architecture

**Outcome:** introduced V2 superseding V1. Adopted CloudEvents v1.0 wire format, Watermill routing, Postgres + River queue (NATS opt-in), Apprise-style URL+tag channel addressing, Knock-style preference matrix, OpenTelemetry observability, Postgres RLS multi-tenancy, SLSA L3 supply-chain stack, MJML email templates, `nicksnyder/go-i18n` for i18n. Filled in every V1 TBD; added seven flavors beyond `pherald`+`sherald` (`bherald`, `dherald`, `aherald`, `scherald`, `iherald`, `rherald`, `cherald`).

## HRD-003 — V2 r2 self-review

**Outcome:** closed V1's V2-R-01..V2-R-14 — defined missing tables (`webhook_sources` / `channel_addresses` / `thread_refs` / `quarantined_messages`), pinned Go ≥ 1.22 + license, added §16.1 Data retention + GDPR, §17.1 OTel env-var table, §26.5 Operator quickstart, §26.6 Disaster recovery, §27.3 Cost considerations.

## HRD-004 — V2 r3 self-review

**Outcome:** closed V3-R-01..V3-R-12 — added remaining Go type definitions (`Subscriber`, `CloudEventEnvelope`, `TraceContext`, `Branding`, `ChannelID`, `PreferenceSet`), §9.6 Database migration tooling, §3.4 Worker pools + SIGHUP hot-reload, §5.7 Ingress API URLs, §7.5 AI-agent subscribers, §8.3 Workable-item lifecycle, §11.14 `null://` sandbox channel, §17.4.1 Per-channel SLO budgets, §24.1 Machine-readable API specs, §5.4.1 Outbound idempotency, §3.5 Time/clock abstraction.

## HRD-005 — V3 r1 operator-product layer

**Outcome:** authored V3 superseding V2. New top-level sections §31 Project integration contract, §32 Inbound processing pipeline (30 s poll + FIFO + 7 worker stages + 4-layer anti-spam), §33 LLM/agent dispatch (Claude Code session-resolution + `<<<HERALD-DISPATCH-v1>>>` envelope), §34 Reply protocol (queued→processing→result tri-stage), §35 Versioned reports with Git linkage, §36 Multi-format outbound attachments (.md + .html + .pdf + .docx). Expanded §18.2 Project Herald with §18.2.1..§18.2.5 Investigation-before-Fixing flow + criticality + type classification + attachment storage at `issues/users/attachments/<WORKABLE_ITEM_ID>/`.

## HRD-006 — V3 r2 flavor refinement

**Outcome:** refined every non-`pherald` flavor for richer channel interaction. §18.1.1 cross-flavor primitives (slash/prefix command palette, 9-emoji reaction set, interactive buttons, modal forms, thread/forum-topic affinity, capability-degradation rule). §18.2.6..§18.10.1 per-flavor channel-interaction tables documenting buttons / reactions / slash commands / modals / threads tuned to each flavor's use case. Archived V2 to `docs/specs/mvp/archive/`.
