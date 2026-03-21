package hub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// =========================================================================
// MockHubClient — reusable in LoopOrchestrator tests
// =========================================================================

// MockHubClient implements HubClient for testing.
type MockHubClient struct {
	SubmitJobFunc    func(ctx context.Context, req HubJobRequest) (string, error)
	CancelJobFunc    func(ctx context.Context, jobID string) error
	GetJobStatusFunc func(ctx context.Context, jobID string) (*HubJobStatus, error)
}

func (m *MockHubClient) SubmitJob(ctx context.Context, req HubJobRequest) (string, error) {
	return m.SubmitJobFunc(ctx, req)
}

func (m *MockHubClient) CancelJob(ctx context.Context, jobID string) error {
	return m.CancelJobFunc(ctx, jobID)
}

func (m *MockHubClient) GetJobStatus(ctx context.Context, jobID string) (*HubJobStatus, error) {
	return m.GetJobStatusFunc(ctx, jobID)
}

// compile-time assertion: MockHubClient satisfies HubClient.
var _ HubClient = (*MockHubClient)(nil)

// =========================================================================
// HubClient interface compile tests
// =========================================================================

func TestMockHubClient_SubmitJob(t *testing.T) {
	m := &MockHubClient{
		SubmitJobFunc: func(ctx context.Context, req HubJobRequest) (string, error) {
			return "job-mock-1", nil
		},
	}
	id, err := m.SubmitJob(context.Background(), HubJobRequest{Command: "echo hi"})
	if err != nil {
		t.Fatalf("SubmitJob: %v", err)
	}
	if id != "job-mock-1" {
		t.Errorf("id = %q, want job-mock-1", id)
	}
}

func TestMockHubClient_CancelJob(t *testing.T) {
	called := false
	m := &MockHubClient{
		CancelJobFunc: func(ctx context.Context, jobID string) error {
			called = true
			return nil
		},
	}
	if err := m.CancelJob(context.Background(), "job-x"); err != nil {
		t.Fatalf("CancelJob: %v", err)
	}
	if !called {
		t.Error("expected CancelJobFunc to be called")
	}
}

func TestMockHubClient_GetJobStatus(t *testing.T) {
	m := &MockHubClient{
		GetJobStatusFunc: func(ctx context.Context, jobID string) (*HubJobStatus, error) {
			return &HubJobStatus{JobID: jobID, Status: "completed"}, nil
		},
	}
	s, err := m.GetJobStatus(context.Background(), "job-y")
	if err != nil {
		t.Fatalf("GetJobStatus: %v", err)
	}
	if s.Status != "completed" {
		t.Errorf("status = %q, want completed", s.Status)
	}
	if s.JobID != "job-y" {
		t.Errorf("jobID = %q, want job-y", s.JobID)
	}
}

