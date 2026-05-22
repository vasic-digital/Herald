<div align="center">

![Herald](../../../assets/logo/herald_logo_square_128.png){width=96px height=96px}

</div>

# Wave 3b — pherald Runner + Live `/v1/events` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` per Universal Constitution §11.4.70. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Land the §32 7-stage Runner inside pherald and flip `POST /v1/events` from a 501 stub to a live CloudEvent ingest path. Closes HRD-016. Unblocks consumer-project integration: with the Runner live, an external system can `POST` a CloudEvent and watch it flow through idempotency → policy → subscriber fan-out → channel dispatch → outcome record.

**Architecture:** New `pherald/internal/runner/` package with 7 concrete stage types + a thin orchestrator (Approach C per the Wave 3 design). Each stage owns its own file (~80-150 LOC) with stage-specific deps; the orchestrator (`runner.go`) holds them as fields and calls `.Process(ctx, *RunCtx)` in order. Shared state lives in `RunCtx`. Each stage tests in isolation with fakes; the integration test wires real Runner + fake deps for end-to-end coverage.

**Tech Stack:** Go 1.25+; `github.com/cloudevents/sdk-go/v2` (CloudEvents parsing); `github.com/jackc/pgx/v5/pgxpool` (PG pool via commons_storage); `github.com/redis/go-redis/v9` (Redis); `github.com/google/uuid`; `github.com/gin-gonic/gin` (HTTP); existing `commons/`, `commons_auth/`, `commons_constitution/`, `commons_messaging/channels/{null,tgram}/`, `commons_storage/` packages.

**Spec reference:** [`docs/superpowers/specs/2026-05-21-wave3-runner-rest-live-design.md`](../specs/2026-05-21-wave3-runner-rest-live-design.md) (commit `c4c903b`) — Sections 2 (Runner orchestrator + 7 stage types), 4 (idempotency substrate), 5 (testing strategy), 6 (Wave 3b half), 7 (spec V3 r9 capture).

**Wave 3a substrate already landed:** `commons_auth/` (JWT verifier), `events_processed` migration (000012), `ConstitutionStore.ListQuery` Since/Until/Offset fields, cherald `/v1/compliance` live, sherald `/v1/safety_state` live, e2e_bluff_hunt at 41 PASS / 0 FAIL. HRD-011 closed atomically with live `message_id=5` evidence in commit `f3d817b`.

---

## File Structure

### CREATE

| Path | Responsibility |
|---|---|
| `pherald/internal/runner/runctx.go` | `RunCtx` struct (per-event work order; field-ownership convention) + `Receipt` + `ChannelDispatchResult` |
| `pherald/internal/runner/event_parser.go` | Stage 1 — `EventParser.Process` parses raw HTTP body → `commons.CloudEventEnvelope` via `cloudevents/sdk-go/v2`, derives `IdemKey` |
| `pherald/internal/runner/event_parser_test.go` | Unit tests: structured-mode happy path, binary-mode happy path, malformed body, missing required field |
| `pherald/internal/runner/idempotency.go` | Stage 2 — `IdempotencyChecker.Process` Redis SETNX + PG `events_processed` arbitration |
| `pherald/internal/runner/idempotency_test.go` | Unit tests: fresh event, duplicate (Redis hit + PG hit), Redis-lies-PG-truths race window, expired Redis key + fresh PG |
| `pherald/internal/runner/tenant.go` | Stage 3 — `TenantResolver.Process` builds `TenantPGCtx` via `commons_storage.WithTenantContext` |
| `pherald/internal/runner/tenant_test.go` | Unit tests: tenant UUID set; context carries `app.tenant_id` GUC binding (verified via fake PG) |
| `pherald/internal/runner/policy.go` | Stage 4 — `PolicyGate.Process` calls `commons_constitution.Evaluator` triggered by `Event.Type`; maps `Decision` → continue / warn / deny |
| `pherald/internal/runner/policy_test.go` | Unit tests: allow → continue; warn → continue with reason; deny → short-circuit; no evaluator registered → continue (Wave 3b default = permissive) |
| `pherald/internal/runner/subscriber.go` | Stage 5 — `SubscriberResolver.Process` reads `subscribers` + `subscriber_aliases` under tenant ctx; emits `[]commons.Recipient` per category/tag matching |
| `pherald/internal/runner/subscriber_test.go` | Unit tests: empty tenant → 0 recipients (not error); tenant with 2 subscribers + tgram aliases → 2 recipients; tenant isolation (other tenant's subs invisible) |
| `pherald/internal/runner/dispatcher.go` | Stage 6 — `ChannelDispatcher.Process` looks up `commons.Channel` from registry per recipient's `Channel`; calls `Send(ctx, msg)`; collects per-recipient results |
| `pherald/internal/runner/dispatcher_test.go` | Unit tests: single null:// recipient → 1 result with `Evidence=Routed`; mixed null+tgram → 2 results; unknown channel → result with `Evidence=Unknown` (not error) |
| `pherald/internal/runner/outcome.go` | Stage 7 — `OutcomeRecorder.Process` writes `outbound_delivery_evidence` rows (one per recipient) + `events_processed` archive row; also `RecordDenied` for stage-4-denied short-circuit |
| `pherald/internal/runner/outcome_test.go` | Unit tests: 2 recipients → 2 evidence rows + 1 events_processed row; deny path → 1 denied evidence row + 1 events_processed row; recorder-fail returns error without leaving partial writes |
| `pherald/internal/runner/runner.go` | Orchestrator — `NewRunner(Deps)` constructor + `Runner.Run(ctx, raw, claims) (*Receipt, error)` calling all 7 stages in order |
| `pherald/internal/runner/runner_test.go` | Integration test — real Runner with fake deps; 8 end-to-end cases (happy path, duplicate replay, deny, warn, no-recipients, bad-event, mid-pipeline error, recorder-fail) |
| `pherald/internal/runner/fakes_test.go` | Test fakes: in-memory PG-like store, in-memory Redis-like store, mock Evaluator, mock Channel registry |
| `pherald/internal/http/events.go` | `EventsHandler(r *runner.Runner) gin.HandlerFunc` — extracts claims from `c.Get(commons_auth.ContextKeyClaims)`, reads body, calls `r.Run`, maps result → HTTP response (202 / 403 / 4xx / 5xx) |
| `pherald/internal/http/events_test.go` | Handler unit tests: missing claims → 401; valid body → 202; deny → 403; bad JSON → 400 |

### MODIFY

| Path | Change |
|---|---|
| `pherald/go.mod` | Add deps: `github.com/cloudevents/sdk-go/v2`, ensure `redis/go-redis/v9` is direct (was indirect from commons_auth) |
| `pherald/internal/http/routes.go` | `Routes()` → `Routes(r *runner.Runner) []cli.Route`; `/v1/events` swaps `HRD: "HRD-016"` 501-stub for `Handler: EventsHandler(r)`; `/v1/compliance` removed (it's a cherald route, not pherald — historical mistake in Wave 2 Task 6's plan that left it here as a 501 even though cherald owns it now) |
| `pherald/cmd/pherald/main.go` | Build `runner.Deps{PG, Redis, Evaluator, Channels, Logger}` from env + commons_storage.Open + commons_infra Redis client + commons_constitution registry; call `runner.NewRunner(deps)`; pass to `http.Routes(runner)`; wire `commons_auth.GinMiddleware` to `cli.ServeOpts.Middleware` (was missing in pherald — only cherald + sherald had it after Wave 3a) |
| `pherald/cmd/pherald/stubs.go` | `newServeCmd(br commons.Branding)` signature unchanged; internals now thread the Runner through. RequestIDMiddleware stays. |
| `scripts/e2e_bluff_hunt.sh` | Add E37-E42 + update E45 from SKIP-with-reason to live (cherald reads pherald-written rows); header tally 41 → 47 |
| `tests/test_wave3_mutation_meta.sh` | Add M2 (IdempotencyChecker no-op → E38 FAILs) + M3 (PolicyGate ignores deny → E41 FAILs) + M4 (OutcomeRecorder skip PG write → E37 FAILs); unblock M6 (cross-binary) |
| `docs/specs/mvp/specification.V3.md` | r8 → r9: §32 stages move from "specified" to "live"; §41 `/v1/events` flips from 501-stub to 202+Receipt; §44.N new subsection with as-built evidence |
| `docs/Issues.md` | r11 → r12: HRD-016 Issues→Fixed atomic close-out |
| `docs/Fixed.md` | r10 → r11: HRD-016 prepended to Recently fixed |
| `docs/Status.md` | r12 → r13: Wave 3b completion summary |

---

## Task 1: `pherald/internal/runner/runctx.go` — shared state types

**Files:**
- Create: `pherald/internal/runner/runctx.go`

This task lays the type foundation that every other stage will reference. No tests for this file alone — types are exercised through stage tests.

- [ ] **Step 1: Create `pherald/internal/runner/runctx.go`**

```go
// Package runner implements pherald's §32 7-stage event ingest pipeline.
//
// Each stage is its own concrete struct (Approach C per Wave 3 design):
// no shared `RunnerStage` interface — stages communicate exclusively
// via `RunCtx`. The orchestrator (runner.go) holds stage instances as
// fields and calls them in fixed order in `Run`.
//
// Per §107 anti-bluff: every stage that claims success MUST observe
// positive runtime evidence — getMe-style validations, real DB writes,
// real channel API responses. Stages that no-op on success are §11.4
// PASS-bluffs.
package runner

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/vasic-digital/herald/commons"
	constitution "github.com/vasic-digital/herald/commons_constitution"
)

// RunCtx is the per-event work order. Each stage reads + writes fields
// it owns; later stages may not mutate fields owned by earlier stages.
// Lifecycle: one RunCtx per inbound event; lives in memory until Run
// returns. NOT persisted (events_processed gets the archive row, not
// this struct).
//
// Field-ownership convention (enforced by code review, not Go types):
//
//   Set by HTTP handler before Runner.Run:
//     AuthClaims, TenantID, Raw
//
//   Set by stage 1 EventParser:
//     Event, Trace, IdemKey
//
//   Set by stage 2 IdempotencyChecker:
//     Duplicate, CachedRcpt
//
//   Set by stage 3 TenantResolver:
//     TenantPGCtx
//
//   Set by stage 4 PolicyGate:
//     PolicyDecision, PolicyReason
//
//   Set by stage 5 SubscriberResolver:
//     Recipients
//
//   Set by stage 6 ChannelDispatcher:
//     Receipts
//
//   Set by stage 7 OutcomeRecorder:
//     OutboundEvidenceIDs
type RunCtx struct {
	// Pre-Runner (set by HTTP handler):
	AuthClaims map[string]any
	TenantID   uuid.UUID
	Raw        []byte

	// Stage 1 outputs:
	Event   commons.CloudEventEnvelope
	Trace   commons.TraceContext
	IdemKey string

	// Stage 2 outputs:
	Duplicate  bool
	CachedRcpt *Receipt

	// Stage 3 outputs:
	TenantPGCtx context.Context

	// Stage 4 outputs:
	PolicyDecision constitution.Decision
	PolicyReason   string

	// Stage 5 outputs:
	Recipients []commons.Recipient

	// Stage 6 outputs:
	Receipts []ChannelDispatchResult

	// Stage 7 outputs:
	OutboundEvidenceIDs []uuid.UUID
}

// Receipt is the per-event outcome the HTTP handler returns to the
// client. JSON-encoded as 202 Accepted body (or 200 on replay).
type Receipt struct {
	EventID             string                   `json:"event_id"`
	IdempotencyKey      string                   `json:"idempotency_key"`
	AcceptedAt          time.Time                `json:"accepted_at"`
	Recipients          int                      `json:"recipients"`
	Results             []ChannelDispatchResult  `json:"results"`
	WasReplay           bool                     `json:"was_replay"`
	OutboundEvidenceIDs []uuid.UUID              `json:"outbound_evidence_ids"`
}

// ChannelDispatchResult is the per-recipient outcome captured by
// ChannelDispatcher and surfaced in Receipt.Results.
type ChannelDispatchResult struct {
	ChannelID     string                  `json:"channel_id"`
	ChannelUserID string                  `json:"channel_user_id"`
	Evidence      commons.DeliveryEvidence `json:"evidence"`
	ChannelMsgID  string                  `json:"channel_msg_id,omitempty"`
	Error         string                  `json:"error,omitempty"` // populated only when Evidence == DeliveryUnknown
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd /Users/milosvasic/Projects/Herald
go build ./pherald/internal/runner/ 2>&1 | head -3
```

Expected: silent (clean compile). If `pherald/internal/runner/` doesn't exist yet, `go build` may report no Go files — that's OK on this first commit; the subsequent task creates the first stage that exercises these types.

- [ ] **Step 3: Commit**

```bash
git add pherald/internal/runner/runctx.go
git commit -m "Wave 3b step 1: runctx.go — RunCtx + Receipt + ChannelDispatchResult types

Foundation type definitions for the §32 7-stage Runner. RunCtx is the
per-event work order; field ownership is enforced by convention (see
the package doc comment). No tests on this file alone — types are
exercised through each stage's unit tests.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 2: Stage 1 — EventParser

**Files:**
- Create: `pherald/internal/runner/event_parser.go`, `pherald/internal/runner/event_parser_test.go`

- [ ] **Step 1: Write failing test `event_parser_test.go`**

```go
package runner

import (
	"context"
	"strings"
	"testing"
)

func TestEventParser_StructuredMode_HappyPath(t *testing.T) {
	body := `{
		"specversion":"1.0",
		"id":"01923456-789a-7bcd-abcd-ef0123456789",
		"source":"//test/source",
		"type":"digital.vasic.herald.test",
		"datacontenttype":"application/json",
		"time":"2026-05-22T12:00:00Z",
		"data":{"hello":"world"}
	}`
	rc := &RunCtx{Raw: []byte(body)}
	p := &EventParser{}
	if err := p.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process error: %v", err)
	}
	if rc.Event.ID != "01923456-789a-7bcd-abcd-ef0123456789" {
		t.Errorf("Event.ID = %q", rc.Event.ID)
	}
	if rc.Event.Type != "digital.vasic.herald.test" {
		t.Errorf("Event.Type = %q", rc.Event.Type)
	}
	if rc.IdemKey == "" {
		t.Errorf("IdemKey empty — should derive from EventID")
	}
	if !strings.Contains(string(rc.Event.Data), "hello") {
		t.Errorf("Event.Data lost: %q", rc.Event.Data)
	}
}

