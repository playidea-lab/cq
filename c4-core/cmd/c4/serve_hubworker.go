//go:build hub

package main

import (
	"fmt"
	"os"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/serve"
)

// registerHubWorkerServeComponent registers the HubWorker component when
// the hub build tag is active and hub.auto_worker is true (or --worker flag).
// The HubWorker continuously claims and executes jobs from the Hub queue.
func registerHubWorkerServeComponent(mgr *serve.Manager, cfg config.C4Config, hubClientAny any) {
	if !serveWorker && !cfg.Hub.AutoWorker {
		return
	}

	hubClient, ok := hubClientAny.(*hub.Client)
	if !ok || hubClient == nil {
		fmt.Fprintln(os.Stderr, "cq serve: hub worker requested but hub client not available")
		return
	}

	hostname, _ := os.Hostname()
	tags := cfg.Hub.WorkerTags
	if len(tags) == 0 {
		tags = []string{"cq-worker"}
	}

	comp := serve.NewHubWorker(hubClient, tags, hostname)
	mgr.Register(comp)
	fmt.Fprintf(os.Stderr, "cq serve: registered hub_worker (tags=%v)\n", tags)
}
