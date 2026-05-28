//go:build integration

// Live-Postgres integration evidence for v1.0.0 Batch D — the 17
// pgxTaskRepository methods landed under HRD-085..089
// (commons_infra/task_repository.go).
//
// Run with:
//
//	HERALD_DB_PASSWORD=... HERALD_REDIS_PASSWORD=... \
//	  go test -tags=integration -timeout 5m -count=1 \
//	  -run 'TestRepo' ./commons_infra/...
//
// or simply (the test sets throwaway env itself, mirroring
// clients_integration_test.go):
//
//	go test -tags=integration -timeout 5m -count=1 -run 'TestRepo' ./commons_infra/...
//
// Requires a running Podman/Docker runtime so QuickstartBoot can bring up
// the Postgres container on host port 24100 (spec §9.4). Without a runtime
// the test cleanly Skips (closed-set reason hardware_not_present). The new
// migration 000013_task_resource_snapshots is applied automatically by
// boot.Up() -> storage.RunMigrations.
//
// ANTI-BLUFF CONTRACT (§107 / §11.4.5 / §11.4.68 positive sink-side
// evidence): every method is exercised against a REAL row, then the
// resulting DB state is read back via an INDEPENDENT raw SELECT (or the
// matching read method) and asserted EXACT — not ">= 1", not "non-nil".
// A write that silently no-ops, or a read that returns the wrong row,
// FAILS here. This is the load-bearing proof that the SQL the unit tests
// only string-matched actually round-trips through Postgres.

package infra

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	db "digital.vasic.database/pkg/database"
	"digital.vasic.models"
	"github.com/google/uuid"
)

// bootRepo brings up a real Postgres (migrations applied) and returns a
// repository bound to its pool plus the raw pool for independent
// read-back SELECTs. It TRUNCATEs the queue tables for test isolation
// (the persistent volume retains rows across runs — same guard as
// TestUp_PopulatesQueue_EnqueueDequeueRoundTrip).
func bootRepo(t *testing.T) (*pgxTaskRepository, db.Database, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	t.Setenv("HERALD_DB_PASSWORD", "test-postgres-password-DO-NOT-USE-IN-PROD")
	t.Setenv("HERALD_REDIS_PASSWORD", "test-redis-password-DO-NOT-USE-IN-PROD")
	t.Setenv("HERALD_PROJECT_NAME", "Herald-Integration-Test")
	t.Setenv("HERALD_TENANT_ID", "00000000-0000-0000-0000-000000000099")

	if os.Getenv("DOCKER_HOST") == "" {
		if sock := os.Getenv("PODMAN_MAC_SOCK"); sock != "" {
			t.Setenv("DOCKER_HOST", "unix://"+sock)
		}
	}

	boot, err := NewQuickstartBoot(Config{Services: []string{"postgres"}})
	if err != nil {
		t.Skipf("compose runtime not available; skipping (closed-set reason: hardware_not_present): %v", err)
	}
	if err := boot.Up(ctx); err != nil {
		t.Fatalf("boot.Up: %v", err)
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
		t.Fatalf("Pool(): %v", err)
	}
	if pool == nil {
		t.Fatal("Pool() returned nil — §107 PASS-bluff guard")
	}
	// Isolation: clear queue + snapshot + event rows from prior runs.
	// CASCADE walks the FK from background_tasks to background_task_events
	// AND task_resource_snapshots (both reference it ON DELETE CASCADE).
	if _, err := pool.Exec(ctx, "TRUNCATE background_tasks RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("test-isolation TRUNCATE: %v", err)
	}
	return newPgxTaskRepository(pool), pool, ctx
}

// seedTask Creates a fresh pending task with a unique id and returns its id.
func seedTask(t *testing.T, repo *pgxTaskRepository, ctx context.Context) string {
	t.Helper()
	id := uuid.NewString()
	payload, _ := json.Marshal(map[string]string{"seed": id})
	task := models.NewBackgroundTask("herald.test.batchd", "seed-"+id[:8], payload)
	task.ID = id
	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("seedTask Create: %v", err)
	}
	return id
}

// --- HRD-085 ---