// newTestServer creates a hub mock server and returns a Client connected to it.
func newTestServer(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	client := &Client{
		baseURL:    ts.URL,
		apiPrefix:  "/v1",
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
	// API key is optional (local daemon mode), so URL alone is sufficient.
	c := NewClient(HubConfig{URL: "http://localhost:8000"})
	if !c.IsAvailable() {
		t.Error("expected IsAvailable() = true with URL but no API key (daemon mode)")
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

func TestNewClient_LegacyAPIKeyEnvFallback(t *testing.T) {
	t.Setenv("C4_HUB_API_KEY", "legacy-key")
	c := NewClient(HubConfig{
		URL:       "http://localhost:8000",
		APIKeyEnv: "C5_API_KEY", // primary env not set
	})
	if c.apiKey != "legacy-key" {
		t.Errorf("apiKey = %q, want legacy-key (C4_HUB_API_KEY fallback)", c.apiKey)
	}
}

func TestNewClient_PrimaryEnvOverridesLegacy(t *testing.T) {
	t.Setenv("C5_API_KEY", "primary-key")
	t.Setenv("C4_HUB_API_KEY", "legacy-key")
	c := NewClient(HubConfig{
		URL:       "http://localhost:8000",
		APIKeyEnv: "C5_API_KEY",
	})
	if c.apiKey != "primary-key" {
		t.Errorf("apiKey = %q, want primary-key (C5_API_KEY takes precedence)", c.apiKey)
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

func TestSetTokenFunc_OverridesAPIKey(t *testing.T) {
	c := &Client{apiKey: "static-key"}
	c.SetTokenFunc(func() string { return "dynamic-jwt" })
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	c.setHeaders(req)
	if got := req.Header.Get("X-API-Key"); got != "dynamic-jwt" {
		t.Errorf("X-API-Key = %q, want dynamic-jwt (tokenFunc should override apiKey)", got)
	}
}

func TestSetTokenFunc_EmptyTokenFallsBackToAPIKey(t *testing.T) {
	c := &Client{apiKey: "static-key"}
	c.SetTokenFunc(func() string { return "" })
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	c.setHeaders(req)
	if got := req.Header.Get("X-API-Key"); got != "static-key" {
		t.Errorf("X-API-Key = %q, want static-key (empty tokenFunc should fall back to apiKey)", got)
	}
}

func TestSetTokenFunc_NoAPIKeyNoToken(t *testing.T) {
	c := &Client{} // no apiKey, no tokenFunc
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	c.setHeaders(req)
	if got := req.Header.Get("X-API-Key"); got != "" {
		t.Errorf("X-API-Key should be empty, got %q", got)
	}
}

// =========================================================================
// HealthCheck
// =========================================================================

func TestHealthCheck_Healthy(t *testing.T) {
	mux := http.NewServeMux()
	// Supabase PostgREST health: GET /rest/v1/ returns 200 with schema info.
	mux.HandleFunc("/rest/v1/", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, map[string]string{"swagger": "2.0"})
	})
	client, _ := newTestServer(t, mux)
	if !client.HealthCheck() {
		t.Error("expected HealthCheck() = true")
	}
}

func TestHealthCheck_Unhealthy(t *testing.T) {
	// HealthCheck returns false on any non-2xx from /rest/v1/.
	// Simulate by returning 503.
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	})
	client, _ := newTestServer(t, mux)
	if client.HealthCheck() {
		t.Error("expected HealthCheck() = false for 503")
	}
}

func TestHealthCheck_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	})
	client, _ := newTestServer(t, mux)
	if client.HealthCheck() {
		t.Error("expected HealthCheck() = false on 500")
	}
}

// =========================================================================
// SubmitJob (Supabase PostgREST: POST /rest/v1/jobs)
// =========================================================================

func TestSubmitJob(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.Header.Get("apikey") != "test-key" {
			t.Errorf("missing apikey header, got %q", r.Header.Get("apikey"))
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing Authorization header, got %q", r.Header.Get("Authorization"))
		}

		var req JobSubmitRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Name != "train-resnet" {
			t.Errorf("name = %q, want train-resnet", req.Name)
		}

		// PostgREST returns array of inserted rows
		jsonResponse(w, []Job{{ID: "job-123", Status: "QUEUED"}})
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
}

