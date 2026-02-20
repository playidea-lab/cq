package handlers

import (
	"encoding/json"
	"fmt"

	"github.com/changmin/c4-core/internal/hub"
)

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

func handleHubWatch(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		JobID  string `json:"job_id"`
		Tail   int    `json:"tail"`
		Offset int    `json:"offset"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.JobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}
	if params.Tail == 0 {
		params.Tail = 200
	}

	resp, err := client.GetJobLogs(params.JobID, params.Offset, params.Tail)
	if err != nil {
		return nil, err
	}

	// Detect job completion and publish event.
	if hubEventPub != nil {
		job, jobErr := client.GetJob(params.JobID)
		if jobErr == nil {
			switch job.Status {
			case "SUCCEEDED":
				payload, _ := json.Marshal(map[string]any{
					"job_id": params.JobID, "name": job.Name, "duration_sec": 0,
				})
				hubEventPub.PublishAsync("hub.job.completed", "c4.hub", payload, hubProjectID)
			case "FAILED":
				exitCode := 0
				if job.ExitCode != nil {
					exitCode = *job.ExitCode
				}
				payload, _ := json.Marshal(map[string]any{
					"job_id": params.JobID, "name": job.Name, "exit_code": exitCode,
				})
				hubEventPub.PublishAsync("hub.job.failed", "c4.hub", payload, hubProjectID)
			}
		}
	}

	return map[string]any{
		"job_id":      resp.JobID,
		"lines":       resp.Lines,
		"total_lines": resp.TotalLines,
		"offset":      resp.Offset,
		"has_more":    resp.HasMore,
	}, nil
}
