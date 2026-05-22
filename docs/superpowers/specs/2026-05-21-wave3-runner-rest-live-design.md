<div align="center">

<img src="../../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Wave 3 — Runner + Live REST Routes Design

| Field | Value |
|---|---|
| Revision | 1 |
| Created | 2026-05-21 |
| Last modified | 2026-05-21 |
| Status | active |
| Status summary | Wave 3 of the master roadmap. Closes HRD-016 (pherald `/v1/events` Runner), HRD-028 (cherald `/v1/compliance` pull), HRD-098 (sherald `/v1/safety_state` daemon-state pull), and HRD-011 (Telegram live evidence) atomically. Lands the §32 7-stage Runner inside pherald, a shared `commons_auth/` JWT middleware (HMAC+JWKS hybrid), Redis+PG hybrid idempotency, and absorbs HRD-018 M2 (PG-backed ConstitutionStore) along the way. Workstream split into Wave 3a (substrate + 2 lighter routes, ~8 tasks) and Wave 3b (Runner + HRD-016 close-out, ~10 tasks). e2e_bluff_hunt invariants grow 33 → ~48. Spec V3 r8 → r9 captures the design. |
| Issues | none |
| Issues summary | — |
| Fixed | (n/a — design doc) |
| Continuation | After approval + spec V3 r9 update, invoke `superpowers:writing-plans` twice (once per sub-wave) to author the Wave 3a + Wave 3b implementation plans, then `superpowers:subagent-driven-development` per Universal §11.4.70 to dispatch task subagents. |

## Constitutional anchors

- **§107 end-user-usability covenant** — every Wave 3 route's "done" is positive runtime evidence captured by `scripts/e2e_bluff_hunt.sh` (real HTTP request → real PG row written → real Telegram bot reply when creds present). Compile-only PASS forbidden; 501-stub-still-returns-501 is forbidden once a route is declared live.
- **§11.4.70 subagent-driven default** — implementation via `superpowers:subagent-driven-development`; one subagent per task; spec + code review per task.
- **§11.4.74 catalogue-check** — before writing `commons_auth/`, search `vasic-digital` + `HelixDevelopment` for an existing JWT/JWKS+Gin middleware module. Disposition recorded in the Wave 3a plan.
- **§11.4.73 spec versioning** — Wave 3 design IS a spec change; V3 r8 → r9 captures it in the same commit cycle (§32 Runner stages move from "specified" to "live"; §41 routes flip from "501-stub" to "200/202").
- **§11.4.79 + §11.4.80** — `commons_auth/` becomes the 14th workspace module; included in CodeGraph index per the recently-landed mandate.

## Table of contents

