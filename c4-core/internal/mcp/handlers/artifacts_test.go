package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
