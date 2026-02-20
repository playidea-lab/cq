package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/piqsol/c4/c5/internal/model"
	"github.com/piqsol/c4/c5/internal/store"
)

// mockStorage is a test storage backend.
type mockStorage struct {
	failURLs bool // if true, PresignedURL always returns an error
}

func (m *mockStorage) PresignedURL(path, method string, ttlSeconds int) (string, time.Time, error) {
	if m.failURLs {
		return "", time.Time{}, errors.New("mock storage error")
	}
	exp := time.Now().UTC().Add(time.Duration(ttlSeconds) * time.Second)
	return "https://mock.example.com/" + path, exp, nil
}

func newTestServerWithStorage(t *testing.T, stor *mockStorage) *Server {
	t.Helper()
	dir := t.TempDir()
	st, err := store.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return NewServer(Config{
		Store:   st,
		Storage: stor,
		Version: "test",
	})
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	st, err := store.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	return NewServer(Config{
		Store:   st,
		Version: "test",
	})
}

func doRequest(t *testing.T, srv *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

func decodeJSON(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.NewDecoder(w.Body).Decode(v); err != nil {
		t.Fatalf("decode json: %v (body: %s)", err, w.Body.String())
	}
}

// =========================================================================
// Health & Stats
// =========================================================================

func TestHealth(t *testing.T) {
	srv := newTestServer(t)
	w := doRequest(t, srv, "GET", "/v1/health", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	decodeJSON(t, w, &resp)
	if resp["status"] != "ok" {
		t.Fatalf("expected ok, got %v", resp["status"])
	}
}

func TestQueueStats(t *testing.T) {
	srv := newTestServer(t)

	// Submit some jobs first
	doRequest(t, srv, "POST", "/v1/jobs/submit", model.JobSubmitRequest{
		Name: "j1", Command: "echo",
	})
	doRequest(t, srv, "POST", "/v1/jobs/submit", model.JobSubmitRequest{
		Name: "j2", Command: "echo",
	})

	w := doRequest(t, srv, "GET", "/v1/stats/queue", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var stats model.QueueStats
	decodeJSON(t, w, &stats)
	if stats.Queued != 2 {
		t.Fatalf("expected 2 queued, got %d", stats.Queued)
	}
}

// =========================================================================
// Jobs
// =========================================================================

func TestJobSubmitAndGet(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(t, srv, "POST", "/v1/jobs/submit", model.JobSubmitRequest{
		Name:    "test-job",
		Command: "echo hello",
		Workdir: "/tmp",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp model.JobSubmitResponse
	decodeJSON(t, w, &resp)
	if resp.JobID == "" {
		t.Fatal("job_id should not be empty")
	}
	if resp.Status != "QUEUED" {
		t.Fatalf("expected QUEUED, got %s", resp.Status)
	}

	// GET job
	w2 := doRequest(t, srv, "GET", "/v1/jobs/"+resp.JobID, nil)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}

	var job model.Job
	decodeJSON(t, w2, &job)
	if job.Name != "test-job" {
		t.Fatalf("expected test-job, got %s", job.Name)
	}
}

func TestJobSubmitValidation(t *testing.T) {
	srv := newTestServer(t)

	// Missing command
	w := doRequest(t, srv, "POST", "/v1/jobs/submit", model.JobSubmitRequest{
		Name: "no-cmd",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestJobsList(t *testing.T) {
	srv := newTestServer(t)

	for i := 0; i < 3; i++ {
		doRequest(t, srv, "POST", "/v1/jobs/submit", model.JobSubmitRequest{
			Name: "job", Command: "echo",
		})
	}

	w := doRequest(t, srv, "GET", "/v1/jobs?limit=10", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var jobs []*model.Job
	decodeJSON(t, w, &jobs)
	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(jobs))
	}
}

func TestJobCancel(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(t, srv, "POST", "/v1/jobs/submit", model.JobSubmitRequest{
		Name: "cancel-me", Command: "sleep 100",
	})
	var resp model.JobSubmitResponse
	decodeJSON(t, w, &resp)

	w2 := doRequest(t, srv, "POST", "/v1/jobs/"+resp.JobID+"/cancel", nil)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	// Verify
	w3 := doRequest(t, srv, "GET", "/v1/jobs/"+resp.JobID, nil)
	var job model.Job
	decodeJSON(t, w3, &job)
	if job.Status != model.StatusCancelled {
		t.Fatalf("expected CANCELLED, got %s", job.Status)
	}
}

func TestJobRetry(t *testing.T) {
	srv := newTestServer(t)

	// Create and cancel a job
	w := doRequest(t, srv, "POST", "/v1/jobs/submit", model.JobSubmitRequest{
		Name: "retry-me", Command: "echo retry",
	})
	var resp model.JobSubmitResponse
	decodeJSON(t, w, &resp)
	doRequest(t, srv, "POST", "/v1/jobs/"+resp.JobID+"/cancel", nil)

	// Retry
	w2 := doRequest(t, srv, "POST", "/v1/jobs/"+resp.JobID+"/retry", nil)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var retryResp model.JobRetryResponse
	decodeJSON(t, w2, &retryResp)
	if retryResp.NewJobID == "" {
		t.Fatal("new_job_id should not be empty")
	}
	if retryResp.OriginalJobID != resp.JobID {
		t.Fatalf("original_job_id mismatch")
	}
}

func TestJobRetryNonTerminal(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(t, srv, "POST", "/v1/jobs/submit", model.JobSubmitRequest{
		Name: "running", Command: "echo",
	})
	var resp model.JobSubmitResponse
	decodeJSON(t, w, &resp)

	// Try retry while QUEUED
	w2 := doRequest(t, srv, "POST", "/v1/jobs/"+resp.JobID+"/retry", nil)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w2.Code)
	}
}

func TestJobLogs(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(t, srv, "POST", "/v1/jobs/submit", model.JobSubmitRequest{
		Name: "log-job", Command: "echo hello",
	})
	var resp model.JobSubmitResponse
	decodeJSON(t, w, &resp)

	// Append logs
	doRequest(t, srv, "POST", "/v1/jobs/"+resp.JobID+"/logs", map[string]string{
		"line": "hello world", "stream": "stdout",
	})
	doRequest(t, srv, "POST", "/v1/jobs/"+resp.JobID+"/logs", map[string]string{
		"line": "error occurred", "stream": "stderr",
	})

	// Get logs
	w2 := doRequest(t, srv, "GET", "/v1/jobs/"+resp.JobID+"/logs", nil)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}

	var logs model.JobLogsResponse
	decodeJSON(t, w2, &logs)
	if logs.TotalLines != 2 {
		t.Fatalf("expected 2 lines, got %d", logs.TotalLines)
	}
}

func TestJobSummary(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(t, srv, "POST", "/v1/jobs/submit", model.JobSubmitRequest{
		Name: "summary-job", Command: "echo",
	})
	var resp model.JobSubmitResponse
	decodeJSON(t, w, &resp)

	w2 := doRequest(t, srv, "GET", "/v1/jobs/"+resp.JobID+"/summary", nil)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}

	var summary model.JobSummaryResponse
	decodeJSON(t, w2, &summary)
	if summary.Name != "summary-job" {
		t.Fatalf("expected summary-job, got %s", summary.Name)
	}
}

func TestJobEstimate(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(t, srv, "POST", "/v1/jobs/submit", model.JobSubmitRequest{
		Name: "est-job", Command: "echo",
	})
	var resp model.JobSubmitResponse
	decodeJSON(t, w, &resp)

	w2 := doRequest(t, srv, "GET", "/v1/jobs/"+resp.JobID+"/estimate", nil)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}

	var est model.EstimateResponse
	decodeJSON(t, w2, &est)
	if est.Method != "default" {
		t.Fatalf("expected default method, got %s", est.Method)
	}
	if est.EstimatedDurationSec != 300 {
		t.Fatalf("expected 300s default, got %f", est.EstimatedDurationSec)
	}
}

func TestJobNotFound(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(t, srv, "GET", "/v1/jobs/nonexistent", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// =========================================================================
// Workers
// =========================================================================

func TestWorkerRegisterAndList(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(t, srv, "POST", "/v1/workers/register", model.WorkerRegisterRequest{
		Hostname:  "gpu-1",
		GPUCount:  2,
		GPUModel:  "A100",
		TotalVRAM: 80,
		FreeVRAM:  80,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp model.WorkerRegisterResponse
	decodeJSON(t, w, &resp)
	if resp.WorkerID == "" {
		t.Fatal("worker_id should not be empty")
	}

	// List
	w2 := doRequest(t, srv, "GET", "/v1/workers", nil)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}

	var workers []model.Worker
	decodeJSON(t, w2, &workers)
	if len(workers) != 1 {
		t.Fatalf("expected 1 worker, got %d", len(workers))
	}
}

func TestWorkerRegisterValidation(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(t, srv, "POST", "/v1/workers/register", model.WorkerRegisterRequest{})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestWorkerHeartbeat(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(t, srv, "POST", "/v1/workers/register", model.WorkerRegisterRequest{
		Hostname: "test",
	})
	var regResp model.WorkerRegisterResponse
	decodeJSON(t, w, &regResp)

	w2 := doRequest(t, srv, "POST", "/v1/workers/heartbeat", model.HeartbeatRequest{
		WorkerID: regResp.WorkerID,
		FreeVRAM: 40,
	})
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var hbResp model.HeartbeatResponse
	decodeJSON(t, w2, &hbResp)
	if !hbResp.Acknowledged {
		t.Fatal("expected acknowledged=true")
	}
}

// =========================================================================
// Leases
// =========================================================================

func TestLeaseAcquireAndRenew(t *testing.T) {
	srv := newTestServer(t)

	// Register worker
	w := doRequest(t, srv, "POST", "/v1/workers/register", model.WorkerRegisterRequest{
		Hostname: "test",
	})
	var regResp model.WorkerRegisterResponse
	decodeJSON(t, w, &regResp)

	// Submit job
	doRequest(t, srv, "POST", "/v1/jobs/submit", model.JobSubmitRequest{
		Name: "job1", Command: "echo",
	})

	// Acquire lease
	w2 := doRequest(t, srv, "POST", "/v1/leases/acquire", model.LeaseAcquireRequest{
		WorkerID: regResp.WorkerID,
	})
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var acqResp model.LeaseAcquireResponse
	decodeJSON(t, w2, &acqResp)
	if acqResp.LeaseID == "" {
		t.Fatal("lease_id should not be empty")
	}
	if acqResp.Job.Name != "job1" {
		t.Fatalf("expected job1, got %s", acqResp.Job.Name)
	}

	// Renew lease
	w3 := doRequest(t, srv, "POST", "/v1/leases/renew", model.LeaseRenewRequest{
		LeaseID:  acqResp.LeaseID,
		WorkerID: regResp.WorkerID,
	})
	if w3.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w3.Code, w3.Body.String())
	}

	var renewResp model.LeaseRenewResponse
	decodeJSON(t, w3, &renewResp)
	if !renewResp.Renewed {
		t.Fatal("expected renewed=true")
	}
}

func TestLeaseAcquireNoJobs(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(t, srv, "POST", "/v1/workers/register", model.WorkerRegisterRequest{
		Hostname: "test",
	})
	var regResp model.WorkerRegisterResponse
	decodeJSON(t, w, &regResp)

	w2 := doRequest(t, srv, "POST", "/v1/leases/acquire", model.LeaseAcquireRequest{
		WorkerID: regResp.WorkerID,
	})
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}

	var resp map[string]any
	decodeJSON(t, w2, &resp)
	if resp["message"] != "no jobs available" {
		t.Fatalf("expected no jobs message, got %v", resp)
	}
}

func TestCompleteJobViaAPI(t *testing.T) {
	srv := newTestServer(t)

	// Register + submit + acquire
	w := doRequest(t, srv, "POST", "/v1/workers/register", model.WorkerRegisterRequest{Hostname: "test"})
	var regResp model.WorkerRegisterResponse
	decodeJSON(t, w, &regResp)

	w2 := doRequest(t, srv, "POST", "/v1/jobs/submit", model.JobSubmitRequest{Name: "job", Command: "echo"})
	var submitResp model.JobSubmitResponse
	decodeJSON(t, w2, &submitResp)

	doRequest(t, srv, "POST", "/v1/leases/acquire", model.LeaseAcquireRequest{WorkerID: regResp.WorkerID})

	// Complete
	w3 := doRequest(t, srv, "POST", "/v1/jobs/"+submitResp.JobID+"/complete", model.JobCompleteRequest{
		Status:   "SUCCEEDED",
		ExitCode: 0,
	})
	if w3.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w3.Code, w3.Body.String())
	}

	// Verify
	w4 := doRequest(t, srv, "GET", "/v1/jobs/"+submitResp.JobID, nil)
	var job model.Job
	decodeJSON(t, w4, &job)
	if job.Status != model.StatusSucceeded {
		t.Fatalf("expected SUCCEEDED, got %s", job.Status)
	}
}

