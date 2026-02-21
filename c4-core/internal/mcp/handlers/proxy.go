package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/mcp"
)

// KnowledgeSyncer abstracts cloud knowledge operations to avoid import cycles.
// Implemented by cloud.KnowledgeCloudClient.
type KnowledgeSyncer interface {
	SyncDocument(params map[string]any, docID string) error
	SearchDocuments(query string, docType string, limit int) ([]map[string]any, error)
	ListDocuments(docType string, limit int) ([]map[string]any, error)
	GetDocument(docID string) (map[string]any, error)
}

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

// LazyAddrGetter provides on-demand address resolution for the sidecar.
// Implemented by bridge.LazyStarter to enable lazy initialization.
type LazyAddrGetter interface {
	Addr() (string, error)
}

// Restarter is implemented by bridge.Sidecar to allow the proxy to trigger restarts.
type Restarter interface {
	Restart() (string, error)
}

// BridgeProxy forwards MCP tool calls to the Python sidecar via JSON-RPC over TCP.
type BridgeProxy struct {
	mu           sync.Mutex
	addrGetter   LazyAddrGetter // lazy address resolution (nil if using static addr)
	addr         string         // cached/static address
	timeout      time.Duration
	restarter    Restarter          // nil if no auto-restart support
	lastFailedAt time.Time          // timestamp of last connection failure
	eventPub     eventbus.Publisher // nil if eventbus not connected
}

// NewBridgeProxy creates a proxy that connects to the Python bridge sidecar.
// If addr is empty, proxy calls will fail immediately instead of timing out.
func NewBridgeProxy(addr string) *BridgeProxy {
	return &BridgeProxy{
		addr:    addr,
		timeout: 10 * time.Second,
	}
}

// NewBridgeProxyLazy creates a proxy with lazy address resolution.
// The sidecar will only start when the first proxy tool is called.
func NewBridgeProxyLazy(addrGetter LazyAddrGetter) *BridgeProxy {
	return &BridgeProxy{
		addrGetter: addrGetter,
		timeout:    10 * time.Second,
	}
}

// SetEventBus attaches an EventBus publisher for forwarding events from the Python sidecar.
// Python methods can include an "_events" field in their JSON-RPC response; the proxy
// extracts those events and publishes them asynchronously via the EventBus.
func (p *BridgeProxy) SetEventBus(pub eventbus.Publisher) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.eventPub = pub
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
// If a recent failure occurred (within 30s), proactively tries restart before the call.
func (p *BridgeProxy) Call(method string, params map[string]any) (map[string]any, error) {
	// Pre-call restart: if last call failed recently, try restart first
	p.mu.Lock()
	recentFail := !p.lastFailedAt.IsZero() && time.Since(p.lastFailedAt) < 30*time.Second
	p.mu.Unlock()
	if recentFail {
		if _, restartErr := p.tryRestart(); restartErr == nil {
			p.mu.Lock()
			p.lastFailedAt = time.Time{}
			p.mu.Unlock()
		}
	}

	result, err := p.doCall(method, params, p.timeout)
	if err != nil && p.isConnError(err) {
		p.mu.Lock()
		p.lastFailedAt = time.Now()
		p.mu.Unlock()
		// Try restart + retry once
		if newAddr, restartErr := p.tryRestart(); restartErr == nil {
			_ = newAddr
			p.mu.Lock()
			p.lastFailedAt = time.Time{}
			p.mu.Unlock()
			return p.doCall(method, params, p.timeout)
		}
	}
	return result, err
}

// CallWithTimeout is like Call but uses a custom timeout for long-running operations.
func (p *BridgeProxy) CallWithTimeout(method string, params map[string]any, timeout time.Duration) (map[string]any, error) {
	result, err := p.doCall(method, params, timeout)
	if err != nil && p.isConnError(err) {
		if newAddr, restartErr := p.tryRestart(); restartErr == nil {
			_ = newAddr
			return p.doCall(method, params, timeout)
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
func (p *BridgeProxy) doCall(method string, params map[string]any, timeout time.Duration) (map[string]any, error) {
	p.mu.Lock()
	addr := p.addr
	addrGetter := p.addrGetter
	p.mu.Unlock()

	// Lazy address resolution: call Addr() on first use
	if addr == "" && addrGetter != nil {
		var err error
		addr, err = addrGetter.Addr()
		if err != nil {
			return nil, fmt.Errorf("Python sidecar not available: %w", err)
		}
		// Cache the address for subsequent calls
		p.mu.Lock()
		p.addr = addr
		p.mu.Unlock()
	}

	if addr == "" {
		return nil, fmt.Errorf("Python sidecar not available. Restart your MCP client to reconnect.")
	}
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, newConnError("bridge connect: %w", err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))

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
					p.publishSidecarEvents(response.Result)
					return response.Result, nil
				}
			}
		}
		if err != nil {
			return nil, newConnError("read response: %w", err)
		}
	}
}

