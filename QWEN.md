<div align="center">

<img src="assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# QWEN.md

This file provides guidance to **Qwen Code** sessions when working with code in this repository, in parity with `CLAUDE.md` and `AGENTS.md`. The constitutional rules in those files apply unchanged to Qwen agents — this file restates the §107 anti-bluff anchor that MUST live verbatim in every root doc per the operator mandate.

For the full Herald-specific ruleset see:
- `CLAUDE.md` (Claude Code session guidance — primary)
- `AGENTS.md` (cross-agent guardrails)
- `docs/guides/HERALD_CONSTITUTION.md` (Herald's project-specific constitutional extensions)
- The parent constitution at `<parent>/constitution/Constitution.md` (Helix Universal Constitution — inherited unconditionally)

## INHERITED FROM Helix Constitution (parent-discovery)

Herald is consumed as a submodule of a parent project that carries the Helix Constitution submodule at `<parent>/constitution/`. Locate it from any nested depth via:

```bash
CONST_DIR="$(bash "$(find . -type d -name constitution -print -quit 2>/dev/null)/find_constitution.sh")"
```

For standalone development of Herald, clone the constitution alongside:

```bash
git clone git@github.com:HelixDevelopment/HelixConstitution.git $(dirname "$PWD")/constitution
```

All rules in `<discovered>/CLAUDE.md` + `<discovered>/Constitution.md` + `<discovered>/QWEN.md` apply unconditionally to Qwen agents.

## §107 — End-user-usability covenant (Herald §107 / Helix §11.4 — MANDATORY ANTI-BLUFF)

**Forensic anchor — verbatim operator mandate:**

> "all existing tests and Challenges do work in anti-bluff manner - they MUST confirm that all tested codebase really works as expected! We had been in position that all tests do execute with success and all Challenges as well, but in reality the most of the features does not work and can't be used! This MUST NOT be the case and execution of tests and Challenges MUST guarantee the quality, the completition and full usability by end users of the product! This MUST BE part of Constitution of our project, its CLAUDE.MD and AGENTS.MD if it is not there already, and to be applied to all Submodules's Constitution, CLAUDE.MD and AGENTS.MD as well (if not there already)!"

The bar for shipping any Herald feature is **NOT** "tests pass" — it is **"the end user of the flavor binary can actually use the feature."** Every PASS (unit, integration, gate, Challenge, smoke, e2e) MUST carry positive runtime evidence that the user-visible behaviour works. Metadata-only / configuration-only / "absence-of-error" / grep-only PASS are §11.4 PASS-bluffs and constitute critical defects regardless of how green the summary line looks.

Canonical Herald authority: `docs/guides/HERALD_CONSTITUTION.md` §107.
Canonical Helix authority: `<discovered>/Constitution.md` §11.4 + §11.4.1..§11.4.16 and `<discovered>/CLAUDE.md` "MANDATORY ANTI-BLUFF COVENANT — END-USER QUALITY GUARANTEE".
Canonical Herald evidence: `scripts/e2e_bluff_hunt.sh` (41 invariants against real services; ALL must PASS).

Inheritance gate invariant **I8a** asserts this covenant anchor is present in CLAUDE.md; **I8b** asserts it in AGENTS.md; **I8c** asserts it in HERALD_CONSTITUTION.md. This QWEN.md restates the anchor for Qwen Code session parity (added 2026-05-22 doc-cleanup pass alongside QWEN.md propagation across all Helix-stack submodules).

## §107.x — docs/qa/ Evidence Mandate (operator mandate, 2026-05-22; cascades from Helix §11.4.83)

**Forensic anchor — verbatim operator mandate (2026-05-22):**

> "every feature that ships MUST carry a recorded e2e communication transcript + any attached materials under `docs/qa/<run-id>/` (per-feature subdirectories). A feature with no QA transcript is itself a §107 PASS-bluff — it claims to work but has no auditable runtime evidence. Bot-driven automation (e.g. Herald's planned `qaherald` binary) MUST preserve full bidirectional communication threads as proof."

Every Herald feature that ships from a Qwen-driven session MUST carry a recorded end-to-end communication transcript plus attached materials committed under `docs/qa/<run-id>/`. Bidirectional transcripts only; bot-driven QA (the planned `qaherald` binary) preserves the full conversation thread. CI release gates refuse to tag any version whose feature-shipping commit lacks its `docs/qa/<run-id>/`. Canonical Helix authority: `<discovered>/Constitution.md` §11.4.83. Canonical Herald authority: `docs/guides/HERALD_CONSTITUTION.md` §107.x.

## §107.y — Working-Tree Quiescence Rule (operator mandate, 2026-05-22; cascades from Helix §11.4.84)

**Short tag:** `working-tree quiescence`.

**Forensic anchor — verbatim operator mandate (2026-05-22):**

> "no subagent commit may proceed while any concurrent mutation gate is in flight in the same checkout. Before `git add`, the committing agent MUST `grep` its own working tree for mutation markers (`MUTATED for paired`, `// always pass`, `return json.Marshal` shortcut paths, etc.). Any unexplained file in the staging area triggers ABORT."

Lesson (forensic — Herald-internal): commit `72e81ab` (logo fix, 2026-05-21) swept a `// always pass` JWT-bypass mutation residue into an unrelated commit; pushed to all four mirrors; fix `d5bd360` ("SECURITY FIX: restore commons_auth/middleware.go JWT verify") landed within the hour. The security-defect window is real; the rule is constitutional.

Qwen-agent-binding rule. Pre-`git add`: grep for mutation markers (`MUTATED for paired`, `// always pass`, `return json.Marshal` shortcuts, `// MUTATION` annotations, `_mutated_*` filenames, `.git/MUTATION_IN_PROGRESS` lockfile); cross-check `git status --porcelain` against declared scope; unaccounted entries ABORT. Active mutation gates serialise before unrelated commits; concurrent subagents use `git worktree add` or the lockfile. The prototype is `tests/test_wave4b_mutation_meta.sh` (`check_quiescence()` at line 92, assertion at line 197). Canonical Helix authority: `<discovered>/Constitution.md` §11.4.84. Canonical Herald authority: `docs/guides/HERALD_CONSTITUTION.md` §107.y.

## Inherited covenant restatements — Helix §11.4.85 / §11.4.87 / §11.4.88 (inherited per §11.4.35)

These three mandates are **inherited** from the HelixConstitution via parent-discovery (§11.4.35) and bind Qwen agents unchanged. This file **restates + cites**, it does NOT redefine or weaken. The literal anchors are required by the §11.4.87 `CM-COVENANT-114-87-PROPAGATION` pre-build gate, which asserts `11.4.85` / `11.4.87` / `11.4.88` appear in every consuming repo's CLAUDE.md / AGENTS.md / QWEN.md.

### §11.4.85 — Stress + Chaos Test Mandate (Helix, 2026-05-24)

No Qwen-driven Herald fix or improvement is done without full-automation **stress** (sustained / concurrent load) AND **chaos** (process-death / network-fault / input-corruption / resource-exhaustion / state-corruption injection) test suites, each PASS citing a captured-evidence artefact under `docs/qa/<run-id>/stress_chaos/` per §11.4.5 + §11.4.69. A happy-path-only PASS is a §11.4 / §107 PASS-bluff at the resilience layer. Canonical authority: HelixConstitution Constitution.md §11.4.85 (inherited per §11.4.35).

### §11.4.87 — Endless-loop autonomous work + zero-idle agent dispatch + anti-bluff testing (Helix, 2026-05-26)

When instructed to "continue in endless loop fully autonomously" (or equivalent), the Qwen agent MUST continue until ALL are simultaneously TRUE — Herald's loop checks `docs/Issues.md` Status-column (zero `In progress`/`Ready for testing`/`In testing`/`Reopened`), `docs/CONTINUATION.md` §3 "Active work" empty, TaskList reports no subagent mid-execution, and no in-flight push/build/sync. Dispatch background subagents for parallelisable non-contending work; idle ONLY while waiting on a result. Every closed item lands four-layer coverage (§11.4.4(b)) with real captured-evidence PASS; tests AND Challenges bound equally. Terminates only on all-clear, explicit operator `STOP`, a §12 host-session-safety demand, or a scheduled wake against a known-future-actionable signal. No `--idle-OK` / `--skip-endless-loop` / `--metadata-only-test-suffices` escape exists. Canonical authority: HelixConstitution Constitution.md §11.4.87 (inherited per §11.4.35).

### §11.4.88 — Background-push mandate (Helix, 2026-05-26)

Every Qwen-driven Herald commit flow MUST release the commit-lock (`.git/.commit_all.lock`) the instant `git commit` returns 0 — BEFORE any push — then spawn the push **detached** (`nohup ./push_all.sh ... &` + `disown`) with a per-remote flock so same-remote pushes serialise while GitHub / GitLab / GitFlic / GitVerse push in parallel. Backgrounded push failures land in `qa-results/push_failures/<TS>_<remote>.log` and the next autonomous-loop tick MUST surface them (silent push-failure is a §11.4 distribution-layer PASS-bluff). The ONLY synchronous-push escape is the explicit `--sync-push` flag for §11.4.41 force-push merge-first paths. Canonical authority: HelixConstitution Constitution.md §11.4.88 (inherited per §11.4.35).

### §11.4.98 — Full-Automation Anti-Bluff Mandate — Live tests MUST be re-runnable end-to-end without manual intervention (Helix, 2026-05-28)

Every Qwen-touched Herald test — unit/integration/e2e/Challenge/stress/chaos/live — MUST be fully self-driving end-to-end with NO human action during execution (operator typing a Telegram message, hand-triggering a webhook, clicking a UI, attaching a file, anything beyond test startup → PASS/FAIL report). A test requiring manual action during execution is **by definition a §11.4 PASS-bluff at the automation layer**, regardless of how thorough the manual run is — it cannot run in CI, cannot validate regressions between manual runs, and the human dependency masks drift. Single permissible exception: one-time credential bootstrap OUTSIDE test execution (`.env` from vault, `~/.bashrc` exports, OAuth at first install, MTProto session activation at first run — configuration, not test driving). Concrete Herald requirements: (1) no "operator MUST type a message" prompts in `tests/test_*.sh` or `_integration_test.go` — Qwen drives programmatically via MTProto user-account (Telegram) / real-user-API (Slack) / webhook fixture / in-process loopback; never human keystrokes; (2) no `claude --resume <UUID>` against the same session UUID the dev conductor is using (Herald 2026-05-28 lesson: silent exit -1 collision — use a dedicated test-only UUID); (3) no 60s human-response windows (§11.4.50 violation); (4) Qwen verifies PASS at `-count=3` consecutive automated runs with self-cleaning state; (5) every existing test classified COMPLIANT vs NON-COMPLIANT in the §11.4.98 audit (release-gate item); (6) no false-positive PASS — silent-skip-as-PASS forbidden, stale-evidence forbidden, §11.4.3 SKIP-with-reason is correct. Currently NON-COMPLIANT Herald tests scheduled for MTProto-driven rewrite: `TestSubscribe_LiveBotAPI`, `tests/test_wave6_live_loop.sh`, Wave 6.5 lifecycle scenarios. Canonical authority: HelixConstitution Constitution.md §11.4.98 (inherited per §11.4.35).

### §11.4.99 — Latest-Source Documentation Cross-Reference (Helix, 2026-05-28)

Every Qwen-touched Herald operator-facing instruction document (docs/requirements/blockers/, docs/guides/, README operator-action sections, troubleshooting, integration setup) MUST cross-reference against latest official online docs of every external service/library BEFORE commit. Misguidance-by-stale-docs = §11.4 PASS-bluff at documentation layer. Case study (Herald 2026-05-28): MTProto guide recommended VoIP + omitted `recover@telegram.org` pre-login email; both contradicted Telegram official docs + gotd/td maintainer; could have caused permanent ban. (A) Workflow: fetch LATEST official docs via WebFetch/MCP (NEVER training data); cross-reference each instruction; seek secondary authoritative sources for sparse official; cite source URLs + date in `## Sources verified` doc footer AND commit footer. (B) Negative findings documented. (C) STALE after 6 months (90 days for risk-classified: messengers/cloud/payment/AI/code-hosting/package-managers); re-verify before operator-authority citation, at vN.0.0, on breaking-change, on operator-error. (D) Risk-classified docs include explicit safety warnings vs latest policies. (E) Composes with §11.4.92 Pass 4 INDEPENDENT. (F) Inheritance per §11.4.35, literal `11.4.99`. (G) Enforcement: missing-footer BLOCKED; stale → §11.4.90 Obsolete `Reason=stale-documentation`. Canonical authority: HelixConstitution Constitution.md §11.4.99 (inherited per §11.4.35).

### §11.4.100 — Video color + visual-quality fidelity (Helix, 2026-05-28)

Universal mandate: every Qwen-touched video-playback test PASS MUST carry a captured-frame deep-analysis artefact proving rendered output matches host-extracted ground-truth source within tolerance — ΔE2000 + RGB/HSV histogram correlation + no pale/over-saturation/hue-shift + gamma/luma fidelity (BT.601/709/2020 + full-range-vs-limited-range explicit), sharpness (Laplacian-variance), aspect-ratio, FPS+speed, continuity (no freeze ≥1s SSIM>0.99, no drop-burst, no glitch, no obstruction-overlay per §11.4.5 OCR census). Metadata-only PASS ("file exists" / frames>0 / codec registered) forbidden; comparison harness MUST be golden-pair + bad-pair self-validated (no false-positive AND no false-negative). **Herald applicability:** non-applicable-but-cite — Herald has NO video-playback surface (pherald downloads video attachments as opaque sha256-blobs via HRD-005 attachment pipeline; no decode/render); mandate binds latently if Herald ever ships a video-rendering surface. Cascade-parallel: §11.4.96 ("Herald has no AOSP build, but the principle binds"). Inheritance per §11.4.35, literal `11.4.100`; required by upcoming `CM-COVENANT-114-100-PROPAGATION` pre-build gate. Canonical authority: HelixConstitution Constitution.md §11.4.100 (inherited per §11.4.35).

### §11.4.106 — Docs Chain documentation-sync mandate (Helix, 2026-05-29)

Universal mandate: the documentation-sync obligations (Issues_Summary §11.4.12, Fixed_Summary §11.4.53, Status/Status_Summary §11.4.45/§11.4.56, README doc-link + export §11.4.57/§11.4.59, universal md→html/pdf §11.4.65, roster fingerprint §11.4.86, workable-items DB↔MD §11.4.93/§11.4.95, CONTINUATION §12.10) are mechanized by **Docs Chain** — a content-hashed incremental DAG sync engine (`digital.vasic.docs_chain`) consumed BY REFERENCE via the constitution submodule (Phase-6 distribution operator-gated §11.4.66). Register chains in `.docs_chain/contexts/*.yaml`; `docs_chain sync` / `verify` **refuses to fake success** (tool-absent → rollback, exit 3) + **refuses silent merges** (both-dirty → conflict, exit 2) — §11.4.6 no-guessing + §11.4.50 determinism made mechanical. **Herald applicability: APPLICABLE (in progress)** — Herald's ~76 md docs + html/pdf/docx siblings migrate from the regen-all `scripts/export_docs.sh` to docs_chain `exec:` transforms preserving Herald's exact logo-CSS pandoc flags (byte-compatible) per `docs/research/docs_chain/HERALD_DOCS_CHAIN_PLAN.md`; planned `verify` drift-gate E146. Pending: Phase-4 CLI (landed 2026-05-31), exec-staging relative-asset contract (G6), binary-hash verify fix (dogfood finding). Cascade-parallel: §11.4.96. Inheritance per §11.4.35, literal anchor `11.4.106`; required by the `CM-COVENANT-114-106-PROPAGATION` pre-build gate. Canonical authority: HelixConstitution Constitution.md §11.4.106 (inherited per §11.4.35).

## Participant identity, attribution & notification-tagging (Herald §109 / contract `docs/design/PARTICIPANT_ATTRIBUTION.md` — inherited per §11.4.35)

**Operator mandate (2026-05-31).** Every messenger must relate each message to a **Participant** (a logical Subscriber/User — one person/agent, with a potentially DIFFERENT username on every messenger); workable items gain `created_by` + `assigned_to`; outbound notifications **@-tag** the right participant per a fixed rule matrix. The **single authoritative contract** every Qwen-driven stream codes against is [`docs/design/PARTICIPANT_ATTRIBUTION.md`](docs/design/PARTICIPANT_ATTRIBUTION.md). Restated (root definitions) in HelixConstitution `Constitution.md` / `CLAUDE.md` / `AGENTS.md` / `QWEN.md`, inherited here per §11.4.35 — restated + cited, never redefined or weakened.

- **Identity model.** PG `subscribers` (canonical messenger-neutral `handle`, `display_name`, `kind ∈ {human, agent, service}`) + `subscriber_aliases` (`subscriber_id`, `channel`, `channel_user_id`, **+ NEW `username TEXT`** — per-channel `@handle` for tagging, distinct from `channel_user_id`). Canonical handle (stored in `created_by`/`assigned_to`) ∈ {`Claude` (reserved system-agent sentinel, NEVER tagged), a human's canonical handle (defaults to their Telegram `@username`; messenger-neutral)}.
- **Operator env var.** `HERALD_TGRAM_OPERATOR_USERNAME` (e.g. `@milos85vasic`), generalizing to `HERALD_<CHANNEL>_OPERATOR_USERNAME` — env var, NOT a DB flag. Operator's canonical handle = that env value.
- **Attribution.** `created_by` = `OperatorHandle()` (Claude Code CLI prompt) / `"Claude"` (System/Claude-detected) / sender's resolved canonical handle (received through Herald). `assigned_to` defaults to `OperatorHandle()`, overridable.
- **Tagging matrix.** @-mention `assigned_to` if human AND ≠ Operator; `created_by` if human AND ≠ Operator AND ≠ `"Claude"`. `"Claude"` and Operator NEVER tagged. De-dup, resolve to each channel's `@username` (skip if no alias). Every layer ships unit + integration + E2E + full-automation tests with real captured evidence under `docs/qa/<run-id>/` — matrix proven by per-cell truth-table + one-cell-flip mutation must FAIL, plus a NEGATIVE Operator-not-tagged case (§107 / §11.4 anti-bluff). Canonical authority: HelixConstitution root §participant-attribution (inherited per §11.4.35); Herald contract `docs/design/PARTICIPANT_ATTRIBUTION.md`.

## Intent recognition & clarification (Herald §110 / contract `docs/design/INTENT_RECOGNITION.md` — inherited from HelixConstitution §11.4.105 per §11.4.35)

**Operator mandate (2026-05-31).** Subscribers must NOT need to know command syntax — no `COMMAND:` prefix, no fixed grammar. A subscriber sends a clear natural-language message and **the System determines the intent**. The **single authoritative contract** every Qwen-driven stream codes against is [`docs/design/INTENT_RECOGNITION.md`](docs/design/INTENT_RECOGNITION.md). Restated (root definitions) in HelixConstitution `Constitution.md` / `CLAUDE.md` / `AGENTS.md` / `QWEN.md` as §11.4.105 (the root-§ being added on the constitution stream), inherited here per §11.4.35 — restated + cited, never redefined or weakened.

- **Three-tier resolution (first tier that succeeds wins).** **Tier 1 — command recognition:** a deterministic `CommandRecognizer` (`pherald/internal/inbound`) maps a clear imperative to a structured action WITHOUT an LLM round-trip (fast-path, no prefix); CONSERVATIVE — only a confident match fast-paths, otherwise it returns "no match" and defers to Tier 2. **Tier 2 — intent inference:** the Claude Code dispatch (the LLM) infers intent and returns a `<<<HERALD-REPLY>>>` action; the `<<<HERALD-DISPATCH-v1>>>` envelope INSTRUCTS the LLM to recognize Herald's command set, map natural language to the right action, and NEVER guess. **Tier 3 — clarify (fallback):** when neither a command nor a confident intent can be determined, `action="clarify"` — REPLY to the original message, TAG the sender (`@username`, via the §11.4.104 `IdentityResolver`), and ask a precise clarifying question naming the candidate intents.
- **Never guess, never ignore.** A wrong action is worse than a clarifying question (§11.4.6 no-guessing); the subscriber is never silently dropped and never has to learn syntax. Every tier ships unit + integration + E2E + full-automation tests with real captured evidence under `docs/qa/<run-id>/` — a Tier-1 truth-table including conservative negatives that MUST fall through to "no match", a Tier-3 E2E asserting the reply body is EXACTLY `@<sender> <specific question>`, and a paired §1.1 mutation (break the confidence guard or drop the clarify tag) that must FAIL (§107 / §11.4 anti-bluff). Canonical authority: HelixConstitution Constitution.md §11.4.105 (inherited per §11.4.35); Herald contract `docs/design/INTENT_RECOGNITION.md`.

## Qwen-specific notes

Qwen agents follow the same disciplines documented for Claude Code:
- Run the anti-bluff battery (`scripts/audit_antibluff.sh`, `scripts/e2e_bluff_hunt.sh`, `tests/test_constitution_inheritance.sh`, `tests/test_wave2_mutation_meta.sh`, `tests/test_wave3_mutation_meta.sh`) before declaring any task done.
- Use the subagent-driven pattern (Universal §11.4.70) for non-trivial work.
- Never commit secrets — see `docs/guides/OPERATOR_CREDENTIALS.md` and the wizard at `pherald wizard credentials`.
- Multi-mirror push via `git push origin <branch>` (origin fans out to github + gitlab + gitflic + gitverse).

For anything not covered here, defer to `CLAUDE.md` — its rules apply verbatim to Qwen sessions.
