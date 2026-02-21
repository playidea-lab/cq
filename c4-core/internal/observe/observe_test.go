package observe_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/observe"
)

// mockPublisher records PublishAsync calls for test assertions.
type mockPublisher struct {
	calls atomic.Int64
}

func (m *mockPublisher) PublishAsync(evType, source string, data json.RawMessage, projectID string) {
	m.calls.Add(1)
}

// TestLoggerOutput verifies JSON structured output from the logger.
func TestLoggerOutput(t *testing.T) {
	var buf bytes.Buffer
	logger := observe.NewLogger(observe.LoggerOpts{
		Format: observe.FormatJSON,
		Level:  slog.LevelDebug,
		Output: &buf,
	})

	logger.Info("test message", "key", "value", "num", 42)

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("logger output is not valid JSON: %v\noutput: %s", err, buf.String())
	}

	if msg, ok := record["msg"].(string); !ok || msg != "test message" {
		t.Errorf("expected msg=test message, got %v", record["msg"])
	}
	if record["key"] != "value" {
		t.Errorf("expected key=value, got %v", record["key"])
	}
	if record["num"] == nil {
		t.Errorf("expected num field, got nil")
	}
}

// TestLoggerTextFormat verifies text format output.
func TestLoggerTextFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := observe.NewLogger(observe.LoggerOpts{
		Format: observe.FormatText,
		Level:  slog.LevelInfo,
		Output: &buf,
	})

	logger.Info("hello world")

	out := buf.String()
	if !strings.Contains(out, "hello world") {
		t.Errorf("expected 'hello world' in text output, got: %s", out)
	}
}

// TestLoggerDefaultOpts verifies defaults work (no panic, no nil).
func TestLoggerDefaultOpts(t *testing.T) {
	logger := observe.NewLogger(observe.LoggerOpts{})
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}

// TestMetricsCount verifies that tool call counts are accurately recorded.
func TestMetricsCount(t *testing.T) {
	m := observe.NewMetrics()

	m.Record("tool_a", 10*time.Millisecond, nil)
	m.Record("tool_a", 20*time.Millisecond, nil)
	m.Record("tool_b", 5*time.Millisecond, errors.New("boom"))

	snap := m.Snapshot()

	if snap.Tools["tool_a"].Calls != 2 {
		t.Errorf("tool_a: expected 2 calls, got %d", snap.Tools["tool_a"].Calls)
	}
	if snap.Tools["tool_a"].Errors != 0 {
		t.Errorf("tool_a: expected 0 errors, got %d", snap.Tools["tool_a"].Errors)
	}
	if snap.Tools["tool_b"].Calls != 1 {
		t.Errorf("tool_b: expected 1 call, got %d", snap.Tools["tool_b"].Calls)
	}
	if snap.Tools["tool_b"].Errors != 1 {
		t.Errorf("tool_b: expected 1 error, got %d", snap.Tools["tool_b"].Errors)
	}
}

// TestMetricsLatency verifies latency accumulation.
func TestMetricsLatency(t *testing.T) {
	m := observe.NewMetrics()

	m.Record("slow_tool", 100*time.Millisecond, nil)
	m.Record("slow_tool", 200*time.Millisecond, nil)

	snap := m.Snapshot()
	toolSnap := snap.Tools["slow_tool"]

	if toolSnap.TotalLatencyMs < 300 {
		t.Errorf("expected total latency >= 300ms, got %.2f", toolSnap.TotalLatencyMs)
	}
	// avg should be ~150ms
	if toolSnap.AvgLatencyMs < 100 || toolSnap.AvgLatencyMs > 200 {
		t.Errorf("expected avg latency ~150ms, got %.2f", toolSnap.AvgLatencyMs)
	}
}

// TestMetricsTotal verifies aggregate totals across all tools.
func TestMetricsTotal(t *testing.T) {
	m := observe.NewMetrics()
	m.Record("a", 1*time.Millisecond, nil)
	m.Record("b", 2*time.Millisecond, errors.New("err"))
	m.Record("c", 3*time.Millisecond, nil)

	snap := m.Snapshot()
	if snap.TotalCalls != 3 {
		t.Errorf("expected total calls=3, got %d", snap.TotalCalls)
	}
	if snap.TotalErrors != 1 {
		t.Errorf("expected total errors=1, got %d", snap.TotalErrors)
	}
}

