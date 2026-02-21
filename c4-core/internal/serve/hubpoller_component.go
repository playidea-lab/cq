//go:build c5_hub

package serve

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/mcp/handlers"
)

const (
	hubPollerDegradedThreshold = 5
	hubPollerInterval          = 30 * time.Second
)

// HubPollerComponent wraps handlers.HubPoller as a managed Component.
type HubPollerComponent struct {
	cfg       config.HubConfig
	pub       eventbus.Publisher
	projectID string

	mu           sync.Mutex
	cancel       context.CancelFunc
	client       *hub.Client
	retryCount   int
	lastCheckErr error
}

// NewHubPollerComponent creates a new HubPoller component.
// cfg provides Hub connection settings; pub is used for event publishing;
// projectID is attached to published events.
func NewHubPollerComponent(cfg config.HubConfig, pub eventbus.Publisher, projectID string) *HubPollerComponent {
	return &HubPollerComponent{
		cfg:       cfg,
		pub:       pub,
		projectID: projectID,
	}
}

func (h *HubPollerComponent) Name() string { return "hubpoller" }

// Start creates the Hub client, verifies connectivity, and starts the poller goroutine.
func (h *HubPollerComponent) Start(ctx context.Context) error {
	hc := hub.NewClient(hub.HubConfig{
		Enabled:   h.cfg.Enabled,
		URL:       h.cfg.URL,
		APIPrefix: h.cfg.APIPrefix,
		APIKey:    h.cfg.APIKey,
		APIKeyEnv: h.cfg.APIKeyEnv,
		TeamID:    h.cfg.TeamID,
	})
	if !hc.IsAvailable() {
		return fmt.Errorf("hub URL not configured")
	}

	h.mu.Lock()
	h.client = hc
	h.mu.Unlock()

	pollerCtx, pollerCancel := context.WithCancel(ctx)

	h.mu.Lock()
	h.cancel = pollerCancel
	h.mu.Unlock()

	poller := handlers.NewHubPoller(hc, h.pub, hubPollerInterval)
	poller.SetProjectID(h.projectID)
	poller.Start(pollerCtx)

	// Start background health checker
	go h.healthLoop(pollerCtx)

	return nil
}

// Stop cancels the poller context, stopping the polling goroutine.
func (h *HubPollerComponent) Stop(_ context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
	return nil
}

// Health returns the current health status based on Hub reachability.
// After hubPollerDegradedThreshold consecutive failures, status becomes "degraded".
func (h *HubPollerComponent) Health() ComponentHealth {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.client == nil {
		return ComponentHealth{Status: "error", Detail: "not started"}
	}

	if h.retryCount >= hubPollerDegradedThreshold {
		detail := fmt.Sprintf("hub unreachable after %d checks", h.retryCount)
		if h.lastCheckErr != nil {
			detail = fmt.Sprintf("hub unreachable after %d checks: %v", h.retryCount, h.lastCheckErr)
		}
		return ComponentHealth{Status: "degraded", Detail: detail}
	}

	return ComponentHealth{Status: "ok"}
}

// healthLoop periodically checks Hub connectivity and updates retry count.
func (h *HubPollerComponent) healthLoop(ctx context.Context) {
	ticker := time.NewTicker(hubPollerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.checkHealth()
		}
	}
}

func (h *HubPollerComponent) checkHealth() {
	h.mu.Lock()
	client := h.client
	h.mu.Unlock()

	if client == nil {
		return
	}

	ok := client.HealthCheck()

	h.mu.Lock()
	defer h.mu.Unlock()

	if ok {
		h.retryCount = 0
		h.lastCheckErr = nil
	} else {
		h.retryCount++
		h.lastCheckErr = fmt.Errorf("health check failed")
	}
}
