package llm

import (
	"os"

	"github.com/changmin/c4-core/internal/config"
)

// NewGatewayFromConfig creates a Gateway with providers auto-registered from config.
// Each provider is created based on the llm_gateway.providers map in config.
// API keys are read from environment variables specified in api_key_env.
func NewGatewayFromConfig(cfg config.C4Config) *Gateway {
	routing := RoutingTable{
		Default: cfg.LLMGateway.Default,
		Aliases: Aliases,
		Routes:  make(map[string]ModelRef),
	}

	// If the default provider has a default_model, register it as the "default" route.
	// This allows Resolve() fallback to pick up a model when no specific route matches.
	if defaultProv, ok := cfg.LLMGateway.Providers[cfg.LLMGateway.Default]; ok && defaultProv.DefaultModel != "" {
		routing.Routes["default"] = ModelRef{
			Provider: cfg.LLMGateway.Default,
			Model:    defaultProv.DefaultModel,
		}
	}

	gw := NewGateway(routing)

	for name, provCfg := range cfg.LLMGateway.Providers {
		if !provCfg.Enabled {
			continue
		}

		apiKey := ""
		if provCfg.APIKeyEnv != "" {
			apiKey = os.Getenv(provCfg.APIKeyEnv)
		}
		baseURL := provCfg.BaseURL

		switch name {
		case "anthropic":
			gw.Register(NewAnthropicProvider(apiKey, baseURL))
		case "openai":
			gw.Register(NewOpenAIProvider(apiKey, baseURL))
		case "gemini":
			gw.Register(NewGeminiProvider(apiKey, baseURL))
		case "ollama":
			gw.Register(NewOllamaProvider(baseURL))
		}
	}

	return gw
}
