package handlers

import (
	"database/sql"
	"encoding/json"
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
