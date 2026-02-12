package handlers

import (
	"encoding/json"
	"fmt"

	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterHubHandlers registers c4_hub_* MCP tools.
func RegisterHubHandlers(reg *mcp.Registry, hubClient *hub.Client) {
	// c4_hub_submit — Submit a job to the Hub queue
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_submit",
		Description: "Submit a job to the PiQ Hub queue for remote GPU execution",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":         map[string]any{"type": "string", "description": "Job name"},
				"workdir":      map[string]any{"type": "string", "description": "Working directory on the worker"},
				"command":      map[string]any{"type": "string", "description": "Command to execute"},
				"env":          map[string]any{"type": "object", "description": "Environment variables"},
				"tags":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Job tags"},
				"requires_gpu": map[string]any{"type": "boolean", "description": "Whether GPU is required (default: true)"},
				"priority":     map[string]any{"type": "integer", "description": "Priority (-100 to 100, default: 0)"},
				"exp_id":       map[string]any{"type": "string", "description": "Experiment ID to link"},
				"memo":         map[string]any{"type": "string", "description": "Experiment memo/hypothesis"},
				"timeout_sec":  map[string]any{"type": "integer", "description": "Timeout in seconds"},
			},
			"required": []string{"name", "workdir", "command"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubSubmit(hubClient, raw)
	})

	// c4_hub_status — Get job status
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_status",
		Description: "Get status of a Hub job",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_id": map[string]any{"type": "string", "description": "Job ID"},
			},
			"required": []string{"job_id"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubStatus(hubClient, raw)
	})

	// c4_hub_list — List jobs
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_list",
		Description: "List Hub jobs with optional status filter",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status": map[string]any{
					"type":        "string",
					"enum":        []string{"QUEUED", "RUNNING", "SUCCEEDED", "FAILED", "CANCELLED"},
					"description": "Filter by status",
				},
				"limit": map[string]any{"type": "integer", "description": "Max results (default: 50)"},
			},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubList(hubClient, raw)
	})

	// c4_hub_cancel — Cancel a job
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_cancel",
		Description: "Cancel a queued or running Hub job",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_id": map[string]any{"type": "string", "description": "Job ID to cancel"},
			},
			"required": []string{"job_id"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubCancel(hubClient, raw)
	})

	// c4_hub_workers — List workers
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_workers",
		Description: "List workers connected to the PiQ Hub with GPU status",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(_ json.RawMessage) (any, error) {
		return handleHubWorkers(hubClient)
	})

	// c4_hub_stats — Queue statistics
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_stats",
		Description: "Get Hub queue statistics (queued/running/succeeded/failed counts)",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(_ json.RawMessage) (any, error) {
		return handleHubStats(hubClient)
	})

	// c4_hub_metrics — Get metrics for a job
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_metrics",
		Description: "Get training metrics for a Hub job",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_id": map[string]any{"type": "string", "description": "Job ID"},
				"limit":  map[string]any{"type": "integer", "description": "Max metric points (default: 100)"},
			},
			"required": []string{"job_id"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubMetrics(hubClient, raw)
	})

	// c4_hub_log_metrics — Log metrics for a job
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_log_metrics",
		Description: "Log training metrics for a running Hub job",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_id":  map[string]any{"type": "string", "description": "Job ID"},
				"step":    map[string]any{"type": "integer", "description": "Training step (0-indexed)"},
				"metrics": map[string]any{"type": "object", "description": "Metric name-value pairs (e.g. {\"loss\": 0.5})"},
			},
			"required": []string{"job_id", "step", "metrics"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubLogMetrics(hubClient, raw)
	})

	// c4_hub_upload — Upload artifact for a job
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_upload",
		Description: "Upload a local file as a Hub job artifact (presigned URL + SHA256 verification)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_id":       map[string]any{"type": "string", "description": "Job ID"},
				"local_path":   map[string]any{"type": "string", "description": "Local file path to upload"},
				"storage_path": map[string]any{"type": "string", "description": "Storage path (e.g. outputs/model.pt)"},
			},
			"required": []string{"job_id", "local_path", "storage_path"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubUpload(hubClient, raw)
	})

	// c4_hub_download — Download artifact from a job
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_download",
		Description: "Download a Hub job artifact to a local file",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_id":     map[string]any{"type": "string", "description": "Job ID"},
				"name":       map[string]any{"type": "string", "description": "Artifact name"},
				"local_path": map[string]any{"type": "string", "description": "Local destination path"},
			},
			"required": []string{"job_id", "name", "local_path"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubDownload(hubClient, raw)
	})
}

