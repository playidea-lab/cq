// Package store defines the core data types and Store interface used by
// the MCP handlers and cloud packages. Extracting these into a separate
// package breaks the import cycle between handlers and cloud.
package store

import "time"

// Task represents a task in the C4 system.
type Task struct {
	ID            string   `json:"task_id"`
	Title         string   `json:"title"`
	Scope         string   `json:"scope,omitempty"`
	DoD           string   `json:"dod"`
	Status        string   `json:"status"`
	Dependencies  []string `json:"dependencies,omitempty"`
	Domain        string   `json:"domain,omitempty"`
	Priority      int      `json:"priority"`
	Model         string   `json:"model,omitempty"`
	ExecutionMode string   `json:"execution_mode,omitempty"` // worker, direct, auto
	WorkerID      string   `json:"worker_id,omitempty"`
	Branch        string   `json:"branch,omitempty"`
	CommitSHA     string   `json:"commit_sha,omitempty"`
	// FilesChanged stores the CSV list of files changed during direct mode (c4_report).
	FilesChanged string `json:"files_changed,omitempty"`
	// ReviewDecisionEvidence stores REQUEST_CHANGES reason (comments) for R-tasks; do not use commit_sha for that.
	ReviewDecisionEvidence string `json:"review_decision_evidence,omitempty"`
	// SupersededBy is set on an R-task when REQUEST_CHANGES creates a newer R-task.
	// It holds the ID of the replacement R-task, making stale reviews easy to filter.
	SupersededBy string `json:"superseded_by,omitempty"`
	// Blocked-task diagnostics (persisted by mark_blocked)
	FailureSignature string `json:"failure_signature,omitempty"`
	Attempts         int    `json:"attempts,omitempty"`
	LastError       string `json:"last_error,omitempty"`
	CreatedAt       string `json:"created_at,omitempty"`
	UpdatedAt       string `json:"updated_at,omitempty"`
}

// TaskAssignment is returned when a worker is assigned a task.
type TaskAssignment struct {
	TaskID           string             `json:"task_id"`
	Title            string             `json:"title"`
	Scope            string             `json:"scope,omitempty"`
	DoD              string             `json:"dod"`
	Dependencies     []string           `json:"dependencies,omitempty"`
	Domain           string             `json:"domain,omitempty"`
	Branch           string             `json:"branch,omitempty"`
	WorkerID         string             `json:"worker_id"`
	WorktreePath     string             `json:"worktree_path,omitempty"`
	Model            string             `json:"recommended_model,omitempty"`
	ReviewContext    *ReviewContext     `json:"review_context,omitempty"`
	SoulContext      string             `json:"soul_context,omitempty"`
	LighthouseSpec   *LighthouseContext `json:"lighthouse_spec,omitempty"`
	KnowledgeContext string             `json:"knowledge_context,omitempty"`
}

// HandoffEvidence is a reference to a CDP or test artifact attached to a submit.
type HandoffEvidence struct {
	Type        string `json:"type"`        // "screenshot" | "log" | "test_result"
	ArtifactID  string `json:"artifact_id"` // c4_artifact_save로 저장된 ID
	Description string `json:"description"`
}

// ReviewContext provides context from the parent implementation task for review tasks.
type ReviewContext struct {
	ParentTaskID string `json:"parent_task_id"`
	CommitSHA    string `json:"commit_sha,omitempty"`
	FilesChanged string            `json:"files_changed,omitempty"`
	Evidence     []HandoffEvidence `json:"evidence,omitempty"`
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
	Success            bool   `json:"success"`
	NextAction         string `json:"next_action"` // "get_next_task", "await_checkpoint", "complete"
	Message            string `json:"message,omitempty"`
	PendingReview      string `json:"pending_review,omitempty"` // R- task ID pending Worker review (non-empty when review_required=true)
	ValidationSkipped  bool   `json:"validation_skipped,omitempty"` // true when validation_results was omitted
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
	PersonaDigest         *PersonaSummary        `json:"persona_digest,omitempty"`
	KnowledgeSearchStats  *KnowledgeSearchStats  `json:"knowledge_search_stats,omitempty"`
}

// KnowledgeSearchStats holds in-session knowledge search hit/miss statistics.
type KnowledgeSearchStats struct {
	TotalSearches int     `json:"total_searches"`
	Hits          int     `json:"hits"`
	Misses        int     `json:"misses"`
	HitRate       float64 `json:"hit_rate"` // Hits / TotalSearches (0 if TotalSearches == 0)
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

// TaskFilter defines filtering criteria for ListTasks.
type TaskFilter struct {
	Status   string `json:"status,omitempty"`
	Domain   string `json:"domain,omitempty"`
	WorkerID string `json:"worker_id,omitempty"`
	Limit    int    `json:"limit,omitempty"`
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
	DeleteTask(taskID string) error
	GetTask(taskID string) (*Task, error)
	AssignTask(workerID string) (*TaskAssignment, error)
	SubmitTask(taskID, workerID, commitSHA, handoff string, results []ValidationResult) (*SubmitResult, error)
	MarkBlocked(taskID, workerID, failureSignature string, attempts int, lastError string) error

	// ListTasks returns tasks matching the filter; backend may be SQLite or cloud. Limit 0 means default (e.g. 50).
	ListTasks(filter TaskFilter) ([]Task, int, error)

	// Direct mode
	ClaimTask(taskID string) (*Task, error)
	ReportTask(taskID, summary string, filesChanged []string) error

	// Supervisor. targetTaskID/targetReviewID are optional; when both set, attribution uses them (explicit linkage). Otherwise resolved from CP task deps.
	Checkpoint(checkpointID, decision, notes string, requiredChanges []string, targetTaskID, targetReviewID string) (*CheckpointResult, error)

	// Review-as-Task: REQUEST_CHANGES creates next version T+R pair
	RequestChanges(reviewTaskID string, comments string, requiredChanges []string) (*RequestChangesResult, error)
}
