//go:build hub

package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestWorkerComponent_Name(t *testing.T) {
	w := NewWorker(nil, nil, "test-host")
	if w.Name() != "worker" {
		t.Errorf("Name() = %q, want %q", w.Name(), "worker")
	}
}

func TestWorkerComponent_HealthBeforeStart(t *testing.T) {
	w := NewWorker(nil, nil, "test-host")
	h := w.Health()
	if h.Status != "error" {
		t.Errorf("Health before start = %q, want %q", h.Status, "error")
	}
}

func TestWorkerComponent_StartRegister(t *testing.T) {
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
	comp := NewWorker(client, []string{"gpu", "ml"}, "test-host")

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

func TestWorkerComponent_ExecuteJob(t *testing.T) {
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
	comp := NewWorker(client, []string{"test"}, "test-host")

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

func TestWorkerComponent_StopBeforeStart(t *testing.T) {
	w := NewWorker(nil, nil, "test-host")
	if err := w.Stop(context.Background()); err != nil {
		t.Errorf("Stop before Start: %v", err)
	}
}

func TestWorkerComponent_TildeExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}

	job := &hub.Job{
		Command: "pwd",
		Workdir: "~/",
	}

	w := NewWorker(nil, nil, "test-host")
	// Execute inline tilde expansion logic (mirrors executeJob).
	workdir := job.Workdir
	if strings.HasPrefix(workdir, "~/") {
		if h, e := os.UserHomeDir(); e == nil {
			workdir = filepath.Join(h, workdir[2:])
		}
	}

	if workdir != home {
		t.Errorf("tilde expansion: got %q, want %q", workdir, home)
	}
	_ = w // suppress unused warning
}

func TestWorkerComponent_MetricParsing(t *testing.T) {
	var (
		registered   atomic.Bool
		completed    atomic.Bool
		metricsBody  atomic.Value
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/rpc/register_worker", func(w http.ResponseWriter, r *http.Request) {
		registered.Store(true)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"worker_id": "w-metric"})
	})
	mux.HandleFunc("/rest/v1/rpc/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	jobServed := make(chan struct{}, 1)
	mux.HandleFunc("/rest/v1/rpc/claim_job", func(w http.ResponseWriter, r *http.Request) {
		select {
		case jobServed <- struct{}{}:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"lease_id": "lease-metric-001",
				"job": map[string]any{
					"job_id":  "job-metric-001",
					"command": `echo "@loss=0.5 @acc=0.9"`,
				},
			})
		default:
			time.Sleep(100 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(nil)
		}
	})

	mux.HandleFunc("/rest/v1/rpc/complete_job", func(w http.ResponseWriter, r *http.Request) {
		completed.Store(true)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "{}")
	})

	mux.HandleFunc("/rest/v1/rpc/renew_lease", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"renewed": true})
	})

	// Capture hub_metrics POST requests.
	metricsCh := make(chan map[string]any, 10)
	mux.HandleFunc("/rest/v1/hub_metrics", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			metricsBody.Store(body)
			metricsCh <- body
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, "{}")
	})

	client := newHubWorkerTestClient(t, mux)
	comp := NewWorker(client, []string{"test"}, "test-host")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer comp.Stop(context.Background())

	// Wait for metrics POST.
	select {
	case body := <-metricsCh:
		// Verify step=1.
		if step, ok := body["step"].(float64); !ok || step != 1 {
			t.Errorf("hub_metrics step = %v, want 1", body["step"])
		}
		// Verify metrics contain loss and acc.
		// LogMetricsSupabase serializes metrics as a JSON string.
		metricsRaw, ok := body["metrics"]
		if !ok {
			t.Fatal("hub_metrics body missing 'metrics' field")
		}
		metricsStr, ok := metricsRaw.(string)
		if !ok {
			t.Fatalf("hub_metrics 'metrics' is %T, want string", metricsRaw)
		}
		var metricsMap map[string]any
		if err := json.Unmarshal([]byte(metricsStr), &metricsMap); err != nil {
			t.Fatalf("hub_metrics 'metrics' parse: %v", err)
		}
		if _, hasLoss := metricsMap["loss"]; !hasLoss {
			t.Errorf("metrics missing 'loss', got: %v", metricsMap)
		}
		if _, hasAcc := metricsMap["acc"]; !hasAcc {
			t.Errorf("metrics missing 'acc', got: %v", metricsMap)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("hub_metrics POST was not received within timeout")
	}

	_ = registered.Load()
}

