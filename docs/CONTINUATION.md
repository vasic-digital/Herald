<div align="center">

<img src="../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Herald — Continuation

| Field | Value |
|---|---|
| Revision | 8 |
| Created | 2026-05-20 |
| Last modified | 2026-05-23 |
| Status | active |
| Status summary | **2026-05-23 session-end checkpoint — safe to shut down. Wave 6.5 lifecycle layer landed (12/12 implementation tasks + premature v0.5.0 tag from a parallel Haiku session). One root-cause bug discovered live + fixed: envelope ACTION FORMAT GUIDANCE (commit `53a7ad3`) — before the fix, every Bug:/Task: defaulted to `action:"reply"` and never wrote Issues.md (§107 PASS-bluff at the prompt layer). qaherald-auto Go framework T1+T2+T3+T4+T5 of 8 landed (full 15-scenario lifecycle test framework using a 2nd Telegram bot `@pherald_qa_bot` as automated subscriber). T6+T7+T8 remaining: pre-flight validator + httptest-driven unit tests + tests/test_wave6.5_lifecycle.sh adapter. After T8 + live run + v0.5.1 tag: ready to start Wave 7 (genericize messenger framework Slack/Email per operator mandate) / close 47 remaining HRDs.** v0.4.0 (Wave 6 closed-loop) + v0.5.0 (Wave 6.5 implementations) both tagged + pushed to all 4 mirrors. Live evidence so far under `docs/qa/HRD-100-2026-05-22T18-27-30-w6live/` (4-event closed loop) + `docs/qa/HRD-101-lifecycle-2026-05-22T20-04-04-w6.5live/` (partial — only S1+S2 evidence; superseded by qaherald-auto). Workspace at 16 modules. Spec V3 at r12 + §32.7 + §32.9 + §33.6 + §33.7 captured. Constitutional propagation of §107.x docs/qa-evidence + §107.y working-tree-quiescence covenant complete across 13 submodules + parent constitution + Herald root (QA1 commit `bff18db` + per-submodule `a8c1cb4..b464136`). Anti-bluff gates green: inheritance 15/15 PASS, audit_antibluff 18/0/1, Wave 6 mutation gate 4/4 PASS, Wave 6.5 mutation gate 6/6 PASS. Pre-existing carry-over: Postgres-SASL-dependent e2e invariants (E7-E12, E14-E16, E37-E42 — 15 FAILs against the running PG container, root cause is local-PG SASL handshake; unrelated to Wave 6/6.5). |
| Issues | HRD-008, HRD-015, HRD-018 (in_progress), HRD-019..HRD-027, HRD-029..HRD-056, HRD-081, HRD-085..HRD-090, HRD-101 (in_progress — qaherald-auto + full lifecycle evidence) |
| Issues summary | see `Issues.md` — 48 open items (HRD-101 in flight) |
| Fixed | HRD-001..HRD-007, HRD-009, HRD-009b, HRD-010, HRD-011, HRD-012, HRD-013, HRD-014, HRD-016, HRD-017, HRD-028, HRD-080, HRD-092..HRD-100 (HRD-018 partial — M1 + M2 components landed) |
| Fixed summary | see `Fixed.md` |
| Continuation | **Resume tomorrow at qaherald-auto T6+T7+T8** (`docs/superpowers/plans/2026-05-23-qaherald-auto.md` commit `52ce679`). T1-T5 landed: `qaherald lifecycle` Cobra subcommand + MessengerClient interface (Telegram impl) + 15 scenarios (S1..S15) + orchestrator + Markdown report — 12 hermetic unit tests PASS under `-race -count=1`. T6: 10-gate pre-flight validator (pherald-bot getMe, qa-bot privacy disabled, group membership, OPERATOR_IDS contains qa-bot, etc.). T7: full unit-test pass against `httptest` impersonating Bot API + canned pherald transcripts. T8: rewrite `tests/test_wave6.5_lifecycle.sh` to delegate to `qaherald lifecycle` (keep `--manual` for old interactive UX). After T8: live e2e run with operator-supplied creds (see §4a below), commit `docs/qa/HRD-101-lifecycle-<run-id>/` + tag `v0.5.1` + 4-mirror push. Subsequent: Wave 7 (genericize messenger framework — Slack/Email next per operator 2026-05-22), then close-out of HRDs 018..027 + 029..056 + 081 + 085..090 for v1.0.0 production-ready, then the docs-audit task #147 (operator manuals + PDF/HTML/DOCX exports for every wave). Operator restated 2026-05-22/23: NO BLUFF, NO FALSE POSITIVES, fix root causes immediately. The 53a7ad3 envelope fix is the canonical example of that rule operating live. Push to all 4 mirrors + every submodule's own origin on every non-trivial commit per Universal §12.10. |

