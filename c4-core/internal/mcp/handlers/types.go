// Package handlers implements the MCP tool handlers for C4.
//
// Each handler corresponds to a C4 MCP tool and delegates to the
// underlying Store interface for data operations. Handlers are
// responsible for JSON parsing, validation, and response formatting.
package handlers

import "time"

// Task represents a task in the C4 system.
type Task struct {
	ID           string   `json:"task_id"`
	Title        string   `json:"title"`
	Scope        string   `json:"scope,omitempty"`
	DoD          string   `json:"dod"`
	Status       string   `json:"status"`
	Dependencies []string `json:"dependencies,omitempty"`
	Domain       string   `json:"domain,omitempty"`
	Priority     int      `json:"priority"`
	Model        string   `json:"model,omitempty"`
	WorkerID     string   `json:"worker_id,omitempty"`
	Branch       string   `json:"branch,omitempty"`
	CommitSHA    string   `json:"commit_sha,omitempty"`
	CreatedAt    string   `json:"created_at,omitempty"`
	UpdatedAt    string   `json:"updated_at,omitempty"`
}

// TaskAssignment is returned when a worker is assigned a task.
type TaskAssignment struct {
	TaskID       string   `json:"task_id"`
	Title        string   `json:"title"`
	Scope        string   `json:"scope,omitempty"`
	DoD          string   `json:"dod"`
	Dependencies []string `json:"dependencies,omitempty"`
	Domain       string   `json:"domain,omitempty"`
	Branch       string   `json:"branch,omitempty"`
	WorkerID     string   `json:"worker_id"`
	WorktreePath string   `json:"worktree_path,omitempty"`
	Model        string   `json:"recommended_model,omitempty"`
}

// ValidationResult holds the result of a single validation run.
type ValidationResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "pass" or "fail"
	Message string `json:"message,omitempty"`
}

// SubmitResult holds the result of a task submission.
type SubmitResult struct {
	Success    bool   `json:"success"`
	NextAction string `json:"next_action"` // "get_next_task", "await_checkpoint", "complete"
	Message    string `json:"message,omitempty"`
}

// CheckpointResult holds the result of a checkpoint decision.
type CheckpointResult struct {
	Success    bool   `json:"success"`
	NextAction string `json:"next_action,omitempty"`
	Message    string `json:"message,omitempty"`
}

// WorkerInfo holds information about a registered worker.
type WorkerInfo struct {
	ID          string    `json:"worker_id"`
	Status      string    `json:"status"` // "idle", "busy"
	CurrentTask string    `json:"current_task,omitempty"`
	LastSeen    time.Time `json:"last_seen"`
}

// ProjectStatus holds the overall project status.
type ProjectStatus struct {
	State        string       `json:"state"` // "INIT", "PLAN", "EXECUTE", etc.
	ProjectName  string       `json:"project_name"`
	TotalTasks   int          `json:"total_tasks"`
	PendingTasks int          `json:"pending_tasks"`
	InProgress   int          `json:"in_progress_tasks"`
	DoneTasks    int          `json:"done_tasks"`
	BlockedTasks int          `json:"blocked_tasks"`
	Workers      []WorkerInfo `json:"workers,omitempty"`
}

// Store defines the data access interface for MCP handlers.
// This is implemented by the SQLite store or any other backend.
type Store interface {
	// State management
	GetStatus() (*ProjectStatus, error)
	Start() error
	Clear(keepConfig bool) error
	TransitionState(from, to string) error

	// Task management
	AddTask(task *Task) error
	GetTask(taskID string) (*Task, error)
	AssignTask(workerID string) (*TaskAssignment, error)
	SubmitTask(taskID, workerID, commitSHA string, results []ValidationResult) (*SubmitResult, error)
	MarkBlocked(taskID, workerID, failureSignature string, attempts int, lastError string) error

	// Direct mode
	ClaimTask(taskID string) (*Task, error)
	ReportTask(taskID, summary string, filesChanged []string) error

	// Supervisor
	Checkpoint(checkpointID, decision, notes string, requiredChanges []string) (*CheckpointResult, error)
}
