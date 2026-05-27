<div align="center">

<img src="../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Herald — §11.4.93 SQLite-SSoT Phase 2 Adoption Assessment (HRD-131)

| Field | Value |
|---|---|
| Date | 2026-05-27 |
| Scope | §11.4.93 Phase-2 adoption assessment — can the constitution's `workable-items` Go binary generate `docs/.workable_items.db` from Herald's `docs/Issues.md` + `docs/Fixed.md`? |
| Mode | READ-ONLY assessment. No DB generated, no commit, no file edited except this doc. |
| HRD | HRD-131 (SQLite SSoT migration — Phase 1 filed/scope-locked; this assesses Phase 2). |
| Binary under assessment | `/Users/milosvasic/Projects/constitution/scripts/workable-items/` (sibling repo, discovered per parent-discovery; NOT a Herald submodule) |
| Constitution commit | `3c9c4e9` — "§11.4.94 … + §LA Phase 2 workable-items Go-binary scaffold (User mandates, 2026-05-27)" |
| Canonical authority | HelixConstitution `Constitution.md` §11.4.93 (inherited per §11.4.35); §11.4.74 (submodule-catalogue-first — do NOT reimplement) |

## 1. Executive summary

The constitution `workable-items` binary is a **Phase-2 scaffold, not a functional tool**. Every subcommand
(`sync md-to-db`, `sync db-to-md`, `diff`, `validate`, `add`, `close`, `report`) is hard-stubbed to print a
"not yet implemented" line and exit `2`. There is **no Markdown parser, no SQLite access layer, no renderer, no
tests** — the `pkg/itemsdb/`, `pkg/mdparser/`, `pkg/mdrender/`, and `tests/` directories named in the README **do
not exist on disk**. Only `cmd/workable-items/main.go` (115 lines of flag dispatch + stub functions), `schema.sql`,
`go.mod`, and `README.md` are present.

**Herald cannot adopt the binary "as-is" for Phase 2**, because there is no implemented `sync md-to-db` to run.
The README itself states this explicitly: Phase 2 = "Go binary scaffold + DDL committed (this PWU)"; the actual
parse-and-upsert lands in **Phase 3** (`sync md-to-db`) and the regen in **Phase 4** (`sync db-to-md`).

**Recommendation: `blocked-on-binary-completion` → the unblocking work is a constitution-side task (implement
Phase 3 `sync md-to-db` parser).** Per §11.4.74, Herald must NOT write a Herald-local parser. A secondary
finding is that even once Phase 3 lands, the parser must be **format-aware**: Herald uses a *table-row* tracker
format, and the constitution's `atm_id`-keyed schema + the `ATM-NNN` heading-per-item convention referenced in the
schema comments do **not** match Herald's `HRD-NNN` table shape. See §4–§5.

## 2. Binary-readiness matrix

`go build ./cmd/workable-items` succeeds (Go 1.21 module, single dep `github.com/mattn/go-sqlite3 v1.14.22`).
Built to a scratch path `/tmp/wi-scratch-build`; **no DB and no file under Herald `docs/` was written.** Runtime
behaviour of every subcommand:

| Subcommand | Declared in usage | Implemented? | Runtime behaviour | Works on Herald's format? |
|---|---|---|---|---|
| `sync md-to-db` | yes | **NO** — stub | prints "Phase 3 not yet implemented …", exit `2` | N/A — never parses |
| `sync db-to-md` | yes | **NO** — stub | prints "Phase 4 not yet implemented …", exit `2` | N/A — never renders |
| `diff` | yes | **NO** — stub | prints "Phase 3/4 dependent …", exit `2` | N/A |
| `validate` | yes | **NO** — stub | prints "Phase 2 scaffold — validate not yet wired to schema.sql", exit `2` | N/A |
| `add` | yes | **NO** — stub | prints "Phase 3 dependent …", exit `2` | N/A |
| `close` | yes | **NO** — stub | prints "Phase 3 dependent …", exit `2` | N/A |
| `report` | yes | **NO** — stub | prints "Phase 3 dependent …", exit `2` | N/A |
| `-h` / `--help` / `help` | yes | yes | prints usage, exit `0` | N/A |

