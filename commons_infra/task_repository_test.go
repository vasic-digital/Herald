// Hermetic unit tests for the pgxTaskRepository SQL-binding layer
// (HRD-085..089 — v1.0.0 Batch D).
//
// SCOPE + ANTI-BLUFF DISCLOSURE (§107 / CONST-050(A)): these tests use a
// recording fake db.Database to assert the QUERY SHAPE and ARGUMENT
// CONSTRUCTION of each method, plus the pure-Go validation guards
// (invalid status, limit<=0, nil snapshot, progress clamp, soft-delete
// filter presence). They are the CHEAP layer — they do NOT prove the SQL
// actually round-trips against Postgres. The load-bearing positive
// evidence is the live-PG integration suite in
// task_repository_integration_test.go (//go:build integration). A fake is
// permitted here ONLY because this is a unit test; every other layer must
// hit real Postgres.
//
// What a fake CAN honestly prove: that we send the right SQL with the
// right args + reject bad input before touching the DB. What it CANNOT
// prove: that Postgres accepts the SQL or that the round-trip preserves
// state. Do not mistake a green run here for end-user proof.

package infra

import (
	"context"
	"strings"
	"testing"
	"time"

	db "digital.vasic.database/pkg/database"
	"digital.vasic.models"
)

// --- recording fake db.Database (unit-test only) ---

type recordedCall struct {
	sql  string
	args []any
}

// fakeResult satisfies db.Result with a fixed RowsAffected.
type fakeResult struct {
	affected int64
	affErr   error
}

func (r fakeResult) RowsAffected() (int64, error) { return r.affected, r.affErr }

// fakeRow satisfies db.Row; Scan is a no-op returning scanErr (default
// errNoRows-equivalent "no rows" so GetByID's not-found branch is testable).
type fakeRow struct{ scanErr error }

func (r fakeRow) Scan(dest ...any) error { return r.scanErr }

// fakeRows satisfies db.Rows with zero rows (Next always false).
type fakeRows struct{ closed bool }

func (r *fakeRows) Next() bool          { return false }
func (r *fakeRows) Scan(dest ...any) error { return nil }
func (r *fakeRows) Close() error        { r.closed = true; return nil }
func (r *fakeRows) Err() error          { return nil }

// recordingDB records every Exec/Query/QueryRow so tests can assert SQL +
// args without a live Postgres. It is NOT a production substitute — see
// the file header anti-bluff disclosure.
type recordingDB struct {
	execs     []recordedCall
	queries   []recordedCall
	queryRows []recordedCall
	execResult fakeResult
	rowScanErr error
}

func (d *recordingDB) Connect(ctx context.Context) error { return nil }
func (d *recordingDB) Close() error                       { return nil }
func (d *recordingDB) HealthCheck(ctx context.Context) error { return nil }
func (d *recordingDB) Begin(ctx context.Context) (db.Tx, error) {
	return nil, nil
}
func (d *recordingDB) Exec(ctx context.Context, query string, args ...any) (db.Result, error) {
	d.execs = append(d.execs, recordedCall{sql: query, args: args})
	return d.execResult, nil
}
func (d *recordingDB) Query(ctx context.Context, query string, args ...any) (db.Rows, error) {
	d.queries = append(d.queries, recordedCall{sql: query, args: args})
	return &fakeRows{}, nil
}
func (d *recordingDB) QueryRow(ctx context.Context, query string, args ...any) db.Row {
	d.queryRows = append(d.queryRows, recordedCall{sql: query, args: args})
	return fakeRow{scanErr: d.rowScanErr}
}

func newRecRepo() (*pgxTaskRepository, *recordingDB) {
	rec := &recordingDB{execResult: fakeResult{affected: 1}}
	return newPgxTaskRepository(rec), rec
}

// --- HRD-085 ---

