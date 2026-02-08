package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigYaml(t *testing.T) {
	// Create temp directory with .c4/config.yaml
	tmpDir := t.TempDir()
	c4Dir := filepath.Join(tmpDir, ".c4")
	if err := os.MkdirAll(c4Dir, 0o755); err != nil {
		t.Fatal(err)
	}

	configYAML := `
project_id: test-project
default_branch: develop
work_branch_prefix: "test/w-"
poll_interval_ms: 2000
max_idle_minutes: 30
scope_lock_ttl_sec: 1800
economic_mode:
  enabled: true
  preset: economic
`
	if err := os.WriteFile(filepath.Join(c4Dir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr, err := New(tmpDir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	cfg := mgr.GetConfig()

	if cfg.ProjectID != "test-project" {
		t.Errorf("ProjectID = %q, want %q", cfg.ProjectID, "test-project")
	}
	if cfg.DefaultBranch != "develop" {
		t.Errorf("DefaultBranch = %q, want %q", cfg.DefaultBranch, "develop")
	}
	if cfg.WorkBranchPrefix != "test/w-" {
		t.Errorf("WorkBranchPrefix = %q, want %q", cfg.WorkBranchPrefix, "test/w-")
	}
	if cfg.PollIntervalMs != 2000 {
		t.Errorf("PollIntervalMs = %d, want %d", cfg.PollIntervalMs, 2000)
	}
	if cfg.MaxIdleMinutes != 30 {
		t.Errorf("MaxIdleMinutes = %d, want %d", cfg.MaxIdleMinutes, 30)
	}
	if cfg.ScopeLockTTLSec != 1800 {
		t.Errorf("ScopeLockTTLSec = %d, want %d", cfg.ScopeLockTTLSec, 1800)
	}
}

func TestEconomicPresetResolution(t *testing.T) {
	tests := []struct {
		name           string
		preset         string
		wantImpl       string
		wantReview     string
		wantCheckpoint string
	}{
		{
			name:           "standard preset",
			preset:         "standard",
			wantImpl:       "sonnet",
			wantReview:     "opus",
			wantCheckpoint: "opus",
		},
		{
			name:           "economic preset",
			preset:         "economic",
			wantImpl:       "sonnet",
			wantReview:     "sonnet",
			wantCheckpoint: "sonnet",
		},
		{
			name:           "ultra-economic preset",
			preset:         "ultra-economic",
			wantImpl:       "haiku",
			wantReview:     "sonnet",
			wantCheckpoint: "sonnet",
		},
		{
			name:           "quality preset",
			preset:         "quality",
			wantImpl:       "opus",
			wantReview:     "opus",
			wantCheckpoint: "opus",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			c4Dir := filepath.Join(tmpDir, ".c4")
			if err := os.MkdirAll(c4Dir, 0o755); err != nil {
				t.Fatal(err)
			}

			yaml := "project_id: test\neconomic_mode:\n  enabled: true\n  preset: " + tc.preset + "\n"
			if err := os.WriteFile(filepath.Join(c4Dir, "config.yaml"), []byte(yaml), 0o644); err != nil {
				t.Fatal(err)
			}

			mgr, err := New(tmpDir)
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}

			cfg := mgr.GetConfig()

			if cfg.EconomicMode.ModelRouting.Implementation != tc.wantImpl {
				t.Errorf("Implementation = %q, want %q", cfg.EconomicMode.ModelRouting.Implementation, tc.wantImpl)
			}
			if cfg.EconomicMode.ModelRouting.Review != tc.wantReview {
				t.Errorf("Review = %q, want %q", cfg.EconomicMode.ModelRouting.Review, tc.wantReview)
			}
			if cfg.EconomicMode.ModelRouting.Checkpoint != tc.wantCheckpoint {
				t.Errorf("Checkpoint = %q, want %q", cfg.EconomicMode.ModelRouting.Checkpoint, tc.wantCheckpoint)
			}
		})
	}
}

func TestMissingConfigFileDefaults(t *testing.T) {
	// Use a temp directory with no config file
	tmpDir := t.TempDir()

	mgr, err := New(tmpDir)
	if err != nil {
		t.Fatalf("New() should not fail with missing config: %v", err)
	}

	cfg := mgr.GetConfig()

	// Verify defaults are applied
	if cfg.ProjectID != "c4" {
		t.Errorf("ProjectID = %q, want %q", cfg.ProjectID, "c4")
	}
	if cfg.DefaultBranch != "main" {
		t.Errorf("DefaultBranch = %q, want %q", cfg.DefaultBranch, "main")
	}
	if cfg.PollIntervalMs != 1000 {
		t.Errorf("PollIntervalMs = %d, want %d", cfg.PollIntervalMs, 1000)
	}
	if cfg.MaxIdleMinutes != 60 {
		t.Errorf("MaxIdleMinutes = %d, want %d", cfg.MaxIdleMinutes, 60)
	}
	if cfg.WorkerTTLMinutes != 30 {
		t.Errorf("WorkerTTLMinutes = %d, want %d", cfg.WorkerTTLMinutes, 30)
	}
	if cfg.ScopeLockTTLSec != 3600 {
		t.Errorf("ScopeLockTTLSec = %d, want %d", cfg.ScopeLockTTLSec, 3600)
	}
	if cfg.EconomicMode.Enabled {
		t.Error("EconomicMode.Enabled should be false by default")
	}
	if backend := mgr.GetBackend(); backend != "sqlite" {
		t.Errorf("GetBackend() = %q, want %q", backend, "sqlite")
	}
}

func TestOverrideWithEnvVars(t *testing.T) {
	tmpDir := t.TempDir()

	// Set environment variables
	t.Setenv("C4_PROJECT_ID", "env-project")
	t.Setenv("C4_DEFAULT_BRANCH", "env-main")

	mgr, err := New(tmpDir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Viper's Get should return env var value
	if got := mgr.GetString("project_id"); got != "env-project" {
		t.Errorf("Get(project_id) = %q, want %q", got, "env-project")
	}
	if got := mgr.GetString("default_branch"); got != "env-main" {
		t.Errorf("Get(default_branch) = %q, want %q", got, "env-main")
	}
}

func TestGetModelForTask(t *testing.T) {
	t.Run("economic mode enabled", func(t *testing.T) {
		tmpDir := t.TempDir()
		c4Dir := filepath.Join(tmpDir, ".c4")
		if err := os.MkdirAll(c4Dir, 0o755); err != nil {
			t.Fatal(err)
		}

		yaml := "project_id: test\neconomic_mode:\n  enabled: true\n  preset: standard\n"
		if err := os.WriteFile(filepath.Join(c4Dir, "config.yaml"), []byte(yaml), 0o644); err != nil {
			t.Fatal(err)
		}

		mgr, err := New(tmpDir)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}

		// T- -> implementation (sonnet)
		if got := mgr.GetModelForTask("T-001-0"); got != "sonnet" {
			t.Errorf("T-001-0 -> %q, want %q", got, "sonnet")
		}

		// R- -> review (opus)
		if got := mgr.GetModelForTask("R-001-0"); got != "opus" {
			t.Errorf("R-001-0 -> %q, want %q", got, "opus")
		}

		// CP- -> checkpoint (opus)
		if got := mgr.GetModelForTask("CP-001"); got != "opus" {
			t.Errorf("CP-001 -> %q, want %q", got, "opus")
		}

		// RPR- -> implementation (sonnet)
		if got := mgr.GetModelForTask("RPR-001-1"); got != "sonnet" {
			t.Errorf("RPR-001-1 -> %q, want %q", got, "sonnet")
		}
	})

	t.Run("economic mode disabled", func(t *testing.T) {
		tmpDir := t.TempDir()

		mgr, err := New(tmpDir)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}

		// Should return empty when disabled
		if got := mgr.GetModelForTask("T-001-0"); got != "" {
			t.Errorf("T-001-0 -> %q, want empty", got)
		}
		if got := mgr.GetModelForTask("R-001-0"); got != "" {
			t.Errorf("R-001-0 -> %q, want empty", got)
		}
	})
}

func TestIsPreset(t *testing.T) {
	validPresets := []string{"standard", "economic", "ultra-economic", "quality"}
	for _, p := range validPresets {
		if !IsPreset(p) {
			t.Errorf("IsPreset(%q) = false, want true", p)
		}
	}

	if IsPreset("invalid") {
		t.Error("IsPreset(invalid) = true, want false")
	}
}

func TestGetBackendDefault(t *testing.T) {
	tmpDir := t.TempDir()

	mgr, err := New(tmpDir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if got := mgr.GetBackend(); got != "sqlite" {
		t.Errorf("GetBackend() = %q, want %q", got, "sqlite")
	}
}
