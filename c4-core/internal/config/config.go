// Package config handles C4 project configuration loading and validation.
//
// Configuration is loaded from .c4/config.yaml and includes:
//   - Project metadata (name, domain, description)
//   - Validation rules (lint, unit test, integration test commands)
//   - Worker settings (max workers, heartbeat interval, TTL)
//   - Economic mode presets (standard, economic, quality, etc.)
//
// Environment variable overrides use the C4_ prefix (e.g., C4_PROJECT_ID).
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// ModelRouting maps task type prefixes to Claude model names.
type ModelRouting struct {
	Implementation string `mapstructure:"implementation" yaml:"implementation"`
	Review         string `mapstructure:"review"         yaml:"review"`
	Checkpoint     string `mapstructure:"checkpoint"     yaml:"checkpoint"`
	Scout          string `mapstructure:"scout"          yaml:"scout"`
	Debug          string `mapstructure:"debug"          yaml:"debug"`
	Planning       string `mapstructure:"planning"       yaml:"planning"`
}

// EconomicMode holds economic mode configuration.
type EconomicMode struct {
	Enabled      bool         `mapstructure:"enabled"       yaml:"enabled"`
	Preset       string       `mapstructure:"preset"        yaml:"preset"`
	ModelRouting ModelRouting `mapstructure:"model_routing" yaml:"model_routing"`
}

// WorktreeConfig holds worktree settings.
type WorktreeConfig struct {
	Enabled     bool `mapstructure:"enabled"      yaml:"enabled"`
	AutoCleanup bool `mapstructure:"auto_cleanup" yaml:"auto_cleanup"`
}

// ValidationConfig holds validation command settings.
type ValidationConfig struct {
	Lint string `mapstructure:"lint" yaml:"lint"`
	Unit string `mapstructure:"unit" yaml:"unit"`
}

// C4Config is the top-level configuration schema.
// It mirrors the Python C4Config model for YAML format compatibility.
type C4Config struct {
	ProjectID        string           `mapstructure:"project_id"          yaml:"project_id"`
	DefaultBranch    string           `mapstructure:"default_branch"      yaml:"default_branch"`
	WorkBranchPrefix string           `mapstructure:"work_branch_prefix"  yaml:"work_branch_prefix"`
	PollIntervalMs   int              `mapstructure:"poll_interval_ms"    yaml:"poll_interval_ms"`
	MaxIdleMinutes   int              `mapstructure:"max_idle_minutes"    yaml:"max_idle_minutes"`
	WorkerTTLMinutes int              `mapstructure:"worker_ttl_minutes"  yaml:"worker_ttl_minutes"`
	ScopeLockTTLSec  int              `mapstructure:"scope_lock_ttl_sec"  yaml:"scope_lock_ttl_sec"`
	Domain           string           `mapstructure:"domain"              yaml:"domain"`
	Validation       ValidationConfig `mapstructure:"validation"          yaml:"validation"`
	Worktree         WorktreeConfig   `mapstructure:"worktree"            yaml:"worktree"`
	EconomicMode     EconomicMode     `mapstructure:"economic_mode"       yaml:"economic_mode"`
	ReviewAsTask     bool             `mapstructure:"review_as_task"      yaml:"review_as_task"`
	CheckpointAsTask bool             `mapstructure:"checkpoint_as_task"  yaml:"checkpoint_as_task"`
}

// presetConfigs defines the economic mode presets.
// These match the Python PRESET_CONFIGS dictionary exactly.
var presetConfigs = map[string]ModelRouting{
	"standard": {
		Implementation: "sonnet",
		Review:         "opus",
		Checkpoint:     "opus",
		Scout:          "haiku",
		Debug:          "haiku",
		Planning:       "sonnet",
	},
	"economic": {
		Implementation: "sonnet",
		Review:         "sonnet",
		Checkpoint:     "sonnet",
		Scout:          "haiku",
		Debug:          "haiku",
		Planning:       "sonnet",
	},
	"ultra-economic": {
		Implementation: "haiku",
		Review:         "sonnet",
		Checkpoint:     "sonnet",
		Scout:          "haiku",
		Debug:          "haiku",
		Planning:       "haiku",
	},
	"quality": {
		Implementation: "opus",
		Review:         "opus",
		Checkpoint:     "opus",
		Scout:          "sonnet",
		Debug:          "sonnet",
		Planning:       "opus",
	},
}

// defaultConfig returns a C4Config with sane defaults.
func defaultConfig() C4Config {
	return C4Config{
		ProjectID:        "c4",
		DefaultBranch:    "main",
		WorkBranchPrefix: "c4/w-",
		PollIntervalMs:   1000,
		MaxIdleMinutes:   60,
		WorkerTTLMinutes: 30,
		ScopeLockTTLSec:  3600,
		ReviewAsTask:     true,
		CheckpointAsTask: true,
		Worktree: WorktreeConfig{
			Enabled:     true,
			AutoCleanup: true,
		},
		EconomicMode: EconomicMode{
			Enabled:      false,
			Preset:       "economic",
			ModelRouting: presetConfigs["standard"],
		},
	}
}

