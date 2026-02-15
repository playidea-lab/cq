package handlers

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/changmin/c4-core/internal/mcp"
	_ "modernc.org/sqlite"
)

// toFloat converts int or float64 to float64 for test assertions.
func toFloat(v any) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case float64:
		return n
	case int64:
		return float64(n)
	default:
		return -1
	}
}

// setupLighthouseTest creates an in-memory DB, store, and registry for lighthouse testing.
func setupLighthouseTest(t *testing.T) (*mcp.Registry, *SQLiteStore) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	reg := mcp.NewRegistry()
	// Register some core tools to test collision prevention
	reg.Register(mcp.ToolSchema{Name: "c4_status", Description: "core tool"}, func(args json.RawMessage) (any, error) {
		return nil, nil
	})
	RegisterLighthouseHandlers(reg, store)
	return reg, store
}

// callLighthouse is a helper to call c4_lighthouse with typed args.
func callLighthouse(t *testing.T, reg *mcp.Registry, args map[string]any) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(args)
	result, err := reg.Call("c4_lighthouse", raw)
	if err != nil {
		t.Fatalf("c4_lighthouse(%v) error: %v", args["action"], err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		// Could be a Lighthouse struct
		data, _ := json.Marshal(result)
		var mm map[string]any
		if err := json.Unmarshal(data, &mm); err != nil {
			t.Fatalf("result type = %T, want map", result)
		}
		return mm
	}
	return m
}

// callLighthouseExpectErr calls c4_lighthouse and expects an error.
func callLighthouseExpectErr(t *testing.T, reg *mcp.Registry, args map[string]any) error {
	t.Helper()
	raw, _ := json.Marshal(args)
	_, err := reg.Call("c4_lighthouse", raw)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	return err
}

func TestLighthouseRegister(t *testing.T) {
	reg, _ := setupLighthouseTest(t)

	result := callLighthouse(t, reg, map[string]any{
		"action":       "register",
		"name":         "lh_export",
		"description":  "Export API — batch export project data",
		"spec":         "## Export API\n\nExports project data in JSON format.\n\n### Inputs\n- format: json|csv\n- filter: optional query",
		"input_schema": `{"type":"object","properties":{"format":{"type":"string"},"filter":{"type":"string"}}}`,
	})

	if result["success"] != true {
		t.Errorf("success = %v, want true", result["success"])
	}
	if result["status"] != "stub" {
		t.Errorf("status = %v, want stub", result["status"])
	}

	// Verify the stub is registered in the MCP registry
	if !reg.HasTool("lh_export") {
		t.Fatal("lh_export should be registered in registry")
	}

	// Verify the stub shows up in tools/list with [LIGHTHOUSE] prefix
	tools := reg.ListTools()
	found := false
	for _, tool := range tools {
		if tool.Name == "lh_export" {
			found = true
			if tool.Description != "[LIGHTHOUSE] Export API — batch export project data" {
				t.Errorf("description = %q, want [LIGHTHOUSE] prefix", tool.Description)
			}
		}
	}
	if !found {
		t.Fatal("lh_export not found in ListTools")
	}
}

func TestLighthouseStubReturnsSpec(t *testing.T) {
	reg, _ := setupLighthouseTest(t)

	// Register a lighthouse
	callLighthouse(t, reg, map[string]any{
		"action":      "register",
		"name":        "lh_analyze",
		"description": "Analyze data",
		"spec":        "Returns analysis results",
	})

	// Call the stub directly
	raw, _ := json.Marshal(map[string]any{"query": "test"})
	result, err := reg.Call("lh_analyze", raw)
	if err != nil {
		t.Fatalf("calling stub: %v", err)
	}

	m := result.(map[string]any)
	if m["lighthouse"] != true {
		t.Error("lighthouse flag should be true")
	}
	if m["status"] != "stub" {
		t.Errorf("status = %v, want stub", m["status"])
	}
	if m["spec"] != "Returns analysis results" {
		t.Errorf("spec = %v, want 'Returns analysis results'", m["spec"])
	}
	if m["message"] != "This is a lighthouse stub. The spec above defines the contract." {
		t.Errorf("unexpected message: %v", m["message"])
	}

	// Verify called_with contains the arguments we passed
	calledWith, ok := m["called_with"].(json.RawMessage)
	if !ok {
		t.Fatalf("called_with type = %T, want json.RawMessage", m["called_with"])
	}
	var cw map[string]any
	if err := json.Unmarshal(calledWith, &cw); err != nil {
		t.Fatalf("unmarshal called_with: %v", err)
	}
	if cw["query"] != "test" {
		t.Errorf("called_with.query = %v, want test", cw["query"])
	}
}