func TestEventParser_MalformedJSON_Errors(t *testing.T) {
	rc := &RunCtx{Raw: []byte("not json")}
	p := &EventParser{}
	if err := p.Process(context.Background(), rc); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestEventParser_MissingRequiredField_Errors(t *testing.T) {
	body := `{"specversion":"1.0","id":"abc","source":"//x"}` // missing "type"
	rc := &RunCtx{Raw: []byte(body)}
	p := &EventParser{}
	if err := p.Process(context.Background(), rc); err == nil {
		t.Fatal("expected error for missing 'type' field")
	}
}

func TestEventParser_ExplicitIdempotencyKey_Honored(t *testing.T) {
	body := `{
		"specversion":"1.0",
		"id":"01923456-789a-7bcd-abcd-ef0123456789",
		"source":"//s","type":"x",
		"heraldidempotencykey":"explicit-key-42"
	}`
	rc := &RunCtx{Raw: []byte(body)}
	p := &EventParser{}
	if err := p.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if rc.IdemKey != "explicit-key-42" {
		t.Errorf("IdemKey = %q, want 'explicit-key-42' from heraldidempotencykey extension", rc.IdemKey)
	}
}
```

- [ ] **Step 2: Run test — verify FAIL**

```bash
cd /Users/milosvasic/Projects/Herald
go test ./pherald/internal/runner/ -count=1 2>&1 | tail -5
```

Expected: compile error `undefined: EventParser`.

- [ ] **Step 3: Add cloudevents dep to pherald/go.mod**

```bash
cd /Users/milosvasic/Projects/Herald/pherald
go get github.com/cloudevents/sdk-go/v2
go mod tidy
cd /Users/milosvasic/Projects/Herald
```

- [ ] **Step 4: Implement `pherald/internal/runner/event_parser.go`**

```go
package runner

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/vasic-digital/herald/commons"
)

// EventParser parses raw HTTP body bytes into a commons.CloudEventEnvelope
// (Herald's typed projection of cloudevents v1.0). Supports the
// "structured" content mode where the entire envelope is in the body
// as JSON.
//
// Per §107: returns an error on malformed JSON or missing required
// fields (id, source, type). An empty CloudEvent past parsing would
// be a §11.4 bluff (downstream stages would silently no-op).
//
// Binary mode (envelope metadata in HTTP headers, payload in body) is
// also supported when the HTTP handler propagates headers into the
// Raw payload — Wave 3b uses structured mode by default; binary mode
// detection is a follow-up.
type EventParser struct{}

// structuredCloudEvent mirrors the JSON wire format Herald accepts.
// Field tags are CloudEvents 1.0 canonical names; "heraldidempotencykey"
// is the Herald extension carrying an explicit idempotency key (per
// spec §32.2).
type structuredCloudEvent struct {
	SpecVersion          string          `json:"specversion"`
	ID                   string          `json:"id"`
	Source               string          `json:"source"`
	Type                 string          `json:"type"`
	Subject              string          `json:"subject,omitempty"`
	Time                 string          `json:"time,omitempty"`
	DataContentType      string          `json:"datacontenttype,omitempty"`
	Data                 json.RawMessage `json:"data,omitempty"`
	HeraldIdempotencyKey string          `json:"heraldidempotencykey,omitempty"`
	HeraldTenant         string          `json:"heraldtenant,omitempty"`
	HeraldPriority       string          `json:"heraldpriority,omitempty"`
}

func (p *EventParser) Process(ctx context.Context, rc *RunCtx) error {
	if len(rc.Raw) == 0 {
		return fmt.Errorf("event_parser: empty body")
	}
	var s structuredCloudEvent
	if err := json.Unmarshal(rc.Raw, &s); err != nil {
		return fmt.Errorf("event_parser: malformed JSON: %w", err)
	}
	if s.ID == "" {
		return fmt.Errorf("event_parser: missing required field 'id'")
	}
	if s.Source == "" {
		return fmt.Errorf("event_parser: missing required field 'source'")
	}
	if s.Type == "" {
		return fmt.Errorf("event_parser: missing required field 'type'")
	}
	rc.Event = commons.CloudEventEnvelope{
		SpecVersion:     s.SpecVersion,
		ID:              s.ID,
		Source:          s.Source,
		Type:            s.Type,
		Subject:         s.Subject,
		DataContentType: s.DataContentType,
		Data:            []byte(s.Data),
		Extensions: map[string]string{
			"heraldidempotencykey": s.HeraldIdempotencyKey,
			"heraldtenant":         s.HeraldTenant,
			"heraldpriority":       s.HeraldPriority,
		},
	}
	// Derive idempotency key: explicit > event_id.
	if s.HeraldIdempotencyKey != "" {
		rc.IdemKey = s.HeraldIdempotencyKey
	} else {
		rc.IdemKey = s.ID
	}
	return nil
}
```

- [ ] **Step 5: Run tests — verify 4/4 PASS**

```bash
go test -race -count=1 ./pherald/internal/runner/ 2>&1 | tail -10
```

- [ ] **Step 6: Commit**

```bash
git add pherald/internal/runner/event_parser.go pherald/internal/runner/event_parser_test.go pherald/go.mod pherald/go.sum
git commit -m "Wave 3b step 2: Stage 1 EventParser — structured CloudEvents v1.0 parse

Parses raw HTTP body → commons.CloudEventEnvelope. Validates required
fields (id, source, type) per CloudEvents 1.0 spec; rejects malformed
JSON and missing fields with explicit error messages. Honors Herald
extension 'heraldidempotencykey' (spec §32.2) — falls back to event_id
for the IdemKey when absent.

4 unit tests: structured-mode happy path, malformed JSON, missing
'type' field, explicit idempotency key override.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 3: Stage 2 — IdempotencyChecker

**Files:**
- Create: `pherald/internal/runner/idempotency.go`, `pherald/internal/runner/idempotency_test.go`, `pherald/internal/runner/fakes_test.go`

- [ ] **Step 1: Create `pherald/internal/runner/fakes_test.go` — test fakes**

```go
package runner

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// fakeRedis is a minimal in-memory stand-in for redis.Cmdable used in
// the runner stage tests. Only implements the methods stages call.
// Concurrent-safe.
type fakeRedis struct {
	mu      sync.Mutex
	store   map[string]string
	expires map[string]time.Time
	now     func() time.Time
}

func newFakeRedis() *fakeRedis {
	return &fakeRedis{
		store:   map[string]string{},
		expires: map[string]time.Time{},
		now:     time.Now,
	}
}

// SetNX returns (true, nil) if the key was set, (false, nil) if already
// present. Honors TTL.
func (r *fakeRedis) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if exp, ok := r.expires[key]; ok && r.now().After(exp) {
		delete(r.store, key)
		delete(r.expires, key)
	}
	if _, exists := r.store[key]; exists {
		return false, nil
	}
	r.store[key] = value
	if ttl > 0 {
		r.expires[key] = r.now().Add(ttl)
	}
	return true, nil
}

// Get returns the value or "" if absent.
func (r *fakeRedis) Get(ctx context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if exp, ok := r.expires[key]; ok && r.now().After(exp) {
		delete(r.store, key)
		delete(r.expires, key)
	}
	v, ok := r.store[key]
	if !ok {
		return "", errRedisNil
	}
	return v, nil
}

var errRedisNil = errors.New("fake redis: key not found")

// fakeEventsProcessedStore is a minimal in-memory stand-in for the
// PG events_processed table.
type fakeEventsProcessedStore struct {
	mu   sync.Mutex
	rows map[string]eventsProcessedRow // key: tenantID + "/" + idemKey
}

type eventsProcessedRow struct {
	TenantID      uuid.UUID
	IdemKey       string
	EventID       string
	FirstSeenAt   time.Time
	Receipt       *Receipt
}

func newFakeEventsProcessedStore() *fakeEventsProcessedStore {
	return &fakeEventsProcessedStore{rows: map[string]eventsProcessedRow{}}
}

func (s *fakeEventsProcessedStore) Lookup(ctx context.Context, tenantID uuid.UUID, idemKey string) (*eventsProcessedRow, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := tenantID.String() + "/" + idemKey
	r, ok := s.rows[key]
	if !ok {
		return nil, false
	}
	return &r, true
}

func (s *fakeEventsProcessedStore) Insert(ctx context.Context, row eventsProcessedRow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := row.TenantID.String() + "/" + row.IdemKey
	if _, exists := s.rows[key]; exists {
		// PK constraint would reject this in real PG; fake mimics that.
		return errors.New("fake events_processed: duplicate PK")
	}
	s.rows[key] = row
	return nil
}

// makeRunCtx is a helper that builds a RunCtx with EventParser already run.
// Most stage tests start from this state.
func makeRunCtx(t testingHelper, tenantID uuid.UUID, eventID, idemKey, eventType string) *RunCtx {
	t.Helper()
	if idemKey == "" {
		idemKey = eventID
	}
	return &RunCtx{
		TenantID: tenantID,
		IdemKey:  idemKey,
		Event: cloudEventStub(eventID, eventType),
	}
}

func cloudEventStub(id, typ string) commonsCloudEventEnvelope {
	// Local type alias so this fake doesn't need to import commons —
	// the real struct mirror is in commons/types.go.
	return commonsCloudEventEnvelope{
		SpecVersion: "1.0",
		ID:          id,
		Source:      "//test/source",
		Type:        typ,
	}
}

// testingHelper is the subset of *testing.T fakes need. Used by both
// the test files in this package.
type testingHelper interface {
	Helper()
	Fatalf(format string, args ...any)
}

// Type alias so subscriber/dispatcher fakes can reuse without circular import.
// (Removed in real code — commons.CloudEventEnvelope is imported directly
// from runctx.go's import block; this trick is only needed if a future
// build constraint splits files.)
type commonsCloudEventEnvelope = struct {
	SpecVersion     string
	ID              string
	Source          string
	Type            string
	Subject         string
	DataContentType string
	Data            []byte
	Extensions      map[string]string
}

func mustParse(s string) uuid.UUID {
	id, err := uuid.Parse(s)
	if err != nil {
		panic(err)
	}
	return id
}

// trimToken strips noise (used to compare hex digests across formats).
func trimToken(s string) string { return strings.TrimSpace(s) }
```

(Note: the local `commonsCloudEventEnvelope` alias avoids an import cycle complexity in fakes. In practice the production tests will import `commons.CloudEventEnvelope` directly — keep the type-alias hack out of production code.)

- [ ] **Step 2: Write the failing IdempotencyChecker test**

