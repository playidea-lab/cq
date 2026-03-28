// Package hub implements a client for the PiQ Hub REST API.
//
// It provides job submission, status tracking, worker management,
// and metrics collection — all using stdlib net/http with zero
// external dependencies, following the cloud/ and llm/ patterns.
package hub

import "encoding/json"

// HubConfig holds Hub connection settings.
type HubConfig struct {
	Enabled      bool   `mapstructure:"enabled"       yaml:"enabled"`
	URL          string `mapstructure:"url"           yaml:"url"`           // legacy Hub server URL (kept for backward compat)
	APIPrefix    string `mapstructure:"api_prefix"    yaml:"api_prefix"`    // e.g. "/v1" for Hub server, "" for local daemon
	APIKey       string `mapstructure:"api_key"       yaml:"api_key"`       // legacy Hub API key
	APIKeyEnv    string `mapstructure:"api_key_env"   yaml:"api_key_env"`
	TeamID       string `mapstructure:"team_id"       yaml:"team_id"`
	SupabaseURL  string `mapstructure:"supabase_url"  yaml:"supabase_url"`  // Supabase project URL (overrides URL)
	SupabaseKey  string `mapstructure:"supabase_key"  yaml:"supabase_key"`  // Supabase anon key
}

// Job represents a Hub job.
// Supports both Hub server ("id") and PiQ daemon ("job_id") field names.
type Job struct {
	ID          string            `json:"id"`
	JobID       string            `json:"job_id,omitempty"` // PiQ daemon uses job_id instead of id
	Name        string            `json:"name"`
	Status      string            `json:"status"` // QUEUED, RUNNING, SUCCEEDED, FAILED, CANCELLED
	Priority    int               `json:"priority"`
	Workdir     string            `json:"workdir"`
	Command     string            `json:"command"`
	RequiresGPU    bool              `json:"requires_gpu"`
	VRAMRequiredGB float64           `json:"vram_required_gb,omitempty"`
	Env            json.RawMessage   `json:"env,omitempty"`
	Tags        json.RawMessage   `json:"tags,omitempty"`
	ExpID       string            `json:"exp_id,omitempty"`
	Memo        string            `json:"memo,omitempty"`
	TimeoutSec  int               `json:"timeout_sec,omitempty"`
	CreatedAt   string            `json:"created_at,omitempty"`
	StartedAt   string            `json:"started_at,omitempty"`
	FinishedAt  string            `json:"finished_at,omitempty"`
	ExitCode        *int              `json:"exit_code,omitempty"`
	WorkerID        string            `json:"worker_id,omitempty"`
	InputArtifacts  json.RawMessage   `json:"input_artifacts,omitempty"`
	OutputArtifacts json.RawMessage   `json:"output_artifacts,omitempty"`
	BestMetric      *float64          `json:"best_metric,omitempty"`
	PrimaryMetric   string            `json:"primary_metric,omitempty"`
	LowerIsBetter   *bool             `json:"lower_is_better,omitempty"`
	Capability      string            `json:"capability,omitempty"`
	Result          json.RawMessage   `json:"result,omitempty"`
	Datasets        []string          `json:"datasets,omitempty"`
}

// GetID returns the job ID, preferring "id" (Hub) but falling back to "job_id" (PiQ daemon).
func (j *Job) GetID() string {
	if j.ID != "" {
		return j.ID
	}
	return j.JobID
}

// ArtifactRef references an artifact by its Hub path and optional local path.
type ArtifactRef struct {
	Path      string `json:"path"`
	LocalPath string `json:"local_path,omitempty"`
	Required  bool   `json:"required,omitempty"`
}

