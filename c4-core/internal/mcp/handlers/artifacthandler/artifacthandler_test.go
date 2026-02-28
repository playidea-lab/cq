package artifacthandler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/changmin/c4-core/internal/mcp"
)

func TestArtifactSave_Success(t *testing.T) {
	dir := t.TempDir()

	// Create a source file
	srcPath := filepath.Join(dir, "model.pt")
	if err := os.WriteFile(srcPath, []byte("fake model data"), 0644); err != nil {
		t.Fatal(err)
	}

	args, _ := json.Marshal(map[string]string{
		"source_path": srcPath,
		"name":        "test-model",
		"description": "Test artifact",
	})

	result, err := handleArtifactSave(dir, json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["success"] != true {
		t.Errorf("success = %v, want true", m["success"])
	}
	if m["name"] != "test-model" {
		t.Errorf("name = %v, want test-model", m["name"])
	}
	if m["hash"] == nil || m["hash"] == "" {
		t.Error("expected non-empty hash")
	}
	if m["size"] != 15 { // len("fake model data")
		t.Errorf("size = %v, want 15", m["size"])
	}

	// Verify metadata.json was written
	metaPath := filepath.Join(dir, ".c4", "artifacts", "test-model", "metadata.json")
	if _, err := os.Stat(metaPath); err != nil {
		t.Errorf("metadata.json not found: %v", err)
	}
}

func TestArtifactSave_MissingSource(t *testing.T) {
	dir := t.TempDir()

	args, _ := json.Marshal(map[string]string{
		"source_path": filepath.Join(dir, "nonexistent.pt"),
		"name":        "test-model",
	})

	_, err := handleArtifactSave(dir, json.RawMessage(args))
	if err == nil {
		t.Fatal("expected error for missing source file")
	}
}

func TestArtifactSave_SHA256Consistency(t *testing.T) {
	dir := t.TempDir()

	srcPath := filepath.Join(dir, "data.bin")
	if err := os.WriteFile(srcPath, []byte("consistent data"), 0644); err != nil {
		t.Fatal(err)
	}

	args, _ := json.Marshal(map[string]string{
		"source_path": srcPath,
		"name":        "hash-test",
	})

	result1, _ := handleArtifactSave(dir, json.RawMessage(args))
	m1 := result1.(map[string]any)

	// Save again with same content — hash should be identical
	result2, _ := handleArtifactSave(dir, json.RawMessage(args))
	m2 := result2.(map[string]any)

	if m1["hash"] != m2["hash"] {
		t.Errorf("hash mismatch: %v != %v", m1["hash"], m2["hash"])
	}
}

func TestArtifactList_Empty(t *testing.T) {
	dir := t.TempDir()

	result, err := handleArtifactList(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["count"] != 0 {
		t.Errorf("count = %v, want 0", m["count"])
	}
}

func TestArtifactList_WithItems(t *testing.T) {
	dir := t.TempDir()

	// Create two artifacts via save
	for _, name := range []string{"model-a", "model-b"} {
		srcPath := filepath.Join(dir, name+".bin")
		os.WriteFile(srcPath, []byte("data-"+name), 0644)

		args, _ := json.Marshal(map[string]string{
			"source_path": srcPath,
			"name":        name,
		})
		if _, err := handleArtifactSave(dir, json.RawMessage(args)); err != nil {
			t.Fatalf("save %s: %v", name, err)
		}
	}

	result, err := handleArtifactList(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["count"] != 2 {
		t.Errorf("count = %v, want 2", m["count"])
	}
}

func TestArtifactGet_Success(t *testing.T) {
	dir := t.TempDir()

	// Save an artifact first
	srcPath := filepath.Join(dir, "get-test.bin")
	os.WriteFile(srcPath, []byte("get test data"), 0644)

	saveArgs, _ := json.Marshal(map[string]string{
		"source_path": srcPath,
		"name":        "get-artifact",
		"description": "Artifact for get test",
	})
	if _, err := handleArtifactSave(dir, json.RawMessage(saveArgs)); err != nil {
		t.Fatal(err)
	}

	// Get it
	getArgs, _ := json.Marshal(map[string]string{"name": "get-artifact"})
	result, err := handleArtifactGet(dir, json.RawMessage(getArgs))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["name"] != "get-artifact" {
		t.Errorf("name = %v, want get-artifact", m["name"])
	}
	if m["description"] != "Artifact for get test" {
		t.Errorf("description = %v, want 'Artifact for get test'", m["description"])
	}
}

func TestArtifactGet_NotFound(t *testing.T) {
	dir := t.TempDir()

	args, _ := json.Marshal(map[string]string{"name": "nonexistent"})
	result, err := handleArtifactGet(dir, json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if _, hasError := m["error"]; !hasError {
		t.Error("expected error field for missing artifact")
	}
}

func TestRegister_RegistersExpectedTools(t *testing.T) {
	dir := t.TempDir()
	reg := mcp.NewRegistry()
	Register(reg, dir)

	expected := []string{"c4_artifact_save", "c4_artifact_list", "c4_artifact_get"}
	for _, name := range expected {
		if !reg.HasTool(name) {
			t.Errorf("expected tool %q to be registered", name)
		}
	}

	tools := reg.ListTools()
	if len(tools) != len(expected) {
		t.Errorf("ListTools() returned %d tools, want %d", len(tools), len(expected))
	}
}

func TestRegister_CallsRouteToHandlers(t *testing.T) {
	dir := t.TempDir()
	reg := mcp.NewRegistry()
	Register(reg, dir)

	// c4_artifact_list should succeed (returns empty list)
	result, err := reg.Call("c4_artifact_list", json.RawMessage("{}"))
	if err != nil {
		t.Fatalf("c4_artifact_list: unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["count"] == nil {
		t.Error("expected count field in c4_artifact_list result")
	}

	// c4_artifact_get with missing artifact returns error field (not Go error)
	getArgs, _ := json.Marshal(map[string]string{"name": "no-such-artifact"})
	result2, err := reg.Call("c4_artifact_get", json.RawMessage(getArgs))
	if err != nil {
		t.Fatalf("c4_artifact_get: unexpected error: %v", err)
	}
	m2 := result2.(map[string]any)
	if _, hasError := m2["error"]; !hasError {
		t.Error("expected error field for missing artifact in c4_artifact_get")
	}
}

func TestResolvePath_AbsoluteInRoot(t *testing.T) {
	dir := t.TempDir()
	absPath := filepath.Join(dir, "subdir", "file.txt")

	got, err := resolvePath(dir, absPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != absPath {
		t.Errorf("resolvePath = %q, want %q", got, absPath)
	}
}

func TestResolvePath_AbsoluteEscapes(t *testing.T) {
	dir := t.TempDir()
	outsidePath := "/tmp/evil/file.txt"

	_, err := resolvePath(dir, outsidePath)
	if err == nil {
		t.Fatal("expected error for absolute path escaping root")
	}
	if !strings.Contains(err.Error(), "escapes") {
		t.Errorf("error message %q should mention 'escapes'", err.Error())
	}
}

func TestResolvePath_Relative(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, "models", "v1.pt")

	got, err := resolvePath(dir, "models/v1.pt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("resolvePath = %q, want %q", got, want)
	}
}

func TestResolvePath_Empty(t *testing.T) {
	dir := t.TempDir()

	got, err := resolvePath(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != filepath.Clean(dir) {
		t.Errorf("resolvePath(\"\") = %q, want %q", got, filepath.Clean(dir))
	}
}

func TestResolvePath_TraversalBlocked(t *testing.T) {
	dir := t.TempDir()

	_, err := resolvePath(dir, "../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal attempt")
	}
	if !strings.Contains(err.Error(), "escapes") {
		t.Errorf("error message %q should mention 'escapes'", err.Error())
	}
}