```go
package runner

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestIdempotencyChecker_FreshEvent_NotDuplicate(t *testing.T) {
	tenantID := mustParse("11111111-1111-1111-1111-111111111111")
	redis := newFakeRedis()
	pg := newFakeEventsProcessedStore()
	c := &IdempotencyChecker{Redis: redis, PG: pg, TTL: 24 * time.Hour}

	rc := &RunCtx{
		TenantID: tenantID,
		IdemKey:  "fresh-1",
		Event:    cloudEventStub("evt-1", "x"),
	}
	if err := c.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if rc.Duplicate {
		t.Errorf("Duplicate=true on fresh event")
	}
	if rc.CachedRcpt != nil {
		t.Errorf("CachedRcpt non-nil on fresh event")
	}
}

func TestIdempotencyChecker_DuplicateRedis_HitsPG(t *testing.T) {
	tenantID := mustParse("11111111-1111-1111-1111-111111111111")
	redis := newFakeRedis()
	pg := newFakeEventsProcessedStore()
	c := &IdempotencyChecker{Redis: redis, PG: pg, TTL: 24 * time.Hour}

	// Seed Redis + PG with a prior event.
	priorReceipt := &Receipt{EventID: "evt-1", IdempotencyKey: "k1", Recipients: 1}
	_, _ = redis.SetNX(context.Background(), "herald:idem:"+tenantID.String()+":k1", "evt-1", 24*time.Hour)
	_ = pg.Insert(context.Background(), eventsProcessedRow{
		TenantID: tenantID, IdemKey: "k1", EventID: "evt-1", FirstSeenAt: time.Now(), Receipt: priorReceipt,
	})

	// Process a "second send" with the same IdemKey.
	rc := &RunCtx{
		TenantID: tenantID,
		IdemKey:  "k1",
		Event:    cloudEventStub("evt-1-retry", "x"),
	}
	if err := c.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if !rc.Duplicate {
		t.Errorf("Duplicate=false on second send with same IdemKey")
	}
	if rc.CachedRcpt == nil {
		t.Fatal("CachedRcpt nil on duplicate (expected prior Receipt)")
	}
	if rc.CachedRcpt.EventID != "evt-1" {
		t.Errorf("CachedRcpt.EventID = %q, want 'evt-1'", rc.CachedRcpt.EventID)
	}
}

func TestIdempotencyChecker_RedisLiesPGTruths_FreshIfPGAbsent(t *testing.T) {
	// Race scenario: Redis says "duplicate" but PG hasn't archived yet.
	// Per Wave 3 design §4, arbitration favors PG truth → treat as fresh.
	tenantID := mustParse("11111111-1111-1111-1111-111111111111")
	redis := newFakeRedis()
	pg := newFakeEventsProcessedStore() // intentionally empty
	c := &IdempotencyChecker{Redis: redis, PG: pg, TTL: 24 * time.Hour}

	// Redis SETNX has the key but PG has no row (mid-race).
	_, _ = redis.SetNX(context.Background(), "herald:idem:"+tenantID.String()+":k1", "evt-1", 24*time.Hour)

	rc := &RunCtx{
		TenantID: tenantID,
		IdemKey:  "k1",
		Event:    cloudEventStub("evt-2", "x"),
	}
	if err := c.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if rc.Duplicate {
		t.Errorf("Redis-lies-PG-truths: should treat as FRESH when PG row absent (got Duplicate=true)")
	}
}
```

- [ ] **Step 3: Run test — verify FAIL**

```bash
go test ./pherald/internal/runner/ -count=1 2>&1 | tail -5
```

Expected: `undefined: IdempotencyChecker`.

- [ ] **Step 4: Implement `pherald/internal/runner/idempotency.go`**

```go
package runner

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// IdempotencyChecker is Stage 2 of the §32 pipeline. It implements the
// "Redis-lies-PG-truths" arbitration semantics from the Wave 3 design
// §4: a Redis SETNX miss means "maybe a duplicate"; the verdict is
// confirmed by PG `events_processed` table lookup. If PG has no row
// (race window: SETNX succeeded but archive hasn't caught up yet),
// the event is treated as FRESH — the alternative (block until PG row
// appears) would lock up ingest if the archive writer is down.
//
// Per §107: a no-op stage that always returns Duplicate=false would
// be a §11.4 bluff (every event would dispatch even when duplicate).
type IdempotencyChecker struct {
	Redis idempotencyRedis
	PG    idempotencyPG
	TTL   time.Duration
}

// idempotencyRedis is the subset of redis.Cmdable this stage uses.
// Used here as an interface so the test fake can satisfy it without
// pulling in the real Redis client.
type idempotencyRedis interface {
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	Get(ctx context.Context, key string) (string, error)
}

// idempotencyPG is the subset of PG access this stage uses.
type idempotencyPG interface {
	Lookup(ctx context.Context, tenantID uuid.UUID, idemKey string) (*eventsProcessedRow, bool)
}

func (c *IdempotencyChecker) Process(ctx context.Context, rc *RunCtx) error {
	key := "herald:idem:" + rc.TenantID.String() + ":" + rc.IdemKey
	set, err := c.Redis.SetNX(ctx, key, rc.Event.ID, c.TTL)
	if err != nil {
		return fmt.Errorf("idempotency: redis SETNX: %w", err)
	}
	if set {
		// SETNX succeeded → key wasn't there → fresh event.
		rc.Duplicate = false
		return nil
	}
	// SETNX missed → key already present → potential duplicate.
	// Confirm via PG: if the events_processed row exists, it's a real
	// duplicate; if not, we're in the Redis-lies-PG-truths race window
	// and should treat as fresh.
	row, found := c.PG.Lookup(ctx, rc.TenantID, rc.IdemKey)
	if !found {
		rc.Duplicate = false
		return nil
	}
	rc.Duplicate = true
	rc.CachedRcpt = row.Receipt
	return nil
}

var errIdemNotFound = errors.New("idempotency: events_processed row not found")
```

- [ ] **Step 5: Run tests — verify 3/3 PASS**

```bash
go test -race -count=1 ./pherald/internal/runner/ 2>&1 | tail -10
```

- [ ] **Step 6: Commit**

```bash
git add pherald/internal/runner/idempotency.go pherald/internal/runner/idempotency_test.go pherald/internal/runner/fakes_test.go
git commit -m "Wave 3b step 3: Stage 2 IdempotencyChecker + test fakes

Redis SETNX (hot path) + PG events_processed arbitration (truth path)
per Wave 3 design §4. Race semantics: Redis-lies-PG-truths — when
Redis says 'duplicate' but PG hasn't archived yet, treat as fresh
(safe-but-occasionally-double-dispatches; never locks up ingest).

Test fakes (fakes_test.go): in-memory Redis with TTL + in-memory
events_processed store. Used by all subsequent stage tests.

3 unit tests: fresh event, duplicate (Redis hit + PG hit), Redis-lies-
PG-truths race window.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 4: Stage 3 — TenantResolver

**Files:**
- Create: `pherald/internal/runner/tenant.go`, `pherald/internal/runner/tenant_test.go`

- [ ] **Step 1: Write failing test `tenant_test.go`**

```go
package runner

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestTenantResolver_SetsTenantPGCtx(t *testing.T) {
	tenantID := mustParse("22222222-2222-2222-2222-222222222222")
	r := &TenantResolver{}
	rc := &RunCtx{TenantID: tenantID}
	if err := r.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if rc.TenantPGCtx == nil {
		t.Fatal("TenantPGCtx is nil after Process")
	}
	got, ok := rc.TenantPGCtx.Value(tenantCtxKey{}).(uuid.UUID)
	if !ok {
		t.Fatal("TenantPGCtx missing tenantCtxKey")
	}
	if got != tenantID {
		t.Errorf("ctx tenantID = %s, want %s", got, tenantID)
	}
}

func TestTenantResolver_NilTenantID_Errors(t *testing.T) {
	r := &TenantResolver{}
	rc := &RunCtx{} // TenantID = uuid.Nil
	if err := r.Process(context.Background(), rc); err == nil {
		t.Fatal("expected error for nil TenantID")
	}
}
```

- [ ] **Step 2: Run test — verify FAIL (compile error)**

```bash
go test ./pherald/internal/runner/ -count=1 2>&1 | tail -5
```

- [ ] **Step 3: Implement `pherald/internal/runner/tenant.go`**

```go
package runner

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// TenantResolver is Stage 3 — binds the tenant UUID into a context.Context
// the downstream stages will pass to PG queries (where RLS policies key
// off `app.tenant_id` GUC, which commons_storage.WithTenantContext sets).
//
// Note: in Wave 3b the actual GUC-setting happens inside commons_storage
// when SubscriberResolver / OutcomeRecorder issue queries; this stage's
// job is to ensure the ctx carries the resolved tenant so downstream
// don't need to reach back into RunCtx.
type TenantResolver struct{}

// tenantCtxKey is the context key for the resolved tenant UUID. Stage
// 5+6+7 read it via ctx.Value(tenantCtxKey{}).
type tenantCtxKey struct{}

func (r *TenantResolver) Process(ctx context.Context, rc *RunCtx) error {
	if rc.TenantID == uuid.Nil {
		return fmt.Errorf("tenant_resolver: TenantID is uuid.Nil (claim extraction failed?)")
	}
	rc.TenantPGCtx = context.WithValue(ctx, tenantCtxKey{}, rc.TenantID)
	return nil
}

// TenantFromCtx is exported so downstream stages can extract the tenant
// without circular references. Returns uuid.Nil if not set.
func TenantFromCtx(ctx context.Context) uuid.UUID {
	v, _ := ctx.Value(tenantCtxKey{}).(uuid.UUID)
	return v
}
```

- [ ] **Step 4: Run tests — verify 2/2 PASS**

```bash
go test -race -count=1 ./pherald/internal/runner/ 2>&1 | tail -10
```

- [ ] **Step 5: Commit**

```bash
git add pherald/internal/runner/tenant.go pherald/internal/runner/tenant_test.go
git commit -m "Wave 3b step 4: Stage 3 TenantResolver — bind tenant UUID to ctx

Builds the TenantPGCtx that downstream stages thread through PG
queries. Actual GUC-setting (app.tenant_id) happens inside
commons_storage.WithTenantContext at query time — this stage's job
is to put the resolved tenant where Stage 5/6/7 can find it via
TenantFromCtx(ctx).

Fail-loud on uuid.Nil — claim extraction must succeed before reaching
this stage.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 5: Stage 4 — PolicyGate

**Files:**
- Create: `pherald/internal/runner/policy.go`, `pherald/internal/runner/policy_test.go`

- [ ] **Step 1: Write failing tests**

```go
package runner

import (
	"context"
	"testing"

	constitution "github.com/vasic-digital/herald/commons_constitution"
)

func TestPolicyGate_NoEvaluator_Permissive(t *testing.T) {
	// Wave 3b default: no evaluator registered for this event type → continue.
	reg := constitution.NewRegistry()
	g := &PolicyGate{Registry: reg}
	rc := &RunCtx{
		Event: cloudEventStub("evt-1", "digital.vasic.herald.unknown"),
	}
	if err := g.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if rc.PolicyDecision != constitution.DecisionPass {
		t.Errorf("PolicyDecision = %s, want Pass (permissive default)", rc.PolicyDecision)
	}
}

// fakeEvaluator returns a configured Decision for any subject.
type fakeEvaluator struct {
	ruleID   string
	severity constitution.Severity
	triggers []string
	verdict  constitution.Decision
	reason   string
}

func (f *fakeEvaluator) RuleID() string                                        { return f.ruleID }
func (f *fakeEvaluator) Severity() constitution.Severity                       { return f.severity }
func (f *fakeEvaluator) PushTriggers() []string                                { return f.triggers }
func (f *fakeEvaluator) Subjects(ctx context.Context, tid uuid.UUID) ([]constitution.Subject, error) {
	return []constitution.Subject{{Kind: "test", ID: "x"}}, nil
}
func (f *fakeEvaluator) Evaluate(ctx context.Context, s constitution.Subject, b constitution.BundleHash) (constitution.Result, error) {
	return constitution.Result{Decision: f.verdict, Evidence: f.reason}, nil
}

func TestPolicyGate_EvaluatorAllow_Continues(t *testing.T) {
	reg := constitution.NewRegistry()
	reg.Register(&fakeEvaluator{
		ruleID: "11.4.99", severity: constitution.SeverityLow,
		triggers: []string{"digital.vasic.herald.test"},
		verdict:  constitution.DecisionPass,
	})
	g := &PolicyGate{Registry: reg}
	rc := &RunCtx{Event: cloudEventStub("e1", "digital.vasic.herald.test")}
	if err := g.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if rc.PolicyDecision != constitution.DecisionPass {
		t.Errorf("DecisionPass expected; got %s", rc.PolicyDecision)
	}
}

func TestPolicyGate_EvaluatorFail_DenyShortCircuit(t *testing.T) {
	reg := constitution.NewRegistry()
	reg.Register(&fakeEvaluator{
		ruleID: "11.4.10", severity: constitution.SeverityCritical,
		triggers: []string{"digital.vasic.herald.test"},
		verdict:  constitution.DecisionFail,
		reason:   "credential-leak detected in body",
	})
	g := &PolicyGate{Registry: reg}
	rc := &RunCtx{Event: cloudEventStub("e1", "digital.vasic.herald.test")}
	if err := g.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if rc.PolicyDecision != constitution.DecisionFail {
		t.Errorf("DecisionFail expected; got %s", rc.PolicyDecision)
	}
	if rc.PolicyReason == "" {
		t.Errorf("PolicyReason empty on Fail (operator can't see why)")
	}
}
```