func TestLighthouseList(t *testing.T) {
	reg, _ := setupLighthouseTest(t)

	// Empty list
	result := callLighthouse(t, reg, map[string]any{"action": "list"})
	summary := result["summary"].(map[string]any)
	if toFloat(summary["total"]) != 0 {
		t.Errorf("total = %v, want 0", summary["total"])
	}

	// Register two
	callLighthouse(t, reg, map[string]any{"action": "register", "name": "lh_a", "description": "A"})
	callLighthouse(t, reg, map[string]any{"action": "register", "name": "lh_b", "description": "B"})

	result = callLighthouse(t, reg, map[string]any{"action": "list"})
	summary = result["summary"].(map[string]any)
	if toFloat(summary["total"]) != 2 {
		t.Errorf("total = %v, want 2", summary["total"])
	}
	if toFloat(summary["stubs"]) != 2 {
		t.Errorf("stubs = %v, want 2", summary["stubs"])
	}
}

func TestLighthouseGet(t *testing.T) {
	reg, _ := setupLighthouseTest(t)

	callLighthouse(t, reg, map[string]any{
		"action":      "register",
		"name":        "lh_get_test",
		"description": "Test Get",
		"spec":        "My spec",
	})

	result := callLighthouse(t, reg, map[string]any{"action": "get", "name": "lh_get_test"})
	if result["name"] != "lh_get_test" {
		t.Errorf("name = %v, want lh_get_test", result["name"])
	}
	if result["spec"] != "My spec" {
		t.Errorf("spec = %v, want 'My spec'", result["spec"])
	}
	if result["status"] != "stub" {
		t.Errorf("status = %v, want stub", result["status"])
	}
}

func TestLighthousePromote(t *testing.T) {
	reg, _ := setupLighthouseTest(t)

	callLighthouse(t, reg, map[string]any{"action": "register", "name": "lh_promote_test", "description": "Promo"})

	// Verify stub exists
	if !reg.HasTool("lh_promote_test") {
		t.Fatal("stub should be registered")
	}

	// Promote
	result := callLighthouse(t, reg, map[string]any{"action": "promote", "name": "lh_promote_test"})
	if result["success"] != true {
		t.Errorf("success = %v, want true", result["success"])
	}
	if result["status"] != "implemented" {
		t.Errorf("status = %v, want implemented", result["status"])
	}

	// Stub should be removed from registry
	if reg.HasTool("lh_promote_test") {
		t.Error("stub should be removed from registry after promote")
	}

	// DB status should be "implemented"
	lh := callLighthouse(t, reg, map[string]any{"action": "get", "name": "lh_promote_test"})
	if lh["status"] != "implemented" {
		t.Errorf("DB status = %v, want implemented", lh["status"])
	}
}

func TestLighthouseUpdate(t *testing.T) {
	reg, _ := setupLighthouseTest(t)

	callLighthouse(t, reg, map[string]any{"action": "register", "name": "lh_upd", "description": "V1", "spec": "spec v1"})

	// Update spec
	result := callLighthouse(t, reg, map[string]any{"action": "update", "name": "lh_upd", "spec": "spec v2"})
	if result["success"] != true {
		t.Errorf("success = %v, want true", result["success"])
	}

	// Version should increment
	ver := toFloat(result["version"])
	if ver != 2 {
		t.Errorf("version = %v, want 2", result["version"])
	}

	// Verify spec updated
	lh := callLighthouse(t, reg, map[string]any{"action": "get", "name": "lh_upd"})
	if lh["spec"] != "spec v2" {
		t.Errorf("spec = %v, want 'spec v2'", lh["spec"])
	}

	// Stub handler should also return the new spec
	raw, _ := json.Marshal(map[string]any{})
	stubResult, err := reg.Call("lh_upd", raw)
	if err != nil {
		t.Fatalf("calling updated stub: %v", err)
	}
	m := stubResult.(map[string]any)
	if m["spec"] != "spec v2" {
		t.Errorf("stub spec = %v, want 'spec v2'", m["spec"])
	}
}

func TestLighthouseRemove(t *testing.T) {
	reg, _ := setupLighthouseTest(t)

	callLighthouse(t, reg, map[string]any{"action": "register", "name": "lh_rm", "description": "To Remove"})

	result := callLighthouse(t, reg, map[string]any{"action": "remove", "name": "lh_rm"})
	if result["success"] != true {
		t.Errorf("success = %v, want true", result["success"])
	}

	// Should be removed from registry
	if reg.HasTool("lh_rm") {
		t.Error("lh_rm should be removed from registry")
	}

	// DB should show deprecated
	lh := callLighthouse(t, reg, map[string]any{"action": "get", "name": "lh_rm"})
	if lh["status"] != "deprecated" {
		t.Errorf("status = %v, want deprecated", lh["status"])
	}
}