func TestGetByID_NotFound_ReturnsNilNil(t *testing.T) {
	repo, rec := newRecRepo()
	// Default fakeRow returns "no rows" so the not-found branch triggers.
	rec.rowScanErr = errNoRows
	got, err := repo.GetByID(context.Background(), "missing-id")
	if err != nil {
		t.Fatalf("GetByID not-found should be (nil,nil), got err=%v", err)
	}
	if got != nil {
		t.Fatalf("GetByID not-found should return nil task, got %+v", got)
	}
	if len(rec.queryRows) != 1 {
		t.Fatalf("expected 1 QueryRow, got %d", len(rec.queryRows))
	}
	// Soft-delete filter MUST be present so a Delete'd task reads back as
	// not-found — a SELECT without it would leak deleted rows.
	if !strings.Contains(rec.queryRows[0].sql, "deleted_at IS NULL") {
		t.Fatalf("GetByID SQL must filter deleted_at IS NULL; got: %s", rec.queryRows[0].sql)
	}
	if got, want := rec.queryRows[0].args[0], "missing-id"; got != want {
		t.Fatalf("GetByID arg: got %v want %v", got, want)
	}
}

func TestUpdate_NilTask_Errors(t *testing.T) {
	repo, _ := newRecRepo()
	if err := repo.Update(context.Background(), nil); err == nil {
		t.Fatal("Update(nil) must error")
	}
}

func TestUpdate_SendsAllColumns_ForcesUpdatedAtNow(t *testing.T) {
	repo, rec := newRecRepo()
	task := models.NewBackgroundTask("t.type", "t.name", nil)
	task.ID = "task-1"
	if err := repo.Update(context.Background(), task); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(rec.execs) != 1 {
		t.Fatalf("expected 1 Exec, got %d", len(rec.execs))
	}
	sql := rec.execs[0].sql
	if !strings.Contains(sql, "UPDATE background_tasks SET") {
		t.Fatalf("Update must UPDATE background_tasks; got: %s", sql)
	}
	// updated_at forced server-side, never a caller-supplied stale value.
	if !strings.Contains(sql, "updated_at = now()") {
		t.Fatalf("Update must force updated_at = now(); got: %s", sql)
	}
	// id is the WHERE key (first arg).
	if got := rec.execs[0].args[0]; got != "task-1" {
		t.Fatalf("Update first arg (WHERE id): got %v want task-1", got)
	}
}

func TestUpdate_NoRowsAffected_Errors(t *testing.T) {
	repo, rec := newRecRepo()
	rec.execResult = fakeResult{affected: 0}
	task := models.NewBackgroundTask("t", "n", nil)
	task.ID = "ghost"
	if err := repo.Update(context.Background(), task); err == nil {
		t.Fatal("Update on missing id must error (not silent success-bluff)")
	}
}

