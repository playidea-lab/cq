package mcp

// Middleware wraps a HandlerFunc, returning a new HandlerFunc.
// Middleware is applied in registration order: the first Use'd middleware
// is the outermost wrapper (executed first on entry, last on exit).
type Middleware func(next HandlerFunc) HandlerFunc
