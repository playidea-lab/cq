package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/changmin/c4-core/internal/cloud"
	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/store"
)

// mockStore is a no-op store.Store implementation used in factory tests.
type mockStore struct{}

func (m *mockStore) GetStatus() (*store.ProjectStatus, error)    { return nil, nil }
func (m *mockStore) Start() error                                { return nil }
func (m *mockStore) Clear(keepConfig bool) error                 { return nil }
func (m *mockStore) TransitionState(from, to string) error       { return nil }
func (m *mockStore) AddTask(task *store.Task) error              { return nil }
func (m *mockStore) DeleteTask(taskID string) error              { return nil }
func (m *mockStore) GetTask(taskID string) (*store.Task, error)  { return nil, nil }
func (m *mockStore) AssignTask(workerID string) (*store.TaskAssignment, error) {
	return nil, nil
}
func (m *mockStore) SubmitTask(taskID, workerID, commitSHA, handoff string, results []store.ValidationResult) (*store.SubmitResult, error) {
	return nil, nil
}
func (m *mockStore) MarkBlocked(taskID, workerID, failureSignature string, attempts int, lastError string) error {
	return nil
}
func (m *mockStore) ListTasks(filter store.TaskFilter) ([]store.Task, int, error) {
	return nil, 0, nil
}
func (m *mockStore) ClaimTask(taskID string) (*store.Task, error)                      { return nil, nil }
func (m *mockStore) ReportTask(taskID, summary string, filesChanged []string) error    { return nil }
func (m *mockStore) Checkpoint(checkpointID, decision, notes string, requiredChanges []string, targetTaskID, targetReviewID string) (*store.CheckpointResult, error) {
	return nil, nil
}
func (m *mockStore) RequestChanges(reviewTaskID string, comments string, requiredChanges []string) (*store.RequestChangesResult, error) {
	return nil, nil
}

// TestIsValidCloudMode verifies valid and invalid mode values.
func TestIsValidCloudMode(t *testing.T) {
	tests := []struct {
		mode  string
		valid bool
	}{
		{"local-first", true},
		{"cloud-primary", true},
		{"invalid-value", false},
		{"", false},
		{"local", false},
		{"cloud", false},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			if got := isValidCloudMode(tt.mode); got != tt.valid {
				t.Errorf("isValidCloudMode(%q) = %v, want %v", tt.mode, got, tt.valid)
			}
		})
	}
}

// TestWriteCloudModeToYAML_NewFile verifies that a new config.yaml gets
// the cloud: section added with the correct mode.
func TestWriteCloudModeToYAML_NewFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	if err := writeCloudModeToYAML(configPath, "cloud-primary"); err != nil {
		t.Fatalf("writeCloudModeToYAML: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "cloud:") {
		t.Errorf("expected 'cloud:' section in config; got:\n%s", content)
	}
	if !strings.Contains(content, "  mode: cloud-primary") {
		t.Errorf("expected '  mode: cloud-primary' in config; got:\n%s", content)
	}
}

// TestWriteCloudModeToYAML_UpdateExisting verifies that an existing mode line
// is updated in-place.
func TestWriteCloudModeToYAML_UpdateExisting(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	initial := "project_id: test\ncloud:\n  mode: local-first\n"
	if err := os.WriteFile(configPath, []byte(initial), 0644); err != nil {
		t.Fatalf("writing initial config: %v", err)
	}

	if err := writeCloudModeToYAML(configPath, "cloud-primary"); err != nil {
		t.Fatalf("writeCloudModeToYAML: %v", err)
	}

	data, _ := os.ReadFile(configPath)
	content := string(data)
	if !strings.Contains(content, "  mode: cloud-primary") {
		t.Errorf("expected updated mode; got:\n%s", content)
	}
	if strings.Contains(content, "  mode: local-first") {
		t.Errorf("old mode should have been replaced; got:\n%s", content)
	}
}

