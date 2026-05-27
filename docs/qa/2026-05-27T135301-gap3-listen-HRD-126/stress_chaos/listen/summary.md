# HRD-126 — `pherald listen` inbound-path stress + chaos evidence

GAP-3 plan §1 row 4 (`docs/superpowers/plans/2026-05-27-stress-chaos-suite.md`).
§11.4.85 stress + chaos mandate. Run-id `2026-05-27T135301-gap3-listen-HRD-126`.

Surface: the `pherald listen` inbound path — `inbound.Dispatcher.Handle`, the
unexported `extractReplyToMessageID` reply-quote parser, and the full
`runListen` runtime loop (the hermetic seam the production `RunE` calls).

All evidence captured under `go test -race`. Per §11.4.27, only the channel
boundary (Subscriber) and the CC boundary (CodeDispatcher) are faked; the
Dispatcher, the fan-in orchestration, and the clean-shutdown path run
unmodified. Determinism confirmed by `-count=3` (all 3 iterations green).

## Tests (5 PASS hermetic + 2 loop-level PASS + 1 SKIP-with-reason live)

| Test | Layer | Scenario | Verdict | Evidence file |
|---|---|---|---|---|
| `TestInbound_Stress_ConcurrentDispatch` | `inbound.Dispatcher.Handle` (real) | STRESS — 16 workers × 50 = 800 concurrent events; exactly-once reply; goroutine-leak check | PASS | `latency.json`, `latency_histogram.csv`, `throughput.txt` |
| `TestInbound_Chaos_ExtractReplyToMessageID_Degrades` | `extractReplyToMessageID` (real, unexported) | CHAOS(a) — 12 corrupt/valid `message_id` cases; degrade to (0,err); never panic | PASS | `malformed_payloads.txt` |
| `TestInbound_Chaos_HandleMalformedRaw_NeverPanics` | `inbound.Dispatcher.Handle` (real) | CHAOS(a) — 8-case corrupt-Raw flood (8 workers); recover-guard; degraded replyTo="" | PASS | `handle_malformed_raw.txt` |
| `TestInbound_Chaos_CCFault_SurfacedNotSwallowed` | `inbound.Dispatcher.Handle` (real) | CHAOS(b) — 6 CC faults (killed/truncated/marker-less/empty/unknown-action); tagged error; no reply leak; no hang | PASS | `cc_fault.txt` |
| `TestInbound_Chaos_OnceOnlySideEffectUnderFlood` | `inbound.Dispatcher.Handle` (real) | CHAOS(c) — 50 workers × 20 = 1000 redeliveries of same issue.open; exactly-one durable row | PASS | `once_only_side_effect.txt` |
| `TestListen_Stress_BurstLoopThroughput` | `runListen` (real loop) | STRESS — 400-event burst through the full loop; nothing dropped; clean shutdown under load; no leak | PASS | `listen_loop_throughput.txt` |
| `TestListen_Chaos_ChannelDeathFailsLoud` | `runListen` (real loop) | CHAOS — subscriber dies with non-cancel error; fail-loud tagged error, no hang | PASS | `channel_death.txt` |
| `TestInbound_Chaos_ProcessDeathRestartLive` | real binary | CHAOS(c) real-binary SIGKILL+restart | SKIP-with-reason | `process_death_restart.log` |

## Per-chaos verdicts

- **CHAOS(a) malformed payloads — PASS (hermetic core).** Every corrupt
  `message_id` (nil map, missing key, nil/string/bool/slice/map value) degrades
  to `(0, error)`; valid int/int64/int32/float64/float32 decode correctly.
  `Dispatcher.Handle` under a 64-dispatch corrupt-Raw flood: 0 panics
  (recover-guard), 0 errors, degraded `replyTo=""` (no bogus quote id). Anchors:
  `all_malformed_degraded_no_panic=1`, `panic_free=1`.

- **CHAOS(b) CC subprocess fault — PASS.** All 6 fault shapes surface a
  stage-tagged `inbound:` error (`CC dispatch` / `parse reply` / `unknown
  action`), the subprocess-kill error stays `errors.Is`-reachable, NO reply
  leaks to the user, and Handle never hangs (bounded by a 3s watchdog). Anchor:
  `cc_fault_surfaced_no_reply_leak=1`.

- **CHAOS(c) process-death once-only — PASS hermetic / SKIP-with-reason live.**
  1000x redelivery of the same `issue.open` collapses to exactly ONE durable
  sink row (every redelivery reaches the sink: attempts==1000). This is the
  in-process analogue of a `pherald listen` crash+restart redelivery loop, and
  leans on the same once-only contract HRD-125 proved at the Runner
  events_processed layer. The real-binary SIGKILL+restart variant is
  SKIP-with-reason (needs operator Telegram creds + live PG + a built pherald;
  set `HERALD_STRESS_LIVE_LISTEN=1`). Anchor: `once_only_side_effect=1`.

## Key metrics (representative run)

- STRESS Dispatcher.Handle: 800 events, 0 errors, 800 replies (exactly-once),
  p50=0.41ms p95=1.64ms p99=2.21ms max=3.12ms, ~25k dispatch/s. Goroutines
  2->2 (leaked=0, slack=8).
- STRESS runListen loop: 400 events dispatched+replied, nothing dropped, clean
  shutdown on cancel (<3s), ~18k events/s. Goroutines 2->2 (leaked=0).

## Goroutine-leak result

No leak. Both stress tests snapshot `runtime.NumGoroutine()` before the load and
again after a post-load `runtime.GC()` + settle window; both returned to the
baseline (leaked=0, well under the slack of 8). The Dispatcher.Handle path
spawns no background goroutines; `runListen` tears down its fan-in goroutines on
cancel.

## Panic-free proof

`TestInbound_Chaos_ExtractReplyToMessageID_Degrades` and
`TestInbound_Chaos_HandleMalformedRaw_NeverPanics` each wrap the call in a
`recover()` guard that converts any panic into a hard `t.Fatalf`. 0 panics
observed across 12 table cases + a 64-dispatch corrupt-Raw flood. Anchors
`all_malformed_degraded_no_panic=1` + `panic_free=1`.

## Host-safety (section 12 / 12.6)

See `hostmem.txt`. Bounded concurrency (N<=50 goroutines), KiB-scale allocation
(RunLoad allocates `workers*iter` LoadResult structs only), no fork-bomb, no
GB-alloc, no host-net change, no systemctl. The only process signalling is the
in-process `context.CancelFunc` (SIGTERM analogue) on a test goroutine — no host
or system process is touched. The 74% host used-memory reading in `hostmem.txt`
is ambient host load, NOT test-induced.

## Determinism

`go test -race -count=3` on both `./pherald/internal/inbound/` and
`./pherald/cmd/pherald/` — all 3 iterations green. The contract assertions are
value-true (exactly-once reply count, exactly-one durable row, tagged-error
substrings), never timing-sensitive artifacts.
