// Package mcp implements the MCP (Model Context Protocol) server
// for communication between C4 and AI agents (Claude Code, Cursor, etc.).
//
// The MCP server exposes C4's functionality as tools that can be
// invoked by AI agents through the standardized MCP protocol.
package mcp

import (
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

// Registry manages registered MCP tools and dispatches calls.
type Registry struct {
	mu       sync.RWMutex
	tools    map[string]registeredTool
	ordering []string // preserve registration order
}

type registeredTool struct {
	schema  ToolSchema
	handler HandlerFunc
}

// NewRegistry creates a new empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]registeredTool),
	}
}

// Register adds a tool to the registry. If a tool with the same name
// is already registered, it logs a warning and skips the duplicate.
func (r *Registry) Register(schema ToolSchema, handler HandlerFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[schema.Name]; exists {
		fmt.Fprintf(os.Stderr, "mcp: warning: tool already registered, skipping: %s\n", schema.Name)
		return
	}

	r.tools[schema.Name] = registeredTool{
		schema:  schema,
		handler: handler,
	}
	r.ordering = append(r.ordering, schema.Name)
}

// Call invokes a registered tool by name with the given JSON arguments.
func (r *Registry) Call(name string, args json.RawMessage) (any, error) {
	r.mu.RLock()
	tool, ok := r.tools[name]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("mcp: unknown tool: %s", name)
	}

	return tool.handler(args)
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
	defer r.mu.Unlock()
	if _, exists := r.tools[schema.Name]; !exists {
		return false
	}
	r.tools[schema.Name] = registeredTool{schema: schema, handler: handler}
	return true
}

// Unregister removes a tool from the registry.
// Returns false if the tool is not registered.
func (r *Registry) Unregister(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[name]; !exists {
		return false
	}
	delete(r.tools, name)
	for i, n := range r.ordering {
		if n == name {
			r.ordering = append(r.ordering[:i], r.ordering[i+1:]...)
			break
		}
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
