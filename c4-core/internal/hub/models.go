// Package hub implements a client for the PiQ Hub REST API.
//
// It provides job submission, status tracking, worker management,
// and metrics collection — all using stdlib net/http with zero
// external dependencies, following the cloud/ and llm/ patterns.
package hub

// HubConfig holds Hub connection settings.
type HubConfig struct {
	Enabled   bool   `mapstructure:"enabled"     yaml:"enabled"`
	URL       string `mapstructure:"url"         yaml:"url"`
	APIKey    string `mapstructure:"api_key"     yaml:"api_key"`
	APIKeyEnv string `mapstructure:"api_key_env" yaml:"api_key_env"`
	TeamID    string `mapstructure:"team_id"     yaml:"team_id"`
}

// Job represents a Hub job.
type Job struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Status      string            `json:"status"` // QUEUED, RUNNING, SUCCEEDED, FAILED, CANCELLED
	Priority    int               `json:"priority"`
	Workdir     string            `json:"workdir"`
	Command     string            `json:"command"`
	RequiresGPU bool              `json:"requires_gpu"`
	Env         map[string]string `json:"env,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	ExpID       string            `json:"exp_id,omitempty"`
	Memo        string            `json:"memo,omitempty"`
	TimeoutSec  int               `json:"timeout_sec,omitempty"`
	CreatedAt   string            `json:"created_at,omitempty"`
	StartedAt   string            `json:"started_at,omitempty"`
	FinishedAt  string            `json:"finished_at,omitempty"`
	ExitCode    *int              `json:"exit_code,omitempty"`
	WorkerID    string            `json:"worker_id,omitempty"`
}

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

// JobSubmitResponse is the response from POST /v1/jobs/submit.
type JobSubmitResponse struct {
	JobID         string `json:"job_id"`
	Status        string `json:"status"`
	QueuePosition int    `json:"queue_position"`
}

// Worker represents a Hub worker.
type Worker struct {
	ID        string    `json:"id"`
	Hostname  string    `json:"hostname,omitempty"`
	Status    string    `json:"status"`
	GPUCount  int       `json:"gpu_count"`
	GPUModel  string    `json:"gpu_model,omitempty"`
	TotalVRAM float64   `json:"total_vram_gb"`
	FreeVRAM  float64   `json:"free_vram_gb"`
	GPUs      []GPUInfo `json:"gpus,omitempty"`
}

// GPUInfo holds per-GPU details.
type GPUInfo struct {
	Index       int     `json:"index"`
	Name        string  `json:"name"`
	TotalVRAM   float64 `json:"total_vram_gb"`
	FreeVRAM    float64 `json:"free_vram_gb"`
	Utilization int     `json:"gpu_util_percent"`
	Temperature float64 `json:"temperature"`
}

// MetricEntry represents a single metrics data point.
type MetricEntry struct {
	Step    int            `json:"step"`
	Metrics map[string]any `json:"metrics"`
}

// MetricsResponse is the response from GET /v1/metrics/{id}.
type MetricsResponse struct {
	JobID      string        `json:"job_id"`
	Metrics    []MetricEntry `json:"metrics"`
	TotalSteps int           `json:"total_steps"`
}

// QueueStats holds queue-level statistics.
type QueueStats struct {
	Queued    int `json:"queued"`
	Running   int `json:"running"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	Cancelled int `json:"cancelled"`
}

// WorkerRegisterResponse is the response from POST /v1/workers/register.
type WorkerRegisterResponse struct {
	WorkerID string `json:"worker_id"`
}

// HeartbeatResponse is the response from POST /v1/workers/heartbeat.
type HeartbeatResponse struct {
	Acknowledged bool `json:"acknowledged"`
}

// ClaimResponse is the response from POST /v1/leases/acquire.
type ClaimResponse struct {
	JobID   string `json:"job_id"`
	LeaseID string `json:"lease_id"`
	Job     Job    `json:"job"`
}

// RenewLeaseResponse is the response from POST /v1/leases/renew.
type RenewLeaseResponse struct {
	Renewed      bool   `json:"renewed"`
	NewExpiresAt string `json:"new_expires_at,omitempty"`
}
