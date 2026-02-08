// Package benchmark provides performance benchmarks comparing Go core
// operations against equivalent Python operations for Phase 3 Go/No-Go
// decision.
//
// Run benchmarks:
//
//	go test ./test/benchmark/ -bench=. -benchmem -count=5
//
// Generate pprof profile:
//
//	go test ./test/benchmark/ -bench=BenchmarkMCPStatusCall -cpuprofile=cpu.prof -memprofile=mem.prof
package benchmark

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/mcp/handlers"
)

// mockBenchStore implements handlers.Store for benchmarking.
type mockBenchStore struct{}

func (m *mockBenchStore) GetStatus() (*handlers.ProjectStatus, error) {
	return &handlers.ProjectStatus{
		State:        "EXECUTE",
		ProjectName:  "bench-project",
		TotalTasks:   253,
		PendingTasks: 50,
		InProgress:   3,
		DoneTasks:    200,
		Workers: []handlers.WorkerInfo{
			{ID: "w1", Status: "busy", CurrentTask: "T-001-0"},
			{ID: "w2", Status: "busy", CurrentTask: "T-002-0"},
			{ID: "w3", Status: "idle"},
		},
	}, nil
}

func (m *mockBenchStore) Start() error                  { return nil }
func (m *mockBenchStore) Clear(bool) error              { return nil }
func (m *mockBenchStore) AddTask(*handlers.Task) error  { return nil }
func (m *mockBenchStore) GetTask(string) (*handlers.Task, error) {
	return &handlers.Task{ID: "T-001-0", Title: "Bench Task"}, nil
}
func (m *mockBenchStore) AssignTask(string) (*handlers.TaskAssignment, error) {
	return &handlers.TaskAssignment{
		TaskID: "T-001-0",
		Title:  "Bench Task",
		Scope:  "src/",
		DoD:    "Implement the feature",
		Branch: "c4/w-T-001-0",
	}, nil
}
func (m *mockBenchStore) SubmitTask(string, string, string, []handlers.ValidationResult) (*handlers.SubmitResult, error) {
	return &handlers.SubmitResult{Success: true, NextAction: "get_next_task"}, nil
}
func (m *mockBenchStore) MarkBlocked(string, string, string, int, string) error { return nil }
func (m *mockBenchStore) ClaimTask(string) (*handlers.Task, error) {
	return &handlers.Task{ID: "T-001-0"}, nil
}
func (m *mockBenchStore) ReportTask(string, string, []string) error      { return nil }
func (m *mockBenchStore) TransitionState(string, string) error          { return nil }
func (m *mockBenchStore) Checkpoint(string, string, string, []string) (*handlers.CheckpointResult, error) {
	return &handlers.CheckpointResult{Success: true}, nil
}

// setupRegistry creates an MCP registry with all handlers registered.
func setupRegistry() *mcp.Registry {
	reg := mcp.NewRegistry()
	store := &mockBenchStore{}
	handlers.RegisterAll(reg, store)
	return reg
}

// BenchmarkRegistryCreation measures the time to create and populate a registry.
// Target: <1ms
func BenchmarkRegistryCreation(b *testing.B) {
	store := &mockBenchStore{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reg := mcp.NewRegistry()
		handlers.RegisterAll(reg, store)
		_ = reg
	}
}

// BenchmarkMCPStatusCall measures c4_status response time.
// Target: <50ms p50 (mock store makes this much faster).
func BenchmarkMCPStatusCall(b *testing.B) {
	reg := setupRegistry()
	args := json.RawMessage(`{}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := reg.Call("c4_status", args)
		if err != nil {
			b.Fatalf("c4_status error: %v", err)
		}
	}
}

// BenchmarkMCPGetTask measures c4_get_task response time.
func BenchmarkMCPGetTask(b *testing.B) {
	reg := setupRegistry()
	args := json.RawMessage(`{"worker_id":"bench-worker"}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := reg.Call("c4_get_task", args)
		if err != nil {
			b.Fatalf("c4_get_task error: %v", err)
		}
	}
}

