<div align="center">

<img src="../../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Herald ⇄ Docs Chain — Integration Plan (investigation deliverable)

| Field | Value |
|---|---|
| Created | 2026-05-31 |
| Status | active — investigation complete; wiring sequenced behind the docs_chain Phase-4 CLI landing |
| Authority | operator mandate 2026-05-31 ("incorporate complete use of Docs Chain… investigate where besides the mandatory rules docs_chain can be used… increase efficiency to the maximum") |
| Upstream | `~/Projects/docs_chain` (`https://github.com/vasic-digital/docs_chain`) — cloned flat per CONST-051 |
| Constitution anchor | HelixConstitution §11.4.106 (Docs Chain mandate) + the §11.4.x family it mechanizes (§11.4.12/53/45/56/57/59/60/65/86/93/95, §12.10) |

> **Anti-bluff note.** Every capability below was verified against the **real
> built binary** (`go build ./cmd/docs_chain` from `~/Projects/docs_chain`), not
> from training data or docs prose. The captured run is reproduced in §1.

---

## 1. Verified CLI capability (real binary, 2026-05-31)

The Phase-4 CLI was exercised end-to-end against a live `md → html → pdf`
fixture. Observed, real output:

- `doctor <ctx>` → `parse + graph: OK (3 nodes, 2 edges) / transforms: OK (all required tools present)`, exit 0.
- `graph <ctx>` → Kahn topo order + edge list, exit 0.
- `sync <ctx>` → `applied: committed [guide_html guide_pdf]`, real `GUIDE.pdf` (PDF 1.7, 6890 bytes) produced; evidence written to `qa-results/docs_chain/<TS>Z/`.
- `sync` run 2 → `in-sync (no changes)` (content-hash **early cutoff**, not mtime).
- `verify <ctx>` in-sync → exit 0; after editing the source → `STALE: [guide_html]`, exit 1.
- **Tool-absent** (PATH stripped of pandoc) → `ROLLED-BACK: SKIP (tool absent)… refusing to fake success`, exit 3, **no fake html created** (§11.4 honest SKIP).
- **Both-dirty sync** → `CONFLICT… refusing silent merge (no writes)`, exit 2, files unchanged (§11.4.6 no-guessing).
- `state.json` persisted per-context content hashes; full suite green at `-race` and `-count=3` (§11.4.98 determinism).

**Authoritative context YAML schema** (copied verbatim from the working demo):

```yaml
context: guide
description: One markdown doc with html + pdf exports
nodes:
  guide_md:   { kind: markdown, path: docs/GUIDE.md }
  guide_html: { kind: html,     path: docs/GUIDE.html }
  guide_pdf:  { kind: pdf,      path: docs/GUIDE.pdf }
edges:
  - { type: derive-from, from: guide_md,   to: guide_html, transform: md-to-html }
  - { type: derive-from, from: guide_html, to: guide_pdf,  transform: html-to-pdf }
transforms:
  md-to-html:  { builtin: pandoc-html }
  html-to-pdf: { builtin: weasyprint-pdf }
```

`exec:` transforms are supported (`{ exec: "./script.sh" }`) and receive
`(input_path, output_path)` — confirmed by the conflict-demo's `cp.sh`.

---

## 2. Herald's documentation universe (the migration target)

`git ls-files` of tracked siblings (excluding `submodules/`, `containers/`,
`constitutable/`, `docs/herald/diary/`):

| Sibling | Count |
|---|---|
| `.html` | 75 |
| `.pdf`  | 76 |
| `.docx` | 72 |

~72 source `.md` files carry the full `[html, pdf, docx]` triple. The pipeline
that produces them today is `scripts/export_docs.sh`, which **regenerates
everything** (or the subset passed as args) via `pandoc`:

- HTML: `pandoc -f gfm -t html5 --standalone --css=<rel>/assets/logo/print.css`
- PDF : `pandoc --pdf-engine=weasyprint` (resolves relative `<img>` srcs)
- DOCX: `pandoc` (embeds images directly)

---

## 3. The byte-compatibility constraint (why builtins are the WRONG choice here)

docs_chain's generic builtins do **not** reproduce Herald's exact pandoc
invocation: the demo's `pandoc-html` builtin emitted XHTML with no Herald
`print.css` and no logo styling. Adopting builtins wholesale would silently
restyle all ~223 committed sibling exports and **drop the centered-logo branding
header CSS** — a §11.4 regression at the documentation layer, dressed up as a
"migration".

**Decision: use `exec:` transforms that wrap Herald's exact `export_docs.sh`
pandoc commands**, factored into three tiny single-purpose wrapper scripts
(`scripts/docs_chain/md_to_html.sh`, `…/html_to_pdf.sh`, `…/md_to_docx.sh`).
This:

1. preserves byte-identical, logo-styled output — **zero regen-drift** across 223 files;
2. **closes gap G1 (no docx builtin)** with no docs_chain change — the docx leg is just another `exec` transform;
3. keeps docs_chain's real value: content-hash **incremental** recompute (only-changed, vs `export_docs.sh`'s regen-all), `verify` as a CI **drift-gate**, and atomic-commit / conflict safety.

