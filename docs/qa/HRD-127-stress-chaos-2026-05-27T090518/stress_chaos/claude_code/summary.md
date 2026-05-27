# HRD-127 — claude_code dispatch §11.4.85 stress + chaos summary

Run-id: `HRD-127-stress-chaos-2026-05-27T090518`
Surface: `commons_messaging/dispatch/claude_code` (the Claude Code LLM dispatcher, spec §33).
Plan: `docs/superpowers/plans/2026-05-27-stress-chaos-suite.md` §1 row 5.
Command: `go test -race -count=1 -run 'TestDispatch_Stress|TestDispatch_Chaos|TestBootstrap_Chaos' ./commons_messaging/dispatch/claude_code/`
Go: `go version go1.26.2 darwin/arm64`. Captured: 2026-05-27T090617Z.

The REAL `Dispatcher.Dispatch` / `buildCmd` / `bootstrapSession` / `parseReply` /
`ResolveSession` are exercised. Per §11.4.27 only the EXTERNAL boundary — the
`claude` CLI binary — is faked, via committed hermetic shims under
`commons_messaging/dispatch/claude_code/testdata/fake-claude-*.sh`, selected
through the EXISTING `New(binaryPath,...)` constructor seam (no production env
var added, no production source touched, no real `claude` invoked).

## Per-scenario result

| Scenario | Test | Result | Evidence | Anchor |
|---|---|---|---|---|
| STRESS — N=24×25=600 parallel Dispatch (steady-state `--resume`) | `TestDispatch_Stress_ConcurrentResume` | PASS | `latency.json`, `latency_histogram.csv`, `dispatch_latency.txt` | `session_resolution_concurrency_safe=1`, `bootstrap_count=0` |
| STRESS — N=16 concurrent cold-start bootstrap | `TestDispatch_Stress_ConcurrentColdStartBootstrap` | PASS | `cold_start_bootstrap.txt` | bootstraps∈[1,workers], 1 persisted anchor |
| CHAOS (a) process-death exit 137 (partial write) | `TestDispatch_Chaos_ProcessDeath_Exit137` | PASS | `subprocess_kill.log` | `tagged_error_no_hang=1` |
| CHAOS (a) process-death self-SIGKILL | `TestDispatch_Chaos_ProcessDeath_SelfSIGKILL` | PASS | `subprocess_self_sigkill.log` | `tagged_error_no_hang=1` |
| CHAOS (b) timeout — ctx cancel (Dispatch) | `TestDispatch_Chaos_TimeoutContextCancel` | PASS | `timeout_cancel.log` | `deadline_fired=1` |
| CHAOS (b) timeout — ctx cancel (bootstrap) | `TestBootstrap_Chaos_TimeoutContextCancel` | PASS | `bootstrap_timeout_cancel.log` | `deadline_fired=1` |
| CHAOS (c) truncated `<<<HERALD-REPLY>>>` | `TestDispatch_Chaos_TruncatedReply` | PASS | `truncated_reply.txt` | `explicit_parse_error=1` |

All 7 PASS; `go test -race -count=3` deterministic green (see `race_clean.log`,
`DATA_RACE_lines=0`).

## Stress latency (exec round-trip, healthy fake shim)

{
  "count": 600,
  "elapsed_ms": 1655.011,
  "errors": 0,
  "iterations_each": 25,
  "max_ms": 147.645,
  "min_ms": 8.271,
  "p50_ms": 62.871,
  "p95_ms": 96.39,
  "p99_ms": 116.901,
  "throughput_per_sec": 362.5351996603519,
  "workers": 24
}

Latency is dominated by the per-call `fork/exec` of the shell shim (a real
process spawn each Dispatch) — the honest cost of the exec round-trip. p99 ≈
117ms; 0 errors over 600 concurrent dispatches; -race clean.

## Findings (honest, code-true; not PASS-bluffs)

1. **Cold-start concurrent bootstrap is NOT exactly-once** — `bootstrap.go:50-52`
   documents last-writer-wins with no serialisation. Observed 16 bootstraps for
   16 concurrent cold-start goroutines; exactly one anchor UUID persists, the
   rest become inert orphans. Asserting `bootstrap_count==1` would be a §107
   PASS-bluff. Honest bound asserted: bootstraps∈[1,workers]. Fix direction if
   ever required: advisory `flock` on the anchor before bootstrap.
2. **Timeout cancellation latency depends on `claude` being a single process.**
   Production `Dispatch` wires the ctx into `exec.CommandContext`, whose default
   Cancel SIGKILLs only the DIRECT child; `cmd.Output()` then blocks until the
   stdout pipe is closed by ALL descendants. The real `claude` is one
   long-lived process → on SIGKILL it closes its own stdout and `Output()`
   unblocks immediately (verified: Dispatch returned in 803ms for an 800ms
   deadline, vs the shim's 30s sleep). The fake shim uses `exec sleep` (single
   PID, replace-in-place) to model this faithfully. If `claude` ever spawns
   helper subprocesses that outlive a SIGKILL while holding stdout, cancellation
   latency would degrade; hardening direction is `Cmd.Cancel` + process-group
   kill (`Setpgid` + `kill(-pgid)`). Flagged for the dispatcher owner; out of
   scope for the HRD-127 test layer.

## Host-safety (§12 / §12.6)

Bounded load only: N≤24 goroutines, each spawning ONE short-lived fake-shim
child the Go test owns and reaps. The ONLY process ever killed is a fake-shim
child this test spawned (self-`kill -KILL $$` or ctx-driven SIGKILL); never a
real `claude` or host process. No fork-bomb, no GB-alloc, no host-net change.

claude_code dispatch is an in-process `exec` surface, NOT a resource-exhaustion
surface, so the §12.6 60%-memory ceiling does not gate it (same posture as
HRD-123 / HRD-126). Host-memory probe recorded for the audit trail:

```
{Available:true TotalBytes:19327352832 FreeBytes:4939284480 UsedBytes:14388068352 UsedFraction:0.7444407145182291 Platform:darwin Note: CapturedAtRFC:2026-05-27T14:05:20+05:00}
```
