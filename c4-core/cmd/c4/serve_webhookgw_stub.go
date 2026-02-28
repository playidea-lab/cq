//go:build !c3_eventbus

package main

import (
	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/serve"
)

func registerWebhookGatewayServeComponent(mgr *serve.Manager, cfg config.C4Config, eb *serve.EventBusComponent) {
	// WebhookGateway requires c3_eventbus build tag.
	_, _, _ = mgr, cfg, eb
}
