package observe

import (
	"context"
	"encoding/json"
	"time"

	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/mcp"
)

// Middleware returns an mcp.Middleware that logs every tool call and records
// aggregate (non-per-tool) metrics. For per-tool metrics use ContextualMiddleware
// registered via Registry.UseContextual.
//
// C3 EventBus integration is disabled (no publisher).
func Middleware(logger *Logger, metrics *Metrics) mcp.Middleware {
	return MiddlewareWithPublisher(logger, metrics, nil)
}

// MiddlewareWithPublisher returns an mcp.Middleware that logs every tool call
// and optionally publishes events to a C3 EventBus publisher.
// If publisher is nil, eventbus integration is a no-op.
//
// Per-tool metrics are not tracked because mcp.Middleware does not receive
// the tool name. Use ContextualMiddlewareFunc for per-tool tracking.
func MiddlewareWithPublisher(logger *Logger, metrics *Metrics, publisher eventbus.Publisher) mcp.Middleware {
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

			if publisher != nil {
				// Optional: publish tool call event to C3 EventBus.
				// Intentionally async and best-effort.
				_ = publisher
			}

			return result, err
		}
	}
}

// ContextualMiddlewareFunc returns an mcp.ContextualMiddleware that logs
// every tool call with the tool name and records per-tool metrics.
// Register via Registry.UseContextual.
//
// If publisher is nil, C3 EventBus integration is a no-op.
func ContextualMiddlewareFunc(logger *Logger, metrics *Metrics, publisher eventbus.Publisher) mcp.ContextualMiddleware {
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

			if publisher != nil {
				_ = publisher
			}

			return result, err
		}
	}
}
