package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/changmin/c4-core/internal/mcp"
)

// TestHandleSaveDoc_Spec tests saving spec documents
func TestHandleSaveDoc_Spec(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		args    string
		wantErr bool
		errMsg  string
	}{
		{
			name: "normal save spec",
			args: `{
				"name": "api-spec",
				"content": "# API Specification\n\nRESTful API design..."
			}`,
			wantErr: false,
		},
		{
			name: "empty name creates file",
			args: `{"name": "", "content": "test"}`,
			wantErr: false,
			// Note: handleSaveDoc doesn't validate empty name - it just creates ".md" file
		},
		{
			name: "empty content creates empty file",
			args: `{"name": "empty-test", "content": ""}`,
			wantErr: false,
			// Note: handleSaveDoc doesn't validate empty content - it just creates empty file
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handleSaveDoc(tmpDir, "specs", json.RawMessage(tt.args))

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("error = %q, want substring %q", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			m, ok := result.(map[string]any)
			if !ok {
				t.Fatalf("result type = %T, want map[string]any", result)
			}
			if m["success"] != true {
				t.Errorf("success = %v, want true", m["success"])
			}

			// Verify file was created
			path := filepath.Join(tmpDir, ".c4", "specs", "api-spec.md")
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Errorf("file not created at %s", path)
			}
		})
	}
}

// TestHandleSaveDoc_Design tests saving design documents
func TestHandleSaveDoc_Design(t *testing.T) {
	tmpDir := t.TempDir()

	args := `{
		"name": "system-design",
		"content": "# System Design\n\nArchitecture overview..."
	}`

	result, err := handleSaveDoc(tmpDir, "designs", json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["success"] != true {
		t.Errorf("success = %v, want true", m["success"])
	}

	// Verify file was created in designs directory
	path := filepath.Join(tmpDir, ".c4", "designs", "system-design.md")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("file not created at %s", path)
	}

	// Verify content
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !contains(string(content), "Architecture overview") {
		t.Errorf("content does not match expected")
	}
}

