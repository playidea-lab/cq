package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	maxRetries    = 3
	retryBaseWait = 1 * time.Second
)

// Client communicates with a PiQ Hub server over REST.
type Client struct {
	baseURL    string
	apiPrefix  string // e.g. "/v1" for Hub server, "" for local daemon
	apiKey     string
	tokenFunc  func() string // optional: overrides apiKey per request (e.g. cloud JWT auto-refresh)
	teamID     string
	workerID   string // set after RegisterWorker
	httpClient *http.Client
}

// NewClient creates a Hub client from config.
// The API key is resolved from the environment variable named by cfg.APIKeyEnv,
// falling back to cfg.APIKey if set.
func NewClient(cfg HubConfig) *Client {
	apiKey := cfg.APIKey
	if cfg.APIKeyEnv != "" {
		if v := os.Getenv(cfg.APIKeyEnv); v != "" {
			apiKey = v
		}
	}
	// Legacy fallback: C4_HUB_API_KEY (deprecated, for backward compatibility)
	if apiKey == "" {
		if v := os.Getenv("C4_HUB_API_KEY"); v != "" {
			apiKey = v
		}
	}
	teamID := cfg.TeamID
	if v := os.Getenv("C4_HUB_TEAM_ID"); v != "" {
		teamID = v
	}

	return &Client{
		baseURL:   strings.TrimRight(cfg.URL, "/"),
		apiPrefix: strings.TrimRight(cfg.APIPrefix, "/"),
		apiKey:    apiKey,
		teamID:    teamID,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        20,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// IsAvailable returns true when the client has a URL configured.
// API key is optional for local daemon mode.
func (c *Client) IsAvailable() bool {
	return c.baseURL != ""
}

// SetTokenFunc sets a dynamic token function that overrides the static apiKey
// on every Hub request. This allows the cloud session JWT (with auto-refresh)
// to be used as the Hub Bearer token without re-creating the client.
func (c *Client) SetTokenFunc(fn func() string) {
	c.tokenFunc = fn
}

// setHeaders adds Hub authentication headers.
func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	// Dynamic tokenFunc takes precedence over static apiKey (supports auto-refresh).
	apiKey := c.apiKey
	if c.tokenFunc != nil {
		if t := c.tokenFunc(); t != "" {
			apiKey = t
		}
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	if c.teamID != "" {
		req.Header.Set("X-Team-ID", c.teamID)
	}
	if c.workerID != "" {
		req.Header.Set("X-Worker-ID", c.workerID)
	}
}

// url builds the full URL for a path, prepending the API prefix.
// For Hub server: apiPrefix="/v1", path="/jobs" → baseURL+"/jobs"
// For local daemon: apiPrefix="", path="/jobs" → baseURL+"/jobs"
func (c *Client) url(path string) string {
	return c.baseURL + c.apiPrefix + path
}

// isRetryableStatus returns true for HTTP status codes that warrant a retry.
func isRetryableStatus(code int) bool {
	return code == http.StatusTooManyRequests || code >= 500
}

// doWithRetry executes an HTTP request with exponential backoff retry.
// Only retries on network errors and retryable status codes (429, 5xx).
func (c *Client) doWithRetry(req *http.Request) (*http.Response, error) {
	var lastErr error
	for attempt := range maxRetries {
		if attempt > 0 {
			wait := retryBaseWait << (attempt - 1) // 1s, 2s, 4s
			time.Sleep(wait)

			// Rewind body for retry if present
			if req.GetBody != nil {
				body, err := req.GetBody()
				if err != nil {
					return nil, fmt.Errorf("retry body reset: %w", err)
				}
				req.Body = body
			}
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if isRetryableStatus(resp.StatusCode) {
			resp.Body.Close()
			lastErr = fmt.Errorf("%s %s: %d", req.Method, req.URL.Path, resp.StatusCode)
			continue
		}
		return resp, nil
	}
	return nil, lastErr
}

// get performs a GET request and decodes JSON into dest.
func (c *Client) get(path string, dest any) error {
	req, err := http.NewRequest("GET", c.url(path), nil)
	if err != nil {
		return err
	}
	c.setHeaders(req)

	resp, err := c.doWithRetry(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s: %d %s", path, resp.StatusCode, string(body))
	}

	if dest != nil {
		if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
			return fmt.Errorf("decode %s: %w", path, err)
		}
	}
	return nil
}

// getRaw performs a GET request and returns the raw response body.
func (c *Client) getRaw(path string) ([]byte, error) {
	req, err := http.NewRequest("GET", c.url(path), nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.doWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GET %s: %d %s", path, resp.StatusCode, string(body))
	}
	return body, nil
}

// getWithContext performs a GET request with context and decodes JSON into dest.
func (c *Client) getWithContext(ctx context.Context, path string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.url(path), nil)
	if err != nil {
		return err
	}
	c.setHeaders(req)

	resp, err := c.doWithRetry(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s: %d %s", path, resp.StatusCode, string(body))
	}

	if dest != nil {
		if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
			return fmt.Errorf("decode %s: %w", path, err)
		}
	}
	return nil
}

