package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// TestE2EVersion verifies the --version flag outputs version info.
func TestE2EVersion(t *testing.T) {
	// Use rootCmd directly rather than building a binary
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"--version"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute --version: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "c4 version") {
		t.Errorf("--version output = %q, want to contain 'c4 version'", output)
	}
}

// TestE2EStatusWithDB verifies the status command reads from .c4/tasks.db.
func TestE2EStatusWithDB(t *testing.T) {
	// Setup a temporary project directory with .c4/tasks.db
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Insert state and tasks
	insertState(t, db, "e2e-project", "EXECUTE")
	insertTask(t, db, "T-001-0", "Task one", "done")
	insertTask(t, db, "T-002-0", "Task two", "pending")
	insertTask(t, db, "T-003-0", "Task three", "in_progress")

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runStatus(nil, nil)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("runStatus: %v", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Check output contains project info
	if !strings.Contains(output, "EXECUTE") {
		t.Errorf("status output should contain EXECUTE, got:\n%s", output)
	}
	if !strings.Contains(output, "e2e-project") {
		t.Errorf("status output should contain project name, got:\n%s", output)
	}
	// Check task counts
	if !strings.Contains(output, "3") {
		t.Errorf("status output should contain total task count 3, got:\n%s", output)
	}
}

// TestE2EMCPStdioInitialize verifies the MCP server handles
// the initialize JSON-RPC method correctly.
func TestE2EMCPStdioInitialize(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	srv := newTestMCPServer(t, db)

	reqJSON := `{"jsonrpc":"2.0","id":1,"method":"initialize"}`
	var req mcpRequest
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	resp := srv.handleRequest(&req)
	if resp == nil {
		t.Fatal("nil response")
	}
	if resp.Error != nil {
		t.Fatalf("error: code=%d msg=%s", resp.Error.Code, resp.Error.Message)
	}

	// Serialize and verify it's valid JSON-RPC
	respBytes, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed struct {
		JSONRPC string      `json:"jsonrpc"`
		ID      interface{} `json:"id"`
		Result  interface{} `json:"result"`
	}
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		t.Fatalf("parse response: %v", err)
	}

	if parsed.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q, want 2.0", parsed.JSONRPC)
	}

	// Verify server info in result
	result, ok := parsed.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T, want map", parsed.Result)
	}
	serverInfo, ok := result["serverInfo"].(map[string]interface{})
	if !ok {
		t.Fatalf("serverInfo type = %T, want map", result["serverInfo"])
	}
	if serverInfo["name"] != "c4" {
		t.Errorf("server name = %v, want c4", serverInfo["name"])
	}
}

// TestE2EMCPStdioToolsList verifies the MCP server returns tool definitions.
func TestE2EMCPStdioToolsList(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	srv := newTestMCPServer(t, db)

	req := &mcpRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
	}

	resp := srv.handleRequest(req)
	if resp == nil {
		t.Fatal("nil response")
	}
	if resp.Error != nil {
		t.Fatalf("error: %v", resp.Error)
	}

	resultMap, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map", resp.Result)
	}

	tools, ok := resultMap["tools"].([]map[string]any)
	if !ok {
		t.Fatalf("tools type = %T, want []map[string]any", resultMap["tools"])
	}

	if len(tools) < 3 {
		t.Errorf("expected at least 3 tools, got %d", len(tools))
	}

	// Verify expected tools exist
	names := make(map[string]bool)
	for _, tool := range tools {
		if name, ok := tool["name"].(string); ok {
			names[name] = true
		}
	}
	for _, expected := range []string{"c4_status", "c4_start", "c4_clear"} {
		if !names[expected] {
			t.Errorf("expected tool %q not found in %v", expected, names)
		}
	}
}

