package observe

import (
	"sync"
	"time"
)

// ToolMetrics holds aggregated statistics for a single tool.
type ToolMetrics struct {
	Calls          int64   `json:"calls"`
	Errors         int64   `json:"errors"`
	TotalLatencyMs float64 `json:"total_latency_ms"`
	AvgLatencyMs   float64 `json:"avg_latency_ms"`
}

// MetricsSnapshot is a point-in-time view of all recorded metrics.
// It is safe to marshal to JSON.
type MetricsSnapshot struct {
	Tools       map[string]ToolMetrics `json:"tools"`
	TotalCalls  int64                  `json:"total_calls"`
	TotalErrors int64                  `json:"total_errors"`
}

// toolAccumulator is the internal mutable accumulator for a single tool.
type toolAccumulator struct {
	calls          int64
	errors         int64
	totalLatencyMs float64
}

// Metrics accumulates per-tool call counts, error counts, and latencies.
// All methods are safe for concurrent use.
type Metrics struct {
	mu    sync.Mutex
	tools map[string]*toolAccumulator
}

// NewMetrics creates an empty Metrics accumulator.
func NewMetrics() *Metrics {
	return &Metrics{
		tools: make(map[string]*toolAccumulator),
	}
}

// Record adds one observation for the named tool.
// latency is the wall-clock time spent in the handler.
// If err is non-nil, the call is counted as an error.
func (m *Metrics) Record(tool string, latency time.Duration, err error) {
	m.mu.Lock()
	acc, ok := m.tools[tool]
	if !ok {
		acc = &toolAccumulator{}
		m.tools[tool] = acc
	}
	acc.calls++
	acc.totalLatencyMs += float64(latency.Milliseconds())
	if err != nil {
		acc.errors++
	}
	m.mu.Unlock()
}

// Snapshot returns a deep copy of all current metrics.
// The returned value is JSON-serializable.
func (m *Metrics) Snapshot() MetricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	snap := MetricsSnapshot{
		Tools: make(map[string]ToolMetrics, len(m.tools)),
	}
	for name, acc := range m.tools {
		avg := float64(0)
		if acc.calls > 0 {
			avg = acc.totalLatencyMs / float64(acc.calls)
		}
		snap.Tools[name] = ToolMetrics{
			Calls:          acc.calls,
			Errors:         acc.errors,
			TotalLatencyMs: acc.totalLatencyMs,
			AvgLatencyMs:   avg,
		}
		snap.TotalCalls += acc.calls
		snap.TotalErrors += acc.errors
	}
	return snap
}
