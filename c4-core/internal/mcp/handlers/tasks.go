package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/persona"
	"github.com/changmin/c4-core/internal/store"
	"github.com/changmin/c4-core/internal/task"
)

// getTaskArgs is the input for c4_get_task.
type getTaskArgs struct {
	WorkerID string `json:"worker_id"`
	TaskID   string `json:"task_id"` // optional: request specific task instead of next-in-queue
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

// HeartbeatStore extends Store with explicit worker heartbeat capability.
// Implemented by SQLiteStore; other store implementations (e.g., mock) need not implement it.
// The tasks.go handler performs a type assertion to check availability at runtime.
type HeartbeatStore interface {
	WorkerHeartbeat(workerID string) (int64, error)
}

// RegisterTaskHandlers registers task management tools on the registry.
func RegisterTaskHandlers(reg *mcp.Registry, store Store) {
	// c4_get_task
	reg.Register(mcp.ToolSchema{
		Name:        "c4_get_task",
		Description: "Request next task assignment for a worker. Optionally specify task_id to request a specific task.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"worker_id": map[string]any{
					"type":        "string",
					"description": "Unique identifier for the worker",
				},
				"task_id": map[string]any{
					"type":        "string",
					"description": "Optional: request a specific task by ID instead of next-in-queue",
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
			"required": []string{"task_id", "commit_sha", "worker_id"},
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
				"status":      map[string]any{"type": "string", "description": "Filter by status: pending, in_progress, done, blocked", "enum": []string{"pending", "in_progress", "done", "blocked"}},
				"domain":      map[string]any{"type": "string", "description": "Filter by domain"},
				"worker_id":   map[string]any{"type": "string", "description": "Filter by assigned worker ID"},
				"limit":       map[string]any{"type": "integer", "description": "Max results (default: 50)", "default": 50},
				"include_dod": map[string]any{"type": "boolean", "description": "Include DoD field in response (default: false). Set true to get full DoD content.", "default": false},
			},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleTaskList(store, args)
	})

	// c4_worker_heartbeat
	reg.Register(mcp.ToolSchema{
		Name:        "c4_worker_heartbeat",
		Description: "Send a heartbeat for a worker to prevent stale task reassignment. Call every heartbeat_interval_sec seconds while doing long operations (file editing, builds).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"worker_id": map[string]any{
					"type":        "string",
					"description": "Worker ID (same as used in c4_get_task)",
				},
			},
			"required": []string{"worker_id"},
		},
	}, func(args json.RawMessage) (any, error) {
		var p struct {
			WorkerID string `json:"worker_id"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, fmt.Errorf("invalid args: %w", err)
		}
		if p.WorkerID == "" {
			return nil, fmt.Errorf("worker_id is required")
		}
		hs, ok := store.(HeartbeatStore)
		if !ok {
			return map[string]any{"ok": true, "note": "heartbeat not supported by store"}, nil
		}
		n, err := hs.WorkerHeartbeat(p.WorkerID)
		if err != nil {
			return nil, fmt.Errorf("heartbeat failed: %w", err)
		}
		if n == 0 {
			return map[string]any{"ok": false, "worker_id": p.WorkerID, "tasks_updated": 0}, nil
		}
		return map[string]any{"ok": true, "worker_id": p.WorkerID, "tasks_updated": n}, nil
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

	var assignment *TaskAssignment
	var err error
	if args.TaskID != "" {
		// Specific task requested — claim then enrich via SQLiteStore
		if ss, ok := store.(*SQLiteStore); ok {
			assignment, err = ss.AssignSpecificTask(args.WorkerID, args.TaskID)
		} else {
			return nil, fmt.Errorf("task_id parameter requires SQLiteStore backend")
		}
	} else {
		assignment, err = store.AssignTask(args.WorkerID)
	}
	if err != nil {
		return nil, fmt.Errorf("assigning task: %w", err)
	}

	if assignment == nil {
		// No tasks available — include diagnostic info
		reason := "no ready tasks"
		if ss, ok := store.(*SQLiteStore); ok {
			reason = ss.DiagnoseNoTask()
		}
		return map[string]any{"reason": reason}, nil
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
	// validation_results is optional; if provided, must be non-empty, all must pass.
	if args.ValidationResults != nil && len(args.ValidationResults) == 0 {
		return nil, fmt.Errorf("empty validation_results not allowed: omit the field to skip validation")
	}
	for _, r := range args.ValidationResults {
		if r.Status != "pass" && r.Status != "fail" {
			return nil, fmt.Errorf("validation_results[%s].status %q is invalid: must be \"pass\" or \"fail\"", r.Name, r.Status)
		}
		if r.Status == "fail" {
			return nil, fmt.Errorf("cannot submit: validation %q failed — fix and resubmit, or call c4_mark_blocked if unrecoverable", r.Name)
		}
	}

	// Polish gate enforcement: if diff is large enough, require a polish gate.
	if ss, ok := store.(*SQLiteStore); ok {
		if err := checkPolishGate(ss, args.TaskID, args.CommitSHA); err != nil {
			return nil, err
		}
	}

	result, err := store.SubmitTask(args.TaskID, args.WorkerID, args.CommitSHA, args.Handoff, args.ValidationResults)
	if err != nil {
		return nil, fmt.Errorf("submitting task: %w", err)
	}

	// Async persona learning: extract coding patterns from this commit (non-blocking, non-fatal).
	if ss, ok := store.(*SQLiteStore); ok && ss.projectRoot != "" {
		commitSHA := args.CommitSHA
		projectRoot := ss.projectRoot
		taskID := args.TaskID
		go func() {
			commitRange := commitSHA + "~1.." + commitSHA
			if err := runPersonaLearnFromDiff(projectRoot, commitRange); err != nil {
				slog.Warn("persona: learn_from_diff failed", "commit", commitSHA, "error", err)
			}

			// Stage 3: Warning validity feedback — if a review was approved,
			// record that injected warnings helped prevent rejection.
			_, _, _, taskType := task.ParseTaskID(taskID)
			if taskType == task.TypeReview && ss.knowledgeWriter != nil && args.Handoff != "" {
				// Parse handoff JSON string for verdict
				var handoff map[string]any
				if json.Unmarshal([]byte(args.Handoff), &handoff) == nil {
					if verdict, _ := handoff["verdict"].(string); verdict == "approved" {
						// Find impl task scope from review task's dependencies
						_, baseNum, ver, _ := task.ParseTaskID(taskID)
						implID := fmt.Sprintf("T-%s-%d", baseNum, ver)
						if t, err := ss.GetTask(implID); err == nil && t != nil && t.Scope != "" {
							feedbackMeta := map[string]any{
								"title":    fmt.Sprintf("Warning feedback: %s approved (scope: %s)", taskID, t.Scope),
								"doc_type": "warning-feedback",
								"tags":     []string{"warning-feedback", t.Scope, "approved"},
							}
							feedbackBody := fmt.Sprintf("Review %s approved for scope %s. Injected warnings were effective.", taskID, t.Scope)
							ss.knowledgeWriter.CreateExperiment(feedbackMeta, feedbackBody)
						}
					}
				}
			}
		}()
	}

	return result, nil
}

// checkPolishGate enforces the polish gate rule:
// if the commit diff size >= polish_threshold, a polish=done gate must be present.
func checkPolishGate(ss *SQLiteStore, taskID, commitSHA string) error {
	if ss.config == nil || ss.projectRoot == "" {
		return nil
	}
	threshold := ss.config.GetConfig().Run.PolishThreshold
	if threshold <= 0 {
		return nil
	}

	lines := diffStatLines(ss.projectRoot, commitSHA)
	if lines < threshold {
		return nil
	}

	// Get task's updated_at (when it became in_progress) to scope gate lookup.
	t, err := ss.GetTask(taskID)
	if err != nil {
		return nil // best-effort: don't block submit on lookup failure
	}

	ok, err := ss.HasGateDone("polish", t.UpdatedAt)
	if err != nil {
		return nil // best-effort: don't block submit on DB error
	}
	if !ok {
		return fmt.Errorf("polish gate required: run polish loop or call c4_record_gate")
	}
	return nil
}

// checkRefineGate enforces the refine gate rule on c4_add_todo:
// if pending task count (last 10 min) >= refine_threshold, a refine=done gate must be present.
func checkRefineGate(s *SQLiteStore) error {
	if s.config == nil {
		return nil
	}
	threshold := s.config.GetConfig().Run.RefineThreshold
	if threshold <= 0 {
		return nil
	}

	// Count pending tasks created in the last 10 minutes.
	sinceTime := time.Now().Add(-10 * time.Minute).UTC().Format("2006-01-02 15:04:05")
	var count int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM c4_tasks WHERE status='pending' AND created_at >= ?`,
		sinceTime,
	).Scan(&count); err != nil {
		return nil // best-effort: don't block add on DB error
	}

	if count < threshold {
		return nil
	}

	ok, err := s.HasGateDone("refine", sinceTime)
	if err != nil {
		return nil // best-effort: don't block add on DB error
	}
	if !ok {
		return fmt.Errorf("refine gate required: run critique loop (Phase 4.5) or call c4_record_gate")
	}
	return nil
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

	// Enforce refine gate when pending batch is large.
	if ss, ok := store.(*SQLiteStore); ok {
		if err := checkRefineGate(ss); err != nil {
			return nil, err
		}
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

	// Async: record scope-warning for learn loop
	if ss, ok := store.(*SQLiteStore); ok && ss.knowledgeWriter != nil {
		reviewTaskID := args.ReviewTaskID
		comments := args.Comments
		changes := args.RequiredChanges
		go func() {
			// Derive impl task ID: R-XXX-0 → T-XXX-0 (replace R- prefix with T-)
			_, baseID, version, _ := task.ParseTaskID(reviewTaskID)
			implTaskID := fmt.Sprintf("T-%s-%d", baseID, version)

			scope := ""
			if t, err := ss.GetTask(implTaskID); err == nil && t != nil {
				scope = t.Scope
			}

			// Build content
			var b strings.Builder
			fmt.Fprintf(&b, "## Rejection Reason\n%s\n\n## Required Changes\n", comments)
			for _, c := range changes {
				fmt.Fprintf(&b, "- %s\n", c)
			}
			content := b.String()

			title := fmt.Sprintf("Review rejection: %s", reviewTaskID)
			tags := []string{"scope-warning"}
			if scope != "" {
				tags = append(tags, scope)
			}
			tags = append(tags, reviewTaskID)

			metadata := map[string]any{
				"title":    title,
				"doc_type": "scope-warning",
				"tags":     tags,
				"task_id":  reviewTaskID,
			}
			if _, err := ss.knowledgeWriter.CreateExperiment(metadata, content); err != nil {
				slog.Warn("learn-loop: scope-warning record failed", "error", err, "task", reviewTaskID)
			}

			// Stage 2: Pattern promotion — 3+ warnings on same scope → validation-rule
			if scope != "" && ss.knowledgeSearch != nil {
				warnings, werr := ss.knowledgeSearch.Search("scope-warning "+scope, 10, nil)
				if werr == nil && len(warnings) >= 3 {
					// Check if validation-rule already exists for this scope
					existing, _ := ss.knowledgeSearch.Search("validation-rule "+scope, 1, nil)
					if len(existing) == 0 {
						ruleBody := fmt.Sprintf("## Auto-promoted Validation Rule\nScope: %s\nPromoted after %d review rejections.\n\n## Common Issues\n", scope, len(warnings))
						for i, w := range warnings {
							if i >= 5 {
								break
							}
							if ss.knowledgeReader != nil {
								if body, berr := ss.knowledgeReader.GetBody(w.ID); berr == nil && body != "" {
									ruleBody += fmt.Sprintf("- %s\n", firstLine(body))
								}
							}
						}
						ruleMeta := map[string]any{
							"title":    fmt.Sprintf("Validation rule: %s (auto-promoted)", scope),
							"doc_type": "validation-rule",
							"tags":     []string{"validation-rule", scope, "auto-promoted"},
						}
						if _, err := ss.knowledgeWriter.CreateExperiment(ruleMeta, ruleBody); err != nil {
							slog.Warn("learn-loop: validation-rule promotion failed", "error", err, "scope", scope)
						} else {
							slog.Info("learn-loop: scope-warning promoted to validation-rule", "scope", scope, "warning_count", len(warnings))
						}
					}
				}
			}
		}()
	}

	return result, nil
}

// firstLine returns the first non-empty line from s (for log summaries).
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			return line
		}
	}
	if len(s) > 80 {
		return s[:80]
	}
	return s
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
	Status     string `json:"status"`
	Domain     string `json:"domain"`
	WorkerID   string `json:"worker_id"`
	Limit      int    `json:"limit"`
	IncludeDoD *bool  `json:"include_dod"`
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

	includeDoD := args.IncludeDoD != nil && *args.IncludeDoD
	var result any = tasks
	if !includeDoD {
		brief := make([]map[string]any, len(tasks))
		for i, t := range tasks {
			b, _ := json.Marshal(t)
			var m map[string]any
			_ = json.Unmarshal(b, &m)
			delete(m, "dod")
			brief[i] = m
		}
		result = brief
	}

	return map[string]any{
		"tasks":    result,
		"total":    total,
		"filtered": len(tasks),
	}, nil
}