func TestLighthouseNameCollision(t *testing.T) {
	reg, _ := setupLighthouseTest(t)

	// c4_status is a core tool — should be rejected
	callLighthouseExpectErr(t, reg, map[string]any{
		"action":      "register",
		"name":        "c4_status",
		"description": "Should fail",
	})

	// Duplicate lighthouse name
	callLighthouse(t, reg, map[string]any{"action": "register", "name": "lh_dup", "description": "First"})
	callLighthouseExpectErr(t, reg, map[string]any{
		"action":      "register",
		"name":        "lh_dup",
		"description": "Second",
	})
}

func TestLighthouseStartupLoader(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	// Manually insert a stub lighthouse into DB
	if err := store.saveLighthouse(&Lighthouse{
		Name:        "lh_preloaded",
		Description: "Preloaded stub",
		InputSchema: `{"type":"object","properties":{"x":{"type":"number"}}}`,
		Spec:        "Preloaded spec",
		Status:      "stub",
		Version:     1,
		CreatedBy:   "test",
	}); err != nil {
		t.Fatalf("save lighthouse: %v", err)
	}

	// Also insert an implemented one — should NOT be loaded
	if err := store.saveLighthouse(&Lighthouse{
		Name:        "lh_done",
		Description: "Already done",
		InputSchema: `{}`,
		Status:      "implemented",
		Version:     2,
	}); err != nil {
		t.Fatalf("save lighthouse: %v", err)
	}

	// Create fresh registry and load
	reg := mcp.NewRegistry()
	n := LoadLighthousesOnStartup(reg, store)
	if n != 1 {
		t.Fatalf("loaded = %d, want 1", n)
	}

	// lh_preloaded should be callable
	if !reg.HasTool("lh_preloaded") {
		t.Fatal("lh_preloaded should be registered")
	}

	raw, _ := json.Marshal(map[string]any{"x": 42})
	result, err := reg.Call("lh_preloaded", raw)
	if err != nil {
		t.Fatalf("calling preloaded stub: %v", err)
	}
	m := result.(map[string]any)
	if m["lighthouse"] != true {
		t.Error("lighthouse flag should be true")
	}
	if m["spec"] != "Preloaded spec" {
		t.Errorf("spec = %v, want 'Preloaded spec'", m["spec"])
	}

	// lh_done should NOT be registered
	if reg.HasTool("lh_done") {
		t.Error("implemented lighthouse should not be loaded as stub")
	}
}

func TestLighthouseStatusCount(t *testing.T) {
	reg, store := setupLighthouseTest(t)

	// Register 2 stubs
	callLighthouse(t, reg, map[string]any{"action": "register", "name": "lh_s1", "description": "S1"})
	callLighthouse(t, reg, map[string]any{"action": "register", "name": "lh_s2", "description": "S2"})

	// Promote one
	callLighthouse(t, reg, map[string]any{"action": "promote", "name": "lh_s1"})

	// Check GetStatus
	status, err := store.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if status.LighthouseStubs != 1 {
		t.Errorf("LighthouseStubs = %d, want 1", status.LighthouseStubs)
	}
	if status.LighthouseImplemented != 1 {
		t.Errorf("LighthouseImplemented = %d, want 1", status.LighthouseImplemented)
	}
}

func TestLighthouseFullLifecycle(t *testing.T) {
	reg, _ := setupLighthouseTest(t)

	// 1. Register
	callLighthouse(t, reg, map[string]any{
		"action":       "register",
		"name":         "lh_lifecycle",
		"description":  "Lifecycle test API",
		"spec":         "## API\nReturns hello",
		"input_schema": `{"type":"object","properties":{"name":{"type":"string"}}}`,
	})

	// 2. Call stub — should get spec back
	raw, _ := json.Marshal(map[string]any{"name": "world"})
	stubResult, err := reg.Call("lh_lifecycle", raw)
	if err != nil {
		t.Fatalf("calling stub: %v", err)
	}
	m := stubResult.(map[string]any)
	if m["lighthouse"] != true {
		t.Fatal("should be lighthouse")
	}

	// 3. Update spec
	callLighthouse(t, reg, map[string]any{
		"action": "update",
		"name":   "lh_lifecycle",
		"spec":   "## API v2\nReturns hello with greeting",
	})

	// 4. Call again — should reflect updated spec
	stubResult2, _ := reg.Call("lh_lifecycle", raw)
	m2 := stubResult2.(map[string]any)
	if m2["spec"] != "## API v2\nReturns hello with greeting" {
		t.Errorf("updated spec not reflected: %v", m2["spec"])
	}

	// 5. Promote
	callLighthouse(t, reg, map[string]any{"action": "promote", "name": "lh_lifecycle"})

	// 6. Stub should be gone
	if reg.HasTool("lh_lifecycle") {
		t.Error("stub should be removed after promote")
	}

	// 7. List should show 1 implemented
	listResult := callLighthouse(t, reg, map[string]any{"action": "list"})
	summary := listResult["summary"].(map[string]any)
	if toFloat(summary["implemented"]) != 1 {
		t.Errorf("implemented = %v, want 1", summary["implemented"])
	}
}

