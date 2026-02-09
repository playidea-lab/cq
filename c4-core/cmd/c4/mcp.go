package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/changmin/c4-core/internal/bridge"
	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/mcp/handlers"
	_ "modernc.org/sqlite"
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
	sidecar  *bridge.Sidecar
	db       *sql.DB
}

// newMCPServer creates and initializes the MCP server with all tools registered.
func newMCPServer() (*mcpServer, error) {
	// Open SQLite database
	db, err := sql.Open("sqlite", dbPath())
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Load config (non-fatal on failure)
	cfgMgr, err := config.New(projectDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "c4: config load failed (using defaults): %v\n", err)
		cfgMgr = nil
	}

	// Try to start Python sidecar for proxy tools
	var sidecar *bridge.Sidecar
	var bridgeAddr string

	bridgeCfg := bridge.DefaultSidecarConfig()
	sidecar, err = bridge.StartSidecar(bridgeCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "c4: Python sidecar not available: %v\n", err)
		fmt.Fprintln(os.Stderr, "c4: LSP, Knowledge, and GPU tools will be unavailable")
		bridgeAddr = ""
	} else {
		bridgeAddr = sidecar.Addr()
		fmt.Fprintf(os.Stderr, "c4: Python sidecar started at %s\n", bridgeAddr)
	}

	// Create registry and register all tools (proxy is created inside)
	reg := mcp.NewRegistry()
	proxy := handlers.RegisterAllHandlers(reg, nil, projectDir, bridgeAddr)

	// Create store with all options
	storeOpts := []handlers.StoreOption{
		handlers.WithProjectRoot(projectDir),
	}
	if cfgMgr != nil {
		storeOpts = append(storeOpts, handlers.WithConfig(cfgMgr))
	}
	if proxy != nil {
		storeOpts = append(storeOpts, handlers.WithProxy(proxy))
	}
	store, err := handlers.NewSQLiteStore(db, storeOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating store: %w", err)
	}

	// Re-register handlers with the actual store (core + discovery + persona + soul tools)
	handlers.RegisterAll(reg, store)
	handlers.RegisterDiscoveryHandlers(reg, store, projectDir)
	handlers.RegisterPersonaHandlers(reg, store)
	handlers.RegisterTeamHandlers(reg, projectDir)
	handlers.RegisterSoulHandlers(reg, projectDir)
	handlers.RegisterTwinHandlers(reg, store)

	// Set project role for Soul stage integration
	projectName := filepath.Base(projectDir)
	if projectName != "" && projectName != "." {
		handlers.SetProjectRoleForStage("project-" + projectName)
	}

	// Wire sidecar auto-restart
	if sidecar != nil {
		proxy.SetRestarter(sidecar)
	}

	return &mcpServer{
		registry: reg,
		sidecar:  sidecar,
		db:       db,
	}, nil
}

// serve runs the stdio MCP server loop.
func (s *mcpServer) serve() error {
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	for {
		var req mcpRequest
		if err := decoder.Decode(&req); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("decoding request: %w", err)
		}

		resp := s.handleRequest(&req)
		if resp != nil {
			if err := encoder.Encode(resp); err != nil {
				return fmt.Errorf("encoding response: %w", err)
			}
		}
	}
}

// shutdown cleans up resources.
func (s *mcpServer) shutdown() {
	if s.sidecar != nil {
		_ = s.sidecar.Stop()
	}
	if s.db != nil {
		_ = s.db.Close()
	}
}

// handleRequest dispatches a JSON-RPC request.
func (s *mcpServer) handleRequest(req *mcpRequest) *mcpResponse {
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

		result, err := s.registry.Call(params.Name, params.Arguments)
		if err != nil {
			return &mcpResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &mcpError{Code: -32000, Message: err.Error()},
			}
		}

		// Marshal result to JSON text for MCP content response
		resultJSON, jsonErr := json.Marshal(result)
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
		fmt.Fprintln(os.Stderr, "c4: Go MCP server starting on stdio...")
	}

	srv, err := newMCPServer()
	if err != nil {
		return fmt.Errorf("initializing MCP server: %w", err)
	}
	defer srv.shutdown()

	tools := srv.registry.ListTools()
	fmt.Fprintf(os.Stderr, "c4: %d tools registered\n", len(tools))

	return srv.serve()
}

// openDB opens the tasks database (shared helper for CLI commands).
func openDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath())
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	return db, nil
}