// JobSubmitRequest is the payload for POST /v1/jobs/submit.
type JobSubmitRequest struct {
	ID                  string            `json:"id,omitempty"`
	Name                string            `json:"name"`
	Workdir             string            `json:"workdir"`
	Command             string            `json:"command"`
	Env                 map[string]string `json:"env,omitempty"`
	Tags                []string          `json:"tags,omitempty"`
	RequiresGPU         bool              `json:"requires_gpu"`
	VRAMRequiredGB      float64           `json:"vram_required_gb,omitempty"`
	Priority            int               `json:"priority,omitempty"`
	ExpID               string            `json:"exp_id,omitempty"`
	ExpRunID            string            `json:"exp_run_id,omitempty"`
	Memo                string            `json:"memo,omitempty"`
	TimeoutSec          int               `json:"timeout_sec,omitempty"`
	InputArtifacts      []ArtifactRef     `json:"input_artifacts,omitempty"`
	OutputArtifacts     []ArtifactRef     `json:"output_artifacts,omitempty"`
	SnapshotVersionHash string            `json:"snapshot_version_hash,omitempty"`
	GitHash             string            `json:"git_hash,omitempty"`
	ProjectID           string            `json:"project_id,omitempty"`
	Capability          string            `json:"capability,omitempty"`
	RequiredTags        []string          `json:"required_tags,omitempty"`
	TargetWorker        string            `json:"target_worker,omitempty"`
	Params              map[string]any    `json:"params,omitempty"`
	Datasets            []string          `json:"datasets,omitempty"`
	PrimaryMetric       string            `json:"primary_metric,omitempty"`
	LowerIsBetter       *bool             `json:"lower_is_better,omitempty"`
}

// JobSubmitResponse is the response from POST /v1/jobs/submit.
type JobSubmitResponse struct {
	JobID         string `json:"job_id"`
	Status        string `json:"status"`
	QueuePosition int    `json:"queue_position"`
}

// Worker represents a Hub worker.
type Worker struct {
	ID           string         `json:"id"`
	Name         string         `json:"name,omitempty"`
	Hostname     string         `json:"hostname,omitempty"`
	Status       string         `json:"status"`
	GPUCount     int            `json:"gpu_count"`
	GPUModel     string         `json:"gpu_model,omitempty"`
	TotalVRAM    float64        `json:"total_vram_gb"`
	FreeVRAM     float64        `json:"free_vram_gb"`
	GPUs         []GPUInfo      `json:"gpus,omitempty"`
	UptimeSec    int64          `json:"uptime_sec,omitempty"`
	LastJobAt    string         `json:"last_job_at,omitempty"`
	Capabilities []string       `json:"capabilities,omitempty"`
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

// InputPresignedArtifact is a pre-signed download URL for an input artifact.
type InputPresignedArtifact struct {
	Path      string `json:"path"`
	LocalPath string `json:"local_path,omitempty"`
	URL       string `json:"url"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

// ClaimResponse is the response from POST /v1/leases/acquire.
type ClaimResponse struct {
	JobID              string                   `json:"job_id"`
	LeaseID            string                   `json:"lease_id"`
	Job                Job                      `json:"job"`
	InputPresignedURLs []InputPresignedArtifact `json:"input_presigned_urls,omitempty"`
}

// RenewLeaseResponse is the response from POST /v1/leases/renew.
type RenewLeaseResponse struct {
	Renewed      bool   `json:"renewed"`
	NewExpiresAt string `json:"new_expires_at,omitempty"`
}

// JobLogsResponse is the response from GET /v1/jobs/{id}/logs.
type JobLogsResponse struct {
	JobID      string   `json:"job_id"`
	Lines      []string `json:"lines"`
	TotalLines int      `json:"total_lines"`
	Offset     int      `json:"offset"`
	HasMore    bool     `json:"has_more"`
}

// JobSummaryResponse is the response from GET /v1/jobs/{id}/summary.
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

// JobRetryResponse is the response from POST /v1/jobs/{id}/retry.
type JobRetryResponse struct {
	NewJobID      string `json:"new_job_id"`
	Status        string `json:"status"`
	OriginalJobID string `json:"original_job_id"`
}

// JobEstimateResponse is the response from GET /jobs/{id}/estimate.
// Confidence may be string ("high"/"medium"/"low") from Hub server
// or float (0.0-1.0) from PiQ daemon.
type JobEstimateResponse struct {
	EstimatedDurationSec float64 `json:"estimated_duration_sec"`
	QueueWaitSec         float64 `json:"queue_wait_sec,omitempty"`
	EstimatedStartTime   string  `json:"estimated_start_time,omitempty"`
	EstimatedEndTime     string  `json:"estimated_completion_time,omitempty"`
	Confidence           any     `json:"confidence"`           // string or float64
	Method               string  `json:"method"`               // historical, similar_jobs, default, global_avg
	BlockingReason       *string `json:"blocking_reason,omitempty"` // PiQ daemon field
}