// getRawWithContext performs a GET request with context and returns the raw response body.
func (c *Client) getRawWithContext(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.url(path), nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.doWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GET %s: %d %s", path, resp.StatusCode, string(body))
	}
	return body, nil
}

// post performs a POST request with JSON body and decodes the response.
func (c *Client) post(path string, body, dest any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequest("POST", c.url(path), strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	c.setHeaders(req)
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(string(data))), nil
	}

	resp, err := c.doWithRetry(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s: %d %s", path, resp.StatusCode, string(body))
	}

	if dest != nil {
		if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
			return fmt.Errorf("decode %s: %w", path, err)
		}
	}
	return nil
}


// =========================================================================
// Job API
// =========================================================================

// SubmitJob submits a new job to the Hub queue.
func (c *Client) SubmitJob(req *JobSubmitRequest) (*JobSubmitResponse, error) {
	var resp JobSubmitResponse
	if err := c.post("/jobs/submit", req, &resp); err != nil {
		return nil, fmt.Errorf("submit job: %w", err)
	}
	return &resp, nil
}

// GetJob returns the status of a single job.
func (c *Client) GetJob(jobID string) (*Job, error) {
	var job Job
	if err := c.get("/jobs/"+jobID, &job); err != nil {
		return nil, fmt.Errorf("get job: %w", err)
	}
	return &job, nil
}

// ListJobs returns jobs filtered by status (empty = all).
// Handles both array responses (Hub server) and wrapped responses (PiQ daemon: {"jobs": [...]}).
func (c *Client) ListJobs(status string, limit int) ([]Job, error) {
	path := "/jobs"
	params := []string{}
	if status != "" {
		params = append(params, "status="+status)
	}
	if limit > 0 {
		params = append(params, fmt.Sprintf("limit=%d", limit))
	}
	if len(params) > 0 {
		path += "?" + strings.Join(params, "&")
	}

	raw, err := c.getRaw(path)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}

	// Try wrapped format (PiQ daemon: {"jobs": [...]})
	var wrapped struct {
		Jobs []Job `json:"jobs"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Jobs != nil {
		return wrapped.Jobs, nil
	}

	// Fallback: direct array (Hub server)
	var jobs []Job
	if err := json.Unmarshal(raw, &jobs); err != nil {
		return nil, fmt.Errorf("list jobs: decode: %w", err)
	}
	return jobs, nil
}

// CancelJob cancels a queued or running job.
func (c *Client) CancelJob(jobID string) error {
	if err := c.post("/jobs/"+jobID+"/cancel", nil, nil); err != nil {
		return fmt.Errorf("cancel job: %w", err)
	}
	return nil
}

// CompleteJob reports job completion.
func (c *Client) CompleteJob(jobID, status string, exitCode int) error {
	body := map[string]any{
		"status":    status,
		"exit_code": exitCode,
	}
	if err := c.post("/jobs/"+jobID+"/complete", body, nil); err != nil {
		return fmt.Errorf("complete job: %w", err)
	}
	return nil
}

// ListJobsCtx returns jobs filtered by status (empty = all) using the provided context.
// limit ≤ 0 means no limit (server default).
// Returns a slice of Job pointers for use with context-aware callers (e.g. HubPoller).
func (c *Client) ListJobsCtx(ctx context.Context, status string, limit int) ([]*Job, error) {
	path := "/jobs"
	params := []string{}
	if status != "" {
		params = append(params, "status="+status)
	}
	if limit > 0 {
		params = append(params, fmt.Sprintf("limit=%d", limit))
	}
	if len(params) > 0 {
		path += "?" + strings.Join(params, "&")
	}

	raw, err := c.getRawWithContext(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}

	// Try wrapped format (PiQ daemon: {"jobs": [...]})
	var wrapped struct {
		Jobs []*Job `json:"jobs"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Jobs != nil {
		return wrapped.Jobs, nil
	}

	// Fallback: direct array (Hub server)
	var jobs []*Job
	if err := json.Unmarshal(raw, &jobs); err != nil {
		return nil, fmt.Errorf("list jobs: decode: %w", err)
	}
	return jobs, nil
}

// GetJobLogsCtx returns log lines for a job using the provided context.
func (c *Client) GetJobLogsCtx(ctx context.Context, jobID string, offset, limit int) (*JobLogsResponse, error) {
	path := fmt.Sprintf("/jobs/%s/logs?offset=%d&limit=%d", jobID, offset, limit)
	var resp JobLogsResponse
	if err := c.getWithContext(ctx, path, &resp); err != nil {
		return nil, fmt.Errorf("get job logs: %w", err)
	}
	return &resp, nil
}

// =========================================================================
// Workers API
// =========================================================================