// publishSidecarEvents extracts the "_events" field from a Python sidecar response
// and publishes each event via the EventBus. The "_events" field is removed from the
// result so callers never see it.
func (p *BridgeProxy) publishSidecarEvents(result map[string]any) {
	if result == nil {
		return
	}
	events, ok := result["_events"]
	if !ok {
		return
	}
	delete(result, "_events")

	p.mu.Lock()
	pub := p.eventPub
	p.mu.Unlock()
	if pub == nil {
		return
	}

	evList, ok := events.([]any)
	if !ok {
		return
	}
	for _, ev := range evList {
		m, ok := ev.(map[string]any)
		if !ok {
			continue
		}
		evType, _ := m["type"].(string)
		source, _ := m["source"].(string)
		projectID, _ := m["project_id"].(string)
		if evType == "" {
			continue
		}
		data, _ := json.Marshal(m["data"])
		pub.PublishAsync(evType, source, data, projectID)
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

// proxyHandlerWithTimeout creates a proxy handler with a custom timeout for long-running operations.
func proxyHandlerWithTimeout(proxy *BridgeProxy, method string, timeout time.Duration) mcp.HandlerFunc {
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
		return proxy.CallWithTimeout(method, params, timeout)
	}
}

// parseParams extracts map[string]any from JSON-RPC raw arguments.
func parseParams(rawArgs json.RawMessage) map[string]any {
	var params map[string]any
	if len(rawArgs) > 0 {
		if err := json.Unmarshal(rawArgs, &params); err != nil {
			params = nil
		}
	}
	if params == nil {
		params = make(map[string]any)
	}
	return params
}

// knowledgeRecordHandler wraps the proxy call with async cloud sync.
// After Python returns success, the document data from params is pushed to cloud.
func knowledgeRecordHandler(proxy *BridgeProxy, rpcMethod string, kc KnowledgeSyncer) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		params := parseParams(rawArgs)
		result, err := proxy.Call(rpcMethod, params)
		if err != nil {
			return nil, err
		}

		// Async cloud push on success
		if kc != nil {
			if success, _ := result["success"].(bool); success {
				docID, _ := result["doc_id"].(string)
				go func() {
					if syncErr := kc.SyncDocument(params, docID); syncErr != nil {
						fmt.Fprintf(os.Stderr, "c4: knowledge cloud sync: %v\n", syncErr)
					}
				}()
			}
		}

		return result, nil
	}
}

// knowledgeSearchHandler wraps the proxy call and merges cloud search results.
func knowledgeSearchHandler(proxy *BridgeProxy, rpcMethod string, kc KnowledgeSyncer) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		params := parseParams(rawArgs)

		// Always search local first
		localResult, err := proxy.Call(rpcMethod, params)
		if err != nil {
			return nil, err
		}

		// Merge cloud results (best-effort)
		if kc != nil {
			query, _ := params["query"].(string)
			if query == "" {
				query, _ = params["context"].(string) // pattern_suggest uses "context"
			}
			docType, _ := params["doc_type"].(string)
			limit := 10
			if l, ok := params["limit"].(float64); ok {
				limit = int(l)
			}

			cloudDocs, cloudErr := kc.SearchDocuments(query, docType, limit)
			if cloudErr == nil && len(cloudDocs) > 0 {
				localResult["cloud_results"] = cloudDocs
				localResult["cloud_count"] = len(cloudDocs)
			}
		}

		return localResult, nil
	}
}

