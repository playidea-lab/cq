package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start MCP server (stdio)",
	Long: `Start a Model Context Protocol server on stdin/stdout.
This allows AI agents to interact with C4 via the MCP protocol.

The server reads JSON-RPC requests from stdin and writes responses to stdout.`,
	RunE: runMCP,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

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

// toolInfo describes a single MCP tool.
type toolInfo struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

func runMCP(cmd *cobra.Command, args []string) error {
	if verbose {
		fmt.Fprintln(os.Stderr, "C4 MCP server starting on stdio...")
	}

	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	for {
		var req mcpRequest
		if err := decoder.Decode(&req); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("failed to decode request: %w", err)
		}

		resp := handleMCPRequest(&req)
		if resp != nil {
			if err := encoder.Encode(resp); err != nil {
				return fmt.Errorf("failed to encode response: %w", err)
			}
		}
	}
}

func handleMCPRequest(req *mcpRequest) *mcpResponse {
	switch req.Method {
	case "initialize":
		return &mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: initializeResult{
				ProtocolVersion: "2024-11-05",
				ServerInfo: serverInfo{
					Name:    "c4",
					Version: "0.1.0",
				},
			},
		}

	case "tools/list":
		tools := []toolInfo{
			{
				Name:        "c4_status",
				Description: "Show project state and task counts",
				InputSchema: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
			{
				Name:        "c4_start",
				Description: "Transition to EXECUTE state",
				InputSchema: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
			{
				Name:        "c4_stop",
				Description: "Transition to HALTED state",
				InputSchema: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		}
		return &mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]interface{}{"tools": tools},
		}

	case "tools/call":
		// Parse tool call params
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

		result, err := callTool(params.Name, params.Arguments)
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
			Result: map[string]interface{}{
				"content": []map[string]interface{}{
					{"type": "text", "text": result},
				},
			},
		}

	case "notifications/initialized":
		// Notification, no response needed
		return nil

	default:
		return &mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &mcpError{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)},
		}
	}
}

// callTool dispatches a tool call to the appropriate handler.
func callTool(name string, _ json.RawMessage) (string, error) {
	switch name {
	case "c4_status":
		return callStatusTool()
	case "c4_start":
		return callStartTool()
	case "c4_stop":
		return callStopTool()
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

func callStatusTool() (string, error) {
	db, err := openDB()
	if err != nil {
		return "", err
	}
	defer db.Close()

	state, err := loadProjectState(db)
	if err != nil {
		return "", err
	}

	counts, err := countTasks(db)
	if err != nil {
		return "", err
	}

	result, _ := json.Marshal(map[string]interface{}{
		"status":      state.Status,
		"project_id":  state.ProjectID,
		"total":       counts.Total,
		"pending":     counts.Pending,
		"in_progress": counts.InProgress,
		"done":        counts.Done,
		"blocked":     counts.Blocked,
	})
	return string(result), nil
}

func callStartTool() (string, error) {
	db, err := openDB()
	if err != nil {
		return "", err
	}
	defer db.Close()

	state, err := loadProjectState(db)
	if err != nil {
		return "", err
	}

	if state.Status != "PLAN" && state.Status != "HALTED" {
		return "", fmt.Errorf("cannot start from state %s", state.Status)
	}

	if err := transitionToExecute(db, state); err != nil {
		return "", err
	}

	return `{"status":"EXECUTE","success":true}`, nil
}

func callStopTool() (string, error) {
	db, err := openDB()
	if err != nil {
		return "", err
	}
	defer db.Close()

	state, err := loadProjectState(db)
	if err != nil {
		return "", err
	}

	if state.Status != "EXECUTE" {
		return "", fmt.Errorf("cannot stop from state %s", state.Status)
	}

	if err := transitionToHalted(db, state); err != nil {
		return "", err
	}

	return `{"status":"HALTED","success":true}`, nil
}

// openDB opens the tasks database (shared helper for MCP tools).
func openDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath())
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	return db, nil
}
