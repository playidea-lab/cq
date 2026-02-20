//go:build c7_observe

package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/observe"
)

// observeState holds the shared logger and metrics for observe MCP tools.
// It is set once during initialization and read from handlers.
var observeState struct {
	mu      sync.RWMutex
	logger  *observe.Logger
	metrics *observe.Metrics
	level   slog.Level
	format  observe.Format
}

// InitObserveState sets the global logger and metrics used by observe MCP tools.
// This must be called before RegisterObserveHandlers.
func InitObserveState(logger *observe.Logger, metrics *observe.Metrics, level slog.Level, format observe.Format) {
	observeState.mu.Lock()
	defer observeState.mu.Unlock()
	observeState.logger = logger
	observeState.metrics = metrics
	observeState.level = level
	observeState.format = format
}

// RegisterObserveHandlers registers the c4_observe_* MCP tools.
func RegisterObserveHandlers(reg *mcp.Registry) {
	// c4_observe_metrics — return current metrics snapshot
	reg.Register(mcp.ToolSchema{
		Name:        "c4_observe_metrics",
		Description: "Return a current snapshot of per-tool call counts, error counts, and latencies",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(raw json.RawMessage) (any, error) {
		observeState.mu.RLock()
		m := observeState.metrics
		observeState.mu.RUnlock()
		if m == nil {
			return nil, fmt.Errorf("observe not initialized")
		}
		return m.Snapshot(), nil
	})

	// c4_observe_logs — return last N log entries (filter by level/tool)
	reg.Register(mcp.ToolSchema{
		Name:        "c4_observe_logs",
		Description: "Return recent structured log entries. Supports filtering by minimum level and tool name prefix.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of log entries to return (default: 50)",
				},
				"level": map[string]any{
					"type":        "string",
					"description": "Minimum log level filter: debug, info, warn, error (default: info)",
					"enum":        []string{"debug", "info", "warn", "error"},
				},
				"tool": map[string]any{
					"type":        "string",
					"description": "Optional tool name prefix filter",
				},
			},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			Limit int    `json:"limit"`
			Level string `json:"level"`
			Tool  string `json:"tool"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		if args.Limit <= 0 {
			args.Limit = 50
		}

		observeState.mu.RLock()
		rb := globalLogRing
		observeState.mu.RUnlock()

		if rb == nil {
			return map[string]any{
				"entries": []any{},
				"note":    "in-memory log ring not available; log capture requires observe initialization",
			}, nil
		}

		minLevel := slog.LevelInfo
		switch args.Level {
		case "debug":
			minLevel = slog.LevelDebug
		case "warn":
			minLevel = slog.LevelWarn
		case "error":
			minLevel = slog.LevelError
		}

		entries := rb.Filter(args.Limit, minLevel, args.Tool)
		return map[string]any{
			"entries": entries,
			"count":   len(entries),
		}, nil
	})

	// c4_observe_config — dynamically change log level/format
	reg.Register(mcp.ToolSchema{
		Name:        "c4_observe_config",
		Description: "Read or update observability configuration (log level, log format) at runtime",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"log_level": map[string]any{
					"type":        "string",
					"description": "Set log level: debug, info, warn, error",
					"enum":        []string{"debug", "info", "warn", "error"},
				},
				"log_format": map[string]any{
					"type":        "string",
					"description": "Set log format: json, text",
					"enum":        []string{"json", "text"},
				},
			},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			LogLevel  string `json:"log_level"`
			LogFormat string `json:"log_format"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}

		observeState.mu.Lock()
		defer observeState.mu.Unlock()

		changed := map[string]string{}

		if args.LogLevel != "" {
			var lvl slog.Level
			switch args.LogLevel {
			case "debug":
				lvl = slog.LevelDebug
			case "info":
				lvl = slog.LevelInfo
			case "warn":
				lvl = slog.LevelWarn
			case "error":
				lvl = slog.LevelError
			default:
				return nil, fmt.Errorf("unknown log_level %q", args.LogLevel)
			}
			observeState.level = lvl
			changed["log_level"] = args.LogLevel
		}

		if args.LogFormat != "" {
			var logFmt observe.Format
			switch args.LogFormat {
			case "json":
				logFmt = observe.FormatJSON
			case "text":
				logFmt = observe.FormatText
			default:
				return nil, fmt.Errorf("unknown log_format %q", args.LogFormat)
			}
			observeState.format = logFmt
			changed["log_format"] = args.LogFormat
		}

		// Report current config
		levelStr := "info"
		switch observeState.level {
		case slog.LevelDebug:
			levelStr = "debug"
		case slog.LevelWarn:
			levelStr = "warn"
		case slog.LevelError:
			levelStr = "error"
		}
		formatStr := "json"
		if observeState.format == observe.FormatText {
			formatStr = "text"
		}

		return map[string]any{
			"log_level":  levelStr,
			"log_format": formatStr,
			"changed":    changed,
		}, nil
	})

	// c4_observe_health — system health check
	reg.Register(mcp.ToolSchema{
		Name:        "c4_observe_health",
		Description: "Return health status of observable C4 components (logger, metrics, log ring)",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(raw json.RawMessage) (any, error) {
		observeState.mu.RLock()
		logger := observeState.logger
		metrics := observeState.metrics
		rb := globalLogRing
		observeState.mu.RUnlock()

		components := map[string]any{}

		if logger != nil {
			components["logger"] = map[string]any{"status": "ok"}
		} else {
			components["logger"] = map[string]any{"status": "not_initialized"}
		}

		if metrics != nil {
			snap := metrics.Snapshot()
			components["metrics"] = map[string]any{
				"status":       "ok",
				"total_calls":  snap.TotalCalls,
				"total_errors": snap.TotalErrors,
				"tools_tracked": len(snap.Tools),
			}
		} else {
			components["metrics"] = map[string]any{"status": "not_initialized"}
		}

		if rb != nil {
			components["log_ring"] = map[string]any{
				"status": "ok",
				"size":   rb.Size(),
			}
		} else {
			components["log_ring"] = map[string]any{"status": "not_initialized"}
		}

		allOK := logger != nil && metrics != nil
		overallStatus := "ok"
		if !allOK {
			overallStatus = "degraded"
		}

		return map[string]any{
			"status":     overallStatus,
			"components": components,
		}, nil
	})
}

