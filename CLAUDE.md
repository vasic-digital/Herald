<div align="center">

<img src="assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# CLAUDE.md

| Field | Value |
|---|---|
| Revision | 20 |
| Created | 2026-05-15 |
| Last modified | 2026-05-31 |
| Status | active |
| Status summary | **r20: refreshed the stale "first-implementation cycle r1 (2026-05-20) / Go-scaffold landed" project-status prose to current reality — **18** workspace modules, waves 2–7 shipped, tag **v0.6.0** (2026-05-28), live `pherald listen` + Claude Code dispatch + Telegram & Slack adapters + REST API + participant-attribution (§11.4.104) + intent-recognition (§11.4.105) + Docs Chain (§11.4.106); HRD-010/011/012/016 now LIVE; ~a-dozen open HRDs remain. Status prose ONLY — covenant/build/anti-bluff/inheritance content unchanged (merged a parallel stream's project-status prose onto main's r19 §11.4.106 header). Prior r19: propagated HelixConstitution §11.4.106 (Docs Chain documentation-sync mandate, 2026-05-29) into the inherited-covenant cluster as a short-form restatement citing the literal anchor `11.4.106`, inherited per §11.4.35. Herald-applicability classification: APPLICABLE (integration in progress) — Docs Chain mechanizes Herald's ~76-doc html/pdf/docx sibling sync (today regen-all `scripts/export_docs.sh`) via `exec:` transforms preserving Herald's exact logo-CSS pandoc flags; cites the migration plan `docs/research/docs_chain/HERALD_DOCS_CHAIN_PLAN.md`, the planned `verify` drift-gate E146, and the honest open prerequisites (Phase-4 CLI landed 2026-05-31; exec-staging relative-asset gap G6; binary-hash `verify` defect found by dogfooding, fix in flight). Restated + cited, not redefined; anchor now present in CLAUDE.md/AGENTS.md/QWEN.md/HERALD_CONSTITUTION.md §108.p. Prior r18: propagated the intent-recognition mandate (operator 2026-05-31) as a short-form restatement — new "Intent recognition & clarification" section + ToC entry — citing the authoritative contract `docs/design/INTENT_RECOGNITION.md` and noting inheritance from HelixConstitution §11.4.105 (the root-§ being added on the constitution stream), inherited per §11.4.35. Covers: subscribers speak plain natural language (NO command syntax, no `COMMAND:` prefix); three-tier resolution (Tier 1 deterministic CommandRecognizer fast-path → Tier 2 LLM intent inference via the `<<<HERALD-DISPATCH-v1>>>` envelope → Tier 3 `action="clarify"` reply-tag-and-ask fallback via the §11.4.104 IdentityResolver); never guess an action, never ignore a message. Restated + cited, not redefined.** Prior r17: propagated the participant/attribution/tagging mandate (operator 2026-05-31) as a short-form restatement — new "Participant identity, attribution & notification-tagging" section + ToC entry — citing the authoritative contract `docs/design/PARTICIPANT_ATTRIBUTION.md` and noting inheritance from HelixConstitution per §11.4.35. Covers: per-messenger Participant identity (`subscribers`/`subscriber_aliases.username`), the `HERALD_<CHANNEL>_OPERATOR_USERNAME` operator env var (`HERALD_TGRAM_OPERATOR_USERNAME=@milos85vasic`), `created_by`/`assigned_to` attribution, and the @-tagging matrix (Claude + Operator never tagged). Restated + cited, not redefined.** Prior r16: propagated HelixConstitution §11.4.100 (Video color + visual-quality fidelity mandate, 2026-05-28) as a short-form restatement citing the literal anchor `11.4.100`, inherited per §11.4.35; restated + cited, not redefined. Herald-applicability classification: non-applicable-but-cite (Herald has NO video-playback surface — `pherald listen` downloads video attachments as opaque sha256-blobs without decoding/rendering); mandate binds latently if Herald ever ships a video-rendering surface in future. Cascade-parallel: §11.4.96 ("Herald has no AOSP build, but the principle binds"). Required by the upcoming `CM-COVENANT-114-100-PROPAGATION` pre-build gate. Prior r13: propagated HelixConstitution §11.4.95–§11.4.97 (workable-items SQLite DB is tracked-in-git not gitignored, safe-parallel-work-with-long-build catalogue + mandate, maximum-use-of-idle-time + progress-update cadence) as short-form restatements citing the literal anchors `11.4.95`/`11.4.96`/`11.4.97`, inherited per §11.4.35; restated + cited, not redefined.** Prior r12: propagated HelixConstitution §11.4.89–§11.4.94 (background-test execution, Obsolete status + obsolescence audit, summary-doc clarity, multi-pass change-evaluation, SQLite workable-items SSoT, zero-idle parallel-by-default) as short-form restatements citing the literal anchors, inherited per §11.4.35; restated + cited, not redefined.** Prior r11: propagated HelixConstitution §11.4.85 (stress + chaos test mandate) + §11.4.87 (endless-loop autonomous work + zero-idle agent dispatch + anti-bluff testing) + §11.4.88 (background-push: commit-lock release immediately after commit, push detached) into this file's covenant cluster — short-form restatements citing the literal section numbers, inherited per §11.4.35; required by the §11.4.87 `CM-COVENANT-114-87-PROPAGATION` pre-build gate which asserts the literal anchor `11.4.87` is present in every consuming repo's CLAUDE.md/AGENTS.md/QWEN.md. Herald restates + cites, does not redefine or weaken. Prior r10: Wave 6 (pherald inbound runtime + Claude Code headless bridge — code-doc closure; tag v0.4.0 deferred to T13b post-live-evidence).** New `pherald listen` Cobra subcommand drives the closed-loop runtime: Telegram `getUpdates` long-poll + bot self-filter (§32.9 anti-echo via `bot.Me.Username`) + `OnPhoto`/`OnDocument`/`OnVoice` handlers with sha256 content-addressed attachment download (`~/.herald/inbox/<sha>.<ext>`, idempotent) + Claude Code dispatch with Opus pinned in argv (`claude --model claude-opus-4-7`, §33.7) + envelope pre-text — verbatim operator wording "We have received new message from our communication channel ..." preceding the existing `<<<HERALD-DISPATCH-v1>>>` block (§33.6) + `<<<HERALD-REPLY>>>` parser with `action` routing (`reply` / `issue.open` / `event.emit`) + `tgram.SendReply` wiring `reply_to_message_id`. T10a `--qa-out-dir` JSONL journaling (§107.x). 12 Wave-6 commits (T1=`ad87d7f` → T12=`96c7c6b`). 8 new e2e invariants E63-E70 (currently SKIP-with-documented-reason — convert to PASS once T10b lands operator-supplied live evidence). Wave 6 mutation gate `tests/test_wave6_mutation_meta.sh` (3 paired hermetic mutations, 4/4 PASS). HRD-100 atomic Issues→Fixed. Spec V3 r11→r12 with §32.9 + §33.6 + §33.7 + §43 `pherald listen` row. Workspace at 16 modules (Wave 5 added `qaherald`; Wave 6 added NO new module — inbound is `pherald/internal/inbound/`). Prior r9: Wave 5 T1 `qaherald` module skeleton landed (16th workspace module + 8th flavor binary). Prior r8: Wave 4b TOON status pointer; tag v0.3.0 substrate. Prior r5/r6: §107 End-user-usability covenant section restating the verbatim operator mandate at Herald level + ToC entry; ties to HERALD_CONSTITUTION.md §107 + inheritance-gate invariant I8a (paired with I8b/c on AGENTS.md + HERALD_CONSTITUTION.md). |
| Issues | none |
| Issues summary | — |
| Fixed | spec-path references (r2), pre-implementation-language update (r3), submodules + HRD-docs + codegraph-index enumeration (r4), §107 mandate restatement + I8a anchor (r5), Wave-2/3a workspace-module-count refresh (r7), Wave 4a + 4b status pointer + e2e/mutation tally refresh (r8), Wave 5 T1 qaherald skeleton + commons_tls enumeration fold-in (r9), Wave 6 inbound runtime + CC headless bridge code-doc closure (r10), Helix §11.4.85 + §11.4.87 + §11.4.88 propagation (r11), Helix §11.4.89–§11.4.94 propagation (r12), Helix §11.4.95–§11.4.97 propagation (r13) |
| Fixed summary | aligned with HRD-009/HRD-009b/HRD-013/HRD-014 landing in the same commit; r4 closes the discoverability gaps observed during a fresh `/init` review; r5 closes the Herald-level explicit-restatement gap identified by the 2026-05-20 audit; r7 closes the doc drift observed during the Wave 3a final review; r8 records the Wave 4a→4b transport/wire-format substrate upgrades; r9 lands Wave 5 T1 (`qaherald/cmd/qaherald` + `commons/branding.go` qa flavor) and back-fills the Wave 4a `commons_tls` workspace enumeration that was missing from r7/r8; r10 lands Wave 6 — pherald inbound runtime (`pherald listen` Cobra subcommand, `pherald/internal/inbound/` Dispatcher with `<<<HERALD-REPLY>>>` action routing, tgram bot self-filter + `OnPhoto`/`OnDocument`/`OnVoice` with sha256 content-addressed attachment download, claude_code dispatcher pinned to Opus with verbatim pre-text envelope, tgram.SendReply with `reply_to_message_id`, T10a `--qa-out-dir` JSONL journaling); spec V3 r11→r12 + HRD-100 atomic Issues→Fixed; tag v0.4.0 deferred to T13b (post-T10b live evidence); r11 propagates the three new inherited HelixConstitution mandates (§11.4.85 stress+chaos test mandate, §11.4.87 endless-loop autonomous work + zero-idle agent dispatch + anti-bluff testing, §11.4.88 background-push) into this file's covenant cluster as short-form restatements per the §11.4.87 `CM-COVENANT-114-87-PROPAGATION` pre-build gate (literal anchors `11.4.85`/`11.4.87`/`11.4.88` now present), inherited per §11.4.35; restated + cited, not redefined; r12 propagates the next six inherited HelixConstitution mandates (§11.4.89 background-test execution, §11.4.90 Obsolete status + obsolescence audit, §11.4.91 summary-doc clarity, §11.4.92 multi-pass change-evaluation, §11.4.93 SQLite workable-items SSoT, §11.4.94 zero-idle parallel-by-default) into the same covenant cluster as short-form restatements citing the literal anchors `11.4.89`/`11.4.90`/`11.4.91`/`11.4.92`/`11.4.93`/`11.4.94`, inherited per §11.4.35; restated + cited, not redefined; r13 propagates the next three inherited HelixConstitution mandates (§11.4.95 workable-items SQLite DB tracked-in-git-not-gitignored, §11.4.96 safe-parallel-work-with-long-build catalogue + mandate, §11.4.97 maximum-use-of-idle-time + progress-update cadence) into the same covenant cluster as short-form restatements citing the literal anchors `11.4.95`/`11.4.96`/`11.4.97`, inherited per §11.4.35; restated + cited, not redefined. |
| Continuation | bump again when T10b lands operator-supplied live evidence under `docs/qa/HRD-100-<run-id>/` and T13b tags v0.4.0; then continue with Wave 7 (genericize messenger-channel framework per operator mandate 2026-05-22 — Slack/Max/Email next), Wave 3c carry-over (HRD-024 iherald paging, HRD-033 destructive-guard body, remaining HRD-018..028 constitution bindings), §43 command catalogue HRD-029..056, comprehensive docs audit (task #147), and the OOM-Protect Herald flavor (`oherald`, future post-Wave-6 closure per operator 2026-05-22). |

## Table of contents

- [INHERITED FROM Helix Constitution (parent-discovery)](#inherited-from-helix-constitution-parent-discovery)
- [Project status](#project-status)
- [End-user-usability covenant (Herald §107 / Helix §11.4 — MANDATORY ANTI-BLUFF)](#end-user-usability-covenant-herald-107--helix-114--mandatory-anti-bluff)
- [Inherited covenant restatements — Helix §11.4.85 / §11.4.87 / §11.4.88 / §11.4.89 / §11.4.90 / §11.4.91 / §11.4.92 / §11.4.93 / §11.4.94 / §11.4.95 / §11.4.96 / §11.4.97](#inherited-covenant-restatements--helix-11485--11487--11488--11489--11490--11491--11492--11493--11494--11495--11496--11497-inherited-per-11435)
- [Participant identity, attribution & notification-tagging (Herald §109 / contract `docs/design/PARTICIPANT_ATTRIBUTION.md`)](#participant-identity-attribution--notification-tagging-herald-109--contract-docsdesignparticipant_attributionmd--inherited-from-helixconstitution-per-11435)
- [Intent recognition & clarification (Herald §110 / contract `docs/design/INTENT_RECOGNITION.md`)](#intent-recognition--clarification-herald-110--contract-docsdesignintent_recognitionmd--inherited-from-helixconstitution-114105-per-11435)
- [Mission (from the spec)](#mission-from-the-spec)
- [Intended stack](#intended-stack)
- [Multi-host mirror convention](#multi-host-mirror-convention)
- [Inheritance gate (run before any commit touching root docs)](#inheritance-gate-run-before-any-commit-touching-root-docs)
- [Spec-change rule (load-bearing — `docs/specs/mvp/specification.V4.md` §"Specification documents")](#spec-change-rule-load-bearing-docsspecsmvpspecificationmd-specification-documents)
- [Project conventions from the spec (apply when scaffolding)](#project-conventions-from-the-spec-apply-when-scaffolding)
- [`constitutable/` directory (parent-project extension hook)](#constitutable-directory-parent-project-extension-hook)
- [Documentation artefacts (PDF/HTML siblings)](#documentation-artefacts-pdfhtml-siblings)
- [Notes for future scaffolding](#notes-for-future-scaffolding)

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## INHERITED FROM Helix Constitution (parent-discovery)

Herald is consumed as a submodule of a parent project that already carries the Helix Constitution submodule at `<parent>/constitution/`. Herald therefore does **NOT** keep its own copy. Locate the constitution from any nested depth by walking up parents until you find `<ancestor>/constitution/Constitution.md`, or by running the canonical helper:

```bash
CONST_DIR="$(bash "$(find . -type d -name constitution -print -quit 2>/dev/null)/find_constitution.sh")"
# or, more robustly, from any starting directory:
CONST_DIR="$(bash <ancestor>/constitution/find_constitution.sh)"
```

For standalone development of Herald (no parent project), clone the constitution alongside Herald:

```bash
git clone git@github.com:HelixDevelopment/HelixConstitution.git \
    $(dirname "$PWD")/constitution
```

Once located, all rules in `<discovered>/CLAUDE.md` and the `<discovered>/Constitution.md` it references apply unconditionally. Herald-specific rules below extend them — they MUST NOT weaken any inherited rule. When this file disagrees with the discovered constitution, the constitution wins.

Canonical: <https://github.com/HelixDevelopment/HelixConstitution>

> **Read order on a cold start:**
> 1. `<discovered-constitution>/CLAUDE.md` + `Constitution.md` — universal Helix rules. Inherited unconditionally.
> 2. `<discovered-constitution>/AGENTS.md` — agent guardrails (anti-bluff, no-guessing, paired mutations).
> 3. `README.md` — Herald overview + how-to.
> 4. This file (Herald-specific notes below).
> 5. `docs/guides/HERALD_CONSTITUTION.md` — Herald's project-specific constitutional extensions.
> 6. `docs/guides/CONSTITUTION_INHERITANCE.md` — operator/agent guide for the discovery contract + the inheritance gate.
> 7. `docs/specs/mvp/specification.V4.md` — mission spec (mostly TBD).

## Project status

Herald is in **active multi-wave implementation** as of 2026-05-31 — well past the original first-implementation scaffold (r1, 2026-05-20). Waves 2 through 7 have shipped; the latest tag is **v0.6.0** (2026-05-28); the live `pherald listen` inbound runtime, Claude Code dispatch, Telegram + Slack channel adapters, participant-attribution (§11.4.104), natural-language intent-recognition (§11.4.105), and the Docs Chain documentation-sync mandate (§11.4.106) are all landed. Roughly a dozen HRDs remain open in `docs/Issues.md` (evidence-gated live-integration + later-iteration channels); the bulk of the catalogue migrated to `Fixed.md` in the v1.0.0 closure sweep. The repo contains:

- `README.md` — mission, deployment model, inheritance contract, quickstart.
- `docs/specs/mvp/specification.V4.md` — MVP spec (participant-attribution §11.4.104 + intent-recognition §11.4.105 incorporated; the prior V-series archived under `docs/specs/mvp/archive/`).
- `docs/guides/HERALD_CONSTITUTION.md` — Herald's project constitution (extends Helix).
- `docs/guides/CONSTITUTION_INHERITANCE.md` — operator/agent guide for parent-discovery + gate semantics.
- `upstreams/` — Herald's mirror declarations (see below).
- `tests/test_constitution_inheritance.sh` + `_meta.sh` — paired inheritance gate (§1.1).
- `.gitignore` tuned for Go + `.DS_Store`.

Herald does **not** ship a `constitution/` submodule of its own; the parent project provides it. See `docs/guides/CONSTITUTION_INHERITANCE.md`.

**As of 2026-05-31** the codebase spans **18** Go workspace modules (10 shared/foundation + 8 flavor binaries) with the inbound runtime, channel adapters, and storage all live. Representative components:

- `go.work` (gitignored locally; check in if the project wants reproducible workspaces — current convention: gitignored per spec §9.1).
- `commons/` (L0) — `commons/types.go` with the full §11.0 Channel contract + Subscriber/CloudEventEnvelope/TraceContext/Branding/ChannelID; `commons/clock.go` Clock abstraction; `commons/uuidv7.go`; `commons/branding.go` per-flavor factory.
- `commons_prefix/` — §8.2 3-letter prefix algorithm.
- `commons_messaging/channels/null/` — full §11.14 `null://` sandbox adapter (working, tested).
- `commons_messaging/channels/tgram/` — Telegram adapter (live `getUpdates` long-poll + send + reply + `@`-mention render; drives `pherald listen`).
- `commons_messaging/channels/slack/` — Slack adapter (Wave 7 multi-channel generalization).
- `commons_messaging/dispatch/claude_code/` — Claude Code session-resolution + envelope formatter; live `claude` invocation wired with Opus pinned and per-message timeout bound (HRD-146).
- `commons_storage/` — SQL migrations embedded via `//go:embed`; pgx + River + Redis wiring live.
- `commons_workable/` + `commons_watch/` — workable-items SQLite SSoT store + per-property change-feed + fsnotify/WAL-poll watcher (the ATMOSphere integration substrate).
- `pherald/cmd/pherald/` — Cobra CLI; `pherald version`, `pherald listen` (inbound runtime), `pherald watch` (workable-items daemon), and the §43 command catalogue all wired end-to-end.
- `quickstart/` — Herald-specific Docker/Podman Compose + Dockerfile + otel-config + `.env.example` per §26.5. Migrated from `containers/quickstart/` 2026-05-20 when the `containers/` submodule was added.
- `containers/` — git submodule pointing at `git@github.com:vasic-digital/containers.git` (the `digital.vasic.containers` Go module — runtime auto-detection + on-demand container boot + lifecycle/health). Imported by Foundation tests + the `pherald doctor` subcommand to bring Postgres + Redis + OTel up on-demand.

**Build + test:** from the repo root:

```bash
go build ./commons/... ./commons_prefix/... ./commons_messaging/... ./commons_storage/... ./commons_constitution/... ./commons_infra/... ./pherald/...
go test -race -count=1 ./commons/... ./commons_prefix/... ./commons_messaging/... ./commons_storage/... ./commons_constitution/... ./commons_infra/... ./pherald/...
```

Tests pass on Go 1.25+ (verified on 1.26). Workspace is configured via `go.work` listing **18** Herald modules: the 10 shared/foundation modules (`commons`, `commons_auth` (Wave 3a JWT verifier / Gin middleware), `commons_constitution`, `commons_infra`, `commons_messaging`, `commons_prefix`, `commons_storage`, `commons_tls` (Wave 4a auto-cert loader for the dual HTTP/3+TLS listener), `commons_watch` (fsnotify + WAL-poll change watcher for the workable-items SSoT), `commons_workable` (workable-items SQLite SSoT store + per-property change-feed — the ATMOSphere integration substrate)) plus the 8 flavor binaries (`pherald`, `sherald`, `cherald`, `bherald`, `rherald`, `iherald`, `scherald`, plus `qaherald` (Wave 5 — QA bot, pherald ↔ Telegram round-trip automation per §107.x docs/qa evidence mandate)). `go.work` itself is gitignored per spec §9.1; a fresh clone needs `go work init && go work use ./...` (or copy the existing snippet from the project's local working tree). The 9 Helix-stack submodules under `submodules/` are referenced via `replace` directives in the consuming modules' `go.mod`, not via `go.work`.

**Anti-bluff verification (run before any release tag or risky commit):**

```bash
scripts/audit_antibluff.sh      # 3 invariants: §11.4 anchor + tests + paired meta
scripts/codegraph_validate.sh   # CodeGraph index integrity (7 probes)
scripts/e2e_bluff_hunt.sh       # 14 end-to-end checks against real services
```

CodeGraph index lives in `.codegraph/` (`codegraph.db` + `config.json`, both gitignored). Rebuild with `scripts/codegraph_setup.sh` when source layout changes; `codegraph_validate.sh` will FAIL otherwise.

`scripts/e2e_bluff_hunt.sh` is the canonical end-to-end smoke per Universal §11.4. It builds pherald, runs the full test suite, starts a real Gin server + hits every /v1 route + asserts response bodies, boots a real Postgres container via the `containers/` submodule, runs the M2 integration tests against it, and graceful-shutdowns. ALL 14 invariants must PASS — a single FAIL means a real feature is broken for end users.

When the user asks to "add a feature" the spec is the source of truth — find the relevant §, then the relevant module + package, then the relevant HRD-NNN if one is already open. New work opens a new HRD-NNN in `docs/Issues.md` per V3 §8.3 lifecycle.

Do not invent build/test commands beyond what `go test ./<module>/...` provides. Live-integration tests (Telegram bot, Claude Code session, real Postgres) require operator-supplied credentials — see `docs/CONTINUATION.md` for the live-test handoff prompt.

## End-user-usability covenant (Herald §107 / Helix §11.4 — MANDATORY ANTI-BLUFF)

**Forensic anchor — verbatim operator mandate:**

> "all existing tests and Challenges do work in anti-bluff manner - they MUST confirm that all tested codebase really works as expected! We had been in position that all tests do execute with success and all Challenges as well, but in reality the most of the features does not work and can't be used! This MUST NOT be the case and execution of tests and Challenges MUST guarantee the quality, the completition and full usability by end users of the product! This MUST BE part of Constitution of our project, its CLAUDE.MD and AGENTS.MD if it is not there already, and to be applied to all Submodules's Constitution, CLAUDE.MD and AGENTS.MD as well (if not there already)!"

The bar for shipping any Herald feature is **NOT** "tests pass" — it is **"the end user of the flavor binary can actually use the feature."** Every PASS (unit, integration, gate, Challenge, smoke, e2e) MUST carry positive runtime evidence that the user-visible behaviour works. Metadata-only / configuration-only / "absence-of-error" / grep-only PASS are §11.4 PASS-bluffs and constitute critical defects regardless of how green the summary line looks. Canonical Herald authority: `docs/guides/HERALD_CONSTITUTION.md` §107. Canonical Helix authority: `<discovered>/Constitution.md` §11.4 + §11.4.1..§11.4.16 and `<discovered>/CLAUDE.md` "MANDATORY ANTI-BLUFF COVENANT — END-USER QUALITY GUARANTEE". Canonical Herald evidence: `scripts/e2e_bluff_hunt.sh` (14 invariants against real services; ALL must PASS). Inheritance gate invariant **I8a** asserts this covenant anchor is present in this file.

## §107.x — docs/qa/ Evidence Mandate (operator mandate, 2026-05-22; cascades from Helix §11.4.83)

**Forensic anchor — verbatim operator mandate (2026-05-22):**

> "every feature that ships MUST carry a recorded e2e communication transcript + any attached materials under `docs/qa/<run-id>/` (per-feature subdirectories). A feature with no QA transcript is itself a §107 PASS-bluff — it claims to work but has no auditable runtime evidence. Bot-driven automation (e.g. Herald's planned `qaherald` binary) MUST preserve full bidirectional communication threads as proof."

Every Herald feature that ships — every flavor binary (`pherald`, `sherald`, `cherald`, `bherald`, `rherald`, `iherald`, `scherald`, future `qaherald`) and every `/v1/*` route they expose — MUST carry a recorded end-to-end communication transcript plus all attached materials (Telegram screenshots, Gin response bodies in JSON or TOON, OpenTelemetry trace exports, container logs) committed under `docs/qa/<run-id>/`. A Herald feature that ships without a `docs/qa/<run-id>/` directory is by definition a §107 PASS-bluff — its e2e_bluff_hunt PASS line claims it works for end users, but no auditable runtime evidence proves an end user ever exercised it.

Operative rule for Herald. (1) Every Herald HRD-NNN (V3 §8.3) work-item that introduces a user-visible feature MUST land its `docs/qa/HRD-NNN/` directory (timestamp-prefixed if multiple runs) in the same logical work effort. (2) Bidirectional transcripts only — for Telegram-driven features (HRD-011, planned `qaherald`), capture both directions of the bot conversation; for Gin /v1 routes, capture both the request payload and the full response body. (3) Attached materials commit verbatim — never link to external Slack/Drive/Telegram URLs; the artefact lives in-repo. (4) The planned `qaherald` binary (HRD-NNN to be assigned) is Herald's QA bot: it drives pherald ↔ Telegram round-trips and preserves the full conversation thread under `docs/qa/qaherald-<TS>/`. (5) Release gates (`scripts/release.sh` when implemented + the existing tag-time guard) REFUSE to tag a release whose feature-shipping commits lack their `docs/qa/<run-id>/`. (6) `scripts/e2e_bluff_hunt.sh` invariants for new features MUST cite the `docs/qa/<run-id>/` artefact as their positive-evidence anchor (§11.4.2 / §11.4.5 composition).

**Cascade authority.** Helix Universal Constitution §11.4.83 (the verbatim operator mandate is anchored there). Herald §107.x is the project-binding restatement.

**Enforcement.** A feature commit that lacks `docs/qa/<run-id>/` triggers FAIL at the release-gate layer. The §11.4.4 test-interrupt-on-discovery applies — the entire release cycle stops until evidence lands.

**Non-compliance is a release blocker.** No `--qa-evidence-optional`, `--qa-transcript-later`, `--qa-bot-summary-suffices` flag exists for Herald.

## §107.y — Working-Tree Quiescence Rule (operator mandate, 2026-05-22; cascades from Helix §11.4.84)

**Short tag:** `working-tree quiescence`.

**Forensic anchor — verbatim operator mandate (2026-05-22):**

> "no subagent commit may proceed while any concurrent mutation gate is in flight in the same checkout. Before `git add`, the committing agent MUST `grep` its own working tree for mutation markers (`MUTATED for paired`, `// always pass`, `return json.Marshal` shortcut paths, etc.). Any unexplained file in the staging area triggers ABORT."

**Lesson (forensic case study — Herald-internal).** On 2026-05-21 a logo-fix subagent (commit `72e81ab`, "Fix: replace pandoc {width=96px} image attr with HTML <img> tag") ran in this very checkout while a paired §1.1 Wave 4b mutation gate had temporarily injected an `// always pass` shortcut into `commons_auth/middleware.go` (JWT-bypass mutation, intended for the paired-mutation cycle to mutate → assert FAIL → restore). The logo-fix subagent's `git add` swept the mutation residue into its commit; the resulting commit was pushed to all four mirrors before any other agent caught it. Within the hour the SECURITY FIX (`d5bd360`, "SECURITY FIX: restore commons_auth/middleware.go JWT verify (mutation residue in 72e81ab)") restored the verify path, but the production-equivalent-binary-with-bypassed-JWT window is a real security-defect window — small, but non-zero, and demonstrably exploitable during that interval. This is no longer hypothetical; it happened. The rule below is the constitutional outcome.

**Operative rule for Herald.**

1. **Pre-`git add` quiescence check.** Every commit flow (main thread + subagent) MUST grep the working tree for the canonical Herald mutation markers BEFORE `git add`:
   - `MUTATED for paired` (the canonical paired-§1.1 marker emitted by `tests/test_wave4b_mutation_meta.sh`)
   - `// always pass`, `// MUTATION`, `# MUTATION` (Go + shell mutation annotations)
   - `return json.Marshal` shortcut paths in commons or commons_messaging (Wave 4b TOON mutation residue)
   - `_mutated_*` filename suffixes
   - `.git/MUTATION_IN_PROGRESS` (the lockfile — see (3))
2. **Scope-match.** Cross-check `git status --porcelain` against the subagent's declared scope. Any file outside the declared scope → ABORT. The subagent MUST explicitly account for every modified / untracked / staged entry.
3. **Lockfile serialisation.** When any mutation gate is in flight, the gate's first action is `touch .git/MUTATION_IN_PROGRESS`; its last action (in trap-on-exit) is `rm .git/MUTATION_IN_PROGRESS`. Any subagent that finds this lockfile present MUST refuse to `git add` and ABORT until the gate completes the mutate → assert-FAIL → restore → assert-PASS cycle and removes the lockfile.
4. **Worktree isolation (preferred).** When parallel subagents are required (§11.4.20 / §11.4.70 subagent-driven default), prefer `git worktree add` per subagent over single-checkout concurrency — eliminates the cross-mutation race by construction.
5. **Pre-push mutation-residue scanner.** `scripts/mutation_residue_audit.sh` (to be implemented; HRD-NNN to be assigned) MUST run before every push. Any commit in the pushed range containing a mutation marker → push BLOCKED, commit MUST be reverted or amended before mirrors are updated.

**Prototype enforcement.** `tests/test_wave4b_mutation_meta.sh` ALREADY includes a working-tree quiescence check (`check_quiescence()` at line 92; assertion at line 197 — "Working-tree quiescence — assert no MUTATED markers leaked"). It is the canonical prototype. Generalising it across all paired-§1.1 gates is open work; the universal scanner (`scripts/mutation_residue_audit.sh`) is the planned roll-out.

**Cascade authority.** Helix Universal Constitution §11.4.84. Herald §107.y is the project-binding restatement.

**Enforcement.** A mutation marker that lands in any tagged Herald commit is a critical defect regardless of how briefly it persisted — see commit `72e81ab` / `d5bd360` as proof.

**Non-compliance is a release blocker.** No `--allow-residue`, `--skip-quiescence`, `--mutation-cleanup-later` flag exists.

## Inherited covenant restatements — Helix §11.4.85 / §11.4.87 / §11.4.88 / §11.4.89 / §11.4.90 / §11.4.91 / §11.4.92 / §11.4.93 / §11.4.94 / §11.4.95 / §11.4.96 / §11.4.97 / §11.4.98 / §11.4.99 / §11.4.100 / §11.4.106 (inherited per §11.4.35)

These twelve mandates are **inherited** from the HelixConstitution via parent-discovery (§11.4.35). Herald **restates + cites** them here — it does **NOT** redefine or weaken them. The literal anchors below are required by the §11.4.87 `CM-COVENANT-114-87-PROPAGATION` pre-build gate, which asserts that `11.4.85`, `11.4.87`, and `11.4.88` appear in every consuming repo's `CLAUDE.md` / `AGENTS.md` / `QWEN.md`.

### §11.4.85 — Stress + Chaos Test Mandate (Helix, 2026-05-24)

Every fix or improvement landed in Herald MUST ship with full-automation **stress** AND **chaos** test suites — sustained/concurrent load + failure-injection (process-death, network-fault, input-corruption, resource-exhaustion, state-corruption) — each PASS citing a captured-evidence artefact per §11.4.5 + §11.4.69. A fix that PASSes its happy-path test but has never been exercised under stress or fault-injection is a §11.4 / §107 PASS-bluff at the resilience layer. For Herald this binds the flavor binaries (`pherald listen` inbound long-poll under concurrent updates, Gin `/v1/*` routes under sustained load, claude_code dispatch under process-death, container flows under disk/OOM pressure) — stress + chaos evidence lands under `docs/qa/<run-id>/stress_chaos/` and is cited by the matching `e2e_bluff_hunt` invariant. Canonical authority: HelixConstitution Constitution.md §11.4.85 (inherited per §11.4.35).

### §11.4.87 — Endless-loop autonomous work + zero-idle agent dispatch + anti-bluff testing (Helix, 2026-05-26)

When the operator instructs Herald work to "continue in endless loop fully autonomously" (or equivalent), it is a HARD-CONTRACT covenant: (A) continue until ALL are simultaneously TRUE — Herald's autonomous loop checks `docs/Issues.md` Status-column has zero `In progress`/`Ready for testing`/`In testing`/`Reopened` entries, `docs/CONTINUATION.md` §3 "Active work" is empty, the TaskList reports no subagent mid-execution, and no in-flight external dependency (build, push, sync) remains; (B) dispatch background subagents for parallelisable, non-contending work rather than serialising — idle is permitted ONLY while waiting on a result; (C) every closed item lands four-layer test coverage (§11.4.4(b)) with real captured-evidence PASS; (D) anti-bluff end-to-end — tests AND Challenges are bound equally; (E) the loop terminates only on all-clear, an explicit operator `STOP`, a §12 host-session-safety demand, or a scheduled wake against a known-future-actionable signal. No `--idle-OK` / `--skip-endless-loop` / `--metadata-only-test-suffices` escape hatch exists. Canonical authority: HelixConstitution Constitution.md §11.4.87 (inherited per §11.4.35).

### §11.4.88 — Background-push mandate: commit-lock release immediately after commit, push runs detached (Helix, 2026-05-26)

Herald commit flows MUST release the commit-lock (`.git/.commit_all.lock`) the instant `git commit` returns 0 — BEFORE any push runs — because the commit is durable on local disk regardless of push outcome; gating further local work on a remote round-trip is the exact zero-idle anti-pattern §11.4.87 prohibits. The push then runs **detached** (`nohup ./push_all.sh ... &` then `disown`), with per-remote flock (`.git/.push.<remote>.lock`) serialising same-remote invocations while DIFFERENT remotes (GitHub / GitLab / GitFlic / GitVerse) push in parallel. Backgrounded push failures land in `qa-results/push_failures/<TS>_<remote>.log` and the next autonomous-loop tick MUST surface them (silent push-failure is a §11.4 PASS-bluff at the distribution layer). The ONLY synchronous-push escape is the explicit `--sync-push` flag for §11.4.41 force-push merge-first paths. Canonical authority: HelixConstitution Constitution.md §11.4.88 (inherited per §11.4.35).

### §11.4.89 — Background test execution mandate (Helix, 2026-05-27)

Any Herald test cycle expected to exceed ~30s — the `tests/test_wave*_mutation_meta.sh` gates, `scripts/e2e_bluff_hunt.sh`, future stress/chaos suites — MUST run detached (`nohup … > qa-results/<test_id>_<TS>.log 2>&1 &` + `disown`); the main work stream returns immediately to the §11.4.42 priority queue and polls the log/exit-status rather than blocking. Composes with §107.y / §11.4.84: a backgrounded mutation gate still mutates the shared tree, so only ONE runs against the main checkout at a time (serialised by the `.git/MUTATION_IN_PROGRESS` lockfile + `scripts/mutation_residue_audit.sh` pre-push scanner); concurrent gates require separate `git worktree` checkouts. Foreground permitted only for <30s tests or on explicit operator request. Canonical authority: HelixConstitution Constitution.md §11.4.89 (inherited per §11.4.35).

### §11.4.90 — Obsolete status + per-item obsolescence audit (Helix, 2026-05-27)

Herald's HRD Status closed-set gains a 4th terminal value `Obsolete (→ Fixed.md)` for items no longer valid (Reason closed-set: superseded-by-design-change / superseded-by-later-mandate / feature-removed / duplicate-of / unsupported-topology). Every `Obsolete` HRD heading MUST carry an `**Obsolete-Details:**` line within 8 non-blank lines: Since (ISO date), Reason, Superseding-item (§/HRD ref), Triple-check evidence (git-log/grep/runtime path per §11.4.6 — bare assertion forbidden). At every release-gate sweep, re-evaluate every open + Fixed HRD for obsolescence; migrations are atomic per §11.4.19. Canonical authority: HelixConstitution Constitution.md §11.4.90 (inherited per §11.4.35).

### §11.4.91 — Summary-doc clarity (Helix, 2026-05-27)

Every one-liner in `docs/Issues_Summary.md` / `docs/Fixed_Summary.md` / `docs/Status_Summary.md` / README doc-link rows MUST be a self-contained clause (≥6 words OR ≥40 chars) naming the SUBJECT + the PROBLEM/GOAL — never a section-label fragment (`Composes with`, `Closure criteria`), bare metadata (`Critical`, `Bug`), a status restatement, or a bare HRD-id. Derive each from the source long-form H1/H2 heading, never invented. Canonical authority: HelixConstitution Constitution.md §11.4.91 (inherited per §11.4.35).

### §11.4.92 — Multi-pass change-evaluation discipline (Helix, 2026-05-27)

Every non-trivial Herald change MUST pass — and document — a 5-pass evaluation before it is commit-ready: Pass 1 main-task captured-evidence (§11.4.5, no "should work"); Pass 2 regression-blast-radius (every touched file + every importer/caller audited); Pass 3 cross-feature interaction (shared state/timing/env — e.g. a gate edit checks §107.y quiescence + §11.4.89 backgrounding); Pass 4 deep-research validation (§11.4.8 — external precedent or literal "NO external solution found"); Pass 5 anti-bluff confirmation (no metadata-only/config-only/script-bug PASS). Evidence lands in the commit footer or `docs/qa/`/`qa-results/`. Trivial changes (typo, revision-bump, MD-export regen touching zero source) are exempt only with explicit commit-message citation. Canonical authority: HelixConstitution Constitution.md §11.4.92 (inherited per §11.4.35).

### §11.4.93 — SQLite-backed single-source-of-truth for workable items (Helix, 2026-05-27)

Herald's text-based HRD trackers (`docs/Issues.md` / `Fixed.md` / `*_Summary.md` / `Status.md` / `CONTINUATION.md` §3) migrate to a SQLite single-source-of-truth (`docs/.workable_items.db`, **version-controlled as Herald's authoritative SSoT** per operator mandate 2026-05-27 — a deliberate Herald divergence from the parent §11.4.93 gitignored-with-regeneration default, since for Herald the DB IS the authoritative artefact and MUST be committed, not regenerated-on-clone; only the transient WAL/SHM sidecars stay ignored) with bidirectional MD↔DB regeneration so sync-drift is mechanically impossible. The Go binary lives in the constitution submodule (`constitution/scripts/workable-items/`) — Herald references it per §11.4.74 catalogue-first, never reimplements. Migration is 6-phase; Herald files the tracking HRD (Phase 1) and progresses incrementally. Canonical authority: HelixConstitution Constitution.md §11.4.93 (inherited per §11.4.35).

### §11.4.94 — Zero-idle priority-first parallel-by-default operating mode (Helix, 2026-05-27)

Binding always-on contract: Herald work is NEVER idle while a priority-queued item can progress. Before any wake/sleep/"waiting for X", survey the priority queue, identify all non-contending items, and dispatch them in parallel (subagent-driven per §11.4.20/§11.4.70 when non-trivial; background per §11.4.89 when >30s). Pick highest-Severity/priority first. The conductor remains the integration + commit + push seam; parallel work MUST NOT compromise stability (composes with §107.y quiescence + §11.4.92 multi-pass + §12 host-safety). Idle is permitted ONLY when every item is genuinely blocked on external dependency, the operator issued STOP, or §12 host-session-safety demands it. Canonical authority: HelixConstitution Constitution.md §11.4.94 (inherited per §11.4.35).

### §11.4.95 — Workable-items SQLite DB is TRACKED in git, never gitignored (Helix, 2026-05-27)

Herald's `.gitignore` ALREADY encodes this policy (operator correction 2026-05-27, recorded in §108.h): WHEN the workable-items SQLite DB is materialized it is version-controlled — committed + pushed alongside every state change, WAL-checkpointed (`PRAGMA wal_checkpoint(TRUNCATE)`) before commit so only the transient `-wal`/`-shm` sidecars stay gitignored, and never force-rewritten without a §9.2 hardlinked-backup. This is an explicit §11.4.30 carve-out and AMENDS the earlier §11.4.93 / §108.h "gitignored-with-regeneration" text. **Materialization status (anti-bluff, 2026-05-30):** the DB artefact is NOT YET present in the Herald repo — `git ls-files '*.db'` is empty and no `docs/workable_items.db` exists on disk. HRD-131 Phase 2+ (the in-repo DB) is deferred; the workable-items DB has so far been materialized only on the ATMOSphere host (per CONTINUATION r22), not committed into Herald. This clause therefore states the binding policy for WHEN the DB lands, NOT a claim that a committed DB exists today. **Path note:** the constitution's canonical path is `docs/workable_items.db` (no leading dot); Herald's HRD-131 currently references `docs/.workable_items.db` — reconcile to the canonical path when the DB is implemented. Canonical authority: HelixConstitution Constitution.md §11.4.95 (inherited per §11.4.35).

### §11.4.96 — Safe-parallel-work-with-long-build catalogue + mandate (Helix, 2026-05-27)

Herald has no AOSP build, but the principle binds: during ANY long-running Herald operation (a backgrounded mutation gate, `scripts/e2e_bluff_hunt.sh`, a §11.4.85 stress/chaos suite, a doc export) the conductor MUST consult the safe-parallel catalogue and dispatch non-contending work rather than idle. SAFE-in-parallel for Herald: (A) MD/docs work, (B) `scripts/` helpers, (C) gate authoring, (D) test authoring, (E/F) commit + push to mirrors, (H) read-only analysis subagents, (I) web research, (J) workable-items DB ops. UNSAFE-during-a-running-gate (maps to §107.y): `git checkout` / `reset --hard` / `clean` on files a gate is transiently mutating, a SECOND concurrent mutation gate against the same checkout, host-session-safety breaches (§12). Subagent-driven default per §11.4.20 / §11.4.70. Canonical authority: HelixConstitution Constitution.md §11.4.96 (inherited per §11.4.35).

### §11.4.97 — Maximum-use-of-idle-time mandate + progress-update cadence (Helix, 2026-05-27)

(A) Every minute of conductor idle time during which progressable, non-externally-blocked work exists is a violation — dispatch work continuously through the whole idle window, not just at scheduled wakes. (B) Emit concise (1–3 line) operator-facing progress updates at milestone boundaries with NO prompt required: every HEAD advance (what landed), every subagent return (integrated), every constitutional anchor propagated, every captured-evidence artefact (`docs/qa/` / `qa-results/` path), every Issues→Fixed/Obsolete closure. (C) Continuous physical-proof gathering per §11.4.5 / §11.4.6 / §11.4.69 — every closed item carries positive captured evidence committed alongside the closure. (E) Idle ONLY when genuinely blocked (operator STOP, external dependency, §12 host-safety). Canonical authority: HelixConstitution Constitution.md §11.4.97 (inherited per §11.4.35).

### §11.4.98 — Full-Automation Anti-Bluff Mandate — Live tests MUST be re-runnable end-to-end without manual intervention (Helix, 2026-05-28)

Every Herald test — unit/integration/e2e/Challenge/stress/chaos/live — MUST be fully self-driving end-to-end with NO human action during execution (operator typing a Telegram message, hand-triggering a webhook, clicking a UI, attaching a file, anything beyond test startup → PASS/FAIL report). A test requiring manual action during execution is **by definition a §11.4 PASS-bluff at the automation layer**, regardless of how thorough the manual run is — it cannot run in CI, cannot validate regressions between manual runs, and the human dependency masks drift. Single permissible exception: one-time credential bootstrap OUTSIDE test execution (`.env` populated from a vault, shell exports in `~/.bashrc`, OAuth approval at first install, MTProto session activation at first run — configuration, not test driving). Concrete Herald requirements: (1) no "operator MUST type a message" prompts in `tests/test_*.sh` or `_integration_test.go` — drive programmatically via MTProto user-account (Telegram), real-user-API (Slack), webhook fixture, or in-process loopback; (2) no `claude --resume <UUID>` against the same session UUID the dev conductor is using (Herald 2026-05-28 lesson: silent exit -1 collision — use a dedicated test-only UUID); (3) no 60s human-response windows (§11.4.50 determinism violation); (4) PASS at `-count=3` consecutive automated runs with self-cleaning state; (5) every existing test classified COMPLIANT vs NON-COMPLIANT in the §11.4.98 audit (release-gate item); (6) no false-positive PASS — silent-skip-as-PASS forbidden, stale-evidence forbidden, §11.4.3 SKIP-with-reason is correct. Currently NON-COMPLIANT Herald tests scheduled for MTProto-driven rewrite: `TestSubscribe_LiveBotAPI`, `tests/test_wave6_live_loop.sh`, Wave 6.5 lifecycle scenarios. Canonical authority: HelixConstitution Constitution.md §11.4.98 (inherited per §11.4.35).

### §11.4.99 — Latest-Source Documentation Cross-Reference Mandate — instructions/guides/manuals MUST be verified against latest official online sources BEFORE publication (Helix, 2026-05-28)

Every Herald operator-facing instruction document — `docs/requirements/blockers/*.md`, `docs/guides/*.md`, README operator-action sections, troubleshooting cookbooks, `OPERATOR_CREDENTIALS.md`, `MESSENGER_CHANNELS.md`, `TELEGRAM.md`, `HERALD_CONSTITUTION.md`, integration setup walkthroughs — MUST be cross-referenced against the LATEST official online documentation of every external service/library being documented BEFORE the commit lands. Misguidance-by-stale-docs is a §11.4 PASS-bluff at the documentation layer. Anchoring case study (Herald 2026-05-28): first-draft MTProto setup guide recommended VoIP numbers + omitted the `recover@telegram.org` pre-login email — both directly contradicted Telegram's official docs (https://core.telegram.org/api/obtaining_api_id) and the gotd/td maintainer guidance (vendored at `submodules/gotd-td/.github/SUPPORT.md`); following the original guide could have permanently banned the operator's Telegram account. (A) Pre-commit cross-reference workflow: fetch latest official docs via WebFetch/MCP/direct browsing (NEVER training data, memory, or prior committed docs); cross-reference each instruction; seek secondary authoritative sources (library maintainer SUPPORT.md, official changelogs, vetted community FAQs) when the official source is sparse/silent; cite source URLs + date in a `## Sources verified` footer on the document; cite the cross-reference in the commit-message footer (`Sources verified <date>: <urls>`). (B) Negative findings documented explicitly — gaps/silences/contradictions never silently assumed authoritative. (C) Re-verification cadence: 6-month default staleness, 90-day staleness for risk-classified services (Telegram/messenger/cloud/payment/AI-LLM/code-hosting/package-managers); re-verify before operator-authority citation, at every vN.0.0 release boundary, on service breaking-change announcements, on operator-error report. (D) Risk-classified services MUST include explicit safety warnings cross-referenced against latest published policies. (E) Composes with §11.4.92 Pass 4 INDEPENDENT — cannot substitute. (F) Inheritance per §11.4.35 with literal anchor `11.4.99`. (G) Enforcement: missing `Sources verified` footer (doc + commit) → release-gate BLOCK; stale-beyond-grace → §11.4.90 Obsolete with `Reason=stale-documentation`. Canonical authority: HelixConstitution Constitution.md §11.4.99 (inherited per §11.4.35).

### §11.4.100 — Video color + visual-quality fidelity mandate — every video-playback PASS MUST carry captured-frame deep-analysis proving rendered output matches source within tolerance (Helix, 2026-05-28)

Universal mandate (§11.4.17 classification): every user-visible video-playback test PASS MUST carry a captured-frame deep-analysis artefact proving the device's actually-rendered output matches the host-extracted ground-truth source within documented tolerance, across the closed-set checks — ΔE2000 + RGB/HSV histogram correlation + no pale/washed-out + no over-saturation + no hue shift + gamma/luma fidelity (the BT.601/709/2020 + full-range-vs-limited-range negotiation is the single most common real-world cause of pale rendering and MUST be exercised explicitly), sharpness (Laplacian-variance ≥ source within tolerance), aspect ratio (no stretch / letterbox-crop), frame rate + speed (sustained window), and continuity (no freeze ≥1s SSIM>0.99, no drop-burst, no glitch, no obstruction-overlay per the §11.4.5 OCR census). Metadata-only PASS (file exists / frames>0 / codec registered) is forbidden. Comparison harness MUST itself be golden-pair + bad-pair self-validated to prevent false-positive AND false-negative. **Herald applicability:** Herald is a messaging/notification system with NO video-playback surface — `pherald listen` downloads video attachments (`OnVideo` / `OnDocument` MIME-typed `video/*`) to `~/.herald/inbox/<sha>.<ext>` as opaque sha256-content-addressed blobs (HRD-005 attachment pipeline) but never decodes / renders / displays them; the Claude Code dispatcher receives the file path as text reference, not a rendered frame. The mandate binds latently — if Herald ever ships a video-rendering surface (preview thumbnails in a future dashboard flavor, an in-app player, an inline diff viewer), §11.4.100 applies in full at that point. Current Herald-side compliance: opaque-blob handling validated by sha256 + content-length + MIME match (no rendering claim made, so no rendering-fidelity claim to bluff). Cascade-pattern parallel: §11.4.96 ("Herald has no AOSP build, but the principle binds") — same restatement-with-non-applicability-but-cite shape. Inheritance per §11.4.35 with literal anchor `11.4.100` required by the upcoming `CM-COVENANT-114-100-PROPAGATION` pre-build gate. Canonical authority: HelixConstitution Constitution.md §11.4.100 (inherited per §11.4.35).

### §11.4.106 — Docs Chain documentation-sync mandate — derived doc artefacts MUST be kept in sync via the shared Docs Chain engine, never ad-hoc (Helix, 2026-05-29)

Universal mandate: the documentation-sync obligations (Issues_Summary parity §11.4.12, Fixed_Summary §11.4.53, Status/Status_Summary §11.4.45/§11.4.56, README doc-link + always-sync export §11.4.57/§11.4.59, universal markdown→html/pdf export §11.4.65, roster fingerprint §11.4.86, workable-items DB↔MD §11.4.93/§11.4.95, CONTINUATION exports §12.10) are mechanized by **Docs Chain** — a content-hashed incremental DAG sync engine (`digital.vasic.docs_chain`) consumed BY REFERENCE through the constitution submodule (Phase-6 distribution is operator-gated per §11.4.66). A consuming project registers its chains in `.docs_chain/contexts/*.yaml` and runs `docs_chain sync` / `verify`; the engine **refuses to fake success** (tool-absent → honest rollback, exit 3) and **refuses silent merges** (both-dirty → conflict, exit 2), making §11.4.6 no-guessing + §11.4.50 deterministic-consistency mechanical rather than aspirational. **Herald applicability: APPLICABLE (integration in progress).** Herald's ~76 markdown docs each carry html/pdf/docx siblings produced today by the regen-all `scripts/export_docs.sh`; the migration plan ([`docs/research/docs_chain/HERALD_DOCS_CHAIN_PLAN.md`](docs/research/docs_chain/HERALD_DOCS_CHAIN_PLAN.md)) wires them via docs_chain `exec:` transforms (preserving Herald's exact logo-CSS pandoc flags — byte-compatible, zero regen-drift across ~223 siblings) to gain incremental recompute + a `verify` drift-gate (planned e2e invariant E146). Pending: the Phase-4 CLI (landed 2026-05-31), the exec-staging relative-asset contract (gap G6), and the binary-hash `verify` fix found by dogfooding (binary node-kinds were hashed inconsistently between the sync-record and verify-check paths). Cascade-pattern parallel: §11.4.96 ("Herald has no AOSP build, but the principle binds") — restatement-with-applicability-and-cite shape. Inheritance per §11.4.35 with literal anchor `11.4.106` required by the `CM-COVENANT-114-106-PROPAGATION` pre-build gate. Canonical authority: HelixConstitution Constitution.md §11.4.106 (inherited per §11.4.35).

## Participant identity, attribution & notification-tagging (Herald §109 / contract `docs/design/PARTICIPANT_ATTRIBUTION.md` — inherited from HelixConstitution per §11.4.35)

**Operator mandate (2026-05-31).** Every messenger must relate each message to a **Participant** (a logical Subscriber/User — one person/agent, with a potentially DIFFERENT username on every messenger); workable items gain `created_by` + `assigned_to` attribution; outbound notifications **@-tag** the right participant per a fixed rule matrix. The **single authoritative contract** every implementation stream codes against is [`docs/design/PARTICIPANT_ATTRIBUTION.md`](docs/design/PARTICIPANT_ATTRIBUTION.md). These rules are restated (root definitions) in HelixConstitution `Constitution.md` / `CLAUDE.md` / `AGENTS.md` / `QWEN.md` and inherited here per §11.4.35 — Herald restates + cites, it does NOT redefine or weaken.

- **Identity model.** A Participant is backed by PG `subscribers` (logical party: canonical messenger-neutral `handle`, `display_name`, `kind ∈ {human, agent, service}`) + `subscriber_aliases` (per-channel handle: `subscriber_id`, `channel`, `channel_user_id`, **+ NEW `username TEXT`** — the per-channel `@handle` used for tagging, distinct from `channel_user_id`). The **canonical handle** (the string stored in `created_by`/`assigned_to`) is either `Claude` (the reserved system-agent sentinel; `kind=agent`; NEVER tagged) or a human's canonical handle (defaults to their Telegram `@username` since Telegram is the primary messenger, but is messenger-neutral; per-channel `@username`s resolve via `subscriber_aliases`).
- **Operator env var.** The one human who drives the system via the Claude Code CLI is designated by env var, NOT a DB flag: `HERALD_TGRAM_OPERATOR_USERNAME` (e.g. `@milos85vasic`), generalizing to `HERALD_<CHANNEL>_OPERATOR_USERNAME` (`HERALD_SLACK_OPERATOR_USERNAME`, …). The operator's canonical handle equals that env value; the operator is a normal Participant.
- **Attribution.** `created_by` = `OperatorHandle()` when opened via the Claude Code CLI prompt; `"Claude"` when System/Claude detects the issue/task/improvement/missing-feature; the sender's resolved canonical handle when received through Herald (a subscriber message). `assigned_to` defaults to `OperatorHandle()`, overridable explicitly. Both columns store the canonical handle string.
- **Tagging matrix.** On any workable-item event, the notification to each channel @-mentions: the `assigned_to` handle if it is a human AND ≠ Operator; the `created_by` handle if it is a human AND ≠ Operator AND ≠ `"Claude"`. `"Claude"` is NEVER tagged (it is the system); the Operator is NEVER tagged (no self-ping). De-dup, then resolve each handle to that channel's `@username` (skip if the participant has no alias on that channel). Concretely: assigned-to-Operator → no tag; opened-by-Operator-assigned-to-another → tag the assignee; opened-by-a-non-Operator-non-Claude subscriber → tag them.

The anti-bluff covenant (§107 / Helix §11.4) is a hard precondition of every PASS for this feature — every layer ships unit + integration + E2E + full-automation tests producing real captured evidence under `docs/qa/<run-id>/` (the tagging matrix proven by a truth-table test with a per-cell mutation, plus a NEGATIVE case proving the Operator is NOT tagged).

## Intent recognition & clarification (Herald §110 / contract `docs/design/INTENT_RECOGNITION.md` — inherited from HelixConstitution §11.4.105 per §11.4.35)

**Operator mandate (2026-05-31).** Subscribers must **NOT** need to know command syntax — there is no `COMMAND:` prefix and no fixed grammar. A subscriber sends a clear natural-language message; **the System determines the intent**. The **single authoritative contract** every implementation stream codes against is [`docs/design/INTENT_RECOGNITION.md`](docs/design/INTENT_RECOGNITION.md). These rules are restated (root definitions) in HelixConstitution `Constitution.md` / `CLAUDE.md` / `AGENTS.md` / `QWEN.md` as §11.4.105 (the root-§ being added on the constitution stream) and inherited here per §11.4.35 — Herald restates + cites, it does NOT redefine or weaken.

- **Three-tier resolution (first tier that succeeds wins).** **Tier 1 — command recognition:** a deterministic `CommandRecognizer` (`pherald/internal/inbound`) maps a clear natural-language imperative to a structured action WITHOUT an LLM round-trip (fast-path; no prefix needed). It is deliberately CONSERVATIVE — only a confident match (clear imperative verb + resolvable target) fast-paths; when in doubt it returns "no match" and defers to Tier 2. **Tier 2 — intent inference:** when no command matches, the Claude Code dispatch (the LLM) infers intent from the message and returns a `<<<HERALD-REPLY>>>` action; the `<<<HERALD-DISPATCH-v1>>>` envelope INSTRUCTS the LLM to recognize Herald's command set, map natural language to the right action, and NEVER guess. **Tier 3 — clarify (fallback):** when neither a command nor a confident intent can be determined, the resolution is `action="clarify"` — the System REPLIES to the original message, TAGS the sender (`@username`, resolved via the §11.4.104 `IdentityResolver`), and asks a precise clarifying question naming the candidate intents.
- **Never guess, never ignore.** A wrong action is worse than a clarifying question (§11.4.6 no-guessing). Tier 3 is the anti-annoyance guarantee — the subscriber is never silently dropped and never has to learn syntax; at worst they get a friendly, specific "@user, did you mean X or Y?".
- **Command set (Tier 1).** Natural-language imperatives map (case-insensitive, phrasing-tolerant) to the existing inbound actions: close/mark-done/assign/status-change → `item.update`; "open a bug/task/feature: …" → `issue.open` (created_by=sender); "investigate ATM-N" → `investigation.start`; a question / conversational text → `reply`; genuinely ambiguous → `clarify`. See contract §2 for the full table.

The anti-bluff covenant (§107 / Helix §11.4) is a hard precondition of every PASS — every tier ships unit + integration + E2E + full-automation tests with real captured evidence under `docs/qa/<run-id>/` (a Tier-1 truth-table including the conservative negatives that MUST fall through to "no match"; a Tier-3 E2E asserting the reply body is EXACTLY `@<sender> <specific question>`; a paired §1.1 mutation that breaks the confidence guard or drops the clarify tag MUST FAIL).

## Mission (from the spec)

> Ingesting system events and reliably fanning them out to multiple notification channels so every alert reaches the right destination without confusion.

The spec mentions "Integration into the Constitution" as a planned section — Herald is intended to plug into a larger governance/policy system that lives outside this repo. Treat that as a real constraint when designing interfaces, even though the details are TBD.

## Intended stack

Go, inferred from `.gitignore` (`*.test`, `go.work`, `go.work.sum`, `coverage.*`, `profile.cov`). When you scaffold, default to standard `cmd/` + `internal/` layout unless the user asks otherwise, and use `go test ./...` / `go build ./...` / `go vet ./...` — there is no project-specific tooling to override these yet.

## Multi-host mirror convention

`upstreams/` contains one script per mirror host:

- `GitHub.sh` — primary: `git@github.com:vasic-digital/Herald.git`
- `GitLab.sh` — `git@gitlab.com:vasic-digital/herald.git`
- `GitFlic.sh` — `git@gitflic.ru:vasic-digital/herald.git`
- `GitVerse.sh` — `git@gitverse.ru:vasic-digital/Herald.git`

Each script exports a single `UPSTREAMABLE_REPOSITORY` variable and is meant to be **sourced** (`. upstreams/GitHub.sh`), not executed for its output. The naming is intentional — capitalization matches the host's brand (GitFlic, GitVerse). Don't normalize these to lowercase or collapse them into one file; the per-host split is the interface other tooling (likely external mirror-push scripts) keys on.

When adding a new mirror, copy an existing script and change only the URL — keep the `#!/bin/bash` shebang, blank line, and `export` form identical so any consumer that greps these files keeps working.

## Inheritance gate (run before any commit touching root docs)

```bash
bash tests/test_constitution_inheritance.sh        # 6 invariants (I1–I6), 9 checks
bash tests/test_constitution_inheritance_meta.sh   # paired §1.1 mutation proof
```

The gate inline-walks parents for `<ancestor>/constitution/Constitution.md`. I5 is split into I5a–d (one check per root doc that must declare parent-discovery: `CLAUDE.md`, `AGENTS.md`, `docs/guides/HERALD_CONSTITUTION.md`, `README.md`). I6 forbids re-introducing `<repo-root>/constitution/` or `.gitmodules` — the §104 invariant.

If either script fails, fix at root cause per Universal §11.4.4. Never silently accept the FAIL.

## Spec-change rule (load-bearing — `docs/specs/mvp/specification.V4.md` §"Specification documents")

Any modification to a file under `docs/specs/` (any depth) triggers **mandatory comprehensive planning and implementation** of the implied changes — you may not edit the spec in isolation. This rule does **not** apply to creating or renaming files; for those, ask the operator what to do with the new path. Treat every spec edit as a project-wide ripple, not a doc tweak.

## Project conventions from the spec (apply when scaffolding)

These are declared in `docs/specs/mvp/specification.V4.md` and are easy to miss because no code enforces them yet:

- **Workable-item prefix:** `HRD-` (e.g. `HRD-001`). Use it for issues, status entries, fix logs.
- **Flavor binaries:** each Herald flavor ships as its own CLI binary, named `<prefix>herald` — `pherald` (Project Herald), `sherald` (System Herald), etc. Designed for CI / pipeline / cron / AI-agent invocation.
- **Layered shared code:** `commons` → `commons_messaging` (level 1) → … → flavor. Put new shared code in the **lowest layer that still makes sense**; flavors inherit upward. `commons_messaging` owns the Telegram / Max / Slack / Email / Markdown-export integrations.
- **Messenger integration priority:** Telegram → Max → Slack (then Email, then Markdown/PDF/HTML export). Microsoft Teams, Lark, Discord, WhatsApp, Viber are explicitly later iterations — don't pre-implement.
- **Conversation diary:** every message in/out is appended to `docs/herald/diary/main.md` and re-exported to `main.pdf` + `main.html` in sync. Don't break this invariant when designing channel I/O.
- **Container stack:** Postgres (main DB) + Redis (in-memory) bundled via the `containers` submodule (`https://github.com/vasic-digital/containers`). All container names start with `herald`; all host ports start with `70XXX` (70001, 70002, …) to avoid collisions.
- **Credentials:** `.env` (git-ignored) with a committed `.env.example`. Resolution order: exported shell vars from `.bashrc`/`.zshrc` load first; `.env` is fallback only (never overrides shell exports). **For step-by-step setup of every supported messenger + LLM dispatcher**, see `docs/guides/OPERATOR_CREDENTIALS.md` — comprehensive guide covering Postgres, Redis, Telegram (HRD-011), Claude Code (HRD-012), plus reserved env-var names for planned channels (Slack, Email, Max, Teams, …) and dispatchers. The guide includes an audit checklist to run before every commit.
- **Vendored SDKs:** any official/unofficial messenger SDK or API client we depend on goes in as a **git submodule**, e.g. `commons_messaging/sdk/telegram` or `commons_messaging/api/telegram` — not `go get`'d into `go.mod`.

## `constitutable/` directory (parent-project extension hook)

The empty `constitutable/` directory at the repo root is intentional. Per the spec, a parent project may drop additional `Constitution.md` / `CLAUDE.md` / `AGENTS.md` (in `constitutable/`, `constitutable/<flavor>/`, `constitutable/<flavor>/<variant>/`, etc.) to layer extensions or overrides on top of the discovered `constitution/` submodule. Apply-order: `constitution/` submodule → `constitutable/` extensions → Herald's own docs. Do not delete the directory because it's empty.

## Documentation artefacts (PDF/HTML siblings)

`docs/guides/HERALD_CONSTITUTION.md` and `docs/guides/CONSTITUTION_INHERITANCE.md` each ship with a committed `.pdf` sibling. When you edit one of these Markdown files, the PDF goes stale — flag it; do not regenerate silently unless the operator asks.

The HRD-lifecycle docs in `docs/` also ship as PDF/HTML/DOCX quadruples: `Issues.md` (open HRDs per V3 §8.3), `Fixed.md` (closed-HRD log per §11.4.19 atomic migration), `Status.md` (status summary), `CONTINUATION.md` (live-test handoff prompt for operator-supplied credentials). The `*_Summary.md` variants are derived; do not hand-edit.

**Logo branding (added 2026-05-21).** Every tracked Markdown doc now leads with a centered Herald logo header (pandoc-friendly `<div align="center">` wrapping a `<img src="..." alt="Herald" width="96" height="96" />` image reference to `assets/logo/herald_logo_square_128.png`). The export pipeline propagates that logo into the HTML, PDF, and DOCX siblings:

- Logo source: `assets/logo/herald_logo.png` (1664x928 RGB master). Square + transparent variants live under `assets/logo/herald_logo_square_{32,64,128,256,512,1024}.png` (chroma-keyed white → alpha). `assets/logo/herald_logo.svg` is a vector wrapper around the 512px PNG. `assets/logo/print.css` carries print/screen styling for the HTML/PDF route.
- Injection: idempotent re-runnable via `python3 scripts/branding_inject_logo.py <md ...>` — skips submodules/, containers/, constitutable/, docs/diary/, LICENSE; respects YAML front-matter; computes the relative path per doc depth.
- Export: `bash scripts/export_docs.sh [<md>...]` regenerates HTML (pandoc), PDF (`--pdf-engine=weasyprint`), DOCX (pandoc-native) for every `.md` that already has at least one sibling artefact committed. Pass no args to regenerate everything.
- When you add a new `.md`, run the injector once; when you edit one whose sibling exports are tracked, run the exporter for that one file.

## Notes for future scaffolding

- `submodules/` holds 9 vendored Helix-stack modules (each its own `git@github.com:vasic-digital/<name>.git` repo): `auth`, `background`, `cache`, `config`, `database`, `eventbus`, `middleware`, `observability`, `recovery`. They are referenced via `replace` directives in the consuming Herald modules' `go.mod`, NOT via `go.work` (which lists the **18** Herald-owned modules — 10 shared/foundation incl. `commons_watch` + `commons_workable` + 8 flavor binaries). Do not add the Helix-stack submodules to `go.work`.
- The repo is in `main` branch and committed under "Milos Vasic" — no other contributors yet.
- `.claude/` exists but is empty; project-local Claude config can go there.
- `LICENSE` is present (do not overwrite without asking).
- `.DS_Store` is now git-ignored; do not re-add the previously-stray files.
