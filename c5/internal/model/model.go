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

// ArtifactRef is a reference to an artifact in the c5-artifacts bucket.
type ArtifactRef struct {
	Path      string `json:"path"`                 // c5-artifacts bucket 내 경로
	LocalPath string `json:"local_path,omitempty"` // 워커 로컬 저장 경로
	Required  bool   `json:"required,omitempty"`   // false 시 missing 허용
}

// Job represents a submitted job with its full lifecycle state.
type Job struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Status          JobStatus         `json:"status"`
	Priority        int               `json:"priority"`
	Workdir         string            `json:"workdir"`
	Command         string            `json:"command"`
	RequiresGPU     bool              `json:"requires_gpu"`
	VRAMRequiredGB  float64           `json:"vram_required_gb,omitempty"`
	Env             map[string]string `json:"env,omitempty"`
	Tags            []string          `json:"tags,omitempty"`
	ExpID           string            `json:"exp_id,omitempty"`
	Memo            string            `json:"memo,omitempty"`
	TimeoutSec      int               `json:"timeout_sec,omitempty"`
	ProjectID       string            `json:"project_id,omitempty"`
	SubmittedBy     string            `json:"submitted_by,omitempty"`
	WorkerID        string            `json:"worker_id,omitempty"`
	InputArtifacts  []ArtifactRef     `json:"input_artifacts,omitempty"`
	OutputArtifacts []ArtifactRef     `json:"output_artifacts,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	StartedAt       *time.Time        `json:"started_at,omitempty"`
	FinishedAt      *time.Time        `json:"finished_at,omitempty"`
	ExitCode        *int              `json:"exit_code,omitempty"`
	Capability      string            `json:"capability,omitempty"`
	Params          map[string]any    `json:"params,omitempty"`
	Result          map[string]any    `json:"result,omitempty"`
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

// Capability describes a named function a worker can perform (MCP tool equivalent).
// Workers declare capabilities at registration; agents discover and invoke them.
type Capability struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	Version     string         `json:"version,omitempty"`
	Command     string         `json:"command,omitempty"` // script to run; receives C5_PARAMS env var
}

// CapabilityRegistration is a Capability with worker routing metadata.
type CapabilityRegistration struct {
	Capability
	WorkerID  string `json:"worker_id"`
	ProjectID string `json:"project_id,omitempty"`
	UpdatedAt string `json:"updated_at"`
}

// CapabilityWorker is a worker summary inside a CapabilityGroup.
type CapabilityWorker struct {
	WorkerID string `json:"worker_id"`
	Hostname string `json:"hostname,omitempty"`
	Status   string `json:"status"`
	GPUModel string `json:"gpu_model,omitempty"`
}

// CapabilityGroup aggregates workers that offer the same capability.
type CapabilityGroup struct {
	Capability
	Workers []CapabilityWorker `json:"workers"`
}

// CapabilityListResponse is returned from GET /v1/capabilities.
type CapabilityListResponse struct {
	Capabilities []CapabilityGroup `json:"capabilities"`
}

// CapabilityInvokeRequest is the payload for POST /v1/capabilities/invoke.
type CapabilityInvokeRequest struct {
	Capability string         `json:"capability"`
	Params     map[string]any `json:"params,omitempty"`
	Name       string         `json:"name,omitempty"`
	Priority   int            `json:"priority,omitempty"`
	TimeoutSec int            `json:"timeout_sec,omitempty"`
	Memo       string         `json:"memo,omitempty"`
	ProjectID  string         `json:"project_id,omitempty"`
}

// CapabilityInvokeResponse is returned from POST /v1/capabilities/invoke.
type CapabilityInvokeResponse struct {
	JobID         string `json:"job_id"`
	Status        string `json:"status"`
	QueuePosition int    `json:"queue_position"`
	Capability    string `json:"capability"`
}

// CapabilityUpdateRequest is the payload for POST /v1/capabilities/update.
type CapabilityUpdateRequest struct {
	WorkerID      string       `json:"worker_id"`
	CapabilitySet []Capability `json:"capability_set"`
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
	ProjectID     string    `json:"project_id,omitempty"`
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
	Name            string            `json:"name"`
	Workdir         string            `json:"workdir"`
	Command         string            `json:"command"`
	Env             map[string]string `json:"env,omitempty"`
	Tags            []string          `json:"tags,omitempty"`
	RequiresGPU     bool              `json:"requires_gpu"`
	VRAMRequiredGB  float64           `json:"vram_required_gb,omitempty"`
	Priority        int               `json:"priority,omitempty"`
	ExpID           string            `json:"exp_id,omitempty"`
	Memo            string            `json:"memo,omitempty"`
	TimeoutSec      int               `json:"timeout_sec,omitempty"`
	ProjectID       string            `json:"project_id,omitempty"`
	SubmittedBy     string            `json:"submitted_by,omitempty"`
	InputArtifacts  []ArtifactRef     `json:"input_artifacts,omitempty"`
	OutputArtifacts []ArtifactRef     `json:"output_artifacts,omitempty"`
	Capability      string            `json:"capability,omitempty"`
	Params          map[string]any    `json:"params,omitempty"`
}

// JobSubmitResponse is returned from POST /v1/jobs/submit.
type JobSubmitResponse struct {
	JobID         string `json:"job_id"`
	Status        string `json:"status"`
	QueuePosition int    `json:"queue_position"`
}

// JobCompleteRequest is the payload for POST /v1/jobs/{id}/complete.
type JobCompleteRequest struct {
	Status   string         `json:"status"`
	ExitCode *int           `json:"exit_code,omitempty"`
	Result   map[string]any `json:"result,omitempty"` // structured result from capability execution
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
// hub.Client sends {"capabilities": {...}} — the handler extracts fields from the map.
type WorkerRegisterRequest struct {
	Hostname     string         `json:"hostname"`
	GPUCount     int            `json:"gpu_count"`
	GPUModel     string         `json:"gpu_model,omitempty"`
	TotalVRAM    float64        `json:"total_vram_gb"`
	FreeVRAM     float64        `json:"free_vram_gb"`
	Tags         []string       `json:"tags,omitempty"`
	ProjectID    string         `json:"project_id,omitempty"`
	Capabilities  map[string]any `json:"capabilities,omitempty"`
	CapabilitySet []Capability   `json:"capability_set,omitempty"` // structured capabilities
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
	WorkerID    string  `json:"worker_id"`
	FreeVRAM    float64 `json:"free_vram_gb,omitempty"`
	WaitSeconds int     `json:"wait_seconds,omitempty"` // 0 = return immediately, >0 = long-poll timeout
}

// InputPresignedArtifact is a presigned download URL for a job input artifact.
type InputPresignedArtifact struct {
	Path      string `json:"path"`
	LocalPath string `json:"local_path,omitempty"`
	URL       string `json:"url"`
	ExpiresAt string `json:"expires_at,omitempty"`
	Required  bool   `json:"required,omitempty"` // propagated from ArtifactRef; false = skip on failure
}

// LeaseAcquireResponse is returned from POST /v1/leases/acquire.
type LeaseAcquireResponse struct {
	JobID               string                   `json:"job_id"`
	LeaseID             string                   `json:"lease_id"`
	Job                 Job                      `json:"job"`
	InputPresignedURLs  []InputPresignedArtifact `json:"input_presigned_urls,omitempty"`
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
// Fields are compatible with hub.Client's JobEstimateResponse.
type EstimateResponse struct {
	EstimatedDurationSec float64 `json:"estimated_duration_sec"`
	QueueWaitSec         float64 `json:"queue_wait_sec,omitempty"`
	EstimatedStartTime   string  `json:"estimated_start_time,omitempty"`
	Confidence           float64 `json:"confidence"`
	Method               string  `json:"method"` // historical, similar_jobs, global_avg, default
	BlockingReason       *string `json:"blocking_reason,omitempty"`
}

// =========================================================================
// DAG Models (hub.Client compatible)
// =========================================================================

// DAG represents a directed acyclic graph of job nodes.
type DAG struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	Tags         []string        `json:"tags,omitempty"`
	ProjectID    string          `json:"project_id,omitempty"`
	Status       string          `json:"status,omitempty"` // pending, running, completed, failed
	Nodes        []DAGNode       `json:"nodes,omitempty"`
	Dependencies []DAGDependency `json:"dependencies,omitempty"`
	CreatedAt    string          `json:"created_at,omitempty"`
	StartedAt    string          `json:"started_at,omitempty"`
	FinishedAt   string          `json:"finished_at,omitempty"`
}

// DAGNode represents a single executable node in a DAG.
type DAGNode struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Command     string            `json:"command"`
	WorkingDir  string            `json:"working_dir,omitempty"`
	Env         map[string]string `json:"environment,omitempty"`
	GPUCount    int               `json:"gpu_count,omitempty"`
	MaxRetries  int               `json:"max_retries,omitempty"`
	Status      string            `json:"status,omitempty"` // pending, running, succeeded, failed, skipped
	JobID       string            `json:"job_id,omitempty"`
	StartedAt   string            `json:"started_at,omitempty"`
	FinishedAt  string            `json:"finished_at,omitempty"`
	ExitCode    *int              `json:"exit_code,omitempty"`
}

// DAGDependency represents a directed edge between two nodes.
type DAGDependency struct {
	SourceID string `json:"source_id"`
	TargetID string `json:"target_id"`
	Type     string `json:"dependency_type,omitempty"` // sequential, data_dependency, conditional
}

// DAGCreateRequest is the payload for POST /v1/dags.
type DAGCreateRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// DAGCreateResponse is the response from POST /v1/dags.
type DAGCreateResponse struct {
	DAGID  string `json:"dag_id"`
	Status string `json:"status"`
}

// DAGAddNodeRequest is the payload for POST /v1/dags/{id}/nodes.
type DAGAddNodeRequest struct {
	Name        string            `json:"name"`
	Command     string            `json:"command"`
	Description string            `json:"description,omitempty"`
	WorkingDir  string            `json:"working_dir,omitempty"`
	Env         map[string]string `json:"environment,omitempty"`
	GPUCount    int               `json:"gpu_count,omitempty"`
	MaxRetries  int               `json:"max_retries,omitempty"`
}

// DAGAddNodeResponse is the response from POST /v1/dags/{id}/nodes.
type DAGAddNodeResponse struct {
	NodeID string `json:"node_id"`
	Name   string `json:"name"`
}

// DAGAddDependencyRequest is the payload for POST /v1/dags/{id}/dependencies.
type DAGAddDependencyRequest struct {
	SourceID string `json:"source_id"`
	TargetID string `json:"target_id"`
	Type     string `json:"dependency_type,omitempty"`
}

// DAGExecuteRequest is the payload for POST /v1/dags/{id}/execute.
type DAGExecuteRequest struct {
	DryRun bool `json:"dry_run,omitempty"`
}

// DAGExecuteResponse is the response from POST /v1/dags/{id}/execute.
type DAGExecuteResponse struct {
	DAGID      string   `json:"dag_id"`
	Status     string   `json:"status"`
	NodeOrder  []string `json:"node_order,omitempty"`
	Validation string   `json:"validation,omitempty"`
	Errors     []string `json:"errors,omitempty"`
}

// DAGFromYAMLRequest is the payload for POST /v1/dags/from-yaml.
type DAGFromYAMLRequest struct {
	YAMLContent string `json:"yaml_content"`
}

// =========================================================================
// Edge Models (hub.Client compatible)
// =========================================================================

// Edge represents a registered edge device for artifact deployment.
type Edge struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	ProjectID string            `json:"project_id,omitempty"`
	Status    string            `json:"status"` // online, offline
	Tags      []string          `json:"tags,omitempty"`
	Arch      string            `json:"arch,omitempty"`
	Runtime   string            `json:"runtime,omitempty"`
	Storage   float64           `json:"storage_gb,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	LastSeen  string            `json:"last_seen,omitempty"`
}