func TestSubmitJob_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"message":"bad request"}`))
	})
	client, _ := newTestServer(t, mux)

	_, err := client.SubmitJob(&JobSubmitRequest{Name: "x", Workdir: ".", Command: "echo"})
	if err == nil {
		t.Fatal("expected error on 400")
	}
}

// =========================================================================
// GetJob (Supabase PostgREST: GET /rest/v1/jobs?id=eq.{id})
// =========================================================================

func TestGetJob(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Query().Get("id") != "eq.job-456" {
			t.Errorf("id filter = %q, want eq.job-456", r.URL.Query().Get("id"))
		}
		// PostgREST returns array
		jsonResponse(w, []Job{{
			ID:      "job-456",
			Name:    "eval",
			Status:  "RUNNING",
			Workdir: "/work",
			Command: "python eval.py",
		}})
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
// ListJobs (Supabase PostgREST: GET /rest/v1/jobs?order=created_at.desc&...)
// =========================================================================

func TestListJobs(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("status") != "eq.RUNNING" {
			t.Errorf("status filter = %q, want eq.RUNNING", q.Get("status"))
		}
		if q.Get("limit") != "10" {
			t.Errorf("limit = %q, want 10", q.Get("limit"))
		}
		if q.Get("order") != "created_at.desc" {
			t.Errorf("order = %q, want created_at.desc", q.Get("order"))
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
	mux.HandleFunc("/rest/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		// No status or limit filter, but order should still be present
		if q.Get("status") != "" {
			t.Errorf("unexpected status filter: %s", q.Get("status"))
		}
		if q.Get("limit") != "" {
			t.Errorf("unexpected limit: %s", q.Get("limit"))
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
// CancelJob (Supabase PostgREST: PATCH /rest/v1/jobs?id=eq.{id})
// =========================================================================

func TestCancelJob(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
		if r.URL.Query().Get("id") != "eq.job-789" {
			t.Errorf("id filter = %q, want eq.job-789", r.URL.Query().Get("id"))
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["status"] != "CANCELLED" {
			t.Errorf("status = %v, want CANCELLED", body["status"])
		}
		w.WriteHeader(204)
	})
	client, _ := newTestServer(t, mux)

	if err := client.CancelJob("job-789"); err != nil {
		t.Fatalf("CancelJob: %v", err)
	}
}

// =========================================================================
// CompleteJob (Supabase RPC: POST /rest/v1/rpc/complete_job)
// =========================================================================

func TestCompleteJob(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/rpc/complete_job", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["p_job_id"] != "job-100" {
			t.Errorf("p_job_id = %v, want job-100", body["p_job_id"])
		}
		if body["p_status"] != "SUCCEEDED" {
			t.Errorf("p_status = %v, want SUCCEEDED", body["p_status"])
		}
		w.WriteHeader(200)
		w.Write([]byte(`null`))
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
	// Supabase PostgREST: GET /rest/v1/hub_workers?status=neq.offline&...
	mux.HandleFunc("/rest/v1/hub_workers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
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
	// Supabase RPC: POST /rest/v1/rpc/register_worker
	mux.HandleFunc("/rest/v1/rpc/register_worker", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["p_worker_id"] == nil {
			t.Error("expected p_worker_id in body")
		}
		// Return worker row with the worker ID
		jsonResponse(w, map[string]any{"id": "worker-abc", "status": "online"})
	})
	client, _ := newTestServer(t, mux)
	client.workerID = "worker-abc" // pre-set so RPC uses it

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
	// Supabase PostgREST PATCH: /rest/v1/hub_workers?id=eq.w1
	mux.HandleFunc("/rest/v1/hub_workers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
		if !strings.Contains(r.URL.RawQuery, "id=eq.w1") {
			t.Errorf("expected id=eq.w1 in query, got %q", r.URL.RawQuery)
		}
		w.WriteHeader(200)
		w.Write([]byte(`[]`))
	})
	client, _ := newTestServer(t, mux)
	client.workerID = "w1"

	if err := client.Heartbeat("online"); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
}

func TestHeartbeat_NotRegistered(t *testing.T) {
	// Heartbeat should fail immediately if no worker ID is set.
	client := &Client{}
	if err := client.Heartbeat("online"); err == nil {
		t.Error("expected error when worker not registered")
	}
}

// =========================================================================
// ClaimJob
// =========================================================================

func TestClaimJob(t *testing.T) {
	mux := http.NewServeMux()
	// Supabase RPC: POST /rest/v1/rpc/claim_job
	// Returns jsonb {lease_id: "...", job: {...}}
	mux.HandleFunc("/rest/v1/rpc/claim_job", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		jsonResponse(w, map[string]any{
			"lease_id": "lease-1",
			"job":      map[string]any{"id": "j1", "name": "train", "status": "RUNNING"},
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
	// RPC returns null when no job is available.
	mux.HandleFunc("/rest/v1/rpc/claim_job", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`null`))
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
	// Supabase RPC: POST /rest/v1/rpc/renew_lease (returns void → null)
	mux.HandleFunc("/rest/v1/rpc/renew_lease", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["p_lease_id"] != "lease-1" {
			t.Errorf("p_lease_id = %v, want lease-1", body["p_lease_id"])
		}
		w.WriteHeader(200)
		w.Write([]byte(`null`))
	})
	client, _ := newTestServer(t, mux)
	client.workerID = "w1"

	exp, err := client.RenewLease("lease-1")
	if err != nil {
		t.Fatalf("RenewLease: %v", err)
	}
	if exp == "" {
		t.Error("expected non-empty expires_at")
	}
}

func TestRenewLease_Error(t *testing.T) {
	mux := http.NewServeMux()
	// RPC returns 422 when lease not found (PostgreSQL raises exception).
	mux.HandleFunc("/rest/v1/rpc/renew_lease", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		w.Write([]byte(`{"message":"lease not found: lease-expired"}`))
	})
	client, _ := newTestServer(t, mux)
	client.workerID = "w1"

	_, err := client.RenewLease("lease-expired")
	if err == nil {
		t.Error("expected error when lease not found")
	}
}

// =========================================================================
// Error handling
// =========================================================================

func TestHTTP4xx(t *testing.T) {
	mux := http.NewServeMux()
	// GetJob now calls /rest/v1/jobs?id=eq.bad via supabaseGet
	mux.HandleFunc("/rest/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"message":"not found"}`))
	})
	client, _ := newTestServer(t, mux)

	_, err := client.GetJob("bad")
	if err == nil {
		t.Fatal("expected error on 404")
	}
}

