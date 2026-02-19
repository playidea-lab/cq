package handlers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/store"
	"github.com/changmin/c4-core/internal/task"
)

// getTaskArgs is the input for c4_get_task.
type getTaskArgs struct {
	WorkerID string `json:"worker_id"`
}

// submitArgs is the input for c4_submit.
type submitArgs struct {
	TaskID            string             `json:"task_id"`
	CommitSHA         string             `json:"commit_sha"`
	ValidationResults []ValidationResult `json:"validation_results"`
	WorkerID          string             `json:"worker_id"`
	Handoff           string             `json:"handoff,omitempty"` // Structured handoff: discoveries, concerns, feedback
}

// addTodoArgs is the input for c4_add_todo.
type addTodoArgs struct {
	TaskID         string   `json:"task_id"`
	Title          string   `json:"title"`
	Scope          string   `json:"scope,omitempty"`
	DoD            string   `json:"dod"`
	Dependencies   []string `json:"dependencies,omitempty"`
	Domain         string   `json:"domain,omitempty"`
	Priority       int      `json:"priority"`
	Model          string   `json:"model,omitempty"`
	ExecutionMode  string   `json:"execution_mode,omitempty"`
	ReviewRequired *bool    `json:"review_required,omitempty"`
}

// requestChangesArgs is the input for c4_request_changes.
type requestChangesArgs struct {
	ReviewTaskID    string   `json:"review_task_id"`
	Comments        string   `json:"comments"`
	RequiredChanges []string `json:"required_changes"`
}

// markBlockedArgs is the input for c4_mark_blocked.
type markBlockedArgs struct {
	TaskID           string `json:"task_id"`
	WorkerID         string `json:"worker_id"`
	FailureSignature string `json:"failure_signature"`
	Attempts         int    `json:"attempts"`
	LastError        string `json:"last_error,omitempty"`
}

func resolveExecutionMode(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "worker":
		return "worker", nil
	case "direct":
		return "direct", nil
	case "auto":
		return "auto", nil
	default:
		return "", fmt.Errorf("invalid execution_mode: %s (must be worker, direct, or auto)", mode)
	}
}