// EdgeRegisterRequest is the payload for POST /v1/edges/register.
type EdgeRegisterRequest struct {
	Name    string            `json:"name"`
	Tags    []string          `json:"tags,omitempty"`
	Arch    string            `json:"arch,omitempty"`
	Runtime string            `json:"runtime,omitempty"`
	Storage float64           `json:"storage_gb,omitempty"`
	Meta    map[string]string `json:"metadata,omitempty"`
}

// EdgeRegisterResponse is the response from POST /v1/edges/register.
type EdgeRegisterResponse struct {
	EdgeID string `json:"edge_id"`
}

// EdgeHeartbeatRequest is the payload for POST /v1/edges/heartbeat.
type EdgeHeartbeatRequest struct {
	EdgeID string `json:"edge_id"`
	Status string `json:"status,omitempty"`
}

// =========================================================================
// Deploy Models (hub.Client compatible)
// =========================================================================

// DeployRule defines an automatic deployment trigger.
type DeployRule struct {
	ID              string `json:"id"`
	Name            string `json:"name,omitempty"`
	ProjectID       string `json:"project_id,omitempty"`
	Trigger         string `json:"trigger"`
	EdgeFilter      string `json:"edge_filter"`
	ArtifactPattern string `json:"artifact_pattern"`
	PostCommand     string `json:"post_command,omitempty"`
	Enabled         bool   `json:"enabled"`
	CreatedAt       string `json:"created_at,omitempty"`
}