- [Section 1 — Architecture + module layout](#section-1--architecture--module-layout)
- [Section 2 — Runner orchestrator + 7 stage types](#section-2--runner-orchestrator--7-stage-types)
- [Section 3 — HRD-028 cherald + HRD-098 sherald handlers](#section-3--hrd-028-cherald--hrd-098-sherald-handlers)
- [Section 4 — `commons_auth/` + idempotency substrate](#section-4--commons_auth--idempotency-substrate)
- [Section 5 — HRD-011 close-out + testing strategy](#section-5--hrd-011-close-out--testing-strategy)
- [Section 6 — Sequencing decomposition (Wave 3a + 3b)](#section-6--sequencing-decomposition-wave-3a--3b)
- [Section 7 — Spec V3 impact (r8 → r9)](#section-7--spec-v3-impact-r8--r9)
- [Section 8 — Open questions](#section-8--open-questions)
- [Section 9 — Catalogue-Check verdict](#section-9--catalogue-check-verdict)

---

## Section 1 — Architecture + module layout

Wave 3 closes the gap between Herald's "scaffold + healthz" Wave 2 deliverable and a working event-fan-out platform. Three currently-501 routes flip to live: pherald `POST /v1/events` ingests CloudEvents and routes them through the §32 7-stage Runner; cherald `GET /v1/compliance` returns paginated `constitution_state` rows; sherald `GET /v1/safety_state` returns daemon-local counters. HRD-011 Telegram closes incidentally when the Runner's dispatcher routes a real event through the existing telebot.v3 adapter with operator-supplied credentials.

### Module-by-module breakdown

| Package | Status | Purpose |
|---|---|---|
| `commons_auth/` | **NEW** (added to `go.work`, 13→14 modules) | JWT verifier with HMAC and JWKS modes + `gin.HandlerFunc` middleware factory. Shared across all three serving flavors to avoid duplicate verifier implementations. |
| `pherald/internal/runner/` | **NEW** | Runner orchestrator (`runner.go`) + 7 stage concrete types in their own files: `event_parser.go`, `idempotency.go`, `tenant.go`, `policy.go`, `subscriber.go`, `dispatcher.go`, `outcome.go`. No shared interface — concrete types per Approach C decision. |
| `cherald/internal/compliance/` | **NEW** | `/v1/compliance` handler that queries `commons_constitution.ConstitutionStore` and returns paginated, tenant-scoped rows. |
| `sherald/internal/safety/` | **NEW** | Daemon-local Aggregator (open events counter, current mem%, last destructive-op log) + `/v1/safety_state` handler + background mem-sample goroutine. Process-local — no PG read on the hot path. |
| `commons_storage/migrations/` | **EXTEND** | New migration `000010_events_processed.sql` for the inbound-idempotency archive table (RLS-enforced, 30-day retention via `expires_at` column). |
| `commons_constitution/` | **EXTEND** | HRD-018 M2 work: PG-backed `ConstitutionStore` implementation + `constitution_state` migration + Mode-ladder PG persistence. The Store remains an interface; the in-memory M1 implementation stays for tests. |
| `commons_messaging/channels/null` | unchanged | Used by Runner for sandbox dispatch and the always-on E37 invariant. |
| `commons_messaging/channels/tgram` | unchanged | Existing code-complete adapter; HRD-011 closes when a real event routes through it. |
| `pherald/cmd/pherald/main.go`, `cherald/cmd/cherald/main.go`, `sherald/cmd/sherald/main.go` | **MODIFY** | Each wires `commons_auth.GinMiddleware` into `cli.ServeOpts.Middleware` (the hook added in Wave 2 T6). Pherald additionally wires the Runner's `Deps`. |
| `pherald/internal/http/routes.go`, `cherald/internal/http/routes.go`, `sherald/internal/http/routes.go` | **MODIFY** | Each route swaps its `cli.Route{Handler: nil, HRD: ...}` 501-stub for `Handler: <concrete>`. Same `[]cli.Route` slice — `cli.ServeOpts.Routes` consumes it unchanged. |

### Routing diagram

```
POST /v1/events (pherald, :24791)
  → [JWT middleware] → [EventParser] → [IdempotencyChecker]
  → [TenantResolver] → [PolicyGate] → [SubscriberResolver]
  → [ChannelDispatcher] → [OutcomeRecorder] → 202 Accepted + Receipt JSON

GET /v1/compliance (cherald, :24792)
  → [JWT middleware] → [ConstitutionStore.List(tenant, filter, page)]
  → 200 + paginated rows JSON

GET /v1/safety_state (sherald, :24793)
  → [JWT middleware] → [safety.Aggregator.Snapshot()]
  → 200 + state JSON
```

### Module-boundary discipline

Runner stages depend only on `commons_*` packages plus interfaces declared inside `pherald/internal/runner/`. No stage imports a sibling stage's package — they communicate exclusively via the shared `RunCtx` struct that the orchestrator threads through. This is what makes each stage testable in isolation with fakes.

---

## Section 2 — Runner orchestrator + 7 stage types

### Shared state: `RunCtx`

```go
// RunCtx is the per-event work order. Each stage reads + writes fields
// it owns; later stages may not mutate fields owned by earlier stages.
// Lifecycle: one RunCtx per inbound event; lives in memory until Run
// returns. NOT persisted.
type RunCtx struct {
    // Set by JWT middleware before Runner.Run is called:
    AuthClaims map[string]any
    TenantID   uuid.UUID

    // Set by EventParser:
    Raw     []byte
    Event   commons.CloudEventEnvelope
    Trace   commons.TraceContext
    IdemKey string

    // Set by IdempotencyChecker:
    Duplicate  bool
    CachedRcpt *Receipt

    // Set by TenantResolver:
    TenantPGCtx context.Context

    // Set by PolicyGate:
    PolicyDecision commons_constitution.Decision
    PolicyReason   string

    // Set by SubscriberResolver:
    Recipients []commons.Recipient

    // Set by ChannelDispatcher:
    Receipts []ChannelDispatchResult

    // Set by OutcomeRecorder:
    OutboundEvidenceIDs []uuid.UUID
}
```

### The 7 stages

| Stage | File | Owns (writes) | Reads | External deps |
|---|---|---|---|---|
| 1. EventParser | `event_parser.go` | `Raw`, `Event`, `Trace`, `IdemKey` | (HTTP body) | github.com/cloudevents/sdk-go/v2 |
| 2. IdempotencyChecker | `idempotency.go` | `Duplicate`, `CachedRcpt` | `IdemKey`, `TenantID` | Redis (commons_infra) + PG (`events_processed`) |
| 3. TenantResolver | `tenant.go` | `TenantPGCtx` | `TenantID` | commons_storage `WithTenantContext` |
| 4. PolicyGate | `policy.go` | `PolicyDecision`, `PolicyReason` | `Event`, `TenantPGCtx` | commons_constitution.Evaluator |
| 5. SubscriberResolver | `subscriber.go` | `Recipients` | `Event`, `TenantPGCtx` | PG (subscribers + preferences §7) |
| 6. ChannelDispatcher | `dispatcher.go` | `Receipts` | `Recipients`, `Event` | commons.Channel registry (null + tgram) |
| 7. OutcomeRecorder | `outcome.go` | `OutboundEvidenceIDs` | `Receipts`, `IdemKey` | PG `outbound_delivery_evidence` + Redis (mark idem-key processed) |

### Orchestrator

```go
type Runner struct {
    parser  *EventParser
    idem    *IdempotencyChecker
    tenant  *TenantResolver
    policy  *PolicyGate
    subs    *SubscriberResolver
    chans   *ChannelDispatcher
    outcome *OutcomeRecorder
}

func (r *Runner) Run(ctx context.Context, raw []byte, claims map[string]any) (*Receipt, error) {
    rc := &RunCtx{Raw: raw, AuthClaims: claims, TenantID: extractTenant(claims)}
    if err := r.parser.Process(ctx, rc); err != nil { return nil, err }
    if err := r.idem.Process(ctx, rc); err != nil { return nil, err }
    if rc.Duplicate {
        return rc.CachedRcpt, nil  // replay short-circuit
    }
    if err := r.tenant.Process(ctx, rc); err != nil { return nil, err }
    if err := r.policy.Process(ctx, rc); err != nil { return nil, err }
    if rc.PolicyDecision == commons_constitution.DecisionDeny {
        return r.outcome.RecordDenied(ctx, rc)
    }
    if err := r.subs.Process(ctx, rc); err != nil { return nil, err }
    if err := r.chans.Process(ctx, rc); err != nil { return nil, err }
    return r.outcome.Process(ctx, rc)
}
```

### Dependency injection

```go
type Deps struct {
    PG        *pgxpool.Pool
    Redis     redis.Cmdable
    Evaluator commons_constitution.Evaluator
    Channels  map[commons.ChannelID]commons.Channel
    Logger    *slog.Logger
}

func NewRunner(d Deps) *Runner {
    return &Runner{
        parser:  &EventParser{},
        idem:    &IdempotencyChecker{Redis: d.Redis, PG: d.PG, TTL: 24 * time.Hour},
        tenant:  &TenantResolver{PG: d.PG},
        policy:  &PolicyGate{Evaluator: d.Evaluator},
        subs:    &SubscriberResolver{PG: d.PG},
        chans:   &ChannelDispatcher{Channels: d.Channels, Logger: d.Logger},
        outcome: &OutcomeRecorder{PG: d.PG, Redis: d.Redis},
    }
}
```

### HTTP handler wiring

```go
// pherald/internal/http/routes.go
func Routes(runner *runner.Runner) []cli.Route {
    return []cli.Route{
        {Method: "POST", Path: "/v1/events", Handler: eventsIngestHandler(runner)},
    }
}

func eventsIngestHandler(r *runner.Runner) gin.HandlerFunc {
    return func(c *gin.Context) {
        body, _ := io.ReadAll(c.Request.Body)
        claims, _ := c.Get(commons_auth.ContextKeyClaims)
        rcpt, err := r.Run(c.Request.Context(), body, claims.(map[string]any))
        if err != nil {
            c.JSON(mapErrorToStatus(err), gin.H{"error": err.Error()})
            return
        }
        if rcpt.WasReplay {
            c.Header("X-Herald-Replay", "true")
        }
        c.JSON(202, rcpt)
    }
}
```

### Short-circuit semantics

- **Duplicate (Stage 2 hit)** — return cached Receipt with `X-Herald-Replay: true` header, status 202, no dispatch.
- **PolicyDecision = Deny (Stage 4)** — skip stages 5+6, jump to `outcome.RecordDenied` which writes a "denied" row to `outbound_delivery_evidence`, emits the `.policy.violation` CloudEvent via `commons_constitution.Evaluator`. HTTP 403.
- **PolicyDecision = Warn (Stage 4)** — continue normally; `PolicyReason` logged + carried into outcome row for the audit trail.
- **Stage error mid-pipeline** — no further stages run; OutcomeRecorder is responsible for recording the failure if it runs. Errors before OutcomeRecorder cannot be persisted (no row to update yet) — logged + returned.

### Concurrency

- Runner is goroutine-safe; same instance handles all concurrent requests.
- Stage instances are stateless (deps are pool handles + clients which are themselves safe).
- ChannelDispatcher fan-out within a single event is sequential in Wave 3 (one recipient at a time). Parallel fan-out is a Wave 4 optimization tracked under a future HRD.

---

## Section 3 — HRD-028 cherald + HRD-098 sherald handlers

### HRD-028: `cherald/internal/compliance/`

**Data source:** `commons_constitution.ConstitutionStore`. Wave 3 absorbs HRD-018 M2 so this Store is PG-backed by the time HRD-028 lands. The handler is Store-implementation-agnostic — Store is an interface; M1 in-memory stays available for tests.

**Request shape:**
```
GET /v1/compliance
    ?rule_id=11.4.10                  (optional)
    &decision=warn                    (optional — allow|warn|deny|all; default: all)
    &since=2026-05-01T00:00:00Z       (optional — RFC3339, inclusive)
    &until=2026-05-21T23:59:59Z       (optional — RFC3339, inclusive)
    &page=1                           (default: 1)
    &page_size=50                     (default: 50, max: 200)
Authorization: Bearer <JWT>
```

**Response shape (200):**
```json
{
  "page": 1,
  "page_size": 50,
  "total": 173,
  "tenant_id": "...",
  "results": [
    {
      "id": "uuid",
      "rule_id": "11.4.10",
      "subject": "git@gitlab.com:vasic-digital/herald.git@9019abf",
      "decision": "deny",
      "reason": "credential.leak: API key fragment matched in commit body",
      "evaluator_at": "2026-05-21T10:32:14.123Z",
      "bundle_hash": "sha256:..."
    }
  ]
}
```

**Tenant scope:** strict — handler calls `commons_storage.WithTenantContext(ctx, tenantID)` before Store query so RLS gates at the PG layer.

**Empty result:** 200 with `results: []` + `total: 0`. Never 404.

**Pagination:** offset-based (page + page_size). YAGNI for Wave 3; cursor-based is a Wave 5+ scaling optimization.

**Errors:**
- Invalid `since`/`until` parse → 400 `{error:"invalid time format", field:"since"}`.
- `page_size > 200` → 400.
- JWT missing tenant claim → 401 (middleware catches before handler).

### HRD-098: `sherald/internal/safety/`

**Data source:** **process-local in-memory state.** Daemon-only — no PG read on the hot path. Counters update via internal hooks called by sherald's §43 stub bodies as they ship.

**Aggregator type:**
```go
type Aggregator struct {
    mu                sync.RWMutex
    openEvents        atomic.Int64
    lastDestructiveOp *DestructiveOp
    startedAt         time.Time
    lastMemSampleAt   time.Time
    lastMemPercent    float64
}

type DestructiveOp struct {
    Op        string    `json:"op"`         // "rm" | "git-reset" | "git-push-force"
    Path      string    `json:"path"`
    Operator  string    `json:"operator"`
    Blocked   bool      `json:"blocked"`
    BlockedAt time.Time `json:"at"`
    HRDRule   string    `json:"hrd_rule"`
}

func (a *Aggregator) Snapshot() SafetyState { ... }
```

**Response shape (200):**
```json
{
  "binary": "sherald",
  "started_at": "2026-05-21T17:30:00Z",
  "uptime_seconds": 14523,
  "open_events": 0,
  "current_mem_percent": 23.4,
  "last_mem_sample_at": "2026-05-21T21:32:01Z",
  "last_destructive_op": {
    "op": "git-push-force",
    "path": "/Users/m/projects/x.git",
    "operator": "m@m",
    "blocked": true,
    "at": "2026-05-21T20:14:32Z",
    "hrd_rule": "HRD-046"
  }
}
```

When no destructive op has been seen yet, `last_destructive_op` is `null`, not omitted. Missing field would be a §107 bluff (operator can't tell "nothing happened" from "API broken").

**Process scope:** process-global, not tenant-scoped. Sherald is a host-safety daemon, not multi-tenant.

**Daemon-loop integration:** sherald `serve` starts a background goroutine that samples mem% every 10s (tunable via `HERALD_SAFETY_MEM_SAMPLE_INTERVAL`) and updates `Aggregator.lastMemPercent`.

**Concurrency:** Aggregator uses RWMutex for the destructive_op pointer; counters are atomic. Snapshot takes a single read-lock.

---

## Section 4 — `commons_auth/` + idempotency substrate

### `commons_auth/` package

**Module layout:**
```
commons_auth/
├── go.mod
├── verifier.go      # interface + factory
├── hmac.go          # HS256 verifier
├── jwks.go          # RS256/ES256 verifier with Redis cache
├── middleware.go    # gin.HandlerFunc factory
├── claims.go        # ContextKey constants + claim extraction helpers
└── verifier_test.go
```

**Public surface:**
```go
type Verifier interface {
    Verify(token string) (map[string]any, error)
}

type Mode string
const (
    ModeHMAC Mode = "hmac"
    ModeJWKS Mode = "jwks"
)

type Config struct {
    Mode           Mode
    HMACSecret     []byte
    JWKSURL        string
    JWKSCacheTTL   time.Duration
    RequiredClaims []string  // default ["tenant", "sub"]
    Clock          clockwork.Clock
}

func NewVerifierFromEnv(redis redis.Cmdable) (Verifier, error)
func GinMiddleware(v Verifier) gin.HandlerFunc

const ContextKeyClaims = "herald.auth.claims"
```

**HMAC verifier (`hmac.go`):** `github.com/golang-jwt/jwt/v5` HS256 + configured secret. Required claims validated explicitly.

**JWKS verifier (`jwks.go`):** HTTPS JWKS fetch, cached in Redis under `herald:auth:jwks:<sha256-of-url>` with `JWKSCacheTTL` (default 5m). Cache miss → fetch. RS256 + ES256 supported. On `kid` not in cache → force one re-fetch before declaring 401 (covers rotation race). Redis-down → fall back to in-memory cache so the gate degrades rather than locking out all traffic.

**Environment variables:**
- `HERALD_AUTH_MODE` — `hmac` or `jwks`
- `HERALD_AUTH_HMAC_SECRET` — required when mode=hmac
- `HERALD_AUTH_JWKS_URL` — required when mode=jwks
- `HERALD_AUTH_JWKS_TTL` — optional, Go duration string

**Error mapping:** all auth failures return 401 with a typed JSON body that names the rejected condition (`missing bearer token`, `invalid token`, `token expired`, `token missing claim`) without leaking signature internals.

**Why a separate top-level module:** every serving flavor needs this; living in `commons/` would bloat that module's go.mod with golang-jwt + go-redis deps; a dedicated module keeps dependency boundaries clean.

### Idempotency substrate

**Migration `commons_storage/migrations/000010_events_processed.sql`:**

```sql
CREATE TABLE events_processed (
    tenant_id        UUID        NOT NULL,
    idempotency_key  TEXT        NOT NULL,
    event_id         UUID        NOT NULL,
    first_seen_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at       TIMESTAMPTZ NOT NULL DEFAULT (now() + INTERVAL '30 days'),
    PRIMARY KEY (tenant_id, idempotency_key)
);

CREATE INDEX events_processed_expires_idx ON events_processed (expires_at);
CREATE INDEX events_processed_event_id_idx ON events_processed (event_id);

ALTER TABLE events_processed ENABLE ROW LEVEL SECURITY;
ALTER TABLE events_processed FORCE  ROW LEVEL SECURITY;

CREATE POLICY events_processed_tenant_isolation ON events_processed
    USING (tenant_id::text = current_setting('app.current_tenant_id', true));
```

**Hot-path semantics (Stage 2):**

1. **Redis SETNX** `herald:idem:<tenant_id>:<idem_key>` = `<event_id>` EX 86400.
   - SET → not duplicate; remember event_id.
   - NOT SET → already present; GET the value to find original event_id.
2. **If duplicate per Redis:**
   - Query PG `events_processed` for the row under `TenantPGCtx`.
   - If found → query `outbound_delivery_evidence` for rows with that event_id → build cached Receipt → set `RunCtx.Duplicate=true`.
   - If NOT found (race: Redis says dup but PG archive lags) → treat as fresh, accept the event, write new evidence row. "Redis-lies-PG-truths" arbitration: prefer occasional double-dispatch over locking up the ingest path.
3. **If fresh per Redis:** continue Runner pipeline. OutcomeRecorder writes `events_processed` row asynchronously after the dispatch row.

**Replay return shape:** cached Receipt has same `event_id`, `idempotency_key`, per-channel `Receipts` from the original dispatch. HTTP response is 202 + `X-Herald-Replay: true` header so clients can distinguish replays.

**Retention:** `expires_at` column drives nightly cleanup via HRD-047 (`scherald status-digest` periodic sweep) once that §43 command lands. Until then, the table grows unboundedly — known issue (acceptable because dedup hits the 24h Redis TTL anyway; the PG archive is just audit/replay).

---

## Section 5 — HRD-011 close-out + testing strategy

### HRD-011 Telegram live close-out (incidental)

The existing `commons_messaging/channels/tgram/` adapter is code-complete. HRD-011 has been waiting for live evidence — a real send through a real bot to a real chat with a real PG row. Wave 3 closes HRD-011 by routing a real CloudEvent through the Runner's ChannelDispatcher → tgram → bot delivery → `outbound_delivery_evidence` persistence. **No new tgram code** — the close-out is a byproduct of E2E exercising the Wave 3 path with `HERALD_TGRAM_BOT_TOKEN` + `HERALD_TGRAM_CHAT_ID` exported.

E40 (new e2e invariant) is the close-out invariant. If creds are unset, E40 SKIPs (per the existing E17/E18/E34 documented-SKIP-with-reason pattern); HRD-011 stays open. If creds are set, E40 PASSES, and the Wave 3 final commit performs the atomic Issues→Fixed migration for HRD-011.

### New `scripts/e2e_bluff_hunt.sh` invariants (E35-E48)

Wave 3 grows the e2e suite 33 → 48. Numbering picks up after the existing E34 vertical-slice SKIP.

**Auth gate (E35-E36):**
- **E35** — POST /v1/events without `Authorization` → 401 `{error:"missing bearer token"}`.
- **E36** — POST /v1/events with JWT signed by a different HMAC secret → 401 `{error:"invalid token"}`.

**Runner live ingest (E37-E42):**
- **E37** — POST /v1/events with valid JWT + null:// recipient → 202 + Receipt JSON with `event_id`, `idempotency_key`, `receipts[].evidence="routed"`. Verify row in `outbound_delivery_evidence`. **No live creds needed.**
- **E38** — POST same payload twice → second response `X-Herald-Replay: true` + same Receipt; only ONE evidence row. Idempotency proof.
- **E39** — POST same `idempotency_key` but different `event_id` → second treated as duplicate per spec §32.
- **E40** — POST with valid JWT + tgram recipient (env-gated, SKIP if creds absent) → real Telegram delivery + evidence row. **HRD-011 close-out invariant.**
- **E41** — POST with deny-triggering payload → 403 `{error:"denied", rule_id:"11.4.10"}` + denial row in `outbound_delivery_evidence`.
- **E42** — POST → restart pherald → POST same payload → still duplicate (PG events_processed survived; Redis was rebuilt). Persistence proof.

**HRD-028 cherald compliance pull (E43-E45):**
- **E43** — GET /v1/compliance without JWT → 401.
- **E44** — GET /v1/compliance with valid JWT, no rows for tenant → 200 + `{results:[], total:0}`.
- **E45** — POST a policy-warn event via pherald → GET /v1/compliance from cherald → see the warn row. Cross-binary integration proof.

**HRD-098 sherald safety_state (E46-E48):**
- **E46** — GET /v1/safety_state without JWT → 401.
- **E47** — GET on fresh sherald → 200 with `open_events=0`, `last_destructive_op=null`, `current_mem_percent>0`, `uptime_seconds>=1`.
- **E48** — POST synthetic destructive-op event → GET → see `last_destructive_op` populated.

### Paired §1.1 mutation gate — `tests/test_wave3_mutation_meta.sh`

| # | Mutation | Should FAIL invariant |
|---|---|---|
| M1 | Strip JWT verification from `commons_auth/middleware.go` | E35 |
| M2 | Make IdempotencyChecker always declare events fresh | E38 |
| M3 | Make PolicyGate ignore deny decisions | E41 |
| M4 | Make OutcomeRecorder skip the PG write | E37 |
| M5 | Make sherald Aggregator return zeroes always | E47 |
| M6 | Make cherald compliance handler return empty page regardless of filter | E45 |

Same hardlink-backup-restore pattern as `test_wave2_mutation_meta.sh`. Post-flight verifies full battery green after restores.

### Unit + integration tests inside Go modules

- **`commons_auth/verifier_test.go`** — HMAC round-trip, expired token, missing claim, wrong signature, JWKS cache hit/miss/rotation, Redis-down fallback.
- **`pherald/internal/runner/*_test.go`** — one file per stage; table-driven. Stage tests use fakes (in-memory PG-like, in-memory Redis-like, mock Evaluator). Each stage has at least one bluff-guard test that fails if the stage no-ops.
- **`pherald/internal/runner/runner_test.go`** — integration test wiring real Runner with fake deps + 8 end-to-end cases (happy path, duplicate, deny, warn, no-recipients, bad-event, mid-pipeline error, recorder-fail).
- **`cherald/internal/compliance/handler_test.go`** — pagination edge cases, filter parsing, tenant isolation (two tenants, second only sees its rows).
- **`sherald/internal/safety/aggregator_test.go`** — concurrent updates, Snapshot under load, race-detector clean.

### Anti-bluff sign-off battery (post-Wave-3)

```
test_constitution_inheritance.sh         → 15/15 PASS
test_constitution_inheritance_meta.sh    → META-PASS
test_i6_refinement_meta.sh               → 3/3 PASS
test_i8_usability_meta.sh                → 5/5 PASS
test_wave2_mutation_meta.sh              → 4/4 PASS
test_wave3_mutation_meta.sh              → 7/7 PASS (6 mutations + post-flight)
scripts/audit_antibluff.sh               → 17 PASS / 0 FAIL / 1 SKIP
scripts/codegraph_validate.sh            → 8 PASS / 0 FAIL / 2 SKIP
scripts/e2e_bluff_hunt.sh                → ≥45 PASS / 0 FAIL / ≤3 SKIP
```

E40 MAY stay SKIP if operator runs the gate without exporting Telegram creds — same SKIP-with-reason pattern as E17/E18/E34. If E40 PASSES, HRD-011 atomically migrates Issues→Fixed in the same Wave 3 final commit.

---

## Section 6 — Sequencing decomposition (Wave 3a + 3b)

### Implicit HRD-018 dependency

HRD-028 queries `constitution_state` rows. That table lives in HRD-018's migration set. HRD-018 status: M1 complete (in-memory); M2 (PG-backed Store + migrations) TBD. Wave 3 absorbs HRD-018 M2 so HRD-028 can ship live.

### Wave 3a — substrate + lighter routes (~8 tasks)

| # | Task | Closes |
|---|---|---|
| 3a-1 | `commons_auth/` module — Verifier + HMAC + JWKS + Gin middleware + tests | (new infra) |
| 3a-2 | `commons_storage` migration `000010_events_processed.sql` | (new infra) |
| 3a-3 | HRD-018 M2 — PG-backed ConstitutionStore + `constitution_state` migration + Mode-ladder PG persistence | HRD-018 M2 |
| 3a-4 | `cherald/internal/compliance/` handler + tests | (build-up) |
| 3a-5 | cherald `main.go` wires JWT middleware + compliance handler; routes swap | HRD-028 |
| 3a-6 | `sherald/internal/safety/` Aggregator + handler + daemon mem-sample goroutine + tests | (build-up) |
| 3a-7 | sherald `main.go` wires JWT middleware + safety handler; routes swap | HRD-098 |
| 3a-8 | e2e E35-E36 + E43-E48 + Wave-3 mutation gate fragment + Issues→Fixed for HRD-028, HRD-098 + multi-mirror push | (close-out 3a) |

End of 3a: 2 routes live, JWT shared infra in place, e2e ≈ 40+ invariants. HRD-018 M2 captured.

### Wave 3b — pherald Runner + HRD-011 close-out (~10 tasks)

Stands on 3a's substrate. Lands HRD-016 + closes HRD-011 incidentally.

| # | Task | Closes |
|---|---|---|
| 3b-1 | `pherald/internal/runner/event_parser.go` + tests | — |
| 3b-2 | `pherald/internal/runner/idempotency.go` + tests | — |
| 3b-3 | `pherald/internal/runner/tenant.go` + tests | — |
| 3b-4 | `pherald/internal/runner/policy.go` + tests | — |
| 3b-5 | `pherald/internal/runner/subscriber.go` + tests | — |
| 3b-6 | `pherald/internal/runner/dispatcher.go` + tests | — |
| 3b-7 | `pherald/internal/runner/outcome.go` + tests | — |
| 3b-8 | `pherald/internal/runner/runner.go` orchestrator + integration test | — |
| 3b-9 | pherald `main.go` wires Deps + middleware + route swap | HRD-016 |
| 3b-10 | e2e E37-E42 + full Wave-3 mutation gate + Issues→Fixed for HRD-016 (+ HRD-011 if E40 PASSES) + spec V3 r9 + Status r10 + multi-mirror push | HRD-011 (conditional), HRD-016 |

End of 3b: all 3 routes live, e2e at 48 invariants, HRD-011 closed if operator exported Telegram creds at test time.

### Why split

1. **3a is independently valuable** — even if 3b stalls, 3a delivers JWT infra + 2 live routes + HRD-018 M2 progress.
2. **Clean spec boundary** — 3a = "REST routes over existing substrate"; 3b = "build the §32 pipeline".
3. **Two plans, two implementation cycles** — each plan can be its own `superpowers:subagent-driven-development` run with bounded context.
4. **Different reviewer profiles** — 3a is mostly mechanical CRUD-shaped routes; 3b needs architectural review of stage boundaries and idempotency arbitration.

### Mirror push cadence

- **3a final commit** → push to all 4 mirrors.
- **3b final commit** → push to all 4 mirrors.

Two pushes total, each behind a full anti-bluff battery.

---

## Section 7 — Spec V3 impact (r8 → r9)

Wave 3 design IS a spec change per Universal §11.4.73. The spec V3 r8 → r9 bump captures:

| Section | Change |
|---|---|
| Metadata table | `Revision` 8 → 9; `Last modified` update; `Status summary` rewritten to reflect Wave 3 close-outs. |
| §32 (Inbound processing pipeline) | Move from "specified" to "live (pherald only — Wave 3b)". Each of the 7 stages references its `pherald/internal/runner/<file>.go` implementation. |
| §41 (REST API surface) | Three routes flip from 501-stub to live: `POST /v1/events` (pherald), `GET /v1/compliance` (cherald), `GET /v1/safety_state` (sherald). Add the request/response shapes from this design's Sections 2-3. |
| §42 + §44 (Constitution substrate) | HRD-018 M2 evidence appended to §44.9 (PG-backed Store, `constitution_state` migration, Mode-ladder PG). M3 (admin REST) stays open. |
| §44 (Foundation contract) | New §44.M Wave 3a + §44.N Wave 3b subsections with as-built evidence. |
| §43 catalogue | HRD-093 `commons_auth/` catalogue-check addendum: `no-match → vendor as Herald-internal package`. |
| §16 (Data retention) | New row for `events_processed` (30-day retention via `expires_at`; sweep job tracked under HRD-047). |
| §32.2 (Idempotency) | Document the Redis-lies-PG-truths arbitration semantics for replay vs fresh-after-race. |

All four sibling artefacts (HTML/PDF/DOCX + the spec V3 .md) regenerate via `scripts/export_docs.sh` per the post-logo-branding convention.

---

## Section 8 — Open questions

These are deliberately deferred to either implementation-time discovery or follow-up HRDs:

1. **Parallel ChannelDispatcher fan-out within a single event** — Wave 3 dispatches sequentially. Parallel goroutines would speed up fan-out for events with N>1 recipients. Trade-off: per-channel error handling becomes harder. **Defer:** track under future HRD when an operator complains about latency.
2. **CloudEvents binary mode** — Wave 3 EventParser supports both structured (JSON body with all metadata in the body) and binary (metadata in HTTP headers) modes per the cloudevents/sdk-go/v2 contract. Verify test coverage during 3b-1.
3. **Mode-ladder admin REST (HRD-018 M3)** — flipping `allow|warn|enforce` per binding per tenant without redeploy. Wave 3 ships the PG persistence; the admin REST endpoints are HRD-018 M3, deferred to Wave 4.
4. **Subscriber preference UI** — operators currently express preferences via direct PG INSERTs or a `pherald subscriber add` stub. The real preference UI (channel-prefs modal, quiet-hours editor) is §7 + §11.0 work, out of Wave 3 scope.
5. **Dead-letter queue** — Wave 3 records dispatch failures in `outbound_delivery_evidence` with `evidence=failed`. A separate `dead_letters` table + `pherald deadletter replay` workflow is §5.4 — already opened as `subscriber forget` / `deadletter list` stubs in pherald CLI; full bodies are Wave 4+.
6. **Webhook source signature verification** — `/v1/events` currently trusts the JWT. Per-source HMAC signature checks (`webhook_sources` table from §5.5) are deferred — Wave 4 work.
7. **OpenAPI surface** — spec §41 mandates OpenAPI tags on every route. Wave 3 routes get the tags inline in their handlers; the actual `/openapi.json` endpoint generation is a separate HRD.

---

## Section 9 — Catalogue-Check verdict

Per Universal §11.4.74, every new code unit's introduction triggers a catalogue-check against `vasic-digital/` + `HelixDevelopment/` orgs.

| New unit | Catalogue verdict | Evidence |
|---|---|---|
| `commons_auth/` (JWT verifier + Gin middleware) | **no-match → vendor as Herald-internal package** | `digital.vasic.auth` exists but is a session-auth / login-flow module, not a JWT verification middleware. No existing JWT+JWKS+Redis-cache-aware Gin middleware in either org. Track under HRD-093 (commons_auth scaffold). |
| `pherald/internal/runner/` (7-stage pipeline) | **no-match → vendor as Herald-internal package** | The §32 pipeline is Herald-specific by design — it encodes Herald's CloudEvents schema + idempotency model + PG schema. No existing pipeline orchestrator in either org maps. |
| `cherald/internal/compliance/` + `sherald/internal/safety/` | **no-match → vendor as Herald-internal package** | Flavor-specific handlers; no existing equivalent. |
| `commons_storage` migration `000010_events_processed` | **extend** | `digital.vasic.database` already provides the migration runner + RLS plumbing. New migration is additive. |
| HRD-018 M2 ConstitutionStore PG impl | **extend** | `digital.vasic.database` extension; the Store interface itself was established in HRD-018 M1. |

The full per-HRD catalogue-check evidence files live in `docs/catalogue-checks/HRD-093-commons-auth.md` and (for the other units) inline in the Wave 3a / Wave 3b implementation plans.

---

## Implementation handoff

After this design is committed and the user approves, the next step is **TWO `superpowers:writing-plans` invocations** — one for Wave 3a (`docs/superpowers/plans/2026-05-21-wave3a-substrate-and-lighter-routes.md`), one for Wave 3b (`docs/superpowers/plans/2026-05-21-wave3b-runner.md`). Both plans use `superpowers:subagent-driven-development` per Universal §11.4.70.
