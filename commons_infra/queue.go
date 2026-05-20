// Package infra — task-queue seam.
//
// LONG-TERM INTENT (per §11.4.74 Catalogue-Check: extend, don't reimplement):
// TaskQueue MUST become a thin alias for digital.vasic.background.TaskQueue
// once Herald incorporates the `digital.vasic.models` submodule. The
// upstream interface declares:
//
//	type TaskQueue interface {
//	    Enqueue(ctx context.Context, task *models.BackgroundTask) error
//	    Dequeue(ctx context.Context, workerID string, requirements ResourceRequirements) (*models.BackgroundTask, error)
//	    Peek(ctx context.Context, count int) ([]*models.BackgroundTask, error)
//	    Requeue(ctx context.Context, taskID string, delay time.Duration) error
//	    MoveToDeadLetter(ctx context.Context, taskID string, reason string) error
//	    GetPendingCount(ctx context.Context) (int64, error)
//	    GetRunningCount(ctx context.Context) (int64, error)
//	    GetQueueDepth(ctx context.Context) (map[models.TaskPriority]int64, error)
//	}
//
// — see submodules/background/interfaces.go.
//
// HRD-010 TASK 1 DEFERRAL (DONE_WITH_CONCERNS):
//
//	digital.vasic.background depends on digital.vasic.models (via
//	`replace digital.vasic.models => ../Models` in its go.mod). The
//	`Models` submodule is NOT currently checked into Herald's tree.
//	Importing digital.vasic.background from commons_infra therefore
//	fails to resolve the transitive `digital.vasic.models` import.
//
//	Per the HRD-010 Task 1 plan: "If it turns out the type cannot be
//	cleanly aliased without crossing an internal/ boundary, defer the
//	Task = bg.BackgroundTask alias and just declare type TaskQueue =
//	bg.TaskQueue. Document the deferral as DONE_WITH_CONCERNS." The
//	missing `Models` submodule is the analogous blocker — not an
//	internal/ boundary, but a missing-replace-target boundary that
//	requires controller-level intervention to resolve (incorporate
//	the `Models` submodule into Herald's tree, add the replace
//	directives, then swap this interface for the alias).
//
//	Until then: the Herald-local interface below mirrors the upstream
//	shape EXACTLY (same method names, same parameter shapes modulo the
//	*models.BackgroundTask which becomes `any` here as a placeholder).
//	When the alias is restored, the compile will catch any drift in
//	the upstream API at this seam — which is the §11.4.74 contract.
//
// CONCERNS for the controller to investigate (raised at Task-1 boundary):
//
//	1. Should Herald incorporate `Models` (and `Concurrency`, which is
//	   the second transitive `digital.vasic.background` replace target)
//	   as submodules under Herald's tree before Task 5? If yes, Task 2
//	   is the right place to add them (it's the wiring-Up() task).
//
//	2. Alternative: extract the BackgroundTask + TaskPriority + queue-
//	   only interfaces into a smaller `digital.vasic.background/pkg/
//	   queue` package that does NOT depend on the full `models`
//	   surface. That's a §11.4.74 in-place-extension of the upstream
//	   submodule — preferred over Herald owning a duplicate type tree.

package infra

import (
	"context"
	"time"
)

// TaskPriority mirrors digital.vasic.models.TaskPriority. Kept as a typed
// string so the eventual alias is structurally identical to upstream.
//
// DEFERRED: becomes `type TaskPriority = models.TaskPriority` once the
// Models submodule is incorporated. See queue.go header for the deferral
// rationale (HRD-010 Task 1 DONE_WITH_CONCERNS).
type TaskPriority string

// Task mirrors digital.vasic.models.BackgroundTask. Kept as a placeholder
// struct so the Herald-local interface compiles; the alias swap (Task 5)
// will replace this with `type Task = models.BackgroundTask`.
//
// DEFERRED per §11.4.74: do NOT add fields here. The upstream type IS
// the source-of-truth. This stub exists ONLY to keep the seam compiling
// until the Models submodule lands; it is intentionally bare so any
// caller who tries to access fields fails loudly at compile time and
// gets routed to the upstream type.
type Task struct{}

// ResourceRequirements mirrors digital.vasic.background.ResourceRequirements.
// Same deferral rationale as Task above.
type ResourceRequirements struct {
	CPUCores int
	MemoryMB int
	DiskMB   int
	GPUCount int
	Priority TaskPriority
}

// TaskQueue is Herald's local mirror of digital.vasic.background.TaskQueue.
// The method set is structurally identical to the upstream interface so
// when the alias is restored (post-Models-submodule incorporation), no
// Herald caller needs to change.
//
// DEFERRED per HRD-010 Task 1: becomes `type TaskQueue = bg.TaskQueue`
// when the Models submodule is incorporated. See queue.go header.
type TaskQueue interface {
	Enqueue(ctx context.Context, task *Task) error
	Dequeue(ctx context.Context, workerID string, requirements ResourceRequirements) (*Task, error)
	Peek(ctx context.Context, count int) ([]*Task, error)
	Requeue(ctx context.Context, taskID string, delay time.Duration) error
	MoveToDeadLetter(ctx context.Context, taskID string, reason string) error
	GetPendingCount(ctx context.Context) (int64, error)
	GetRunningCount(ctx context.Context) (int64, error)
	GetQueueDepth(ctx context.Context) (map[TaskPriority]int64, error)
}
