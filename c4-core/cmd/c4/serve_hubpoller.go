//go:build c5_hub

package main

import (
	"fmt"
	"os"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/serve"
)

// registerHubPollerServeComponent registers the HubPoller component when
// the c5_hub build tag is active. The HubPoller periodically polls C5 Hub
// for job status changes and publishes hub.job.completed/failed events.
// eb is reserved for future publisher wiring; events are silently dropped for now.
// prebuiltClient, if non-nil, is the already-configured hub.Client from initHub
// (secrets store + cloud JWT resolved); it is preferred over creating a new client
// from cfg.Hub to avoid re-resolving credentials.
// hubClientAny is typed as any so serve.go (no build tag) can pass
// srv.initCtx.hubClient without importing the hub package directly.
func registerHubPollerServeComponent(mgr *serve.Manager, cfg config.C4Config, eb *serve.EventBusComponent, hubClientAny any) {
	if !cfg.Serve.HubPoller.Enabled || !cfg.Hub.Enabled {
		return
	}

	var pub eventbus.Publisher
	if eb != nil {
		pub = eb.Publisher()
	} else {
		pub = eventbus.NoopPublisher{}
	}

	var comp *serve.HubPollerComponent
	if hc, ok := hubClientAny.(*hub.Client); ok && hc != nil {
		// Use the pre-built client (secrets + JWT already resolved by initHub).
		comp = serve.NewHubPollerComponentWithClient(hc, pub, cfg.ProjectID)
	} else {
		comp = serve.NewHubPollerComponent(cfg.Hub, pub, cfg.ProjectID)
	}
	mgr.Register(comp)
	fmt.Fprintf(os.Stderr, "cq serve: registered hubpoller\n")
}
