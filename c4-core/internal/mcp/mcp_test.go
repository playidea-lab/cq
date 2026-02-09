package mcp

import (
	"encoding/json"
	"testing"
)

// TestMCPRegistryRegisterAndCall verifies basic registration and dispatch.
func TestMCPRegistryRegisterAndCall(t *testing.T) {
	reg := NewRegistry()

	reg.Register(ToolSchema{
		Name:        "c4_status",
		Description: "Get project status",
		InputSchema: map[string]any{"type": "object"},
	}, func(args json.RawMessage) (any, error) {
		return map[string]any{"state": "EXECUTE", "total_tasks": 10}, nil
	})

	result, err := reg.Call("c4_status", json.RawMessage("{}"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	if m["state"] != "EXECUTE" {
		t.Errorf("state = %v, want EXECUTE", m["state"])
	}
}

// TestMCPRegistryUnknownTool verifies error for unknown tool.
func TestMCPRegistryUnknownTool(t *testing.T) {
	reg := NewRegistry()

	_, err := reg.Call("nonexistent", json.RawMessage("{}"))
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

// TestMCPRegistryHasTool verifies HasTool behavior.
func TestMCPRegistryHasTool(t *testing.T) {
	reg := NewRegistry()

	if reg.HasTool("c4_status") {
		t.Error("should not have tool before registration")
	}

	reg.Register(ToolSchema{
		Name:        "c4_status",
		Description: "Get status",
		InputSchema: map[string]any{"type": "object"},
	}, func(args json.RawMessage) (any, error) {
		return nil, nil
	})

	if !reg.HasTool("c4_status") {
		t.Error("should have tool after registration")
	}
}

// TestMCPRegistryListTools verifies list order and completeness.
func TestMCPRegistryListTools(t *testing.T) {
	reg := NewRegistry()

	tools := []string{"c4_status", "c4_get_task", "c4_submit"}
	for _, name := range tools {
		reg.Register(ToolSchema{
			Name:        name,
			Description: "desc for " + name,
			InputSchema: map[string]any{"type": "object"},
		}, func(args json.RawMessage) (any, error) {
			return nil, nil
		})
	}

	listed := reg.ListTools()
	if len(listed) != 3 {
		t.Fatalf("ListTools returned %d, want 3", len(listed))
	}

	// Verify order matches registration order
	for i, name := range tools {
		if listed[i].Name != name {
			t.Errorf("listed[%d].Name = %q, want %q", i, listed[i].Name, name)
		}
	}
}

// TestMCPRegistryDuplicateSkips verifies that duplicate registration is skipped gracefully.
func TestMCPRegistryDuplicateSkips(t *testing.T) {
	reg := NewRegistry()

	reg.Register(ToolSchema{Name: "c4_test"}, func(args json.RawMessage) (any, error) {
		return "first", nil
	})

	// Second registration should be silently skipped (no panic)
	reg.Register(ToolSchema{Name: "c4_test"}, func(args json.RawMessage) (any, error) {
		return "second", nil
	})

	// Should still have only one tool, with the first handler
	tools := reg.ListTools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	result, err := reg.Call("c4_test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "first" {
		t.Fatalf("expected first handler to be kept, got %v", result)
	}
}

// TestMCPRegistryReplace verifies Replace swaps an existing tool's handler.
func TestMCPRegistryReplace(t *testing.T) {
	reg := NewRegistry()

	reg.Register(ToolSchema{Name: "tool_a", Description: "original"}, func(args json.RawMessage) (any, error) {
		return "original", nil
	})

	// Replace with new handler and description
	ok := reg.Replace(ToolSchema{Name: "tool_a", Description: "replaced"}, func(args json.RawMessage) (any, error) {
		return "replaced", nil
	})
	if !ok {
		t.Fatal("Replace returned false for existing tool")
	}

	// Handler should be the new one
	result, err := reg.Call("tool_a", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "replaced" {
		t.Errorf("result = %v, want replaced", result)
	}

	// Schema description should be updated
	tools := reg.ListTools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Description != "replaced" {
		t.Errorf("description = %q, want replaced", tools[0].Description)
	}
}

// TestMCPRegistryReplaceNonExistent verifies Replace returns false for unknown tools.
func TestMCPRegistryReplaceNonExistent(t *testing.T) {
	reg := NewRegistry()
	ok := reg.Replace(ToolSchema{Name: "nonexistent"}, func(args json.RawMessage) (any, error) {
		return nil, nil
	})
	if ok {
		t.Fatal("Replace returned true for non-existent tool")
	}
}

// TestMCPRegistryUnregister verifies Unregister removes a tool.
func TestMCPRegistryUnregister(t *testing.T) {
	reg := NewRegistry()

	reg.Register(ToolSchema{Name: "tool_a"}, func(args json.RawMessage) (any, error) { return nil, nil })
	reg.Register(ToolSchema{Name: "tool_b"}, func(args json.RawMessage) (any, error) { return nil, nil })
	reg.Register(ToolSchema{Name: "tool_c"}, func(args json.RawMessage) (any, error) { return nil, nil })

	ok := reg.Unregister("tool_b")
	if !ok {
		t.Fatal("Unregister returned false for existing tool")
	}

	if reg.HasTool("tool_b") {
		t.Error("tool_b should be removed")
	}

	// Verify ordering is correct (tool_a, tool_c)
	tools := reg.ListTools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	if tools[0].Name != "tool_a" || tools[1].Name != "tool_c" {
		t.Errorf("tools = [%s, %s], want [tool_a, tool_c]", tools[0].Name, tools[1].Name)
	}
}

// TestMCPRegistryUnregisterNonExistent verifies Unregister returns false for unknown tools.
func TestMCPRegistryUnregisterNonExistent(t *testing.T) {
	reg := NewRegistry()
	ok := reg.Unregister("nonexistent")
	if ok {
		t.Fatal("Unregister returned true for non-existent tool")
	}
}

// TestMCPToolSchemaJSON verifies ToolSchema JSON serialization.
func TestMCPToolSchemaJSON(t *testing.T) {
	schema := ToolSchema{
		Name:        "c4_status",
		Description: "Get project status",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{
					"type":        "string",
					"description": "Project identifier",
				},
			},
		},
	}

	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded ToolSchema
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Name != "c4_status" {
		t.Errorf("Name = %q, want %q", decoded.Name, "c4_status")
	}
	if decoded.Description != "Get project status" {
		t.Errorf("Description = %q, want %q", decoded.Description, "Get project status")
	}
}

// TestMCPHandlerJSONResponse verifies that tool handlers produce
// valid JSON-RPC compatible responses.
func TestMCPHandlerJSONResponse(t *testing.T) {
	reg := NewRegistry()

	reg.Register(ToolSchema{
		Name:        "c4_get_task",
		Description: "Request task assignment",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"worker_id": map[string]any{"type": "string"},
			},
			"required": []string{"worker_id"},
		},
	}, func(args json.RawMessage) (any, error) {
		var params struct {
			WorkerID string `json:"worker_id"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, err
		}
		return map[string]any{
			"task_id":   "T-001-0",
			"title":     "Build feature",
			"worker_id": params.WorkerID,
		}, nil
	})

	result, err := reg.Call("c4_get_task", json.RawMessage(`{"worker_id":"worker-abc12345"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Result must be JSON-serializable
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("result not JSON-serializable: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("result not valid JSON: %v", err)
	}

	if resp["task_id"] != "T-001-0" {
		t.Errorf("task_id = %v, want T-001-0", resp["task_id"])
	}
	if resp["worker_id"] != "worker-abc12345" {
		t.Errorf("worker_id = %v, want worker-abc12345", resp["worker_id"])
	}
}
