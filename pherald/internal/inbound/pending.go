// Package inbound — pending.go (WS-4 / HRD-152): the act-with-confirmation
// pending-action store.
//
// investigation.start is ACT-WITH-CONFIRMATION (operator decision
// 2026-05-29): an investigation gathers info + returns a report, and any
// PROPOSED mutating action is NOT executed immediately. Instead the
// dispatcher records the proposal in a pendingStore keyed by a token and
// emits a "Reply CONFIRM <token> to apply: …" prompt. A subsequent
// "CONFIRM <token>" message looks the proposal up and executes it.
//
// §107 anchor: the store is the single source of truth for "what will a
// CONFIRM execute" — a CONFIRM with no matching token returns an error
// (no fabricated success), and the entry is consumed on execution so a
// replayed CONFIRM cannot double-apply.
package inbound

import (
	"errors"
	"sync"
)

// ErrNoPendingAction is returned by pendingStore.take when no entry
// matches the given token (unknown / already-consumed / expired).
var ErrNoPendingAction = errors.New("inbound: no pending action for token")

// pendingStore is a concurrency-safe token→ProposedAction map. pherald
// listen is single-goroutine per channel today, but the mutex future-
// proofs the multi-channel-fanout case (V3 §32.2) and keeps -race clean.
type pendingStore struct {
	mu sync.Mutex
	m  map[string]ProposedAction
}

func newPendingStore() *pendingStore {
	return &pendingStore{m: make(map[string]ProposedAction)}
}

// put records a proposal under token, overwriting any prior entry for
// the same token (deterministic-token tests reuse "TOKEN1").
func (p *pendingStore) put(token string, a ProposedAction) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.m[token] = a
}

// take returns and removes the proposal for token. Consuming on take
// means a replayed CONFIRM cannot double-apply the mutation.
func (p *pendingStore) take(token string) (ProposedAction, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	a, ok := p.m[token]
	if !ok {
		return ProposedAction{}, ErrNoPendingAction
	}
	delete(p.m, token)
	return a, nil
}
