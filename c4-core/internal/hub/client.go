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

// Client communicates with a PiQ Hub server over REST or Supabase PostgREST.
type Client struct {
	baseURL      string
	apiPrefix    string        // e.g. "/v1" for Hub server, "" for local daemon
	apiKey       string        // legacy Hub API key (also used as Supabase anon key for X-API-Key compat)
	tokenFunc    func() string // optional: overrides apiKey per request (e.g. cloud JWT auto-refresh)
	teamID       string
	workerID     string   // set after RegisterWorker
	capabilities []string // set after RegisterWorker; passed to claim_job RPC
	supabaseURL  string   // Supabase project URL (e.g. https://xyz.supabase.co)
	supabaseKey  string   // Supabase anon key
	httpClient   *http.Client
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

	// Default api_prefix to "/v1" when not explicitly configured.
	apiPrefix := strings.TrimRight(cfg.APIPrefix, "/")
	if apiPrefix == "" {
		apiPrefix = "/v1"
	}

	supabaseKey := cfg.SupabaseKey
	if supabaseKey == "" {
		supabaseKey = apiKey // fall back to legacy API key as Supabase anon key
	}

	return &Client{
		baseURL:     strings.TrimRight(cfg.URL, "/"),
		apiPrefix:   apiPrefix,
		apiKey:      apiKey,
		teamID:      teamID,
		supabaseURL: strings.TrimRight(cfg.SupabaseURL, "/"),
		supabaseKey: supabaseKey,
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

// IsAvailable returns true when the client has a URL configured (either legacy or Supabase).
// API key is optional for local daemon mode.
func (c *Client) IsAvailable() bool {
	return c.baseURL != "" || c.supabaseURL != ""
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

// supabaseRestURL builds a Supabase PostgREST URL for table/RPC operations.
// path should start with "/rest/v1/..." or "/rest/v1/rpc/...".
func (c *Client) supabaseRestURL(path string) string {
	base := c.supabaseURL
	if base == "" {
		base = c.baseURL // fallback for tests
	}
	return base + path
}

// setSupabaseHeaders sets Supabase PostgREST authentication headers.
// PostgREST requires: apikey=anon_key (always), Authorization=Bearer JWT (for RLS).
func (c *Client) setSupabaseHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")

	// apikey header: always the Supabase anon key (or service_role key).
	anonKey := c.supabaseKey
	if anonKey != "" {
		req.Header.Set("apikey", anonKey)
	}

	// Authorization header: prefer JWT from tokenFunc, then supabaseKey, then apiKey.
	bearerToken := anonKey
	if c.tokenFunc != nil {
		if t := c.tokenFunc(); t != "" {
			bearerToken = t
		}
	} else if c.apiKey != "" {
		bearerToken = c.apiKey
	}
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}

	if c.workerID != "" {
		req.Header.Set("X-Worker-ID", c.workerID)
	}
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


// supabaseGet performs a GET to a Supabase PostgREST URL and decodes JSON into dest.
func (c *Client) supabaseGet(path string, dest any) error {
	req, err := http.NewRequest("GET", c.supabaseRestURL(path), nil)
	if err != nil {
		return err
	}
	c.setSupabaseHeaders(req)
	req.Header.Set("Accept", "application/json")

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

// supabasePost performs a POST to a Supabase PostgREST URL with JSON body.
func (c *Client) supabasePost(path string, body, dest any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequest("POST", c.supabaseRestURL(path), strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	c.setSupabaseHeaders(req)
	req.Header.Set("Prefer", "return=representation")
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

// supabasePatch performs a PATCH to a Supabase PostgREST URL.
func (c *Client) supabasePatch(path string, body, dest any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequest("PATCH", c.supabaseRestURL(path), strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	c.setSupabaseHeaders(req)
	req.Header.Set("Prefer", "return=representation")
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(string(data))), nil
	}

	resp, err := c.doWithRetry(req)
	if err != nil {
		return fmt.Errorf("PATCH %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PATCH %s: %d %s", path, resp.StatusCode, string(body))
	}

	if dest != nil {
		if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
			return fmt.Errorf("decode %s: %w", path, err)
		}
	}
	return nil
}

// supabaseRPC calls a Supabase PostgREST RPC function.
func (c *Client) supabaseRPC(funcName string, body, dest any) error {
	return c.supabasePost("/rest/v1/rpc/"+funcName, body, dest)
}

// =========================================================================
// Job API
// =========================================================================

// SubmitJob submits a new job to Supabase (INSERT into hub_jobs table).
func (c *Client) SubmitJob(req *JobSubmitRequest) (*JobSubmitResponse, error) {
	if req.ID == "" {
		req.ID = "j-" + newID()
	}
	var rows []Job
	if err := c.supabasePost("/rest/v1/hub_jobs", req, &rows); err != nil {
		return nil, fmt.Errorf("submit job: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("submit job: empty response")
	}
	return &JobSubmitResponse{
		JobID:  rows[0].ID,
		Status: rows[0].Status,
	}, nil
}

// GetJob returns the status of a single job from Supabase.
func (c *Client) GetJob(jobID string) (*Job, error) {
	var rows []Job
	if err := c.supabaseGet("/rest/v1/hub_jobs?id=eq."+jobID, &rows); err != nil {
		return nil, fmt.Errorf("get job: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("get job: not found: %s", jobID)
	}
	return &rows[0], nil
}

// ListJobs returns jobs filtered by status (empty = all) from Supabase.
func (c *Client) ListJobs(status string, limit int) ([]Job, error) {
	path := "/rest/v1/hub_jobs?order=created_at.desc"
	if status != "" {
		path += "&status=eq." + status
	}
	if limit > 0 {
		path += fmt.Sprintf("&limit=%d", limit)
	}

	var jobs []Job
	if err := c.supabaseGet(path, &jobs); err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	return jobs, nil
}

// CancelJob cancels a queued or running job via Supabase PATCH.
func (c *Client) CancelJob(jobID string) error {
	body := map[string]any{"status": "CANCELLED"}
	if err := c.supabasePatch("/rest/v1/hub_jobs?id=eq."+jobID, body, nil); err != nil {
		return fmt.Errorf("cancel job: %w", err)
	}
	return nil
}

// CompleteJob reports job completion via Supabase RPC.
func (c *Client) CompleteJob(jobID, status string, exitCode int) error {
	body := map[string]any{
		"p_job_id":   jobID,
		"p_status":   status,
		"p_exit_code": exitCode,
	}
	if err := c.supabaseRPC("complete_job", body, nil); err != nil {
		return fmt.Errorf("complete job: %w", err)
	}
	return nil
}

// ListJobsCtx returns jobs filtered by status (empty = all) using the provided context.
// limit ≤ 0 means no limit (server default).
// Returns a slice of Job pointers for use with context-aware callers (e.g. HubPoller).
func (c *Client) ListJobsCtx(ctx context.Context, status string, limit int) ([]*Job, error) {
	path := "/rest/v1/hub_jobs?order=created_at.desc"
	if status != "" {
		path += "&status=eq." + status
	}
	if limit > 0 {
		path += fmt.Sprintf("&limit=%d", limit)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", c.supabaseRestURL(path), nil)
	if err != nil {
		return nil, err
	}
	c.setSupabaseHeaders(req)
	req.Header.Set("Accept", "application/json")

	resp, err := c.doWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list jobs: %d %s", resp.StatusCode, string(body))
	}

	var jobs []*Job
	if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
		return nil, fmt.Errorf("list jobs: decode: %w", err)
	}
	return jobs, nil
}

// GetJobLogsCtx returns log lines for a job via Supabase using the provided context.
func (c *Client) GetJobLogsCtx(ctx context.Context, jobID string, offset, limit int) (*JobLogsResponse, error) {
	path := fmt.Sprintf("/rest/v1/hub_job_logs?job_id=eq.%s&order=id.asc&offset=%d&limit=%d", jobID, offset, limit)
	req, err := http.NewRequestWithContext(ctx, "GET", c.supabaseRestURL(path), nil)
	if err != nil {
		return nil, err
	}
	c.setSupabaseHeaders(req)
	req.Header.Set("Accept", "application/json")

	resp, err := c.doWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("get job logs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get job logs: %d %s", resp.StatusCode, string(body))
	}

	var rows []struct {
		Line string `json:"line"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, fmt.Errorf("get job logs: decode: %w", err)
	}
	result := &JobLogsResponse{
		JobID:  jobID,
		Offset: offset,
		Lines:  make([]string, len(rows)),
	}
	for i, r := range rows {
		result.Lines[i] = r.Line
	}
	result.TotalLines = offset + len(rows)
	result.HasMore = len(rows) == limit
	return result, nil
}

// =========================================================================
// Workers API
// =========================================================================

// ListWorkers returns registered workers from Supabase hub_workers table.
// If activeOnly is true (default), only non-offline workers are returned.
// Each worker's Capabilities field is populated from hub_capabilities.
func (c *Client) ListWorkers(activeOnly ...bool) ([]Worker, error) {
	path := "/rest/v1/hub_workers?status=neq.offline&order=registered_at.desc"
	if len(activeOnly) > 0 && !activeOnly[0] {
		path = "/rest/v1/hub_workers?order=registered_at.desc"
	}
	var workers []Worker
	if err := c.supabaseGet(path, &workers); err != nil {
		return nil, fmt.Errorf("list workers: %w", err)
	}

	// Enrich workers with capabilities from hub_capabilities table.
	type capRow struct {
		WorkerID string `json:"worker_id"`
		Name     string `json:"name"`
	}
	var caps []capRow
	capPath := "/rest/v1/hub_capabilities?select=worker_id,name"
	if err := c.supabaseGet(capPath, &caps); err == nil {
		capMap := make(map[string][]string)
		for _, cap := range caps {
			capMap[cap.WorkerID] = append(capMap[cap.WorkerID], cap.Name)
		}
		for i := range workers {
			if c, ok := capMap[workers[i].ID]; ok {
				workers[i].Capabilities = c
			}
		}
	}

	return workers, nil
}

// BasicWorker is a simplified worker representation for unified views.
type BasicWorker struct {
	ID       string   `json:"id"`
	Hostname string   `json:"hostname"`
	Status   string   `json:"status"`
	Tags     []string `json:"tags"`
	GPUModel string   `json:"gpu_model,omitempty"`
}

// ListWorkersBasic returns a simplified worker list + pending job count.
func (c *Client) ListWorkersBasic() ([]BasicWorker, int, error) {
	workers, err := c.ListWorkers()
	if err != nil {
		return nil, 0, err
	}

	result := make([]BasicWorker, 0, len(workers))
	for _, w := range workers {
		result = append(result, BasicWorker{
			ID:       w.ID,
			Hostname: w.Hostname,
			Status:   w.Status,
			Tags:     w.Capabilities,
			GPUModel: w.GPUModel,
		})
	}

	// Count pending jobs.
	var pendingJobs int
	var jobs []Job
	if err := c.supabaseGet("/rest/v1/hub_jobs?status=eq.QUEUED&select=id", &jobs); err == nil {
		pendingJobs = len(jobs)
	}

	return result, pendingJobs, nil
}

// PruneWorkers removes workers whose last_heartbeat is older than 5 minutes.
// If dryRun is true, counts would-be pruned workers without deleting them.
func (c *Client) PruneWorkers(dryRun bool) (int, error) {
	threshold := time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339)
	path := "/rest/v1/hub_workers?last_heartbeat=lt." + threshold

	if dryRun {
		// Count workers that would be pruned without deleting.
		var workers []Worker
		if err := c.supabaseGet(path, &workers); err != nil {
			return 0, fmt.Errorf("prune workers (dry run): %w", err)
		}
		return len(workers), nil
	}

	// DELETE stale workers.
	req, err := http.NewRequest("DELETE", c.supabaseRestURL(path), nil)
	if err != nil {
		return 0, err
	}
	c.setSupabaseHeaders(req)
	req.Header.Set("Prefer", "return=representation")

	resp, err := c.doWithRetry(req)
	if err != nil {
		return 0, fmt.Errorf("prune workers: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("prune workers: %d %s", resp.StatusCode, string(body))
	}

	var deleted []Worker
	if err := json.NewDecoder(resp.Body).Decode(&deleted); err != nil {
		return 0, nil // DELETE may return empty body; treat as 0 deleted
	}
	return len(deleted), nil
}

// HealthCheck returns true if the Supabase PostgREST endpoint is reachable.
func (c *Client) HealthCheck() bool {
	req, err := http.NewRequest("GET", c.supabaseRestURL("/rest/v1/"), nil)
	if err != nil {
		return false
	}
	c.setSupabaseHeaders(req)
	req.Header.Set("Accept", "application/json")

	resp, err := c.doWithRetry(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 400
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
// Experiment API
// =========================================================================

// CreateExperimentRun starts a new experiment run and returns its run_id.
func (c *Client) CreateExperimentRun(name, capability string) (string, error) {
	body := map[string]any{
		"name":       name,
		"capability": capability,
	}
	var resp struct {
		RunID string `json:"run_id"`
	}
	if err := c.post("/experiment/run", body, &resp); err != nil {
		return "", fmt.Errorf("create experiment run: %w", err)
	}
	return resp.RunID, nil
}

// =========================================================================
// Capability API
// =========================================================================

// InvokeCapabilityRequest is the payload for POST /v1/capabilities/invoke.
type InvokeCapabilityRequest struct {
	Capability string         `json:"capability"`
	Params     map[string]any `json:"params,omitempty"`
	Name       string         `json:"name,omitempty"`
	Priority   int            `json:"priority,omitempty"`
	TimeoutSec int            `json:"timeout_sec,omitempty"`
}

// InvokeCapabilityResponse is the response from POST /v1/capabilities/invoke.
type InvokeCapabilityResponse struct {
	JobID         string `json:"job_id"`
	Status        string `json:"status"`
	QueuePosition int    `json:"queue_position"`
}

// InvokeCapability submits a job via the capability broker (POST /v1/capabilities/invoke).
func (c *Client) InvokeCapability(req *InvokeCapabilityRequest) (*InvokeCapabilityResponse, error) {
	var resp InvokeCapabilityResponse
	if err := c.post("/capabilities/invoke", req, &resp); err != nil {
		return nil, fmt.Errorf("invoke capability: %w", err)
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
