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
