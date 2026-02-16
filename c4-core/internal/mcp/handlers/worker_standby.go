package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/worker"
)

// WorkerDeps holds dependencies for worker standby tools.
type WorkerDeps struct {
	HubClient     *hub.Client
	ShutdownStore *worker.ShutdownStore
	Keeper        *ContextKeeper // may be nil if C1 not enabled
}

// RegisterWorkerHandlers registers c4_worker_standby, c4_worker_complete, c4_worker_shutdown.
func RegisterWorkerHandlers(reg *mcp.Registry, deps *WorkerDeps) {
	registerWorkerStandby(reg, deps)
	registerWorkerComplete(reg, deps)
	registerWorkerShutdown(reg, deps)
}

// registerWorkerStandby registers the blocking standby tool.
func registerWorkerStandby(reg *mcp.Registry, deps *WorkerDeps) {
	reg.Register(mcp.ToolSchema{
		Name:        "c4_worker_standby",
		Description: "Register as a Hub worker and block until a job is available or shutdown is requested. Polls every 5 seconds with 30-second heartbeats. Returns job info when available.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"worker_id": map[string]any{
					"type":        "string",
					"description": "Worker identifier (e.g. 'worker-abc'). Used for channel naming and tracking.",
				},
				"capabilities": map[string]any{
					"type":        "object",
					"description": "Worker capabilities (e.g. {\"tags\": [\"c4-worker\", \"mcp\"]})",
				},
			},
			"required": []string{"worker_id"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleWorkerStandby(deps, raw)
	})
}

