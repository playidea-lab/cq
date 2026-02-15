// Package model defines the core data types for C5 distributed job queue.
//
// JSON tags are compatible with C4's hub.Client so that the same client
// can talk to both C5 server and the local daemon without changes.
package model

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// JobStatus represents the lifecycle state of a job.
type JobStatus string

const (
	StatusQueued    JobStatus = "QUEUED"
	StatusRunning   JobStatus = "RUNNING"
	StatusSucceeded JobStatus = "SUCCEEDED"
	StatusFailed    JobStatus = "FAILED"
	StatusCancelled JobStatus = "CANCELLED"
)

// IsTerminal returns true for final job states.
func (s JobStatus) IsTerminal() bool {
	switch s {
	case StatusSucceeded, StatusFailed, StatusCancelled:
		return true
	}
	return false
}

// Job represents a submitted job with its full lifecycle state.
type Job struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Status      JobStatus         `json:"status"`
	Priority    int               `json:"priority"`
	Workdir     string            `json:"workdir"`
	Command     string            `json:"command"`
	RequiresGPU bool              `json:"requires_gpu"`
	Env         map[string]string `json:"env,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	ExpID       string            `json:"exp_id,omitempty"`
	Memo        string            `json:"memo,omitempty"`
	TimeoutSec  int               `json:"timeout_sec,omitempty"`
	WorkerID    string            `json:"worker_id,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	StartedAt   *time.Time        `json:"started_at,omitempty"`
	FinishedAt  *time.Time        `json:"finished_at,omitempty"`
	ExitCode    *int              `json:"exit_code,omitempty"`
}

// DurationSec returns the job duration in seconds, or nil if not yet finished.
func (j *Job) DurationSec() *float64 {
	if j.StartedAt == nil || j.FinishedAt == nil {
		return nil
	}
	d := j.FinishedAt.Sub(*j.StartedAt).Seconds()
	return &d
}

// CommandHash returns a SHA-256 prefix of the normalized command,
// used for duration estimation based on similar past jobs.
func (j *Job) CommandHash() string {
	return NormalizeCommandHash(j.Command)
}

