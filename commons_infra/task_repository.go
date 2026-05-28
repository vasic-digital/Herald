// Package infra — minimal TaskRepository implementation backing the
// upstream digital.vasic.background.PostgresTaskQueue.
//
// §11.4.74 catalogue-check + extend-don't-reimplement:
//
// The upstream `digital.vasic.background.PostgresTaskQueue` requires a
// `background.TaskRepository` implementation but does NOT ship one in the
// submodule itself — the README documents that consuming projects own the
// repository because the schema (column names, indexes, partition strategy)
// is consumer-specific. HelixAgent owns an internal/database one for its
// schema; Herald owns this one for its schema (commons_storage migration
// 000009).
//
// This is therefore extend-don't-reimplement at the seam: we reuse the
// upstream PostgresTaskQueue's logic (Enqueue/Dequeue/Peek/Requeue/
// MoveToDeadLetter/GetPendingCount/GetRunningCount/GetQueueDepth) and own
// only the thin SQL-binding repository underneath it. The repository is
// the project-specific layer; the queue logic is the universal layer; the
// split aligns with §11.4.74.
//
// MVP scope (HRD-010 Task 5): implements the three methods the Enqueue+
// Dequeue hot path exercises (Create, Dequeue, LogEvent). The remaining
// 19 methods of the TaskRepository interface return ErrUnsupported with
// a clear pointer to the HRD-NNN that will land each. This keeps the
// surface honest — no caller gets a silent zero-result; an unimplemented
// method fails loudly at the call site so the gap is obvious.
//
// When additional flows land, replace the ErrUnsupported stub with a real
// implementation + a paired integration test under
// commons_infra/clients_integration_test.go. NO bluff stubs (§107):
// returning nil + nil from a method that should query the DB IS a bluff.
package infra

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	db "digital.vasic.database/pkg/database"
	"digital.vasic.models"
	"github.com/google/uuid"
)

// ErrUnsupported is returned by stub repository methods that have not
// been implemented yet. Each return site cites the HRD-NNN that will
// land the missing functionality, so callers can route the failure
// directly to the open issue.
var ErrUnsupported = errors.New("commons_infra: TaskRepository method not yet implemented; see HRD pointer")

// pgxTaskRepository is Herald's minimal-but-honest implementation of
// background.TaskRepository against the `background_tasks` +
// `background_task_events` tables (migration 000009).
//
// The struct holds the universal `db.Database` interface — NOT the raw
// pgxpool.Pool — so it composes with the Helix-stack abstraction and
// can be swapped to the SQLite adapter in unit tests if needed (though
// no unit tests are written against this yet; the E15 integration test
// is the load-bearing positive evidence per §11.4.68).
type pgxTaskRepository struct {
	database db.Database
}

// newPgxTaskRepository constructs a repository backed by an open
// Herald-spec database (commons_storage.Open's return value).
func newPgxTaskRepository(database db.Database) *pgxTaskRepository {
	return &pgxTaskRepository{database: database}
}

