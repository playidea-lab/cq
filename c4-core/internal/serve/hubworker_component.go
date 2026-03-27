//go:build hub

package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/changmin/c4-core/internal/hub"
)

const (
	hubWorkerPollInterval = 20 * time.Second
	hubWorkerHeartbeat    = 30 * time.Second
	hubWorkerLeaseRenew   = 60 * time.Second
	hubWorkerGracePeriod  = 5 * time.Second
)

// HubWorkerComponent implements Component. It registers as a Hub worker on Start
// and continuously claims + executes jobs until Stop is called.
type HubWorkerComponent struct {
	client   *hub.Client
	tags     []string
	hostname string

	mu      sync.Mutex
	cancel  context.CancelFunc
	started bool
	jobCmd  *exec.Cmd // currently running job subprocess
}

// NewHubWorker creates a HubWorkerComponent.
func NewHubWorker(client *hub.Client, tags []string, hostname string) *HubWorkerComponent {
	return &HubWorkerComponent{
		client:   client,
		tags:     tags,
		hostname: hostname,
	}
}

func (w *HubWorkerComponent) Name() string { return "hub_worker" }

func (w *HubWorkerComponent) Start(ctx context.Context) error {
	// Register with Hub.
	caps := map[string]any{
		"hostname": w.hostname,
		"tags":     w.tags,
	}
	workerID, err := w.client.RegisterWorker(caps)
	if err != nil {
		return fmt.Errorf("hub worker register: %w", err)
	}
	fmt.Fprintf(os.Stderr, "cq serve: hub worker registered (id=%s, tags=%v)\n", workerID, w.tags)

	ctx, cancel := context.WithCancel(ctx)
	w.mu.Lock()
	w.cancel = cancel
	w.started = true
	w.mu.Unlock()

	go w.heartbeatLoop(ctx)
	go w.jobLoop(ctx)
	return nil
}

func (w *HubWorkerComponent) Stop(_ context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}

	// Graceful shutdown of running job.
	if w.jobCmd != nil && w.jobCmd.Process != nil {
		_ = w.jobCmd.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() {
			_ = w.jobCmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(hubWorkerGracePeriod):
			_ = w.jobCmd.Process.Kill()
		}
	}
	return nil
}

func (w *HubWorkerComponent) Health() ComponentHealth {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.started {
		return ComponentHealth{Status: "error", Detail: "not started"}
	}
	return ComponentHealth{Status: "ok"}
}

// heartbeatLoop sends periodic heartbeats to Hub.
func (w *HubWorkerComponent) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(hubWorkerHeartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			status := "idle"
			w.mu.Lock()
			if w.jobCmd != nil {
				status = "working"
			}
			w.mu.Unlock()
			if err := w.client.Heartbeat(status); err != nil {
				fmt.Fprintf(os.Stderr, "cq serve: hub worker heartbeat: %v\n", err)
			}
		}
	}
}

// jobLoop continuously polls for jobs, executes them, and reports results.
func (w *HubWorkerComponent) jobLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Poll for a job (long-poll style, 20s server wait).
		job, leaseID, err := w.client.ClaimJobWithWait(ctx, 0, 20)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			fmt.Fprintf(os.Stderr, "cq serve: hub worker claim error: %v\n", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(hubWorkerPollInterval):
			}
			continue
		}
		if job == nil {
			// No job available — poll again.
			continue
		}

		jobID := job.GetID()
		fmt.Fprintf(os.Stderr, "cq serve: hub worker claimed job %s: %s\n", jobID, job.Command)

		// Start lease renewal for this job.
		leaseCtx, leaseCancel := context.WithCancel(ctx)
		if leaseID != "" {
			go w.leaseRenewLoop(leaseCtx, leaseID, jobID)
		}

		// Execute job.
		exitCode, jobErr := w.executeJob(ctx, job)
		leaseCancel()

		// Report result.
		status := "SUCCEEDED"
		if exitCode != 0 || jobErr != nil {
			status = "FAILED"
		}
		if completeErr := w.client.CompleteJob(jobID, status, exitCode); completeErr != nil {
			fmt.Fprintf(os.Stderr, "cq serve: hub worker complete job %s: %v\n", jobID, completeErr)
		}

		if jobErr != nil {
			fmt.Fprintf(os.Stderr, "cq serve: hub worker job %s failed: %v\n", jobID, jobErr)
		} else {
			fmt.Fprintf(os.Stderr, "cq serve: hub worker job %s %s (exit=%d)\n", jobID, status, exitCode)
		}
	}
}

// executeJob runs a job command as a subprocess.
func (w *HubWorkerComponent) executeJob(ctx context.Context, job *hub.Job) (int, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", job.Command)

	// Use job workdir if specified, otherwise current directory.
	if job.Workdir != "" {
		cmd.Dir = job.Workdir
	}

	// Inherit environment + add job-specific env vars.
	cmd.Env = os.Environ()
	if len(job.Env) > 0 {
		var envMap map[string]string
		if err := json.Unmarshal(job.Env, &envMap); err == nil {
			for k, v := range envMap {
				cmd.Env = append(cmd.Env, k+"="+v)
			}
		}
	}

	cmd.Stdout = os.Stderr // route to serve log
	cmd.Stderr = os.Stderr

	w.mu.Lock()
	w.jobCmd = cmd
	w.mu.Unlock()

	err := cmd.Run()

	w.mu.Lock()
	w.jobCmd = nil
	w.mu.Unlock()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return 1, err
	}
	return 0, nil
}

// leaseRenewLoop renews the job lease periodically until ctx is cancelled.
func (w *HubWorkerComponent) leaseRenewLoop(ctx context.Context, leaseID, jobID string) {
	ticker := time.NewTicker(hubWorkerLeaseRenew)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := w.client.RenewLease(leaseID); err != nil {
				fmt.Fprintf(os.Stderr, "cq serve: hub worker lease renew (job=%s): %v\n", jobID, err)
			}
		}
	}
}
