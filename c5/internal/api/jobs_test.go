package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/piqsol/c4/c5/internal/model"
	"github.com/piqsol/c4/c5/internal/store"
)

// newTestServerWithKey creates a test server with a master API key.
func newTestServerWithKey(t *testing.T, masterKey string) *Server {
	t.Helper()
	dir := t.TempDir()
	st, err := store.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return NewServer(Config{Store: st, APIKey: masterKey, Version: "test"})
}

// createProjectKey creates a per-project API key via the admin endpoint.
func createProjectKey(t *testing.T, ts *httptest.Server, masterKey, projectID string) string {
	t.Helper()
	body := `{"project_id":"` + projectID + `"}`
	req, _ := http.NewRequest("POST", ts.URL+"/v1/admin/api-keys", strings.NewReader(body))
	req.Header.Set("X-API-Key", masterKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create key request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create project key: got %d", resp.StatusCode)
	}
	var cr model.CreateAPIKeyResponse
	json.NewDecoder(resp.Body).Decode(&cr)
	return cr.Key
}

// TestJobSubmit_SubmitterID verifies that a project key submission sets submitted_by=project_id.
func TestJobSubmit_SubmitterID(t *testing.T) {
	const masterKey = "master-key-test"
	const projectID = "proj-audit-test"

	srv := newTestServerWithKey(t, masterKey)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	defer srv.Close()

	projectKey := createProjectKey(t, ts, masterKey, projectID)

	// Submit job using project key.
	jobBody := `{"name":"audit-job","command":"echo test","workdir":"."}`
	req, _ := http.NewRequest("POST", ts.URL+"/v1/jobs/submit", strings.NewReader(jobBody))
	req.Header.Set("X-API-Key", projectKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("submit job: got %d", resp.StatusCode)
	}
	var jobResp model.JobSubmitResponse
	json.NewDecoder(resp.Body).Decode(&jobResp)

	// Fetch the job and check submitted_by.
	req, _ = http.NewRequest("GET", ts.URL+"/v1/jobs/"+jobResp.JobID, nil)
	req.Header.Set("X-API-Key", projectKey)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	defer resp2.Body.Close()

	var job model.Job
	json.NewDecoder(resp2.Body).Decode(&job)

	if job.SubmittedBy != projectID {
		t.Errorf("submitted_by: got %q, want %q", job.SubmittedBy, projectID)
	}
	if job.ProjectID != projectID {
		t.Errorf("project_id: got %q, want %q", job.ProjectID, projectID)
	}
}

// TestJobSubmit_MasterKey verifies that master key submission leaves submitted_by empty.
func TestJobSubmit_MasterKey(t *testing.T) {
	const masterKey = "master-key-test"

	srv := newTestServerWithKey(t, masterKey)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	defer srv.Close()

	// Submit job using master key (no project_id in auth context).
	jobBody := `{"name":"master-job","command":"echo test","workdir":"."}`
	req, _ := http.NewRequest("POST", ts.URL+"/v1/jobs/submit", strings.NewReader(jobBody))
	req.Header.Set("X-API-Key", masterKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("submit job: got %d", resp.StatusCode)
	}
	var jobResp model.JobSubmitResponse
	json.NewDecoder(resp.Body).Decode(&jobResp)

	// Fetch the job; submitted_by must be empty for master key.
	req, _ = http.NewRequest("GET", ts.URL+"/v1/jobs/"+jobResp.JobID, nil)
	req.Header.Set("X-API-Key", masterKey)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	defer resp2.Body.Close()

	var job model.Job
	json.NewDecoder(resp2.Body).Decode(&job)

	if job.SubmittedBy != "" {
		t.Errorf("submitted_by: got %q, want empty for master key", job.SubmittedBy)
	}
}

