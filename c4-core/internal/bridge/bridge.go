// Package bridge implements the Python-Go bridge for gradual migration.
//
// During the migration period, the Go core communicates with the
// existing Python C4 system via gRPC or subprocess calls.
// This allows incremental replacement of Python components with Go.
package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// PythonBridge communicates with the Python C4 system via subprocess.
type PythonBridge struct {
	// Command is the Python interpreter command (e.g., "python3", "uv").
	Command string
	// Args are arguments passed to the command.
	Args []string
	// Timeout is the maximum time to wait for a response.
	Timeout time.Duration
}

// NewPythonBridge creates a bridge with default settings.
func NewPythonBridge() *PythonBridge {
	return &PythonBridge{
		Command: "uv",
		Args:    []string{"run", "python", "-m", "c4.mcp"},
		Timeout: 30 * time.Second,
	}
}

// Call sends a JSON-RPC request to the Python MCP server and returns the response.
func (b *PythonBridge) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if b.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, b.Timeout)
		defer cancel()
	}

	// Build JSON-RPC request
	request := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	}

	reqBytes, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Execute subprocess
	cmd := exec.CommandContext(ctx, b.Command, b.Args...)
	cmd.Stdin = bytes.NewReader(reqBytes)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("subprocess failed: %w (stderr: %s)", err, stderr.String())
	}

	// Parse JSON-RPC response
	var response struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      any             `json:"id"`
		Result  json.RawMessage `json:"result,omitempty"`
		Error   *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return nil, fmt.Errorf("parse response: %w (raw: %s)", err, stdout.String())
	}

	if response.Error != nil {
		return nil, fmt.Errorf("JSON-RPC error %d: %s", response.Error.Code, response.Error.Message)
	}

	return response.Result, nil
}

// IsAvailable checks if the Python MCP server is accessible.
func (b *PythonBridge) IsAvailable() bool {
	_, err := exec.LookPath(b.Command)
	return err == nil
}