// ListWorkers returns registered workers. If activeOnly is true (default),
// only non-offline workers are returned.
func (c *Client) ListWorkers(activeOnly ...bool) ([]Worker, error) {
	path := "/workers?active_only=true"
	if len(activeOnly) > 0 && !activeOnly[0] {
		path = "/workers?active_only=false"
	}
	var workers []Worker
	if err := c.get(path, &workers); err != nil {
		return nil, fmt.Errorf("list workers: %w", err)
	}
	return workers, nil
}

// PruneWorkers removes offline workers. If dryRun is true, only returns
// the list of workers that would be pruned without actually deleting them.
func (c *Client) PruneWorkers(dryRun bool) (int, error) {
	body := map[string]any{"dry_run": dryRun}
	var resp struct {
		Purged int `json:"purged"`
	}
	if err := c.post("/workers/prune", body, &resp); err != nil {
		return 0, fmt.Errorf("prune workers: %w", err)
	}
	return resp.Purged, nil
}

// HealthCheck returns true if the Hub is reachable and healthy.
func (c *Client) HealthCheck() bool {
	var result map[string]any
	if err := c.get("/health", &result); err != nil {
		return false
	}
	s, _ := result["status"].(string)
	return s == "healthy" || s == "ok"
}

// =========================================================================
// Stats API
// =========================================================================

// GetQueueStats returns queue-level statistics.
func (c *Client) GetQueueStats() (*QueueStats, error) {
	var stats QueueStats
	if err := c.get("/stats/queue", &stats); err != nil {
		return nil, fmt.Errorf("get queue stats: %w", err)
	}
	return &stats, nil
}

// =========================================================================
// Metrics API
// =========================================================================

// LogMetrics logs metrics for a running job.
func (c *Client) LogMetrics(jobID string, step int, metrics map[string]any) error {
	body := map[string]any{
		"step":    step,
		"metrics": metrics,
	}
	if err := c.post("/metrics/"+jobID, body, nil); err != nil {
		return fmt.Errorf("log metrics: %w", err)
	}
	return nil
}

// GetMetrics returns metrics for a job.
func (c *Client) GetMetrics(jobID string, limit int) (*MetricsResponse, error) {
	path := fmt.Sprintf("/metrics/%s?limit=%d", jobID, limit)
	var resp MetricsResponse
	if err := c.get(path, &resp); err != nil {
		return nil, fmt.Errorf("get metrics: %w", err)
	}
	return &resp, nil
}

// GetJobLogs returns log lines for a job.
func (c *Client) GetJobLogs(jobID string, offset, limit int) (*JobLogsResponse, error) {
	path := fmt.Sprintf("/jobs/%s/logs?offset=%d&limit=%d", jobID, offset, limit)
	var resp JobLogsResponse
	if err := c.get(path, &resp); err != nil {
		return nil, fmt.Errorf("get job logs: %w", err)
	}
	return &resp, nil
}

// GetJobSummary returns a comprehensive summary of a job with metrics.
func (c *Client) GetJobSummary(jobID string) (*JobSummaryResponse, error) {
	var resp JobSummaryResponse
	if err := c.get("/jobs/"+jobID+"/summary", &resp); err != nil {
		return nil, fmt.Errorf("get job summary: %w", err)
	}
	return &resp, nil
}

// RetryJob resubmits a failed or cancelled job with the same configuration.
func (c *Client) RetryJob(jobID string) (*JobRetryResponse, error) {
	var resp JobRetryResponse
	if err := c.post("/jobs/"+jobID+"/retry", nil, &resp); err != nil {
		return nil, fmt.Errorf("retry job: %w", err)
	}
	return &resp, nil
}

// GetJobEstimate returns a time estimate for a job based on historical data.
func (c *Client) GetJobEstimate(jobID string) (*JobEstimateResponse, error) {
	var resp JobEstimateResponse
	if err := c.get("/jobs/"+jobID+"/estimate", &resp); err != nil {
		return nil, fmt.Errorf("get job estimate: %w", err)
	}
	return &resp, nil
}

// =========================================================================
// HubClient interface — used by LoopOrchestrator (injected dependency)
// =========================================================================

// HubJobRequest is the minimal job submission payload for the LoopOrchestrator.
type HubJobRequest struct {
	HypothesisID     string
	ExperimentSpecID string
	Command          string
	ProjectID        string
}

// HubJobStatus is the minimal job status for the LoopOrchestrator.
type HubJobStatus struct {
	JobID       string
	Status      string // "pending"|"running"|"completed"|"failed"|"cancelled"
	CompletedAt *time.Time
}

// HubClient is the interface that LoopOrchestrator uses to interact with the Hub.
// The concrete *Client implements this interface; tests use MockHubClient.
type HubClient interface {
	SubmitJob(ctx context.Context, req HubJobRequest) (string, error)
	CancelJob(ctx context.Context, jobID string) error
	GetJobStatus(ctx context.Context, jobID string) (*HubJobStatus, error)
}