// Create inserts a new task row. Called by PostgresTaskQueue.Enqueue.
//
// The column list matches commons_storage migration 000009's
// background_tasks table exactly; struct fields map 1:1 to columns.
// JSON-bearing fields are marshalled inline because pgx accepts
// json.RawMessage as a []byte alias.
func (r *pgxTaskRepository) Create(ctx context.Context, task *models.BackgroundTask) error {
	if task == nil {
		return errors.New("commons_infra.Create: nil task")
	}
	now := time.Now().UTC()
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = now
	}
	if task.ScheduledAt.IsZero() {
		task.ScheduledAt = now
	}
	configJSON, err := json.Marshal(task.Config)
	if err != nil {
		return fmt.Errorf("commons_infra.Create: marshal config: %w", err)
	}
	notifJSON, err := json.Marshal(task.NotificationConfig)
	if err != nil {
		return fmt.Errorf("commons_infra.Create: marshal notification_config: %w", err)
	}
	if len(task.Payload) == 0 {
		task.Payload = json.RawMessage(`{}`)
	}
	if len(task.ErrorHistory) == 0 {
		task.ErrorHistory = json.RawMessage(`[]`)
	}
	if len(task.Tags) == 0 {
		task.Tags = json.RawMessage(`[]`)
	}
	if len(task.Metadata) == 0 {
		task.Metadata = json.RawMessage(`{}`)
	}
	// JSONB-NULL bridge: pgx serialises a typed []byte(nil) to '' which
	// Postgres rejects with SQLSTATE 22P02. For nullable JSONB columns
	// we must pass an explicit `any(nil)` so pgx emits SQL NULL.
	var checkpointArg any
	if len(task.Checkpoint) > 0 {
		checkpointArg = string(task.Checkpoint)
	}

	const sql = `
		INSERT INTO background_tasks (
			id, task_type, task_name, correlation_id, parent_task_id,
			payload, config, priority,
			status, progress, progress_message, checkpoint,
			max_retries, retry_count, retry_delay_seconds, last_error, error_history,
			worker_id, process_pid, started_at, completed_at, last_heartbeat, deadline,
			required_cpu_cores, required_memory_mb, estimated_duration_seconds, actual_duration_seconds,
			notification_config,
			user_id, session_id, tags, metadata,
			created_at, updated_at, scheduled_at, deleted_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8,
			$9, $10, $11, $12,
			$13, $14, $15, $16, $17,
			$18, $19, $20, $21, $22, $23,
			$24, $25, $26, $27,
			$28,
			$29, $30, $31, $32,
			$33, $34, $35, $36
		)
	`
	// pgx v5 typing rule: passing a typed `[]byte` to a JSONB column makes
	// the driver send the bytes as a bytea cast — Postgres then rejects
	// it with SQLSTATE 22P02 (invalid input for type json). Cast each
	// JSON-bearing arg to `string` so pgx sends it as the canonical TEXT
	// representation, which Postgres happily implicit-casts to JSONB.
	_, err = r.database.Exec(ctx, sql,
		task.ID, task.TaskType, task.TaskName, task.CorrelationID, task.ParentTaskID,
		string(task.Payload), string(configJSON), string(task.Priority),
		string(task.Status), task.Progress, task.ProgressMessage, checkpointArg,
		task.MaxRetries, task.RetryCount, task.RetryDelaySeconds, task.LastError, string(task.ErrorHistory),
		task.WorkerID, task.ProcessPID, task.StartedAt, task.CompletedAt, task.LastHeartbeat, task.Deadline,
		task.RequiredCPUCores, task.RequiredMemoryMB, task.EstimatedDurationSeconds, task.ActualDurationSeconds,
		string(notifJSON),
		task.UserID, task.SessionID, string(task.Tags), string(task.Metadata),
		task.CreatedAt, task.UpdatedAt, task.ScheduledAt, task.DeletedAt,
	)
	if err != nil {
		return fmt.Errorf("commons_infra.Create: insert: %w", err)
	}
	return nil
}

// Dequeue atomically claims the next eligible pending task for the named
// worker. Called by PostgresTaskQueue.Dequeue. Uses FOR UPDATE SKIP LOCKED
// per the canonical upstream pattern (background docs §INTEGRATION_GUIDE).
//
// Resource-requirement filters: maxCPU / maxMemoryMB constrain the
// eligible-task set. A zero value means "no limit" per the upstream
// PostgresTaskQueue.Dequeue convention (see submodules/background/
// task_queue.go).
//
// Returns (nil, nil) when no eligible task is available — this is NOT
// an error; the queue is just empty.
func (r *pgxTaskRepository) Dequeue(ctx context.Context, workerID string, maxCPUCores, maxMemoryMB int) (*models.BackgroundTask, error) {
	const sql = `
		UPDATE background_tasks
		SET status = 'running',
		    worker_id = $1,
		    started_at = now(),
		    last_heartbeat = now(),
		    updated_at = now()
		WHERE id = (
			SELECT id FROM background_tasks
			WHERE status = 'pending'
			  AND deleted_at IS NULL
			  AND scheduled_at <= now()
			  AND ($2 = 0 OR required_cpu_cores <= $2)
			  AND ($3 = 0 OR required_memory_mb <= $3)
			ORDER BY
			  CASE priority
			    WHEN 'critical'   THEN 0
			    WHEN 'high'       THEN 1
			    WHEN 'normal'     THEN 2
			    WHEN 'low'        THEN 3
			    WHEN 'background' THEN 4
			    ELSE 2
			  END,
			  created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		RETURNING
			id, task_type, task_name, correlation_id, parent_task_id,
			payload, config, priority,
			status, progress, progress_message, checkpoint,
			max_retries, retry_count, retry_delay_seconds, last_error, error_history,
			worker_id, process_pid, started_at, completed_at, last_heartbeat, deadline,
			required_cpu_cores, required_memory_mb, estimated_duration_seconds, actual_duration_seconds,
			notification_config,
			user_id, session_id, tags, metadata,
			created_at, updated_at, scheduled_at, deleted_at
	`
	row := r.database.QueryRow(ctx, sql, workerID, maxCPUCores, maxMemoryMB)
	task, err := scanTask(row)
	if errors.Is(err, errNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("commons_infra.Dequeue: %w", err)
	}
	return task, nil
}