// RegisterTaskHandlers registers task management tools on the registry.
func RegisterTaskHandlers(reg *mcp.Registry, store Store) {
	// c4_get_task
	reg.Register(mcp.ToolSchema{
		Name:        "c4_get_task",
		Description: "Request next task assignment for a worker",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"worker_id": map[string]any{
					"type":        "string",
					"description": "Unique identifier for the worker",
				},
			},
			"required": []string{"worker_id"},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleGetTask(store, args)
	})

	// c4_submit
	reg.Register(mcp.ToolSchema{
		Name:        "c4_submit",
		Description: "Report task completion with validation results",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_id": map[string]any{
					"type":        "string",
					"description": "ID of the completed task",
				},
				"commit_sha": map[string]any{
					"type":        "string",
					"description": "Git commit SHA of the work",
				},
				"validation_results": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name":    map[string]any{"type": "string"},
							"status":  map[string]any{"type": "string", "enum": []string{"pass", "fail"}},
							"message": map[string]any{"type": "string"},
						},
						"required": []string{"name", "status"},
					},
					"description": "Results of validation runs (lint, test, etc.)",
				},
				"worker_id": map[string]any{
					"type":        "string",
					"description": "Worker ID submitting the task (for ownership verification)",
				},
				"handoff": map[string]any{
					"type":        "string",
					"description": "Structured handoff note (discoveries, concerns, feedback for next agent)",
				},
			},
			"required": []string{"task_id", "commit_sha", "validation_results", "worker_id"},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleSubmit(store, args)
	})

	// c4_add_todo
	reg.Register(mcp.ToolSchema{
		Name:        "c4_add_todo",
		Description: "Add a new task to the queue with optional dependencies",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_id":      map[string]any{"type": "string", "description": "Unique task ID (e.g., T-001)"},
				"title":        map[string]any{"type": "string", "description": "Task title"},
				"scope":        map[string]any{"type": "string", "description": "File/directory scope for lock"},
				"dod":          map[string]any{"type": "string", "description": "Definition of Done"},
				"dependencies": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Task IDs that must complete first"},
				"domain":       map[string]any{"type": "string", "description": "Domain for agent routing"},
				"priority":     map[string]any{"type": "integer", "description": "Higher priority tasks assigned first (default: 0)"},
				"model":        map[string]any{"type": "string", "enum": []string{"opus", "sonnet", "haiku"}, "description": "Claude model tier for this task"},
				"execution_mode": map[string]any{
					"type":        "string",
					"enum":        []string{"worker", "direct", "auto"},
					"description": "Execution mode (default: worker)",
					"default":     "worker",
				},
				"review_required": map[string]any{
					"type":        "boolean",
					"description": "Whether to auto-generate review task on completion (default: true)",
					"default":     true,
				},
			},
			"required": []string{"task_id", "title", "dod"},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleAddTodo(store, args)
	})

	// c4_request_changes
	reg.Register(mcp.ToolSchema{
		Name:        "c4_request_changes",
		Description: "Reject a review task and create next version (T-001-0 → T-001-1 + R-001-1)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"review_task_id":   map[string]any{"type": "string", "description": "Review task to reject (R-XXX-N)"},
				"comments":         map[string]any{"type": "string", "description": "Reason for rejection"},
				"required_changes": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "List of required changes"},
			},
			"required": []string{"review_task_id", "comments", "required_changes"},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleRequestChanges(store, args)
	})

	// c4_mark_blocked
	reg.Register(mcp.ToolSchema{
		Name:        "c4_mark_blocked",
		Description: "Mark a task as blocked after max retry attempts",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_id":           map[string]any{"type": "string", "description": "ID of the blocked task"},
				"worker_id":         map[string]any{"type": "string", "description": "ID of the worker"},
				"failure_signature": map[string]any{"type": "string", "description": "Error signature from validation failures"},
				"attempts":          map[string]any{"type": "integer", "description": "Number of fix attempts made"},
				"last_error":        map[string]any{"type": "string", "description": "Last error message"},
			},
			"required": []string{"task_id", "worker_id", "failure_signature", "attempts"},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleMarkBlocked(store, args)
	})

	// c4_task_list
	reg.Register(mcp.ToolSchema{
		Name:        "c4_task_list",
		Description: "List tasks with optional status/domain/worker filtering",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status":    map[string]any{"type": "string", "description": "Filter by status: pending, in_progress, done, blocked", "enum": []string{"pending", "in_progress", "done", "blocked"}},
				"domain":    map[string]any{"type": "string", "description": "Filter by domain"},
				"worker_id": map[string]any{"type": "string", "description": "Filter by assigned worker ID"},
				"limit":     map[string]any{"type": "integer", "description": "Max results (default: 50)", "default": 50},
			},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleTaskList(store, args)
	})
}

func handleGetTask(store Store, rawArgs json.RawMessage) (any, error) {
	var args getTaskArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("parsing arguments: %w", err)
	}

	if args.WorkerID == "" {
		return nil, fmt.Errorf("worker_id is required")
	}

	assignment, err := store.AssignTask(args.WorkerID)
	if err != nil {
		return nil, fmt.Errorf("assigning task: %w", err)
	}

	if assignment == nil {
		// No tasks available
		return map[string]any{}, nil
	}

	return assignment, nil
}

func handleSubmit(store Store, rawArgs json.RawMessage) (any, error) {
	var args submitArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("parsing arguments: %w", err)
	}

	if args.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	if args.CommitSHA == "" {
		return nil, fmt.Errorf("commit_sha is required")
	}
	if args.WorkerID == "" {
		return nil, fmt.Errorf("worker_id is required")
	}

	result, err := store.SubmitTask(args.TaskID, args.WorkerID, args.CommitSHA, args.Handoff, args.ValidationResults)
	if err != nil {
		return nil, fmt.Errorf("submitting task: %w", err)
	}

	return result, nil
}

