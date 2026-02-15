package handlers

import (
	"encoding/json"
	"testing"

	"github.com/changmin/c4-core/internal/daemon"
)

func TestGpuStatusHandler_NoGPU(t *testing.T) {
	// GpuMonitor will fail on macOS/no-GPU — should return fallback
	mon := daemon.NewGpuMonitor()
	handler := gpuStatusHandler(mon)

	result, err := handler(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}

	// Should have gpu_count (0 on macOS/no-GPU)
	if _, ok := m["gpu_count"]; !ok {
		t.Error("missing gpu_count field")
	}
	if _, ok := m["backend"]; !ok {
		t.Error("missing backend field")
	}
}

func TestJobSubmitHandler_NoCommand(t *testing.T) {
	handler := jobSubmitHandler(nil)

	args, _ := json.Marshal(map[string]any{})
	result, err := handler(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["error"] != "command is required" {
		t.Errorf("error = %v, want 'command is required'", m["error"])
	}
}

func TestJobSubmitHandler_NoStore(t *testing.T) {
	handler := jobSubmitHandler(nil)

	args, _ := json.Marshal(map[string]any{"command": "python train.py"})
	result, err := handler(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["error"] != "GPU job scheduler not available" {
		t.Errorf("error = %v, want 'GPU job scheduler not available'", m["error"])
	}
}

func TestJobSubmitHandler_WithStore(t *testing.T) {
	dir := t.TempDir()
	store, err := daemon.NewStore(dir + "/daemon.db")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	handler := jobSubmitHandler(store)

	args, _ := json.Marshal(map[string]any{"command": "python train.py", "priority": 5})
	result, err := handler(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["success"] != true {
		t.Errorf("success = %v, want true", m["success"])
	}
	if m["job_id"] == nil || m["job_id"] == "" {
		t.Error("expected non-empty job_id")
	}
}