func TestHTTP5xx(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/hub_workers", func(w http.ResponseWriter, r *http.Request) {
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

// =========================================================================
// Artifact model tests (T-838-0)
// =========================================================================

func TestHubSubmitRequest_WithArtifacts(t *testing.T) {
	req := JobSubmitRequest{
		Name:    "train-job",
		Workdir: "/workspace",
		Command: "python train.py",
		InputArtifacts: []ArtifactRef{
			{Path: "datasets/cifar10.tar.gz", LocalPath: "/data/cifar10.tar.gz", Required: true},
		},
		OutputArtifacts: []ArtifactRef{
			{Path: "models/resnet50.pt", LocalPath: "/output/resnet50.pt"},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Verify C5-compatible field names are present.
	if _, ok := decoded["input_artifacts"]; !ok {
		t.Error("expected input_artifacts field in JSON")
	}
	if _, ok := decoded["output_artifacts"]; !ok {
		t.Error("expected output_artifacts field in JSON")
	}

	// Verify artifact fields are correctly serialized.
	inputs, _ := decoded["input_artifacts"].([]any)
	if len(inputs) != 1 {
		t.Fatalf("input_artifacts len = %d, want 1", len(inputs))
	}
	input0 := inputs[0].(map[string]any)
	if input0["path"] != "datasets/cifar10.tar.gz" {
		t.Errorf("input path = %v, want datasets/cifar10.tar.gz", input0["path"])
	}
	if input0["local_path"] != "/data/cifar10.tar.gz" {
		t.Errorf("input local_path = %v, want /data/cifar10.tar.gz", input0["local_path"])
	}
	if input0["required"] != true {
		t.Errorf("input required = %v, want true", input0["required"])
	}

	// Verify empty artifacts are omitted (omitempty).
	reqEmpty := JobSubmitRequest{Name: "x", Workdir: ".", Command: "echo"}
	dataEmpty, _ := json.Marshal(reqEmpty)
	var decodedEmpty map[string]any
	json.Unmarshal(dataEmpty, &decodedEmpty)
	if _, ok := decodedEmpty["input_artifacts"]; ok {
		t.Error("input_artifacts should be omitted when empty")
	}
}

func TestLeaseAcquireResponse_ParsesPresignedURLs(t *testing.T) {
	raw := `{
		"job_id": "job-42",
		"lease_id": "lease-99",
		"job": {"id": "job-42", "name": "train", "status": "RUNNING"},
		"input_presigned_urls": [
			{"path": "datasets/cifar10.tar.gz", "local_path": "/data/cifar10.tar.gz", "url": "https://s3.example.com/signed-url", "expires_at": "2026-02-21T00:00:00Z"}
		]
	}`

	var resp ClaimResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal ClaimResponse: %v", err)
	}

	if resp.JobID != "job-42" {
		t.Errorf("JobID = %q, want job-42", resp.JobID)
	}
	if resp.LeaseID != "lease-99" {
		t.Errorf("LeaseID = %q, want lease-99", resp.LeaseID)
	}
	if len(resp.InputPresignedURLs) != 1 {
		t.Fatalf("InputPresignedURLs len = %d, want 1", len(resp.InputPresignedURLs))
	}
	u := resp.InputPresignedURLs[0]
	if u.Path != "datasets/cifar10.tar.gz" {
		t.Errorf("presigned path = %q, want datasets/cifar10.tar.gz", u.Path)
	}
	if u.URL != "https://s3.example.com/signed-url" {
		t.Errorf("presigned url = %q, want https://s3.example.com/signed-url", u.URL)
	}
	if u.ExpiresAt != "2026-02-21T00:00:00Z" {
		t.Errorf("expires_at = %q, want 2026-02-21T00:00:00Z", u.ExpiresAt)
	}

	// Verify backward compatibility: response without presigned URLs still parses fine.
	rawLegacy := `{"job_id": "job-1", "lease_id": "lease-1", "job": {"id": "job-1", "status": "RUNNING"}}`
	var legacyResp ClaimResponse
	if err := json.Unmarshal([]byte(rawLegacy), &legacyResp); err != nil {
		t.Fatalf("unmarshal legacy ClaimResponse: %v", err)
	}
	if legacyResp.InputPresignedURLs != nil {
		t.Error("InputPresignedURLs should be nil for legacy response")
	}
}

// =========================================================================
// ListJobsCtx
// =========================================================================

func TestListJobsCtx(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Query().Get("status") != "eq.completed" {
			t.Errorf("status filter = %q, want eq.completed", r.URL.Query().Get("status"))
		}
		jsonResponse(w, []*Job{
			{ID: "j1", Status: "completed"},
			{ID: "j2", Status: "completed"},
		})
	})
	client, _ := newTestServer(t, mux)

	jobs, err := client.ListJobsCtx(context.Background(), "completed", 0)
	if err != nil {
		t.Fatalf("ListJobsCtx: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("len = %d, want 2", len(jobs))
	}
	if jobs[0].ID != "j1" {
		t.Errorf("jobs[0].ID = %q, want j1", jobs[0].ID)
	}
}

func TestListJobsCtx_NoFilter(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		// order=created_at.desc is always present, but no status or limit
		if r.URL.Query().Get("status") != "" {
			t.Errorf("unexpected status filter: %s", r.URL.Query().Get("status"))
		}
		jsonResponse(w, []*Job{})
	})
	client, _ := newTestServer(t, mux)

	jobs, err := client.ListJobsCtx(context.Background(), "", 0)
	if err != nil {
		t.Fatalf("ListJobsCtx: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("expected empty, got %d", len(jobs))
	}
}