func TestRepoGetByID_RoundTrip(t *testing.T) {
	repo, _, ctx := bootRepo(t)
	id := seedTask(t, repo, ctx)

	got, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("GetByID returned nil for an existing task — §107 sink-side bluff")
	}
	if got.ID != id {
		t.Fatalf("GetByID wrong row: got %s want %s", got.ID, id)
	}
	if got.Status != models.TaskStatusPending {
		t.Fatalf("GetByID status: got %s want pending", got.Status)
	}

	// Not-found returns (nil, nil).
	missing, err := repo.GetByID(ctx, "definitely-not-present")
	if err != nil {
		t.Fatalf("GetByID missing should be (nil,nil), got err=%v", err)
	}
	if missing != nil {
		t.Fatalf("GetByID missing should be nil, got %+v", missing)
	}
}

func TestRepoUpdate_RoundTrip(t *testing.T) {
	repo, pool, ctx := bootRepo(t)
	id := seedTask(t, repo, ctx)

	task, err := repo.GetByID(ctx, id)
	if err != nil || task == nil {
		t.Fatalf("GetByID pre-Update: %v / %v", err, task)
	}
	task.Status = models.TaskStatusRunning
	task.RetryCount = 2
	msg := "halfway"
	task.ProgressMessage = &msg
	task.Progress = 50
	if err := repo.Update(ctx, task); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Independent read-back via raw SELECT (sink-side proof).
	var status string
	var retry int
	var progress float64
	if err := pool.QueryRow(ctx,
		"SELECT status, retry_count, progress FROM background_tasks WHERE id = $1", id,
	).Scan(&status, &retry, &progress); err != nil {
		t.Fatalf("read-back: %v", err)
	}
	if status != "running" || retry != 2 || progress != 50 {
		t.Fatalf("Update did not persist: status=%s retry=%d progress=%v (want running/2/50)", status, retry, progress)
	}

	// Update on a non-existent id must error (no silent success).
	ghost := models.NewBackgroundTask("t", "n", nil)
	ghost.ID = "ghost-update"
	if err := repo.Update(ctx, ghost); err == nil {
		t.Fatal("Update on missing id must error")
	}
}

func TestRepoDelete_SoftDelete_RoundTrip(t *testing.T) {
	repo, pool, ctx := bootRepo(t)
	id := seedTask(t, repo, ctx)

	if err := repo.Delete(ctx, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// Row still physically present (soft delete) with deleted_at set.
	var deletedAt *time.Time
	if err := pool.QueryRow(ctx,
		"SELECT deleted_at FROM background_tasks WHERE id = $1", id,
	).Scan(&deletedAt); err != nil {
		t.Fatalf("read-back after Delete: %v (soft delete should keep the row)", err)
	}
	if deletedAt == nil {
		t.Fatal("Delete must stamp deleted_at (soft delete)")
	}
	// GetByID now treats it as not-found.
	got, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID after Delete: %v", err)
	}
	if got != nil {
		t.Fatal("GetByID must not return a soft-deleted task")
	}
	// Second Delete is a no-op-on-deleted -> error (loud).
	if err := repo.Delete(ctx, id); err == nil {
		t.Fatal("Delete of already-deleted task must error")
	}
}

// --- HRD-086 ---

func TestRepoUpdateStatus_RoundTrip(t *testing.T) {
	repo, pool, ctx := bootRepo(t)
	id := seedTask(t, repo, ctx)

	if err := repo.UpdateStatus(ctx, id, models.TaskStatusCompleted); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	var status string
	var completedAt *time.Time
	if err := pool.QueryRow(ctx,
		"SELECT status, completed_at FROM background_tasks WHERE id = $1", id,
	).Scan(&status, &completedAt); err != nil {
		t.Fatalf("read-back: %v", err)
	}
	if status != "completed" {
		t.Fatalf("UpdateStatus: got %s want completed", status)
	}
	if completedAt == nil {
		t.Fatal("UpdateStatus to terminal must stamp completed_at")
	}
	if err := repo.UpdateStatus(ctx, "ghost", models.TaskStatusFailed); err == nil {
		t.Fatal("UpdateStatus on missing id must error")
	}
	if err := repo.UpdateStatus(ctx, id, models.TaskStatus("garbage")); err == nil {
		t.Fatal("UpdateStatus must reject unknown status")
	}
}

