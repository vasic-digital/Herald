<div align="center">

<img src="../../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# v1.0.0 Issues→Fixed Migration Audit — Verified Manifest

| Field | Value |
|---|---|
| Audit date | 2026-06-02 |
| Auditor | read-only audit subagent (forensic, anti-bluff §107 / §11.4) |
| Scope | every open/in_progress HRD in `docs/Issues.md` r48 |
| Mode | READ-ONLY — this manifest is the ONLY file written; nothing migrated |

## Headline finding (anti-bluff correction)

The prior-audit premise — "~52 HRDs are code-complete + evidence-backed but stuck
in `open`/`in_progress` because the atomic migration was never run" — **does NOT
match committed reality.** That bulk migration **already happened**: `docs/Issues.md`
revision **r43** atomically migrated **51 code-verified HRDs** (HRD-018..056,
HRD-085..089, HRD-090, HRD-110..154 clusters) Issues→Fixed, and r44/r47 closed the
remainder. The e2e invariants the prior audit cited (E90-E96 etc.) are present and
their HRDs are **already in `Fixed.md`** (see `docs/Issues.md` line 18 "Fixed" field).

`docs/Issues.md` today lists only **10** open/in_progress HRDs — NOT ~52. Of those,
**exactly 1** is genuinely READY-TO-MIGRATE (HRD-115); the other 9 are correctly
left open (1 active bug, 8 externally/operator-gated). Migrating any of the 9 would
be a §107 PASS-bluff.

## Summary table

| Classification | Count | HRD ids |
|---|---|---|
| READY-TO-MIGRATE | 1 | HRD-115 |
| NOT-READY | 9 | HRD-008, HRD-081, HRD-131, HRD-150, HRD-155, HRD-156, HRD-157, HRD-158, HRD-159 |
| NEEDS-LIVE-EVIDENCE | 0 | — |
| **Total open** | **10** | |

## Ordered list — what the conductor can SAFELY migrate now

1. **HRD-115** — Slack channel adapter (Socket Mode). Migrate Issues→Fixed. All
   four closure layers met with LIVE captured evidence (see citation below). The
   Issues.md row itself already states: *"Ready for Issues→Fixed at the next
   §11.4.19 sweep."*

That is the **only** safe migration. Do NOT migrate any other open HRD.

---

## Per-HRD verdicts (with forensic citations)

### HRD-115 — Wave 7 T6 Slack channel adapter (Socket Mode) — **READY-TO-MIGRATE**

- **Code present + builds.** `commons_messaging/channels/slack/` — slack.go,
  send.go, subscribe.go, healthcheck.go, selfidentity.go, attachments.go,
  mentions.go (§109 `<@U…>` tagging), thread_context.go. `go build
  ./commons_messaging/channels/slack/... ./pherald/...` → BUILD OK.
- **Tests present.** 22 hermetic httptest tests (`slack_test.go`, `send_test.go`,
  `subscribe_test.go`, `selfidentity_test.go`, `attachments_test.go`,
  `mentions_test.go`, `thread_context_test.go`, `stress_chaos_test.go`,
  `send_integration_test.go`). `var _ channels.Channel = (*slack.Adapter)(nil)`
  pinned via `TestSlackSatisfiesChannel`.
- **e2e invariants present.** `scripts/e2e_bluff_hunt.sh` E124–E131 (line 2371+);
  E127 is the triple-guarded LIVE Slack invariant; `TestSlack_Live_Send` exists at
  `qaherald/internal/messenger/slack_live_integration_test.go:26` (E127 anchor).
- **LIVE captured evidence (the §107 anti-bluff anchor — this is the decisive
  proof, not "tests pass"):**
  - `docs/qa/HRD-115-LIVE-20260602T074249Z/` — live outbound Send (real `ts`),
    Socket-Mode app-token validity, `pherald listen --channels slack` real boot
    (`listen-boot.txt`, `live-transcript.txt`).
  - `docs/qa/HRD-115-LIVE-roundtrip-2026-06-02T10-39-43Z/TRANSCRIPT.md` — fully
    self-driving round-trip: Slack user posts → bot receives via Socket Mode →
    autonomy chain (inbound→cc.dispatch→cc.reply) proven via `transcript.jsonl`
    AND reply-DELIVERY proven (Tier-1 deterministic reply `Looking up the status
    of ATM-1881…` landed back in Slack, ts>probe, threaded under the query ts),
    plus Leg 3 in-thread processing and Leg 4 thread-context awareness — all
    live-captured. `TestSlack_LiveRoundTrip` at
    `qaherald/internal/lifecycle/slack_roundtrip_test.go:76`.
- **Commits.** `1f2b60c` (live-parity wiring + live send), `f5c9aa5` (round-trip
  reply-DELIVERY LIVE PASS), `28af644` (in-thread), `bc246a2`/`14fed75`
  (thread-context, both channels live-proven).
- **Residual is explicitly NOT a Slack defect.** The Slack adapter is at full live
  parity with Telegram (send + inbound + reply-delivery all live-proven). The only
  remaining gap — empty CC/Tier-2 reply text from a fresh bootstrap session — is a
  CROSS-CHANNEL issue (Telegram has it too) carved out into **HRD-159**. Closing
  HRD-115 is therefore correct and not a bluff.

