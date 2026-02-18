package handlers

import (
	"encoding/json"
	"fmt"

	"github.com/changmin/c4-core/internal/mcp"
)

// claimArgs is the input for c4_claim.
type claimArgs struct {
	TaskID string `json:"task_id"`
}

// reportArgs is the input for c4_report.
type reportArgs struct {
	TaskID       string   `json:"task_id"`
	Summary      string   `json:"summary"`
	FilesChanged []string `json:"files_changed,omitempty"`
}

// checkpointArgs is the input for c4_checkpoint.
type checkpointArgs struct {
	CheckpointID    string   `json:"checkpoint_id"`
	Decision        string   `json:"decision"` // "APPROVE", "REQUEST_CHANGES", "REPLAN"
	Notes           string   `json:"notes"`
	RequiredChanges []string `json:"required_changes,omitempty"`
	TargetTaskID    string   `json:"target_task_id,omitempty"`   // explicit linkage for attribution (no latest-in_progress heuristic)
	TargetReviewID  string   `json:"target_review_id,omitempty"` // explicit linkage for attribution
}

// RegisterTrackingHandlers registers direct-mode and supervisor tools on the registry.
func RegisterTrackingHandlers(reg *mcp.Registry, store Store) {
	// c4_claim
	reg.Register(mcp.ToolSchema{
		Name:        "c4_claim",
		Description: "Claim a task for direct execution by the main session (no worker protocol)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_id": map[string]any{
					"type":        "string",
					"description": "ID of the task to claim",
				},
			},
			"required": []string{"task_id"},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleClaim(store, args)
	})

	// c4_report
	reg.Register(mcp.ToolSchema{
		Name:        "c4_report",
		Description: "Report task completion for direct mode",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_id": map[string]any{
					"type":        "string",
					"description": "ID of the completed task",
				},
				"summary": map[string]any{
					"type":        "string",
					"description": "Summary of work done",
				},
				"files_changed": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "List of files changed during the work",
				},
			},
			"required": []string{"task_id", "summary"},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleReport(store, args)
	})

	// c4_checkpoint
	reg.Register(mcp.ToolSchema{
		Name:        "c4_checkpoint",
		Description: "Record supervisor checkpoint decision. Use target_task_id/target_review_id for explicit attribution (no latest-in_progress heuristic).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"checkpoint_id":    map[string]any{"type": "string"},
				"decision":         map[string]any{"type": "string", "enum": []string{"APPROVE", "REQUEST_CHANGES", "REPLAN"}},
				"notes":            map[string]any{"type": "string"},
				"required_changes": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "List of required changes (for REQUEST_CHANGES)"},
				"target_task_id":   map[string]any{"type": "string", "description": "Explicit target implementation task for attribution (optional)"},
				"target_review_id": map[string]any{"type": "string", "description": "Explicit target review task for attribution (optional)"},
			},
			"required": []string{"checkpoint_id", "decision", "notes"},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleCheckpoint(store, args)
	})
}

func handleClaim(store Store, rawArgs json.RawMessage) (any, error) {
	var args claimArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("parsing arguments: %w", err)
	}

	if args.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}

	task, err := store.ClaimTask(args.TaskID)
	if err != nil {
		return nil, fmt.Errorf("claiming task: %w", err)
	}

	result := map[string]any{
		"success": true,
		"task_id": task.ID,
		"title":   task.Title,
		"scope":   task.Scope,
		"dod":     task.DoD,
		"status":  task.Status,
		"message": fmt.Sprintf("Task %s claimed for direct execution", task.ID),
	}

	// Direct-mode operator hints for faster Codex workflows.
	suggestedValidations := []string{"lint", "unit"}
	if ss, ok := store.(*SQLiteStore); ok && ss.config != nil {
		cfg := ss.config.GetConfig()
		suggestedValidations = suggestedValidations[:0]
		if cfg.Validation.Lint != "" {
			suggestedValidations = append(suggestedValidations, "lint")
		}
		if cfg.Validation.Unit != "" {
			suggestedValidations = append(suggestedValidations, "unit")
		}
		if len(suggestedValidations) == 0 {
			suggestedValidations = []string{"lint", "unit"}
		}
	}
	result["suggested_validations"] = suggestedValidations
	result["recommended_commit_message"] = fmt.Sprintf("feat: %s (%s)", task.Title, task.ID)
	result["report_template"] = map[string]any{
		"summary_format": "What changed + Why + Validation result",
		"files_changed":  []string{"path/to/file1", "path/to/file2"},
	}
	result["next_steps"] = []string{
		"Implement requested changes in claimed scope",
		fmt.Sprintf("Run c4_run_validation with %v", suggestedValidations),
		"Commit your changes",
		"Call c4_report with task_id, summary, files_changed",
	}

	// Best-effort: enrich with Twin context
	if ss, ok := store.(*SQLiteStore); ok {
		if tc := ss.BuildTwinContext(task); tc != nil {
			result["twin_context"] = tc
		}
	}

	return result, nil
}

func handleReport(store Store, rawArgs json.RawMessage) (any, error) {
	var args reportArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("parsing arguments: %w", err)
	}

	if args.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	if args.Summary == "" {
		return nil, fmt.Errorf("summary is required")
	}

	if err := store.ReportTask(args.TaskID, args.Summary, args.FilesChanged); err != nil {
		return nil, fmt.Errorf("reporting task: %w", err)
	}

	return map[string]any{
		"success": true,
		"task_id": args.TaskID,
		"status":  "done",
		"message": fmt.Sprintf("Task %s reported as complete", args.TaskID),
	}, nil
}

func handleCheckpoint(store Store, rawArgs json.RawMessage) (any, error) {
	var args checkpointArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("parsing arguments: %w", err)
	}

	if args.CheckpointID == "" {
		return nil, fmt.Errorf("checkpoint_id is required")
	}
	if args.Decision == "" {
		return nil, fmt.Errorf("decision is required")
	}
	if args.Notes == "" {
		return nil, fmt.Errorf("notes is required")
	}

	// Validate decision value
	switch args.Decision {
	case "APPROVE", "REQUEST_CHANGES", "REPLAN":
		// valid
	default:
		return nil, fmt.Errorf("invalid decision: %s (must be APPROVE, REQUEST_CHANGES, or REPLAN)", args.Decision)
	}

	cpResult, err := store.Checkpoint(args.CheckpointID, args.Decision, args.Notes, args.RequiredChanges, args.TargetTaskID, args.TargetReviewID)
	if err != nil {
		return nil, fmt.Errorf("recording checkpoint: %w", err)
	}

	// Enrich with Twin review + strategic review lenses
	result := map[string]any{
		"success":     cpResult.Success,
		"next_action": cpResult.NextAction,
		"message":     cpResult.Message,
		"review_lenses": BuildCheckpointReviewPrompt(),
	}
	if ss, ok := store.(*SQLiteStore); ok {
		if tr := ss.BuildTwinReview(); tr != nil {
			result["twin_review"] = tr
		}
	}

	return result, nil
}