// AdminStore is the minimal interface required by RegisterTaskAdminHandlers.
// Implemented by SQLiteStore but separated to avoid polluting the main Store interface.
type AdminStore interface {
	StaleTasks(minMinutes int) ([]Task, error)
	ResetTask(taskID string) error
}

// RegisterTaskAdminHandlers registers c4_stale_tasks and c4_reset_task recovery tools.
// These are Option-B manual recovery tools for stuck workers.
func RegisterTaskAdminHandlers(reg *mcp.Registry, s AdminStore) {
	// c4_stale_tasks — list in_progress tasks that haven't been updated recently.
	reg.Register(mcp.ToolSchema{
		Name:        "c4_stale_tasks",
		Description: "List in_progress tasks that haven't been updated recently (possible stuck workers). Default threshold is 30 minutes.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"min_minutes": map[string]any{
					"type":        "integer",
					"description": "Age threshold in minutes (default 30)",
				},
			},
		},
	}, func(args json.RawMessage) (any, error) {
		var a struct {
			MinMinutes int `json:"min_minutes"`
		}
		_ = json.Unmarshal(args, &a)
		if a.MinMinutes <= 0 {
			a.MinMinutes = 30
		}
		tasks, err := s.StaleTasks(a.MinMinutes)
		if err != nil {
			return nil, fmt.Errorf("listing stale tasks: %w", err)
		}
		return map[string]any{
			"stale_tasks":  tasks,
			"count":        len(tasks),
			"threshold_min": a.MinMinutes,
		}, nil
	})

	// c4_reset_task — reset a single stuck in_progress task back to pending.
	reg.Register(mcp.ToolSchema{
		Name:        "c4_reset_task",
		Description: "Reset a stuck in_progress task back to pending so a new worker can pick it up. Only works on in_progress tasks.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_id": map[string]any{
					"type":        "string",
					"description": "ID of the stuck task to reset",
				},
			},
			"required": []string{"task_id"},
		},
	}, func(args json.RawMessage) (any, error) {
		var a struct {
			TaskID string `json:"task_id"`
		}
		if err := json.Unmarshal(args, &a); err != nil || a.TaskID == "" {
			return nil, fmt.Errorf("task_id required")
		}
		if err := s.ResetTask(a.TaskID); err != nil {
			return nil, err
		}
		return map[string]any{
			"task_id": a.TaskID,
			"status":  "reset to pending",
		}, nil
	})
}

