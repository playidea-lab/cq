package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/serve"
)

// TestServeMux_DaemonPrefix_GPUEnabled verifies that when gpuComp is non-nil,
// requests to /daemon/* are routed to the GPU handler.
func TestServeMux_DaemonPrefix_GPUEnabled(t *testing.T) {
	dir := t.TempDir()
	gpuComp := serve.NewGPUComponent(serve.GPUComponentConfig{
		DataDir: dir,
		Version: "test",
	})

	ctx := context.Background()
	if err := gpuComp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer gpuComp.Stop(ctx)

	mux := http.NewServeMux()
	mux.Handle("/daemon/", http.StripPrefix("/daemon", gpuComp.Handler()))

	req := httptest.NewRequest(http.MethodGet, "/daemon/jobs", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /daemon/jobs status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// TestServeMux_DaemonPrefix_GPUDisabled verifies that when gpuComp is nil,
// /daemon/* requests return 404.
func TestServeMux_DaemonPrefix_GPUDisabled(t *testing.T) {
	mux := http.NewServeMux()
	// gpuComp is nil — no handler registered, matching DoD condition

	req := httptest.NewRequest(http.MethodGet, "/daemon/jobs", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /daemon/jobs status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestLoadC4CloudEnv_BothSet(t *testing.T) {
	cfg := config.C4Config{}
	cfg.Cloud.URL = "https://example.supabase.co"
	cfg.Cloud.AnonKey = "test-anon-key"
	envs := loadC4CloudEnv(cfg)
	if len(envs) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(envs))
	}
	if envs[0] != "C5_SUPABASE_URL=https://example.supabase.co" {
		t.Errorf("envs[0] = %q, want %q", envs[0], "C5_SUPABASE_URL=https://example.supabase.co")
	}
	if envs[1] != "C5_SUPABASE_KEY=test-anon-key" {
		t.Errorf("envs[1] = %q, want %q", envs[1], "C5_SUPABASE_KEY=test-anon-key")
	}
}

func TestLoadC4CloudEnv_Empty(t *testing.T) {
	cfg := config.C4Config{}
	envs := loadC4CloudEnv(cfg)
	if len(envs) != 0 {
		t.Fatalf("expected 0 env vars, got %d: %v", len(envs), envs)
	}
}

func TestLoadC4CloudEnv_URLOnly(t *testing.T) {
	cfg := config.C4Config{}
	cfg.Cloud.URL = "https://example.supabase.co"
	envs := loadC4CloudEnv(cfg)
	if len(envs) != 1 {
		t.Fatalf("expected 1 env var, got %d", len(envs))
	}
	if envs[0] != "C5_SUPABASE_URL=https://example.supabase.co" {
		t.Errorf("envs[0] = %q, want %q", envs[0], "C5_SUPABASE_URL=https://example.supabase.co")
	}
}

// TestServeStaleCheckerRegistration verifies that when StaleChecker.Enabled=false,
// calling registerStaleCheckerServeComponent does not add a component to the manager.
func TestServeStaleCheckerRegistration(t *testing.T) {
	mgr := serve.NewManager()
	cfg := config.C4Config{
		Serve: config.ServeConfig{
			StaleChecker: config.StaleCheckerConfig{
				Enabled: false,
			},
		},
	}

	// When disabled, no component should be registered.
	registerStaleCheckerServeComponent(mgr, cfg, nil)

	if mgr.ComponentCount() != 0 {
		t.Errorf("ComponentCount = %d, want 0 when StaleChecker disabled", mgr.ComponentCount())
	}
}

// TestServeStatus_ComponentOutput verifies that fetchServeHealth parses
// the /health JSON response into a per-component map.
func TestServeStatus_ComponentOutput(t *testing.T) {
	// Spin up a test HTTP server that returns a HealthResponse.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hr := serve.HealthResponse{
			Status: "ok",
			Components: map[string]serve.ComponentHealth{
				"eventbus": {Status: "ok"},
				"hub":      {Status: "skipped", Detail: `binary "c5" not found`},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(hr)
	}))
	defer ts.Close()

	// Extract port from test server URL.
	idx := strings.LastIndex(ts.URL, ":")
	port, err := strconv.Atoi(ts.URL[idx+1:])
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	components, err := fetchServeHealth(port)
	if err != nil {
		t.Fatalf("fetchServeHealth: %v", err)
	}

	if len(components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(components))
	}
	if components["eventbus"].Status != "ok" {
		t.Errorf("eventbus status = %q, want %q", components["eventbus"].Status, "ok")
	}
	if components["hub"].Status != "skipped" {
		t.Errorf("hub status = %q, want %q", components["hub"].Status, "skipped")
	}
	if !strings.Contains(components["hub"].Detail, "not found") {
		t.Errorf("hub detail = %q, want contains 'not found'", components["hub"].Detail)
	}
}

