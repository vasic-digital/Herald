<div align="center">

<img src="../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Herald — Session Resume (2026-05-27)

> **Read this first to continue tomorrow.** This is the single entry-point for resuming Herald work. It points at the canonical lifecycle handoff (`docs/CONTINUATION.md`) and the execution plans, and records the exact next action, the operating discipline, and the open decisions.

| Field | Value |
|---|---|
| Date | 2026-05-27 (session end) |
| HEAD | `d42c165` (all 4 mirrors synced: GitHub / GitLab / GitFlic / GitVerse) |
| Anti-bluff status | `scripts/e2e_bluff_hunt.sh` = **81 PASS / 0 FAIL**; all mutation gates green |
| Open HRDs | **37** (was 40 at session start; 13 closed this session) |
| Working tree | clean; residue scan CLEAN; no mutation lockfile; no background processes |
| Safe to power off? | **YES** — see §8 |
| Canonical handoff | `docs/CONTINUATION.md` (r13) — the authoritative lifecycle continuation doc |

---

## 1. One-line resume instruction

**Resume at v1.0.0 Batch B — HRD-019..025 (§42 flavor constitution bindings).** They emit constitution events *through* the now-complete `commons_constitution` Evaluator + audit foundation (landed in Batch A). Full sequence + context in `docs/superpowers/plans/2026-05-27-v1.0.0-execution.md`.

---

## 2. Machine state — verified clean at session end

- All work **committed + pushed to all 4 mirrors** at `d42c165` (0 unpushed commits).
- Working tree clean; `scripts/mutation_residue_audit.sh` → CLEAN; `.git/MUTATION_IN_PROGRESS` absent.
- No background gates / e2e / exports / pushes running.
- `qa-results/` (gitignored) holds this session's detached-run logs.

## 3. What this session accomplished (operator mandate: "do EVERYTHING")

**27 commits this continuation, every unit anti-bluff-verified with real captured evidence, all pushed.**

### Constitution compliance (the earlier "pull + follow every rule" instruction)
- Pulled the inherited HelixConstitution sibling `3a085b9 → 3c9c4e9` (gained §11.4.89–94).
- **GAP-1**: propagated §11.4.89–94 into `CLAUDE.md` r12 / `AGENTS.md` r8 / `docs/guides/HERALD_CONSTITUTION.md` r6 §108.d-i. Inheritance gate 15 PASS / 0 FAIL.
- **GAP-2**: built `scripts/mutation_residue_audit.sh` (§107.y pre-push residue scanner) + back-filled `check_quiescence()` + `.git/MUTATION_IN_PROGRESS` lockfile into wave2/3/4 gates.
- **§11.4.91** summary-doc clarity; **§11.4.90** obsolescence audit (zero obsolete rows); operator correction: **`docs/.workable_items.db` is version-controlled, NOT gitignored** (a deliberate Herald divergence from parent §11.4.93 — see `memory` + the §108.h restatement).
- All 6 mutation gates re-verified green (wave2/3/4/4b/6/6.5), one-at-a-time.

### Workstream 1 — GAP-3 §11.4.85 stress+chaos + HRD-132 — ✅ COMPLETE
- 8 units (HRD-122..130): scaffold `commons/stresschaos/`, Runner, `/v1/events`, `/v1/compliance`+`/v1/safety_state`, `pherald listen`, claude_code, container/resource (§12.6-safe), e2e E81-E88 wiring, paired §1.1 mutation gate `tests/test_stress_chaos_mutation_meta.sh` (6/0).
- `e2e_bluff_hunt` 60 → 78 PASS.
- **HRD-132** (real bug GAP-3 found): Runner **claim-before-dispatch** — `events_processed` claimed at Stage 2 (`INSERT … ON CONFLICT DO NOTHING`, `RowsAffected==1`) is now the authoritative **exactly-once dispatch** gate; closed a latent `CachedRcpt` data race. wave3 M4 honestly retired (HRD-132 made it non-load-bearing).