// runPersonaLearnFromDiff extracts coding patterns from the given git commit range
// and appends them to .c4/souls/<user>/raw_patterns.json.
// This is the lightweight core of personaLearnFromDiffHandler (no LLM/ontology).
func runPersonaLearnFromDiff(projectRoot, commitRange string) error {
	// Get changed files
	cmd := exec.CommandContext(context.Background(), "git", "diff", "--name-only", commitRange)
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("git diff --name-only: %w", err)
	}

	files := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(files) == 0 || (len(files) == 1 && files[0] == "") {
		return nil // no changed files — not an error
	}

	parts := strings.SplitN(commitRange, "..", 2)
	if len(parts) != 2 {
		return fmt.Errorf("commitRange must be 'before..after' format, got: %s", commitRange)
	}
	beforeRef, afterRef := parts[0], parts[1]

	var allPatterns []persona.EditPattern
	for _, file := range files {
		if file == "" || !isCodeFile(file) {
			continue
		}
		before, _ := exec.CommandContext(context.Background(), "git", "show", beforeRef+":"+file).CombinedOutput()
		after, _ := exec.CommandContext(context.Background(), "git", "show", afterRef+":"+file).CombinedOutput()

		if len(before) == 0 && len(after) == 0 {
			continue
		}
		patterns := persona.AnalyzeEdits(string(before), string(after))
		for i := range patterns {
			patterns[i].Description = fmt.Sprintf("[%s] %s", file, patterns[i].Description)
		}
		allPatterns = append(allPatterns, patterns...)
	}

	if len(allPatterns) == 0 {
		return nil
	}

	username := os.Getenv("USER")
	if username == "" {
		username = "default"
	}

	relPath := fmt.Sprintf(".c4/souls/%s/raw_patterns.json", username)
	patternsPath := relPath
	if projectRoot != "" {
		patternsPath = projectRoot + "/" + relPath
	}

	var existing []persona.EditPattern
	if data, readErr := os.ReadFile(patternsPath); readErr == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &existing)
	}

	if len(existing) == 0 {
		if err := persona.SeedFromGlobal(patternsPath, username); err != nil {
			slog.Warn("persona: SeedFromGlobal failed", "error", err)
		}
		if data, readErr := os.ReadFile(patternsPath); readErr == nil && len(data) > 0 {
			_ = json.Unmarshal(data, &existing)
		}
	}

	existing = append(existing, allPatterns...)
	_ = os.MkdirAll(strings.TrimSuffix(patternsPath, "raw_patterns.json"), 0755)
	data, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(patternsPath, data, 0644); err != nil {
		return fmt.Errorf("write raw_patterns: %w", err)
	}

	if err := persona.MergeToGlobal(patternsPath, username); err != nil {
		slog.Warn("persona: MergeToGlobal failed", "error", err)
	}

	slog.Info("persona: learn_from_diff complete", "patterns", len(allPatterns), "commit_range", commitRange)
	return nil
}
