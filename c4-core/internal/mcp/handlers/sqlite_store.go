package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// SQLiteStore implements the handlers.Store interface backed by SQLite.
// It operates on the shared .c4/tasks.db used by both Go and Python.
type SQLiteStore struct {
	db        *sql.DB
	projectID string
}

// NewSQLiteStore creates a new SQLite-backed Store.
func NewSQLiteStore(db *sql.DB) (*SQLiteStore, error) {
	s := &SQLiteStore{db: db}

	// Read project ID from state (table may not exist yet)
	var stateJSON string
	err := db.QueryRow("SELECT state_json FROM c4_state LIMIT 1").Scan(&stateJSON)
	if err == nil {
		var m map[string]any
		if jsonErr := json.Unmarshal([]byte(stateJSON), &m); jsonErr == nil {
			if pid, ok := m["project_id"].(string); ok {
				s.projectID = pid
			}
		}
	}
	// Ignore errors — table may not exist yet and that's OK

	return s, nil
}

// GetStatus returns the current project status with task counts.
func (s *SQLiteStore) GetStatus() (*ProjectStatus, error) {
	status := &ProjectStatus{State: "INIT", ProjectName: s.projectID}

	// Read state
	var stateJSON string
	err := s.db.QueryRow("SELECT state_json FROM c4_state LIMIT 1").Scan(&stateJSON)
	if err == nil {
		var m map[string]any
		if jsonErr := json.Unmarshal([]byte(stateJSON), &m); jsonErr == nil {
			if st, ok := m["status"].(string); ok {
				status.State = st
			}
			if pn, ok := m["project_id"].(string); ok {
				status.ProjectName = pn
			}
		}
	}

	// Count tasks by status
	rows, err := s.db.Query("SELECT status, COUNT(*) FROM c4_tasks GROUP BY status")
	if err != nil {
		// Table might not exist yet
		return status, nil
	}
	defer rows.Close()

	for rows.Next() {
		var st string
		var count int
		if err := rows.Scan(&st, &count); err != nil {
			continue
		}
		status.TotalTasks += count
		switch st {
		case "pending":
			status.PendingTasks = count
		case "in_progress":
			status.InProgress = count
		case "done":
			status.DoneTasks = count
		case "blocked":
			status.BlockedTasks = count
		}
	}

	return status, nil
}

// Start transitions the project to EXECUTE state.
func (s *SQLiteStore) Start() error {
	var stateJSON string
	err := s.db.QueryRow("SELECT state_json FROM c4_state LIMIT 1").Scan(&stateJSON)
	if err != nil {
		return fmt.Errorf("reading state: %w", err)
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(stateJSON), &m); err != nil {
		return fmt.Errorf("parsing state: %w", err)
	}

	current, _ := m["status"].(string)
	if current != "PLAN" && current != "HALTED" {
		return fmt.Errorf("cannot start from state %s (must be PLAN or HALTED)", current)
	}

	m["status"] = "EXECUTE"
	updated, _ := json.Marshal(m)
	pid, _ := m["project_id"].(string)

	_, err = s.db.Exec(
		"UPDATE c4_state SET state_json = ?, updated_at = CURRENT_TIMESTAMP WHERE project_id = ?",
		string(updated), pid,
	)
	return err
}

// Clear resets the C4 state.
func (s *SQLiteStore) Clear(keepConfig bool) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete all tasks
	if _, err := tx.Exec("DELETE FROM c4_tasks"); err != nil {
		// Table might not exist
		_ = err
	}

	// Reset state
	if _, err := tx.Exec("DELETE FROM c4_state"); err != nil {
		_ = err
	}

	return tx.Commit()
}

