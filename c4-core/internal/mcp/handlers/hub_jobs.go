//go:build c5_hub

package handlers

import (
	"encoding/json"
	"fmt"

	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/mcp"
)

var hubEventPub eventbus.Publisher
var hubProjectID string

// SetHubEventBus sets the EventBus publisher and project ID for Hub event publishing.
func SetHubEventBus(pub eventbus.Publisher, projectID string) {
	hubEventPub = pub
	hubProjectID = projectID
}

// GetHubEventPub returns the current Hub EventBus publisher, or NoopPublisher if not set.
func GetHubEventPub() eventbus.Publisher {
	if hubEventPub != nil {
		return hubEventPub
	}
	return eventbus.NoopPublisher{}
}

func registerHubJobHandlers(reg *mcp.Registry, hubClient *hub.Client) {
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
				"input_artifacts": map[string]any{
					"type":        "array",
					"description": "Input artifact paths to download before job execution",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"path":       map[string]any{"type": "string", "description": "Hub artifact path"},
							"local_path": map[string]any{"type": "string", "description": "Local destination path on worker"},
							"required":   map[string]any{"type": "boolean", "description": "Fail if artifact missing"},
						},
						"required": []string{"path"},
					},
				},
				"output_artifacts": map[string]any{
					"type":        "array",
					"description": "Output artifact paths to upload after job completion",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"path":       map[string]any{"type": "string", "description": "Hub artifact path"},
							"local_path": map[string]any{"type": "string", "description": "Local source path on worker"},
						},
						"required": []string{"path"},
					},
				},
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

	// c4_hub_watch — Watch job logs (tail)
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_watch",
		Description: "Watch job logs. Returns the last N lines of output from a running or completed job",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_id": map[string]any{"type": "string", "description": "Job ID"},
				"tail":   map[string]any{"type": "integer", "description": "Number of lines to return (default: 200)"},
				"offset": map[string]any{"type": "integer", "description": "Line offset to start from (default: 0, 0=from end)"},
			},
			"required": []string{"job_id"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubWatch(hubClient, raw)
	})

	// c4_hub_summary — Get job summary with metrics
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_summary",
		Description: "Get comprehensive job summary including status, duration, metrics, and log tail",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_id": map[string]any{"type": "string", "description": "Job ID"},
			},
			"required": []string{"job_id"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubSummary(hubClient, raw)
	})

	// c4_hub_retry — Retry a failed or cancelled job
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_retry",
		Description: "Retry a failed or cancelled job with the same configuration. Creates a new job",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_id": map[string]any{"type": "string", "description": "Job ID of the failed/cancelled job to retry"},
			},
			"required": []string{"job_id"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubRetry(hubClient, raw)
	})

	// c4_hub_estimate — Get time estimate for a job
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_estimate",
		Description: "Get estimated duration and queue wait time for a job based on historical data",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_id": map[string]any{"type": "string", "description": "Job ID"},
			},
			"required": []string{"job_id"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubEstimate(hubClient, raw)
	})

	// c4_hub_lease_renew — Renew a job lease to prevent expiry.
	// Lease renewal policy: renew when ≥60 seconds remain before expiry to avoid
	// the Hub re-queuing the job. A typical lease lasts 5 minutes; renew every
	// ~4 minutes (i.e. when <60 seconds of remaining lease time is detected) or
	// proactively on a fixed interval shorter than the lease TTL. On failure,
	// retry up to 3 times with exponential backoff before aborting the job.
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_lease_renew",
		Description: "Renew a job lease to prevent the Hub from re-queuing the job. Call periodically while executing a long-running job (recommended: every 4 minutes for a 5-minute lease). Returns the new expiry timestamp.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"lease_id": map[string]any{"type": "string", "description": "Lease ID returned by c4_worker_standby"},
			},
			"required": []string{"lease_id"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubLeaseRenew(hubClient, raw)
	})
}

// =========================================================================
// Job handler implementations
// =========================================================================