// TestHandleGetDoc tests retrieving documents
func TestHandleGetDoc(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test spec first
	specsDir := filepath.Join(tmpDir, ".c4", "specs")
	if err := os.MkdirAll(specsDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	testContent := "# Test Spec\n\nContent here"
	testPath := filepath.Join(specsDir, "test-spec.md")
	if err := os.WriteFile(testPath, []byte(testContent), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	tests := []struct {
		name       string
		args       string
		wantErr    bool
		wantNotFound bool
		wantContent string
	}{
		{
			name:        "normal get spec",
			args:        `{"name": "test-spec"}`,
			wantErr:     false,
			wantContent: testContent,
		},
		{
			name:         "non-existent spec",
			args:         `{"name": "nonexistent"}`,
			wantErr:      false,
			wantNotFound: true,
		},
		{
			name:         "empty name treated as non-existent",
			args:         `{}`,
			wantErr:      false,
			wantNotFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handleGetDoc(tmpDir, "specs", json.RawMessage(tt.args))

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			m, ok := result.(map[string]any)
			if !ok {
				t.Fatalf("result type = %T, want map[string]any", result)
			}

			if tt.wantNotFound {
				if _, hasError := m["error"]; !hasError {
					t.Error("expected error field for non-existent doc")
				}
				return
			}

			if content, ok := m["content"].(string); ok {
				if content != tt.wantContent {
					t.Errorf("content = %q, want %q", content, tt.wantContent)
				}
			} else {
				t.Error("content field missing or wrong type")
			}
		})
	}
}

// TestHandleListDocs tests listing documents
func TestHandleListDocs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple test specs
	specsDir := filepath.Join(tmpDir, ".c4", "specs")
	if err := os.MkdirAll(specsDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	specs := []string{"spec1.md", "spec2.md", "spec3.yaml"}
	for _, name := range specs {
		path := filepath.Join(specsDir, name)
		if err := os.WriteFile(path, []byte("test content"), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	result, err := handleListDocs(tmpDir, "specs")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}

	count, ok := m["count"].(int)
	if !ok {
		t.Fatal("count field missing or wrong type")
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
}

// TestHandleListDocs_EmptyDirectory tests listing when directory doesn't exist
func TestHandleListDocs_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	result, err := handleListDocs(tmpDir, "specs")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	count, ok := m["count"].(int)
	if !ok {
		t.Fatal("count field missing or wrong type")
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

// TestDiscoveryComplete tests the discovery complete transition
func TestDiscoveryComplete(t *testing.T) {
	store := newMockStore()

	handler := makeTransitionHandler(store, "DISCOVERY", "DESIGN")
	result, err := handler(json.RawMessage("{}"))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}

	if m["success"] != true {
		t.Errorf("success = %v, want true", m["success"])
	}

	msg, ok := m["message"].(string)
	if !ok {
		t.Fatal("message field missing or wrong type")
	}
	if !contains(msg, "DISCOVERY complete") {
		t.Errorf("message = %q, want substring 'DISCOVERY complete'", msg)
	}
}

// TestDesignComplete tests the design complete transition
func TestDesignComplete(t *testing.T) {
	store := newMockStore()

	handler := makeTransitionHandler(store, "DESIGN", "PLAN")
	result, err := handler(json.RawMessage("{}"))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}

	if m["success"] != true {
		t.Errorf("success = %v, want true", m["success"])
	}

	msg, ok := m["message"].(string)
	if !ok {
		t.Fatal("message field missing or wrong type")
	}
	if !contains(msg, "DESIGN complete") {
		t.Errorf("message = %q, want substring 'DESIGN complete'", msg)
	}
	if !contains(msg, "PLAN") {
		t.Errorf("message = %q, want substring 'PLAN'", msg)
	}
}

// TestDiscoveryHandlersViaRegistry tests handlers through MCP registry
func TestDiscoveryHandlersViaRegistry(t *testing.T) {
	tmpDir := t.TempDir()
	store := newMockStore()
	reg := mcp.NewRegistry()
	RegisterDiscoveryHandlers(reg, store, tmpDir)

	t.Run("c4_save_spec via registry", func(t *testing.T) {
		args := `{"name": "registry-spec", "content": "# Registry Test"}`
		result, err := reg.Call("c4_save_spec", json.RawMessage(args))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		m := result.(map[string]any)
		if m["success"] != true {
			t.Errorf("success = %v, want true", m["success"])
		}
	})

	t.Run("c4_get_spec via registry", func(t *testing.T) {
		args := `{"name": "registry-spec"}`
		result, err := reg.Call("c4_get_spec", json.RawMessage(args))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		m := result.(map[string]any)
		if _, ok := m["content"]; !ok {
			t.Error("content field missing")
		}
	})

	t.Run("c4_list_specs via registry", func(t *testing.T) {
		result, err := reg.Call("c4_list_specs", json.RawMessage("{}"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		m := result.(map[string]any)
		count, ok := m["count"].(int)
		if !ok {
			t.Fatal("count field missing")
		}
		if count < 1 {
			t.Errorf("count = %d, want at least 1", count)
		}
	})

	t.Run("c4_save_design via registry", func(t *testing.T) {
		args := `{"name": "registry-design", "content": "# Design Test"}`
		result, err := reg.Call("c4_save_design", json.RawMessage(args))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		m := result.(map[string]any)
		if m["success"] != true {
			t.Errorf("success = %v, want true", m["success"])
		}
	})

	t.Run("c4_get_design via registry", func(t *testing.T) {
		args := `{"name": "registry-design"}`
		result, err := reg.Call("c4_get_design", json.RawMessage(args))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		m := result.(map[string]any)
		if _, ok := m["content"]; !ok {
			t.Error("content field missing")
		}
	})

	t.Run("c4_list_designs via registry", func(t *testing.T) {
		result, err := reg.Call("c4_list_designs", json.RawMessage("{}"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		m := result.(map[string]any)
		count, ok := m["count"].(int)
		if !ok {
			t.Fatal("count field missing")
		}
		if count < 1 {
			t.Errorf("count = %d, want at least 1", count)
		}
	})

	t.Run("c4_discovery_complete via registry", func(t *testing.T) {
		result, err := reg.Call("c4_discovery_complete", json.RawMessage("{}"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		m := result.(map[string]any)
		if m["success"] != true {
			t.Errorf("success = %v, want true", m["success"])
		}
	})

	t.Run("c4_design_complete via registry", func(t *testing.T) {
		result, err := reg.Call("c4_design_complete", json.RawMessage("{}"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		m := result.(map[string]any)
		if m["success"] != true {
			t.Errorf("success = %v, want true", m["success"])
		}
	})

	t.Run("c4_ensure_supervisor via registry", func(t *testing.T) {
		result, err := reg.Call("c4_ensure_supervisor", json.RawMessage("{}"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		m := result.(map[string]any)
		if m["success"] != true {
			t.Errorf("success = %v, want true", m["success"])
		}
	})
}

// TestHandleSaveDoc_Overwrite tests overwriting existing documents
func TestHandleSaveDoc_Overwrite(t *testing.T) {
	tmpDir := t.TempDir()

	// Save first version
	args1 := `{"name": "overwrite-test", "content": "Version 1"}`
	_, err := handleSaveDoc(tmpDir, "specs", json.RawMessage(args1))
	if err != nil {
		t.Fatalf("first save failed: %v", err)
	}

	// Overwrite with second version
	args2 := `{"name": "overwrite-test", "content": "Version 2 - Updated"}`
	result, err := handleSaveDoc(tmpDir, "specs", json.RawMessage(args2))
	if err != nil {
		t.Fatalf("second save failed: %v", err)
	}

	m := result.(map[string]any)
	if m["success"] != true {
		t.Errorf("success = %v, want true", m["success"])
	}

	// Verify content is updated
	path := filepath.Join(tmpDir, ".c4", "specs", "overwrite-test.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !contains(string(content), "Version 2") {
		t.Errorf("content not updated: %s", string(content))
	}
}
