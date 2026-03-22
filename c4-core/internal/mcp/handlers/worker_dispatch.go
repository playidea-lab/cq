//go:build hub

package handlers

import (
	"encoding/json"
	"fmt"

	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterDispatchHandler registers the hub_dispatch_job MCP tool.
// Hub calls this tool to push-dispatch a job to the worker.
func RegisterDispatchHandler(reg *mcp.Registry) {
	reg.Register(mcp.ToolSchema{
		Name:        "hub_dispatch_job",
		Description: "Receive a job dispatched by Hub via push. Immediately accepts and returns job_id.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_id":     map[string]any{"type": "string", "description": "Job identifier assigned by Hub"},
				"lease_id":   map[string]any{"type": "string", "description": "Lease ID for the job"},
				"command":    map[string]any{"type": "string", "description": "Command to execute"},
				"workdir":    map[string]any{"type": "string", "description": "Working directory on the worker"},
				"env":        map[string]any{"type": "object", "description": "Environment variables"},
				"tags":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Job tags"},
				"params":     map[string]any{"type": "object", "description": "Additional job parameters"},
				"capability": map[string]any{"type": "string", "description": "Required worker capability"},
				"hub_url":    map[string]any{"type": "string", "description": "Hub URL for callbacks"},
			},
			"required": []string{"job_id"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleDispatchJob(raw)
	})
}

func handleDispatchJob(raw json.RawMessage) (any, error) {
	var params struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if params.JobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}
	return map[string]any{
		"status": "accepted",
		"job_id": params.JobID,
	}, nil
}
