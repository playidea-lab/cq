package hub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Client communicates with a PiQ Hub server over REST.
type Client struct {
	baseURL    string
	apiKey     string
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
	teamID := cfg.TeamID
	if v := os.Getenv("C4_HUB_TEAM_ID"); v != "" {
		teamID = v
	}

	return &Client{
		baseURL: strings.TrimRight(cfg.URL, "/"),
		apiKey:  apiKey,
		teamID:  teamID,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// IsAvailable returns true when the client has a URL and API key configured.
func (c *Client) IsAvailable() bool {
	return c.baseURL != "" && c.apiKey != ""
}

// setHeaders adds Hub authentication headers.
func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.apiKey)
	if c.teamID != "" {
		req.Header.Set("X-Team-ID", c.teamID)
	}
	if c.workerID != "" {
		req.Header.Set("X-Worker-ID", c.workerID)
	}
}

// get performs a GET request and decodes JSON into dest.
func (c *Client) get(path string, dest any) error {
	req, err := http.NewRequest("GET", c.baseURL+path, nil)
	if err != nil {
		return err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
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

// post performs a POST request with JSON body and decodes the response.
func (c *Client) post(path string, body, dest any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequest("POST", c.baseURL+path, strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
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

// patch performs a PATCH request with JSON body and decodes the response.
func (c *Client) patch(path string, body, dest any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequest("PATCH", c.baseURL+path, strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
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

// =========================================================================
// Job API
// =========================================================================

// SubmitJob submits a new job to the Hub queue.
func (c *Client) SubmitJob(req *JobSubmitRequest) (*JobSubmitResponse, error) {
	var resp JobSubmitResponse
	if err := c.post("/v1/jobs/submit", req, &resp); err != nil {
		return nil, fmt.Errorf("submit job: %w", err)
	}
	return &resp, nil
}

// GetJob returns the status of a single job.
func (c *Client) GetJob(jobID string) (*Job, error) {
	var job Job
	if err := c.get("/v1/jobs/"+jobID, &job); err != nil {
		return nil, fmt.Errorf("get job: %w", err)
	}
	return &job, nil
}

// ListJobs returns jobs filtered by status (empty = all).
func (c *Client) ListJobs(status string, limit int) ([]Job, error) {
	path := "/v1/jobs"
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

	var jobs []Job
	if err := c.get(path, &jobs); err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	return jobs, nil
}

// CancelJob cancels a queued or running job.
func (c *Client) CancelJob(jobID string) error {
	if err := c.post("/v1/jobs/"+jobID+"/cancel", nil, nil); err != nil {
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
	if err := c.post("/v1/jobs/"+jobID+"/complete", body, nil); err != nil {
		return fmt.Errorf("complete job: %w", err)
	}
	return nil
}

// =========================================================================
// Workers API
// =========================================================================

// ListWorkers returns all registered workers.
func (c *Client) ListWorkers() ([]Worker, error) {
	var workers []Worker
	if err := c.get("/v1/workers", &workers); err != nil {
		return nil, fmt.Errorf("list workers: %w", err)
	}
	return workers, nil
}

// HealthCheck returns true if the Hub is reachable and healthy.
func (c *Client) HealthCheck() bool {
	var result map[string]any
	if err := c.get("/health", &result); err != nil {
		return false
	}
	s, _ := result["status"].(string)
	return s == "healthy"
}

// =========================================================================
// Stats API
// =========================================================================

// GetQueueStats returns queue-level statistics.
func (c *Client) GetQueueStats() (*QueueStats, error) {
	var stats QueueStats
	if err := c.get("/v1/stats/queue", &stats); err != nil {
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
	if err := c.post("/v1/metrics/"+jobID, body, nil); err != nil {
		return fmt.Errorf("log metrics: %w", err)
	}
	return nil
}

// GetMetrics returns metrics for a job.
func (c *Client) GetMetrics(jobID string, limit int) (*MetricsResponse, error) {
	path := fmt.Sprintf("/v1/metrics/%s?limit=%d", jobID, limit)
	var resp MetricsResponse
	if err := c.get(path, &resp); err != nil {
		return nil, fmt.Errorf("get metrics: %w", err)
	}
	return &resp, nil
}

// GetJobLogs returns log lines for a job.
func (c *Client) GetJobLogs(jobID string, offset, limit int) (*JobLogsResponse, error) {
	path := fmt.Sprintf("/v1/jobs/%s/logs?offset=%d&limit=%d", jobID, offset, limit)
	var resp JobLogsResponse
	if err := c.get(path, &resp); err != nil {
		return nil, fmt.Errorf("get job logs: %w", err)
	}
	return &resp, nil
}

// GetJobSummary returns a comprehensive summary of a job with metrics.
func (c *Client) GetJobSummary(jobID string) (*JobSummaryResponse, error) {
	var resp JobSummaryResponse
	if err := c.get("/v1/jobs/"+jobID+"/summary", &resp); err != nil {
		return nil, fmt.Errorf("get job summary: %w", err)
	}
	return &resp, nil
}

// RetryJob resubmits a failed or cancelled job with the same configuration.
func (c *Client) RetryJob(jobID string) (*JobRetryResponse, error) {
	var resp JobRetryResponse
	if err := c.post("/v1/jobs/"+jobID+"/retry", nil, &resp); err != nil {
		return nil, fmt.Errorf("retry job: %w", err)
	}
	return &resp, nil
}

// GetJobEstimate returns a time estimate for a job based on historical data.
func (c *Client) GetJobEstimate(jobID string) (*JobEstimateResponse, error) {
	var resp JobEstimateResponse
	if err := c.get("/v1/jobs/"+jobID+"/estimate", &resp); err != nil {
		return nil, fmt.Errorf("get job estimate: %w", err)
	}
	return &resp, nil
}