// AddTask inserts a new task.
func (s *SQLiteStore) AddTask(task *Task) error {
	deps := ""
	if len(task.Dependencies) > 0 {
		depsJSON, _ := json.Marshal(task.Dependencies)
		deps = string(depsJSON)
	}

	_, err := s.db.Exec(`
		INSERT INTO c4_tasks (task_id, title, scope, dod, status, dependencies, domain, priority, model, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		task.ID, task.Title, task.Scope, task.DoD, "pending",
		deps, task.Domain, task.Priority, task.Model,
	)
	return err
}

// GetTask retrieves a task by ID.
func (s *SQLiteStore) GetTask(taskID string) (*Task, error) {
	var t Task
	var deps sql.NullString
	var createdAt, updatedAt sql.NullString

	err := s.db.QueryRow(`
		SELECT task_id, title, scope, dod, status, dependencies, domain, priority, model, worker_id, branch, commit_sha, created_at, updated_at
		FROM c4_tasks WHERE task_id = ?`, taskID,
	).Scan(&t.ID, &t.Title, &t.Scope, &t.DoD, &t.Status, &deps,
		&t.Domain, &t.Priority, &t.Model, &t.WorkerID, &t.Branch, &t.CommitSHA,
		&createdAt, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	if err != nil {
		return nil, err
	}

	if deps.Valid && deps.String != "" {
		_ = json.Unmarshal([]byte(deps.String), &t.Dependencies)
	}
	if createdAt.Valid {
		t.CreatedAt = createdAt.String
	}
	if updatedAt.Valid {
		t.UpdatedAt = updatedAt.String
	}

	return &t, nil
}

// AssignTask finds and assigns the next available task to a worker.
func (s *SQLiteStore) AssignTask(workerID string) (*TaskAssignment, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Find next pending task with all dependencies done
	var taskID, title, scope, dod, domain, model string
	var deps sql.NullString
	var priority int

	err = tx.QueryRow(`
		SELECT t.task_id, t.title, t.scope, t.dod, t.dependencies, t.domain, t.priority, t.model
		FROM c4_tasks t
		WHERE t.status = 'pending'
		AND NOT EXISTS (
			SELECT 1 FROM json_each(COALESCE(t.dependencies, '[]')) AS dep
			JOIN c4_tasks dt ON dt.task_id = dep.value
			WHERE dt.status != 'done'
		)
		ORDER BY t.priority DESC, t.created_at ASC
		LIMIT 1`,
	).Scan(&taskID, &title, &scope, &dod, &deps, &domain, &priority, &model)

	if err == sql.ErrNoRows {
		return nil, nil // No tasks available
	}
	if err != nil {
		return nil, fmt.Errorf("finding task: %w", err)
	}

	// Assign to worker
	_, err = tx.Exec(`
		UPDATE c4_tasks SET status = 'in_progress', worker_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE task_id = ?`, workerID, taskID,
	)
	if err != nil {
		return nil, fmt.Errorf("assigning task: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	assignment := &TaskAssignment{
		TaskID:   taskID,
		Title:    title,
		Scope:    scope,
		DoD:      dod,
		Domain:   domain,
		WorkerID: workerID,
		Model:    model,
	}

	if deps.Valid && deps.String != "" {
		_ = json.Unmarshal([]byte(deps.String), &assignment.Dependencies)
	}

	return assignment, nil
}

// SubmitTask marks a task as done.
func (s *SQLiteStore) SubmitTask(taskID, workerID, commitSHA string, results []ValidationResult) (*SubmitResult, error) {
	// Check for failures
	for _, r := range results {
		if r.Status == "fail" {
			return &SubmitResult{
				Success:    false,
				NextAction: "get_next_task",
				Message:    fmt.Sprintf("Validation failed: %s — %s", r.Name, r.Message),
			}, nil
		}
	}

	_, err := s.db.Exec(`
		UPDATE c4_tasks SET status = 'done', commit_sha = ?, updated_at = CURRENT_TIMESTAMP
		WHERE task_id = ?`, commitSHA, taskID,
	)
	if err != nil {
		return nil, fmt.Errorf("updating task: %w", err)
	}

	// Check if there are more tasks
	var pending int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM c4_tasks WHERE status IN ('pending', 'in_progress')").Scan(&pending)

	nextAction := "get_next_task"
	if pending == 0 {
		nextAction = "complete"
	}

	return &SubmitResult{
		Success:    true,
		NextAction: nextAction,
		Message:    fmt.Sprintf("Task %s submitted successfully", taskID),
	}, nil
}

// MarkBlocked marks a task as blocked.
func (s *SQLiteStore) MarkBlocked(taskID, workerID, failureSignature string, attempts int, lastError string) error {
	_, err := s.db.Exec(`
		UPDATE c4_tasks SET status = 'blocked', worker_id = '', updated_at = CURRENT_TIMESTAMP
		WHERE task_id = ?`, taskID,
	)
	return err
}

// ClaimTask claims a task for direct execution.
func (s *SQLiteStore) ClaimTask(taskID string) (*Task, error) {
	task, err := s.GetTask(taskID)
	if err != nil {
		return nil, err
	}

	if task.Status != "pending" {
		return nil, fmt.Errorf("task %s is %s, not pending", taskID, task.Status)
	}

	_, err = s.db.Exec(`
		UPDATE c4_tasks SET status = 'in_progress', worker_id = 'direct', updated_at = CURRENT_TIMESTAMP
		WHERE task_id = ?`, taskID,
	)
	if err != nil {
		return nil, err
	}

	task.Status = "in_progress"
	task.WorkerID = "direct"
	return task, nil
}

// ReportTask marks a directly-claimed task as done.
func (s *SQLiteStore) ReportTask(taskID, summary string, filesChanged []string) error {
	files := ""
	if len(filesChanged) > 0 {
		files = strings.Join(filesChanged, ",")
	}
	_ = files // stored as metadata if needed

	_, err := s.db.Exec(`
		UPDATE c4_tasks SET status = 'done', updated_at = CURRENT_TIMESTAMP
		WHERE task_id = ?`, taskID,
	)
	return err
}

// Checkpoint records a checkpoint decision.
func (s *SQLiteStore) Checkpoint(checkpointID, decision, notes string, requiredChanges []string) (*CheckpointResult, error) {
	// Record in a checkpoint log table (best effort)
	changesJSON := "[]"
	if len(requiredChanges) > 0 {
		b, _ := json.Marshal(requiredChanges)
		changesJSON = string(b)
	}

	_, _ = s.db.Exec(`
		INSERT OR REPLACE INTO c4_checkpoints (checkpoint_id, decision, notes, required_changes, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		checkpointID, decision, notes, changesJSON, time.Now().Format(time.RFC3339),
	)

	result := &CheckpointResult{
		Success: true,
		Message: fmt.Sprintf("Checkpoint %s: %s", checkpointID, decision),
	}

	switch decision {
	case "APPROVE":
		result.NextAction = "continue"
	case "REQUEST_CHANGES":
		result.NextAction = "apply_changes"
	case "REPLAN":
		result.NextAction = "replan"
	}

	return result, nil
}
