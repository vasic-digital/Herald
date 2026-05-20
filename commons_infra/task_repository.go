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
	"time"

	db "digital.vasic.database/pkg/database"
	"digital.vasic.models"
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
	_, err = r.database.Exec(ctx, sql, taskID, eventType, payload, workerID)
	if err != nil {
		return fmt.Errorf("commons_infra.LogEvent: insert: %w", err)
	}
	return nil
}

// --- Methods not yet implemented. Each returns ErrUnsupported with a ---
// --- pointer to the HRD-NNN that will land the missing functionality ---
// --- per §107 (no silent zero-results) + §11.4.68 (positive evidence). ---

// GetByID — required for Requeue, MoveToDeadLetter. HRD-085 will implement.
func (r *pgxTaskRepository) GetByID(ctx context.Context, id string) (*models.BackgroundTask, error) {
	return nil, fmt.Errorf("GetByID: %w (HRD-085)", ErrUnsupported)
}

// Update — required for Requeue. HRD-085.
func (r *pgxTaskRepository) Update(ctx context.Context, task *models.BackgroundTask) error {
	return fmt.Errorf("Update: %w (HRD-085)", ErrUnsupported)
}

// Delete — operator-tool only. HRD-085.
func (r *pgxTaskRepository) Delete(ctx context.Context, id string) error {
	return fmt.Errorf("Delete: %w (HRD-085)", ErrUnsupported)
}

// UpdateStatus — worker-side. HRD-086.
func (r *pgxTaskRepository) UpdateStatus(ctx context.Context, id string, status models.TaskStatus) error {
	return fmt.Errorf("UpdateStatus: %w (HRD-086)", ErrUnsupported)
}

// UpdateProgress — worker-side. HRD-086.
func (r *pgxTaskRepository) UpdateProgress(ctx context.Context, id string, progress float64, message string) error {
	return fmt.Errorf("UpdateProgress: %w (HRD-086)", ErrUnsupported)
}

// UpdateHeartbeat — worker-side. HRD-086.
func (r *pgxTaskRepository) UpdateHeartbeat(ctx context.Context, id string) error {
	return fmt.Errorf("UpdateHeartbeat: %w (HRD-086)", ErrUnsupported)
}

// SaveCheckpoint — worker-side. HRD-086.
func (r *pgxTaskRepository) SaveCheckpoint(ctx context.Context, id string, checkpoint []byte) error {
	return fmt.Errorf("SaveCheckpoint: %w (HRD-086)", ErrUnsupported)
}

// GetByStatus — admin/stats. HRD-087.
func (r *pgxTaskRepository) GetByStatus(ctx context.Context, status models.TaskStatus, limit, offset int) ([]*models.BackgroundTask, error) {
	return nil, fmt.Errorf("GetByStatus: %w (HRD-087)", ErrUnsupported)
}

// GetPendingTasks — Peek delegate. HRD-087.
func (r *pgxTaskRepository) GetPendingTasks(ctx context.Context, limit int) ([]*models.BackgroundTask, error) {
	return nil, fmt.Errorf("GetPendingTasks: %w (HRD-087)", ErrUnsupported)
}

// GetStaleTasks — stuck-detector. HRD-088.
func (r *pgxTaskRepository) GetStaleTasks(ctx context.Context, threshold time.Duration) ([]*models.BackgroundTask, error) {
	return nil, fmt.Errorf("GetStaleTasks: %w (HRD-088)", ErrUnsupported)
}

// GetByWorkerID — worker-recovery. HRD-088.
func (r *pgxTaskRepository) GetByWorkerID(ctx context.Context, workerID string) ([]*models.BackgroundTask, error) {
	return nil, fmt.Errorf("GetByWorkerID: %w (HRD-088)", ErrUnsupported)
}

// CountByStatus — stats. HRD-087.
func (r *pgxTaskRepository) CountByStatus(ctx context.Context) (map[models.TaskStatus]int64, error) {
	return nil, fmt.Errorf("CountByStatus: %w (HRD-087)", ErrUnsupported)
}

// SaveResourceSnapshot — resource-monitor. HRD-089.
func (r *pgxTaskRepository) SaveResourceSnapshot(ctx context.Context, snapshot *models.ResourceSnapshot) error {
	return fmt.Errorf("SaveResourceSnapshot: %w (HRD-089)", ErrUnsupported)
}

// GetResourceSnapshots — resource-monitor. HRD-089.
func (r *pgxTaskRepository) GetResourceSnapshots(ctx context.Context, taskID string, limit int) ([]*models.ResourceSnapshot, error) {
	return nil, fmt.Errorf("GetResourceSnapshots: %w (HRD-089)", ErrUnsupported)
}

// GetTaskHistory — admin/audit. HRD-087.
func (r *pgxTaskRepository) GetTaskHistory(ctx context.Context, taskID string, limit int) ([]*models.TaskExecutionHistory, error) {
	return nil, fmt.Errorf("GetTaskHistory: %w (HRD-087)", ErrUnsupported)
}

// MoveToDeadLetter — failure path. HRD-090.
func (r *pgxTaskRepository) MoveToDeadLetter(ctx context.Context, taskID, reason string) error {
	return fmt.Errorf("MoveToDeadLetter: %w (HRD-090)", ErrUnsupported)
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
		if errMsgContains(err, "no rows") {
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

// errMsgContains is a tiny helper to bridge the pgx-driver-specific
// "no rows" error (pgx.ErrNoRows.Error() == "no rows in result set")
// without importing pgx directly here.
func errMsgContains(err error, substr string) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for i := 0; i+len(substr) <= len(msg); i++ {
		if msg[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