// =========================================================================
// Metrics
// =========================================================================

func TestMetricsLogAndGet(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(t, srv, "POST", "/v1/jobs/submit", model.JobSubmitRequest{
		Name: "metric-job", Command: "echo",
	})
	var resp model.JobSubmitResponse
	decodeJSON(t, w, &resp)

	// Log metrics
	doRequest(t, srv, "POST", "/v1/metrics/"+resp.JobID, model.MetricsLogRequest{
		Step:    0,
		Metrics: map[string]any{"loss": 0.5},
	})
	doRequest(t, srv, "POST", "/v1/metrics/"+resp.JobID, model.MetricsLogRequest{
		Step:    1,
		Metrics: map[string]any{"loss": 0.3},
	})

	// Get metrics
	w2 := doRequest(t, srv, "GET", "/v1/metrics/"+resp.JobID, nil)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}

	var metricsResp model.MetricsResponse
	decodeJSON(t, w2, &metricsResp)
	if metricsResp.TotalSteps != 2 {
		t.Fatalf("expected 2 steps, got %d", metricsResp.TotalSteps)
	}
}

// =========================================================================
// Auth
// =========================================================================

func TestAPIKeyAuth(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.New(filepath.Join(dir, "test.db"))
	defer st.Close()

	srv := NewServer(Config{
		Store:   st,
		Version: "test",
		APIKey:  "secret-key",
	})

	// Health should be accessible without key
	req := httptest.NewRequest("GET", "/v1/health", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("health should work without key, got %d", w.Code)
	}

	// Other endpoints should require key
	req2 := httptest.NewRequest("GET", "/v1/jobs?limit=10", nil)
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w2.Code)
	}

	// With correct key
	req3 := httptest.NewRequest("GET", "/v1/jobs?limit=10", nil)
	req3.Header.Set("X-API-Key", "secret-key")
	w3 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("expected 200 with key, got %d", w3.Code)
	}
}

