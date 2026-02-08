package task

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// SQLiteTaskStore implements TaskStore using a local SQLite database.
//
// This is compatible with the Python C4 daemon's .c4/c4.db schema:
//
//	c4_tasks (
//	    project_id TEXT,
//	    task_id    TEXT,
//	    task_json  TEXT NOT NULL,
//	    status     TEXT NOT NULL,
//	    assigned_to TEXT,
//	    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
//	    PRIMARY KEY (project_id, task_id)
//	)
//
// Tasks are serialized to JSON in the task_json column, with status
// and assigned_to denormalized for efficient queries.
type SQLiteTaskStore struct {
	db        *sql.DB
	projectID string
}

// Compile-time interface check.
var _ TaskStore = (*SQLiteTaskStore)(nil)

// NewSQLiteTaskStore creates a SQLite-backed task store.
// The database tables are created if they don't exist.
func NewSQLiteTaskStore(db *sql.DB, projectID string) (*SQLiteTaskStore, error) {
	store := &SQLiteTaskStore{
		db:        db,
		projectID: projectID,
	}

	if err := store.initSchema(); err != nil {
		return nil, fmt.Errorf("init schema: %w", err)
	}

	return store, nil
}

// initSchema creates the c4_tasks table if it doesn't exist.
// Matches the Python SQLiteTaskStore schema exactly.
func (s *SQLiteTaskStore) initSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS c4_tasks (
			project_id  TEXT,
			task_id     TEXT,
			task_json   TEXT NOT NULL,
			status      TEXT NOT NULL,
			assigned_to TEXT,
			updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (project_id, task_id)
		)
	`)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_c4_tasks_status
		ON c4_tasks (project_id, status)
	`)
	return err
}