func TestRepoUpdateProgress_RoundTrip(t *testing.T) {
	repo, pool, ctx := bootRepo(t)
	id := seedTask(t, repo, ctx)

	if err := repo.UpdateProgress(ctx, id, 73.5, "almost"); err != nil {
		t.Fatalf("UpdateProgress: %v", err)
	}
	var progress float64
	var msg *string
	if err := pool.QueryRow(ctx,
		"SELECT progress, progress_message FROM background_tasks WHERE id = $1", id,
	).Scan(&progress, &msg); err != nil {
		t.Fatalf("read-back: %v", err)
	}
	if progress != 73.5 || msg == nil || *msg != "almost" {
		t.Fatalf("UpdateProgress: got %v / %v want 73.5 / almost", progress, msg)
	}
	// Clamp proof: 150 -> 100.
	if err := repo.UpdateProgress(ctx, id, 150, "over"); err != nil {
		t.Fatalf("UpdateProgress clamp: %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT progress FROM background_tasks WHERE id = $1", id).Scan(&progress); err != nil {
		t.Fatalf("read-back clamp: %v", err)
	}
	if progress != 100 {
		t.Fatalf("UpdateProgress must clamp 150 -> 100, got %v", progress)
	}
}

func TestRepoUpdateHeartbeat_RoundTrip(t *testing.T) {
	repo, pool, ctx := bootRepo(t)
	id := seedTask(t, repo, ctx)

	before := time.Now().UTC()
	if err := repo.UpdateHeartbeat(ctx, id); err != nil {
		t.Fatalf("UpdateHeartbeat: %v", err)
	}
	var hb *time.Time
	if err := pool.QueryRow(ctx,
		"SELECT last_heartbeat FROM background_tasks WHERE id = $1", id,
	).Scan(&hb); err != nil {
		t.Fatalf("read-back: %v", err)
	}
	if hb == nil {
		t.Fatal("UpdateHeartbeat must set last_heartbeat")
	}
	if hb.Before(before.Add(-time.Second)) {
		t.Fatalf("UpdateHeartbeat stamped a stale time: %s (before %s)", hb, before)
	}
}

func TestRepoSaveCheckpoint_RoundTrip(t *testing.T) {
	repo, pool, ctx := bootRepo(t)
	id := seedTask(t, repo, ctx)

	cp := []byte(`{"step":7,"offset":1024}`)
	if err := repo.SaveCheckpoint(ctx, id, cp); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}
	var stored []byte
	if err := pool.QueryRow(ctx,
		"SELECT checkpoint FROM background_tasks WHERE id = $1", id,
	).Scan(&stored); err != nil {
		t.Fatalf("read-back: %v", err)
	}
	// JSONB normalises whitespace but preserves semantics — compare parsed.
	var want, got map[string]any
	if err := json.Unmarshal(cp, &want); err != nil {
		t.Fatalf("unmarshal want: %v", err)
	}
	if err := json.Unmarshal(stored, &got); err != nil {
		t.Fatalf("stored checkpoint is not valid JSON: %v (%s)", err, stored)
	}
	if got["step"] != want["step"] || got["offset"] != want["offset"] {
		t.Fatalf("SaveCheckpoint round-trip mismatch: got %v want %v", got, want)
	}
}

// --- HRD-087 ---

func TestRepoGetByStatus_RoundTrip(t *testing.T) {
	repo, _, ctx := bootRepo(t)
	// Seed 3 tasks, move 2 to running.
	id1 := seedTask(t, repo, ctx)
	id2 := seedTask(t, repo, ctx)
	_ = seedTask(t, repo, ctx) // stays pending
	if err := repo.UpdateStatus(ctx, id1, models.TaskStatusRunning); err != nil {
		t.Fatalf("UpdateStatus id1: %v", err)
	}
	if err := repo.UpdateStatus(ctx, id2, models.TaskStatusRunning); err != nil {
		t.Fatalf("UpdateStatus id2: %v", err)
	}

	running, err := repo.GetByStatus(ctx, models.TaskStatusRunning, 10, 0)
	if err != nil {
		t.Fatalf("GetByStatus: %v", err)
	}
	if len(running) != 2 {
		t.Fatalf("GetByStatus(running): got %d want EXACTLY 2", len(running))
	}
	for _, task := range running {
		if task.Status != models.TaskStatusRunning {
			t.Fatalf("GetByStatus returned a non-running task: %s", task.Status)
		}
	}
	// Pagination: limit 1 returns 1.
	page, err := repo.GetByStatus(ctx, models.TaskStatusRunning, 1, 0)
	if err != nil {
		t.Fatalf("GetByStatus page: %v", err)
	}
	if len(page) != 1 {
		t.Fatalf("GetByStatus(limit=1): got %d want 1", len(page))
	}
}