func handleWorkerStandby(deps *WorkerDeps, raw json.RawMessage) (any, error) {
	var params struct {
		WorkerID     string         `json:"worker_id"`
		Capabilities map[string]any `json:"capabilities"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if params.WorkerID == "" {
		return nil, fmt.Errorf("worker_id is required")
	}

	// Register with Hub (first time)
	caps := params.Capabilities
	if caps == nil {
		caps = map[string]any{"tags": []string{"c4-worker", "mcp"}}
	}
	// Ensure hostname is set (required by C5 Hub)
	if _, ok := caps["hostname"]; !ok {
		hostname, _ := os.Hostname()
		if hostname == "" {
			hostname = params.WorkerID
		}
		caps["hostname"] = hostname
	}
	hubWorkerID, err := deps.HubClient.RegisterWorker(caps)
	if err != nil {
		return nil, fmt.Errorf("register worker: %w", err)
	}
	fmt.Fprintf(os.Stderr, "c4: worker %s registered (hub_id=%s)\n", params.WorkerID, hubWorkerID)

	// Setup C1 channel and presence if keeper available
	if deps.Keeper != nil {
		channelName := "worker-" + params.WorkerID
		if _, chErr := deps.Keeper.EnsureChannel(channelName, "Worker "+params.WorkerID+" channel", "worker"); chErr != nil {
			fmt.Fprintf(os.Stderr, "c4: worker channel creation failed: %v\n", chErr)
		}
		deps.Keeper.c1.EnsureMember("agent", params.WorkerID, params.WorkerID)
		deps.Keeper.c1.UpdatePresence("agent", params.WorkerID, "idle", "Waiting for jobs")
	}

	// Polling loop: 5s poll, 30s heartbeat
	pollTicker := time.NewTicker(5 * time.Second)
	defer pollTicker.Stop()
	heartbeatTicker := time.NewTicker(30 * time.Second)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-pollTicker.C:
			// Check shutdown signal
			if reason, ok := deps.ShutdownStore.ConsumeSignal(params.WorkerID); ok {
				if deps.Keeper != nil {
					deps.Keeper.c1.UpdatePresence("agent", params.WorkerID, "offline", "Shutdown: "+reason)
				}
				return map[string]any{
					"shutdown": true,
					"reason":   reason,
				}, nil
			}

			// Try to claim a job
			job, leaseID, err := deps.HubClient.ClaimJob(0)
			if err != nil {
				fmt.Fprintf(os.Stderr, "c4: worker %s claim error: %v\n", params.WorkerID, err)
				continue
			}
			if job == nil {
				continue
			}

			// Job found — update presence and return
			if deps.Keeper != nil {
				deps.Keeper.c1.UpdatePresence("agent", params.WorkerID, "working", "Job: "+job.GetID())
				channelName := "worker-" + params.WorkerID
				deps.Keeper.AutoPost(channelName, fmt.Sprintf("🔧 Claimed job %s: %s", job.GetID(), job.Command))
			}

			return map[string]any{
				"job_id":   job.GetID(),
				"command":  job.Command,
				"lease_id": leaseID,
				"name":     job.Name,
				"workdir":  job.Workdir,
				"env":      job.Env,
				"tags":     job.Tags,
			}, nil

		case <-heartbeatTicker.C:
			if err := deps.HubClient.Heartbeat("idle"); err != nil {
				fmt.Fprintf(os.Stderr, "c4: worker %s heartbeat failed: %v\n", params.WorkerID, err)
			}
		}
	}
}

// registerWorkerComplete registers the job completion tool.
func registerWorkerComplete(reg *mcp.Registry, deps *WorkerDeps) {
	reg.Register(mcp.ToolSchema{
		Name:        "c4_worker_complete",
		Description: "Report job completion with status and optional commit SHA. Updates Hub, Messenger channel, and EventBus.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_id":     map[string]any{"type": "string", "description": "Job ID to complete"},
				"lease_id":   map[string]any{"type": "string", "description": "Lease ID from standby"},
				"worker_id":  map[string]any{"type": "string", "description": "Worker identifier"},
				"status":     map[string]any{"type": "string", "enum": []string{"SUCCEEDED", "FAILED"}, "description": "Job result status"},
				"commit_sha": map[string]any{"type": "string", "description": "Git commit SHA if code was committed"},
				"summary":    map[string]any{"type": "string", "description": "Brief summary of work done"},
			},
			"required": []string{"job_id", "worker_id", "status"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleWorkerComplete(deps, raw)
	})
}

func handleWorkerComplete(deps *WorkerDeps, raw json.RawMessage) (any, error) {
	var params struct {
		JobID     string `json:"job_id"`
		LeaseID   string `json:"lease_id"`
		WorkerID  string `json:"worker_id"`
		Status    string `json:"status"`
		CommitSHA string `json:"commit_sha"`
		Summary   string `json:"summary"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if params.JobID == "" || params.WorkerID == "" || params.Status == "" {
		return nil, fmt.Errorf("job_id, worker_id, and status are required")
	}

	// Complete job on Hub
	exitCode := 0
	if params.Status == "FAILED" {
		exitCode = 1
	}
	if err := deps.HubClient.CompleteJob(params.JobID, params.Status, exitCode); err != nil {
		return nil, fmt.Errorf("complete job: %w", err)
	}

	// Update C1 presence and post to worker channel
	if deps.Keeper != nil {
		deps.Keeper.c1.UpdatePresence("agent", params.WorkerID, "idle", "Waiting for jobs")

		channelName := "worker-" + params.WorkerID
		emoji := "✅"
		if params.Status == "FAILED" {
			emoji = "❌"
		}
		msg := fmt.Sprintf("%s Job %s: %s", emoji, params.JobID, params.Status)
		if params.CommitSHA != "" {
			msg += "\nCommit: " + params.CommitSHA
		}
		if params.Summary != "" {
			msg += "\n" + params.Summary
		}
		deps.Keeper.AutoPost(channelName, msg)
	}

	return map[string]any{
		"status": "completed",
		"job_id": params.JobID,
	}, nil
}

// registerWorkerShutdown registers the graceful shutdown tool.
func registerWorkerShutdown(reg *mcp.Registry, deps *WorkerDeps) {
	reg.Register(mcp.ToolSchema{
		Name:        "c4_worker_shutdown",
		Description: "Request graceful shutdown of a worker. The worker will stop on its next poll cycle.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"worker_id": map[string]any{"type": "string", "description": "Worker to shut down"},
				"reason":    map[string]any{"type": "string", "description": "Shutdown reason"},
			},
			"required": []string{"worker_id"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleWorkerShutdown(deps, raw)
	})
}

func handleWorkerShutdown(deps *WorkerDeps, raw json.RawMessage) (any, error) {
	var params struct {
		WorkerID string `json:"worker_id"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if params.WorkerID == "" {
		return nil, fmt.Errorf("worker_id is required")
	}
	if params.Reason == "" {
		params.Reason = "shutdown requested"
	}

	// Store shutdown signal
	if err := deps.ShutdownStore.StoreSignal(params.WorkerID, params.Reason); err != nil {
		return nil, fmt.Errorf("store shutdown signal: %w", err)
	}

	// Post to worker channel
	if deps.Keeper != nil {
		channelName := "worker-" + params.WorkerID
		deps.Keeper.AutoPost(channelName, "🛑 Shutdown requested: "+params.Reason)
	}

	return map[string]any{
		"status":    "signal_stored",
		"worker_id": params.WorkerID,
		"reason":    params.Reason,
	}, nil
}
