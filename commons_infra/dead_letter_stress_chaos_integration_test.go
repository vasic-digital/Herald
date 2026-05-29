//go:build integration

// §11.4.85 stress + chaos evidence for HRD-090 MoveToDeadLetter against real
// Postgres. Run with:
//
//	go test -tags=integration -timeout 8m -count=1 -race \
//	  -run 'TestMoveToDeadLetter_StressChaos' ./commons_infra/...
//
// QA-ANCHOR: HRD-090-STRESS-CHAOS-20260529
//
// Stress: 50 concurrent MoveToDeadLetter on distinct tasks through one repo —
// proves no lost writes / no deadlock / no data race (under -race) and exactly
// 50 dead-letter rows land. Chaos: (a) a failing emitter must NOT roll back the
// durable move (the task IS dead-lettered; the emit error is surfaced); (b) a
// cancelled context must leave NO orphan — the tx is atomic, so either the move
// fully lands or not at all.

package infra

import (
	"context"
	"errors"
	"testing"

	"github.com/vasic-digital/herald/commons/stresschaos"
)

func TestMoveToDeadLetter_StressChaos(t *testing.T) {
	repo, pool, ctx := bootRepo(t)

	t.Run("Stress_ConcurrentDistinctTasks", func(t *testing.T) {
		const n = 50
		ids := make([]string, n)
		for i := 0; i < n; i++ {
			ids[i] = seedTask(t, repo, ctx)
		}

		// nil emitter: this subtest isolates DB-move concurrency correctness.
		// The emit path is proven in TestRepoMoveToDeadLetter_RoundTripAndEmit.
		sum := stresschaos.RunLoad(n, 1, func(worker, iter int) error {
			return repo.MoveToDeadLetter(ctx, ids[worker], "stress dead-letter")
		})
		if sum.Errors != 0 {
			t.Fatalf("stress: %d/%d moves errored (want 0); p99=%.2fms", sum.Errors, sum.Count, sum.Latency.P99MS)
		}
		t.Logf("stress: %d concurrent moves, 0 errors, %.0f moves/s, p99=%.2fms",
			sum.Count, sum.ThroughputPS, sum.Latency.P99MS)

		// Exactly n dead-letter rows + n terminal source rows — no lost writes,
		// no double-insert.
		var dlqCount, terminalCount int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM dead_letter_tasks").Scan(&dlqCount); err != nil {
			t.Fatalf("count dead_letter_tasks: %v", err)
		}
		if dlqCount != n {
			t.Errorf("dead_letter_tasks has %d rows; want %d (lost or duplicated writes)", dlqCount, n)
		}
		if err := pool.QueryRow(ctx,
			"SELECT count(*) FROM background_tasks WHERE status = 'dead_letter'").Scan(&terminalCount); err != nil {
			t.Fatalf("count terminal tasks: %v", err)
		}
		if terminalCount != n {
			t.Errorf("%d tasks marked dead_letter; want %d", terminalCount, n)
		}
	})

	t.Run("Chaos_EmitFailureStillMovesAtomically", func(t *testing.T) {
		id := seedTask(t, repo, ctx)
		failing := &recordingEmitter{err: errors.New("injected bus failure")}
		chaosRepo := newPgxTaskRepositoryWithEmitter(pool, failing)

		err := chaosRepo.MoveToDeadLetter(ctx, id, "emit-failure chaos")
		if err == nil {
			t.Fatal("a failing emitter must surface an error (not be swallowed)")
		}
		// ...but the DURABLE move must have committed regardless.
		var status string
		if err := pool.QueryRow(ctx,
			"SELECT status FROM background_tasks WHERE id = $1", id).Scan(&status); err != nil {
			t.Fatalf("read-back status: %v", err)
		}
		if status != "dead_letter" {
			t.Errorf("emit failure rolled back the move: status=%q want dead_letter", status)
		}
		var dlq int
		if err := pool.QueryRow(ctx,
			"SELECT count(*) FROM dead_letter_tasks WHERE original_task_id = $1", id).Scan(&dlq); err != nil {
			t.Fatalf("count dlq: %v", err)
		}
		if dlq != 1 {
			t.Errorf("emit failure lost the dlq snapshot: %d rows want 1", dlq)
		}
		// The emit WAS attempted exactly once (proves the surfaced error came
		// from the emit, not a swallow).
		if len(failing.deadLetters) != 1 {
			t.Errorf("emit attempted %d times; want 1", len(failing.deadLetters))
		}
	})

	t.Run("Chaos_CancelledContextNoOrphan", func(t *testing.T) {
		id := seedTask(t, repo, ctx)
		cctx, cancel := context.WithCancel(ctx)
		cancel() // cancel BEFORE the move — every DB call should fail fast.

		if err := repo.MoveToDeadLetter(cctx, id, "cancelled chaos"); err == nil {
			t.Fatal("move under a cancelled context must error")
		}
		// Atomicity: no orphan dlq row, source status unchanged.
		var dlq int
		if err := pool.QueryRow(ctx,
			"SELECT count(*) FROM dead_letter_tasks WHERE original_task_id = $1", id).Scan(&dlq); err != nil {
			t.Fatalf("count dlq: %v", err)
		}
		if dlq != 0 {
			t.Errorf("cancelled move left %d orphan dlq rows; want 0 (atomicity violated)", dlq)
		}
		var status string
		if err := pool.QueryRow(ctx,
			"SELECT status FROM background_tasks WHERE id = $1", id).Scan(&status); err != nil {
			t.Fatalf("read-back status: %v", err)
		}
		if status == "dead_letter" {
			t.Error("cancelled move still marked the source row dead_letter (atomicity violated)")
		}
	})
}
