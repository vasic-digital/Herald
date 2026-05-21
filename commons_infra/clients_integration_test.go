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
	"encoding/json"
	"os"
	"testing"
	"time"

	"digital.vasic.models"
	"github.com/google/uuid"
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

// TestUp_PopulatesQueue_EnqueueDequeueRoundTrip is HRD-010 Task 5's E15
// live-evidence anchor. After QuickstartBoot.Up() the Queue getter MUST
// return a real digital.vasic.background.TaskQueue bound to the booted
// Postgres container's `background_tasks` table (migration 000009),
// AND that queue MUST persist a task such that a subsequent Dequeue
// returns the SAME task by ID — proving the task survived the
// Postgres-side INSERT → UPDATE...RETURNING transaction, not just an
// in-memory map.
//
// §11.4.68 positive sink-side evidence: the strong assertion is on
// `got.ID == enqueueID` AND `got.Status == TaskStatusRunning` (the
// dequeue transitions pending → running atomically). Asserting only
// "got non-nil task" would be a §107 PASS-bluff that an in-memory
// fake could satisfy.
//
// Compose with §11.4.5 captured-evidence: the failure message embeds
// the exact diff if IDs don't match, so a real defect produces an
// actionable failure log, not just "got != want".
func TestUp_PopulatesQueue_EnqueueDequeueRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	t.Setenv("HERALD_DB_PASSWORD", "test-postgres-password-DO-NOT-USE-IN-PROD")
	t.Setenv("HERALD_REDIS_PASSWORD", "test-redis-password-DO-NOT-USE-IN-PROD")
	t.Setenv("HERALD_PROJECT_NAME", "Herald-Integration-Test")
	t.Setenv("HERALD_TENANT_ID", "00000000-0000-0000-0000-000000000099")

	if os.Getenv("DOCKER_HOST") == "" {
		if sock := os.Getenv("PODMAN_MAC_SOCK"); sock != "" {
			t.Setenv("DOCKER_HOST", "unix://"+sock)
		}
	}

	// Queue is Postgres-backed (per HRD-010 Task 5 architecture); request
	// only postgres to keep the boot fast. Redis isn't required for E15.
	boot, err := NewQuickstartBoot(Config{
		Services: []string{"postgres"},
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

	q, err := boot.Queue()
	if err != nil {
		t.Fatalf("Queue() after Up: %v", err)
	}
	if q == nil {
		t.Fatal("Queue() returned nil without error — §107 PASS-bluff guard")
	}

	// §107 test-isolation guard: the persistent Postgres volume retains
	// `background_tasks` rows across test runs. If a prior run left a
	// PENDING task (test crash, host kill, podman runaway), that leftover
	// will be dequeued BEFORE the row we're about to enqueue — silently
	// turning this test into a flaky PASS on fresh CI but FAIL on a
	// developer host with accumulated state. The cure is to TRUNCATE the
	// queue tables before each run so the test starts from a known-empty
	// state. This is a TEST fix, not a production fix: the production
	// Create+Dequeue path is verified by the strong assertions below.
	//
	// Anti-bluff §107 captured 2026-05-21: this guard was added after the
	// E15 round-trip was discovered to PASS-bluff in environments where
	// the volume `herald-quickstart_herald-pg` had accumulated PENDING
	// rows from earlier failed runs.
	pool, err := boot.Pool()
	if err != nil {
		t.Fatalf("Pool() after Up: %v", err)
	}
	// Truncate the queue + its event-log child (RESTART IDENTITY drops
	// any sequence state; CASCADE walks the FK to background_task_events).
	if _, err := pool.Exec(ctx, "TRUNCATE background_tasks RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("test-isolation TRUNCATE: %v (the schema-side §107 guard cannot run — anti-bluff invalid)", err)
	}
	// Confirm zero rows: read-back assertion proves the truncate landed.
	// Pre-assertion §11.4.5 captured-evidence: a non-zero count here means
	// the TRUNCATE silently no-op'd; we'd be running the test with leftover
	// state — abort immediately with a loud signal rather than producing a
	// flaky downstream FAIL.
	var preCount int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM background_tasks").Scan(&preCount); err != nil {
		t.Fatalf("pre-test row-count read: %v", err)
	}
	if preCount != 0 {
		t.Fatalf("background_tasks not empty after TRUNCATE: got %d rows (§107 test-isolation guard failed; cannot trust the round-trip)", preCount)
	}

	// Construct a task using the upstream constructor so all defaults
	// (TaskConfig, TaskStatusPending, retry counts, timestamps) are set
	// honestly. Assign an explicit ID so we can assert round-trip identity.
	taskID := uuid.NewString()
	payload, _ := json.Marshal(map[string]string{"hello": "world"})
	task := models.NewBackgroundTask("herald.test.e15", "round-trip", payload)
	task.ID = taskID
	task.Priority = models.TaskPriorityHigh // ensure ordering puts it ahead of any leftover task

	if err := q.Enqueue(ctx, task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Post-Enqueue read-back: exact-1 row, exact-ID match, status=pending.
	// This is the §11.4.68 positive sink-side evidence step — it proves
	// Create actually persisted the row in the SAME state Enqueue claimed,
	// independent of the Dequeue path. A previous version of this test
	// jumped straight from Enqueue to Dequeue, which could not distinguish
	// "Create silently rolled back" from "Dequeue dequeued a leftover".
	var postEnqCount int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM background_tasks WHERE id = $1 AND status = 'pending'", taskID).Scan(&postEnqCount); err != nil {
		t.Fatalf("post-Enqueue row-count read: %v", err)
	}
	if postEnqCount != 1 {
		t.Fatalf("post-Enqueue row count for id=%s: got %d want 1 (§107 sink-side: Enqueue did not persist the task)", taskID, postEnqCount)
	}

	// Dequeue with no resource constraints — the test task has tiny CPU/mem
	// requirements (1 core / 512 MB per NewBackgroundTask defaults) so any
	// host satisfies the implicit upstream filter.
	workerID := "test-worker-" + taskID
	got, err := q.Dequeue(ctx, workerID, bgResReq())
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if got == nil {
		t.Fatal("Dequeue returned nil task — §107 sink-side bluff guard: the queue claimed PASS without actually returning the enqueued row")
	}
	if got.ID != taskID {
		t.Fatalf("Dequeue returned wrong task ID: want %s got %s (queue dequeued a leftover from a prior run, OR the round-trip lost the task)", taskID, got.ID)
	}
	if got.Status != models.TaskStatusRunning {
		t.Fatalf("Dequeue did not transition status to running: got %s (proves the UPDATE...RETURNING atomic-claim is broken)", got.Status)
	}
	if got.WorkerID == nil || *got.WorkerID != workerID {
		t.Fatalf("Dequeue did not claim for the requesting worker: got %v want %s", got.WorkerID, workerID)
	}

	// Final read-back: row's terminal state matches the dequeue's RETURNING
	// payload. Independent SELECT proves the UPDATE was committed (not just
	// returned through a half-committed transaction). §107 sink-side.
	var finalStatus, finalWorker string
	if err := pool.QueryRow(ctx, "SELECT status, worker_id FROM background_tasks WHERE id = $1", taskID).Scan(&finalStatus, &finalWorker); err != nil {
		t.Fatalf("final read-back: %v", err)
	}
	if finalStatus != "running" {
		t.Fatalf("final status: got %s want running", finalStatus)
	}
	if finalWorker != workerID {
		t.Fatalf("final worker_id: got %s want %s", finalWorker, workerID)
	}
}

// bgResReq is a tiny helper to construct an empty ResourceRequirements
// (zero values mean "no limit" per upstream PostgresTaskQueue.Dequeue
// convention).
func bgResReq() ResourceRequirements {
	return ResourceRequirements{}
}
