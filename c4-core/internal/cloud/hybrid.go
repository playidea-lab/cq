// Package cloud provides the HybridStore that wraps a local SQLiteStore
// and a remote CloudStore, implementing a local-first architecture.
//
// Writes go to SQLite immediately and are asynchronously pushed to the
// cloud. Reads always come from the local SQLite for speed and offline
// safety. Cloud failures are logged but never block local operations.
package cloud

import (
	"fmt"
	"os"
	"sync"

	"github.com/changmin/c4-core/internal/store"
)

// Compile-time interface check.
var _ store.Store = (*HybridStore)(nil)

// HybridStore implements store.Store with local-first writes.
// All reads come from the local store. All writes go to the local
// store first, then are asynchronously pushed to the cloud store.
type HybridStore struct {
	local  store.Store
	remote store.Store

	mu       sync.Mutex
	failures int // count of consecutive cloud failures (for monitoring)
}

// NewHybridStore creates a HybridStore wrapping local and remote stores.
func NewHybridStore(local, remote store.Store) *HybridStore {
	return &HybridStore{
		local:  local,
		remote: remote,
	}
}

// Local returns the underlying local store.
func (h *HybridStore) Local() store.Store {
	return h.local
}

// asyncCloud runs a cloud operation in a goroutine. Failures are logged
// but never propagated — the local store is the source of truth.
func (h *HybridStore) asyncCloud(op string, fn func() error) {
	go func() {
		if err := fn(); err != nil {
			h.mu.Lock()
			h.failures++
			h.mu.Unlock()
			fmt.Fprintf(os.Stderr, "c4: cloud %s failed (non-fatal): %v\n", op, err)
		} else {
			h.mu.Lock()
			h.failures = 0
			h.mu.Unlock()
		}
	}()
}

// CloudFailures returns the number of consecutive cloud failures.
func (h *HybridStore) CloudFailures() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.failures
}

// =========================================================================
// Read operations — always local
// =========================================================================

// GetStatus reads from local store.
func (h *HybridStore) GetStatus() (*store.ProjectStatus, error) {
	return h.local.GetStatus()
}

// GetTask reads from local store.
func (h *HybridStore) GetTask(taskID string) (*store.Task, error) {
	return h.local.GetTask(taskID)
}

// ListTasks reads from local store (declared backend; no hard SQLite coupling at call site).
func (h *HybridStore) ListTasks(filter store.TaskFilter) ([]store.Task, int, error) {
	return h.local.ListTasks(filter)
}

// DeleteTask removes from local; async delete on cloud.
func (h *HybridStore) DeleteTask(taskID string) error {
	if err := h.local.DeleteTask(taskID); err != nil {
		return err
	}
	h.asyncCloud("delete_task", func() error {
		return h.remote.DeleteTask(taskID)
	})
	return nil
}

// =========================================================================
// Write operations — local first, then async cloud
// =========================================================================

// Start transitions locally and pushes to cloud.
func (h *HybridStore) Start() error {
	if err := h.local.Start(); err != nil {
		return err
	}
	h.asyncCloud("start", func() error {
		return h.remote.Start()
	})
	return nil
}

// Clear resets locally and pushes to cloud.
func (h *HybridStore) Clear(keepConfig bool) error {
	if err := h.local.Clear(keepConfig); err != nil {
		return err
	}
	h.asyncCloud("clear", func() error {
		return h.remote.Clear(keepConfig)
	})
	return nil
}

// TransitionState transitions locally and pushes to cloud.
func (h *HybridStore) TransitionState(from, to string) error {
	if err := h.local.TransitionState(from, to); err != nil {
		return err
	}
	h.asyncCloud("transition_state", func() error {
		return h.remote.TransitionState(from, to)
	})
	return nil
}

// AddTask adds locally and pushes to cloud.
func (h *HybridStore) AddTask(task *store.Task) error {
	if err := h.local.AddTask(task); err != nil {
		return err
	}
	h.asyncCloud("add_task", func() error {
		return h.remote.AddTask(task)
	})
	return nil
}

// AssignTask assigns locally and pushes to cloud.
func (h *HybridStore) AssignTask(workerID string) (*store.TaskAssignment, error) {
	assignment, err := h.local.AssignTask(workerID)
	if err != nil {
		return nil, err
	}
	if assignment != nil {
		h.asyncCloud("assign_task", func() error {
			_, remoteErr := h.remote.AssignTask(workerID)
			return remoteErr
		})
	}
	return assignment, nil
}

// SubmitTask submits locally and pushes to cloud.
func (h *HybridStore) SubmitTask(taskID, workerID, commitSHA, handoff string, results []store.ValidationResult) (*store.SubmitResult, error) {
	result, err := h.local.SubmitTask(taskID, workerID, commitSHA, handoff, results)
	if err != nil {
		return nil, err
	}
	h.asyncCloud("submit_task", func() error {
		_, remoteErr := h.remote.SubmitTask(taskID, workerID, commitSHA, handoff, results)
		return remoteErr
	})
	return result, nil
}

// MarkBlocked marks locally and pushes to cloud.
func (h *HybridStore) MarkBlocked(taskID, workerID, failureSignature string, attempts int, lastError string) error {
	if err := h.local.MarkBlocked(taskID, workerID, failureSignature, attempts, lastError); err != nil {
		return err
	}
	h.asyncCloud("mark_blocked", func() error {
		return h.remote.MarkBlocked(taskID, workerID, failureSignature, attempts, lastError)
	})
	return nil
}

// ClaimTask claims locally and pushes to cloud.
func (h *HybridStore) ClaimTask(taskID string) (*store.Task, error) {
	task, err := h.local.ClaimTask(taskID)
	if err != nil {
		return nil, err
	}
	h.asyncCloud("claim_task", func() error {
		_, remoteErr := h.remote.ClaimTask(taskID)
		return remoteErr
	})
	return task, nil
}

// ReportTask reports locally and pushes to cloud.
func (h *HybridStore) ReportTask(taskID, summary string, filesChanged []string) error {
	if err := h.local.ReportTask(taskID, summary, filesChanged); err != nil {
		return err
	}
	h.asyncCloud("report_task", func() error {
		return h.remote.ReportTask(taskID, summary, filesChanged)
	})
	return nil
}

// Checkpoint records locally and pushes to cloud.
func (h *HybridStore) Checkpoint(checkpointID, decision, notes string, requiredChanges []string, targetTaskID, targetReviewID string) (*store.CheckpointResult, error) {
	result, err := h.local.Checkpoint(checkpointID, decision, notes, requiredChanges, targetTaskID, targetReviewID)
	if err != nil {
		return nil, err
	}
	h.asyncCloud("checkpoint", func() error {
		_, remoteErr := h.remote.Checkpoint(checkpointID, decision, notes, requiredChanges, targetTaskID, targetReviewID)
		return remoteErr
	})
	return result, nil
}

// RequestChanges processes locally and pushes to cloud.
func (h *HybridStore) RequestChanges(reviewTaskID string, comments string, requiredChanges []string) (*store.RequestChangesResult, error) {
	result, err := h.local.RequestChanges(reviewTaskID, comments, requiredChanges)
	if err != nil {
		return nil, err
	}
	h.asyncCloud("request_changes", func() error {
		_, remoteErr := h.remote.RequestChanges(reviewTaskID, comments, requiredChanges)
		return remoteErr
	})
	return result, nil
}