func TestDelete_IsSoftDelete(t *testing.T) {
	repo, rec := newRecRepo()
	if err := repo.Delete(context.Background(), "task-x"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	sql := rec.execs[0].sql
	if !strings.Contains(sql, "deleted_at = now()") {
		t.Fatalf("Delete must be a soft delete (deleted_at = now()); got: %s", sql)
	}
	if strings.Contains(strings.ToUpper(sql), "DELETE FROM") {
		t.Fatalf("Delete must NOT hard-DELETE (preserve audit + event FK); got: %s", sql)
	}
}

func TestDelete_NoRowsAffected_Errors(t *testing.T) {
	repo, rec := newRecRepo()
	rec.execResult = fakeResult{affected: 0}
	if err := repo.Delete(context.Background(), "ghost"); err == nil {
		t.Fatal("Delete on missing id must error")
	}
}

// --- HRD-086 ---

func TestUpdateStatus_RejectsUnknownStatus(t *testing.T) {
	repo, rec := newRecRepo()
	if err := repo.UpdateStatus(context.Background(), "id", models.TaskStatus("garbage")); err == nil {
		t.Fatal("UpdateStatus must reject an unknown status")
	}
	if len(rec.execs) != 0 {
		t.Fatal("UpdateStatus must not touch the DB on invalid status")
	}
}

func TestUpdateStatus_TerminalStampsCompletedAt(t *testing.T) {
	repo, rec := newRecRepo()
	if err := repo.UpdateStatus(context.Background(), "id", models.TaskStatusCompleted); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	sql := rec.execs[0].sql
	if !strings.Contains(sql, "completed_at") {
		t.Fatalf("UpdateStatus to a terminal status must touch completed_at; got: %s", sql)
	}
	// The IsTerminal() flag is passed as the $3 guard arg.
	if got := rec.execs[0].args[2]; got != true {
		t.Fatalf("UpdateStatus terminal flag: got %v want true", got)
	}
}

func TestUpdateProgress_ClampsRange(t *testing.T) {
	repo, rec := newRecRepo()
	if err := repo.UpdateProgress(context.Background(), "id", 150, "over"); err != nil {
		t.Fatalf("UpdateProgress: %v", err)
	}
	if got := rec.execs[0].args[1]; got != float64(100) {
		t.Fatalf("UpdateProgress must clamp 150 -> 100, got %v", got)
	}
	if err := repo.UpdateProgress(context.Background(), "id", -5, "under"); err != nil {
		t.Fatalf("UpdateProgress: %v", err)
	}
	if got := rec.execs[1].args[1]; got != float64(0) {
		t.Fatalf("UpdateProgress must clamp -5 -> 0, got %v", got)
	}
}

func TestUpdateHeartbeat_SetsHeartbeatNow(t *testing.T) {
	repo, rec := newRecRepo()
	if err := repo.UpdateHeartbeat(context.Background(), "id"); err != nil {
		t.Fatalf("UpdateHeartbeat: %v", err)
	}
	if !strings.Contains(rec.execs[0].sql, "last_heartbeat = now()") {
		t.Fatalf("UpdateHeartbeat must stamp last_heartbeat = now(); got: %s", rec.execs[0].sql)
	}
}

func TestSaveCheckpoint_EmptyPassesNull(t *testing.T) {
	repo, rec := newRecRepo()
	if err := repo.SaveCheckpoint(context.Background(), "id", nil); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}
	// Empty checkpoint -> SQL NULL (arg is nil), clearing any prior value.
	if got := rec.execs[0].args[1]; got != nil {
		t.Fatalf("SaveCheckpoint empty must pass nil (SQL NULL), got %v", got)
	}
	// Non-empty checkpoint -> string-cast (pgx JSONB typing).
	if err := repo.SaveCheckpoint(context.Background(), "id", []byte(`{"k":1}`)); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}
	if got := rec.execs[1].args[1]; got != `{"k":1}` {
		t.Fatalf("SaveCheckpoint non-empty must string-cast the blob, got %v", got)
	}
}

// --- HRD-087 ---

func TestGetByStatus_RejectsBadInput(t *testing.T) {
	repo, _ := newRecRepo()
	if _, err := repo.GetByStatus(context.Background(), models.TaskStatus("nope"), 10, 0); err == nil {
		t.Fatal("GetByStatus must reject unknown status")
	}
	if _, err := repo.GetByStatus(context.Background(), models.TaskStatusPending, 0, 0); err == nil {
		t.Fatal("GetByStatus must reject limit <= 0")
	}
}

func TestGetByStatus_PaginatesNewestFirst(t *testing.T) {
	repo, rec := newRecRepo()
	got, err := repo.GetByStatus(context.Background(), models.TaskStatusRunning, 5, 10)
	if err != nil {
		t.Fatalf("GetByStatus: %v", err)
	}
	if got == nil {
		t.Fatal("GetByStatus must return non-nil empty slice, not nil")
	}
	sql := rec.queries[0].sql
	if !strings.Contains(sql, "ORDER BY created_at DESC") {
		t.Fatalf("GetByStatus must order newest-first; got: %s", sql)
	}
	if !strings.Contains(sql, "LIMIT $2 OFFSET $3") {
		t.Fatalf("GetByStatus must paginate with LIMIT/OFFSET; got: %s", sql)
	}
	// args: status, limit, offset
	if rec.queries[0].args[1] != 5 || rec.queries[0].args[2] != 10 {
		t.Fatalf("GetByStatus args: got %v want [running 5 10]", rec.queries[0].args)
	}
}

func TestGetPendingTasks_UsesDequeueOrdering(t *testing.T) {
	repo, rec := newRecRepo()
	if _, err := repo.GetPendingTasks(context.Background(), 3); err != nil {
		t.Fatalf("GetPendingTasks: %v", err)
	}
	sql := rec.queries[0].sql
	if !strings.Contains(sql, "status = 'pending'") || !strings.Contains(sql, "scheduled_at <= now()") {
		t.Fatalf("GetPendingTasks must match the Dequeue eligibility filter; got: %s", sql)
	}
	if !strings.Contains(sql, "WHEN 'critical'") {
		t.Fatalf("GetPendingTasks must use the priority ordering of Dequeue; got: %s", sql)
	}
}