**Verdict: 0 of 7 functional subcommands are implemented.** The binary is a complete-and-buildable *scaffold*
(it dispatches argv correctly and prints honest not-impl messages — itself anti-bluff-correct), but it performs
zero work. There is no `schema.sql` execution path: `main.go` never imports `go-sqlite3` nor opens a DB; the
`go.mod` dependency is declared for the *future* Phase-3 access layer but is currently unused by `main.go`.

Confirmed absences on disk (`find` of the directory tree):

- `cmd/workable-items/main.go` — present (115 lines, stub dispatcher).
- `schema.sql` — present (DDL only; 6 tables).
- `go.mod`, `README.md` — present.
- `pkg/itemsdb/`, `pkg/mdparser/`, `pkg/mdrender/` — **ABSENT** (README "Phase 2/3/4 scaffold" rows describe planned, not existing, packages).
- `tests/` — **ABSENT** (README anti-bluff coverage table is a *plan*, not landed tests).

## 3. Format-compatibility analysis

### 3.1 Herald's actual tracker format (`docs/Issues.md` + `docs/Fixed.md`)

Herald uses a **Markdown TABLE-per-section** format, NOT a heading-per-item format. The header is a metadata
key/value table; the item registry is a pipe table.

`docs/Issues.md` — `## Open` section table columns:

```
| ID | Type | Status | Criticality | Title | Opened | Last update | References |
```

Example row: `| HRD-015 | task | open | low | Inheritance gate I8 invariants … | 2026-05-20 | 2026-05-20 | spec V3 §40 … ; Catalogue-Check: no-match (2026-05-20) |`

`docs/Fixed.md` — `## Recently fixed` section table has a **different** column set:

```
| ID | Type | Criticality | Title | Closed | Commit | Reference |
```

Key observations about Herald's actual data:

- Item IDs are `HRD-NNN` (e.g. `HRD-015`, `HRD-132`), not `ATM-NNN`.
- `Type` vocabulary observed: `task`, `bug` (lowercase).
- `Status` vocabulary observed (Issues.md): `open`, `in_progress` (lowercase, underscore) — these are **Herald's
  own** status strings.
- `Criticality` column (`low`/`middle`/`high`) — a **distinct named column**, not folded into status.
- `References` cell carries a free-text blob including the load-bearing `Catalogue-Check:` line (§11.4.74).
- Fixed.md additionally has `Closed` (date) and `Commit` (sha) columns and **drops** the `Status`, `Opened`, and
  `Last update` columns that Issues.md has.

### 3.2 The constitution parser's expected format

The Phase-3 parser does not exist, so its expected format can only be read from the schema + README intent:

- **Identifier:** `schema.sql` keys `items` on `atm_id TEXT PRIMARY KEY` with the comment "§11.4.54 ATM-NNN
  ticket identifier". The schema is written for the **`ATM-NNN`** convention (the rockchip/ATM project the
  constitution binary was built for), not `HRD-NNN`.
- **Title source:** `title` column comment is "Heading line text (full H2 heading including code prefix per
  §11.4.54)" — i.e. the parser is designed to read a **heading-per-item** layout (`## ATM-NNN …`), NOT pipe-table
  rows.
- **Type closed-set:** `CHECK (type IN ('Bug', 'Feature', 'Task'))` — **Title-Case**.
- **Status closed-set (8+ values):** `'Queued', 'In progress', 'Ready for testing', 'In testing', 'Reopened',
  'Operator-blocked', 'Fixed (→ Fixed.md)', 'Implemented (→ Fixed.md)', 'Completed (→ Fixed.md)', 'Obsolete (→
  Fixed.md)'` — Title-Case with arrow suffixes.

### 3.3 The concrete mismatch

