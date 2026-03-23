package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/changmin/c4-core/internal/bridge"
	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/mcp/apps"
	"github.com/changmin/c4-core/internal/secrets"
)

// NOTE: The "mcp" subcommand is registered by fallback.go (mcpFallbackCmd)
// which wraps the Go MCP server with a Python fallback mechanism.
// The runMCP function below is the core Go MCP server implementation.

// mcpRequest represents a JSON-RPC 2.0 request.
type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// mcpResponse represents a JSON-RPC 2.0 response.
type mcpResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *mcpError   `json:"error,omitempty"`
}

// mcpError represents a JSON-RPC 2.0 error.
type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// serverInfo is returned on initialize.
type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// initializeResult is the response to the initialize method.
type initializeResult struct {
	ProtocolVersion string     `json:"protocolVersion"`
	ServerInfo      serverInfo `json:"serverInfo"`
	Capabilities    struct {
		Tools struct{} `json:"tools"`
	} `json:"capabilities"`
}

// mcpServer holds the state of a running MCP server instance.
type mcpServer struct {
	registry *mcp.Registry
	sidecar  *bridge.LazyStarter // lazy-initialized Python sidecar
	db       *sql.DB
	// initCtx holds all component-specific state; shutdown delegates to componentShutdownHooks.
	initCtx        *initContext
	knowledgeStore *knowledge.Store        // Go native knowledge store (Tier 2)
	knowledgeUsage *knowledge.UsageTracker // usage tracking for 3-way RRF
	secretStore    *secrets.Store          // global encrypted secret store (~/.c4/secrets.db)
	resourceStore  *apps.ResourceStore     // MCP Apps ui:// resource store
}

// serve runs the stdio MCP server loop with concurrent request handling.
// Requests are read sequentially from stdin but handled concurrently in goroutines.
// Responses are written back through a mutex-protected encoder to preserve stdio integrity.
// This prevents slow sidecar proxy calls (e.g. c4_find_symbol) from blocking
// fast native operations (e.g. c4_status) — eliminating head-of-line blocking.
func (s *mcpServer) serve() error {
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	var writerMu sync.Mutex

	// pending tracks in-flight tools/call requests for cancellation.
	// Key: fmt.Sprint(request.ID), Value: context cancel func.
	var pendingMu sync.Mutex
	pending := make(map[string]context.CancelFunc)

	// Wire up tools/list_changed notification so clients re-fetch after lighthouse register.
	s.registry.OnChange = func() {
		writerMu.Lock()
		_ = encoder.Encode(map[string]any{
			"jsonrpc": "2.0",
			"method":  "notifications/tools/list_changed",
		})
		writerMu.Unlock()
	}

	for {
		var req mcpRequest
		if err := decoder.Decode(&req); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("decoding request: %w", err)
		}

		// Notifications (no ID): handle inline without a response.
		if req.ID == nil {
			// notifications/cancelled: cancel the corresponding in-flight request.
			if req.Method == "notifications/cancelled" {
				var params struct {
					RequestID interface{} `json:"requestId"`
				}
				if err := json.Unmarshal(req.Params, &params); err == nil && params.RequestID != nil {
					key := fmt.Sprint(params.RequestID)
					pendingMu.Lock()
					if cancel, ok := pending[key]; ok {
						cancel()
						delete(pending, key)
					}
					pendingMu.Unlock()
				}
			} else {
				_ = s.handleRequest(&req)
			}
			continue
		}

		// Handle each request concurrently to avoid head-of-line blocking.
		go func(r mcpRequest) {
			key := fmt.Sprint(r.ID)

			// Recover from panics in handler goroutines to prevent server crash.
			// An unrecovered panic in any goroutine kills the entire process,
			// causing Claude Code to see "Connection closed".
			defer func() {
				if rec := recover(); rec != nil {
					fmt.Fprintf(os.Stderr, "cq: panic in handler goroutine (id=%s): %v\n", key, rec)
					if r.ID != nil {
						errResp := &mcpResponse{
							JSONRPC: "2.0",
							ID:      r.ID,
							Error:   &mcpError{Code: -32000, Message: fmt.Sprintf("internal error: %v", rec)},
						}
						writerMu.Lock()
						_ = encoder.Encode(errResp)
						writerMu.Unlock()
					}
				}
			}()

			// For tools/call, create a cancellable context and register it.
			ctx := context.Background()
			if r.Method == "tools/call" {
				var callCtx context.Context
				var cancel context.CancelFunc
				callCtx, cancel = context.WithCancel(context.Background())
				ctx = callCtx
				pendingMu.Lock()
				pending[key] = cancel
				pendingMu.Unlock()
				defer func() {
					pendingMu.Lock()
					delete(pending, key)
					pendingMu.Unlock()
					cancel()
				}()
			}

			resp := s.handleRequestWithCtx(&r, ctx)
			if resp != nil {
				writerMu.Lock()
				_ = encoder.Encode(resp)
				writerMu.Unlock()
			}
		}(req)
	}
}

