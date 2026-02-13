package daemon_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/daemon"
	"github.com/changmin/c4-core/internal/hub"
)

// setupIntegration creates a full daemon stack (store + scheduler + server)
// and returns a test HTTP server. Caller should defer ts.Close() and store.Close().
func setupIntegration(t *testing.T) (*httptest.Server, *daemon.Store) {
	t.Helper()
	dir := t.TempDir()
	store, err := daemon.NewStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}

	sched := daemon.NewScheduler(store, daemon.SchedulerConfig{
		DataDir:      dir,
		PollInterval: 50 * time.Millisecond,
	})

	srv := daemon.NewServer(daemon.ServerConfig{
		Store:     store,
		Scheduler: sched,
		Version:   "test-0.1.0",
	})

	ts := httptest.NewServer(srv.Handler())
	return ts, store
}

// =========================================================================
// Integration Tests — HTTP endpoint lifecycle
// =========================================================================

func TestIntegration_HealthCheck(t *testing.T) {
	ts, store := setupIntegration(t)
	defer ts.Close()
	defer store.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if result["status"] != "ok" {
		t.Errorf("health status = %v, want ok", result["status"])
	}
	if result["version"] != "test-0.1.0" {
		t.Errorf("version = %v, want test-0.1.0", result["version"])
	}
}

func TestIntegration_SubmitAndGetJob(t *testing.T) {
	ts, store := setupIntegration(t)
	defer ts.Close()
	defer store.Close()

	// Submit a job
	body := `{"name":"test-job","command":"echo hello","workdir":"."}`
	resp, err := http.Post(ts.URL+"/jobs/submit", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		t.Fatalf("submit status = %d, want 201", resp.StatusCode)
	}

	var submitResp daemon.JobSubmitResponse
	json.NewDecoder(resp.Body).Decode(&submitResp)
	if submitResp.JobID == "" {
		t.Fatal("job_id should not be empty")
	}
	if submitResp.Status != "QUEUED" {
		t.Errorf("status = %s, want QUEUED", submitResp.Status)
	}

	// Get the job
	resp2, err := http.Get(ts.URL + "/jobs/" + submitResp.JobID)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()

	var job daemon.Job
	json.NewDecoder(resp2.Body).Decode(&job)
	if job.ID != submitResp.JobID {
		t.Errorf("job_id = %s, want %s", job.ID, submitResp.JobID)
	}
	if job.Name != "test-job" {
		t.Errorf("name = %s, want test-job", job.Name)
	}
	if string(job.Status) != "QUEUED" {
		t.Errorf("status = %s, want QUEUED", job.Status)
	}
}