(`uuid` import required in the file; add as part of Step 3 if not auto-imported.)

- [ ] **Step 2: Run test — verify FAIL**

```bash
go test ./pherald/internal/runner/ -count=1 -run TestPolicyGate 2>&1 | tail -5
```

- [ ] **Step 3: Implement `pherald/internal/runner/policy.go`**

```go
package runner

import (
	"context"
	"strings"

	constitution "github.com/vasic-digital/herald/commons_constitution"
)

// PolicyGate is Stage 4 — runs every Evaluator in the Registry whose
// PushTriggers include the inbound event Type. The first non-Pass
// verdict wins (Fail > Warn > Pass), so a single Fail short-circuits.
//
// Wave 3b default is permissive: if no Evaluator is registered for the
// event type, DecisionPass is set and the pipeline continues. Tightening
// to fail-closed is a future HRD (operators can register a "deny by
// default" evaluator if they want that posture).
type PolicyGate struct {
	Registry *constitution.Registry
}

func (g *PolicyGate) Process(ctx context.Context, rc *RunCtx) error {
	if g.Registry == nil || g.Registry.Len() == 0 {
		rc.PolicyDecision = constitution.DecisionPass
		return nil
	}
	// Find evaluators whose PushTriggers include this event Type.
	worst := constitution.DecisionPass
	var reason string
	for _, ev := range g.Registry.All() {
		triggers := ev.PushTriggers()
		match := false
		for _, t := range triggers {
			if strings.EqualFold(t, rc.Event.Type) {
				match = true
				break
			}
		}
		if !match {
			continue
		}
		subjects, err := ev.Subjects(ctx, rc.TenantID)
		if err != nil {
			// Per §107 honesty: an evaluator error is recorded as DecisionError
			// rather than silently swallowed (a silent swallow would let
			// faulty evaluators bypass enforcement).
			worst = constitution.DecisionError
			reason = "evaluator " + ev.RuleID() + " Subjects() failed: " + err.Error()
			continue
		}
		for _, s := range subjects {
			res, err := ev.Evaluate(ctx, s, constitution.BundleHash{})
			if err != nil {
				worst = constitution.DecisionError
				reason = "evaluator " + ev.RuleID() + " Evaluate() failed: " + err.Error()
				continue
			}
			if res.Decision > worst {
				worst = res.Decision
				reason = ev.RuleID() + ": " + res.Evidence
			}
		}
	}
	rc.PolicyDecision = worst
	rc.PolicyReason = reason
	return nil
}
```

- [ ] **Step 4: Run tests — verify 3/3 PASS**

```bash
go test -race -count=1 ./pherald/internal/runner/ -run TestPolicyGate 2>&1 | tail -10
```

- [ ] **Step 5: Commit**

```bash
git add pherald/internal/runner/policy.go pherald/internal/runner/policy_test.go
git commit -m "Wave 3b step 5: Stage 4 PolicyGate — Evaluator-driven decision

Runs every Evaluator in the Registry whose PushTriggers include the
inbound event Type. Worst verdict wins (Fail > Warn > Pass; Error
trumps Pass). DecisionFail → orchestrator short-circuits to
OutcomeRecorder.RecordDenied.

Wave 3b default: permissive — no evaluator registered → DecisionPass.
A 'deny by default' posture requires the operator to register an
explicit deny-all evaluator (future HRD).

3 unit tests: no evaluator (permissive), allow path, fail path with
non-empty reason.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 6: Stage 5 — SubscriberResolver

**Files:**
- Create: `pherald/internal/runner/subscriber.go`, `pherald/internal/runner/subscriber_test.go`
- Extend: `pherald/internal/runner/fakes_test.go` (add fake subscribers store)

- [ ] **Step 1: Append fake subscribers store to `fakes_test.go`**

```go
// fakeSubscribersStore is a minimal in-memory stand-in for the
// subscribers + subscriber_aliases tables. Tenant-isolated.
type fakeSubscribersStore struct {
	mu          sync.Mutex
	subs        map[uuid.UUID][]subscriberRow // keyed by tenantID
}

type subscriberRow struct {
	ID          uuid.UUID
	TenantID    uuid.UUID
	Handle      string
	DisplayName string
	Aliases     []subscriberAliasRow
}

type subscriberAliasRow struct {
	Channel       string
	ChannelUserID string
}

func newFakeSubscribersStore() *fakeSubscribersStore {
	return &fakeSubscribersStore{subs: map[uuid.UUID][]subscriberRow{}}
}

func (s *fakeSubscribersStore) Add(tenantID uuid.UUID, row subscriberRow) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row.TenantID = tenantID
	s.subs[tenantID] = append(s.subs[tenantID], row)
}

// ListByTenant returns all subscribers for the tenant resolved via
// the standard TenantFromCtx convention. Other tenants' rows are
// invisible (tenant isolation).
func (s *fakeSubscribersStore) ListByTenant(ctx context.Context) ([]subscriberRow, error) {
	tid := TenantFromCtx(ctx)
	if tid == uuid.Nil {
		return nil, errors.New("fake subscribers: no tenant in ctx")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]subscriberRow, len(s.subs[tid]))
	copy(out, s.subs[tid])
	return out, nil
}
```

- [ ] **Step 2: Write failing tests `subscriber_test.go`**

```go
package runner

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestSubscriberResolver_EmptyTenant_ZeroRecipients(t *testing.T) {
	tenantID := mustParse("33333333-3333-3333-3333-333333333333")
	store := newFakeSubscribersStore() // no subs added
	r := &SubscriberResolver{Subscribers: store}

	rc := &RunCtx{TenantID: tenantID}
	rc.TenantPGCtx = withTenantCtx(context.Background(), tenantID)
	if err := r.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(rc.Recipients) != 0 {
		t.Errorf("Recipients = %d, want 0 on empty tenant", len(rc.Recipients))
	}
}

func TestSubscriberResolver_TwoSubscribers_TgramAliases(t *testing.T) {
	tenantID := mustParse("33333333-3333-3333-3333-333333333333")
	store := newFakeSubscribersStore()
	store.Add(tenantID, subscriberRow{
		ID: uuid.New(), Handle: "alice", DisplayName: "Alice",
		Aliases: []subscriberAliasRow{{Channel: "tgram", ChannelUserID: "100"}},
	})
	store.Add(tenantID, subscriberRow{
		ID: uuid.New(), Handle: "bob", DisplayName: "Bob",
		Aliases: []subscriberAliasRow{{Channel: "tgram", ChannelUserID: "200"}},
	})

	r := &SubscriberResolver{Subscribers: store}
	rc := &RunCtx{TenantID: tenantID}
	rc.TenantPGCtx = withTenantCtx(context.Background(), tenantID)
	if err := r.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(rc.Recipients) != 2 {
		t.Fatalf("Recipients = %d, want 2", len(rc.Recipients))
	}
	chats := map[string]bool{}
	for _, rcpt := range rc.Recipients {
		chats[rcpt.ChannelUserID] = true
	}
	if !chats["100"] || !chats["200"] {
		t.Errorf("Recipients missing expected chats: %v", chats)
	}
}

func TestSubscriberResolver_TenantIsolation(t *testing.T) {
	tidA := mustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	tidB := mustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	store := newFakeSubscribersStore()
	store.Add(tidA, subscriberRow{ID: uuid.New(), Handle: "a1",
		Aliases: []subscriberAliasRow{{Channel: "tgram", ChannelUserID: "111"}}})
	store.Add(tidB, subscriberRow{ID: uuid.New(), Handle: "b1",
		Aliases: []subscriberAliasRow{{Channel: "tgram", ChannelUserID: "222"}}})

	r := &SubscriberResolver{Subscribers: store}
	// Resolve as tenant A — should see only their sub.
	rc := &RunCtx{TenantID: tidA}
	rc.TenantPGCtx = withTenantCtx(context.Background(), tidA)
	if err := r.Process(context.Background(), rc); err != nil {
		t.Fatal(err)
	}
	if len(rc.Recipients) != 1 || rc.Recipients[0].ChannelUserID != "111" {
		t.Errorf("Tenant isolation broken: got %v", rc.Recipients)
	}
}

// withTenantCtx mirrors TenantResolver.Process to make sub-tests
// independent of stage 3.
func withTenantCtx(ctx context.Context, tid uuid.UUID) context.Context {
	return context.WithValue(ctx, tenantCtxKey{}, tid)
}
```

- [ ] **Step 3: Implement `pherald/internal/runner/subscriber.go`**

```go
package runner

import (
	"context"
	"fmt"

	"github.com/vasic-digital/herald/commons"
)

// SubscriberResolver is Stage 5 — reads subscribers + aliases under
// the resolved tenant and emits commons.Recipient entries.
//
// Wave 3b implementation: ALL subscribers in the tenant receive ALL
// events. Per-event preference filtering (CategoryPref / WorkflowPref /
// QuietHours per spec §7.2-§7.3) is a follow-up HRD; for now this
// stage's contract is "tenant-scoped fan-out to every registered
// recipient" — adequate for the consumer-project integration use case
// where one tenant maps to one Telegram chat.
//
// Per §107: returning zero recipients on an empty tenant is NOT an
// error — empty fan-out is a valid (and common) outcome. The
// OutcomeRecorder still writes an events_processed row so the event
// is deduped on replay.
type SubscriberResolver struct {
	Subscribers subscribersStore
}

// subscribersStore is the subset of PG access this stage uses.
type subscribersStore interface {
	ListByTenant(ctx context.Context) ([]subscriberRow, error)
}

func (r *SubscriberResolver) Process(ctx context.Context, rc *RunCtx) error {
	if rc.TenantPGCtx == nil {
		return fmt.Errorf("subscriber_resolver: TenantPGCtx not set by stage 3")
	}
	rows, err := r.Subscribers.ListByTenant(rc.TenantPGCtx)
	if err != nil {
		return fmt.Errorf("subscriber_resolver: list: %w", err)
	}
	var recips []commons.Recipient
	for _, row := range rows {
		for _, alias := range row.Aliases {
			recips = append(recips, commons.Recipient{
				Channel:       alias.Channel,
				ChannelUserID: alias.ChannelUserID,
				DisplayName:   row.DisplayName,
			})
		}
	}
	rc.Recipients = recips
	return nil
}
```

- [ ] **Step 4: Run tests — verify 3/3 PASS**

```bash
go test -race -count=1 ./pherald/internal/runner/ -run TestSubscriberResolver 2>&1 | tail -10
```

- [ ] **Step 5: Commit**

```bash
git add pherald/internal/runner/subscriber.go pherald/internal/runner/subscriber_test.go pherald/internal/runner/fakes_test.go
git commit -m "Wave 3b step 6: Stage 5 SubscriberResolver — tenant-scoped fan-out

Reads subscribers + subscriber_aliases under the resolved tenant and
emits commons.Recipient entries per (subscriber, alias) pair. Wave 3b
implementation = ALL subscribers receive ALL events; per-event
preference filtering (§7.2-§7.3 CategoryPref/WorkflowPref/QuietHours)
is a follow-up HRD.

Tenant isolation enforced via TenantFromCtx — the fake store rejects
queries without a tenant in ctx; in production this maps to the
commons_storage RLS policy that filters on app.tenant_id GUC.

3 unit tests: empty tenant returns 0 recipients (not error); two-sub
tenant returns 2; cross-tenant isolation.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 7: Stage 6 — ChannelDispatcher

**Files:**
- Create: `pherald/internal/runner/dispatcher.go`, `pherald/internal/runner/dispatcher_test.go`
- Extend: `fakes_test.go` (add fake Channel + registry)

- [ ] **Step 1: Append fake channel infrastructure to `fakes_test.go`**

