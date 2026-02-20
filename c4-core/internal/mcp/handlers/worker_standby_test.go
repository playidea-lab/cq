package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/changmin/c4-core/internal/cloud"
	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/worker"
	_ "modernc.org/sqlite"
)

// testWorkerDeps creates WorkerDeps for testing with a mock Hub server.
func testWorkerDeps(t *testing.T, hubHandler http.HandlerFunc, keeperHandler http.HandlerFunc) (*WorkerDeps, *httptest.Server) {
	t.Helper()

	// Mock Hub server
	hubServer := httptest.NewServer(hubHandler)
	t.Cleanup(hubServer.Close)

	hubClient := hub.NewClient(hub.HubConfig{
		Enabled: true,
		URL:     hubServer.URL,
	})

	// Shutdown store
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	shutdownStore, err := worker.NewShutdownStore(db)
	if err != nil {
		t.Fatal(err)
	}

	deps := &WorkerDeps{
		HubClient:     hubClient,
		ShutdownStore: shutdownStore,
	}

	// Optionally add keeper
	if keeperHandler != nil {
		keeperServer := httptest.NewServer(keeperHandler)
		t.Cleanup(keeperServer.Close)
		c1 := NewC1Handler(keeperServer.URL, "test-key", cloud.NewStaticTokenProvider("test-token"), "proj-1")
		deps.Keeper = NewContextKeeper(c1, nil)
	}

	return deps, hubServer
}

func TestWorkerComplete_Success(t *testing.T) {
	var completeCalled bool
	hubHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/jobs/job-123/complete" {
			completeCalled = true
			json.NewEncoder(w).Encode(map[string]any{"status": "SUCCEEDED"})
			return
		}
		w.WriteHeader(404)
	})

	deps, _ := testWorkerDeps(t, hubHandler, nil)
	reg := mcp.NewRegistry()
	RegisterWorkerHandlers(reg, deps)

	result, err := reg.Call("c4_worker_complete", json.RawMessage(`{
		"job_id": "job-123",
		"worker_id": "w1",
		"status": "SUCCEEDED",
		"commit_sha": "abc123"
	}`))
	if err != nil {
		t.Fatal(err)
	}

	if !completeCalled {
		t.Error("expected Hub CompleteJob to be called")
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map", result)
	}
	if m["status"] != "completed" {
		t.Errorf("status = %v, want completed", m["status"])
	}
}

func TestWorkerComplete_MissingRequired(t *testing.T) {
	deps, _ := testWorkerDeps(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), nil)
	reg := mcp.NewRegistry()
	RegisterWorkerHandlers(reg, deps)

	_, err := reg.Call("c4_worker_complete", json.RawMessage(`{"job_id": "j1"}`))
	if err == nil {
		t.Fatal("expected error for missing required fields")
	}
}

func TestWorkerShutdown_StoresSignal(t *testing.T) {
	deps, _ := testWorkerDeps(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), nil)
	reg := mcp.NewRegistry()
	RegisterWorkerHandlers(reg, deps)

	result, err := reg.Call("c4_worker_shutdown", json.RawMessage(`{
		"worker_id": "w1",
		"reason": "maintenance"
	}`))
	if err != nil {
		t.Fatal(err)
	}

	m := result.(map[string]any)
	if m["status"] != "signal_stored" {
		t.Errorf("status = %v, want signal_stored", m["status"])
	}

	// Verify signal can be consumed
	reason, ok := deps.ShutdownStore.ConsumeSignal("w1")
	if !ok || reason != "maintenance" {
		t.Errorf("signal = (%q, %v), want (maintenance, true)", reason, ok)
	}
}

func TestWorkerShutdown_DefaultReason(t *testing.T) {
	deps, _ := testWorkerDeps(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), nil)
	reg := mcp.NewRegistry()
	RegisterWorkerHandlers(reg, deps)

	result, err := reg.Call("c4_worker_shutdown", json.RawMessage(`{"worker_id": "w2"}`))
	if err != nil {
		t.Fatal(err)
	}

	m := result.(map[string]any)
	if m["reason"] != "shutdown requested" {
		t.Errorf("reason = %v, want 'shutdown requested'", m["reason"])
	}
}

func TestWorkerStandby_ShutdownSignal(t *testing.T) {
	// Register worker succeeds, then shutdown signal is pre-stored
	hubHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/workers/register" {
			json.NewEncoder(w).Encode(map[string]any{"worker_id": "hub-w1"})
			return
		}
		w.WriteHeader(404)
	})

	deps, _ := testWorkerDeps(t, hubHandler, nil)

	// Pre-store a shutdown signal so the loop exits immediately
	deps.ShutdownStore.StoreSignal("w1", "test exit")

	result, err := handleWorkerStandby(context.Background(), deps, json.RawMessage(`{"worker_id": "w1"}`))
	if err != nil {
		t.Fatal(err)
	}

	m := result.(map[string]any)
	if m["shutdown"] != true {
		t.Errorf("expected shutdown=true, got %v", m["shutdown"])
	}
	if m["reason"] != "test exit" {
		t.Errorf("reason = %v, want 'test exit'", m["reason"])
	}
}