func TestIntegration_ListJobs(t *testing.T) {
	ts, store := setupIntegration(t)
	defer ts.Close()
	defer store.Close()

	// Submit 3 jobs
	for i := 0; i < 3; i++ {
		body := fmt.Sprintf(`{"name":"job-%d","command":"echo %d","workdir":"."}`, i, i)
		http.Post(ts.URL+"/jobs/submit", "application/json", strings.NewReader(body))
	}

	// List all jobs
	resp, err := http.Get(ts.URL + "/jobs")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result struct {
		Jobs       []daemon.Job `json:"jobs"`
		TotalCount int          `json:"total_count"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.TotalCount != 3 {
		t.Errorf("total_count = %d, want 3", result.TotalCount)
	}

	// List with status filter
	resp2, err := http.Get(ts.URL + "/jobs?status=QUEUED")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()

	var result2 struct {
		Jobs       []daemon.Job `json:"jobs"`
		TotalCount int          `json:"total_count"`
	}
	json.NewDecoder(resp2.Body).Decode(&result2)
	if result2.TotalCount != 3 {
		t.Errorf("queued count = %d, want 3", result2.TotalCount)
	}
}

func TestIntegration_QueueStats(t *testing.T) {
	ts, store := setupIntegration(t)
	defer ts.Close()
	defer store.Close()

	// Submit 2 jobs
	for i := 0; i < 2; i++ {
		body := fmt.Sprintf(`{"name":"j%d","command":"echo %d","workdir":"."}`, i, i)
		http.Post(ts.URL+"/jobs/submit", "application/json", strings.NewReader(body))
	}

	resp, err := http.Get(ts.URL + "/stats/queue")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var stats daemon.QueueStats
	json.NewDecoder(resp.Body).Decode(&stats)
	if stats.Queued != 2 {
		t.Errorf("queued = %d, want 2", stats.Queued)
	}
}

func TestIntegration_CancelJob(t *testing.T) {
	ts, store := setupIntegration(t)
	defer ts.Close()
	defer store.Close()

	// Submit
	body := `{"name":"cancel-me","command":"sleep 100","workdir":"."}`
	resp, _ := http.Post(ts.URL+"/jobs/submit", "application/json", strings.NewReader(body))
	var sr daemon.JobSubmitResponse
	json.NewDecoder(resp.Body).Decode(&sr)
	resp.Body.Close()

	// Cancel
	resp2, err := http.Post(ts.URL+"/jobs/"+sr.JobID+"/cancel", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != 200 {
		t.Fatalf("cancel status = %d, want 200", resp2.StatusCode)
	}

	// Verify cancelled
	resp3, _ := http.Get(ts.URL + "/jobs/" + sr.JobID)
	var job daemon.Job
	json.NewDecoder(resp3.Body).Decode(&job)
	resp3.Body.Close()

	if string(job.Status) != "CANCELLED" {
		t.Errorf("status = %s, want CANCELLED", job.Status)
	}
}

func TestIntegration_CompleteJob(t *testing.T) {
	ts, store := setupIntegration(t)
	defer ts.Close()
	defer store.Close()

	// Submit
	body := `{"name":"complete-me","command":"echo ok","workdir":"."}`
	resp, _ := http.Post(ts.URL+"/jobs/submit", "application/json", strings.NewReader(body))
	var sr daemon.JobSubmitResponse
	json.NewDecoder(resp.Body).Decode(&sr)
	resp.Body.Close()

	// Start the job manually via store
	store.StartJob(sr.JobID, 12345, nil)

	// Complete via API
	completeBody := `{"status":"SUCCEEDED","exit_code":0}`
	resp2, err := http.Post(ts.URL+"/jobs/"+sr.JobID+"/complete", "application/json", strings.NewReader(completeBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != 200 {
		t.Fatalf("complete status = %d, want 200", resp2.StatusCode)
	}

	// Verify
	resp3, _ := http.Get(ts.URL + "/jobs/" + sr.JobID)
	var job daemon.Job
	json.NewDecoder(resp3.Body).Decode(&job)
	resp3.Body.Close()

	if string(job.Status) != "SUCCEEDED" {
		t.Errorf("status = %s, want SUCCEEDED", job.Status)
	}
}

func TestIntegration_RetryJob(t *testing.T) {
	ts, store := setupIntegration(t)
	defer ts.Close()
	defer store.Close()

	// Submit + fail
	body := `{"name":"retry-me","command":"echo fail","workdir":"."}`
	resp, _ := http.Post(ts.URL+"/jobs/submit", "application/json", strings.NewReader(body))
	var sr daemon.JobSubmitResponse
	json.NewDecoder(resp.Body).Decode(&sr)
	resp.Body.Close()

	store.StartJob(sr.JobID, 999, nil)
	store.CompleteJob(sr.JobID, daemon.StatusFailed, 1)

	// Retry
	resp2, err := http.Post(ts.URL+"/jobs/"+sr.JobID+"/retry", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != 200 {
		t.Fatalf("retry status = %d, want 200", resp2.StatusCode)
	}

	var retryResp struct {
		NewJobID      string `json:"new_job_id"`
		Status        string `json:"status"`
		OriginalJobID string `json:"original_job_id"`
	}
	json.NewDecoder(resp2.Body).Decode(&retryResp)

	if retryResp.NewJobID == "" {
		t.Fatal("new_job_id should not be empty")
	}
	if retryResp.OriginalJobID != sr.JobID {
		t.Errorf("original_job_id = %s, want %s", retryResp.OriginalJobID, sr.JobID)
	}
	if retryResp.Status != "QUEUED" {
		t.Errorf("status = %s, want QUEUED", retryResp.Status)
	}
}

func TestIntegration_EstimateJob(t *testing.T) {
	ts, store := setupIntegration(t)
	defer ts.Close()
	defer store.Close()

	// Submit a job
	body := `{"name":"estimate-me","command":"echo test","workdir":"."}`
	resp, _ := http.Post(ts.URL+"/jobs/submit", "application/json", strings.NewReader(body))
	var sr daemon.JobSubmitResponse
	json.NewDecoder(resp.Body).Decode(&sr)
	resp.Body.Close()

	// Get estimate (no history → default method)
	resp2, err := http.Get(ts.URL + "/jobs/" + sr.JobID + "/estimate")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()

	var est daemon.EstimateResult
	json.NewDecoder(resp2.Body).Decode(&est)

	if est.Method != "default" {
		t.Errorf("method = %s, want default", est.Method)
	}
	if est.EstimatedDurationSec != 300 {
		t.Errorf("duration = %f, want 300", est.EstimatedDurationSec)
	}
}

func TestIntegration_GPUStatus(t *testing.T) {
	ts, store := setupIntegration(t)
	defer ts.Close()
	defer store.Close()

	resp, err := http.Get(ts.URL + "/gpu/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	// No GPU on test machine → available = false
	if result["available"] != false {
		t.Logf("GPU available = %v (test machine has GPU)", result["available"])
	}
}

func TestIntegration_MethodNotAllowed(t *testing.T) {
	ts, store := setupIntegration(t)
	defer ts.Close()
	defer store.Close()

	// GET on POST-only endpoint
	resp, err := http.Get(ts.URL + "/jobs/submit")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 405 {
		t.Errorf("status = %d, want 405", resp.StatusCode)
	}
}

func TestIntegration_InvalidJSON(t *testing.T) {
	ts, store := setupIntegration(t)
	defer ts.Close()
	defer store.Close()

	resp, err := http.Post(ts.URL+"/jobs/submit", "application/json", strings.NewReader("{bad json"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestIntegration_MissingCommand(t *testing.T) {
	ts, store := setupIntegration(t)
	defer ts.Close()
	defer store.Close()

	resp, err := http.Post(ts.URL+"/jobs/submit", "application/json", strings.NewReader(`{"name":"no-cmd"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestIntegration_NotFound(t *testing.T) {
	ts, store := setupIntegration(t)
	defer ts.Close()
	defer store.Close()

	resp, err := http.Get(ts.URL + "/jobs/nonexistent-id")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

// =========================================================================
// Hub Client Compatibility Tests
// =========================================================================

func TestHubCompat_HealthCheck(t *testing.T) {
	ts, store := setupIntegration(t)
	defer ts.Close()
	defer store.Close()

	client := hub.NewClient(hub.HubConfig{
		URL:       ts.URL,
		APIPrefix: "", // local daemon mode
	})

	if !client.HealthCheck() {
		t.Error("hub client health check failed against daemon")
	}
}

func TestHubCompat_SubmitAndGetJob(t *testing.T) {
	ts, store := setupIntegration(t)
	defer ts.Close()
	defer store.Close()

	client := hub.NewClient(hub.HubConfig{
		URL:       ts.URL,
		APIPrefix: "",
	})

	// Submit
	submitResp, err := client.SubmitJob(&hub.JobSubmitRequest{
		Name:    "hub-test",
		Command: "echo hello",
		Workdir: ".",
	})
	if err != nil {
		t.Fatal(err)
	}
	if submitResp.JobID == "" {
		t.Fatal("job_id should not be empty")
	}
	if submitResp.Status != "QUEUED" {
		t.Errorf("status = %s, want QUEUED", submitResp.Status)
	}

	// Get
	job, err := client.GetJob(submitResp.JobID)
	if err != nil {
		t.Fatal(err)
	}
	// daemon uses "job_id" field → hub.Job.JobID
	if job.GetID() != submitResp.JobID {
		t.Errorf("job id = %s, want %s", job.GetID(), submitResp.JobID)
	}
	if job.Status != "QUEUED" {
		t.Errorf("status = %s, want QUEUED", job.Status)
	}
	if job.Name != "hub-test" {
		t.Errorf("name = %s, want hub-test", job.Name)
	}
}

func TestHubCompat_ListJobs(t *testing.T) {
	ts, store := setupIntegration(t)
	defer ts.Close()
	defer store.Close()

	client := hub.NewClient(hub.HubConfig{
		URL:       ts.URL,
		APIPrefix: "",
	})

	// Submit 2 jobs
	for i := 0; i < 2; i++ {
		client.SubmitJob(&hub.JobSubmitRequest{
			Name:    fmt.Sprintf("list-%d", i),
			Command: fmt.Sprintf("echo %d", i),
			Workdir: ".",
		})
	}

	// List all
	jobs, err := client.ListJobs("", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Errorf("job count = %d, want 2", len(jobs))
	}

	// List with status filter
	jobs2, err := client.ListJobs("QUEUED", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs2) != 2 {
		t.Errorf("queued count = %d, want 2", len(jobs2))
	}
}

func TestHubCompat_CancelJob(t *testing.T) {
	ts, store := setupIntegration(t)
	defer ts.Close()
	defer store.Close()

	client := hub.NewClient(hub.HubConfig{
		URL:       ts.URL,
		APIPrefix: "",
	})

	submitResp, _ := client.SubmitJob(&hub.JobSubmitRequest{
		Name:    "cancel-hub",
		Command: "sleep 100",
		Workdir: ".",
	})

	err := client.CancelJob(submitResp.JobID)
	if err != nil {
		t.Fatal(err)
	}

	job, _ := client.GetJob(submitResp.JobID)
	if job.Status != "CANCELLED" {
		t.Errorf("status = %s, want CANCELLED", job.Status)
	}
}

func TestHubCompat_QueueStats(t *testing.T) {
	ts, store := setupIntegration(t)
	defer ts.Close()
	defer store.Close()

	client := hub.NewClient(hub.HubConfig{
		URL:       ts.URL,
		APIPrefix: "",
	})

	client.SubmitJob(&hub.JobSubmitRequest{
		Name:    "stats-job",
		Command: "echo stats",
		Workdir: ".",
	})

	stats, err := client.GetQueueStats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.Queued != 1 {
		t.Errorf("queued = %d, want 1", stats.Queued)
	}
}

func TestHubCompat_JobLogs(t *testing.T) {
	ts, store := setupIntegration(t)
	defer ts.Close()
	defer store.Close()

	client := hub.NewClient(hub.HubConfig{
		URL:       ts.URL,
		APIPrefix: "",
	})

	submitResp, _ := client.SubmitJob(&hub.JobSubmitRequest{
		Name:    "logs-job",
		Command: "echo hello",
		Workdir: ".",
	})

	// Logs endpoint exists even when no log file yet (returns empty)
	logs, err := client.GetJobLogs(submitResp.JobID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if logs.JobID != submitResp.JobID {
		t.Errorf("log job_id = %s, want %s", logs.JobID, submitResp.JobID)
	}
}

func TestHubCompat_JobSummary(t *testing.T) {
	ts, store := setupIntegration(t)
	defer ts.Close()
	defer store.Close()

	client := hub.NewClient(hub.HubConfig{
		URL:       ts.URL,
		APIPrefix: "",
	})

	submitResp, _ := client.SubmitJob(&hub.JobSubmitRequest{
		Name:    "summary-job",
		Command: "echo summary",
		Workdir: ".",
	})

	summary, err := client.GetJobSummary(submitResp.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.JobID != submitResp.JobID {
		t.Errorf("summary job_id = %s, want %s", summary.JobID, submitResp.JobID)
	}
	if summary.Name != "summary-job" {
		t.Errorf("name = %s, want summary-job", summary.Name)
	}
}

func TestHubCompat_RetryJob(t *testing.T) {
	ts, store := setupIntegration(t)
	defer ts.Close()
	defer store.Close()

	client := hub.NewClient(hub.HubConfig{
		URL:       ts.URL,
		APIPrefix: "",
	})

	submitResp, _ := client.SubmitJob(&hub.JobSubmitRequest{
		Name:    "retry-hub",
		Command: "echo retry",
		Workdir: ".",
	})

	// Make it terminal first
	store.StartJob(submitResp.JobID, 999, nil)
	store.CompleteJob(submitResp.JobID, daemon.StatusFailed, 1)

	retryResp, err := client.RetryJob(submitResp.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if retryResp.NewJobID == "" {
		t.Error("new_job_id should not be empty")
	}
	if retryResp.OriginalJobID != submitResp.JobID {
		t.Errorf("original = %s, want %s", retryResp.OriginalJobID, submitResp.JobID)
	}
}

func TestHubCompat_JobEstimate(t *testing.T) {
	ts, store := setupIntegration(t)
	defer ts.Close()
	defer store.Close()

	client := hub.NewClient(hub.HubConfig{
		URL:       ts.URL,
		APIPrefix: "",
	})

	submitResp, _ := client.SubmitJob(&hub.JobSubmitRequest{
		Name:    "estimate-hub",
		Command: "echo est",
		Workdir: ".",
	})

	est, err := client.GetJobEstimate(submitResp.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if est.Method != "default" {
		t.Errorf("method = %s, want default", est.Method)
	}
	if est.EstimatedDurationSec != 300 {
		t.Errorf("duration = %f, want 300", est.EstimatedDurationSec)
	}
}

func TestHubCompat_CompleteJob(t *testing.T) {
	ts, store := setupIntegration(t)
	defer ts.Close()
	defer store.Close()

	client := hub.NewClient(hub.HubConfig{
		URL:       ts.URL,
		APIPrefix: "",
	})

	submitResp, _ := client.SubmitJob(&hub.JobSubmitRequest{
		Name:    "complete-hub",
		Command: "echo done",
		Workdir: ".",
	})

	// Start via store
	store.StartJob(submitResp.JobID, 1234, nil)

	// Complete via hub client
	err := client.CompleteJob(submitResp.JobID, "SUCCEEDED", 0)
	if err != nil {
		t.Fatal(err)
	}

	// Verify
	job, _ := client.GetJob(submitResp.JobID)
	if job.Status != "SUCCEEDED" {
		t.Errorf("status = %s, want SUCCEEDED", job.Status)
	}
}
