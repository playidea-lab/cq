package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/changmin/c4-core/internal/shell"
)

// Scheduler manages the job queue, process execution, and timeouts.
type Scheduler struct {
	store   *Store
	dataDir string // e.g. ~/.c4/daemon — jobs/{id}/ subdirectories

	mu       sync.Mutex
	running  map[string]*runningJob // jobID → process info
	cancel   context.CancelFunc
	stopped  chan struct{}

	// GPU allocation (simple slot-based)
	gpuTotal    int   // total GPUs available (0 = no GPU)
	gpuUsed     []int // indices of GPUs currently in use

	// Callbacks for testing
	maxConcurrent int // 0 = unlimited CPU jobs; GPU jobs limited by gpuTotal
	pollInterval  time.Duration
}

// runningJob tracks a running process.
type runningJob struct {
	cmd      *exec.Cmd
	logFile  *os.File
	cancelFn context.CancelFunc
	startAt  time.Time
	exited   bool // set under s.mu after cmd.Wait() returns
}

// SchedulerConfig holds scheduler settings.
type SchedulerConfig struct {
	DataDir       string
	GPUCount      int
	MaxConcurrent int           // max concurrent CPU jobs (0 = unlimited)
	PollInterval  time.Duration // how often to check (default 1s)
}

// NewScheduler creates a scheduler attached to a store.
func NewScheduler(store *Store, cfg SchedulerConfig) *Scheduler {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 1 * time.Second
	}
	return &Scheduler{
		store:         store,
		dataDir:       cfg.DataDir,
		running:       make(map[string]*runningJob),
		gpuTotal:      cfg.GPUCount,
		gpuUsed:       nil,
		maxConcurrent: cfg.MaxConcurrent,
		pollInterval:  cfg.PollInterval,
	}
}

// Start begins the scheduler loop in a goroutine.
func (s *Scheduler) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)
	s.stopped = make(chan struct{})

	go func() {
		defer close(s.stopped)
		ticker := time.NewTicker(s.pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				s.cleanup()
				return
			case <-ticker.C:
				s.checkRunningJobs()
				s.scheduleQueuedJobs()
			}
		}
	}()
}

// Stop gracefully stops the scheduler and waits for cleanup.
func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.stopped != nil {
		<-s.stopped
	}
}

// cleanup kills all running jobs on shutdown.
func (s *Scheduler) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, rj := range s.running {
		rj.cancelFn()
		s.killProcessGroup(rj.cmd)
		rj.logFile.Close()
		s.store.CompleteJob(id, StatusFailed, -1)
	}
	s.running = make(map[string]*runningJob)
}

// scheduleQueuedJobs picks the highest-priority queued job and starts it.
func (s *Scheduler) scheduleQueuedJobs() {
	queued, err := s.store.GetQueuedJobs()
	if err != nil {
		log.Printf("scheduler: get queued: %v", err)
		return
	}

	for _, job := range queued {
		if !s.canSchedule(job) {
			continue
		}

		gpuIndices := s.allocateGPU(job)
		if err := s.startJob(job, gpuIndices); err != nil {
			log.Printf("scheduler: start %s: %v", job.ID, err)
			continue
		}
	}
}

// canSchedule returns true if a job can be started now.
func (s *Scheduler) canSchedule(job *Job) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if job.RequiresGPU && s.gpuTotal > 0 {
		needed := job.GPUCount
		if needed == 0 {
			needed = 1
		}
		available := s.gpuTotal - len(s.gpuUsed)
		return available >= needed
	}

	// CPU jobs: check maxConcurrent
	if s.maxConcurrent > 0 {
		cpuRunning := 0
		for _, rj := range s.running {
			_ = rj // count all running
			cpuRunning++
		}
		return cpuRunning < s.maxConcurrent
	}

	return true
}

// allocateGPU reserves GPU indices for a job.
func (s *Scheduler) allocateGPU(job *Job) []int {
	if !job.RequiresGPU || s.gpuTotal == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	needed := job.GPUCount
	if needed == 0 {
		needed = 1
	}

	usedSet := make(map[int]bool)
	for _, idx := range s.gpuUsed {
		usedSet[idx] = true
	}

	var allocated []int
	for i := 0; i < s.gpuTotal && len(allocated) < needed; i++ {
		if !usedSet[i] {
			allocated = append(allocated, i)
		}
	}

	if len(allocated) < needed {
		return nil // not enough GPUs (shouldn't happen if canSchedule passed)
	}

	s.gpuUsed = append(s.gpuUsed, allocated...)
	return allocated
}

// releaseGPU frees GPU indices after a job finishes.
func (s *Scheduler) releaseGPU(indices []int) {
	if len(indices) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	freeSet := make(map[int]bool)
	for _, idx := range indices {
		freeSet[idx] = true
	}

	var remaining []int
	for _, idx := range s.gpuUsed {
		if !freeSet[idx] {
			remaining = append(remaining, idx)
		}
	}
	s.gpuUsed = remaining
}

