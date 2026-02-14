package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func tempServer(t *testing.T) (*Server, *Store) {
	t.Helper()
	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	sched := NewScheduler(store, SchedulerConfig{
		DataDir:       dir,
		MaxConcurrent: 4,
		PollInterval:  50 * time.Millisecond,
	})

	srv := NewServer(ServerConfig{
		Store:     store,
		Scheduler: sched,
		Version:   "test",
	})
	return srv, store
}

func doRequest(srv *Server, method, path string, body string) *httptest.ResponseRecorder {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

func decodeJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode JSON: %v\nbody: %s", err, w.Body.String())
	}
	return result
}

func TestServer_Health(t *testing.T) {
	srv, _ := tempServer(t)

	w := doRequest(srv, "GET", "/health", "")
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}

	result := decodeJSON(t, w)
	if result["status"] != "ok" {
		t.Errorf("status = %v", result["status"])
	}
	if result["version"] != "test" {
		t.Errorf("version = %v", result["version"])
	}
}

func TestServer_SubmitJob(t *testing.T) {
	srv, _ := tempServer(t)

	body := `{"name":"train","command":"echo hi","workdir":"/tmp"}`
	w := doRequest(srv, "POST", "/jobs/submit", body)
	if w.Code != 201 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	result := decodeJSON(t, w)
	if result["job_id"] == nil || result["job_id"] == "" {
		t.Error("expected job_id")
	}
	if result["status"] != "QUEUED" {
		t.Errorf("status = %v", result["status"])
	}
}

func TestServer_SubmitJob_NoCommand(t *testing.T) {
	srv, _ := tempServer(t)

	body := `{"name":"bad"}`
	w := doRequest(srv, "POST", "/jobs/submit", body)
	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestServer_SubmitJob_Defaults(t *testing.T) {
	srv, store := tempServer(t)

	body := `{"command":"echo hi"}`
	w := doRequest(srv, "POST", "/jobs/submit", body)
	if w.Code != 201 {
		t.Fatalf("status = %d", w.Code)
	}

	result := decodeJSON(t, w)
	jobID := result["job_id"].(string)

	job, _ := store.GetJob(jobID)
	if job.Name != "untitled" {
		t.Errorf("name = %s, want untitled", job.Name)
	}
	if job.Workdir != "." {
		t.Errorf("workdir = %s, want .", job.Workdir)
	}
}

func TestServer_ListJobs(t *testing.T) {
	srv, store := tempServer(t)

	store.CreateJob(&JobSubmitRequest{Name: "j1", Command: "echo 1", Workdir: "."})
	store.CreateJob(&JobSubmitRequest{Name: "j2", Command: "echo 2", Workdir: "."})

	w := doRequest(srv, "GET", "/jobs", "")
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}

	result := decodeJSON(t, w)
	jobs := result["jobs"].([]any)
	if len(jobs) != 2 {
		t.Errorf("len(jobs) = %d, want 2", len(jobs))
	}
}

func TestServer_ListJobs_FilterStatus(t *testing.T) {
	srv, store := tempServer(t)

	store.CreateJob(&JobSubmitRequest{Name: "j1", Command: "echo 1", Workdir: "."})
	j2, _ := store.CreateJob(&JobSubmitRequest{Name: "j2", Command: "echo 2", Workdir: "."})
	store.StartJob(j2.ID, 1, nil)

	w := doRequest(srv, "GET", "/jobs?status=RUNNING", "")
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}

	result := decodeJSON(t, w)
	jobs := result["jobs"].([]any)
	if len(jobs) != 1 {
		t.Errorf("len(jobs) = %d, want 1", len(jobs))
	}
}

func TestServer_GetJob(t *testing.T) {
	srv, store := tempServer(t)

	job, _ := store.CreateJob(&JobSubmitRequest{Name: "test", Command: "echo hi", Workdir: "."})

	w := doRequest(srv, "GET", "/jobs/"+job.ID, "")
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}

	result := decodeJSON(t, w)
	if result["job_id"] != job.ID {
		t.Errorf("job_id = %v, want %s", result["job_id"], job.ID)
	}
}

func TestServer_GetJob_NotFound(t *testing.T) {
	srv, _ := tempServer(t)

	w := doRequest(srv, "GET", "/jobs/nonexistent", "")
	if w.Code != 404 {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestServer_CancelJob(t *testing.T) {
	srv, store := tempServer(t)

	job, _ := store.CreateJob(&JobSubmitRequest{Name: "cancel", Command: "echo hi", Workdir: "."})

	w := doRequest(srv, "POST", "/jobs/"+job.ID+"/cancel", "")
	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	result := decodeJSON(t, w)
	if result["status"] != "CANCELLED" {
		t.Errorf("status = %v", result["status"])
	}

	got, _ := store.GetJob(job.ID)
	if got.Status != StatusCancelled {
		t.Errorf("stored status = %s", got.Status)
	}
}

func TestServer_QueueStats(t *testing.T) {
	srv, store := tempServer(t)

	store.CreateJob(&JobSubmitRequest{Name: "q1", Command: "echo 1", Workdir: "."})
	store.CreateJob(&JobSubmitRequest{Name: "q2", Command: "echo 2", Workdir: "."})

	w := doRequest(srv, "GET", "/stats/queue", "")
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}

	var stats QueueStats
	json.Unmarshal(w.Body.Bytes(), &stats)
	if stats.Queued != 2 {
		t.Errorf("queued = %d, want 2", stats.Queued)
	}
}

func TestServer_GPUStatus_Unavailable(t *testing.T) {
	srv, _ := tempServer(t)
	// gpuMonitor is nil → unavailable

	w := doRequest(srv, "GET", "/gpu/status", "")
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}

	result := decodeJSON(t, w)
	if result["available"] != false {
		t.Errorf("available = %v, want false", result["available"])
	}
}