// LogEvent appends an execution-history row for the task. Called by
// PostgresTaskQueue.Enqueue + Dequeue on the happy path to record
// task.created / task.started events.
//
// Failures are tolerated by the upstream queue (it logs to its own
// logrus instance and continues) so this method's error return is
// best-effort.
func (r *pgxTaskRepository) LogEvent(ctx context.Context, taskID, eventType string, data map[string]interface{}, workerID *string) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("commons_infra.LogEvent: marshal data: %w", err)
	}
	const sql = `
		INSERT INTO background_task_events (task_id, event_type, event_data, worker_id)
		VALUES ($1, $2, $3, $4)
	`
	// event_data is JSONB — pass the marshalled JSON as a string, not []byte:
	// the db.Database layer encodes []byte as bytea, which PG refuses to coerce
	// into jsonb (SQLSTATE 22P02). Mirrors the string()-cast pattern in Create.
	// (Latent pre-Batch-D bug surfaced by the HRD-087 GetTaskHistory real-PG test.)
	_, err = r.database.Exec(ctx, sql, taskID, eventType, string(payload), workerID)
	if err != nil {
		return fmt.Errorf("commons_infra.LogEvent: insert: %w", err)
	}
	return nil
}

// taskColumns is the canonical 36-column projection for a BackgroundTask
// row, in the EXACT order scanTask expects. Every single-row + multi-row
// reader below selects this list so column order never drifts from the
// scanTask contract (the same order Dequeue's RETURNING clause uses).
const taskColumns = `
	id, task_type, task_name, correlation_id, parent_task_id,
	payload, config, priority,
	status, progress, progress_message, checkpoint,
	max_retries, retry_count, retry_delay_seconds, last_error, error_history,
	worker_id, process_pid, started_at, completed_at, last_heartbeat, deadline,
	required_cpu_cores, required_memory_mb, estimated_duration_seconds, actual_duration_seconds,
	notification_config,
	user_id, session_id, tags, metadata,
	created_at, updated_at, scheduled_at, deleted_at`

// scanTasks drains a multi-row result set into []*models.BackgroundTask
// using the same per-row scan logic as scanTask. Closes rows on return.
// Returns an empty (non-nil) slice when the query matched zero rows — a
// nil slice would be a §107 ambiguity ("did the read run?"); callers get
// an explicit empty result.
func scanTasks(rows db.Rows) ([]*models.BackgroundTask, error) {
	defer rows.Close()
	out := make([]*models.BackgroundTask, 0)
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scanTasks: iterate: %w", err)
	}
	return out, nil
}

// --- HRD-085 — CRUD reads + full-row update + delete. ---

// GetByID reads a single task row by primary key. Required by the upstream
// Requeue + MoveToDeadLetter flows. Returns (nil, nil) when no row matches
// the id — the interface contract treats "not found" as a non-error,
// distinct from an actual query failure (mirrors Dequeue's empty-queue
// nil,nil convention). Soft-deleted rows (deleted_at IS NOT NULL) are
// excluded so a Delete'd task reads back as not-found.
func (r *pgxTaskRepository) GetByID(ctx context.Context, id string) (*models.BackgroundTask, error) {
	sql := `SELECT ` + taskColumns + ` FROM background_tasks WHERE id = $1 AND deleted_at IS NULL`
	row := r.database.QueryRow(ctx, sql, id)
	task, err := scanTask(row)
	if errors.Is(err, errNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("commons_infra.GetByID: %w", err)
	}
	return task, nil
}

// Update writes every mutable column of task back to its row by id.
// Used by the upstream Requeue path (it mutates retry_count / status /
// scheduled_at then calls Update). The id + created_at are immutable;
// updated_at is forced to now() server-side so callers cannot stamp a
// stale value. JSON-bearing columns are cast to string for the same
// pgx-v5 JSONB-typing reason as Create (a typed []byte triggers SQLSTATE
// 22P02). Returns an error if the id does not exist (RowsAffected == 0) so
// a no-op Update is loud rather than a silent §107 success-bluff.
func (r *pgxTaskRepository) Update(ctx context.Context, task *models.BackgroundTask) error {
	if task == nil {
		return errors.New("commons_infra.Update: nil task")
	}
	configJSON, err := json.Marshal(task.Config)
	if err != nil {
		return fmt.Errorf("commons_infra.Update: marshal config: %w", err)
	}
	notifJSON, err := json.Marshal(task.NotificationConfig)
	if err != nil {
		return fmt.Errorf("commons_infra.Update: marshal notification_config: %w", err)
	}
	if len(task.Payload) == 0 {
		task.Payload = json.RawMessage(`{}`)
	}
	if len(task.ErrorHistory) == 0 {
		task.ErrorHistory = json.RawMessage(`[]`)
	}
	if len(task.Tags) == 0 {
		task.Tags = json.RawMessage(`[]`)
	}
	if len(task.Metadata) == 0 {
		task.Metadata = json.RawMessage(`{}`)
	}
	// Nullable JSONB checkpoint: pass any(nil) so pgx emits SQL NULL rather
	// than '' (SQLSTATE 22P02). Same bridge as Create.
	var checkpointArg any
	if len(task.Checkpoint) > 0 {
		checkpointArg = string(task.Checkpoint)
	}

	const sql = `
		UPDATE background_tasks SET
			task_type = $2, task_name = $3, correlation_id = $4, parent_task_id = $5,
			payload = $6, config = $7, priority = $8,
			status = $9, progress = $10, progress_message = $11, checkpoint = $12,
			max_retries = $13, retry_count = $14, retry_delay_seconds = $15, last_error = $16, error_history = $17,
			worker_id = $18, process_pid = $19, started_at = $20, completed_at = $21, last_heartbeat = $22, deadline = $23,
			required_cpu_cores = $24, required_memory_mb = $25, estimated_duration_seconds = $26, actual_duration_seconds = $27,
			notification_config = $28,
			user_id = $29, session_id = $30, tags = $31, metadata = $32,
			scheduled_at = $33, deleted_at = $34,
			updated_at = now()
		WHERE id = $1
	`
	res, err := r.database.Exec(ctx, sql,
		task.ID,
		task.TaskType, task.TaskName, task.CorrelationID, task.ParentTaskID,
		string(task.Payload), string(configJSON), string(task.Priority),
		string(task.Status), task.Progress, task.ProgressMessage, checkpointArg,
		task.MaxRetries, task.RetryCount, task.RetryDelaySeconds, task.LastError, string(task.ErrorHistory),
		task.WorkerID, task.ProcessPID, task.StartedAt, task.CompletedAt, task.LastHeartbeat, task.Deadline,
		task.RequiredCPUCores, task.RequiredMemoryMB, task.EstimatedDurationSeconds, task.ActualDurationSeconds,
		string(notifJSON),
		task.UserID, task.SessionID, string(task.Tags), string(task.Metadata),
		task.ScheduledAt, task.DeletedAt,
	)
	if err != nil {
		return fmt.Errorf("commons_infra.Update: %w", err)
	}
	return errIfNoRows(res, "Update", task.ID)
}

