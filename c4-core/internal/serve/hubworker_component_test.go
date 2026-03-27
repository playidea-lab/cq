//go:build hub

package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/hub"
)

// newHubWorkerTestClient creates a hub.Client pointing at a test server.
func newHubWorkerTestClient(t *testing.T, mux *http.ServeMux) *hub.Client {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return hub.NewClient(hub.HubConfig{
		SupabaseURL: srv.URL,
		SupabaseKey: "test-key",
		TeamID:      "test-project",
	})
}

func TestHubWorkerComponent_Name(t *testing.T) {
	w := NewHubWorker(nil, nil, "test-host")
	if w.Name() != "hub_worker" {
		t.Errorf("Name() = %q, want %q", w.Name(), "hub_worker")
	}
}

func TestHubWorkerComponent_HealthBeforeStart(t *testing.T) {
	w := NewHubWorker(nil, nil, "test-host")
	h := w.Health()
	if h.Status != "error" {
		t.Errorf("Health before start = %q, want %q", h.Status, "error")
	}
}

func TestHubWorkerComponent_StartRegister(t *testing.T) {
	var registered atomic.Bool

	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/rpc/register_worker", func(w http.ResponseWriter, r *http.Request) {
		registered.Store(true)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"worker_id": "w-123"})
	})
	mux.HandleFunc("/rest/v1/rpc/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/rest/v1/rpc/claim_job", func(w http.ResponseWriter, r *http.Request) {
		// No job available — return empty.
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(nil)
	})

	client := newHubWorkerTestClient(t, mux)
	comp := NewHubWorker(client, []string{"gpu", "ml"}, "test-host")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer comp.Stop(context.Background())

	if !registered.Load() {
		t.Error("expected RegisterWorker to be called")
	}

	h := comp.Health()
	if h.Status != "ok" {
		t.Errorf("Health after start = %q, want %q", h.Status, "ok")
	}
}

func TestHubWorkerComponent_ExecuteJob(t *testing.T) {
	var (
		registered atomic.Bool
		completed  atomic.Bool
		jobStatus  atomic.Value
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/rpc/register_worker", func(w http.ResponseWriter, r *http.Request) {
		registered.Store(true)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"worker_id": "w-456"})
	})
	mux.HandleFunc("/rest/v1/rpc/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	jobServed := make(chan struct{}, 1)
	mux.HandleFunc("/rest/v1/rpc/claim_job", func(w http.ResponseWriter, r *http.Request) {
		select {
		case jobServed <- struct{}{}:
			// First call: return a job.
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"job_id":   "job-001",
				"command":  "echo hello-from-hub-worker",
				"lease_id": "lease-001",
			})
		default:
			// Subsequent calls: no more jobs — block briefly.
			time.Sleep(100 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(nil)
		}
	})

	mux.HandleFunc("/rest/v1/rpc/complete_job", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		completed.Store(true)
		if s, ok := body["p_status"].(string); ok {
			jobStatus.Store(s)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "{}")
	})

	mux.HandleFunc("/rest/v1/rpc/renew_lease", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"renewed": true})
	})

	client := newHubWorkerTestClient(t, mux)
	comp := NewHubWorker(client, []string{"test"}, "test-host")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for job completion.
	deadline := time.After(3 * time.Second)
	for {
		if completed.Load() {
			break
		}
		select {
		case <-deadline:
			t.Fatal("job was not completed within timeout")
		case <-time.After(50 * time.Millisecond):
		}
	}

	comp.Stop(context.Background())

	if s, ok := jobStatus.Load().(string); !ok || s != "SUCCEEDED" {
		t.Errorf("job status = %v, want SUCCEEDED", jobStatus.Load())
	}
}

func TestHubWorkerComponent_StopBeforeStart(t *testing.T) {
	w := NewHubWorker(nil, nil, "test-host")
	if err := w.Stop(context.Background()); err != nil {
		t.Errorf("Stop before Start: %v", err)
	}
}
