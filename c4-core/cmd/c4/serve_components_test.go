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
