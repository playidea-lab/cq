package handlers

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/changmin/c4-core/internal/daemon"
	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterGPUNativeHandlers registers GPU tools as Go native handlers.
// Uses daemon.GpuMonitor for nvidia-smi detection and daemon.Store for job submission.
// If gpuStore is nil, job_submit will return an error.
func RegisterGPUNativeHandlers(reg *mcp.Registry, gpuStore *daemon.Store) {
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
				"command":  map[string]any{"type": "string", "description": "Command to run"},
				"name":     map[string]any{"type": "string", "description": "Job name (default: gpu-job)"},
				"workdir":  map[string]any{"type": "string", "description": "Working directory (default: current directory)"},
				"gpu_id":   map[string]any{"type": "integer", "description": "Specific GPU ID (optional)"},
				"priority": map[string]any{"type": "integer", "description": "Job priority (default: 5)"},
			},
			"required": []string{"command"},
		},
	}, jobSubmitHandler(gpuStore))
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
			Command  string `json:"command"`
			Name     string `json:"name"`
			Workdir  string `json:"workdir"`
			GPUID    *int   `json:"gpu_id"`
			Priority int    `json:"priority"`
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

		req := &daemon.JobSubmitRequest{
			Name:        jobName,
			Command:     params.Command,
			Workdir:     workdir,
			RequiresGPU: true,
			GPUCount:    1,
			Priority:    params.Priority,
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
