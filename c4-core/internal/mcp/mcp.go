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

// ToolSchemaMeta carries optional MCP Apps metadata for a tool.
// When set, tool responses may include _meta.ui.resourceUri pointing to a ui:// resource.
type ToolSchemaMeta struct {
	UI *ToolSchemaUI `json:"ui,omitempty"`
}

// ToolSchemaUI describes the UI resource associated with a tool's response.
type ToolSchemaUI struct {
	ResourceUri string `json:"resourceUri"` // e.g. "ui://c4/task-card"
}

// ToolSchema describes a single MCP tool's metadata.
type ToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema map[string]any  `json:"inputSchema"`
	Meta        *ToolSchemaMeta `json:"_meta,omitempty"` // optional MCP Apps metadata
}

// toolNameKey is an unexported context key used to carry the dispatched tool name
// through the middleware chain, enabling observe.Middleware to perform per-tool tracking.
type toolNameKey struct{}

// ToolNameFromContext returns the tool name stored in ctx by CallWithContext.
// Returns "" when called outside a middleware chain.
func ToolNameFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(toolNameKey{}).(string); ok {
		return v
	}
	return ""
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
	ctxMws      []ContextualMiddleware // context-aware middlewares applied with ctx+name

	// VisibleTools, when non-nil, restricts which tools are returned by ListTools().
	// Tools not in the set are still callable (skills/workers invoke them directly)
	// but won't appear in tools/list. Set via SetVisibleTools().
	VisibleTools map[string]bool

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

// UseContextual appends context-aware middlewares.
// Contextual middlewares receive the call context (with tool name via ToolNameFromContext)
// and tool name, and are applied innermost after plain Middlewares.
func (r *Registry) UseContextual(mw ...ContextualMiddleware) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ctxMws = append(r.ctxMws, mw...)
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
// The tool name is injected into ctx via toolNameKey for observability middleware.
func (r *Registry) CallWithContext(ctx context.Context, name string, args json.RawMessage) (any, error) {
	r.mu.RLock()
	tool, ok := r.tools[name]
	onCall := r.OnCall
	mws := r.middlewares   // snapshot under lock
	ctxMws := r.ctxMws     // snapshot under lock
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("mcp: unknown tool: %s", name)
	}

	if onCall != nil {
		onCall()
	}

	// Inject tool name into context so contextual and observability middlewares can read it.
	ctx = context.WithValue(ctx, toolNameKey{}, name)

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

	// Apply contextual middlewares first (innermost), in reverse registration order.
	h := base
	for i := len(ctxMws) - 1; i >= 0; i-- {
		h = ctxMws[i](ctx, name, h)
	}

	// Apply plain middlewares (outermost), in reverse registration order.
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}

	return h(args)
}

// Dispatch is a convenience alias for CallWithContext.
func (r *Registry) Dispatch(ctx context.Context, name string, args json.RawMessage) (any, error) {
	return r.CallWithContext(ctx, name, args)
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

// SetVisibleTools restricts ListTools() to only return the named tools.
// Pass nil to show all tools (default). Tools not in the set remain callable.
func (r *Registry) SetVisibleTools(names []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(names) == 0 {
		r.VisibleTools = nil
		return
	}
	r.VisibleTools = make(map[string]bool, len(names))
	for _, n := range names {
		r.VisibleTools[n] = true
	}
}

// ListTools returns registered tool schemas in registration order.
// If VisibleTools is set, only those tools are returned.
// All tools remain callable regardless of visibility.
func (r *Registry) ListTools() []ToolSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()

	schemas := make([]ToolSchema, 0, len(r.ordering))
	for _, name := range r.ordering {
		if r.VisibleTools != nil && !r.VisibleTools[name] {
			continue
		}
		schemas = append(schemas, r.tools[name].schema)
	}
	return schemas
}

// ListAllTools returns all registered tool schemas, ignoring VisibleTools filter.
func (r *Registry) ListAllTools() []ToolSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()

	schemas := make([]ToolSchema, 0, len(r.ordering))
	for _, name := range r.ordering {
		schemas = append(schemas, r.tools[name].schema)
	}
	return schemas
}