// CreateTask inserts a new task into the SQLite database.
func (s *SQLiteTaskStore) CreateTask(task *Task) error {
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now()
	}

	taskJSON, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshal task: %w", err)
	}

	_, err = s.db.Exec(
		`INSERT INTO c4_tasks (project_id, task_id, task_json, status, assigned_to, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		s.projectID, task.ID, string(taskJSON),
		string(task.Status), task.AssignedTo, time.Now(),
	)
	if err != nil {
		return fmt.Errorf("insert task %s: %w", task.ID, err)
	}
	return nil
}

// GetTask retrieves a task by ID.
func (s *SQLiteTaskStore) GetTask(id string) (*Task, error) {
	var taskJSON string
	err := s.db.QueryRow(
		"SELECT task_json FROM c4_tasks WHERE project_id = ? AND task_id = ?",
		s.projectID, id,
	).Scan(&taskJSON)

	if err == sql.ErrNoRows {
		return nil, ErrTaskNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query task %s: %w", id, err)
	}

	var task Task
	if err := json.Unmarshal([]byte(taskJSON), &task); err != nil {
		return nil, fmt.Errorf("unmarshal task %s: %w", id, err)
	}
	return &task, nil
}

// UpdateTask updates an existing task.
func (s *SQLiteTaskStore) UpdateTask(task *Task) error {
	taskJSON, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshal task: %w", err)
	}

	result, err := s.db.Exec(
		`UPDATE c4_tasks SET task_json = ?, status = ?, assigned_to = ?, updated_at = ?
		 WHERE project_id = ? AND task_id = ?`,
		string(taskJSON), string(task.Status), task.AssignedTo, time.Now(),
		s.projectID, task.ID,
	)
	if err != nil {
		return fmt.Errorf("update task %s: %w", task.ID, err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrTaskNotFound
	}
	return nil
}

// ListTasks returns all tasks for the project.
func (s *SQLiteTaskStore) ListTasks(projectID string) ([]*Task, error) {
	pid := projectID
	if pid == "" {
		pid = s.projectID
	}

	rows, err := s.db.Query(
		"SELECT task_json FROM c4_tasks WHERE project_id = ?",
		pid,
	)
	if err != nil {
		return nil, fmt.Errorf("query tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		var taskJSON string
		if err := rows.Scan(&taskJSON); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		var task Task
		if err := json.Unmarshal([]byte(taskJSON), &task); err != nil {
			return nil, fmt.Errorf("unmarshal task: %w", err)
		}
		tasks = append(tasks, &task)
	}
	return tasks, rows.Err()
}

// DeleteTask removes a task by ID.
func (s *SQLiteTaskStore) DeleteTask(id string) error {
	result, err := s.db.Exec(
		"DELETE FROM c4_tasks WHERE project_id = ? AND task_id = ?",
		s.projectID, id,
	)
	if err != nil {
		return fmt.Errorf("delete task %s: %w", id, err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrTaskNotFound
	}
	return nil
}

// GetNextTask finds the highest-priority pending task whose dependencies
// are all done and whose scope is not locked by another worker.
// Assigns the task to the given worker atomically.
func (s *SQLiteTaskStore) GetNextTask(workerID string) (*Task, error) {
	// Load all tasks to resolve dependencies and scope locks
	allTasks, err := s.ListTasks("")
	if err != nil {
		return nil, err
	}

	// Build lookup maps
	taskMap := make(map[string]*Task, len(allTasks))
	scopeLocks := make(map[string]string) // scope -> worker_id
	for _, t := range allTasks {
		taskMap[t.ID] = t
		if t.Status == StatusInProgress && t.Scope != "" {
			scopeLocks[t.Scope] = t.AssignedTo
		}
	}

	// Find eligible candidates
	var candidates []*Task
	for _, t := range allTasks {
		if t.Status != StatusPending {
			continue
		}
		// Check dependencies
		if !depsMetForTask(t, taskMap) {
			continue
		}
		// Check scope lock
		if t.Scope != "" {
			owner, locked := scopeLocks[t.Scope]
			if locked && owner != workerID {
				continue
			}
		}
		candidates = append(candidates, t)
	}

	if len(candidates) == 0 {
		return nil, ErrNoAvailableTask
	}

	// Sort by priority descending, then created_at ascending
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority > candidates[j].Priority
		}
		return candidates[i].CreatedAt.Before(candidates[j].CreatedAt)
	})

	selected := candidates[0]
	selected.Status = StatusInProgress
	selected.AssignedTo = workerID

	if err := s.UpdateTask(selected); err != nil {
		return nil, fmt.Errorf("assign task %s: %w", selected.ID, err)
	}

	return selected, nil
}

// CompleteTask marks a task as done and auto-generates a review task
// for implementation tasks.
func (s *SQLiteTaskStore) CompleteTask(taskID string, workerID string, commitSHA string) (*Task, error) {
	task, err := s.GetTask(taskID)
	if err != nil {
		return nil, err
	}

	if task.Status != StatusInProgress {
		return nil, ErrNotInProgress
	}
	if task.AssignedTo != workerID {
		return nil, ErrWorkerMismatch
	}

	now := time.Now()
	task.Status = StatusDone
	task.CommitSHA = commitSHA
	task.CompletedAt = now

	if err := s.UpdateTask(task); err != nil {
		return nil, fmt.Errorf("complete task %s: %w", taskID, err)
	}

	// Auto-generate review task for implementation tasks
	if task.Type == TypeImplementation {
		reviewID := ReviewID(task.BaseID, task.Version)
		reviewTask := &Task{
			ID:           reviewID,
			Title:        fmt.Sprintf("Review: %s", task.Title),
			Scope:        task.Scope,
			DoD:          fmt.Sprintf("Review implementation of %s (commit: %s)", taskID, commitSHA),
			Dependencies: []string{taskID},
			Status:       StatusPending,
			Domain:       task.Domain,
			Model:        task.Model,
			Type:         TypeReview,
			BaseID:       task.BaseID,
			Version:      task.Version,
			ParentID:     taskID,
			CompletedBy:  workerID,
			CreatedAt:    now,
		}
		if err := s.CreateTask(reviewTask); err != nil {
			return nil, fmt.Errorf("create review task: %w", err)
		}
		return reviewTask, nil
	}

	return nil, nil
}

// depsMetForTask checks if all of a task's dependencies are done.
func depsMetForTask(task *Task, taskMap map[string]*Task) bool {
	for _, depID := range task.Dependencies {
		dep, ok := taskMap[depID]
		if !ok || dep.Status != StatusDone {
			return false
		}
	}
	return true
}
