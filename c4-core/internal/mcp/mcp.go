// Package mcp implements the MCP (Model Context Protocol) server
// for communication between C4 and AI agents (Claude Code, Cursor, etc.).
//
// The MCP server exposes C4's functionality as tools that can be
// invoked by AI agents through the standardized MCP protocol.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// ToolSchema describes a single MCP tool's metadata.
type ToolSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// HandlerFunc is the function signature for tool handlers.
type HandlerFunc func(args json.RawMessage) (any, error)

// BlockingHandlerFunc is the function signature for cancellable blocking tool handlers.
// The context is cancelled when the MCP client sends notifications/cancelled for this request.
type BlockingHandlerFunc func(ctx context.Context, args json.RawMessage) (any, error)

// Registry manages registered MCP tools and dispatches calls.
type Registry struct {
	mu          sync.RWMutex
	tools       map[string]registeredTool
	ordering    []string // preserve registration order
	middlewares []Middleware

	// OnChange is called after Register/Replace/Unregister mutate the tool list.
	// Used by the MCP server to send notifications/tools/list_changed.
	OnChange func()

	// OnCall is called at the beginning of every tool dispatch (Option C: implicit heartbeat).
	// Set to SQLiteStore.TouchCurrentWorkerHeartbeat to refresh the active worker's task
	// updated_at on every tool call, preventing false stale-task detection.
	OnCall func()
}

type registeredTool struct {
	schema   ToolSchema
	handler  HandlerFunc         // nil if blocking
	bhandler BlockingHandlerFunc // nil if not blocking
}

// NewRegistry creates a new empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]registeredTool),
	}
}

// Use appends middlewares to the registry's middleware chain.
// Middlewares are applied in registration order: the first Use'd middleware
// is the outermost wrapper (executed first on entry, last on exit).
// Use must be called before tool registration for consistent behavior.
func (r *Registry) Use(mw ...Middleware) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.middlewares = append(r.middlewares, mw...)
}

// Register adds a tool to the registry. If a tool with the same name
// is already registered, it logs a warning and skips the duplicate.
func (r *Registry) Register(schema ToolSchema, handler HandlerFunc) {
	r.mu.Lock()

	if _, exists := r.tools[schema.Name]; exists {
		r.mu.Unlock()
		fmt.Fprintf(os.Stderr, "mcp: warning: tool already registered, skipping: %s\n", schema.Name)
		return
	}

	r.tools[schema.Name] = registeredTool{
		schema:  schema,
		handler: handler,
	}
	r.ordering = append(r.ordering, schema.Name)
	onChange := r.OnChange
	r.mu.Unlock()

	if onChange != nil {
		onChange()
	}
}

// RegisterBlocking adds a cancellable blocking tool to the registry.
// The handler receives a context that is cancelled when the MCP client sends
// notifications/cancelled for the corresponding request.
func (r *Registry) RegisterBlocking(schema ToolSchema, handler BlockingHandlerFunc) {
	r.mu.Lock()

	if _, exists := r.tools[schema.Name]; exists {
		r.mu.Unlock()
		fmt.Fprintf(os.Stderr, "mcp: warning: tool already registered, skipping: %s\n", schema.Name)
		return
	}

	r.tools[schema.Name] = registeredTool{
		schema:   schema,
		bhandler: handler,
	}
	r.ordering = append(r.ordering, schema.Name)
	onChange := r.OnChange
	r.mu.Unlock()

	if onChange != nil {
		onChange()
	}
}

// Call invokes a registered tool by name with the given JSON arguments.
// Calls OnCall (if set) before dispatching — used for implicit worker heartbeat.
func (r *Registry) Call(name string, args json.RawMessage) (any, error) {
	return r.CallWithContext(context.Background(), name, args)
}

// CallWithContext invokes a registered tool, passing ctx to blocking handlers.
// For regular handlers ctx is ignored; for blocking handlers ctx enables cancellation.
// The handler is wrapped with the registered middleware chain before execution.
func (r *Registry) CallWithContext(ctx context.Context, name string, args json.RawMessage) (any, error) {
	r.mu.RLock()
	tool, ok := r.tools[name]
	onCall := r.OnCall
	mws := r.middlewares // snapshot under lock
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("mcp: unknown tool: %s", name)
	}

	if onCall != nil {
		onCall()
	}

	// Build the base handler: adapt BlockingHandlerFunc to HandlerFunc by capturing ctx.
	var base HandlerFunc
	if tool.bhandler != nil {
		bh := tool.bhandler
		base = func(a json.RawMessage) (any, error) {
			return bh(ctx, a)
		}
	} else {
		base = tool.handler
	}

	// Apply middleware chain in reverse order so that the first Use'd
	// middleware is the outermost wrapper.
	h := base
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}

	return h(args)
}

// HasTool returns true if a tool with the given name is registered.
func (r *Registry) HasTool(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.tools[name]
	return ok
}

// Replace atomically swaps handler+schema for an existing tool.
// Returns false if the tool is not registered.
func (r *Registry) Replace(schema ToolSchema, handler HandlerFunc) bool {
	r.mu.Lock()
	if _, exists := r.tools[schema.Name]; !exists {
		r.mu.Unlock()
		return false
	}
	r.tools[schema.Name] = registeredTool{schema: schema, handler: handler}
	onChange := r.OnChange
	r.mu.Unlock()

	if onChange != nil {
		onChange()
	}
	return true
}

// Unregister removes a tool from the registry.
// Returns false if the tool is not registered.
func (r *Registry) Unregister(name string) bool {
	r.mu.Lock()
	if _, exists := r.tools[name]; !exists {
		r.mu.Unlock()
		return false
	}
	delete(r.tools, name)
	for i, n := range r.ordering {
		if n == name {
			r.ordering = append(r.ordering[:i], r.ordering[i+1:]...)
			break
		}
	}
	onChange := r.OnChange
	r.mu.Unlock()

	if onChange != nil {
		onChange()
	}
	return true
}

// GetToolSchema returns the schema for a registered tool.
// Returns false if the tool is not registered.
func (r *Registry) GetToolSchema(name string) (ToolSchema, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	if !ok {
		return ToolSchema{}, false
	}
	return tool.schema, true
}

// ListTools returns all registered tool schemas in registration order.
func (r *Registry) ListTools() []ToolSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()

	schemas := make([]ToolSchema, 0, len(r.ordering))
	for _, name := range r.ordering {
		schemas = append(schemas, r.tools[name].schema)
	}
	return schemas
}
