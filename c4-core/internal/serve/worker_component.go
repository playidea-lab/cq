//go:build hub

package serve

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/changmin/c4-core/internal/hub"
)

const (
	workerPollInterval = 20 * time.Second
	workerHeartbeat    = 30 * time.Second
	workerLeaseRenew   = 60 * time.Second
	workerGracePeriod  = 5 * time.Second
)

// WorkerComponent implements Component. It registers as a Hub worker on Start
// and continuously claims + executes jobs until Stop is called.
type WorkerComponent struct {
	client   *hub.Client
	tags     []string
	hostname string

	mu         sync.Mutex
	cancel     context.CancelFunc
	started    bool
	jobCmd     *exec.Cmd                          // currently running job subprocess
	notifyFunc func(jobID, status string, exitCode int) // job completion callback
}

// SetNotifyFunc sets a callback invoked (in a goroutine) after each job completes.
func (w *WorkerComponent) SetNotifyFunc(fn func(jobID, status string, exitCode int)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.notifyFunc = fn
}

// NewWorker creates a WorkerComponent.
func NewWorker(client *hub.Client, tags []string, hostname string) *WorkerComponent {
	return &WorkerComponent{
		client:   client,
		tags:     tags,
		hostname: hostname,
	}
}

func (w *WorkerComponent) Name() string { return "worker" }

func (w *WorkerComponent) Start(ctx context.Context) error {
	// Register with Hub.
	// RegisterWorker extracts map keys as capability strings, so each tag
	// must be a separate key. "hostname" is also passed as a key.
	caps := map[string]any{
		"hostname": w.hostname,
	}
	for _, tag := range w.tags {
		caps[tag] = true
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

func (w *WorkerComponent) Stop(_ context.Context) error {
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
		case <-time.After(workerGracePeriod):
			_ = w.jobCmd.Process.Kill()
		}
	}
	return nil
}

func (w *WorkerComponent) Health() ComponentHealth {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.started {
		return ComponentHealth{Status: "error", Detail: "not started"}
	}
	return ComponentHealth{Status: "ok"}
}

// heartbeatLoop sends periodic heartbeats to Hub.
func (w *WorkerComponent) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(workerHeartbeat)
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
func (w *WorkerComponent) jobLoop(ctx context.Context) {
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
			case <-time.After(workerPollInterval):
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
		} else {
			w.mu.Lock()
			notify := w.notifyFunc
			w.mu.Unlock()
			if notify != nil {
				go notify(jobID, status, exitCode)
			}
		}

		if jobErr != nil {
			fmt.Fprintf(os.Stderr, "cq serve: hub worker job %s failed: %v\n", jobID, jobErr)
		} else {
			fmt.Fprintf(os.Stderr, "cq serve: hub worker job %s %s (exit=%d)\n", jobID, status, exitCode)
		}
	}
}

// executeJob runs a job command as a subprocess.
func (w *WorkerComponent) executeJob(ctx context.Context, job *hub.Job) (int, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", job.Command)

	// Use job workdir if specified, otherwise current directory.
	// Expand leading ~/ to the user's home directory.
	workdir := job.Workdir
	if strings.HasPrefix(workdir, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			workdir = filepath.Join(home, workdir[2:])
		}
	}
	if workdir != "" {
		cmd.Dir = workdir
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

	cmd.Stderr = os.Stderr

	// Pipe stdout through a metric scanner; fall back to direct routing on error.
	stdoutPipe, pipeErr := cmd.StdoutPipe()
	if pipeErr != nil {
		cmd.Stdout = os.Stderr
	}

	w.mu.Lock()
	w.jobCmd = cmd
	w.mu.Unlock()

	if err := cmd.Start(); err != nil {
		w.mu.Lock()
		w.jobCmd = nil
		w.mu.Unlock()
		return 1, err
	}

	// Scan stdout, tee to stderr and parse metrics.
	if pipeErr == nil {
		jobID := job.GetID()
		step := 0
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Fprintln(os.Stderr, line)
			if metrics := hub.ParseMetrics(line); metrics != nil {
				step++
				if err := w.client.LogMetricsSupabase(jobID, step, metrics); err != nil {
					fmt.Fprintf(os.Stderr, "cq serve: metric log: %v\n", err)
				}
			}
		}
	}

	err := cmd.Wait()

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
func (w *WorkerComponent) leaseRenewLoop(ctx context.Context, leaseID, jobID string) {
	ticker := time.NewTicker(workerLeaseRenew)
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
