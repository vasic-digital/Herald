<div align="center">

<img src="../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Herald — HRD Obsolescence (§11.4.90) + docs/qa Coverage (§107.x / §11.4.83) Audit — 2026-05-27

| Field | Value |
|---|---|
| Date | 2026-05-27 |
| Repo | Herald (`/Users/milosvasic/Projects/Herald`) @ HEAD `0ab4b97` |
| Auditor | read-only subagent |
| Method | static-read-only (Read / Grep / Glob / `git log` / `git grep` / `git show`; NO execution of any gate, e2e, build, container, or server) |
| Scope | (1) §11.4.90 HRD obsolescence audit (new 4th terminal status `Obsolete (→ Fixed.md)`); (2) GAP-4 §107.x / §11.4.83 `docs/qa/<run-id>/` coverage matrix |
| Sources | `docs/Issues.md` r15, `docs/Fixed.md` r14, `docs/CONTINUATION.md`, `docs/research/telegram-bot-to-bot-constraint.md`, `docs/research/constitution-compliance-audit-2026-05-27.md`, `docs/superpowers/plans/2026-05-27-wave7-generic-messenger.md`, `git log` |

---

## 0. Executive summary

**Audit 1 (obsolescence).** Of the 28 closed (Fixed) + 48 open/in-progress HRD rows enumerated below, the static-evidence review finds **1 obsolescence CANDIDATE** — and it is a *latent / not-yet-tracked* item rather than a row presently sitting in `Issues.md`/`Fixed.md`. The qaherald-auto **2nd-bot-as-subscriber** automation approach (built under Wave 5 tasks #168–172, no formal HRD-NNN was ever assigned to it in `Issues.md`/`Fixed.md`) is **proven dead** by the Telegram bot-to-bot platform wall (`docs/research/telegram-bot-to-bot-constraint.md`, commit `5267f14`). Any HRD that gets opened to track "2nd bot drives pherald inbound" is `superseded-by-design-change`. **No row currently in `Issues.md` or `Fixed.md` is obsolete** — every shipped/Fixed feature is still in use, and every open §42/§43 HRD is still a valid forward work-item. The remaining 47 open HRDs are marked **RETAIN (still valid)**.

A secondary structural finding worth the operator's attention (NOT an obsolescence verdict): the **HRD-114..121 numbering collision** — `docs/CONTINUATION.md:16` lists `HRD-114..HRD-121 (Wave 7 T5-T12 pending)` as *Issues*, while `docs/Fixed.md:30` shows **HRD-114 already CLOSED** as Wave 7 T5 (multi-channel `pherald listen`). The Wave 7 plan (`…wave7-generic-messenger.md:19`) assigns HRD-110..121 one-per-task. HRD-115..121 exist ONLY in the plan; they are not yet opened in `Issues.md`. This is a tracking-drift defect, not obsolescence.

**Audit 2 (docs/qa coverage).** Only **2** `docs/qa/<run-id>/` directories exist (`HRD-100-…-w6live`, `HRD-101-lifecycle-…-w6.5live`). Against the inventory of shipped features: **2 COVERED, 7 MISSING, 6 PRE-MANDATE-EXEMPT** (shipped before the 2026-05-22 §107.x cutoff). The top back-fill gaps are the **Wave 7 multi-channel + channel-framework features (HRD-110..114)** which shipped on 2026-05-27 — *five days after* the mandate — with zero `docs/qa/` transcript, and the **qaherald Wave 5 binary** which shipped post-mandate without an HRD or a transcript. This confirms and refines GAP-4 from `constitution-compliance-audit-2026-05-27.md`.

---

## Audit 1 — §11.4.90 HRD obsolescence

### 1.1 Method + the obsolescence test applied

§11.4.90 adds a 4th terminal status `Obsolete (→ Fixed.md)` with the closed-set Reason vocabulary `superseded-by-design-change | superseded-by-later-mandate | feature-removed | duplicate-of | unsupported-topology`. Per §11.4.90 ("There MUST NOT be any mistake") and §11.4.6 (no-guessing), an item is flagged obsolete **only** when POSITIVE evidence (git-log / grep / research-doc) proves it is no longer valid; otherwise it is `RETAIN (still valid)`. Bare assertion is forbidden. Every row below was assessed against that bar.

### 1.2 Full HRD enumeration — Fixed (closed) items

| ID | Title (abridged) | Status | Obsolescence verdict |
|---|---|---|---|
| HRD-001 | V1 — initial MVP specification + Review + Recommendations | Fixed | RETAIN — foundational spec history; superseded-as-content by V3 but the work-item itself is correctly closed, not obsolete |
| HRD-002 | V2 r1 — architectural authoring | Fixed | RETAIN — archived spec lineage; closed correctly |
| HRD-003 | V2 r2 — close prose↔definition gaps | Fixed | RETAIN |
| HRD-004 | V2 r3 — Go type contract closure | Fixed | RETAIN |
| HRD-005 | V3 r1 operator-product layer (§31..§36) | Fixed | RETAIN |
| HRD-006 | V3 r2 flavor refinement — 9 flavors × per-channel tables | Fixed | RETAIN |
| HRD-007 | V3 r3 cross-doc sync + tracking-doc scaffold | Fixed | RETAIN |
| HRD-009 | commons module — full §11.0 Go type contract | Fixed | RETAIN — live foundation, imported everywhere |
| HRD-009b | commons_prefix — §8.2 3-letter prefix algorithm | Fixed | RETAIN |
| HRD-010 | commons_storage live wiring (pgx + River + Redis + migrations) | Fixed | RETAIN — live |
| HRD-011 | Telegram channel adapter live (telebot.v3) | Fixed | RETAIN — live transport; HRD-100/101 round-trips depend on it |
| HRD-012 | Claude Code dispatcher live (`claude --resume`) | Fixed | RETAIN — live inbound dispatch hot path |
| HRD-013 | commons_messaging + null:// adapter | Fixed | RETAIN |
| HRD-014 | pherald CLI scaffold (Cobra root + version + stubs) | Fixed | RETAIN |
| HRD-016 | pherald `/v1/events` Runner (7-stage pipeline via Gin) | Fixed | RETAIN — live REST surface |
| HRD-017 | Propagate Universal §11.4.73 + §11.4.74 into constitution | Fixed | RETAIN |
| HRD-028 | cherald `/v1/compliance` live | Fixed | RETAIN — live route |
| HRD-080 | Refine I6 inheritance-gate invariant | Fixed | RETAIN — gate still active |
| HRD-092 | commons/cli/ shared CLI scaffold (Wave 2) | Fixed | RETAIN — base of all flavor binaries |
| HRD-093 | sherald flavor scaffold | Fixed | RETAIN — live binary |
| HRD-094 | cherald flavor scaffold | Fixed | RETAIN |
| HRD-095 | bherald flavor scaffold | Fixed | RETAIN |
| HRD-096 | rherald flavor scaffold | Fixed | RETAIN |
| HRD-097 | iherald + scherald flavor scaffolds | Fixed | RETAIN |
| HRD-098 | sherald `/v1/safety_state` live | Fixed | RETAIN — live route |
| HRD-099 | commons_auth JWT verifier + Gin middleware | Fixed | RETAIN — live, gates every /v1 route |
| HRD-100 | Wave 6 — pherald inbound runtime + CC headless bridge | Fixed | RETAIN — live closed loop (msg 25→26 evidence) |
| HRD-110 | Wave 7 T1 — extract `channels.Channel` interface | Fixed | RETAIN — current Wave 7 substrate |
| HRD-111 | Wave 7 T2 — channel registry + init() registration | Fixed | RETAIN |
| HRD-112 | Wave 7 T3 — per-channel inbox subdirs | Fixed | RETAIN |
| HRD-113 | Wave 7 T4 — generalize bot self-filter via BotSelfIdentity | Fixed | RETAIN |
| HRD-114 | Wave 7 T5 — multi-channel `pherald listen` | Fixed | RETAIN — current head of Wave 7 |

Note: **HRD-101** (Wave 6.5 lifecycle, S1+S2 live evidence) is recorded as Fixed in `docs/CONTINUATION.md:18` (`HRD-092..HRD-101`) and has a `docs/qa/HRD-101-…/` dir, but it does **not** appear as a row in `docs/Fixed.md`'s "Recently fixed" table (table tops out at HRD-110 + HRD-100). That is a Fixed.md table-completeness drift (tracking defect), not obsolescence — RETAIN.

### 1.3 Full HRD enumeration — Open / In-progress items

| ID | Title (abridged) | Status | Obsolescence verdict |
|---|---|---|---|
| HRD-008 | Operator quickstart compose validation | in_progress | RETAIN — still a valid pending operator validation |
| HRD-015 | Inheritance gate I8 invariants for Go scaffold | open | RETAIN — gate work still valid |
| HRD-018 | commons_constitution Evaluator + 12 emit helpers + migrations | in_progress | RETAIN — core §42 work, partially landed |
| HRD-019 | cherald constitution bindings (~30 policy.violation rules) | open | RETAIN — forward §42 work |
| HRD-020 | sherald host-safety + repo-safety bindings | open | RETAIN |
| HRD-021 | bherald CI/test bindings | open | RETAIN |
| HRD-022 | rherald release bindings | open | RETAIN |
| HRD-023 | pherald project bindings | open | RETAIN |
| HRD-024 | iherald constitution-rule escalation bindings (paging) | open | RETAIN — carry-over to Wave 3c, still valid |
| HRD-025 | scherald scheduled-audit bindings | open | RETAIN |
| HRD-026 | Constitution-bundle hash captureer | open | RETAIN |
| HRD-027 | Mode-ladder runtime config | open | RETAIN |
| HRD-029 | §2 `pherald commit-push` single-entrypoint | open | RETAIN — §43 catalogue |
| HRD-030 | §3 `pherald submodule propagate` | open | RETAIN |
| HRD-031 | §4 `rherald tag mirror` | open | RETAIN |
| HRD-032 | §5 `rherald changelog generate` | open | RETAIN |
| HRD-033 | §9.1 `sherald destructive guard` | open | RETAIN — destructive-guard body still pending |
| HRD-034 | §9.3 `sherald backup snapshot` | open | RETAIN |
| HRD-035 | §11.4.2 `bherald evidence capture` | open | RETAIN |
| HRD-036 | §11.4.10 `cherald creds scan` | open | RETAIN |
| HRD-037 | §11.4.12 `cherald docs sync` | open | RETAIN |
| HRD-038 | §11.4.18 `cherald script-docs check` | open | RETAIN |
| HRD-039 | §11.4.19/.23 `cherald fixed align` + colorize | open | RETAIN — note: colorizer should add the new §11.4.90 `Obsolete` HTML class, but the HRD itself is valid |
| HRD-040 | §11.4.26 `sherald constitution pull` | open | RETAIN |
| HRD-041 | §11.4.27 `bherald test-tier verify` | open | RETAIN |
| HRD-042 | §11.4.31 `cherald submanifest verify` | open | RETAIN |
| HRD-043 | §11.4.36 `pherald install-upstreams` | open | RETAIN |
| HRD-044 | §11.4.37 `pherald fetch-guard` | open | RETAIN |
| HRD-045 | §11.4.40 `rherald gate retest` | open | RETAIN |
| HRD-046 | §11.4.41 `sherald force-push gate` | open | RETAIN |
| HRD-047 | §11.4.45/.56 `scherald status digest` | open | RETAIN |
| HRD-048 | §11.4.53 `cherald fixed-summary sync` | open | RETAIN |
| HRD-049 | §11.4.55 `pherald reopen <HRD>` | open | RETAIN |
| HRD-050 | §11.4.59 `cherald readme sync` | open | RETAIN |
| HRD-051 | §11.4.60 `cherald composite-gate` | open | RETAIN |
| HRD-052 | §11.4.65 `cherald export` (md/html/pdf/docx) | open | RETAIN |
| HRD-053 | §11.4.71 `pherald pre-push` | open | RETAIN |
| HRD-054 | §11.4.73 `cherald spec-version check` | open | RETAIN |
| HRD-055 | §11.4.74 `cherald catalogue-check <pr>` | open | RETAIN |
| HRD-056 | §12.6 `sherald mem-budget watch` | open | RETAIN |
| HRD-081 | Extend containers/pkg/compose for podman-compose runtime | open | RETAIN — real upstream gap, workaround in `commons_infra/boot.go` |
| HRD-085 | pgxTaskRepository GetByID/Update/Delete | open | RETAIN — TaskRepository surface still required |
| HRD-086 | pgxTaskRepository UpdateStatus/Progress/Heartbeat/Checkpoint | open | RETAIN |
| HRD-087 | pgxTaskRepository GetByStatus/Pending/Count/History | open | RETAIN |
| HRD-088 | pgxTaskRepository GetStaleTasks/GetByWorkerID | open | RETAIN |
| HRD-089 | pgxTaskRepository SaveResourceSnapshot/Get | open | RETAIN |
| HRD-090 | pgxTaskRepository MoveToDeadLetter | open | RETAIN |

**Open/in-progress obsolescence verdict: 0 obsolete / 47 RETAIN.** Every open §42/§43 binding and every TaskRepository stub remains a valid forward work-item; none is contradicted by a later design change, mandate, removed feature, duplicate, or unsupported topology. The 2026-05-22 messenger-genericize mandate (Wave 7) and the 2026-05-27 constitution mandates (§11.4.89–94) *add* work — they do not invalidate any existing HRD.

### 1.4 Obsolescence CANDIDATE (1) — drafted Obsolete-Details

There is exactly one candidate, and it is a **latent / not-yet-formally-tracked** work-item (no HRD-NNN row exists for it in `Issues.md`/`Fixed.md`). It is surfaced here so that IF/WHEN an HRD is opened to track it, it is opened directly as `Obsolete`, never as `open`.

**CANDIDATE — qaherald-auto "2nd Telegram bot as subscriber" automation approach** (built under Wave 5 / qaherald-auto, TaskList #168–172; no HRD-NNN assigned).

> **Obsolete-Details:**
> - **Since:** 2026-05-27
> - **Reason:** `superseded-by-design-change`
> - **Superseding-item:** Wave 7 messenger-genericize mandate + planned MTProto user-client `MessengerClient` impl (per `docs/research/telegram-bot-to-bot-constraint.md` "Path A"); operator-mandate 2026-05-22 (genericize messenger framework). The scenario engine, orchestrator, report generator, and 12 hermetic tests (`qaherald/internal/lifecycle/`, `qaherald/internal/messenger/`) are **NOT** obsolete — only the *transport binding* (2nd-bot → group → pherald-bot `getUpdates`) is dead and is replaced by an MTProto user-session transport or a Telegram-API-double.
> - **Triple-check evidence:**
>   1. `docs/research/telegram-bot-to-bot-constraint.md` (commit `5267f14`): live 15-scenario run produced **0 PASS / 14 FAIL / 1 SKIP**, every scenario `await-reply: context deadline exceeded`; pherald `getUpdates?limit=10` returned **0 updates** after both a plain-message probe (`sent_message_id=14`) and an @mention probe (`sent_message_id=15`), with both bots reporting `can_read_all_group_messages: true` — proving the failure is the absolute Telegram platform rule "bots cannot see other bots' group messages," not a Herald defect.
>   2. `git log` commit `5267f14` "docs/research: forensic finding — Telegram bot-to-bot message invisibility breaks 2nd-bot automation" + `b45e45d` "FIX qaherald preflight G1: real getChatMember membership proof" (the harness was hardened, then the transport was found structurally impossible).
>   3. `docs/CONTINUATION.md:15` Status summary: "**MAJOR FINDING: the 2nd-bot qaherald-auto automation is structurally impossible** … Real-channel inbound automation requires MTProto … OPERATOR DECISION PENDING."

**Why this is NOT yet an `Issues.md`/`Fixed.md` migration:** no HRD row was ever opened for the 2nd-bot approach (Wave 5 qaherald and qaherald-auto were tracked via TaskList only, never assigned an HRD-NNN — confirmed by `git grep "qaherald" docs/Issues.md docs/Fixed.md` returning only incidental mentions inside Wave 7 rows). Per §11.4.90 the operator should: (a) decide whether to open a retroactive HRD for the qaherald-auto transport solely to record it as `Obsolete (→ Fixed.md)` with the Details above, OR (b) record the obsolescence inline in the forthcoming MTProto/Wave-8 HRD's References cell as the superseded predecessor. Either path preserves the triple-checked audit trail §11.4.90 requires.

### 1.5 Strongest single obsolescence finding

The qaherald-auto 2nd-bot transport (CANDIDATE above) is the single strongest finding: it has the rare combination of (a) a dedicated forensic research doc with physical wire-level evidence (`getUpdates` returning 0), (b) a clean `git log` trail, and (c) an explicit operator-facing CONTINUATION record naming it "structurally impossible." It cleanly satisfies the §11.4.90 "no mistake / positive evidence / no bare assertion" bar under `superseded-by-design-change`.

### 1.6 Structural drift flagged (NOT obsolescence — for operator action)

- **HRD-114..121 numbering collision.** `docs/CONTINUATION.md:16` lists `HRD-114..HRD-121` as *open Issues* ("Wave 7 T5-T12 pending"), but `docs/Fixed.md:30` shows HRD-114 **already Fixed** (Wave 7 T5). The Wave 7 plan maps HRD-115=T6 (Slack), HRD-116=T7, … HRD-121=T12. HRD-115..121 do not yet exist as rows in `docs/Issues.md`. CONTINUATION is stale by one task. Fix: open HRD-115..121 in `Issues.md` (or correct CONTINUATION to `HRD-115..HRD-121`).
- **HRD-101 absent from `Fixed.md` table** though recorded Fixed in CONTINUATION + has a `docs/qa/` dir. Fix: back-fill the HRD-101 row into `docs/Fixed.md` (table-completeness, §11.4.19 atomic-migration hygiene).

---

## Audit 2 — GAP-4 docs/qa coverage (§107.x / §11.4.83)

### 2.1 Existing `docs/qa/` run-dirs (Glob result)

```
docs/qa/HRD-100-2026-05-22T18-27-30-w6live/        (transcript.jsonl, claude-session.jsonl, claude-code-session-uuid.txt,
                                                     pherald-listen.log, attachments/, README.md)
docs/qa/HRD-101-lifecycle-2026-05-23T03-16-17-w6.5live/  (transcript.jsonl, issues-before.md, fixed-before.md,
                                                          attachments/, README.md)
```

Two run-dirs total. The §107.x cutoff is **2026-05-22** (mandate landed via TaskList #105 + `docs/qa` mandate in CLAUDE.md §107.x).

### 2.2 Coverage matrix

| Feature | Shipped (wave / HRD / commit) | Ship date | docs/qa dir present? | Verdict |
|---|---|---|---|---|
| Wave 2 — sherald flavor (HRD-093) | Wave 2, `HRD-093` | 2026-05-21 | No | PRE-MANDATE-EXEMPT (pre-2026-05-22) |
| Wave 2 — cherald flavor (HRD-094) | Wave 2, `HRD-094` | 2026-05-21 | No | PRE-MANDATE-EXEMPT |
| Wave 2 — bherald flavor (HRD-095) | Wave 2, `HRD-095` | 2026-05-21 | No | PRE-MANDATE-EXEMPT |
| Wave 2 — rherald flavor (HRD-096) | Wave 2, `HRD-096` | 2026-05-21 | No | PRE-MANDATE-EXEMPT |
| Wave 2 — iherald + scherald flavors (HRD-097) | Wave 2, `HRD-097` | 2026-05-21 | No | PRE-MANDATE-EXEMPT |
| Wave 3a — commons_auth JWT verifier (HRD-099) | Wave 3a, `HRD-099` | 2026-05-21 | No | PRE-MANDATE-EXEMPT |
| Wave 3a — cherald `/v1/compliance` (HRD-028) | Wave 3a, `HRD-028` | 2026-05-21 | No (e2e E43/E44 only) | PRE-MANDATE-EXEMPT (shipped 2026-05-21; e2e wire-byte evidence exists in `e2e_bluff_hunt.sh`, not a `docs/qa/` dir) |
| Wave 3a — sherald `/v1/safety_state` (HRD-098) | Wave 3a, `HRD-098` | 2026-05-21 | No (e2e E46/E47 only) | PRE-MANDATE-EXEMPT |
| Wave 3b — pherald `/v1/events` Runner (HRD-016) | Wave 3b, `HRD-016` | 2026-05-22 | No (e2e E37-E42 only) | MISSING — closed 2026-05-22, on/after cutoff; only honest-SKIP e2e, no `docs/qa/HRD-016/` transcript |
| Telegram adapter live (HRD-011) | `HRD-011`, `140a2f1` | 2026-05-22 | Partial (live `message_id=5` recorded in Fixed.md prose; no dedicated dir) | MISSING — on/after cutoff; live evidence is prose-only, not a committed `docs/qa/HRD-011/` artefact |
| Claude Code dispatcher live (HRD-012) | `HRD-012`, `702b5a3`/`4718c0e` | 2026-05-21 | No | PRE-MANDATE-EXEMPT (closed 2026-05-21; live timings in Fixed.md prose) |
| Wave 4a — HTTP/3 + Brotli + Alt-Svc + TLS 1.3 | Wave 4a, tag v0.2.0 | ~2026-05-22+ | No (e2e E49-E55 only) | MISSING — substrate shipped post-cutoff without a `docs/qa/` transcript |
| Wave 4b — TOON content negotiation | Wave 4b, tag v0.3.0 | ~2026-05-22+ | No (e2e E56-E62 only) | MISSING |
| Wave 5 — qaherald binary (round-trip automation) | Wave 5, TaskList #124-130 (no HRD) | ~2026-05-22+ | No | MISSING — post-cutoff feature, no HRD AND no `docs/qa/` dir |
| Wave 6 — pherald inbound runtime (HRD-100) | Wave 6, `HRD-100`, T1..T12 | 2026-05-22 | **Yes** (`HRD-100-…-w6live/`) | COVERED — full bidirectional transcript + claude session + attachments |
| Wave 6.5 — ticket lifecycle (HRD-101) | Wave 6.5, `HRD-101` | 2026-05-23 | **Yes** (`HRD-101-lifecycle-…-w6.5live/`) | COVERED — S1+S2 transcript + issues/fixed-before snapshots (note: only 2 of 15 scenarios captured live; S3-S15 documented as reproducible, not captured) |
| Wave 7 T1-T5 — channel framework + multi-channel listen (HRD-110..114) | Wave 7, `HRD-110..114`, `688819a`..`42448a2` | 2026-05-27 | No | MISSING — **highest-priority gap**: 5 features shipped 5 days post-cutoff with zero `docs/qa/` transcript; Fixed.md rows explicitly defer live evidence to "Wave 7 T9 e2e invariant E81/E82/E83" (not yet run) |

**Tally: 2 COVERED · 7 MISSING · 6 PRE-MANDATE-EXEMPT.**
(MISSING = HRD-016, HRD-011, Wave 4a, Wave 4b, Wave 5 qaherald, Wave 7 HRD-110..114 grouped, and HRD-016's Runner — counted as 7 distinct shipped-feature lines on/after the 2026-05-22 cutoff. The Wave 7 HRD-110..114 line is a cluster of 5 HRDs but is one shipped feature-family.)

### 2.3 Top docs/qa back-fill gaps (ranked)

1. **Wave 7 HRD-110..114 (channel framework + multi-channel `pherald listen`)** — HIGHEST. Five HRDs closed 2026-05-27, five days after the §107.x mandate, each citing "live runtime evidence lands via Wave 7 T9 e2e invariant E81/E82/E83" which has not yet been run. Per §107.x every one of these is currently a PASS-bluff at the QA-evidence layer until either a `docs/qa/<run-id>/` transcript lands (T9 multi-channel round-trip) or they are explicitly re-classified. Their unit tests are strong, but unit PASS ≠ end-user transcript.
2. **Wave 5 qaherald binary** — HIGH. A post-cutoff shipped binary with neither an HRD-NNN nor a `docs/qa/` dir. Note this is distinct from the qaherald-auto 2nd-bot transport obsolescence (Audit 1): the qaherald round-trip automation binary itself shipped without QA evidence.
3. **HRD-016 pherald `/v1/events` Runner** — MEDIUM. Closed 2026-05-22 (on cutoff). Has e2e E37-E42 but they honest-SKIP when local Postgres :24100 is absent; no `docs/qa/HRD-016/` request+response-body transcript was committed.
4. **HRD-011 Telegram live** — MEDIUM. Live `message_id=5` is recorded as prose in `docs/Fixed.md`, but §107.x(3) requires the artefact in-repo, not a prose reference; a `docs/qa/HRD-011/` capture of the getMe→getChat→sendMessage chain would close it.
5. **Wave 4a (HTTP/3) + Wave 4b (TOON)** — MEDIUM/LOW. Transport substrate shipped post-cutoff; e2e E49-E62 assert wire-bytes but no `docs/qa/<run-id>/` transcript dir exists. Lower user-visibility than the inbound features above.

This matrix confirms GAP-4 in `docs/research/constitution-compliance-audit-2026-05-27.md:80` and refines it with per-feature ship-dates and the PRE-MANDATE-EXEMPT cutoff classification §107.x permits for the Wave-2/3a features that shipped 2026-05-21 (before the 2026-05-22 mandate).

---

## 3. Non-execution attestation

This audit was performed **read-only**. The auditor:

- ran **NO** mutation gate (`tests/test_wave*_mutation_meta.sh` / any `*_meta.sh`), **NO** `scripts/e2e_bluff_hunt.sh`, **NO** `go build`/`go test`, **NO** container or server boot;
- executed **NO** `git` command that mutates the working tree or index (only `git log` / `git grep` / read-only inspection);
- did **NOT** touch the in-flight background mutation gate, `.git/MUTATION_IN_PROGRESS`, any `.go` file, any `tests/` file, or any existing tracked file;
- made **NO** edit to any existing file and **NO** Go-source change;
- wrote **EXACTLY ONE** new file — this report at `docs/research/hrd-obsolescence-and-qa-coverage-audit-2026-05-27.md`.

Any `MUTATED` markers visible in `git status` during this audit belong to the concurrently-running mutation gate and were deliberately left untouched.