// Delete soft-deletes a task by stamping deleted_at = now(). The schema
// carries a deleted_at column (migration 000009) and the dispatch hot path
// (Dequeue) already filters `deleted_at IS NULL`, so a soft delete cleanly
// removes the task from every active query while preserving its row +
// execution-history for audit (§107 auditability) and keeping the
// background_task_events FK intact (a hard DELETE would CASCADE-drop the
// event log). Returns an error if id does not exist so a no-op delete is
// loud, not a silent success-bluff.
func (r *pgxTaskRepository) Delete(ctx context.Context, id string) error {
	const sql = `UPDATE background_tasks SET deleted_at = now(), updated_at = now() WHERE id = $1 AND deleted_at IS NULL`
	res, err := r.database.Exec(ctx, sql, id)
	if err != nil {
		return fmt.Errorf("commons_infra.Delete: %w", err)
	}
	return errIfNoRows(res, "Delete", id)
}

// --- HRD-086 — worker-side status / progress / heartbeat / checkpoint. ---

// UpdateStatus transitions a task to the given status, stamping
// completed_at when the target is terminal (so Duration() resolves). The
// upstream models.TaskStatus enum (submodules/Models/background_task.go)
// is a typed string with no hard transition matrix — IsTerminal()/IsActive()
// are advisory helpers, not a closed FSM — so this method does NOT reject
// transitions (the upstream queue owns transition policy; the repository
// is the persistence layer). The only guard is rejecting an unknown status
// value, which would corrupt the row. Returns error if id not found.
func (r *pgxTaskRepository) UpdateStatus(ctx context.Context, id string, status models.TaskStatus) error {
	if !validTaskStatus(status) {
		return fmt.Errorf("commons_infra.UpdateStatus: unknown status %q", status)
	}
	// completed_at is set only on the terminal transition and only if not
	// already set, so re-stamping a terminal status is idempotent.
	const sql = `
		UPDATE background_tasks
		SET status = $2,
		    completed_at = CASE
		        WHEN $3 AND completed_at IS NULL THEN now()
		        ELSE completed_at
		    END,
		    updated_at = now()
		WHERE id = $1
	`
	res, err := r.database.Exec(ctx, sql, id, string(status), status.IsTerminal())
	if err != nil {
		return fmt.Errorf("commons_infra.UpdateStatus: %w", err)
	}
	return errIfNoRows(res, "UpdateStatus", id)
}

// UpdateProgress writes the 0-100 progress percent + an optional human
// message. progress is clamped to [0,100] defensively — a worker that
// reports 137% or -5 is a bug, but persisting the raw value would corrupt
// downstream UI; clamping keeps the column honest. Returns error if id not
// found.
func (r *pgxTaskRepository) UpdateProgress(ctx context.Context, id string, progress float64, message string) error {
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}
	const sql = `
		UPDATE background_tasks
		SET progress = $2, progress_message = $3, updated_at = now()
		WHERE id = $1
	`
	res, err := r.database.Exec(ctx, sql, id, progress, message)
	if err != nil {
		return fmt.Errorf("commons_infra.UpdateProgress: %w", err)
	}
	return errIfNoRows(res, "UpdateProgress", id)
}

