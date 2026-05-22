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