### Workstream 2 — §11.4.93 SQLite SSoT — ⏸ OPERATOR-DEFERRED
- The constitution's `workable-items` Go binary is an **unimplemented scaffold (0/7 subcommands)** + schema-incompatible with Herald's HRD-NNN pipe-table format. Per §11.4.74 Herald can't reimplement — the fix is a **constitution-repo task** affecting all consumers.
- Assessment: `docs/research/workable-items-phase2-assessment-2026-05-27.md`. HRD-131 stays **open-deferred**.

### Workstream 3 — v1.0.0 — 🔄 PLANNED + Batch A DONE
- Plan: `docs/superpowers/plans/2026-05-27-v1.0.0-execution.md` (Batches A→E, tag cadence v0.5→v1.0.0).
- **Batch A done** (HRD-018/026/027): `commons_constitution` **audit write-through** (the `constitution_audit` table existed but *nothing wrote to it* — a §107 bluff: Runner set `Audited=true` persisting zero rows; FIXED) + admin **mode-flip REST** (`GET/GET-one/PUT /v1/compliance/modes/:rule_id`). `e2e_bluff_hunt` 78 → **81 PASS** (E89 wired). §11.4.85 stress: 512 concurrent emit→persist → exactly 512 audit rows.

### Workstream 4 — GAP-4 docs/qa back-fill + nano-docs (#147) — ⏳ queued

### Real defects caught by anti-bluff verification this session
1. **HRD-132** — duplicate notification dispatch under concurrent replay (now exactly-once).
2. **Audit write-through bluff** — `Audited=true` with zero persisted rows (Batch A fix).
3. **wave4 M2** — confirmed already load-bearing (stale prior-session diagnosis).
4. **HRD-123** flaky 8 MiB-body assertion (§11.4.50 — transport-rejection is a valid defense).
5. **E82 stale anchor** — committed evidence out of sync with the test (re-captured).
6. **wave3 M4** — my own HRD-132 refactor made it non-load-bearing (honestly retired).

## 4. Where to resume — v1.0.0 remaining batches (priority order)

Per `docs/superpowers/plans/2026-05-27-v1.0.0-execution.md`:

| Batch | HRDs | What | Notes |
|---|---|---|---|
| **B (NEXT)** | HRD-019..025 | §42 flavor constitution bindings (cherald/sherald/bherald/rherald/iherald/scherald/pherald) | Depend on the now-complete `commons_constitution` Evaluator+audit foundation; wire flavors to emit through it |
| C | HRD-029..056 | §43 command catalogue (5 flavor clusters) | Many are thin wrappers over existing Helix-stack submodules (§11.4.74 reuse) |
| D | HRD-081, HRD-085..090 | `commons_infra` TaskRepository surfaces | 085-089 are A-independent (parallelisable); 090 deps A |
| E | HRD-115..121 | Wave 7 T6-T12 (Slack adapter etc.) | **Gotcha:** Wave 7 e2e invariants must renumber off E81-E89 → use **E90+**; slack-go submodule needs the **I6 inheritance-gate re-check** after the `.gitmodules` edit |

**Every v1.0.0 feature ships with**: TDD (RED→GREEN), `docs/qa/<run-id>/` evidence (§107.x), an `e2e_bluff_hunt` invariant (next free = **E90+**), a paired §1.1 mutation, and §11.4.85 stress/chaos (reuse `commons/stresschaos/`).

**Tag cadence**: v0.5.0 (Batch A) → v0.6.0 (B) → v0.7.0 (C+D) → v0.8.0 (E) → v1.0.0 (HRD-008 sign-off + full-suite green).