// --- TDD Loop Enhancement Tests ---

func TestLighthouseRegisterAutoTask(t *testing.T) {
	reg, store := setupLighthouseTest(t)

	result := callLighthouse(t, reg, map[string]any{
		"action":       "register",
		"name":         "lh_auto",
		"description":  "Auto task test",
		"spec":         "## Spec\nReturns data",
		"input_schema": `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`,
	})

	// Should have task_id in result
	taskID, ok := result["task_id"].(string)
	if !ok || taskID == "" {
		t.Fatalf("task_id not in result: %v", result)
	}
	if taskID != "T-LH-lh_auto-0" {
		t.Errorf("task_id = %q, want T-LH-lh_auto-0", taskID)
	}

	// Task should exist in store
	task, err := store.GetTask(taskID)
	if err != nil {
		t.Fatalf("GetTask(%s): %v", taskID, err)
	}
	if task.Status != "pending" {
		t.Errorf("task status = %q, want pending", task.Status)
	}
	if task.Domain != "lighthouse" {
		t.Errorf("task domain = %q, want lighthouse", task.Domain)
	}

	// Lighthouse record should have task_id
	lh, err := store.getLighthouse("lh_auto")
	if err != nil {
		t.Fatalf("getLighthouse: %v", err)
	}
	if lh.TaskID != taskID {
		t.Errorf("lighthouse task_id = %q, want %q", lh.TaskID, taskID)
	}
}

func TestLighthouseRegisterNoAutoTask(t *testing.T) {
	reg, store := setupLighthouseTest(t)

	result := callLighthouse(t, reg, map[string]any{
		"action":      "register",
		"name":        "lh_no_auto",
		"description": "No auto task",
		"auto_task":   false,
	})

	// Should NOT have task_id
	if _, ok := result["task_id"]; ok {
		t.Error("task_id should not be present when auto_task=false")
	}

	// No task should exist
	_, err := store.GetTask("T-LH-lh_no_auto-0")
	if err == nil {
		t.Error("task should not exist when auto_task=false")
	}

	// Lighthouse should still be registered
	lh, err := store.getLighthouse("lh_no_auto")
	if err != nil {
		t.Fatalf("getLighthouse: %v", err)
	}
	if lh.TaskID != "" {
		t.Errorf("lighthouse task_id = %q, want empty", lh.TaskID)
	}
}

func TestAssignTaskWithLighthouseSpec(t *testing.T) {
	_, store := setupLighthouseTest(t)

	// Create a lighthouse with task
	lh := &Lighthouse{
		Name:        "lh_assign_test",
		Description: "Assign test API",
		InputSchema: `{"type":"object","properties":{"x":{"type":"number"}}}`,
		Spec:        "Returns x squared",
		Status:      "stub",
		Version:     1,
		CreatedBy:   "test",
		TaskID:      "T-LH-lh_assign_test-0",
	}
	if err := store.saveLighthouse(lh); err != nil {
		t.Fatalf("saveLighthouse: %v", err)
	}

	// Create the corresponding task
	task := &Task{
		ID:     "T-LH-lh_assign_test-0",
		Title:  "Implement lighthouse: lh_assign_test",
		DoD:    "Implement matching spec",
		Domain: "lighthouse",
	}
	if err := store.AddTask(task); err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	// Assign the task
	assignment, err := store.AssignTask("worker-1")
	if err != nil {
		t.Fatalf("AssignTask: %v", err)
	}
	if assignment == nil {
		t.Fatal("assignment is nil")
	}
	if assignment.TaskID != "T-LH-lh_assign_test-0" {
		t.Errorf("assigned task = %q, want T-LH-lh_assign_test-0", assignment.TaskID)
	}

	// LighthouseSpec should be injected
	if assignment.LighthouseSpec == nil {
		t.Fatal("LighthouseSpec should not be nil")
	}
	if assignment.LighthouseSpec.Name != "lh_assign_test" {
		t.Errorf("spec name = %q, want lh_assign_test", assignment.LighthouseSpec.Name)
	}
	if assignment.LighthouseSpec.Spec != "Returns x squared" {
		t.Errorf("spec = %q, want 'Returns x squared'", assignment.LighthouseSpec.Spec)
	}
	if assignment.LighthouseSpec.InputSchema != `{"type":"object","properties":{"x":{"type":"number"}}}` {
		t.Errorf("input_schema mismatch: %q", assignment.LighthouseSpec.InputSchema)
	}
}

