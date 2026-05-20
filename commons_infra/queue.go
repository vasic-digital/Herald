// Package infra — task-queue seam.
//
// LONG-TERM INTENT (per §11.4.74 Catalogue-Check: extend, don't reimplement):
// As of HRD-010 Task 5 (2026-05-20), TaskQueue is a thin alias for
// `digital.vasic.background.TaskQueue` — the §11.4.74-compliant extension
// of the upstream Helix-stack queue. The Task 1 deferral (missing Models +
// Concurrency submodules) is RESOLVED: Task 5 vendored both submodules
// under `submodules/Models` and `submodules/Concurrency` so the transitive
// `digital.vasic.background` replace targets resolve cleanly.
//
// The Herald-local mirror types previously declared here (Task / TaskPriority
// / ResourceRequirements / TaskQueue interface) have been replaced with
// aliases pointing at the upstream. Any caller that referenced the mirror
// types continues to work — that was the whole point of the §11.4.74
// stub-with-matching-shape contract.
package infra

import (
	bg "digital.vasic.background"
	"digital.vasic.models"
)

// TaskPriority aliases digital.vasic.models.TaskPriority. The upstream
// declares a typed string with weighted enums (Critical / High / Normal /
// Low / Background). See submodules/Models/background_task.go for the
// full enum surface + Weight() method.
type TaskPriority = models.TaskPriority

// Task aliases digital.vasic.models.BackgroundTask. The upstream is the
// source-of-truth for fields, JSON tags, db tags, retry semantics, and
// the BackgroundTask constructor (models.NewBackgroundTask). See
// submodules/Models/background_task.go.
type Task = models.BackgroundTask

// ResourceRequirements aliases digital.vasic.background.ResourceRequirements
// (declared in submodules/background/interfaces.go). Used by Dequeue to
// filter eligible tasks by CPU/memory/disk/GPU + priority floor.
type ResourceRequirements = bg.ResourceRequirements

// TaskQueue aliases digital.vasic.background.TaskQueue. Upstream methods
// (Enqueue / Dequeue / Peek / Requeue / MoveToDeadLetter / GetPendingCount /
// GetRunningCount / GetQueueDepth) are inherited transparently — any drift
// at the upstream surface will trip a compile error here, which is the
// §11.4.74 in-place-extension contract.
type TaskQueue = bg.TaskQueue