// knowledgePullHandler pulls documents from cloud to local store via Python RPC.
// Uses KnowledgeSyncer.ListDocuments for lightweight listing, then fetches full docs
// individually via GetDocument. Creates/updates local docs via BridgeProxy.Call("KnowledgeRecord")
// which bypasses the MCP handler (no cloud re-push).
func knowledgePullHandler(proxy *BridgeProxy, kc KnowledgeSyncer) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		if kc == nil {
			return nil, fmt.Errorf("cloud not configured")
		}
		params := parseParams(rawArgs)

		docType, _ := params["doc_type"].(string)
		limit := 50
		if l, ok := params["limit"].(float64); ok && l > 0 {
			limit = int(l)
		}
		force, _ := params["force"].(bool)

		// 1. List cloud docs (lightweight — no body)
		cloudDocs, err := kc.ListDocuments(docType, limit)
		if err != nil {
			return nil, fmt.Errorf("cloud list: %w", err)
		}

		var pulled, skipped, updated []string
		var pullErrors []string

		for _, cdoc := range cloudDocs {
			docID, _ := cdoc["doc_id"].(string)
			if docID == "" {
				continue
			}

			// 2. Check local existence via Python RPC
			localDoc, localErr := proxy.Call("KnowledgeGet", map[string]any{"doc_id": docID})
			localExists := localErr == nil && localDoc["error"] == nil

			if localExists && !force {
				// Compare version: cloud newer → update, otherwise skip
				cloudVer, _ := cdoc["version"].(float64)
				localVer := float64(0)
				if v, ok := localDoc["version"].(float64); ok {
					localVer = v
				}
				if cloudVer <= localVer {
					skipped = append(skipped, docID)
					continue
				}
			}

			// 3. Fetch full doc from cloud (includes body)
			fullDoc, getErr := kc.GetDocument(docID)
			if getErr != nil {
				pullErrors = append(pullErrors, fmt.Sprintf("%s: %v", docID, getErr))
				continue
			}

			// 4. Transform cloud row → KnowledgeRecord params
			tags := fullDoc["tags"]
			if tagsStr, ok := tags.(string); ok {
				var tagList []any
				if json.Unmarshal([]byte(tagsStr), &tagList) == nil {
					tags = tagList
				}
			}

			rpcParams := map[string]any{
				"id":       docID,
				"doc_type": fullDoc["doc_type"],
				"title":    fullDoc["title"],
				"body":     fullDoc["body"],
				"domain":   fullDoc["domain"],
				"tags":     tags,
			}

			// 5. Create/update local via Python RPC (bypasses MCP handler = no cloud re-push)
			result, recErr := proxy.Call("KnowledgeRecord", rpcParams)
			if recErr != nil {
				pullErrors = append(pullErrors, fmt.Sprintf("%s: %v", docID, recErr))
				continue
			}
			if success, _ := result["success"].(bool); success {
				if localExists {
					updated = append(updated, docID)
				} else {
					pulled = append(pulled, docID)
				}
			}
		}

		return map[string]any{
			"pulled":  len(pulled),
			"updated": len(updated),
			"skipped": len(skipped),
			"errors":  pullErrors,
			"details": map[string]any{
				"pulled_ids":  pulled,
				"updated_ids": updated,
				"skipped_ids": skipped,
			},
		}, nil
	}
}

