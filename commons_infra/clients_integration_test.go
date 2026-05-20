//go:build integration

// Live-integration evidence for HRD-010 Tasks 2 + 4:
//   - TestUp_PopulatesPool: QuickstartBoot.Up() opens a real pgx pool
//     against the booted Postgres container (Task 2 — E14 anchor).
//   - TestUp_PopulatesRedis_TTLRoundTrip: Up() opens a real Redis
//     client, Set/Get/TTL-expire round-trip proves end-to-end live
//     connectivity (Task 4 — E16 anchor per §107 / §11.4.68 positive
//     sink-side evidence).
//
// Run with:
//
//	go test -tags=integration -timeout 5m -count=1 -run 'TestUp_' ./commons_infra/...
//
// Requires a running Podman or Docker runtime on the host. Without one the
// default `go test ./commons_infra/...` (no -tags) cleanly skips this file.
//
// Anti-bluff per §107 + §11.4.5 + §11.4.68: each test asserts on
// positive sink-side evidence — the pgx pool returns a non-nil handle
// after Up; the Redis client survives a Set→Get→wait→Exists=false
// cycle that ONLY a live Redis enforcing TTLs can satisfy. A version
// of either test that called the getter without checking would be a
// bluff per §107.

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

// TestUp_PopulatesRedis_TTLRoundTrip is HRD-010 Task 4's E16 live-evidence
// anchor: after QuickstartBoot.Up() the Redis getter MUST return a real
// client connected to the booted Redis container, AND that client MUST
// enforce TTL on Set — the wait-past-TTL Exists=false branch is the
// load-bearing §11.4.68 positive-sink-side assertion that distinguishes
// a real Redis from any in-memory map fake.
func TestUp_PopulatesRedis_TTLRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	// Required env vars per the quickstart compose's interpolation rules.
	// Same throwaway values as TestUp_PopulatesPool — t.Setenv auto-restores.
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
		Services: []string{"postgres", "redis"},
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

	rc, err := boot.Redis()
	if err != nil {
		t.Fatalf("Redis() after Up: %v", err)
	}
	if rc == nil {
		t.Fatal("Redis() returned nil without error — §107 PASS-bluff guard")
	}

	// Set with a short TTL — long enough to read back, short enough that
	// the test wait time is bounded.
	key := "herald:test:ttl:" + time.Now().UTC().Format("20060102150405.000000")
	if err := rc.Set(ctx, key, []byte("hello"), 2*time.Second); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Read back immediately — must be present.
	got, err := rc.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get before TTL: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("Get value mismatch: got %q want %q", got, "hello")
	}

	// Wait past TTL — Redis server-side must have expired the key.
	// 2500ms > 2000ms TTL gives a 500ms safety margin for the expirer
	// cycle without making the test slow.
	time.Sleep(2500 * time.Millisecond)
	exists, err := rc.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists after TTL: %v", err)
	}
	if exists {
		t.Fatalf("key %q still present after TTL — Redis didn't enforce TTL (§107 / §11.4.68 bluff trap)", key)
	}
}
