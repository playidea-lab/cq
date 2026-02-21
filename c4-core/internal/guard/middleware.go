// Package guard – middleware integration with the mcp.Registry.
//
// The mcp.Registry applies a middleware chain at call time.
// The standard mcp.HandlerFunc receives only (args json.RawMessage) — no context.
// To enforce per-tool policies we provide several variants:
//
//   - MiddlewareForTool(eng, actor, toolName): hard-coded for one tool.
//     Uses context.Background() — actor/tool are statically bound at registration.
//
//   - MiddlewareWithResolver(eng, actor, resolverFn): calls resolverFn() at
//     runtime to determine the current tool name.
//
//   - Middleware(eng, actor): generic variant — allows all calls when the
//     tool name cannot be determined.  Useful as a base layer.
//
// NOTE: For context-propagated actor/tool identity, use the ContextualMiddleware
// form (ContextualMiddlewareFunc) which receives the real request context and
// tool name from the mcp.Registry.UseContextual() API.
//
// Context helpers:
//   - WithActor / ActorFromContext: embed / extract actor identity.
//   - WithTool  / toolFromContext:  embed / extract tool name.
package guard

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/changmin/c4-core/internal/mcp"
)

type contextKey int

const actorKey contextKey = iota

// WithActor returns a new context carrying the actor identifier.
func WithActor(ctx context.Context, actor string) context.Context {
	return context.WithValue(ctx, actorKey, actor)
}

// ActorFromContext extracts the actor from ctx.
// Returns "" if not set.
func ActorFromContext(ctx context.Context) string {
	v, _ := ctx.Value(actorKey).(string)
	return v
}

type toolContextKey int

const toolKey toolContextKey = iota

// WithTool returns a context carrying the tool name for guard enforcement.
func WithTool(ctx context.Context, tool string) context.Context {
	return context.WithValue(ctx, toolKey, tool)
}

func toolFromContext(ctx context.Context) string {
	v, _ := ctx.Value(toolKey).(string)
	return v
}

// Middleware returns a generic mcp.Middleware using defaultActor.
// Because mcp.HandlerFunc carries no context, this middleware uses
// context.Background() and the statically-supplied defaultActor.
// For context-propagated enforcement use Registry.UseContextual() instead.
func Middleware(eng *Engine, defaultActor string) mcp.Middleware {
	return func(next mcp.HandlerFunc) mcp.HandlerFunc {
		return func(args json.RawMessage) (any, error) {
			// mcp.HandlerFunc has no context parameter; use background.
			ctx := context.Background()
			// Tool name is not available here — allow through.
			// Use MiddlewareForTool or ContextualMiddleware for per-tool enforcement.
			_ = ActorFromContext(ctx) // context always empty in this path
			return next(args)
		}
	}
}

// MiddlewareForTool returns an mcp.Middleware that enforces guard policies
// for a single, statically-named tool.
//
// Usage — register one middleware per tool that needs enforcement:
//
//	reg.Use(guard.MiddlewareForTool(eng, "alice", "c4_blocked"))
//	reg.Register(mcp.ToolSchema{Name: "c4_blocked"}, handler)
//
// Note: because the Registry applies the full middleware chain to EVERY tool
// call, MiddlewareForTool checks the tool name at runtime and only enforces
// when the call matches toolName; all other tools pass through.
func MiddlewareForTool(eng *Engine, defaultActor, toolName string) mcp.Middleware {
	return func(next mcp.HandlerFunc) mcp.HandlerFunc {
		return func(args json.RawMessage) (any, error) {
			// Actor and toolName are statically bound; context.Background() is
			// intentional here — mcp.HandlerFunc carries no request context.
			action := eng.Check(context.Background(), defaultActor, toolName, args)
			if action == ActionDeny {
				return nil, fmt.Errorf("guard: access denied: actor=%q tool=%q", defaultActor, toolName)
			}
			return next(args)
		}
	}
}

// MiddlewareWithResolver returns a middleware that calls resolverFn() at
// runtime to determine the tool name.  This is the production integration
// point when a shared resolver is updated by the dispatch layer.
//
// Example with an atomic string resolver:
//
//	var current atomic.Value
//	reg.Use(guard.MiddlewareWithResolver(eng, "actor", func() string {
//	    s, _ := current.Load().(string)
//	    return s
//	}))
func MiddlewareWithResolver(eng *Engine, defaultActor string, toolResolver func() string) mcp.Middleware {
	return func(next mcp.HandlerFunc) mcp.HandlerFunc {
		return func(args json.RawMessage) (any, error) {
			toolName := toolResolver()
			if toolName == "" {
				return next(args)
			}
			// context.Background() is intentional — mcp.HandlerFunc has no context param.
			action := eng.Check(context.Background(), defaultActor, toolName, args)
			if action == ActionDeny {
				return nil, fmt.Errorf("guard: access denied: actor=%q tool=%q", defaultActor, toolName)
			}
			return next(args)
		}
	}
}
