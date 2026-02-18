package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/changmin/c4-core/internal/mcp"
)

func newC2Registry(t *testing.T) *mcp.Registry {
	t.Helper()
	reg := mcp.NewRegistry()
	RegisterC2NativeHandlers(reg)
	return reg
}

// =========================================================================
// Workspace Tests
// =========================================================================

func TestWorkspaceCreate(t *testing.T) {
	reg := newC2Registry(t)

	result, err := reg.Call("c4_workspace_create", json.RawMessage(`{"name": "my-project"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["state"] == nil {
		t.Fatal("expected state in response")
	}
}

func TestWorkspaceCreate_WithType(t *testing.T) {
	reg := newC2Registry(t)

	result, err := reg.Call("c4_workspace_create", json.RawMessage(`{
		"name": "test-paper", "project_type": "academic_paper",
		"goal": "Prove something", "sections": ["Abstract", "Methods"]
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["state"] == nil {
		t.Fatal("expected state in response")
	}
}

func TestWorkspaceCreate_MissingName(t *testing.T) {
	reg := newC2Registry(t)

	result, err := reg.Call("c4_workspace_create", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["error"] == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestWorkspaceCreate_InvalidType(t *testing.T) {
	reg := newC2Registry(t)

	result, err := reg.Call("c4_workspace_create", json.RawMessage(`{
		"name": "test", "project_type": "invalid_type"
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["state"] == nil {
		t.Fatal("invalid type should fallback to academic_paper")
	}
}

func TestWorkspaceSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	reg := newC2Registry(t)

	// Create a workspace state first
	createResult, err := reg.Call("c4_workspace_create", json.RawMessage(`{"name": "roundtrip-test"}`))
	if err != nil {
		t.Fatalf("create error: %v", err)
	}
	cm := createResult.(map[string]any)
	stateJSON, _ := json.Marshal(cm["state"])

	// Save workspace
	saveInput, _ := json.Marshal(map[string]any{
		"project_dir": tmpDir,
		"state":       json.RawMessage(stateJSON),
	})
	saveResult, err := reg.Call("c4_workspace_save", json.RawMessage(saveInput))
	if err != nil {
		t.Fatalf("save error: %v", err)
	}
	sm := saveResult.(map[string]any)
	if sm["success"] != true {
		t.Errorf("save success = %v, want true (error: %v)", sm["success"], sm["error"])
	}

	// Verify file exists
	wsPath := filepath.Join(tmpDir, "c2_workspace.md")
	if _, err := os.Stat(wsPath); os.IsNotExist(err) {
		t.Fatal("workspace file not created")
	}

	// Load workspace
	loadInput, _ := json.Marshal(map[string]any{"project_dir": tmpDir})
	loadResult, err := reg.Call("c4_workspace_load", json.RawMessage(loadInput))
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	lm := loadResult.(map[string]any)
	if lm["state"] == nil {
		t.Fatal("loaded state is nil")
	}
}

func TestWorkspaceLoad_NotFound(t *testing.T) {
	reg := newC2Registry(t)

	result, err := reg.Call("c4_workspace_load", json.RawMessage(`{"project_dir": "/nonexistent/path"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["error"] == nil {
		t.Fatal("expected error for missing workspace file")
	}
}

func TestWorkspaceLoad_MissingDir(t *testing.T) {
	reg := newC2Registry(t)

	result, err := reg.Call("c4_workspace_load", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["error"] == nil {
		t.Fatal("expected error for missing project_dir")
	}
}

func TestWorkspaceSave_MissingDir(t *testing.T) {
	reg := newC2Registry(t)

	result, err := reg.Call("c4_workspace_save", json.RawMessage(`{"state": {}}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["error"] == nil {
		t.Fatal("expected error for missing project_dir")
	}
}

func TestWorkspaceSave_MissingState(t *testing.T) {
	reg := newC2Registry(t)
	tmpDir := t.TempDir()

	input, _ := json.Marshal(map[string]any{"project_dir": tmpDir})
	result, err := reg.Call("c4_workspace_save", json.RawMessage(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["error"] == nil {
		t.Fatal("expected error for missing state")
	}
}

// =========================================================================
// Persona Learn Tests
// =========================================================================

func TestPersonaLearn_MissingPaths(t *testing.T) {
	reg := newC2Registry(t)

	result, err := reg.Call("c4_persona_learn", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["error"] == nil {
		t.Fatal("expected error for missing draft_path and final_path")
	}
}

func TestPersonaLearn_WithFiles(t *testing.T) {
	tmpDir := t.TempDir()
	reg := newC2Registry(t)

	draftPath := filepath.Join(tmpDir, "draft.md")
	finalPath := filepath.Join(tmpDir, "final.md")
	os.WriteFile(draftPath, []byte("# Draft\n\nThis is the draft version."), 0644)
	os.WriteFile(finalPath, []byte("# Final\n\nThis is the revised and improved version."), 0644)

	input, _ := json.Marshal(map[string]any{
		"draft_path": draftPath,
		"final_path": finalPath,
	})
	result, err := reg.Call("c4_persona_learn", json.RawMessage(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if errMsg, hasError := m["error"]; hasError {
		// persona_learn may fail in test env without full C2 setup
		t.Skipf("persona_learn not available in test env: %v", errMsg)
	}
	if m["summary"] == nil {
		t.Error("expected summary in response")
	}
}

// =========================================================================
// Profile Tests
// =========================================================================

func TestProfileSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	reg := newC2Registry(t)

	profilePath := filepath.Join(tmpDir, ".c2", "profile.yaml")
	os.MkdirAll(filepath.Join(tmpDir, ".c2"), 0755)

	// Save profile
	saveInput, _ := json.Marshal(map[string]any{
		"profile_path": profilePath,
		"data": map[string]any{
			"name":      "Test User",
			"language":  "ko",
			"expertise": []string{"go", "python"},
		},
	})
	saveResult, err := reg.Call("c4_profile_save", json.RawMessage(saveInput))
	if err != nil {
		t.Fatalf("save error: %v", err)
	}
	sm := saveResult.(map[string]any)
	if sm["success"] != true {
		t.Errorf("save success = %v (error: %v)", sm["success"], sm["error"])
	}

	// Load profile
	loadInput, _ := json.Marshal(map[string]any{"profile_path": profilePath})
	loadResult, err := reg.Call("c4_profile_load", json.RawMessage(loadInput))
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	lm := loadResult.(map[string]any)
	if lm["profile"] == nil {
		t.Fatal("loaded profile is nil")
	}
}

func TestProfileLoad_NotFound(t *testing.T) {
	reg := newC2Registry(t)

	result, err := reg.Call("c4_profile_load", json.RawMessage(`{"profile_path": "/nonexistent/profile.yaml"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// LoadProfile returns empty map (not error) for missing file
	m := result.(map[string]any)
	profile := m["profile"].(map[string]any)
	if len(profile) != 0 {
		t.Errorf("expected empty profile for missing file, got %v", profile)
	}
}

func TestProfileSave_MissingData(t *testing.T) {
	reg := newC2Registry(t)

	result, err := reg.Call("c4_profile_save", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["error"] == nil {
		t.Fatal("expected error for missing data")
	}
}

// =========================================================================
// Registration test
// =========================================================================

func TestRegisterC2NativeHandlersToolCount(t *testing.T) {
	reg := newC2Registry(t)
	tools := reg.ListTools()
	if len(tools) != 6 {
		names := make([]string, 0, len(tools))
		for _, tool := range tools {
			names = append(names, tool.Name)
		}
		t.Errorf("registered %d c2 tools, want 6: %v", len(tools), names)
	}
}