func TestLighthousePromoteSchemaValidation(t *testing.T) {
	reg, _ := setupLighthouseTest(t)

	// Register lighthouse with schema requiring "name" and "age"
	callLighthouse(t, reg, map[string]any{
		"action":       "register",
		"name":         "lh_schema_check",
		"description":  "Schema check API",
		"input_schema": `{"type":"object","properties":{"name":{"type":"string"},"age":{"type":"number"}},"required":["name"]}`,
		"auto_task":    false,
	})

	// Unregister the lighthouse stub and register a "real" tool under the SAME name
	// with a DIFFERENT schema (missing "age" property, non-[LIGHTHOUSE] description)
	reg.Unregister("lh_schema_check")
	reg.Register(mcp.ToolSchema{
		Name:        "lh_schema_check",
		Description: "Real implementation", // no [LIGHTHOUSE] prefix
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"required": []any{"name"},
		},
	}, func(args json.RawMessage) (any, error) {
		return map[string]any{"ok": true}, nil
	})

	// Promote — F-01 fix: GetToolSchema runs BEFORE Unregister, so schema_warnings should appear
	result := callLighthouse(t, reg, map[string]any{"action": "promote", "name": "lh_schema_check"})
	if result["success"] != true {
		t.Fatalf("promote failed: %v", result)
	}

	// Should have schema_warnings about missing "age" property
	warnings, ok := result["schema_warnings"].([]string)
	if !ok || len(warnings) == 0 {
		t.Fatal("expected schema_warnings in promote result for missing 'age'")
	}

	foundAgeWarning := false
	for _, w := range warnings {
		if w == "lighthouse property 'age' not found in real tool" {
			foundAgeWarning = true
		}
	}
	if !foundAgeWarning {
		t.Errorf("warnings = %v, want warning about 'age'", warnings)
	}

	// Test validateSchemaCompat directly: matching schema — no warnings (superset OK)
	noWarnings := validateSchemaCompat(
		`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`,
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":  map[string]any{"type": "string"},
				"extra": map[string]any{"type": "number"},
			},
			"required": []any{"name"},
		},
	)
	if len(noWarnings) != 0 {
		t.Errorf("expected no warnings for superset schema, got %v", noWarnings)
	}
}

func TestLighthousePromoteTaskCompletion(t *testing.T) {
	reg, store := setupLighthouseTest(t)

	// Register with auto_task
	result := callLighthouse(t, reg, map[string]any{
		"action":      "register",
		"name":        "lh_task_done",
		"description": "Task done test",
		"spec":        "Do something",
	})

	taskID := result["task_id"].(string)

	// Verify task is pending
	task, _ := store.GetTask(taskID)
	if task.Status != "pending" {
		t.Fatalf("task status = %q, want pending", task.Status)
	}

	// Promote
	promoteResult := callLighthouse(t, reg, map[string]any{"action": "promote", "name": "lh_task_done"})
	if promoteResult["success"] != true {
		t.Fatalf("promote failed: %v", promoteResult)
	}

	// Task completed should be in result
	if promoteResult["task_completed"] != taskID {
		t.Errorf("task_completed = %v, want %q", promoteResult["task_completed"], taskID)
	}

	// Task should be "done" in store
	task, err := store.GetTask(taskID)
	if err != nil {
		t.Fatalf("GetTask after promote: %v", err)
	}
	if task.Status != "done" {
		t.Errorf("task status after promote = %q, want done", task.Status)
	}
}

func TestGetToolSchema(t *testing.T) {
	reg := mcp.NewRegistry()

	// Register a tool
	schema := mcp.ToolSchema{
		Name:        "test_tool",
		Description: "A test tool",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"required": []any{"name"},
		},
	}
	reg.Register(schema, func(args json.RawMessage) (any, error) {
		return nil, nil
	})

	// GetToolSchema should return the schema
	got, ok := reg.GetToolSchema("test_tool")
	if !ok {
		t.Fatal("test_tool should be found")
	}
	if got.Name != "test_tool" {
		t.Errorf("name = %q, want test_tool", got.Name)
	}
	if got.Description != "A test tool" {
		t.Errorf("description = %q, want 'A test tool'", got.Description)
	}
	props, ok := got.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties not found in schema")
	}
	if _, ok := props["name"]; !ok {
		t.Error("property 'name' not found in schema")
	}

	// Non-existent tool
	_, ok = reg.GetToolSchema("nonexistent")
	if ok {
		t.Error("nonexistent tool should not be found")
	}
}