// DeployRuleCreateRequest is the payload for POST /v1/deploy/rules.
type DeployRuleCreateRequest struct {
	Name            string `json:"name,omitempty"`
	Trigger         string `json:"trigger"`
	EdgeFilter      string `json:"edge_filter"`
	ArtifactPattern string `json:"artifact_pattern"`
	PostCommand     string `json:"post_command,omitempty"`
}

// DeployRuleCreateResponse is the response from POST /v1/deploy/rules.
type DeployRuleCreateResponse struct {
	RuleID string `json:"rule_id"`
}

// Deployment represents a deployment instance.
type Deployment struct {
	ID         string         `json:"id"`
	RuleID     string         `json:"rule_id,omitempty"`
	JobID      string         `json:"job_id,omitempty"`
	Status     string         `json:"status"` // pending, deploying, completed, failed, partial
	Targets    []DeployTarget `json:"targets,omitempty"`
	CreatedAt  string         `json:"created_at,omitempty"`
	FinishedAt string         `json:"finished_at,omitempty"`
}

// DeployTarget represents the deployment status for a single edge device.
type DeployTarget struct {
	EdgeID    string `json:"edge_id"`
	EdgeName  string `json:"edge_name,omitempty"`
	Status    string `json:"status"` // pending, downloading, deploying, succeeded, failed
	Error     string `json:"error,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
	DoneAt    string `json:"done_at,omitempty"`
}

// DeployTriggerRequest is the payload for POST /v1/deploy/trigger.
type DeployTriggerRequest struct {
	JobID           string   `json:"job_id"`
	RuleID          string   `json:"rule_id,omitempty"` // set when creating deployment from a rule
	ArtifactPattern string   `json:"artifact_pattern,omitempty"`
	EdgeFilter      string   `json:"edge_filter,omitempty"`
	EdgeIDs         []string `json:"edge_ids,omitempty"`
	PostCommand     string   `json:"post_command,omitempty"`
}

// DeployTriggerResponse is the response from POST /v1/deploy/trigger.
type DeployTriggerResponse struct {
	DeployID    string `json:"deploy_id"`
	Status      string `json:"status"`
	TargetCount int    `json:"target_count"`
}

// PendingAssignment is a pending deployment assignment for an edge (store layer).
type PendingAssignment struct {
	DeployID        string `json:"deploy_id"`
	JobID           string `json:"job_id"`
	ArtifactPattern string `json:"artifact_pattern"`
	PostCommand     string `json:"post_command,omitempty"`
}

// DeployAssignmentArtifact is one artifact with download URL (API response).
type DeployAssignmentArtifact struct {
	Path string `json:"path"`
	URL  string `json:"url"`
}

// DeployAssignmentResponse is one item of GET /v1/deploy/assignments/{edge_id} response.
type DeployAssignmentResponse struct {
	DeployID        string                     `json:"deploy_id"`
	JobID           string                     `json:"job_id"`
	ArtifactPattern string                     `json:"artifact_pattern"`
	PostCommand     string                     `json:"post_command,omitempty"`
	Artifacts       []DeployAssignmentArtifact `json:"artifacts,omitempty"`
}

// DeployTargetStatusRequest is the payload for POST /v1/deploy/target-status (edge agent reports target status).
type DeployTargetStatusRequest struct {
	DeployID string `json:"deploy_id"`
	EdgeID   string `json:"edge_id"`
	Status   string `json:"status"` // downloading, deploying, succeeded, failed
	Error    string `json:"error,omitempty"`
}

// =========================================================================
// Artifact Models (hub.Client compatible)
// =========================================================================

// Artifact represents a file artifact associated with a job.
type Artifact struct {
	ID          string `json:"id"`
	JobID       string `json:"job_id"`
	Path        string `json:"path"`
	ContentHash string `json:"content_hash,omitempty"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
	Confirmed   bool   `json:"confirmed"`
	CreatedAt   string `json:"created_at,omitempty"`
}