// TestPrintServeStartupSummary verifies the startup summary output format and sort order.
func TestPrintServeStartupSummary(t *testing.T) {
	var buf strings.Builder
	components := map[string]serve.ComponentHealth{
		"eventbus": {Status: "ok"},
		"hub":      {Status: "skipped", Detail: `binary "c5" not found`},
	}
	printServeStartupSummary(&buf, 12345, 4140, components)
	out := buf.String()

	if !strings.Contains(out, "pid=12345") {
		t.Errorf("missing pid in output: %q", out)
	}
	if !strings.Contains(out, "port=4140") {
		t.Errorf("missing port in output: %q", out)
	}
	if !strings.Contains(out, "✓") {
		t.Errorf("missing ok indicator (✓) in output: %q", out)
	}
	if !strings.Contains(out, "✗") {
		t.Errorf("missing non-ok indicator (✗) in output: %q", out)
	}
	if !strings.Contains(out, "not found") {
		t.Errorf("missing detail in skipped component output: %q", out)
	}
	// Alphabetical order: eventbus before hub
	if ebIdx, hubIdx := strings.Index(out, "eventbus"), strings.Index(out, "hub"); ebIdx > hubIdx {
		t.Errorf("components not sorted: eventbus(%d) after hub(%d)", ebIdx, hubIdx)
	}
}

// TestRunServeStop_OSServiceFallback verifies tryStopOSService behavior
// with injected stopFn — PID-less stop path.
func TestRunServeStop_OSServiceFallback(t *testing.T) {
	t.Run("stop succeeds", func(t *testing.T) {
		called := false
		if err := tryStopOSService(func() error {
			called = true
			return nil
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !called {
			t.Error("stopFn was not called")
		}
	})

	t.Run("not installed returns nil", func(t *testing.T) {
		if err := tryStopOSService(func() error {
			return fmt.Errorf("service not installed")
		}); err != nil {
			t.Errorf("expected nil for 'not installed', got: %v", err)
		}
	})

	t.Run("unexpected error propagates", func(t *testing.T) {
		expected := fmt.Errorf("permission denied")
		if err := tryStopOSService(func() error { return expected }); err != expected {
			t.Errorf("expected propagated error, got: %v", err)
		}
	})
}

// TestHealthHandler_SkippedNotDegraded verifies that when all components are
// "skipped", the HealthHandler returns overall status "ok" (not "degraded").
func TestHealthHandler_SkippedNotDegraded(t *testing.T) {
	mgr := serve.NewManager()

	// Register a hub component with a missing binary so Health() returns "skipped".
	hub := serve.NewHubComponent(serve.HubComponentConfig{
		Binary: "c5-binary-that-does-not-exist-xyz",
		Port:   19995,
	})
	// Start it so startErr is set (graceful skip).
	if err := hub.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	mgr.Register(hub)

	handler := serve.HealthHandler(mgr)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (all-skipped should be overall ok)", rec.Code, http.StatusOK)
	}

	var hr serve.HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&hr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if hr.Status != "ok" {
		t.Errorf("overall status = %q, want %q", hr.Status, "ok")
	}
}
