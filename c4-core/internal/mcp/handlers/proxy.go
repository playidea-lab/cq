package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/changmin/c4-core/internal/mcp"
)

// connError wraps connection-level errors that indicate the sidecar is unreachable.
// Used as a sentinel type to trigger auto-restart without string matching.
type connError struct {
	wrapped error
}

func (e *connError) Error() string { return e.wrapped.Error() }
func (e *connError) Unwrap() error { return e.wrapped }

func newConnError(format string, a ...any) error {
	return &connError{wrapped: fmt.Errorf(format, a...)}
}

// Restarter is implemented by bridge.Sidecar to allow the proxy to trigger restarts.
type Restarter interface {
	Restart() (string, error)
}

// BridgeProxy forwards MCP tool calls to the Python sidecar via JSON-RPC over TCP.
type BridgeProxy struct {
	mu        sync.Mutex
	addr      string
	timeout   time.Duration
	restarter Restarter // nil if no auto-restart support
}

// NewBridgeProxy creates a proxy that connects to the Python bridge sidecar.
// If addr is empty, proxy calls will fail immediately instead of timing out.
func NewBridgeProxy(addr string) *BridgeProxy {
	return &BridgeProxy{
		addr:    addr,
		timeout: 10 * time.Second,
	}
}

// SetRestarter attaches a Restarter (typically *bridge.Sidecar) for auto-recovery.
func (p *BridgeProxy) SetRestarter(r Restarter) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.restarter = r
}

// UpdateAddr updates the sidecar address (called after restart).
func (p *BridgeProxy) UpdateAddr(addr string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.addr = addr
}

// Call sends a JSON-RPC request to the Python sidecar and returns the result.
// On connection failure, attempts to restart the sidecar once and retry.
func (p *BridgeProxy) Call(method string, params map[string]any) (map[string]any, error) {
	result, err := p.doCall(method, params)
	if err != nil && p.isConnError(err) {
		// Try restart + retry once
		if newAddr, restartErr := p.tryRestart(); restartErr == nil {
			_ = newAddr
			return p.doCall(method, params)
		}
	}
	return result, err
}

// isConnError returns true for errors that indicate the sidecar is unreachable.
func (p *BridgeProxy) isConnError(err error) bool {
	var ce *connError
	return errors.As(err, &ce)
}

// tryRestart attempts to restart the sidecar via the Restarter interface.
func (p *BridgeProxy) tryRestart() (string, error) {
	p.mu.Lock()
	r := p.restarter
	p.mu.Unlock()

	if r == nil {
		return "", fmt.Errorf("no restarter configured")
	}

	newAddr, err := r.Restart()
	if err != nil {
		return "", err
	}

	p.UpdateAddr(newAddr)
	return newAddr, nil
}

// maxResponseSize is the maximum allowed response size from the sidecar (10 MB).
const maxResponseSize = 10 * 1024 * 1024

// doCall performs a single JSON-RPC call without retry.
func (p *BridgeProxy) doCall(method string, params map[string]any) (map[string]any, error) {
	p.mu.Lock()
	addr := p.addr
	p.mu.Unlock()

	if addr == "" {
		return nil, fmt.Errorf("Python sidecar not available (no bridge address)")
	}
	conn, err := net.DialTimeout("tcp", addr, p.timeout)
	if err != nil {
		return nil, newConnError("bridge connect: %w", err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(p.timeout))

	// Send request
	request := map[string]any{
		"method": method,
		"params": params,
	}
	reqBytes, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	reqBytes = append(reqBytes, '\n')

	if _, err := conn.Write(reqBytes); err != nil {
		return nil, newConnError("write request: %w", err)
	}

	// Read response (line-delimited JSON)
	buf := make([]byte, 0, 64*1024)
	tmp := make([]byte, 4096)
	for {
		n, err := conn.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			if len(buf) > maxResponseSize {
				return nil, fmt.Errorf("response exceeds %d bytes limit", maxResponseSize)
			}
			// Check for newline delimiter
			for i := range buf {
				if buf[i] == '\n' {
					// Parse response
					var response struct {
						Result map[string]any `json:"result"`
						Error  *string        `json:"error"`
					}
					if err := json.Unmarshal(buf[:i], &response); err != nil {
						return nil, fmt.Errorf("parse response: %w", err)
					}
					if response.Error != nil {
						return nil, fmt.Errorf("bridge error: %s", *response.Error)
					}
					return response.Result, nil
				}
			}
		}
		if err != nil {
			return nil, newConnError("read response: %w", err)
		}
	}
}

// IsAvailable checks if the Python sidecar is reachable.
func (p *BridgeProxy) IsAvailable() bool {
	p.mu.Lock()
	addr := p.addr
	p.mu.Unlock()

	if addr == "" {
		return false
	}
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// proxyHandler creates a generic MCP handler that proxies to the Python sidecar.
func proxyHandler(proxy *BridgeProxy, method string) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		var params map[string]any
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &params); err != nil {
				return nil, fmt.Errorf("parsing arguments: %w", err)
			}
		}
		if params == nil {
			params = make(map[string]any)
		}
		return proxy.Call(method, params)
	}
}

