<div align="center">

![Herald](../../../assets/logo/herald_logo_square_128.png){width=96px height=96px}

</div>

# Wave 4 — HTTP/3 (QUIC) + Brotli + TOON Transport Design

| Field | Value |
|---|---|
| Revision | 1 |
| Created | 2026-05-22 |
| Last modified | 2026-05-22 |
| Status | active |
| Status summary | r1 — Design doc only (no code). Captures the operator mandate (2026-05-22) that every Herald REST API surface MUST serve HTTP/3 (QUIC) primary with Brotli as default compression, HTTP/2 + gzip as honest fallback, and TOON (Token-Oriented Object Notation) as the primary wire data format with JSON as fallback. Catalogue-check (§11.4.74) discovered THREE relevant vasic-digital submodules already exist — `digital.vasic.http3` (quic-go wrapper), `digital.vasic.middleware/pkg/brotli` + `pkg/altsvc` (Brotli middleware + Alt-Svc advertiser), `digital.vasic.toon` (currently sentinel-error scaffold pending toon-format/toon-go integration). Recommended split into Wave 4a (HTTP/3 + Brotli + content-negotiation substrate, ~10 tasks) and Wave 4b (TOON adoption end-to-end, ~7 tasks) because TOON's upstream is a separate dependency surface and Wave 4a delivers an independently shippable end-user benefit. e2e_bluff_hunt invariants would grow ~48 → ~63 (Wave 4a) → ~70 (Wave 4b). Spec V3 r9 → r10 captures §41 transport subsection. |
| Issues | none |
| Issues summary | — |
| Fixed | (n/a — design doc) |
| Continuation | Operator review → answer the 4 open questions in Section 9 → spec V3 r9 → r10 in the same commit cycle as Wave 3b close → invoke `superpowers:writing-plans` (once per sub-wave) → execute via `superpowers:subagent-driven-development` per §11.4.70. |

## Constitutional anchors

