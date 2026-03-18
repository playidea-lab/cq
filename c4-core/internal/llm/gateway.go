package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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

// CacheAlertPublisher is a minimal interface for publishing cache miss alert events.
// Satisfied by eventbus.Publisher, eventbus.NoopPublisher, and test stubs.
type CacheAlertPublisher interface {
	PublishAsync(evType, source string, data json.RawMessage, projectID string)
}

// cacheAlertMonitor tracks recent cache hit/miss attempts and fires alerts.
// It uses a state-transition model: fires once when crossing below threshold,
// resets when crossing back above.
type cacheAlertMonitor struct {
	threshold  float64
	publisher  CacheAlertPublisher
	projectID  string
	alertFired bool // true = currently in "below threshold" state
}

// cacheAlertPayload is the event payload for llm.cache_miss_alert.
type cacheAlertPayload struct {
	GlobalCacheHitRate float64 `json:"global_cache_hit_rate"`
	Threshold          float64 `json:"threshold"`
	WindowCalls        int     `json:"window_calls"`
	Provider           string  `json:"provider"`
}

// Gateway is the central LLM orchestration hub.
// It manages provider registration, request routing, and cost tracking.
type Gateway struct {
	mu             sync.RWMutex
	providers      map[string]Provider
	routing        RoutingTable
	tracker        *CostTracker
	cacheByDefault bool
	alertMu        sync.Mutex
	alert          *cacheAlertMonitor
}

// CacheByDefault returns whether system prompt caching is enabled by default.
func (g *Gateway) CacheByDefault() bool { return g.cacheByDefault }

// SetCacheAlert configures cache hit rate alerting.
// When GlobalCacheHitRate drops below threshold for the first time, a
// "llm.cache_miss_alert" event is published via pub and a slog.Warn is emitted.
// threshold=0.0 disables alerting.
func (g *Gateway) SetCacheAlert(threshold float64, pub CacheAlertPublisher, projectID string) {
	g.alertMu.Lock()
	defer g.alertMu.Unlock()
	if threshold == 0.0 || pub == nil {
		g.alert = nil
		return
	}
	g.alert = &cacheAlertMonitor{
		threshold: threshold,
		publisher: pub,
		projectID: projectID,
	}
}

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

// Tracker returns the Gateway's CostTracker, allowing callers to wire DB persistence.
func (g *Gateway) Tracker() *CostTracker {
	return g.tracker
}

// Register adds a provider to the gateway.
func (g *Gateway) Register(p Provider) {
	g.mu.Lock()
	g.providers[p.Name()] = p
	g.mu.Unlock()
}

// SetRoute sets a routing entry for a task type.
func (g *Gateway) SetRoute(taskType string, ref ModelRef) {
	g.mu.Lock()
	g.routing.Routes[taskType] = ref
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

	// Apply cacheByDefault if not already explicitly set by the caller
	if g.cacheByDefault && !req.CacheSystemPrompt {
		req.CacheSystemPrompt = true
	}

	start := time.Now()
	resp, err := provider.Chat(ctx, req)
	latency := time.Since(start)

	if err != nil {
		return nil, fmt.Errorf("provider %q chat error: %w", ref.Provider, err)
	}

	g.tracker.Record(ref.Provider, resp.Model, resp.Usage, latency)
	g.checkCacheAlert(ref.Provider)
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

// checkCacheAlert inspects the current GlobalCacheHitRate and fires an alert
// if it has transitioned below the configured threshold.
func (g *Gateway) checkCacheAlert(provider string) {
	g.alertMu.Lock()
	mon := g.alert
	g.alertMu.Unlock()

	if mon == nil {
		return
	}

	report := g.tracker.Report()
	// Only evaluate when there are cache attempts.
	totalAttempts := 0
	for _, pc := range report.ByProvider {
		totalAttempts += pc.CacheReadTok + pc.CacheWriteTok
	}
	if totalAttempts == 0 {
		return
	}

	rate := report.GlobalCacheHitRate

	g.alertMu.Lock()
	defer g.alertMu.Unlock()

	if g.alert == nil {
		return
	}

	belowThreshold := rate < mon.threshold

	if belowThreshold && !mon.alertFired {
		// State transition: above → below threshold. Fire alert once.
		mon.alertFired = true

		slog.Warn("llm: cache hit rate below threshold",
			"rate", rate,
			"threshold", mon.threshold,
			"provider", provider,
		)

		payload := cacheAlertPayload{
			GlobalCacheHitRate: rate,
			Threshold:          mon.threshold,
			WindowCalls:        report.TotalReqs,
			Provider:           provider,
		}
		data, err := json.Marshal(payload)
		if err == nil {
			mon.publisher.PublishAsync("llm.cache_miss_alert", "c4.llm", data, mon.projectID)
		}
	} else if !belowThreshold {
		// Reset state: allow future alert if it drops again.
		mon.alertFired = false
	}
}
