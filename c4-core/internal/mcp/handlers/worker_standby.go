//go:build c5_hub

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/mcp/handlers/messengerhandler"
	"github.com/changmin/c4-core/internal/worker"
)

// workerEntry tracks an active standby goroutine.
type workerEntry struct {
	cancel context.CancelFunc
	gen    uint64 // generation: incremented on each new standby for the same worker_id
}

// WorkerDeps holds dependencies for worker standby tools.
type WorkerDeps struct {
	HubClient     *hub.Client
	ShutdownStore *worker.ShutdownStore
	Keeper        *messengerhandler.ContextKeeper // may be nil if C1 not enabled
	MCPURL        string                   // local mcphttp URL advertised to Hub (e.g. "http://127.0.0.1:4142")

	// activeWorkers tracks running standby goroutines by worker_id.
	// A new standby cancels the previous goroutine for the same worker_id.
	activeWorkersMu sync.Mutex
	activeWorkers   map[string]workerEntry
	nextGen         uint64

	// leaseRenewals tracks background lease renewal goroutines by job_id.
	// Cancelled when the job completes via c4_worker_complete.
	leaseRenewalsMu sync.Mutex
	leaseRenewals   map[string]context.CancelFunc
}

// RegisterWorkerHandlers registers c4_worker_standby, c4_worker_complete, c4_worker_shutdown.
func RegisterWorkerHandlers(reg *mcp.Registry, deps *WorkerDeps) {
	registerWorkerStandby(reg, deps)
	registerWorkerComplete(reg, deps)
	registerWorkerShutdown(reg, deps)
}

// registerWorkerStandby registers the blocking standby tool.
func registerWorkerStandby(reg *mcp.Registry, deps *WorkerDeps) {
	reg.RegisterBlocking(mcp.ToolSchema{
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
	}, func(ctx context.Context, raw json.RawMessage) (any, error) {
		return handleWorkerStandby(ctx, deps, raw)
	})
}