func TestWorkerStandby_ClaimsJob(t *testing.T) {
	registerCalled := false
	hubHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/workers/register" {
			registerCalled = true
			json.NewEncoder(w).Encode(map[string]any{"worker_id": "hub-w1"})
			return
		}
		if r.URL.Path == "/leases/acquire" {
			json.NewEncoder(w).Encode(map[string]any{
				"job_id":   "job-42",
				"lease_id": "lease-99",
				"job": map[string]any{
					"id":      "job-42",
					"command": "task_id=T-001-0",
					"name":    "test-job",
					"workdir": "/tmp",
				},
			})
			return
		}
		w.WriteHeader(404)
	})

	deps, _ := testWorkerDeps(t, hubHandler, nil)

	result, err := handleWorkerStandby(context.Background(), deps, json.RawMessage(`{"worker_id": "w1"}`))
	if err != nil {
		t.Fatal(err)
	}

	if !registerCalled {
		t.Error("expected RegisterWorker to be called")
	}

	m := result.(map[string]any)
	if m["job_id"] != "job-42" {
		t.Errorf("job_id = %v, want job-42", m["job_id"])
	}
	if m["command"] != "task_id=T-001-0" {
		t.Errorf("command = %v, want task_id=T-001-0", m["command"])
	}
	if m["lease_id"] != "lease-99" {
		t.Errorf("lease_id = %v, want lease-99", m["lease_id"])
	}
}

// Nil-deps boundary tests (CR-005)

func TestWorkerStandby_NilDeps_ReturnsError(t *testing.T) {
	_, err := handleWorkerStandby(context.Background(), nil, json.RawMessage(`{"worker_id": "w1"}`))
	if err == nil {
		t.Fatal("expected error for nil deps")
	}
	if err.Error() != "hub client not configured" {
		t.Errorf("err = %q, want %q", err.Error(), "hub client not configured")
	}
}

func TestWorkerStandby_NilHubClient_ReturnsError(t *testing.T) {
	deps := &WorkerDeps{HubClient: nil, ShutdownStore: nil}
	_, err := handleWorkerStandby(context.Background(), deps, json.RawMessage(`{"worker_id": "w1"}`))
	if err == nil {
		t.Fatal("expected error for nil HubClient")
	}
	if err.Error() != "hub client not configured" {
		t.Errorf("err = %q, want %q", err.Error(), "hub client not configured")
	}
}

func TestWorkerComplete_NilDeps_ReturnsError(t *testing.T) {
	_, err := handleWorkerComplete(nil, json.RawMessage(`{"job_id": "j1", "worker_id": "w1", "status": "SUCCEEDED"}`))
	if err == nil {
		t.Fatal("expected error for nil deps")
	}
	if err.Error() != "hub client not configured" {
		t.Errorf("err = %q, want %q", err.Error(), "hub client not configured")
	}
}

func TestWorkerShutdown_NilDeps_ReturnsError(t *testing.T) {
	_, err := handleWorkerShutdown(nil, json.RawMessage(`{"worker_id": "w1"}`))
	if err == nil {
		t.Fatal("expected error for nil deps")
	}
	if err.Error() != "shutdown store not configured" {
		t.Errorf("err = %q, want %q", err.Error(), "shutdown store not configured")
	}
}

func TestWorkerShutdown_NilShutdownStore_ReturnsError(t *testing.T) {
	deps := &WorkerDeps{ShutdownStore: nil}
	_, err := handleWorkerShutdown(deps, json.RawMessage(`{"worker_id": "w1"}`))
	if err == nil {
		t.Fatal("expected error for nil ShutdownStore")
	}
	if err.Error() != "shutdown store not configured" {
		t.Errorf("err = %q, want %q", err.Error(), "shutdown store not configured")
	}
}

// Status enum validation tests (CR-006)

func TestWorkerComplete_InvalidStatus_UNKNOWN(t *testing.T) {
	hubHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CompleteJob must NOT be called for invalid status
		if r.URL.Path == "/jobs/job-123/complete" {
			t.Error("CompleteJob should not be called for invalid status")
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(404)
	})
	deps, _ := testWorkerDeps(t, hubHandler, nil)
	reg := mcp.NewRegistry()
	RegisterWorkerHandlers(reg, deps)

	_, err := reg.Call("c4_worker_complete", json.RawMessage(`{
		"job_id": "job-123",
		"worker_id": "w1",
		"status": "UNKNOWN"
	}`))
	if err == nil {
		t.Fatal("expected error for status=UNKNOWN")
	}
}

func TestWorkerComplete_InvalidStatus_Empty(t *testing.T) {
	deps, _ := testWorkerDeps(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), nil)
	reg := mcp.NewRegistry()
	RegisterWorkerHandlers(reg, deps)

	_, err := reg.Call("c4_worker_complete", json.RawMessage(`{
		"job_id": "job-123",
		"worker_id": "w1",
		"status": ""
	}`))
	if err == nil {
		t.Fatal("expected error for empty status")
	}
}

func TestWorkerHandlers_Registration(t *testing.T) {
	deps, _ := testWorkerDeps(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), nil)
	reg := mcp.NewRegistry()
	RegisterWorkerHandlers(reg, deps)

	tools := reg.ListTools()
	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}

	expected := []string{"c4_worker_standby", "c4_worker_complete", "c4_worker_shutdown"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("tool %s not registered", name)
		}
	}
}
