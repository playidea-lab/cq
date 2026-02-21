//go:build !(c5_hub && c3_eventbus)

package main

import (
	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/serve"
)

func registerSSESubscriberServeComponent(mgr *serve.Manager, cfg config.C4Config, eb *serve.EventBusComponent) {
	// SSESubscriber requires both c5_hub and c3_eventbus build tags.
	_, _, _ = mgr, cfg, eb
}