// TestE2EMCPStatusTool verifies the c4_status MCP tool returns project data.
func TestE2EMCPStatusTool(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	insertState(t, db, "e2e-mcp", "EXECUTE")
	insertTask(t, db, "T-001-0", "Task one", "done")
	insertTask(t, db, "T-002-0", "Task two", "pending")

	srv := newTestMCPServer(t, db)

	req := &mcpRequest{
		JSONRPC: "2.0",
		ID:      10,
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "c4_status",
			"arguments": {}
		}`),
	}

	resp := srv.handleRequest(req)
	if resp == nil {
		t.Fatal("nil response")
	}
	if resp.Error != nil {
		t.Fatalf("error: %v", resp.Error)
	}

	// Verify response has content
	resultMap, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map", resp.Result)
	}

	content, ok := resultMap["content"].([]map[string]any)
	if !ok {
		t.Fatalf("content type = %T, want []map", resultMap["content"])
	}

	if len(content) == 0 {
		t.Fatal("content is empty")
	}

	// Verify inner JSON
	text, ok := content[0]["text"].(string)
	if !ok {
		t.Fatalf("text type = %T, want string", content[0]["text"])
	}

	var status map[string]interface{}
	if err := json.Unmarshal([]byte(text), &status); err != nil {
		t.Fatalf("parse status: %v (text: %s)", err, text)
	}

	if status["state"] != "EXECUTE" {
		t.Errorf("state = %v, want EXECUTE", status["state"])
	}
}

// TestE2ESubcommandRegistration verifies all expected subcommands are registered.
func TestE2ESubcommandRegistration(t *testing.T) {
	expected := map[string]bool{
		"status":   false,
		"run":      false,
		"stop":     false,
		"add-task": false,
		"mcp":      false,
	}

	for _, cmd := range rootCmd.Commands() {
		if _, ok := expected[cmd.Use]; ok {
			expected[cmd.Use] = true
		}
	}

	for name, found := range expected {
		if !found {
			t.Errorf("subcommand %q not registered on rootCmd", name)
		}
	}
}

// TestE2EHelpOutput verifies help includes all subcommands.
func TestE2EHelpOutput(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"--help"})
	rootCmd.Execute()

	output := buf.String()
	for _, sub := range []string{"status", "run", "stop", "add-task", "mcp"} {
		if !strings.Contains(output, sub) {
			t.Errorf("help output should mention %q, got:\n%s", sub, output)
		}
	}
}

// TestE2EBuildBinary verifies the binary can be built with go build.
func TestE2EBuildBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build test in short mode")
	}

	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "c4")

	cmd := exec.Command("go", "build", "-o", binPath, ".")
	cmd.Dir = filepath.Join(testGoModDir(t), "cmd", "c4")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v\n%s", err, output)
	}

	// Verify binary exists and is executable
	info, err := os.Stat(binPath)
	if err != nil {
		t.Fatalf("stat binary: %v", err)
	}
	if info.Size() < 1024 {
		t.Errorf("binary too small (%d bytes)", info.Size())
	}
}

// TestE2EBinaryVersion verifies the built binary outputs version.
func TestE2EBinaryVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary version test in short mode")
	}

	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "c4")

	// Build with version ldflags
	buildCmd := exec.Command("go", "build",
		"-ldflags", "-X main.version=test-1.0.0",
		"-o", binPath, ".")
	buildCmd.Dir = filepath.Join(testGoModDir(t), "cmd", "c4")
	buildCmd.Env = append(os.Environ(), "CGO_ENABLED=0")

	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, output)
	}

	// Run --version
	versionCmd := exec.Command(binPath, "--version")
	output, err := versionCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("c4 --version: %v\n%s", err, output)
	}

	if !strings.Contains(string(output), "test-1.0.0") {
		t.Errorf("version output = %q, want to contain 'test-1.0.0'", output)
	}
}

// testGoModDir finds the c4-core module root directory.
func testGoModDir(t *testing.T) string {
	t.Helper()

	// Walk up from the test file to find go.mod
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find go.mod")
		}
		dir = parent
	}
}
