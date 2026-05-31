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
`(input_path, output_path)` — confirmed by the conflict-demo's `cp.sh`. Builtins
shipped: `pandoc-html`, `weasyprint-pdf`, **`pandoc-docx`**, `members-fingerprint`.

### 1.1 Verify-gate defect found by dogfooding (2026-05-31) — fixed in docs_chain

Running the engine on docs_chain's **own** 6 docs (a `self-docs` context,
markdown → html/pdf/docx) surfaced a real defect, captured live:

- After `sync`, an immediate read-only `verify` reported **all `.docx` nodes and
  some `.pdf` nodes STALE (exit 1)** while **every `.html` node verified clean**.
- Yet two consecutive syncs of the same source produced **byte-identical** output
  (`cmp` identical) — so the transforms are deterministic; the bug was **inconsistent
  hashing of binary node-kinds between the sync-record path and the verify-check
  path** (a text-normalizing content hasher applied to binary docx/pdf bytes in one
  path but not the other). html, being genuine text, normalized consistently and
  stayed stable.

**Impact:** the `verify` drift-gate — the single most valuable thing docs_chain
adds over `export_docs.sh` — is unusable for docx/pdf until fixed (every CI run
would false-positive STALE with no source change). Fix **in flight** in docs_chain
as of 2026-05-31 (binary kinds to be hashed by raw bytes consistently in both
paths) + a pinned regression test; acceptance proof = dogfood `sync` then `verify`
→ exit 0 for all kinds at `-count=3`.
**This is why §5 step 4's "zero-diff" proof must run `verify` immediately after
`sync` and assert exit 0 — not merely that files were produced.**

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

> docs_chain ships a `pandoc-docx` builtin, but it (like `pandoc-html`) does NOT
> carry Herald's exact flags / logo CSS, so the exec-wrapper decision stands for
> byte-compatibility. A later docs_chain enhancement could add flag-parameterized
> builtins (`pandoc-html5-css`, …) to retire the wrappers — a nicety, not a Herald
> blocker.

### 3.1 Open question to resolve before authoring the wrappers (no-guessing, §11.4.6)

docs_chain stages `exec:` transforms to **temp** input/output files. Herald's html
export resolves a **relative** `--css=…/assets/logo/print.css` and the markdown
embeds **relative** `<img src="…/assets/logo/…">`. From a temp staging dir those
relative paths will NOT resolve, breaking the logo CSS and image embedding. Before
authoring `scripts/docs_chain/*.sh`, the exact exec-adapter contract must be read
from the **final** docs_chain source (does it pass the original node path? set cwd
to project root? expose `$DOCS_CHAIN_ROOT`?). The wrapper must reconstruct an
absolute `print.css` path and run pandoc with a cwd/`--resource-path` that resolves
the in-tree assets — verified by a byte-diff self-test against a committed sibling.
This is tracked as **gap G6** (§6) and is the gating unknown for §5 step 2.

**G6 RESOLVED (2026-05-31).** Read from final docs_chain source: exec transforms run with
`cmd.Dir = projectRoot` (constant for both sync AND verify — `internal/runner/runner.go:350`),
argv `<in_tmp> <out_tmp> [args…]`, and during `verify` the in/out temps are staged to a throwaway
temp dir (`runner.go:414`). So a wrapper must NOT derive the doc's location from the staged temp
(it differs between sync and verify). Fix: pass the doc's **real directory** as an explicit
transform `arg` (TransformSpec.Args, appended after the IO paths), resolve in/out to absolute, and
`cd "$root/$realdir"` so the relative `--css`/`<img>` resolution is IDENTICAL in sync and verify →
deterministic, verify-stable output. Implemented in `scripts/docs_chain/{md_to_html,md_to_pdf,md_to_docx}.sh`.
**Fleet-rollout note:** since `realdir` is per-doc, the generated fleet context uses per-directory
transform variants (one `md2html_<dirslug>` set per distinct doc directory, ~10 total).

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

> **§5 STATUS — FLEET ROLLOUT DONE (2026-05-31).** Steps 1–5 complete; step 6's
> guidance flip is captured below. The full corpus is wired and proven:
> `.docs_chain/contexts/herald-docs.yaml` (generated by
> `scripts/docs_chain/gen_context.py`) covers **66 docs / 263 nodes / 197 edges /
> 33 per-directory transforms** across 11 directories. `doctor` exit 0; cold
> `sync` exit 0; `verify` exit 0 ×3 (deterministic). The bulk `export_docs.sh`
> sweep is **retained as the wrapper implementation reference + manual fallback**
> — NOT deleted — but the routine doc-commit flow now uses `docs_chain sync`
> (incremental, content-hash, verify-gated). Evidence:
> `docs/qa/DOCS-CHAIN-FLEET-20260531T1301Z/`.

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
- **G5** — *resolved* by the Phase-4 CLI (the only consumer interface), which
  landed 2026-05-31 (`docs_chain` branch `phase4-cli-config` → main); §5.1 unblocked.
- **G6 — RESOLVED 2026-05-31** (§3.1): exec transforms get staged temps + `cwd=projectRoot`;
  the wrapper takes the doc's real dir as an explicit arg and `cd`s there → deterministic,
  verify-stable. Implemented + pilot-proven.
