package hub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestServer creates a hub mock server and returns a Client connected to it.
func newTestServer(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	client := &Client{
		baseURL:    ts.URL,
		apiKey:     "test-key",
		teamID:     "test-team",
		httpClient: http.DefaultClient,
	}
	return client, ts
}

func jsonResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// =========================================================================
// NewClient + IsAvailable
// =========================================================================

func TestNewClient(t *testing.T) {
	c := NewClient(HubConfig{
		URL:    "http://localhost:8000",
		APIKey: "key123",
		TeamID: "team1",
	})
	if c.baseURL != "http://localhost:8000" {
		t.Errorf("baseURL = %q, want http://localhost:8000", c.baseURL)
	}
	if !c.IsAvailable() {
		t.Error("expected IsAvailable() = true")
	}
}

func TestNewClient_TrailingSlash(t *testing.T) {
	c := NewClient(HubConfig{URL: "http://localhost:8000/", APIKey: "k"})
	if c.baseURL != "http://localhost:8000" {
		t.Errorf("trailing slash not trimmed: %q", c.baseURL)
	}
}

func TestIsAvailable_NoKey(t *testing.T) {
	c := NewClient(HubConfig{URL: "http://localhost:8000"})
	if c.IsAvailable() {
		t.Error("expected IsAvailable() = false when no API key")
	}
}

func TestIsAvailable_NoURL(t *testing.T) {
	c := NewClient(HubConfig{APIKey: "k"})
	if c.IsAvailable() {
		t.Error("expected IsAvailable() = false when no URL")
	}
}

func TestNewClient_APIKeyEnv(t *testing.T) {
	t.Setenv("TEST_HUB_KEY", "env-key-value")
	c := NewClient(HubConfig{
		URL:       "http://localhost:8000",
		APIKeyEnv: "TEST_HUB_KEY",
	})
	if c.apiKey != "env-key-value" {
		t.Errorf("apiKey = %q, want env-key-value", c.apiKey)
	}
}

// =========================================================================
// setHeaders
// =========================================================================

func TestSetHeaders(t *testing.T) {
	c := &Client{apiKey: "k", teamID: "t", workerID: "w"}
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	c.setHeaders(req)

	if got := req.Header.Get("X-API-Key"); got != "k" {
		t.Errorf("X-API-Key = %q, want k", got)
	}
	if got := req.Header.Get("X-Team-ID"); got != "t" {
		t.Errorf("X-Team-ID = %q, want t", got)
	}
	if got := req.Header.Get("X-Worker-ID"); got != "w" {
		t.Errorf("X-Worker-ID = %q, want w", got)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
}

func TestSetHeaders_NoWorkerID(t *testing.T) {
	c := &Client{apiKey: "k"}
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	c.setHeaders(req)
	if got := req.Header.Get("X-Worker-ID"); got != "" {
		t.Errorf("X-Worker-ID should be empty, got %q", got)
	}
}

// =========================================================================
// HealthCheck
// =========================================================================

func TestHealthCheck_Healthy(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, map[string]string{"status": "healthy"})
	})
	client, _ := newTestServer(t, mux)
	if !client.HealthCheck() {
		t.Error("expected HealthCheck() = true")
	}
}

func TestHealthCheck_Unhealthy(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, map[string]string{"status": "degraded"})
	})
	client, _ := newTestServer(t, mux)
	if client.HealthCheck() {
		t.Error("expected HealthCheck() = false for non-healthy")
	}
}

func TestHealthCheck_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	})
	client, _ := newTestServer(t, mux)
	if client.HealthCheck() {
		t.Error("expected HealthCheck() = false on 500")
	}
}

// =========================================================================
// SubmitJob
// =========================================================================