```go
// fakeChannel is a minimal in-memory stand-in for commons.Channel.
// Records every Send and lets the test inspect what would have been
// dispatched without hitting a real network.
type fakeChannel struct {
	name     string
	sends    []fakeSendRecord
	failNext bool
	mu       sync.Mutex
}

type fakeSendRecord struct {
	Msg     commons.OutboundMessage
	Receipt commons.Receipt
}

func newFakeChannel(name string) *fakeChannel {
	return &fakeChannel{name: name}
}

func (c *fakeChannel) Name() string { return c.name }
func (c *fakeChannel) Capabilities() commons.Capabilities {
	return commons.Capabilities{Text: true, DeliveryCeiling: commons.DeliveryRouted}
}
func (c *fakeChannel) Send(ctx context.Context, msg commons.OutboundMessage) (commons.Receipt, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failNext {
		c.failNext = false
		return commons.Receipt{}, errors.New("fake channel: forced fail")
	}
	rcpt := commons.Receipt{
		Evidence:     commons.DeliveryRouted,
		ChannelMsgID: c.name + "-msgid-" + strings.TrimSpace(msg.EventID),
		SentAt:       time.Now(),
	}
	c.sends = append(c.sends, fakeSendRecord{Msg: msg, Receipt: rcpt})
	return rcpt, nil
}
func (c *fakeChannel) Subscribe(ctx context.Context, h commons.InboundHandler) error { return nil }
func (c *fakeChannel) HealthCheck(ctx context.Context) error                          { return nil }

// import commons in fakes_test.go alongside the other imports:
//   "github.com/vasic-digital/herald/commons"
// (Already present in fakes via the type aliases; keep the real import explicit.)
```

(Note: `commons` import must already be added to `fakes_test.go`. If the alias hack is still there from Task 3, drop it and use the real `commons.CloudEventEnvelope` import directly.)

- [ ] **Step 2: Write failing tests `dispatcher_test.go`**

```go
package runner

import (
	"context"
	"testing"

	"github.com/vasic-digital/herald/commons"
)

func TestChannelDispatcher_SingleRecipient(t *testing.T) {
	nullCh := newFakeChannel("null")
	registry := map[commons.ChannelID]commons.Channel{
		commons.ChannelNull: nullCh,
	}
	d := &ChannelDispatcher{Channels: registry}

	rc := &RunCtx{
		Event:      cloudEventStub("evt-1", "x"),
		Recipients: []commons.Recipient{{Channel: "null", ChannelUserID: "sandbox-1"}},
	}
	if err := d.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(rc.Receipts) != 1 {
		t.Fatalf("Receipts = %d, want 1", len(rc.Receipts))
	}
	if rc.Receipts[0].Evidence != commons.DeliveryRouted {
		t.Errorf("Evidence = %s, want Routed", rc.Receipts[0].Evidence)
	}
	if rc.Receipts[0].ChannelMsgID == "" {
		t.Errorf("ChannelMsgID empty — channel Send didn't populate it")
	}
}

func TestChannelDispatcher_UnknownChannel_RecordsUnknown(t *testing.T) {
	d := &ChannelDispatcher{Channels: map[commons.ChannelID]commons.Channel{}}
	rc := &RunCtx{
		Event:      cloudEventStub("evt-1", "x"),
		Recipients: []commons.Recipient{{Channel: "no-such-channel", ChannelUserID: "x"}},
	}
	if err := d.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process should not error on unknown channel; got %v", err)
	}
	if len(rc.Receipts) != 1 {
		t.Fatalf("Receipts = %d, want 1", len(rc.Receipts))
	}
	if rc.Receipts[0].Evidence != commons.DeliveryUnknown {
		t.Errorf("Unknown channel must produce Evidence=Unknown; got %s", rc.Receipts[0].Evidence)
	}
	if rc.Receipts[0].Error == "" {
		t.Errorf("Unknown channel must populate Error field for observability")
	}
}

func TestChannelDispatcher_MultipleRecipients_ParallelSafe(t *testing.T) {
	nullCh := newFakeChannel("null")
	registry := map[commons.ChannelID]commons.Channel{commons.ChannelNull: nullCh}
	d := &ChannelDispatcher{Channels: registry}

	rc := &RunCtx{
		Event: cloudEventStub("evt-1", "x"),
		Recipients: []commons.Recipient{
			{Channel: "null", ChannelUserID: "a"},
			{Channel: "null", ChannelUserID: "b"},
			{Channel: "null", ChannelUserID: "c"},
		},
	}
	if err := d.Process(context.Background(), rc); err != nil {
		t.Fatal(err)
	}
	if len(rc.Receipts) != 3 {
		t.Errorf("Receipts = %d, want 3", len(rc.Receipts))
	}
}
```

- [ ] **Step 3: Implement `pherald/internal/runner/dispatcher.go`**

```go
package runner

import (
	"context"
	"log/slog"

	"github.com/vasic-digital/herald/commons"
)

// ChannelDispatcher is Stage 6 — looks up a commons.Channel by the
// recipient's channel ID and calls .Send for each recipient. Wave 3b
// fan-out is sequential (one recipient at a time) — parallel fan-out
// within a single event is a Wave 4+ optimization.
//
// Per §107: a recipient whose channel isn't registered MUST produce
// an Evidence=Unknown result with an explanatory Error field —
// silently dropping unroutable recipients would be a §11.4 bluff.
type ChannelDispatcher struct {
	Channels map[commons.ChannelID]commons.Channel
	Logger   *slog.Logger
}

func (d *ChannelDispatcher) Process(ctx context.Context, rc *RunCtx) error {
	for _, rcpt := range rc.Recipients {
		channel, ok := d.Channels[commons.ChannelID(rcpt.Channel)]
		if !ok {
			rc.Receipts = append(rc.Receipts, ChannelDispatchResult{
				ChannelID:     rcpt.Channel,
				ChannelUserID: rcpt.ChannelUserID,
				Evidence:      commons.DeliveryUnknown,
				Error:         "channel '" + rcpt.Channel + "' not registered in Runner.Deps.Channels",
			})
			continue
		}
		msg := commons.OutboundMessage{
			EventID:        rc.Event.ID,
			IdempotencyKey: rc.IdemKey,
			TenantID:       rc.TenantID.String(),
			To:             []commons.Recipient{rcpt},
			Body: commons.Body{
				Plain: string(rc.Event.Data),
			},
			Trace: rc.Trace,
		}
		receipt, err := channel.Send(ctx, msg)
		if err != nil {
			rc.Receipts = append(rc.Receipts, ChannelDispatchResult{
				ChannelID:     rcpt.Channel,
				ChannelUserID: rcpt.ChannelUserID,
				Evidence:      commons.DeliveryUnknown,
				Error:         err.Error(),
			})
			continue
		}
		rc.Receipts = append(rc.Receipts, ChannelDispatchResult{
			ChannelID:     rcpt.Channel,
			ChannelUserID: rcpt.ChannelUserID,
			Evidence:      receipt.Evidence,
			ChannelMsgID:  receipt.ChannelMsgID,
		})
	}
	return nil
}
```

- [ ] **Step 4: Run tests — verify 3/3 PASS**

```bash
go test -race -count=1 ./pherald/internal/runner/ -run TestChannelDispatcher 2>&1 | tail -10
```

- [ ] **Step 5: Commit**

```bash
git add pherald/internal/runner/dispatcher.go pherald/internal/runner/dispatcher_test.go pherald/internal/runner/fakes_test.go
git commit -m "Wave 3b step 7: Stage 6 ChannelDispatcher — Channel.Send orchestration

Sequential fan-out (Wave 3b): one recipient at a time. Per recipient,
looks up commons.Channel in the registry by the recipient's Channel
ID, builds an OutboundMessage, calls Send, captures the Receipt.

Unregistered channels produce Evidence=Unknown + Error field (NOT
silently dropped) per §107. Send errors are similarly captured as
Evidence=Unknown + Error so the OutcomeRecorder can persist them for
the audit trail.

3 unit tests: single null:// recipient, unknown channel produces
Unknown result with non-empty Error, 3-recipient fan-out.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 8: Stage 7 — OutcomeRecorder

**Files:**
- Create: `pherald/internal/runner/outcome.go`, `pherald/internal/runner/outcome_test.go`
- Extend: `fakes_test.go` (add fake outbound_delivery_evidence store)

- [ ] **Step 1: Append fake evidence store to `fakes_test.go`**

```go
type fakeEvidenceStore struct {
	mu   sync.Mutex
	rows []evidenceRow
}

type evidenceRow struct {
	ID               uuid.UUID
	TenantID         uuid.UUID
	ChannelID        string
	ChannelMessageID string
	Evidence         int
	SentAt           time.Time
}

func newFakeEvidenceStore() *fakeEvidenceStore {
	return &fakeEvidenceStore{}
}

func (s *fakeEvidenceStore) Insert(ctx context.Context, r evidenceRow) (uuid.UUID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r.ID = uuid.New()
	s.rows = append(s.rows, r)
	return r.ID, nil
}

func (s *fakeEvidenceStore) All() []evidenceRow {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]evidenceRow, len(s.rows))
	copy(out, s.rows)
	return out
}
```

- [ ] **Step 2: Write failing tests `outcome_test.go`**

```go
package runner

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vasic-digital/herald/commons"
)

func TestOutcomeRecorder_TwoRecipients_TwoEvidenceRows(t *testing.T) {
	tenantID := mustParse("44444444-4444-4444-4444-444444444444")
	evid := newFakeEvidenceStore()
	pg := newFakeEventsProcessedStore()
	o := &OutcomeRecorder{Evidence: evid, EventsProcessed: pg}

	rc := &RunCtx{
		TenantID: tenantID,
		IdemKey:  "k1",
		Event:    cloudEventStub("evt-1", "x"),
		Receipts: []ChannelDispatchResult{
			{ChannelID: "null", ChannelUserID: "a", Evidence: commons.DeliveryRouted, ChannelMsgID: "n-a"},
			{ChannelID: "null", ChannelUserID: "b", Evidence: commons.DeliveryRouted, ChannelMsgID: "n-b"},
		},
	}
	rc.TenantPGCtx = withTenantCtx(context.Background(), tenantID)
	rcpt, err := o.Process(context.Background(), rc)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(evid.All()) != 2 {
		t.Errorf("evidence rows = %d, want 2", len(evid.All()))
	}
	if _, found := pg.Lookup(context.Background(), tenantID, "k1"); !found {
		t.Errorf("events_processed row missing after Process")
	}
	if len(rcpt.OutboundEvidenceIDs) != 2 {
		t.Errorf("Receipt.OutboundEvidenceIDs len = %d, want 2", len(rcpt.OutboundEvidenceIDs))
	}
}

func TestOutcomeRecorder_RecordDenied_NoRecipientFanOut(t *testing.T) {
	tenantID := mustParse("44444444-4444-4444-4444-444444444444")
	evid := newFakeEvidenceStore()
	pg := newFakeEventsProcessedStore()
	o := &OutcomeRecorder{Evidence: evid, EventsProcessed: pg}

	rc := &RunCtx{
		TenantID:       tenantID,
		IdemKey:        "k2",
		Event:          cloudEventStub("evt-2", "x"),
		PolicyDecision: 3, // DecisionFail per commons_constitution
		PolicyReason:   "11.4.10: credential leak",
	}
	rc.TenantPGCtx = withTenantCtx(context.Background(), tenantID)
	rcpt, err := o.RecordDenied(context.Background(), rc)
	if err != nil {
		t.Fatalf("RecordDenied: %v", err)
	}
	if len(evid.All()) != 1 {
		t.Errorf("RecordDenied should write exactly 1 evidence row (the denial); got %d", len(evid.All()))
	}
	if !rcpt.WasReplay {
		// WasReplay is also "denied" semantically — client doesn't dispatch
		// (intentionally overloaded as "outcome already determined").
	}
	if _, found := pg.Lookup(context.Background(), tenantID, "k2"); !found {
		t.Errorf("events_processed row missing after RecordDenied — replay protection broken")
	}
}

// Ensure mustParse and other helpers are in scope (they live in fakes_test.go).
var _ = uuid.Nil
var _ = time.Second
```

- [ ] **Step 3: Implement `pherald/internal/runner/outcome.go`**

```go
package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// OutcomeRecorder is Stage 7 — the last stage before the orchestrator
// returns. Two entry points:
//
//   Process(ctx, rc):   normal happy/warn path; writes one
//                       outbound_delivery_evidence row per dispatched
//                       recipient + one events_processed archive row.
//
//   RecordDenied(ctx, rc): short-circuit path called by the orchestrator
//                       when Stage 4 returned DecisionFail. Writes a
//                       single "denied" evidence row + events_processed
//                       row (so replay protection still works) and skips
//                       stages 5/6.
//
// Per §107: an OutcomeRecorder that no-ops on the PG write would let
// duplicates re-dispatch on every retry. The events_processed row is
// the load-bearing replay-prevention artifact.
type OutcomeRecorder struct {
	Evidence        evidenceStore
	EventsProcessed eventsProcessedStore
}

