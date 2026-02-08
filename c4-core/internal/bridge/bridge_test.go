package bridge

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestMCPBridgeCallMockProcess tests the bridge using a mock subprocess
// that echoes a JSON-RPC response on stdout.
func TestMCPBridgeCallMockProcess(t *testing.T) {
	// Create a mock "Python" script that reads JSON-RPC from stdin
	// and writes a valid response to stdout.
	tmpDir := t.TempDir()
	mockScript := filepath.Join(tmpDir, "mock_mcp.sh")

	script := `#!/bin/sh
# Read stdin (JSON-RPC request) and respond with a status result
cat > /dev/null
echo '{"jsonrpc":"2.0","id":1,"result":{"status":"EXECUTE","project_id":"test"}}'
`
	if err := os.WriteFile(mockScript, []byte(script), 0o755); err != nil {
		t.Fatalf("write mock script: %v", err)
	}

	bridge := &PythonBridge{
		Command: "sh",
		Args:    []string{mockScript},
		Timeout: 5 * time.Second,
	}

	ctx := context.Background()
	result, err := bridge.Call(ctx, "tools/call", map[string]any{
		"name":      "c4_status",
		"arguments": map[string]any{},
	})
	if err != nil {
		t.Fatalf("Call error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if resp["status"] != "EXECUTE" {
		t.Errorf("status = %v, want EXECUTE", resp["status"])
	}
	if resp["project_id"] != "test" {
		t.Errorf("project_id = %v, want test", resp["project_id"])
	}
}

// TestMCPBridgeCallError tests that bridge reports JSON-RPC errors correctly.
func TestMCPBridgeCallError(t *testing.T) {
	tmpDir := t.TempDir()
	mockScript := filepath.Join(tmpDir, "mock_error.sh")

	script := `#!/bin/sh
cat > /dev/null
echo '{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"method not found"}}'
`
	if err := os.WriteFile(mockScript, []byte(script), 0o755); err != nil {
		t.Fatalf("write mock script: %v", err)
	}

	bridge := &PythonBridge{
		Command: "sh",
		Args:    []string{mockScript},
		Timeout: 5 * time.Second,
	}

	_, err := bridge.Call(context.Background(), "unknown/method", nil)
	if err == nil {
		t.Fatal("expected error for JSON-RPC error response")
	}

	if !contains(err.Error(), "method not found") {
		t.Errorf("error = %v, want to contain 'method not found'", err)
	}
}

// TestMCPBridgeSubprocessFailure tests that bridge handles subprocess crashes.
func TestMCPBridgeSubprocessFailure(t *testing.T) {
	bridge := &PythonBridge{
		Command: "false", // exits with code 1
		Args:    nil,
		Timeout: 5 * time.Second,
	}

	_, err := bridge.Call(context.Background(), "test", nil)
	if err == nil {
		t.Fatal("expected error for subprocess failure")
	}

	if !contains(err.Error(), "subprocess failed") {
		t.Errorf("error = %v, want to contain 'subprocess failed'", err)
	}
}

// TestMCPBridgeTimeout tests that bridge enforces timeout.
func TestMCPBridgeTimeout(t *testing.T) {
	bridge := &PythonBridge{
		Command: "sleep",
		Args:    []string{"10"},
		Timeout: 50 * time.Millisecond,
	}

	_, err := bridge.Call(context.Background(), "test", nil)
	if err == nil {
		t.Fatal("expected error for timeout")
	}
}

// TestMCPBridgeIsAvailable tests availability check.
func TestMCPBridgeIsAvailable(t *testing.T) {
	// "sh" should be available on all Unix systems
	bridge := &PythonBridge{Command: "sh"}
	if !bridge.IsAvailable() {
		t.Error("sh should be available")
	}

	// Nonexistent command
	bridge = &PythonBridge{Command: "nonexistent_command_xyz"}
	if bridge.IsAvailable() {
		t.Error("nonexistent command should not be available")
	}
}

// TestMCPBridgeNewDefault tests default configuration.
func TestMCPBridgeNewDefault(t *testing.T) {
	bridge := NewPythonBridge()
	if bridge.Command != "uv" {
		t.Errorf("Command = %q, want 'uv'", bridge.Command)
	}
	if bridge.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", bridge.Timeout)
	}

	// Check if uv is available (informational, not a test failure)
	if _, err := exec.LookPath("uv"); err != nil {
		t.Logf("uv not found in PATH (expected in some environments)")
	}
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