func TestServer_GPUStatus_WithMock(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "test.db"))
	defer store.Close()

	sched := NewScheduler(store, SchedulerConfig{
		DataDir:       dir,
		MaxConcurrent: 4,
		PollInterval:  50 * time.Millisecond,
	})

	gpu := mockGpuMonitor(mockNvidiaSmiOutput)

	srv := NewServer(ServerConfig{
		Store:      store,
		Scheduler:  sched,
		GpuMonitor: gpu,
		Version:    "test",
	})

	w := doRequest(srv, "GET", "/gpu/status", "")
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}

	result := decodeJSON(t, w)
	if result["available"] != true {
		t.Errorf("available = %v, want true", result["available"])
	}
	count := result["count"].(float64)
	if int(count) != 2 {
		t.Errorf("count = %v, want 2", count)
	}
}

func TestServer_DaemonStop(t *testing.T) {
	srv, _ := tempServer(t)

	var stopped atomic.Bool
	srv.cancelFunc = func() { stopped.Store(true) }

	w := doRequest(srv, "POST", "/daemon/stop", "")
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}

	// Wait for the async cancel
	time.Sleep(200 * time.Millisecond)
	if !stopped.Load() {
		t.Error("expected cancelFunc to be called")
	}
}

func TestServer_Retry(t *testing.T) {
	srv, store := tempServer(t)

	job, _ := store.CreateJob(&JobSubmitRequest{Name: "retry-me", Command: "echo hi", Workdir: "/tmp"})
	store.StartJob(job.ID, 1, nil)
	store.CompleteJob(job.ID, StatusFailed, 1)

	w := doRequest(srv, "POST", "/jobs/"+job.ID+"/retry", "")
	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	result := decodeJSON(t, w)
	if result["original_job_id"] != job.ID {
		t.Errorf("original_job_id = %v", result["original_job_id"])
	}
	if result["status"] != "QUEUED" {
		t.Errorf("status = %v", result["status"])
	}
	newID := result["new_job_id"].(string)
	if newID == "" || newID == job.ID {
		t.Error("expected different new job ID")
	}
}

func TestServer_Retry_NotTerminal(t *testing.T) {
	srv, store := tempServer(t)

	job, _ := store.CreateJob(&JobSubmitRequest{Name: "running", Command: "echo hi", Workdir: "/tmp"})
	store.StartJob(job.ID, 1, nil)

	w := doRequest(srv, "POST", "/jobs/"+job.ID+"/retry", "")
	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestServer_Estimate(t *testing.T) {
	srv, store := tempServer(t)

	job, _ := store.CreateJob(&JobSubmitRequest{Name: "est", Command: "echo hi", Workdir: "/tmp"})

	w := doRequest(srv, "GET", "/jobs/"+job.ID+"/estimate", "")
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}

	result := decodeJSON(t, w)
	if result["method"] != "default" {
		t.Errorf("method = %v, want default (no history)", result["method"])
	}
}

func TestServer_Summary(t *testing.T) {
	srv, store := tempServer(t)

	job, _ := store.CreateJob(&JobSubmitRequest{Name: "sum", Command: "echo hi", Workdir: "/tmp"})
	store.StartJob(job.ID, 1, nil)
	store.CompleteJob(job.ID, StatusSucceeded, 0)

	w := doRequest(srv, "GET", "/jobs/"+job.ID+"/summary", "")
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}

	result := decodeJSON(t, w)
	if result["name"] != "sum" {
		t.Errorf("name = %v", result["name"])
	}
	if result["status"] != "SUCCEEDED" {
		t.Errorf("status = %v", result["status"])
	}
}

func TestServer_MethodNotAllowed(t *testing.T) {
	srv, _ := tempServer(t)

	w := doRequest(srv, "DELETE", "/health", "")
	if w.Code != 405 {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestServer_JobLogs_Empty(t *testing.T) {
	srv, store := tempServer(t)

	job, _ := store.CreateJob(&JobSubmitRequest{Name: "nolog", Command: "echo hi", Workdir: "."})

	w := doRequest(srv, "GET", "/jobs/"+job.ID+"/logs", "")
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}

	result := decodeJSON(t, w)
	if result["job_id"] != job.ID {
		t.Errorf("job_id = %v", result["job_id"])
	}
}
