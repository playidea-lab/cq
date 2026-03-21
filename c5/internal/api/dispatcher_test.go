package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/piqsol/c4/c5/internal/model"
	"github.com/piqsol/c4/c5/internal/store"
)

func newTestStoreForDispatch(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	st, err := store.New(filepath.Join(dir, "dispatch_test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// TestPushDispatch_Success verifies TryPushDispatch delivers the job to an MCP worker.
func TestPushDispatch_Success(t *testing.T) {
	st := newTestStoreForDispatch(t)

	// Start a fake MCP server that accepts the push.
	// We decode into a generic map since mcpRequest.Params is json.RawMessage.
	var received struct {
		JSONRPC string         `json:"jsonrpc"`
		ID      any            `json:"id"`
		Method  string         `json:"method"`
		Params  pushMCPCallParams `json:"params"`
	}
	fakeMCP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer fakeMCP.Close()

	// Register a push-capable worker pointing at the fake MCP server.
	_, err := st.RegisterWorker(&model.WorkerRegisterRequest{
		Hostname: "dispatch-worker",
		MCPURL:   fakeMCP.URL,
	})
	if err != nil {
		t.Fatalf("RegisterWorker: %v", err)
	}

	// Create a queued job.
	job, err := st.CreateJob(&model.JobSubmitRequest{
		Name:    "dispatch-job",
		Command: "echo dispatched",
		Workdir: "/tmp",
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	d := newPushDispatcher(st)
	if err := d.TryPushDispatch(job); err != nil {
		t.Fatalf("TryPushDispatch: %v", err)
	}

	// Verify the JSON-RPC payload reached the fake MCP server.
	if received.Method != "tools/call" {
		t.Fatalf("expected method tools/call, got %q", received.Method)
	}
	if received.Params.Name != "hub_dispatch_job" {
		t.Fatalf("expected tool hub_dispatch_job, got %q", received.Params.Name)
	}
	if received.Params.Arguments["job_id"] != job.ID {
		t.Fatalf("expected job_id %s, got %v", job.ID, received.Params.Arguments["job_id"])
	}

	// Job should now be RUNNING.
	got, err := st.GetJob(job.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.Status != model.StatusRunning {
		t.Fatalf("expected RUNNING after push, got %s", got.Status)
	}
}

// TestPushDispatch_Fallback verifies that when no mcp_url worker exists, TryPushDispatch
// returns nil (fall back to pull) and the job remains QUEUED.
func TestPushDispatch_Fallback(t *testing.T) {
	st := newTestStoreForDispatch(t)

	// Register a worker without mcp_url.
	_, err := st.RegisterWorker(&model.WorkerRegisterRequest{
		Hostname: "plain-worker",
		MCPURL:   "",
	})
	if err != nil {
		t.Fatalf("RegisterWorker: %v", err)
	}

	job, err := st.CreateJob(&model.JobSubmitRequest{
		Name:    "fallback-job",
		Command: "echo fallback",
		Workdir: "/tmp",
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	d := newPushDispatcher(st)
	if err := d.TryPushDispatch(job); err != nil {
		t.Fatalf("TryPushDispatch: unexpected error: %v", err)
	}

	// Job should remain QUEUED (no push-capable worker found).
	got, err := st.GetJob(job.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.Status != model.StatusQueued {
		t.Fatalf("expected QUEUED (fallback), got %s", got.Status)
	}
}

// TestPushDispatch_WorkerError verifies that when the MCP POST fails, the job is requeued.
func TestPushDispatch_WorkerError(t *testing.T) {
	st := newTestStoreForDispatch(t)

	// Fake MCP server that returns an error.
	fakeMCP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer fakeMCP.Close()

	_, err := st.RegisterWorker(&model.WorkerRegisterRequest{
		Hostname: "failing-worker",
		MCPURL:   fakeMCP.URL,
	})
	if err != nil {
		t.Fatalf("RegisterWorker: %v", err)
	}

	job, err := st.CreateJob(&model.JobSubmitRequest{
		Name:    "error-job",
		Command: "echo error",
		Workdir: "/tmp",
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	d := newPushDispatcher(st)
	// TryPushDispatch should return an error (push failed).
	if err := d.TryPushDispatch(job); err == nil {
		t.Fatal("expected error from failing worker, got nil")
	}

	// Job should be requeued to QUEUED after push failure.
	got, err := st.GetJob(job.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.Status != model.StatusQueued {
		t.Fatalf("expected QUEUED after push failure rollback, got %s", got.Status)
	}
}