- **§107 / §11.4 end-user-usability covenant** — every HTTP/3 + Brotli + TOON PASS MUST carry positive runtime evidence on the wire: real QUIC handshake observed (`quic-go` client connects + receives `application/toon` or `application/json` body with `Content-Encoding: br`), real Brotli magic-bytes assertion (`0xCE 0xB2 0xCF 0x81`... — actually Brotli is stream-aligned so we assert *decompressibility + smaller-than-gzip*), real TOON round-trip (marshal → unmarshal → deep-equal). Metadata-only PASS forbidden. Section 6 enumerates the 15+ new invariants.
- **§11.4.74 catalogue-check** — performed BEFORE drafting. Three matching submodules already exist in `vasic-digital/`:
  - `vasic-digital/http3@1d0df7b` — `digital.vasic.http3` — quic-go/http3 wrapper (HEAD 2026-05-20). **Disposition: REUSE.** Currently NOT vendored by Herald — needs to be added under `submodules/http3/` and referenced via `go.mod` replace directive (same pattern as the 9 existing Helix-stack submodules).
  - `vasic-digital/middleware@ea68252` — `digital.vasic.middleware/pkg/brotli` + `pkg/altsvc` (already vendored at `submodules/middleware/`). **Disposition: REUSE + minor extend.** Both packages exist and compile; need to verify Gin adapter handles streaming-write semantics (pkg/gin/gin.go wraps net/http middleware — current brotli middleware buffers entire body which is fine for Herald's small responses but should be measured).
  - `vasic-digital/TOON@fc2ab55` — `digital.vasic.toon` — currently SCAFFOLD with `ErrTOONEncodingNotImplemented` sentinel everywhere. Upstream reference impl is `toon-format/toon-go` (137 stars, MIT licensed, community-driven, official spec at `toon-format/spec` 291 stars, parent format spec at `toon-format/toon` 24,335 stars). **Disposition: EXTEND** — wire `toon-format/toon-go` as the real encoder inside `digital.vasic.toon` and replace the sentinel-error API surface (which is honestly documented as bluff-avoidance, NOT a behavioural contract to preserve).
- **§11.4.70 subagent-driven default** — implementation via `superpowers:subagent-driven-development`; one subagent per Wave 4a/4b task; spec + code review per task.
- **§11.4.73 spec versioning** — V3 r9 → r10 captures transport extension. Section 8 enumerates the §41 + §5.7 + new §41.7 (Transport) deltas.
- **§11.4.78 CodeGraph** — `commons_transport/` (the new module, if Approach 2 in Section 3.3 is chosen) becomes the 15th workspace module; included in CodeGraph index per §11.4.78. Submodules vendored under `submodules/http3/` are excluded per the `.codegraph/config.json` mandate.
- **§11.4.79 / §11.4.80** — workspace `go.work` accounting: 14 → 15 modules (if `commons_transport/` lands as a new module). The 7 Wave-2 added flavor binaries continue to compose via the same `cli.ServeOpts` hook — we are NOT forking the serve plane per flavor.
- **CONST-051 decoupling** — extensions to `digital.vasic.http3`, `digital.vasic.middleware`, `digital.vasic.toon` MUST remain Herald-agnostic. Any Herald-specific behaviour (e.g., the "TOON default with JSON fallback" content-negotiation policy) lives inside `commons_transport/` or the flavor's `internal/http/`, NEVER inside the vendored submodule.
- **CONST-054 helix-deps** — vendoring `digital.vasic.http3` requires writing/updating `helix-deps.yaml` declarations across the dependency chain. No nested own-org submodule chains permitted (CONST-051(C)).

## Table of contents

- [Section 1 — Current state survey (Herald REST surface as-built)](#section-1--current-state-survey-herald-rest-surface-as-built)
- [Section 2 — Submodule survey + extension scope](#section-2--submodule-survey--extension-scope)
- [Section 3 — Architecture options + recommendation (HTTP/3 listener strategy)](#section-3--architecture-options--recommendation-http3-listener-strategy)
- [Section 4 — Compression negotiation design](#section-4--compression-negotiation-design)
- [Section 5 — TOON adoption design](#section-5--toon-adoption-design)
- [Section 6 — Anti-bluff testing strategy](#section-6--anti-bluff-testing-strategy)
- [Section 7 — Sequencing + wave-split recommendation](#section-7--sequencing--wave-split-recommendation)
- [Section 8 — Spec V3 r9 → r10 impact](#section-8--spec-v3-r9--r10-impact)
- [Section 9 — Open questions for operator](#section-9--open-questions-for-operator)
- [Section 10 — Catalogue-Check verdict](#section-10--catalogue-check-verdict)

---

## Section 1 — Current state survey (Herald REST surface as-built)

After Wave 3a/3b the active REST plane consists of three live + one stub route across four flavor binaries. All four bind a Gin engine through the shared `commons/cli` scaffold (`commons/cli/serve.go` — `ServeCmd` ➜ `gin.New()` ➜ `srv.ListenAndServe()`). The complete inventory:

| Flavor | Port (default) | Route | Status | Owner file |
|---|---|---|---|---|
| `pherald` | 24791 (per Wave 2 branding) | `POST /v1/events` | LIVE (Wave 3b — Runner-backed) | `pherald/internal/http/routes.go` |
| `cherald` | 24792 | `GET /v1/compliance` | LIVE (Wave 3a) | `cherald/internal/http/routes.go` ➜ `cherald/internal/compliance.Handler` |
| `sherald` | 24793 | `GET /v1/safety_state` | LIVE (Wave 3a) | `sherald/internal/http/routes.go` ➜ `sherald/internal/safety.Handler` |
| `iherald` | 24794 | `POST /v1/webhooks/page` | 501 stub (HRD-024) | `iherald/internal/http/routes.go` |
| All flavors | each flavor's port | `GET /v1/healthz`, `GET /v1/readyz`, `GET /metrics` | LIVE (built-in via `commons/cli/routes.go`) | `commons/cli/routes.go` |

### Shared engine wiring (the single integration seam)

`commons/cli/serve.go:40` — `ServeCmd(opts ServeOpts) *cobra.Command`. The function:

1. Sets `gin.SetMode(gin.ReleaseMode)`.
2. Constructs the engine: `r := gin.New(); r.Use(gin.Recovery())`.
3. Registers built-in observability routes (healthz/readyz/metrics) BEFORE applying flavor middleware — explicitly so probes work without auth.
4. Applies the per-flavor `opts.Middleware` slice (currently used by Wave 3a for `commons_auth.GinMiddleware` JWT verification).
5. Iterates `opts.Routes` and registers each; nil-handler + non-empty HRD entries get `StubRouteHandler` (501).
6. Binds `&http.Server{Addr, Handler: r}` and calls `srv.ListenAndServe()` (TCP, plain HTTP/1.1+HTTP/2).
7. Graceful shutdown on SIGTERM/SIGINT/context-cancel.

**Critical for this design:** every flavor's serve subcommand passes through `ServeCmd`. There is exactly ONE place to add HTTP/3 support and exactly ONE place to add response-compression + content-negotiation middleware. We do NOT need to touch every flavor's `cmd/<flavor>/main.go` or `internal/http/routes.go` to land Wave 4 — the work happens almost entirely in `commons/cli/serve.go` + a new `commons_transport/` module (Approach 2 below) or directly inside `commons/cli/` (Approach 1 below).

### What is NOT in scope today

- **No HTTP/3.** `srv.ListenAndServe()` is plain TCP. No UDP listener. No Alt-Svc header advertised.
- **No response compression.** No gzip, no Brotli, no Accept-Encoding inspection. Every body goes out uncompressed (which is fine for Wave 2/3 because the bodies are ≤ 2 KiB JSON).
- **No content negotiation.** Every handler calls `c.JSON(...)` (Gin's hard-coded JSON encoder backed by `encoding/json` — or sonic if the build tag is set). There is no Accept-header inspection, no path to switch encoders.
- **No TLS.** Wave 2/3 serve subcommands listen plaintext; TLS is delegated to an upstream reverse proxy or sidecar in deployment. HTTP/3 mandates TLS 1.3 on the wire, so adding HTTP/3 forces TLS termination INTO the Herald binary (or onto a sidecar; Section 9 Q3 calls this out).

### Touch list for Wave 4

| Path | Action |
|---|---|
| `commons/cli/serve.go` | Add HTTP/3 listener wiring (Approach A in §3) or delegate to `commons_transport.Serve()` (Approach B). |
| `commons/cli/routes.go` | No change required — every handler is content-type-agnostic at this layer; the encoder swap happens via middleware (§4 + §5). |
| `commons_transport/` (NEW module, if Approach 2 chosen) | New foundation module owning: HTTP/3 listener + TLS-1.3 cert sourcing, Brotli+gzip middleware, content-negotiation middleware, TOON+JSON encoder shim, Alt-Svc advertiser. Module name `github.com/vasic-digital/herald/commons_transport`. |
| `pherald/internal/http/routes.go` etc. | Zero change — same `cli.Route` slice; the swap to TOON happens by replacing `c.JSON()` calls with a `respond.Negotiate(c, payload)` helper in `commons_transport/` (or via response-encoding middleware that overrides Gin's writer). Section 5.2 details the exact strategy. |
| `submodules/http3/` (NEW vendored submodule) | New git submodule pointing at `git@github.com:vasic-digital/http3.git`. Referenced via `replace` directive in `commons_transport/go.mod` (or in each flavor's `go.mod` if we keep Approach 1). |
| `submodules/middleware/` (existing) | No source change in the submodule. We CONSUME the existing `pkg/brotli`, `pkg/altsvc`, `pkg/gin` packages. If we discover gaps (streaming, level-tuning beyond `DefaultCompression`), the extension lives upstream in the submodule. |
| `submodules/TOON/` (NEW vendored submodule) | New git submodule pointing at `git@github.com:vasic-digital/TOON.git`. EXTENSION required: replace `ErrTOONEncodingNotImplemented` sentinel with a thin facade over `toon-format/toon-go`. Section 5.3 + Wave 4b detail. |
| `scripts/e2e_bluff_hunt.sh` | +~15 invariants for Wave 4a (HTTP/3 + Brotli + Alt-Svc + content negotiation), +~7 invariants for Wave 4b (TOON + JSON fallback + round-trip). |
| `tests/test_wave2_mutation_meta.sh` (or a new `test_wave4_mutation_meta.sh`) | +~5 paired mutations: strip-H3-listener-fails, strip-Brotli-fails, force-JSON-when-TOON-requested-fails, mismatched-content-type-fails, missing-TLS-cert-fails. |
| `docs/specs/mvp/specification.V3.md` | r9 → r10 adds §41.7 Transport subsection + amends §5.7 to declare HTTP/3 primary + §9 stack additions. |
| `docs/guides/HTTP3_BROTLI_TOON.md` | NEW operator guide (deployment, TLS cert provisioning, client-side how-to). |
| `docs/INTEGRATION.md` | NEW (or extend existing) — client-side recipes (curl with `--http3`, h2load benchmarks, Go client snippet using `digital.vasic.http3` consumer-side). |
| `submodules/{http3,middleware,TOON}/CLAUDE.md` / `AGENTS.md` / `QWEN.md` | Verify §107 anti-bluff anchor present in each. Already verified for `middleware/CLAUDE.md` and `TOON/README.md` during this survey. Cross-check during the Wave 4a `T1` cataloguing task. |

---

## Section 2 — Submodule survey + extension scope

The §11.4.74 catalogue check returned **three matches** in `vasic-digital/`. Full disposition:

### 2.1 `vasic-digital/http3` — `digital.vasic.http3`

- **URL:** `git@github.com:vasic-digital/http3.git`
- **HEAD:** `1d0df7b700436b70a361c3ba14d0520b070e7df9` (2026-05-20)
- **Module path:** `digital.vasic.http3`
- **Status:** Working implementation. `pkg/server/server.go` wraps `quic-go/http3.Server` with TLS-1.3 validation, ALPN h3 NextProtos injection, lifecycle (`Start/Shutdown/Done/Addr`), and `Config.Validate()`. Includes `internal/testcert` for tests, `integration_test.go` with `freeUDPPort()` + `h3Client()` helpers, fuzz tests, challenges.
- **Vendored in Herald today?** NO. Not in `submodules/` and not in `go.work`. Must be added in Wave 4a T1.
- **Disposition: REUSE.** The submodule's API is exactly what Herald needs: pass `net/http.Handler` (Gin satisfies this), get back a server with `Start/Shutdown`. No source change required for Wave 4a's basic dual-listener pattern.
- **Possible future extension** (NOT Wave 4 scope): expose connection-level QUIC metrics (RTT, packet loss, congestion window) for `digital.vasic.observability` to scrape. The submodule README states "There is no middleware, no logging, no metrics — those belong in their own modules and compose at the `http.Handler` level" — so observability integration goes via `digital.vasic.observability` middleware running atop the handler, NOT inside this submodule. Defer to a future HRD.

### 2.2 `vasic-digital/middleware` — `digital.vasic.middleware`

- **URL:** `git@github.com:vasic-digital/middleware.git`
- **HEAD:** `ea682529127e3fdd986f53868ebcb79c3cfe61d8` (current local pin)
- **Vendored in Herald today?** YES — `submodules/middleware/`.
- **Relevant packages:**
  - `pkg/brotli/brotli.go` (118 lines) — `Config{Level, MinLength, CompressibleTypes}` + `New(cfg) func(http.Handler) http.Handler`. Reads `Accept-Encoding: br`, buffers body, encodes only if body ≥ `MinLength` AND content-type matches a compressible prefix. Sets `Content-Encoding: br`, adds `Vary: Accept-Encoding`, strips `Content-Length`. Default config includes `application/json`. **Will need to add `application/toon` to default `CompressibleTypes`** when Wave 4b lands (one-line change upstream).
  - `pkg/altsvc/altsvc.go` (41 lines) — `Config{Enabled, H3Port, MaxAge}` + `New(cfg) func(http.Handler) http.Handler`. Sets `Alt-Svc: h3=":<port>"; ma=<max-age>` header on every response. Default port `:443`, default ma `86400`.
  - `pkg/gin/gin.go` (19 lines) — `Wrap(middleware func(http.Handler) http.Handler) gin.HandlerFunc`. Bridges net/http middleware into Gin. Already what we need to wire Brotli + Alt-Svc into the `commons/cli/serve.go` engine.
- **Disposition: REUSE.** No source extensions required for Wave 4a. Wave 4b touches one default constant (+`application/toon` in `DefaultConfig().CompressibleTypes`) — that's a clean upstream PR + pointer bump in the same commit cycle per CONST-051(B) (no Herald-specific context introduced; TOON is a public registered content-type via `toon-format/spec`).
- **Anti-bluff verification gap to fill:** the existing `pkg/brotli/brotli_test.go` + `brotli_coverage_test.go` tests assert headers + status. Wave 4a should add a paired-mutation gate that *deliberately disables the Brotli encoder* (e.g., set `MinLength: 1 << 62`) and asserts the e2e response is uncompressed — confirming the gate fires only when Brotli actually compressed. This lives in Herald's `tests/test_wave4_mutation_meta.sh`, not upstream.

### 2.3 `vasic-digital/TOON` — `digital.vasic.toon`

- **URL:** `git@github.com:vasic-digital/TOON.git`
- **HEAD:** `fc2ab55729436402081f90c11b6bd73119c00837` (2026-05-20)
- **Module path:** `digital.vasic.toon`
- **Vendored in Herald today?** NO.
- **Current status:** SCAFFOLD with `ErrTOONEncodingNotImplemented` sentinel returned by every entry point (`Marshal`, `Unmarshal`, `MarshalIndent`, `Encoder.Encode`, `Decoder.Decode`, `Compare`). This is *honest scaffolding* per the round-27 §11.4 audit — a previous bluff revision silently delegated to `encoding/json` while claiming TOON encoding; that bluff was reverted and the sentinel was added as an anti-regression marker. The repo also exposes `ContentType = "application/toon"`, `IsTOONContentType(header)` (string inspection), `TokenEstimate(data []byte) int` (len/4 heuristic) — these three are honest, anti-bluff-tested, and ship today.
- **Disposition: EXTEND.** The README explicitly says: *"Native TOON encoding will be wired once one of the following is available: upstream `toon-format/toon-go` library, OR an in-repo hand-written TOON encoder."* — and `toon-format/toon-go` v0.x has been published (137 stars, MIT, active). Wave 4b's first task is to **wire `toon-format/toon-go` inside `digital.vasic.toon`** as a thin facade: replace each sentinel-returning function with a delegation to `github.com/toon-format/toon-go`. The submodule retains its `IsTOONContentType` + `TokenEstimate` helpers + `ContentType` constant. Upstream PR + pointer bump in Wave 4b T1.
- **Anti-bluff guarantee** (per the submodule's own README clause 6): the existing `--anti-bluff-mutate` Challenge mode must continue to pass. After Wave 4b's integration, `mutate` deliberately switches the underlying encoder to a no-op and asserts the Challenge FAILS — the same shape as the current sentinel test.

### 2.4 What is NOT in the catalogue (no-match — vendor new code)

- **`commons_transport/`** — Herald-specific composition layer. Combines `digital.vasic.http3` (listener) + `digital.vasic.middleware/pkg/brotli` + `pkg/altsvc` + `digital.vasic.toon` (codec) + a content-negotiation policy. The policy itself — "TOON default with JSON fallback when Accept-Encoding excludes application/toon, gzip fallback when Accept-Encoding excludes br" — is Herald-domain configuration, not a universal building block. CONST-051(B) forbids putting that policy into any of the three submodules. **`commons_transport/` is the 15th workspace module in Wave 4a.**

### 2.5 helix-deps cascade

Adding `submodules/http3/` and `submodules/TOON/` triggers CONST-054 manifest work:

- Herald's root MUST gain a project-level manifest (suggested: `helix-deps.yaml` at repo root) declaring both new submodules. Today Herald has nine vendored submodules under `submodules/` referenced via `replace` directives in consuming modules' `go.mod` — the pattern continues for http3 + TOON.
- Both upstreams already ship `helix-deps.yaml` declaring `deps: []` (leaf modules) — no nested own-org chains are introduced. CONST-051(C) compliance verified.

---

## Section 3 — Architecture options + recommendation (HTTP/3 listener strategy)

### 3.1 Option A — Dual listener inside the Herald binary (RECOMMENDED)

```
+---------------------+        +---------------------+
| net/http TCP :24791 | ---->  | gin.Engine          |
+---------------------+        |  Use(Brotli)        |
                               |  Use(AltSvc)        |
+---------------------+        |  Use(JWT)           |
| http3.Server UDP    | ---->  |  Routes(...)        |
| :24791 (same port)  |        +---------------------+
+---------------------+                 ^
                                        |
                            shared net/http.Handler
```

- The Gin engine is built ONCE inside `ServeCmd`.
- Two listeners share the handler: `srv.ListenAndServe()` (TCP, HTTP/1.1+HTTP/2) AND a `digital.vasic.http3.Server` (UDP, HTTP/3).
- Both bind to the SAME port (24791 in pherald's case) — TCP listens on tcp/24791, UDP listens on udp/24791. Standard pattern; routing across the protocol families happens at the L4 layer in the kernel.
- The HTTP/2 listener advertises HTTP/3 availability via the Alt-Svc header (`digital.vasic.middleware/pkg/altsvc`). Clients that understand Alt-Svc (curl with `--http3`, modern Chromium, modern Firefox, Go clients using `digital.vasic.http3` consumer-side) upgrade to the QUIC connection on their second request.
- HTTP/1.1+HTTP/2 fallback is automatic — clients that don't support HTTP/3 (or that hit a UDP-blocking middlebox) stay on TCP. No application-layer logic required.
- **TLS:** required for HTTP/3. Wave 4a T2 must wire a TLS cert source. Three sub-options:
  - T2.a — operator-supplied (`--tls-cert` + `--tls-key` flags or `[server.tls]` config block).
  - T2.b — `internal/testcert` style self-signed for dev (lifted from `digital.vasic.http3/internal/testcert`); see Section 9 Q3.
  - T2.c — defer cert acquisition to a sidecar or reverse-proxy (Caddy / Cloudflare) and have Herald only speak plaintext HTTP/2 internally — **violates the mandate** ("MUST work primary as http3"), so this is NOT the recommendation. Listed for completeness.
- **Trade-offs:**
  - **+** Single binary continues to be a single binary; no new sidecar deployment topology.
  - **+** Operator sees the same `<flavor>herald serve` UX; the QUIC port comes online automatically.
  - **+** Full control over Brotli + Alt-Svc + content-negotiation policy.
  - **+** Reuses `digital.vasic.http3` exactly as documented.
  - **−** Brings `quic-go` + crypto/tls into every flavor binary's dependency closure. Binary size grows by ~3-4 MiB per flavor (quic-go is well-optimized; not a deal-breaker).
  - **−** Forces every operator deployment to have a TLS cert reachable (or accept self-signed). New friction in dev. Section 9 Q3 needs operator input.

### 3.2 Option B — Cloud-edge / reverse-proxy termination

Caddy or Cloudflare terminates HTTP/3 + TLS externally and proxies HTTP/2 plaintext to Herald internally. Herald stays on the current `srv.ListenAndServe()` path.

- **Trade-offs:**
  - **+** Zero changes to Herald's binary; deployment-only swap.
  - **+** Hardware QUIC acceleration possible at the edge.
  - **−** **VIOLATES the operator mandate.** The user said: *"all REST API we have MUST work primary as http3 (quic)"* — that's the Herald binary's API. Forcing a Caddy dependency for the *primary* transport is a structural violation. Even if Herald is deployed behind Caddy in production, the Herald binary itself must speak HTTP/3 to satisfy "primary" semantically + to support direct (non-proxied) deployments (operator's laptop, CI, agent integrations).
  - **−** Breaks the §3.3 Wave-2 "container is the deployment unit" expectation in spec V3 §41.6.
- **Decision: REJECT** as the primary strategy. Caddy/Cloudflare front-ending is a valid deployment-time choice, but Herald MUST ALSO directly serve HTTP/3. (Compatible: dual listener inside Herald + Caddy frontends in production = belt-and-braces.)

### 3.3 Option C — Migrate off Gin to a framework with native HTTP/3 (REJECTED)

There is no Go HTTP framework with native HTTP/3 today that is more featureful than Gin. Fiber + fasthttp does NOT speak HTTP/3 yet (fasthttp is HTTP/1.1-only by design). chi + net/http + quic-go is the closest pattern to Approach A. Migration cost is enormous; benefit is zero. **REJECT.**

### 3.4 Where to put the new code — Approach 1 vs Approach 2

Independent of the listener strategy (A vs B vs C), there is a separate question: do we cram the HTTP/3 + Brotli + content-negotiation wiring into `commons/cli/serve.go`, or do we extract a new `commons_transport/` module?

#### Approach 1 — All inside `commons/cli/`

- Add the `digital.vasic.http3.Server` start/stop to `serve.go`.
- Wire Brotli + Alt-Svc Gin-Wrapped middleware in `serve.go` (using `digital.vasic.middleware/pkg/gin.Wrap`).
- Add Accept-Encoding inspection + TOON/JSON negotiation helper inside `commons/cli/routes.go` (sibling to `HealthzHandler` etc.).
- **+** Minimal new surface area. `go.work` stays at 14 modules.
- **−** `commons/cli/` becomes overloaded — Cobra commands + HTTP server + transport policy in one foundation module. Violates the single-responsibility convention Herald has held.
- **−** Cyclical-import risk: `commons_transport` (TOON encoder) would have to be imported by `commons/cli/`, but later flavors might want to call into `commons_transport` directly from their `internal/http/<route>.go` handlers — creates an upward import from flavor → `commons/cli/` (the routing handler hosts a logical lower-level concern in a higher-level module).

#### Approach 2 — Extract `commons_transport/` (RECOMMENDED)

- New foundation module `commons_transport/` containing:
  - `commons_transport/serve.go` — `Serve(opts ServeOpts) error` that swallows the listener split (TCP + QUIC) behind a single call. `commons/cli/serve.go`'s `ServeCmd` delegates to `commons_transport.Serve`.
  - `commons_transport/middleware.go` — Brotli + Alt-Svc + content-negotiation middleware factories. Pre-baked Gin handler chains.
  - `commons_transport/codec.go` — `Negotiate(c *gin.Context, payload any) error` helper that inspects `Accept` and writes TOON or JSON.
  - `commons_transport/tls.go` — cert sourcing (file flags + dev self-signed via `digital.vasic.http3/internal/testcert` lifted into a public `commons_transport/testtls/`).
- **+** Single-responsibility honored: `commons/cli/` = Cobra glue; `commons_transport/` = wire format + listener.
- **+** Flavor handlers can call `commons_transport.Negotiate(c, payload)` directly without dragging Cobra in.
- **+** Mirrors the §10 Commons layered architecture: `commons` → `commons_transport` → `commons/cli` ← used by flavor `cmd/`.
- **+** Cleaner test surface: `commons_transport/` gets its own focused tests in addition to the e2e suite.
- **−** 15th workspace module. Minor docs sync (§11.4.79/.80 module-count enumeration in CLAUDE.md + the spec).
- **−** One more git submodule reference + one more `replace` directive to add.

**RECOMMENDATION:** **Approach 1A** (dual listener INSIDE Herald) + **Approach 2** (extract `commons_transport/` as the 15th workspace module). Together they yield the cleanest layering, satisfy the operator mandate, and minimize cross-flavor churn.

---

## Section 4 — Compression negotiation design

### 4.1 Negotiation policy

Quoted from the operator: *"Brotli as default for compression. Fallback to ... regular compression if needed."* — meaning:

| Client `Accept-Encoding` includes ... | Server applies |
|---|---|
| `br` | Brotli (default level = `brotli.DefaultCompression` = quality 5) |
| `gzip` (but not `br`) | gzip (default level = `flate.DefaultCompression` = 6) |
| Neither (or `identity`) | No compression — pass body through |
| Body length < `MinLength` (default 256 bytes) | No compression (avoid overhead for tiny bodies) |
| Content-Type ∉ `CompressibleTypes` | No compression (binary payloads stay binary) |

`digital.vasic.middleware/pkg/brotli` already implements the Brotli leg. For gzip we either:
- (4.1.a) reuse `gin-contrib/gzip` (already a popular Gin community package), OR
- (4.1.b) author a `digital.vasic.middleware/pkg/gzip` upstream (small — ~80 LOC mirroring `pkg/brotli`).

**Recommendation:** 4.1.b — write a sibling `pkg/gzip` upstream. Keeps the dependency surface fully under `vasic-digital/`, mirrors `pkg/brotli`'s shape, supports CONST-051(B) decoupling. Extension PR sized ~80 LOC + tests + Challenge.

### 4.2 Wiring inside `commons_transport/`

```go
// commons_transport/middleware.go (sketch)
func ResponseCompression(cfg CompressionConfig) gin.HandlerFunc {
    brotli := brotli.New(cfg.Brotli)
    gzip   := gzip.New(cfg.Gzip)
    return func(c *gin.Context) {
        ae := c.GetHeader("Accept-Encoding")
        switch {
        case strings.Contains(ae, "br"):
            ginadapter.Wrap(brotli)(c)
        case strings.Contains(ae, "gzip"):
            ginadapter.Wrap(gzip)(c)
        default:
            c.Next()  // identity
        }
    }
}
```

The negotiation lives in one place; the underlying compression libs (`pkg/brotli`, `pkg/gzip`) stay project-agnostic.

### 4.3 Anti-bluff verification

The test that proves "Brotli was actually applied" CANNOT be a header check alone — a buggy middleware that sets `Content-Encoding: br` while writing identity bytes is a §11.4 PASS-bluff. The test MUST:

1. Read the response body's raw bytes.
2. Attempt to decompress with `andybalholm/brotli.NewReader`.
3. Assert decompressed bytes equal the expected payload (round-trip).
4. Assert the wire body is shorter than the same payload uncompressed AND shorter than the same payload gzipped (Brotli typically wins at quality 5+ for JSON of 256 B – 8 KiB; this is the size class of Herald's actual responses).

The same shape applies to gzip: decompress + round-trip + size assertion.

### 4.4 MinLength + content-type filtering

Default `MinLength: 256` is reasonable for Herald — `/v1/healthz` returns ~80 bytes (no compression), `/v1/safety_state` returns ~600-1500 bytes (compressed), `/v1/compliance` paginated list can be 4-10 KiB (compressed). `/metrics` should NOT be compressed at the Brotli layer because Prometheus scrapers expect identity by default (and most scrapers don't ask for `Accept-Encoding: br`); the existing default config skips uncompressible types correctly.

---

## Section 5 — TOON adoption design

### 5.1 What is TOON?

From the discovered `vasic-digital/TOON` + the upstream `toon-format/toon` (24,335 stars):

**Token-Oriented Object Notation** is a compact, human-readable text format designed to reduce LLM prompt tokens for the same structured data. It's NOT a binary format — it's a column-tabular reframing of JSON.

```
# JSON (78 chars after whitespace strip):
{"users":[{"id":1,"name":"Alice","role":"admin"},{"id":2,"name":"Bob","role":"user"}]}

# TOON (53 chars):
users[2]{id,name,role}:
  1,Alice,admin
  2,Bob,user
```

The format spec lives at `toon-format/spec` (291 stars). The Go implementation is `toon-format/toon-go` (137 stars). The Content-Type registration is `application/toon`. Token savings advertised in the upstream README: 30-60% for structured collection data.

Herald cares about TOON because:
1. The operator mandate explicitly requires it as the primary data type.
2. Several Herald consumers ARE LLM agents (`/v1/events` ingest from AI subscribers per §7.5; Claude Code dispatcher per §12.x).
3. The token savings compound — every event a sherald agent reads is fewer tokens in the agent's context window.

### 5.2 Wiring strategy

The handler-side change is small. Today every handler writes JSON via `c.JSON(status, payload)`. After Wave 4b, handlers write `commons_transport.Negotiate(c, status, payload)`:

```go
// commons_transport/codec.go (sketch)
func Negotiate(c *gin.Context, status int, payload any) error {
    accept := c.GetHeader("Accept")
    switch {
    case wantsTOON(accept):
        b, err := toon.Marshal(payload)
        if err != nil { return err }
        c.Data(status, toon.ContentType, b)        // "application/toon"
        return nil
    default:
        c.JSON(status, payload)
        return nil
    }
}

func wantsTOON(accept string) bool {
    // Explicit TOON preference OR `*/*` with TOON-server-default flag set
    return strings.Contains(accept, toon.ContentType) ||
           (strings.Contains(accept, "*/*") && serverDefaultsTOON)
}
```

`serverDefaultsTOON` is configurable per the operator mandate ("Primary data structure / type should be Toon instead of JSON, however JSON shall be supported as fallback or as an option"). Default `true` in Herald; clients can force JSON by sending `Accept: application/json`.

### 5.3 Upstream extension scope (`digital.vasic.toon`)

Wave 4b T1 PR against `vasic-digital/TOON`:

- Add `github.com/toon-format/toon-go` as a runtime dependency.
- Replace each `return ErrTOONEncodingNotImplemented` with `return toongo.Marshal(...)` / equivalent delegation.
- Preserve `ContentType`, `IsTOONContentType`, `TokenEstimate` unchanged.
- Update `pkg/toon/toon_test.go` to assert real round-trip behaviour (the sentinel-error tests become reverse — assert NO sentinel returned and a real TOON document is produced).
- Update `challenges/scripts/toon_describe_challenge.sh` `--anti-bluff-mutate` mode: planted bluff is now "swap encoder to return identity JSON" and the gate asserts FAIL when planted.
- Bump version + README explanatory edit (the "STATUS: PENDING_IMPLEMENTATION" section flips to "STATUS: IMPLEMENTED via toon-format/toon-go vX.Y").

CONST-051(B) check: nothing Herald-specific lands in the submodule. The Negotiate policy stays in `commons_transport/`.

### 5.4 Wave 4b non-goals (deferred)

- TOON-encoded REQUEST bodies (clients posting `application/toon`). Wave 4b ships RESPONSE TOON only. Inbound CloudEvents stay JSON because that's the §4.1 CloudEvents wire spec and we don't unilaterally extend a registered IANA media type. Future HRD when toon-format/spec defines an `Application/cloudevents+toon` profile.
- Schema-aware TOON length markers (`toon.WithLengthMarkers(true)` from the upstream README). Useful for streaming, optional, defer to a future HRD.
- TOON encoding for `/metrics` Prometheus output. Prometheus has its own text format; we don't violate that.

### 5.5 Fallback chain (operator mandate)

> *"JSON shall be supported as fallback or as an option."*

Implemented as a clean four-rung ladder:

1. **Client preference** — explicit `Accept: application/toon` wins TOON; explicit `Accept: application/json` wins JSON.
2. **Server default** — if `Accept: */*` AND `serverDefaultsTOON = true` (Herald's default), TOON wins.
3. **Encoding failure** — if `toon.Marshal` returns a non-sentinel error (e.g., unsupported type), `Negotiate` falls back to JSON with a logged warning and an `X-Herald-Codec-Fallback: 1` header. The header lets observability + audit detect silent fallback.
4. **Encoder unavailable** — if `digital.vasic.toon` is built with a build tag that strips the encoder (`-tags=no_toon`), every call uses JSON. The build tag exists for tiny-binary deployments.

Anti-bluff: every encoder fallback emits a Prometheus counter `<flavor>_codec_fallback_total{reason="<rung>"}` and an OTel span event. Tests assert the counter increments under each rung's trigger.

---

## Section 6 — Anti-bluff testing strategy

The §107 covenant requires positive runtime evidence. We add ~15 new e2e invariants (Wave 4a) + ~7 more (Wave 4b) to `scripts/e2e_bluff_hunt.sh`, plus ~5 paired mutation gates.

### 6.1 Wave 4a e2e invariants (~15 new — E49 .. E63)

| Tag | Invariant | Evidence captured |
|---|---|---|
| E49 | `digital.vasic.http3` Go module imports cleanly in `commons_transport/` (`go list -m all` includes it). | `go list` output. |
| E50 | `commons_transport.Serve` starts a UDP listener on the configured port. | `netstat -anp udp` shows the port; `ss -ulnp` (Linux) or `lsof -iUDP -P` (macOS). |
| E51 | `commons_transport.Serve` starts a TCP listener on the SAME port. | Same family of probes against TCP. |
| E52 | `curl --http3 -k https://127.0.0.1:24791/v1/healthz` returns 200 + flavor JSON. **Real HTTP/3 handshake**, not a 1.1 fallback. | curl `--verbose --http3` output capturing `* HTTP/3 SSL connection ...`; response body asserted equals expected JSON. |
| E53 | `curl --http2 -k https://127.0.0.1:24791/v1/healthz` returns 200 over HTTP/2 (TCP fallback). | curl `--http2 --verbose` output capturing `using HTTP/2`. |
| E54 | HTTP/2 response Alt-Svc header advertises `h3=":24791"`. | Header dump from curl. |
| E55 | `curl -H "Accept-Encoding: br" ...` response is Brotli-encoded. | Body length < uncompressed; `brotli.NewReader(body).ReadAll()` returns expected JSON; magic bytes / decompressibility asserted. |
| E56 | Same call as E55 but small body (`/v1/healthz` ~80 B) does NOT compress (below `MinLength`). | Response `Content-Encoding` header absent; body == identity JSON. |
| E57 | `curl -H "Accept-Encoding: gzip" ...` response is gzip-encoded + decompresses to expected JSON. | `gzip.NewReader(body).ReadAll()` round-trip. |
| E58 | `curl -H "Accept-Encoding: identity" ...` returns uncompressed. | `Content-Encoding` absent; body == identity JSON. |
| E59 | Brotli body is SHORTER than gzip body for the same JSON payload of ≥ 1 KiB (real compression benefit, not just header presence). | Numeric size comparison from captured bodies. |
| E60 | TLS cert chain presented by Herald passes TLS 1.3 validation against the dev CA. | `openssl s_client -connect 127.0.0.1:24791 -tls1_3 -alpn h3` returns success. |
| E61 | Operator-supplied cert via `--tls-cert` flag overrides dev cert. | curl with `--cacert <operator-ca>` succeeds. |
| E62 | `cherald` + `sherald` + `pherald` all serve HTTP/3 — each binary independently. | Per-flavor sub-block running E50/E52/E55. |
| E63 | Wire-level proof of UDP traffic during an HTTP/3 request. | `tcpdump -i lo udp port 24791 -c 5 -w /tmp/h3.pcap` during the curl call; PCAP has ≥ 5 UDP packets to/from 24791. |

### 6.2 Wave 4b e2e invariants (~7 new — E64 .. E70)

| Tag | Invariant | Evidence captured |
|---|---|---|
| E64 | `digital.vasic.toon.Marshal(payload)` produces non-empty TOON bytes (not a sentinel error) after the upstream PR lands. | `len(b) > 0`; `err == nil`. |
| E65 | TOON round-trip — `Unmarshal(Marshal(p)) == p` for the actual `/v1/compliance` paginated row payload (real production type). | `reflect.DeepEqual` after round-trip. |
| E66 | `curl -H "Accept: application/toon" /v1/compliance` returns 200 + `Content-Type: application/toon` + TOON-decodable body. | Decoder round-trip to the original Go struct. |
| E67 | `curl -H "Accept: application/json" /v1/compliance` returns 200 + `Content-Type: application/json` + valid JSON. | `json.Unmarshal` succeeds. |
| E68 | `curl -H "Accept: */*" /v1/compliance` returns 200 + `Content-Type: application/toon` (server default = TOON). | Same as E66. |
| E69 | TOON body is shorter than JSON body for the same payload (≥ 200 bytes savings on /v1/compliance with 10 rows). | Numeric size comparison. |
| E70 | Codec fallback counter increments when a payload that the encoder rejects triggers JSON fallback. | `<flavor>_codec_fallback_total{reason="encoder_error"}` increments + response carries `X-Herald-Codec-Fallback: 1`. |

### 6.3 Paired-mutation gates (`tests/test_wave4_mutation_meta.sh` — new ~5 mutations)

Each mutation is applied to a SCRATCH copy of the source tree, then `e2e_bluff_hunt.sh` runs against the mutated tree and MUST FAIL on the indicated invariants. A mutated tree that still PASSES is a §11.4 PASS-bluff in the gate itself.

| Mutation | Targets | Expected to fail |
|---|---|---|
| M1 — Strip HTTP/3 listener | comment out `commons_transport.startQUIC()` call | E50, E52, E54, E62, E63 |
| M2 — Strip Brotli middleware | comment out `brotli.New(...)` wrapper | E55, E59 |
| M3 — Force JSON when TOON requested | hard-code `wantsTOON()` to return `false` | E66, E68 |
| M4 — Mismatched Content-Type header | swap `c.Data(status, toon.ContentType, b)` to `c.Data(status, "application/json", b)` | E66 (decoder is content-type sniffing; mismatched type breaks the explicit handshake) |
| M5 — Skip TLS-1.3 mandate | downgrade `tls.VersionTLS13` to `tls.VersionTLS12` | E60 (handshake refuses); E52 (HTTP/3 client refuses non-1.3 ALPN h3) |

All five must FAIL on planted mutations and PASS on clean. The meta-test asserts this two-mode behaviour per §1.1.

### 6.4 Real-services discipline (CONST-050(B))

No mocks. Every e2e invariant runs against:
- A real `<flavor>herald serve` process (built from current source).
- A real `digital.vasic.http3` server (no stub).
- A real `andybalholm/brotli` codec (no stub).
- A real `toon-format/toon-go` codec (Wave 4b only).
- A real OS UDP socket (verified via `tcpdump` / `ss` / `lsof`).

Unit tests for codec edge cases live separately under each module's `*_test.go` per CONST-050(A); they are NOT a substitute for the e2e suite.

---

## Section 7 — Sequencing + wave-split recommendation

### 7.1 Comparison to prior waves

| Wave | Tasks | Substrate scope | LoC delta (rough) |
|---|---|---|---|
| Wave 2 | 15 | 6 new flavor binaries + branding + `commons/cli/` scaffold | ~3000 |
| Wave 3a | 8 | `commons_auth` + cherald/sherald live + Mode-ladder PG | ~1800 |
| Wave 3b | 10 | pherald Runner + `/v1/events` live + HRD-011 close | ~2500 |
| Wave 4a (proposed) | 10 | `commons_transport` + http3 + Brotli + Alt-Svc + TLS + middleware-wire + 15 e2e | ~2000 |
| Wave 4b (proposed) | 7 | TOON upstream PR + Herald codec wiring + `/v1/*` content-negotiation + 7 e2e | ~1200 |

Wave 4 as a single wave would total ~17 tasks and is too large for one cycle.

### 7.2 Why split

1. **Independent operator value.** Wave 4a (HTTP/3 + Brotli) ships on its own — clients get faster transport + smaller bodies even before TOON lands. The user explicitly framed compression and transport as ONE concern and TOON as a separate concern ("Primary data structure / type should be Toon instead of JSON").
2. **Independent risk.** Wave 4a's risk is in TLS cert sourcing (Section 9 Q3) — operator decisions live there. Wave 4b's risk is in the `digital.vasic.toon` upstream PR + `toon-format/toon-go` integration stability — independent risk surface.
3. **Independent dependencies.** `digital.vasic.http3` is production-ready today; `digital.vasic.toon` needs an upstream PR before Herald can consume the real encoder. Splitting prevents Wave 4a from blocking on Wave 4b's upstream.
4. **Mutation gate sizing.** Wave 4a's M1+M2+M5 are infrastructure mutations (listener + middleware + TLS). Wave 4b's M3+M4 are codec mutations. Keeping the meta-test additions per-wave lets them ship in the same commit as the gate they protect.

### 7.3 Wave 4a task sketch (10 tasks)

| # | Task | Anti-bluff evidence |
|---|---|---|
| T1 | Catalogue-Check (§11.4.74) + add `submodules/http3/` (NEW) — git submodule, `replace` in `commons_transport/go.mod`. | `submodules/http3/` exists with expected SHA; `go list` resolves. |
| T2 | Create `commons_transport/` 15th workspace module — `serve.go`, `tls.go`, `cert sourcing`, dev self-signed cert helper lifted to public `commons_transport/testtls/`. | Module compiles standalone; unit tests pass; `go.work` updated. |
| T3 | Wire `digital.vasic.http3.Server` as the QUIC listener inside `commons_transport.Serve`. Dual TCP + UDP listener on same port. | E50, E51 PASS against a built `pherald serve`. |
| T4 | Wire `digital.vasic.middleware/pkg/brotli` + `pkg/gzip` (new upstream PR for gzip per §4.1) + `pkg/altsvc` via `pkg/gin.Wrap` into the response middleware chain. | E55, E57, E54 PASS. |
| T5 | `commons_transport.ResponseCompression` negotiator that picks br / gzip / identity from `Accept-Encoding`. Default MinLength 256, default level Brotli quality 5. | E55, E56, E57, E58 PASS; E59 PASS (Brotli < gzip). |
| T6 | `commons/cli/serve.go` delegates to `commons_transport.Serve`. Backwards-compatible — existing `ServeOpts` continues to work. | Wave 3a/3b e2e (E1..E48) remain PASS; no regressions. |
| T7 | All four serving flavors (`pherald`, `cherald`, `sherald`, `iherald`) verified end-to-end with HTTP/3 + Brotli. | E62 PASS for all four. |
| T8 | TLS cert provisioning paths — operator flag, env var, dev self-signed. | E60, E61 PASS. |
| T9 | `e2e_bluff_hunt.sh` E49 .. E63 (15 new invariants) + paired meta-test mutations M1, M2, M5. | Gate PASSes clean, FAILs each mutation. |
| T10 | Spec V3 r9 → r10 — §41.7 Transport subsection added; CLAUDE.md / AGENTS.md / QWEN.md module-count `14 → 15` updated; `docs/guides/HTTP3_BROTLI_TOON.md` operator guide drafted; `docs/INTEGRATION.md` extended with HTTP/3 client recipes; PDF/HTML/DOCX siblings regenerated. | Inheritance gate + audit_antibluff PASS; sibling artefacts diff matches. |

### 7.4 Wave 4b task sketch (7 tasks)

| # | Task | Anti-bluff evidence |
|---|---|---|
| T1 | Upstream PR against `vasic-digital/TOON` — wire `toon-format/toon-go` behind the existing `Marshal/Unmarshal/Encoder/Decoder` API; replace sentinel with real codec; preserve `ContentType` + `IsTOONContentType` + `TokenEstimate`; update tests + Challenge. | PR merged; `digital.vasic.toon` pointer bumped. |
| T2 | Add `submodules/TOON/` to Herald — git submodule, `replace` in `commons_transport/go.mod`. | `go list` resolves; round-trip works in a smoke test. |
| T3 | `commons_transport/codec.go` — `Negotiate(c, status, payload)` helper that inspects Accept + writes TOON or JSON. | Unit tests of all four rungs of §5.5 ladder. |
| T4 | Replace `c.JSON(...)` calls in `cherald/internal/compliance.Handler`, `sherald/internal/safety.Handler`, `pherald/internal/runner/*.go` (Receipt response) with `commons_transport.Negotiate(c, status, payload)`. | E66, E67, E68 PASS. |
| T5 | Codec fallback observability — Prometheus counter + OTel span event + `X-Herald-Codec-Fallback` header. | E70 PASS. |
| T6 | `e2e_bluff_hunt.sh` E64 .. E70 (7 new invariants) + paired meta-test mutations M3, M4. | Gate PASSes clean, FAILs each mutation. |
| T7 | Spec V3 r10 → r11 — TOON adoption captured; operator guide extended with TOON client recipes (curl with `Accept: application/toon`, Go snippet using `digital.vasic.toon` consumer-side); PDF/HTML/DOCX siblings regenerated. | Inheritance gate + audit_antibluff PASS. |

### 7.5 Total invariant + mutation accounting

| Phase | Invariants total | Mutations total |
|---|---|---|
| Wave 3b end (today's HEAD) | 48 (E1..E48) | 4 |
| Wave 4a end | 63 (E1..E63) | 7 (M1+M2+M5 added) |
| Wave 4b end | 70 (E1..E70) | 9 (M3+M4 added) |

---

## Section 8 — Spec V3 r9 → r10 impact

V3 r9 captures Wave 3 (Runner + REST live). r10 captures Wave 4a (HTTP/3 + Brotli). r11 captures Wave 4b (TOON). Each rev bump is a secondary-version increase per §11.4.73.

### 8.1 New §41.7 Transport subsection (full text drafted in Wave 4a T10)

Outline:

- **§41.7.1 Wire protocol primary: HTTP/3 (QUIC).** Every flavor's `serve` subcommand binds a TCP listener (HTTP/1.1+HTTP/2) AND a UDP listener (HTTP/3) on the same port. TLS 1.3 is mandatory on both — HTTP/3 by spec, HTTP/2+1.1 by Herald policy. Wave 4a is the substrate; Wave 4b composes the TOON wire format on top.
- **§41.7.2 Compression default: Brotli.** Server applies Brotli when client `Accept-Encoding` includes `br`; gzip when only `gzip` is present; identity when neither. Default Brotli quality 5 (`brotli.DefaultCompression`), default MinLength 256 bytes. Skipped for binary content types.
- **§41.7.3 Content negotiation: TOON-primary with JSON fallback** (added in r11). Server prefers `application/toon` when client sends `Accept: */*` or `Accept: application/toon`; serves `application/json` when client explicitly requests JSON or when encoding fails. Codec fallback is observable.
- **§41.7.4 Alt-Svc advertisement.** Every HTTP/1.1+HTTP/2 response carries `Alt-Svc: h3=":<port>"; ma=86400` so clients can upgrade to QUIC on subsequent requests.
- **§41.7.5 TLS cert sourcing.** Operator-supplied (file flag) primary; dev self-signed via `commons_transport/testtls` for local testing; sidecar/reverse-proxy termination is a deployment-time option that does NOT obviate the Herald binary's own HTTP/3 listener.
- **§41.7.6 Backwards compatibility.** Legacy clients posting plain HTTP/1.1 with `Accept: application/json` and no `Accept-Encoding` continue to work unchanged — Herald falls back through the chain automatically. No client breakage.
- **§41.7.7 Anti-bluff posture.** Every transport feature carries positive runtime evidence in `e2e_bluff_hunt.sh` E49-E70 + paired mutations M1-M5.

### 8.2 §5.7 Ingress API URLs amendment

The existing §5.7 declares plain HTTP. Amend to reference §41.7 and clarify that the HTTP/3 listener uses the same port (no new port allocation). The `containers/quickstart/Dockerfile` may need a UDP port mapping addition — verify in Wave 4a T7.

### 8.3 §9.3 / §9.4 stack additions

Add `quic-go` + `andybalholm/brotli` + (Wave 4b) `toon-format/toon-go` to the §9 dependency catalogue. Update §9.4 container port mapping note: "Every flavor's port serves BOTH TCP (HTTP/1.1+HTTP/2) and UDP (HTTP/3)."

### 8.4 §44.M Wave-4 milestone

New §44.M sub-section "Wave 4a — HTTP/3 + Brotli transport substrate" and "Wave 4b — TOON adoption" with the as-built evidence summary (invariants passing, mutation gate behaviour, submodule pin SHAs, doc sibling regeneration confirmation).

### 8.5 CLAUDE.md / AGENTS.md / QWEN.md module-count refresh

`go.work` listing transitions: 14 → 15 (Wave 4a adds `commons_transport`) → 15 (Wave 4b adds no new workspace module). The "Project status" section in CLAUDE.md needs a one-line refresh.

---

## Section 9 — Open questions for operator

1. **Q1 — TLS cert sourcing for local dev.** HTTP/3 requires TLS 1.3. For `pherald serve` on a developer laptop, do we (a) auto-generate a self-signed cert on first run + store under `~/.herald/dev-cert.pem`, (b) require operator to provide `--tls-cert` / `--tls-key` flags every time, (c) ship a pinned dev CA in the repo (rejected — secret-leak risk per CONST-042), or (d) something else? **Recommend (a) with clear "DEV ONLY" warning logged + production deployment template referencing operator's PKI.** Decision affects Wave 4a T8.

2. **Q2 — Brotli compression level tuning.** `digital.vasic.middleware/pkg/brotli` default is `brotli.DefaultCompression` (quality 5). Quality 11 gives ~10% better ratio but ~5× CPU. For Herald's small response bodies (200 B – 10 KiB) quality 5 is the right balance; quality 11 is wasted CPU on the request hot path. Confirm 5 is acceptable, or specify a different default. **Recommend keeping quality 5.**

3. **Q3 — Production TLS termination split.** When Herald is deployed behind a Caddy / nginx / Cloudflare frontend, does Herald terminate TLS itself OR is `--insecure-no-tls` permitted on the loopback between frontend and Herald? Strict reading of the operator mandate ("all REST API we have MUST work primary as http3") says Herald MUST always be capable of speaking HTTP/3 + TLS directly — so `--insecure-no-tls` is a dev-only escape hatch and production is always-TLS. Recommend that strict reading. **Confirm or modify.**

4. **Q4 — TOON encoding for the Receipt object returned by `POST /v1/events`.** The Runner returns a Receipt body (event ID, idempotency status, fanout summary). Is the Receipt subject to TOON encoding in Wave 4b? **Recommend YES** — it's a structured, schema-stable response and is exactly the kind of payload that benefits from TOON's column compression when receipts are bulk-fetched (which spec §41.3 `GET /v1/events/{id}` permits). Confirm or scope down.

5. **Q5 (minor) — Should HTTP/3 be an opt-in flag (`--http3` or env var `HERALD_ENABLE_HTTP3=1`) in Wave 4a, or always-on by default?** The mandate says "primary" — so always-on is the literal reading. However, for testing infrastructure that can't open UDP ports (e.g., some CI runners), an env-var disable is operationally useful. **Recommend always-on with an `HERALD_DISABLE_HTTP3=1` escape hatch for legacy CI.** TCP listener never disabled (Alt-Svc fallback path stays intact).

---

## Section 10 — Catalogue-Check verdict (per Universal §11.4.74)

Survey conducted 2026-05-22 across `vasic-digital` + `HelixDevelopment` orgs on GitHub via `gh search repos`. Internet was reachable (verified via `curl -sS https://api.github.com`). Catalogue-Check field for each Wave 4 sub-deliverable:

| Sub-deliverable | Verdict | Details |
|---|---|---|
| HTTP/3 (QUIC) server | **REUSE `vasic-digital/http3@1d0df7b`** | Production-ready `quic-go/http3` wrapper. Add as `submodules/http3/`. No source change required. |
| Brotli response middleware | **REUSE `vasic-digital/middleware/pkg/brotli@ea68252`** | Already vendored at `submodules/middleware/`. Default config matches Herald's needs (will gain `application/toon` in Wave 4b T0 — one-line upstream PR). |
| gzip fallback middleware | **EXTEND `vasic-digital/middleware`** | Add new `pkg/gzip/` upstream mirroring `pkg/brotli` shape. ~80 LOC + tests + Challenge. Wave 4a T4 spawns the upstream PR. |
| Alt-Svc advertiser | **REUSE `vasic-digital/middleware/pkg/altsvc@ea68252`** | Already vendored. Wire via `pkg/gin.Wrap`. |
| TOON encoder | **EXTEND `vasic-digital/TOON@fc2ab55`** | Currently SCAFFOLD with honest sentinel errors. Wire `toon-format/toon-go` upstream as the real encoder; preserve the existing helper API (`ContentType`, `IsTOONContentType`, `TokenEstimate`). Wave 4b T1. |
| Content-negotiation policy (TOON-primary + JSON-fallback + br/gzip/identity ladder) | **NO MATCH** — new code in `commons_transport/` | Herald-specific policy composing the three submodules. CONST-051(B) prohibits putting this in any of the three. New 15th workspace module. |
| TLS cert sourcing (operator flag + dev self-signed) | **PARTIAL REUSE** of `vasic-digital/http3/internal/testcert` for the self-signed dev helper; otherwise new Herald-side code | Lift `internal/testcert` to a public `commons_transport/testtls/` (small upstream PR to publicize). |
| Observability / connection-level QUIC metrics | **DEFER** — out of Wave 4 scope | `digital.vasic.observability` is the right home for metrics; HRD opens after Wave 4b lands and operators have asked for QUIC-specific gauges. |

**Verdict summary:** 4 REUSE, 2 EXTEND (small upstream PRs), 1 NO-MATCH (new `commons_transport/` module), 1 DEFER. No duplicate implementations introduced. CONST-051 decoupling honored throughout — Herald-specific policy lives only inside Herald.

---

*End of design document. Operator review + answers to Section 9 questions are the unblockers for `superpowers:writing-plans` invocations against Wave 4a and Wave 4b separately.*