- **Verify-binary-hash — FIXED 2026-05-31** (§1.1, docs_chain `59fe323`): binary kinds were
  hashed via a text-normalizing hasher + pandoc/weasyprint embedded timestamps; fixed with a
  RawByteHasher + per-node-kind hashing + SOURCE_DATE_EPOCH + weasyprint `--base-url`, with a
  pinned regression test. `verify` exit 0 after sync for docx/pdf, proven.

## 7. PILOT LANDED (2026-05-31) — the wiring works end-to-end

`docs/guides/COMMONS_WATCH.md` wired via `.docs_chain/contexts/herald-pilot.yaml` + the three
`scripts/docs_chain/*.sh` exec wrappers. Captured evidence: `docs/qa/DOCS-CHAIN-PILOT-20260531T0757Z/pilot-verify.txt`.
Results: `doctor` exit 0; cold `sync` exit 0; `verify` exit 0 ×3 (stable); negative control (edit
source → `verify` STALE exit 1 → restore → exit 0); **html byte-IDENTICAL** to the committed
`export_docs.sh` output; docx/pdf differ by only 3–6 bytes (the SOURCE_DATE_EPOCH determinism
improvement); logo print.css + image preserved in html, rendered in pdf; docx is logo-less — but
so is the committed docx (pandoc's docx writer never embedded the raw-HTML logo; **status quo, not
a regression**). Gated by `e2e_bluff_hunt.sh` **E146** (SKIP-with-reason when the binary/pandoc/
weasyprint are absent). **Remaining: fleet rollout** — generate the full-corpus context with
per-directory transform variants (§3.1 note), one-time re-render to the deterministic baseline,
then retire the bulk `export_docs.sh` sweep.

## 8. FLEET ROLLOUT LANDED (2026-05-31) — full corpus wired + proven

The pilot generalized to the whole documentation corpus.

**Scope.** Every tracked `.md` with a committed `.html` sibling, EXCLUDING
`submodules/`, `containers/`, `constitutable/`, `docs/herald/diary/`,
`docs/superpowers/`, and `docs/specs/mvp/archive/` (frozen historical specs).
Enumerated by `git ls-files` → **66 docs across 11 directories** (docs/guides 19,
docs/research/protocols 14, docs/guides/messengers 9, docs 8, docs/guides/dispatchers 6,
docs/catalogue-checks 3, repo-root `.` 3, docs/specs/mvp 1, docs/requirements/blockers 1,
docs/design 1, docs/audits 1).

**Generated, not hand-authored.** `scripts/docs_chain/gen_context.py` writes
`.docs_chain/contexts/herald-docs.yaml` deterministically from `git ls-files`
(re-running is byte-identical). Per doc: 1 markdown node + one
{html,pdf,docx} node **per extension that is already committed** (so a doc with
only html+pdf committed — e.g. `docs/guides/CONSTITUTION_INHERITANCE` — gets NO
docx leg; the rollout rebaselines existing siblings, it never invents a new
committed artefact). Result: **263 nodes, 197 edges, 33 transforms** (one
`md2html_<dirslug>`/`md2pdf_<dirslug>`/`md2docx_<dirslug>` triple per distinct
directory, each passing that directory as the exec `args:` per the G6 contract §3.1).

**Proof (captured: `docs/qa/DOCS-CHAIN-FLEET-20260531T1301Z/`).**
`doctor` exit 0 (263 nodes, 197 edges, all tools present); cold `sync`
(`rm -f state.json` first) exit 0, all 197 nodes committed; `verify` exit 0 **×3
consecutive** (determinism). Per-doc safety table (`per-doc-safety-table.tsv`):

| Check | Result |
|---|---|
| HTML byte-IDENTICAL to committed `export_docs.sh` output | **66 / 66** (zero drift) |
| HTML CHANGED | 0 |
| logo `<img …herald_logo_square*.png>` present in html | 66 / 66 |
| `print.css` referenced in html | 66 / 66 |
| PDF valid (`file`=PDF document, >1 KB) | 66 / 66 |
| DOCX valid zip (`word/document.xml` present + text) | 65 / 65 wired (1 NA: CONSTITUTION_INHERITANCE has no committed docx) |

**One-time deterministic rebaseline.** html: 0 changed (byte-identical). pdf: 20
changed (byte-delta −1..−85 — SOURCE_DATE_EPOCH + metadata determinism). docx: 64
changed (byte-delta −4..−6 — SOURCE_DATE_EPOCH timestamp normalization). All
deltas are tiny + benign (no content change); spot-checked `docs/guides/PHERALD.docx`
remains a valid Word doc with the expected text. The docx logo-less state is the
status quo (pandoc's docx writer never embedded the raw-HTML `<div><img>` logo —
not a regression, identical to the pre-rollout committed docx).

**Gated by `e2e_bluff_hunt.sh` E147** (additive — E146 still guards the pilot
COMMONS_WATCH): cold `sync` → `verify` in-sync (exit 0) over the full corpus, then
`git checkout` restores the siblings so the gate leaves the tree clean.
SKIP-with-reason (§11.4.3) when the binary/pandoc/weasyprint are absent.

---

## Sources verified 2026-05-31

- Real binary run (§1) — `~/Projects/docs_chain` Phase-4 CLI, captured live.
- `~/Projects/docs_chain/docs/{CONSTITUTION_INTEGRATION,USE_CASE_CATALOGUE,ARCHITECTURE,CONFIG_SCHEMA,USER_GUIDE}.md` (revision 2, 2026-05-29).
- Herald `scripts/export_docs.sh` (pandoc flag inventory, §2).
- `git ls-files` sibling census (§2).