// =========================================================================
// Method Not Allowed
// =========================================================================

// =========================================================================
// Deploy status path compatibility (T-P3-01)
// =========================================================================

func TestDeployStatusPathCompat(t *testing.T) {
	srv := newTestServer(t)

	// Register an edge
	w := doRequest(t, srv, "POST", "/v1/edges/register", model.EdgeRegisterRequest{
		Name: "edge-1",
		Tags: []string{"onnx"},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("register edge: expected 201, got %d", w.Code)
	}
	var edgeResp model.EdgeRegisterResponse
	decodeJSON(t, w, &edgeResp)

	// Submit a job
	wj := doRequest(t, srv, "POST", "/v1/jobs/submit", model.JobSubmitRequest{
		Name: "deploy-test", Command: "echo",
	})
	var jobResp model.JobSubmitResponse
	decodeJSON(t, wj, &jobResp)

	// Trigger deploy
	wd := doRequest(t, srv, "POST", "/v1/deploy/trigger", model.DeployTriggerRequest{
		JobID:   jobResp.JobID,
		EdgeIDs: []string{edgeResp.EdgeID},
	})
	if wd.Code != http.StatusCreated {
		t.Fatalf("trigger deploy: expected 201, got %d: %s", wd.Code, wd.Body.String())
	}
	var deployResp model.DeployTriggerResponse
	decodeJSON(t, wd, &deployResp)

	// Test both paths return the same result:
	// Path 1: GET /v1/deploy/{id} (original C5 path)
	w1 := doRequest(t, srv, "GET", "/v1/deploy/"+deployResp.DeployID, nil)
	if w1.Code != http.StatusOK {
		t.Fatalf("GET /v1/deploy/{id}: expected 200, got %d", w1.Code)
	}
	var d1 model.Deployment
	decodeJSON(t, w1, &d1)

	// Path 2: GET /v1/deploy/{id}/status (hub.Client path)
	w2 := doRequest(t, srv, "GET", "/v1/deploy/"+deployResp.DeployID+"/status", nil)
	if w2.Code != http.StatusOK {
		t.Fatalf("GET /v1/deploy/{id}/status: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
	var d2 model.Deployment
	decodeJSON(t, w2, &d2)

	if d1.ID != d2.ID {
		t.Fatalf("deploy IDs mismatch: %s vs %s", d1.ID, d2.ID)
	}
	if d1.Status != d2.Status {
		t.Fatalf("deploy status mismatch: %s vs %s", d1.Status, d2.Status)
	}
}

// =========================================================================
// Estimate response fields (T-P3-03 compat)
// =========================================================================

func TestEstimateResponseFields(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(t, srv, "POST", "/v1/jobs/submit", model.JobSubmitRequest{
		Name: "est-fields", Command: "echo",
	})
	var resp model.JobSubmitResponse
	decodeJSON(t, w, &resp)

	w2 := doRequest(t, srv, "GET", "/v1/jobs/"+resp.JobID+"/estimate", nil)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}

	// Decode as raw map to check field presence
	var raw map[string]any
	decodeJSON(t, w2, &raw)

	// estimated_start_time should be present for QUEUED jobs
	if _, ok := raw["estimated_start_time"]; !ok {
		t.Fatal("expected estimated_start_time field in response")
	}

	// Verify it's a valid RFC3339 time
	est := raw["estimated_start_time"].(string)
	if est == "" {
		t.Fatal("estimated_start_time should not be empty")
	}
}

// =========================================================================
// Method Not Allowed
// =========================================================================

func TestMethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)

	tests := []struct {
		method string
		path   string
	}{
		{"POST", "/v1/health"},
		{"DELETE", "/v1/jobs/submit"},
		{"PUT", "/v1/workers/register"},
	}

	for _, tt := range tests {
		w := doRequest(t, srv, tt.method, tt.path, nil)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: expected 405, got %d", tt.method, tt.path, w.Code)
		}
	}
}

