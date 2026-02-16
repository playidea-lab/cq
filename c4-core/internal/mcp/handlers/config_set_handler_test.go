package handlers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInferType(t *testing.T) {
	tests := []struct {
		input string
		want  any
	}{
		{"true", true},
		{"false", false},
		{"True", true},
		{"FALSE", false},
		{"42", 42},
		{"0", 0},
		{"hello", "hello"},
		{"http://localhost:8080", "http://localhost:8080"},
		{"", ""},
	}
	for _, tt := range tests {
		got := inferType(tt.input)
		if got != tt.want {
			t.Errorf("inferType(%q) = %v (%T), want %v (%T)", tt.input, got, got, tt.want, tt.want)
		}
	}
}

func TestUpdateYAMLValue_TopLevel(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".c4", "config.yaml")
	os.MkdirAll(filepath.Dir(configPath), 0755)

	initial := `# Project config
project_id: c4
default_branch: main
`
	os.WriteFile(configPath, []byte(initial), 0644)

	// Update existing top-level key
	if err := updateYAMLValue(configPath, "project_id", "my-project"); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(configPath)
	content := string(data)
	if !strings.Contains(content, "project_id: my-project") {
		t.Errorf("expected project_id: my-project, got:\n%s", content)
	}
	// Comment preserved
	if !strings.Contains(content, "# Project config") {
		t.Error("comment was lost")
	}
}

func TestUpdateYAMLValue_Nested(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".c4", "config.yaml")
	os.MkdirAll(filepath.Dir(configPath), 0755)

	initial := `permission_reviewer:
  enabled: false
  model: haiku               # haiku, sonnet, opus
  timeout: 10
`
	os.WriteFile(configPath, []byte(initial), 0644)

	// Update nested key
	if err := updateYAMLValue(configPath, "permission_reviewer.enabled", "true"); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(configPath)
	content := string(data)
	if !strings.Contains(content, "  enabled: true") {
		t.Errorf("expected enabled: true, got:\n%s", content)
	}

	// Update model (should preserve inline comment)
	if err := updateYAMLValue(configPath, "permission_reviewer.model", "sonnet"); err != nil {
		t.Fatal(err)
	}

	data, _ = os.ReadFile(configPath)
	content = string(data)
	if !strings.Contains(content, "model: sonnet") {
		t.Errorf("expected model: sonnet, got:\n%s", content)
	}
	if !strings.Contains(content, "# haiku, sonnet, opus") {
		t.Errorf("inline comment was lost, got:\n%s", content)
	}
}

func TestUpdateYAMLValue_ThreeLevel(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".c4", "config.yaml")
	os.MkdirAll(filepath.Dir(configPath), 0755)

	initial := `economic_mode:
  enabled: true
  preset: standard
  model_routing:
    implementation: sonnet
    review: opus
`
	os.WriteFile(configPath, []byte(initial), 0644)

	if err := updateYAMLValue(configPath, "economic_mode.model_routing.implementation", "haiku"); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(configPath)
	content := string(data)
	if !strings.Contains(content, "    implementation: haiku") {
		t.Errorf("expected implementation: haiku, got:\n%s", content)
	}
	// Other values unchanged
	if !strings.Contains(content, "    review: opus") {
		t.Errorf("review should be unchanged, got:\n%s", content)
	}
}

func TestUpdateYAMLValue_AppendNew(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".c4", "config.yaml")
	os.MkdirAll(filepath.Dir(configPath), 0755)

	initial := `project_id: c4
`
	os.WriteFile(configPath, []byte(initial), 0644)

	// Append new top-level key
	if err := updateYAMLValue(configPath, "domain", "backend"); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(configPath)
	content := string(data)
	if !strings.Contains(content, "domain: backend") {
		t.Errorf("expected domain: backend, got:\n%s", content)
	}

	// Append new section with nested key
	if err := updateYAMLValue(configPath, "hub.enabled", "true"); err != nil {
		t.Fatal(err)
	}

	data, _ = os.ReadFile(configPath)
	content = string(data)
	if !strings.Contains(content, "hub:") {
		t.Errorf("expected hub: section, got:\n%s", content)
	}
	if !strings.Contains(content, "  enabled: true") {
		t.Errorf("expected nested enabled: true, got:\n%s", content)
	}
}

func TestUpdateYAMLValue_CreateFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".c4", "config.yaml")

	if err := updateYAMLValue(configPath, "project_id", "new-project"); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(configPath)
	content := string(data)
	if !strings.Contains(content, "project_id: new-project") {
		t.Errorf("expected project_id: new-project, got:\n%s", content)
	}
}

func TestUpdateYAMLValue_PreserveStructure(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".c4", "config.yaml")
	os.MkdirAll(filepath.Dir(configPath), 0755)

	initial := `# Main settings
project_id: c4

# Cloud config
cloud:
  enabled: false
  project_id: "c4"

# Hub config
hub:
  enabled: true
  url: "http://localhost:7123"
`
	os.WriteFile(configPath, []byte(initial), 0644)

	// Update cloud.enabled
	if err := updateYAMLValue(configPath, "cloud.enabled", "true"); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(configPath)
	content := string(data)

	// cloud.enabled updated
	if !strings.Contains(content, "  enabled: true") {
		t.Errorf("expected cloud enabled: true, got:\n%s", content)
	}
	// hub section unchanged
	if !strings.Contains(content, "  url: \"http://localhost:7123\"") {
		t.Errorf("hub url should be unchanged, got:\n%s", content)
	}
	// Comments preserved
	if !strings.Contains(content, "# Cloud config") {
		t.Errorf("comment lost, got:\n%s", content)
	}
}