func TestSubmitJob(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/jobs/submit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("missing X-API-Key header")
		}

		var req JobSubmitRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Name != "train-resnet" {
			t.Errorf("name = %q, want train-resnet", req.Name)
		}

		jsonResponse(w, JobSubmitResponse{
			JobID:         "job-123",
			Status:        "QUEUED",
			QueuePosition: 3,
		})
	})
	client, _ := newTestServer(t, mux)

	resp, err := client.SubmitJob(&JobSubmitRequest{
		Name:        "train-resnet",
		Workdir:     "/workspace",
		Command:     "python train.py",
		RequiresGPU: true,
	})
	if err != nil {
		t.Fatalf("SubmitJob: %v", err)
	}
	if resp.JobID != "job-123" {
		t.Errorf("JobID = %q, want job-123", resp.JobID)
	}
	if resp.QueuePosition != 3 {
		t.Errorf("QueuePosition = %d, want 3", resp.QueuePosition)
	}
}

func TestSubmitJob_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/jobs/submit", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"detail":"bad request"}`))
	})
	client, _ := newTestServer(t, mux)

	_, err := client.SubmitJob(&JobSubmitRequest{Name: "x", Workdir: ".", Command: "echo"})
	if err == nil {
		t.Fatal("expected error on 400")
	}
}

// =========================================================================
// GetJob
// =========================================================================

func TestGetJob(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/jobs/job-456", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		jsonResponse(w, Job{
			ID:      "job-456",
			Name:    "eval",
			Status:  "RUNNING",
			Workdir: "/work",
			Command: "python eval.py",
		})
	})
	client, _ := newTestServer(t, mux)

	job, err := client.GetJob("job-456")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if job.Status != "RUNNING" {
		t.Errorf("Status = %q, want RUNNING", job.Status)
	}
}

// =========================================================================
// ListJobs
// =========================================================================

func TestListJobs(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("status")
		limit := r.URL.Query().Get("limit")
		if status != "RUNNING" {
			t.Errorf("status = %q, want RUNNING", status)
		}
		if limit != "10" {
			t.Errorf("limit = %q, want 10", limit)
		}
		jsonResponse(w, []Job{
			{ID: "j1", Status: "RUNNING"},
			{ID: "j2", Status: "RUNNING"},
		})
	})
	client, _ := newTestServer(t, mux)

	jobs, err := client.ListJobs("RUNNING", 10)
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("len = %d, want 2", len(jobs))
	}
}

func TestListJobs_NoFilter(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		jsonResponse(w, []Job{})
	})
	client, _ := newTestServer(t, mux)

	_, err := client.ListJobs("", 0)
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
}

// =========================================================================
// CancelJob
// =========================================================================

func TestCancelJob(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/jobs/job-789/cancel", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		jsonResponse(w, map[string]bool{"cancelled": true})
	})
	client, _ := newTestServer(t, mux)

	if err := client.CancelJob("job-789"); err != nil {
		t.Fatalf("CancelJob: %v", err)
	}
}

// =========================================================================
// CompleteJob
// =========================================================================

func TestCompleteJob(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/jobs/job-100/complete", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["status"] != "SUCCEEDED" {
			t.Errorf("status = %v, want SUCCEEDED", body["status"])
		}
		jsonResponse(w, map[string]bool{"acknowledged": true})
	})
	client, _ := newTestServer(t, mux)

	if err := client.CompleteJob("job-100", "SUCCEEDED", 0); err != nil {
		t.Fatalf("CompleteJob: %v", err)
	}
}

// =========================================================================
// ListWorkers
// =========================================================================

func TestListWorkers(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/workers", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, []Worker{
			{ID: "w1", Status: "online", GPUCount: 2, GPUModel: "RTX 4090"},
		})
	})
	client, _ := newTestServer(t, mux)

	workers, err := client.ListWorkers()
	if err != nil {
		t.Fatalf("ListWorkers: %v", err)
	}
	if len(workers) != 1 {
		t.Fatalf("len = %d, want 1", len(workers))
	}
	if workers[0].GPUModel != "RTX 4090" {
		t.Errorf("GPUModel = %q", workers[0].GPUModel)
	}
}

// =========================================================================
// GetQueueStats
// =========================================================================

