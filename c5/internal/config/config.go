package config

import (
	"os"
	"path/filepath"
)

// Config is the top-level configuration for the C5 server.
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	EventBus EventBusConfig `yaml:"eventbus"`
	Storage  StorageConfig  `yaml:"storage"`
	LLM      LLMConfig      `yaml:"llm"`
}

// LLMConfig holds LLM settings for server-side processing.
type LLMConfig struct {
	Provider  string `yaml:"provider"`   // "openai" (default) | "anthropic"
	BaseURL   string `yaml:"base_url"`   // e.g. "https://generativelanguage.googleapis.com/v1beta/openai"
	APIKey    string `yaml:"api_key"`    // API key for the LLM provider
	Model     string `yaml:"model"`      // default "gemini-3-flash-preview"
	MaxTokens int    `yaml:"max_tokens"` // default 4096
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Host             string `yaml:"host"`                // default "0.0.0.0"
	Port             int    `yaml:"port"`                // default 8585
	PublicURL        string `yaml:"public_url"`          // external URL for OAuth redirects (e.g. "https://hub.example.com")
	GPUWorkerGPUOnly bool   `yaml:"gpu_worker_gpu_only"` // default false; if true, GPU workers only accept GPU jobs
}

// EventBusConfig holds C3 EventBus connection settings.
type EventBusConfig struct {
	URL   string `yaml:"url"`   // default "" (disabled)
	Token string `yaml:"token"` // default ""
}

// StorageConfig holds local storage settings.
type StorageConfig struct {
	Path             string `yaml:"path"`               // default "~/.local/share/c5"
	SupabaseURL      string `yaml:"supabase_url"`       // "" = disabled
	SupabaseKey      string `yaml:"supabase_key"`
	MaxArtifactBytes int64  `yaml:"max_artifact_bytes"` // default 10GB (local backend only)
}

// Default returns a Config populated with default values.
func Default() Config {
	return Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8585,
		},
		EventBus: EventBusConfig{
			URL:   "",
			Token: "",
		},
		Storage: StorageConfig{
			Path:             defaultStoragePath(),
			MaxArtifactBytes: 10 << 30, // 10GB
		},
		LLM: LLMConfig{
			Provider:  "openai",
			Model:     "gemini-3-flash-preview",
			MaxTokens: 4096,
		},
	}
}

func defaultStoragePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".local/share/c5"
	}
	return filepath.Join(home, ".local", "share", "c5")
}

// IsEventBusEnabled reports whether the EventBus integration is active.
func (c *Config) IsEventBusEnabled() bool {
	return c.EventBus.URL != ""
}

// IsSupabaseEnabled reports whether Supabase storage integration is active.
func (c *Config) IsSupabaseEnabled() bool {
	return c.Storage.SupabaseURL != "" && c.Storage.SupabaseKey != ""
}

// IsLLMEnabled reports whether server-side LLM processing is configured.
func (c *Config) IsLLMEnabled() bool {
	if c.LLM.Provider == "anthropic" {
		return c.LLM.APIKey != "" || os.Getenv("C5_ANTHROPIC_API_KEY") != ""
	}
	return c.LLM.BaseURL != "" && c.LLM.APIKey != ""
}