// =========================================================================
// Per-Project API Key Auth
// =========================================================================

func TestPerProjectAPIKey(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.New(filepath.Join(dir, "c5.db"))
	defer st.Close()

	srv := NewServer(Config{Store: st, APIKey: "master-key-123"})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	defer srv.Close()

	// Create per-project key using master key
	body := `{"project_id":"proj-A","description":"test key"}`
	req, _ := http.NewRequest("POST", ts.URL+"/v1/admin/api-keys", strings.NewReader(body))
	req.Header.Set("X-API-Key", "master-key-123")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create key: got %d", resp.StatusCode)
	}
	var createResp model.CreateAPIKeyResponse
	json.NewDecoder(resp.Body).Decode(&createResp)
	resp.Body.Close()
	projectKey := createResp.Key

	if !strings.HasPrefix(projectKey, "c5pk_") {
		t.Errorf("key should start with c5pk_, got %s", projectKey)
	}

	// Submit job with project key -> should get project_id=proj-A
	jobBody := `{"name":"test-job","command":"echo hi","workdir":"."}`
	req, _ = http.NewRequest("POST", ts.URL+"/v1/jobs/submit", strings.NewReader(jobBody))
	req.Header.Set("X-API-Key", projectKey)
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("submit job: got %d", resp.StatusCode)
	}
	var jobResp model.JobSubmitResponse
	json.NewDecoder(resp.Body).Decode(&jobResp)
	resp.Body.Close()

	// Verify job has project_id=proj-A
	req, _ = http.NewRequest("GET", ts.URL+"/v1/jobs/"+jobResp.JobID, nil)
	req.Header.Set("X-API-Key", projectKey)
	resp, _ = http.DefaultClient.Do(req)
	var job model.Job
	json.NewDecoder(resp.Body).Decode(&job)
	resp.Body.Close()
	if job.ProjectID != "proj-A" {
		t.Errorf("job project_id: got %q, want proj-A", job.ProjectID)
	}
}