// PresignedURLRequest is the payload for POST /v1/storage/presigned-url.
type PresignedURLRequest struct {
	Path        string `json:"path"`
	Method      string `json:"method"` // GET or PUT
	TTLSeconds  int    `json:"ttl_seconds,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}

// PresignedURLResponse is the response from POST /v1/storage/presigned-url.
type PresignedURLResponse struct {
	URL       string `json:"url"`
	ExpiresAt string `json:"expires_at"`
}

// ArtifactConfirmRequest is the payload for POST /v1/artifacts/{job_id}/confirm.
type ArtifactConfirmRequest struct {
	Path        string `json:"path"`
	ContentHash string `json:"content_hash"`
	SizeBytes   int64  `json:"size_bytes"`
}

// ArtifactConfirmResponse is the response from POST /v1/artifacts/{job_id}/confirm.
type ArtifactConfirmResponse struct {
	ArtifactID string `json:"artifact_id"`
	Confirmed  bool   `json:"confirmed"`
}

// ArtifactURLResponse is the response from GET /v1/artifacts/{job_id}/url/{name}.
type ArtifactURLResponse struct {
	URL string `json:"url"`
}

// =========================================================================
// WebSocket Models (hub.Client compatible)
// =========================================================================

// MetricMessage represents a message on the metrics WebSocket.
type MetricMessage struct {
	Type    string         `json:"type"` // metric, status, history, error
	JobID   string         `json:"job_id,omitempty"`
	Step    int            `json:"step,omitempty"`
	Metrics map[string]any `json:"metrics,omitempty"`
	Status  string         `json:"status,omitempty"`
	Error   string         `json:"error,omitempty"`
}

// =========================================================================
// API Key Models
// =========================================================================

// APIKeyInfo represents metadata about an API key (hash is never exposed).
type APIKeyInfo struct {
	KeyHash     string `json:"key_hash"`
	ProjectID   string `json:"project_id"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// CreateAPIKeyRequest is the payload for POST /v1/admin/api-keys.
type CreateAPIKeyRequest struct {
	ProjectID   string `json:"project_id"`
	Description string `json:"description,omitempty"`
}

// CreateAPIKeyResponse is the response from POST /v1/admin/api-keys.
type CreateAPIKeyResponse struct {
	Key       string `json:"key"`        // raw key (only shown once)
	KeyHash   string `json:"key_hash"`   // SHA256 hash for reference
	ProjectID string `json:"project_id"`
}

// =========================================================================
// Device Auth Models (OAuth 2.0 Device Authorization Grant)
// =========================================================================

// DeviceSessionStatus is the lifecycle state of an OAuth device session.
type DeviceSessionStatus string

const (
	DeviceSessionPending DeviceSessionStatus = "pending" // waiting for browser callback
	DeviceSessionReady   DeviceSessionStatus = "ready"   // auth_code received, ready for token exchange
	DeviceSessionExpired DeviceSessionStatus = "expired" // expired or rate-limited
)

// DeviceSession tracks a device authorization flow session.
type DeviceSession struct {
	State         string    `json:"state"`
	UserCode      string    `json:"user_code"`
	CSRFToken     string    `json:"csrf_token,omitempty"`
	CodeChallenge string    `json:"code_challenge"`
	SupabaseURL   string    `json:"supabase_url"`
	AuthCode      string    `json:"-"`      // never serialised to client
	Status        string    `json:"status"` // pending, ready, expired
	PollCount     int       `json:"poll_count"`
	TokenAttempts int       `json:"token_attempts"`
	ExpiresAt     time.Time `json:"expires_at"`
	CreatedAt     time.Time `json:"created_at"`
}

// TokenRequest is the body for POST /v1/auth/device/{state}/token.
type TokenRequest struct {
	CodeVerifier string `json:"code_verifier"`
}

// ctxKey is a context key type for auth data.
type ctxKey string

const (
	// CtxProjectID holds the authenticated project ID in request context.
	CtxProjectID ctxKey = "project_id"
	// CtxIsMaster indicates whether the request was made with the master key.
	CtxIsMaster ctxKey = "is_master"
)

// SHA256Hex returns the hex-encoded SHA256 hash of s.
func SHA256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:])
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
