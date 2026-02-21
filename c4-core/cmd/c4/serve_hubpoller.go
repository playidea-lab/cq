//go:build c5_hub

package main

import (
	"fmt"
	"os"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/serve"
)

// registerHubPollerServeComponent registers the HubPoller component when
// the c5_hub build tag is active. The HubPoller periodically polls C5 Hub
// for job status changes and publishes hub.job.completed/failed events.
// eb may be nil (if EventBus is disabled); events will be dropped silently.
func registerHubPollerServeComponent(mgr *serve.Manager, cfg config.C4Config, eb *serve.EventBusComponent) {
	if !cfg.Serve.HubPoller.Enabled || !cfg.Hub.Enabled {
		return
	}

	var pub eventbus.Publisher = eventbus.NoopPublisher{}
	if eb != nil {
		if sockPath := eb.SocketPath(); sockPath != "" {
			if client, err := eventbus.NewClient(sockPath); err == nil {
				pub = client
			}
		}
	}

	mgr.Register(serve.NewHubPollerComponent(cfg.Hub, pub, cfg.ProjectID))
	fmt.Fprintf(os.Stderr, "cq serve: registered hubpoller\n")
}