// Manager provides access to C4 configuration.
type Manager struct {
	v      *viper.Viper
	config C4Config
}

// New creates a new config Manager and loads configuration from the
// given project root directory. It looks for .c4/config.yaml.
//
// If the config file does not exist, defaults are used.
// Environment variables with the C4_ prefix override config values.
func New(projectRoot string) (*Manager, error) {
	v := viper.New()

	// Set defaults
	defaults := defaultConfig()
	v.SetDefault("project_id", defaults.ProjectID)
	v.SetDefault("default_branch", defaults.DefaultBranch)
	v.SetDefault("work_branch_prefix", defaults.WorkBranchPrefix)
	v.SetDefault("poll_interval_ms", defaults.PollIntervalMs)
	v.SetDefault("max_idle_minutes", defaults.MaxIdleMinutes)
	v.SetDefault("worker_ttl_minutes", defaults.WorkerTTLMinutes)
	v.SetDefault("scope_lock_ttl_sec", defaults.ScopeLockTTLSec)
	v.SetDefault("review_as_task", defaults.ReviewAsTask)
	v.SetDefault("checkpoint_as_task", defaults.CheckpointAsTask)
	v.SetDefault("worktree.enabled", defaults.Worktree.Enabled)
	v.SetDefault("worktree.auto_cleanup", defaults.Worktree.AutoCleanup)
	v.SetDefault("economic_mode.enabled", defaults.EconomicMode.Enabled)
	v.SetDefault("economic_mode.preset", defaults.EconomicMode.Preset)

	// Config file location
	configDir := filepath.Join(projectRoot, ".c4")
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(configDir)

	// Environment variable overrides (C4_PROJECT_ID, C4_DEFAULT_BRANCH, etc.)
	v.SetEnvPrefix("C4")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Read config file (ignore "not found" errors)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// Only return error for real read errors, not missing file
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("reading config: %w", err)
			}
		}
	}

	// Unmarshal into struct
	var cfg C4Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Resolve preset if economic mode is enabled
	if cfg.EconomicMode.Enabled {
		cfg.resolvePreset()
	} else {
		// When disabled, still set default routing
		cfg.EconomicMode.ModelRouting = presetConfigs["standard"]
	}

	return &Manager{v: v, config: cfg}, nil
}

// resolvePreset applies the preset model routing if no custom routing is set.
func (c *C4Config) resolvePreset() {
	preset, ok := presetConfigs[c.EconomicMode.Preset]
	if !ok {
		// Unknown preset, default to standard
		preset = presetConfigs["standard"]
	}

	// Only apply preset values where config doesn't have explicit overrides
	routing := &c.EconomicMode.ModelRouting
	if routing.Implementation == "" {
		routing.Implementation = preset.Implementation
	}
	if routing.Review == "" {
		routing.Review = preset.Review
	}
	if routing.Checkpoint == "" {
		routing.Checkpoint = preset.Checkpoint
	}
	if routing.Scout == "" {
		routing.Scout = preset.Scout
	}
	if routing.Debug == "" {
		routing.Debug = preset.Debug
	}
	if routing.Planning == "" {
		routing.Planning = preset.Planning
	}
}

// Get returns a configuration value by key using dot-notation.
// For example: Get("economic_mode.preset") or Get("project_id").
func (m *Manager) Get(key string) any {
	return m.v.Get(key)
}

// GetString returns a configuration value as a string.
func (m *Manager) GetString(key string) string {
	return m.v.GetString(key)
}

// GetConfig returns the parsed C4Config struct.
func (m *Manager) GetConfig() C4Config {
	return m.config
}

// GetBackend returns the store backend type ("sqlite" by default).
func (m *Manager) GetBackend() string {
	backend := m.v.GetString("store.backend")
	if backend == "" {
		return "sqlite"
	}
	return backend
}

// GetModelForTask returns the recommended model for a task type.
//
// Task type is determined from the task ID prefix:
//   - T-XXX -> implementation
//   - R-XXX -> review
//   - CP-XXX -> checkpoint
//   - RPR-XXX -> implementation (repair)
//
// If economic mode is disabled, returns empty string (use default).
func (m *Manager) GetModelForTask(taskID string) string {
	if !m.config.EconomicMode.Enabled {
		return ""
	}

	routing := m.config.EconomicMode.ModelRouting

	switch {
	case strings.HasPrefix(taskID, "R-"):
		return routing.Review
	case strings.HasPrefix(taskID, "CP-"):
		return routing.Checkpoint
	default:
		// T- prefix, RPR- prefix, or any other defaults to implementation
		return routing.Implementation
	}
}

// IsPreset checks if a preset name is valid.
func IsPreset(name string) bool {
	_, ok := presetConfigs[name]
	return ok
}

// PresetNames returns all available preset names.
func PresetNames() []string {
	names := make([]string, 0, len(presetConfigs))
	for name := range presetConfigs {
		names = append(names, name)
	}
	return names
}