func TestLighthouseRegisterNameValidation(t *testing.T) {
	reg, _ := setupLighthouseTest(t)

	badNames := []string{
		"hello world",    // space
		"foo\nbar",       // newline
		"123start",       // starts with digit
		"",               // empty (separate check but still invalid)
		"a-b-c-d-e-f-g-h-i-j-k-l-m-n-o-p-q-r-s-t-u-v-w-x-y-z-aa-bb-cc-dd-ee-ff-gg", // too long
	}

	for _, name := range badNames {
		raw, _ := json.Marshal(map[string]any{
			"action":      "register",
			"name":        name,
			"description": "Should fail",
		})
		_, err := reg.Call("c4_lighthouse", raw)
		if err == nil {
			t.Errorf("expected error for name %q, got nil", name)
		}
	}

	// Valid names should work
	goodNames := []string{"lh_test", "my-tool", "A_valid_Name", "_private"}
	for _, name := range goodNames {
		result := callLighthouse(t, reg, map[string]any{
			"action":      "register",
			"name":        name,
			"description": "Valid",
			"auto_task":   false,
		})
		if result["success"] != true {
			t.Errorf("name %q should be valid, got: %v", name, result)
		}
	}
}

func TestLighthouseStartupLoaderCoreToolCollision(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	// Insert a stub lighthouse with a name that will collide with a core tool
	if err := store.saveLighthouse(&Lighthouse{
		Name: "c4_status", Description: "Collides with core", InputSchema: `{}`,
		Status: "stub", Version: 1, CreatedBy: "test",
	}); err != nil {
		t.Fatalf("save lighthouse: %v", err)
	}

	// Register core tool first
	reg := mcp.NewRegistry()
	reg.Register(mcp.ToolSchema{Name: "c4_status", Description: "core"}, func(args json.RawMessage) (any, error) {
		return nil, nil
	})

	// Load — should skip the colliding lighthouse
	n := LoadLighthousesOnStartup(reg, store)
	if n != 0 {
		t.Errorf("loaded = %d, want 0 (core tool collision should be skipped)", n)
	}

	// Core tool should still be there with original description
	schema, ok := reg.GetToolSchema("c4_status")
	if !ok {
		t.Fatal("c4_status should still be registered")
	}
	if schema.Description != "core" {
		t.Errorf("description = %q, want 'core' (should not be overwritten)", schema.Description)
	}
}

func TestLighthouseUpdateImplementedBlocked(t *testing.T) {
	reg, _ := setupLighthouseTest(t)

	// Register and promote
	callLighthouse(t, reg, map[string]any{
		"action": "register", "name": "lh_impl_upd", "description": "Will promote", "auto_task": false,
	})
	callLighthouse(t, reg, map[string]any{"action": "promote", "name": "lh_impl_upd"})

	// Try to update — should fail because it's "implemented" not "stub"
	err := callLighthouseExpectErr(t, reg, map[string]any{
		"action": "update", "name": "lh_impl_upd", "description": "New desc",
	})
	if err == nil {
		t.Fatal("expected error updating implemented lighthouse")
	}
	if !strings.Contains(err.Error(), "only stubs can be updated") {
		t.Errorf("error = %v, want 'only stubs can be updated'", err)
	}
}

func TestLighthouseRegisterInvalidJSON(t *testing.T) {
	reg, _ := setupLighthouseTest(t)

	raw, _ := json.Marshal(map[string]any{
		"action":       "register",
		"name":         "lh_bad_json",
		"description":  "Bad schema",
		"input_schema": "not json at all",
	})
	_, err := reg.Call("c4_lighthouse", raw)
	if err == nil {
		t.Fatal("expected error for invalid JSON schema, got nil")
	}
}

func TestLighthouseReregisterAfterRemove(t *testing.T) {
	reg, _ := setupLighthouseTest(t)

	callLighthouse(t, reg, map[string]any{
		"action": "register", "name": "lh_rereg", "description": "V1", "auto_task": false,
	})
	callLighthouse(t, reg, map[string]any{"action": "remove", "name": "lh_rereg"})

	// Re-register with same name should be blocked (deprecated record still exists)
	err := callLighthouseExpectErr(t, reg, map[string]any{
		"action": "register", "name": "lh_rereg", "description": "V2",
	})
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %v, want 'already exists'", err)
	}
}