// UpdateHeartbeat stamps last_heartbeat = now() so the stuck-detector's
// HasStaleHeartbeat scan (GetStaleTasks) sees the task as alive. Also bumps
// updated_at so the row's mtime reflects the liveness signal. Returns error
// if id not found.
func (r *pgxTaskRepository) UpdateHeartbeat(ctx context.Context, id string) error {
	const sql = `UPDATE background_tasks SET last_heartbeat = now(), updated_at = now() WHERE id = $1`
	res, err := r.database.Exec(ctx, sql, id)
	if err != nil {
		return fmt.Errorf("commons_infra.UpdateHeartbeat: %w", err)
	}
	return errIfNoRows(res, "UpdateHeartbeat", id)
}

// SaveCheckpoint persists an opaque checkpoint blob on the task row so a
// paused task can later Resume. The checkpoint column is JSONB (migration
// 000009); the upstream TaskExecutor.Pause returns []byte that is itself
// JSON, so we cast to string (pgx-v5 JSONB-typing rule) and pass SQL NULL
// for an empty/nil checkpoint (clearing a prior checkpoint). Returns error
// if id not found.
func (r *pgxTaskRepository) SaveCheckpoint(ctx context.Context, id string, checkpoint []byte) error {
	var checkpointArg any
	if len(checkpoint) > 0 {
		checkpointArg = string(checkpoint)
	}
	const sql = `UPDATE background_tasks SET checkpoint = $2, updated_at = now() WHERE id = $1`
	res, err := r.database.Exec(ctx, sql, id, checkpointArg)
	if err != nil {
		return fmt.Errorf("commons_infra.SaveCheckpoint: %w", err)
	}
	return errIfNoRows(res, "SaveCheckpoint", id)
}

// --- HRD-087 — paginated query + counts + execution history. ---

// GetByStatus returns up to limit tasks with the given status, skipping
// offset rows, newest-first. Soft-deleted rows are excluded. limit <= 0 is
// rejected (an unbounded read would be a footgun); offset < 0 is clamped to
// 0. Admin / stats surface.
func (r *pgxTaskRepository) GetByStatus(ctx context.Context, status models.TaskStatus, limit, offset int) ([]*models.BackgroundTask, error) {
	if !validTaskStatus(status) {
		return nil, fmt.Errorf("commons_infra.GetByStatus: unknown status %q", status)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("commons_infra.GetByStatus: limit must be > 0, got %d", limit)
	}
	if offset < 0 {
		offset = 0
	}
	sql := `SELECT ` + taskColumns + `
		FROM background_tasks
		WHERE status = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`
	rows, err := r.database.Query(ctx, sql, string(status), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("commons_infra.GetByStatus: query: %w", err)
	}
	tasks, err := scanTasks(rows)
	if err != nil {
		return nil, fmt.Errorf("commons_infra.GetByStatus: %w", err)
	}
	return tasks, nil
}

// GetPendingTasks returns up to limit pending, schedulable tasks in the
// SAME priority-then-FIFO order as the Dequeue hot path (so a Peek reflects
// the order tasks will actually be claimed). Only rows whose scheduled_at
// has arrived are included, matching Dequeue's eligibility filter. Delegate
// for the upstream Peek. limit <= 0 is rejected.
func (r *pgxTaskRepository) GetPendingTasks(ctx context.Context, limit int) ([]*models.BackgroundTask, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("commons_infra.GetPendingTasks: limit must be > 0, got %d", limit)
	}
	sql := `SELECT ` + taskColumns + `
		FROM background_tasks
		WHERE status = 'pending' AND deleted_at IS NULL AND scheduled_at <= now()
		ORDER BY
		  CASE priority
		    WHEN 'critical'   THEN 0
		    WHEN 'high'       THEN 1
		    WHEN 'normal'     THEN 2
		    WHEN 'low'        THEN 3
		    WHEN 'background' THEN 4
		    ELSE 2
		  END,
		  created_at ASC
		LIMIT $1`
	rows, err := r.database.Query(ctx, sql, limit)
	if err != nil {
		return nil, fmt.Errorf("commons_infra.GetPendingTasks: query: %w", err)
	}
	tasks, err := scanTasks(rows)
	if err != nil {
		return nil, fmt.Errorf("commons_infra.GetPendingTasks: %w", err)
	}
	return tasks, nil
}

// CountByStatus returns a bucketed count of non-deleted tasks keyed by
// status. Statuses with zero rows are simply absent from the map (the
// caller treats a missing key as 0). Stats surface.
func (r *pgxTaskRepository) CountByStatus(ctx context.Context) (map[models.TaskStatus]int64, error) {
	const sql = `
		SELECT status, count(*)
		FROM background_tasks
		WHERE deleted_at IS NULL
		GROUP BY status
	`
	rows, err := r.database.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("commons_infra.CountByStatus: query: %w", err)
	}
	defer rows.Close()
	out := make(map[models.TaskStatus]int64)
	for rows.Next() {
		var status string
		var n int64
		if err := rows.Scan(&status, &n); err != nil {
			return nil, fmt.Errorf("commons_infra.CountByStatus: scan: %w", err)
		}
		out[models.TaskStatus(status)] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("commons_infra.CountByStatus: iterate: %w", err)
	}
	return out, nil
}