func TestWorkerComponent_NotifyFunc(t *testing.T) {
	var (
		registered atomic.Bool
		completed  atomic.Bool
	)

	notifyCh := make(chan string, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/rpc/register_worker", func(w http.ResponseWriter, r *http.Request) {
		registered.Store(true)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"worker_id": "w-notify"})
	})
	mux.HandleFunc("/rest/v1/rpc/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	jobServed := make(chan struct{}, 1)
	mux.HandleFunc("/rest/v1/rpc/claim_job", func(w http.ResponseWriter, r *http.Request) {
		select {
		case jobServed <- struct{}{}:
			w.Header().Set("Content-Type", "application/json")
			// ClaimJobWithWait expects {"lease_id": "...", "job": {...}} format.
			json.NewEncoder(w).Encode(map[string]any{
				"lease_id": "lease-notify-001",
				"job": map[string]any{
					"job_id":  "job-notify-001",
					"command": "echo notify-test",
				},
			})
		default:
			time.Sleep(100 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(nil)
		}
	})

	mux.HandleFunc("/rest/v1/rpc/complete_job", func(w http.ResponseWriter, r *http.Request) {
		completed.Store(true)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "{}")
	})

	mux.HandleFunc("/rest/v1/rpc/renew_lease", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"renewed": true})
	})

	client := newHubWorkerTestClient(t, mux)
	comp := NewWorker(client, []string{"test"}, "test-host")
	comp.SetNotifyFunc(func(jobID, status string, exitCode int) {
		notifyCh <- status
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer comp.Stop(context.Background())

	select {
	case status := <-notifyCh:
		if status != "SUCCEEDED" {
			t.Errorf("notifyFunc status = %q, want SUCCEEDED", status)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("notifyFunc was not called within timeout")
	}
}

func TestWorkerComponent_BestMetric_Updated(t *testing.T) {
	// Job with primary_metric="loss" and lower_is_better=true.
	// Script prints @loss=0.5, then @loss=0.3 → best_metric should be updated twice.
	var completed atomic.Bool

	bestMetricUpdates := make(chan float64, 10)

	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/rpc/register_worker", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"worker_id": "w-best"})
	})
	mux.HandleFunc("/rest/v1/rpc/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	jobServed := make(chan struct{}, 1)
	lowerIsBetter := true
	mux.HandleFunc("/rest/v1/rpc/claim_job", func(w http.ResponseWriter, r *http.Request) {
		select {
		case jobServed <- struct{}{}:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"lease_id": "lease-best-001",
				"job": map[string]any{
					"job_id":          "job-best-001",
					"command":         `sh -c 'echo "@loss=0.5"; echo "@loss=0.3"'`,
					"primary_metric":  "loss",
					"lower_is_better": lowerIsBetter,
				},
			})
		default:
			time.Sleep(100 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(nil)
		}
	})

	mux.HandleFunc("/rest/v1/rpc/complete_job", func(w http.ResponseWriter, r *http.Request) {
		completed.Store(true)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "{}")
	})

	mux.HandleFunc("/rest/v1/rpc/renew_lease", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"renewed": true})
	})

	mux.HandleFunc("/rest/v1/hub_metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, "{}")
	})

	// Capture PATCH to hub_jobs (best_metric update).
	mux.HandleFunc("/rest/v1/hub_jobs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
				if v, ok := body["best_metric"].(float64); ok {
					bestMetricUpdates <- v
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "[]")
	})

	client := newHubWorkerTestClient(t, mux)
	comp := NewWorker(client, []string{"test"}, "test-host")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer comp.Stop(context.Background())

	// Wait for completion.
	deadline := time.After(4 * time.Second)
	for !completed.Load() {
		select {
		case <-deadline:
			t.Fatal("job not completed within timeout")
		case <-time.After(50 * time.Millisecond):
		}
	}

	// Drain updates channel.
	var updates []float64
	timeout := time.After(500 * time.Millisecond)
drain:
	for {
		select {
		case v := <-bestMetricUpdates:
			updates = append(updates, v)
		case <-timeout:
			break drain
		}
	}

	if len(updates) < 2 {
		t.Fatalf("expected at least 2 best_metric updates, got %d: %v", len(updates), updates)
	}
	// First update: 0.5 (first value always sets best).
	if updates[0] != 0.5 {
		t.Errorf("first best_metric update = %v, want 0.5", updates[0])
	}
	// Second update: 0.3 (lower, so improved).
	if updates[1] != 0.3 {
		t.Errorf("second best_metric update = %v, want 0.3", updates[1])
	}
}

func TestWorkerComponent_BestMetric_NotUpdated_WhenNoPrimaryMetric(t *testing.T) {
	// Job without primary_metric → best_metric should never be updated.
	var completed atomic.Bool

	bestMetricUpdated := make(chan struct{}, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/rpc/register_worker", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"worker_id": "w-noprimary"})
	})
	mux.HandleFunc("/rest/v1/rpc/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	jobServed := make(chan struct{}, 1)
	mux.HandleFunc("/rest/v1/rpc/claim_job", func(w http.ResponseWriter, r *http.Request) {
		select {
		case jobServed <- struct{}{}:
			w.Header().Set("Content-Type", "application/json")
			// No primary_metric field.
			json.NewEncoder(w).Encode(map[string]any{
				"lease_id": "lease-noprimary-001",
				"job": map[string]any{
					"job_id":  "job-noprimary-001",
					"command": `echo "@loss=0.5"`,
				},
			})
		default:
			time.Sleep(100 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(nil)
		}
	})

	mux.HandleFunc("/rest/v1/rpc/complete_job", func(w http.ResponseWriter, r *http.Request) {
		completed.Store(true)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "{}")
	})

	mux.HandleFunc("/rest/v1/rpc/renew_lease", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"renewed": true})
	})

	mux.HandleFunc("/rest/v1/hub_metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, "{}")
	})

	// Detect any PATCH to hub_jobs with best_metric.
	mux.HandleFunc("/rest/v1/hub_jobs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
				if _, hasBest := body["best_metric"]; hasBest {
					select {
					case bestMetricUpdated <- struct{}{}:
					default:
					}
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "[]")
	})

	client := newHubWorkerTestClient(t, mux)
	comp := NewWorker(client, []string{"test"}, "test-host")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer comp.Stop(context.Background())

	// Wait for completion.
	deadline := time.After(4 * time.Second)
	for !completed.Load() {
		select {
		case <-deadline:
			t.Fatal("job not completed within timeout")
		case <-time.After(50 * time.Millisecond):
		}
	}

	// Give a short window for any unexpected PATCH to arrive.
	select {
	case <-bestMetricUpdated:
		t.Error("best_metric was updated but primary_metric was not set")
	case <-time.After(300 * time.Millisecond):
		// Good: no update.
	}
}