// logRingBuffer is a minimal circular buffer of log entries for c4_observe_logs.
type logRingBuffer struct {
	mu      sync.Mutex
	entries []logEntry
	size    int
	head    int
	count   int
}

type logEntry struct {
	Level   slog.Level `json:"level"`
	LevelStr string    `json:"level_str"`
	Msg     string     `json:"msg"`
	Tool    string     `json:"tool,omitempty"`
	Attrs   map[string]any `json:"attrs,omitempty"`
}

// globalLogRing is the in-memory ring buffer populated by the observe middleware.
// It is nil when observe is not initialized.
var globalLogRing *logRingBuffer

// NewLogRingBuffer creates a ring buffer holding up to capacity entries.
func NewLogRingBuffer(capacity int) *logRingBuffer {
	return &logRingBuffer{
		entries: make([]logEntry, capacity),
		size:    capacity,
	}
}

// Add appends an entry to the ring buffer (overwrites oldest when full).
func (r *logRingBuffer) Add(e logEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[r.head] = e
	r.head = (r.head + 1) % r.size
	if r.count < r.size {
		r.count++
	}
}

// Size returns the current number of stored entries.
func (r *logRingBuffer) Size() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count
}

// Filter returns up to limit entries matching minLevel and tool prefix (newest first).
func (r *logRingBuffer) Filter(limit int, minLevel slog.Level, toolFilter string) []logEntry {
	r.mu.Lock()
	defer r.mu.Unlock()

	result := make([]logEntry, 0, limit)
	// Iterate from newest to oldest.
	for i := 0; i < r.count && len(result) < limit; i++ {
		idx := (r.head - 1 - i + r.size) % r.size
		e := r.entries[idx]
		if e.Level < minLevel {
			continue
		}
		if toolFilter != "" && e.Tool != toolFilter {
			continue
		}
		result = append(result, e)
	}
	return result
}

// InitLogRingBuffer initializes the global log ring buffer with the given capacity.
// Called during observe initialization.
func InitLogRingBuffer(capacity int) *logRingBuffer {
	observeState.mu.Lock()
	defer observeState.mu.Unlock()
	rb := NewLogRingBuffer(capacity)
	globalLogRing = rb
	return rb
}
