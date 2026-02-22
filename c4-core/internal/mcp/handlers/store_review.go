package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/task"
)

// completeReviewTask returns the paired R-task ID if it exists as pending,
// leaving it for a real review Worker to process (no auto-cascade).
// Uses ParseTaskID + ReviewID so the review ID is collision-safe and consistent with
// validated task ID grammar; non-conforming taskID fails fast (ValidateTaskID) and is skipped.
// Best-effort: DB errors don't block task completion.
func (s *SQLiteStore) completeReviewTask(taskID string) string {
	if task.ValidateTaskID(taskID) != nil {
		return ""
	}
	_, baseID, version, taskType := task.ParseTaskID(taskID)
	if taskType != task.TypeImplementation {
		return ""
	}
	reviewID := task.ReviewID(baseID, version)
	var status string
	if err := s.db.QueryRow(`SELECT status FROM c4_tasks WHERE task_id=?`, reviewID).Scan(&status); err != nil {
		return ""
	}
	if status == "pending" || status == "in_progress" {
		s.logTrace("review_pending", "system", reviewID,
			fmt.Sprintf("Review task %s awaiting worker for %s", reviewID, taskID))
		return reviewID
	}
	return ""
}

func (s *SQLiteStore) resolveCheckpointTargets(checkpointID string) (string, string, error) {
	var depsRaw string
	if err := s.db.QueryRow("SELECT dependencies FROM c4_tasks WHERE task_id=?", checkpointID).Scan(&depsRaw); err != nil {
		if err == sql.ErrNoRows {
			return "", "", nil
		}
		return "", "", err
	}

	depsRaw = strings.TrimSpace(depsRaw)
	if depsRaw == "" {
		return "", "", nil
	}

	deps := make([]string, 0)
	if err := json.Unmarshal([]byte(depsRaw), &deps); err != nil {
		for _, dep := range strings.Split(depsRaw, ",") {
			dep = strings.Trim(dep, " \t\r\n\"[]")
			if dep != "" {
				deps = append(deps, dep)
			}
		}
	}

	targetTaskID := ""
	targetReviewID := ""
	for _, dep := range deps {
		dep = strings.Trim(dep, " \t\r\n\"[]")
		if dep == "" {
			continue
		}
		switch {
		case strings.HasPrefix(dep, "R-"):
			targetReviewID = dep
			if targetTaskID == "" {
				targetTaskID = "T-" + strings.TrimPrefix(dep, "R-")
			}
		case strings.HasPrefix(dep, "T-"):
			if targetTaskID == "" {
				targetTaskID = dep
			}
		}
		if targetTaskID != "" && targetReviewID != "" {
			break
		}
	}
	return targetTaskID, targetReviewID, nil
}

func normalizeRequiredChanges(requiredChanges []string) []string {
	seen := make(map[string]struct{}, len(requiredChanges))
	normalized := make([]string, 0, len(requiredChanges))
	for _, change := range requiredChanges {
		change = strings.TrimSpace(change)
		if change == "" {
			continue
		}
		if _, exists := seen[change]; exists {
			continue
		}
		seen[change] = struct{}{}
		normalized = append(normalized, change)
	}
	return normalized
}

