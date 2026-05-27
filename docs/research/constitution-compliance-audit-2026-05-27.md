<div align="center">

<img src="../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Herald Constitution-Compliance Audit — 2026-05-27

| Field | Value |
|---|---|
| Date | 2026-05-27 |
| Repo | Herald (`/Users/milosvasic/Projects/Herald`) @ HEAD `bdbe9f1` |
| Inherited constitution | sibling `/Users/milosvasic/Projects/constitution/` (parent-discovery per §11.4.35) |
| Constitution HEAD audited | **`acbcc6c`** (task brief said `3a085b9`; the constitution has ADVANCED since Herald's last pull — see GAP-1) |
| Auditor | read-only subagent |
| Method | static-read-only (Read / Grep / Glob / `git log`/`git grep`/`git show`; NO execution of any gate, e2e, or container) |
| Scope | §11.4 anti-bluff covenant family (§11.4..§11.4.93) + §1.1 paired-mutation + inheritance gate + secrets hygiene |

---

## 0. Executive summary

Herald's anti-bluff infrastructure is **substantially mature and genuinely load-bearing** — `e2e_bluff_hunt.sh` (80 invariants) drives real binaries/servers and asserts wire-level behaviour, six paired-mutation gates exist, and the inheritance gate (I1–I8c) structurally matches the current root docs. The dominant gaps are **(a) drift against the constitution's newest mandates (§11.4.89–§11.4.93, added after Herald's last pull), (b) two universal anti-bluff helpers explicitly mandated but still unimplemented (`mutation_residue_audit.sh` for §11.4.84/§107.y, and any stress/chaos suite for §11.4.85), and (c) docs/qa evidence coverage that lags far behind the count of shipped features (§11.4.83/§107.x).**

Secrets-hygiene scan: **CLEAN** — no full bot token, private key, or MTProto `app_hash` is present in any tracked file. The only sensitive-looking string is the bare numeric bot ID `8971749017` (an identifier, not a credential) in `docs/CONTINUATION.md`.

Tally: **18 COMPLIANT / 9 PARTIAL / 6 GAP** (N/A rules omitted from the tally; see matrix).

---

## 1. Compliance matrix

| Rule | Short name | Verdict | Evidence |
|---|---|---|---|
| §11.4 | End-user quality guarantee (forensic anchor) | COMPLIANT | `CLAUDE.md` §107 covenant block; `docs/guides/HERALD_CONSTITUTION.md` §107; gate `tests/test_constitution_inheritance.sh:70` (I2) + `:133-137` (I8a/b/c) |
| §11.4.1 | FAIL-bluffs forbidden | COMPLIANT | `scripts/e2e_bluff_hunt.sh:158` `set -uo pipefail` + `check()` helper `:201-213` records genuine fail; mutation gates assert FAIL on mutation, not on script crash |
| §11.4.2 | Recorded-evidence requirement | PARTIAL | e2e invariants cite captured artefacts (`docs/qa/HRD-100/transcript.jsonl`, response bodies). But only 2 `docs/qa/` run-dirs exist for ~80 shipped feature-invariants → most features lack a recorded transcript (see §11.4.83 GAP) |
| §11.4.3 | Per-topology SKIP-with-reason | COMPLIANT | 68 explicit `SKIP …§11.4.3 explicit SKIP-with-reason` lines in `scripts/e2e_bluff_hunt.sh` (e.g. `:370`, `:443`, `:460`); never PASS-by-default |
| §11.4.4 | Test-interrupt + 4-layer coverage | PARTIAL | Pre-build (gate) + runtime (e2e) + meta (mutation) layers present per feature; HelixQA-Challenge layer N/A for a Go service. But §11.4.4(b) four-layer is not uniformly met for items closed without a `docs/qa/` transcript |
| §11.4.5 | Captured-evidence quality analysis | PARTIAL | E58/E59/E60 analyze TOON wire-bytes (real content checks); E47 asserts `current_mem_percent>0` etc. Audio/video clauses N/A. Quality analysis is thin for the inbound-runtime transcripts (E64–E69 mostly SKIP) |
| §11.4.6 | No-guessing | COMPLIANT | SKIP reasons are concrete + closed-set (`hardware_not_present`, creds-absent); CONTINUATION debt items state facts with commit SHAs |
| §11.4.7 | Demotion-evidence | N/A | No demotion events in scope; no `UNCONFIRMED:` items observed |
| §11.4.8 | Deep-web-research-before-impl | COMPLIANT | `docs/research/telegram-bot-to-bot-constraint.md` + `docs/research/protocols/` document pre-implementation research |
| §11.4.10 / .10.A | Credentials-handling + pre-store leak audit | COMPLIANT | `.env` not tracked (confirmed); `quickstart/.env.example` committed with placeholder values; `docs/guides/OPERATOR_CREDENTIALS.md` audit checklist; no real token in tracked tree (see §3 scan) |
| §11.4.11 | File-layout discipline | COMPLIANT | scripts under `scripts/`, tests under `tests/`, docs under `docs/`, research under `docs/research/` |
| §11.4.12 | Auto-generated docs sync | PARTIAL | `scripts/export_docs.sh` exists + regenerates HTML/PDF/DOCX. But CONTINUATION debt item (c) records `docs/Issues.md`/`docs/Fixed.md` siblings STALE after HRD-110..113 migrations — sync drift acknowledged but not yet closed |
| §11.4.15 | Item-status tracking | COMPLIANT | `docs/Issues.md` uses the closed-set Status vocabulary; CONTINUATION §3 "Active work" table tracks live items |
| §11.4.17 | Universal-vs-project classification | COMPLIANT | Herald CLAUDE.md §"Inherited covenant restatements" explicitly restates+cites, does not redefine |
| §11.4.19 | Fixed-document atomic migration | COMPLIANT | HRD-100/HRD-101 atomic Issues→Fixed migrations recorded; `docs/Fixed.md` present |
| §11.4.20 / .70 | Subagent-driven-by-default | COMPLIANT | TaskList shows 80+ subagent-dispatched tasks; Wave work executed subagent-driven |
| §11.4.27 | No-fakes-beyond-unit + 100% type coverage | COMPLIANT | e2e drives REAL Gin servers + REAL binaries + (when present) REAL Postgres/Redis; integration tags `-tags=integration` |
| §11.4.35 | Canonical-root inheritance clarity | COMPLIANT | `CLAUDE.md` "INHERITED FROM Helix Constitution" + I5a–d gate (`tests/test_constitution_inheritance.sh:82-90`); no embedded `constitution/` submodule (I6 `:109`) |
| §11.4.40 | Full-suite retest before release tag | COMPLIANT | `e2e_bluff_hunt.sh` is the canonical pre-tag battery; CONTINUATION §4a/§4b gate tags on live evidence |
| §11.4.65 | Universal Markdown export | COMPLIANT | `scripts/export_docs.sh` + `scripts/branding_inject_logo.py` produce HTML/PDF/DOCX siblings |
| §11.4.83 | docs/qa/ end-user evidence mandate | **GAP** | Only **2** `docs/qa/` run-dirs (`HRD-100-…-w6live`, `HRD-101-lifecycle-…-w6.5live`). The 6 Wave-2 flavor binaries, Wave 3a/3b routes, Wave 4a HTTP/3, Wave 4b TOON, and Wave 7 multi-channel all shipped WITHOUT a `docs/qa/<run-id>/` transcript. Per §11.4.83 each is a §107 PASS-bluff at the QA-evidence layer (`docs/qa/` Glob) |
| §11.4.84 / §107.y | Working-tree quiescence + residue scanner | **GAP** | `check_quiescence()` exists in `tests/test_wave4b_mutation_meta.sh:92`, `test_wave6_mutation_meta.sh`, `test_wave6.5_mutation_meta.sh` — but is ABSENT from `test_wave2/3/4_mutation_meta.sh`. The mandated universal `scripts/mutation_residue_audit.sh` is **MISSING**. No gate uses the `.git/MUTATION_IN_PROGRESS` lockfile §11.4.84(3) requires |
| §11.4.85 | Stress + chaos test mandate | **GAP** | NO stress/chaos test files exist (`tests/*stress*` / `*chaos*` glob empty); NO `scripts/stress_chaos.sh` helper. Wave-7 plan T11 lists it as future. Every fix since 2026-05-24 shipped without stress/chaos coverage → resilience-layer PASS-bluff |
| §11.4.86 | Roster/corpus Status-doc auto-sync | N/A | Herald has no roster/asset-corpus-backed Status doc (no installed-app roster, no media corpus). Rule's own clause exempts projects without such a subject |
| §11.4.87 | Endless-loop autonomous + zero-idle | PARTIAL | Anchor literal `11.4.87` present in `CLAUDE.md` (9×) + `AGENTS.md` (7×) — propagation gate satisfied. CONTINUATION cites the termination contract. But CONTINUATION §3 "Active work" is NON-empty (HRD-008/010/011/012/015/016/017) and 107 open HRD rows remain → the loop's all-clear condition is not met (expected mid-project; not a violation, but the contract is live, not satisfied) |
| §11.4.88 | Background-push mandate | PARTIAL | Anchor literal `11.4.88` present in `CLAUDE.md` (8×) + `AGENTS.md` (7×) — propagation satisfied. But Herald has NO `commit_all.sh` / `push_all.sh` in-repo; the per-remote flock + `nohup … & disown` wiring §11.4.88(B)(C) mandates is not present in Herald's own tooling (`upstreams/*.sh` only export the URL) → `CM-BACKGROUND-PUSH-WIRED` would FAIL if run against Herald |
| §11.4.89 | Background test execution mandate | **GAP** | Added constitution `98a6ff8` 2026-05-27 (AFTER Herald's last pull `3a085b9`). NOT propagated: `CLAUDE.md`/`AGENTS.md` anchor count for `11.4.89` = **0**. No background-test wiring in Herald scripts |
| §11.4.90 | Obsolete status + obsolescence audit | **GAP (drift)** | Added 2026-05-27 (`ea7e284`). NOT propagated (anchor `11.4.90` count = 0); §11.4.15 closed-set in `docs/Issues.md` lacks the `Obsolete (→ Fixed.md)` 4th terminal value + colorizer class |
| §11.4.91 | Summary-doc clarity mandate | PARTIAL (drift) | Added 2026-05-27. Not propagated (count = 0). Herald's `*_Summary.md` one-liners not yet re-evaluated against the forbidden-section-label list |
| §11.4.92 | Multi-pass change-evaluation discipline | PARTIAL (drift) | Added 2026-05-27. Not propagated (count = 0) |
| §11.4.93 | SQLite single-source-of-truth for workable items | PARTIAL (drift) | Added 2026-05-27. Not propagated (count = 0); Herald tracks workable items in Markdown `docs/Issues.md`, not a SQLite SoT |
| §1.1 | Paired-mutation proof | PARTIAL | Six gates exist (wave2/3/4/4b/6/6.5). Structure is correct (mutate→assert-FAIL→restore→quiescence→post-flight, e.g. `test_wave4b_mutation_meta.sh:104-214`). BUT CONTINUATION debt + task #180/#181 record `wave4` M2 + (formerly) `wave6.5` M3 stale anchors → at least one gate is non-functional; structure not verified by execution per safety constraints |

---

## 2. GAPS + PARTIALS — actionable (ranked by severity)

### GAP-2 (CRITICAL) — §11.4.84 / §107.y residue scanner missing + quiescence not universal
`scripts/mutation_residue_audit.sh` is mandated by Herald's own CLAUDE.md §107.y(5) and constitution §11.4.84(5) but does **not exist**. Only 3 of 6 mutation gates carry `check_quiescence()`; `test_wave2/3/4_mutation_meta.sh` have none, and no gate uses the `.git/MUTATION_IN_PROGRESS` lockfile. This is the exact rule born from the `72e81ab`/`d5bd360` security-defect incident.
**Remediation:** implement `scripts/mutation_residue_audit.sh` (pre-push scanner for `MUTATED`, `// always pass`, `_mutated_*`, `.w*meta-backup`); add `check_quiescence()` + lockfile to wave2/3/4 gates. Open a new HRD (the §107.y text already reserves "HRD-NNN to be assigned").

### GAP-3 (HIGH) — §11.4.85 stress + chaos suites entirely absent
No stress or chaos test exists anywhere in `tests/`; no `scripts/stress_chaos.sh`. Every fix landed since 2026-05-24 (incl. the `bdbe9f1` Runner nil-Redis fix and Wave 7) shipped without the mandated resilience coverage.
**Remediation:** ship `scripts/stress_chaos.sh` helpers + a stress/chaos suite for the live surfaces named in CLAUDE.md §11.4.85 (`pherald listen` concurrent updates, `/v1/*` sustained load, claude_code dispatch process-death, container disk/OOM). Evidence under `docs/qa/<run-id>/stress_chaos/`. This is Wave-7 T11 in the plan — promote it.

### GAP-4 (HIGH) — §11.4.83 / §107.x docs/qa coverage lags shipped features
Only `HRD-100` (Wave 6) and `HRD-101` (Wave 6.5) have `docs/qa/` transcripts. Waves 2/3a/3b/4a/4b/7 (flavor binaries, JWT routes, HTTP/3, TOON, multi-channel) shipped without one.
**Remediation:** back-fill `docs/qa/<HRD>/` transcripts (request+response bodies for /v1 routes; version-probe captures for flavor binaries) OR explicitly classify pre-§11.4.83 features as exempt-by-date in `docs/Fixed.md`. Tie each new e2e invariant's positive-evidence anchor to its `docs/qa/<run-id>/` per §107.x(6).

### GAP-1 (HIGH) — constitution drift: §11.4.89–§11.4.93 unpropagated
The constitution advanced from `3a085b9` (Herald's last pull, 2026-05-26) to `acbcc6c` (2026-05-27), gaining §11.4.89 (background-test), §11.4.90 (Obsolete status), §11.4.91 (summary clarity), §11.4.92 (multi-pass eval), §11.4.93 (SQLite SoT). Herald's `CLAUDE.md`/`AGENTS.md` carry zero anchors for any of them, so the `CM-COVENANT-114-89-PROPAGATION` (and successors) pre-build gates would FAIL.
**Remediation:** pull constitution to `acbcc6c`; add short-form restatement blocks citing literal `11.4.89`/`90`/`91`/`92`/`93` to `CLAUDE.md` + `AGENTS.md` + `QWEN.md` per §11.4.35; extend `docs/Issues.md` Status closed-set with `Obsolete (→ Fixed.md)` + colorizer class (§11.4.90). Open one HRD per mandate or a single propagation HRD.

### PARTIAL-5 (MEDIUM) — §1.1 stale mutation-gate anchors
Task #180 (wave4 M2 Brotli gate stale anchors `gin.SetMode`/`SafetyState`) is still pending; #181 (clean one-at-a-time re-run) pending. A gate whose mutation anchor no longer matches the source silently never mutates → a §11.4.1 FAIL-bluff (or a non-functional gate). CONTINUATION earlier recorded the same class for wave6.5 M3 (now fixed `8071248`).
**Remediation:** refresh the wave4 M2 perl anchors to current source; re-run each gate one-at-a-time foreground (per the operator's no-concurrent-mutation-gates rule) to confirm mutate→FAIL→restore.

### PARTIAL-6 (MEDIUM) — §11.4.88 background-push not wired in Herald's own tooling
Herald has no `commit_all.sh`/`push_all.sh`; the per-remote-flock + detached-push machinery the mandate requires lives only in the parent project. `upstreams/*.sh` only export URLs.
**Remediation:** either adopt the parent's `commit_all.sh`/`push_all.sh` into Herald or document that Herald inherits push tooling from the parent checkout; ensure `CM-BACKGROUND-PUSH-WIRED` has a satisfiable target.

### PARTIAL-7 (LOW) — §11.4.12 sibling-export drift
`docs/Issues.md`/`docs/Fixed.md` PDF/HTML/DOCX siblings stale after HRD-110..113 (CONTINUATION debt item c).
**Remediation:** `bash scripts/export_docs.sh docs/Issues.md docs/Fixed.md` at Wave-7 close (already queued).

### PARTIAL-8 (INFORMATIONAL) — §11.4.87 loop live, not satisfied
CONTINUATION §3 "Active work" non-empty + 107 open HRD rows. Expected for a mid-project; recorded for completeness, not a defect.

---

## 3. Secrets-hygiene scan

Operator mandate: "no sensitive data can leak or be git versioned through logs or git repo." Scans were run with `git grep` over the tracked tree (`.pdf`/`.docx` binaries excluded where noted).

| Pattern | Intent | Result |
|---|---|---|
| `[0-9]{8,10}:[A-Za-z0-9_-]{30,}` | Telegram bot token (`id:hash`) | Only PLACEHOLDERS: `1234567890:AAH1lab2c3d4e5f6g7h8i9j0kK_LmN3oPqR4S5t` (docs), `0000000000:XXX…` + `9999999999:ENVTOKEN…` (test fixtures), `1234567890:AAAA-sensitive-bot-token-DO-NOT-LEAK` (negative-test fixture in `qaherald/internal/messenger/telegram_test.go:633`). **No real token.** |
| `8971749017:[A-Za-z0-9_-]{20,}` | The reportedly-leaked `@pherald_qa_bot` FULL token | **NOT FOUND** — only the bare numeric ID `8971749017` appears (`docs/CONTINUATION.md:147`, `.html:472`), which is a non-secret identifier, not the `id:hash` credential. |
| `pherald_qa_bot` | QA-bot references | Present in `docs/CONTINUATION.md`, `docs/research/telegram-bot-to-bot-constraint.md` as prose + the bare ID; no token. CONTINUATION already instructs operator to `/revoke`. |
| `HERALD_QA_BOT_TOKEN[ =:]+<value>` | Assigned QA token | **Never assigned a value** in any tracked file (only referenced as an env-var NAME). |
| `app_hash` / `api_hash` | MTProto secret | Only prose ("operator must provide my.telegram.org app_id+app_hash") — no actual hash value. |
| `BEGIN … PRIVATE KEY` | Private keys | **None.** |
| tracked `.env` | Live credential file | **Not tracked** (only `quickstart/.env.example` placeholder). |

**Verdict: CLEAN.** No credential, key, or secret value is git-versioned. The single residual note: the bare bot ID `8971749017` in CONTINUATION is a low-value identifier; combined with the operator's already-recorded `/revoke` instruction, it is not a leak per the mandate. (Recommend the operator confirm the `@pherald_qa_bot` token was revoked, since CONTINUATION flags it as having leaked into a chat transcript that lives OUTSIDE this repo.)

---

## 4. Inheritance gate structural confirmation (read-only)

`tests/test_constitution_inheritance.sh` (read, NOT executed) still structurally matches the current root docs:

- **I1** (`:53`) walks parents for `constitution/Constitution.md` — matches the sibling-dir parent-discovery contract.
- **I2/I3/I4** (`:70/74/78`) assert the §11.4 anchor in Constitution.md / CLAUDE.md / AGENTS.md of the discovered constitution.
- **I5a–d** (`:82-90`) one check per root doc declaring parent-discovery (CLAUDE.md, AGENTS.md, HERALD_CONSTITUTION.md, README.md) — all four docs present.
- **I6** (`:109`) forbids an embedded `constitution/` submodule — confirmed none present; note the gate was refined (I6 now permits a `.gitmodules` for vendored SDKs, per the in-file comment `:98`), so the upcoming Wave-7 T6 slack-go submodule will not trip it.
- **I7a–c** (`:119-123`) spec-change-rule anchor in CLAUDE.md/AGENTS.md/HERALD_CONSTITUTION.md §106 — present.
- **I8a–c** (`:133-137`) §107 end-user-usability covenant anchor in CLAUDE.md/AGENTS.md/HERALD_CONSTITUTION.md — present; paired by `test_i8_usability_meta.sh`.

Structural match: **OK.** (Functional PASS not asserted — gate not executed per safety constraints.)

---

## 5. Non-execution attestation

This audit was performed **read-only**. The auditor:

- ran **NO** mutation gate (no `tests/test_wave*_mutation_meta.sh`, no `*_meta.sh`, no script that mutates tracked source);
- ran **NO** `scripts/e2e_bluff_hunt.sh`, `scripts/release.sh`, container boot, or server start;
- executed **NO** `git` command that mutates the working tree or index (only `git log`/`git grep`/`git status`/`git show` read-only);
- made **NO** edit to any existing file and **NO** Go-source change;
- wrote **EXACTLY ONE** new file — this report at `docs/research/constitution-compliance-audit-2026-05-27.md`.

`git status --porcelain` at audit start showed a clean tree (no `MUTATED` markers, no `.w*meta-backup` residue). No residue was found at any point.