// GetTaskHistory returns up to limit execution-history events for the task,
// newest-first, from the background_task_events table (migration 000009,
// the LogEvent target). limit <= 0 is rejected. Admin / audit surface.
func (r *pgxTaskRepository) GetTaskHistory(ctx context.Context, taskID string, limit int) ([]*models.TaskExecutionHistory, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("commons_infra.GetTaskHistory: limit must be > 0, got %d", limit)
	}
	const sql = `
		SELECT id, task_id, event_type, event_data, worker_id, created_at
		FROM background_task_events
		WHERE task_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`
	rows, err := r.database.Query(ctx, sql, taskID, limit)
	if err != nil {
		return nil, fmt.Errorf("commons_infra.GetTaskHistory: query: %w", err)
	}
	defer rows.Close()
	out := make([]*models.TaskExecutionHistory, 0)
	for rows.Next() {
		var h models.TaskExecutionHistory
		var eventData []byte
		if err := rows.Scan(&h.ID, &h.TaskID, &h.EventType, &eventData, &h.WorkerID, &h.CreatedAt); err != nil {
			return nil, fmt.Errorf("commons_infra.GetTaskHistory: scan: %w", err)
		}
		h.EventData = eventData
		out = append(out, &h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("commons_infra.GetTaskHistory: iterate: %w", err)
	}
	return out, nil
}

// --- HRD-088 — stale-task + worker-recovery scans. ---

// GetStaleTasks returns tasks whose last_heartbeat is older than threshold
// (or never set) AND that are in an active state (queued/running) — the
// stuck-detector's candidate set. Terminal + pending tasks are excluded:
// a pending task has no heartbeat by design, and a terminal one is done.
// threshold <= 0 is rejected (it would match every active task). Soft-
// deleted rows are excluded.
func (r *pgxTaskRepository) GetStaleTasks(ctx context.Context, threshold time.Duration) ([]*models.BackgroundTask, error) {
	if threshold <= 0 {
		return nil, fmt.Errorf("commons_infra.GetStaleTasks: threshold must be > 0, got %s", threshold)
	}
	// Compute the cutoff in Go so the comparison uses the same monotonic
	// clock semantics as the rest of Herald (server now() minus an interval
	// would also work, but passing an explicit timestamptz keeps the test
	// deterministic against an injected clock). A task with NULL
	// last_heartbeat that is active is ALSO stale (it never checked in).
	cutoff := time.Now().UTC().Add(-threshold)
	sql := `SELECT ` + taskColumns + `
		FROM background_tasks
		WHERE deleted_at IS NULL
		  AND status IN ('queued', 'running')
		  AND (last_heartbeat IS NULL OR last_heartbeat < $1)
		ORDER BY last_heartbeat ASC NULLS FIRST`
	rows, err := r.database.Query(ctx, sql, cutoff)
	if err != nil {
		return nil, fmt.Errorf("commons_infra.GetStaleTasks: query: %w", err)
	}
	tasks, err := scanTasks(rows)
	if err != nil {
		return nil, fmt.Errorf("commons_infra.GetStaleTasks: %w", err)
	}
	return tasks, nil
}

// GetByWorkerID returns every non-deleted task currently assigned to the
// given worker, newest-first — the worker-recovery scan (on worker restart,
// reclaim or fail the tasks it was holding). An empty workerID is rejected
// (it would match the implicit-NULL bucket ambiguously).
func (r *pgxTaskRepository) GetByWorkerID(ctx context.Context, workerID string) ([]*models.BackgroundTask, error) {
	if workerID == "" {
		return nil, fmt.Errorf("commons_infra.GetByWorkerID: empty workerID")
	}
	sql := `SELECT ` + taskColumns + `
		FROM background_tasks
		WHERE worker_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC`
	rows, err := r.database.Query(ctx, sql, workerID)
	if err != nil {
		return nil, fmt.Errorf("commons_infra.GetByWorkerID: query: %w", err)
	}
	tasks, err := scanTasks(rows)
	if err != nil {
		return nil, fmt.Errorf("commons_infra.GetByWorkerID: %w", err)
	}
	return tasks, nil
}

// --- HRD-089 — resource-snapshot append + paginated read. ---

