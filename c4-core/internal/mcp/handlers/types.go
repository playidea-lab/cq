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
	TaskID        string         `json:"task_id"`
	Title         string         `json:"title"`
	Scope         string         `json:"scope,omitempty"`
	DoD           string         `json:"dod"`
	Dependencies  []string       `json:"dependencies,omitempty"`
	Domain        string         `json:"domain,omitempty"`
	Branch        string         `json:"branch,omitempty"`
	WorkerID       string             `json:"worker_id"`
	WorktreePath   string             `json:"worktree_path,omitempty"`
	Model          string             `json:"recommended_model,omitempty"`
	ReviewContext  *ReviewContext     `json:"review_context,omitempty"`
	SoulContext    string             `json:"soul_context,omitempty"`
	LighthouseSpec *LighthouseContext `json:"lighthouse_spec,omitempty"`
}

// ReviewContext provides context from the parent implementation task for review tasks.
type ReviewContext struct {
	ParentTaskID string `json:"parent_task_id"`
	CommitSHA    string `json:"commit_sha,omitempty"`
	FilesChanged string `json:"files_changed,omitempty"`
}

// RequestChangesResult holds the result of a REQUEST_CHANGES operation.
type RequestChangesResult struct {
	Success      bool   `json:"success"`
	NextTaskID   string `json:"next_task_id"`
	NextReviewID string `json:"next_review_id"`
	Version      int    `json:"version"`
	Message      string `json:"message"`
}

// WorkerConfigInfo exposes worker/config info in c4_status.
type WorkerConfigInfo struct {
	WorkBranchPrefix string `json:"work_branch_prefix"`
	DefaultBranch    string `json:"default_branch"`
	WorktreeEnabled  bool   `json:"worktree_enabled"`
	ReviewAsTask     bool   `json:"review_as_task"`
	MaxRevision      int    `json:"max_revision"`
}

// ValidationResult holds the result of a single validation run.
type ValidationResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "pass" or "fail"
	Message string `json:"message,omitempty"`
}

// SubmitResult holds the result of a task submission.
type SubmitResult struct {
	Success       bool   `json:"success"`
	NextAction    string `json:"next_action"` // "get_next_task", "await_checkpoint", "complete"
	Message       string `json:"message,omitempty"`
	PendingReview string `json:"pending_review,omitempty"` // R- task ID awaiting review (auto-judge hint)
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

// EconomicModeInfo exposes economic mode configuration in c4_status.
type EconomicModeInfo struct {
	Enabled      bool              `json:"enabled"`
	Preset       string            `json:"preset,omitempty"`
	ModelRouting map[string]string `json:"model_routing,omitempty"`
}

// ProjectStatus holds the overall project status.
type ProjectStatus struct {
	State                 string            `json:"state"` // "INIT", "PLAN", "EXECUTE", etc.
	ProjectName           string            `json:"project_name"`
	TotalTasks            int               `json:"total_tasks"`
	PendingTasks          int               `json:"pending_tasks"`
	ReadyTasks            int               `json:"ready_tasks"`
	BlockedByDeps         int               `json:"blocked_by_dependencies"`
	ReadyTaskIDs          []string          `json:"ready_task_ids,omitempty"`
	InProgress            int               `json:"in_progress_tasks"`
	DoneTasks             int               `json:"done_tasks"`
	BlockedTasks          int               `json:"blocked_tasks"`
	Workers               []WorkerInfo      `json:"workers,omitempty"`
	EconomicMode          *EconomicModeInfo `json:"economic_mode,omitempty"`
	WorkerConfig          *WorkerConfigInfo `json:"worker_config,omitempty"`
	ActiveSoulRoles       []string          `json:"active_soul_roles,omitempty"`
	LighthouseStubs       int               `json:"lighthouse_stubs,omitempty"`
	LighthouseImplemented int               `json:"lighthouse_implemented,omitempty"`
	OrphanReviews         int               `json:"orphan_reviews,omitempty"`
	PersonaDigest         *PersonaSummary   `json:"persona_digest,omitempty"`
}

// PersonaSummary provides a quick overview of persona performance.
type PersonaSummary struct {
	TotalTasks   int     `json:"total_tasks"`
	ApprovalRate float64 `json:"approval_rate"`
}

// Lighthouse represents a spec-as-MCP stub tool.
type Lighthouse struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema string `json:"input_schema"`
	Spec        string `json:"spec"`
	Status      string `json:"status"` // "stub", "implemented", "deprecated"
	Version     int    `json:"version"`
	TaskID      string `json:"task_id,omitempty"`
	CreatedBy   string `json:"created_by,omitempty"`
	PromotedBy  string `json:"promoted_by,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// LighthouseContext provides lighthouse spec context for T-LH- tasks.
type LighthouseContext struct {
	Name        string `json:"name"`
	Spec        string `json:"spec"`
	InputSchema string `json:"input_schema"`
	Description string `json:"description"`
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
	SubmitTask(taskID, workerID, commitSHA, handoff string, results []ValidationResult) (*SubmitResult, error)
	MarkBlocked(taskID, workerID, failureSignature string, attempts int, lastError string) error

	// Direct mode
	ClaimTask(taskID string) (*Task, error)
	ReportTask(taskID, summary string, filesChanged []string) error

	// Supervisor
	Checkpoint(checkpointID, decision, notes string, requiredChanges []string) (*CheckpointResult, error)

	// Review-as-Task: REQUEST_CHANGES creates next version T+R pair
	RequestChanges(reviewTaskID string, comments string, requiredChanges []string) (*RequestChangesResult, error)
}