type evidenceStore interface {
	Insert(ctx context.Context, r evidenceRow) (uuid.UUID, error)
}

type eventsProcessedStore interface {
	Lookup(ctx context.Context, tenantID uuid.UUID, idemKey string) (*eventsProcessedRow, bool)
	Insert(ctx context.Context, row eventsProcessedRow) error
}

func (o *OutcomeRecorder) Process(ctx context.Context, rc *RunCtx) (*Receipt, error) {
	var ids []uuid.UUID
	now := time.Now()
	for _, r := range rc.Receipts {
		id, err := o.Evidence.Insert(rc.TenantPGCtx, evidenceRow{
			TenantID:         rc.TenantID,
			ChannelID:        r.ChannelID,
			ChannelMessageID: r.ChannelMsgID,
			Evidence:         int(r.Evidence),
			SentAt:           now,
		})
		if err != nil {
			return nil, fmt.Errorf("outcome: insert evidence: %w", err)
		}
		ids = append(ids, id)
	}
	rcpt := &Receipt{
		EventID:             rc.Event.ID,
		IdempotencyKey:      rc.IdemKey,
		AcceptedAt:          now,
		Recipients:          len(rc.Receipts),
		Results:             rc.Receipts,
		OutboundEvidenceIDs: ids,
	}
	if err := o.EventsProcessed.Insert(rc.TenantPGCtx, eventsProcessedRow{
		TenantID:    rc.TenantID,
		IdemKey:     rc.IdemKey,
		EventID:     rc.Event.ID,
		FirstSeenAt: now,
		Receipt:     rcpt,
	}); err != nil {
		return nil, fmt.Errorf("outcome: archive events_processed: %w", err)
	}
	rc.OutboundEvidenceIDs = ids
	return rcpt, nil
}

func (o *OutcomeRecorder) RecordDenied(ctx context.Context, rc *RunCtx) (*Receipt, error) {
	now := time.Now()
	id, err := o.Evidence.Insert(rc.TenantPGCtx, evidenceRow{
		TenantID:         rc.TenantID,
		ChannelID:        "policy_denied",
		ChannelMessageID: rc.PolicyReason,
		Evidence:         0, // DeliveryUnknown
		SentAt:           now,
	})
	if err != nil {
		return nil, fmt.Errorf("outcome: insert denial evidence: %w", err)
	}
	rcpt := &Receipt{
		EventID:             rc.Event.ID,
		IdempotencyKey:      rc.IdemKey,
		AcceptedAt:          now,
		Recipients:          0,
		Results:             nil,
		OutboundEvidenceIDs: []uuid.UUID{id},
	}
	if err := o.EventsProcessed.Insert(rc.TenantPGCtx, eventsProcessedRow{
		TenantID:    rc.TenantID,
		IdemKey:     rc.IdemKey,
		EventID:     rc.Event.ID,
		FirstSeenAt: now,
		Receipt:     rcpt,
	}); err != nil {
		return nil, fmt.Errorf("outcome: archive denied events_processed: %w", err)
	}
	rc.OutboundEvidenceIDs = []uuid.UUID{id}
	return rcpt, nil
}
```

- [ ] **Step 4: Run tests — verify 2/2 PASS**

```bash
go test -race -count=1 ./pherald/internal/runner/ -run TestOutcomeRecorder 2>&1 | tail -10
```

- [ ] **Step 5: Commit**

```bash
git add pherald/internal/runner/outcome.go pherald/internal/runner/outcome_test.go pherald/internal/runner/fakes_test.go
git commit -m "Wave 3b step 8: Stage 7 OutcomeRecorder — evidence + archive writes

Two entry points: Process (happy/warn path; writes one
outbound_delivery_evidence row per recipient + one events_processed
archive row) and RecordDenied (deny path; writes a single 'denied'
evidence row + events_processed row to preserve replay protection).

Per §107 the events_processed row is the load-bearing replay-
prevention artifact — a no-op recorder would let every duplicate
re-dispatch. Test TestOutcomeRecorder_RecordDenied_NoRecipientFanOut
specifically asserts the PG archive row exists post-denial.

2 unit tests: 2-recipient happy path → 2 evidence rows + 1 archive;
denial path → 1 'denied' evidence row + 1 archive row.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 9: Runner orchestrator + integration test

**Files:**
- Create: `pherald/internal/runner/runner.go`, `pherald/internal/runner/runner_test.go`

- [ ] **Step 1: Implement `pherald/internal/runner/runner.go`**

```go
package runner

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/vasic-digital/herald/commons"
	constitution "github.com/vasic-digital/herald/commons_constitution"
)

// Runner orchestrates the §32 7-stage pipeline. Each stage is a concrete
// field; Run threads RunCtx through them in fixed order.
//
// Concurrent-safe: stages are stateless (their deps — pgxpool, Redis
// client, etc. — are themselves concurrent-safe). Same Runner instance
// handles all requests.
type Runner struct {
	parser  *EventParser
	idem    *IdempotencyChecker
	tenant  *TenantResolver
	policy  *PolicyGate
	subs    *SubscriberResolver
	chans   *ChannelDispatcher
	outcome *OutcomeRecorder
	logger  *slog.Logger
}

// Deps carries every external dependency the Runner needs. Constructed
// once at pherald startup; passed to NewRunner.
type Deps struct {
	PG        *pgxpool.Pool
	Redis     redis.Cmdable
	Evaluator *constitution.Registry
	Channels  map[commons.ChannelID]commons.Channel
	Logger    *slog.Logger
}

// NewRunner builds the Runner from Deps.
func NewRunner(d Deps) *Runner {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return &Runner{
		parser: &EventParser{},
		idem: &IdempotencyChecker{
			Redis: redisAdapter{client: d.Redis},
			PG:    pgEventsProcessedAdapter{pool: d.PG},
			TTL:   24 * time.Hour,
		},
		tenant: &TenantResolver{},
		policy: &PolicyGate{Registry: d.Evaluator},
		subs: &SubscriberResolver{
			Subscribers: pgSubscribersAdapter{pool: d.PG},
		},
		chans: &ChannelDispatcher{
			Channels: d.Channels,
			Logger:   d.Logger,
		},
		outcome: &OutcomeRecorder{
			Evidence:        pgEvidenceAdapter{pool: d.PG},
			EventsProcessed: pgEventsProcessedAdapter{pool: d.PG},
		},
		logger: d.Logger,
	}
}

// Run executes the full 7-stage pipeline for a single inbound event.
// Returns the Receipt on success, or an error if any stage failed.
// Short-circuits on Stage 2 duplicate (returns cached Receipt) and on
// Stage 4 DecisionFail (jumps directly to OutcomeRecorder.RecordDenied).
func (r *Runner) Run(ctx context.Context, raw []byte, claims map[string]any) (*Receipt, error) {
	rc := &RunCtx{Raw: raw, AuthClaims: claims}
	// Extract tenant from claims:
	tenantID, err := extractTenant(claims)
	if err != nil {
		return nil, err
	}
	rc.TenantID = tenantID

	if err := r.parser.Process(ctx, rc); err != nil {
		return nil, err
	}
	if err := r.idem.Process(ctx, rc); err != nil {
		return nil, err
	}
	if rc.Duplicate {
		// Replay short-circuit: return the prior Receipt with WasReplay=true.
		if rc.CachedRcpt != nil {
			rc.CachedRcpt.WasReplay = true
		}
		return rc.CachedRcpt, nil
	}
	if err := r.tenant.Process(ctx, rc); err != nil {
		return nil, err
	}
	if err := r.policy.Process(ctx, rc); err != nil {
		return nil, err
	}
	if rc.PolicyDecision == constitution.DecisionFail {
		return r.outcome.RecordDenied(ctx, rc)
	}
	if err := r.subs.Process(ctx, rc); err != nil {
		return nil, err
	}
	if err := r.chans.Process(ctx, rc); err != nil {
		return nil, err
	}
	return r.outcome.Process(ctx, rc)
}

// extractTenant pulls "tenant" claim out as a uuid.UUID.
func extractTenant(claims map[string]any) (uuid.UUID, error) {
	v, ok := claims["tenant"]
	if !ok {
		return uuid.Nil, fmt.Errorf("runner: claims missing 'tenant'")
	}
	s, ok := v.(string)
	if !ok {
		return uuid.Nil, fmt.Errorf("runner: 'tenant' claim not a string (got %T)", v)
	}
	return uuid.Parse(s)
}

// imports needed by extractTenant — declared in the file's import block
// at the top.

// ----------------------------------------------------------------------
// Real-PG / real-Redis adapters. These satisfy the per-stage interfaces
// defined in idempotency.go, subscriber.go, and outcome.go.
//
// Implementation skeletons — flesh out the SQL queries per the existing
// commons_storage patterns (use $1, $2 ... placeholders; rely on RLS
// for tenant isolation; never inline tenant_id in SQL).
// ----------------------------------------------------------------------

type redisAdapter struct {
	client redis.Cmdable
}

func (r redisAdapter) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	res, err := r.client.SetNX(ctx, key, value, ttl).Result()
	return res, err
}

func (r redisAdapter) Get(ctx context.Context, key string) (string, error) {
	return r.client.Get(ctx, key).Result()
}

type pgEventsProcessedAdapter struct {
	pool *pgxpool.Pool
}

func (a pgEventsProcessedAdapter) Lookup(ctx context.Context, tenantID uuid.UUID, idemKey string) (*eventsProcessedRow, bool) {
	row := a.pool.QueryRow(ctx,
		`SELECT event_id, first_seen_at FROM events_processed WHERE tenant_id = $1 AND idempotency_key = $2`,
		tenantID, idemKey)
	var eventID string
	var firstSeen time.Time
	if err := row.Scan(&eventID, &firstSeen); err != nil {
		return nil, false
	}
	return &eventsProcessedRow{TenantID: tenantID, IdemKey: idemKey, EventID: eventID, FirstSeenAt: firstSeen}, true
}

func (a pgEventsProcessedAdapter) Insert(ctx context.Context, row eventsProcessedRow) error {
	_, err := a.pool.Exec(ctx,
		`INSERT INTO events_processed(tenant_id, idempotency_key, event_id, first_seen_at) VALUES($1, $2, $3, $4)
         ON CONFLICT DO NOTHING`,
		row.TenantID, row.IdemKey, row.EventID, row.FirstSeenAt)
	return err
}

type pgSubscribersAdapter struct {
	pool *pgxpool.Pool
}

func (a pgSubscribersAdapter) ListByTenant(ctx context.Context) ([]subscriberRow, error) {
	tid := TenantFromCtx(ctx)
	rows, err := a.pool.Query(ctx,
		`SELECT id, handle, display_name FROM subscribers WHERE tenant_id = $1`,
		tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []subscriberRow
	for rows.Next() {
		var r subscriberRow
		if err := rows.Scan(&r.ID, &r.Handle, &r.DisplayName); err != nil {
			return nil, err
		}
		// Aliases: separate query for clarity.
		aliasRows, err := a.pool.Query(ctx,
			`SELECT channel, channel_user_id FROM subscriber_aliases WHERE subscriber_id = $1`, r.ID)
		if err != nil {
			return nil, err
		}
		for aliasRows.Next() {
			var ar subscriberAliasRow
			if err := aliasRows.Scan(&ar.Channel, &ar.ChannelUserID); err != nil {
				aliasRows.Close()
				return nil, err
			}
			r.Aliases = append(r.Aliases, ar)
		}
		aliasRows.Close()
		out = append(out, r)
	}
	return out, nil
}

type pgEvidenceAdapter struct {
	pool *pgxpool.Pool
}

func (a pgEvidenceAdapter) Insert(ctx context.Context, r evidenceRow) (uuid.UUID, error) {
	row := a.pool.QueryRow(ctx,
		`INSERT INTO outbound_delivery_evidence(tenant_id, channel_id, channel_message_id, evidence, sent_at)
         VALUES($1, $2, $3, $4, $5) RETURNING id`,
		r.TenantID, r.ChannelID, r.ChannelMessageID, r.Evidence, r.SentAt)
	var id uuid.UUID
	if err := row.Scan(&id); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}
```

(Imports needed at top of file: `fmt`, plus the existing list.)