// TestMetricsJSONSerializable verifies Snapshot is JSON-serializable.
func TestMetricsJSONSerializable(t *testing.T) {
	m := observe.NewMetrics()
	m.Record("tool", 50*time.Millisecond, nil)

	snap := m.Snapshot()
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("Snapshot is not JSON-serializable: %v", err)
	}

	var decoded observe.MetricsSnapshot
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal MetricsSnapshot: %v", err)
	}
	if decoded.Tools["tool"].Calls != 1 {
		t.Errorf("expected 1 call after round-trip, got %d", decoded.Tools["tool"].Calls)
	}
}

// TestMetricsConcurrentSafety verifies no data races under concurrent access.
func TestMetricsConcurrentSafety(t *testing.T) {
	m := observe.NewMetrics()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.Record("concurrent", time.Millisecond, nil)
		}()
	}
	wg.Wait()

	snap := m.Snapshot()
	if snap.Tools["concurrent"].Calls != 100 {
		t.Errorf("expected 100 calls, got %d", snap.Tools["concurrent"].Calls)
	}
}

// TestMiddlewareIntegration verifies that ContextualMiddlewareFunc logs
// and records per-tool metrics when registered via UseContextual.
func TestMiddlewareIntegration(t *testing.T) {
	var logBuf bytes.Buffer
	logger := observe.NewLogger(observe.LoggerOpts{
		Format: observe.FormatJSON,
		Level:  slog.LevelDebug,
		Output: &logBuf,
	})
	metrics := observe.NewMetrics()

	reg := mcp.NewRegistry()
	reg.UseContextual(observe.ContextualMiddlewareFunc(logger, metrics, nil))

	// Register a simple tool.
	reg.Register(mcp.ToolSchema{Name: "echo", Description: "echoes input"}, func(args json.RawMessage) (any, error) {
		return "pong", nil
	})

	result, err := reg.Dispatch(t.Context(), "echo", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}
	if result != "pong" {
		t.Errorf("expected pong, got %v", result)
	}

	// Verify metrics recorded per-tool.
	snap := metrics.Snapshot()
	if snap.Tools["echo"].Calls != 1 {
		t.Errorf("expected 1 call recorded in metrics for 'echo', got %d", snap.Tools["echo"].Calls)
	}

	// Verify log output has tool name.
	logOut := logBuf.String()
	if !strings.Contains(logOut, "echo") {
		t.Errorf("expected log output to contain 'echo', got: %s", logOut)
	}
}

// TestMiddlewareIntegrationError verifies error path is logged and counted.
func TestMiddlewareIntegrationError(t *testing.T) {
	var logBuf bytes.Buffer
	logger := observe.NewLogger(observe.LoggerOpts{
		Format: observe.FormatJSON,
		Level:  slog.LevelDebug,
		Output: &logBuf,
	})
	metrics := observe.NewMetrics()

	reg := mcp.NewRegistry()
	reg.UseContextual(observe.ContextualMiddlewareFunc(logger, metrics, nil))

	reg.Register(mcp.ToolSchema{Name: "fail", Description: "always fails"}, func(args json.RawMessage) (any, error) {
		return nil, errors.New("intentional failure")
	})

	_, err := reg.Dispatch(t.Context(), "fail", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error from failing handler")
	}

	snap := metrics.Snapshot()
	if snap.Tools["fail"].Errors != 1 {
		t.Errorf("expected 1 error in metrics for 'fail', got %d", snap.Tools["fail"].Errors)
	}

	// Log should contain tool name.
	logOut := logBuf.String()
	if !strings.Contains(logOut, "fail") {
		t.Errorf("expected log to contain tool name 'fail', got: %s", logOut)
	}
}

