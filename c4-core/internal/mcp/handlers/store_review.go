package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/task"
)

// completeReviewTask marks the paired R-task as done when a T-task completes.
// Returns the review task ID if cascaded, empty string otherwise.
// Best-effort: errors are logged but don't block task completion.
func (s *SQLiteStore) completeReviewTask(taskID string) string {
	if !strings.HasPrefix(taskID, "T-") {
		return ""
	}
	reviewID := "R-" + strings.TrimPrefix(taskID, "T-")
	result, err := s.db.Exec(
		`UPDATE c4_tasks SET status='done', worker_id='auto-cascade', updated_at=CURRENT_TIMESTAMP
		 WHERE task_id=? AND status IN ('pending','in_progress')`,
		reviewID,
	)
	if err != nil {
		return ""
	}
	if n, _ := result.RowsAffected(); n > 0 {
		s.logTrace("review_cascade", "system", reviewID,
			fmt.Sprintf("Auto-completed review for %s", taskID))
		return reviewID
	}
	return ""
}

// Checkpoint records a checkpoint decision.
func (s *SQLiteStore) Checkpoint(checkpointID, decision, notes string, requiredChanges []string) (*CheckpointResult, error) {
	changesJSON := "[]"
	if len(requiredChanges) > 0 {
		b, _ := json.Marshal(requiredChanges)
		changesJSON = string(b)
	}

	if _, err := s.db.Exec(`
		INSERT OR REPLACE INTO c4_checkpoints (checkpoint_id, decision, notes, required_changes, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		checkpointID, decision, notes, changesJSON, time.Now().Format(time.RFC3339),
	); err != nil {
		fmt.Fprintf(os.Stderr, "c4: checkpoint INSERT %s: %v\n", checkpointID, err)
	}

	result := &CheckpointResult{
		Success: true,
		Message: fmt.Sprintf("Checkpoint %s: %s", checkpointID, decision),
	}

	switch decision {
	case "APPROVE":
		result.NextAction = "continue"
	case "REQUEST_CHANGES":
		result.NextAction = "apply_changes"
		// Best-effort: record rejection for most recent active task
		var recentTaskID, recentWorkerID string
		if err := s.db.QueryRow(`SELECT task_id, worker_id FROM c4_tasks
			WHERE status='in_progress' ORDER BY updated_at DESC LIMIT 1`).Scan(&recentTaskID, &recentWorkerID); err != nil {
			fmt.Fprintf(os.Stderr, "c4: checkpoint: recent task lookup: %v\n", err)
		}
		if recentTaskID != "" {
			if recentWorkerID == "" {
				recentWorkerID = "direct"
			}
			s.recordPersonaStat(recentWorkerID, recentTaskID, "rejected")
		}
	case "REPLAN":
		result.NextAction = "replan"
	}

	return result, nil
}

// RequestChanges rejects a review task and creates the next version T+R pair.
func (s *SQLiteStore) RequestChanges(reviewTaskID string, comments string, requiredChanges []string) (*RequestChangesResult, error) {
	// 1. Parse review task ID
	_, baseID, version, taskType := task.ParseTaskID(reviewTaskID)
	if taskType != task.TypeReview {
		return nil, fmt.Errorf("%s is not a review task (got type %s)", reviewTaskID, taskType)
	}

	// 2. Check max_revision
	nextVersion := version + 1
	if s.config != nil {
		cfg := s.config.GetConfig()
		if cfg.MaxRevision > 0 && nextVersion >= cfg.MaxRevision {
			return nil, fmt.Errorf("max revision %d reached for base %s", cfg.MaxRevision, baseID)
		}
	}

	// 3. Mark current R task as done with REQUEST_CHANGES result
	_, err := s.db.Exec(`UPDATE c4_tasks SET status='done', commit_sha=?, updated_at=CURRENT_TIMESTAMP WHERE task_id=?`,
		"REQUEST_CHANGES: "+comments, reviewTaskID)
	if err != nil {
		return nil, fmt.Errorf("updating review task: %w", err)
	}

	// 4. Look up parent T's DoD and record rejection
	parentTaskID := fmt.Sprintf("T-%s-%d", baseID, version)
	var originalDoD string
	if err := s.db.QueryRow("SELECT dod FROM c4_tasks WHERE task_id=?", parentTaskID).Scan(&originalDoD); err != nil {
		fmt.Fprintf(os.Stderr, "c4: request-changes: original DoD lookup: %v\n", err)
	}

	// Record rejection for parent T-task's worker
	var parentWorkerID string
	if err := s.db.QueryRow("SELECT worker_id FROM c4_tasks WHERE task_id=?", parentTaskID).Scan(&parentWorkerID); err != nil {
		fmt.Fprintf(os.Stderr, "c4: request-changes: parent worker lookup: %v\n", err)
	}
	if parentWorkerID == "" {
		parentWorkerID = "direct"
	}
	s.recordPersonaStat(parentWorkerID, parentTaskID, "rejected")
	s.autoLearn(parentWorkerID)

	// 5. Create next version T + R
	changesText := strings.Join(requiredChanges, "\n- ")
	newDoD := fmt.Sprintf("Changes requested:\n- %s\n\nOriginal DoD:\n%s", changesText, originalDoD)

	nextTaskID := task.NextVersionID("T", baseID, version)
	nextReviewID := task.ReviewID(baseID, nextVersion)

	// T-XXX-(N+1) — fix task
	if err := s.AddTask(&Task{
		ID:           nextTaskID,
		Title:        fmt.Sprintf("Fix: %s", parentTaskID),
		DoD:          newDoD,
		Status:       "pending",
		Dependencies: []string{reviewTaskID},
		Priority:     10,
	}); err != nil {
		return nil, fmt.Errorf("creating fix task %s: %w", nextTaskID, err)
	}

	// R-XXX-(N+1) — review of fix
	if err := s.AddTask(&Task{
		ID:           nextReviewID,
		Title:        fmt.Sprintf("Review: %s", nextTaskID),
		DoD:          BuildReviewDoD(nextTaskID, fmt.Sprintf("Fix requested changes for %s:\n- %s", parentTaskID, changesText), 0),
		Status:       "pending",
		Dependencies: []string{nextTaskID},
	}); err != nil {
		return nil, fmt.Errorf("creating review task %s: %w", nextReviewID, err)
	}

	return &RequestChangesResult{
		Success:      true,
		NextTaskID:   nextTaskID,
		NextReviewID: nextReviewID,
		Version:      nextVersion,
		Message:      fmt.Sprintf("Created %s + %s (v%d)", nextTaskID, nextReviewID, nextVersion),
	}, nil
}