func TestAutoPromoteLighthouseOnSubmit(t *testing.T) {
	reg, store := setupLighthouseTest(t)

	// Register lighthouse with auto_task
	result := callLighthouse(t, reg, map[string]any{
		"action":      "register",
		"name":        "lh_auto_promote",
		"description": "Auto promote test",
		"spec":        "Returns data",
	})
	taskID := result["task_id"].(string)
	if taskID != "T-LH-lh_auto_promote-0" {
		t.Fatalf("task_id = %q, want T-LH-lh_auto_promote-0", taskID)
	}

	// Wire registry to store for auto-promote
	store.registry = reg

	// Assign task (moves to in_progress)
	assignment, err := store.AssignTask("worker-1")
	if err != nil {
		t.Fatalf("AssignTask: %v", err)
	}
	if assignment.TaskID != taskID {
		t.Fatalf("assigned = %q, want %q", assignment.TaskID, taskID)
	}

	// Submit task
	submitResult, err := store.SubmitTask(taskID, "worker-1", "abc123", "", nil)
	if err != nil {
		t.Fatalf("SubmitTask: %v", err)
	}
	if !submitResult.Success {
		t.Fatalf("submit failed: %s", submitResult.Message)
	}

	// Lighthouse should be promoted
	lh, err := store.getLighthouse("lh_auto_promote")
	if err != nil {
		t.Fatalf("getLighthouse: %v", err)
	}
	if lh.Status != "implemented" {
		t.Errorf("lighthouse status = %q, want implemented", lh.Status)
	}

	// Stub should be removed from registry
	if reg.HasTool("lh_auto_promote") {
		t.Error("stub should be removed from registry after auto-promote")
	}
}

func TestAutoPromoteLighthouseOnReport(t *testing.T) {
	reg, store := setupLighthouseTest(t)

	// Register lighthouse with auto_task
	result := callLighthouse(t, reg, map[string]any{
		"action":      "register",
		"name":        "lh_report_promote",
		"description": "Report promote test",
		"spec":        "Returns data",
	})
	taskID := result["task_id"].(string)

	// Wire registry to store for auto-promote
	store.registry = reg

	// Claim task (direct mode)
	_, err := store.ClaimTask(taskID)
	if err != nil {
		t.Fatalf("ClaimTask: %v", err)
	}

	// Report task
	err = store.ReportTask(taskID, "implemented lighthouse", []string{"handler.go"})
	if err != nil {
		t.Fatalf("ReportTask: %v", err)
	}

	// Lighthouse should be promoted
	lh, err := store.getLighthouse("lh_report_promote")
	if err != nil {
		t.Fatalf("getLighthouse: %v", err)
	}
	if lh.Status != "implemented" {
		t.Errorf("lighthouse status = %q, want implemented", lh.Status)
	}

	// Stub should be removed from registry
	if reg.HasTool("lh_report_promote") {
		t.Error("stub should be removed from registry after auto-promote")
	}
}

func TestAutoPromoteSkipsNonLHTask(t *testing.T) {
	reg, store := setupLighthouseTest(t)

	// Register a lighthouse (creates T-LH-lh_skip_test-0)
	callLighthouse(t, reg, map[string]any{
		"action":      "register",
		"name":        "lh_skip_test",
		"description": "Should not be affected",
		"spec":        "Spec",
		"auto_task":   false, // no auto task — keeps things clean
	})

	// Wire registry
	store.registry = reg

	// Create a normal (non-T-LH) task
	normalTask := &Task{
		ID:    "T-001-0",
		Title: "Normal task",
		DoD:   "Do something",
	}
	if err := store.AddTask(normalTask); err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	// Claim and report the normal task (direct mode)
	if _, err := store.ClaimTask("T-001-0"); err != nil {
		t.Fatalf("ClaimTask: %v", err)
	}
	if err := store.ReportTask("T-001-0", "done", []string{"file.go"}); err != nil {
		t.Fatalf("ReportTask: %v", err)
	}

	// Lighthouse should still be stub (not promoted)
	lh, err := store.getLighthouse("lh_skip_test")
	if err != nil {
		t.Fatalf("getLighthouse: %v", err)
	}
	if lh.Status != "stub" {
		t.Errorf("lighthouse status = %q, want stub (should not be affected by non-LH task)", lh.Status)
	}

	// Stub should still be in registry
	if !reg.HasTool("lh_skip_test") {
		t.Error("stub should remain in registry when non-LH task completes")
	}
}

func TestAutoPromoteLighthouseHyphenatedName(t *testing.T) {
	reg, store := setupLighthouseTest(t)

	// Register lighthouse with hyphenated name (e.g., "my-cool-tool")
	result := callLighthouse(t, reg, map[string]any{
		"action":      "register",
		"name":        "my-cool-tool",
		"description": "Hyphenated name test",
		"spec":        "Returns data",
	})
	taskID := result["task_id"].(string)
	if taskID != "T-LH-my-cool-tool-0" {
		t.Fatalf("task_id = %q, want T-LH-my-cool-tool-0", taskID)
	}

	// Wire registry
	store.registry = reg

	// Assign and submit
	assignment, err := store.AssignTask("worker-1")
	if err != nil {
		t.Fatalf("AssignTask: %v", err)
	}
	if assignment.TaskID != taskID {
		t.Fatalf("assigned = %q, want %q", assignment.TaskID, taskID)
	}

	submitResult, err := store.SubmitTask(taskID, "worker-1", "def456", "", nil)
	if err != nil {
		t.Fatalf("SubmitTask: %v", err)
	}
	if !submitResult.Success {
		t.Fatalf("submit failed: %s", submitResult.Message)
	}

	// Lighthouse "my-cool-tool" should be promoted (LastIndex correctly parses hyphenated names)
	lh, err := store.getLighthouse("my-cool-tool")
	if err != nil {
		t.Fatalf("getLighthouse: %v", err)
	}
	if lh.Status != "implemented" {
		t.Errorf("lighthouse status = %q, want implemented", lh.Status)
	}

	// Stub should be removed from registry
	if reg.HasTool("my-cool-tool") {
		t.Error("stub should be removed from registry after auto-promote")
	}
}

