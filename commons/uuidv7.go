// Package commons — UUIDv7 generator per spec V3 §4.3 / §9.2 (uuidv7
// primary keys).
//
// UUIDv7 is naturally time-ordered (the first 48 bits are a
// Unix-millisecond timestamp), giving us B-tree-friendly indexes
// without the locality problems of UUIDv4. The implementation here is
// a thin wrapper around google/uuid v1.6+ which ships UUIDv7 native.
package commons

import (
	"github.com/google/uuid"
)

// NewUUIDv7 returns a fresh time-ordered UUID.
//
// Use this everywhere Herald generates primary keys (idempotency
// keys, dead-letter rows, inbound message ids, …) so that index
// inserts append to the rightmost leaf and stay cache-hot.
func NewUUIDv7() (uuid.UUID, error) {
	return uuid.NewV7()
}

// MustUUIDv7 panics on the (extremely rare) generator failure path.
// Use only at process boot or in test fixtures; production code
// should call NewUUIDv7 and propagate the error.
func MustUUIDv7() uuid.UUID {
	u, err := uuid.NewV7()
	if err != nil {
		panic("commons: uuidv7 generation failed: " + err.Error())
	}
	return u
}
