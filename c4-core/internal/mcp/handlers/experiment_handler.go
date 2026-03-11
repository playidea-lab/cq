package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/changmin/c4-core/internal/mcp"
)

// ExperimentStore defines the data access interface for experiment runs.
type ExperimentStore interface {
	// StartRun creates a new experiment run and returns its ID.
	StartRun(ctx context.Context, name, config string) (string, error)
	// RecordCheckpoint records a checkpoint metric and returns true if it's the best so far.
	RecordCheckpoint(ctx context.Context, runID string, metric float64, path string) (bool, error)
	// ShouldContinue returns true if the run has not been cancelled or completed.
	ShouldContinue(ctx context.Context, runID string) (bool, error)
	// CompleteRun marks the run as complete with a final metric.
	CompleteRun(ctx context.Context, runID, status string, finalMetric float64) error
}

// ExperimentHandlers holds dependencies for experiment MCP handlers.
type ExperimentHandlers struct {
	Store           ExperimentStore
	KnowledgeRecord func(ctx context.Context, title, content, domain string) error // nil if knowledge disabled
}

// RegisterExperimentHandlers registers c4_experiment_register, c4_run_complete,
// and c4_run_should_continue MCP tools.
func RegisterExperimentHandlers(reg *mcp.Registry, h ExperimentHandlers) {
	if h.Store == nil {
		return
	}

	reg.RegisterBlocking(mcp.ToolSchema{
		Name:        "c4_experiment_register",
		Description: "Register a new experiment run and return a run_id for checkpoint tracking.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":   map[string]any{"type": "string", "description": "Experiment name"},
				"config": map[string]any{"type": "string", "description": "JSON config snapshot (optional)"},
			},
			"required": []string{"name"},
		},
	}, registerRunHandler(h))

	reg.RegisterBlocking(mcp.ToolSchema{
		Name:        "c4_run_complete",
		Description: "Mark an experiment run complete and auto-bridge results to knowledge store.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"run_id":       map[string]any{"type": "string", "description": "Run ID from c4_experiment_register"},
				"status":       map[string]any{"type": "string", "description": "Completion status: success, failed, cancelled"},
				"final_metric": map[string]any{"type": "number", "description": "Final metric value (e.g. loss, accuracy)"},
				"summary":      map[string]any{"type": "string", "description": "Human-readable summary for knowledge record"},
			},
			"required": []string{"run_id", "status"},
		},
	}, completeRunHandler(h))

	reg.RegisterBlocking(mcp.ToolSchema{
		Name:        "c4_run_should_continue",
		Description: "Check whether an experiment run should continue (not cancelled/completed).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"run_id": map[string]any{"type": "string", "description": "Run ID from c4_experiment_register"},
			},
			"required": []string{"run_id"},
		},
	}, shouldContinueHandler(h))
}

func registerRunHandler(h ExperimentHandlers) mcp.BlockingHandlerFunc {
	return func(ctx context.Context, rawArgs json.RawMessage) (any, error) {
		var args struct {
			Name   string `json:"name"`
			Config string `json:"config"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return map[string]any{"error": "invalid arguments"}, nil
		}
		if args.Name == "" {
			return map[string]any{"error": "name is required"}, nil
		}

		runID, err := h.Store.StartRun(ctx, args.Name, args.Config)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("RegisterRun failed: %v", err)}, nil
		}
		return map[string]any{
			"success":    true,
			"run_id":     runID,
			"registered": time.Now().UTC().Format(time.RFC3339),
		}, nil
	}
}

func completeRunHandler(h ExperimentHandlers) mcp.BlockingHandlerFunc {
	return func(ctx context.Context, rawArgs json.RawMessage) (any, error) {
		var args struct {
			RunID       string  `json:"run_id"`
			Status      string  `json:"status"`
			FinalMetric float64 `json:"final_metric"`
			Summary     string  `json:"summary"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return map[string]any{"error": "invalid arguments"}, nil
		}
		if args.RunID == "" {
			return map[string]any{"error": "run_id is required"}, nil
		}
		if args.Status == "" {
			return map[string]any{"error": "status is required"}, nil
		}

		if err := h.Store.CompleteRun(ctx, args.RunID, args.Status, args.FinalMetric); err != nil {
			return map[string]any{"error": fmt.Sprintf("CompleteRun failed: %v", err)}, nil
		}

		// Auto-bridge: record results to knowledge store asynchronously.
		// Use context.WithoutCancel so the goroutine outlives the MCP request.
		if h.KnowledgeRecord != nil {
			title := fmt.Sprintf("Experiment %s: %s", args.RunID, args.Status)
			content := args.Summary
			if content == "" {
				content = fmt.Sprintf("run_id=%s status=%s final_metric=%g",
					args.RunID, args.Status, args.FinalMetric)
			}
			ctx2 := context.WithoutCancel(ctx)
			go func() {
				if err := h.KnowledgeRecord(ctx2, title, content, "experiment"); err != nil {
					fmt.Printf("c4: experiment auto-bridge failed: %v\n", err)
				}
			}()
		}

		return map[string]any{
			"success": true,
			"run_id":  args.RunID,
			"status":  args.Status,
		}, nil
	}
}

func shouldContinueHandler(h ExperimentHandlers) mcp.BlockingHandlerFunc {
	return func(ctx context.Context, rawArgs json.RawMessage) (any, error) {
		var args struct {
			RunID string `json:"run_id"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return map[string]any{"error": "invalid arguments"}, nil
		}
		if args.RunID == "" {
			return map[string]any{"error": "run_id is required"}, nil
		}

		ok, err := h.Store.ShouldContinue(ctx, args.RunID)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("ShouldContinue failed: %v", err)}, nil
		}
		return map[string]any{
			"run_id":          args.RunID,
			"should_continue": ok,
		}, nil
	}
}