func handleAddTodo(store Store, rawArgs json.RawMessage) (any, error) {
	var args addTodoArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("parsing arguments: %w", err)
	}

	if args.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	if args.Title == "" {
		return nil, fmt.Errorf("title is required")
	}
	if args.DoD == "" {
		return nil, fmt.Errorf("dod is required")
	}
	if err := task.ValidateTaskID(args.TaskID); err != nil {
		return nil, err
	}
	executionMode, err := resolveExecutionMode(args.ExecutionMode)
	if err != nil {
		return nil, err
	}
	for _, depID := range args.Dependencies {
		if err := task.ValidateTaskID(depID); err != nil {
			return nil, fmt.Errorf("invalid dependency %q: %w", depID, err)
		}
	}

	t := &Task{
		ID:           args.TaskID,
		Title:        args.Title,
		Scope:        args.Scope,
		DoD:          args.DoD,
		Status:       "pending",
		Dependencies: args.Dependencies,
		Domain:       args.Domain,
		Priority:     args.Priority,
		Model:        args.Model,
		ExecutionMode: executionMode,
	}

	if err := store.AddTask(t); err != nil {
		return nil, fmt.Errorf("adding task: %w", err)
	}

	result := map[string]any{
		"success": true,
		"task_id": args.TaskID,
		"message": fmt.Sprintf("Task %s added to queue", args.TaskID),
	}

	// When review_required=true, create review task; on failure rollback main task (non-best-effort).
	reviewRequired := args.ReviewRequired == nil || *args.ReviewRequired
	if reviewRequired && strings.HasPrefix(args.TaskID, "T-") {
		_, baseID, version, _ := task.ParseTaskID(args.TaskID)
		reviewID := task.ReviewID(baseID, version)
		reviewTask := &Task{
			ID:           reviewID,
			Title:        fmt.Sprintf("Review: %s", args.Title),
			DoD:          BuildReviewDoD(args.TaskID, args.DoD, 0),
			Status:       "pending",
			Dependencies: []string{args.TaskID},
			Domain:       args.Domain,
			Priority:     args.Priority,
		}
		if err := store.AddTask(reviewTask); err != nil {
			_ = store.DeleteTask(args.TaskID)
			return nil, fmt.Errorf("review task %s: %w (main task rolled back)", reviewID, err)
		}
		result["review_task_id"] = reviewID
	}

	return result, nil
}

func handleRequestChanges(store Store, rawArgs json.RawMessage) (any, error) {
	var args requestChangesArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("parsing arguments: %w", err)
	}

	if args.ReviewTaskID == "" {
		return nil, fmt.Errorf("review_task_id is required")
	}
	if args.Comments == "" {
		return nil, fmt.Errorf("comments is required")
	}
	args.RequiredChanges = normalizeRequiredChanges(args.RequiredChanges)
	if len(args.RequiredChanges) == 0 {
		return nil, fmt.Errorf("required_changes must contain at least one non-empty item")
	}
	if err := task.ValidateTaskID(args.ReviewTaskID); err != nil {
		return nil, err
	}
	if _, _, _, taskType := task.ParseTaskID(args.ReviewTaskID); taskType != task.TypeReview {
		return nil, fmt.Errorf("review_task_id must be an R- task: %s", args.ReviewTaskID)
	}

	result, err := store.RequestChanges(args.ReviewTaskID, args.Comments, args.RequiredChanges)
	if err != nil {
		return nil, fmt.Errorf("request changes: %w", err)
	}

	return result, nil
}

func handleMarkBlocked(store Store, rawArgs json.RawMessage) (any, error) {
	var args markBlockedArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("parsing arguments: %w", err)
	}

	if args.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	if args.WorkerID == "" {
		return nil, fmt.Errorf("worker_id is required")
	}
	if args.FailureSignature == "" {
		return nil, fmt.Errorf("failure_signature is required")
	}

	if err := store.MarkBlocked(args.TaskID, args.WorkerID, args.FailureSignature, args.Attempts, args.LastError); err != nil {
		return nil, fmt.Errorf("marking blocked: %w", err)
	}

	return map[string]any{
		"success": true,
		"task_id": args.TaskID,
		"status":  "blocked",
		"message": fmt.Sprintf("Task %s marked as blocked after %d attempts", args.TaskID, args.Attempts),
	}, nil
}

// taskListArgs is the input for c4_task_list.
type taskListArgs struct {
	Status   string `json:"status"`
	Domain   string `json:"domain"`
	WorkerID string `json:"worker_id"`
	Limit    int    `json:"limit"`
}

func handleTaskList(s Store, rawArgs json.RawMessage) (any, error) {
	var args taskListArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("parsing arguments: %w", err)
	}

	tasks, total, err := s.ListTasks(store.TaskFilter{
		Status:   args.Status,
		Domain:   args.Domain,
		WorkerID: args.WorkerID,
		Limit:    args.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing tasks: %w", err)
	}

	return map[string]any{
		"tasks":    tasks,
		"total":    total,
		"filtered": len(tasks),
	}, nil
}
