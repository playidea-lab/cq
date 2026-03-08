package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// captureStdout redirects os.Stdout to a pipe and returns a flush function
// that closes the write end, restores os.Stdout, and returns the captured output.
// t.Cleanup closes both pipe ends and restores stdout if flush() was never called,
// ensuring no fd leaks even on early test failure.
func captureStdout(t *testing.T) func() string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	flushed := false
	t.Cleanup(func() {
		if !flushed {
			w.Close()
			r.Close()
			os.Stdout = old
		}
	})
	return func() string {
		flushed = true
		w.Close()
		os.Stdout = old
		var buf bytes.Buffer
		io.Copy(&buf, r)
		r.Close()
		return buf.String()
	}
}

// TestToolExtractProperties verifies that extractProperties returns the correct map.
func TestToolExtractProperties(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "search query"},
			"limit": map[string]any{"type": "integer", "description": "max results"},
		},
	}

	props := extractProperties(schema)
	if len(props) != 2 {
		t.Fatalf("expected 2 props, got %d", len(props))
	}
	if _, ok := props["query"]; !ok {
		t.Error("expected 'query' property")
	}
	if _, ok := props["limit"]; !ok {
		t.Error("expected 'limit' property")
	}
}

// TestToolExtractPropertiesNil verifies nil safety.
func TestToolExtractPropertiesNil(t *testing.T) {
	if props := extractProperties(nil); props != nil {
		t.Errorf("expected nil, got %v", props)
	}
	if props := extractProperties(map[string]any{}); props != nil {
		t.Errorf("expected nil for missing properties key, got %v", props)
	}
}

// TestToolPrettyPrintString verifies string output.
func TestToolPrettyPrintString(t *testing.T) {
	flush := captureStdout(t)
	if err := prettyPrint("hello world"); err != nil {
		t.Fatalf("prettyPrint string: %v", err)
	}
	out := flush()
	if !strings.Contains(out, "hello world") {
		t.Errorf("expected 'hello world' in output, got %q", out)
	}
}

// TestToolPrettyPrintMCPContent verifies MCP text-content array rendering.
func TestToolPrettyPrintMCPContent(t *testing.T) {
	flush := captureStdout(t)
	content := []any{
		map[string]any{"type": "text", "text": "status: OK"},
	}
	if err := prettyPrint(content); err != nil {
		t.Fatalf("prettyPrint content: %v", err)
	}
	out := flush()
	if !strings.Contains(out, "status: OK") {
		t.Errorf("expected 'status: OK' in output, got %q", out)
	}
}

// TestToolListRegistered verifies that core tools are registered in a test MCP server.
func TestToolListRegistered(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	srv := newTestMCPServer(t, db)
	tools := srv.registry.ListTools()
	if len(tools) == 0 {
		t.Fatal("expected at least one tool registered")
	}

	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Name] = true
	}
	for _, required := range []string{"c4_status", "c4_get_task", "c4_submit"} {
		if !names[required] {
			t.Errorf("expected tool %q in registry", required)
		}
	}
}

// TestToolGetSchema verifies GetToolSchema returns the correct schema.
func TestToolGetSchema(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	srv := newTestMCPServer(t, db)

	schema, ok := srv.registry.GetToolSchema("c4_status")
	if !ok {
		t.Fatal("c4_status not registered")
	}
	if schema.Name != "c4_status" {
		t.Errorf("name = %q, want 'c4_status'", schema.Name)
	}
	if schema.Description == "" {
		t.Error("expected non-empty description")
	}
}

// TestToolExecToolJSON verifies execTool outputs valid JSON with --json flag.
func TestToolExecToolJSON(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	insertState(t, db, "tool-test", "EXECUTE")

	srv := newTestMCPServer(t, db)

	schema, _ := srv.registry.GetToolSchema("c4_status")
	props := extractProperties(schema.InputSchema)

	// Build a cobra.Command with --json=true and --timeout
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().Duration("timeout", 60*time.Second, "")
	if err := cmd.ParseFlags([]string{"--json"}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}

	flush := captureStdout(t)
	err := execTool(cmd, "c4_status", srv, props)
	out := flush()

	if err != nil {
		t.Fatalf("execTool error: %v", err)
	}

	// Output should be valid JSON
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, out)
	}
	if _, ok := result["state"]; !ok {
		t.Errorf("expected 'state' key in c4_status result, got keys: %v", result)
	}
}

// TestToolListRoutingViaRunToolDynamic verifies that runToolDynamic forwards
// "list" to runToolList and does NOT fall through to the "unknown tool" error path.
func TestToolListRoutingViaRunToolDynamic(t *testing.T) {
	// setupTestDB sets the global projectDir so the newMCPServer call inside
	// runToolDynamic resolves the correct (temporary) project DB path.
	_, cleanup := setupTestDB(t)
	defer cleanup()

	flush := captureStdout(t)
	// Call the actual routing function with args=["list"]
	err := runToolDynamic(toolCmd, []string{"list"})
	out := flush()

	if err != nil {
		t.Fatalf("runToolDynamic([list]) error: %v", err)
	}
	if strings.Contains(out, "unknown tool") {
		t.Errorf("routing produced 'unknown tool' error — list not forwarded correctly: %q", out)
	}
	if !strings.Contains(out, "TOOL") {
		t.Errorf("expected 'TOOL' header in list output, got: %q", out)
	}
}

// TestToolUnknownToolError verifies that GetToolSchema returns false for unknown tools.
func TestToolUnknownToolError(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	srv := newTestMCPServer(t, db)

	_, ok := srv.registry.GetToolSchema("nonexistent_tool_xyz")
	if ok {
		t.Error("expected false for unknown tool GetToolSchema")
	}
}

// TestToolExecToolBadIntegerArg verifies that a non-numeric string for an integer
// flag returns an error rather than silently using zero.
func TestToolExecToolBadIntegerArg(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	srv := newTestMCPServer(t, db)

	props := map[string]any{
		"limit": map[string]any{"type": "integer", "description": "max results"},
	}

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().Duration("timeout", 60*time.Second, "")
	cmd.Flags().String("limit", "", "")
	if err := cmd.ParseFlags([]string{"--limit=notanumber"}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}

	err := execTool(cmd, "c4_status", srv, props)
	if err == nil {
		t.Fatal("expected error for non-numeric integer flag, got nil")
	}
	if !strings.Contains(err.Error(), "not a valid integer") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestTool_notification_set(t *testing.T) {
	srv, err := newMCPServer()
	if err != nil {
		t.Fatalf("newMCPServer: %v", err)
	}
	defer srv.shutdown()

	tools := srv.registry.ListTools()
	toolNames := make(map[string]bool, len(tools))
	for _, tool := range tools {
		toolNames[tool.Name] = true
	}

	for _, name := range []string{"c4_notification_set", "c4_notification_get", "c4_notify"} {
		if !toolNames[name] {
			t.Errorf("tool %s not registered", name)
		}
	}
}