// submitAndAcquireJob submits a job, registers a worker, and acquires a lease
// (transitions the job to RUNNING status). Returns the jobID and apiKey.
func submitAndAcquireJob(t *testing.T, srv *Server, masterKey string) string {
	t.Helper()
	// Submit.
	var submitResp model.JobSubmitResponse
	w := doRequest(t, srv, "POST", "/v1/jobs/submit",
		map[string]any{"name": "sse-test-job", "command": "echo hi", "workdir": "."})
	if w.Code != http.StatusCreated {
		t.Fatalf("submit job: got %d: %s", w.Code, w.Body.String())
	}
	json.NewDecoder(w.Body).Decode(&submitResp)

	// Register worker.
	var regResp model.WorkerRegisterResponse
	w2 := doRequest(t, srv, "POST", "/v1/workers/register",
		model.WorkerRegisterRequest{Hostname: "sse-test-worker"})
	if w2.Code != http.StatusCreated {
		t.Fatalf("register worker: got %d: %s", w2.Code, w2.Body.String())
	}
	json.NewDecoder(w2.Body).Decode(&regResp)

	// Acquire lease (sets job to RUNNING).
	w3 := doRequest(t, srv, "POST", "/v1/leases/acquire",
		model.LeaseAcquireRequest{WorkerID: regResp.WorkerID})
	if w3.Code != http.StatusOK {
		t.Fatalf("acquire lease: got %d: %s", w3.Code, w3.Body.String())
	}

	return submitResp.JobID
}

// TestSSEBroadcastOnJobComplete verifies that completing a job with exit_code=0
// causes a "hub.job.completed" SSE event to be delivered to a subscriber.
func TestSSEBroadcastOnJobComplete(t *testing.T) {
	srv := newTestServer(t)

	jobID := submitAndAcquireJob(t, srv, "")

	// Register SSE subscriber.
	ch := make(chan string, 16)
	srv.sseSubs.Store(ch, "") // master subscriber
	defer srv.sseSubs.Delete(ch)

	// Complete the job with success.
	exitZero := 0
	w := doRequest(t, srv, "POST", "/v1/jobs/"+jobID+"/complete",
		model.JobCompleteRequest{Status: "SUCCEEDED", ExitCode: &exitZero})
	if w.Code != http.StatusOK {
		t.Fatalf("complete job: got %d: %s", w.Code, w.Body.String())
	}

	// Expect SSE event.
	select {
	case msg := <-ch:
		if !strings.Contains(msg, "hub.job.completed") {
			t.Fatalf("expected hub.job.completed, got: %s", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive SSE event for job completion")
	}
}

// TestSSEBroadcastOnJobFailed verifies that completing a job with exit_code!=0
// causes a "hub.job.failed" SSE event to be delivered.
func TestSSEBroadcastOnJobFailed(t *testing.T) {
	srv := newTestServer(t)

	jobID := submitAndAcquireJob(t, srv, "")

	// Register SSE subscriber.
	ch := make(chan string, 16)
	srv.sseSubs.Store(ch, "") // master subscriber
	defer srv.sseSubs.Delete(ch)

	// Complete the job with failure (exit_code=1).
	exitOne := 1
	w := doRequest(t, srv, "POST", "/v1/jobs/"+jobID+"/complete",
		model.JobCompleteRequest{Status: "FAILED", ExitCode: &exitOne})
	if w.Code != http.StatusOK {
		t.Fatalf("complete job: got %d: %s", w.Code, w.Body.String())
	}

	// Expect SSE event with failed type.
	select {
	case msg := <-ch:
		if !strings.Contains(msg, "hub.job.failed") {
			t.Fatalf("expected hub.job.failed, got: %s", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive SSE event for job failure")
	}
}

// TestSSEBroadcastSkipsNilJob verifies that when GetJob fails (nil completedJob),
// broadcastSSEEvent is not called and the handler does not panic.
func TestSSEBroadcastSkipsNilJob(t *testing.T) {
	srv := newTestServer(t)

	// Register SSE subscriber.
	ch := make(chan string, 16)
	srv.sseSubs.Store(ch, "")
	defer srv.sseSubs.Delete(ch)

	// Attempt to complete a non-existent job — CompleteJob will fail before GetJob is called.
	exitZero := 0
	w := doRequest(t, srv, "POST", "/v1/jobs/nonexistent-job-id/complete",
		model.JobCompleteRequest{Status: "SUCCEEDED", ExitCode: &exitZero})
	// Expect an error (job not found) — no panic.
	if w.Code == http.StatusOK {
		t.Fatal("expected non-200 for nonexistent job")
	}

	// No SSE event should be delivered.
	select {
	case msg := <-ch:
		t.Fatalf("expected no SSE event for nil job, got: %s", msg)
	case <-time.After(100 * time.Millisecond):
		// OK — no broadcast
	}
}
