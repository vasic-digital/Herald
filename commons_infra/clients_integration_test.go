//go:build integration

// Live-integration evidence for HRD-010 Task 2: QuickstartBoot.Up() opens
// a real pgx pool against the booted Postgres container.
//
// Run with:
//
//	go test -tags=integration -timeout 5m -count=1 -run TestUp_PopulatesPool ./commons_infra/...
//
// Requires a running Podman or Docker runtime on the host. Without one the
// default `go test ./commons_infra/...` (no -tags) cleanly skips this file.
//
// Anti-bluff per §107 + §11.4.5: asserts pool non-nil and error nil after
// Up(). A version of this test that called Pool() without checking would
// be a bluff — the very contract Task 1's ErrNotBooted skeleton exists to
// guard against.

package infra

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestUp_PopulatesPool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	// Test-scope env: the quickstart compose declares ${HERALD_DB_PASSWORD}
	// (and friends) as required. Use throwaway values; t.Setenv auto-restores
	// on test end so nothing leaks beyond this test.
	t.Setenv("HERALD_DB_PASSWORD", "test-postgres-password-DO-NOT-USE-IN-PROD")
	t.Setenv("HERALD_REDIS_PASSWORD", "test-redis-password-DO-NOT-USE-IN-PROD")
	t.Setenv("HERALD_PROJECT_NAME", "Herald-Integration-Test")
	t.Setenv("HERALD_TENANT_ID", "00000000-0000-0000-0000-000000000099")

	if os.Getenv("DOCKER_HOST") == "" {
		if sock := os.Getenv("PODMAN_MAC_SOCK"); sock != "" {
			t.Setenv("DOCKER_HOST", "unix://"+sock)
		}
	}

	boot, err := NewQuickstartBoot(Config{
		Services: []string{"postgres"}, // limit blast radius: only postgres
	})
	if err != nil {
		t.Skipf("compose runtime not available; skipping (closed-set reason: hardware_not_present): %v", err)
	}

	if err := boot.Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
	}
	t.Cleanup(func() {
		downCtx, downCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer downCancel()
		if err := boot.Down(downCtx); err != nil {
			t.Logf("boot.Down (cleanup): %v", err)
		}
	})

	pool, err := boot.Pool()
	if err != nil {
		t.Fatalf("Pool() after Up(): %v", err)
	}
	if pool == nil {
		t.Fatal("Pool() returned nil without error — §107 PASS-bluff guard")
	}
}
