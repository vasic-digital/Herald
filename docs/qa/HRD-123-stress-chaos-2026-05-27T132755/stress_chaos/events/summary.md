# HRD-123 — Gin `/v1/events` §11.4.85 stress + chaos summary

| Field | Value |
|---|---|
| Surface | `pherald/internal/http/events.go` -> `EventsHandler` -> `runner.Run` (7-stage pipeline) |
| Plan | `docs/superpowers/plans/2026-05-27-stress-chaos-suite.md` §1 row 1 (GAP-3 / §11.4.85 / §108.a) |
| Test file | `pherald/internal/http/events_stress_chaos_test.go` |
| Run id | `HRD-123-stress-chaos-2026-05-27T132755` |
| Run mode | hermetic (in-process `httptest` server dialed by `net/http.Client`; FakeRunner boundary fakes per §11.4.27) |
| Race detector | clean (`go test -race -count=1`) |
| Host | darwin; Go 1.26.2; host-mem baseline 0.696 (see `host_memory_headroom.txt` — NOT a resource-exhaustion surface) |

## Real captured numbers

### STRESS — healthy path (`latency.json` / `throughput.csv` / `latency_histogram.csv`)

- 12 workers x 200 = **2400 CloudEvent POSTs**, **0 errors**, **0 5xx**, all 202 Accepted.
- Latency: **p50=0.706 ms, p95=1.755 ms, p99=2.665 ms, max=5.722 ms, min=0.314 ms**.
- Throughput: **~13,322 req/s**; elapsed 180.1 ms.
- Anchor: `zero_5xx_healthy=1`.

## Per-chaos-scenario verdict

| Scenario | Verdict | Evidence | Key numbers |
|---|---|---|---|
| (b) Redis-down / duplicate-key under load | **PASS** | `redis_down_fallback.log` | 1200 POSTs (per-worker key) -> 12 accepted(202) + 1188 replayed(200) + 0 other; 0 5xx; 0 hang; p99=2.528 ms |
| (c) input-corruption (oversized 8 MiB / truncated / non-UTF8 / array / missing-id / garbage / empty) | **PASS** | `categorised_errors.txt` | 7 cases x 5 reps -> every body -> tagged 400 (`event_parser: ...`); 0 panic; 0 5xx; 0 transport errors |
| (d) auth storm (1000 random 32-byte bearer tokens vs REAL HMAC verifier) | **PASS** | `auth_storm.log` | 1000 random tokens -> 1000 x 401; 0 bypass; 0 5xx; p99=2.173 ms |
| (a) PG-drop mid-request (container pause) | **SKIP-with-reason** | `recovery_trace.log` | No container runtime; fail-loud-on-PG-error proven hermetically at runner layer (HRD-125 `TestRunner_Chaos_PGDeadlockSurfacedNotSwallowed`). Set `HERALD_STRESS_LIVE_PG=1` + `DOCKER_HOST` for the live variant. |

## §11.4.85 anti-bluff notes

- All four hermetic scenarios drive the REAL `EventsHandler` -> `runner.Run` through
  the production Gin middleware chain (`TOONMiddleware` + auth gate) over a real
  in-process `httptest` server. Only the external boundary (PG / Redis / channel) is
  faked, via `runner.NewFakeRunner` whose fakes faithfully model the production
  adapter contracts.
- The auth storm exercises the GENUINE `commons_auth.GinMiddleware` + a REAL HMAC
  `commons_auth.Verifier` (HS256). Random bytes never produce a valid signature, so a
  single non-401 would be a real auth-bypass defect — no token is minted, so no new
  dependency is added to `pherald/go.mod`.
- The race detector is clean. The SHARED-key cross-worker concurrent-replay case
  triggers a REAL data race in production code at `runner.go:132`
  (`rc.CachedRcpt.WasReplay` mutates a `*Receipt` the Receipt-caching store hands to
  every concurrent replay). That is the HRD-132 latent race ("activates with Wave 4+
  Receipt caching") — owned by HRD-132, proven at the runner layer by HRD-125. This
  HTTP test uses per-worker keys (production-faithful: real PG does NOT cache the
  Receipt today) so the HTTP-surface degrade contract is proven race-free without
  asserting a property the code cannot honour (which would be a §107 PASS-bluff). See
  `redis_down_fallback.log` FINDING block.
- Dispatch exactly-once is NOT guaranteed under concurrent replay (HRD-132);
  `events_processed` archival IS exactly-once. The nil-Redis PG-only fallback
  idempotency is proven at the runner layer (HRD-125 `nil_redis_degrade.txt`).