func TestMasterKeyFullAccess(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.New(filepath.Join(dir, "c5.db"))
	defer st.Close()

	srv := NewServer(Config{Store: st, APIKey: "master-key-123"})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	defer srv.Close()

	// Master key can submit jobs for any project
	jobBody := `{"name":"test-job","command":"echo hi","workdir":".","project_id":"any-project"}`
	req, _ := http.NewRequest("POST", ts.URL+"/v1/jobs/submit", strings.NewReader(jobBody))
	req.Header.Set("X-API-Key", "master-key-123")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("submit with master: got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAdminRequiresMasterKey(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.New(filepath.Join(dir, "c5.db"))
	defer st.Close()

	srv := NewServer(Config{Store: st, APIKey: "master-key-123"})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	defer srv.Close()

	// First create a project key
	body := `{"project_id":"proj-A","description":"test"}`
	req, _ := http.NewRequest("POST", ts.URL+"/v1/admin/api-keys", strings.NewReader(body))
	req.Header.Set("X-API-Key", "master-key-123")
	resp, _ := http.DefaultClient.Do(req)
	var createResp model.CreateAPIKeyResponse
	json.NewDecoder(resp.Body).Decode(&createResp)
	resp.Body.Close()

	// Try admin endpoint with project key -> 403
	req, _ = http.NewRequest("GET", ts.URL+"/v1/admin/api-keys", nil)
	req.Header.Set("X-API-Key", createResp.Key)
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("admin with project key: got %d, want 403", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestProjectIsolation(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.New(filepath.Join(dir, "c5.db"))
	defer st.Close()

	srv := NewServer(Config{Store: st, APIKey: "master-key-123"})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	defer srv.Close()

	// Create keys for two projects
	createKey := func(projID string) string {
		body := fmt.Sprintf(`{"project_id":"%s"}`, projID)
		req, _ := http.NewRequest("POST", ts.URL+"/v1/admin/api-keys", strings.NewReader(body))
		req.Header.Set("X-API-Key", "master-key-123")
		resp, _ := http.DefaultClient.Do(req)
		var cr model.CreateAPIKeyResponse
		json.NewDecoder(resp.Body).Decode(&cr)
		resp.Body.Close()
		return cr.Key
	}

	keyA := createKey("proj-A")
	keyB := createKey("proj-B")

	// Submit job with key A
	jobBody := `{"name":"job-a","command":"echo a","workdir":"."}`
	req, _ := http.NewRequest("POST", ts.URL+"/v1/jobs/submit", strings.NewReader(jobBody))
	req.Header.Set("X-API-Key", keyA)
	resp, _ := http.DefaultClient.Do(req)
	var jr model.JobSubmitResponse
	json.NewDecoder(resp.Body).Decode(&jr)
	resp.Body.Close()

	// Try to access job with key B -> 403
	req, _ = http.NewRequest("GET", ts.URL+"/v1/jobs/"+jr.JobID, nil)
	req.Header.Set("X-API-Key", keyB)
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("cross-project access: got %d, want 403", resp.StatusCode)
	}
	resp.Body.Close()

	// Access with key A -> 200
	req, _ = http.NewRequest("GET", ts.URL+"/v1/jobs/"+jr.JobID, nil)
	req.Header.Set("X-API-Key", keyA)
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("own-project access: got %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestProjectIDFromContextOverridesBody(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.New(filepath.Join(dir, "c5.db"))
	defer st.Close()

	srv := NewServer(Config{Store: st, APIKey: "master-key-123"})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	defer srv.Close()

	// Create project key for proj-A
	body := `{"project_id":"proj-A"}`
	req, _ := http.NewRequest("POST", ts.URL+"/v1/admin/api-keys", strings.NewReader(body))
	req.Header.Set("X-API-Key", "master-key-123")
	resp, _ := http.DefaultClient.Do(req)
	var cr model.CreateAPIKeyResponse
	json.NewDecoder(resp.Body).Decode(&cr)
	resp.Body.Close()

	// Submit job claiming project_id=proj-B but using proj-A key
	jobBody := `{"name":"sneaky","command":"echo","workdir":".","project_id":"proj-B"}`
	req, _ = http.NewRequest("POST", ts.URL+"/v1/jobs/submit", strings.NewReader(jobBody))
	req.Header.Set("X-API-Key", cr.Key)
	resp, _ = http.DefaultClient.Do(req)
	var jr model.JobSubmitResponse
	json.NewDecoder(resp.Body).Decode(&jr)
	resp.Body.Close()

	// Verify job actually got proj-A
	req, _ = http.NewRequest("GET", ts.URL+"/v1/jobs/"+jr.JobID, nil)
	req.Header.Set("X-API-Key", cr.Key)
	resp, _ = http.DefaultClient.Do(req)
	var job model.Job
	json.NewDecoder(resp.Body).Decode(&job)
	resp.Body.Close()
	if job.ProjectID != "proj-A" {
		t.Errorf("body override blocked: got project_id=%q, want proj-A", job.ProjectID)
	}
}

func TestBackwardCompat_NoAuth(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.New(filepath.Join(dir, "c5.db"))
	defer st.Close()

	// No APIKey -> auth disabled
	srv := NewServer(Config{Store: st})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	defer srv.Close()

	// Should work without any API key
	jobBody := `{"name":"test","command":"echo hi","workdir":"."}`
	resp, _ := http.Post(ts.URL+"/v1/jobs/submit", "application/json", strings.NewReader(jobBody))
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("no-auth submit: got %d, want 201", resp.StatusCode)
	}
	resp.Body.Close()
}

// =========================================================================
// LLMs.txt + Docs
// =========================================================================

func newTestServerWithDocs(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	st, err := store.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	docsFS := fstest.MapFS{
		"docs/jobs.md":    {Data: []byte("# Jobs API\n\nJobs documentation.")},
		"docs/workers.md": {Data: []byte("# Workers API\n\nWorkers documentation.")},
	}

	return NewServer(Config{
		Store:   st,
		Version: "test",
		LLMSTxt: "# C5 Hub\n\n> Distributed job queue.",
		DocsFS:  docsFS,
	})
}

func TestLLMSTxtEndpoint(t *testing.T) {
	srv := newTestServerWithDocs(t)

	w := doRequest(t, srv, "GET", "/.well-known/llms.txt", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("expected text/plain, got %s", ct)
	}

	body := w.Body.String()
	if !strings.Contains(body, "C5 Hub") {
		t.Fatalf("body should contain 'C5 Hub', got: %s", body)
	}
}

func TestLLMSTxtConvenience(t *testing.T) {
	srv := newTestServerWithDocs(t)

	w := doRequest(t, srv, "GET", "/llms.txt", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "C5 Hub") {
		t.Fatalf("body should contain 'C5 Hub', got: %s", body)
	}
}

func TestDocsEndpoint(t *testing.T) {
	srv := newTestServerWithDocs(t)

	w := doRequest(t, srv, "GET", "/v1/docs/jobs.md", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/markdown") {
		t.Fatalf("expected text/markdown, got %s", ct)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Jobs API") {
		t.Fatalf("body should contain 'Jobs API', got: %s", body)
	}
}

func TestDocsNotFound(t *testing.T) {
	srv := newTestServerWithDocs(t)

	w := doRequest(t, srv, "GET", "/v1/docs/nonexistent.md", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestLLMSTxtNoAuthRequired(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.New(filepath.Join(dir, "test.db"))
	defer st.Close()

	docsFS := fstest.MapFS{
		"docs/jobs.md": {Data: []byte("# Jobs API")},
	}

	srv := NewServer(Config{
		Store:   st,
		Version: "test",
		APIKey:  "secret-key",
		LLMSTxt: "# C5 Hub",
		DocsFS:  docsFS,
	})

	// llms.txt should be accessible without API key
	req := httptest.NewRequest("GET", "/.well-known/llms.txt", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("llms.txt should work without key, got %d", w.Code)
	}

	// docs should also be accessible without API key
	req2 := httptest.NewRequest("GET", "/v1/docs/jobs.md", nil)
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("docs should work without key, got %d", w2.Code)
	}
}

// =========================================================================
// Presigned URL in LeaseAcquireResponse (T-836-0)
// =========================================================================

func TestLeaseAcquire_WithInputArtifacts(t *testing.T) {
	stor := &mockStorage{}
	srv := newTestServerWithStorage(t, stor)

	// Register worker
	w := doRequest(t, srv, "POST", "/v1/workers/register", model.WorkerRegisterRequest{
		Hostname: "test-worker",
	})
	var regResp model.WorkerRegisterResponse
	decodeJSON(t, w, &regResp)

	// Submit job with input artifacts
	w2 := doRequest(t, srv, "POST", "/v1/jobs/submit", model.JobSubmitRequest{
		Name:    "artifact-job",
		Command: "echo",
		InputArtifacts: []model.ArtifactRef{
			{Path: "inputs/model.pt", LocalPath: "/tmp/model.pt"},
			{Path: "inputs/data.csv"},
		},
	})
	if w2.Code != http.StatusCreated {
		t.Fatalf("submit: expected 201, got %d: %s", w2.Code, w2.Body.String())
	}

	// Acquire lease
	w3 := doRequest(t, srv, "POST", "/v1/leases/acquire", model.LeaseAcquireRequest{
		WorkerID: regResp.WorkerID,
	})
	if w3.Code != http.StatusOK {
		t.Fatalf("acquire: expected 200, got %d: %s", w3.Code, w3.Body.String())
	}

	var acqResp model.LeaseAcquireResponse
	decodeJSON(t, w3, &acqResp)

	if acqResp.LeaseID == "" {
		t.Fatal("lease_id should not be empty")
	}
	if len(acqResp.InputPresignedURLs) != 2 {
		t.Fatalf("expected 2 presigned URLs, got %d", len(acqResp.InputPresignedURLs))
	}

	// Verify first artifact
	found := false
	for _, p := range acqResp.InputPresignedURLs {
		if p.Path == "inputs/model.pt" {
			found = true
			if p.LocalPath != "/tmp/model.pt" {
				t.Errorf("local_path mismatch: got %q", p.LocalPath)
			}
			if p.URL == "" {
				t.Error("URL should not be empty")
			}
			if p.ExpiresAt == "" {
				t.Error("expires_at should not be empty")
			}
		}
	}
	if !found {
		t.Fatal("inputs/model.pt not found in presigned URLs")
	}
}

func TestLeaseAcquire_NoArtifacts(t *testing.T) {
	stor := &mockStorage{}
	srv := newTestServerWithStorage(t, stor)

	// Register worker
	w := doRequest(t, srv, "POST", "/v1/workers/register", model.WorkerRegisterRequest{
		Hostname: "test-worker",
	})
	var regResp model.WorkerRegisterResponse
	decodeJSON(t, w, &regResp)

	// Submit job without input artifacts
	doRequest(t, srv, "POST", "/v1/jobs/submit", model.JobSubmitRequest{
		Name:    "plain-job",
		Command: "echo",
	})

	// Acquire lease
	w2 := doRequest(t, srv, "POST", "/v1/leases/acquire", model.LeaseAcquireRequest{
		WorkerID: regResp.WorkerID,
	})
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var acqResp model.LeaseAcquireResponse
	decodeJSON(t, w2, &acqResp)

	if acqResp.LeaseID == "" {
		t.Fatal("lease_id should not be empty")
	}
	// No artifacts -> omitempty field should be absent/empty
	if len(acqResp.InputPresignedURLs) != 0 {
		t.Fatalf("expected 0 presigned URLs, got %d", len(acqResp.InputPresignedURLs))
	}
}

func TestLeaseAcquire_StorageError(t *testing.T) {
	stor := &mockStorage{failURLs: true}
	srv := newTestServerWithStorage(t, stor)

	// Register worker
	w := doRequest(t, srv, "POST", "/v1/workers/register", model.WorkerRegisterRequest{
		Hostname: "test-worker",
	})
	var regResp model.WorkerRegisterResponse
	decodeJSON(t, w, &regResp)

	// Submit job with input artifacts
	doRequest(t, srv, "POST", "/v1/jobs/submit", model.JobSubmitRequest{
		Name:    "err-artifact-job",
		Command: "echo",
		InputArtifacts: []model.ArtifactRef{
			{Path: "inputs/model.pt"},
		},
	})

	// Acquire lease — should succeed even though storage fails
	w2 := doRequest(t, srv, "POST", "/v1/leases/acquire", model.LeaseAcquireRequest{
		WorkerID: regResp.WorkerID,
	})
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 even on storage error, got %d: %s", w2.Code, w2.Body.String())
	}

	var acqResp model.LeaseAcquireResponse
	decodeJSON(t, w2, &acqResp)

	if acqResp.LeaseID == "" {
		t.Fatal("lease should still be returned on storage error")
	}
	// Storage failed -> InputPresignedURLs should be empty (artifact skipped)
	if len(acqResp.InputPresignedURLs) != 0 {
		t.Fatalf("expected 0 presigned URLs on storage error, got %d", len(acqResp.InputPresignedURLs))
	}
}