func TestGetPendingTasks_RejectsBadLimit(t *testing.T) {
	repo, _ := newRecRepo()
	if _, err := repo.GetPendingTasks(context.Background(), 0); err == nil {
		t.Fatal("GetPendingTasks must reject limit <= 0")
	}
}

func TestCountByStatus_GroupsByStatus(t *testing.T) {
	repo, rec := newRecRepo()
	got, err := repo.CountByStatus(context.Background())
	if err != nil {
		t.Fatalf("CountByStatus: %v", err)
	}
	if got == nil {
		t.Fatal("CountByStatus must return a non-nil map")
	}
	if !strings.Contains(rec.queries[0].sql, "GROUP BY status") {
		t.Fatalf("CountByStatus must GROUP BY status; got: %s", rec.queries[0].sql)
	}
}

func TestGetTaskHistory_RejectsBadLimit(t *testing.T) {
	repo, _ := newRecRepo()
	if _, err := repo.GetTaskHistory(context.Background(), "id", -1); err == nil {
		t.Fatal("GetTaskHistory must reject limit <= 0")
	}
}

func TestGetTaskHistory_ReadsEventsTableNewestFirst(t *testing.T) {
	repo, rec := newRecRepo()
	if _, err := repo.GetTaskHistory(context.Background(), "task-h", 5); err != nil {
		t.Fatalf("GetTaskHistory: %v", err)
	}
	sql := rec.queries[0].sql
	if !strings.Contains(sql, "FROM background_task_events") {
		t.Fatalf("GetTaskHistory must read background_task_events; got: %s", sql)
	}
	if !strings.Contains(sql, "ORDER BY created_at DESC") {
		t.Fatalf("GetTaskHistory must order newest-first; got: %s", sql)
	}
}

// --- HRD-088 ---

func TestGetStaleTasks_RejectsNonPositiveThreshold(t *testing.T) {
	repo, _ := newRecRepo()
	if _, err := repo.GetStaleTasks(context.Background(), 0); err == nil {
		t.Fatal("GetStaleTasks must reject threshold <= 0")
	}
}

func TestGetStaleTasks_FiltersActiveAndStaleHeartbeat(t *testing.T) {
	repo, rec := newRecRepo()
	if _, err := repo.GetStaleTasks(context.Background(), time.Minute); err != nil {
		t.Fatalf("GetStaleTasks: %v", err)
	}
	sql := rec.queries[0].sql
	if !strings.Contains(sql, "status IN ('queued', 'running')") {
		t.Fatalf("GetStaleTasks must restrict to active statuses; got: %s", sql)
	}
	if !strings.Contains(sql, "last_heartbeat IS NULL OR last_heartbeat < $1") {
		t.Fatalf("GetStaleTasks must include never-heartbeated + stale rows; got: %s", sql)
	}
	// The cutoff arg must be a time roughly threshold-ago.
	cutoff, ok := rec.queries[0].args[0].(time.Time)
	if !ok {
		t.Fatalf("GetStaleTasks cutoff arg must be time.Time, got %T", rec.queries[0].args[0])
	}
	if time.Since(cutoff) < 50*time.Second {
		t.Fatalf("GetStaleTasks cutoff should be ~1min ago, got %s ago", time.Since(cutoff))
	}
}

func TestGetByWorkerID_RejectsEmptyWorker(t *testing.T) {
	repo, _ := newRecRepo()
	if _, err := repo.GetByWorkerID(context.Background(), ""); err == nil {
		t.Fatal("GetByWorkerID must reject empty workerID")
	}
}

func TestGetByWorkerID_FiltersByWorker(t *testing.T) {
	repo, rec := newRecRepo()
	if _, err := repo.GetByWorkerID(context.Background(), "worker-7"); err != nil {
		t.Fatalf("GetByWorkerID: %v", err)
	}
	if !strings.Contains(rec.queries[0].sql, "worker_id = $1") {
		t.Fatalf("GetByWorkerID must filter worker_id = $1; got: %s", rec.queries[0].sql)
	}
	if rec.queries[0].args[0] != "worker-7" {
		t.Fatalf("GetByWorkerID arg: got %v want worker-7", rec.queries[0].args[0])
	}
}

