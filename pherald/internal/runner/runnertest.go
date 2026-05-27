// Wave 4b Task 5 — exported test-fake Runner constructor.
//
// This file is the moral equivalent of net/http/httptest: it lives in the
// runner package (because the stage interfaces — idempotencyRedis,
// idempotencyPG, subscribersStore, evidenceStore, eventsProcessedStore —
// reference unexported types like eventsProcessedRow / subscriberRow /
// evidenceRow that an external package cannot satisfy) and exports a
// NewFakeRunner constructor for use by HTTP-handler tests in sibling
// packages (e.g. pherald/internal/http).
//
// The same in-memory fakes that drive fakes_test.go's
// fakeRedis/fakeEventsProcessedStore/fakeSubscribersStore/fakeEvidenceStore
// are re-exported here as Fake* types so external test packages can build a
// fully-wired Runner WITHOUT pulling in real Postgres or Redis.
//
// §107 anti-bluff: these fakes implement the SAME stage contracts the real
// PG/Redis adapters do — Lookup/Insert/SetNX/Get/ListByTenant all return
// realistic values; the runner's 7 stages see no behavioural difference
// from the production path. The W4b T5 tests that exercise TOON request +
// response codec negotiation through the EventsHandler can therefore drive
// the full pipeline to a Receipt without needing a PG container or a
// miniredis instance.
//
// Constraint: this file MUST stay below the test_audit budget by carrying
// only test-fake code. The runner package's production binary path imports
// nothing defined here.

package runner

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/vasic-digital/herald/commons"
)

// NewFakeRunner returns a Runner wired with in-memory fakes for every
// stage's external dependency (Redis, PG events_processed, PG subscribers,
// PG outbound_delivery_evidence, channel dispatcher). The returned Runner
// drives all 7 stages end-to-end against the fakes:
//
//   - Idempotency: in-memory map (SetNX + Get + Lookup).
//   - Tenant resolution: synthesises a PG-equivalent context via the
//     production TenantResolver path (no DB needed; TenantResolver just
//     stamps the tenant into ctx via WithTenant).
//   - Policy: empty registry → permissive (every event accepted).
//   - Subscribers: empty by default; tests can call SubscribersStore().Add
//     to seed before driving the Runner.
//   - Channels: null:// always registered (see commons.ChannelNull); tests
//     that need TOON envelope round-trip don't actually need a delivery
//     channel firing.
//   - Outcome: in-memory evidence + events_processed; tests can inspect
//     after Run to verify writes happened.
//
// Each call returns a fresh Runner + its FakeStores handle so per-test
// state is isolated. The handle exposes the underlying fakes via getter
// methods so a test can seed subscribers or assert on evidence rows.
func NewFakeRunner() (*Runner, *FakeStores) {
	stores := &FakeStores{
		redis:    newTestFakeRedis(),
		procd:    newTestFakeEventsProcessedStore(),
		subs:     newTestFakeSubscribersStore(),
		evid:     newTestFakeEvidenceStore(),
		channels: map[commons.ChannelID]commons.Channel{},
	}
	// Register a no-op channel for "null" so the dispatcher has at least
	// one entry. Tests can override by adding to stores.channels before
	// the first Run.
	stores.channels[commons.ChannelNull] = newTestFakeChannel("null")

	r := &Runner{
		parser:  &EventParser{},
		idem:    &IdempotencyChecker{Redis: stores.redis, PG: stores.procd, TTL: 24 * time.Hour},
		tenant:  &TenantResolver{},
		policy:  &PolicyGate{Registry: nil},
		subs:    &SubscriberResolver{Subscribers: stores.subs},
		chans:   &ChannelDispatcher{Channels: stores.channels},
		outcome: &OutcomeRecorder{Evidence: stores.evid, EventsProcessed: stores.procd},
	}
	return r, stores
}

// FakeStores is the per-test handle returned by NewFakeRunner. Tests can
// seed the SubscribersStore before Run and inspect the EvidenceStore
// after.
type FakeStores struct {
	redis    *testFakeRedis
	procd    *testFakeEventsProcessedStore
	subs     *testFakeSubscribersStore
	evid     *testFakeEvidenceStore
	channels map[commons.ChannelID]commons.Channel
}

// AddSubscriber seeds the in-memory subscribersStore with one subscriber
// (one alias) under the given tenant. Helper to keep test bodies small.
func (s *FakeStores) AddSubscriber(tenantID uuid.UUID, handle, displayName, channel, channelUserID string) {
	s.subs.Add(tenantID, subscriberRow{
		ID:          uuid.New(),
		Handle:      handle,
		DisplayName: displayName,
		Aliases: []subscriberAliasRow{
			{Channel: channel, ChannelUserID: channelUserID},
		},
	})
}

// EvidenceCount returns the number of outbound_delivery_evidence rows the
// fake recorded — proves the OutcomeRecorder ran (§107 evidence).
func (s *FakeStores) EvidenceCount() int {
	return len(s.evid.All())
}

// -------- in-memory fakes (test-only; mirror fakes_test.go) ---------------

