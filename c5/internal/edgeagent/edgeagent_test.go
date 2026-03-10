package edgeagent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/piqsol/c4/c5/internal/model"
)

// TestConfigDefaults verifies Config struct initialisation with explicit field values.
func TestConfigDefaults(t *testing.T) {
	cfg := Config{
		HubURL:       "http://localhost:8585",
		APIKey:       "test-key",
		EdgeName:     "test-edge",
		Workdir:      t.TempDir(),
		PollInterval: 100 * time.Millisecond,
	}
	if cfg.HubURL == "" {
		t.Error("HubURL should not be empty")
	}
	if cfg.EdgeName == "" {
		t.Error("EdgeName should not be empty")
	}
	if cfg.PollInterval <= 0 {
		t.Error("PollInterval should be positive")
	}
}

// TestSleepOrDone_ContextCancel verifies sleepOrDone returns immediately when ctx is cancelled.
func TestSleepOrDone_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	start := time.Now()
	sleepOrDone(ctx, 10*time.Second) // should return immediately, not sleep 10s
	elapsed := time.Since(start)

	if elapsed > time.Second {
		t.Errorf("sleepOrDone took too long after cancel: %v", elapsed)
	}
}

// TestSleepOrDone_Timeout verifies sleepOrDone sleeps for approximately d when ctx is not cancelled.
func TestSleepOrDone_Timeout(t *testing.T) {
	ctx := context.Background()
	d := 50 * time.Millisecond

	start := time.Now()
	sleepOrDone(ctx, d)
	elapsed := time.Since(start)

	if elapsed < d {
		t.Errorf("sleepOrDone returned before duration: elapsed=%v, want>=%v", elapsed, d)
	}
}

// TestRun_ContextCancel verifies Run() exits cleanly when the context is cancelled
// after a successful registration against a fake Hub.
func TestRun_ContextCancel(t *testing.T) {
	// Fake Hub: handles register, assignments polling, and target-status.
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/edges/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		resp := model.EdgeRegisterResponse{EdgeID: "edge-test-001"}
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})

	mux.HandleFunc("/v1/edges/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/v1/deploy/assignments/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Return empty assignment list so processAssignment is not called.
		json.NewEncoder(w).Encode([]model.DeployAssignmentResponse{}) //nolint:errcheck
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())

	cfg := Config{
		HubURL:       srv.URL,
		APIKey:       "",
		EdgeName:     "test-edge",
		Workdir:      t.TempDir(),
		PollInterval: 50 * time.Millisecond,
	}

	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, cfg)
	}()

	// Let the agent run at least one poll cycle.
	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		// context.Canceled is the expected return value.
		if err != context.Canceled {
			t.Errorf("Run() returned unexpected error: %v (want context.Canceled)", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run() did not exit within timeout after context cancel")
	}
}