func handleWorkerStandby(mcpCtx context.Context, deps *WorkerDeps, raw json.RawMessage) (any, error) {
	if deps == nil || deps.HubClient == nil {
		return nil, fmt.Errorf("hub client not configured")
	}
	if deps.ShutdownStore == nil {
		return nil, fmt.Errorf("shutdown store not configured")
	}
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

	// Cancel any existing standby goroutine for the same worker_id, then register this one.
	// Derive from mcpCtx so that ESC/interrupt from the MCP client propagates cancellation.
	ctx, cancel := context.WithCancel(mcpCtx)
	deps.activeWorkersMu.Lock()
	if deps.activeWorkers == nil {
		deps.activeWorkers = make(map[string]workerEntry)
	}
	if old, ok := deps.activeWorkers[params.WorkerID]; ok {
		old.cancel() // signal the stale goroutine to exit
	}
	deps.nextGen++
	myGen := deps.nextGen
	deps.activeWorkers[params.WorkerID] = workerEntry{cancel: cancel, gen: myGen}
	deps.activeWorkersMu.Unlock()

	// Always clean up when this goroutine exits.
	defer func() {
		cancel()
		deps.activeWorkersMu.Lock()
		// Only set offline if we are still the current owner (not replaced by a newer standby)
		if entry, ok := deps.activeWorkers[params.WorkerID]; ok && entry.gen == myGen {
			delete(deps.activeWorkers, params.WorkerID)
			deps.activeWorkersMu.Unlock()
			if deps.Keeper != nil {
				deps.Keeper.C1.UpdatePresence("agent", params.WorkerID, "offline", "Standby ended")
			}
		} else {
			deps.activeWorkersMu.Unlock()
		}
	}()

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
	// Inject local MCP URL so Hub can push-dispatch jobs to this worker.
	if deps.MCPURL != "" {
		if _, ok := caps["mcp_url"]; !ok {
			caps["mcp_url"] = deps.MCPURL
		}
	}
	hubWorkerID, err := deps.HubClient.RegisterWorker(caps)
	if err != nil {
		return nil, fmt.Errorf("register worker: %w", err)
	}
	fmt.Fprintf(os.Stderr, "c4: worker %s registered (hub_id=%s)\n", params.WorkerID, hubWorkerID)
	if pub := GetHubEventPub(); pub != nil {
		payload, _ := json.Marshal(map[string]any{"worker_id": params.WorkerID, "hub_worker_id": hubWorkerID, "capabilities": caps})
		pub.PublishAsync("hub.worker.registered", "c4.hub", payload, GetHubProjectID())
	}

	// Setup shared #cq channel and presence if keeper available
	var cqChannelID string
	if deps.Keeper != nil {
		var chErr error
		cqChannelID, chErr = deps.Keeper.EnsureChannel("cq", "Shared worker dispatch channel", "worker")
		if chErr != nil {
			fmt.Fprintf(os.Stderr, "c4: #cq channel creation failed: %v\n", chErr)
		}
		deps.Keeper.C1.EnsureMember("agent", params.WorkerID, params.WorkerID)
		deps.Keeper.C1.UpdatePresence("agent", params.WorkerID, "online", "Waiting for jobs in #cq")
	}

	// hubPollResult carries the result of a Hub long-poll attempt.
	type hubPollResult struct {
		job     *hub.Job
		leaseID string
		err     error
	}

	// hubCh receives results from the Hub long-poll goroutine (buffered to avoid leaks).
	hubCh := make(chan hubPollResult, 1)

	// hubPollCtx/Cancel controls the current Hub goroutine.
	hubPollCtx, hubPollCancel := context.WithCancel(ctx)

	// startHubPoll launches a new Hub long-poll goroutine.
	// The server blocks up to 20s waiting for a job; response is near-instant when one arrives.
	startHubPoll := func() {
		go func() {
			job, leaseID, err := deps.HubClient.ClaimJobWithWait(hubPollCtx, 0, 20)
			select {
			case hubCh <- hubPollResult{job, leaseID, err}:
			case <-hubPollCtx.Done():
			}
		}()
	}
	startHubPoll()
	defer hubPollCancel()

	// c1Ticker: check C1 mentions and shutdown signals every 5s.
	c1Ticker := time.NewTicker(5 * time.Second)
	defer c1Ticker.Stop()
	heartbeatTicker := time.NewTicker(30 * time.Second)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Cancelled by a new standby call for the same worker_id or MCP interrupt
			return map[string]any{"shutdown": true, "reason": "cancelled"}, nil

		case result := <-hubCh:
			// Hub long-poll returned
			if result.err != nil {
				fmt.Fprintf(os.Stderr, "c4: worker %s hub poll error: %v\n", params.WorkerID, result.err)
				startHubPoll() // retry
				continue
			}
			if result.job == nil {
				startHubPoll() // no job this round, poll again immediately
				continue
			}
			// Hub job found — update presence and return
			if deps.Keeper != nil {
				deps.Keeper.C1.UpdatePresence("agent", params.WorkerID, "working", "Job: "+result.job.GetID())
				deps.Keeper.AutoPost("cq", fmt.Sprintf("Worker %s claimed job %s: %s", params.WorkerID, result.job.GetID(), result.job.Command))
			}
			// Start background lease renewal goroutine for this job.
			if result.leaseID != "" {
				deps.startLeaseRenewal(result.job.GetID(), result.leaseID, params.WorkerID)
			}
			return map[string]any{
				"job_id":   result.job.GetID(),
				"command":  result.job.Command,
				"lease_id": result.leaseID,
				"name":     result.job.Name,
				"workdir":  result.job.Workdir,
				"env":      result.job.Env,
				"tags":     result.job.Tags,
			}, nil

		case <-c1Ticker.C:
			// Check shutdown signal
			if reason, ok := deps.ShutdownStore.ConsumeSignal(params.WorkerID); ok {
				if deps.Keeper != nil {
					deps.Keeper.C1.UpdatePresence("agent", params.WorkerID, "offline", "Shutdown: "+reason)
				}
				return map[string]any{"shutdown": true, "reason": reason}, nil
			}

			// Poll #cq channel for @cq mentions
			if deps.Keeper != nil && cqChannelID != "" {
				mentions, pollErr := deps.Keeper.C1.PollCqMentions(cqChannelID, 5)
				if pollErr != nil {
					fmt.Fprintf(os.Stderr, "c4: worker %s poll #cq error: %v\n", params.WorkerID, pollErr)
				}
				for _, msg := range mentions {
					claimed, claimErr := deps.Keeper.C1.ClaimMessage(msg.ID, params.WorkerID)
					if claimErr != nil {
						fmt.Fprintf(os.Stderr, "c4: worker %s claim msg %s error: %v\n", params.WorkerID, msg.ID, claimErr)
						continue
					}
					if claimed {
						deps.Keeper.C1.UpdatePresence("agent", params.WorkerID, "idle", "Dispatched: "+msg.ID)
						return map[string]any{
							"dispatched":  true,
							"message_id":  msg.ID,
							"content":     msg.Content,
							"sender_name": msg.SenderName,
							"channel":     "cq",
						}, nil
					}
				}
			}

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
	if deps == nil || deps.HubClient == nil {
		return nil, fmt.Errorf("hub client not configured")
	}
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

	// Validate status enum
	var exitCode int
	switch params.Status {
	case "SUCCEEDED":
		exitCode = 0
	case "FAILED":
		exitCode = 1
	default:
		return nil, fmt.Errorf("invalid status: %q (must be SUCCEEDED or FAILED)", params.Status)
	}

	// Stop background lease renewal for this job (if active).
	deps.stopLeaseRenewal(params.JobID)

	// Complete job on Hub
	if err := deps.HubClient.CompleteJob(params.JobID, params.Status, exitCode); err != nil {
		return nil, fmt.Errorf("complete job: %w", err)
	}

	// Update C1 presence and post to #cq channel
	if deps.Keeper != nil {
		deps.Keeper.C1.UpdatePresence("agent", params.WorkerID, "idle", "Job done, waiting for next")

		statusIcon := "done"
		if params.Status == "FAILED" {
			statusIcon = "failed"
		}
		msg := fmt.Sprintf("Worker %s: Job %s %s", params.WorkerID, params.JobID, statusIcon)
		if params.CommitSHA != "" {
			msg += " commit=" + params.CommitSHA
		}
		if params.Summary != "" {
			msg += "\n" + params.Summary
		}
		deps.Keeper.AutoPost("cq", msg)
	}
	evType := "hub.job.completed"
	if params.Status == "FAILED" {
		evType = "hub.job.failed"
	}
	if pub := GetHubEventPub(); pub != nil {
		payload, _ := json.Marshal(map[string]any{
			"job_id": params.JobID, "status": params.Status, "exit_code": exitCode,
			"commit_sha": params.CommitSHA, "summary": params.Summary, "worker_id": params.WorkerID,
		})
		pub.PublishAsync(evType, "c4.hub", payload, GetHubProjectID())
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
	if deps == nil || deps.ShutdownStore == nil {
		return nil, fmt.Errorf("shutdown store not configured")
	}
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

	// Update presence and post to #cq channel
	if deps.Keeper != nil {
		deps.Keeper.C1.UpdatePresence("agent", params.WorkerID, "offline", "Shutdown: "+params.Reason)
		deps.Keeper.AutoPost("cq", "Worker "+params.WorkerID+" shutdown: "+params.Reason)
	}

	return map[string]any{
		"status":    "signal_stored",
		"worker_id": params.WorkerID,
		"reason":    params.Reason,
	}, nil
}

// leaseRenewalInterval is the period between automatic lease renewals.
// Set to 60 seconds (well within the typical 5-minute lease TTL).
const leaseRenewalInterval = 60 * time.Second

// leaseRenewalMaxFailures is the number of consecutive renewal failures before
// the goroutine stores a shutdown signal for the worker and exits.
const leaseRenewalMaxFailures = 3

// startLeaseRenewal launches a background goroutine that renews the lease for
// jobID every leaseRenewalInterval. The goroutine is tracked by jobID so
// stopLeaseRenewal can cancel it when the job completes.
func (d *WorkerDeps) startLeaseRenewal(jobID, leaseID, workerID string) {
	ctx, cancel := context.WithCancel(context.Background())
	d.leaseRenewalsMu.Lock()
	if d.leaseRenewals == nil {
		d.leaseRenewals = make(map[string]context.CancelFunc)
	}
	// Cancel any existing renewal for the same job (safety).
	if old, ok := d.leaseRenewals[jobID]; ok {
		old()
	}
	d.leaseRenewals[jobID] = cancel
	d.leaseRenewalsMu.Unlock()

	go func() {
		defer func() {
			cancel()
			d.leaseRenewalsMu.Lock()
			if fn, ok := d.leaseRenewals[jobID]; ok && fn != nil {
				delete(d.leaseRenewals, jobID)
			}
			d.leaseRenewalsMu.Unlock()
		}()

		ticker := time.NewTicker(leaseRenewalInterval)
		defer ticker.Stop()
		failures := 0

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, err := d.HubClient.RenewLease(leaseID)
				if err != nil {
					failures++
					fmt.Fprintf(os.Stderr, "c4: lease renewal failed for job %s (attempt %d/%d): %v\n",
						jobID, failures, leaseRenewalMaxFailures, err)
					if failures >= leaseRenewalMaxFailures {
						fmt.Fprintf(os.Stderr, "c4: lease renewal exceeded %d consecutive failures for job %s — signalling worker shutdown\n",
							leaseRenewalMaxFailures, jobID)
						if d.ShutdownStore != nil {
							_ = d.ShutdownStore.StoreSignal(workerID, "lease renewal failed for job "+jobID)
						}
						return
					}
				} else {
					failures = 0 // reset on success
				}
			}
		}
	}()
}

// stopLeaseRenewal cancels the background lease renewal goroutine for jobID.
func (d *WorkerDeps) stopLeaseRenewal(jobID string) {
	if d == nil {
		return
	}
	d.leaseRenewalsMu.Lock()
	defer d.leaseRenewalsMu.Unlock()
	if fn, ok := d.leaseRenewals[jobID]; ok {
		fn()
		delete(d.leaseRenewals, jobID)
	}
}
