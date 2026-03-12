package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/piqsol/c4/c5/internal/model"
	"github.com/piqsol/c4/c5/internal/store"
)

// registerWorkerAndAcquire is a helper that registers a worker and acquires the
// lease for the next queued job, returning the acquired job ID.
func registerWorkerAndAcquire(t *testing.T, srv *Server, workerID string) string {
	t.Helper()

	// Register worker.
	ww := doRequest(t, srv, "POST", "/v1/workers/register", model.WorkerRegisterRequest{
		Hostname: workerID,
		GPUCount: 0,
	})
	if ww.Code != http.StatusCreated {
		t.Fatalf("register worker: got %d", ww.Code)
	}
	var regResp model.WorkerRegisterResponse
	decodeJSON(t, ww, &regResp)

	// Acquire lease.
	wl := doRequest(t, srv, "POST", "/v1/leases/acquire", model.LeaseAcquireRequest{
		WorkerID: regResp.WorkerID,
	})
	if wl.Code != http.StatusOK {
		t.Fatalf("acquire lease: got %d", wl.Code)
	}
	var acq model.LeaseAcquireResponse
	decodeJSON(t, wl, &acq)
	return acq.Job.ID
}

// TestHubJobCompletion_WithExperiment_TriggersCompleteRun verifies that completing
// a job with exp_run_id calls CompleteRun on the experiment store (status=success).
func TestHubJobCompletion_WithExperiment_TriggersCompleteRun(t *testing.T) {
	srv := newTestServer(t)

	// Create an experiment run.
	w := doRequest(t, srv, "POST", "/v1/experiment/run", map[string]string{
		"name": "train-run", "capability": "pose",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create run: got %d", w.Code)
	}
	var runResp map[string]any
	decodeJSON(t, w, &runResp)
	runID := runResp["run_id"].(string)

	// Submit a job linked to the run.
	w2 := doRequest(t, srv, "POST", "/v1/jobs/submit", model.JobSubmitRequest{
		Name:     "train-job",
		Command:  "echo ok",
		Workdir:  ".",
		ExpRunID: runID,
	})
	if w2.Code != http.StatusCreated {
		t.Fatalf("submit job: got %d", w2.Code)
	}
	var jobResp model.JobSubmitResponse
	decodeJSON(t, w2, &jobResp)
	jobID := jobResp.JobID

	// Register worker and acquire lease.
	acquiredID := registerWorkerAndAcquire(t, srv, "w-exp-complete")
	if acquiredID != jobID {
		t.Fatalf("acquired wrong job: want %s, got %s", jobID, acquiredID)
	}

	// Complete the job as succeeded.
	exitCode := 0
	w3 := doRequest(t, srv, "POST", "/v1/jobs/"+jobID+"/complete", model.JobCompleteRequest{
		Status:   "SUCCEEDED",
		ExitCode: &exitCode,
	})
	if w3.Code != http.StatusOK {
		t.Fatalf("complete job: got %d: %s", w3.Code, w3.Body.String())
	}

	// Verify the experiment run was completed with status "success" (mapped from StatusSucceeded).
	runs := searchRunsDirect(t, srv, "train-run")
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0]["status"] != "success" {
		t.Fatalf("expected run status=success, got %v", runs[0]["status"])
	}
}

// TestHubJobCompletion_WithoutExperiment_NoOp verifies that completing a job
// without exp_run_id does not affect experiment runs.
func TestHubJobCompletion_WithoutExperiment_NoOp(t *testing.T) {
	srv := newTestServer(t)

	// Submit job with no exp_run_id.
	w := doRequest(t, srv, "POST", "/v1/jobs/submit", model.JobSubmitRequest{
		Name:    "plain-job",
		Command: "echo",
		Workdir: ".",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("submit job: got %d", w.Code)
	}
	var jobResp model.JobSubmitResponse
	decodeJSON(t, w, &jobResp)
	jobID := jobResp.JobID

	// Register worker and acquire lease.
	registerWorkerAndAcquire(t, srv, "w-no-exp")

	// Complete the job.
	exitCode := 0
	w2 := doRequest(t, srv, "POST", "/v1/jobs/"+jobID+"/complete", model.JobCompleteRequest{
		Status:   "SUCCEEDED",
		ExitCode: &exitCode,
	})
	if w2.Code != http.StatusOK {
		t.Fatalf("complete job: got %d: %s", w2.Code, w2.Body.String())
	}

	// No experiment runs should exist.
	runs := searchRunsDirect(t, srv, "")
	if len(runs) != 0 {
		t.Fatalf("expected 0 runs, got %d", len(runs))
	}
}

// TestHubJobCompletion_Cancelled_TriggersCompleteRun verifies that cancelling a job
// with exp_run_id calls CompleteRun with status "cancelled".
func TestHubJobCompletion_Cancelled_TriggersCompleteRun(t *testing.T) {
	srv := newTestServer(t)

	// Create an experiment run.
	w := doRequest(t, srv, "POST", "/v1/experiment/run", map[string]string{
		"name": "cancel-run", "capability": "cap1",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create run: got %d", w.Code)
	}
	var runResp map[string]any
	decodeJSON(t, w, &runResp)
	runID := runResp["run_id"].(string)

	// Submit a job linked to the run.
	w2 := doRequest(t, srv, "POST", "/v1/jobs/submit", model.JobSubmitRequest{
		Name:     "cancel-job",
		Command:  "echo ok",
		Workdir:  ".",
		ExpRunID: runID,
	})
	if w2.Code != http.StatusCreated {
		t.Fatalf("submit job: got %d", w2.Code)
	}
	var jobResp model.JobSubmitResponse
	decodeJSON(t, w2, &jobResp)
	jobID := jobResp.JobID

	// Register worker and acquire lease so job transitions to RUNNING.
	acquiredID := registerWorkerAndAcquire(t, srv, "w-cancel-exp")
	if acquiredID != jobID {
		t.Fatalf("acquired wrong job: want %s, got %s", jobID, acquiredID)
	}

	// Cancel the job via POST /v1/jobs/{id}/cancel.
	w3 := doRequest(t, srv, "POST", "/v1/jobs/"+jobID+"/cancel", nil)
	if w3.Code != http.StatusOK {
		t.Fatalf("cancel job: got %d: %s", w3.Code, w3.Body.String())
	}

	// Verify the experiment run was completed with status "cancelled".
	runs := searchRunsDirect(t, srv, "cancel-run")
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0]["status"] != "cancelled" {
		t.Fatalf("expected run status=cancelled, got %v", runs[0]["status"])
	}
}

// searchRunsDirect calls GET /v1/experiment/search via the test server recorder.
func searchRunsDirect(t *testing.T, srv *Server, query string) []map[string]any {
	t.Helper()
	path := "/v1/experiment/search"
	if query != "" {
		path += "?query=" + query
	}
	w := doRequest(t, srv, "GET", path, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("search runs: got %d: %s", w.Code, w.Body.String())
	}
	var runs []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&runs); err != nil {
		return []map[string]any{}
	}
	if runs == nil {
		return []map[string]any{}
	}
	return runs
}

// Ensure the store.ExperimentRun type is visible (import check).
var _ = store.ExperimentRun{}
