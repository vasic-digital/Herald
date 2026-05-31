<div align="center">

<img src="../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Herald — Open-Work Audit (2026-05-31)

| Field | Value |
|---|---|
| Created | 2026-05-31 |
| HEAD at audit | `9852d5b` (feat(docs_chain): roll Docs Chain out to the FULL documentation corpus) |
| Scope | (1) open-HRD reconciliation, (2) project-status staleness, (3) broken links / stale refs, (4) §11.4.98 full-automation test compliance |
| Method | READ-ONLY: grep/read impl + tests + `docs/qa/` + `git log`. Every claim file:line / command-output cited (§11.4.6 no-guessing). |
| Status | informational audit (not a tracker doc) |

---

## TOP ACTIONABLE — closeable NOW, autonomously (ranked by value)

| # | Item | Why closeable now | Evidence |
|---|---|---|---|
| 1 | **Project-status staleness in README.md + AGENTS.md + HERALD_CONSTITUTION §101** | Pure doc-truth fix; current reality is fully committed + verifiable. README/AGENTS still say "**pre-implementation**", "no `go.mod`, no Go source code yet". Reality: 18 modules, 8 flavor binaries, v0.6.0 tagged, 70+ Fixed HRDs. | `README.md:48,57`; `AGENTS.md:64,77`; `docs/guides/HERALD_CONSTITUTION.md:25,48,50`; `go.work` = 18 modules; `git tag` = v0.1.0..v0.6.0 |
| 2 | **Stale `specification.V3.md` active-edit refs in `docs/research/protocols/{A2A,MCP}.md`** | V4 is active; V3 archived under `docs/specs/mvp/archive/`. These two docs instruct future writers to "Add to `docs/specs/mvp/specification.V3.md`" — a moved path. Fix = repoint to V4 (or annotate as archived/historical). | `docs/research/protocols/A2A.md:24,342`; `docs/research/protocols/MCP.md:24,314` |
| 3 | **Gate `TestVerticalSlice_TelegramClaudeRoundTrip` behind an opt-in skip (§11.4.98 NON-COMPLIANT, MISSED by 534e785)** | Genuine remaining manual-action-during-execution test; the 2026-05-30 retirement commit `534e785` gated the *other three* but missed this one. One-line opt-in gate (mirror `HERALD_W6_LIVE_LOOP=1` pattern) + Obsolete-candidate header. | `commons_messaging/vertical_slice_integration_test.go:19,66,270` ("operator did not hand-send a message within the 150s window") |
| 4 | **Confirm HRD-143/144/145/015 correctly Obsolete (closure rubber-stamp)** | All four already carry full §11.4.90 `Obsolete-Details` lines with triple-check evidence; another stream is migrating them to Fixed. AUDIT CONFIRMS they are correctly classified. | `docs/Issues.md:35-38` |
| 5 | **HRD-156 partial-evidence note** | T-A (outbound) is LIVE-PROVEN (real MTProto read-back, 8.18s PASS). T-B..T-F remain. Not closeable, but the Issues.md framing ("externally gated, no evidence") understates real progress. | `docs/qa/HRD-156-LIVE-20260530T132303Z/atmosphere_outbound_live.log:8` (bot message_id=211575 read back via MTProto) |

**Executive counts:** stale project-status claims = **9** (see §2); broken local links = **1** (frozen QA artefact — non-actionable); stale-V3-active-ref doc hits = **4** (A2A ×2 + MCP ×2); remaining genuine §11.4.98 NON-COMPLIANT test = **1** (`TestVerticalSlice_…`).

---

## (1) OPEN-HRD RECONCILIATION

`docs/Issues.md:16` lists 16 IDs as open/in-progress/Obsolete-pending. Classification per item:

| HRD | Issues.md status | TRUE status (this audit) | Evidence / blocker |
|---|---|---|---|
| **HRD-008** | in_progress | **(c) evidence-gated — operator deploy** | Quickstart compose validation needs a live end-to-end run on operator hardware. `docs/Issues.md:53`. Scaffold shipped (`quickstart/`); only the operator can validate the live container boot. Not autonomously closeable. |
| **HRD-015** | Obsolete | **(a) correctly Obsolete — confirm + migrate** | Full `Obsolete-Details` (Since 2026-05-30, superseded-by-design-change; I8 slot repurposed for §107 I8a/b/c). `docs/Issues.md:38`; `tests/test_constitution_inheritance.sh` I8a/b/c. Closed via `6421f45`. CONFIRMED. |
| **HRD-081** | open | **(b/c) genuinely-open — upstream extension** | Extend `vasic-digital/containers` compose runtime detection (podman vs docker `--wait`). `docs/Issues.md:39`. §11.4.76 forbids reimplementing locally; a workaround already lives in `commons_infra/boot.go` (TCP-probe). Autonomously completable ONLY by editing the `containers/` submodule upstream — cross-repo. |
| **HRD-115** | in_progress | **(c) evidence-gated — LIVE Slack creds** | Slack adapter code COMPLETE + hermetic-tested (`commons_messaging/channels/slack/`, 22 tests, `docs/qa/HRD-115-20260528T080000Z/`). Closure operator-gated on LIVE round-trip `docs/qa/HRD-115-LIVE-*/`; E127 LIVE-Slack is SKIP-with-reason (`scripts/e2e_bluff_hunt.sh` E127). Needs `HERALD_SLACK_BOT_TOKEN`+`HERALD_SLACK_CHANNEL_ID`. Closing without live evidence = §107 PASS-bluff. |
| **HRD-117** | in_progress | **(a) ACTUALLY-DONE — spec doc-only, closeable** | Wave 7 spec edits already landed (V3→...→V4; the §11.0/§32.2/§43 multi-channel content is in the active `specification.V4.md`). `docs/Issues.md:41`. Doc-only, no live-evidence gate. This is the lowest-risk autonomous close (verify V4 carries the §11.0 inbound-runtime note + §32.2 Slack-LIVE + §43 multi-channel `pherald listen` row, then migrate Issues→Fixed). |
| **HRD-131** | open | **(c) externally blocked** | SQLite SSoT migration Phase 2 blocked: `constitution/scripts/workable-items/` is a non-functional scaffold (0/7 subcommands; assessment `docs/research/workable-items-phase2-assessment-2026-05-27.md`). §11.4.74 forbids reimplementing. `docs/Issues.md:42`. Not autonomously closeable in Herald. |
| **HRD-143** | Obsolete | **(a) correctly Obsolete — confirm + migrate** | Full triple-check `Obsolete-Details`; supersedes `TestSubscribe_LiveBotAPI` via HRD-140; legacy test still exists + gated (`commons_messaging/channels/tgram/subscribe_integration_test.go:31-41`). `docs/Issues.md:35`. CONFIRMED. |
| **HRD-144** | Obsolete | **(a) correctly Obsolete — confirm + migrate** | Supersedes `tests/test_wave6_live_loop.sh` via HRD-141; legacy script gated behind `HERALD_W6_LIVE_LOOP=1` (`tests/test_wave6_live_loop.sh:7-14`, commit `534e785`). `docs/Issues.md:36`. CONFIRMED. |
| **HRD-145** | Obsolete | **(a) correctly Obsolete — confirm + migrate** | Supersedes `qaherald lifecycle --manual` via HRD-142; `--manual` flag retained but gated (`qaherald/cmd/qaherald/lifecycle.go:62,110,174`). `docs/Issues.md:37`. CONFIRMED. |
| **HRD-150** | open | **(c) externally blocked — constitution upstream** | ATMOSphere WS-1 MD↔SQLite regenerator; needs the constitution workable-items tool operationalized upstream (same blocker as HRD-131/155). `docs/Issues.md:43`. Cross-repo. |
| **HRD-155** | open | **(c) externally blocked — constitution upstream** | Implement the tool's `add`/`close`/`report` + ATMOSphere parser IN the constitution repo. `docs/Issues.md:44`. Issues.md r42 narrative (`docs/Issues.md:15`) claims a parser-extension commit `995bd2f` was pushed to 4 upstreams — but the HRD row is still `open`; status-vs-narrative drift. Cross-repo, not autonomously closeable in Herald. |
| **HRD-156** | open | **(c) partially LIVE-proven, rest gated** | **T-A outbound is LIVE-PASS** (`docs/qa/HRD-156-LIVE-20260530T132303Z/`: bot message_id=211575 read back via real MTProto, 8.18s). T-B inbound / T-C byte-diff / T-D/E stress-chaos / T-F mutation-gate remain. E139 hermetic watch→notify PASSES (`scripts/e2e_bluff_hunt.sh` E139). Full closure needs the remaining layers + sustained live creds. |
| **HRD-157** | open | **(c) cross-repo deploy** | Register Herald as `tools/herald` submodule in ATMOSphere + host-daemon deploy. Recent commit `790341c` added the deploy-runbook DISCOVERABILITY link only (`docs/INTEGRATION.md:444`); `b917e30` added the deploy tooling (`deploy/atmosphere-herald/`). The HOST-SIDE registration + live run is ATMOSphere-repo + operator-host work — NOT done, NOT autonomously closeable here. Issues.md r42 narrative (`docs/Issues.md:15`) claims `tools/herald` was registered upstream (`3368645`) — again HRD row still `open` (status-vs-narrative drift). |
| **HRD-158** | open | **(b) partially autonomously-completable** | Anti-bluff covenant verbatim-phrase propagation to QWEN.md across constitution + ATMOSphere + 25 submodules + Herald README. The HelixConstitution + ATMOSphere edits are cross-repo (`995bd2f`/`8ec2b38`/`5462a38` per r42 narrative); the **Herald README leading-clause** portion IS autonomously doable in-repo. Mixed. |