// RegisterProxyHandlers registers MCP tools that are proxied to the Python sidecar.
// These tools require Python dependencies: LSP (7) + Onboard (1) = 8 tools.
// Knowledge tools (7) moved to Go native — see knowledge_native.go.
func RegisterProxyHandlers(reg *mcp.Registry, proxy *BridgeProxy, rootDir string) {
	// LSP tools (7) — Python Jedi/multilspy for Python/JS/TS, go/ast for Go
	// find_symbol + get_symbols_overview: Go-aware (auto-routes .go files to native parser)
	// replace/insert/rename/refs: Python/JS/TS only
	reg.Register(mcp.ToolSchema{
		Name:        "c4_find_symbol",
		Description: "Find symbol definitions by name across the project. Supports Python/JS/TS and Go. Name must be exact match (e.g. 'MyClass' not 'My'). Path is required to avoid timeout.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "description": "Symbol name to find (exact match, e.g. 'MyClass' or 'MyClass/method')"},
				"path": map[string]any{"type": "string", "description": "File or directory path to scope search (recommended — omitting may timeout)"},
			},
			"required": []string{"name"},
		},
	}, goAwareFindSymbol(proxy, rootDir))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_get_symbols_overview",
		Description: "Get overview of all symbols in a file. Supports Python/JS/TS, Go, and Dart",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "File path (Python/JS/TS only)"},
			},
			"required": []string{"path"},
		},
	}, goAwareSymbolsOverview(proxy, rootDir))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_replace_symbol_body",
		Description: "Replace the body of a symbol (function, class, method). Python/JS/TS only",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path":   map[string]any{"type": "string", "description": "File path"},
				"symbol_name": map[string]any{"type": "string", "description": "Symbol name"},
				"new_body":    map[string]any{"type": "string", "description": "New body content"},
			},
			"required": []string{"file_path", "symbol_name", "new_body"},
		},
	}, languageGuardedProxy(proxy, "ReplaceSymbolBody", "c4_replace_symbol_body"))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_insert_before_symbol",
		Description: "Insert content before a symbol definition. Python/JS/TS only",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path":   map[string]any{"type": "string", "description": "File path"},
				"symbol_name": map[string]any{"type": "string", "description": "Symbol name"},
				"content":     map[string]any{"type": "string", "description": "Content to insert"},
			},
			"required": []string{"file_path", "symbol_name", "content"},
		},
	}, languageGuardedProxy(proxy, "InsertBeforeSymbol", "c4_insert_before_symbol"))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_insert_after_symbol",
		Description: "Insert content after a symbol definition. Python/JS/TS only",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path":   map[string]any{"type": "string", "description": "File path"},
				"symbol_name": map[string]any{"type": "string", "description": "Symbol name"},
				"content":     map[string]any{"type": "string", "description": "Content to insert"},
			},
			"required": []string{"file_path", "symbol_name", "content"},
		},
	}, languageGuardedProxy(proxy, "InsertAfterSymbol", "c4_insert_after_symbol"))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_rename_symbol",
		Description: "Rename a symbol across all references. Python/JS/TS only",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{"type": "string", "description": "File path"},
				"old_name":  map[string]any{"type": "string", "description": "Current symbol name"},
				"new_name":  map[string]any{"type": "string", "description": "New symbol name"},
			},
			"required": []string{"file_path", "old_name", "new_name"},
		},
	}, languageGuardedProxy(proxy, "RenameSymbol", "c4_rename_symbol"))

	// LSP tool: find referencing symbols — delegated to Python
	reg.Register(mcp.ToolSchema{
		Name:        "c4_find_referencing_symbols",
		Description: "Find all references to a symbol across the project. Python/JS/TS only",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path":   map[string]any{"type": "string", "description": "File path containing the symbol"},
				"symbol_name": map[string]any{"type": "string", "description": "Symbol name to find references for"},
			},
			"required": []string{"file_path", "symbol_name"},
		},
	}, languageGuardedProxy(proxy, "FindReferencingSymbols", "c4_find_referencing_symbols"))

	// NOTE: Knowledge tools (7) moved to Go native — see knowledge_native.go
	// NOTE: GPU tools (2) moved to Go native — see gpu_native.go

	// Onboard tool — scans project structure via LSP/tree-sitter (30s timeout for large projects)
	reg.Register(mcp.ToolSchema{
		Name:        "c4_onboard",
		Description: "Scan project and generate pat-project-map.md (languages, symbols, dependencies)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"max_files": map[string]any{"type": "integer", "description": "Maximum files to scan (default: 500)"},
				"force":     map[string]any{"type": "boolean", "description": "Force regeneration even if map exists (default: false)"},
			},
		},
	}, proxyHandlerWithTimeout(proxy, "ProjectOnboard", 30*time.Second))
}

// registerKnowledgeProxy registers 7 knowledge tools as Python proxy fallback.
// Used when KnowledgeStore is unavailable (Go native not initialized).
func registerKnowledgeProxy(reg *mcp.Registry, proxy *BridgeProxy, knowledgeCloud KnowledgeSyncer) {
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
	}, knowledgeSearchHandler(proxy, "KnowledgeSearch", knowledgeCloud))

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
	}, knowledgeRecordHandler(proxy, "KnowledgeRecord", knowledgeCloud))

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
	}, knowledgeRecordHandler(proxy, "KnowledgeRecord", knowledgeCloud))

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
	}, knowledgeSearchHandler(proxy, "KnowledgeSearch", knowledgeCloud))

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
	}, knowledgeSearchHandler(proxy, "KnowledgeSearch", knowledgeCloud))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_knowledge_pull",
		Description: "Pull knowledge documents from cloud to local store",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"doc_type": map[string]any{"type": "string", "description": "Filter by type (experiment, pattern, insight, hypothesis)"},
				"limit":    map[string]any{"type": "integer", "description": "Max documents to pull (default: 50)"},
				"force":    map[string]any{"type": "boolean", "description": "Overwrite existing local docs (default: false)"},
			},
		},
	}, knowledgePullHandler(proxy, knowledgeCloud))
}
