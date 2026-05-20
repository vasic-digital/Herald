//go:build integration

// Live boot smoke test for the on-demand-infra invariant (§11.4.76).
//
// Run with:
//
//	go test -tags=integration -timeout 5m -count=1 ./commons_infra/...
//
// Requires a running Podman or Docker runtime on the host (the §11.4.76
// expectation is that the test entry point itself boots infra — but the
// container runtime daemon MUST be reachable, i.e. operator has run
// `podman machine start` or Docker Desktop is up).
//
// Anti-bluff per §11.4.76 + §11.4.69: this test DOES boot a real
// container, observes a real "running" status, and tears down. A
// version of this test that skips the actual Up call would be a bluff.

package infra

import (
	"context"
	"net"
	"os"
	"testing"
	"time"
)

func TestLiveBoot_PostgresOnly(t *testing.T) {
	t.Helper()
	// Hard timeout so a stuck boot doesn't burn the CI clock.
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	// Override DOCKER_HOST if podman's mac socket variant is present —
	// matches the canonical local-dev path the operator's `podman machine
	// start` output emits.
	if os.Getenv("DOCKER_HOST") == "" {
		if sock := os.Getenv("PODMAN_MAC_SOCK"); sock != "" {
			t.Setenv("DOCKER_HOST", "unix://"+sock)
		}
	}

	// The quickstart compose declares ${HERALD_DB_PASSWORD}, ${HERALD_REDIS_
	// PASSWORD}, etc. as required env. Tests cannot rely on the operator's
	// .env file (it's gitignored), so we set throwaway test values here.
	// Test scope only — never leaked beyond t.Setenv (auto-restored on t end).
	t.Setenv("HERALD_DB_PASSWORD", "test-postgres-password-DO-NOT-USE-IN-PROD")
	t.Setenv("HERALD_REDIS_PASSWORD", "test-redis-password-DO-NOT-USE-IN-PROD")
	t.Setenv("HERALD_PROJECT_NAME", "Herald-Integration-Test")
	t.Setenv("HERALD_TENANT_ID", "00000000-0000-0000-0000-000000000099")

	boot, err := NewQuickstartBoot(Config{
		Services: []string{"postgres"}, // limit blast radius: only postgres
	})
	if err != nil {
		t.Skipf("compose runtime not available; skipping (closed-set reason: hardware_not_present): %v", err)
	}

	if err := boot.Up(ctx); err != nil {
		t.Fatalf("boot.Up: %v", err)
	}
	// Anti-bluff §11.4.76 healthcheck: TCP-probe the host-mapped Postgres
	// port from spec §9.4 (24100). Status()-parsing varies across compose
	// runtimes (HRD-081); a raw TCP open is the ultimate proof "the
	// service is reachable by something approximating a real client."
	const postgresAddr = "127.0.0.1:24100"
	healthDeadline := time.Now().Add(90 * time.Second)
	var lastErr error
	postgresReachable := false
	for time.Now().Before(healthDeadline) {
		c, err := net.DialTimeout("tcp", postgresAddr, 2*time.Second)
		if err == nil {
			_ = c.Close()
			postgresReachable = true
			break
		}
		lastErr = err
		time.Sleep(2 * time.Second)
	}
	if !postgresReachable {
		t.Fatalf("Postgres at %s did not become TCP-reachable within 90s (would be §11.4.76 bluff if test PASSed here); last dial error: %v", postgresAddr, lastErr)
	}
	defer func() {
		// Always tear down — leaving a Postgres running between test runs
		// causes the next iteration's Up to find an unexpected container.
		// Down context is separate so a cancelled ctx doesn't break cleanup.
		downCtx, downCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer downCancel()
		if err := boot.Down(downCtx); err != nil {
			t.Logf("boot.Down (cleanup): %v", err)
		}
	}()

	// Status verification (informational only — podman-compose ps output
	// parsing has known compatibility gaps with the orchestrator, tracked
	// in HRD-081). The load-bearing anti-bluff proof is the TCP probe
	// above; Status() output is best-effort secondary evidence.
	statuses, err := boot.Status(ctx)
	if err != nil {
		t.Logf("boot.Status (informational): %v", err)
	} else {
		t.Logf("boot.Status returned %d service(s): %+v", len(statuses), statuses)
	}
}
