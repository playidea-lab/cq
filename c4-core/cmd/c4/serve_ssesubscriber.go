//go:build c5_hub && c3_eventbus

package main

import (
	"fmt"
	"os"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/serve"
)

// registerSSESubscriberServeComponent registers the SSESubscriberComponent when
// both c5_hub and c3_eventbus build tags are active.
// It connects to the C5 Hub SSE endpoint and forwards events to the local EventBus.
// eb is reserved for future publisher wiring; events are silently dropped for now.
func registerSSESubscriberServeComponent(mgr *serve.Manager, cfg config.C4Config, eb *serve.EventBusComponent) {
	if !cfg.Serve.SSESubscriber.Enabled || !cfg.Hub.Enabled {
		return
	}

	_ = eb // reserved for future publisher wiring

	apiKey := cfg.Hub.APIKey
	if apiKey == "" && cfg.Hub.APIKeyEnv != "" {
		apiKey = os.Getenv(cfg.Hub.APIKeyEnv)
	}

	comp := serve.NewSSESubscriberComponent(serve.SSESubscriberConfig{
		URL:       cfg.Hub.URL,
		APIKey:    apiKey,
		ProjectID: cfg.ProjectID,
	}, eventbus.NoopPublisher{})

	mgr.Register(comp)
	fmt.Fprintf(os.Stderr, "cq serve: registered ssesubscriber (hub url: %s)\n", cfg.Hub.URL)
}