**Immediate cleanup logged (#194)**: `commons_constitution/audit_stress_chaos_test.go` writes evidence to `docs/qa` on every run (no `t.TempDir`-unless-`HERALD_STRESS_QA_DIR` guard like the events/runner stress tests) → dirties tracked evidence each e2e run. Add the guard alongside Batch B.

## 5. Canonical reference docs (the map)

| Doc | Purpose |
|---|---|
| `docs/CONTINUATION.md` (r13) | **Authoritative lifecycle handoff** — Issues/Fixed metadata, active work, resume-at |
| `docs/Issues.md` (r17) / `docs/Fixed.md` (r15) | The HRD trackers (SSoT until §11.4.93 lands) |
| `docs/superpowers/plans/2026-05-27-v1.0.0-execution.md` | The v1.0.0 batch plan (workstream 3) |
| `docs/superpowers/plans/2026-05-27-stress-chaos-suite.md` | GAP-3 plan (done; pattern reference) |
| `docs/research/workable-items-phase2-assessment-2026-05-27.md` | Why §11.4.93 is blocked |
| `docs/research/constitution-compliance-audit-2026-05-27.md` | Full constitution-compliance audit |
| `docs/research/hrd-obsolescence-and-qa-coverage-audit-2026-05-27.md` | §11.4.90 + §107.x coverage audit |
| `docs/research/telegram-bot-to-bot-constraint.md` | Why qaherald-auto needs MTProto |
| `CLAUDE.md` / `AGENTS.md` / `docs/guides/HERALD_CONSTITUTION.md` | Project rules + §11.4.89-94 restatements |

## 6. Operating discipline (hard-won this session — DO NOT violate)

- **Constitution is a SIBLING dir** at `/Users/milosvasic/Projects/constitution` (parent-discovery, NOT a Herald submodule). Pull it there.
- **Mutation gates run ONE-AT-A-TIME, never concurrently** (§107.y; real incident 2026-05-27). They transiently mutate source + restore via git-checkout, so: (a) commit your work **locally first** before running a gate (its restore would otherwise destroy uncommitted work), (b) never run two gates against the same checkout, (c) the `.git/MUTATION_IN_PROGRESS` lockfile + `scripts/mutation_residue_audit.sh` enforce this.
- **§11.4.92 Pass-1**: the conductor **independently re-runs** a subagent's tests (especially `-count=3` for determinism) — never trust a pasted "PASS". This caught 2 flaky/stale issues this session.
- **§11.4.89 background tests**: long suites run detached (`nohup … > qa-results/<id>.log 2>&1 & disown`); poll, don't block. **§11.4.88 background push**: `git push origin main` fans out to all 4 mirrors (the `origin` multi-URL remote).
- **PG/Redis for tests**: `export DOCKER_HOST="unix:///var/folders/t3/dmp1fb1d61xbl27trnjtr0_c0000gn/T/podman/podman-machine-default-api.sock"`; PG `herald-postgres:24100` pw `herald_dev`, Redis `:24200`. Hermetic-fake is preferred; real-PG via the `containers/` submodule.
- **Docs**: edited `.md` with committed siblings → re-run `bash scripts/export_docs.sh <file>` (HTML/PDF/DOCX). Scope-stage commits (`git add <paths>`, never `-A`).
- **Subagent-driven** (§11.4.94(D)): dispatch fresh-context subagents for non-trivial units; conductor stays the commit/integration/verify seam.

## 7. Open decisions / operator actions

- **§11.4.93 SQLite SSoT** — deferred; needs the constitution-repo `workable-items` binary implemented (cross-project, affects all consumers). Decide who/when.
- **MTProto real-channel automation** — needs operator `app_id` + `app_hash` from my.telegram.org + 1-time phone login (for `qaherald` real-channel inbound; the 2nd-bot approach is dead per the bot-to-bot wall).
- **`/revoke @pherald_qa_bot`** — the bot token leaked into an external chat transcript earlier; revoke via @BotFather (not in any tracked Herald file — secrets scan CLEAN).

## 8. Safe to power off the host?

**YES — safe to power off now.** Verified at session end:
- 0 unpushed commits; all 4 mirrors at `d42c165` (work is durable both locally AND on all remotes — even total local disk loss loses nothing).
- Working tree clean; no mutation residue; no `.git/MUTATION_IN_PROGRESS` lockfile.
- No background processes (gates / e2e / exports / pushes) running — nothing will be interrupted mid-write.

To resume tomorrow: open this file, then `docs/CONTINUATION.md`, then the v1.0.0 plan, and start **Batch B (HRD-019)**.