| Dimension | Constitution-parser-expected | Herald-actual | Match? |
|---|---|---|---|
| Item layout | heading-per-item (`## ATM-NNN …` H2) | **pipe table rows** under `## Open` / `## Recently fixed` | **NO** |
| ID scheme | `ATM-NNN` | `HRD-NNN` | NO (cosmetic — regex prefix) |
| `type` vocab | `Bug` / `Feature` / `Task` (Title-Case) | `bug` / `task` (lowercase) | **NO** (case + missing `Feature`) |
| `status` vocab | `Queued` / `In progress` / `Fixed (→ Fixed.md)` … (Title-Case, arrow suffix) | `open` / `in_progress` (lowercase) | **NO** |
| Criticality | not a column (folded into `severity` free-text) | distinct `Criticality` column (`low`/`middle`/`high`) | partial — maps to `severity` |
| References / Catalogue-Check | no dedicated column | `References` free-text cell carries `Catalogue-Check:` | **NO column** in schema |
| Issues vs Fixed columns | one `current_location` flag drives both | **two different table shapes** (Fixed.md drops Status/Opened, adds Closed/Commit) | **NO** |

**Bottom line:** even if Phase 3 were implemented to the schema's evident intent, its parser would expect a
heading-per-item `ATM-NNN` Title-Case-status layout and would **fail to parse Herald's pipe-table `HRD-NNN`
lowercase-status format**. The mismatch is structural (table-vs-heading), not merely cosmetic.

## 4. Schema fit (`schema.sql` `items` table vs Herald columns)

| Herald column | `items` column | Fit |
|---|---|---|
| `ID` (HRD-NNN) | `atm_id` (PK) | Works as an opaque PK string (the `ATM-NNN` label in comments is not a CHECK constraint), but the `atm_id` *name* is project-specific. |
| `Type` (`task`/`bug`) | `type CHECK IN ('Bug','Feature','Task')` | **Blocks** — Herald's lowercase `task`/`bug` violate the Title-Case CHECK; insert would fail. |
| `Status` (`open`/`in_progress`) | `status CHECK IN ('Queued','In progress',…)` | **Blocks** — Herald's `open`/`in_progress` are not in the closed-set; insert would fail. |
| `Criticality` (`low`/`middle`/`high`) | `severity` (free-text, no CHECK) | Fits (free-text), but Herald uses it as a *first-class* column the schema treats as optional. |
| `Title` | `title NOT NULL` | Fits. |
| `Opened` / `Last update` / `Closed` | `created_at` / `last_modified` (+ `item_history.on_date`) | Partial — Fixed.md `Closed` + `Commit` would need `item_history` rows (event `Fixed`/`Completed` + `evidence_path`), which the parser would have to synthesise. |
| `References` (+ `Catalogue-Check`) | `composes_with` (JSON array) | **Poor fit** — Herald's References is free-text prose carrying the load-bearing §11.4.74 `Catalogue-Check:` line; no schema column captures Catalogue-Check verbatim. Would be lossy. |
| `Commit` (sha, Fixed.md) | (none) | **No column** — closest is `item_history.evidence_path`; would need mapping. |

The schema's `description NOT NULL` + the §11.4.91 "≥6 words OR ≥40 chars" floor (README-noted as
"enforced at insert") also has **no source column** in Herald's table format — Herald rows have `Title` but no
separate `description` field, so the parser would have to derive description from Title (most Herald titles are
long enough to clear the floor, but this is a parser policy decision the binary owner must make).

**Verdict:** the schema is *close in spirit* (it models the same workable-item lifecycle) but its closed-set
CHECK constraints on `type` and `status` are written to a **different vocabulary** than Herald's, and there is
**no first-class home** for Herald's `Criticality` column, free-text `References`/`Catalogue-Check`, or Fixed.md
`Commit` sha. Adopting it requires either (a) the constitution schema/parser learning Herald's vocabulary +
columns, or (b) a documented value-mapping (e.g. `task`→`Task`, `open`→`Queued`, Criticality→`severity`,
References→`composes_with` + a new `catalogue_check`/`commit` column).

## 5. Gap analysis & recommendation

### 5.1 The three candidate paths

- **(a) adopt-as-is (Phase 2 = run it):** REJECTED — there is nothing to run; `sync md-to-db` is a stub that
  exits `2` and writes no DB.
- **(b) Herald-side adapter / local parser:** **FORBIDDEN by §11.4.74** — Herald must not reimplement a
  workable-items parser locally when the canonical tool lives in the constitution. A Herald-local parser would be
  a duplicate of the constitution's planned Phase-3 `pkg/mdparser/`.