**Net autonomously-closeable in-Herald NOW:** HRD-117 (spec doc-only, full close) is the cleanest. HRD-158 (Herald-README portion) is a partial. Everything else is creds-gated, operator-deploy-gated, or cross-repo-blocked.

**STATUS-VS-NARRATIVE DRIFT (flag for the tracker-fixing stream):** the `docs/Issues.md:15` r42 narrative describes HRD-155 and HRD-157 cross-repo commits as *landed + pushed*, yet both rows at `docs/Issues.md:44,46` remain `open`. Either the rows should advance to reflect the landed upstream work (with the in-Herald remainder re-scoped) or the narrative overstates completion. Re-reconcile against the actual ATMOSphere/constitution repos before closing.

---

## (2) PROJECT-STATUS STALENESS — enumerated stale claims

Current ground truth: **18 go.work modules** (`go.work`), **8 flavor binaries** (pherald/sherald/cherald/bherald/rherald/iherald/scherald/qaherald), **tags v0.1.0..v0.6.0** (`git tag`), Docs Chain integrated (`9852d5b`), 70+ Fixed HRDs (`docs/Issues.md:18`).

| # | File:line | Stale claim | Current truth |
|---|---|---|---|
| 1 | `README.md:48` | "Herald is **pre-implementation** (2026-05-15). The repository currently contains:" | First-implementation cycle long past; 18 modules build+test green; v0.6.0 tagged. |
| 2 | `README.md:57` | "There is no `go.mod`, no Go source code, and no build tooling yet... layout will be used when scaffolding starts." | go.mod present in all 18 modules; full Go source; `go build ./...` green; `scripts/e2e_bluff_hunt.sh` runs 148+ invariants. |
| 3 | `AGENTS.md:64` | "Herald is **pre-implementation**. As of 2026-05-15 the repo contains:" | Same as #1. |
| 4 | `AGENTS.md:77` | "**As of 2026-05-20** the Go scaffold landed (first-implementation cycle r1). **5 Go modules** (`commons`, `commons_prefix`, `commons_messaging`, `commons_storage`, `pherald`)..." | 18 modules (10 shared/foundation + 8 flavor binaries), not 5; waves 2–8 shipped. |
| 5 | `docs/guides/HERALD_CONSTITUTION.md:25` (ToC) + `:48` (heading) | "§101. **Pre-implementation status**" | Implementation well underway; the section title itself is stale. |
| 6 | `docs/guides/HERALD_CONSTITUTION.md:50` | "Herald is pre-implementation. Until a `go.mod` is committed, no clause... may... fabricate build/test infrastructure that doesn't yet exist. Confirm... scaffold vs. fill spec... before writing code" | go.mod committed across 18 modules; the disambiguation precondition is obsolete. |
| 7 | `docs/Status.md:32` | "**Implementation-r1, Foundation complete + Wave 2 flavor scaffolds landed.** Spec V3 r8 is active... remaining cycle is wiring live channel integrations (HRD-011, HRD-016, HRD-024, HRD-028, HRD-098)" | Spec is now V4 (V3 archived); HRD-011/016/024/028/098 are all Fixed (`docs/Issues.md:18`); waves 2–8 shipped. |
| 8 | `docs/Status.md:65-69` | "inheritance gate 15 PASS"; "**E2E bluff-hunt... 41 PASS / 0 FAIL / 5 SKIP**"; "**Submodules: 12 vendored**" | e2e is now ~148 invariants (`docs/Status.md:15` r21 narrative itself says "148 PASS / 1 FAIL"); 17 vendored submodules (`docs/Status.md:15`). The §32 status-table block at `:52-69` is a frozen Wave-2/3a snapshot. |
| 9 | `docs/Status.md:59` | "`iherald`... `/v1/webhooks/page` returns honest 501 + HRD-024." | iherald `/v1/webhooks/page` is now LIVE (commit `1550c04`/`246c6e8`; HRD-024 Fixed). The 501-stub framing is stale. |