func TestGetQueueStats(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/stats/queue", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, QueueStats{
			Queued: 5, Running: 2, Succeeded: 10, Failed: 1, Cancelled: 0,
		})
	})
	client, _ := newTestServer(t, mux)

	stats, err := client.GetQueueStats()
	if err != nil {
		t.Fatalf("GetQueueStats: %v", err)
	}
	if stats.Queued != 5 {
		t.Errorf("Queued = %d, want 5", stats.Queued)
	}
	if stats.Running != 2 {
		t.Errorf("Running = %d, want 2", stats.Running)
	}
}

// =========================================================================
// Metrics
// =========================================================================

func TestLogMetrics(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/metrics/job-200", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["step"] != float64(42) {
			t.Errorf("step = %v, want 42", body["step"])
		}
		jsonResponse(w, map[string]bool{"acknowledged": true})
	})
	client, _ := newTestServer(t, mux)

	err := client.LogMetrics("job-200", 42, map[string]any{"loss": 0.5})
	if err != nil {
		t.Fatalf("LogMetrics: %v", err)
	}
}

func TestGetMetrics(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/metrics/job-300", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		jsonResponse(w, MetricsResponse{
			JobID:      "job-300",
			TotalSteps: 100,
			Metrics: []MetricEntry{
				{Step: 0, Metrics: map[string]any{"loss": 1.0}},
				{Step: 50, Metrics: map[string]any{"loss": 0.5}},
			},
		})
	})
	client, _ := newTestServer(t, mux)

	resp, err := client.GetMetrics("job-300", 100)
	if err != nil {
		t.Fatalf("GetMetrics: %v", err)
	}
	if resp.TotalSteps != 100 {
		t.Errorf("TotalSteps = %d, want 100", resp.TotalSteps)
	}
	if len(resp.Metrics) != 2 {
		t.Errorf("len(Metrics) = %d, want 2", len(resp.Metrics))
	}
}

// =========================================================================
// Worker Registration + Heartbeat
// =========================================================================

func TestRegisterWorker(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/workers/register", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, WorkerRegisterResponse{WorkerID: "worker-abc"})
	})
	client, _ := newTestServer(t, mux)

	wid, err := client.RegisterWorker(map[string]any{"gpu_count": 1})
	if err != nil {
		t.Fatalf("RegisterWorker: %v", err)
	}
	if wid != "worker-abc" {
		t.Errorf("workerID = %q, want worker-abc", wid)
	}
	if client.workerID != "worker-abc" {
		t.Error("workerID not stored on client")
	}
}

func TestHeartbeat(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/workers/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Worker-ID") != "w1" {
			t.Errorf("missing X-Worker-ID header")
		}
		jsonResponse(w, HeartbeatResponse{Acknowledged: true})
	})
	client, _ := newTestServer(t, mux)
	client.workerID = "w1"

	if err := client.Heartbeat("online"); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
}

func TestHeartbeat_NotAcknowledged(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/workers/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, HeartbeatResponse{Acknowledged: false})
	})
	client, _ := newTestServer(t, mux)
	client.workerID = "w1"

	if err := client.Heartbeat("online"); err == nil {
		t.Error("expected error when not acknowledged")
	}
}

// =========================================================================
// ClaimJob
// =========================================================================

func TestClaimJob(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/leases/acquire", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, ClaimResponse{
			JobID:   "j1",
			LeaseID: "lease-1",
			Job:     Job{ID: "j1", Name: "train", Status: "RUNNING"},
		})
	})
	client, _ := newTestServer(t, mux)
	client.workerID = "w1"

	job, leaseID, err := client.ClaimJob(24.0)
	if err != nil {
		t.Fatalf("ClaimJob: %v", err)
	}
	if job == nil {
		t.Fatal("expected non-nil job")
	}
	if job.ID != "j1" {
		t.Errorf("job.ID = %q, want j1", job.ID)
	}
	if leaseID != "lease-1" {
		t.Errorf("leaseID = %q, want lease-1", leaseID)
	}
}

