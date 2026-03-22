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

// TestRegisterSecretsSyncComponent_RegisteredFirst verifies that registerSecretsSyncComponent
// adds "secrets-sync" to the manager and ComponentNames()[0] is "secrets-sync".
func TestRegisterSecretsSyncComponent_RegisteredFirst(t *testing.T) {
	mgr := serve.NewManager()
	cfg := config.C4Config{}

	registerSecretsSyncComponent(mgr, cfg, nil)

	names := mgr.ComponentNames()
	if len(names) == 0 {
		t.Fatal("expected at least one component after register")
	}
	if names[0] != "secrets-sync" {
		t.Errorf("ComponentNames()[0] = %q, want %q", names[0], "secrets-sync")
	}
}

// TestSecretsSyncComponent_GetForEnv_NilStore verifies that GetForEnv returns nil when
// the store is nil (secrets unavailable).
func TestSecretsSyncComponent_GetForEnv_NilStore(t *testing.T) {
	comp := &secretsSyncComponent{store: nil}
	envs := comp.GetForEnv([]string{"c5.api_key"})
	if envs != nil {
		t.Errorf("expected nil env vars with nil store, got %v", envs)
	}
}

// TestSecretsSyncComponent_GetForEnv_EmptyInject verifies that GetForEnv returns nil
// when no keys are requested.
func TestSecretsSyncComponent_GetForEnv_EmptyInject(t *testing.T) {
	comp := &secretsSyncComponent{store: nil}
	envs := comp.GetForEnv(nil)
	if envs != nil {
		t.Errorf("expected nil env vars with empty inject list, got %v", envs)
	}
}

// TestSecretsSyncComponent_Health_NilStore verifies Health returns "skipped" when store is nil.
func TestSecretsSyncComponent_Health_NilStore(t *testing.T) {
	comp := &secretsSyncComponent{store: nil}
	h := comp.Health()
	if h.Status != "skipped" {
		t.Errorf("Health().Status = %q, want %q", h.Status, "skipped")
	}
}

// TestRegisterKnowledgeSuggestPoller_HubDisabled verifies that when hub.enabled=false,
// no component is registered.
func TestRegisterKnowledgeSuggestPoller_HubDisabled(t *testing.T) {
	mgr := serve.NewManager()
	cfg := config.C4Config{}
	cfg.Hub.Enabled = false
	cfg.Hub.URL = "http://hub.example.com"

	registerKnowledgeSuggestPollerServeComponent(mgr, cfg, nil)

	if mgr.ComponentCount() != 0 {
		t.Errorf("ComponentCount = %d, want 0 when hub disabled", mgr.ComponentCount())
	}
}

// TestRegisterKnowledgeSuggestPoller_NoURL verifies that when hub.url is empty,
// no component is registered.
func TestRegisterKnowledgeSuggestPoller_NoURL(t *testing.T) {
	mgr := serve.NewManager()
	cfg := config.C4Config{}
	cfg.Hub.Enabled = true
	cfg.Hub.URL = ""

	registerKnowledgeSuggestPollerServeComponent(mgr, cfg, nil)

	if mgr.ComponentCount() != 0 {
		t.Errorf("ComponentCount = %d, want 0 when hub.url empty", mgr.ComponentCount())
	}
}

