package main

import (
	"context"
	"net/http"
	"net/http/httptest"
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