func TestListJobsCtx_ContextCancelled(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, []*Job{})
	})
	client, _ := newTestServer(t, mux)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := client.ListJobsCtx(ctx, "completed", 0)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// =========================================================================
// GetJobLogsCtx
// =========================================================================

func TestGetJobLogsCtx(t *testing.T) {
	mux := http.NewServeMux()
	// New implementation calls /rest/v1/job_logs?job_id=eq.{id}&...
	mux.HandleFunc("/rest/v1/job_logs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Query().Get("job_id") != "eq.job-ctx-1" {
			t.Errorf("job_id filter = %q, want eq.job-ctx-1", r.URL.Query().Get("job_id"))
		}
		// Returns array of log row objects
		jsonResponse(w, []map[string]any{
			{"content": "MPJPE=45.2"},
			{"content": "PA_MPJPE=32.1"},
		})
	})
	client, _ := newTestServer(t, mux)

	resp, err := client.GetJobLogsCtx(context.Background(), "job-ctx-1", 0, 1000)
	if err != nil {
		t.Fatalf("GetJobLogsCtx: %v", err)
	}
	if len(resp.Lines) != 2 {
		t.Errorf("len(Lines) = %d, want 2", len(resp.Lines))
	}
	if resp.Lines[0] != "MPJPE=45.2" {
		t.Errorf("Lines[0] = %q, want MPJPE=45.2", resp.Lines[0])
	}
}

func TestGetJobLogsCtx_ContextCancelled(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/job_logs", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, []map[string]any{})
	})
	client, _ := newTestServer(t, mux)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := client.GetJobLogsCtx(ctx, "job-ctx-2", 0, 100)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
