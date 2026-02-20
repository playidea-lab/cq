//go:build gpu


package handlers

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/changmin/c4-core/internal/daemon"
	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterGPUNativeHandlers registers GPU tools as Go native handlers.
// Uses daemon.GpuMonitor for nvidia-smi detection and daemon.Store for job management.
// If gpuStore is nil, all job tools return an error.
// If scheduler is nil, c4_job_cancel only calls Store.CancelJob (no process kill).
func RegisterGPUNativeHandlers(reg *mcp.Registry, gpuStore *daemon.Store, scheduler *daemon.Scheduler) {
	gpuMon := daemon.NewGpuMonitor()

	reg.Register(mcp.ToolSchema{
		Name:        "c4_gpu_status",
		Description: "Get GPU device status and availability",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, gpuStatusHandler(gpuMon))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_job_submit",
		Description: "Submit a job to the GPU scheduler",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command":     map[string]any{"type": "string", "description": "Command to run"},
				"name":        map[string]any{"type": "string", "description": "Job name (default: gpu-job)"},
				"workdir":     map[string]any{"type": "string", "description": "Working directory (default: current directory)"},
				"gpu_id":      map[string]any{"type": "integer", "description": "Specific GPU ID (optional)"},
				"priority":    map[string]any{"type": "integer", "description": "Job priority (default: 5)"},
				"exp_id":      map[string]any{"type": "string", "description": "Experiment ID (optional)"},
				"tags":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Tags (optional)"},
				"env":         map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Environment variables (optional)"},
				"timeout_sec": map[string]any{"type": "integer", "description": "Timeout in seconds (optional, 0 = no timeout)"},
				"memo":        map[string]any{"type": "string", "description": "Memo/note (optional)"},
			},
			"required": []string{"command"},
		},
	}, jobSubmitHandler(gpuStore))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_job_list",
		Description: "List jobs with optional status filter",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status": map[string]any{
					"type":        "string",
					"description": "Filter by status: QUEUED, RUNNING, SUCCEEDED, FAILED, CANCELLED",
					"enum":        []string{"QUEUED", "RUNNING", "SUCCEEDED", "FAILED", "CANCELLED"},
				},
				"limit":  map[string]any{"type": "integer", "description": "Max results (default: 20)"},
				"offset": map[string]any{"type": "integer", "description": "Offset for pagination (default: 0)"},
			},
		},
	}, jobListHandler(gpuStore))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_job_status",
		Description: "Get detailed status of a job including logs and metrics",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_id": map[string]any{"type": "string", "description": "Job ID"},
			},
			"required": []string{"job_id"},
		},
	}, jobStatusHandler(gpuStore, scheduler))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_job_cancel",
		Description: "Cancel a queued or running job",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_id": map[string]any{"type": "string", "description": "Job ID"},
			},
			"required": []string{"job_id"},
		},
	}, jobCancelHandler(gpuStore, scheduler))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_job_summary",
		Description: "Get queue-level statistics (counts by status)",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, jobSummaryHandler(gpuStore))
}

func gpuStatusHandler(mon *daemon.GpuMonitor) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		gpus, err := mon.GetAllGPUs()
		if err != nil {
			// macOS / no GPU fallback
			backend := "cpu"
			if runtime.GOOS == "darwin" {
				backend = "mps"
			}
			return map[string]any{
				"gpu_count": 0,
				"gpus":      []any{},
				"backend":   backend,
			}, nil
		}

		gpuList := make([]map[string]any, 0, len(gpus))
		for _, g := range gpus {
			gpuList = append(gpuList, map[string]any{
				"index":           g.Index,
				"name":            g.Name,
				"backend":         "cuda",
				"total_vram_gb":   g.TotalVRAM,
				"free_vram_gb":    g.FreeVRAM,
				"utilization_pct": float64(g.Utilization),
			})
		}

		backend := "cpu"
		if len(gpuList) > 0 {
			backend = "cuda"
		}

		return map[string]any{
			"gpu_count": len(gpuList),
			"gpus":      gpuList,
			"backend":   backend,
		}, nil
	}
}

