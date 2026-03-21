package observe

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

// LoadBenchConfig parses a TOML file at the given path into a BenchConfig.
func LoadBenchConfig(path string) (*BenchConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("bench config: read %q: %w", path, err)
	}
	return ParseBenchConfig(data)
}

// ParseBenchConfig parses raw TOML bytes into a BenchConfig.
func ParseBenchConfig(data []byte) (*BenchConfig, error) {
	var cfg BenchConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("bench config: parse: %w", err)
	}
	return &cfg, nil
}
