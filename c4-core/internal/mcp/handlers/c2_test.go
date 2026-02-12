package handlers

import (
	"encoding/json"
	"testing"

	"github.com/changmin/c4-core/internal/mcp"
)

// TestC2ProxyHandlersRegistration verifies all 8 C2 tools are registered.
func TestC2ProxyHandlersRegistration(t *testing.T) {
	reg := mcp.NewRegistry()
	addr, cleanup := startMockSidecar(t)
	defer cleanup()

	proxy := NewBridgeProxy(addr)
	RegisterC2ProxyHandlers(reg, proxy)

	expectedTools := []string{
		"c4_parse_document",
		"c4_extract_text",
		"c4_workspace_create",
		"c4_workspace_load",
		"c4_workspace_save",
		"c4_persona_learn",
		"c4_profile_load",
		"c4_profile_save",
	}

	for _, name := range expectedTools {
		if !reg.HasTool(name) {
			t.Errorf("missing tool: %s", name)
		}
	}
}

// TestC2ProxyHandlersCalls verifies each C2 tool can proxy to the sidecar.
func TestC2ProxyHandlersCalls(t *testing.T) {
	reg := mcp.NewRegistry()
	addr, cleanup := startMockSidecar(t)
	defer cleanup()

	proxy := NewBridgeProxy(addr)
	RegisterC2ProxyHandlers(reg, proxy)

	tests := []struct {
		name   string
		tool   string
		params map[string]any
	}{
		{"parse_document", "c4_parse_document", map[string]any{"file_path": "/tmp/test.docx"}},
		{"extract_text", "c4_extract_text", map[string]any{"file_path": "/tmp/test.docx"}},
		{"workspace_create", "c4_workspace_create", map[string]any{"name": "test"}},
		{"workspace_load", "c4_workspace_load", map[string]any{"project_dir": "/tmp/project"}},
		{"workspace_save", "c4_workspace_save", map[string]any{"project_dir": "/tmp/project", "state": map[string]any{}}},
		{"persona_learn", "c4_persona_learn", map[string]any{"draft_path": "/a.md", "final_path": "/b.md"}},
		{"profile_load", "c4_profile_load", map[string]any{}},
		{"profile_save", "c4_profile_save", map[string]any{"data": map[string]any{"name": "test"}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args, _ := json.Marshal(tc.params)
			result, err := reg.Call(tc.tool, json.RawMessage(args))
			if err != nil {
				t.Fatalf("%s call failed: %v", tc.tool, err)
			}
			if result == nil {
				t.Fatalf("%s returned nil result", tc.tool)
			}
		})
	}
}

// TestC2ToolSchemas verifies the input schemas have expected required fields.
func TestC2ToolSchemas(t *testing.T) {
	reg := mcp.NewRegistry()
	addr, cleanup := startMockSidecar(t)
	defer cleanup()

	proxy := NewBridgeProxy(addr)
	RegisterC2ProxyHandlers(reg, proxy)

	tests := []struct {
		tool     string
		required []string
	}{
		{"c4_parse_document", []string{"file_path"}},
		{"c4_extract_text", []string{"file_path"}},
		{"c4_workspace_create", []string{"name"}},
		{"c4_workspace_load", []string{"project_dir"}},
		{"c4_workspace_save", []string{"project_dir", "state"}},
		{"c4_persona_learn", []string{"draft_path", "final_path"}},
		{"c4_profile_save", []string{"data"}},
	}

	tools := reg.ListTools()
	toolMap := make(map[string]mcp.ToolSchema)
	for _, tool := range tools {
		toolMap[tool.Name] = tool
	}

	for _, tc := range tests {
		t.Run(tc.tool, func(t *testing.T) {
			schema, ok := toolMap[tc.tool]
			if !ok {
				t.Fatalf("tool not found: %s", tc.tool)
			}

			requiredRaw, ok := schema.InputSchema["required"]
			if !ok {
				t.Fatalf("no required field in schema for %s", tc.tool)
			}

			required, ok := requiredRaw.([]string)
			if !ok {
				t.Fatalf("required is not []string for %s", tc.tool)
			}

			requiredSet := make(map[string]bool)
			for _, r := range required {
				requiredSet[r] = true
			}

			for _, expected := range tc.required {
				if !requiredSet[expected] {
					t.Errorf("%s: missing required field %q", tc.tool, expected)
				}
			}
		})
	}
}
