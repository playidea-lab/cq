package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/secrets"
)

// capturingSlogHandler captures slog records for test assertions.
type capturingSlogHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *capturingSlogHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *capturingSlogHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}

func (h *capturingSlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler { return h }
func (h *capturingSlogHandler) WithGroup(name string) slog.Handler       { return h }

func (h *capturingSlogHandler) hasWarn(substr string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.records {
		if r.Level == slog.LevelWarn && strings.Contains(r.Message, substr) {
			return true
		}
	}
	return false
}

// makeLLMConfig writes a .c4/config.yaml with the given YAML content and returns a Manager.
func makeLLMConfig(t *testing.T, yamlContent string) *config.Manager {
	t.Helper()
	dir := t.TempDir()
	c4Dir := filepath.Join(dir, ".c4")
	if err := os.MkdirAll(c4Dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(c4Dir, "config.yaml"), []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mgr, err := config.New(dir)
	if err != nil {
		t.Fatalf("config.New: %v", err)
	}
	return mgr
}

// TestLLMKeyFromSecrets verifies that an API key stored in the secret store
// is picked up by toLLMGatewayConfig.
func TestLLMKeyFromSecrets(t *testing.T) {
	mgr := makeLLMConfig(t, `
llm_gateway:
  enabled: true
  providers:
    anthropic:
      enabled: true
`)
	// Create a temporary secret store and inject the key.
	dir := t.TempDir()
	ss, err := secrets.NewWithPaths(
		filepath.Join(dir, "secrets.db"),
		filepath.Join(dir, "master.key"),
	)
	if err != nil {
		t.Fatalf("secrets.NewWithPaths: %v", err)
	}
	defer ss.Close()
	if err := ss.Set("anthropic.api_key", "sk-test-from-secrets"); err != nil {
		t.Fatalf("ss.Set: %v", err)
	}

	cfg := toLLMGatewayConfig(mgr, ss, nil)
	p, ok := cfg.Providers["anthropic"]
	if !ok {
		t.Fatal("anthropic provider not found")
	}
	if p.APIKey != "sk-test-from-secrets" {
		t.Errorf("APIKey = %q, want %q", p.APIKey, "sk-test-from-secrets")
	}
	if !p.Enabled {
		t.Error("expected provider to be enabled")
	}
}

// TestLLMKeyFromEnvFallback verifies that when the secret store has no key,
// the default env var (ANTHROPIC_API_KEY) is used.
func TestLLMKeyFromEnvFallback(t *testing.T) {
	mgr := makeLLMConfig(t, `
llm_gateway:
  enabled: true
  providers:
    anthropic:
      enabled: true
`)
	t.Setenv("ANTHROPIC_API_KEY", "sk-env-fallback")

	cfg := toLLMGatewayConfig(mgr, nil, nil)
	p, ok := cfg.Providers["anthropic"]
	if !ok {
		t.Fatal("anthropic provider not found")
	}
	if p.APIKey != "sk-env-fallback" {
		t.Errorf("APIKey = %q, want %q", p.APIKey, "sk-env-fallback")
	}
	if !p.Enabled {
		t.Error("expected provider to be enabled")
	}
}

// TestLLMDeprecationWarn verifies that when config.yaml contains a deprecated
// api_key field under llm_gateway.providers, slog.Warn is emitted.
func TestLLMDeprecationWarn(t *testing.T) {
	handler := &capturingSlogHandler{}
	old := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(old) })

	mgr := makeLLMConfig(t, `
llm_gateway:
  enabled: true
  providers:
    anthropic:
      enabled: true
      api_key: "sk-old-insecure-key"
`)

	toLLMGatewayConfig(mgr, nil, nil)

	if !handler.hasWarn("llm_gateway api_key in config deprecated") {
		t.Error("expected deprecation slog.Warn for api_key in config, got none")
	}
}

// TestLLMNoKeyDisablesProvider verifies that when no API key is available
// from either the secret store or env vars, the provider is disabled and
// a slog.Warn is emitted.
func TestLLMNoKeyDisablesProvider(t *testing.T) {
	handler := &capturingSlogHandler{}
	old := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(old) })

	// Ensure env vars are not set.
	t.Setenv("ANTHROPIC_API_KEY", "")

	mgr := makeLLMConfig(t, `
llm_gateway:
  enabled: true
  providers:
    anthropic:
      enabled: true
`)

	cfg := toLLMGatewayConfig(mgr, nil, nil)
	p, ok := cfg.Providers["anthropic"]
	if !ok {
		t.Fatal("anthropic provider not found")
	}
	if p.Enabled {
		t.Error("expected provider to be disabled when no API key available")
	}
	if !handler.hasWarn("no API key for provider") {
		t.Error("expected slog.Warn about missing API key, got none")
	}
}

// TestLLMOllamaNoKeyStillEnabled verifies that ollama is not disabled when
// no API key is configured, since ollama does not require an API key.
func TestLLMOllamaNoKeyStillEnabled(t *testing.T) {
	handler := &capturingSlogHandler{}
	old := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(old) })

	mgr := makeLLMConfig(t, `
llm_gateway:
  enabled: true
  providers:
    ollama:
      enabled: true
      base_url: "http://localhost:11434"
`)

	cfg := toLLMGatewayConfig(mgr, nil, nil)
	p, ok := cfg.Providers["ollama"]
	if !ok {
		t.Fatal("ollama provider not found")
	}
	if !p.Enabled {
		t.Error("expected ollama provider to be enabled even without an API key")
	}
	if handler.hasWarn("no API key for provider") {
		t.Error("ollama should not emit 'no API key' warning")
	}
}
