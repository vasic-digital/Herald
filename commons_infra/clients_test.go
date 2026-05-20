// Anti-bluff unit tests for the QuickstartBoot.Pool/Queue/Redis aggregator.
//
// Per §107 + §11.4.5: a Pool/Queue/Redis getter that silently returns nil
// when Up() has not been called is a bluff — callers will dereference the
// nil and crash at runtime, but the test PASSes because no client was
// asked for. These tests pin the contract that pre-Up() access returns
// ErrNotBooted and the caller is expected to abort.
//
// Live-integration evidence (real pgx pool, real River queue, real Redis
// client) lands in later HRD-010 tasks (E14/E15/E16); this file covers
// only the not-booted seam.

package infra

import (
	"errors"
	"testing"
)

func TestClients_PoolBeforeUp_ReturnsErrNotBooted(t *testing.T) {
	boot, err := NewQuickstartBoot(Config{})
	if err != nil {
		// Allowed to fail with "no compose runtime" on machines without
		// docker/podman installed. SKIP per §11.4.69 closed-set reason.
		t.Skipf("compose runtime not available: %v (closed-set reason: hardware_not_present)", err)
	}
	_, err = boot.Pool()
	if !errors.Is(err, ErrNotBooted) {
		t.Fatalf("expected ErrNotBooted, got %v", err)
	}
}

func TestClients_QueueBeforeUp_ReturnsErrNotBooted(t *testing.T) {
	boot, err := NewQuickstartBoot(Config{})
	if err != nil {
		t.Skipf("compose runtime not available: %v (closed-set reason: hardware_not_present)", err)
	}
	_, err = boot.Queue()
	if !errors.Is(err, ErrNotBooted) {
		t.Fatalf("expected ErrNotBooted, got %v", err)
	}
}

func TestClients_RedisBeforeUp_ReturnsErrNotBooted(t *testing.T) {
	boot, err := NewQuickstartBoot(Config{})
	if err != nil {
		t.Skipf("compose runtime not available: %v (closed-set reason: hardware_not_present)", err)
	}
	_, err = boot.Redis()
	if !errors.Is(err, ErrNotBooted) {
		t.Fatalf("expected ErrNotBooted, got %v", err)
	}
}
