<div align="center">

<img src="../../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# HRD-153 / HRD-156 — watch→notify + inbound→CRUD core e2e evidence

| Field | Value |
|---|---|
| Run ID | `HRD-153-20260529T082513Z` |
| Captured | 2026-05-29 (UTC) |
| Feature | ATMOSphere integration — workable-items SSoT watch→notify daemon (`pherald watch`) + inbound→CRUD item mutation wiring (`pherald listen`) |
| §107.x posture | recorded e2e transcript + captured runtime evidence for the shipping core |

## What this evidence proves

This run is the §107.x positive-runtime-evidence anchor for the
**watch→notify + inbound→CRUD core** that the HRD-153 (ATMOSphere WS-2/WS-7)
and HRD-152 (inbound item-mutation) work delivered. It proves the
user-visible behaviour works — not merely that the process boots.

### 1. `pherald watch` — watch → diff → notify (fully real pipeline)

`TestRunWatch_EndToEndOutbound` drives the **entire** outbound pipeline with
ZERO pipeline mocks:

- **Real SQLite** — `commons_workable.Open` over a temp `workable_items.db`
  (WAL mode), seeded with a baseline item so create/update/delete are
  detected against a non-empty prior snapshot (not an empty-DB special case).
- **Real fsnotify watcher** — `commons_watch.New` (fsnotify + WAL-poll
  fallback on the `.db`/`-wal`/`-shm` sidecars).
- **Real diff** — `commons_workable.Diff(prev, curr)` per-property change set.
- **Real fan-out** — `workflow.Notifier` over the production
  `runner.ChannelDispatcher` into a recording `commons.Channel` sink.

The test mutates the DB three ways through the real `Repo` and asserts the
exact rendered diff message reaches the fan-out each time:

| Mutation | Asserted rendered message |
|---|---|
| `repo.Create(HRD-901)` | `🆕 HRD-901 created` |
| `repo.Update(status → In progress)` | `🔄 HRD-901 status: Queued → In progress` |
| `repo.Delete(HRD-901, Issues)` | `🗑️ HRD-901 removed` |

It also asserts clean shutdown on `ctx` cancel (prompt return, no goroutine
leak: `delta ≤ 1`).

### 2. `pherald listen` — inbound → workable-item CRUD (production wiring, HRD-152)

`listen.go` now opens the workable-items SQLite SSoT (`--db` flag /
`HERALD_WORKABLE_DB` env / default `docs/workable_items.db`, mirroring
`pherald watch`'s open pattern) and stamps a real
`inbound.NewRepoMutator(workable.NewRepo(store))` into the inbound
`Dispatcher` `Config.Items`. This backs the `item.update`, `item.delete`,
and confirmed-`investigation.start` action paths against the same real
SQLite SSoT that `pherald watch` observes — so an inbound LLM-routed
mutation is picked up by the watcher and fanned out as a notification.

Graceful behaviour preserved: when the DB path is empty/unset and the env is
unset, `Items` stays `nil` and those actions return the dispatcher's
explicit `"no ItemMutator configured"` error (§107 fail-loud, never a silent
drop). The router CRUD logic itself is unit-tested in
`pherald/internal/inbound` (RepoMutator exercised against a real SQLite
Store in `repo_mutator_test.go`; the action-routing matrix against a
recording fake).

## Test names

- `pherald/cmd/pherald/watch_test.go` → `TestRunWatch_EndToEndOutbound`
- `pherald/cmd/pherald/watch_test.go` → `TestWatchCmd_Registered`
- `pherald/internal/inbound/repo_mutator_test.go` → RepoMutator vs real SQLite
- `pherald/internal/inbound/dispatcher_test.go` → action-routing matrix

## Reproduce

```bash
# Captured evidence (this run):
go test -race -v -run 'TestRunWatch_EndToEndOutbound' ./pherald/cmd/pherald/...

# E139 invariant form (as it runs inside scripts/e2e_bluff_hunt.sh):
go test -race -count=1 -run 'TestRunWatch_EndToEndOutbound' ./pherald/cmd/pherald/...

# Inbound CRUD router + RepoMutator unit/integration:
go test -race -count=1 ./pherald/internal/inbound/...

# Real-binary proof of the HRD-152 production wiring (--db flag present):
go build -o /tmp/ph ./pherald/cmd/pherald && /tmp/ph listen --help | grep -- --db
```

Captured output of the verbose run is in
[`watch_notify_e2e.log`](./watch_notify_e2e.log) (this directory).

## §11.4.98 full-automation note

The hermetic core proven here is **fully self-driving end-to-end** with no
human action during execution — temp DB created in-process, real fsnotify
watcher, programmatic `repo.Create/Update/Delete`, recording-channel
readback, deterministic `Ready` startup-ordering signal (no sleeps for
correctness), self-cleaning `t.TempDir()` state. It runs unconditionally in
CI (`commons_workable` is in-process SQLite, **not** Postgres-gated), which
is why E139 in `scripts/e2e_bluff_hunt.sh` carries no PG gate.

The **LIVE Telegram round-trip** end of HRD-156 (a real subscriber on a real
channel receiving the watch→notify message over the wire) is **NOT** covered
by this hermetic core. It is pending the MTProto credential bootstrap
(`~/.config/herald/mtproto.session` + `HERALD_MTPROTO_*` env, the one-time
out-of-test configuration permitted by §11.4.98). Until that bootstrap
lands, the live round-trip is an honest **SKIP-with-reason** per §11.4.3 (the
same posture as E135–E137); claiming a green live PASS without it would be a
§11.4 PASS-bluff at the automation layer.

## Sources / authority

- Herald §107.x docs/qa evidence mandate (CLAUDE.md) — cascades from Helix §11.4.83.
- §11.4.98 full-automation anti-bluff mandate (live tests re-runnable without manual intervention).
- §11.4.5 captured-evidence composition; §11.4.3 SKIP-with-reason.
