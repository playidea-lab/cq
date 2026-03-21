package observe

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/changmin/c4-core/internal/mcp"
)

// observeEventPublisher is a local interface to avoid a direct import of
// the eventbus package. Satisfied by *eventbus.Client and any compatible type.
type observeEventPublisher interface {
	PublishAsync(evType, source string, data json.RawMessage, projectID string)
}

var (
	globalPublisherMu sync.RWMutex
	globalPublisher   observeEventPublisher

	globalTraceCollectorMu sync.RWMutex
	globalTraceCollector   *TraceCollector
)

// SetEventBus sets the package-level EventBus publisher used by all
// ContextualMiddlewareFunc instances. Safe for concurrent use.
func SetEventBus(p observeEventPublisher) {
	globalPublisherMu.Lock()
	defer globalPublisherMu.Unlock()
	globalPublisher = p
}

// SetTraceCollector sets the package-level TraceCollector used by all
// ContextualMiddlewareFunc instances. Safe for concurrent use.
func SetTraceCollector(tc *TraceCollector) {
	globalTraceCollectorMu.Lock()
	defer globalTraceCollectorMu.Unlock()
	globalTraceCollector = tc
}

// Middleware returns an mcp.Middleware that logs every tool call and records
// aggregate (non-per-tool) metrics. For per-tool metrics use ContextualMiddleware
// registered via Registry.UseContextual.
//
// C3 EventBus integration is disabled (no publisher).
func Middleware(logger *Logger, metrics *Metrics) mcp.Middleware {
	return MiddlewareWithPublisher(logger, metrics, nil)
}

// MiddlewareWithPublisher returns an mcp.Middleware that logs every tool call
// and records aggregate metrics. The publisher parameter is unused here because
// mcp.Middleware does not receive the tool name required for tool.called events.
// EventBus integration (tool.called) is handled exclusively by ContextualMiddlewareFunc.
func MiddlewareWithPublisher(logger *Logger, metrics *Metrics, _ observeEventPublisher) mcp.Middleware {
	return func(next mcp.HandlerFunc) mcp.HandlerFunc {
		return func(args json.RawMessage) (any, error) {
			start := time.Now()
			result, err := next(args)
			elapsed := time.Since(start)

			if err != nil {
				logger.Error("tool call failed",
					"latency_ms", elapsed.Milliseconds(),
					"error", err.Error(),
				)
				metrics.Record("_all", elapsed, err)
			} else {
				logger.Debug("tool call completed",
					"latency_ms", elapsed.Milliseconds(),
				)
				metrics.Record("_all", elapsed, nil)
			}

			return result, err
		}
	}
}

// ContextualMiddlewareFunc returns an mcp.ContextualMiddleware that logs
// every tool call with the tool name and records per-tool metrics.
// Register via Registry.UseContextual.
//
// If publisher is nil, C3 EventBus integration falls back to the package-level
// globalPublisher set via SetEventBus. If both are nil, eventbus integration is a no-op.
func ContextualMiddlewareFunc(logger *Logger, metrics *Metrics, publisher observeEventPublisher) mcp.ContextualMiddleware {
	return func(ctx context.Context, name string, next mcp.HandlerFunc) mcp.HandlerFunc {
		return func(args json.RawMessage) (any, error) {
			start := time.Now()
			result, err := next(args)
			elapsed := time.Since(start)

			if err != nil {
				logger.Error("tool call failed",
					"tool", name,
					"latency_ms", elapsed.Milliseconds(),
					"error", err.Error(),
				)
			} else {
				logger.Debug("tool call completed",
					"tool", name,
					"latency_ms", elapsed.Milliseconds(),
				)
			}

			metrics.Record(name, elapsed, err)

			// Resolve publisher: arg-level takes priority, then package-level global.
			pub := publisher
			if pub == nil {
				globalPublisherMu.RLock()
				pub = globalPublisher
				globalPublisherMu.RUnlock()
			}

			if pub != nil {
				data, _ := json.Marshal(map[string]any{
					"tool":       name,
					"latency_ms": elapsed.Milliseconds(),
					"error":      err != nil,
				})
				pub.PublishAsync("tool.called", "c4_observe", data, "")
			}

			// Record TOOL_CALL step to TraceCollector (package-level global).
			globalTraceCollectorMu.RLock()
			tc := globalTraceCollector
			globalTraceCollectorMu.RUnlock()

			if tc != nil {
				errMsg := ""
				if err != nil {
					errMsg = err.Error()
				}
				tc.ensureTrace("unattributed")
				tc.AddStep("unattributed", TraceStep{
					StepType:  StepTypeTool,
					ToolName:  name,
					LatencyMs: elapsed.Milliseconds(),
					Success:   err == nil,
					ErrorMsg:  errMsg,
				})
			}

			return result, err
		}
	}
}
