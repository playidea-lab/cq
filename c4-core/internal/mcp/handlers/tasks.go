package handlers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/changmin/c4-core/internal/mcp"
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
	WorkerID          string             `json:"worker_id,omitempty"`
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
			},
			"required": []string{"task_id", "commit_sha", "validation_results"},
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
				"comments":        map[string]any{"type": "string", "description": "Reason for rejection"},
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
				"worker_id":        map[string]any{"type": "string", "description": "ID of the worker"},
				"failure_signature": map[string]any{"type": "string", "description": "Error signature from validation failures"},
				"attempts":         map[string]any{"type": "integer", "description": "Number of fix attempts made"},
				"last_error":       map[string]any{"type": "string", "description": "Last error message"},
			},
			"required": []string{"task_id", "worker_id", "failure_signature", "attempts"},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleMarkBlocked(store, args)
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

	result, err := store.SubmitTask(args.TaskID, args.WorkerID, args.CommitSHA, args.ValidationResults)
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
	}

	if err := store.AddTask(t); err != nil {
		return nil, fmt.Errorf("adding task: %w", err)
	}

	result := map[string]any{
		"success": true,
		"task_id": args.TaskID,
		"message": fmt.Sprintf("Task %s added to queue", args.TaskID),
	}

	// Auto-generate R-XXX review task if review_required=true (default) and T- prefix
	reviewRequired := args.ReviewRequired == nil || *args.ReviewRequired
	if reviewRequired && strings.HasPrefix(args.TaskID, "T-") {
		_, baseID, version, _ := task.ParseTaskID(args.TaskID)
		reviewID := task.ReviewID(baseID, version)
		reviewTask := &Task{
			ID:           reviewID,
			Title:        fmt.Sprintf("Review: %s", args.Title),
			DoD:          fmt.Sprintf("Review implementation of %s\n\nOriginal DoD:\n%s", args.TaskID, args.DoD),
			Status:       "pending",
			Dependencies: []string{args.TaskID},
			Domain:       args.Domain,
			Priority:     args.Priority,
		}
		if err := store.AddTask(reviewTask); err != nil {
			// Best-effort: log but don't fail the main task
			fmt.Printf("c4: warning: failed to create review task %s: %v\n", reviewID, err)
		} else {
			result["review_task_id"] = reviewID
		}
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
	if len(args.RequiredChanges) == 0 {
		return nil, fmt.Errorf("required_changes must not be empty")
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