func handleHubSubmit(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		Name            string            `json:"name"`
		Workdir         string            `json:"workdir"`
		Command         string            `json:"command"`
		Env             map[string]string `json:"env"`
		Tags            []string          `json:"tags"`
		RequiresGPU     *bool             `json:"requires_gpu"`
		Priority        int               `json:"priority"`
		ExpID           string            `json:"exp_id"`
		Memo            string            `json:"memo"`
		TimeoutSec      int               `json:"timeout_sec"`
		InputArtifacts  []hub.ArtifactRef `json:"input_artifacts"`
		OutputArtifacts []hub.ArtifactRef `json:"output_artifacts"`
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
		Name:            params.Name,
		Workdir:         params.Workdir,
		Command:         params.Command,
		Env:             params.Env,
		Tags:            params.Tags,
		RequiresGPU:     requiresGPU,
		Priority:        params.Priority,
		ExpID:           params.ExpID,
		Memo:            params.Memo,
		TimeoutSec:      params.TimeoutSec,
		InputArtifacts:  params.InputArtifacts,
		OutputArtifacts: params.OutputArtifacts,
	})
	if err != nil {
		return nil, err
	}
	if hubEventPub != nil {
		payload, _ := json.Marshal(map[string]any{
			"job_id": resp.JobID, "name": params.Name, "command": params.Command,
			"workdir": params.Workdir, "tags": params.Tags, "priority": params.Priority,
		})
		hubEventPub.PublishAsync("hub.job.submitted", "c4.hub", payload, hubProjectID)
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
	if hubEventPub != nil {
		payload, _ := json.Marshal(map[string]any{"job_id": params.JobID, "cancelled_by": "mcp"})
		hubEventPub.PublishAsync("hub.job.cancelled", "c4.hub", payload, hubProjectID)
	}
	return map[string]any{
		"cancelled": true,
		"job_id":    params.JobID,
	}, nil
}

func handleHubSummary(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.JobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}

	resp, err := client.GetJobSummary(params.JobID)
	if err != nil {
		return nil, err
	}

	result := map[string]any{
		"job_id": resp.JobID,
		"name":   resp.Name,
		"status": resp.Status,
	}
	if resp.DurationSec != nil {
		result["duration_seconds"] = *resp.DurationSec
	}
	if resp.ExitCode != nil {
		result["exit_code"] = *resp.ExitCode
	}
	if resp.FailureReason != "" {
		result["failure_reason"] = resp.FailureReason
	}
	if len(resp.Metrics) > 0 {
		result["metrics"] = resp.Metrics
	}
	if len(resp.LogTail) > 0 {
		result["log_tail"] = resp.LogTail
	}
	return result, nil
}

func handleHubRetry(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.JobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}

	resp, err := client.RetryJob(params.JobID)
	if err != nil {
		return nil, err
	}

	if hubEventPub != nil {
		payload, _ := json.Marshal(map[string]any{
			"original_job_id": resp.OriginalJobID, "new_job_id": resp.NewJobID,
		})
		hubEventPub.PublishAsync("hub.job.retried", "c4.hub", payload, hubProjectID)
	}

	return map[string]any{
		"new_job_id":      resp.NewJobID,
		"status":          resp.Status,
		"original_job_id": resp.OriginalJobID,
	}, nil
}

func handleHubLeaseRenew(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		LeaseID string `json:"lease_id"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.LeaseID == "" {
		return nil, fmt.Errorf("lease_id is required")
	}

	newExpiresAt, err := client.RenewLease(params.LeaseID)
	if err != nil {
		return nil, fmt.Errorf("renew lease: %w", err)
	}

	return map[string]any{
		"renewed":        true,
		"lease_id":       params.LeaseID,
		"new_expires_at": newExpiresAt,
	}, nil
}

func handleHubEstimate(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.JobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}

	resp, err := client.GetJobEstimate(params.JobID)
	if err != nil {
		return nil, err
	}

	result := map[string]any{
		"estimated_duration_sec": resp.EstimatedDurationSec,
		"confidence":             resp.Confidence,
		"method":                 resp.Method,
	}
	if resp.QueueWaitSec > 0 {
		result["queue_wait_sec"] = resp.QueueWaitSec
	}
	if resp.EstimatedStartTime != "" {
		result["estimated_start_time"] = resp.EstimatedStartTime
	}
	if resp.EstimatedEndTime != "" {
		result["estimated_completion_time"] = resp.EstimatedEndTime
	}
	return result, nil
}