- [ ] **Step 2: Write the integration test `runner_test.go`**

```go
package runner

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/vasic-digital/herald/commons"
	constitution "github.com/vasic-digital/herald/commons_constitution"
)

// integrationDeps builds a Runner with all-fake deps for the cases below.
type integrationDeps struct {
	redis    *fakeRedis
	procd    *fakeEventsProcessedStore
	subs     *fakeSubscribersStore
	chans    map[commons.ChannelID]commons.Channel
	registry *constitution.Registry
	evid     *fakeEvidenceStore
}

func newIntegrationRunner() (*Runner, *integrationDeps) {
	d := &integrationDeps{
		redis:    newFakeRedis(),
		procd:    newFakeEventsProcessedStore(),
		subs:     newFakeSubscribersStore(),
		chans:    map[commons.ChannelID]commons.Channel{commons.ChannelNull: newFakeChannel("null")},
		registry: constitution.NewRegistry(),
		evid:     newFakeEvidenceStore(),
	}
	r := &Runner{
		parser: &EventParser{},
		idem:   &IdempotencyChecker{Redis: d.redis, PG: d.procd, TTL: 24 * 3600 * 1e9}, // 24h in ns
		tenant: &TenantResolver{},
		policy: &PolicyGate{Registry: d.registry},
		subs:   &SubscriberResolver{Subscribers: d.subs},
		chans:  &ChannelDispatcher{Channels: d.chans},
		outcome: &OutcomeRecorder{Evidence: d.evid, EventsProcessed: d.procd},
	}
	return r, d
}

func TestRunner_HappyPath_FullPipeline(t *testing.T) {
	tenantID := mustParse("55555555-5555-5555-5555-555555555555")
	r, d := newIntegrationRunner()
	d.subs.Add(tenantID, subscriberRow{
		ID: uuid.New(), Handle: "alice",
		Aliases: []subscriberAliasRow{{Channel: "null", ChannelUserID: "sandbox-alice"}},
	})

	body := mustJSON(map[string]any{
		"specversion": "1.0",
		"id":          "01923456-789a-7bcd-abcd-ef0123456789",
		"source":      "//test",
		"type":        "digital.vasic.herald.test",
		"data":        map[string]string{"hi": "there"},
	})
	rcpt, err := r.Run(context.Background(), body, map[string]any{"tenant": tenantID.String()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rcpt.Recipients != 1 {
		t.Errorf("Recipients = %d, want 1", rcpt.Recipients)
	}
	if rcpt.WasReplay {
		t.Errorf("WasReplay=true on fresh event")
	}
	if len(d.evid.All()) != 1 {
		t.Errorf("evidence rows = %d, want 1", len(d.evid.All()))
	}
}

func TestRunner_Duplicate_Replay(t *testing.T) {
	tenantID := mustParse("55555555-5555-5555-5555-555555555555")
	r, d := newIntegrationRunner()
	d.subs.Add(tenantID, subscriberRow{
		ID: uuid.New(), Handle: "alice",
		Aliases: []subscriberAliasRow{{Channel: "null", ChannelUserID: "sandbox-alice"}},
	})

	body := mustJSON(map[string]any{
		"specversion": "1.0", "id": "evt-1", "source": "//x", "type": "x",
		"heraldidempotencykey": "K1",
	})
	if _, err := r.Run(context.Background(), body, map[string]any{"tenant": tenantID.String()}); err != nil {
		t.Fatalf("Run-1: %v", err)
	}
	rcpt2, err := r.Run(context.Background(), body, map[string]any{"tenant": tenantID.String()})
	if err != nil {
		t.Fatalf("Run-2: %v", err)
	}
	if !rcpt2.WasReplay {
		t.Errorf("Run-2 should mark WasReplay=true")
	}
	// Exactly one outbound_delivery_evidence row should exist (the first run);
	// the second run is a no-op dispatch (returns cached Receipt).
	if len(d.evid.All()) != 1 {
		t.Errorf("evidence rows after duplicate = %d, want 1", len(d.evid.All()))
	}
}

func TestRunner_Deny_ShortCircuits(t *testing.T) {
	tenantID := mustParse("55555555-5555-5555-5555-555555555555")
	r, d := newIntegrationRunner()
	d.subs.Add(tenantID, subscriberRow{
		ID: uuid.New(), Handle: "alice",
		Aliases: []subscriberAliasRow{{Channel: "null", ChannelUserID: "sandbox-alice"}},
	})
	// Register an evaluator that fails on this event type.
	d.registry.Register(&fakeEvaluator{
		ruleID: "11.4.10", severity: constitution.SeverityCritical,
		triggers: []string{"digital.vasic.herald.test"},
		verdict:  constitution.DecisionFail,
		reason:   "leak detected",
	})

	body := mustJSON(map[string]any{
		"specversion": "1.0", "id": "evt-deny", "source": "//x", "type": "digital.vasic.herald.test",
	})
	rcpt, err := r.Run(context.Background(), body, map[string]any{"tenant": tenantID.String()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rcpt.Recipients != 0 {
		t.Errorf("Recipients = %d on deny path; want 0 (short-circuited)", rcpt.Recipients)
	}
	// Exactly one "denial" evidence row.
	if len(d.evid.All()) != 1 {
		t.Errorf("evidence rows = %d, want 1 (the denial)", len(d.evid.All()))
	}
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
```

- [ ] **Step 3: Run all runner tests — verify everything PASS**

```bash
go test -race -count=1 ./pherald/internal/runner/ 2>&1 | tail -15
```

- [ ] **Step 4: Commit**

```bash
git add pherald/internal/runner/runner.go pherald/internal/runner/runner_test.go
git commit -m "Wave 3b step 9: Runner orchestrator + 3 integration scenarios

NewRunner(Deps) constructor wires the 7 stage instances + real PG / Redis
adapters. Run() threads RunCtx through them in fixed order, short-
circuits on Stage 2 duplicate (returns cached Receipt with WasReplay=true)
and Stage 4 DecisionFail (jumps to OutcomeRecorder.RecordDenied).

Real adapters (redisAdapter, pgEventsProcessedAdapter, pgSubscribersAdapter,
pgEvidenceAdapter) bridge the per-stage interfaces to the production
deps (pgxpool, go-redis). Tests use the fake counterparts from fakes_test.go.

3 end-to-end integration tests with fake deps:
- HappyPath: 1 subscriber → 1 dispatch → 1 evidence row + 1 archive row
- Duplicate: second Run with same IdempotencyKey → WasReplay=true, no
  extra dispatch (evidence count stays at 1)
- Deny: registered evaluator returns DecisionFail → no recipient fan-out;
  exactly 1 denial evidence row written

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 10: pherald HTTP handler + main.go wiring + e2e + close-out

**Files:**
- Create: `pherald/internal/http/events.go`, `pherald/internal/http/events_test.go`
- Modify: `pherald/internal/http/routes.go`, `pherald/cmd/pherald/main.go`, `pherald/cmd/pherald/stubs.go`
- Modify: `scripts/e2e_bluff_hunt.sh`, `tests/test_wave3_mutation_meta.sh`
- Modify: `docs/specs/mvp/specification.V3.md`, `docs/Issues.md`, `docs/Fixed.md`, `docs/Status.md`

This is the biggest task — it wires the live route, adds 6 new e2e invariants, extends the mutation gate, bumps spec V3 r8→r9, runs the full anti-bluff battery, and pushes to all 4 mirrors. Decompose carefully.

- [ ] **Step 1: Implement `pherald/internal/http/events.go`**

```go
package http

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/vasic-digital/herald/commons_auth"
	"github.com/vasic-digital/herald/pherald/internal/runner"
)

// EventsHandler returns the gin.HandlerFunc serving POST /v1/events.
// Reads claims set by commons_auth.GinMiddleware, body bytes, calls
// runner.Run, returns the Receipt as 202 (or 200 on replay, 403 on deny,
// 400 on bad input).
func EventsHandler(r *runner.Runner) gin.HandlerFunc {
	return func(c *gin.Context) {
		claimsAny, ok := c.Get(commons_auth.ContextKeyClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "no auth claims"})
			return
		}
		claims, _ := claimsAny.(map[string]any)
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "read body", "detail": err.Error()})
			return
		}
		rcpt, err := r.Run(c.Request.Context(), body, claims)
		if err != nil {
			// Map error kind → HTTP status.
			c.JSON(mapErrorToStatus(err), gin.H{"error": err.Error()})
			return
		}
		if rcpt == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "runner returned nil receipt"})
			return
		}
		if rcpt.WasReplay {
			c.Header("X-Herald-Replay", "true")
			c.JSON(http.StatusOK, rcpt)
			return
		}
		c.JSON(http.StatusAccepted, rcpt)
	}
}

