//go:build !(hub && c3_eventbus)

package main

import (
	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/serve"
)

func registerSSESubscriberServeComponent(mgr *serve.Manager, cfg config.C4Config, eb *serve.EventBusComponent, wakeCh chan struct{}) {
	// SSESubscriber requires both hub and c3_eventbus build tags.
	_, _, _, _ = mgr, cfg, eb, wakeCh
}