// startJob launches a job process.
func (s *Scheduler) startJob(job *Job, gpuIndices []int) error {
	// Create job directory
	jobDir := filepath.Join(s.dataDir, "jobs", job.ID)
	if err := os.MkdirAll(jobDir, 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	// Create log file
	logPath := filepath.Join(jobDir, "output.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("create log: %w", err)
	}

	// Build command with context for timeout
	var ctx context.Context
	var cancelFn context.CancelFunc
	if job.TimeoutSec > 0 {
		ctx, cancelFn = context.WithTimeout(context.Background(), time.Duration(job.TimeoutSec)*time.Second)
	} else {
		ctx, cancelFn = context.WithCancel(context.Background())
	}

	cmd := shell.Command(ctx, job.Command)
	cmd.Dir = job.Workdir
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// Process group for clean kill (platform-specific)
	setSysProcAttr(cmd)

	// Environment
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "JOB_ID="+job.ID)
	cmd.Env = append(cmd.Env, "RUN_DIR="+jobDir)
	if len(gpuIndices) > 0 {
		indices := make([]string, len(gpuIndices))
		for i, idx := range gpuIndices {
			indices[i] = strconv.Itoa(idx)
		}
		cmd.Env = append(cmd.Env, "CUDA_VISIBLE_DEVICES="+strings.Join(indices, ","))
	}
	for k, v := range job.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	if err := cmd.Start(); err != nil {
		cancelFn()
		logFile.Close()
		return fmt.Errorf("start process: %w", err)
	}

	// Update store
	pid := cmd.Process.Pid
	if err := s.store.StartJob(job.ID, pid, gpuIndices); err != nil {
		cancelFn()
		s.killProcessGroup(cmd)
		logFile.Close()
		return fmt.Errorf("store start: %w", err)
	}

	s.mu.Lock()
	s.running[job.ID] = &runningJob{
		cmd:      cmd,
		logFile:  logFile,
		cancelFn: cancelFn,
		startAt:  time.Now(),
	}
	s.mu.Unlock()

	// Wait for completion in background
	go s.waitForCompletion(job.ID, gpuIndices)

	return nil
}

// waitForCompletion waits for a process to exit and updates the store.
func (s *Scheduler) waitForCompletion(jobID string, gpuIndices []int) {
	s.mu.Lock()
	rj, ok := s.running[jobID]
	s.mu.Unlock()
	if !ok {
		return
	}

	err := rj.cmd.Wait()

	s.mu.Lock()
	rj.exited = true
	s.mu.Unlock()

	rj.logFile.Close()

	exitCode := 0
	status := StatusSucceeded
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
		status = StatusFailed
	}

	s.store.CompleteJob(jobID, status, exitCode)
	s.loadMetricsJSON(jobID)
	s.releaseGPU(gpuIndices)

	s.mu.Lock()
	delete(s.running, jobID)
	s.mu.Unlock()
}

// loadMetricsJSON looks for a metrics file after job completion and stores it.
// It checks MetricsPath first, then falls back to {workdir}/metrics.json.
// Errors are logged but do not affect the job's terminal status.
func (s *Scheduler) loadMetricsJSON(jobID string) {
	job, err := s.store.GetJob(jobID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "c4: daemon: loadMetricsJSON get job %s: %v\n", jobID, err)
		return
	}

	metricsPath := job.MetricsPath
	if metricsPath == "" {
		metricsPath = filepath.Join(job.Workdir, "metrics.json")
	}

	data, err := os.ReadFile(metricsPath)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "c4: daemon: loadMetricsJSON read %s: %v\n", metricsPath, err)
		}
		return
	}

	var metrics map[string]any
	if err := json.Unmarshal(data, &metrics); err != nil {
		fmt.Fprintf(os.Stderr, "c4: daemon: loadMetricsJSON parse %s: %v\n", metricsPath, err)
		return
	}

	if err := s.store.SetJobMetrics(jobID, metrics); err != nil {
		fmt.Fprintf(os.Stderr, "c4: daemon: loadMetricsJSON store %s: %v\n", jobID, err)
	}
}

// checkRunningJobs verifies running processes are still alive and handles timeouts.
// Note: most completion is handled by waitForCompletion goroutines.
// This method serves as a safety net for orphaned entries.
func (s *Scheduler) checkRunningJobs() {
	s.mu.Lock()
	var orphans []string
	for id, rj := range s.running {
		if rj.exited {
			orphans = append(orphans, id)
		}
	}
	s.mu.Unlock()

	for _, id := range orphans {
		s.mu.Lock()
		rj, ok := s.running[id]
		if ok {
			rj.logFile.Close()
			delete(s.running, id)
		}
		s.mu.Unlock()

		if ok {
			s.store.CompleteJob(id, StatusFailed, -1)
		}
	}
}

// Cancel cancels a job. If running, kills the process group.
func (s *Scheduler) Cancel(jobID string) error {
	s.mu.Lock()
	rj, isRunning := s.running[jobID]
	s.mu.Unlock()

	if isRunning {
		rj.cancelFn()
		s.killProcessGroup(rj.cmd)
		// waitForCompletion will handle cleanup, but we override to CANCELLED
	}

	return s.store.CancelJob(jobID)
}

// GetJobLog reads the output log for a job.
func (s *Scheduler) GetJobLog(jobID string, offset, limit int) ([]string, int, bool, error) {
	logPath := filepath.Join(s.dataDir, "jobs", jobID, "output.log")
	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, false, nil
		}
		return nil, 0, false, err
	}
	defer f.Close()

	// Read all lines
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, 0, false, err
	}

	allLines := strings.Split(string(data), "\n")
	// Remove trailing empty line from split
	if len(allLines) > 0 && allLines[len(allLines)-1] == "" {
		allLines = allLines[:len(allLines)-1]
	}

	total := len(allLines)

	// Apply offset
	if offset >= total {
		return nil, total, false, nil
	}
	lines := allLines[offset:]

	// Apply limit
	hasMore := false
	if limit > 0 && len(lines) > limit {
		lines = lines[:limit]
		hasMore = true
	}

	return lines, total, hasMore, nil
}

// RunningCount returns the number of currently running jobs.
func (s *Scheduler) RunningCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.running)
}
