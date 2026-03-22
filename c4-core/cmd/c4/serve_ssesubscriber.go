//go:build hub && c3_eventbus

package main

import (
	"fmt"
	"os"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/serve"
)

// registerSSESubscriberServeComponent registers the SSESubscriberComponent when
// both hub and c3_eventbus build tags are active.
// It connects to the C5 Hub SSE endpoint and forwards events to the local EventBus.
// If eb is nil (EventBus disabled), events are silently dropped via NoopPublisher.
// If wakeCh is non-nil, the component will signal it on hub.job.completed/failed events.
func registerSSESubscriberServeComponent(mgr *serve.Manager, cfg config.C4Config, eb *serve.EventBusComponent, wakeCh chan struct{}) {
	if !cfg.Serve.SSESubscriber.Enabled || !cfg.Hub.Enabled {
		return
	}

	apiKey := cfg.Hub.APIKey
	if apiKey == "" && cfg.Hub.APIKeyEnv != "" {
		apiKey = os.Getenv(cfg.Hub.APIKeyEnv)
	}

	var pub eventbus.Publisher
	if eb != nil {
		pub = eb.Publisher()
	} else {
		pub = eventbus.NoopPublisher{}
	}

	comp := serve.NewSSESubscriberComponent(serve.SSESubscriberConfig{
		URL:       cfg.Hub.URL,
		APIKey:    apiKey,
		ProjectID: cfg.ProjectID,
	}, pub)

	if wakeCh != nil {
		comp.SetWakeChannel(wakeCh)
	}

	mgr.Register(comp)
	fmt.Fprintf(os.Stderr, "cq serve: registered ssesubscriber (hub url: %s)\n", cfg.Hub.URL)
}
