//go:build c3_eventbus

package main

import (
	"fmt"
	"os"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/serve"
)

// registerWebhookGatewayServeComponent registers the WebhookGateway component when
// the c3_eventbus build tag is active. WebhookGateway receives inbound webhooks from
// external services (e.g. Dooray slash commands) and publishes events to the local EventBus.
// eb may be nil (if EventBus is disabled); events will be dropped silently.
func registerWebhookGatewayServeComponent(mgr *serve.Manager, cfg config.C4Config, eb *serve.EventBusComponent) {
	if !cfg.Serve.WebhookGateway.Enabled {
		return
	}

	port := cfg.WebhookGateway.Port
	if port == 0 {
		port = 4142
	}
	host := cfg.WebhookGateway.Host
	if host == "" {
		host = "127.0.0.1"
	}

	// cmd_token: config value, fall back to env var.
	doorayCfg := cfg.Dooray
	if doorayCfg.CmdToken == "" {
		doorayCfg.CmdToken = os.Getenv("DOORAY_CMD_TOKEN")
	}

	var pub eventbus.Publisher
	if eb != nil {
		pub = eb.Publisher()
	}

	mgr.Register(serve.NewWebhookGatewayComponent(host, port, doorayCfg, pub))
	fmt.Fprintf(os.Stderr, "cq serve: registered webhook-gateway (port %d)\n", port)
}
