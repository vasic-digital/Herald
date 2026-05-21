<div align="center">

![Herald](../assets/logo/herald_logo_square_128.png){width=96px height=96px}

</div>

# Herald — Continuation

| Field | Value |
|---|---|
| Revision | 6 |
| Created | 2026-05-20 |
| Last modified | 2026-05-21 |
| Status | active |
| Status summary | **Wave 3a + Wave 2 + Foundation M1/M2/M3 all landed.** Workspace at 14 Go modules (`go.work` gitignored): commons + commons_auth + commons_constitution + commons_infra + commons_messaging + commons_prefix + commons_storage + pherald + 6 Wave-2 flavor binaries (sherald, cherald, bherald, rherald, iherald, scherald). Three live REST surfaces (pherald serve plumbing + cherald `/v1/compliance` + sherald `/v1/safety_state`); pherald `/v1/events` ingest still 501 pending Wave 3b Runner. Anti-bluff battery green: e2e_bluff_hunt 41 PASS / 0 FAIL / 5 SKIP; audit_antibluff 16/0/1; codegraph_validate 7/0/2; inheritance gate 15/15 + META-PASS; 5 paired §1.1 mutation gates green. Spec V3 at r8 (r9 owns Wave 3b spec capture). Doc-cleanup pass (this commit): Status r10→r11, Issues r9→r10, Fixed r8→r9 renumbered Wave 3a commons_auth HRD-093 → HRD-099 to resolve collision with Wave 2 sherald-scaffold HRD-093. |
| Issues | HRD-008, HRD-011 (code complete; live evidence pending), HRD-015, HRD-016, HRD-018 (in_progress), HRD-019..HRD-027, HRD-029..HRD-056, HRD-081, HRD-085..HRD-090 |
| Issues summary | see `Issues.md` — 49 open items |
| Fixed | HRD-001..HRD-007, HRD-009, HRD-009b, HRD-010, HRD-012, HRD-013, HRD-014, HRD-017, HRD-028, HRD-080, HRD-092..HRD-099 (HRD-018 partial — M1 + M2 components landed) |
| Fixed summary | see `Fixed.md` |
| Continuation | **Wave 3b next**: pherald Runner (`pherald/internal/runner/` — 7 stages from §32) + live `POST /v1/events` ingest (HRD-016 close-out) + Telegram live evidence (HRD-011 — operator exports HERALD_TGRAM_BOT_TOKEN + HERALD_TGRAM_CHAT_ID; e2e E40 flips SKIP→PASS). Wave 3b plan still to be written (next `superpowers:writing-plans` invocation). After Wave 3b: §43 command-catalogue rollout (HRD-029..056), iherald `/v1/webhooks/page` live (HRD-024), sherald destructive-guard body (HRD-033 — unlocks E48), HRD-091 codegraph submodule-traversal gap. Spec V3 r9 bump owned by Wave 3b. Multi-mirror push after each milestone. Update on every non-trivial commit (per Universal §12.10). |

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

## §5. Long-form pointers

- `docs/specs/mvp/specification.V3.md` — full active spec (Revision 4).
- `docs/specs/mvp/specification.V3.md#30-v2-self-review-log` — every review pass.
- `docs/guides/HERALD_CONSTITUTION.md` — §101..§106 extending Universal.
- `docs/guides/CONSTITUTION_INHERITANCE.md` — parent-discovery + gate.
- `tests/test_constitution_inheritance.sh` — the gate.
- `quickstart/` — HRD-008 scaffold.
- `commons/types.go` — the §11.0 type contract reference.