type testFakeRedis struct {
	mu      sync.Mutex
	store   map[string]string
	expires map[string]time.Time
	now     func() time.Time
}

func newTestFakeRedis() *testFakeRedis {
	return &testFakeRedis{
		store:   map[string]string{},
		expires: map[string]time.Time{},
		now:     time.Now,
	}
}

func (r *testFakeRedis) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
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

func (r *testFakeRedis) Get(ctx context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if exp, ok := r.expires[key]; ok && r.now().After(exp) {
		delete(r.store, key)
		delete(r.expires, key)
	}
	v, ok := r.store[key]
	if !ok {
		return "", errTestRedisNil
	}
	return v, nil
}

var errTestRedisNil = errors.New("runnertest redis: key not found")

type testFakeEventsProcessedStore struct {
	mu   sync.Mutex
	rows map[string]eventsProcessedRow
}

func newTestFakeEventsProcessedStore() *testFakeEventsProcessedStore {
	return &testFakeEventsProcessedStore{rows: map[string]eventsProcessedRow{}}
}

func (s *testFakeEventsProcessedStore) Lookup(ctx context.Context, tenantID uuid.UUID, idemKey string) (*eventsProcessedRow, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := tenantID.String() + "/" + idemKey
	r, ok := s.rows[key]
	if !ok {
		return nil, false
	}
	return &r, true
}

func (s *testFakeEventsProcessedStore) Insert(ctx context.Context, row eventsProcessedRow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := row.TenantID.String() + "/" + row.IdemKey
	if _, exists := s.rows[key]; exists {
		// Post-HRD-132 the Stage-2 Claim already wrote this row; the
		// Stage-7 archive Insert is an idempotent ON CONFLICT DO NOTHING
		// no-op (matches production pgEventsProcessedAdapter.Insert).
		return nil
	}
	s.rows[key] = row
	return nil
}

// Claim models the HRD-132 authoritative dispatch gate: atomic
// `INSERT … ON CONFLICT DO NOTHING` returning whether THIS caller inserted
// the row (claimed=true → fresh) or it already existed (claimed=false →
// duplicate). The mutex serialises concurrent claims as the PG PRIMARY KEY
// does in production, so exactly one concurrent caller per key wins.
func (s *testFakeEventsProcessedStore) Claim(ctx context.Context, row eventsProcessedRow) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := row.TenantID.String() + "/" + row.IdemKey
	if _, exists := s.rows[key]; exists {
		return false, nil
	}
	s.rows[key] = row
	return true, nil
}

type testFakeSubscribersStore struct {
	mu   sync.Mutex
	subs map[uuid.UUID][]subscriberRow
}

func newTestFakeSubscribersStore() *testFakeSubscribersStore {
	return &testFakeSubscribersStore{subs: map[uuid.UUID][]subscriberRow{}}
}

func (s *testFakeSubscribersStore) Add(tenantID uuid.UUID, row subscriberRow) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row.TenantID = tenantID
	s.subs[tenantID] = append(s.subs[tenantID], row)
}

func (s *testFakeSubscribersStore) ListByTenant(ctx context.Context) ([]subscriberRow, error) {
	tid := TenantFromCtx(ctx)
	if tid == uuid.Nil {
		return nil, errors.New("runnertest subscribers: no tenant in ctx")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]subscriberRow, len(s.subs[tid]))
	copy(out, s.subs[tid])
	return out, nil
}

type testFakeEvidenceStore struct {
	mu   sync.Mutex
	rows []evidenceRow
}

func newTestFakeEvidenceStore() *testFakeEvidenceStore {
	return &testFakeEvidenceStore{}
}

func (s *testFakeEvidenceStore) Insert(ctx context.Context, r evidenceRow) (uuid.UUID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r.ID = uuid.New()
	s.rows = append(s.rows, r)
	return r.ID, nil
}

func (s *testFakeEvidenceStore) All() []evidenceRow {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]evidenceRow, len(s.rows))
	copy(out, s.rows)
	return out
}

// testFakeChannel is a minimal commons.Channel implementation that
// records every Send. Used as the default "null" channel in
// NewFakeRunner so the dispatcher has at least one registered adapter.
type testFakeChannel struct {
	mu    sync.Mutex
	name  string
	sends []commons.OutboundMessage
}

func newTestFakeChannel(name string) *testFakeChannel {
	return &testFakeChannel{name: name}
}

func (c *testFakeChannel) Name() string { return c.name }

func (c *testFakeChannel) Capabilities() commons.Capabilities {
	return commons.Capabilities{Text: true, DeliveryCeiling: commons.DeliveryRouted}
}

func (c *testFakeChannel) Send(ctx context.Context, msg commons.OutboundMessage) (commons.Receipt, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sends = append(c.sends, msg)
	return commons.Receipt{
		Evidence:     commons.DeliveryRouted,
		ChannelMsgID: c.name + "-msgid-" + msg.EventID,
		SentAt:       time.Now(),
	}, nil
}

func (c *testFakeChannel) Subscribe(ctx context.Context, h commons.InboundHandler) error { return nil }
func (c *testFakeChannel) HealthCheck(ctx context.Context) error                         { return nil }