// TestMetricsWithoutEventBus verifies graceful no-op when eventbus publisher is nil.
func TestMetricsWithoutEventBus(t *testing.T) {
	var logBuf bytes.Buffer
	logger := observe.NewLogger(observe.LoggerOpts{
		Format: observe.FormatJSON,
		Output: &logBuf,
	})
	metrics := observe.NewMetrics()

	// ContextualMiddlewareFunc with nil publisher = no-op eventbus.
	reg := mcp.NewRegistry()
	reg.UseContextual(observe.ContextualMiddlewareFunc(logger, metrics, nil))
	reg.Register(mcp.ToolSchema{Name: "noop_eb", Description: "test"}, func(args json.RawMessage) (any, error) {
		return "ok", nil
	})

	_, err := reg.Dispatch(t.Context(), "noop_eb", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	snap := metrics.Snapshot()
	if snap.Tools["noop_eb"].Calls != 1 {
		t.Errorf("expected 1 call, got %d", snap.Tools["noop_eb"].Calls)
	}
}

// TestMiddlewarePlain verifies that Middleware (non-contextual) works without panicking.
func TestMiddlewarePlain(t *testing.T) {
	var logBuf bytes.Buffer
	logger := observe.NewLogger(observe.LoggerOpts{
		Format: observe.FormatJSON,
		Level:  slog.LevelDebug,
		Output: &logBuf,
	})
	metrics := observe.NewMetrics()

	reg := mcp.NewRegistry()
	reg.Use(observe.Middleware(logger, metrics))
	reg.Register(mcp.ToolSchema{Name: "plain", Description: "plain test"}, func(args json.RawMessage) (any, error) {
		return "ok", nil
	})

	_, err := reg.Dispatch(t.Context(), "plain", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Plain middleware records to "_all" key, not per-tool.
	snap := metrics.Snapshot()
	if snap.Tools["_all"].Calls != 1 {
		t.Errorf("expected 1 call in '_all' bucket, got %d", snap.Tools["_all"].Calls)
	}
}

// TestSetEventBus_DynamicPublish verifies that after calling SetEventBus the
// ContextualMiddlewareFunc dispatches PublishAsync on each tool call.
func TestSetEventBus_DynamicPublish(t *testing.T) {
	// Reset global publisher after the test to avoid test pollution.
	t.Cleanup(func() { observe.SetEventBus(nil) })

	pub := &mockPublisher{}
	observe.SetEventBus(pub)

	var logBuf bytes.Buffer
	logger := observe.NewLogger(observe.LoggerOpts{Format: observe.FormatJSON, Output: &logBuf})
	metrics := observe.NewMetrics()

	reg := mcp.NewRegistry()
	// Pass nil publisher so the global one is used.
	reg.UseContextual(observe.ContextualMiddlewareFunc(logger, metrics, nil))
	reg.Register(mcp.ToolSchema{Name: "ping", Description: "test"}, func(args json.RawMessage) (any, error) {
		return "pong", nil
	})

	_, err := reg.Dispatch(t.Context(), "ping", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected dispatch error: %v", err)
	}

	if got := pub.calls.Load(); got != 1 {
		t.Errorf("expected 1 PublishAsync call, got %d", got)
	}
}

// TestSetEventBus_Concurrent verifies that SetEventBus and PublishAsync are
// safe under concurrent access (run with -race to detect data races).
func TestSetEventBus_Concurrent(t *testing.T) {
	t.Cleanup(func() { observe.SetEventBus(nil) })

	var logBuf bytes.Buffer
	logger := observe.NewLogger(observe.LoggerOpts{Format: observe.FormatJSON, Output: &logBuf})
	metrics := observe.NewMetrics()

	reg := mcp.NewRegistry()
	reg.UseContextual(observe.ContextualMiddlewareFunc(logger, metrics, nil))
	reg.Register(mcp.ToolSchema{Name: "concurrent_tool", Description: "test"}, func(args json.RawMessage) (any, error) {
		return "ok", nil
	})

	pub := &mockPublisher{}

	var wg sync.WaitGroup
	const goroutines = 50

	// Concurrently set the publisher and dispatch tool calls.
	for i := 0; i < goroutines; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			observe.SetEventBus(pub)
		}()
		go func() {
			defer wg.Done()
			_, _ = reg.Dispatch(t.Context(), "concurrent_tool", json.RawMessage(`{}`))
		}()
	}
	wg.Wait()
}
