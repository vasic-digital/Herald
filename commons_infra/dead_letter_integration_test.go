//go:build integration

// Live-Postgres + live-EventBus integration evidence for HRD-090
// (commons_infra.pgxTaskRepository.MoveToDeadLetter).
//
// Run with:
//
//	go test -tags=integration -timeout 5m -count=1 \
//	  -run 'TestRepoMoveToDeadLetter' ./commons_infra/...
//
// Requires a Podman/Docker runtime so QuickstartBoot can bring up the
// Postgres container on host port 24100 (spec §9.4). Without a runtime the
// test cleanly Skips (closed-set reason hardware_not_present). Migration
// 000014_dead_letter_tasks is applied automatically by boot.Up().
//
// ANTI-BLUFF CONTRACT (§107 / §11.4.5 / §11.4.68): the move is exercised
// against a REAL row, then BOTH downstream sinks are verified independently:
//  1. the source row's terminal state via raw SELECT on background_tasks,
//  2. the dead_letter_tasks snapshot row via raw SELECT (reason + JSON
//     round-trip of the original id), and
//  3. the .queue.dead_letter governance event actually fanned out on a REAL
//     MemoryBus subscriber (not a counter, not a mock) — the load-bearing
//     proof the emit path works end-to-end, the gap boot.go documents.

package infra

import (
	"encoding/json"
	"testing"
	"time"

	"digital.vasic.models"
	constitution "github.com/vasic-digital/herald/commons_constitution"
)

func TestRepoMoveToDeadLetter_RoundTripAndEmit(t *testing.T) {
	baseRepo, pool, ctx := bootRepo(t)
	id := seedTask(t, baseRepo, ctx)

	// Mark the seeded task as having burned its retries so the snapshot
	// carries a non-zero failure count (proves FailureCount propagation).
	task, err := baseRepo.GetByID(ctx, id)
	if err != nil || task == nil {
		t.Fatalf("GetByID pre-move: %v / %v", err, task)
	}
	task.Status = models.TaskStatusRunning
	task.RetryCount = 3
	if err := baseRepo.Update(ctx, task); err != nil {
		t.Fatalf("Update pre-move: %v", err)
	}

	// Wire a REAL bus + subscriber + emitter onto the same live pool.
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{})
	defer bus.Close()
	sub, err := bus.Subscribe("*")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Cancel()
	em, err := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "digital.vasic.herald/commons_infra-it"})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}
	repo := newPgxTaskRepositoryWithEmitter(pool, em)

	const reason = "exhausted retries (HRD-090 integration)"
	if err := repo.MoveToDeadLetter(ctx, id, reason); err != nil {
		t.Fatalf("MoveToDeadLetter: %v", err)
	}

	// Sink 1: the source row is now terminal (dead_letter) with the reason.
	var status, lastErr string
	var completedAt *time.Time
	if err := pool.QueryRow(ctx,
		"SELECT status, last_error, completed_at FROM background_tasks WHERE id = $1", id,
	).Scan(&status, &lastErr, &completedAt); err != nil {
		t.Fatalf("read-back background_tasks: %v", err)
	}
	if status != "dead_letter" {
		t.Errorf("source row status = %q; want dead_letter", status)
	}
	if lastErr != reason {
		t.Errorf("source row last_error = %q; want %q", lastErr, reason)
	}
	if completedAt == nil {
		t.Error("source row completed_at not stamped on terminal move")
	}

	// Sink 2: the dead_letter_tasks snapshot row exists and round-trips.
	var originalID, failureReason string
	var failureCount int
	var reprocessed bool
	var taskData []byte
	if err := pool.QueryRow(ctx,
		`SELECT original_task_id, failure_reason, failure_count, reprocessed, task_data
		   FROM dead_letter_tasks WHERE original_task_id = $1`, id,
	).Scan(&originalID, &failureReason, &failureCount, &reprocessed, &taskData); err != nil {
		t.Fatalf("read-back dead_letter_tasks: %v", err)
	}
	if originalID != id {
		t.Errorf("dlq original_task_id = %q; want %q", originalID, id)
	}
	if failureReason != reason {
		t.Errorf("dlq failure_reason = %q; want %q", failureReason, reason)
	}
	if failureCount != 3 {
		t.Errorf("dlq failure_count = %d; want 3", failureCount)
	}
	if reprocessed {
		t.Error("dlq reprocessed = true; want false on a fresh move")
	}
	var snap map[string]any
	if err := json.Unmarshal(taskData, &snap); err != nil {
		t.Fatalf("dlq task_data not valid JSON: %v", err)
	}
	if snap["id"] != id {
		t.Errorf("dlq task_data snapshot id = %v; want %q", snap["id"], id)
	}

	// Sink 3: the .queue.dead_letter governance event actually fanned out.
	select {
	case e := <-sub.Channel:
		want := constitution.EventNamespace + "." + constitution.ClassQueueDeadLetter
		if e.Type != want {
			t.Errorf("event type = %q; want %q", e.Type, want)
		}
		if e.Subject != "task:"+id {
			t.Errorf("event subject = %q; want task:%s", e.Subject, id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no .queue.dead_letter event received — emit path is a §107 bluff")
	}

	// Not-found move must error loudly (no silent success).
	if err := repo.MoveToDeadLetter(ctx, "ghost-id-never-existed", "x"); err == nil {
		t.Fatal("MoveToDeadLetter on a missing id must error")
	}
}
