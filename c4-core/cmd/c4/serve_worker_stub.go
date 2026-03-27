//go:build !hub

package main

import (
	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/serve"
)

func registerWorkerServeComponent(mgr *serve.Manager, cfg config.C4Config, hubClientAny any) {
	_, _, _ = mgr, cfg, hubClientAny
}
