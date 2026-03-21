//go:build c7_observe

package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/mcp/handlers"
	"github.com/changmin/c4-core/internal/observe"
)

func init() {
	registerPreStoreHook(initObserve)
	registerEBWireHook(func(_ *initContext, ebClient *eventbus.Client) {
		observe.SetEventBus(ebClient)
	})
}

// initObserve creates the logger, metrics accumulator, and log ring buffer,
// then wires the contextual middleware onto the registry and registers MCP tools.
// Runs as a pre-store hook so the middleware is active for all registered tools.
func initObserve(ctx *initContext) error {
	if ctx.cfgMgr == nil {
		return nil
	}
	cfg := ctx.cfgMgr.GetConfig().Observe
	if !cfg.Enabled {
		return nil
	}

	// Resolve log level.
	level := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	// Resolve log format.
	format := observe.FormatJSON
	if cfg.LogFormat == "text" {
		format = observe.FormatText
	}

	logger := observe.NewLogger(observe.LoggerOpts{
		Format: format,
		Level:  level,
		Output: os.Stderr,
	})
	metrics := observe.NewMetrics()

	// Wire contextual middleware so every tool call is logged and metered.
	ctx.reg.UseContextual(observe.ContextualMiddlewareFunc(logger, metrics, nil))

	// Create TraceCollector and wire DB persistence if available.
	tc := observe.NewTraceCollector()
	if ctx.db != nil {
		tc.SetDB(ctx.db)
	}
	ctx.traceCollector = tc

	// Initialize handler state and log ring (capacity = 500 entries).
	handlers.InitObserveState(logger, metrics, level, format)
	handlers.InitLogRingBuffer(500)

	// Initialize trace analysis handlers when a DB is available.
	if ctx.db != nil {
		analyzer := observe.NewTraceAnalyzer(ctx.db)
		policy := observe.NewTraceDrivenPolicy(analyzer, 0)
		handlers.InitObserveTraceState(ctx.db, analyzer, policy)
	}

	// Register MCP tools.
	handlers.RegisterObserveHandlers(ctx.reg)
	handlers.RegisterObserveTraceHandlers(ctx.reg)

	fmt.Fprintf(os.Stderr, "cq: observe enabled (level=%s, format=%s, 7 tools)\n", cfg.LogLevel, cfg.LogFormat)
	return nil
}