func TestClaimJob_NoJob(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/leases/acquire", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, ClaimResponse{})
	})
	client, _ := newTestServer(t, mux)
	client.workerID = "w1"

	job, _, err := client.ClaimJob(24.0)
	if err != nil {
		t.Fatalf("ClaimJob: %v", err)
	}
	if job != nil {
		t.Error("expected nil job when queue is empty")
	}
}

// =========================================================================
// RenewLease
// =========================================================================

func TestRenewLease(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/leases/renew", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, RenewLeaseResponse{
			Renewed:      true,
			NewExpiresAt: "2026-01-01T00:00:00Z",
		})
	})
	client, _ := newTestServer(t, mux)
	client.workerID = "w1"

	exp, err := client.RenewLease("lease-1")
	if err != nil {
		t.Fatalf("RenewLease: %v", err)
	}
	if exp != "2026-01-01T00:00:00Z" {
		t.Errorf("expires = %q", exp)
	}
}

func TestRenewLease_NotRenewed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/leases/renew", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, RenewLeaseResponse{Renewed: false})
	})
	client, _ := newTestServer(t, mux)
	client.workerID = "w1"

	_, err := client.RenewLease("lease-expired")
	if err == nil {
		t.Error("expected error when lease not renewed")
	}
}

// =========================================================================
// Error handling
// =========================================================================

func TestHTTP4xx(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/jobs/bad", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"detail":"not found"}`))
	})
	client, _ := newTestServer(t, mux)

	_, err := client.GetJob("bad")
	if err == nil {
		t.Fatal("expected error on 404")
	}
}

func TestHTTP5xx(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/workers", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("internal error"))
	})
	client, _ := newTestServer(t, mux)

	_, err := client.ListWorkers()
	if err == nil {
		t.Fatal("expected error on 500")
	}
}

// =========================================================================
// GetJobLogs
// =========================================================================

func TestGetJobLogs(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/jobs/job-500/logs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		offset := r.URL.Query().Get("offset")
		limit := r.URL.Query().Get("limit")
		if offset != "0" {
			t.Errorf("offset = %q, want 0", offset)
		}
		if limit != "200" {
			t.Errorf("limit = %q, want 200", limit)
		}
		jsonResponse(w, JobLogsResponse{
			JobID:      "job-500",
			Lines:      []string{"epoch 1/10 loss=0.9", "epoch 2/10 loss=0.7", "epoch 3/10 loss=0.5"},
			TotalLines: 100,
			Offset:     0,
			HasMore:    true,
		})
	})
	client, _ := newTestServer(t, mux)

	resp, err := client.GetJobLogs("job-500", 0, 200)
	if err != nil {
		t.Fatalf("GetJobLogs: %v", err)
	}
	if len(resp.Lines) != 3 {
		t.Errorf("len(Lines) = %d, want 3", len(resp.Lines))
	}
	if resp.TotalLines != 100 {
		t.Errorf("TotalLines = %d, want 100", resp.TotalLines)
	}
	if !resp.HasMore {
		t.Error("expected HasMore = true")
	}
}

func TestGetJobLogs_Empty(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/jobs/job-501/logs", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, JobLogsResponse{
			JobID:      "job-501",
			Lines:      []string{},
			TotalLines: 0,
			HasMore:    false,
		})
	})
	client, _ := newTestServer(t, mux)

	resp, err := client.GetJobLogs("job-501", 0, 100)
	if err != nil {
		t.Fatalf("GetJobLogs: %v", err)
	}
	if len(resp.Lines) != 0 {
		t.Errorf("expected empty lines, got %d", len(resp.Lines))
	}
	if resp.HasMore {
		t.Error("expected HasMore = false")
	}
}

// =========================================================================
// GetJobSummary
// =========================================================================