Note: `README.md:260` mislabels the active spec link as "Herald spec V3 (source of truth)" while the URL correctly points to `specification.V4.md` — a stale *label* (V3→V4). `QWEN.md` carries no `pre-implementation`/`2026-05-15` status claim (grep returned nothing) — QWEN is clean on this axis. `CLAUDE.md` already says "first-implementation cycle (r1) as of 2026-05-20" + "18 modules" in places but still has the §"Project status" `As of 2026-05-20 the Go scaffold has landed` framing — being fixed by the other stream per the brief.

---

## (3) BROKEN INTERNAL LINKS / STALE REFS

**Broken local markdown links (tracked .md, excl submodules/containers/constitutable/superpowers/diary/spec-archive):** scanned every `]( … )` relative link, resolved against each file's directory.

- **1 hit, NON-ACTIONABLE:** `docs/qa/HRD-101-lifecycle-2026-05-23T03-16-17-w6.5live/issues-before.md:94` → `Fixed.md`. This file is a *frozen QA-evidence snapshot* of a past `Issues.md` (captured under `docs/qa/`); the relative `Fixed.md` resolves into the qa dir where no `Fixed.md` exists. Correct behaviour — evidence artefacts are immutable; do NOT "fix" it.

**Stale `specification.V3.md`-as-active references** (V4 active, V3 at `docs/specs/mvp/archive/`):

| File:line | Text | Verdict |
|---|---|---|
| `docs/research/protocols/A2A.md:24` | "Wave 4b will add a §4x.x to specification.V3.md" | STALE — should reference V4 (or mark historical). |
| `docs/research/protocols/A2A.md:342` | "Add to `docs/specs/mvp/specification.V3.md`:" | STALE — unqualified V3 path no longer exists; moved to archive/. |
| `docs/research/protocols/MCP.md:24` | "Wave 4a will add a §4x.x to specification.V3.md" | STALE — same. |
| `docs/research/protocols/MCP.md:314` | "Add to `docs/specs/mvp/specification.V3.md`:" | STALE — same. |
| `AGENTS.md:15` | revision-note mentions the V3→V4 path-sync; `:Status` body itself fine | the actual AGENTS spec-pointers were synced in r15; this is just historical narrative. NOT stale. |
| `docs/Status_Summary.md:37` | lists V3 under `archived_specs[]` with the archive/ path | CORRECT (V3 is archived). NOT stale. |
| `docs/catalogue-checks/HRD-092-commons-cli.md:79` | points at `archive/specification.V3.md` explicitly | CORRECT. NOT stale. |
| `docs/specs/mvp/specification.V4.md` (×7) | supersession-chain + changelog mentions, all qualified `archive/specification.V3.md` | CORRECT (deliberate evolution-chain refs). NOT stale. |

