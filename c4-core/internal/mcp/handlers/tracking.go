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
		Description: "Record supervisor checkpoint decision",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"checkpoint_id": map[string]any{"type": "string"},
				"decision": map[string]any{
					"type": "string",
					"enum": []string{"APPROVE", "REQUEST_CHANGES", "REPLAN"},
				},
				"notes": map[string]any{"type": "string"},
				"required_changes": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "List of required changes (for REQUEST_CHANGES)",
				},
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

	return map[string]any{
		"success": true,
		"task_id": task.ID,
		"title":   task.Title,
		"scope":   task.Scope,
		"dod":     task.DoD,
		"status":  task.Status,
		"message": fmt.Sprintf("Task %s claimed for direct execution", task.ID),
	}, nil
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

	result, err := store.Checkpoint(args.CheckpointID, args.Decision, args.Notes, args.RequiredChanges)
	if err != nil {
		return nil, fmt.Errorf("recording checkpoint: %w", err)
	}

	return result, nil
}