func TestStartupAutoPromoteWhenRealToolExists(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	// Insert a stub lighthouse
	if err := store.saveLighthouse(&Lighthouse{
		Name: "c4_health", Description: "Health check", InputSchema: `{"type":"object"}`,
		Status: "stub", Version: 1, CreatedBy: "test",
	}); err != nil {
		t.Fatalf("save lighthouse: %v", err)
	}

	// Register real tool FIRST (simulates RegisterHealthHandler running before LoadLighthousesOnStartup)
	reg := mcp.NewRegistry()
	reg.Register(mcp.ToolSchema{Name: "c4_health", Description: "Check subsystem health"}, func(args json.RawMessage) (any, error) {
		return map[string]any{"status": "healthy"}, nil
	})

	// Load — should auto-promote the stub since real tool exists
	n := LoadLighthousesOnStartup(reg, store)
	if n != 0 {
		t.Errorf("loaded = %d, want 0 (no stubs should be registered)", n)
	}

	// Real tool should still be there with original description
	schema, ok := reg.GetToolSchema("c4_health")
	if !ok {
		t.Fatal("c4_health should still be registered")
	}
	if schema.Description != "Check subsystem health" {
		t.Errorf("description = %q, want real tool description", schema.Description)
	}

	// Lighthouse status should be auto-promoted to "implemented"
	lh, err := store.getLighthouse("c4_health")
	if err != nil {
		t.Fatalf("getLighthouse: %v", err)
	}
	if lh.Status != "implemented" {
		t.Errorf("status = %q, want implemented (auto-promoted)", lh.Status)
	}
	if lh.PromotedBy != "auto-startup" {
		t.Errorf("promoted_by = %q, want auto-startup", lh.PromotedBy)
	}
}

func TestPromoteKeepsRealToolInRegistry(t *testing.T) {
	reg, _ := setupLighthouseTest(t)

	// Register a lighthouse stub
	callLighthouse(t, reg, map[string]any{
		"action": "register", "name": "lh_real_keep", "description": "Will have real impl", "auto_task": false,
	})

	// Now register a "real" tool with the same name (overwrites the stub)
	reg.Replace(mcp.ToolSchema{Name: "lh_real_keep", Description: "Real implementation"}, func(args json.RawMessage) (any, error) {
		return map[string]any{"real": true}, nil
	})

	// Promote — should NOT unregister the real tool
	result := callLighthouse(t, reg, map[string]any{"action": "promote", "name": "lh_real_keep"})
	if result["success"] != true {
		t.Fatalf("promote failed: %v", result)
	}

	// Real tool should still be registered
	if !reg.HasTool("lh_real_keep") {
		t.Error("real tool should remain in registry after promote")
	}

	// Verify it's the real implementation
	schema, ok := reg.GetToolSchema("lh_real_keep")
	if !ok {
		t.Fatal("tool should exist")
	}
	if schema.Description != "Real implementation" {
		t.Errorf("description = %q, want 'Real implementation'", schema.Description)
	}
}

func TestLighthouseAutoTaskFailurePath(t *testing.T) {
	reg, store := setupLighthouseTest(t)

	// Pre-create a task with the ID that auto_task would use, causing AddTask to fail
	dupeTask := &Task{
		ID:    "T-LH-lh_dupe_task-0",
		Title: "Pre-existing task",
		DoD:   "Already here",
	}
	if err := store.AddTask(dupeTask); err != nil {
		t.Fatalf("AddTask setup: %v", err)
	}

	// Register lighthouse — auto_task should fail silently (task ID collision)
	// but registration itself should still succeed
	result := callLighthouse(t, reg, map[string]any{
		"action":      "register",
		"name":        "lh_dupe_task",
		"description": "Task conflict test",
	})

	if result["success"] != true {
		t.Fatalf("register should succeed even when auto_task fails: %v", result)
	}

	// task_id should NOT be in result since AddTask failed
	if _, ok := result["task_id"]; ok {
		t.Error("task_id should not be present when auto_task creation fails")
	}

	// Lighthouse should still be registered in registry
	if !reg.HasTool("lh_dupe_task") {
		t.Error("lighthouse stub should be registered even when auto_task fails")
	}
}
