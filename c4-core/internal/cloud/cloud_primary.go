package cloud

import (
	"fmt"
	"os"

	"github.com/changmin/c4-core/internal/store"
)

// Compile-time interface check.
var _ store.Store = (*CloudPrimaryStore)(nil)

// CloudPrimaryStore implements store.Store with cloud-primary reads/writes.
// Reads come from the cloud store first; on cloud failure it falls back to local.
// Writes go to the cloud first; local is updated only on cloud success.
//
// This is the inverse of HybridStore: cloud is the authoritative source of truth.
// Use when multiple machines share a project via Supabase and cloud consistency is preferred.
type CloudPrimaryStore struct {
	local  store.Store
	remote store.Store
}

// NewCloudPrimaryStore creates a CloudPrimaryStore wrapping local and remote stores.
func NewCloudPrimaryStore(local, remote store.Store) *CloudPrimaryStore {
	return &CloudPrimaryStore{
		local:  local,
		remote: remote,
	}
}

// asyncLocal runs a local sync operation in a goroutine. Failures are logged
// but never propagated — cloud is the source of truth in cloud-primary mode.
func (c *CloudPrimaryStore) asyncLocal(op string, fn func() error) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "c4: local %s panic (non-fatal): %v\n", op, r)
			}
		}()
		if err := fn(); err != nil {
			fmt.Fprintf(os.Stderr, "c4: local %s failed (non-fatal): %v\n", op, err)
		}
	}()
}

// =========================================================================
// Read operations — cloud first, local fallback
// =========================================================================

// GetStatus reads from cloud; falls back to local on error.
func (c *CloudPrimaryStore) GetStatus() (*store.ProjectStatus, error) {
	if status, err := c.remote.GetStatus(); err == nil {
		return status, nil
	}
	return c.local.GetStatus()
}

// GetTask reads from cloud; falls back to local on error.
func (c *CloudPrimaryStore) GetTask(taskID string) (*store.Task, error) {
	if task, err := c.remote.GetTask(taskID); err == nil {
		return task, nil
	}
	return c.local.GetTask(taskID)
}

// ListTasks reads from cloud; falls back to local on error.
func (c *CloudPrimaryStore) ListTasks(filter store.TaskFilter) ([]store.Task, int, error) {
	if tasks, total, err := c.remote.ListTasks(filter); err == nil {
		return tasks, total, nil
	}
	return c.local.ListTasks(filter)
}

// DeleteTask removes from cloud first; async delete on local.
func (c *CloudPrimaryStore) DeleteTask(taskID string) error {
	if err := c.remote.DeleteTask(taskID); err != nil {
		return err
	}
	c.asyncLocal("delete_task", func() error {
		return c.local.DeleteTask(taskID)
	})
	return nil
}

// =========================================================================
// Write operations — cloud first, then async local
// =========================================================================

// Start transitions cloud first and syncs to local.
func (c *CloudPrimaryStore) Start() error {
	if err := c.remote.Start(); err != nil {
		return err
	}
	c.asyncLocal("start", func() error {
		return c.local.Start()
	})
	return nil
}

// Clear resets cloud first and syncs to local.
func (c *CloudPrimaryStore) Clear(keepConfig bool) error {
	if err := c.remote.Clear(keepConfig); err != nil {
		return err
	}
	c.asyncLocal("clear", func() error {
		return c.local.Clear(keepConfig)
	})
	return nil
}

// TransitionState transitions cloud first and syncs to local.
func (c *CloudPrimaryStore) TransitionState(from, to string) error {
	if err := c.remote.TransitionState(from, to); err != nil {
		return err
	}
	c.asyncLocal("transition_state", func() error {
		return c.local.TransitionState(from, to)
	})
	return nil
}

// AddTask adds to cloud first and syncs to local.
func (c *CloudPrimaryStore) AddTask(task *store.Task) error {
	if err := c.remote.AddTask(task); err != nil {
		return err
	}
	c.asyncLocal("add_task", func() error {
		return c.local.AddTask(task)
	})
	return nil
}

// AssignTask assigns via cloud first and syncs to local.
func (c *CloudPrimaryStore) AssignTask(workerID string) (*store.TaskAssignment, error) {
	assignment, err := c.remote.AssignTask(workerID)
	if err != nil {
		return nil, err
	}
	if assignment != nil {
		c.asyncLocal("assign_task", func() error {
			_, localErr := c.local.AssignTask(workerID)
			return localErr
		})
	}
	return assignment, nil
}

// SubmitTask submits via cloud first and syncs to local.
func (c *CloudPrimaryStore) SubmitTask(taskID, workerID, commitSHA, handoff string, results []store.ValidationResult) (*store.SubmitResult, error) {
	result, err := c.remote.SubmitTask(taskID, workerID, commitSHA, handoff, results)
	if err != nil {
		return nil, err
	}
	c.asyncLocal("submit_task", func() error {
		_, localErr := c.local.SubmitTask(taskID, workerID, commitSHA, handoff, results)
		return localErr
	})
	return result, nil
}

// MarkBlocked marks via cloud first and syncs to local.
func (c *CloudPrimaryStore) MarkBlocked(taskID, workerID, failureSignature string, attempts int, lastError string) error {
	if err := c.remote.MarkBlocked(taskID, workerID, failureSignature, attempts, lastError); err != nil {
		return err
	}
	c.asyncLocal("mark_blocked", func() error {
		return c.local.MarkBlocked(taskID, workerID, failureSignature, attempts, lastError)
	})
	return nil
}

// ClaimTask claims via cloud first and syncs to local.
func (c *CloudPrimaryStore) ClaimTask(taskID string) (*store.Task, error) {
	task, err := c.remote.ClaimTask(taskID)
	if err != nil {
		return nil, err
	}
	c.asyncLocal("claim_task", func() error {
		_, localErr := c.local.ClaimTask(taskID)
		return localErr
	})
	return task, nil
}

// ReportTask reports via cloud first and syncs to local.
func (c *CloudPrimaryStore) ReportTask(taskID, summary string, filesChanged []string) error {
	if err := c.remote.ReportTask(taskID, summary, filesChanged); err != nil {
		return err
	}
	c.asyncLocal("report_task", func() error {
		return c.local.ReportTask(taskID, summary, filesChanged)
	})
	return nil
}

// Checkpoint records via cloud first and syncs to local.
func (c *CloudPrimaryStore) Checkpoint(checkpointID, decision, notes string, requiredChanges []string, targetTaskID, targetReviewID string) (*store.CheckpointResult, error) {
	result, err := c.remote.Checkpoint(checkpointID, decision, notes, requiredChanges, targetTaskID, targetReviewID)
	if err != nil {
		return nil, err
	}
	c.asyncLocal("checkpoint", func() error {
		_, localErr := c.local.Checkpoint(checkpointID, decision, notes, requiredChanges, targetTaskID, targetReviewID)
		return localErr
	})
	return result, nil
}

// RequestChanges processes via cloud first and syncs to local.
func (c *CloudPrimaryStore) RequestChanges(reviewTaskID string, comments string, requiredChanges []string) (*store.RequestChangesResult, error) {
	result, err := c.remote.RequestChanges(reviewTaskID, comments, requiredChanges)
	if err != nil {
		return nil, err
	}
	c.asyncLocal("request_changes", func() error {
		_, localErr := c.local.RequestChanges(reviewTaskID, comments, requiredChanges)
		return localErr
	})
	return result, nil
}