// SaveResourceSnapshot append-inserts one resource sample for a task into
// task_resource_snapshots (migration 000013). Append-only: a snapshot is a
// point-in-time sample, never updated. An empty snapshot.ID is auto-filled
// with a UUIDv7 (time-ordered) so callers that don't pre-assign one still
// get a stable PK; an empty SampledAt defaults to now(). Returns an error
// on a nil snapshot or missing TaskID (the FK would reject it anyway, but
// failing early gives a clearer message).
func (r *pgxTaskRepository) SaveResourceSnapshot(ctx context.Context, snapshot *models.ResourceSnapshot) error {
	if snapshot == nil {
		return errors.New("commons_infra.SaveResourceSnapshot: nil snapshot")
	}
	if snapshot.TaskID == "" {
		return errors.New("commons_infra.SaveResourceSnapshot: empty TaskID")
	}
	if snapshot.ID == "" {
		// Time-ordered UUIDv7 so snapshot PKs sort chronologically,
		// matching the sampled_at read order (google/uuid v1.6+ ships V7
		// native). Falls back to a random V4 if V7 generation ever errors.
		if v7, err := uuid.NewV7(); err == nil {
			snapshot.ID = v7.String()
		} else {
			snapshot.ID = uuid.NewString()
		}
	}
	if snapshot.SampledAt.IsZero() {
		snapshot.SampledAt = time.Now().UTC()
	}
	const sql = `
		INSERT INTO task_resource_snapshots (
			id, task_id,
			cpu_percent, cpu_user_time, cpu_system_time,
			memory_rss_bytes, memory_vms_bytes, memory_percent,
			io_read_bytes, io_write_bytes, io_read_count, io_write_count,
			net_bytes_sent, net_bytes_recv, net_connections,
			open_files, open_fds,
			process_state, thread_count,
			sampled_at
		) VALUES (
			$1, $2,
			$3, $4, $5,
			$6, $7, $8,
			$9, $10, $11, $12,
			$13, $14, $15,
			$16, $17,
			$18, $19,
			$20
		)
	`
	_, err := r.database.Exec(ctx, sql,
		snapshot.ID, snapshot.TaskID,
		snapshot.CPUPercent, snapshot.CPUUserTime, snapshot.CPUSystemTime,
		snapshot.MemoryRSSBytes, snapshot.MemoryVMSBytes, snapshot.MemoryPercent,
		snapshot.IOReadBytes, snapshot.IOWriteBytes, snapshot.IOReadCount, snapshot.IOWriteCount,
		snapshot.NetBytesSent, snapshot.NetBytesRecv, snapshot.NetConnections,
		snapshot.OpenFiles, snapshot.OpenFDs,
		snapshot.ProcessState, snapshot.ThreadCount,
		snapshot.SampledAt,
	)
	if err != nil {
		return fmt.Errorf("commons_infra.SaveResourceSnapshot: insert: %w", err)
	}
	return nil
}

// GetResourceSnapshots returns up to limit resource samples for the task,
// reverse-chronological (newest sample first) — the time-series read for
// the stuck-detector + resource-monitor UI. limit <= 0 is rejected.
func (r *pgxTaskRepository) GetResourceSnapshots(ctx context.Context, taskID string, limit int) ([]*models.ResourceSnapshot, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("commons_infra.GetResourceSnapshots: limit must be > 0, got %d", limit)
	}
	const sql = `
		SELECT
			id, task_id,
			cpu_percent, cpu_user_time, cpu_system_time,
			memory_rss_bytes, memory_vms_bytes, memory_percent,
			io_read_bytes, io_write_bytes, io_read_count, io_write_count,
			net_bytes_sent, net_bytes_recv, net_connections,
			open_files, open_fds,
			process_state, thread_count,
			sampled_at
		FROM task_resource_snapshots
		WHERE task_id = $1
		ORDER BY sampled_at DESC
		LIMIT $2
	`
	rows, err := r.database.Query(ctx, sql, taskID, limit)
	if err != nil {
		return nil, fmt.Errorf("commons_infra.GetResourceSnapshots: query: %w", err)
	}
	defer rows.Close()
	out := make([]*models.ResourceSnapshot, 0)
	for rows.Next() {
		var s models.ResourceSnapshot
		if err := rows.Scan(
			&s.ID, &s.TaskID,
			&s.CPUPercent, &s.CPUUserTime, &s.CPUSystemTime,
			&s.MemoryRSSBytes, &s.MemoryVMSBytes, &s.MemoryPercent,
			&s.IOReadBytes, &s.IOWriteBytes, &s.IOReadCount, &s.IOWriteCount,
			&s.NetBytesSent, &s.NetBytesRecv, &s.NetConnections,
			&s.OpenFiles, &s.OpenFDs,
			&s.ProcessState, &s.ThreadCount,
			&s.SampledAt,
		); err != nil {
			return nil, fmt.Errorf("commons_infra.GetResourceSnapshots: scan: %w", err)
		}
		out = append(out, &s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("commons_infra.GetResourceSnapshots: iterate: %w", err)
	}
	return out, nil
}

// --- HRD-090 — failure path. Still open (out of Batch D scope). ---