// --- HRD-089 ---

func TestSaveResourceSnapshot_NilAndMissingTaskID(t *testing.T) {
	repo, _ := newRecRepo()
	if err := repo.SaveResourceSnapshot(context.Background(), nil); err == nil {
		t.Fatal("SaveResourceSnapshot(nil) must error")
	}
	if err := repo.SaveResourceSnapshot(context.Background(), &models.ResourceSnapshot{}); err == nil {
		t.Fatal("SaveResourceSnapshot with empty TaskID must error")
	}
}

func TestSaveResourceSnapshot_AutoFillsIDAndSampledAt(t *testing.T) {
	repo, rec := newRecRepo()
	snap := &models.ResourceSnapshot{TaskID: "task-r", CPUPercent: 42.5}
	if err := repo.SaveResourceSnapshot(context.Background(), snap); err != nil {
		t.Fatalf("SaveResourceSnapshot: %v", err)
	}
	if snap.ID == "" {
		t.Fatal("SaveResourceSnapshot must auto-fill an empty ID")
	}
	if snap.SampledAt.IsZero() {
		t.Fatal("SaveResourceSnapshot must auto-fill an empty SampledAt")
	}
	if !strings.Contains(rec.execs[0].sql, "INSERT INTO task_resource_snapshots") {
		t.Fatalf("SaveResourceSnapshot must INSERT into task_resource_snapshots; got: %s", rec.execs[0].sql)
	}
	// 20 columns -> 20 args.
	if len(rec.execs[0].args) != 20 {
		t.Fatalf("SaveResourceSnapshot must bind 20 args, got %d", len(rec.execs[0].args))
	}
}

func TestGetResourceSnapshots_RejectsBadLimit(t *testing.T) {
	repo, _ := newRecRepo()
	if _, err := repo.GetResourceSnapshots(context.Background(), "id", 0); err == nil {
		t.Fatal("GetResourceSnapshots must reject limit <= 0")
	}
}

func TestGetResourceSnapshots_ReverseChronological(t *testing.T) {
	repo, rec := newRecRepo()
	got, err := repo.GetResourceSnapshots(context.Background(), "task-r", 10)
	if err != nil {
		t.Fatalf("GetResourceSnapshots: %v", err)
	}
	if got == nil {
		t.Fatal("GetResourceSnapshots must return a non-nil empty slice")
	}
	sql := rec.queries[0].sql
	if !strings.Contains(sql, "FROM task_resource_snapshots") {
		t.Fatalf("GetResourceSnapshots must read task_resource_snapshots; got: %s", sql)
	}
	if !strings.Contains(sql, "ORDER BY sampled_at DESC") {
		t.Fatalf("GetResourceSnapshots must be reverse-chronological; got: %s", sql)
	}
}

// --- HRD-090 (still unsupported by design) ---

func TestMoveToDeadLetter_StillUnsupported(t *testing.T) {
	repo, _ := newRecRepo()
	err := repo.MoveToDeadLetter(context.Background(), "id", "reason")
	if err == nil {
		t.Fatal("MoveToDeadLetter must still return ErrUnsupported (HRD-090 out of Batch D scope)")
	}
	if !strings.Contains(err.Error(), "HRD-090") {
		t.Fatalf("MoveToDeadLetter error must cite HRD-090, got: %v", err)
	}
}

// --- helper guards ---

func TestValidTaskStatus(t *testing.T) {
	for _, s := range []models.TaskStatus{
		models.TaskStatusPending, models.TaskStatusQueued, models.TaskStatusRunning,
		models.TaskStatusPaused, models.TaskStatusCompleted, models.TaskStatusFailed,
		models.TaskStatusStuck, models.TaskStatusCancelled, models.TaskStatusDeadLetter,
	} {
		if !validTaskStatus(s) {
			t.Fatalf("validTaskStatus(%q) should be true", s)
		}
	}
	if validTaskStatus(models.TaskStatus("bogus")) {
		t.Fatal("validTaskStatus(bogus) should be false")
	}
}