// TestMetricsReporter verifies MetricsCommand stdout is parsed and POST'd to Hub.
func TestMetricsReporter(t *testing.T) {
	var received []byte
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/metrics") {
			received, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer hub.Close()

	mr := newMetricsReporter("edge-1", hub.URL, "", "echo accuracy=0.91", 50*time.Millisecond, &http.Client{Timeout: 5 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	mr.Start(ctx)

	if !strings.Contains(string(received), "accuracy") {
		t.Errorf("expected metrics POST to contain 'accuracy', got: %s", string(received))
	}
}

// TestControlPollerCollect verifies that collect action uploads to Drive when DriveURL is set.
func TestControlPollerCollect(t *testing.T) {
	// Create a temp file to upload
	dir := t.TempDir()
	testFile := filepath.Join(dir, "data.bin")
	if err := os.WriteFile(testFile, []byte("hello-drive"), 0o644); err != nil {
		t.Fatal(err)
	}

	var driveGot []byte
	drive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Supabase Storage API: POST /storage/v1/object/c4-drive/{path}
		if strings.HasPrefix(r.URL.Path, "/storage/v1/object/c4-drive/") {
			driveGot, _ = io.ReadAll(r.Body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer drive.Close()

	msgs := []model.EdgeControlMessage{{
		Action: "collect",
		Params: map[string]string{"local_path": testFile},
	}}
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/control") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(msgs)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer hub.Close()

	cp := newControlPoller("edge-1", hub.URL, "", drive.URL, "", dir, &http.Client{Timeout: 5 * time.Second})
	ctx := context.Background()
	retrieved, err := cp.Poll(ctx)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	for _, m := range retrieved {
		cp.handle(ctx, &m)
	}

	if string(driveGot) != "hello-drive" {
		t.Errorf("drive received %q, want %q", string(driveGot), "hello-drive")
	}
}

// TestControlPollerExec verifies that exec action runs a shell command on the edge.
func TestControlPollerExec(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/control") {
			w.Header().Set("Content-Type", "application/json")
			msgs := []model.EdgeControlMessage{{
				Action: "exec",
				Params: map[string]string{"cmd": "echo hello-exec"},
			}}
			json.NewEncoder(w).Encode(msgs) //nolint:errcheck
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer hub.Close()

	cp := newControlPoller("edge-1", hub.URL, "", "", "", "", &http.Client{Timeout: 5 * time.Second})
	ctx := context.Background()
	retrieved, err := cp.Poll(ctx)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	// exec succeeds without error — just verify no panic
	for _, m := range retrieved {
		cp.handle(ctx, &m)
	}
}

// TestControlPollerCollect_NoWorkdir verifies that collect proceeds for any path when workdir is empty.
func TestControlPollerCollect_NoWorkdir(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "result.bin")
	if err := os.WriteFile(testFile, []byte("no-workdir-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	var driveGot []byte
	drive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/storage/v1/object/c4-drive/") {
			driveGot, _ = io.ReadAll(r.Body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer drive.Close()

	msgs := []model.EdgeControlMessage{{
		Action: "collect",
		Params: map[string]string{"local_path": testFile},
	}}
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/control") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(msgs) //nolint:errcheck
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer hub.Close()

	// workdir="" → path guard disabled; testFile is outside any workdir restriction
	cp := newControlPoller("edge-1", hub.URL, "", drive.URL, "", "", &http.Client{Timeout: 5 * time.Second})
	ctx := context.Background()
	retrieved, err := cp.Poll(ctx)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	for _, m := range retrieved {
		cp.handle(ctx, &m)
	}

	if string(driveGot) != "no-workdir-data" {
		t.Errorf("expected upload with no workdir guard, drive received %q", string(driveGot))
	}
}

// TestHealthCheckPass verifies that a passing health check results in "succeeded" status.
func TestHealthCheckPass(t *testing.T) {
	var lastStatus string
	assignCount := 0
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/v1/edges/register":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(model.EdgeRegisterResponse{EdgeID: "e1"})
		case r.Method == "POST" && r.URL.Path == "/v1/deploy/target-status":
			var req model.DeployTargetStatusRequest
			json.NewDecoder(r.Body).Decode(&req)
			lastStatus = req.Status
			w.WriteHeader(http.StatusOK)
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/v1/deploy/assignments/"):
			w.Header().Set("Content-Type", "application/json")
			if assignCount == 0 {
				assignCount++
				json.NewEncoder(w).Encode([]model.DeployAssignmentResponse{{
					DeployID:    "d1",
					Artifacts:   []model.DeployAssignmentArtifact{},
					HealthCheck: model.HealthCheck{Command: "exit 0", TimeoutSec: 5},
				}})
				return
			}
			json.NewEncoder(w).Encode([]model.DeployAssignmentResponse{})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer hub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	Run(ctx, Config{ //nolint:errcheck
		HubURL:       hub.URL,
		EdgeName:     "e1",
		Workdir:      t.TempDir(),
		PollInterval: 50 * time.Millisecond,
	})

	if lastStatus != "succeeded" {
		t.Errorf("expected status=succeeded, got %q", lastStatus)
	}
}

// TestHealthCheckFail verifies that a failing health check triggers rollback and "failed" status.
func TestHealthCheckFail(t *testing.T) {
	var lastStatus string
	callCount := 0
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/v1/edges/register":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(model.EdgeRegisterResponse{EdgeID: "e2"})
		case r.Method == "POST" && r.URL.Path == "/v1/deploy/target-status":
			var req model.DeployTargetStatusRequest
			json.NewDecoder(r.Body).Decode(&req)
			lastStatus = req.Status
			w.WriteHeader(http.StatusOK)
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/v1/deploy/assignments/"):
			w.Header().Set("Content-Type", "application/json")
			if callCount == 0 {
				callCount++
				json.NewEncoder(w).Encode([]model.DeployAssignmentResponse{{
					DeployID:    "d2",
					Artifacts:   []model.DeployAssignmentArtifact{},
					HealthCheck: model.HealthCheck{Command: "exit 1", TimeoutSec: 5},
				}})
				return
			}
			json.NewEncoder(w).Encode([]model.DeployAssignmentResponse{})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer hub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	Run(ctx, Config{ //nolint:errcheck
		HubURL:       hub.URL,
		EdgeName:     "e2",
		Workdir:      t.TempDir(),
		PollInterval: 50 * time.Millisecond,
	})

	if lastStatus != "failed" {
		t.Errorf("expected status=failed after health check fail, got %q", lastStatus)
	}
}

// TestRollbackManager verifies BeforeDeploy creates .prev and Rollback restores it.
func TestRollbackManager(t *testing.T) {
	dir := t.TempDir()
	deployDir := filepath.Join(dir, "deploy")
	if err := os.MkdirAll(deployDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write initial file
	modelFile := filepath.Join(deployDir, "model.onnx")
	h1 := []byte("hash-v1-content")
	if err := os.WriteFile(modelFile, h1, 0o644); err != nil {
		t.Fatal(err)
	}

	rb := newRollbackManager(deployDir)

	// Backup
	if err := rb.BeforeDeploy(deployDir); err != nil {
		t.Fatalf("BeforeDeploy: %v", err)
	}
	prev := deployDir + ".prev"
	if _, err := os.Stat(prev); err != nil {
		t.Fatalf(".prev should exist: %v", err)
	}

	// Simulate new deploy
	h2 := []byte("hash-v2-content")
	if err := os.WriteFile(modelFile, h2, 0o644); err != nil {
		t.Fatal(err)
	}

	// Rollback
	if err := rb.Rollback(deployDir); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	// Verify restored
	got, err := os.ReadFile(filepath.Join(deployDir, "model.onnx"))
	if err != nil {
		t.Fatalf("read after rollback: %v", err)
	}
	if string(got) != string(h1) {
		t.Errorf("after rollback got %q, want %q", string(got), string(h1))
	}
}