Net actionable: **4 stale V3-as-active hits** (A2A ×2, MCP ×2) + the README:260 V3-label noted in §2.

---

## (4) §11.4.98 FULL-AUTOMATION TEST COMPLIANCE

**Constitution-flagged trio — REMEDIATED:**

| Legacy NON-COMPLIANT | State | MTProto replacement | Replacement evidence |
|---|---|---|---|
| `TestSubscribe_LiveBotAPI` | Gated: skips unless attended; Obsolete HRD-143. `commons_messaging/channels/tgram/subscribe_integration_test.go:31-51` | `TestMTProto_Subscribe_AutonomousRoundTrip` (E135) | `docs/qa/HRD-LIVE-MTPROTO-20260528T125321Z/E135_subscribe/`; `scripts/e2e_bluff_hunt.sh:2441` |
| `tests/test_wave6_live_loop.sh` | Gated behind `HERALD_W6_LIVE_LOOP=1` opt-in; Obsolete HRD-144. `tests/test_wave6_live_loop.sh:7-14,40-45` (commit `534e785`) | `TestMTProto_Wave6_AutonomousClosedLoop` (E136) | `docs/qa/HRD-LIVE-MTPROTO-20260528T125321Z/E136_wave6_closed_loop/`; `scripts/e2e_bluff_hunt.sh:2444` |
| `qaherald lifecycle --manual` | Manual mode gated behind `--manual`/`HERALD_W65_MANUAL=1` opt-in; Obsolete HRD-145. `tests/test_wave6.5_lifecycle.sh:46-61`; `qaherald/cmd/qaherald/lifecycle.go:110` | `TestMTProto_Wave65_LifecycleAutonomous` (E137) | `docs/qa/HRD-LIVE-MTPROTO-20260528T125321Z/E137_wave65_lifecycle/`; `scripts/e2e_bluff_hunt.sh:2447` |

All three replacements are real (files: `qaherald/internal/lifecycle/mtproto_subscribe_test.go`, `mtproto_wave6_loop_test.go`, `mtproto_wave65_lifecycle_test.go`), build-tag-gated (`-tags=integration_mtproto`, `go vet` exit 0), and PASSED LIVE per the `HRD-LIVE-MTPROTO-20260528T125321Z` evidence dir. The legacy ones are retired-not-deleted (retained as documentation artefacts per their Obsolete-Details). **Trio: COMPLIANT.**

**REMAINING NON-COMPLIANT (1) — newly identified, MISSED by 534e785:**

- **`commons_messaging/vertical_slice_integration_test.go` → `TestVerticalSlice_TelegramClaudeRoundTrip`** — requires the operator to **hand-send a Telegram message within a 150s window during test execution** (`:19` "The operator MUST hand-send a Telegram message"; `:66` "operator hand-sends a Telegram message"; `:270` `t.Fatal("VS: handler never invoked — operator did not hand-send a message within the 150s window?")`). It honest-SKIPs on missing creds/env (`:73-99`) but, unlike the constitution-flagged trio, it has **NO opt-in retirement gate** — if `HERALD_TGRAM_LIVE_INBOUND=1` plus the other env are set, it demands runtime human action and would FAIL unattended. This is a §11.4.98 violation (manual-action-during-execution) that the `534e785` "gate/retire 4 NON-COMPLIANT" pass did not cover. **Remediation:** add the same opt-in gate (`HERALD_VS_LIVE=1`-style) + §11.4.90 Obsolete-candidate header pointing at `TestMTProto_Wave6_AutonomousClosedLoop` (E136), which is its autonomous superset (Telegram→CC→reply round-trip). One-line, autonomously doable.

No other tracked `*_test.go` / `tests/*.sh` requires manual action during execution (grep for `hand-send`/`type a message`/`within the 60s`/`attended` returned only the four files above, three of which are gated).