// Checkpoint records a checkpoint decision.
// targetTaskID/targetReviewID: when both non-empty, use for attribution and persistence (explicit linkage). Never use "latest in_progress" heuristic.
func (s *SQLiteStore) Checkpoint(checkpointID, decision, notes string, requiredChanges []string, targetTaskID, targetReviewID string) (*CheckpointResult, error) {
	changesJSON := "[]"
	if len(requiredChanges) > 0 {
		b, _ := json.Marshal(requiredChanges)
		changesJSON = string(b)
	}

	if targetTaskID == "" || targetReviewID == "" {
		var err error
		targetTaskID, targetReviewID, err = s.resolveCheckpointTargets(checkpointID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "c4: checkpoint target lookup %s: %v\n", checkpointID, err)
		}
	}

	if _, err := s.db.Exec(`
		INSERT OR REPLACE INTO c4_checkpoints (checkpoint_id, decision, notes, required_changes, target_task_id, target_review_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		checkpointID, decision, notes, changesJSON, targetTaskID, targetReviewID, time.Now().Format(time.RFC3339),
	); err != nil {
		return nil, fmt.Errorf("checkpoint INSERT %s: %w", checkpointID, err)
	}

	// Publish checkpoint event
	switch decision {
	case "APPROVE", "APPROVE_FINAL":
		s.notifyEventBus("checkpoint.approved", map[string]any{
			"checkpoint_id": checkpointID, "decision": "APPROVE", "notes": notes,
		})
	case "REQUEST_CHANGES", "REPLAN":
		s.notifyEventBus("checkpoint.rejected", map[string]any{
			"checkpoint_id": checkpointID, "decision": decision, "notes": notes,
			"required_changes": requiredChanges, "target_task_id": targetTaskID, "target_review_id": targetReviewID,
		})
	}

	result := &CheckpointResult{
		Success: true,
		Message: fmt.Sprintf("Checkpoint %s: %s", checkpointID, decision),
	}

	switch decision {
	case "APPROVE", "APPROVE_FINAL":
		result.NextAction = "continue"
	case "REQUEST_CHANGES":
		result.NextAction = "apply_changes"
		if targetTaskID != "" {
			var targetWorkerID string
			if err := s.db.QueryRow("SELECT worker_id FROM c4_tasks WHERE task_id=?", targetTaskID).Scan(&targetWorkerID); err != nil {
				fmt.Fprintf(os.Stderr, "c4: checkpoint: target worker lookup %s: %v\n", targetTaskID, err)
			} else {
				if targetWorkerID == "" {
					targetWorkerID = "direct"
				}
				s.recordPersonaStat(targetWorkerID, targetTaskID, "rejected")
			}
		}
	case "REPLAN":
		result.NextAction = "replan"
	}

	return result, nil
}

// RequestChanges rejects a review task and creates the next version T+R pair.
func (s *SQLiteStore) RequestChanges(reviewTaskID string, comments string, requiredChanges []string) (*RequestChangesResult, error) {
	requiredChanges = normalizeRequiredChanges(requiredChanges)
	if len(requiredChanges) == 0 {
		return nil, fmt.Errorf("required_changes must contain at least one non-empty item")
	}

	// 1. Validate and parse review task ID (fail-fast for non-conforming IDs)
	if err := task.ValidateTaskID(reviewTaskID); err != nil {
		return nil, fmt.Errorf("invalid review task ID %q: %w", reviewTaskID, err)
	}
	_, baseID, version, taskType := task.ParseTaskID(reviewTaskID)
	if taskType != task.TypeReview {
		return nil, fmt.Errorf("%s is not a review task (got type %s)", reviewTaskID, taskType)
	}
	normalizedReviewID := task.ReviewID(baseID, version)

	// 2. Check max_revision
	// max_revision is the maximum allowed REQUEST_CHANGES count.
	// Policy: max_revision=N allows exactly N implementation attempts (versions 0..N-1).
	// Block when nextVersion >= MaxRevision: e.g. max_revision=3 allows versions 0,1,2 and blocks at version 3.
	nextVersion := version + 1
	if s.config != nil {
		cfg := s.config.GetConfig()
		if cfg.MaxRevision > 0 && nextVersion >= cfg.MaxRevision {
			return nil, fmt.Errorf("max revision %d exceeded for base %s", cfg.MaxRevision, baseID)
		}
	}

	// 3. Mark current R task as done with REQUEST_CHANGES result (reason in dedicated field, not commit_sha)
	_, err := s.db.Exec(`UPDATE c4_tasks SET status='done', review_decision_evidence=?, commit_sha='', updated_at=CURRENT_TIMESTAMP WHERE task_id=?`,
		comments, normalizedReviewID)
	if err != nil {
		return nil, fmt.Errorf("updating review task: %w", err)
	}

	// 4. Look up parent T's DoD, scope, and record rejection
	parentTaskID := fmt.Sprintf("T-%s-%d", baseID, version)
	var originalDoD string
	if err := s.db.QueryRow("SELECT dod FROM c4_tasks WHERE task_id=?", parentTaskID).Scan(&originalDoD); err != nil {
		fmt.Fprintf(os.Stderr, "c4: request-changes: original DoD lookup: %v\n", err)
	}

	var parentScope string
	if err := s.db.QueryRow("SELECT scope FROM c4_tasks WHERE task_id=?", parentTaskID).Scan(&parentScope); err != nil {
		fmt.Fprintf(os.Stderr, "c4: request-changes: scope lookup: %v\n", err)
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

	// Append past solutions from knowledge (best-effort, 2s timeout)
	if pastSols := searchPastSolutions(s.knowledgeSearch, s.knowledgeReader, comments, 3); len(pastSols) > 0 {
		newDoD += "\n\n## Past Solutions\n" + strings.Join(pastSols, "\n")
	}

	nextTaskID := task.NextVersionID("T", baseID, version)
	nextReviewID := task.ReviewID(baseID, nextVersion)

	// Mark old R-task as superseded (best-effort, non-fatal)
	if _, err2 := s.db.Exec(`UPDATE c4_tasks SET superseded_by=? WHERE task_id=?`,
		nextReviewID, normalizedReviewID); err2 != nil {
		fmt.Fprintf(os.Stderr, "c4: superseded_by update: %v\n", err2)
	}

	// T-XXX-(N+1) — fix task (inherits scope from parent T)
	if err := s.AddTask(&Task{
		ID:           nextTaskID,
		Scope:        parentScope,
		Title:        fmt.Sprintf("Fix: %s", parentTaskID),
		DoD:          newDoD,
		Status:       "pending",
		Dependencies: []string{normalizedReviewID},
		Priority:     10,
	}); err != nil {
		return nil, fmt.Errorf("creating fix task %s: %w", nextTaskID, err)
	}

	// R-XXX-(N+1) — review of fix (inherits scope from parent T)
	if err := s.AddTask(&Task{
		ID:           nextReviewID,
		Scope:        parentScope,
		Title:        fmt.Sprintf("Review: %s", nextTaskID),
		DoD:          BuildReviewDoD(nextTaskID, fmt.Sprintf("Fix requested changes for %s:\n- %s", parentTaskID, changesText), 0),
		Status:       "pending",
		Dependencies: []string{nextTaskID},
	}); err != nil {
		return nil, fmt.Errorf("creating review task %s: %w", nextReviewID, err)
	}

	// Publish review.changes_requested event
	s.notifyEventBus("review.changes_requested", map[string]any{
		"review_task_id": normalizedReviewID,
		"next_task_id":   nextTaskID,
		"next_review_id": nextReviewID,
		"version":        nextVersion,
		"comments":       comments,
	})

	return &RequestChangesResult{
		Success:      true,
		NextTaskID:   nextTaskID,
		NextReviewID: nextReviewID,
		Version:      nextVersion,
		Message:      fmt.Sprintf("Created %s + %s (v%d)", nextTaskID, nextReviewID, nextVersion),
	}, nil
}
