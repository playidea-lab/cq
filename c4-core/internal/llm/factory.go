package llm

import "os"

// GatewayProviderConfig holds per-provider settings for the LLM gateway.
// This mirrors config.LLMProviderConfig to avoid importing the config package.
type GatewayProviderConfig struct {
	Enabled      bool
	APIKey       string
	APIKeyEnv    string
	BaseURL      string
	DefaultModel string
}

// GatewayConfig holds settings needed to construct a Gateway.
// This mirrors the relevant fields from config.C4Config.LLMGateway
// to avoid importing the config package directly.
type GatewayConfig struct {
	Default        string
	CacheByDefault bool
	Providers      map[string]GatewayProviderConfig
}

// NewGatewayFromConfig creates a Gateway with providers auto-registered from config.
// Each provider is created based on the providers map in GatewayConfig.
// API keys are read from environment variables specified in APIKeyEnv.
func NewGatewayFromConfig(cfg GatewayConfig) *Gateway {
	routing := RoutingTable{
		Default: cfg.Default,
		Aliases: Aliases,
		Routes:  make(map[string]ModelRef),
	}

	// If the default provider has a default_model, register it as the "default" route.
	// This allows Resolve() fallback to pick up a model when no specific route matches.
	if defaultProv, ok := cfg.Providers[cfg.Default]; ok && defaultProv.DefaultModel != "" {
		routing.Routes["default"] = ModelRef{
			Provider: cfg.Default,
			Model:    defaultProv.DefaultModel,
		}
	}

	gw := NewGateway(routing)
	gw.cacheByDefault = cfg.CacheByDefault

	for name, provCfg := range cfg.Providers {
		if !provCfg.Enabled {
			continue
		}

		apiKey := provCfg.APIKey
		if apiKey == "" && provCfg.APIKeyEnv != "" {
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