- **(c) extend / complete the constitution binary first:** **CORRECT PATH.** Phase 3 (`sync md-to-db`) must be
  implemented in the constitution repo, and it must be format-aware enough to parse Herald's table layout +
  vocabulary (or accept a per-project config mapping).

### 5.2 Recommendation

**`blocked-on-binary-completion` — the constitution binary's Phase 3 (`sync md-to-db` parser + `pkg/itemsdb`
SQLite access layer) must land before Herald can generate `docs/.workable_items.db`.** This is **constitution-side
work**, not Herald-side, per §11.4.74.

Because Herald's tracker is a **pipe-table** format with **lowercase `task`/`bug` types**, **lowercase
`open`/`in_progress` statuses**, a **distinct `Criticality` column**, and a **free-text `References`/`Catalogue-Check`
cell** — none of which match the constitution schema's heading-per-item `ATM-NNN` Title-Case-status intent — the
Phase-3 implementation MUST be designed to handle table-row input and a configurable type/status/column vocabulary
(or the constitution + Herald jointly agree a canonical vocabulary and Herald migrates its trackers to it). The
schema's `type`/`status` CHECK constraints will reject Herald's current vocabulary as written.

### 5.3 Concrete next action

1. **Where it lands:** `constitution` repo (`scripts/workable-items/`), NOT Herald. Open/advance the
   constitution-side §LA Phase-3 work item to implement `pkg/mdparser/` + `pkg/itemsdb/` + wire `runSync("md-to-db")`.
2. **Format requirement to feed back to the constitution binary owner:** the parser must support a **pipe-table
   tracker layout** (columns `ID | Type | Status | Criticality | Title | Opened | Last update | References` for
   Issues; `ID | Type | Criticality | Title | Closed | Commit | Reference` for Fixed) and a **per-project
   vocabulary mapping** (Herald: `task`/`bug` → `Task`/`Bug`; `open`/`in_progress` → `Queued`/`In progress`;
   `Criticality` → `severity`; `Commit` sha + `Catalogue-Check` need dedicated columns or `composes_with`/`item_history`
   mapping). A `schema.sql` amendment (add `criticality`, `catalogue_check`, `commit_sha` columns; relax or
   parametrise the `type`/`status` CHECK closed-sets) is likely required.
3. **Herald-side follow-on (deferred until the constitution Phase 3 lands):** HRD-131 stays OPEN. Once the
   constitution binary can parse Herald's format, Herald's Phase-2/3 step is to run
   `workable-items sync md-to-db` against `docs/Issues.md` + `docs/Fixed.md` and commit the generated
   `docs/.workable_items.db` (gitignored or committed per the §11.4.93 6-phase plan — to be decided at that
   time). Herald writes NO parser of its own.
4. **Interim:** Herald continues to hand-maintain `docs/Issues.md` + `docs/Fixed.md` as the source of truth
   (Phase 6 "legacy text-direct edits prohibited" is not yet in force).

## 6. Non-execution attestation

This assessment was conducted **READ-ONLY**:

- Files read: constitution `README.md`, `cmd/workable-items/main.go`, `schema.sql`, `go.mod`; Herald
  `docs/Issues.md`, `docs/Fixed.md`.
- The constitution binary was built to a **scratch path** (`/tmp/wi-scratch-build`) only. Its stub subcommands
  (`sync md-to-db`, `validate`) were invoked solely to confirm they exit `2` (not-impl) and write nothing — they
  did NOT write to Herald `docs/` and did NOT create any SQLite DB.
- **No `docs/.workable_items.db` was generated** (verified absent before and after).
- **No mutation gate, e2e suite, or Challenge was run.**
- **No `git add` / `commit` / `push` / `checkout -- <file>` / `stash` was performed** in either repo.
- The constitution sibling repo was **not edited** (clean working tree before and after; only a read-only
  `git log` + `git status --porcelain` was run).
- Herald working tree was clean at pre-flight (`git status --porcelain` empty) — no MUTATED markers in tracked
  `.go` files; the constitution `workable-items` `.go` is also marker-free.
- The **only** file written by this assessment is this research doc.
