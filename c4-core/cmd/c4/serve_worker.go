//go:build hub

package main

import (
	"fmt"
	"os"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/serve"
)

// registerWorkerServeComponent registers the Worker component when
// the hub build tag is active and worker is enabled.
// Checks: --worker flag, worker.enabled config, hub.auto_worker (legacy compat).
func registerWorkerServeComponent(mgr *serve.Manager, cfg config.C4Config, hubClientAny any) {
	enabled := serveWorker || cfg.Worker.Enabled || cfg.Hub.AutoWorker
	if !enabled {
		return
	}

	hubClient, ok := hubClientAny.(*hub.Client)
	if !ok || hubClient == nil {
		fmt.Fprintln(os.Stderr, "cq serve: worker requested but hub client not available")
		return
	}

	hostname, _ := os.Hostname()

	// Tags: worker.tags > hub.worker_tags > default
	tags := cfg.Worker.Tags
	if len(tags) == 0 {
		tags = cfg.Hub.WorkerTags
	}
	if len(tags) == 0 {
		tags = []string{"cq-worker"}
	}

	comp := serve.NewWorker(hubClient, tags, hostname)
	mgr.Register(comp)
	fmt.Fprintf(os.Stderr, "cq serve: registered worker (tags=%v)\n", tags)
}