// =========================================================================
// Handler implementations
// =========================================================================

func handleHubSubmit(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		Name        string            `json:"name"`
		Workdir     string            `json:"workdir"`
		Command     string            `json:"command"`
		Env         map[string]string `json:"env"`
		Tags        []string          `json:"tags"`
		RequiresGPU *bool             `json:"requires_gpu"`
		Priority    int               `json:"priority"`
		ExpID       string            `json:"exp_id"`
		Memo        string            `json:"memo"`
		TimeoutSec  int               `json:"timeout_sec"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.Name == "" || params.Workdir == "" || params.Command == "" {
		return nil, fmt.Errorf("name, workdir, and command are required")
	}

	requiresGPU := true
	if params.RequiresGPU != nil {
		requiresGPU = *params.RequiresGPU
	}

	resp, err := client.SubmitJob(&hub.JobSubmitRequest{
		Name:        params.Name,
		Workdir:     params.Workdir,
		Command:     params.Command,
		Env:         params.Env,
		Tags:        params.Tags,
		RequiresGPU: requiresGPU,
		Priority:    params.Priority,
		ExpID:       params.ExpID,
		Memo:        params.Memo,
		TimeoutSec:  params.TimeoutSec,
	})
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"job_id":         resp.JobID,
		"status":         resp.Status,
		"queue_position": resp.QueuePosition,
	}, nil
}

func handleHubStatus(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.JobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}

	job, err := client.GetJob(params.JobID)
	if err != nil {
		return nil, err
	}

	return job, nil
}

func handleHubList(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		Status string `json:"status"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.Limit == 0 {
		params.Limit = 50
	}

	jobs, err := client.ListJobs(params.Status, params.Limit)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"jobs":  jobs,
		"count": len(jobs),
	}, nil
}

func handleHubCancel(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.JobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}

	if err := client.CancelJob(params.JobID); err != nil {
		return nil, err
	}

	return map[string]any{
		"cancelled": true,
		"job_id":    params.JobID,
	}, nil
}

func handleHubWorkers(client *hub.Client) (any, error) {
	workers, err := client.ListWorkers()
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"workers": workers,
		"count":   len(workers),
	}, nil
}

func handleHubStats(client *hub.Client) (any, error) {
	stats, err := client.GetQueueStats()
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"queued":    stats.Queued,
		"running":   stats.Running,
		"succeeded": stats.Succeeded,
		"failed":    stats.Failed,
		"cancelled": stats.Cancelled,
		"total":     stats.Queued + stats.Running + stats.Succeeded + stats.Failed + stats.Cancelled,
	}, nil
}

func handleHubMetrics(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		JobID string `json:"job_id"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.JobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}
	if params.Limit == 0 {
		params.Limit = 100
	}

	resp, err := client.GetMetrics(params.JobID, params.Limit)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"job_id":      resp.JobID,
		"metrics":     resp.Metrics,
		"total_steps": resp.TotalSteps,
	}, nil
}

func handleHubLogMetrics(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		JobID   string         `json:"job_id"`
		Step    int            `json:"step"`
		Metrics map[string]any `json:"metrics"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.JobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}
	if params.Metrics == nil {
		return nil, fmt.Errorf("metrics is required")
	}

	if err := client.LogMetrics(params.JobID, params.Step, params.Metrics); err != nil {
		return nil, err
	}

	return map[string]any{
		"logged": true,
		"job_id": params.JobID,
		"step":   params.Step,
	}, nil
}

func handleHubUpload(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		JobID       string `json:"job_id"`
		LocalPath   string `json:"local_path"`
		StoragePath string `json:"storage_path"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.JobID == "" || params.LocalPath == "" || params.StoragePath == "" {
		return nil, fmt.Errorf("job_id, local_path, and storage_path are required")
	}

	resp, err := client.UploadArtifact(params.JobID, params.StoragePath, params.LocalPath)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"uploaded":    true,
		"artifact_id": resp.ArtifactID,
		"job_id":      params.JobID,
		"path":        params.StoragePath,
	}, nil
}

func handleHubDownload(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		JobID     string `json:"job_id"`
		Name      string `json:"name"`
		LocalPath string `json:"local_path"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.JobID == "" || params.Name == "" || params.LocalPath == "" {
		return nil, fmt.Errorf("job_id, name, and local_path are required")
	}

	if err := client.DownloadArtifact(params.JobID, params.Name, params.LocalPath); err != nil {
		return nil, err
	}

	return map[string]any{
		"downloaded": true,
		"job_id":     params.JobID,
		"name":       params.Name,
		"local_path": params.LocalPath,
	}, nil
}