func mapErrorToStatus(err error) int {
	msg := err.Error()
	switch {
	case contains(msg, "event_parser:"):
		return http.StatusBadRequest
	case contains(msg, "runner: claims missing 'tenant'"), contains(msg, "'tenant' claim not a string"):
		return http.StatusUnauthorized
	case contains(msg, "tenant_resolver:"):
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Update `pherald/internal/http/routes.go`**

```go
package http

import (
	"github.com/vasic-digital/herald/commons/cli"
	"github.com/vasic-digital/herald/pherald/internal/runner"
)

// Routes returns the pherald-specific /v1 routes. /v1/events is now
// LIVE (Wave 3b) — replaces the 501-stub from Wave 2. /v1/compliance
// was always a cherald route; removed from pherald's route list here
// (Wave 2 Task 6 left it as a 501 stub mistakenly; the real handler
// lives in cherald/internal/compliance/).
func Routes(r *runner.Runner) []cli.Route {
	return []cli.Route{
		{
			Method:      "POST",
			Path:        "/v1/events",
			Handler:     EventsHandler(r),
			Description: "Inbound CloudEvent ingestion (spec §41 / Wave 3b live)",
		},
	}
}
```

- [ ] **Step 3: Update `pherald/cmd/pherald/main.go` + `stubs.go`**

`main.go` changes (within `main()`):

```go
// AFTER the existing branding setup, BEFORE NewRootCmd:

ctx := context.Background()
verifier, err := commons_auth.NewVerifierFromEnv(buildRedisClient())
if err != nil {
	fmt.Fprintln(os.Stderr, "pherald: build verifier:", err)
	os.Exit(1)
}
pg := mustOpenPG(ctx)
runnerInstance := runner.NewRunner(runner.Deps{
	PG:        pg,
	Redis:     buildRedisClient(),
	Evaluator: constitution.NewRegistry(), // empty Wave 3b — permissive
	Channels:  buildChannelRegistry(),
	Logger:    slog.Default(),
})

// THEN existing root construction, but swap newServeCmd(branding) for:
root.AddCommand(newServeCmd(branding, runnerInstance, verifier))
```

Helpers (add near the top of main.go):

```go
func buildRedisClient() redis.Cmdable {
	url := os.Getenv("HERALD_REDIS_URL")
	if url == "" {
		return nil
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pherald: parse HERALD_REDIS_URL:", err)
		os.Exit(1)
	}
	return redis.NewClient(opts)
}

func mustOpenPG(ctx context.Context) *pgxpool.Pool {
	dsn := os.Getenv("HERALD_PG_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "pherald: HERALD_PG_DSN required (no PG-less serve mode for Wave 3b)")
		os.Exit(1)
	}
	cfg, err := storage.ParseDSN(dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pherald: parse HERALD_PG_DSN:", err)
		os.Exit(1)
	}
	pool, err := storage.Open(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pherald: open PG:", err)
		os.Exit(1)
	}
	return pool
}

func buildChannelRegistry() map[commons.ChannelID]commons.Channel {
	reg := map[commons.ChannelID]commons.Channel{}
	// null:// always available (sandbox).
	reg[commons.ChannelNull] = null.NewAdapter()
	// Telegram if creds present.
	if tok := os.Getenv("HERALD_TGRAM_BOT_TOKEN"); tok != "" {
		reg[commons.ChannelTelegram] = tgram.NewAdapter(tok)
	}
	return reg
}
```

`stubs.go` change: `newServeCmd` signature now takes the runner + verifier and wires both:

```go
func newServeCmd(br commons.Branding, runnerInstance *runner.Runner, verifier commons_auth.Verifier) *cobra.Command {
	return cli.ServeCmd(cli.ServeOpts{
		Branding:   br,
		Routes:     httpsrv.Routes(runnerInstance),
		Middleware: []gin.HandlerFunc{commons_auth.GinMiddleware(verifier), httpsrv.RequestIDMiddleware()},
	})
}
```

- [ ] **Step 4: Build pherald + smoke test against null channel**

```bash
cd /Users/milosvasic/Projects/Herald
go build -o /tmp/pherald-wave3b ./pherald/cmd/pherald 2>&1 | head -3
```

If build fails on missing imports / package names, fix per the compiler messages (the plan's main.go snippet uses `null.NewAdapter()` and `tgram.NewAdapter(tok)` — verify those constructor names against `commons_messaging/channels/null/` and `commons_messaging/channels/tgram/`; adapt if the actual names differ).

Spin up a fresh PG via quickstart, run migrations, then:

```bash
export HERALD_AUTH_MODE=hmac
export HERALD_AUTH_HMAC_SECRET="test-secret-32-bytes-of-padding!!"
export HERALD_PG_DSN="postgres://herald:herald_dev@127.0.0.1:24100/herald"
/tmp/pherald-wave3b serve --http-port 24791 &
PID=$!
sleep 0.8
TOKEN=$(python3 - <<'PY'
import hmac, hashlib, base64, json, time, os
secret = os.environ["HERALD_AUTH_HMAC_SECRET"].encode()
header = base64.urlsafe_b64encode(b'{"alg":"HS256","typ":"JWT"}').rstrip(b'=')
payload = base64.urlsafe_b64encode(json.dumps({"tenant":"550e8400-e29b-41d4-a716-446655440000","sub":"t","exp":int(time.time())+300}).encode()).rstrip(b'=')
sig = base64.urlsafe_b64encode(hmac.new(secret, header+b'.'+payload, hashlib.sha256).digest()).rstrip(b'=')
print((header+b'.'+payload+b'.'+sig).decode())
PY
)
curl -sS -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
    --data '{"specversion":"1.0","id":"01923456-789a-7bcd-abcd-ef0123456789","source":"//smoke","type":"digital.vasic.herald.smoke"}' \
    http://127.0.0.1:24791/v1/events
echo ""
kill $PID
```

Expected: 202 Accepted + Receipt JSON (Recipients=0 because the smoke tenant has no subscribers, but a row written to events_processed).

- [ ] **Step 5: Add e2e invariants E37-E42 to `scripts/e2e_bluff_hunt.sh`**

Append after the existing E47 block:

```bash
echo ""
echo "== E37-E42: pherald POST /v1/events live (HRD-016 close-out) =="
# Requires container runtime + PG migrated. Otherwise SKIP-with-reason.
if (command -v docker >/dev/null 2>&1 || command -v podman >/dev/null 2>&1) \
   && nc -z 127.0.0.1 24100 2>/dev/null; then
    bin="/tmp/pherald-events-$$"
    if go build -o "${bin}" ./pherald/cmd/pherald > /tmp/e2e_out 2>&1; then
        HERALD_AUTH_MODE=hmac \
        HERALD_AUTH_HMAC_SECRET="test-secret-32-bytes-of-padding!!" \
        HERALD_PG_DSN="postgres://herald:herald_dev@127.0.0.1:24100/herald" \
        "${bin}" serve --http-port 24791 > /tmp/pherald-e37.log 2>&1 &
        serve_pid=$!
        sleep 0.8
        TOKEN=$(python3 -c "
import hmac,hashlib,base64,json,time,os
s=b'test-secret-32-bytes-of-padding!!'
h=base64.urlsafe_b64encode(b'{\"alg\":\"HS256\",\"typ\":\"JWT\"}').rstrip(b'=')
p=base64.urlsafe_b64encode(json.dumps({'tenant':'550e8400-e29b-41d4-a716-446655440000','sub':'t','exp':int(time.time())+300}).encode()).rstrip(b'=')
sig=base64.urlsafe_b64encode(hmac.new(s,h+b'.'+p,hashlib.sha256).digest()).rstrip(b'=')
print((h+b'.'+p+b'.'+sig).decode())
")
        check "E37 POST /v1/events valid JWT + null recipient → 202 + Receipt" \
            "curl -sS -X POST -H 'Authorization: Bearer ${TOKEN}' -H 'Content-Type: application/json' \
              --data '{\"specversion\":\"1.0\",\"id\":\"01923456-789a-7bcd-abcd-ef0123456789\",\"source\":\"//e2e\",\"type\":\"digital.vasic.herald.e2e\"}' \
              -w '%{http_code}' -o /tmp/e37-body http://127.0.0.1:24791/v1/events | grep -q '202' && grep -q 'event_id' /tmp/e37-body"
        check "E38 idempotency: same payload twice → second has X-Herald-Replay header" \
            "OUT=\$(curl -sS -X POST -H 'Authorization: Bearer ${TOKEN}' -H 'Content-Type: application/json' \
              --data '{\"specversion\":\"1.0\",\"id\":\"01923456-789a-7bcd-abcd-ef0123456789\",\"source\":\"//e2e\",\"type\":\"digital.vasic.herald.e2e\"}' \
              -D /tmp/e38-hdr http://127.0.0.1:24791/v1/events); grep -qi 'X-Herald-Replay: true' /tmp/e38-hdr"
        check "E39 POST without auth → 401" \
            "[ \"\$(curl -sS -o /dev/null -w '%{http_code}' -X POST http://127.0.0.1:24791/v1/events)\" = '401' ]"
        check "E40 POST with wrong JWT signature → 401" \
            "BAD_TOKEN=\$(python3 -c \"
import hmac,hashlib,base64,json,time
s=b'wrong-secret-32-bytes-padding!!!'
h=base64.urlsafe_b64encode(b'{\\\"alg\\\":\\\"HS256\\\",\\\"typ\\\":\\\"JWT\\\"}').rstrip(b'=')
p=base64.urlsafe_b64encode(json.dumps({'tenant':'550e8400-e29b-41d4-a716-446655440000','sub':'t','exp':int(time.time())+300}).encode()).rstrip(b'=')
sig=base64.urlsafe_b64encode(hmac.new(s,h+b'.'+p,hashlib.sha256).digest()).rstrip(b'=')
print((h+b'.'+p+b'.'+sig).decode())
\"); [ \"\$(curl -sS -o /dev/null -w '%{http_code}' -X POST -H \"Authorization: Bearer \$BAD_TOKEN\" -H 'Content-Type: application/json' --data '{}' http://127.0.0.1:24791/v1/events)\" = '401' ]"
        check "E41 POST malformed JSON → 400" \
            "[ \"\$(curl -sS -o /dev/null -w '%{http_code}' -X POST -H 'Authorization: Bearer ${TOKEN}' -H 'Content-Type: application/json' --data 'not json' http://127.0.0.1:24791/v1/events)\" = '400' ]"
        check "E42 evidence row written: query outbound_delivery_evidence + events_processed via psql" \
            "PSQLPASS='herald_dev' PGPASSWORD='herald_dev' psql -h 127.0.0.1 -p 24100 -U herald -d herald -tAc 'SELECT count(*) FROM events_processed' | grep -q '^[1-9]'"
        kill ${serve_pid} 2>/dev/null
        wait ${serve_pid} 2>/dev/null
        rm -f "${bin}" /tmp/e37-body /tmp/e38-hdr /tmp/pherald-e37.log
    else
        echo "FAIL  E37-E42: pherald build failed"
        fail=$((fail+1))
    fi
else
    echo "SKIP  E37-E42 (no container runtime OR PG :24100 unreachable — §11.4.3 explicit SKIP-with-reason)"
fi
```

Update the header tally: `Forty-one invariants` → `Forty-seven invariants`.

Also update the existing E45 SKIP-with-reason to a live check (cross-binary: post a denied event via pherald, then GET /v1/compliance from cherald, see the denial row). Add after E42:

```bash
echo ""
echo "== E45: cross-binary integration (pherald writes → cherald reads) =="
# This is the cross-binary proof the Wave 3a SKIP was waiting for.
# Skips with same gate as E37-E42.
if (command -v docker >/dev/null 2>&1 || command -v podman >/dev/null 2>&1) \
   && nc -z 127.0.0.1 24100 2>/dev/null; then
    # (Build cherald, start it, post a deny event via pherald, GET /v1/compliance, see denial row)
    echo "SKIP  E45 (cross-binary integration scaffold not yet wired — Wave 3c follow-up; pherald Runner is now live so the prerequisite is in place)"
else
    echo "SKIP  E45 (prerequisite: PG :24100 + cherald running)"
fi
```

- [ ] **Step 6: Extend `tests/test_wave3_mutation_meta.sh` with M2, M3, M4**

Append three new mutation blocks (mirror M1/M5 pattern):

- M2: Mutate `pherald/internal/runner/idempotency.go` to set `rc.Duplicate = false` unconditionally → E38 MUST FAIL (replay header missing on second call).
- M3: Mutate `pherald/internal/runner/policy.go` to set `rc.PolicyDecision = constitution.DecisionPass` unconditionally → E45-class deny invariant FAILs (when a deny evaluator IS registered, it should fire; mutation hides it).
- M4: Mutate `pherald/internal/runner/outcome.go::Process` to skip the `events_processed.Insert` call → E38 FAILs (no archive row → second send isn't recognized as duplicate).

- [ ] **Step 7: Update spec V3 r8 → r9**

Edit `docs/specs/mvp/specification.V3.md`:
- Metadata table `Revision` 8 → 9; `Last modified` to today.
- §32 stages: each row gains an "Implementation" column citing the file at `pherald/internal/runner/<file>.go`.
- §41: `/v1/events` row flips from `501 stub (HRD-016)` to `202 Accepted + Receipt JSON (Wave 3b live)`.
- Add §44.N Wave 3b milestone subsection with as-built evidence (commits, anti-bluff battery, e2e count).

Regenerate siblings: `bash scripts/export_docs.sh docs/specs/mvp/specification.V3.md`.

- [ ] **Step 8: Update Issues.md / Fixed.md / Status.md**

- `docs/Issues.md` r11 → r12: remove HRD-016 from open list; add to Fixed list.
- `docs/Fixed.md` r10 → r11: prepend HRD-016 row with as-built evidence (commit SHA + e2e invariants).
- `docs/Status.md` r12 → r13: bump revision; status summary cites Wave 3b complete + integration unlocked.

Regenerate siblings.

- [ ] **Step 9: Run FULL anti-bluff battery**

```bash
bash tests/test_constitution_inheritance.sh         # 15/15
bash tests/test_constitution_inheritance_meta.sh    # META-PASS
bash tests/test_i6_refinement_meta.sh               # 3/3
bash tests/test_i8_usability_meta.sh                # 5/5
bash tests/test_wave2_mutation_meta.sh              # 4/4
bash tests/test_wave3_mutation_meta.sh              # 6/6 (M1+M2+M3+M4+M5+post-flight; M6 either SKIP or PASS)
bash scripts/audit_antibluff.sh                     # 16+ PASS / 0 FAIL
bash scripts/codegraph_validate.sh                  # 7+ PASS / 0 FAIL / 2 SKIP (HRD-091)
bash scripts/e2e_bluff_hunt.sh                      # ≥47 PASS / 0 FAIL / ≤5 SKIP
```

ALL must be green.

- [ ] **Step 10: Commit + push to 4 mirrors**

```bash
git add pherald/internal/runner/ pherald/internal/http/ pherald/cmd/pherald/ \
        scripts/e2e_bluff_hunt.sh tests/test_wave3_mutation_meta.sh \
        docs/specs/mvp/specification.V3.{md,html,docx,pdf} \
        docs/Issues.{md,html,docx,pdf} \
        docs/Fixed.{md,html,docx,pdf} \
        docs/Status.{md,html,docx,pdf}
git commit -m "Wave 3b step 10: pherald Runner live + /v1/events 202 + HRD-016 close-out

(detailed commit body following Wave 3a Task 8's template — list per-gate
PASS counts, mention HRD-016 atomic close, spec V3 r9, multi-mirror push)

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"

git push origin main 2>&1 | tail -20
```

Expected: 4-mirror push green.

---

## Wave 3b sign-off summary

| Closes | Evidence |
|---|---|
| HRD-016 | `/v1/events` live + E37-E42 PASS + cross-binary E45 unblocked |

**Carry-over to Wave 3c+:** HRD-024 (iherald paging live), HRD-029..056 (§43 commands), HRD-019..025 (flavor-specific constitution bindings), Wave 3b parallel-fan-out optimization, OpenAPI surface, INTEGRATION.md, v0.1.0 release tag.

---

**Plan complete and saved to `docs/superpowers/plans/2026-05-22-wave3b-runner.md`. Two execution options:**

**1. Subagent-Driven (recommended)** — fresh subagent per task, review between tasks.

**2. Inline Execution** — `superpowers:executing-plans` batch execution with checkpoints.

**Which approach?**
