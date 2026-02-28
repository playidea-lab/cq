package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
	if v, ok := os.LookupEnv("C5_PUBLIC_URL"); ok {
		cfg.Server.PublicURL = v
	}
	if v, ok := os.LookupEnv("C5_LLM_BASE_URL"); ok {
		cfg.LLM.BaseURL = v
	}
	if v, ok := os.LookupEnv("C5_LLM_API_KEY"); ok {
		cfg.LLM.APIKey = v
	}
	if v, ok := os.LookupEnv("C5_LLM_MODEL"); ok {
		cfg.LLM.Model = v
	}
	if v, ok := os.LookupEnv("C5_DOORAY_WEBHOOK_URL"); ok {
		cfg.Dooray.WebhookURL = v
	}
	if v, ok := os.LookupEnv("C5_DOORAY_CMD_TOKEN"); ok {
		cfg.Dooray.CmdToken = v
	}
	if v, ok := os.LookupEnv("C5_DOORAY_CHANNELS"); ok && v != "" {
		cfg.Dooray.Channels = parseDoorayChannels(v)
	}
}

// parseDoorayChannels parses "channelId1=projectId1,channelId2=projectId2" format.
func parseDoorayChannels(s string) map[string]DoorayChannelCfg {
	result := make(map[string]DoorayChannelCfg)
	for _, part := range strings.Split(s, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) == 2 && kv[0] != "" && kv[1] != "" {
			result[strings.TrimSpace(kv[0])] = DoorayChannelCfg{
				ProjectID: strings.TrimSpace(kv[1]),
			}
		}
	}
	return result
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

llm:
  # OpenAI-compatible LLM base URL (required for server-side Dooray processing).
  # Examples:
  #   Gemini:  https://generativelanguage.googleapis.com/v1beta/openai
  #   Ollama:  http://localhost:11434/v1
  #   OpenAI:  https://api.openai.com/v1
  # base_url: ""
  # LLM API key. Also settable via C5_LLM_API_KEY env var.
  # api_key: ""
  # Model ID. Default: gemini-3-flash-preview
  # model: "gemini-3-flash-preview"
  # Max tokens in the LLM response. Default: 4096.
  # max_tokens: 4096

dooray:
  # Default Incoming Webhook URL for LLM responses.
  # Also settable via C5_DOORAY_WEBHOOK_URL env var.
  # webhook_url: ""
  # Slash command token for verifying Dooray requests.
  # Also settable via C5_DOORAY_CMD_TOKEN env var.
  # cmd_token: ""
  # Per-channel routing: maps channelID to projectID (and optional per-channel webhook).
  # Also settable via C5_DOORAY_CHANNELS env var (format: "ch1=proj1,ch2=proj2").
  # channels:
  #   "4225637197401340462":
  #     project_id: "c4"
  #     webhook_url: ""  # optional; falls back to dooray.webhook_url
`
}
