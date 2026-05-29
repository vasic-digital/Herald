# Worker "hourly OOM + telemetry-drop" — Phase-1 investigation (2026-05-29)

| Field | Value |
|---|---|
| Trigger | Session-memory note: "worker hourly OOM + telemetry-drop; 5 suspect paths; heap-profile repro provided" |
| Method | `superpowers:systematic-debugging` Phase 1 (root-cause investigation, no fixes) |
| Verdict | **Premise does NOT match committed repository state.** No reproducible OOM found in-repo. One genuine LATENT risk surfaced → HRD-146. |

## Finding: the reported symptom has no committed basis

A systematic read of the repo (and git history) found **none** of the artefacts
the premise assumed:

- **No hourly worker.** No `@hourly` / cron / `PeriodicJob` / `time.Hour`-driven
  scheduler exists. The only `time.Hour` uses are Redis idempotency-key TTLs
  (`pherald/internal/runner/runner.go`), not a worker interval. The only ticker
  is a 30s stall safety-net in `subscribe.go`.
- **No telemetry subsystem.** No OTel/observability wiring exists in any
  Herald-owned module — only "reserved for future OTel instrumentation"
  comments. The reported "telemetry-drop" has no code to originate from.
- **No heap profile / no pprof.** No `*.pprof`/`*.prof`, no `runtime/pprof` or
  `net/http/pprof` import anywhere in Herald source.
- **No prior "5-path triage" committed.** Nothing in `docs/`, `.remember/`, or
  `qa-results/` records it.
- **No OOM in git history.** `git log --all | grep -iE 'oom|leak|memory'` finds
  only an unrelated `.gitignore`/PID-file hardening commit.

The `docs/qa/HRD-128-*` directories are **pre-emptive §11.4.85 resource-
exhaustion stress suites** (they assert `leaked=1`/`0 goroutine leak` as PASS
evidence), NOT records of an observed OOM incident.

The five suspect subsystems were each read; `dispatch`, `river`, `attachments`,
`postgres` showed correctly-closed rows/pools/readers and no accumulating
structures. ("river" is not even used — `commons_infra/queue.go` is a type-alias
shim; there is no `river.Worker`.)

## The one genuine latent risk (→ HRD-146)

`pherald listen` dispatches each inbound message through
`claude_code.Dispatch` (`commons_messaging/dispatch/claude_code/dispatch.go`)
which calls `cmd.Output()` — **fully buffering the child's stdout in memory** —
with **no per-message timeout** beyond the long-lived Subscribe ctx
(`subscribe.go` invokes `h.Handle(ctx, ev)` on telebot dispatch goroutines with
the Subscribe-lifetime ctx; `go bot.Start()`).

Hypothesis (NOT a confirmed defect): under a hung/slow `claude` child plus
sustained inbound load, dispatch goroutines + their stdout buffers could
accumulate for the process lifetime — an unbounded-growth path. This is the
closest thing in-repo to the reported symptom, but it is untested under real
hung-child load (the in-repo stress tests use stub dispatchers) and would
require a live `pherald listen` + a real/slow `claude` binary + added pprof to
confirm.

Filed as **HRD-146** (dispatch hardening: per-message timeout + bounded stdout).
NOT fixed here — no fabricated fix for a symptom with no committed root cause
(systematic-debugging "no root cause in repo" terminal state).

## Anti-bluff note

Reporting "premise does not match reality" rather than manufacturing a fix is
the §11.4 discipline: a speculative OOM patch would have been a PASS-bluff
(claiming to fix a defect that has no committed evidence). The latent dispatch
risk is recorded honestly as a hypothesis-grade hardening task, not a bug.
