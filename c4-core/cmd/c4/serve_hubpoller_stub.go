//go:build !c5_hub

package main

import (
	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/serve"
)

// hubClientStub is a placeholder type when c5_hub build tag is inactive.
type hubClientStub = any

func registerHubPollerServeComponent(mgr *serve.Manager, cfg config.C4Config, eb *serve.EventBusComponent, prebuiltClient hubClientStub) {
	_, _, _, _ = mgr, cfg, eb, prebuiltClient
}