// TestWriteCloudModeToYAML_InsertIntoExistingCloudSection verifies that mode
// is inserted when cloud: exists but mode is absent.
func TestWriteCloudModeToYAML_InsertIntoExistingCloudSection(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	initial := "project_id: test\ncloud:\n  enabled: true\n"
	if err := os.WriteFile(configPath, []byte(initial), 0644); err != nil {
		t.Fatalf("writing initial config: %v", err)
	}

	if err := writeCloudModeToYAML(configPath, "local-first"); err != nil {
		t.Fatalf("writeCloudModeToYAML: %v", err)
	}

	data, _ := os.ReadFile(configPath)
	content := string(data)
	if !strings.Contains(content, "  mode: local-first") {
		t.Errorf("expected mode inserted; got:\n%s", content)
	}
}

// TestCloudFactorySwitch verifies that the store factory selects the correct
// implementation based on cloud.mode.
func TestCloudFactorySwitch(t *testing.T) {
	dir := t.TempDir()
	c4Dir := filepath.Join(dir, ".c4")
	if err := os.MkdirAll(c4Dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Helper to write a config.yaml with a given mode.
	writeConfig := func(mode string) {
		var content string
		if mode != "" {
			content = "cloud:\n  mode: " + mode + "\n"
		}
		if err := os.WriteFile(filepath.Join(c4Dir, "config.yaml"), []byte(content), 0644); err != nil {
			t.Fatalf("write config: %v", err)
		}
	}

	t.Run("cloud-primary mode selects CloudPrimaryStore", func(t *testing.T) {
		writeConfig("cloud-primary")
		cfgMgr, err := config.New(dir)
		if err != nil {
			t.Fatalf("config.New: %v", err)
		}
		if cfgMgr.GetConfig().Cloud.Mode != "cloud-primary" {
			t.Errorf("expected cloud-primary, got %q", cfgMgr.GetConfig().Cloud.Mode)
		}
		local := &mockStore{}
		remote := &mockStore{}
		s := selectCloudStore(cfgMgr.GetConfig().Cloud.Mode, local, remote)
		if _, ok := s.(*cloud.CloudPrimaryStore); !ok {
			t.Errorf("expected *cloud.CloudPrimaryStore for cloud-primary mode, got %T", s)
		}
	})

	t.Run("local-first mode selects HybridStore", func(t *testing.T) {
		writeConfig("local-first")
		cfgMgr, err := config.New(dir)
		if err != nil {
			t.Fatalf("config.New: %v", err)
		}
		local := &mockStore{}
		remote := &mockStore{}
		s := selectCloudStore(cfgMgr.GetConfig().Cloud.Mode, local, remote)
		if _, ok := s.(*cloud.HybridStore); !ok {
			t.Errorf("expected *cloud.HybridStore for local-first mode, got %T", s)
		}
	})

	t.Run("unset mode defaults to HybridStore", func(t *testing.T) {
		writeConfig("")
		cfgMgr, err := config.New(dir)
		if err != nil {
			t.Fatalf("config.New: %v", err)
		}
		local := &mockStore{}
		remote := &mockStore{}
		s := selectCloudStore(cfgMgr.GetConfig().Cloud.Mode, local, remote)
		if _, ok := s.(*cloud.HybridStore); !ok {
			t.Errorf("expected *cloud.HybridStore for empty mode, got %T", s)
		}
	})
}

// TestCloudModeSetInvalidValue verifies that invalid values are rejected.
func TestCloudModeSetInvalidValue(t *testing.T) {
	dir := t.TempDir()
	origDir := projectDir
	t.Cleanup(func() { projectDir = origDir })
	projectDir = dir // override global for test

	os.MkdirAll(filepath.Join(dir, ".c4"), 0755)

	err := runCloudModeSet(nil, []string{"invalid-value"})
	if err == nil {
		t.Fatal("expected error for invalid mode, got nil")
	}
	if !strings.Contains(err.Error(), "invalid cloud mode") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestGetActiveProjectID verifies C4_PROJECT_ID env var takes priority over config.yaml.
func TestGetActiveProjectID(t *testing.T) {
	t.Run("C4_PROJECT_ID env var overrides config", func(t *testing.T) {
		t.Setenv("C4_PROJECT_ID", "test-id")
		dir := t.TempDir()
		got := getActiveProjectID(dir)
		if got != "test-id" {
			t.Errorf("expected %q, got %q", "test-id", got)
		}
	})

	t.Run("empty C4_PROJECT_ID falls back to config.yaml", func(t *testing.T) {
		t.Setenv("C4_PROJECT_ID", "")
		dir := t.TempDir()
		c4Dir := filepath.Join(dir, ".c4")
		os.MkdirAll(c4Dir, 0755)
		cfgContent := "cloud:\n  active_project_id: proj-from-config\n"
		os.WriteFile(filepath.Join(c4Dir, "config.yaml"), []byte(cfgContent), 0644)
		got := getActiveProjectID(dir)
		if got != "proj-from-config" {
			t.Errorf("expected %q, got %q", "proj-from-config", got)
		}
	})

	// Tests for getActiveProjectIDWithProjects (dir-name match + single auto-select).
	projects := []cloud.Project{
		{ID: "id-alpha", Name: "alpha"},
		{ID: "id-beta", Name: "beta"},
	}

	t.Run("env var takes priority over projects list", func(t *testing.T) {
		t.Setenv("C4_PROJECT_ID", "env-id")
		dir := t.TempDir()
		got := getActiveProjectIDWithProjects(dir, projects)
		if got != "env-id" {
			t.Errorf("expected %q, got %q", "env-id", got)
		}
	})

	t.Run("config active_project_id takes priority over dir-name match", func(t *testing.T) {
		t.Setenv("C4_PROJECT_ID", "")
		dir := t.TempDir()
		c4Dir := filepath.Join(dir, ".c4")
		os.MkdirAll(c4Dir, 0755)
		cfgContent := "cloud:\n  active_project_id: config-proj\n"
		os.WriteFile(filepath.Join(c4Dir, "config.yaml"), []byte(cfgContent), 0644)
		got := getActiveProjectIDWithProjects(dir, projects)
		if got != "config-proj" {
			t.Errorf("expected %q, got %q", "config-proj", got)
		}
	})

	t.Run("directory name matches project name case-insensitively", func(t *testing.T) {
		t.Setenv("C4_PROJECT_ID", "")
		dir := t.TempDir()
		// Create a subdirectory named "Alpha" to match project "alpha".
		subDir := filepath.Join(dir, "Alpha")
		os.MkdirAll(subDir, 0755)
		origDir, _ := os.Getwd()
		os.Chdir(subDir)
		t.Cleanup(func() { os.Chdir(origDir) })
		got := getActiveProjectIDWithProjects(dir, projects)
		if got != "id-alpha" {
			t.Errorf("expected %q, got %q", "id-alpha", got)
		}
	})

	t.Run("single project auto-selected when no other match", func(t *testing.T) {
		t.Setenv("C4_PROJECT_ID", "")
		dir := t.TempDir()
		single := []cloud.Project{{ID: "only-proj", Name: "unrelated-name"}}
		got := getActiveProjectIDWithProjects(dir, single)
		if got != "only-proj" {
			t.Errorf("expected %q, got %q", "only-proj", got)
		}
	})

	t.Run("returns empty when multiple projects and no match", func(t *testing.T) {
		t.Setenv("C4_PROJECT_ID", "")
		dir := t.TempDir()
		got := getActiveProjectIDWithProjects(dir, projects)
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})

	t.Run("returns empty for empty project list", func(t *testing.T) {
		t.Setenv("C4_PROJECT_ID", "")
		dir := t.TempDir()
		got := getActiveProjectIDWithProjects(dir, nil)
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})
}