// Worker represents a remote worker node.
type Worker struct {
	ID            string    `json:"id"`
	Hostname      string    `json:"hostname,omitempty"`
	Status        string    `json:"status"` // online, offline, busy
	GPUCount      int       `json:"gpu_count"`
	GPUModel      string    `json:"gpu_model,omitempty"`
	TotalVRAM     float64   `json:"total_vram_gb"`
	FreeVRAM      float64   `json:"free_vram_gb"`
	Tags          []string  `json:"tags,omitempty"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	RegisteredAt  time.Time `json:"registered_at"`
}

// Lease tracks the assignment of a job to a worker with expiry.
type Lease struct {
	ID        string    `json:"lease_id"`
	JobID     string    `json:"job_id"`
	WorkerID  string    `json:"worker_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// MetricEntry represents a single metrics data point for a job.
type MetricEntry struct {
	Step      int            `json:"step"`
	Metrics   map[string]any `json:"metrics"`
	CreatedAt time.Time      `json:"created_at,omitempty"`
}

// LogEntry represents a single log line for a job.
type LogEntry struct {
	Line      string    `json:"line"`
	Stream    string    `json:"stream"` // stdout, stderr
	CreatedAt time.Time `json:"created_at"`
}

// ----- Request / Response types (hub.Client compatible) -----

// JobSubmitRequest is the payload for POST /v1/jobs/submit.
type JobSubmitRequest struct {
	Name        string            `json:"name"`
	Workdir     string            `json:"workdir"`
	Command     string            `json:"command"`
	Env         map[string]string `json:"env,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	RequiresGPU bool              `json:"requires_gpu"`
	Priority    int               `json:"priority,omitempty"`
	ExpID       string            `json:"exp_id,omitempty"`
	Memo        string            `json:"memo,omitempty"`
	TimeoutSec  int               `json:"timeout_sec,omitempty"`
}

// JobSubmitResponse is returned from POST /v1/jobs/submit.
type JobSubmitResponse struct {
	JobID         string `json:"job_id"`
	Status        string `json:"status"`
	QueuePosition int    `json:"queue_position"`
}

// JobCompleteRequest is the payload for POST /v1/jobs/{id}/complete.
type JobCompleteRequest struct {
	Status   string `json:"status"`
	ExitCode int    `json:"exit_code"`
}

// QueueStats holds aggregate counts by status.
type QueueStats struct {
	Queued    int `json:"queued"`
	Running   int `json:"running"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	Cancelled int `json:"cancelled"`
}

// WorkerRegisterRequest is the payload for POST /v1/workers/register.
type WorkerRegisterRequest struct {
	Hostname  string   `json:"hostname"`
	GPUCount  int      `json:"gpu_count"`
	GPUModel  string   `json:"gpu_model,omitempty"`
	TotalVRAM float64  `json:"total_vram_gb"`
	FreeVRAM  float64  `json:"free_vram_gb"`
	Tags      []string `json:"tags,omitempty"`
}

// WorkerRegisterResponse is returned from POST /v1/workers/register.
type WorkerRegisterResponse struct {
	WorkerID string `json:"worker_id"`
}

// HeartbeatRequest is the payload for POST /v1/workers/heartbeat.
type HeartbeatRequest struct {
	WorkerID  string  `json:"worker_id"`
	Status    string  `json:"status,omitempty"`
	FreeVRAM  float64 `json:"free_vram_gb,omitempty"`
	GPUCount  int     `json:"gpu_count,omitempty"`
}

// HeartbeatResponse is returned from POST /v1/workers/heartbeat.
type HeartbeatResponse struct {
	Acknowledged bool `json:"acknowledged"`
}

// LeaseAcquireRequest is the payload for POST /v1/leases/acquire.
type LeaseAcquireRequest struct {
	WorkerID string  `json:"worker_id"`
	FreeVRAM float64 `json:"free_vram_gb,omitempty"`
}

// LeaseAcquireResponse is returned from POST /v1/leases/acquire.
type LeaseAcquireResponse struct {
	JobID   string `json:"job_id"`
	LeaseID string `json:"lease_id"`
	Job     Job    `json:"job"`
}

// LeaseRenewRequest is the payload for POST /v1/leases/renew.
type LeaseRenewRequest struct {
	LeaseID  string `json:"lease_id"`
	WorkerID string `json:"worker_id"`
}

// LeaseRenewResponse is returned from POST /v1/leases/renew.
type LeaseRenewResponse struct {
	Renewed      bool   `json:"renewed"`
	NewExpiresAt string `json:"new_expires_at,omitempty"`
}

// MetricsLogRequest is the payload for POST /v1/metrics/{job_id}.
type MetricsLogRequest struct {
	Step    int            `json:"step"`
	Metrics map[string]any `json:"metrics"`
}

// MetricsResponse is returned from GET /v1/metrics/{job_id}.
type MetricsResponse struct {
	JobID      string        `json:"job_id"`
	Metrics    []MetricEntry `json:"metrics"`
	TotalSteps int           `json:"total_steps"`
}

// JobLogsResponse is returned from GET /v1/jobs/{id}/logs.
type JobLogsResponse struct {
	JobID      string   `json:"job_id"`
	Lines      []string `json:"lines"`
	TotalLines int      `json:"total_lines"`
	Offset     int      `json:"offset"`
	HasMore    bool     `json:"has_more"`
}

// JobSummaryResponse is returned from GET /v1/jobs/{id}/summary.
type JobSummaryResponse struct {
	JobID         string         `json:"job_id"`
	Name          string         `json:"name"`
	Status        string         `json:"status"`
	DurationSec   *float64       `json:"duration_seconds,omitempty"`
	ExitCode      *int           `json:"exit_code,omitempty"`
	FailureReason string         `json:"failure_reason,omitempty"`
	Metrics       map[string]any `json:"metrics,omitempty"`
	LogTail       []string       `json:"log_tail,omitempty"`
}

// JobRetryResponse is returned from POST /v1/jobs/{id}/retry.
type JobRetryResponse struct {
	NewJobID      string `json:"new_job_id"`
	Status        string `json:"status"`
	OriginalJobID string `json:"original_job_id"`
}

// EstimateResponse is returned from GET /v1/jobs/{id}/estimate.
type EstimateResponse struct {
	EstimatedDurationSec float64 `json:"estimated_duration_sec"`
	QueueWaitSec         float64 `json:"queue_wait_sec,omitempty"`
	Confidence           float64 `json:"confidence"`
	Method               string  `json:"method"` // historical, similar_jobs, global_avg, default
}

// ----- Command normalization for duration estimation -----

var (
	seedPattern      = regexp.MustCompile(`--seed[= ]\d+`)
	timestampPattern = regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T_ ]\d{2}[:\-]\d{2}`)
	runIDPattern     = regexp.MustCompile(`(?:run|exp|job)[_-]\w{6,}`)
	tmpPathPattern   = regexp.MustCompile(`/tmp/\S+`)
	epochPattern     = regexp.MustCompile(`\b1[6-9]\d{8,9}\b`)
)

// NormalizeCommandHash normalizes a command and returns its SHA-256 prefix.
func NormalizeCommandHash(command string) string {
	normalized := normalizeCommand(command)
	h := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("%x", h[:8])
}

func normalizeCommand(cmd string) string {
	cmd = seedPattern.ReplaceAllString(cmd, "--seed SEED")
	cmd = timestampPattern.ReplaceAllString(cmd, "TIMESTAMP")
	cmd = runIDPattern.ReplaceAllString(cmd, "RUN_ID")
	cmd = tmpPathPattern.ReplaceAllString(cmd, "/tmp/TMPPATH")
	cmd = epochPattern.ReplaceAllString(cmd, "EPOCH")
	cmd = strings.Join(strings.Fields(cmd), " ")
	return cmd
}
