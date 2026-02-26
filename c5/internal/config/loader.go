package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

// DefaultConfigPath returns the XDG-based default config file path:
// ~/.config/c5/c5.yaml
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "~/.config/c5/c5.yaml"
	}
	return filepath.Join(home, ".config", "c5", "c5.yaml")
}

// Load reads the config from configPath and returns a *Config.
// If configPath is empty, DefaultConfigPath() is used.
// If the file does not exist, Default() is returned without error.
// Fields missing from the file are filled with default values.
func Load(configPath string) (*Config, error) {
	if configPath == "" {
		configPath = DefaultConfigPath()
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg := Default()
			applyEnvOverrides(&cfg)
			return &cfg, nil
		}
		return nil, fmt.Errorf("config: read %q: %w", configPath, err)
	}

	// Start from defaults so missing fields retain their default values.
	cfg := Default()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %q: %w", configPath, err)
	}
	applyEnvOverrides(&cfg)

	return &cfg, nil
}

// applyEnvOverrides overrides config values from environment variables.
// C5_SUPABASE_URL, C5_SUPABASE_KEY, and C5_PORT are supported.
func applyEnvOverrides(cfg *Config) {
	if v, ok := os.LookupEnv("C5_SUPABASE_URL"); ok {
		cfg.Storage.SupabaseURL = v
	}
	if v, ok := os.LookupEnv("C5_SUPABASE_KEY"); ok {
		cfg.Storage.SupabaseKey = v
	}
	if v, ok := os.LookupEnv("C5_PORT"); ok {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = port
		}
	}
}

// ExampleConfigYAML returns a commented example configuration YAML string
// that can be written to ~/.config/c5/c5.yaml.
func ExampleConfigYAML() string {
	return `# C5 Hub Server configuration
# Default path: ~/.config/c5/c5.yaml

server:
  # Host to bind the HTTP server on.
  host: "0.0.0.0"
  # Port to listen on.
  port: 8585
  # If true, GPU workers only accept GPU jobs (no CPU fallback).
  # gpu_worker_gpu_only: false

eventbus:
  # C3 EventBus base URL. Leave empty to disable integration.
  url: ""
  # Bearer token for EventBus authentication.
  token: ""

storage:
  # Local storage directory for C5 data.
  path: "~/.local/share/c5"
  # Supabase project URL. Leave empty to use local storage only.
  supabase_url: ""
  # Supabase service-role key. Required when supabase_url is set.
  supabase_key: ""
  # Maximum artifact upload size in bytes (local backend only). Default: 10GB.
  # max_artifact_bytes: 10737418240
`
}
