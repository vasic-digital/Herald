package runner

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/vasic-digital/herald/commons"
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
		// Post-HRD-132 the Stage-2 claim already wrote this row, so the
		// Stage-7 archive Insert is an idempotent ON CONFLICT DO NOTHING
		// no-op — model that (NOT a PK error, matching the production
		// pgEventsProcessedAdapter.Insert which uses ON CONFLICT DO NOTHING).
		return nil
	}
	s.rows[key] = row
	return nil
}

// Claim models the HRD-132 authoritative dispatch gate: an atomic
// `INSERT … ON CONFLICT DO NOTHING` reporting whether THIS caller inserted
// the row (claimed=true → fresh) or it already existed (claimed=false →
// duplicate). Concurrent-safe under the same mutex the production PK
// serialises on.
func (s *fakeEventsProcessedStore) Claim(ctx context.Context, row eventsProcessedRow) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := row.TenantID.String() + "/" + row.IdemKey
	if _, exists := s.rows[key]; exists {
		return false, nil // ON CONFLICT DO NOTHING → 0 rows affected
	}
	s.rows[key] = row
	return true, nil
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
		Event:    cloudEventStub(eventID, eventType),
	}
}

// cloudEventStub returns a minimal commons.CloudEventEnvelope suitable
// for stage tests that just need the ID + Type set.
func cloudEventStub(id, typ string) commons.CloudEventEnvelope {
	return commons.CloudEventEnvelope{
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

func mustParse(s string) uuid.UUID {
	id, err := uuid.Parse(s)
	if err != nil {
		panic(err)
	}
	return id
}

// trimToken strips noise (used to compare hex digests across formats).
func trimToken(s string) string { return strings.TrimSpace(s) }

// fakeSubscribersStore is a minimal in-memory stand-in for the
// subscribers + subscriber_aliases tables. Tenant-isolated: queries
// without a tenant in ctx are rejected (mirrors the PG RLS posture
// where unset app.tenant_id GUC yields zero rows).
//
// subscriberRow + subscriberAliasRow live in production code
// (subscriber.go) so the real PG adapter (T9) can return them; see
// the eventsProcessedRow precedent in idempotency.go.
type fakeSubscribersStore struct {
	mu   sync.Mutex
	subs map[uuid.UUID][]subscriberRow // keyed by tenantID
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

// fakeChannel is a minimal in-memory stand-in for commons.Channel.
// Records every Send and lets the test inspect what would have been
// dispatched without hitting a real network. Concurrent-safe so the
// dispatcher fan-out test can exercise it without races.
type fakeChannel struct {
	mu       sync.Mutex
	name     string
	sends    []fakeSendRecord
	failNext bool
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
func (c *fakeChannel) HealthCheck(ctx context.Context) error                         { return nil }

// fakeEvidenceStore is a minimal in-memory stand-in for the PG
// `outbound_delivery_evidence` table. Records every Insert with a fresh
// UUID and lets the test inspect the rows. Concurrent-safe.
//
// evidenceRow lives in production code (outcome.go) so the real PG
// adapter (T9) can accept it; see the eventsProcessedRow / subscriberRow
// precedent.
type fakeEvidenceStore struct {
	mu   sync.Mutex
	rows []evidenceRow
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
