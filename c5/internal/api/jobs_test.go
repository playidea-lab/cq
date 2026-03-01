package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

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
