package edgeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