// shutdown cleans up resources.
// Component-specific cleanup is delegated to componentShutdownHooks registered by each
// build-tagged init file (mcp_init_*.go). Core resources (sidecar, knowledge, db) are
// cleaned up here since they are always present regardless of build tags.
func (s *mcpServer) shutdown() {
	// Run component shutdown hooks in reverse registration order.
	for i := len(componentShutdownHooks) - 1; i >= 0; i-- {
		if s.initCtx != nil {
			componentShutdownHooks[i](s.initCtx)
		}
	}
	if s.sidecar != nil {
		_ = s.sidecar.Stop()
	}
	if s.knowledgeUsage != nil {
		s.knowledgeUsage.Close()
	}
	if s.knowledgeStore != nil {
		s.knowledgeStore.Close()
	}
	if s.secretStore != nil {
		_ = s.secretStore.Close()
	}
	if s.db != nil {
		_ = s.db.Close()
	}
}

// handleRequest dispatches a JSON-RPC request.
// handleRequest handles a request without a specific context (used for notifications).
func (s *mcpServer) handleRequest(req *mcpRequest) *mcpResponse {
	return s.handleRequestWithCtx(req, context.Background())
}

// handleRequestWithCtx handles a request with the given context.
// For tools/call, ctx is passed to blocking handlers to support cancellation.
func (s *mcpServer) handleRequestWithCtx(req *mcpRequest, ctx context.Context) *mcpResponse {
	switch req.Method {
	case "initialize":
		return &mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: initializeResult{
				ProtocolVersion: "2024-11-05",
				ServerInfo: serverInfo{
					Name:    "c4",
					Version: version,
				},
			},
		}

	case "tools/list":
		schemas := s.registry.ListTools()
		tools := make([]map[string]any, 0, len(schemas))
		for _, t := range schemas {
			tools = append(tools, map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"inputSchema": t.InputSchema,
			})
		}
		return &mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{"tools": tools},
		}

	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return &mcpResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &mcpError{Code: -32602, Message: "invalid params"},
			}
		}

		result, err := s.registry.CallWithContext(ctx, params.Name, params.Arguments)
		if err != nil {
			return &mcpResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &mcpError{Code: -32000, Message: err.Error()},
			}
		}

		// Marshal result to indented JSON for readability in tool responses
		resultJSON, jsonErr := json.MarshalIndent(result, "", "  ")
		if jsonErr != nil {
			return &mcpResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &mcpError{Code: -32000, Message: "failed to serialize result"},
			}
		}

		return &mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": string(resultJSON)},
				},
			},
		}

	case "resources/read":
		var params struct {
			URI string `json:"uri"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return &mcpResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &mcpError{Code: -32602, Message: "invalid params"},
			}
		}
		if s.resourceStore == nil {
			return &mcpResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &mcpError{Code: -32000, Message: "resource store not available"},
			}
		}
		content, mimeType, err := s.resourceStore.HandleResourcesRead(params.URI)
		if err != nil {
			return &mcpResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &mcpError{Code: -32000, Message: err.Error()},
			}
		}
		return &mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"contents": []map[string]any{
					{"uri": params.URI, "mimeType": mimeType, "text": content},
				},
			},
		}

	case "notifications/initialized":
		return nil

	default:
		return &mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &mcpError{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)},
		}
	}
}

// runMCP creates and runs the Go MCP server.
// This is called from fallback.go's startGoMCPServer.
func runMCP() error {
	if verbose {
		fmt.Fprintln(os.Stderr, "cq: Go MCP server starting on stdio...")
	}

	srv, err := newMCPServer()
	if err != nil {
		return fmt.Errorf("initializing MCP server: %w", err)
	}
	// Protect against double-close if signal and stdin-EOF occur concurrently.
	var shutdownOnce sync.Once
	doShutdown := func() { shutdownOnce.Do(srv.shutdown) }
	defer doShutdown()

	// Signal handler: ensure sidecar cleanup on SIGTERM/SIGINT.
	// Without this, signals terminate the process before defer runs,
	// leaving orphan sidecar processes.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		fmt.Fprintf(os.Stderr, "cq: received %s, shutting down\n", sig)
		doShutdown()
		os.Exit(0)
	}()

	tools := srv.registry.ListTools()
	fmt.Fprintf(os.Stderr, "cq: %d tools registered\n", len(tools))

	return srv.serve()
}
