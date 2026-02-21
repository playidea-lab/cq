package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGPUComponent_Name(t *testing.T) {
	c := NewGPUComponent(GPUComponentConfig{})
	if c.Name() != "gpu" {
		t.Errorf("Name() = %q, want %q", c.Name(), "gpu")
	}
}

func TestGPUComponent_StartStop(t *testing.T) {
	dir := t.TempDir()
	c := NewGPUComponent(GPUComponentConfig{
		DataDir: dir,
		Version: "test-v1",
	})

	ctx := context.Background()

	// Start
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Verify DB was created
	dbPath := filepath.Join(dir, "daemon.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("daemon.db not created")
	}

	// Verify running state
	if !c.running {
		t.Error("component should be running after Start")
	}

	// Double start should error
	if err := c.Start(ctx); err == nil {
		t.Error("expected error on double Start")
	}

	// Stop
	if err := c.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if c.running {
		t.Error("component should not be running after Stop")
	}

	// Double stop should be safe
	if err := c.Stop(ctx); err != nil {
		t.Fatalf("double Stop: %v", err)
	}
}

func TestGPUComponent_Health_NotRunning(t *testing.T) {
	c := NewGPUComponent(GPUComponentConfig{})
	h := c.Health()
	if h.Status != "error" {
		t.Errorf("Status = %q, want %q", h.Status, "error")
	}
	if !strings.Contains(h.Detail, "not running") {
		t.Errorf("Detail = %q, want contains 'not running'", h.Detail)
	}
}

func TestGPUComponent_Health_Running(t *testing.T) {
	dir := t.TempDir()
	c := NewGPUComponent(GPUComponentConfig{
		DataDir: dir,
		Version: "test",
	})

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Stop(ctx)

	h := c.Health()
	if h.Status != "ok" {
		t.Errorf("Status = %q, want %q", h.Status, "ok")
	}
	// On a CI/dev machine without GPU, should report cpu-only
	if !strings.Contains(h.Detail, "active jobs") {
		t.Errorf("Detail = %q, want contains 'active jobs'", h.Detail)
	}
}

func TestGPUComponent_CPUOnlyMode(t *testing.T) {
	// On most dev machines, GPUCount will be 0
	// Verify the component operates in CPU-only mode gracefully
	dir := t.TempDir()
	c := NewGPUComponent(GPUComponentConfig{
		DataDir: dir,
		MaxJobs: 2,
		Version: "cpu-test",
	})

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Stop(ctx)

	h := c.Health()
	if h.Status != "ok" {
		t.Errorf("Status = %q, want %q", h.Status, "ok")
	}
	if !strings.Contains(h.Detail, "cpu-only") {
		// Only check if no GPU is actually present
		gpuCount := c.gpu.GPUCount()
		if gpuCount == 0 && !strings.Contains(h.Detail, "cpu-only") {
			t.Errorf("Detail = %q, expected cpu-only mode", h.Detail)
		}
	}
}

func TestGPUComponent_Handler_NotStarted(t *testing.T) {
	c := NewGPUComponent(GPUComponentConfig{})

	handler := c.Handler()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestGPUComponent_Handler_HealthEndpoint(t *testing.T) {
	dir := t.TempDir()
	c := NewGPUComponent(GPUComponentConfig{
		DataDir: dir,
		Version: "handler-test",
	})

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Stop(ctx)

	handler := c.Handler()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("health status = %v, want ok", resp["status"])
	}
}

func TestGPUComponent_Handler_GPUStatusEndpoint(t *testing.T) {
	dir := t.TempDir()
	c := NewGPUComponent(GPUComponentConfig{
		DataDir: dir,
		Version: "gpu-test",
	})

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Stop(ctx)

	handler := c.Handler()
	req := httptest.NewRequest(http.MethodGet, "/gpu/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Should have "available" field
	if _, ok := resp["available"]; !ok {
		t.Error("expected 'available' field in gpu/status response")
	}
}

func TestGPUComponent_Handler_JobsListEndpoint(t *testing.T) {
	dir := t.TempDir()
	c := NewGPUComponent(GPUComponentConfig{
		DataDir: dir,
		Version: "jobs-test",
	})

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Stop(ctx)

	handler := c.Handler()
	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := resp["jobs"]; !ok {
		t.Error("expected 'jobs' field in /jobs response")
	}
}

func TestGPUComponent_WithManager(t *testing.T) {
	dir := t.TempDir()
	c := NewGPUComponent(GPUComponentConfig{
		DataDir: dir,
		Version: "mgr-test",
	})

	mgr := NewManager()
	mgr.Register(c)

	ctx := context.Background()
	if err := mgr.StartAll(ctx); err != nil {
		t.Fatalf("StartAll: %v", err)
	}

	// Check health through manager
	hm := mgr.HealthMap()
	if h, ok := hm["gpu"]; !ok {
		t.Error("gpu not found in health map")
	} else if h.Status != "ok" {
		t.Errorf("gpu health status = %q, want %q", h.Status, "ok")
	}

	if err := mgr.StopAll(ctx); err != nil {
		t.Fatalf("StopAll: %v", err)
	}
}
