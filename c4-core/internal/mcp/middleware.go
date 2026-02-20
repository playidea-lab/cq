package mcp

import "context"

// Middleware wraps a HandlerFunc, returning a new HandlerFunc.
// Middleware is applied in registration order: the first Use'd middleware
// is the outermost wrapper (executed first on entry, last on exit).
type Middleware func(next HandlerFunc) HandlerFunc

// ContextualMiddleware is a context-aware variant of Middleware.
// It receives the call context (with tool name injected via ToolNameFromContext)
// and the tool name directly. Register via Registry.UseContextual.
type ContextualMiddleware func(ctx context.Context, name string, next HandlerFunc) HandlerFunc