> A later, optional docs_chain enhancement could add `pandoc-html5-css` /
> `pandoc-docx` builtins parameterized by flags, retiring the wrappers. Not
> required for complete adoption; tracked as a docs_chain-side nicety, not a
> Herald blocker.

---

## 4. Where docs_chain helps Herald *beyond* the mandatory sync rules

Investigation result (the operator's explicit question). docs_chain is useful in
Herald wherever a derived artefact must track a source:

| Use | docs_chain mechanism | Replaces / improves | Status |
|---|---|---|---|
| Per-doc `md → {html,pdf,docx}` | `derive-from` edges + `exec` transforms | `export_docs.sh` regen-all → **incremental** | wireable now (§5) |
| `Issues.md → Issues_Summary.md` (§11.4.12) | `derive-from` + early-cutoff | hand-sync drift | needs a summary `exec` generator (gap G3) |
| `Fixed.md → Fixed_Summary.md` (§11.4.53) | same | same | gap G3 |
| `Status.md → Status_Summary.md` (§11.4.56) | same | same | gap G3 |
| README doc-link section (§11.4.57) | multi-input `derive-from` | manual edits | gap G3 |
| `CONTINUATION.md` exports (§12.10) | `derive-from` | `export_docs.sh` | wireable now |
| Workable-items `DB ↔ MD` (§11.4.93/95) | bidirectional `sync` edge | manual regen | blocked on DB materialization (gap G2) |
| Drift-gate in `e2e_bluff_hunt.sh` | `verify --all` exit 1 on stale | no current gate | wire as **E146** (§5) |

**Net efficiency win:** today every doc touch triggers a full `export_docs.sh`
sweep (all 223 siblings re-rendered). With docs_chain, `sync --all` recomputes
**only the changed source's** descendants (content-hash cutoff), and `verify
--all` becomes a fast, deterministic CI assertion that no sibling is stale —
turning a slow blind regen into an incremental, gated, evidence-emitting step.

---

## 5. Sequenced rollout (each step gated on real evidence)

1. **[blocked → unblocks on CLI landing]** docs_chain Phase-4 CLI committed +
   pushed from `~/Projects/docs_chain`; build `docs_chain` binary, record its
   commit in `helix-deps`-style pin.
2. Add `scripts/docs_chain/{md_to_html,html_to_pdf,md_to_docx}.sh` wrappers
   (verbatim `export_docs.sh` pandoc flags) + a self-test proving byte-identical
   output vs the current committed sibling for one representative doc.
3. Add `.docs_chain/contexts/herald-docs.yaml` covering the guide/root/tracker
   doc set with `md→html→pdf` + `md→docx` edges via the wrappers; `.gitignore`
   `state.json` + `*.docs_chain.tmp`.
4. `docs_chain doctor --all` + `sync --all`; diff produced siblings against the
   committed ones → must be **zero diff** (no-regression proof, captured under
   `docs/qa/DOCS-CHAIN-<run-id>/`).
5. Add **E146** to `scripts/e2e_bluff_hunt.sh`: `docs_chain verify --all` exit 0
   on a clean tree, exit 1 after a deliberate source edit (paired positive +
   negative, §11.4.2). SKIP-with-reason until the binary is present on PATH.
6. Retire the bulk `export_docs.sh` sweep from the routine doc-commit flow
   (keep the script as the wrapper implementation + a fallback); update CLAUDE.md
   "Documentation artefacts" guidance to point at `docs_chain sync`.

---

## 6. Honest open gaps (NOT yet solved — no bluff)

- **G2** — workable-items DB not materialized in-repo yet (§11.4.95 / HRD-131
  Phase-2 deferred); the `DB ↔ MD` sync edge cannot be wired until it lands.
- **G3** — no `Issues_Summary` / `Fixed_Summary` / `Status_Summary` / README
  doc-link **generator** exists as an `exec` script; the summaries are currently
  hand-maintained. docs_chain can *gate* their freshness only once a deterministic
  generator exists.
- **G4** — `scripts/branding_inject_logo.py` **mutates the source `.md`** (adds
  the logo header). That is an in-place authoring step, not a derive edge; it must
  run **before** docs_chain sees the source, or be modeled as a pre-sync hook —
  not as a docs_chain node (which would fight the atomic-commit model).
- **G5** — *resolved by the in-flight Phase-4 CLI* (the only consumer interface);
  this plan's §5.1 unblocks the moment it lands.

---

## Sources verified 2026-05-31

- Real binary run (§1) — `~/Projects/docs_chain` Phase-4 CLI, captured live.
- `~/Projects/docs_chain/docs/{CONSTITUTION_INTEGRATION,USE_CASE_CATALOGUE,ARCHITECTURE,CONFIG_SCHEMA,USER_GUIDE}.md` (revision 2, 2026-05-29).
- Herald `scripts/export_docs.sh` (pandoc flag inventory, §2).
- `git ls-files` sibling census (§2).