func jobSubmitHandler(store *daemon.Store) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		var params struct {
			Command    string            `json:"command"`
			Name       string            `json:"name"`
			Workdir    string            `json:"workdir"`
			GPUID      *int              `json:"gpu_id"`
			Priority   int               `json:"priority"`
			ExpID      string            `json:"exp_id"`
			Tags       []string          `json:"tags"`
			Env        map[string]string `json:"env"`
			TimeoutSec int               `json:"timeout_sec"`
			Memo       string            `json:"memo"`
		}
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &params); err != nil {
				return nil, fmt.Errorf("parsing arguments: %w", err)
			}
		}
		if params.Command == "" {
			return map[string]any{"error": "command is required"}, nil
		}

		if store == nil {
			return map[string]any{"error": "GPU job scheduler not available"}, nil
		}

		jobName := params.Name
		if jobName == "" {
			jobName = "gpu-job"
		}
		workdir := params.Workdir
		if workdir == "" {
			workdir = "."
		}

		requiresGPU := params.GPUID != nil
		gpuCount := 0
		if requiresGPU {
			gpuCount = 1
		}
		req := &daemon.JobSubmitRequest{
			Name:        jobName,
			Command:     params.Command,
			Workdir:     workdir,
			RequiresGPU: requiresGPU,
			GPUCount:    gpuCount,
			Priority:    params.Priority,
			ExpID:       params.ExpID,
			Tags:        params.Tags,
			Env:         params.Env,
			TimeoutSec:  params.TimeoutSec,
			Memo:        params.Memo,
		}

		job, err := store.CreateJob(req)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("JobSubmit failed: %v", err)}, nil
		}

		return map[string]any{
			"success": true,
			"job_id":  job.ID,
			"message": fmt.Sprintf("Job submitted: %s", job.ID),
		}, nil
	}
}

func jobListHandler(store *daemon.Store) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		if store == nil {
			return map[string]any{"error": "GPU job scheduler not available"}, nil
		}

		var params struct {
			Status string `json:"status"`
			Limit  int    `json:"limit"`
			Offset int    `json:"offset"`
		}
		params.Limit = 20 // default
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &params); err != nil {
				return nil, fmt.Errorf("parsing arguments: %w", err)
			}
			if params.Limit == 0 {
				params.Limit = 20
			}
		}

		jobs, err := store.ListJobs(params.Status, params.Limit, params.Offset)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("ListJobs failed: %v", err)}, nil
		}

		if jobs == nil {
			return []any{}, nil
		}

		result := make([]map[string]any, 0, len(jobs))
		for _, j := range jobs {
			item := map[string]any{
				"job_id":     j.ID,
				"name":       j.Name,
				"status":     string(j.Status),
				"priority":   j.Priority,
				"command":    j.Command,
				"workdir":    j.Workdir,
				"created_at": j.CreatedAt.Format("2006-01-02T15:04:05Z"),
			}
			if j.StartedAt != nil {
				item["started_at"] = j.StartedAt.Format("2006-01-02T15:04:05Z")
			}
			if j.FinishedAt != nil {
				item["finished_at"] = j.FinishedAt.Format("2006-01-02T15:04:05Z")
			}
			if j.ExpID != "" {
				item["exp_id"] = j.ExpID
			}
			if len(j.Tags) > 0 {
				item["tags"] = j.Tags
			}
			result = append(result, item)
		}
		return result, nil
	}
}

