package llm

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ModelRef identifies a specific provider + model combination.
type ModelRef struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// RoutingTable defines how task types map to models.
type RoutingTable struct {
	Default string              `json:"default"`
	Routes  map[string]ModelRef `json:"routes"`
	Aliases map[string]string   `json:"aliases"`
}

// Gateway is the central LLM orchestration hub.
// It manages provider registration, request routing, and cost tracking.
type Gateway struct {
	mu             sync.RWMutex
	providers      map[string]Provider
	routing        RoutingTable
	tracker        *CostTracker
	cacheByDefault bool
}

// CacheByDefault returns whether system prompt caching is enabled by default.
func (g *Gateway) CacheByDefault() bool { return g.cacheByDefault }

// NewGateway creates a Gateway with the given routing table.
func NewGateway(routing RoutingTable) *Gateway {
	if routing.Routes == nil {
		routing.Routes = make(map[string]ModelRef)
	}
	if routing.Aliases == nil {
		routing.Aliases = make(map[string]string)
	}
	return &Gateway{
		providers: make(map[string]Provider),
		routing:   routing,
		tracker:   NewCostTracker(),
	}
}

// Register adds a provider to the gateway.
func (g *Gateway) Register(p Provider) {
	g.mu.Lock()
	g.providers[p.Name()] = p
	g.mu.Unlock()
}

// Resolve determines which provider and model to use based on routing rules.
//
// Priority:
//  1. modelHint as "provider/model" -> direct use
//  2. modelHint as alias -> resolve from aliases
//  3. taskType in Routes -> use that route
//  4. Default provider with modelHint as model name
//  5. Routes["default"] fallback
func (g *Gateway) Resolve(taskType, modelHint string) ModelRef {
	// 1. Direct provider/model format
	if strings.Contains(modelHint, "/") {
		parts := strings.SplitN(modelHint, "/", 2)
		return ModelRef{Provider: parts[0], Model: parts[1]}
	}

	// 2. Check gateway-level aliases
	if modelHint != "" {
		if full, ok := g.routing.Aliases[modelHint]; ok {
			modelHint = full
		}
	}

	// 3. Task type route
	if taskType != "" {
		if ref, ok := g.routing.Routes[taskType]; ok {
			if modelHint != "" {
				ref.Model = modelHint
			}
			return ref
		}
	}

	// 4. Default provider with model hint
	if modelHint != "" {
		return ModelRef{Provider: g.routing.Default, Model: modelHint}
	}

	// 5. Default route
	if ref, ok := g.routing.Routes["default"]; ok {
		return ref
	}

	return ModelRef{Provider: g.routing.Default}
}

// Chat routes a request to the appropriate provider and records cost.
func (g *Gateway) Chat(ctx context.Context, taskType string, req *ChatRequest) (*ChatResponse, error) {
	ref := g.Resolve(taskType, req.Model)

	g.mu.RLock()
	provider, ok := g.providers[ref.Provider]
	g.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("provider %q not registered", ref.Provider)
	}
	if !provider.IsAvailable() {
		return nil, fmt.Errorf("provider %q is not available", ref.Provider)
	}

	// Set the resolved model on the request
	if ref.Model != "" {
		req.Model = ref.Model
	}

	start := time.Now()
	resp, err := provider.Chat(ctx, req)
	latency := time.Since(start)

	if err != nil {
		return nil, fmt.Errorf("provider %q chat error: %w", ref.Provider, err)
	}

	g.tracker.Record(ref.Provider, resp.Model, resp.Usage, latency)
	return resp, nil
}

// EmbedProvider is an optional interface that providers can implement for embedding support.
type EmbedProvider interface {
	Embed(ctx context.Context, texts []string, model string) (*EmbedResponse, error)
}

// Embed routes an embedding request to the appropriate provider.
// Uses the "embedding" route in the routing table to determine provider/model.
func (g *Gateway) Embed(ctx context.Context, taskType string, texts []string) ([][]float32, error) {
	ref := g.Resolve(taskType, "")

	g.mu.RLock()
	provider, ok := g.providers[ref.Provider]
	g.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("provider %q not registered", ref.Provider)
	}
	if !provider.IsAvailable() {
		return nil, fmt.Errorf("provider %q is not available", ref.Provider)
	}

	embedder, ok := provider.(EmbedProvider)
	if !ok {
		return nil, fmt.Errorf("provider %q does not support embeddings", ref.Provider)
	}

	start := time.Now()
	resp, err := embedder.Embed(ctx, texts, ref.Model)
	latency := time.Since(start)

	if err != nil {
		return nil, fmt.Errorf("provider %q embed error: %w", ref.Provider, err)
	}

	g.tracker.Record(ref.Provider, resp.Model, resp.Usage, latency)
	return resp.Embeddings, nil
}

// ListProviders returns the status of all registered providers.
func (g *Gateway) ListProviders() []ProviderStatus {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make([]ProviderStatus, 0, len(g.providers))
	for _, p := range g.providers {
		result = append(result, ProviderStatus{
			Name:      p.Name(),
			Available: p.IsAvailable(),
			Models:    p.Models(),
		})
	}
	return result
}

// CostReport returns the aggregate cost report.
func (g *Gateway) CostReport() CostReport {
	return g.tracker.Report()
}

// ProviderCount returns the number of registered providers (for testing).
func (g *Gateway) ProviderCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.providers)
}