// BenchmarkMCPSubmit measures c4_submit response time.
func BenchmarkMCPSubmit(b *testing.B) {
	reg := setupRegistry()
	args := json.RawMessage(`{"task_id":"T-001-0","worker_id":"bench-worker","commit_sha":"abc123","validation_results":[{"name":"lint","status":"pass"}]}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := reg.Call("c4_submit", args)
		if err != nil {
			b.Fatalf("c4_submit error: %v", err)
		}
	}
}

// BenchmarkMCPAddTodo measures c4_add_todo response time.
func BenchmarkMCPAddTodo(b *testing.B) {
	reg := setupRegistry()
	args := json.RawMessage(`{"task_id":"T-BENCH-001-0","title":"Benchmark task","scope":"src/bench/","dod":"Implement benchmarks","validations":["lint","unit"]}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := reg.Call("c4_add_todo", args)
		if err != nil {
			b.Fatalf("c4_add_todo error: %v", err)
		}
	}
}

// BenchmarkToolLookup measures tool name resolution time.
func BenchmarkToolLookup(b *testing.B) {
	reg := setupRegistry()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = reg.HasTool("c4_status")
		_ = reg.HasTool("c4_get_task")
		_ = reg.HasTool("c4_submit")
		_ = reg.HasTool("nonexistent")
	}
}

// BenchmarkListTools measures the time to list all tool schemas.
func BenchmarkListTools(b *testing.B) {
	reg := setupRegistry()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tools := reg.ListTools()
		if len(tools) != 10 {
			b.Fatalf("expected 10 tools, got %d", len(tools))
		}
	}
}

// BenchmarkJSONParsing measures JSON argument parsing overhead.
func BenchmarkJSONParsing(b *testing.B) {
	type submitArgs struct {
		TaskID            string                      `json:"task_id"`
		WorkerID          string                      `json:"worker_id"`
		CommitSHA         string                      `json:"commit_sha"`
		ValidationResults []handlers.ValidationResult `json:"validation_results"`
	}
	raw := json.RawMessage(`{"task_id":"T-001-0","worker_id":"worker-abc","commit_sha":"abc123def456","validation_results":[{"name":"lint","status":"pass","message":"ok"},{"name":"unit","status":"pass","message":"42 passed"}]}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var args submitArgs
		if err := json.Unmarshal(raw, &args); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkConfigLoad measures config loading time.
// Target: fast since it reads from filesystem + YAML parse.
func BenchmarkConfigLoad(b *testing.B) {
	tmpDir := b.TempDir()
	configDir := filepath.Join(tmpDir, ".c4")
	os.MkdirAll(configDir, 0o755)
	os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(`
project_id: bench
domain: library
validation:
  lint: "echo ok"
  unit: "echo ok"
economic_mode:
  enabled: true
  preset: standard
`), 0o644)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := config.New(tmpDir)
		if err != nil {
			b.Fatalf("config.New error: %v", err)
		}
	}
}

// BenchmarkWorkerCreation measures worker struct creation time.
// Target: <1ms
func BenchmarkWorkerCreation(b *testing.B) {
	type Worker struct {
		ID      string
		State   string
		TaskID  string
		Created time.Time
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := Worker{
			ID:      fmt.Sprintf("worker-%d", i),
			State:   "idle",
			TaskID:  "",
			Created: time.Now(),
		}
		_ = w
	}
}

// BenchmarkMemoryBaseline reports Go runtime memory usage.
func BenchmarkMemoryBaseline(b *testing.B) {
	var m runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m)
	b.ReportMetric(float64(m.Alloc)/1024/1024, "MB_alloc")
	b.ReportMetric(float64(m.Sys)/1024/1024, "MB_sys")

	// Create registry and measure delta
	reg := setupRegistry()
	_ = reg
	runtime.GC()
	runtime.ReadMemStats(&m)
	b.ReportMetric(float64(m.Alloc)/1024/1024, "MB_after_registry")
}