func jobStatusHandler(store *daemon.Store, scheduler *daemon.Scheduler) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		if store == nil {
			return map[string]any{"error": "GPU job scheduler not available"}, nil
		}

		var params struct {
			JobID string `json:"job_id"`
		}
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &params); err != nil {
				return nil, fmt.Errorf("parsing arguments: %w", err)
			}
		}
		if params.JobID == "" {
			return map[string]any{"error": "job_id is required"}, nil
		}

		job, err := store.GetJob(params.JobID)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("GetJob failed: %v", err)}, nil
		}

		result := map[string]any{
			"job_id":      job.ID,
			"name":        job.Name,
			"status":      string(job.Status),
			"priority":    job.Priority,
			"command":     job.Command,
			"workdir":     job.Workdir,
			"requires_gpu": job.RequiresGPU,
			"gpu_count":   job.GPUCount,
			"created_at":  job.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
		if job.StartedAt != nil {
			result["started_at"] = job.StartedAt.Format("2006-01-02T15:04:05Z")
		}
		if job.FinishedAt != nil {
			result["finished_at"] = job.FinishedAt.Format("2006-01-02T15:04:05Z")
		}
		if job.ExitCode != nil {
			result["exit_code"] = *job.ExitCode
		}
		if job.PID > 0 {
			result["pid"] = job.PID
		}
		if len(job.GPUIndices) > 0 {
			result["gpu_indices"] = job.GPUIndices
		}
		if job.ExpID != "" {
			result["exp_id"] = job.ExpID
		}
		if job.Memo != "" {
			result["memo"] = job.Memo
		}
		if len(job.Tags) > 0 {
			result["tags"] = job.Tags
		}
		if job.TimeoutSec > 0 {
			result["timeout_sec"] = job.TimeoutSec
		}
		if d := job.DurationSec(); d != nil {
			result["duration_sec"] = *d
		}

		// Fetch last 50 log lines if scheduler is available
		if scheduler != nil {
			lines, total, _, err := scheduler.GetJobLog(params.JobID, 0, 0)
			if err == nil && len(lines) > 0 {
				// Return last 50 lines
				offset := 0
				if total > 50 {
					offset = total - 50
				}
				logLines, _, _, _ := scheduler.GetJobLog(params.JobID, offset, 50)
				result["logs"] = logLines
				result["log_total_lines"] = total
			}
		}

		return result, nil
	}
}

func jobCancelHandler(store *daemon.Store, scheduler *daemon.Scheduler) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		if store == nil {
			return map[string]any{"error": "GPU job scheduler not available"}, nil
		}

		var params struct {
			JobID string `json:"job_id"`
		}
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &params); err != nil {
				return nil, fmt.Errorf("parsing arguments: %w", err)
			}
		}
		if params.JobID == "" {
			return map[string]any{"error": "job_id is required"}, nil
		}

		var message string
		if scheduler != nil {
			// Full cancel: kill process + update store
			if err := scheduler.Cancel(params.JobID); err != nil {
				return map[string]any{"success": false, "message": fmt.Sprintf("cancel failed: %v", err)}, nil
			}
			message = fmt.Sprintf("Job %s cancelled", params.JobID)
		} else {
			// Store-only cancel: no process kill possible
			if err := store.CancelJob(params.JobID); err != nil {
				return map[string]any{"success": false, "message": fmt.Sprintf("cancel failed: %v", err)}, nil
			}
			message = fmt.Sprintf("Job %s cancelled (note: process kill unavailable — scheduler not running)", params.JobID)
		}

		return map[string]any{
			"success": true,
			"message": message,
		}, nil
	}
}

func jobSummaryHandler(store *daemon.Store) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		if store == nil {
			return map[string]any{"error": "GPU job scheduler not available"}, nil
		}

		stats, err := store.GetQueueStats()
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("GetQueueStats failed: %v", err)}, nil
		}

		total := stats.Queued + stats.Running + stats.Succeeded + stats.Failed + stats.Cancelled
		return map[string]any{
			"queued":    stats.Queued,
			"running":   stats.Running,
			"succeeded": stats.Succeeded,
			"failed":    stats.Failed,
			"cancelled": stats.Cancelled,
			"total":     total,
		}, nil
	}
}