> **Conductor action:** migrate HRD-115 Issues→Fixed atomically per §11.4.19,
> preserving its full References/LIVE-PROGRESS cell. Flip its status to Fixed and
> update the Issues.md `Issues` field (10→9) + the `Fixed` field (+HRD-115).

---

### HRD-159 — §11.4.98 CC reply-DELIVERY evidence gap (cross-channel) — **NOT-READY**

- **What is present:** the robustness guard only — `pherald/internal/inbound/
  dispatcher.go` `actReply` treats an empty reply as a logged no-op (does NOT crash
  the listener); `TestDispatcherEmptyReplyDoesNotCrash`
  (`dispatcher_test.go:301`). Commit `43abe9b`.
- **What is MISSING (why it cannot close):** the actual feature — proving the
  Tier-2 CC reply text is delivered back to the messenger from a context-aware
  session — is unimplemented. A fresh `pherald listen` bootstrap session returns
  EMPTY reply text → nothing posted back. None of the three documented fix options
  (a: seed bootstrap session with repo/reply context; b: dedicated long-lived
  context-rich QA session; c: assert delivery against a deterministic fast-path)
  has landed. The slack round-trip test SKIPs-with-reason on this leg
  (`slack_roundtrip_test.go`); the tgram loop masks it as a soft BONUS
  (`mtproto_wave6_loop_test.go:255`).
- This is an open, high-criticality **bug** opened 2026-06-02. Correctly OPEN.

### HRD-008 — Operator-side quickstart compose validation — **NOT-READY**

- Code/scaffold present: `quickstart/docker-compose.quickstart.yml`,
  `Dockerfile.pherald`, `otel-config.yaml`. But the row's closure criterion is a
  **live end-to-end operator run** (Postgres + Redis + OTel + pherald container)
  which is operator-gated and has no `docs/qa/<run-id>/` live transcript. Closing
  on scaffold-presence alone is a §107 PASS-bluff. Correctly in_progress.

### HRD-081 — containers compose podman/docker runtime detection — **NOT-READY**

- Only a **workaround** exists in `commons_infra/boot.go` (lines 148, 208 — drops
  `WithWait`, TCP-probes for healthcheck). The actual fix must land UPSTREAM in
  `vasic-digital/containers` (`pkg/compose`) per §11.4.76 "extend upstream
  submodule, never reimplement." Upstream change not done. Externally gated.

### HRD-131 — text trackers → versioned SQLite SSoT (6-phase) — **NOT-READY**

- Phase 2 EXTERNALLY BLOCKED: `constitution/scripts/workable-items/` is a
  non-functional scaffold (0/7 subcommands). `git ls-files '*.db'` is EMPTY and no
  `docs/.workable_items.db` / `docs/workable_items.db` exists on disk — confirming
  CLAUDE.md §11.4.95's own "DB artefact is NOT YET present" statement. Cannot
  proceed without reimplementing the binary (§11.4.74 forbids). Correctly open.

### HRD-150 — ATMOSphere WS-1 MD↔SQLite regenerator + drift resolution — **NOT-READY**

- Depends on the same blocked constitution workable-items tool (must supply the
  ATMOSphere-format parser the tool lacks). External blocker. Correctly open.

### HRD-155 — ATMOSphere WS-1 operationalize SSoT tool + reconcile HRD-131 — **NOT-READY**

- Requires implementing the tool's `add`/`close`/`report` + ATMOSphere parser
  UPSTREAM in the constitution repo (cross-repo, §11.4.74). External blocker for
  SSoT materialization. Correctly open.

### HRD-156 — ATMOSphere WS-5 anti-bluff full-automation suite — **NOT-READY**

- Partial live evidence exists (`docs/qa/HRD-156-LIVE-20260530T132303Z/
  atmosphere_outbound_live.log`) but the full T-A..T-F matrix (MTProto live, exact
  diff byte-assert, stress/chaos, paired §1.1 gate, HelixQA Challenge wrap) is not
  complete and is gated on one-time MTProto credential bootstrap + ATMOSphere
  cross-repo deployment. Correctly open.

### HRD-157 — ATMOSphere Phase 3 register tools/herald submodule + host runner — **NOT-READY**

- Ops/deployment on the ATMOSphere host (register submodule, materialize
  `docs/workable_items.db`, wire host-side `pherald watch`/`listen`, live
  T-A..T-F). Cross-repo + operator/host-gated. Correctly open.

### HRD-158 — ATMOSphere WS-5 covenant verbatim-phrase propagation — **NOT-READY**

- Cross-repo doc cascade to root `constitution/QWEN.md` + ATMOSphere root + ~25
  submodules (high blast radius, §11.4.26 fetch-first governance, operator
  confirmation required). Not a Herald-local code closure. Correctly open.

---

## Methodology / anti-bluff notes

- Every READY verdict is backed by: (a) impl files that exist + `go build` green,
  (b) named tests in-tree, (c) e2e invariant ids in `scripts/e2e_bluff_hunt.sh`,
  AND (d) a live `docs/qa/<run-id>/` captured transcript. A verdict resting on
  fewer than all four was downgraded.
- No HRD was classified READY on self-reported Issues.md status alone.
- The conductor must still run the §11.4.19 atomic-migration mechanics (move the
  row, update the `Issues`/`Fixed` summary fields, regenerate `*_Summary.md`
  siblings) — this manifest only authorizes WHICH item is safe to move.