// MoveToDeadLetter — failure path. HRD-090. Intentionally still
// ErrUnsupported: the dead-letter table (and its retention/reprocess
// semantics per models.DeadLetterTask) is a separate migration + design
// effort tracked under HRD-090, NOT part of v1.0.0 Batch D (HRD-085..089).
// Returning ErrUnsupported keeps the gap loud per §107 rather than
// silently no-op'ing a failed task into oblivion.
func (r *pgxTaskRepository) MoveToDeadLetter(ctx context.Context, taskID, reason string) error {
	return fmt.Errorf("MoveToDeadLetter: %w (HRD-090)", ErrUnsupported)
}

// errIfNoRows converts a zero-RowsAffected Exec result into an explicit
// not-found error so a write against a missing id fails loudly rather than
// reporting a silent §107 success-bluff. If RowsAffected is unsupported by
// the driver (returns an error), we treat the write as having landed —
// the alternative (failing every write) would be worse.
func errIfNoRows(res db.Result, op, id string) error {
	n, err := res.RowsAffected()
	if err != nil {
		// Driver doesn't report affected rows — cannot prove not-found;
		// assume success rather than fail a write that may have landed.
		return nil
	}
	if n == 0 {
		return fmt.Errorf("commons_infra.%s: no task with id %q (not found)", op, id)
	}
	return nil
}

// validTaskStatus reports whether s is one of the known models.TaskStatus
// enum values (submodules/Models/background_task.go). Guards UpdateStatus +
// GetByStatus against persisting / querying a garbage status string that
// would silently corrupt the row or match nothing.
func validTaskStatus(s models.TaskStatus) bool {
	switch s {
	case models.TaskStatusPending, models.TaskStatusQueued, models.TaskStatusRunning,
		models.TaskStatusPaused, models.TaskStatusCompleted, models.TaskStatusFailed,
		models.TaskStatusStuck, models.TaskStatusCancelled, models.TaskStatusDeadLetter:
		return true
	default:
		return false
	}
}

// errNoRows is the sentinel a scan returns when QueryRow finds no row.
// We use a Herald-local sentinel so the SQL-driver-specific "no rows"
// (pgx.ErrNoRows) does not leak into the upstream queue logic.
var errNoRows = errors.New("commons_infra: no rows")

// scanTask reads a 36-column BackgroundTask row from the given Row.
// Column order MUST match the SELECT/RETURNING list in Dequeue (and any
// future single-row reader). Field types match models.BackgroundTask
// declared in submodules/Models/background_task.go.
func scanTask(row db.Row) (*models.BackgroundTask, error) {
	var t models.BackgroundTask
	var (
		priority string
		status   string
		payload  []byte
		config   []byte
		errHist  []byte
		checkPt  []byte
		notifCfg []byte
		tags     []byte
		metadata []byte
	)
	if err := row.Scan(
		&t.ID, &t.TaskType, &t.TaskName, &t.CorrelationID, &t.ParentTaskID,
		&payload, &config, &priority,
		&status, &t.Progress, &t.ProgressMessage, &checkPt,
		&t.MaxRetries, &t.RetryCount, &t.RetryDelaySeconds, &t.LastError, &errHist,
		&t.WorkerID, &t.ProcessPID, &t.StartedAt, &t.CompletedAt, &t.LastHeartbeat, &t.Deadline,
		&t.RequiredCPUCores, &t.RequiredMemoryMB, &t.EstimatedDurationSeconds, &t.ActualDurationSeconds,
		&notifCfg,
		&t.UserID, &t.SessionID, &tags, &metadata,
		&t.CreatedAt, &t.UpdatedAt, &t.ScheduledAt, &t.DeletedAt,
	); err != nil {
		// Bridge pgx-driver-specific no-rows to a Herald-local sentinel.
		// The error message contains "no rows" in standard pgx; the upstream
		// queue treats nil-task + nil-error as "queue empty".
		// TODO: replace substring-match with a proper ErrNoRows sentinel once
		// digital.vasic.database exposes one (follow-up HRD).
		if strings.Contains(err.Error(), "no rows") {
			return nil, errNoRows
		}
		return nil, fmt.Errorf("scanTask: %w", err)
	}
	t.Priority = models.TaskPriority(priority)
	t.Status = models.TaskStatus(status)
	t.Payload = payload
	if len(config) > 0 {
		if err := json.Unmarshal(config, &t.Config); err != nil {
			return nil, fmt.Errorf("scanTask: unmarshal config: %w", err)
		}
	}
	t.ErrorHistory = errHist
	t.Checkpoint = checkPt
	if len(notifCfg) > 0 {
		if err := json.Unmarshal(notifCfg, &t.NotificationConfig); err != nil {
			return nil, fmt.Errorf("scanTask: unmarshal notification_config: %w", err)
		}
	}
	t.Tags = tags
	t.Metadata = metadata
	return &t, nil
}
