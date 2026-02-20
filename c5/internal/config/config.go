package config

// Config is the top-level configuration for the C5 server.
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	EventBus EventBusConfig `yaml:"eventbus"`
	Storage  StorageConfig  `yaml:"storage"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Host string `yaml:"host"` // default "0.0.0.0"
	Port int    `yaml:"port"` // default 8585
}

// EventBusConfig holds C3 EventBus connection settings.
type EventBusConfig struct {
	URL   string `yaml:"url"`   // default "" (disabled)
	Token string `yaml:"token"` // default ""
}

// StorageConfig holds local storage settings.
type StorageConfig struct {
	Path        string `yaml:"path"`         // default "~/.local/share/c5"
	SupabaseURL string `yaml:"supabase_url"` // "" = disabled
	SupabaseKey string `yaml:"supabase_key"`
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
			Path: "~/.local/share/c5",
		},
	}
}

// IsEventBusEnabled reports whether the EventBus integration is active.
func (c *Config) IsEventBusEnabled() bool {
	return c.EventBus.URL != ""
}

// IsSupabaseEnabled reports whether Supabase storage integration is active.
func (c *Config) IsSupabaseEnabled() bool {
	return c.Storage.SupabaseURL != "" && c.Storage.SupabaseKey != ""
}
