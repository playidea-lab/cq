// Package daemon implements a local job scheduler and HTTP server.
//
// It replaces the Python PiQ daemon with a Go implementation that
// provides job queuing, process execution, GPU monitoring, and
// PiQ-compatible REST API — using only stdlib and modernc.org/sqlite.
package daemon

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
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

// Job represents a scheduled job with its full lifecycle state.
type Job struct {
	ID           string            `json:"job_id"`
	Name         string            `json:"name"`
	Status       JobStatus         `json:"status"`
	Priority     int               `json:"priority"`
	Workdir      string            `json:"workdir"`
	Command      string            `json:"command"`
	RequiresGPU  bool              `json:"requires_gpu"`
	GPUCount     int               `json:"gpu_count"`
	Env          map[string]string `json:"env,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
	ExpID        string            `json:"exp_id,omitempty"`
	Memo         string            `json:"memo,omitempty"`
	TimeoutSec   int               `json:"timeout_sec,omitempty"`
	Capability   string            `json:"capability,omitempty"`
	RequiredTags []string          `json:"required_tags,omitempty"`
	TargetWorker string            `json:"target_worker,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	StartedAt    *time.Time        `json:"started_at,omitempty"`
	FinishedAt   *time.Time        `json:"finished_at,omitempty"`
	ExitCode     *int              `json:"exit_code,omitempty"`
	PID          int               `json:"pid,omitempty"`
	GPUIndices   []int             `json:"gpu_indices,omitempty"`
	MetricsPath  string            `json:"metrics_path,omitempty"`
	Metrics      map[string]any    `json:"metrics,omitempty"`
}

// DurationSec returns the job duration in seconds, or nil if not yet finished.
func (j *Job) DurationSec() *float64 {
	if j.StartedAt == nil || j.FinishedAt == nil {
		return nil
	}
	d := j.FinishedAt.Sub(*j.StartedAt).Seconds()
	return &d
}

// CommandHash returns a SHA-256 prefix of the command string,
// used for duration estimation based on similar past jobs.
func (j *Job) CommandHash() string {
	h := sha256.Sum256([]byte(j.Command))
	return fmt.Sprintf("%x", h[:8])
}

// JobSubmitRequest is the payload for POST /jobs/submit.
type JobSubmitRequest struct {
	Name         string            `json:"name"`
	Workdir      string            `json:"workdir"`
	Command      string            `json:"command"`
	Env          map[string]string `json:"env,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
	RequiresGPU  bool              `json:"requires_gpu"`
	GPUCount     int               `json:"gpu_count,omitempty"`
	Priority     int               `json:"priority,omitempty"`
	ExpID        string            `json:"exp_id,omitempty"`
	Memo         string            `json:"memo,omitempty"`
	TimeoutSec   int               `json:"timeout_sec,omitempty"`
	MetricsPath  string            `json:"metrics_path,omitempty"`
	Capability   string            `json:"capability,omitempty"`
	RequiredTags []string          `json:"required_tags,omitempty"`
	TargetWorker string            `json:"target_worker,omitempty"`
}

// JobSubmitResponse is returned from POST /jobs/submit.
type JobSubmitResponse struct {
	JobID         string `json:"job_id"`
	Status        string `json:"status"`
	QueuePosition int    `json:"queue_position"`
}

// JobCompleteRequest is the payload for POST /jobs/{id}/complete.
type JobCompleteRequest struct {
	Status   string `json:"status"`
	ExitCode int    `json:"exit_code"`
}

// QueueStats holds queue-level statistics.
type QueueStats struct {
	Queued    int `json:"queued"`
	Running   int `json:"running"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	Cancelled int `json:"cancelled"`
}

// GpuInfo holds per-GPU details from nvidia-smi.
type GpuInfo struct {
	Index       int     `json:"index"`
	Name        string  `json:"name"`
	TotalVRAM   float64 `json:"total_vram_gb"`
	FreeVRAM    float64 `json:"free_vram_gb"`
	Utilization int     `json:"gpu_util_percent"`
	Temperature float64 `json:"temperature"`
}

// JobDuration records historical execution time for command estimation.
type JobDuration struct {
	CommandHash string  `json:"command_hash"`
	DurationSec float64 `json:"duration_sec"`
	CreatedAt   time.Time
}

// marshalJSON is a helper that marshals a value to a JSON string,
// returning "null" on error. Used for storing maps/slices in SQLite.
func marshalJSON(v any) string {
	if v == nil {
		return "null"
	}
	data, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(data)
}
