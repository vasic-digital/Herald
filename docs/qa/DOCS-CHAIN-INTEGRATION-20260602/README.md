# QA Evidence — Herald ⇄ Docs Chain integration completion (2026-06-02)

**Run-id:** `DOCS-CHAIN-INTEGRATION-20260602`
**Feature:** Full integration + use of the `docs_chain` engine by Herald per §11.4.106 (HelixConstitution) / §108.p (HERALD_CONSTITUTION).
**Anti-bluff covenant:** §107 / Helix §11.4 — every PASS below carries real captured runtime evidence against the **real built `docs_chain` binary**, never metadata-only.

## What this run proves

1. **The docs_chain engine is complete + working** (sibling repo `~/Projects/docs_chain`, committed `80d3d8d`, pushed to `origin`/github). The previously-PLANNED Phase-4/5 capabilities are now IMPLEMENTED and exercised against the real binary:
   - fsnotify `watch` daemon (full-automation test: a real source edit triggers a real re-sync),
   - bidirectional pure-Go `md-to-sqlite` / `sqlite-to-md` builtins (modernc.org/sqlite, no cgo),
   - `colorize-html` (§11.4.23) via x/net/html,
   - a latent **real bug fixed**: the old SQLite path hashed schema only (`sqlite_master`), so row-level edits never tripped `verify`; a full schema+rows dump now catches single-cell drift.
   - Evidence: `03-docs_chain-engine-e2e-GREEN.txt` (`scripts/e2e.sh` → **20 PASS / 0 FAIL / 0 SKIP**, drives the real binary across every subcommand + the documented exit-code contract).

2. **Herald's corpus is wired and the drift-gate is real.** `doctor --all` parses `herald-docs` (263 nodes / 197 edges = ~66 markdown × html/pdf/docx) + `herald-pilot` (4/3); transforms present.

3. **The drift-gate caught REAL drift, then fixed it** (the core anti-bluff result):
   - `01-fullcorpus-verify-STALE-exit1.log` — full-corpus `verify` exited **1** with 13 STALE docx/pdf siblings.
   - This was proven to be *real source drift* (recent markdown edits committed without regenerating siblings — the ad-hoc `export_docs.sh` has no drift detection), **NOT** transform non-determinism, by a byte-identical double-derive:
     ```
     md_to_docx.sh CLAUDE.md  → derive1 == derive2  (deterministic transform)
     derive1 != committed CLAUDE.docx               (committed sibling was stale)
     ```
   - `02-sync-regenerate-exit0.log` — `docs_chain sync herald-docs` regenerated the stale siblings (exit 0). Net git change = **exactly the 13 stale files**; the other ~187 nodes re-derived byte-identical (zero spurious churn — a second proof of transform determinism).
   - `04-postsync-verify-exit0.log` — post-sync `verify` now exits **0** (`herald-docs  in-sync`), corpus fully reconciled.

## The §11.4.106 payoff

docs_chain mechanically caught documentation drift that the human-driven `export_docs.sh` flow missed. The routine doc-export flow for Herald is now `docs_chain sync herald-docs` + `docs_chain verify herald-docs` (exit-0 gate), not ad-hoc regeneration.

## Files

| File | What it proves |
|------|----------------|
| `01-fullcorpus-verify-STALE-exit1.log` | drift-gate detects 13 real stale siblings (exit 1) |
| `02-sync-regenerate-exit0.log` | sync regenerates exactly the stale set (exit 0) |
| `03-docs_chain-engine-e2e-GREEN.txt` | real-binary engine e2e 20/0/0 GREEN |
| `04-postsync-verify-exit0.log` | corpus back in sync, verify exit 0 |

**Note on `qa-results/` vs `docs/qa/`:** Herald gitignores `qa-results/` (transient run logs). Per §107.x the auditable committed evidence lives here under `docs/qa/<run-id>/` — these transcripts are copies of the real run logs.