## Table of contents

- [§0. How to use this document](#0-how-to-use-this-document)
- [§1. Snapshot](#1-snapshot)
- [§2. Last commit landed](#2-last-commit-landed)
- [§3. Active work](#3-active-work)
- [§4. Next concrete steps](#4-next-concrete-steps)
- [§5. Long-form pointers](#5-long-form-pointers)

## §0. How to use this document

Paste the following block into any CLI agent (Claude Code / OpenCode / Cursor / Aider / Gemini CLI) to resume Herald work exactly where it was left:

> You are working on the Herald project at `~/Projects/Herald` (also reachable as the `Herald/` submodule of a consuming project). The Helix Universal Constitution lives at `<ancestor>/constitution/` (parent-walk discovery). Read in this order: `CLAUDE.md`, `AGENTS.md`, `README.md`, `docs/guides/HERALD_CONSTITUTION.md`, `docs/guides/CONSTITUTION_INHERITANCE.md`, `docs/specs/mvp/specification.V3.md`. Then read `docs/CONTINUATION.md` (this file) for live state, `docs/Issues.md` for open work, `docs/Status.md` for current phase, `docs/Fixed.md` for closed history. Go workspace builds via `go test ./commons/... ./commons_prefix/... ./commons_messaging/... ./commons_storage/...`. Inheritance gate `tests/test_constitution_inheritance.sh` MUST exit 0 before any commit. Multi-mirror fan-out push to four hosts (GitHub + GitLab + GitFlic + GitVerse) is mandatory per Constitution §103.

## §1. Snapshot

- **Active spec:** `docs/specs/mvp/specification.V3.md` Revision 4 (~4300 lines).
- **Archived specs:** V1 + V2 in `docs/specs/mvp/archive/` (frozen).
- **Go modules:** `commons`, `commons_prefix`, `commons_messaging`, `commons_storage`, `pherald` (all compile + unit tests pass).
- **Container scaffold:** `quickstart/{docker-compose.quickstart.yml, Dockerfile.pherald, otel-config.yaml, .env.example}` for §26.5 quickstart.
- **CLI:** `pherald version --json` returns canonical build info; `serve`/`send`/`doctor`/`migrate`/`subscriber`/`deadletter` stubbed with HRD-NNN pointers.
- **Inheritance gate:** 12 PASS / 0 FAIL. Meta-test ✓.
- **Phase:** implementation r1.

## §2. Last commit landed

This commit (V3 r4 + Go scaffold + tracking-doc refresh) closes HRD-009/HRD-009b/HRD-013/HRD-014 (with a Universal §11.4.19 atomic Issues.md → Fixed.md migration) and lands spec V3 §37–§41 (tracker events, workable-item announcement contract, message presentation + Herald Canonical Template, docs/tests completeness, REST API surface). Builds + tests:

```
$ go test ./commons/... ./commons_prefix/... ./commons_messaging/... ./commons_storage/...
ok  	github.com/vasic-digital/herald/commons	1.135s
ok  	github.com/vasic-digital/herald/commons_prefix	0.639s
ok  	github.com/vasic-digital/herald/commons_messaging/channels/null	0.890s
ok  	github.com/vasic-digital/herald/commons_messaging/dispatch/claude_code	1.381s
ok  	github.com/vasic-digital/herald/commons_storage	1.630s
```

## §3. Active work

| ID | Status | What |
|---|---|---|
| HRD-008 | in_progress | Operator-side Quickstart compose validation (Postgres + Redis + OTel + pherald container) — scaffold shipped this commit; live end-to-end run pending operator. |
| HRD-010 | open | commons_storage live (pgx pool + golang-migrate driver + River queue + Redis ACL). |
| HRD-011 | open | Telegram adapter live (telebot SDK + getUpdates long-poll + webhook secret_token). |
| HRD-012 | open | Claude Code dispatcher live (`claude --resume` + parse `<<<HERALD-REPLY>>>`). |
| HRD-015 | open | Inheritance gate I8 invariants for Go scaffold (go.work + commons/types.go + null adapter test passes). |
| HRD-016 | open | REST API surface via Gin Gonic per spec §41. |
| HRD-017 | open | Propagate Universal §11.4.6X spec-versioning + submodule-catalogue-first mandates into the parent constitution. |

## §4. Next concrete steps

0. **HRD-018 Catalogue-Check survey + `commons_constitution` scaffold** — first action of the §42 implementation rollout. **MUST start** with a `vasic-digital` + `HelixDevelopment` catalogue survey per the brand-new Universal §11.4.74. Record `Catalogue-Check: reuse|extend|no-match` on HRD-018 before any Go code lands. Then scaffold the `Evaluator` interface, 12 event-class emit helpers, `constitution_state` + `constitution_bindings` migrations, bundle-hash captureer, mode-ladder runtime config.

1. **HRD-008 quickstart validation** — On a fresh laptop with Podman or Docker:
   ```
   git clone <Herald repo>
   git submodule update --init
   cd quickstart
   podman build -t herald/pherald:dev -f Dockerfile.pherald ../..
   cp .env.example ../../.env && $EDITOR ../../.env
   podman-compose -f docker-compose.quickstart.yml up -d
   curl --retry 30 --retry-delay 2 http://localhost:24090/readyz
   ```
   The current `pherald serve` returns "not implemented" — HRD-010/HRD-011/HRD-012 must land first to make the live `curl POST /v1/events` succeed end-to-end. Validation reveals which of the spec's assumptions (port ranges, Compose syntax, OTel collector config, Postgres healthcheck) hold against real infrastructure.
2. **HRD-010** — wire `commons_storage/storage.go`'s `MigrationDriver` to golang-migrate; bring pgx + River + Redis client up; add integration tests under `//go:build integration`.
3. **HRD-011** — replace `commons_messaging/channels/tgram/tgram.go` stub with a live implementation against `gopkg.in/telebot.v3` or `github.com/mymmrac/telego`; recorded HTTP fixtures under `testdata/`.
4. **HRD-012** — replace `commons_messaging/dispatch/claude_code/claude_code.go`'s `Dispatch` stub with a real `claude --resume` invocation; capture session UUID; parse `<<<HERALD-REPLY>>>`.
5. **HRD-016** — scaffold `pherald/internal/http/` with Gin routes per V3 §41; wire `pherald serve` to mount the Gin router on `http_port`.
6. **HRD-017** — propagate Universal §11.4.6X new mandates (spec-versioning + submodule-catalogue-first) into the constitution submodule.

## §4a. Wave 6 live-test handoff (T10b — gating tag v0.4.0)

**Operator-supplied credentials required** — Wave 6's e2e invariants E63-E70 land as honest SKIPs until the live closed-loop runs against real Telegram + real Claude Code with real chat messages exchanged.

**Step-by-step.**

1. Export credentials in the shell that will drive the test:
   ```bash
   export HERALD_TGRAM_BOT_TOKEN="<your-bot-token-from-BotFather>"
   export HERALD_TGRAM_CHAT_ID="<numeric-chat-id-where-bot-is-admin>"
   export HERALD_CLAUDE_BIN="$(command -v claude)"   # path to the claude CLI binary
   export HERALD_PROJECT_NAME="Herald"                # or any operator-chosen session name
   ```
2. Pick a `<run-id>` (timestamp + 4-char nonce):
   ```bash
   RUN_ID="$(date -u +%Y-%m-%dT%H-%M-%S)-$(uuidgen | cut -c1-4)"
   QA_DIR="docs/qa/HRD-100-${RUN_ID}"
   mkdir -p "${QA_DIR}/attachments"
   ```
3. Type ONE message in the configured Telegram chat (script reads it via `getUpdates` within 60s).
4. Run the live closed-loop test:
   ```bash
   bash tests/test_wave6_live_loop.sh
   ```
   The script builds `pherald`, observes the original `message_id`, starts `pherald listen --bot-token "$HERALD_TGRAM_BOT_TOKEN" --chat-id "$HERALD_TGRAM_CHAT_ID" --qa-out-dir "$QA_DIR"` in the background, waits up to 45s for a reply with `reply_to_message_id == original`, and exits 0 on PASS.
5. Capture supplementary logs:
   ```bash
   cp /tmp/pherald-w6.log "${QA_DIR}/pherald-listen.log"
   # claude stdout/stderr — paths depend on the journaling setup. Copy whatever
   # the test produced into ${QA_DIR}/claude-stdout.log + claude-stderr.log.
   ```
6. Author a brief `${QA_DIR}/README.md` (5 lines minimum) narrating: who ran it, when, what message was sent, what reply came back, any anomalies. This is NOT auto-generated — operator-written narrative is the §107.x evidence anchor.
7. `git add ${QA_DIR}/ && git commit -m "Wave 6 step 10b: docs/qa/HRD-100-${RUN_ID}/ live closed-loop evidence"` then proceed to T13b (`v0.4.0` tag + 4-mirror push).

**If the script SKIPs** (any env var unset): the test prints `SKIP: <reason>` and exits 0. That is honest §11.4.3 hardware-absent SKIP-with-reason, not a PASS. Tag `v0.4.0` MUST NOT be created until at least one PASS run is committed to `docs/qa/HRD-100-<run-id>/`.

## §4b. qaherald-auto resume runbook (next session — gating tag v0.5.1)

**Status as of 2026-05-23 session end:**

- HEAD `8ad4cb9` on `main`; all 4 mirrors in sync (`github.com:vasic-digital/Herald.git`, `gitlab.com:vasic-digital/herald.git`, `gitflic.ru:vasic-digital/herald.git`, `gitverse.ru:vasic-digital/Herald.git`).
- Tag `v0.4.0` shipped (Wave 6 closed-loop). Tag `v0.5.0` shipped (Wave 6.5 lifecycle implementations — premature: only S1+S2 live evidence + carries the pre-`53a7ad3` envelope-action-guidance bug). Tag `v0.5.1` is the target after qaherald-auto T6-T8 land + full 15-scenario evidence committed.
- Plan: `docs/superpowers/plans/2026-05-23-qaherald-auto.md` (commit `52ce679`).
- Done: T1 skeleton + T2 messenger + T3 scenarios + T4 orchestrator + T5 report (combined commit `8ad4cb9` for T3-T5).
- TODO: **T6 pre-flight + T7 hermetic tests + T8 shell-adapter** then live run + tag.

### Resume prerequisites

The following are **already provisioned** in `~/.zshrc` (export-on-shell-start):

```bash
HERALD_TGRAM_BOT_TOKEN     # @atmosphere_worker_bot — pherald-bot (Telegram bot 8823384001)
HERALD_TGRAM_CHAT_ID       # -4946584787 (ATMOSphere Development group)
HERALD_OPERATOR_IDS        # 2057253161 (the operator's user-id)
HERALD_QA_BOT_TOKEN        # @pherald_qa_bot — 2nd bot (8971749017, privacy DISABLED, member of the group)
```

**The qa-bot token is plaintext in `~/.zshrc`** — operator should `/revoke` via @BotFather after Wave 6.5 close-out and regenerate (the token leaked into one prior session's bash output earlier today, was never committed to git but lives in `/private/tmp/claude-501/...` operator-local files).

The containers `herald-postgres` (port 24100) and `herald-redis` (port 24200) are LIVE — both have been Up since the start of this session; data persists across podman restarts. Schema is at migration v12 (all 12 applied).

### Resume sequence (suggested next-session order)

1. **Pull state + verify**:
   ```bash
   cd ~/Projects/Herald && git pull origin main && git log --oneline -10
   git status --short  # expect clean
   git stash list      # 2 stashes preserved: T8 Wave-5 salvage + premature S3-S5 transcript WIP
   ```

2. **Spawn T6 (pre-flight)** via a fresh subagent — read `docs/superpowers/plans/2026-05-23-qaherald-auto.md` Task 6 in full. Implementation lands at `qaherald/internal/lifecycle/preflight.go` + `preflight_test.go`. 10 gates: pherald-bot reachable via getMe, qa-bot reachable, qa-bot privacy disabled, qa-bot is group member, OPERATOR_IDS contains qa-bot, etc. Each gate has distinct exit code for diagnostics.

3. **Spawn T7 (full hermetic tests)** — extends T3-T5's 12 tests with httptest-based scenario simulation. Coverage targets: every scenario PASS path + every FAIL diagnostic + S9 SKIP path + S14 outbound-attachment sha256 round-trip + S11/S12/S13 inbound-attachment download + Issues.md/Fixed.md fs-mutation assertion.

4. **Spawn T8 (shell-script adapter)** — rewrites `tests/test_wave6.5_lifecycle.sh` to delegate to `qaherald lifecycle` for automated runs. Keeps the original `--manual` flag for the operator-typing interactive UX as a fallback.

5. **Run the live e2e (the §107 watershed)**:
   ```bash
   cd ~/Projects/Herald
   # Add qa-bot user-id to OPERATOR_IDS so S5/S6/S8/S10 succeed
   export HERALD_OPERATOR_IDS="${HERALD_OPERATOR_IDS},8971749017"
   # Re-export from .zshrc (or just open a new shell)
   source ~/.zshrc

   # Build pherald + qaherald
   go build -o /tmp/pherald ./pherald/cmd/pherald
   go build -o /tmp/qaherald ./qaherald/cmd/qaherald

   # Start pherald listen in background with QA journaling
   RUN_ID="$(date -u +%Y-%m-%dT%H-%M-%S)-w6.5live"
   PHERALD_QA_DIR="docs/qa/HRD-101-lifecycle-${RUN_ID}-pherald"
   QAUTO_QA_DIR="docs/qa/HRD-101-lifecycle-${RUN_ID}"
   mkdir -p "${PHERALD_QA_DIR}/attachments" "${QAUTO_QA_DIR}/attachments"
   /tmp/pherald listen --qa-out-dir "${PHERALD_QA_DIR}" --docs-dir docs &
   PHERALD_PID=$!
   trap 'kill -TERM $PHERALD_PID 2>/dev/null' EXIT
   sleep 5

   # Run qaherald lifecycle (T6-T8 must be done first)
   /tmp/qaherald lifecycle \
     --pherald-bot-username=atmosphere_worker_bot \
     --pherald-qa-out-dir="${PHERALD_QA_DIR}" \
     --out="${QAUTO_QA_DIR}" \
     --run-id="${RUN_ID}"
   # Exit 0 on all-PASS or all-PASS-with-S9-SKIP. Non-zero on any FAIL.
   ```

6. **Capture evidence**:
   - `${PHERALD_QA_DIR}/transcript.jsonl` — pherald's view (classifications, dispatch, replies)
   - `${QAUTO_QA_DIR}/transcript.jsonl` — qaherald-auto's view (sends, assertions)
   - `${QAUTO_QA_DIR}/report.md` — Markdown summary
   - `${QAUTO_QA_DIR}/attachments/<sha256>.<ext>` — content-addressed inbound + outbound attachments
   - `docs/Issues.md` + `docs/Fixed.md` — mutated by S5/S6/S8/S10 (and reverted by S10 + S15 cleanups; both files end in valid state)

7. **Author operator README + commit**:
   ```bash
   cat > "${QAUTO_QA_DIR}/README.md" <<EOF
   # HRD-101 Wave 6.5 lifecycle evidence — <run-id>

   <5+ lines narrating: who ran it, when, which scenarios PASSed/FAILed/SKIPped,
   bot reply quality observations, any anomalies. Operator-written, NOT auto-generated.>
   EOF
   git add docs/qa/HRD-101-lifecycle-${RUN_ID}*
   git commit -m "Wave 6.5 step 13: HRD-101 live 15-scenario lifecycle evidence (qaherald-auto)"
   ```

8. **Tag v0.5.1 + 4-mirror push**:
   ```bash
   git tag -a v0.5.1 -m "Wave 6.5 close: full lifecycle live evidence + envelope action-guidance fix"
   git push origin main
   git push origin v0.5.1
   # Verify all 4 mirrors converged on the same SHA + tag
   for mirror in github.com:vasic-digital/Herald.git gitlab.com:vasic-digital/herald.git gitflic.ru:vasic-digital/herald.git gitverse.ru:vasic-digital/Herald.git; do
     git ls-remote "git@${mirror}" v0.5.1
   done
   ```

### Open / known constraints for tomorrow

- **S9 (non-operator Done: rejection)**: requires `HERALD_QA_BOT_TOKEN_NON_OPERATOR` env (a THIRD bot account whose user-id is NOT in OPERATOR_IDS). Without it, S9 emits a SKIP-with-reason. Operator may register a third bot OR accept the SKIP (S9's logic is already unit-tested hermetically in T5's mutation gate).
- **First-scenario CC bootstrap**: ~30s on first inbound message because the Claude Code session must spawn. Subsequent scenarios use the cached session (~5-15s typical).
- **Issues.md mutation race**: if you run `qaherald lifecycle` while ANOTHER pherald listen is also processing in the same checkout, Issues.md may receive double-writes. Single-pherald-listen invocation is the safe default.
- **Pre-existing carry-over**: 15 e2e_bluff_hunt FAILs on Postgres-SASL invariants (E7-E12 + E14-E16 + E37-E42). Root cause is local-PG container's SASL handshake; not related to Wave 6/6.5; can be addressed in a separate HRD post-v1.0.0.

### Safe-shutdown checklist (NOW — end of 2026-05-23 session)

| Check | State |
|---|---|
| All committed work pushed to 4 mirrors | YES — HEAD `8ad4cb9` confirmed on github+gitlab+gitflic+gitverse |
| Local working tree clean | YES — `git status --short` empty |
| Stashes preserved | 2 stashes: T8 Wave-5 salvage (older) + premature mid-test S3-S5 transcript WIP (newer, will be regenerated cleanly by qaherald-auto in next session) |
| Background pherald/qaherald processes | NONE running (`ps aux \| grep pherald` empty) |
| Container state | `herald-postgres` + `herald-redis` Up 17 hours — persist data across `podman stop && podman start`; can be left running OR stopped (`podman stop herald-postgres herald-redis`); next session restart is `podman start herald-postgres herald-redis` |
| `/tmp/*.log` files | one operator-token leak shred-deleted earlier today; remaining transient logs contain no credentials (verified) |
| `.zshrc` | contains 4 HERALD_* exports including `HERALD_QA_BOT_TOKEN` plaintext — operator should `/revoke` + regenerate via @BotFather post-Wave-6.5 closure |
| Memory entries | All session findings persisted under `/Users/milosvasic/.claude/projects/-Users-milosvasic-Projects-Herald/memory/` — survive across sessions |

**Machine is safe to shut down.** Resume tomorrow by `cd ~/Projects/Herald && git pull origin main` and start with qaherald-auto T6.

## §5. Long-form pointers

- `docs/specs/mvp/specification.V3.md` — full active spec (Revision 4).
- `docs/specs/mvp/specification.V3.md#30-v2-self-review-log` — every review pass.
- `docs/guides/HERALD_CONSTITUTION.md` — §101..§106 extending Universal.
- `docs/guides/CONSTITUTION_INHERITANCE.md` — parent-discovery + gate.
- `tests/test_constitution_inheritance.sh` — the gate.
- `quickstart/` — HRD-008 scaffold.
- `commons/types.go` — the §11.0 type contract reference.