func TestGetJobSummary(t *testing.T) {
	mux := http.NewServeMux()
	dur := 540.5
	exitCode := 0
	mux.HandleFunc("/v1/jobs/job-600/summary", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		jsonResponse(w, JobSummaryResponse{
			JobID:       "job-600",
			Name:        "train-resnet",
			Status:      "SUCCEEDED",
			DurationSec: &dur,
			ExitCode:    &exitCode,
			Metrics:     map[string]any{"loss": 0.05, "accuracy": 0.97},
			LogTail:     []string{"Training complete", "Best accuracy: 0.97"},
		})
	})
	client, _ := newTestServer(t, mux)

	resp, err := client.GetJobSummary("job-600")
	if err != nil {
		t.Fatalf("GetJobSummary: %v", err)
	}
	if resp.Status != "SUCCEEDED" {
		t.Errorf("Status = %q, want SUCCEEDED", resp.Status)
	}
	if resp.DurationSec == nil || *resp.DurationSec != 540.5 {
		t.Errorf("DurationSec = %v, want 540.5", resp.DurationSec)
	}
	if len(resp.Metrics) != 2 {
		t.Errorf("len(Metrics) = %d, want 2", len(resp.Metrics))
	}
}

func TestGetJobSummary_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/jobs/bad-id/summary", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"detail":"not found"}`))
	})
	client, _ := newTestServer(t, mux)

	_, err := client.GetJobSummary("bad-id")
	if err == nil {
		t.Fatal("expected error on 404")
	}
}

// =========================================================================
// RetryJob
// =========================================================================

func TestRetryJob(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/jobs/job-700/retry", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		jsonResponse(w, JobRetryResponse{
			NewJobID:      "job-701",
			Status:        "QUEUED",
			OriginalJobID: "job-700",
		})
	})
	client, _ := newTestServer(t, mux)

	resp, err := client.RetryJob("job-700")
	if err != nil {
		t.Fatalf("RetryJob: %v", err)
	}
	if resp.NewJobID != "job-701" {
		t.Errorf("NewJobID = %q, want job-701", resp.NewJobID)
	}
	if resp.OriginalJobID != "job-700" {
		t.Errorf("OriginalJobID = %q, want job-700", resp.OriginalJobID)
	}
}

func TestRetryJob_NotRetryable(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/jobs/job-running/retry", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"detail":"job is still running"}`))
	})
	client, _ := newTestServer(t, mux)

	_, err := client.RetryJob("job-running")
	if err == nil {
		t.Fatal("expected error on 400")
	}
}

// =========================================================================
// GetJobEstimate
// =========================================================================

func TestGetJobEstimate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/jobs/job-800/estimate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		jsonResponse(w, JobEstimateResponse{
			EstimatedDurationSec: 3600,
			QueueWaitSec:         120,
			EstimatedStartTime:   "2026-02-13T10:00:00Z",
			EstimatedEndTime:     "2026-02-13T11:00:00Z",
			Confidence:           "high",
			Method:               "historical",
		})
	})
	client, _ := newTestServer(t, mux)

	resp, err := client.GetJobEstimate("job-800")
	if err != nil {
		t.Fatalf("GetJobEstimate: %v", err)
	}
	if resp.EstimatedDurationSec != 3600 {
		t.Errorf("EstimatedDurationSec = %f, want 3600", resp.EstimatedDurationSec)
	}
	if resp.Confidence != "high" {
		t.Errorf("Confidence = %q, want high", resp.Confidence)
	}
	if resp.Method != "historical" {
		t.Errorf("Method = %q, want historical", resp.Method)
	}
}

func TestGetJobEstimate_NoHistory(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/jobs/job-new/estimate", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, JobEstimateResponse{
			EstimatedDurationSec: 1800,
			Confidence:           "low",
			Method:               "default",
		})
	})
	client, _ := newTestServer(t, mux)

	resp, err := client.GetJobEstimate("job-new")
	if err != nil {
		t.Fatalf("GetJobEstimate: %v", err)
	}
	if resp.Confidence != "low" {
		t.Errorf("Confidence = %q, want low", resp.Confidence)
	}
	if resp.Method != "default" {
		t.Errorf("Method = %q, want default", resp.Method)
	}
}