// RegisterProxyHandlers registers MCP tools that are proxied to the Python sidecar.
// These tools require Python dependencies (LSP, Knowledge, GPU).
func RegisterProxyHandlers(reg *mcp.Registry, proxy *BridgeProxy) {
	// LSP tools (6) — delegated to Python tree-sitter + multilspy + Jedi
	reg.Register(mcp.ToolSchema{
		Name:        "c4_find_symbol",
		Description: "Find symbol definitions by name across the project",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "description": "Symbol name to find"},
				"path": map[string]any{"type": "string", "description": "Optional file path to scope the search"},
			},
			"required": []string{"name"},
		},
	}, proxyHandler(proxy, "FindSymbol"))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_get_symbols_overview",
		Description: "Get overview of all symbols in a file",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "File path"},
			},
			"required": []string{"path"},
		},
	}, proxyHandler(proxy, "GetSymbolsOverview"))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_replace_symbol_body",
		Description: "Replace the body of a symbol (function, class, method)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path":   map[string]any{"type": "string", "description": "File path"},
				"symbol_name": map[string]any{"type": "string", "description": "Symbol name"},
				"new_body":    map[string]any{"type": "string", "description": "New body content"},
			},
			"required": []string{"file_path", "symbol_name", "new_body"},
		},
	}, proxyHandler(proxy, "ReplaceSymbolBody"))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_insert_before_symbol",
		Description: "Insert content before a symbol definition",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path":   map[string]any{"type": "string", "description": "File path"},
				"symbol_name": map[string]any{"type": "string", "description": "Symbol name"},
				"content":     map[string]any{"type": "string", "description": "Content to insert"},
			},
			"required": []string{"file_path", "symbol_name", "content"},
		},
	}, proxyHandler(proxy, "InsertBeforeSymbol"))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_insert_after_symbol",
		Description: "Insert content after a symbol definition",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path":   map[string]any{"type": "string", "description": "File path"},
				"symbol_name": map[string]any{"type": "string", "description": "Symbol name"},
				"content":     map[string]any{"type": "string", "description": "Content to insert"},
			},
			"required": []string{"file_path", "symbol_name", "content"},
		},
	}, proxyHandler(proxy, "InsertAfterSymbol"))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_rename_symbol",
		Description: "Rename a symbol across all references",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{"type": "string", "description": "File path"},
				"old_name":  map[string]any{"type": "string", "description": "Current symbol name"},
				"new_name":  map[string]any{"type": "string", "description": "New symbol name"},
			},
			"required": []string{"file_path", "old_name", "new_name"},
		},
	}, proxyHandler(proxy, "RenameSymbol"))

	// LSP tool: find referencing symbols — delegated to Python
	reg.Register(mcp.ToolSchema{
		Name:        "c4_find_referencing_symbols",
		Description: "Find all references to a symbol across the project",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path":   map[string]any{"type": "string", "description": "File path containing the symbol"},
				"symbol_name": map[string]any{"type": "string", "description": "Symbol name to find references for"},
			},
			"required": []string{"file_path", "symbol_name"},
		},
	}, proxyHandler(proxy, "FindReferencingSymbols"))

	// Knowledge tools (3) — delegated to Python FTS5 + sqlite-vec
	reg.Register(mcp.ToolSchema{
		Name:        "c4_knowledge_search",
		Description: "Search knowledge base documents with hybrid vector + FTS search",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":    map[string]any{"type": "string", "description": "Search query"},
				"doc_type": map[string]any{"type": "string", "description": "Filter by type (experiment, pattern, insight, hypothesis)"},
				"limit":    map[string]any{"type": "integer", "description": "Max results (default: 10)"},
			},
			"required": []string{"query"},
		},
	}, proxyHandler(proxy, "KnowledgeSearch"))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_knowledge_record",
		Description: "Record a new knowledge document",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"doc_type": map[string]any{"type": "string", "description": "Document type: experiment, pattern, insight, hypothesis"},
				"title":    map[string]any{"type": "string", "description": "Document title"},
				"content":  map[string]any{"type": "string", "description": "Document content (markdown)"},
				"tags":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional tags"},
			},
			"required": []string{"doc_type", "title", "content"},
		},
	}, proxyHandler(proxy, "KnowledgeRecord"))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_knowledge_get",
		Description: "Get a knowledge document by ID",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"doc_id": map[string]any{"type": "string", "description": "Document ID"},
			},
			"required": []string{"doc_id"},
		},
	}, proxyHandler(proxy, "KnowledgeGet"))

	// Knowledge legacy tools (3) — delegated to Python knowledge store
	reg.Register(mcp.ToolSchema{
		Name:        "c4_experiment_record",
		Description: "Record an experiment result",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":   map[string]any{"type": "string", "description": "Experiment title"},
				"content": map[string]any{"type": "string", "description": "Experiment details and results"},
				"tags":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
			"required": []string{"title", "content"},
		},
	}, proxyHandler(proxy, "KnowledgeRecord"))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_experiment_search",
		Description: "Search experiment records",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "Search query"},
				"limit": map[string]any{"type": "integer", "description": "Max results"},
			},
			"required": []string{"query"},
		},
	}, proxyHandler(proxy, "KnowledgeSearch"))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_pattern_suggest",
		Description: "Get pattern suggestions based on current context",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"context": map[string]any{"type": "string", "description": "Current context or problem description"},
			},
			"required": []string{"context"},
		},
	}, proxyHandler(proxy, "KnowledgeSearch"))

	// GPU tools (2) — delegated to Python CUDA/MPS detection
	reg.Register(mcp.ToolSchema{
		Name:        "c4_gpu_status",
		Description: "Get GPU device status and availability",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, proxyHandler(proxy, "GPUStatus"))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_job_submit",
		Description: "Submit a job to the GPU scheduler",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command":  map[string]any{"type": "string", "description": "Command to run"},
				"gpu_id":   map[string]any{"type": "integer", "description": "Specific GPU ID (optional)"},
				"priority": map[string]any{"type": "integer", "description": "Job priority (default: 5)"},
			},
			"required": []string{"command"},
		},
	}, proxyHandler(proxy, "JobSubmit"))
}
