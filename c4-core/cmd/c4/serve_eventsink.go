//go:build c3_eventbus

package main

import (
	"fmt"
	"os"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/serve"
)

// registerEventSinkServeComponent registers the EventSink component when
// the c3_eventbus build tag is active. The EventSink receives events from
// C5 Hub (POST /v1/events/publish) and forwards them to the local EventBus.
// eb may be nil (if EventBus is disabled); events will be dropped silently.
func registerEventSinkServeComponent(mgr *serve.Manager, cfg config.C4Config, eb *serve.EventBusComponent) {
	if !cfg.Serve.EventSink.Enabled {
		return
	}

	port := cfg.EventSink.Port
	if port == 0 {
		port = 4141
	}

	var pub eventbus.Publisher = eventbus.NoopPublisher{}
	if eb != nil {
		if sockPath := eb.SocketPath(); sockPath != "" {
			if client, err := eventbus.NewClient(sockPath); err == nil {
				pub = client
			}
		}
	}

	mgr.Register(serve.NewEventSinkComponent(port, cfg.EventSink.Token, pub))
	fmt.Fprintf(os.Stderr, "cq serve: registered eventsink (port %d)\n", port)
}