func TestRepoGetPendingTasks_RoundTrip(t *testing.T) {
	repo, _, ctx := bootRepo(t)
	seedTask(t, repo, ctx)
	seedTask(t, repo, ctx)

	pending, err := repo.GetPendingTasks(ctx, 10)
	if err != nil {
		t.Fatalf("GetPendingTasks: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("GetPendingTasks: got %d want EXACTLY 2", len(pending))
	}
	for _, task := range pending {
		if task.Status != models.TaskStatusPending {
			t.Fatalf("GetPendingTasks returned non-pending: %s", task.Status)
		}
	}
}

func TestRepoCountByStatus_RoundTrip(t *testing.T) {
	repo, _, ctx := bootRepo(t)
	id1 := seedTask(t, repo, ctx)
	seedTask(t, repo, ctx) // pending
	if err := repo.UpdateStatus(ctx, id1, models.TaskStatusFailed); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	counts, err := repo.CountByStatus(ctx)
	if err != nil {
		t.Fatalf("CountByStatus: %v", err)
	}
	if counts[models.TaskStatusPending] != 1 {
		t.Fatalf("CountByStatus pending: got %d want 1", counts[models.TaskStatusPending])
	}
	if counts[models.TaskStatusFailed] != 1 {
		t.Fatalf("CountByStatus failed: got %d want 1", counts[models.TaskStatusFailed])
	}
	// A status with zero rows is absent (treated as 0).
	if _, present := counts[models.TaskStatusStuck]; present {
		t.Fatalf("CountByStatus should omit zero-count statuses; stuck present=%v", counts[models.TaskStatusStuck])
	}
}

func TestRepoGetTaskHistory_RoundTrip(t *testing.T) {
	repo, _, ctx := bootRepo(t)
	id := seedTask(t, repo, ctx)

	worker := "worker-hist"
	if err := repo.LogEvent(ctx, id, "task.created", map[string]interface{}{"a": 1}, nil); err != nil {
		t.Fatalf("LogEvent created: %v", err)
	}
	if err := repo.LogEvent(ctx, id, "task.started", map[string]interface{}{"b": 2}, &worker); err != nil {
		t.Fatalf("LogEvent started: %v", err)
	}

	hist, err := repo.GetTaskHistory(ctx, id, 10)
	if err != nil {
		t.Fatalf("GetTaskHistory: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("GetTaskHistory: got %d events want EXACTLY 2", len(hist))
	}
	// Newest-first: started (with worker) before created.
	if hist[0].EventType != "task.started" {
		t.Fatalf("GetTaskHistory must be newest-first; got first=%s", hist[0].EventType)
	}
	if hist[0].WorkerID == nil || *hist[0].WorkerID != worker {
		t.Fatalf("GetTaskHistory worker_id: got %v want %s", hist[0].WorkerID, worker)
	}
	if hist[0].TaskID != id {
		t.Fatalf("GetTaskHistory task_id: got %s want %s", hist[0].TaskID, id)
	}
}

// --- HRD-088 ---

func TestRepoGetStaleTasks_RoundTrip(t *testing.T) {
	repo, pool, ctx := bootRepo(t)
	stale := seedTask(t, repo, ctx)
	fresh := seedTask(t, repo, ctx)

	// Both must be active to be stale-candidates.
	if err := repo.UpdateStatus(ctx, stale, models.TaskStatusRunning); err != nil {
		t.Fatalf("UpdateStatus stale: %v", err)
	}
	if err := repo.UpdateStatus(ctx, fresh, models.TaskStatusRunning); err != nil {
		t.Fatalf("UpdateStatus fresh: %v", err)
	}
	// fresh just heartbeated; stale's heartbeat is 1h ago (raw UPDATE so we
	// control the exact instant).
	if err := repo.UpdateHeartbeat(ctx, fresh); err != nil {
		t.Fatalf("UpdateHeartbeat fresh: %v", err)
	}
	if _, err := pool.Exec(ctx,
		"UPDATE background_tasks SET last_heartbeat = now() - interval '1 hour' WHERE id = $1", stale,
	); err != nil {
		t.Fatalf("backdate stale heartbeat: %v", err)
	}

	got, err := repo.GetStaleTasks(ctx, 5*time.Minute)
	if err != nil {
		t.Fatalf("GetStaleTasks: %v", err)
	}
	// Exactly the stale one (fresh heartbeated within 5min).
	foundStale, foundFresh := false, false
	for _, task := range got {
		if task.ID == stale {
			foundStale = true
		}
		if task.ID == fresh {
			foundFresh = true
		}
	}
	if !foundStale {
		t.Fatal("GetStaleTasks must include the 1h-stale running task")
	}
	if foundFresh {
		t.Fatal("GetStaleTasks must NOT include the freshly-heartbeated task")
	}
}

func TestRepoGetByWorkerID_RoundTrip(t *testing.T) {
	repo, pool, ctx := bootRepo(t)
	mine1 := seedTask(t, repo, ctx)
	mine2 := seedTask(t, repo, ctx)
	other := seedTask(t, repo, ctx)

	if _, err := pool.Exec(ctx, "UPDATE background_tasks SET worker_id = $1 WHERE id IN ($2, $3)", "worker-A", mine1, mine2); err != nil {
		t.Fatalf("assign worker-A: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE background_tasks SET worker_id = $1 WHERE id = $2", "worker-B", other); err != nil {
		t.Fatalf("assign worker-B: %v", err)
	}

	got, err := repo.GetByWorkerID(ctx, "worker-A")
	if err != nil {
		t.Fatalf("GetByWorkerID: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("GetByWorkerID(worker-A): got %d want EXACTLY 2", len(got))
	}
	for _, task := range got {
		if task.WorkerID == nil || *task.WorkerID != "worker-A" {
			t.Fatalf("GetByWorkerID leaked another worker's task: %v", task.WorkerID)
		}
	}
}

// --- HRD-089 ---

func TestRepoResourceSnapshots_RoundTrip(t *testing.T) {
	repo, _, ctx := bootRepo(t)
	id := seedTask(t, repo, ctx)

	// Save two snapshots with distinct sampled_at so ordering is provable.
	s1 := &models.ResourceSnapshot{
		TaskID:         id,
		CPUPercent:     12.5,
		MemoryRSSBytes: 1024 * 1024,
		IOReadBytes:    4096,
		ProcessState:   "running",
		ThreadCount:    8,
		SampledAt:      time.Now().UTC().Add(-1 * time.Minute),
	}
	s2 := &models.ResourceSnapshot{
		TaskID:         id,
		CPUPercent:     88.0,
		MemoryRSSBytes: 2 * 1024 * 1024,
		ProcessState:   "running",
		ThreadCount:    16,
		SampledAt:      time.Now().UTC(),
	}
	if err := repo.SaveResourceSnapshot(ctx, s1); err != nil {
		t.Fatalf("SaveResourceSnapshot s1: %v", err)
	}
	if err := repo.SaveResourceSnapshot(ctx, s2); err != nil {
		t.Fatalf("SaveResourceSnapshot s2: %v", err)
	}
	if s1.ID == "" || s2.ID == "" {
		t.Fatal("SaveResourceSnapshot must auto-fill IDs")
	}

	snaps, err := repo.GetResourceSnapshots(ctx, id, 10)
	if err != nil {
		t.Fatalf("GetResourceSnapshots: %v", err)
	}
	if len(snaps) != 2 {
		t.Fatalf("GetResourceSnapshots: got %d want EXACTLY 2", len(snaps))
	}
	// Reverse-chronological: s2 (now) first, s1 (1min ago) second.
	if snaps[0].CPUPercent != 88.0 {
		t.Fatalf("GetResourceSnapshots must be newest-first: got first CPU=%v want 88.0", snaps[0].CPUPercent)
	}
	if snaps[1].CPUPercent != 12.5 {
		t.Fatalf("GetResourceSnapshots second sample CPU: got %v want 12.5", snaps[1].CPUPercent)
	}
	// Field fidelity round-trip on the newest.
	if snaps[0].MemoryRSSBytes != 2*1024*1024 || snaps[0].ThreadCount != 16 || snaps[0].ProcessState != "running" {
		t.Fatalf("GetResourceSnapshots field fidelity broke: %+v", snaps[0])
	}
	if snaps[0].TaskID != id {
		t.Fatalf("GetResourceSnapshots task_id: got %s want %s", snaps[0].TaskID, id)
	}

	// Limit honoured.
	one, err := repo.GetResourceSnapshots(ctx, id, 1)
	if err != nil {
		t.Fatalf("GetResourceSnapshots(limit=1): %v", err)
	}
	if len(one) != 1 {
		t.Fatalf("GetResourceSnapshots(limit=1): got %d want 1", len(one))
	}
}
